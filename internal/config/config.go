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
	MaxIterations int `toml:"max_iterations"`

	// Roles maps role names to agent names (iter, review, repair, bootstrap)
	Roles RolesConfig `toml:"roles"`

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

	// Logging configuration
	LogLevel      string `toml:"log_level"`
	LogFormat     string `toml:"log_format"`
	LogTimestamps bool   `toml:"log_timestamps"`
	LogCaller     bool   `toml:"log_caller"`

	// Project root (computed)
	ProjectRoot string `toml:"-"`
}

// RolesConfig maps role names to agent names.
type RolesConfig map[string]string

// IterSchedule returns the agent for a given iteration number.
// Looks up roles["iter"] or returns empty string if not configured.
func (c *Config) IterSchedule(iter int) string {
	return c.Roles.GetAgent("iter")
}

// GetReviewAgent returns the agent to use for the review pass.
// Looks up roles["review"] or returns empty string if not configured.
func (c *Config) GetReviewAgent() string {
	return c.Roles.GetAgent("review")
}

// GetBootstrapAgent returns the agent to use for bootstrap operations.
// Looks up roles["bootstrap"] or returns empty string if not configured.
func (c *Config) GetBootstrapAgent() string {
	return c.Roles.GetAgent("bootstrap")
}

// GetRepairAgent returns the agent to use for repair operations.
// Looks up roles["repair"] or returns empty string if not configured.
func (c *Config) GetRepairAgent() string {
	return c.Roles.GetAgent("repair")
}

// GetAgent returns the agent name for a given role.
func (rc RolesConfig) GetAgent(role string) string {
	if rc == nil {
		return ""
	}
	return normalizeAgent(rc[role])
}

// SetAgent sets the agent name for a given role.
func (rc *RolesConfig) SetAgent(role string, agent string) {
	if *rc == nil {
		*rc = make(RolesConfig)
	}
	(*rc)[role] = agent
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
	if v, ok := raw["prompt_format"]; ok {
		promptFormat, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("prompt_format must be a string")
		}
		agent.PromptFormat = PromptFormat(promptFormat)
	}
	if v, ok := raw["parser"]; ok {
		parser, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("parser must be a string")
		}
		agent.Parser = parser
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

// PromptFormat specifies how the prompt is passed to the agent.
type PromptFormat string

const (
	// PromptFormatStdin passes the prompt via stdin.
	PromptFormatStdin PromptFormat = "stdin"
	// PromptFormatArg passes the prompt as a command-line argument.
	PromptFormatArg PromptFormat = "arg"
)

// Agent holds configuration for a single agent type.
type Agent struct {
	Binary      string       `toml:"binary"`
	Model       string       `toml:"model"`
	Reasoning   string       `toml:"reasoning"` // Reasoning effort for codex (e.g., "low", "medium", "high")
	Args        []string     `toml:"args"`      // Extra arguments passed to the agent binary
	PromptFormat PromptFormat `toml:"prompt_format"` // How to pass the prompt: "stdin" or "arg"
	Parser      string       `toml:"parser"`   // Parser script path (e.g., "claude_parser.py", "~/.looper/parsers/custom.js", "builtin:claude")
	// Additional flags can be added here as needed
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
		"roles",
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

	// Track non-default values
	if tempCfg.TodoFile != "" && tempCfg.TodoFile != DefaultTodoFile {
		setSource(cfg, &cfg.TodoFile, tempCfg.TodoFile, sources, "todo_file", source)
	}
	if tempCfg.SchemaFile != "" && tempCfg.SchemaFile != "to-do.schema.json" {
		setSource(cfg, &cfg.SchemaFile, tempCfg.SchemaFile, sources, "schema_file", source)
	}
	if tempCfg.LogDir != "" && tempCfg.LogDir != DefaultLogDir {
		setSource(cfg, &cfg.LogDir, tempCfg.LogDir, sources, "log_dir", source)
	}
	if tempCfg.MaxIterations != 0 && tempCfg.MaxIterations != DefaultMaxIterations {
		setSource(cfg, &cfg.MaxIterations, tempCfg.MaxIterations, sources, "max_iterations", source)
	}
	if tempCfg.ApplySummary != DefaultApplySummary {
		setSource(cfg, &cfg.ApplySummary, tempCfg.ApplySummary, sources, "apply_summary", source)
	}
	if tempCfg.GitInit != true {
		setSource(cfg, &cfg.GitInit, tempCfg.GitInit, sources, "git_init", source)
	}
	if tempCfg.HookCommand != "" {
		setSource(cfg, &cfg.HookCommand, tempCfg.HookCommand, sources, "hook_command", source)
	}
	if tempCfg.LoopDelaySeconds != 0 {
		setSource(cfg, &cfg.LoopDelaySeconds, tempCfg.LoopDelaySeconds, sources, "loop_delay_seconds", source)
	}

	// Handle agent configs
	mergeAgentSources(cfg, tempCfg, sources, source, "codex", DefaultAgentBinaries()["codex"])
	mergeAgentSources(cfg, tempCfg, sources, source, "claude", DefaultAgentBinaries()["claude"])

	return nil
}

// setSource is a helper for loadConfigFileWithSources.
func setSource[T any](cfg *Config, field *T, value T, sources map[string]ConfigSource, name string, source ConfigSource) {
	*field = value
	sources[name] = source
}

// mergeAgentSources merges a single agent's config from a file into cfg with source tracking.
func mergeAgentSources(cfg *Config, tempCfg *Config, sources map[string]ConfigSource, source ConfigSource, name, defaultBinary string) {
	agent := tempCfg.Agents.GetAgent(name)
	if agent.Binary != "" {
		if agent.Binary != defaultBinary {
			sources[name+"_binary"] = source
		}
		cfg.Agents.SetAgent(name, agent)
	} else if cfg.Agents.GetAgent(name).Binary == "" {
		cfg.Agents.SetAgent(name, Agent{Binary: defaultBinary})
	}
	if agent.Model != "" {
		sources[name+"_model"] = source
		a := cfg.Agents.GetAgent(name)
		a.Model = agent.Model
		cfg.Agents.SetAgent(name, a)
	}
	if agent.Reasoning != "" {
		sources[name+"_reasoning"] = source
		a := cfg.Agents.GetAgent(name)
		a.Reasoning = agent.Reasoning
		cfg.Agents.SetAgent(name, a)
	}
	if len(agent.Args) > 0 {
		sources[name+"_args"] = source
		a := cfg.Agents.GetAgent(name)
		a.Args = agent.Args
		cfg.Agents.SetAgent(name, a)
	}
}

// loadFromEnvWithSources loads environment variables and updates source tracking.
func loadFromEnvWithSources(cfg *Config, sources map[string]ConfigSource) {
	loadFromEnvHelper(cfg, sources, SourceEnv)
}

// loadFromEnv overrides config from environment variables.
func loadFromEnv(cfg *Config) {
	loadFromEnvHelper(cfg, nil, "")
}

// loadFromEnvHelper is the shared implementation for env loading.
// If sources is non-nil, it tracks the source of each value.
func loadFromEnvHelper(cfg *Config, sources map[string]ConfigSource, source ConfigSource) {
	setEnv := func(field, value string) {
		if sources != nil {
			sources[field] = source
		}
	}
	setEnvInt := func(field string, value int) {
		if sources != nil {
			sources[field] = source
		}
	}
	setEnvBool := func(field string, value bool) {
		if sources != nil {
			sources[field] = source
		}
	}
	setAgentEnv := func(agentType, field, value string) {
		if sources != nil {
			sources[field] = source
		}
	}
	setAgentArgs := func(agentType, field string, value []string) {
		if sources != nil {
			sources[field] = source
		}
	}
	setAgentReasoning := func(agentType, field string, value string) {
		if sources != nil {
			sources[field] = source
		}
	}

	if v := os.Getenv("LOOPER_TODO"); v != "" {
		cfg.TodoFile = v
		setEnv("todo_file", v)
	}
	if v := os.Getenv("LOOPER_SCHEMA"); v != "" {
		cfg.SchemaFile = v
		setEnv("schema_file", v)
	}
	if v := os.Getenv("LOOPER_BASE_DIR"); v != "" {
		cfg.LogDir = v
		setEnv("log_dir", v)
	}
	if v := os.Getenv("LOOPER_LOG_DIR"); v != "" {
		cfg.LogDir = v
		setEnv("log_dir", v)
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
			setEnvInt("max_iterations", i)
		}
	}
	if v := os.Getenv("LOOPER_APPLY_SUMMARY"); v != "" {
		cfg.ApplySummary = boolFromString(v)
		setEnvBool("apply_summary", cfg.ApplySummary)
	}
	if v := os.Getenv("LOOPER_GIT_INIT"); v != "" {
		cfg.GitInit = boolFromString(v)
		setEnvBool("git_init", cfg.GitInit)
	}
	if v := os.Getenv("LOOPER_HOOK"); v != "" {
		cfg.HookCommand = v
		setEnv("hook_command", v)
	}
	if v := os.Getenv("LOOPER_LOOP_DELAY"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.LoopDelaySeconds = i
			setEnvInt("loop_delay_seconds", i)
		}
	}
	if v := os.Getenv("CODEX_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Binary = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentEnv("codex", "codex_binary", v)
	}
	if v := os.Getenv("CLAUDE_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Binary = v
		cfg.Agents.SetAgent("claude", agent)
		setAgentEnv("claude", "claude_binary", v)
	}
	if v := os.Getenv("CODEX_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Model = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentEnv("codex", "codex_model", v)
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Model = v
		cfg.Agents.SetAgent("claude", agent)
		setAgentEnv("claude", "claude_model", v)
	}
	if v := os.Getenv("CODEX_REASONING"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentReasoning("codex", "codex_reasoning", v)
	}
	if v := os.Getenv("CODEX_REASONING_EFFORT"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentReasoning("codex", "codex_reasoning", v)
	}
	if v := os.Getenv("CODEX_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Args = splitAndTrim(v, ",")
		cfg.Agents.SetAgent("codex", agent)
		setAgentArgs("codex", "codex_args", agent.Args)
	}
	if v := os.Getenv("CLAUDE_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Args = splitAndTrim(v, ",")
		cfg.Agents.SetAgent("claude", agent)
		setAgentArgs("claude", "claude_args", agent.Args)
	}

	// Logging configuration
	if v := os.Getenv("LOOPER_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
		setEnv("log_level", v)
	}
	if v := os.Getenv("LOOPER_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
		setEnv("log_format", v)
	}
	if v := os.Getenv("LOOPER_LOG_TIMESTAMPS"); v != "" {
		cfg.LogTimestamps = boolFromString(v)
		setEnvBool("log_timestamps", cfg.LogTimestamps)
	}
	if v := os.Getenv("LOOPER_LOG_CALLER"); v != "" {
		cfg.LogCaller = boolFromString(v)
		setEnvBool("log_caller", cfg.LogCaller)
	}
}

// parseFlagsWithSources parses CLI flags and updates source tracking.
func parseFlagsWithSources(cfg *Config, fs *flag.FlagSet, args []string, sources map[string]ConfigSource) error {
	return parseFlagsHelper(cfg, fs, args, sources, SourceFlag)
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
	cfg.ApplySummary = DefaultApplySummary
	cfg.GitInit = true
	cfg.LoopDelaySeconds = 0

	// Logging defaults
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}

	// Set default roles
	if cfg.Roles == nil {
		cfg.Roles = make(RolesConfig)
	}
	cfg.Roles["iter"] = "claude"
	cfg.Roles["review"] = "claude"
	cfg.Roles["repair"] = "claude"
	cfg.Roles["bootstrap"] = "claude"

	// Default agent binaries with parsers
	cfg.Agents.SetAgent("codex", Agent{
		Binary: DefaultAgentBinaries()["codex"],
		PromptFormat: PromptFormatStdin,
		Parser: "codex_parser.py",
	})
	cfg.Agents.SetAgent("claude", Agent{
		Binary: DefaultAgentBinaries()["claude"],
		PromptFormat: PromptFormatArg,
		Parser: "claude_parser.py",
	})
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
	return parseFlagsHelper(cfg, fs, args, nil, "")
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
		codexArgs := splitAndTrim(codexArgsStr, ",")
		claudeArgs := splitAndTrim(claudeArgsStr, ",")
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
				codexAgent.Args = splitAndTrim(codexArgsStr, ",")
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
				claudeAgent.Args = splitAndTrim(claudeArgsStr, ",")
			}
			cfg.Agents.SetAgent("claude", claudeAgent)
		}
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

// GetAgentPromptFormat returns the prompt format for the given agent type.
// It checks both custom agents and built-in agents.
// Returns "stdin" if not configured (the traditional format for codex/claude).
func (c *Config) GetAgentPromptFormat(agentType string) PromptFormat {
	agentType = normalizeAgent(agentType)
	if agentType == "" {
		return PromptFormatStdin
	}
	agent := c.Agents.GetAgent(agentType)
	if agent.PromptFormat == "" {
		// Default: codex uses stdin, claude uses arg
		if agentType == "claude" {
			return PromptFormatArg
		}
		return PromptFormatStdin
	}
	return agent.PromptFormat
}

// GetAgentParser returns the parser script path for the given agent type.
// It checks both custom agents and built-in agents.
// Returns empty string if not configured (use built-in Go parsing).
func (c *Config) GetAgentParser(agentType string) string {
	agentType = normalizeAgent(agentType)
	if agentType == "" {
		return ""
	}
	return c.Agents.GetAgent(agentType).Parser
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
