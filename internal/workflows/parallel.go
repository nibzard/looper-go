package workflows

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/logging"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
)

const (
	// WorkflowTypeParallel is the parallel execution workflow.
	WorkflowTypeParallel WorkflowType = "parallel"
)

func init() {
	Register(WorkflowTypeParallel, NewParallelLoopFactory())
}

// NewParallelLoopFactory creates a factory for the parallel workflow.
func NewParallelLoopFactory() WorkflowFactory {
	return func(cfg interface{}, workDir string, todoFile interface{}) (Workflow, error) {
		// Handle nil config for description purposes
		var cfgTyped *config.Config
		if cfg != nil {
			var ok bool
			cfgTyped, ok = cfg.(*config.Config)
			if !ok {
				return nil, fmt.Errorf("expected *config.Config, got %T", cfg)
			}
		}

		settings := getConfigSettings(cfgTyped, WorkflowTypeParallel)

		return &ParallelLoop{
			cfg:           cfgTyped,
			workDir:       workDir,
			maxConcurrent: GetInt(settings, "max_concurrent", 3),
			failFast:      GetBool(settings, "fail_fast", false),
		}, nil
	}
}

// ParallelLoop runs tasks concurrently with a semaphore for limiting concurrency.
type ParallelLoop struct {
	cfg           *config.Config
	workDir       string
	maxConcurrent int
	failFast      bool
	todoFile      *todo.File
}

// Run executes tasks in parallel with limited concurrency.
func (p *ParallelLoop) Run(ctx context.Context) error {
	// Load todo file
	todoPath := p.cfg.TodoFile
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(p.workDir, todoPath)
	}

	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return fmt.Errorf("load todo file: %w", err)
	}
	p.todoFile = todoFile

	// Get all pending tasks
	taskIDs := p.getPendingTaskIDs()
	if len(taskIDs) == 0 {
		fmt.Println("No tasks to run.")
		return nil
	}

	fmt.Printf("Running %d tasks with max concurrency of %d\n", len(taskIDs), p.maxConcurrent)

	// Create semaphore for concurrency limiting
	sem := make(chan struct{}, p.maxConcurrent)
	errCh := make(chan error, len(taskIDs))
	var wg sync.WaitGroup

	// Process each task
	for _, taskID := range taskIDs {
		wg.Add(1)
		go func(tid string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Run the task
			if err := p.runTaskByID(ctx, tid); err != nil {
				errCh <- fmt.Errorf("task %s: %w", tid, err)
			}
		}(taskID)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Collect errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
		if p.failFast {
			return fmt.Errorf("task failed (fail_fast=true): %w", err)
		}
	}

	// Report any errors
	if len(errors) > 0 {
		fmt.Printf("\nCompleted with %d errors:\n", len(errors))
		for _, err := range errors {
			fmt.Printf("  - %v\n", err)
		}
	}

	return nil
}

// Description returns a description of the parallel workflow.
func (p *ParallelLoop) Description() string {
	return "Concurrent task execution with configurable limits"
}

// getPendingTaskIDs returns IDs of all pending tasks.
func (p *ParallelLoop) getPendingTaskIDs() []string {
	var taskIDs []string
	for _, task := range p.todoFile.Tasks {
		if task.Status == todo.StatusTodo {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	return taskIDs
}

// runTaskByID executes a single task by ID with an agent.
func (p *ParallelLoop) runTaskByID(ctx context.Context, taskID string) error {
	// Get task from todo file
	task := p.todoFile.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	fmt.Printf("[%s] Starting: %s\n", task.ID, task.Title)

	// Determine agent type
	agentType := p.cfg.IterSchedule(1)

	// Build agent config
	agentCfg := agents.Config{
		Binary: p.cfg.GetAgentBinary(agentType),
		Model:  p.cfg.GetAgentModel(agentType),
		WorkDir: p.workDir,
	}

	// Create agent
	agent, err := agents.NewAgent(agents.AgentType(agentType), agentCfg)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	// Build prompt
	prompt := p.buildPrompt(task)

	// Create log writer
	runLogger, err := logging.NewRunLogger(p.cfg.LogDir, p.workDir)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer runLogger.Close()
	logWriter := agents.NewIOStreamLogWriter(runLogger.Writer())

	// Run agent
	summary, err := agent.Run(ctx, prompt, logWriter)
	if err != nil {
		return err
	}

	fmt.Printf("[%s] Completed\n", task.ID)
	if summary.Summary != "" {
		fmt.Printf("  %s\n", summary.Summary)
	}

	return nil
}

// buildPrompt creates a prompt for a task.
func (p *ParallelLoop) buildPrompt(task *todo.Task) string {
	promptStore := prompts.NewStore(p.workDir, "")
	renderer := prompts.NewRenderer(promptStore)

	data := prompts.NewData(
		p.cfg.TodoFile,
		p.cfg.SchemaFile,
		p.workDir,
		prompts.Task{
			ID:     task.ID,
			Title:  task.Title,
			Status: string(task.Status),
		},
		1,
		p.cfg.IterSchedule(1),
		time.Now(),
	)

	prompt, _ := renderer.Render(prompts.IterationPrompt, data)
	return prompt
}
