// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nibzard/looper/internal/config"
	"github.com/nibzard/looper/internal/logging"
	"github.com/nibzard/looper/internal/loop"
	"github.com/nibzard/looper/internal/todo"
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
	maxIter := fs.Int("max-iterations", cfg.MaxIterations, "Maximum iterations")
	schedule := fs.String("schedule", cfg.Schedule, "Iteration schedule (codex|claude|odd-even|round-robin)")
	repairAgent := fs.String("repair-agent", cfg.RepairAgent, "Agent for repair operations (codex|claude)")
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

	// Update config with parsed values
	cfg.MaxIterations = *maxIter
	cfg.Schedule = *schedule
	cfg.RepairAgent = *repairAgent
	cfg.ApplySummary = *applySummary
	cfg.GitInit = *gitInit
	cfg.HookCommand = *hook
	cfg.LoopDelaySeconds = *loopDelay

	// Make todo file path absolute
	if !filepath.IsAbs(cfg.TodoFile) {
		cfg.TodoFile = filepath.Join(cfg.ProjectRoot, cfg.TodoFile)
	}

	// Create and run loop
	l, err := loop.New(cfg, cfg.ProjectRoot)
	if err != nil {
		return fmt.Errorf("initializing loop: %w", err)
	}

	return l.Run(ctx)
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

	// Check todo file
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}
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
			schemaPath := cfg.SchemaFile
			if !filepath.IsAbs(schemaPath) {
				schemaPath = filepath.Join(cfg.ProjectRoot, schemaPath)
			}
			result := todoFile.Validate(todo.ValidationOptions{SchemaPath: schemaPath})
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
	schemaPath := cfg.SchemaFile
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(cfg.ProjectRoot, schemaPath)
	}
	fmt.Printf("Schema file: %s\n", schemaPath)
	if _, err := os.Stat(schemaPath); err != nil {
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

	// Check agent binaries
	agents := []struct {
		name   string
		binary string
	}{
		{"codex", cfg.Agents.Codex.Binary},
		{"claude", cfg.Agents.Claude.Binary},
	}
	for _, agent := range agents {
		fmt.Printf("Agent binary (%s): %s\n", agent.name, agent.binary)
		if agent.binary == "" {
			fmt.Println("  ⚠️  Not configured")
			continue
		}
		if _, err := os.Stat(agent.binary); err == nil {
			fmt.Println("  ✅ OK")
		} else {
			// Try to find it in PATH
			if _, err := exec.LookPath(agent.binary); err == nil {
				fmt.Println("  ✅ OK (found in PATH)")
			} else {
				fmt.Printf("  ❌ Not found: %v\n", err)
				allOK = false
			}
		}
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
	promptDir := filepath.Join(cfg.ProjectRoot, "prompts")
	fmt.Printf("Prompts directory: %s\n", promptDir)
	promptInfo, err := os.Stat(promptDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ⚠️  Not found (will use embedded prompts)")
		} else {
			fmt.Printf("  ❌ Error: %v\n", err)
			allOK = false
		}
	} else if !promptInfo.IsDir() {
		fmt.Println("  ❌ Error: path is not a directory")
		allOK = false
	} else {
		fmt.Println("  ✅ OK")
		if *verbose {
			promptFiles := []string{
				"bootstrap.txt", "iteration.txt", "repair.txt", "review.txt",
				"summary.schema.json",
			}
			for _, pf := range promptFiles {
				p := filepath.Join(promptDir, pf)
				if _, err := os.Stat(p); err != nil {
					fmt.Printf("    ⚠️  %s: not found\n", pf)
				} else {
					fmt.Printf("    ✅ %s\n", pf)
				}
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
	fmt.Fprintln(w, "  -max-iterations int")
	fmt.Fprintln(w, "        Maximum iterations (default 50)")
	fmt.Fprintln(w, "  -schedule string")
	fmt.Fprintln(w, "        Iteration schedule (codex|claude|odd-even|round-robin)")
	fmt.Fprintln(w, "  -repair-agent string")
	fmt.Fprintln(w, "        Agent for repair operations (codex|claude)")
	fmt.Fprintln(w, "  -apply-summary")
	fmt.Fprintln(w, "        Apply summaries to task file (default true)")
	fmt.Fprintln(w, "  -git-init")
	fmt.Fprintln(w, "        Initialize git repo if missing (default true)")
	fmt.Fprintln(w, "  -hook string")
	fmt.Fprintln(w, "        Hook command to run after each iteration")
	fmt.Fprintln(w, "  -loop-delay int")
	fmt.Fprintln(w, "        Delay between iterations in seconds (default 0)")
	fmt.Fprintln(w)
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
	for _, t := range tasks {
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
