// Package config handles configuration loading and defaults.
package config

import (
	"fmt"

	"github.com/nibzard/looper-go/internal/looperdir"
	"github.com/nibzard/looper-go/internal/utils"
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
	DefaultTodoFile      = looperdir.Dir + "/" + looperdir.DefaultTodoFile
	DefaultSchemaFile    = looperdir.Dir + "/" + looperdir.DefaultSchemaFile
	DefaultConfigFile    = looperdir.Dir + "/" + looperdir.DefaultConfigFile
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

	// Workflow selection (default: "traditional")
	Workflow string `toml:"workflow"`

	// Workflow-specific configuration
	WorkflowConfigs map[string]map[string]any `toml:"workflows"`

	// Loop settings
	MaxIterations int `toml:"max_iterations"`

	// Roles maps role names to agent names (iter, review, repair, bootstrap)
	Roles RolesConfig `toml:"roles"`

	// Agents
	Agents AgentConfig `toml:"agents"`

	// Plugins
	Plugins PluginConfig `toml:"plugins"`

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

	// Parallel execution configuration
	Parallel ParallelConfig `toml:"parallel"`

	// Project root (computed)
	ProjectRoot string `toml:"-"`
}

// RolesConfig maps role names to agent names.
type RolesConfig map[string]string

// GetAgent returns the agent name for a given role.
func (rc RolesConfig) GetAgent(role string) string {
	if rc == nil {
		return ""
	}
	return utils.NormalizeAgentName(rc[role])
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
	key := utils.NormalizeAgentName(agentType)
	if key == "" {
		return Agent{}
	}
	return ac[key]
}

// SetAgent sets the configuration for a given agent type.
func (ac *AgentConfig) SetAgent(agentType string, config Agent) {
	key := utils.NormalizeAgentName(agentType)
	if key == "" {
		return
	}
	if *ac == nil {
		*ac = AgentConfig{}
	}
	(*ac)[key] = config
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

// PluginConfig holds plugin-specific configuration.
// It is a map keyed by plugin name.
type PluginConfig map[string]PluginSettings

// PluginSettings holds configuration for a single plugin.
type PluginSettings struct {
	// Timeout is the maximum duration to wait for plugin execution.
	Timeout string `toml:"timeout"`

	// WorkDir is the working directory for plugin execution.
	WorkDir string `toml:"work_dir"`

	// Binary is the path to the plugin binary (overrides manifest).
	Binary string `toml:"binary"`

	// Model is the model to use for agent plugins.
	Model string `toml:"model"`

	// Reasoning is the reasoning effort for agent plugins.
	Reasoning string `toml:"reasoning"`

	// Enabled allows disabling a plugin without uninstalling it.
	Enabled bool `toml:"enabled"`

	// Additional plugin-specific settings can be added here.
	// We use a map for flexibility.
	Extra map[string]any `toml:"-"`
}

// ParallelConfig holds configuration for parallel task execution.
type ParallelConfig struct {
	// Enabled enables parallel execution of tasks.
	Enabled bool `toml:"enabled"`

	// MaxTasks is the maximum number of tasks to run concurrently.
	// 0 means unlimited (bounded by available dependencies).
	MaxTasks int `toml:"max_tasks"`

	// MaxAgentsPerTask is the maximum number of agents to run per task.
	// 1 means single agent (default), >1 enables multi-agent consensus.
	MaxAgentsPerTask int `toml:"max_agents_per_task"`

	// Strategy determines how tasks are selected for parallel execution.
	Strategy ParallelStrategy `toml:"strategy"`

	// FailFast stops all execution on the first task failure.
	FailFast bool `toml:"fail_fast"`

	// OutputMode determines how output from concurrent tasks is handled.
	OutputMode ParallelOutputMode `toml:"output_mode"`
}

// ParallelStrategy defines the task selection strategy for parallel execution.
type ParallelStrategy string

const (
	// StrategyPriority selects highest priority tasks first.
	StrategyPriority ParallelStrategy = "priority"
	// StrategyDependency respects dependencies when selecting tasks.
	StrategyDependency ParallelStrategy = "dependency"
	// StrategyMixed balances priority and dependency awareness.
	StrategyMixed ParallelStrategy = "mixed"
)

// ParallelOutputMode defines how output from concurrent tasks is displayed.
type ParallelOutputMode string

const (
	// OutputMultiplexed interleaves output with task/agent ID prefixes.
	OutputMultiplexed ParallelOutputMode = "multiplexed"
	// OutputBuffered buffers output per task and displays it on completion.
	OutputBuffered ParallelOutputMode = "buffered"
	// OutputSummary shows only summaries without detailed output.
	OutputSummary ParallelOutputMode = "summary"
)

// GetPlugin returns the configuration for a given plugin name.
func (pc PluginConfig) GetPlugin(pluginName string) PluginSettings {
	if pc == nil {
		return PluginSettings{}
	}
	return pc[pluginName]
}

// SetPlugin sets the configuration for a given plugin name.
func (pc *PluginConfig) SetPlugin(pluginName string, settings PluginSettings) {
	if *pc == nil {
		*pc = PluginConfig{}
	}
	(*pc)[pluginName] = settings
}
