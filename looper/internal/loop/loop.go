package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nibzard/looper/internal/agents"
	"github.com/nibzard/looper/internal/config"
	"github.com/nibzard/looper/internal/prompts"
	"github.com/nibzard/looper/internal/todo"
)

const repairAgentType = agents.AgentTypeCodex

// Loop manages the iteration flow and state transitions.
type Loop struct {
	cfg         *config.Config
	promptStore *prompts.Store
	renderer    *prompts.Renderer
	todoFile    *todo.File
	todoPath    string
	schemaPath  string
	logDir      string
	workDir     string
}

// New creates a new loop instance.
func New(cfg *config.Config, workDir string) (*Loop, error) {
	// Create prompt store
	promptStore := prompts.NewStore(workDir, cfg.PromptDir)

	// Resolve todo file path
	todoPath := cfg.TodoFile
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(workDir, todoPath)
	}

	// Resolve schema path
	schemaPath := cfg.SchemaFile
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(workDir, schemaPath)
	}

	// Load or bootstrap todo file and validate/repair if needed.
	todoFile, err := loadAndValidateTodo(workDir, todoPath, schemaPath, promptStore, cfg)
	if err != nil {
		return nil, fmt.Errorf("load todo file: %w", err)
	}

	// Create log directory
	logDir := cfg.LogDir
	if !filepath.IsAbs(logDir) {
		logDir = filepath.Join(workDir, logDir)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	return &Loop{
		cfg:         cfg,
		promptStore: promptStore,
		renderer:    prompts.NewRenderer(promptStore),
		todoFile:    todoFile,
		todoPath:    todoPath,
		schemaPath:  schemaPath,
		logDir:      logDir,
		workDir:     workDir,
	}, nil
}

func loadAndValidateTodo(workDir, todoPath, schemaPath string, promptStore *prompts.Store, cfg *config.Config) (*todo.File, error) {
	// Load or bootstrap todo file
	todoFile, err := loadOrBootstrapTodo(workDir, todoPath, schemaPath, promptStore, cfg)
	if err != nil {
		return nil, err
	}

	// Validate todo file
	result := todoFile.Validate(todo.ValidationOptions{
		SchemaPath: schemaPath,
	})
	if !result.Valid {
		// Try to repair the file
		todoFile, err = repairTodoFile(workDir, todoPath, schemaPath, promptStore, cfg, result)
		if err != nil {
			return nil, fmt.Errorf("todo file validation failed and repair failed: %w (validation errors: %v)", err, result.Errors)
		}
	}

	return todoFile, nil
}

// loadOrBootstrapTodo loads the todo file, or bootstraps it if missing.
func loadOrBootstrapTodo(workDir, todoPath, schemaPath string, promptStore *prompts.Store, cfg *config.Config) (*todo.File, error) {
	// Try to load existing file
	todoFile, err := todo.Load(todoPath)
	if err == nil {
		return todoFile, nil
	}

	// File doesn't exist - bootstrap it
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Todo file not found at %s, bootstrapping...\n", todoPath)
		if err := bootstrapTodo(workDir, todoPath, schemaPath, promptStore, cfg); err != nil {
			return nil, fmt.Errorf("bootstrap todo file: %w", err)
		}
		// Load the newly created file
		return todo.Load(todoPath)
	}

	// Attempt repair for load errors (e.g., invalid JSON)
	fmt.Fprintf(os.Stderr, "Todo file failed to load, attempting repair...\n")
	validationResult := &todo.ValidationResult{
		Valid:  false,
		Errors: []error{err},
	}
	todoFile, repairErr := repairTodoFile(workDir, todoPath, schemaPath, promptStore, cfg, validationResult)
	if repairErr != nil {
		return nil, fmt.Errorf("load todo file: %w; repair failed: %v", err, repairErr)
	}

	return todoFile, nil
}

// bootstrapTodo creates a new todo file by running the bootstrap agent.
func bootstrapTodo(workDir, todoPath, schemaPath string, promptStore *prompts.Store, cfg *config.Config) error {
	ctx := context.Background()

	renderer := prompts.NewRenderer(promptStore)
	promptData := prompts.NewData(
		todoPath,
		schemaPath,
		workDir,
		prompts.Task{},
		0,
		"bootstrap",
		time.Now(),
	)
	prompt, err := renderer.Render(prompts.BootstrapPrompt, promptData)
	if err != nil {
		return fmt.Errorf("render bootstrap prompt: %w", err)
	}

	// Create bootstrap agent
	agentCfg := agents.Config{
		Binary: cfg.GetAgentBinary(string(repairAgentType)),
		Model:  cfg.GetAgentModel(string(repairAgentType)),
		WorkDir: workDir,
	}
	agent, err := agents.NewAgent(repairAgentType, agentCfg)
	if err != nil {
		return fmt.Errorf("create bootstrap agent: %w", err)
	}

	// Log to stderr for bootstrap
	logWriter := agents.NewIOStreamLogWriter(os.Stderr)

	_, err = agent.Run(ctx, prompt, logWriter)
	if err != nil {
		return fmt.Errorf("run bootstrap agent: %w", err)
	}

	return nil
}

// repairTodoFile repairs an invalid todo file by running the repair agent.
func repairTodoFile(workDir, todoPath, schemaPath string, promptStore *prompts.Store, cfg *config.Config, validationResult *todo.ValidationResult) (*todo.File, error) {
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Todo file validation failed, attempting repair...\n")
	for _, e := range validationResult.Errors {
		fmt.Fprintf(os.Stderr, "  - %v\n", e)
	}

	renderer := prompts.NewRenderer(promptStore)
	promptData := prompts.NewData(
		todoPath,
		schemaPath,
		workDir,
		prompts.Task{},
		0,
		"repair",
		time.Now(),
	)
	prompt, err := renderer.Render(prompts.RepairPrompt, promptData)
	if err != nil {
		return nil, fmt.Errorf("render repair prompt: %w", err)
	}

	// Create repair agent
	agentCfg := agents.Config{
		Binary: cfg.GetAgentBinary(string(repairAgentType)),
		Model:  cfg.GetAgentModel(string(repairAgentType)),
		WorkDir: workDir,
	}
	agent, err := agents.NewAgent(repairAgentType, agentCfg)
	if err != nil {
		return nil, fmt.Errorf("create repair agent: %w", err)
	}

	// Log to stderr for repair
	logWriter := agents.NewIOStreamLogWriter(os.Stderr)

	_, err = agent.Run(ctx, prompt, logWriter)
	if err != nil {
		return nil, fmt.Errorf("run repair agent: %w", err)
	}

	// Reload the repaired file
	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return nil, fmt.Errorf("load repaired todo file: %w", err)
	}

	// Validate the repaired file
	result := todoFile.Validate(todo.ValidationOptions{
		SchemaPath: schemaPath,
	})
	if !result.Valid {
		return nil, fmt.Errorf("repaired file still invalid: %w", result.Errors[0])
	}

	fmt.Fprintf(os.Stderr, "Todo file repaired successfully.\n")
	return todoFile, nil
}

// Run executes the main loop.
func (l *Loop) Run(ctx context.Context) error {
	for i := 1; i <= l.cfg.MaxIterations; i++ {
		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Select task
		task := l.todoFile.SelectTask()
		if task == nil {
			// No tasks found - run review pass
			if err := l.runReview(ctx, i); err != nil {
				return fmt.Errorf("review pass: %w", err)
			}
			// Check if any tasks were added
			task = l.todoFile.SelectTask()
			if task == nil {
				// Still no tasks - add project-done marker
				l.addProjectDoneMarker()
				break
			}
			continue
		}

		// Run iteration
		if err := l.runIteration(ctx, i, task); err != nil {
			return fmt.Errorf("iteration %d: %w", i, err)
		}

		// Delay between iterations
		if l.cfg.LoopDelaySeconds > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(l.cfg.LoopDelaySeconds) * time.Second):
			}
		}
	}

	return nil
}

// runIteration executes a single iteration for a task.
func (l *Loop) runIteration(ctx context.Context, iter int, task *todo.Task) error {
	taskID := task.ID
	taskTitle := task.Title
	taskStatus := string(task.Status)

	// Mark task as doing
	if err := l.todoFile.SetTaskDoing(taskID); err != nil {
		return fmt.Errorf("mark task doing: %w", err)
	}
	if err := l.saveTodo(); err != nil {
		return fmt.Errorf("save todo file: %w", err)
	}

	// Determine agent type
	agentType := l.cfg.IterSchedule(iter)

	// Create log file
	logPath := l.logPath(iter, taskID)
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()
	logWriter := agents.NewIOStreamLogWriter(logFile)

	// Also log to stdout if needed
	var multiLogWriter agents.LogWriter = logWriter
	if os.Getenv("LOOPER_QUIET") == "" {
		stdoutWriter := agents.NewIOStreamLogWriter(os.Stdout)
		stdoutWriter.SetIndent("  ")
		multiLogWriter = agents.NewMultiLogWriter(logWriter, stdoutWriter)
	}

	// Render prompt
	promptData := prompts.NewData(
		l.todoPath,
		l.schemaPath,
		l.workDir,
		prompts.Task{
			ID:     taskID,
			Title:  taskTitle,
			Status: taskStatus,
		},
		iter,
		agentType,
		time.Now(),
	)
	prompt, err := l.renderer.Render(prompts.IterationPrompt, promptData)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	// Run agent
	agentCfg := agents.Config{
		Binary:        l.cfg.GetAgentBinary(agentType),
		Model:         l.cfg.GetAgentModel(agentType),
		WorkDir:       l.workDir,
		LastMessagePath: l.lastMessagePath(iter, taskID),
	}
	agent, err := agents.NewAgent(agents.AgentType(agentType), agentCfg)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	summary, err := agent.Run(ctx, prompt, multiLogWriter)
	if err != nil {
		// Mark task as blocked on error
		_ = l.todoFile.SetTaskStatus(taskID, todo.StatusBlocked)
		_ = l.saveTodo()
		return fmt.Errorf("run agent: %w", err)
	}

	if err := l.reloadTodo(); err != nil {
		_ = multiLogWriter.Write(agents.LogEvent{
			Type:      "error",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("reload todo file: %v", err),
		})
		return fmt.Errorf("reload todo file: %w", err)
	}

	// Validate summary matches selected task
	if summary.TaskID != "" && summary.TaskID != taskID {
		// Warn and skip summary apply
		_ = multiLogWriter.Write(agents.LogEvent{
			Type:      "error",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("summary task_id %q does not match selected task %q, skipping summary apply", summary.TaskID, taskID),
		})
		return nil
	}

	// Apply summary
	if l.cfg.ApplySummary && summary != nil {
		if err := l.applySummary(summary); err != nil {
			_ = multiLogWriter.Write(agents.LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("apply summary: %v", err),
			})
			return fmt.Errorf("apply summary: %w", err)
		}
	}

	return nil
}

// runReview executes the review pass.
func (l *Loop) runReview(ctx context.Context, iter int) error {
	// Create log file
	logPath := l.logPath(iter, "review")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()
	logWriter := agents.NewIOStreamLogWriter(logFile)

	var multiLogWriter agents.LogWriter = logWriter
	if os.Getenv("LOOPER_QUIET") == "" {
		stdoutWriter := agents.NewIOStreamLogWriter(os.Stdout)
		stdoutWriter.SetIndent("  ")
		multiLogWriter = agents.NewMultiLogWriter(logWriter, stdoutWriter)
	}

	// Render review prompt
	promptData := prompts.NewData(
		l.todoPath,
		l.schemaPath,
		l.workDir,
		prompts.Task{},
		iter,
		"review",
		time.Now(),
	)
	prompt, err := l.renderer.Render(prompts.ReviewPrompt, promptData)
	if err != nil {
		return fmt.Errorf("render review prompt: %w", err)
	}

	// Run review agent (always codex)
	agentCfg := agents.Config{
		Binary:          l.cfg.GetAgentBinary("codex"),
		Model:           l.cfg.GetAgentModel("codex"),
		WorkDir:         l.workDir,
		LastMessagePath: l.lastMessagePath(iter, "review"),
	}
	agent := agents.NewCodexAgent(agentCfg)

	summary, err := agent.Run(ctx, prompt, multiLogWriter)
	if err != nil {
		return fmt.Errorf("run review agent: %w", err)
	}

	if err := l.reloadTodo(); err != nil {
		_ = multiLogWriter.Write(agents.LogEvent{
			Type:      "error",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("reload todo file: %v", err),
		})
		return fmt.Errorf("reload todo file: %w", err)
	}

	// Apply summary if it adds tasks
	if l.cfg.ApplySummary && summary != nil {
		if err := l.applySummary(summary); err != nil {
			return fmt.Errorf("apply review summary: %w", err)
		}
	}

	return nil
}

// applySummary applies a summary to the todo file.
func (l *Loop) applySummary(summary *agents.Summary) error {
	if summary.TaskID == "" {
		return nil
	}

	// Map status
	var status todo.Status
	switch summary.Status {
	case "done":
		status = todo.StatusDone
	case "blocked":
		status = todo.StatusBlocked
	case "doing":
		status = todo.StatusDoing
	default:
		status = todo.StatusTodo
	}

	// Update existing task or add new one
	task := l.todoFile.GetTask(summary.TaskID)
	if task != nil {
		// Update existing task
		if err := l.todoFile.SetTaskStatus(summary.TaskID, status); err != nil {
			return fmt.Errorf("set task status: %w", err)
		}
		if summary.Summary != "" || len(summary.Files) > 0 || len(summary.Blockers) > 0 {
			if err := l.todoFile.UpdateTask(summary.TaskID, func(t *todo.Task) {
				if summary.Summary != "" {
					t.Details = summary.Summary
				}
				if len(summary.Files) > 0 {
					t.Files = summary.Files
				}
				if len(summary.Blockers) > 0 {
					t.Blockers = summary.Blockers
				}
			}); err != nil {
				return fmt.Errorf("update task: %w", err)
			}
		}
	} else {
		// Add new task
		l.todoFile.AddTask(todo.Task{
			ID:       summary.TaskID,
			Title:    summary.Summary,
			Priority: 2,
			Status:   status,
			Details:  summary.Summary,
			Files:    summary.Files,
			Blockers: summary.Blockers,
		})
	}

	return l.saveTodo()
}

// addProjectDoneMarker adds a project-done task to indicate completion.
func (l *Loop) addProjectDoneMarker() {
	if l.hasProjectDoneMarker() {
		return
	}
	l.todoFile.AddTask(todo.Task{
		ID:       "PROJECT-DONE",
		Title:    "Project done: no new tasks",
		Priority: 5,
		Status:   todo.StatusDone,
		Details:  "Review found no new tasks.",
		Tags:     []string{"project-done"},
	})
	_ = l.saveTodo()
}

// hasProjectDoneMarker returns true if a project-done task already exists.
func (l *Loop) hasProjectDoneMarker() bool {
	for i := range l.todoFile.Tasks {
		task := l.todoFile.Tasks[i]
		if strings.EqualFold(task.ID, "project-done") {
			return true
		}
		for _, tag := range task.Tags {
			if strings.EqualFold(tag, "project-done") {
				return true
			}
		}
	}
	return false
}

// saveTodo saves the todo file.
func (l *Loop) saveTodo() error {
	return l.todoFile.Save(l.todoPath)
}

// reloadTodo reloads the todo file and applies validation/repair.
func (l *Loop) reloadTodo() error {
	todoFile, err := loadAndValidateTodo(l.workDir, l.todoPath, l.schemaPath, l.promptStore, l.cfg)
	if err != nil {
		return err
	}
	l.todoFile = todoFile
	return nil
}

// logPath returns the path for a log file.
func (l *Loop) logPath(iter int, label string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	pid := os.Getpid()
	return filepath.Join(l.logDir, fmt.Sprintf("%s-%d-%s.jsonl", timestamp, pid, label))
}

// lastMessagePath returns the path for a last message file.
func (l *Loop) lastMessagePath(iter int, label string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	pid := os.Getpid()
	return filepath.Join(l.logDir, fmt.Sprintf("%s-%d-%s.last.json", timestamp, pid, label))
}
