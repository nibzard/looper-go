// Package config handles configuration loading and defaults.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	PromptDir  string `toml:"-"` // Hidden, dev-only (requires LOOPER_PROMPT_MODE=dev)

	// Dev options (hidden, require LOOPER_PROMPT_MODE=dev)
	PrintPrompt bool `toml:"-"` // Print rendered prompts before running

	// User prompt for bootstrap (not persisted in config file)
	UserPrompt string `toml:"-"` // User-provided prompt to drive bootstrap

	// Loop settings
	MaxIterations int    `toml:"max_iterations"`
	Schedule      string `toml:"schedule"` // codex, claude, odd-even, round-robin
	RepairAgent   string `toml:"repair_agent"` // codex or claude
	ReviewAgent   string `toml:"review_agent"` // codex or claude (default: codex)
	BootstrapAgent string `toml:"bootstrap_agent"` // codex or claude (default: codex)

	// Scheduling options for odd-even and round-robin
	OddAgent      string   `toml:"odd_agent"`      // agent for odd iterations (default: codex)
	EvenAgent     string   `toml:"even_agent"`     // agent for even iterations (default: claude)
	RRAgents      []string `toml:"rr_agents"`      // agent list for round-robin (default: claude,codex)

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
// For backward compatibility, it supports both the old fixed format (codex/claude fields)
// and the new map-based format (custom agents).
type AgentConfig struct {
	// Built-in agents (backward compatible)
	Codex  Agent          `toml:"codex"`
	Claude Agent          `toml:"claude"`
	Agents map[string]Agent `toml:"agents,omitempty"` // Custom agents by name
}

// GetAgent returns the configuration for a given agent type.
// It first checks the custom agents map, then falls back to built-in agents.
func (ac *AgentConfig) GetAgent(agentType string) Agent {
	// Check custom agents first
	if ac.Agents != nil {
		if agent, ok := ac.Agents[agentType]; ok {
			return agent
		}
	}
	// Fall back to built-in agents
	switch normalizeAgent(agentType) {
	case "codex":
		return ac.Codex
	case "claude":
		return ac.Claude
	default:
		// Return empty agent for unknown types
		return Agent{}
	}
}

// SetAgent sets the configuration for a given agent type.
// For built-in agents (codex, claude), it sets the respective field.
// For custom agents, it adds/updates the agents map.
func (ac *AgentConfig) SetAgent(agentType string, config Agent) {
	switch normalizeAgent(agentType) {
	case "codex":
		ac.Codex = config
	case "claude":
		ac.Claude = config
	default:
		if ac.Agents == nil {
			ac.Agents = make(map[string]Agent)
		}
		ac.Agents[agentType] = config
	}
}

// Agent holds configuration for a single agent type.
type Agent struct {
	Binary string `toml:"binary"`
	Model  string `toml:"model"`
	// Additional flags can be added here as needed
}

// IterSchedule returns the agent for a given iteration number.
func (c *Config) IterSchedule(iter int) string {
	switch normalizeSchedule(c.Schedule) {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	case "odd-even":
		if iter%2 == 1 {
			// Odd iteration (1, 3, 5, ...)
			if agent := normalizeAgent(c.OddAgent); agent != "" {
				return agent
			}
			return "codex"
		}
		// Even iteration (2, 4, 6, ...)
		if agent := normalizeAgent(c.EvenAgent); agent != "" {
			return agent
		}
		return "claude"
	case "round-robin":
		agents := normalizeAgentList(c.RRAgents)
		if len(agents) == 0 {
			// Default: claude, codex
			agents = []string{"claude", "codex"}
		}
		if len(agents) == 0 {
			return "codex"
		}
		// iter is 1-indexed, convert to 0-indexed for array access
		idx := (iter - 1) % len(agents)
		return agents[idx]
	default:
		return "codex"
	}
}

// GetReviewAgent returns the agent to use for the review pass.
// It returns the configured review_agent or defaults to "codex" if empty.
func (c *Config) GetReviewAgent() string {
	agent := normalizeAgent(c.ReviewAgent)
	if agent != "" {
		return agent
	}
	return "codex"
}

// GetBootstrapAgent returns the agent to use for bootstrap operations.
// It returns the configured bootstrap_agent or defaults to "codex" if empty.
func (c *Config) GetBootstrapAgent() string {
	agent := normalizeAgent(c.BootstrapAgent)
	if agent != "" {
		return agent
	}
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
	cfg.ReviewAgent = ""   // Empty means use default (codex)
	cfg.BootstrapAgent = "" // Empty means use default (codex)
	cfg.OddAgent = ""      // Empty means use default (codex)
	cfg.EvenAgent = ""     // Empty means use default (claude)
	cfg.RRAgents = nil     // nil means use default (claude,codex)
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
	if devModeEnabled() {
		if v := os.Getenv("LOOPER_PROMPT_DIR"); v != "" {
			cfg.PromptDir = v
		}
		if v := os.Getenv("LOOPER_PRINT_PROMPT"); v != "" {
			cfg.PrintPrompt = boolFromString(v)
		}
	}
	if v := os.Getenv("LOOPER_PROMPT"); v != "" {
		cfg.UserPrompt = v
	}
	if v := os.Getenv("LOOPER_MAX_ITERATIONS"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.MaxIterations = i
		}
	}
	if v := os.Getenv("LOOPER_ITER_SCHEDULE"); v != "" {
		cfg.Schedule = v
	} else if v := os.Getenv("LOOPER_SCHEDULE"); v != "" {
		cfg.Schedule = v
	}
	if v := os.Getenv("LOOPER_REPAIR_AGENT"); v != "" {
		cfg.RepairAgent = v
	}
	if v := os.Getenv("LOOPER_REVIEW_AGENT"); v != "" {
		cfg.ReviewAgent = v
	}
	if v := os.Getenv("LOOPER_BOOTSTRAP_AGENT"); v != "" {
		cfg.BootstrapAgent = v
	}
	if v := os.Getenv("LOOPER_ITER_ODD_AGENT"); v != "" {
		cfg.OddAgent = v
	}
	if v := os.Getenv("LOOPER_ITER_EVEN_AGENT"); v != "" {
		cfg.EvenAgent = v
	}
	if v := os.Getenv("LOOPER_ITER_RR_AGENTS"); v != "" {
		cfg.RRAgents = splitAndTrim(v, ",")
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

func normalizeSchedule(schedule string) string {
	s := strings.ToLower(strings.TrimSpace(schedule))
	switch s {
	case "odd_even", "odd-even", "oddeven":
		return "odd-even"
	case "round_robin", "round-robin", "roundrobin", "rr":
		return "round-robin"
	case "codex", "claude":
		return s
	default:
		return s
	}
}

func normalizeAgent(agent string) string {
	return strings.ToLower(strings.TrimSpace(agent))
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

// parseFlags defines and parses CLI flags.
func parseFlags(cfg *Config, fs *flag.FlagSet, args []string) error {
	if fs == nil {
		fs = flag.NewFlagSet("looper", flag.ContinueOnError)
	}

	// Path flags
	fs.StringVar(&cfg.TodoFile, "todo", cfg.TodoFile, "Path to task file")
	fs.StringVar(&cfg.SchemaFile, "schema", cfg.SchemaFile, "Path to schema file")
	fs.StringVar(&cfg.LogDir, "log-dir", cfg.LogDir, "Log directory")

	// Dev-only flags (only work when LOOPER_PROMPT_MODE=dev)
	if devModeEnabled() {
		fs.StringVar(&cfg.PromptDir, "prompt-dir", cfg.PromptDir, "Prompt directory override (dev only)")
		fs.BoolVar(&cfg.PrintPrompt, "print-prompt", cfg.PrintPrompt, "Print rendered prompts before running (dev only)")
	}

	// Loop settings
	fs.IntVar(&cfg.MaxIterations, "max-iterations", cfg.MaxIterations, "Maximum iterations")
	fs.StringVar(&cfg.Schedule, "schedule", cfg.Schedule, "Iteration schedule (agent name|odd-even|round-robin)")
	fs.StringVar(&cfg.RepairAgent, "repair-agent", cfg.RepairAgent, "Agent for repair operations")
	fs.StringVar(&cfg.ReviewAgent, "review-agent", cfg.ReviewAgent, "Agent for review pass")
	fs.StringVar(&cfg.BootstrapAgent, "bootstrap-agent", cfg.BootstrapAgent, "Agent for bootstrap operations")
	fs.StringVar(&cfg.OddAgent, "odd-agent", cfg.OddAgent, "Agent for odd iterations in odd-even schedule")
	fs.StringVar(&cfg.EvenAgent, "even-agent", cfg.EvenAgent, "Agent for even iterations in odd-even schedule")

	// Round-robin agents - need a custom var flag since StringVar doesn't handle slices
	var rrAgentsStr string
	if cfg.RRAgents != nil {
		rrAgentsStr = strings.Join(cfg.RRAgents, ",")
	}
	fs.StringVar(&rrAgentsStr, "rr-agents", rrAgentsStr, "Comma-separated agent list for round-robin schedule")

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

	if err := fs.Parse(args); err != nil {
		return err
	}
	if rrAgentsStr != "" {
		cfg.RRAgents = splitAndTrim(rrAgentsStr, ",")
	}
	return nil
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

// expandPath expands home directory and environment variables in paths.
// It supports ~/ or ~\ prefixes and %VAR% expansion on Windows.
func expandPath(p string) string {
	if p == "" {
		return p
	}

	expanded := expandEnv(p)
	if expanded == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return expanded
		}
		return home
	}
	if strings.HasPrefix(expanded, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(expanded, "~\\")) {
		home, err := os.UserHomeDir()
		if err != nil {
			return expanded
		}
		return filepath.Join(home, expanded[2:])
	}
	return expanded
}

func expandEnv(p string) string {
	expanded := os.ExpandEnv(p)
	if runtime.GOOS != "windows" {
		return expanded
	}
	return expandWindowsEnv(expanded)
}

func expandWindowsEnv(p string) string {
	if !strings.Contains(p, "%") {
		return p
	}
	var b strings.Builder
	for i := 0; i < len(p); {
		if p[i] == '%' {
			end := strings.IndexByte(p[i+1:], '%')
			if end >= 0 {
				key := p[i+1 : i+1+end]
				if key == "" {
					b.WriteByte('%')
					i++
					continue
				}
				if val, ok := os.LookupEnv(key); ok {
					b.WriteString(val)
				} else {
					b.WriteByte('%')
					b.WriteString(key)
					b.WriteByte('%')
				}
				i += end + 2
				continue
			}
		}
		b.WriteByte(p[i])
		i++
	}
	return b.String()
}

// GetAgentBinary returns the binary path for the given agent type.
// It checks both custom agents and built-in agents.
func (c *Config) GetAgentBinary(agentType string) string {
	return c.Agents.GetAgent(agentType).Binary
}

// GetAgentModel returns the model for the given agent type.
// It checks both custom agents and built-in agents.
func (c *Config) GetAgentModel(agentType string) string {
	return c.Agents.GetAgent(agentType).Model
}

// devModeEnabled returns true if dev mode is enabled via LOOPER_PROMPT_MODE=dev.
// Dev mode enables --prompt-dir and --print-prompt flags for prompt development.
func devModeEnabled() bool {
	return os.Getenv("LOOPER_PROMPT_MODE") == "dev"
}

// PromptDevModeEnabled reports whether prompt development options are enabled.
func PromptDevModeEnabled() bool {
	return devModeEnabled()
}
