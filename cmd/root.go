// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/logging"
	"github.com/nibzard/looper-go/internal/loop"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
	"github.com/nibzard/looper-go/internal/ui"
)

// Version is set via ldflags at build time.
var Version = "dev"

const (
	// Default max iterations for the run command
	defaultMaxIterations = 50
)

// Run executes the looper CLI.
func Run(ctx context.Context, args []string) error {
	// Create a flag set for global options
	fs := flag.NewFlagSet("looper", flag.ContinueOnError)
	fs.Usage = func() {
		printUsage(fs, os.Stderr)
	}
	help := fs.Bool("help", false, "Show help")
	fs.BoolVar(help, "h", false, "Show help")
	showVersion := fs.Bool("version", false, "Show version")
	fs.BoolVar(showVersion, "v", false, "Show version")

	// Global flags
	cfg, err := config.Load(fs, args)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if *help {
		printUsage(fs, os.Stdout)
		return nil
	}
	if *showVersion {
		return versionCommand()
	}

	// Determine the subcommand
	// If no args or first arg is a flag, use "run" as default
	subcommand := "run"
	remainingArgs := fs.Args()
	if len(remainingArgs) > 0 {
		// Check if it looks like a subcommand (doesn't start with -)
		if !strings.HasPrefix(remainingArgs[0], "-") {
			subcommand = remainingArgs[0]
			remainingArgs = remainingArgs[1:]
		}
	}

	// Execute the subcommand
	switch subcommand {
	case "run":
		return runCommand(ctx, cfg, remainingArgs)
	case "tui":
		return tuiCommand(ctx, cfg, remainingArgs)
	case "doctor":
		return doctorCommand(cfg, remainingArgs)
	case "tail":
		return tailCommand(cfg, remainingArgs)
	case "ls":
		return lsCommand(cfg, remainingArgs)
	case "version", "--version", "-v":
		return versionCommand()
	case "help", "--help", "-h":
		printUsage(fs, os.Stdout)
		return nil
	default:
		// If it's not a recognized command, it might be a file path for run
		// Check if it's an existing file
		if fi, err := os.Stat(subcommand); err == nil && !fi.IsDir() {
			// Use this as the todo file path
			cfg.TodoFile = subcommand
			return runCommand(ctx, cfg, remainingArgs)
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcommand)
		printUsage(fs, os.Stderr)
		return fmt.Errorf("unknown command: %s", subcommand)
	}
}

// runCommand executes the main loop.
func runCommand(ctx context.Context, cfg *config.Config, args []string) error {
	// Parse any additional flags for the run command
	fs := flag.NewFlagSet("looper run", flag.ContinueOnError)
	devMode := config.PromptDevModeEnabled()
	maxIter := fs.Int("max-iterations", cfg.MaxIterations, "Maximum iterations")
	schedule := fs.String("schedule", cfg.Schedule, "Iteration schedule (codex|claude|odd-even|round-robin)")
	uiMode := fs.String("ui", "", "UI mode (tui for terminal UI)")
	promptArg := fs.String("prompt", "", "User prompt to drive bootstrap (skips markdown scanning)")
	var promptDir *string
	var printPrompt *bool
	if devMode {
		promptDir = fs.String("prompt-dir", cfg.PromptDir, "Prompt directory override (dev only, requires LOOPER_PROMPT_MODE=dev)")
		printPrompt = fs.Bool("print-prompt", cfg.PrintPrompt, "Print rendered prompts before running (dev only, requires LOOPER_PROMPT_MODE=dev)")
	} else {
		promptDir = fs.String("prompt-dir", "", "")
		printPrompt = fs.Bool("print-prompt", false, "")
	}
	oddAgent := fs.String("odd-agent", cfg.OddAgent, "Agent for odd iterations in odd-even schedule (codex|claude)")
	evenAgent := fs.String("even-agent", cfg.EvenAgent, "Agent for even iterations in odd-even schedule (codex|claude)")
	var rrAgentsStr string
	if cfg.RRAgents != nil {
		rrAgentsStr = strings.Join(cfg.RRAgents, ",")
	}
	fs.StringVar(&rrAgentsStr, "rr-agents", rrAgentsStr, "Comma-separated agent list for round-robin schedule (e.g., claude,codex)")
	repairAgent := fs.String("repair-agent", cfg.RepairAgent, "Agent for repair operations (codex|claude)")
	reviewAgent := fs.String("review-agent", cfg.ReviewAgent, "Agent for review pass (codex|claude)")
	bootstrapAgent := fs.String("bootstrap-agent", cfg.BootstrapAgent, "Agent for bootstrap operations (codex|claude)")
	applySummary := fs.Bool("apply-summary", cfg.ApplySummary, "Apply summaries to task file")
	gitInit := fs.Bool("git-init", cfg.GitInit, "Initialize git repo if missing")
	hook := fs.String("hook", cfg.HookCommand, "Hook command to run after each iteration")
	loopDelay := fs.Int("loop-delay", cfg.LoopDelaySeconds, "Delay between iterations (seconds)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		return fmt.Errorf("unexpected arguments: %v", remaining[1:])
	}
	if len(remaining) == 1 {
		cfg.TodoFile = remaining[0]
	}

	// Store user prompt in config for bootstrap
	if *promptArg != "" {
		cfg.UserPrompt = *promptArg
	}

	// Update config with parsed values
	cfg.MaxIterations = *maxIter
	cfg.Schedule = *schedule
	if devMode {
		cfg.PromptDir = *promptDir
		cfg.PrintPrompt = *printPrompt
	} else {
		cfg.PromptDir = ""
		cfg.PrintPrompt = false
	}
	cfg.OddAgent = *oddAgent
	cfg.EvenAgent = *evenAgent
	if rrAgentsStr != "" {
		cfg.RRAgents = splitAndTrim(rrAgentsStr, ",")
	}
	cfg.RepairAgent = *repairAgent
	cfg.ReviewAgent = *reviewAgent
	cfg.BootstrapAgent = *bootstrapAgent
	cfg.ApplySummary = *applySummary
	cfg.GitInit = *gitInit
	cfg.HookCommand = *hook
	cfg.LoopDelaySeconds = *loopDelay

	// Make todo file path absolute
	if !filepath.IsAbs(cfg.TodoFile) {
		cfg.TodoFile = filepath.Join(cfg.ProjectRoot, cfg.TodoFile)
	}

	// Check if TUI mode is requested
	if *uiMode == "tui" {
		return ui.RunTUI(ctx, cfg, cfg.TodoFile, ui.WithRunLoop(true))
	}

	// Create and run loop
	l, err := loop.New(cfg, cfg.ProjectRoot)
	if err != nil {
		return fmt.Errorf("initializing loop: %w", err)
	}

	return l.Run(ctx)
}

// tuiCommand launches the TUI.
func tuiCommand(ctx context.Context, cfg *config.Config, args []string) error {
	// Parse tui-specific flags
	fs := flag.NewFlagSet("looper tui", flag.ContinueOnError)
	runLoop := fs.Bool("run", false, "Run the loop in the background")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		return fmt.Errorf("unexpected arguments: %v", remaining[1:])
	}
	todoPath := cfg.TodoFile
	if len(remaining) == 1 {
		todoPath = remaining[0]
	}
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}

	return ui.RunTUI(ctx, cfg, todoPath, ui.WithRunLoop(*runLoop))
}

// doctorCommand checks dependencies, config, prompts, and task file validity.
func doctorCommand(cfg *config.Config, args []string) error {
	// Parse doctor-specific flags
	fs := flag.NewFlagSet("looper doctor", flag.ContinueOnError)
	verbose := fs.Bool("v", false, "Verbose output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		return fmt.Errorf("unexpected arguments: %v", remaining[1:])
	}
	todoPath := cfg.TodoFile
	if len(remaining) == 1 {
		todoPath = remaining[0]
	}
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}
	schemaPath := cfg.SchemaFile
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(cfg.ProjectRoot, schemaPath)
	}
	promptDir := resolvePromptDir(cfg)

	fmt.Println("Looper Doctor")
	fmt.Println("=============")
	fmt.Println()

	allOK := true

	// Check project root
	fmt.Printf("Project root: %s\n", cfg.ProjectRoot)
	if _, err := os.Stat(cfg.ProjectRoot); err != nil {
		fmt.Printf("  ❌ Error: %v\n", err)
		allOK = false
	} else {
		fmt.Println("  ✅ OK")
	}
	fmt.Println()

	// Check config
	configOK := true
	normalizedSchedule, scheduleOK := normalizeSchedule(cfg.Schedule)
	repairAgent := normalizeAgent(cfg.RepairAgent)
	reviewAgent := normalizeAgent(cfg.ReviewAgent)
	bootstrapAgent := normalizeAgent(cfg.BootstrapAgent)

	fmt.Println("Config:")
	if scheduleOK {
		fmt.Printf("  ✅ Schedule: %s\n", normalizedSchedule)
	} else {
		fmt.Printf("  ❌ Schedule: %s (expected codex|claude|odd-even|round-robin)\n", cfg.Schedule)
		configOK = false
	}
	if repairAgent == "" {
		fmt.Println("  ❌ Repair agent: empty (expected codex|claude)")
		configOK = false
	} else if !isValidAgent(repairAgent) {
		fmt.Printf("  ❌ Repair agent: %s (expected codex|claude)\n", repairAgent)
		configOK = false
	} else {
		fmt.Printf("  ✅ Repair agent: %s\n", repairAgent)
	}
	if reviewAgent == "" {
		fmt.Println("  ✅ Review agent: (default: codex)")
	} else if !isValidAgent(reviewAgent) {
		fmt.Printf("  ❌ Review agent: %s (expected codex|claude)\n", reviewAgent)
		configOK = false
	} else {
		fmt.Printf("  ✅ Review agent: %s\n", reviewAgent)
	}
	if bootstrapAgent == "" {
		fmt.Println("  ✅ Bootstrap agent: (default: codex)")
	} else if !isValidAgent(bootstrapAgent) {
		fmt.Printf("  ❌ Bootstrap agent: %s (expected codex|claude)\n", bootstrapAgent)
		configOK = false
	} else {
		fmt.Printf("  ✅ Bootstrap agent: %s\n", bootstrapAgent)
	}

	switch normalizedSchedule {
	case "odd-even":
		oddAgent, oddOK, oddDefault := normalizeAgentOrDefault(cfg.OddAgent, "codex")
		if oddOK {
			if oddDefault {
				fmt.Printf("  ✅ Odd agent: %s (default)\n", oddAgent)
			} else {
				fmt.Printf("  ✅ Odd agent: %s\n", oddAgent)
			}
		} else {
			fmt.Printf("  ❌ Odd agent: %s (expected codex|claude)\n", cfg.OddAgent)
			configOK = false
		}

		evenAgent, evenOK, evenDefault := normalizeAgentOrDefault(cfg.EvenAgent, "claude")
		if evenOK {
			if evenDefault {
				fmt.Printf("  ✅ Even agent: %s (default)\n", evenAgent)
			} else {
				fmt.Printf("  ✅ Even agent: %s\n", evenAgent)
			}
		} else {
			fmt.Printf("  ❌ Even agent: %s (expected codex|claude)\n", cfg.EvenAgent)
			configOK = false
		}
	case "round-robin":
		rrAgents := normalizeAgentList(cfg.RRAgents)
		invalidAgents := invalidAgentList(cfg.RRAgents)
		if len(invalidAgents) > 0 {
			fmt.Printf("  ❌ Round-robin agents: invalid values: %s\n", strings.Join(invalidAgents, ", "))
			configOK = false
		} else if len(rrAgents) == 0 {
			fmt.Println("  ✅ Round-robin agents: claude,codex (default)")
		} else {
			fmt.Printf("  ✅ Round-robin agents: %s\n", strings.Join(rrAgents, ","))
		}
	}

	if !configOK {
		allOK = false
	}
	fmt.Println()

	// Check dependencies
	needsClaude := false
	if repairAgent == "claude" {
		needsClaude = true
	}
	if reviewAgent == "claude" {
		needsClaude = true
	}
	if bootstrapAgent == "claude" {
		needsClaude = true
	}
	if scheduleOK && scheduleUsesClaude(normalizedSchedule, cfg.OddAgent, cfg.EvenAgent, cfg.RRAgents) {
		needsClaude = true
	}

	fmt.Println("Dependencies:")
	if !checkBinary("codex", cfg.Agents.Codex.Binary, true) {
		allOK = false
	}
	if !checkBinary("claude", cfg.Agents.Claude.Binary, needsClaude) {
		allOK = false
	}
	if cfg.GitInit || *verbose {
		_ = checkBinary("git (optional)", "git", false)
	}
	fmt.Println()

	// Check todo file
	fmt.Printf("Todo file: %s\n", todoPath)
	todoInfo, err := os.Stat(todoPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ⚠️  Not found (will be bootstrapped on run)")
		} else {
			fmt.Printf("  ❌ Error: %v\n", err)
			allOK = false
		}
	} else if todoInfo.IsDir() {
		fmt.Println("  ❌ Error: path is a directory")
		allOK = false
	} else {
		fmt.Println("  ✅ OK")
		// Validate the file
		todoFile, loadErr := todo.Load(todoPath)
		if loadErr != nil {
			fmt.Printf("  ❌ Load error: %v\n", loadErr)
			allOK = false
		} else {
			result := todoFile.Validate(todo.ValidationOptions{SchemaPath: schemaPath})
			for _, w := range result.Warnings {
				fmt.Printf("  ⚠️  %s\n", w)
			}
			if result.Valid {
				fmt.Println("  ✅ Valid")
			} else {
				fmt.Println("  ❌ Validation failed:")
				for _, e := range result.Errors {
					fmt.Printf("     - %v\n", e)
				}
				allOK = false
			}
			if *verbose {
				fmt.Printf("  Tasks: %d\n", len(todoFile.Tasks))
				for _, t := range todoFile.Tasks {
					fmt.Printf("    - [%s] %s: %s\n", t.Status, t.ID, t.Title)
				}
			}
		}
	}
	fmt.Println()

	// Check schema file
	fmt.Printf("Schema file: %s\n", schemaPath)
	if info, err := os.Stat(schemaPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ⚠️  Not found (will be created on run)")
		} else {
			fmt.Printf("  ❌ Error: %v\n", err)
			allOK = false
		}
	} else if info.IsDir() {
		fmt.Println("  ❌ Error: path is a directory")
		allOK = false
	} else {
		fmt.Println("  ✅ OK")
	}
	fmt.Println()

	// Check log directory
	fmt.Printf("Log directory: %s\n", cfg.LogDir)
	if _, err := os.Stat(cfg.LogDir); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ⚠️  Not found (will be created on run)")
		} else {
			fmt.Printf("  ❌ Error: %v\n", err)
			allOK = false
		}
	} else {
		fmt.Println("  ✅ OK")
	}
	fmt.Println()

	// Check prompts directory
	fmt.Printf("Prompts directory: %s\n", promptDir)
	promptInfo, err := os.Stat(promptDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ❌ Not found")
			allOK = false
		} else {
			fmt.Printf("  ❌ Error: %v\n", err)
			allOK = false
		}
	} else if !promptInfo.IsDir() {
		fmt.Println("  ❌ Error: path is not a directory")
		allOK = false
	} else {
		fmt.Println("  ✅ OK")
		promptFiles := []string{
			prompts.BootstrapPrompt,
			prompts.IterationPrompt,
			prompts.RepairPrompt,
			prompts.ReviewPrompt,
		}
		for _, pf := range promptFiles {
			p := filepath.Join(promptDir, pf)
			if _, err := os.ReadFile(p); err != nil {
				fmt.Printf("  ❌ %s: %v\n", pf, err)
				allOK = false
				continue
			}
			if *verbose {
				fmt.Printf("  ✅ %s\n", pf)
			}
		}

		summaryPath := filepath.Join(promptDir, prompts.SummarySchema)
		summaryData, err := os.ReadFile(summaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("  ⚠️  %s: not found\n", prompts.SummarySchema)
			} else {
				fmt.Printf("  ❌ %s: %v\n", prompts.SummarySchema, err)
				allOK = false
			}
		} else {
			var parsed any
			if err := json.Unmarshal(summaryData, &parsed); err != nil {
				fmt.Printf("  ❌ %s: invalid JSON (%v)\n", prompts.SummarySchema, err)
				allOK = false
			} else if *verbose {
				fmt.Printf("  ✅ %s\n", prompts.SummarySchema)
			}
		}
	}
	fmt.Println()

	// Overall status
	if allOK {
		fmt.Println("✅ All checks passed!")
		return nil
	}
	fmt.Println("⚠️  Some checks failed. Looper may not function correctly.")
	return fmt.Errorf("doctor checks failed")
}

// tailCommand tails the latest log file.
func tailCommand(cfg *config.Config, args []string) error {
	// Parse tail-specific flags
	fs := flag.NewFlagSet("looper tail", flag.ContinueOnError)
	follow := fs.Bool("f", false, "Follow the log (like tail -f)")
	fs.BoolVar(follow, "follow", false, "Follow the log (like tail -f)")
	n := fs.Int("n", 0, "Number of lines to show (0 = all)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Find the log directory
	workDir := cfg.ProjectRoot
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		workDir = wd
	}

	logDir, err := logging.FindLogDir(cfg.LogDir, workDir)
	if err != nil {
		return fmt.Errorf("finding log directory: %w", err)
	}

	// Find the latest JSONL file
	logPath, err := logging.FindLatestLog(logDir)
	if err != nil {
		return fmt.Errorf("finding latest log: %w", err)
	}

	if logPath == "" {
		fmt.Println("No log files found.")
		return nil
	}

	fmt.Printf("Tailing: %s\n", logPath)
	if *follow {
		fmt.Println("(Ctrl+C to stop)")
	}
	fmt.Println()

	return logging.TailLog(os.Stdout, logPath, *n, *follow)
}

// lsCommand lists tasks by status with deterministic ordering.
func lsCommand(cfg *config.Config, args []string) error {
	// Parse ls-specific flags
	fs := flag.NewFlagSet("looper ls", flag.ContinueOnError)
	statusFilter := fs.String("status", "", "Filter by status (todo|doing|blocked|done)")
	verbose := fs.Bool("v", false, "Show more details")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) > 2 {
		return fmt.Errorf("unexpected arguments: %v", remaining[2:])
	}
	if len(remaining) >= 1 && *statusFilter == "" {
		*statusFilter = remaining[0]
		remaining = remaining[1:]
	}
	if len(remaining) > 1 {
		return fmt.Errorf("unexpected arguments: %v", remaining[1:])
	}
	todoPath := cfg.TodoFile
	if len(remaining) == 1 {
		todoPath = remaining[0]
	}

	// Resolve todo file path
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}

	// Load todo file
	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return fmt.Errorf("loading todo file: %w", err)
	}

	// Filter tasks by status if requested
	tasks := todoFile.Tasks
	if *statusFilter != "" {
		var filtered []todo.Task
		for _, t := range tasks {
			if string(t.Status) == *statusFilter {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	// Print tasks grouped by status (if no filter) or just list
	if *statusFilter == "" {
		printTasksByStatus("todo", tasks, todo.StatusTodo, *verbose)
		printTasksByStatus("doing", tasks, todo.StatusDoing, *verbose)
		printTasksByStatus("blocked", tasks, todo.StatusBlocked, *verbose)
		printTasksByStatus("done", tasks, todo.StatusDone, *verbose)
	} else {
		printTaskList(tasks, *verbose)
	}

	return nil
}

// versionCommand prints version information.
func versionCommand() error {
	fmt.Printf("looper version %s\n", Version)
	return nil
}

// printUsage prints the usage message.
func printUsage(fs *flag.FlagSet, w io.Writer) {
	fmt.Fprintln(w, "Looper - A deterministic, autonomous loop runner")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  looper [command] [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  run [file]    Run the loop (default command)")
	fmt.Fprintln(w, "  tui [file]    Launch terminal UI")
	fmt.Fprintln(w, "  doctor [file] Check dependencies, config, and task file validity")
	fmt.Fprintln(w, "  tail          Tail the latest log file")
	fmt.Fprintln(w, "  ls [status] [file]  List tasks by status")
	fmt.Fprintln(w, "  version       Show version information")
	fmt.Fprintln(w, "  help          Show this help message")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global Options:")
	fs.SetOutput(w)
	fs.PrintDefaults()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run Options (use with 'run' command):")
	fmt.Fprintln(w, "  -ui string")
	fmt.Fprintln(w, "        UI mode (tui for terminal UI)")
	fmt.Fprintln(w, "  -prompt string")
	fmt.Fprintln(w, "        User prompt to drive bootstrap (skips markdown scanning)")
	fmt.Fprintln(w, "  -max-iterations int")
	fmt.Fprintln(w, "        Maximum iterations (default 50)")
	fmt.Fprintln(w, "  -schedule string")
	fmt.Fprintln(w, "        Iteration schedule (codex|claude|odd-even|round-robin)")
	fmt.Fprintln(w, "  -odd-agent string")
	fmt.Fprintln(w, "        Agent for odd iterations in odd-even schedule (codex|claude)")
	fmt.Fprintln(w, "  -even-agent string")
	fmt.Fprintln(w, "        Agent for even iterations in odd-even schedule (codex|claude)")
	fmt.Fprintln(w, "  -rr-agents string")
	fmt.Fprintln(w, "        Comma-separated agent list for round-robin (e.g., claude,codex)")
	fmt.Fprintln(w, "  -repair-agent string")
	fmt.Fprintln(w, "        Agent for repair operations (codex|claude)")
	fmt.Fprintln(w, "  -review-agent string")
	fmt.Fprintln(w, "        Agent for review pass (codex|claude)")
	fmt.Fprintln(w, "  -bootstrap-agent string")
	fmt.Fprintln(w, "        Agent for bootstrap operations (codex|claude)")
	fmt.Fprintln(w, "  -apply-summary")
	fmt.Fprintln(w, "        Apply summaries to task file (default true)")
	fmt.Fprintln(w, "  -git-init")
	fmt.Fprintln(w, "        Initialize git repo if missing (default true)")
	fmt.Fprintln(w, "  -hook string")
	fmt.Fprintln(w, "        Hook command to run after each iteration")
	fmt.Fprintln(w, "  -loop-delay int")
	fmt.Fprintln(w, "        Delay between iterations in seconds (default 0)")
	fmt.Fprintln(w)
	if config.PromptDevModeEnabled() {
		fmt.Fprintln(w, "Dev Options (require LOOPER_PROMPT_MODE=dev):")
		fmt.Fprintln(w, "  -prompt-dir string")
		fmt.Fprintln(w, "        Prompt directory override (dev only)")
		fmt.Fprintln(w, "  -print-prompt")
		fmt.Fprintln(w, "        Print rendered prompts before running (dev only)")
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Tail Options (use with 'tail' command):")
	fmt.Fprintln(w, "  -f, --follow")
	fmt.Fprintln(w, "        Follow the log (like tail -f)")
	fmt.Fprintln(w, "  -n int")
	fmt.Fprintln(w, "        Number of lines to show (0 = all)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Ls Options (use with 'ls' command):")
	fmt.Fprintln(w, "  -status string")
	fmt.Fprintln(w, "        Filter by status (todo|doing|blocked|done)")
	fmt.Fprintln(w, "  -v    Show more details")
}

// printTasksByStatus prints tasks of a specific status.
func printTasksByStatus(label string, tasks []todo.Task, status todo.Status, verbose bool) {
	var matching []todo.Task
	for _, t := range tasks {
		if t.Status == status {
			matching = append(matching, t)
		}
	}
	if len(matching) == 0 {
		return
	}
	// Sort deterministically by priority then ID
	sortTasks(matching)
	fmt.Printf("%s (%d):\n", label, len(matching))
	for _, t := range matching {
		printTask(t, verbose)
	}
	fmt.Println()
}

// printTaskList prints a list of tasks.
func printTaskList(tasks []todo.Task, verbose bool) {
	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return
	}
	// Sort deterministically by priority then ID
	sorted := make([]todo.Task, len(tasks))
	copy(sorted, tasks)
	sortTasks(sorted)
	for _, t := range sorted {
		printTask(t, verbose)
	}
}

// printTask prints a single task.
func printTask(t todo.Task, verbose bool) {
	statusIcon := "❓"
	switch t.Status {
	case todo.StatusTodo:
		statusIcon = "📝"
	case todo.StatusDoing:
		statusIcon = "🔄"
	case todo.StatusBlocked:
		statusIcon = "🚫"
	case todo.StatusDone:
		statusIcon = "✅"
	}

	fmt.Printf("  %s [%s] (P%d) %s\n", statusIcon, t.ID, t.Priority, t.Title)

	if verbose {
		if t.Details != "" {
			fmt.Printf("      Details: %s\n", t.Details)
		}
		if len(t.Blockers) > 0 {
			fmt.Printf("      Blockers: %v\n", t.Blockers)
		}
		if len(t.Files) > 0 {
			fmt.Printf("      Files: %v\n", t.Files)
		}
	}
}

// splitAndTrim splits a string by sep and trims whitespace from each part.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// sortTasks sorts tasks deterministically by priority (ascending) then ID (ascending).
func sortTasks(tasks []todo.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func normalizeSchedule(input string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	switch s {
	case "codex", "claude":
		return s, true
	case "odd_even", "odd-even", "oddeven":
		return "odd-even", true
	case "round_robin", "round-robin", "roundrobin", "rr":
		return "round-robin", true
	default:
		if s == "" {
			return "", false
		}
		return s, false
	}
}

func normalizeAgent(input string) string {
	return strings.ToLower(strings.TrimSpace(input))
}

func normalizeAgentList(agents []string) []string {
	if len(agents) == 0 {
		return nil
	}
	result := make([]string, 0, len(agents))
	for _, agent := range agents {
		normalized := normalizeAgent(agent)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func normalizeAgentOrDefault(agent, defaultAgent string) (string, bool, bool) {
	normalized := normalizeAgent(agent)
	if normalized == "" {
		return defaultAgent, true, true
	}
	if !isValidAgent(normalized) {
		return normalized, false, false
	}
	return normalized, true, false
}

func isValidAgent(agent string) bool {
	return agent == "codex" || agent == "claude"
}

func invalidAgentList(agents []string) []string {
	var invalid []string
	for _, agent := range agents {
		normalized := normalizeAgent(agent)
		if normalized == "" {
			continue
		}
		if !isValidAgent(normalized) {
			invalid = append(invalid, normalized)
		}
	}
	return invalid
}

func scheduleUsesClaude(schedule, oddAgent, evenAgent string, rrAgents []string) bool {
	switch schedule {
	case "claude":
		return true
	case "odd-even":
		odd := normalizeAgent(oddAgent)
		if odd == "" {
			odd = "codex"
		}
		even := normalizeAgent(evenAgent)
		if even == "" {
			even = "claude"
		}
		return odd == "claude" || even == "claude"
	case "round-robin":
		agents := normalizeAgentList(rrAgents)
		if len(agents) == 0 {
			agents = []string{"claude", "codex"}
		}
		for _, agent := range agents {
			if agent == "claude" {
				return true
			}
		}
	}
	return false
}

func checkBinary(label, binary string, required bool) bool {
	fmt.Printf("  %s: %s\n", label, binary)
	if strings.TrimSpace(binary) == "" {
		if required {
			fmt.Println("  ❌ Not configured")
			return false
		}
		fmt.Println("  ⚠️  Not configured")
		return true
	}
	if info, err := os.Stat(binary); err == nil {
		if info.IsDir() {
			if required {
				fmt.Println("  ❌ Path is a directory")
				return false
			}
			fmt.Println("  ⚠️  Path is a directory")
			return true
		}
		if !isExecutablePath(binary, info) {
			if required {
				fmt.Println("  ❌ Not executable")
				return false
			}
			fmt.Println("  ⚠️  Not executable")
			return true
		}
		fmt.Println("  ✅ OK")
		return true
	}

	resolved, err := exec.LookPath(binary)
	if err == nil {
		if info, err := os.Stat(resolved); err == nil {
			if info.IsDir() {
				if required {
					fmt.Printf("  ❌ Found in PATH but is a directory: %s\n", resolved)
					return false
				}
				fmt.Printf("  ⚠️  Found in PATH but is a directory: %s\n", resolved)
				return true
			}
			if !isExecutablePath(resolved, info) {
				if required {
					fmt.Printf("  ❌ Found in PATH but not executable: %s\n", resolved)
					return false
				}
				fmt.Printf("  ⚠️  Found in PATH but not executable: %s\n", resolved)
				return true
			}
		}
		fmt.Printf("  ✅ OK (found in PATH: %s)\n", resolved)
		return true
	}

	if required {
		fmt.Printf("  ❌ Not found: %v\n", err)
		return false
	}
	fmt.Printf("  ⚠️  Not found: %v\n", err)
	return true
}

func isExecutablePath(path string, info os.FileInfo) bool {
	if info == nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return isWindowsExecutable(path)
	}
	return info.Mode().Perm()&0111 != 0
}

func isWindowsExecutable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	return windowsExecutableExts()[ext]
}

func windowsExecutableExts() map[string]bool {
	exts := map[string]bool{}
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	for _, ext := range strings.Split(pathext, ";") {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		exts[strings.ToLower(ext)] = true
	}
	return exts
}

func resolvePromptDir(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	promptDir := cfg.PromptDir
	if promptDir == "" {
		promptDir = prompts.DefaultPromptDir(cfg.ProjectRoot)
	} else if !filepath.IsAbs(promptDir) && cfg.ProjectRoot != "" {
		promptDir = filepath.Join(cfg.ProjectRoot, promptDir)
	}
	return promptDir
}
