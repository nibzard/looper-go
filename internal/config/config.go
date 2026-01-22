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

// ConfigSource represents where a configuration value came from.
type ConfigSource string

const (
	SourceDefault  ConfigSource = "default"
	SourceUserFile ConfigSource = "user file"
	SourceProjFile ConfigSource = "project file"
	SourceEnv      ConfigSource = "environment"
	SourceFlag     ConfigSource = "flag"
)

// ConfigWithSources holds configuration along with source information for each field.
type ConfigWithSources struct {
	Config  *Config
	Sources map[string]ConfigSource
}

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
	MaxIterations  int    `toml:"max_iterations"`
	Schedule       string `toml:"schedule"`        // codex, claude, odd-even, round-robin
	RepairAgent    string `toml:"repair_agent"`    // codex or claude
	ReviewAgent    string `toml:"review_agent"`    // codex or claude (default: codex)
	BootstrapAgent string `toml:"bootstrap_agent"` // codex or claude (default: codex)

	// Scheduling options for odd-even and round-robin
	OddAgent  string   `toml:"odd_agent"`  // agent for odd iterations (default: codex)
	EvenAgent string   `toml:"even_agent"` // agent for even iterations (default: claude)
	RRAgents  []string `toml:"rr_agents"`  // agent list for round-robin (default: claude,codex)

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
// It is a map keyed by agent type (codex, claude, or any registered custom agent).
type AgentConfig map[string]Agent

// UnmarshalTOML supports both the new map-based layout and the legacy
// agents.agents.<name> nested format for custom agents.
func (ac *AgentConfig) UnmarshalTOML(data interface{}) error {
	table, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("agents config must be a table")
	}
	if *ac == nil {
		*ac = AgentConfig{}
	}
	return mergeAgentTables(*ac, table)
}

// GetAgent returns the configuration for a given agent type.
func (ac AgentConfig) GetAgent(agentType string) Agent {
	if ac == nil {
		return Agent{}
	}
	key := normalizeAgent(agentType)
	if key == "" {
		return Agent{}
	}
	return ac[key]
}

// SetAgent sets the configuration for a given agent type.
func (ac *AgentConfig) SetAgent(agentType string, config Agent) {
	key := normalizeAgent(agentType)
	if key == "" {
		return
	}
	if *ac == nil {
		*ac = AgentConfig{}
	}
	(*ac)[key] = config
}

func mergeAgentTables(target AgentConfig, table map[string]interface{}) error {
	for key, value := range table {
		if key == "agents" {
			nested, ok := value.(map[string]interface{})
			if !ok {
				return fmt.Errorf("agents.agents must be a table")
			}
			if err := mergeAgentTables(target, nested); err != nil {
				return err
			}
			continue
		}

		raw, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		agent, err := decodeAgentConfig(raw)
		if err != nil {
			return fmt.Errorf("agent %s: %w", key, err)
		}
		target[normalizeAgent(key)] = agent
	}
	return nil
}

func decodeAgentConfig(raw map[string]interface{}) (Agent, error) {
	var agent Agent
	if raw == nil {
		return agent, nil
	}
	if v, ok := raw["binary"]; ok {
		binary, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("binary must be a string")
		}
		agent.Binary = binary
	}
	if v, ok := raw["model"]; ok {
		model, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("model must be a string")
		}
		agent.Model = model
	}
	if v, ok := raw["reasoning"]; ok {
		reasoning, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("reasoning must be a string")
		}
		agent.Reasoning = reasoning
	}
	if v, ok := raw["args"]; ok {
		args, err := parseArgsValue(v)
		if err != nil {
			return agent, err
		}
		agent.Args = args
	}
	return agent, nil
}

func parseArgsValue(v interface{}) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return filterEmptyArgs(val), nil
	case []interface{}:
		args := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("args must be a string array")
			}
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				args = append(args, trimmed)
			}
		}
		return args, nil
	case string:
		return splitAndTrim(val, ","), nil
	default:
		return nil, fmt.Errorf("args must be a string or string array")
	}
}

func filterEmptyArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

// Agent holds configuration for a single agent type.
type Agent struct {
	Binary    string   `toml:"binary"`
	Model     string   `toml:"model"`
	Reasoning string   `toml:"reasoning"` // Reasoning effort for codex (e.g., "low", "medium", "high")
	Args      []string `toml:"args"`      // Extra arguments passed to the agent binary
	// Additional flags can be added here as needed
}

// IterSchedule returns the agent for a given iteration number.
func (c *Config) IterSchedule(iter int) string {
	schedule := normalizeSchedule(c.Schedule)
	switch schedule {
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
		if schedule != "" {
			return schedule
		}
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
// 2. User config file (~/.looper/looper.toml or OS-specific config dir)
// 3. Project config file (looper.toml or .looper.toml in current directory)
// 4. Environment variables
// 5. CLI flags
func Load(fs *flag.FlagSet, args []string) (*Config, error) {
	cfg := &Config{}

	// 1. Set defaults
	setDefaults(cfg)

	// 2. Try to load from user config file
	userConfigFile := findUserConfigFile()
	if userConfigFile != "" {
		if err := loadConfigFile(cfg, userConfigFile); err != nil {
			return nil, fmt.Errorf("loading user config file %s: %w", userConfigFile, err)
		}
	}

	// 3. Try to load from project config file (overrides user config)
	projectConfigFile := findProjectConfigFile()
	if projectConfigFile != "" {
		if err := loadConfigFile(cfg, projectConfigFile); err != nil {
			return nil, fmt.Errorf("loading project config file %s: %w", projectConfigFile, err)
		}
	}

	// 4. Override from environment
	loadFromEnv(cfg)

	// 5. Parse CLI flags (they override everything)
	if err := parseFlags(cfg, fs, args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	// 6. Compute derived values
	if err := finalizeConfig(cfg); err != nil {
		return nil, fmt.Errorf("finalizing config: %w", err)
	}

	return cfg, nil
}

// LoadWithSources loads configuration and tracks the source of each value.
// Returns ConfigWithSources containing the config and a map of field names to their sources.
func LoadWithSources(fs *flag.FlagSet, args []string) (*ConfigWithSources, error) {
	sources := make(map[string]ConfigSource)
	cfg := &Config{}

	// 1. Set defaults (all fields start with default source)
	setDefaults(cfg)
	for _, field := range configFields() {
		sources[field] = SourceDefault
	}

	// 2. Try to load from user config file
	userConfigFile := findUserConfigFile()
	if userConfigFile != "" {
		if err := loadConfigFileWithSources(cfg, userConfigFile, sources, SourceUserFile); err != nil {
			return nil, fmt.Errorf("loading user config file %s: %w", userConfigFile, err)
		}
	}

	// 3. Try to load from project config file (overrides user config)
	projectConfigFile := findProjectConfigFile()
	if projectConfigFile != "" {
		if err := loadConfigFileWithSources(cfg, projectConfigFile, sources, SourceProjFile); err != nil {
			return nil, fmt.Errorf("loading project config file %s: %w", projectConfigFile, err)
		}
	}

	// 4. Override from environment
	loadFromEnvWithSources(cfg, sources)

	// 5. Parse CLI flags (they override everything)
	if err := parseFlagsWithSources(cfg, fs, args, sources); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	// 6. Compute derived values
	if err := finalizeConfig(cfg); err != nil {
		return nil, fmt.Errorf("finalizing config: %w", err)
	}

	return &ConfigWithSources{
		Config:  cfg,
		Sources: sources,
	}, nil
}

// configFields returns the list of configurable field names for source tracking.
func configFields() []string {
	return []string{
		"todo_file",
		"schema_file",
		"log_dir",
		"max_iterations",
		"schedule",
		"repair_agent",
		"review_agent",
		"bootstrap_agent",
		"odd_agent",
		"even_agent",
		"rr_agents",
		"apply_summary",
		"git_init",
		"hook_command",
		"loop_delay_seconds",
		"codex_binary",
		"codex_model",
		"codex_reasoning",
		"codex_args",
		"claude_binary",
		"claude_model",
		"claude_args",
	}
}

// loadConfigFileWithSources loads TOML config and updates source tracking.
func loadConfigFileWithSources(cfg *Config, path string, sources map[string]ConfigSource, source ConfigSource) error {
	// Create a temporary config to see what values are in the file
	tempCfg := &Config{}
	if _, err := toml.DecodeFile(path, tempCfg); err != nil {
		return err
	}

	// Update sources for any non-default values found in the file
	if tempCfg.TodoFile != "" && tempCfg.TodoFile != DefaultTodoFile {
		sources["todo_file"] = source
		cfg.TodoFile = tempCfg.TodoFile
	}
	if tempCfg.SchemaFile != "" && tempCfg.SchemaFile != "to-do.schema.json" {
		sources["schema_file"] = source
		cfg.SchemaFile = tempCfg.SchemaFile
	}
	if tempCfg.LogDir != "" && tempCfg.LogDir != DefaultLogDir {
		sources["log_dir"] = source
		cfg.LogDir = tempCfg.LogDir
	}
	if tempCfg.MaxIterations != 0 && tempCfg.MaxIterations != DefaultMaxIterations {
		sources["max_iterations"] = source
		cfg.MaxIterations = tempCfg.MaxIterations
	}
	if tempCfg.Schedule != "" && tempCfg.Schedule != "codex" {
		sources["schedule"] = source
		cfg.Schedule = tempCfg.Schedule
	}
	if tempCfg.RepairAgent != "" && tempCfg.RepairAgent != "codex" {
		sources["repair_agent"] = source
		cfg.RepairAgent = tempCfg.RepairAgent
	}
	if tempCfg.ReviewAgent != "" {
		sources["review_agent"] = source
		cfg.ReviewAgent = tempCfg.ReviewAgent
	}
	if tempCfg.BootstrapAgent != "" {
		sources["bootstrap_agent"] = source
		cfg.BootstrapAgent = tempCfg.BootstrapAgent
	}
	if tempCfg.OddAgent != "" {
		sources["odd_agent"] = source
		cfg.OddAgent = tempCfg.OddAgent
	}
	if tempCfg.EvenAgent != "" {
		sources["even_agent"] = source
		cfg.EvenAgent = tempCfg.EvenAgent
	}
	if tempCfg.RRAgents != nil {
		sources["rr_agents"] = source
		cfg.RRAgents = tempCfg.RRAgents
	}
	if tempCfg.ApplySummary != DefaultApplySummary {
		sources["apply_summary"] = source
		cfg.ApplySummary = tempCfg.ApplySummary
	}
	if tempCfg.GitInit != true {
		sources["git_init"] = source
		cfg.GitInit = tempCfg.GitInit
	}
	if tempCfg.HookCommand != "" {
		sources["hook_command"] = source
		cfg.HookCommand = tempCfg.HookCommand
	}
	if tempCfg.LoopDelaySeconds != 0 {
		sources["loop_delay_seconds"] = source
		cfg.LoopDelaySeconds = tempCfg.LoopDelaySeconds
	}

	// Handle agent configs
	for agentName, agent := range tempCfg.Agents {
		switch normalizeAgent(agentName) {
		case "codex":
			if agent.Binary != "" && agent.Binary != DefaultAgentBinaries()["codex"] {
				sources["codex_binary"] = source
			}
			if agent.Model != "" {
				sources["codex_model"] = source
			}
			if agent.Reasoning != "" {
				sources["codex_reasoning"] = source
			}
			if len(agent.Args) > 0 {
				sources["codex_args"] = source
			}
			cfg.Agents.SetAgent("codex", agent)
		case "claude":
			if agent.Binary != "" && agent.Binary != DefaultAgentBinaries()["claude"] {
				sources["claude_binary"] = source
			}
			if agent.Model != "" {
				sources["claude_model"] = source
			}
			if len(agent.Args) > 0 {
				sources["claude_args"] = source
			}
			cfg.Agents.SetAgent("claude", agent)
		}
	}

	return nil
}

// loadFromEnvWithSources loads environment variables and updates source tracking.
func loadFromEnvWithSources(cfg *Config, sources map[string]ConfigSource) {
	if v := os.Getenv("LOOPER_TODO"); v != "" {
		cfg.TodoFile = v
		sources["todo_file"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_SCHEMA"); v != "" {
		cfg.SchemaFile = v
		sources["schema_file"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_BASE_DIR"); v != "" {
		cfg.LogDir = v
		sources["log_dir"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_LOG_DIR"); v != "" {
		cfg.LogDir = v
		sources["log_dir"] = SourceEnv
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
			sources["max_iterations"] = SourceEnv
		}
	}
	if v := os.Getenv("LOOPER_ITER_SCHEDULE"); v != "" {
		cfg.Schedule = v
		sources["schedule"] = SourceEnv
	} else if v := os.Getenv("LOOPER_SCHEDULE"); v != "" {
		cfg.Schedule = v
		sources["schedule"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_REPAIR_AGENT"); v != "" {
		cfg.RepairAgent = v
		sources["repair_agent"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_REVIEW_AGENT"); v != "" {
		cfg.ReviewAgent = v
		sources["review_agent"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_BOOTSTRAP_AGENT"); v != "" {
		cfg.BootstrapAgent = v
		sources["bootstrap_agent"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_ITER_ODD_AGENT"); v != "" {
		cfg.OddAgent = v
		sources["odd_agent"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_ITER_EVEN_AGENT"); v != "" {
		cfg.EvenAgent = v
		sources["even_agent"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_ITER_RR_AGENTS"); v != "" {
		cfg.RRAgents = splitAndTrim(v, ",")
		sources["rr_agents"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_APPLY_SUMMARY"); v != "" {
		cfg.ApplySummary = boolFromString(v)
		sources["apply_summary"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_GIT_INIT"); v != "" {
		cfg.GitInit = boolFromString(v)
		sources["git_init"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_HOOK"); v != "" {
		cfg.HookCommand = v
		sources["hook_command"] = SourceEnv
	}
	if v := os.Getenv("LOOPER_LOOP_DELAY"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.LoopDelaySeconds = i
			sources["loop_delay_seconds"] = SourceEnv
		}
	}
	if v := os.Getenv("CODEX_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Binary = v
		cfg.Agents.SetAgent("codex", agent)
		sources["codex_binary"] = SourceEnv
	}
	if v := os.Getenv("CLAUDE_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Binary = v
		cfg.Agents.SetAgent("claude", agent)
		sources["claude_binary"] = SourceEnv
	}
	if v := os.Getenv("CODEX_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Model = v
		cfg.Agents.SetAgent("codex", agent)
		sources["codex_model"] = SourceEnv
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Model = v
		cfg.Agents.SetAgent("claude", agent)
		sources["claude_model"] = SourceEnv
	}
	if v := os.Getenv("CODEX_REASONING"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
		sources["codex_reasoning"] = SourceEnv
	}
	if v := os.Getenv("CODEX_REASONING_EFFORT"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
		sources["codex_reasoning"] = SourceEnv
	}
	if v := os.Getenv("CODEX_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Args = splitAndTrim(v, ",")
		cfg.Agents.SetAgent("codex", agent)
		sources["codex_args"] = SourceEnv
	}
	if v := os.Getenv("CLAUDE_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Args = splitAndTrim(v, ",")
		cfg.Agents.SetAgent("claude", agent)
		sources["claude_args"] = SourceEnv
	}
}

// parseFlagsWithSources parses CLI flags and updates source tracking.
func parseFlagsWithSources(cfg *Config, fs *flag.FlagSet, args []string, sources map[string]ConfigSource) error {
	if fs == nil {
		fs = flag.NewFlagSet("looper", flag.ContinueOnError)
	}

	// Track which flags are explicitly set
	flagSet := make(map[string]bool)

	// Path flags
	var todoFile, schemaFile, logDir string
	fs.StringVar(&todoFile, "todo", cfg.TodoFile, "")
	fs.StringVar(&schemaFile, "schema", cfg.SchemaFile, "")
	fs.StringVar(&logDir, "log-dir", cfg.LogDir, "")
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "todo" {
			flagSet["todo_file"] = true
		}
		if f.Name == "schema" {
			flagSet["schema_file"] = true
		}
		if f.Name == "log-dir" {
			flagSet["log_dir"] = true
		}
	})

	// Dev-only flags (only work when LOOPER_PROMPT_MODE=dev)
	if devModeEnabled() {
		var promptDir string
		var printPrompt bool
		fs.StringVar(&promptDir, "prompt-dir", cfg.PromptDir, "")
		fs.BoolVar(&printPrompt, "print-prompt", cfg.PrintPrompt, "")
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "prompt-dir" {
				cfg.PromptDir = promptDir
			}
			if f.Name == "print-prompt" {
				cfg.PrintPrompt = printPrompt
			}
		})
	}

	// Loop settings
	var maxIter int
	var schedule, repairAgent, reviewAgent, bootstrapAgent, oddAgent, evenAgent string
	fs.IntVar(&maxIter, "max-iterations", cfg.MaxIterations, "")
	fs.StringVar(&schedule, "schedule", cfg.Schedule, "")
	fs.StringVar(&repairAgent, "repair-agent", cfg.RepairAgent, "")
	fs.StringVar(&reviewAgent, "review-agent", cfg.ReviewAgent, "")
	fs.StringVar(&bootstrapAgent, "bootstrap-agent", cfg.BootstrapAgent, "")
	fs.StringVar(&oddAgent, "odd-agent", cfg.OddAgent, "")
	fs.StringVar(&evenAgent, "even-agent", cfg.EvenAgent, "")
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "max-iterations":
			flagSet["max_iterations"] = true
		case "schedule":
			flagSet["schedule"] = true
		case "repair-agent":
			flagSet["repair_agent"] = true
		case "review-agent":
			flagSet["review_agent"] = true
		case "bootstrap-agent":
			flagSet["bootstrap_agent"] = true
		case "odd-agent":
			flagSet["odd_agent"] = true
		case "even-agent":
			flagSet["even_agent"] = true
		}
	})

	// Round-robin agents
	var rrAgentsStr string
	if cfg.RRAgents != nil {
		rrAgentsStr = strings.Join(cfg.RRAgents, ",")
	}
	fs.StringVar(&rrAgentsStr, "rr-agents", rrAgentsStr, "")
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "rr-agents" {
			flagSet["rr_agents"] = true
		}
	})

	// Output
	var applySummary bool
	fs.BoolVar(&applySummary, "apply-summary", cfg.ApplySummary, "")
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "apply-summary" {
			flagSet["apply_summary"] = true
		}
	})

	// Git
	var gitInit bool
	fs.BoolVar(&gitInit, "git-init", cfg.GitInit, "")
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "git-init" {
			flagSet["git_init"] = true
		}
	})

	// Hooks
	var hook string
	fs.StringVar(&hook, "hook", cfg.HookCommand, "")
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "hook" {
			flagSet["hook_command"] = true
		}
	})

	// Delay
	var loopDelay int
	fs.IntVar(&loopDelay, "loop-delay", cfg.LoopDelaySeconds, "")
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "loop-delay" {
			flagSet["loop_delay_seconds"] = true
		}
	})

	// Agents
	codexBinary := cfg.GetAgentBinary("codex")
	claudeBinary := cfg.GetAgentBinary("claude")
	codexModel := cfg.GetAgentModel("codex")
	claudeModel := cfg.GetAgentModel("claude")
	codexReasoning := cfg.GetAgentReasoning("codex")
	codexArgsStr := strings.Join(cfg.GetAgentArgs("codex"), ",")
	claudeArgsStr := strings.Join(cfg.GetAgentArgs("claude"), ",")
	fs.StringVar(&codexBinary, "codex-bin", codexBinary, "")
	fs.StringVar(&claudeBinary, "claude-bin", claudeBinary, "")
	fs.StringVar(&codexModel, "codex-model", codexModel, "")
	fs.StringVar(&claudeModel, "claude-model", claudeModel, "")
	fs.StringVar(&codexReasoning, "codex-reasoning", codexReasoning, "")
	fs.StringVar(&codexArgsStr, "codex-args", codexArgsStr, "")
	fs.StringVar(&claudeArgsStr, "claude-args", claudeArgsStr, "")
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "codex-bin":
			flagSet["codex_binary"] = true
		case "claude-bin":
			flagSet["claude_binary"] = true
		case "codex-model":
			flagSet["codex_model"] = true
		case "claude-model":
			flagSet["claude_model"] = true
		case "codex-reasoning":
			flagSet["codex_reasoning"] = true
		case "codex-args":
			flagSet["codex_args"] = true
		case "claude-args":
			flagSet["claude_args"] = true
		}
	})

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Update config from parsed values
	if flagSet["todo_file"] {
		cfg.TodoFile = todoFile
		sources["todo_file"] = SourceFlag
	}
	if flagSet["schema_file"] {
		cfg.SchemaFile = schemaFile
		sources["schema_file"] = SourceFlag
	}
	if flagSet["log_dir"] {
		cfg.LogDir = logDir
		sources["log_dir"] = SourceFlag
	}
	if flagSet["max_iterations"] {
		cfg.MaxIterations = maxIter
		sources["max_iterations"] = SourceFlag
	}
	if flagSet["schedule"] {
		cfg.Schedule = schedule
		sources["schedule"] = SourceFlag
	}
	if flagSet["repair_agent"] {
		cfg.RepairAgent = repairAgent
		sources["repair_agent"] = SourceFlag
	}
	if flagSet["review_agent"] {
		cfg.ReviewAgent = reviewAgent
		sources["review_agent"] = SourceFlag
	}
	if flagSet["bootstrap_agent"] {
		cfg.BootstrapAgent = bootstrapAgent
		sources["bootstrap_agent"] = SourceFlag
	}
	if flagSet["odd_agent"] {
		cfg.OddAgent = oddAgent
		sources["odd_agent"] = SourceFlag
	}
	if flagSet["even_agent"] {
		cfg.EvenAgent = evenAgent
		sources["even_agent"] = SourceFlag
	}
	if flagSet["rr_agents"] {
		cfg.RRAgents = splitAndTrim(rrAgentsStr, ",")
		sources["rr_agents"] = SourceFlag
	}
	if flagSet["apply_summary"] {
		cfg.ApplySummary = applySummary
		sources["apply_summary"] = SourceFlag
	}
	if flagSet["git_init"] {
		cfg.GitInit = gitInit
		sources["git_init"] = SourceFlag
	}
	if flagSet["hook_command"] {
		cfg.HookCommand = hook
		sources["hook_command"] = SourceFlag
	}
	if flagSet["loop_delay_seconds"] {
		cfg.LoopDelaySeconds = loopDelay
		sources["loop_delay_seconds"] = SourceFlag
	}
	codexArgs := splitAndTrim(codexArgsStr, ",")
	claudeArgs := splitAndTrim(claudeArgsStr, ",")
	if flagSet["codex_binary"] {
		sources["codex_binary"] = SourceFlag
	}
	if flagSet["codex_model"] {
		sources["codex_model"] = SourceFlag
	}
	if flagSet["codex_reasoning"] {
		sources["codex_reasoning"] = SourceFlag
	}
	if flagSet["codex_args"] {
		sources["codex_args"] = SourceFlag
	}
	if flagSet["claude_binary"] {
		sources["claude_binary"] = SourceFlag
	}
	if flagSet["claude_model"] {
		sources["claude_model"] = SourceFlag
	}
	if flagSet["claude_args"] {
		sources["claude_args"] = SourceFlag
	}
	cfg.Agents.SetAgent("codex", Agent{Binary: codexBinary, Model: codexModel, Reasoning: codexReasoning, Args: codexArgs})
	cfg.Agents.SetAgent("claude", Agent{Binary: claudeBinary, Model: claudeModel, Args: claudeArgs})

	return nil
}

// GetConfigFile returns the active config file path (project or user).
func (cws *ConfigWithSources) GetConfigFile() string {
	for _, source := range cws.Sources {
		if source == SourceProjFile {
			projectConfigFile := findProjectConfigFile()
			if projectConfigFile != "" {
				return projectConfigFile
			}
		}
	}
	for _, source := range cws.Sources {
		if source == SourceUserFile {
			userConfigFile := findUserConfigFile()
			if userConfigFile != "" {
				return userConfigFile
			}
		}
	}
	return ""
}

// setDefaults applies default values to the config.
func setDefaults(cfg *Config) {
	cfg.TodoFile = DefaultTodoFile
	cfg.SchemaFile = "to-do.schema.json"
	cfg.LogDir = DefaultLogDir
	cfg.MaxIterations = DefaultMaxIterations
	cfg.Schedule = "codex"
	cfg.RepairAgent = "codex"
	cfg.ReviewAgent = ""    // Empty means use default (codex)
	cfg.BootstrapAgent = "" // Empty means use default (codex)
	cfg.OddAgent = ""       // Empty means use default (codex)
	cfg.EvenAgent = ""      // Empty means use default (claude)
	cfg.RRAgents = nil      // nil means use default (claude,codex)
	cfg.ApplySummary = DefaultApplySummary
	cfg.GitInit = true
	cfg.LoopDelaySeconds = 0

	// Default agent binaries
	cfg.Agents.SetAgent("codex", Agent{Binary: DefaultAgentBinaries()["codex"]})
	cfg.Agents.SetAgent("claude", Agent{Binary: DefaultAgentBinaries()["claude"]})
}

// findProjectConfigFile looks for a config file in the current directory.
func findProjectConfigFile() string {
	// Check for looper.toml in current directory
	names := []string{"looper.toml", ".looper.toml"}
	for _, name := range names {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

// findConfigFile looks for config files in the following order:
// 1. User config directory (~/.looper/looper.toml or OS-specific config dir)
// 2. Project directory (./looper.toml or ./.looper.toml)
// Returns the first config file found, or empty string if none exist.
//
// Deprecated: Use findUserConfigFile and findProjectConfigFile separately
// for proper merge order. This is kept for backwards compatibility.
func findConfigFile() string {
	// First check for user-level config
	if userCfg := findUserConfigFile(); userCfg != "" {
		return userCfg
	}

	// Then check for project config in current directory
	return findProjectConfigFile()
}

// findUserConfigFile looks for a user-level config file.
// Checks ~/.looper/looper.toml first, then falls back to OS-specific
// config directories if ~/.looper doesn't exist.
func findUserConfigFile() string {
	// First try ~/.looper/looper.toml
	home, err := os.UserHomeDir()
	if err == nil {
		userConfigPath := filepath.Join(home, ".looper", "looper.toml")
		if _, err := os.Stat(userConfigPath); err == nil {
			return userConfigPath
		}
	}

	// If ~/.looper doesn't exist, try OS-specific config directories
	if cfgDir := osUserConfigDir(); cfgDir != "" {
		userConfigPath := filepath.Join(cfgDir, "looper", "looper.toml")
		if _, err := os.Stat(userConfigPath); err == nil {
			return userConfigPath
		}
	}

	return ""
}

// osUserConfigDir returns the OS-specific user config directory.
// Returns empty string if the directory cannot be determined.
func osUserConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		// On Windows, use %APPDATA%\looper
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return appdata
		}
	case "darwin":
		// On macOS, use ~/Library/Application Support
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, "Library", "Application Support")
		}
	case "linux", "openbsd", "freebsd", "netbsd":
		// On Linux/BSD, respect XDG_CONFIG_HOME or use ~/.config
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return xdg
		}
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".config")
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
		agent := cfg.Agents.GetAgent("codex")
		agent.Binary = v
		cfg.Agents.SetAgent("codex", agent)
	}
	if v := os.Getenv("CLAUDE_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Binary = v
		cfg.Agents.SetAgent("claude", agent)
	}
	if v := os.Getenv("CODEX_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Model = v
		cfg.Agents.SetAgent("codex", agent)
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Model = v
		cfg.Agents.SetAgent("claude", agent)
	}
	if v := os.Getenv("CODEX_REASONING"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
	}
	if v := os.Getenv("CODEX_REASONING_EFFORT"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
	}
	if v := os.Getenv("CODEX_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Args = splitAndTrim(v, ",")
		cfg.Agents.SetAgent("codex", agent)
	}
	if v := os.Getenv("CLAUDE_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Args = splitAndTrim(v, ",")
		cfg.Agents.SetAgent("claude", agent)
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
	codexBinary := cfg.GetAgentBinary("codex")
	claudeBinary := cfg.GetAgentBinary("claude")
	codexModel := cfg.GetAgentModel("codex")
	claudeModel := cfg.GetAgentModel("claude")
	codexReasoning := cfg.GetAgentReasoning("codex")
	codexArgsStr := strings.Join(cfg.GetAgentArgs("codex"), ",")
	claudeArgsStr := strings.Join(cfg.GetAgentArgs("claude"), ",")
	fs.StringVar(&codexBinary, "codex-bin", codexBinary, "Codex binary")
	fs.StringVar(&claudeBinary, "claude-bin", claudeBinary, "Claude binary")
	fs.StringVar(&codexModel, "codex-model", codexModel, "Codex model")
	fs.StringVar(&claudeModel, "claude-model", claudeModel, "Claude model")
	fs.StringVar(&codexReasoning, "codex-reasoning", codexReasoning, "Codex reasoning effort (e.g., low, medium, high)")
	fs.StringVar(&codexArgsStr, "codex-args", codexArgsStr, "Comma-separated extra args for codex (e.g., --foo,bar)")
	fs.StringVar(&claudeArgsStr, "claude-args", claudeArgsStr, "Comma-separated extra args for claude (e.g., --foo,bar)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if rrAgentsStr != "" {
		cfg.RRAgents = splitAndTrim(rrAgentsStr, ",")
	}
	codexArgs := splitAndTrim(codexArgsStr, ",")
	claudeArgs := splitAndTrim(claudeArgsStr, ",")
	cfg.Agents.SetAgent("codex", Agent{Binary: codexBinary, Model: codexModel, Reasoning: codexReasoning, Args: codexArgs})
	cfg.Agents.SetAgent("claude", Agent{Binary: claudeBinary, Model: claudeModel, Args: claudeArgs})
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
// It checks both custom agents and built-in defaults, then falls back to the agent name.
func (c *Config) GetAgentBinary(agentType string) string {
	agentType = normalizeAgent(agentType)
	if agentType == "" {
		return ""
	}
	if agent := c.Agents.GetAgent(agentType); agent.Binary != "" {
		return agent.Binary
	}
	if binary, ok := DefaultAgentBinaries()[agentType]; ok {
		return binary
	}
	return agentType
}

// GetAgentModel returns the model for the given agent type.
// It checks both custom agents and built-in agents.
func (c *Config) GetAgentModel(agentType string) string {
	agentType = normalizeAgent(agentType)
	if agentType == "" {
		return ""
	}
	return c.Agents.GetAgent(agentType).Model
}

// GetAgentReasoning returns the reasoning effort for the given agent type.
// It checks both custom agents and built-in agents.
func (c *Config) GetAgentReasoning(agentType string) string {
	agentType = normalizeAgent(agentType)
	if agentType == "" {
		return ""
	}
	return c.Agents.GetAgent(agentType).Reasoning
}

// GetAgentArgs returns extra args for the given agent type.
func (c *Config) GetAgentArgs(agentType string) []string {
	agentType = normalizeAgent(agentType)
	if agentType == "" {
		return nil
	}
	args := c.Agents.GetAgent(agentType).Args
	if len(args) == 0 {
		return nil
	}
	copied := make([]string, len(args))
	copy(copied, args)
	return copied
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
