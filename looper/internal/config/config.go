// Package config handles configuration loading and defaults.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Default values.
const (
	DefaultMaxIterations = 50
	DefaultTodoFile      = "to-do.json"
	DefaultLogDir        = "~/.looper"
	DefaultApplySummary  = true
)

// DefaultAgentBinaries returns the default binary names for each agent type.
func DefaultAgentBinaries() map[string]string {
	return map[string]string{
		"codex":  "codex",
		"claude": "claude",
	}
}

// Config holds the full configuration for looper.
type Config struct {
	// Paths
	TodoFile   string `toml:"todo_file"`
	SchemaFile string `toml:"schema_file"`
	LogDir     string `toml:"log_dir"`
	PromptDir  string `toml:"-"` // Hidden, dev-only

	// Loop settings
	MaxIterations int    `toml:"max_iterations"`
	Schedule      string `toml:"schedule"` // codex, claude, odd-even, round-robin
	RepairAgent   string `toml:"repair_agent"` // codex or claude

	// Agents
	Agents AgentConfig `toml:"agents"`

	// Output
	ApplySummary bool `toml:"apply_summary"`

	// Git
	GitInit bool `toml:"git_init"`

	// Hooks
	HookCommand string `toml:"hook_command"`

	// Delay between iterations
	LoopDelaySeconds int `toml:"loop_delay_seconds"`

	// Project root (computed)
	ProjectRoot string `toml:"-"`
}

// AgentConfig holds agent-specific configuration.
type AgentConfig struct {
	Codex  Agent `toml:"codex"`
	Claude Agent `toml:"claude"`
}

// Agent holds configuration for a single agent type.
type Agent struct {
	Binary string `toml:"binary"`
	Model  string `toml:"model"`
	// Additional flags can be added here as needed
}

// IterSchedule returns the agent for a given iteration number.
func (c *Config) IterSchedule(iter int) string {
	switch c.Schedule {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	case "odd-even":
		if iter%2 == 1 {
			return "codex" // odd
		}
		return "claude" // even
	case "round-robin":
		// Simple alternation: codex, claude, codex, claude...
		// TODO: support custom round-robin lists
		if iter%2 == 1 {
			return "codex"
		}
		return "claude"
	default:
		return "codex"
	}
}

// ReviewAgent returns the agent to use for the review pass.
func (c *Config) ReviewAgent() string {
	// Review always uses codex per spec
	return "codex"
}

// Load loads configuration from multiple sources in priority order:
// 1. Defaults
// 2. Config file (TOML)
// 3. Environment variables
// 4. CLI flags
func Load(fs *flag.FlagSet, args []string) (*Config, error) {
	cfg := &Config{}

	// 1. Set defaults
	setDefaults(cfg)

	// 2. Try to load from config file
	configFile := findConfigFile()
	if configFile != "" {
		if err := loadConfigFile(cfg, configFile); err != nil {
			return nil, fmt.Errorf("loading config file %s: %w", configFile, err)
		}
	}

	// 3. Override from environment
	loadFromEnv(cfg)

	// 4. Parse CLI flags (they override everything)
	if err := parseFlags(cfg, fs, args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	// 5. Compute derived values
	if err := finalizeConfig(cfg); err != nil {
		return nil, fmt.Errorf("finalizing config: %w", err)
	}

	return cfg, nil
}

// setDefaults applies default values to the config.
func setDefaults(cfg *Config) {
	cfg.TodoFile = DefaultTodoFile
	cfg.SchemaFile = "to-do.schema.json"
	cfg.LogDir = DefaultLogDir
	cfg.MaxIterations = DefaultMaxIterations
	cfg.Schedule = "codex"
	cfg.RepairAgent = "codex"
	cfg.ApplySummary = DefaultApplySummary
	cfg.GitInit = true
	cfg.LoopDelaySeconds = 0

	// Default agent binaries
	cfg.Agents.Codex.Binary = DefaultAgentBinaries()["codex"]
	cfg.Agents.Claude.Binary = DefaultAgentBinaries()["claude"]
}

// findConfigFile looks for a config file in the current directory.
func findConfigFile() string {
	// Check for looper.toml in current directory
	names := []string{"looper.toml", ".looper.toml"}
	for _, name := range names {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

// loadConfigFile loads TOML config from the given file.
func loadConfigFile(cfg *Config, path string) error {
	_, err := toml.DecodeFile(path, cfg)
	return err
}

// loadFromEnv overrides config from environment variables.
func loadFromEnv(cfg *Config) {
	if v := os.Getenv("LOOPER_TODO"); v != "" {
		cfg.TodoFile = v
	}
	if v := os.Getenv("LOOPER_SCHEMA"); v != "" {
		cfg.SchemaFile = v
	}
	if v := os.Getenv("LOOPER_BASE_DIR"); v != "" {
		cfg.LogDir = v
	}
	if v := os.Getenv("LOOPER_LOG_DIR"); v != "" {
		cfg.LogDir = v
	}
	if v := os.Getenv("LOOPER_PROMPT_DIR"); v != "" {
		cfg.PromptDir = v
	}
	if v := os.Getenv("LOOPER_MAX_ITERATIONS"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.MaxIterations = i
		}
	}
	if v := os.Getenv("LOOPER_SCHEDULE"); v != "" {
		cfg.Schedule = v
	}
	if v := os.Getenv("LOOPER_REPAIR_AGENT"); v != "" {
		cfg.RepairAgent = v
	}
	if v := os.Getenv("LOOPER_APPLY_SUMMARY"); v != "" {
		cfg.ApplySummary = boolFromString(v)
	}
	if v := os.Getenv("LOOPER_GIT_INIT"); v != "" {
		cfg.GitInit = boolFromString(v)
	}
	if v := os.Getenv("LOOPER_HOOK"); v != "" {
		cfg.HookCommand = v
	}
	if v := os.Getenv("LOOPER_LOOP_DELAY"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.LoopDelaySeconds = i
		}
	}
	if v := os.Getenv("CODEX_BIN"); v != "" {
		cfg.Agents.Codex.Binary = v
	}
	if v := os.Getenv("CLAUDE_BIN"); v != "" {
		cfg.Agents.Claude.Binary = v
	}
	if v := os.Getenv("CODEX_MODEL"); v != "" {
		cfg.Agents.Codex.Model = v
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		cfg.Agents.Claude.Model = v
	}
}

// boolFromString parses a boolean from a string.
func boolFromString(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

// parseFlags defines and parses CLI flags.
func parseFlags(cfg *Config, fs *flag.FlagSet, args []string) error {
	if fs == nil {
		fs = flag.NewFlagSet("looper", flag.ContinueOnError)
	}

	// Path flags
	fs.StringVar(&cfg.TodoFile, "todo", cfg.TodoFile, "Path to task file")
	fs.StringVar(&cfg.SchemaFile, "schema", cfg.SchemaFile, "Path to schema file")
	fs.StringVar(&cfg.LogDir, "log-dir", cfg.LogDir, "Log directory")

	// Loop settings
	fs.IntVar(&cfg.MaxIterations, "max-iterations", cfg.MaxIterations, "Maximum iterations")
	fs.StringVar(&cfg.Schedule, "schedule", cfg.Schedule, "Iteration schedule (codex|claude|odd-even|round-robin)")
	fs.StringVar(&cfg.RepairAgent, "repair-agent", cfg.RepairAgent, "Agent for repair operations (codex|claude)")

	// Output
	fs.BoolVar(&cfg.ApplySummary, "apply-summary", cfg.ApplySummary, "Apply summaries to task file")

	// Git
	fs.BoolVar(&cfg.GitInit, "git-init", cfg.GitInit, "Initialize git repo if missing")

	// Hooks
	fs.StringVar(&cfg.HookCommand, "hook", cfg.HookCommand, "Hook command to run after each iteration")

	// Delay
	fs.IntVar(&cfg.LoopDelaySeconds, "loop-delay", cfg.LoopDelaySeconds, "Delay between iterations (seconds)")

	// Agents
	fs.StringVar(&cfg.Agents.Codex.Binary, "codex-bin", cfg.Agents.Codex.Binary, "Codex binary")
	fs.StringVar(&cfg.Agents.Claude.Binary, "claude-bin", cfg.Agents.Claude.Binary, "Claude binary")
	fs.StringVar(&cfg.Agents.Codex.Model, "codex-model", cfg.Agents.Codex.Model, "Codex model")
	fs.StringVar(&cfg.Agents.Claude.Model, "claude-model", cfg.Agents.Claude.Model, "Claude model")

	return fs.Parse(args)
}

// finalizeConfig computes derived values and validates paths.
func finalizeConfig(cfg *Config) error {
	// Expand ~ in paths
	cfg.LogDir = expandPath(cfg.LogDir)

	// Determine project root
	if cfg.ProjectRoot == "" {
		// Use current working directory
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		cfg.ProjectRoot = wd
	}

	// Make paths absolute if they're relative
	if !filepath.IsAbs(cfg.TodoFile) {
		cfg.TodoFile = filepath.Join(cfg.ProjectRoot, cfg.TodoFile)
	}
	if !filepath.IsAbs(cfg.SchemaFile) {
		cfg.SchemaFile = filepath.Join(cfg.ProjectRoot, cfg.SchemaFile)
	}

	return nil
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	return p
}

// GetAgentBinary returns the binary path for the given agent type.
func (c *Config) GetAgentBinary(agentType string) string {
	switch agentType {
	case "codex":
		return c.Agents.Codex.Binary
	case "claude":
		return c.Agents.Claude.Binary
	default:
		return ""
	}
}

// GetAgentModel returns the model for the given agent type.
func (c *Config) GetAgentModel(agentType string) string {
	switch agentType {
	case "codex":
		return c.Agents.Codex.Model
	case "claude":
		return c.Agents.Claude.Model
	default:
		return ""
	}
}
