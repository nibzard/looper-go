package config

import (
	"flag"
	"strings"

	"github.com/nibzard/looper-go/internal/utils"
)

// parseFlags defines and parses CLI flags.
func parseFlags(cfg *Config, fs *flag.FlagSet, args []string) error {
	return parseFlagsHelper(cfg, fs, args, nil, "")
}

// parseFlagsWithSources parses CLI flags and updates source tracking.
func parseFlagsWithSources(cfg *Config, fs *flag.FlagSet, args []string, sources map[string]ConfigSource) error {
	return parseFlagsHelper(cfg, fs, args, sources, SourceFlag)
}

// parseFlagsHelper is the shared implementation for flag parsing.
// If sources is non-nil, it tracks the source of each value.
func parseFlagsHelper(cfg *Config, fs *flag.FlagSet, args []string, sources map[string]ConfigSource, source ConfigSource) error {
	if fs == nil {
		fs = flag.NewFlagSet("looper", flag.ContinueOnError)
	}

	// Track which flags are explicitly set (only used when sources != nil)
	flagSet := make(map[string]bool)

	// Flag binding struct for source tracking
	type flagBinding struct {
		name   string
		target interface{}
		usage  string
	}
	var bindings []flagBinding

	// Path flags
	var todoFile, schemaFile, logDir string
	if sources == nil {
		// Direct binding for non-source-tracking case
		fs.StringVar(&cfg.TodoFile, "todo", cfg.TodoFile, "Path to task file")
		fs.StringVar(&cfg.SchemaFile, "schema", cfg.SchemaFile, "Path to schema file")
		fs.StringVar(&cfg.LogDir, "log-dir", cfg.LogDir, "Log directory")
	} else {
		fs.StringVar(&todoFile, "todo", cfg.TodoFile, "")
		fs.StringVar(&schemaFile, "schema", cfg.SchemaFile, "")
		fs.StringVar(&logDir, "log-dir", cfg.LogDir, "")
		bindings = append(bindings,
			flagBinding{name: "todo", target: &todoFile},
			flagBinding{name: "schema", target: &schemaFile},
			flagBinding{name: "log-dir", target: &logDir},
		)
	}

	// Dev-only flags
	var promptDir string
	var printPrompt bool
	if devModeEnabled() {
		if sources == nil {
			fs.StringVar(&cfg.PromptDir, "prompt-dir", cfg.PromptDir, "Prompt directory override (dev only)")
			fs.BoolVar(&cfg.PrintPrompt, "print-prompt", cfg.PrintPrompt, "Print rendered prompts before running (dev only)")
		} else {
			fs.StringVar(&promptDir, "prompt-dir", cfg.PromptDir, "")
			fs.BoolVar(&printPrompt, "print-prompt", cfg.PrintPrompt, "")
			bindings = append(bindings,
				flagBinding{name: "prompt-dir", target: &promptDir},
				flagBinding{name: "print-prompt", target: &printPrompt},
			)
		}
	}

	// Loop settings
	var maxIter int
	if sources == nil {
		fs.IntVar(&cfg.MaxIterations, "max-iterations", cfg.MaxIterations, "Maximum iterations")
	} else {
		fs.IntVar(&maxIter, "max-iterations", cfg.MaxIterations, "")
		bindings = append(bindings, flagBinding{name: "max-iterations", target: &maxIter})
	}

	// Output
	var applySummary bool
	if sources == nil {
		fs.BoolVar(&cfg.ApplySummary, "apply-summary", cfg.ApplySummary, "Apply summaries to task file")
	} else {
		fs.BoolVar(&applySummary, "apply-summary", cfg.ApplySummary, "")
		bindings = append(bindings, flagBinding{name: "apply-summary", target: &applySummary})
	}

	// Git
	var gitInit bool
	if sources == nil {
		fs.BoolVar(&cfg.GitInit, "git-init", cfg.GitInit, "Initialize git repo if missing")
	} else {
		fs.BoolVar(&gitInit, "git-init", cfg.GitInit, "")
		bindings = append(bindings, flagBinding{name: "git-init", target: &gitInit})
	}

	// Hooks
	var hook string
	if sources == nil {
		fs.StringVar(&cfg.HookCommand, "hook", cfg.HookCommand, "Hook command to run after each iteration")
	} else {
		fs.StringVar(&hook, "hook", cfg.HookCommand, "")
		bindings = append(bindings, flagBinding{name: "hook", target: &hook})
	}

	// Delay
	var loopDelay int
	if sources == nil {
		fs.IntVar(&cfg.LoopDelaySeconds, "loop-delay", cfg.LoopDelaySeconds, "Delay between iterations (seconds)")
	} else {
		fs.IntVar(&loopDelay, "loop-delay", cfg.LoopDelaySeconds, "")
		bindings = append(bindings, flagBinding{name: "loop-delay", target: &loopDelay})
	}

	// Logging
	var logLevel, logFormat string
	var logTimestamps, logCaller bool
	if sources == nil {
		fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")
		fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "Log format (text, json, logfmt)")
		fs.BoolVar(&cfg.LogTimestamps, "log-timestamps", cfg.LogTimestamps, "Show timestamps in logs")
		fs.BoolVar(&cfg.LogCaller, "log-caller", cfg.LogCaller, "Show caller location in logs")
	} else {
		fs.StringVar(&logLevel, "log-level", cfg.LogLevel, "")
		fs.StringVar(&logFormat, "log-format", cfg.LogFormat, "")
		fs.BoolVar(&logTimestamps, "log-timestamps", cfg.LogTimestamps, "")
		fs.BoolVar(&logCaller, "log-caller", cfg.LogCaller, "")
		bindings = append(bindings,
			flagBinding{name: "log-level", target: &logLevel},
			flagBinding{name: "log-format", target: &logFormat},
			flagBinding{name: "log-timestamps", target: &logTimestamps},
			flagBinding{name: "log-caller", target: &logCaller},
		)
	}

	// Agents
	codexBinary := cfg.GetAgentBinary("codex")
	claudeBinary := cfg.GetAgentBinary("claude")
	codexModel := cfg.GetAgentModel("codex")
	claudeModel := cfg.GetAgentModel("claude")
	codexReasoning := cfg.GetAgentReasoning("codex")
	codexArgsStr := strings.Join(cfg.GetAgentArgs("codex"), ",")
	claudeArgsStr := strings.Join(cfg.GetAgentArgs("claude"), ",")

	usage := func(s string) string {
		if sources == nil {
			return s
		}
		return ""
	}

	fs.StringVar(&codexBinary, "codex-bin", codexBinary, usage("Codex binary"))
	fs.StringVar(&claudeBinary, "claude-bin", claudeBinary, usage("Claude binary"))
	fs.StringVar(&codexModel, "codex-model", codexModel, usage("Codex model"))
	fs.StringVar(&claudeModel, "claude-model", claudeModel, usage("Claude model"))
	fs.StringVar(&codexReasoning, "codex-reasoning", codexReasoning, usage("Codex reasoning effort (e.g., low, medium, high)"))
	fs.StringVar(&codexArgsStr, "codex-args", codexArgsStr, usage("Comma-separated extra args for codex (e.g., --foo,bar)"))
	fs.StringVar(&claudeArgsStr, "claude-args", claudeArgsStr, usage("Comma-separated extra args for claude (e.g., --foo,bar)"))

	bindings = append(bindings,
		flagBinding{name: "codex-bin", target: &codexBinary},
		flagBinding{name: "claude-bin", target: &claudeBinary},
		flagBinding{name: "codex-model", target: &codexModel},
		flagBinding{name: "claude-model", target: &claudeModel},
		flagBinding{name: "codex-reasoning", target: &codexReasoning},
		flagBinding{name: "codex-args", target: &codexArgsStr},
		flagBinding{name: "claude-args", target: &claudeArgsStr},
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Map flag names to source field names
	flagToSource := map[string]string{
		"todo":            "todo_file",
		"schema":          "schema_file",
		"log-dir":         "log_dir",
		"max-iterations":  "max_iterations",
		"apply-summary":   "apply_summary",
		"git-init":        "git_init",
		"hook":            "hook_command",
		"loop-delay":      "loop_delay_seconds",
		"log-level":       "log_level",
		"log-format":      "log_format",
		"log-timestamps":  "log_timestamps",
		"log-caller":      "log_caller",
		"codex-bin":       "codex_binary",
		"claude-bin":      "claude_binary",
		"codex-model":     "codex_model",
		"claude-model":    "claude_model",
		"codex-reasoning": "codex_reasoning",
		"codex-args":      "codex_args",
		"claude-args":     "claude_args",
	}

	// Track which flags were set and apply to config
	fs.Visit(func(f *flag.Flag) {
		flagSet[f.Name] = true
		if sources == nil {
			return
		}
		if fieldName, ok := flagToSource[f.Name]; ok {
			sources[fieldName] = source
		}
	})

	// Apply flag values to config
	if sources == nil {
		// Direct binding already applied
		codexArgs := utils.SplitAndTrim(codexArgsStr, ",")
		claudeArgs := utils.SplitAndTrim(claudeArgsStr, ",")
		cfg.Agents.SetAgent("codex", Agent{Binary: codexBinary, Model: codexModel, Reasoning: codexReasoning, Args: codexArgs})
		cfg.Agents.SetAgent("claude", Agent{Binary: claudeBinary, Model: claudeModel, Args: claudeArgs})
	} else {
		// Apply based on which flags were set
		if flagSet["todo"] {
			cfg.TodoFile = todoFile
		}
		if flagSet["schema"] {
			cfg.SchemaFile = schemaFile
		}
		if flagSet["log-dir"] {
			cfg.LogDir = logDir
		}
		if flagSet["prompt-dir"] {
			cfg.PromptDir = promptDir
		}
		if flagSet["print-prompt"] {
			cfg.PrintPrompt = printPrompt
		}
		if flagSet["max-iterations"] {
			cfg.MaxIterations = maxIter
		}
		if flagSet["apply-summary"] {
			cfg.ApplySummary = applySummary
		}
		if flagSet["git-init"] {
			cfg.GitInit = gitInit
		}
		if flagSet["hook"] {
			cfg.HookCommand = hook
		}
		if flagSet["loop-delay"] {
			cfg.LoopDelaySeconds = loopDelay
		}
		if flagSet["log-level"] {
			cfg.LogLevel = logLevel
		}
		if flagSet["log-format"] {
			cfg.LogFormat = logFormat
		}
		if flagSet["log-timestamps"] {
			cfg.LogTimestamps = logTimestamps
		}
		if flagSet["log-caller"] {
			cfg.LogCaller = logCaller
		}

		// Agent flags - only update values when explicitly set
		if flagSet["codex-bin"] || flagSet["codex-model"] || flagSet["codex-reasoning"] || flagSet["codex-args"] {
			codexAgent := cfg.Agents.GetAgent("codex")
			if flagSet["codex-bin"] {
				codexAgent.Binary = codexBinary
			}
			if flagSet["codex-model"] {
				codexAgent.Model = codexModel
			}
			if flagSet["codex-reasoning"] {
				codexAgent.Reasoning = codexReasoning
			}
			if flagSet["codex-args"] {
				codexAgent.Args = utils.SplitAndTrim(codexArgsStr, ",")
			}
			cfg.Agents.SetAgent("codex", codexAgent)
		}
		if flagSet["claude-bin"] || flagSet["claude-model"] || flagSet["claude-args"] {
			claudeAgent := cfg.Agents.GetAgent("claude")
			if flagSet["claude-bin"] {
				claudeAgent.Binary = claudeBinary
			}
			if flagSet["claude-model"] {
				claudeAgent.Model = claudeModel
			}
			if flagSet["claude-args"] {
				claudeAgent.Args = utils.SplitAndTrim(claudeArgsStr, ",")
			}
			cfg.Agents.SetAgent("claude", claudeAgent)
		}
	}

	return nil
}
