package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/hooks"
	"github.com/nibzard/looper-go/internal/logging"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
)

// Loop manages the iteration flow and state transitions.
type Loop struct {
	cfg              *config.Config
	promptStore      *prompts.Store
	renderer         *prompts.Renderer
	todoFile         *todo.File
	todoPath         string
	schemaPath       string
	summarySchemaPath string
	runLogger        *logging.RunLogger
	logWriter        agents.LogWriter
	workDir          string
}

// New creates a new loop instance.
func New(cfg *config.Config, workDir string) (*Loop, error) {
	// Create prompt store
	promptDir := cfg.PromptDir
	if !config.PromptDevModeEnabled() {
		promptDir = ""
	}
	promptStore := prompts.NewStore(workDir, promptDir)

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

	// Resolve summary schema path (from prompt store)
	summarySchemaPath := filepath.Join(promptStore.Dir(), prompts.SummarySchema)

	// Load or bootstrap todo file and validate/repair if needed.
	todoFile, err := loadAndValidateTodo(workDir, todoPath, schemaPath, promptStore, cfg)
	if err != nil {
		return nil, fmt.Errorf("load todo file: %w", err)
	}

	// Initialize run logger
	runLogger, err := logging.NewRunLogger(cfg.LogDir, workDir)
	if err != nil {
		return nil, fmt.Errorf("init run logger: %w", err)
	}
	logWriter := agents.NewIOStreamLogWriter(runLogger.Writer())

	return &Loop{
		cfg:              cfg,
		promptStore:      promptStore,
		renderer:         prompts.NewRenderer(promptStore),
		todoFile:         todoFile,
		todoPath:         todoPath,
		schemaPath:       schemaPath,
		summarySchemaPath: summarySchemaPath,
		runLogger:        runLogger,
		logWriter:        logWriter,
		workDir:          workDir,
	}, nil
}

// ensureSchemaExists ensures the schema file exists, creating it if necessary.
// The schema is read from the bundled prompt assets and written to the project root.
func ensureSchemaExists(schemaPath string) error {
	// Check if schema already exists
	if info, err := os.Stat(schemaPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("schema path is a directory: %s", schemaPath)
		}
		if err := ensureSchemaHasSourceFiles(schemaPath); err != nil {
			return fmt.Errorf("ensure schema fields: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat schema file: %w", err)
	}

	// Ensure directory exists
	schemaDir := filepath.Dir(schemaPath)
	if schemaDir != "" && schemaDir != "." {
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			return fmt.Errorf("create schema directory: %w", err)
		}
	}

	// Read the bundled schema from prompts
	// The schema is embedded in the binary at build time
	bundledSchema, err := prompts.BundledSchema()
	if err != nil {
		return fmt.Errorf("get bundled schema: %w", err)
	}

	// Write the schema to the project root
	if err := os.WriteFile(schemaPath, bundledSchema, 0644); err != nil {
		return fmt.Errorf("write schema file: %w", err)
	}

	return nil
}

func ensureSchemaHasSourceFiles(schemaPath string) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema file: %w", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil
	}

	changed := false

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		properties = map[string]interface{}{}
		schema["properties"] = properties
		changed = true
	}
	if _, ok := properties["source_files"]; !ok {
		properties["source_files"] = map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		}
		changed = true
	}

	requiredSet := map[string]bool{}
	requiredList := []string{}
	switch raw := schema["required"].(type) {
	case []interface{}:
		for _, item := range raw {
			s, ok := item.(string)
			if !ok || requiredSet[s] {
				continue
			}
			requiredSet[s] = true
			requiredList = append(requiredList, s)
		}
	case []string:
		for _, s := range raw {
			if requiredSet[s] {
				continue
			}
			requiredSet[s] = true
			requiredList = append(requiredList, s)
		}
	}

	if !requiredSet["source_files"] {
		requiredList = append(requiredList, "source_files")
		schema["required"] = requiredList
		changed = true
	}

	if !changed {
		return nil
	}

	output, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema file: %w", err)
	}
	output = append(output, '\n')
	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return fmt.Errorf("write schema file: %w", err)
	}

	return nil
}

func loadAndValidateTodo(workDir, todoPath, schemaPath string, promptStore *prompts.Store, cfg *config.Config) (*todo.File, error) {
	// Ensure schema file exists
	if err := ensureSchemaExists(schemaPath); err != nil {
		return nil, fmt.Errorf("ensure schema exists: %w", err)
	}

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
	promptData := prompts.NewDataForBootstrap(
		todoPath,
		schemaPath,
		workDir,
		cfg.UserPrompt,
		time.Now(),
	)
	prompt, err := renderer.Render(prompts.BootstrapPrompt, promptData)
	if err != nil {
		return fmt.Errorf("render bootstrap prompt: %w", err)
	}

	// Create bootstrap agent using the configured bootstrap agent
	bootstrapAgentType := agents.AgentType(cfg.GetBootstrapAgent())
	agentCfg := agents.Config{
		Binary: cfg.GetAgentBinary(string(bootstrapAgentType)),
		Model:  cfg.GetAgentModel(string(bootstrapAgentType)),
		WorkDir: workDir,
	}
	agent, err := agents.NewAgent(bootstrapAgentType, agentCfg)
	if err != nil {
		return fmt.Errorf("create bootstrap agent: %w", err)
	}

	// Log to stderr for bootstrap
	logWriter := agents.NewIOStreamLogWriter(os.Stderr)

	_, err = agent.Run(ctx, prompt, logWriter)
	if err != nil && !errors.Is(err, agents.ErrSummaryMissing) {
		return fmt.Errorf("run bootstrap agent: %w", err)
	}

	info, err := os.Stat(todoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("todo file was not created at %s", todoPath)
		}
		return fmt.Errorf("stat todo file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("todo path is a directory: %s", todoPath)
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

	// Build error message for prompt
	var errMsg string
	if len(validationResult.Errors) > 0 {
		errParts := make([]string, 0, len(validationResult.Errors))
		for _, e := range validationResult.Errors {
			errParts = append(errParts, e.Error())
		}
		errMsg = strings.Join(errParts, "\n")
	}

	renderer := prompts.NewRenderer(promptStore)
	promptData := prompts.NewDataForRepair(
		todoPath,
		schemaPath,
		workDir,
		errMsg,
		time.Now(),
	)
	prompt, err := renderer.Render(prompts.RepairPrompt, promptData)
	if err != nil {
		return nil, fmt.Errorf("render repair prompt: %w", err)
	}

	// Create repair agent using the configured repair agent
	repairAgentType := agents.AgentType(cfg.RepairAgent)
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
	if err != nil && !errors.Is(err, agents.ErrSummaryMissing) {
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
	if l.runLogger != nil {
		defer l.runLogger.Close()
	}
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
	label := iterationLabel(iter)

	// Mark task as doing
	if err := l.todoFile.SetTaskDoing(taskID); err != nil {
		return fmt.Errorf("mark task doing: %w", err)
	}
	if err := l.saveTodo(); err != nil {
		return fmt.Errorf("save todo file: %w", err)
	}

	// Determine agent type
	agentType := l.cfg.IterSchedule(iter)

	logWriter := l.logWriter
	if logWriter == nil {
		logWriter = agents.NullLogWriter{}
	}

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

	// Print prompt if dev mode is enabled
	if l.cfg.PrintPrompt && config.PromptDevModeEnabled() {
		fmt.Fprintf(os.Stdout, "\n=== Iteration %d Prompt ===\n%s\n=== End Prompt ===\n\n", iter, prompt)
	}

	// Run agent
	agentCfg := agents.Config{
		Binary:          l.cfg.GetAgentBinary(agentType),
		Model:           l.cfg.GetAgentModel(agentType),
		WorkDir:         l.workDir,
		LastMessagePath: l.lastMessagePath(label),
	}
	agent, err := agents.NewAgent(agents.AgentType(agentType), agentCfg)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	summary, err := agent.Run(ctx, prompt, multiLogWriter)
	l.runHook(ctx, label, multiLogWriter)
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
	logWriter := l.logWriter
	if logWriter == nil {
		logWriter = agents.NullLogWriter{}
	}

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

	// Print prompt if dev mode is enabled
	if l.cfg.PrintPrompt && config.PromptDevModeEnabled() {
		fmt.Fprintf(os.Stdout, "\n=== Review Prompt ===\n%s\n=== End Prompt ===\n\n", prompt)
	}

	// Run review agent using configured review agent
	reviewAgentType := l.cfg.GetReviewAgent()
	label := reviewLabel(iter)
	agentCfg := agents.Config{
		Binary:          l.cfg.GetAgentBinary(reviewAgentType),
		Model:           l.cfg.GetAgentModel(reviewAgentType),
		WorkDir:         l.workDir,
		LastMessagePath: l.lastMessagePath(label),
	}
	agent, err := agents.NewAgent(agents.AgentType(reviewAgentType), agentCfg)
	if err != nil {
		return fmt.Errorf("create review agent: %w", err)
	}

	summary, err := agent.Run(ctx, prompt, multiLogWriter)
	l.runHook(ctx, label, multiLogWriter)
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

	// Validate summary against schema before applying
	if err := agents.ValidateSummary(summary, l.summarySchemaPath); err != nil {
		return fmt.Errorf("summary validation failed: %w", err)
	}

	if summary.Status == "skipped" {
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

func (l *Loop) runHook(ctx context.Context, label string, logWriter agents.LogWriter) {
	if l.cfg.HookCommand == "" {
		return
	}
	if logWriter == nil {
		logWriter = agents.NullLogWriter{}
	}
	result, err := hooks.Invoke(ctx, hooks.Options{
		Command:         l.cfg.HookCommand,
		LastMessagePath: l.lastMessagePath(label),
		Label:           label,
		WorkDir:         l.workDir,
	})
	if result.Ran {
		_ = logWriter.Write(agents.LogEvent{
			Type:      "command",
			Timestamp: time.Now().UTC(),
			Command:   result.Command,
			ExitCode:  result.ExitCode,
		})
	}
	if err != nil {
		_ = logWriter.Write(agents.LogEvent{
			Type:      "error",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("hook: %v", err),
		})
	}
}

func iterationLabel(iter int) string {
	return fmt.Sprintf("iter-%d", iter)
}

func reviewLabel(iter int) string {
	return fmt.Sprintf("review-%d", iter)
}

// lastMessagePath returns the path for a last message file.
func (l *Loop) lastMessagePath(label string) string {
	if l.runLogger == nil {
		return ""
	}
	return l.runLogger.LastMessagePath(label)
}

// Status represents a status update for TUI monitoring.
type Status struct {
	Iteration int
	TaskID    string
	Status    string
	Message   string
	Error     error
}

// RunWithStatus executes the main loop and sends status updates to the channel.
func (l *Loop) RunWithStatus(ctx context.Context, statusCh chan<- Status) error {
	defer close(statusCh)

	if l.runLogger != nil {
		defer l.runLogger.Close()
	}

	for i := 1; i <= l.cfg.MaxIterations; i++ {
		// Check context
		if ctx.Err() != nil {
			statusCh <- Status{Error: ctx.Err()}
			return ctx.Err()
		}

		// Select task
		task := l.todoFile.SelectTask()
		if task == nil {
			// No tasks found - run review pass
			statusCh <- Status{Iteration: i, Status: "review", Message: "Running review pass..."}
			if err := l.runReview(ctx, i); err != nil {
				statusCh <- Status{Iteration: i, Error: err}
				return fmt.Errorf("review pass: %w", err)
			}
			// Check if any tasks were added
			task = l.todoFile.SelectTask()
			if task == nil {
				// Still no tasks - add project-done marker
				statusCh <- Status{Iteration: i, Status: "done", Message: "All tasks complete"}
				l.addProjectDoneMarker()
				break
			}
			continue
		}

		// Send status update
		statusCh <- Status{Iteration: i, TaskID: task.ID, Status: "starting", Message: "Starting: " + task.Title}

		// Run iteration
		if err := l.runIteration(ctx, i, task); err != nil {
			statusCh <- Status{Iteration: i, TaskID: task.ID, Error: err}
			return fmt.Errorf("iteration %d: %w", i, err)
		}

		statusCh <- Status{Iteration: i, TaskID: task.ID, Status: "complete", Message: "Complete: " + task.Title}

		// Delay between iterations
		if l.cfg.LoopDelaySeconds > 0 {
			select {
			case <-ctx.Done():
				statusCh <- Status{Error: ctx.Err()}
				return ctx.Err()
			case <-time.After(time.Duration(l.cfg.LoopDelaySeconds) * time.Second):
			}
		}
	}

	return nil
}
