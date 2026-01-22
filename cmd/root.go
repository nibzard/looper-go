// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
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
	case "push":
		return pushCommand(ctx, cfg, remainingArgs)
	case "init":
		return initCommand(cfg, remainingArgs)
	case "validate":
		return validateCommand(cfg, remainingArgs)
	case "fmt":
		return fmtCommand(cfg, remainingArgs)
	case "config":
		return configCommand(cfg, args, remainingArgs)
	case "completion":
		return completionCommand(cfg, remainingArgs)
	case "clean":
		return cleanCommand(cfg, remainingArgs)
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
	schedule := fs.String("schedule", cfg.Schedule, "Iteration schedule (agent name|odd-even|round-robin)")
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
	oddAgent := fs.String("odd-agent", cfg.OddAgent, "Agent for odd iterations in odd-even schedule (any registered agent)")
	evenAgent := fs.String("even-agent", cfg.EvenAgent, "Agent for even iterations in odd-even schedule (any registered agent)")
	var rrAgentsStr string
	if cfg.RRAgents != nil {
		rrAgentsStr = strings.Join(cfg.RRAgents, ",")
	}
	fs.StringVar(&rrAgentsStr, "rr-agents", rrAgentsStr, "Comma-separated agent list for round-robin schedule (e.g., claude,codex)")
	repairAgent := fs.String("repair-agent", cfg.RepairAgent, "Agent for repair operations (any registered agent)")
	reviewAgent := fs.String("review-agent", cfg.ReviewAgent, "Agent for review pass (any registered agent)")
	bootstrapAgent := fs.String("bootstrap-agent", cfg.BootstrapAgent, "Agent for bootstrap operations (any registered agent)")
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
		fmt.Printf("  ❌ Schedule: %s (expected registered agent name or odd-even|round-robin)\n", cfg.Schedule)
		configOK = false
	}
	if repairAgent == "" {
		fmt.Println("  ❌ Repair agent: empty (expected any registered agent)")
		configOK = false
	} else if !isValidAgent(repairAgent) {
		fmt.Printf("  ❌ Repair agent: %s (not a registered agent type)\n", repairAgent)
		configOK = false
	} else {
		fmt.Printf("  ✅ Repair agent: %s\n", repairAgent)
	}
	if reviewAgent == "" {
		fmt.Println("  ✅ Review agent: (default: codex)")
	} else if !isValidAgent(reviewAgent) {
		fmt.Printf("  ❌ Review agent: %s (not a registered agent type)\n", reviewAgent)
		configOK = false
	} else {
		fmt.Printf("  ✅ Review agent: %s\n", reviewAgent)
	}
	if bootstrapAgent == "" {
		fmt.Println("  ✅ Bootstrap agent: (default: codex)")
	} else if !isValidAgent(bootstrapAgent) {
		fmt.Printf("  ❌ Bootstrap agent: %s (not a registered agent type)\n", bootstrapAgent)
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
			fmt.Printf("  ❌ Odd agent: %s (not a registered agent type)\n", cfg.OddAgent)
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
			fmt.Printf("  ❌ Even agent: %s (not a registered agent type)\n", cfg.EvenAgent)
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

	fmt.Println("Dependencies:")
	for _, agent := range requiredAgents(cfg, normalizedSchedule, scheduleOK) {
		if !checkBinary(agent, cfg.GetAgentBinary(agent), true) {
			allOK = false
		}
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

// completionCommand outputs shell completion scripts.
func completionCommand(cfg *config.Config, args []string) error {
	// Parse completion-specific flags
	fs := flag.NewFlagSet("looper completion", flag.ContinueOnError)

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("usage: looper completion <shell>\nSupported shells: bash, zsh, fish, powershell")
	}
	if len(remaining) > 1 {
		return fmt.Errorf("unexpected arguments: %v", remaining[1:])
	}

	shell := strings.ToLower(strings.TrimSpace(remaining[0]))

	switch shell {
	case "bash":
		return bashCompletion()
	case "zsh":
		return zshCompletion()
	case "fish":
		return fishCompletion()
	case "powershell", "pwsh":
		return powershellCompletion()
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish, powershell)", shell)
	}
}

// bashCompletion outputs bash completion script.
func bashCompletion() error {
	fmt.Println(`# looper bash completion
_looper_completion() {
    local cur prev words cword
    _init_completion || return

    local commands="run tui doctor tail ls push init validate fmt config clean completion version help"
    local global_flags="--help --h --version --v --todo --schema --log-dir --codex-bin --claude-bin --codex-model --claude-model --codex-reasoning --codex-args --claude-args"

    # If we're completing a flag argument
    if [[ "$prev" == --todo ]] || [[ "$prev" == --schema ]] || [[ "$prev" == --log-dir ]] || \
       [[ "$prev" == --codex-bin ]] || [[ "$prev" == --claude-bin ]] || [[ "$prev" == --codex-model ]] || \
       [[ "$prev" == --claude-model ]] || [[ "$prev" == --codex-reasoning ]] || [[ "$prev" == --prompt-dir ]] || \
       [[ "$prev" == --hook ]] || [[ "$prev" == --config ]]; then
        _filedir
        return
    fi

    # Complete subcommands
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$global_flags" -- "$cur"))
        return
    fi

    # Check if we already have a command
    local i
    for ((i=1; i<$cword; i++)); do
        if [[ "${words[i]}" == @(run|tui|doctor|tail|ls|push|init|validate|fmt|config|clean|completion|version|help) ]]; then
            local cmd="${words[i]}"
            case "$cmd" in
                run)
                    _looper_run_completion
                    ;;
                tui)
                    _looper_tui_completion
                    ;;
                tail)
                    _looper_tail_completion
                    ;;
                ls)
                    _looper_ls_completion
                    ;;
                push)
                    _looper_push_completion
                    ;;
                init)
                    _looper_init_completion
                    ;;
                validate)
                    _looper_validate_completion
                    ;;
                fmt)
                    _looper_fmt_completion
                    ;;
                config)
                    _looper_config_completion
                    ;;
                clean)
                    _looper_clean_completion
                    ;;
            esac
            return
        fi
    done

    # No command yet, offer commands
    COMPREPLY=($(compgen -W "$commands" -- "$cur"))
}

_looper_run_completion() {
    local flags="--ui --prompt --max-iterations --schedule --odd-agent --even-agent --rr-agents --repair-agent --review-agent --bootstrap-agent --apply-summary --git-init --hook --loop-delay --prompt-dir --print-prompt"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    else
        _filedir
    fi
}

_looper_tui_completion() {
    local flags="--run"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    else
        _filedir
    fi
}

_looper_tail_completion() {
    local flags="-f --follow -n"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    fi
}

_looper_ls_completion() {
    local flags="--status -v"
    local statuses="todo doing blocked done"
    if [[ "$prev" == --status ]]; then
        COMPREPLY=($(compgen -W "$statuses" -- "$cur"))
    elif [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    else
        _filedir
    fi
}

_looper_push_completion() {
    local flags="--agent -y"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    fi
}

_looper_init_completion() {
    local flags="--force --skip-config --todo --schema --config"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    fi
}

_looper_validate_completion() {
    local flags="--schema"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    else
        _filedir
    fi
}

_looper_fmt_completion() {
    local flags="--check -w -write -d -diff"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    else
        _filedir
    fi
}

_looper_config_completion() {
    local flags="--json"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    fi
}

_looper_clean_completion() {
    local flags="--dry-run --keep --age"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    fi
}

complete -F _looper_completion looper`)
	return nil
}

// zshCompletion outputs zsh completion script.
func zshCompletion() error {
	fmt.Println(`#compdef looper

_looper() {
    local -a commands
    commands=(
        'run:Run the loop (default command)'
        'tui:Launch terminal UI'
        'doctor:Check dependencies, config, and task file validity'
        'tail:Tail the latest log file'
        'ls:List tasks by status'
        'push:Run a release workflow via the agent'
        'init:Scaffold project files'
        'validate:Validate task file against schema'
        'fmt:Format task file with stable ordering'
        'config:Show effective configuration'
        'clean:Remove old log runs by age or count'
        'completion:Output shell completion script'
        'version:Show version information'
        'help:Show help message'
    )

    local -a global_flags
    global_flags=(
        '--help[Show help]'
        '-h[Show help]'
        '--version[Show version]'
        '-v[Show version]'
        '--todo[Todo file path]'
        '--schema[Schema file path]'
        '--log-dir[Log directory]'
        '--codex-bin[Codex binary path]'
        '--claude-bin[Claude binary path]'
        '--codex-model[Codex model]'
        '--claude-model[Claude model]'
        '--codex-reasoning[Codex reasoning effort]'
        '--codex-args[Extra args for Codex]'
        '--claude-args[Extra args for Claude]'
    )

    case $words[2] in
        run)
            _looper_run
            ;;
        tui)
            _looper_tui
            ;;
        tail)
            _looper_tail
            ;;
        ls)
            _looper_ls
            ;;
        push)
            _looper_push
            ;;
        init)
            _looper_init
            ;;
        validate)
            _looper_validate
            ;;
        fmt)
            _looper_fmt
            ;;
        config)
            _looper_config
            ;;
        clean)
            _looper_clean
            ;;
        *)
            if [[ $words[CURRENT] == -* ]]; then
                _describe 'global options' global_flags
            else
                _describe 'command' commands
            fi
            ;;
    esac
}

_looper_run() {
    _arguments -s \
        '--ui[UI mode (tui for terminal UI)]:mode:(tui)' \
        '--prompt[User prompt to drive bootstrap]' \
        '--max-iterations[Maximum iterations]:iterations:(50 100)' \
        '--schedule[Iteration schedule]:schedule:(codex claude odd-even round-robin)' \
        '--odd-agent[Agent for odd iterations]:agent:(codex claude)' \
        '--even-agent[Agent for even iterations]:agent:(codex claude)' \
        '--rr-agents[Comma-separated agent list for round-robin]:agents' \
        '--repair-agent[Agent for repair operations]:agent:(codex claude)' \
        '--review-agent[Agent for review pass]:agent:(codex claude)' \
        '--bootstrap-agent[Agent for bootstrap operations]:agent:(codex claude)' \
        '--apply-summary[Apply summaries to task file]:bool:(true false)' \
        '--git-init[Initialize git repo if missing]:bool:(true false)' \
        '--hook[Hook command to run after each iteration]:command' \
        '--loop-delay[Delay between iterations in seconds]:seconds' \
        '--prompt-dir[Prompt directory override (dev only)]:dir:_files' \
        '--print-prompt[Print rendered prompts before running (dev only)]:bool:(true false)' \
        '*::todo file:_files'
}

_looper_tui() {
    _arguments -s \
        '--run[Run the loop in the background]:bool:(true false)' \
        '*::todo file:_files'
}

_looper_tail() {
    _arguments -s \
        {-f,--follow}'[Follow the log (like tail -f)]' \
        '-n[Number of lines to show (0 = all)]:lines'
}

_looper_ls() {
    _arguments -s \
        '--status[Filter by status]:status:(todo doing blocked done)' \
        '-v[Show more details]' \
        '*::todo file:_files'
}

_looper_push() {
    _arguments -s \
        '--agent[Agent to use for release workflow]:agent:(codex claude)' \
        '-y[Skip confirmation prompts]'
}

_looper_init() {
    _arguments -s \
        '--force[Overwrite existing files]' \
        '--skip-config[Skip creating looper.toml]' \
        '--todo[Path for to-do.json]:file:_files' \
        '--schema[Path for to-do.schema.json]:file:_files' \
        '--config[Path for looper.toml]:file:_files'
}

_looper_validate() {
    _arguments -s \
        '--schema[Path to schema file]:file:_files' \
        '*::todo file:_files'
}

_looper_fmt() {
    _arguments -s \
        '--check[Check if file is formatted without writing]' \
        {-w,-write}'[Write formatted file back to disk]' \
        {-d,-diff}'[Display diffs of formatting changes]' \
        '*::todo file:_files'
}

_looper_config() {
    _arguments -s \
        '--json[Output in JSON format]'
}

_looper_clean() {
    _arguments -s \
        '--dry-run[Show what would be deleted without deleting]' \
        '--keep[Number of recent runs to keep]:count' \
        '--age[Delete logs older than duration]:duration'
}

_looper`)
	return nil
}

// fishCompletion outputs fish completion script.
func fishCompletion() error {
	fmt.Println(`# looper fish completion

complete -c looper -f

# Global options
complete -c looper -s h -l help -d 'Show help'
complete -c looper -s v -l version -d 'Show version'
complete -c looper -l todo -r -d 'Todo file path'
complete -c looper -l schema -r -d 'Schema file path'
complete -c looper -l log-dir -r -d 'Log directory'
complete -c looper -l codex-bin -r -d 'Codex binary path'
complete -c looper -l claude-bin -r -d 'Claude binary path'
complete -c looper -l codex-model -r -d 'Codex model'
complete -c looper -l claude-model -r -d 'Claude model'
complete -c looper -l codex-reasoning -r -d 'Codex reasoning effort'
complete -c looper -l codex-args -r -d 'Extra args for Codex'
complete -c looper -l claude-args -r -d 'Extra args for Claude'

# Commands
complete -c looper -n __fish_use_subcommand -f -a run -d 'Run the loop (default command)'
complete -c looper -n __fish_use_subcommand -f -a tui -d 'Launch terminal UI'
complete -c looper -n __fish_use_subcommand -f -a doctor -d 'Check dependencies, config, and task file validity'
complete -c looper -n __fish_use_subcommand -f -a tail -d 'Tail the latest log file'
complete -c looper -n __fish_use_subcommand -f -a ls -d 'List tasks by status'
complete -c looper -n __fish_use_subcommand -f -a push -d 'Run a release workflow via the agent'
complete -c looper -n __fish_use_subcommand -f -a init -d 'Scaffold project files'
complete -c looper -n __fish_use_subcommand -f -a validate -d 'Validate task file against schema'
complete -c looper -n __fish_use_subcommand -f -a fmt -d 'Format task file with stable ordering'
complete -c looper -n __fish_use_subcommand -f -a config -d 'Show effective configuration'
complete -c looper -n __fish_use_subcommand -f -a clean -d 'Remove old log runs by age or count'
complete -c looper -n __fish_use_subcommand -f -a completion -d 'Output shell completion script'
complete -c looper -n __fish_use_subcommand -f -a version -d 'Show version information'
complete -c looper -n __fish_use_subcommand -f -a help -d 'Show help message'

# Run command options
complete -c looper -n "__fish_seen_subcommand_from run" -l ui -r -d 'UI mode (tui for terminal UI)' -f -a tui
complete -c looper -n "__fish_seen_subcommand_from run" -l prompt -r -d 'User prompt to drive bootstrap'
complete -c looper -n "__fish_seen_subcommand_from run" -l max-iterations -r -d 'Maximum iterations'
complete -c looper -n "__fish_seen_subcommand_from run" -l schedule -r -d 'Iteration schedule' -f -a 'codex' -f -a 'claude' -f -a 'odd-even' -f -a 'round-robin'
complete -c looper -n "__fish_seen_subcommand_from run" -l odd-agent -r -d 'Agent for odd iterations' -f -a 'codex' -f -a 'claude'
complete -c looper -n "__fish_seen_subcommand_from run" -l even-agent -r -d 'Agent for even iterations' -f -a 'codex' -f -a 'claude'
complete -c looper -n "__fish_seen_subcommand_from run" -l rr-agents -r -d 'Comma-separated agent list for round-robin'
complete -c looper -n "__fish_seen_subcommand_from run" -l repair-agent -r -d 'Agent for repair operations' -f -a 'codex' -f -a 'claude'
complete -c looper -n "__fish_seen_subcommand_from run" -l review-agent -r -d 'Agent for review pass' -f -a 'codex' -f -a 'claude'
complete -c looper -n "__fish_seen_subcommand_from run" -l bootstrap-agent -r -d 'Agent for bootstrap operations' -f -a 'codex' -f -a 'claude'
complete -c looper -n "__fish_seen_subcommand_from run" -l apply-summary -r -d 'Apply summaries to task file' -f -a 'true' -f -a 'false'
complete -c looper -n "__fish_seen_subcommand_from run" -l git-init -r -d 'Initialize git repo if missing' -f -a 'true' -f -a 'false'
complete -c looper -n "__fish_seen_subcommand_from run" -l hook -r -d 'Hook command to run after each iteration'
complete -c looper -n "__fish_seen_subcommand_from run" -l loop-delay -r -d 'Delay between iterations in seconds'

# TUI command options
complete -c looper -n "__fish_seen_subcommand_from tui" -l run -d 'Run the loop in the background'

# Tail command options
complete -c looper -n "__fish_seen_subcommand_from tail" -s f -l follow -d 'Follow the log (like tail -f)'
complete -c looper -n "__fish_seen_subcommand_from tail" -s n -r -d 'Number of lines to show (0 = all)'

# Ls command options
complete -c looper -n "__fish_seen_subcommand_from ls" -l status -r -d 'Filter by status' -f -a 'todo' -f -a 'doing' -f -a 'blocked' -f -a 'done'
complete -c looper -n "__fish_seen_subcommand_from ls" -s v -d 'Show more details'

# Push command options
complete -c looper -n "__fish_seen_subcommand_from push" -l agent -r -d 'Agent to use for release workflow' -f -a 'codex' -f -a 'claude'
complete -c looper -n "__fish_seen_subcommand_from push" -s y -d 'Skip confirmation prompts'

# Init command options
complete -c looper -n "__fish_seen_subcommand_from init" -l force -d 'Overwrite existing files'
complete -c looper -n "__fish_seen_subcommand_from init" -l skip-config -d 'Skip creating looper.toml'
complete -c looper -n "__fish_seen_subcommand_from init" -l todo -r -d 'Path for to-do.json'
complete -c looper -n "__fish_seen_subcommand_from init" -l schema -r -d 'Path for to-do.schema.json'
complete -c looper -n "__fish_seen_subcommand_from init" -l config -r -d 'Path for looper.toml'

# Validate command options
complete -c looper -n "__fish_seen_subcommand_from validate" -l schema -r -d 'Path to schema file'

# Fmt command options
complete -c looper -n "__fish_seen_subcommand_from fmt" -l check -d 'Check if file is formatted without writing'
complete -c looper -n "__fish_seen_subcommand_from fmt" -s w -l write -d 'Write formatted file back to disk'
complete -c looper -n "__fish_seen_subcommand_from fmt" -s d -l diff -d 'Display diffs of formatting changes'

# Config command options
complete -c looper -n "__fish_seen_subcommand_from config" -l json -d 'Output in JSON format'

# Clean command options
complete -c looper -n "__fish_seen_subcommand_from clean" -l dry-run -d 'Show what would be deleted without deleting'
complete -c looper -n "__fish_seen_subcommand_from clean" -l keep -r -d 'Number of recent runs to keep'
complete -c looper -n "__fish_seen_subcommand_from clean" -l age -r -d 'Delete logs older than duration (e.g., 7d, 24h, 30m)'

# Completion command options
complete -c looper -n "__fish_seen_subcommand_from completion" -f -a 'bash' -d 'Output bash completion script'
complete -c looper -n "__fish_seen_subcommand_from completion" -f -a 'zsh' -d 'Output zsh completion script'
complete -c looper -n "__fish_seen_subcommand_from completion" -f -a 'fish' -d 'Output fish completion script'
complete -c looper -n "__fish_seen_subcommand_from completion" -f -a 'powershell' -d 'Output PowerShell completion script'`)
	return nil
}

// powershellCompletion outputs PowerShell completion script.
func powershellCompletion() error {
	fmt.Println(`# looper PowerShell completion

using namespace System.Management.Automation
using namespace System.Management.Automation.Language

Register-ArgumentCompleter -Native -CommandName looper -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @('run', 'tui', 'doctor', 'tail', 'ls', 'push', 'init', 'validate', 'fmt', 'config', 'clean', 'completion', 'version', 'help')

    $globalFlags = @('--help', '-h', '--version', '-v', '--todo', '--schema', '--log-dir', '--codex-bin', '--claude-bin', '--codex-model', '--claude-model', '--codex-reasoning', '--codex-args', '--claude-args')

    function Get-SubCommand {
        $commandAst.CommandElements |
            Where-Object { $_.ParameterType -eq [CommandParameterType] -and $_.Extent -isnot [ParameterExpressionExtent] } |
            Select-Object -ExpandProperty Extent -ExpandProperty Text
    }

    $subCommand = Get-SubCommand

    if ($null -eq $subCommand -or $subCommand -eq '') {
        # Complete subcommands
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterValue, $_)
        }

        # Complete global flags
        $globalFlags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
        }
    } else {
        switch ($subCommand) {
            'run' {
                $flags = @('--ui', '--prompt', '--max-iterations', '--schedule', '--odd-agent', '--even-agent', '--rr-agents', '--repair-agent', '--review-agent', '--bootstrap-agent', '--apply-summary', '--git-init', '--hook', '--loop-delay', '--prompt-dir', '--print-prompt')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'tui' {
                $flags = @('--run')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'tail' {
                $flags = @('-f', '--follow', '-n')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'ls' {
                $flags = @('--status', '-v')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
                if ($commandAst.ToString() -match '--status\s+$') {
                    @('todo', 'doing', 'blocked', 'done') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                        [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterValue, $_)
                    }
                }
            }
            'push' {
                $flags = @('--agent', '-y')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'init' {
                $flags = @('--force', '--skip-config', '--todo', '--schema', '--config')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'validate' {
                $flags = @('--schema')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'fmt' {
                $flags = @('--check', '-w', '--write', '-d', '--diff')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'config' {
                $flags = @('--json')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'clean' {
                $flags = @('--dry-run', '--keep', '--age')
                $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterName, $_)
                }
            }
            'completion' {
                @('bash', 'zsh', 'fish', 'powershell') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, [CompletionResultType]::ParameterValue, $_)
                }
            }
        }
    }
}`)
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
	fmt.Fprintln(w, "  push          Run a release workflow via the agent")
	fmt.Fprintln(w, "  init          Scaffold project files (to-do.json, to-do.schema.json, looper.toml)")
	fmt.Fprintln(w, "  validate      Validate task file against schema")
	fmt.Fprintln(w, "  fmt           Format task file with stable ordering and 2-space indent")
	fmt.Fprintln(w, "  config        Show effective configuration")
	fmt.Fprintln(w, "  clean         Remove old log runs by age or count")
	fmt.Fprintln(w, "  completion    Output shell completion script (bash|zsh|fish|powershell)")
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
	fmt.Fprintln(w, "        Iteration schedule (agent name|odd-even|round-robin)")
	fmt.Fprintln(w, "  -odd-agent string")
	fmt.Fprintln(w, "        Agent for odd iterations in odd-even schedule")
	fmt.Fprintln(w, "  -even-agent string")
	fmt.Fprintln(w, "        Agent for even iterations in odd-even schedule")
	fmt.Fprintln(w, "  -rr-agents string")
	fmt.Fprintln(w, "        Comma-separated agent list for round-robin")
	fmt.Fprintln(w, "  -repair-agent string")
	fmt.Fprintln(w, "        Agent for repair operations")
	fmt.Fprintln(w, "  -review-agent string")
	fmt.Fprintln(w, "        Agent for review pass")
	fmt.Fprintln(w, "  -bootstrap-agent string")
	fmt.Fprintln(w, "        Agent for bootstrap operations")
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
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Push Options (use with 'push' command):")
	fmt.Fprintln(w, "  -agent string")
	fmt.Fprintln(w, "        Agent to use for release workflow")
	fmt.Fprintln(w, "  -y")
	fmt.Fprintln(w, "        Skip confirmation prompts")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Init Options (use with 'init' command):")
	fmt.Fprintln(w, "  -force")
	fmt.Fprintln(w, "        Overwrite existing files")
	fmt.Fprintln(w, "  -skip-config")
	fmt.Fprintln(w, "        Skip creating looper.toml")
	fmt.Fprintln(w, "  -todo string")
	fmt.Fprintln(w, "        Path for to-do.json (default: to-do.json)")
	fmt.Fprintln(w, "  -schema string")
	fmt.Fprintln(w, "        Path for to-do.schema.json (default: to-do.schema.json)")
	fmt.Fprintln(w, "  -config string")
	fmt.Fprintln(w, "        Path for looper.toml (default: looper.toml)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Validate Options (use with 'validate' command):")
	fmt.Fprintln(w, "  -schema string")
	fmt.Fprintln(w, "        Path to schema file (default: to-do.schema.json)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Fmt Options (use with 'fmt' command):")
	fmt.Fprintln(w, "  -check")
	fmt.Fprintln(w, "        Check if file is formatted without writing")
	fmt.Fprintln(w, "  -w, -write")
	fmt.Fprintln(w, "        Write formatted file back to disk")
	fmt.Fprintln(w, "  -d, -diff")
	fmt.Fprintln(w, "        Display diffs of formatting changes")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Clean Options (use with 'clean' command):")
	fmt.Fprintln(w, "  -dry-run")
	fmt.Fprintln(w, "        Show what would be deleted without deleting")
	fmt.Fprintln(w, "  -keep int")
	fmt.Fprintln(w, "        Number of recent runs to keep (0 = delete all)")
	fmt.Fprintln(w, "  -age duration")
	fmt.Fprintln(w, "        Delete logs older than duration (e.g., 7d, 24h, 30m)")
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
// ID comparison is numeric-aware: T2 sorts before T10.
func sortTasks(tasks []todo.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		return todo.CompareIDs(tasks[i].ID, tasks[j].ID)
	})
}

func normalizeSchedule(input string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	switch s {
	case "odd_even", "odd-even", "oddeven":
		return "odd-even", true
	case "round_robin", "round-robin", "roundrobin", "rr":
		return "round-robin", true
	default:
		if s == "" {
			return "", false
		}
		if isValidAgent(s) {
			return s, true
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
	return agents.IsAgentTypeRegistered(agent)
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

func requiredAgents(cfg *config.Config, schedule string, scheduleOK bool) []string {
	required := make(map[string]struct{})
	add := func(agent string) {
		normalized := normalizeAgent(agent)
		if normalized == "" {
			return
		}
		if !isValidAgent(normalized) {
			return
		}
		required[normalized] = struct{}{}
	}

	repairAgent := normalizeAgent(cfg.RepairAgent)
	if repairAgent == "" {
		repairAgent = "codex"
	}
	add(repairAgent)
	add(cfg.GetReviewAgent())
	add(cfg.GetBootstrapAgent())

	if scheduleOK {
		switch schedule {
		case "odd-even":
			oddAgent, oddOK, _ := normalizeAgentOrDefault(cfg.OddAgent, "codex")
			if oddOK {
				add(oddAgent)
			}
			evenAgent, evenOK, _ := normalizeAgentOrDefault(cfg.EvenAgent, "claude")
			if evenOK {
				add(evenAgent)
			}
		case "round-robin":
			agents := normalizeAgentList(cfg.RRAgents)
			if len(agents) == 0 {
				agents = []string{"claude", "codex"}
			}
			for _, agent := range agents {
				add(agent)
			}
		default:
			add(schedule)
		}
	}

	result := make([]string, 0, len(required))
	for agent := range required {
		result = append(result, agent)
	}
	sort.Strings(result)
	return result
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

// pushCommand runs a release workflow via the agent.
func pushCommand(ctx context.Context, cfg *config.Config, args []string) error {
	// Parse push-specific flags
	fs := flag.NewFlagSet("looper push", flag.ContinueOnError)
	agentFlag := fs.String("agent", "codex", "Agent to use for release workflow (any registered agent)")
	yes := fs.Bool("y", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	// Determine work directory
	workDir := cfg.ProjectRoot
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		workDir = wd
	}

	// Check for git
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}

	fmt.Printf("Git found: %s\n", gitPath)

	// Check if we're in a git repository
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a git repository (no .git directory in %s)", workDir)
		}
		return fmt.Errorf("checking git repository: %w", err)
	}

	fmt.Println("Git repository detected.")

	// Check for git remote
	var hasRemote bool
	remotes, err := exec.Command("git", "-C", workDir, "remote", "-v").Output()
	if err == nil && len(remotes) > 0 {
		hasRemote = true
		fmt.Println("Git remote detected.")
	}

	// Check for gh
	var hasGH bool
	ghPath, err := exec.LookPath("gh")
	if err == nil {
		hasGH = true
		fmt.Printf("GitHub CLI found: %s\n", ghPath)

		// Check gh auth status
		if err := exec.Command("gh", "auth", "status").Run(); err != nil {
			return fmt.Errorf("gh auth failed: %w (run 'gh auth login' first)", err)
		}
		fmt.Println("GitHub CLI authenticated.")
	} else {
		fmt.Println("GitHub CLI not found. Will skip GitHub-specific operations.")
	}

	// If no remote and gh is available, offer to create a repo
	if !hasRemote && hasGH {
		createRepo := func() error {
			cmd := exec.Command("gh", "repo", "create", "--public", "--source", ".", "--remote", "origin", "--push")
			cmd.Dir = workDir
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("creating GitHub repo: %w\nOutput: %s", err, string(output))
			}
			hasRemote = true
			fmt.Println("GitHub repository created and remote added.")
			return nil
		}
		if *yes {
			if err := createRepo(); err != nil {
				return err
			}
		} else {
			fmt.Print("\nNo git remote detected. Create a GitHub repository? [y/N] ")
			var response string
			if _, err := fmt.Scanln(&response); err != nil {
				return fmt.Errorf("reading response: %w", err)
			}
			if strings.ToLower(strings.TrimSpace(response)) != "y" {
				fmt.Println("Skipping repository creation.")
			} else if err := createRepo(); err != nil {
				return err
			}
		}
	}

	// Confirm before running agent
	if !*yes {
		fmt.Print("\nProceed with release workflow? [y/N] ")
		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Set up prompt store and renderer
	promptDir := resolvePromptDir(cfg)
	promptStore := prompts.NewStore(workDir, promptDir)
	renderer := prompts.NewRenderer(promptStore)

	// Render push prompt
	promptData := prompts.Data{
		WorkDir: workDir,
		HasGH:   hasGH,
		Now:     time.Now().UTC().Format(time.RFC3339),
	}
	prompt, err := renderer.Render(prompts.PushPrompt, promptData)
	if err != nil {
		return fmt.Errorf("rendering push prompt: %w", err)
	}

	// Set up logging for the push command
	logDir, err := logging.FindLogDir(cfg.LogDir, workDir)
	if err != nil {
		return fmt.Errorf("finding log directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	runLogger, err := logging.NewRunLogger(logDir, workDir)
	if err != nil {
		return fmt.Errorf("creating run logger: %w", err)
	}
	defer runLogger.Close()
	logWriter := agents.NewIOStreamLogWriter(runLogger.Writer())
	label := "push"

	// Also output to stdout unless LOOPER_QUIET is set
	var multiLogWriter agents.LogWriter = logWriter
	if os.Getenv("LOOPER_QUIET") == "" {
		stdoutWriter := agents.NewIOStreamLogWriter(os.Stdout)
		stdoutWriter.SetIndent("  ")
		multiLogWriter = agents.NewMultiLogWriter(logWriter, stdoutWriter)
	}

	// Set up agent
	agentName := normalizeAgent(*agentFlag)
	if agentName == "" {
		return fmt.Errorf("invalid agent type: %s", *agentFlag)
	}
	if !isValidAgent(agentName) {
		return fmt.Errorf("invalid agent type: %s (registered types: %v)", agentName, agents.RegisteredAgentTypes())
	}
	agentType := agents.AgentType(agentName)
	agentConfig := agents.Config{
		Binary:          cfg.GetAgentBinary(agentName),
		Model:           cfg.GetAgentModel(agentName),
		Reasoning:       cfg.GetAgentReasoning(agentName),
		WorkDir:         workDir,
		Args:            nil,
		LastMessagePath: runLogger.LastMessagePath(label),
	}

	// Resolve binary path if not set
	if agentConfig.Binary == "" {
		path, err := agents.FindAgentBinary(agentType)
		if err != nil {
			return fmt.Errorf("finding agent binary: %w", err)
		}
		agentConfig.Binary = path
	}

	agent, err := agents.NewAgent(agentType, agentConfig)
	if err != nil {
		return fmt.Errorf("creating agent: %w", err)
	}

	fmt.Println("\n=== Running Release Workflow ===")

	// Run the agent
	summary, err := agent.Run(ctx, prompt, multiLogWriter)
	if err != nil && !errors.Is(err, agents.ErrSummaryMissing) {
		return fmt.Errorf("running agent: %w", err)
	}

	fmt.Println("\n=== Release Workflow Complete ===")
	if summary != nil {
		fmt.Printf("\nSummary: %s\n", summary.Summary)
		if len(summary.Files) > 0 {
			fmt.Printf("Files: %v\n", summary.Files)
		}
		if len(summary.Blockers) > 0 {
			fmt.Printf("Blockers: %v\n", summary.Blockers)
		}
	}

	fmt.Printf("\nLog saved to: %s\n", runLogger.LogPath)
	return nil
}

// initCommand scaffolds project files (to-do.json, to-do.schema.json, looper.toml).
func initCommand(cfg *config.Config, args []string) error {
	// Parse init-specific flags
	fs := flag.NewFlagSet("looper init", flag.ContinueOnError)
	force := fs.Bool("force", false, "Overwrite existing files")
	skipConfig := fs.Bool("skip-config", false, "Skip creating looper.toml")
	todoFile := fs.String("todo", cfg.TodoFile, "Path for to-do.json")
	schemaFile := fs.String("schema", cfg.SchemaFile, "Path for to-do.schema.json")
	configFile := fs.String("config", "looper.toml", "Path for looper.toml")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	// Determine work directory
	workDir := cfg.ProjectRoot
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		workDir = wd
	}

	// Resolve file paths
	todoPath := *todoFile
	schemaPath := *schemaFile
	configPath := *configFile
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(workDir, todoPath)
	}
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(workDir, schemaPath)
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(workDir, configPath)
	}

	fmt.Println("Looper Init")
	fmt.Println("===========")
	fmt.Println()

	// Create to-do.schema.json
	fmt.Printf("Creating %s...\n", schemaPath)
	schemaAction, err := createSchemaFile(schemaPath, *force)
	if err != nil {
		return fmt.Errorf("creating schema file: %w", err)
	}
	printInitAction("Schema", schemaAction)

	// Create to-do.json
	fmt.Printf("\nCreating %s...\n", todoPath)
	todoAction, err := createTodoFile(todoPath, *force)
	if err != nil {
		return fmt.Errorf("creating todo file: %w", err)
	}
	printInitAction("Todo", todoAction)

	// Create looper.toml
	if !*skipConfig {
		fmt.Printf("\nCreating %s...\n", configPath)
		configAction, err := createConfigFile(configPath, *force)
		if err != nil {
			return fmt.Errorf("creating config file: %w", err)
		}
		printInitAction("Config", configAction)
	}

	fmt.Println()
	fmt.Println("✅ Project initialized successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Edit %s to add your tasks\n", todoPath)
	fmt.Println("  2. Run 'looper' to start the loop")
	fmt.Println("  3. Run 'looper doctor' to check your setup")
	fmt.Println()

	return nil
}

// createSchemaFile creates the schema file.
func createSchemaFile(path string, force bool) (initFileAction, error) {
	return writeInitFile(path, force, func(path string) error {
		schemaContent, err := prompts.BundledSchema()
		if err != nil {
			return fmt.Errorf("loading bundled schema: %w", err)
		}
		if err := os.WriteFile(path, schemaContent, 0644); err != nil {
			return fmt.Errorf("writing schema file: %w", err)
		}
		return nil
	})
}

// createTodoFile creates a minimal todo file.
func createTodoFile(path string, force bool) (initFileAction, error) {
	return writeInitFile(path, force, func(path string) error {
		// Create a minimal todo file with one example task
		todoFile := &todo.File{
			SchemaVersion: 1,
			Project: &todo.Project{
				Name: "",
				Root: ".",
			},
			SourceFiles: []string{"README.md"},
			Tasks: []todo.Task{
				{
					ID:          "T001",
					Title:       "Example: Add project documentation",
					Description: "Create a README.md file documenting the project setup and usage.",
					Reference:   "README.md",
					Priority:    1,
					Status:      todo.StatusTodo,
				},
			},
		}

		// Save the file
		if err := todoFile.Save(path); err != nil {
			return fmt.Errorf("saving todo file: %w", err)
		}
		return nil
	})
}

// createConfigFile creates a looper.toml config file.
func createConfigFile(path string, force bool) (initFileAction, error) {
	return writeInitFile(path, force, func(path string) error {
		// Write the example config
		configContent := config.ExampleConfig()
		if err := os.WriteFile(path, []byte(configContent), 0644); err != nil {
			return fmt.Errorf("writing config file: %w", err)
		}
		return nil
	})
}

type initFileAction string

const (
	initFileCreated     initFileAction = "created"
	initFileOverwritten initFileAction = "overwritten"
	initFileSkipped     initFileAction = "skipped"
)

func writeInitFile(path string, force bool, write func(string) error) (initFileAction, error) {
	exists, err := fileExists(path)
	if err != nil {
		return "", err
	}
	if exists && !force {
		return initFileSkipped, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("creating parent directory: %w", err)
	}
	if err := write(path); err != nil {
		return "", err
	}
	if exists {
		return initFileOverwritten, nil
	}
	return initFileCreated, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func printInitAction(label string, action initFileAction) {
	switch action {
	case initFileCreated:
		fmt.Printf("  ✅ %s file created\n", label)
	case initFileOverwritten:
		fmt.Printf("  ✅ %s file overwritten\n", label)
	case initFileSkipped:
		fmt.Printf("  Skipped (%s file already exists; use --force to overwrite)\n", strings.ToLower(label))
	default:
		fmt.Printf("  %s file unchanged\n", label)
	}
}

// validateCommand validates a task file against the schema.
func validateCommand(cfg *config.Config, args []string) error {
	// Parse validate-specific flags
	fs := flag.NewFlagSet("looper validate", flag.ContinueOnError)
	schemaFlag := fs.String("schema", cfg.SchemaFile, "Path to schema file")

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

	// Resolve paths
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}
	schemaPath := *schemaFlag
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(cfg.ProjectRoot, schemaPath)
	}

	// Check if todo file exists
	if _, err := os.Stat(todoPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("todo file not found: %s", todoPath)
		}
		return fmt.Errorf("accessing todo file: %w", err)
	}

	// Load the todo file
	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return fmt.Errorf("loading todo file: %w", err)
	}

	fmt.Printf("Validating: %s\n", todoPath)
	fmt.Println()

	// Also validate dependencies
	if depErr := todoFile.ValidateDependencies(); depErr != nil {
		switch depErr.(type) {
		case *todo.DependencyCycleError, *todo.MissingDependencyError:
			fmt.Printf("  ❌ Dependency validation failed:\n")
			fmt.Printf("     %v\n", depErr)
			return depErr
		default:
			return fmt.Errorf("validating dependencies: %w", depErr)
		}
	}

	// Validate against schema
	result := todoFile.Validate(todo.ValidationOptions{SchemaPath: schemaPath})

	// Print warnings
	for _, w := range result.Warnings {
		fmt.Printf("  ⚠️  %s\n", w)
	}

	if result.Valid {
		fmt.Println("  ✅ Valid")
		return nil
	}

	fmt.Println("  ❌ Validation failed:")
	for _, e := range result.Errors {
		fmt.Printf("     - %v\n", e)
	}
	return fmt.Errorf("validation failed")
}

// fmtCommand formats a task file with stable ordering and 2-space indentation.
func fmtCommand(cfg *config.Config, args []string) error {
	// Parse fmt-specific flags
	fs := flag.NewFlagSet("looper fmt", flag.ContinueOnError)
	check := fs.Bool("check", false, "Check if file is formatted without writing")
	write := fs.Bool("w", false, "Write formatted file back to disk")
	diff := fs.Bool("d", false, "Display diffs of formatting changes")
	fs.BoolVar(write, "write", false, "Write formatted file back to disk")
	fs.BoolVar(diff, "diff", false, "Display diffs of formatting changes")

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

	// Resolve path
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}

	// Check if todo file exists
	if _, err := os.Stat(todoPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("todo file not found: %s", todoPath)
		}
		return fmt.Errorf("accessing todo file: %w", err)
	}

	// Read the original file
	originalData, err := os.ReadFile(todoPath)
	if err != nil {
		return fmt.Errorf("reading todo file: %w", err)
	}

	// Load the todo file
	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return fmt.Errorf("loading todo file: %w", err)
	}

	// Normalize tasks (sort by priority, then by ID)
	sort.Slice(todoFile.Tasks, func(i, j int) bool {
		if todoFile.Tasks[i].Priority != todoFile.Tasks[j].Priority {
			return todoFile.Tasks[i].Priority < todoFile.Tasks[j].Priority
		}
		return todo.CompareIDs(todoFile.Tasks[i].ID, todoFile.Tasks[j].ID)
	})

	// Marshal with stable formatting
	formattedData, err := json.MarshalIndent(todoFile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling todo file: %w", err)
	}
	formattedData = append(formattedData, '\n')

	// Check if formatting would change the file
	needsFormat := !jsonEqual(originalData, formattedData)

	if *check {
		if needsFormat {
			fmt.Printf("%s: needs formatting\n", todoPath)
			return fmt.Errorf("file is not formatted")
		}
		fmt.Printf("%s: formatted\n", todoPath)
		return nil
	}

	if *diff {
		if needsFormat {
			printDiff(todoPath, originalData, formattedData)
		} else {
			fmt.Printf("%s: already formatted\n", todoPath)
		}
		return nil
	}

	if *write {
		if needsFormat {
			if err := os.WriteFile(todoPath, formattedData, 0644); err != nil {
				return fmt.Errorf("writing todo file: %w", err)
			}
			fmt.Printf("%s: formatted\n", todoPath)
		} else {
			fmt.Printf("%s: already formatted\n", todoPath)
		}
		return nil
	}

	// Default: show if file needs formatting
	if needsFormat {
		fmt.Printf("%s: needs formatting\n", todoPath)
		fmt.Println("Use -w to write formatted file, or -d to see diffs")
		return fmt.Errorf("file is not formatted")
	}
	fmt.Printf("%s: formatted\n", todoPath)
	return nil
}

// jsonEqual compares two JSON byte slices for semantic equality.
func jsonEqual(a, b []byte) bool {
	var va, vb interface{}
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	return fmt.Sprint(va) == fmt.Sprint(vb)
}

// printDiff prints a unified diff between two byte slices.
func printDiff(path string, original, formatted []byte) {
	fmt.Printf("--- %s\n", path)
	fmt.Printf("+++ %s\n", path)

	origLines := strings.Split(string(original), "\n")
	newLines := strings.Split(string(formatted), "\n")

	maxLines := len(origLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	changes := false
	for i := 0; i < maxLines; i++ {
		origLine := ""
		newLine := ""
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if origLine != newLine {
			changes = true
			if origLine != "" {
				fmt.Printf("-%s\n", origLine)
			}
			if newLine != "" {
				fmt.Printf("+%s\n", newLine)
			}
		}
	}

	if !changes {
		fmt.Printf("%s: no changes\n", path)
	}
}

// configCommand shows the effective configuration with source information.
func configCommand(cfg *config.Config, allArgs []string, args []string) error {
	// Parse config-specific flags
	fs := flag.NewFlagSet("looper config", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	// Reload config with source tracking
	// We need to create a new flag set to parse original args
	configFS := flag.NewFlagSet("looper", flag.ContinueOnError)
	configWithSources, err := config.LoadWithSources(configFS, allArgs)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if *jsonOutput {
		return printConfigJSON(configWithSources)
	}

	return printConfigTable(configWithSources)
}

// printConfigTable prints configuration in a human-readable table format.
func printConfigTable(cws *config.ConfigWithSources) error {
	cfg := cws.Config

	fmt.Println("Effective Configuration")
	fmt.Println("======================")
	fmt.Println()

	// Config file info
	configFile := cws.GetConfigFile()
	if configFile != "" {
		fmt.Printf("Config file: %s\n", configFile)
	} else {
		fmt.Println("Config file: (none)")
	}
	fmt.Println()

	// Paths section
	fmt.Println("Paths:")
	printConfigItem("Todo file", cfg.TodoFile, cws.Sources["todo_file"])
	printConfigItem("Schema file", cfg.SchemaFile, cws.Sources["schema_file"])
	printConfigItem("Log directory", cfg.LogDir, cws.Sources["log_dir"])
	fmt.Println()

	// Loop settings section
	fmt.Println("Loop Settings:")
	printConfigItem("Max iterations", fmt.Sprintf("%d", cfg.MaxIterations), cws.Sources["max_iterations"])
	printConfigItem("Schedule", cfg.Schedule, cws.Sources["schedule"])
	printConfigItem("Repair agent", cfg.RepairAgent, cws.Sources["repair_agent"])

	printConfigItem("Review agent", cfg.GetReviewAgent(), cws.Sources["review_agent"])
	printConfigItem("Bootstrap agent", cfg.GetBootstrapAgent(), cws.Sources["bootstrap_agent"])
	fmt.Println()

	// Schedule options
	if cfg.Schedule == "odd-even" || cfg.Schedule == "round-robin" {
		fmt.Println("Schedule Options:")
		if cfg.Schedule == "odd-even" {
			printConfigItem("Odd agent", effectiveOddAgent(cfg), cws.Sources["odd_agent"])
			printConfigItem("Even agent", effectiveEvenAgent(cfg), cws.Sources["even_agent"])
		} else if cfg.Schedule == "round-robin" {
			printConfigItem("Round-robin agents", strings.Join(effectiveRRAgents(cfg), ","), cws.Sources["rr_agents"])
		}
		fmt.Println()
	}

	// Agents section
	fmt.Println("Agents:")
	printAgentConfig("Codex", cfg.Agents.GetAgent("codex"), cws, "codex_binary", "codex_model", "codex_reasoning", "codex_args")
	printAgentConfig("Claude", cfg.Agents.GetAgent("claude"), cws, "claude_binary", "claude_model", "", "claude_args")
	fmt.Println()

	// Output section
	fmt.Println("Output:")
	printConfigItem("Apply summary", fmt.Sprintf("%t", cfg.ApplySummary), cws.Sources["apply_summary"])
	fmt.Println()

	// Git section
	fmt.Println("Git:")
	printConfigItem("Git init", fmt.Sprintf("%t", cfg.GitInit), cws.Sources["git_init"])
	fmt.Println()

	// Hooks section
	if cfg.HookCommand != "" {
		fmt.Println("Hooks:")
		printConfigItem("Hook command", cfg.HookCommand, cws.Sources["hook_command"])
		fmt.Println()
	}

	// Delay section
	if cfg.LoopDelaySeconds > 0 {
		fmt.Println("Timing:")
		printConfigItem("Loop delay", fmt.Sprintf("%d seconds", cfg.LoopDelaySeconds), cws.Sources["loop_delay_seconds"])
		fmt.Println()
	}

	return nil
}

// printConfigItem prints a single configuration item with its source.
func printConfigItem(label, value string, source config.ConfigSource) {
	fmt.Printf("  %-20s %s [%s]\n", label+":", value, source)
}

func effectiveOddAgent(cfg *config.Config) string {
	if agent := normalizeAgent(cfg.OddAgent); agent != "" {
		return agent
	}
	return "codex"
}

func effectiveEvenAgent(cfg *config.Config) string {
	if agent := normalizeAgent(cfg.EvenAgent); agent != "" {
		return agent
	}
	return "claude"
}

func effectiveRRAgents(cfg *config.Config) []string {
	if agents := normalizeAgentList(cfg.RRAgents); len(agents) > 0 {
		return agents
	}
	return []string{"claude", "codex"}
}

// printAgentConfig prints agent configuration.
func printAgentConfig(name string, agent config.Agent, cws *config.ConfigWithSources, binaryField, modelField, reasoningField, argsField string) {
	binary := agent.Binary
	if binary == "" {
		binary = config.DefaultAgentBinaries()[strings.ToLower(name)]
	}
	fmt.Printf("  %s:\n", name)
	printConfigItem("  Binary", binary, cws.Sources[binaryField])
	if agent.Model != "" {
		printConfigItem("  Model", agent.Model, cws.Sources[modelField])
	}
	if reasoningField != "" && agent.Reasoning != "" {
		printConfigItem("  Reasoning", agent.Reasoning, cws.Sources[reasoningField])
	}
	if len(agent.Args) > 0 {
		printConfigItem("  Args", strings.Join(agent.Args, ", "), cws.Sources[argsField])
	}
}

// printConfigJSON prints configuration in JSON format.
func printConfigJSON(cws *config.ConfigWithSources) error {
	cfg := cws.Config

	// Build JSON output
	output := map[string]interface{}{
		"config_file": cws.GetConfigFile(),
		"paths": map[string]interface{}{
			"todo_file":   map[string]interface{}{"value": cfg.TodoFile, "source": cws.Sources["todo_file"]},
			"schema_file": map[string]interface{}{"value": cfg.SchemaFile, "source": cws.Sources["schema_file"]},
			"log_dir":     map[string]interface{}{"value": cfg.LogDir, "source": cws.Sources["log_dir"]},
		},
		"loop_settings": map[string]interface{}{
			"max_iterations":  map[string]interface{}{"value": cfg.MaxIterations, "source": cws.Sources["max_iterations"]},
			"schedule":        map[string]interface{}{"value": cfg.Schedule, "source": cws.Sources["schedule"]},
			"repair_agent":    map[string]interface{}{"value": cfg.RepairAgent, "source": cws.Sources["repair_agent"]},
			"review_agent":    map[string]interface{}{"value": cfg.GetReviewAgent(), "source": cws.Sources["review_agent"]},
			"bootstrap_agent": map[string]interface{}{"value": cfg.GetBootstrapAgent(), "source": cws.Sources["bootstrap_agent"]},
		},
		"agents": map[string]interface{}{
			"codex": map[string]interface{}{
				"binary":    map[string]interface{}{"value": cfg.Agents.GetAgent("codex").Binary, "source": cws.Sources["codex_binary"]},
				"model":     map[string]interface{}{"value": cfg.Agents.GetAgent("codex").Model, "source": cws.Sources["codex_model"]},
				"reasoning": map[string]interface{}{"value": cfg.Agents.GetAgent("codex").Reasoning, "source": cws.Sources["codex_reasoning"]},
				"args":      map[string]interface{}{"value": cfg.Agents.GetAgent("codex").Args, "source": cws.Sources["codex_args"]},
			},
			"claude": map[string]interface{}{
				"binary": map[string]interface{}{"value": cfg.Agents.GetAgent("claude").Binary, "source": cws.Sources["claude_binary"]},
				"model":  map[string]interface{}{"value": cfg.Agents.GetAgent("claude").Model, "source": cws.Sources["claude_model"]},
				"args":   map[string]interface{}{"value": cfg.Agents.GetAgent("claude").Args, "source": cws.Sources["claude_args"]},
			},
		},
		"output": map[string]interface{}{
			"apply_summary": map[string]interface{}{"value": cfg.ApplySummary, "source": cws.Sources["apply_summary"]},
		},
		"git": map[string]interface{}{
			"git_init": map[string]interface{}{"value": cfg.GitInit, "source": cws.Sources["git_init"]},
		},
	}

	if cfg.HookCommand != "" {
		output["hooks"] = map[string]interface{}{
			"hook_command": map[string]interface{}{"value": cfg.HookCommand, "source": cws.Sources["hook_command"]},
		}
	}

	if cfg.LoopDelaySeconds > 0 {
		output["timing"] = map[string]interface{}{
			"loop_delay_seconds": map[string]interface{}{"value": cfg.LoopDelaySeconds, "source": cws.Sources["loop_delay_seconds"]},
		}
	}

	// Handle schedule options
	if cfg.Schedule == "odd-even" {
		output["schedule_options"] = map[string]interface{}{
			"odd_agent":  map[string]interface{}{"value": effectiveOddAgent(cfg), "source": cws.Sources["odd_agent"]},
			"even_agent": map[string]interface{}{"value": effectiveEvenAgent(cfg), "source": cws.Sources["even_agent"]},
		}
	} else if cfg.Schedule == "round-robin" {
		output["schedule_options"] = map[string]interface{}{
			"rr_agents": map[string]interface{}{"value": effectiveRRAgents(cfg), "source": cws.Sources["rr_agents"]},
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// cleanCommand removes old log runs by age or count.
func cleanCommand(cfg *config.Config, args []string) error {
	// Parse clean-specific flags
	fs := flag.NewFlagSet("looper clean", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "Show what would be deleted without deleting")
	keepCount := fs.Int("keep", 0, "Number of recent runs to keep (0 = delete all)")
	keepAge := fs.String("age", "", "Delete logs older than duration (e.g., 7d, 24h, 30m)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	// Determine work directory
	workDir := cfg.ProjectRoot
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		workDir = wd
	}

	// Find the log directory
	logDir, err := logging.FindLogDir(cfg.LogDir, workDir)
	if err != nil {
		return fmt.Errorf("finding log directory: %w", err)
	}

	// Check if log directory exists
	if _, err := os.Stat(logDir); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log directory found.")
			return nil
		}
		return fmt.Errorf("accessing log directory: %w", err)
	}

	// Parse age duration if specified
	var ageCutoff time.Time
	if *keepAge != "" {
		d, err := parseRetentionDuration(*keepAge)
		if err != nil {
			return fmt.Errorf("parsing age duration: %w\nSupported: 7d, 24h, 30m, etc.", err)
		}
		ageCutoff = time.Now().Add(-d)
	}

	// Find all log runs
	runs, err := logging.FindLogRuns(logDir)
	if err != nil {
		return fmt.Errorf("finding log runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No log runs found.")
		return nil
	}

	// Filter runs based on criteria
	var toDelete []logging.LogRun
	var toKeep []logging.LogRun

	for _, run := range runs {
		shouldDelete := false

		// Check age filter
		if !ageCutoff.IsZero() && run.ModTime.Before(ageCutoff) {
			shouldDelete = true
		}

		// Check count filter (only if not already marked for deletion)
		if !shouldDelete && *keepCount > 0 {
			// Runs are sorted by modtime descending (newest first)
			// So we keep the first keepCount runs
			if len(toKeep) >= *keepCount {
				shouldDelete = true
			}
		}

		// Also delete if no filters are specified (confirm with user)
		if *keepCount == 0 && *keepAge == "" {
			shouldDelete = true
		}

		if shouldDelete {
			toDelete = append(toDelete, run)
		} else {
			toKeep = append(toKeep, run)
		}
	}

	// If no filters specified, ask for confirmation
	if *keepCount == 0 && *keepAge == "" && !*dryRun {
		if len(toDelete) == 0 {
			fmt.Println("No log runs to delete.")
			return nil
		}
		fmt.Printf("This will delete all %d log run(s). Continue? [y/N] ", len(toDelete))
		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Print summary
	fmt.Println("Log Cleanup Summary")
	fmt.Println("===================")
	fmt.Printf("Log directory: %s\n", logDir)
	fmt.Printf("Total runs: %d\n", len(runs))
	fmt.Printf("To keep: %d\n", len(toKeep))
	fmt.Printf("To delete: %d\n", len(toDelete))
	if *keepCount > 0 {
		fmt.Printf("Keep count: %d\n", *keepCount)
	}
	if *keepAge != "" {
		fmt.Printf("Age cutoff: %s (logs older than %s)\n", ageCutoff.Format(time.RFC3339), *keepAge)
	}
	if *dryRun {
		fmt.Println("Dry run: no files will be deleted")
	}
	fmt.Println()

	// List files to delete
	if len(toDelete) > 0 {
		fmt.Println("Files to be deleted:")
		for _, run := range toDelete {
			age := time.Since(run.ModTime)
			ageStr := formatDuration(age)
			fmt.Printf("  - %s (modified %s ago, %d files)\n", run.RunID, ageStr, len(run.Files))
		}
		fmt.Println()
	}

	// Delete files
	if !*dryRun && len(toDelete) > 0 {
		var deleted int
		var totalSize int64
		for _, run := range toDelete {
			for _, file := range run.Files {
				info, err := os.Stat(file)
				if err != nil {
					fmt.Printf("Warning: could not stat %s: %v\n", file, err)
					continue
				}
				totalSize += info.Size()

				if err := os.Remove(file); err != nil {
					fmt.Printf("Warning: could not delete %s: %v\n", file, err)
					continue
				}
				deleted++
			}
			// Also remove the last message files
			for _, lastFile := range run.LastMessageFiles {
				if err := os.Remove(lastFile); err != nil {
					// Ignore errors for last message files (may not exist)
				}
			}
		}

		fmt.Printf("Deleted %d file(s), freed %s\n", deleted, formatBytes(totalSize))
	}

	return nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func parseRetentionDuration(input string) (time.Duration, error) {
	if strings.TrimSpace(input) == "" {
		return 0, fmt.Errorf("duration is empty")
	}

	if !strings.Contains(input, "d") {
		return time.ParseDuration(input)
	}

	normalized, err := normalizeDurationWithDays(input)
	if err != nil {
		return 0, err
	}

	return time.ParseDuration(normalized)
}

func normalizeDurationWithDays(input string) (string, error) {
	if !strings.Contains(input, "d") {
		return input, nil
	}

	var b strings.Builder
	for i := 0; i < len(input); {
		c := input[i]
		if (c >= '0' && c <= '9') || c == '.' {
			start := i
			for i < len(input) {
				c = input[i]
				if (c >= '0' && c <= '9') || c == '.' {
					i++
					continue
				}
				break
			}

			if i < len(input) && input[i] == 'd' {
				numStr := input[start:i]
				value, err := strconv.ParseFloat(numStr, 64)
				if err != nil {
					return "", fmt.Errorf("invalid day duration %q: %w", numStr, err)
				}
				hours := value * 24
				b.WriteString(strconv.FormatFloat(hours, 'f', -1, 64))
				b.WriteByte('h')
				i++
				continue
			}

			b.WriteString(input[start:i])
			continue
		}

		b.WriteByte(c)
		i++
	}

	return b.String(), nil
}

// formatBytes formats a byte size in a human-readable way.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
