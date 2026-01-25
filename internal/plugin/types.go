// Package plugin provides a plugin system for looper-go.
// Plugins are external binaries that communicate via JSON-RPC.
package plugin

import (
	"time"
)

// PluginCategory represents the type/category of a plugin.
type PluginCategory string

const (
	PluginCategoryAgent    PluginCategory = "agent"
	PluginCategoryWorkflow PluginCategory = "workflow"
	// Future categories can be added here:
	// PluginCategoryParser PluginCategory = "parser"
	// PluginCategoryPrompt PluginCategory = "prompt"
)

// Plugin represents a loaded plugin with its metadata and configuration.
type Plugin struct {
	// Name is the unique identifier for this plugin.
	Name string

	// Version is the plugin's semantic version.
	Version string

	// Category determines the type of plugin (agent, workflow, etc.).
	Category PluginCategory

	// Manifest holds the parsed manifest data.
	Manifest *Manifest

	// Path is the absolute path to the plugin directory.
	Path string

	// Scope indicates where this plugin was loaded from (user or project).
	Scope PluginScope

	// BinaryPath is the absolute path to the plugin's executable binary.
	BinaryPath string

	// Config holds plugin-specific configuration from looper.toml.
	Config map[string]any
}

// PluginScope indicates where a plugin was loaded from.
type PluginScope int

const (
	// ScopeUser indicates the plugin was loaded from ~/.looper/plugins/
	ScopeUser PluginScope = iota

	// ScopeProject indicates the plugin was loaded from .looper/plugins/
	ScopeProject

	// ScopeBuiltin indicates the plugin is built into the looper binary.
	ScopeBuiltin
)

func (s PluginScope) String() string {
	switch s {
	case ScopeUser:
		return "user"
	case ScopeProject:
		return "project"
	case ScopeBuiltin:
		return "builtin"
	default:
		return "unknown"
	}
}

// Manifest represents the parsed looper-plugin.toml file.
type Manifest struct {
	// Basic metadata
	Name        string `toml:"name"`
	Version     string `toml:"version"`
	Category    string `toml:"category"`
	Description string `toml:"-"`

	// Plugin metadata
	Plugin PluginMetadata `toml:"plugin"`

	// Category-specific configuration
	Agent    *AgentConfig    `toml:"agent,omitempty"`
	Workflow *WorkflowConfig `toml:"workflow,omitempty"`

	// Dependencies
	Dependencies *Dependencies `toml:"dependencies,omitempty"`

	// Capabilities
	Capabilities *Capabilities `toml:"capabilities,omitempty"`
}

// PluginMetadata holds general plugin information.
type PluginMetadata struct {
	Binary           string `toml:"binary"`            // Relative path to binary
	Author           string `toml:"author"`            // Plugin author
	Homepage         string `toml:"homepage"`          // URL to homepage
	License          string `toml:"license"`           // License name
	MinLooperVersion string `toml:"min_looper_version"` // Minimum looper version required
}

// AgentConfig holds agent-specific manifest configuration.
type AgentConfig struct {
	Type               string `toml:"type"`                          // Registered agent type
	SupportsStreaming  bool   `toml:"supports_streaming"`            // Whether agent supports streaming output
	SupportsTools      bool   `toml:"supports_tools"`                // Whether agent supports tool use
	SupportsMCP        bool   `toml:"supports_mcp"`                  // Whether agent supports Model Context Protocol
	DefaultPromptFormat string `toml:"default_prompt_format"`        // Default prompt format (stdin/arg)
}

// WorkflowConfig holds workflow-specific manifest configuration.
type WorkflowConfig struct {
	Type              string `toml:"type"`               // Registered workflow type
	SupportsParallel  bool   `toml:"supports_parallel"`  // Whether workflow supports parallel execution
	SupportsReview    bool   `toml:"supports_review"`    // Whether workflow has a review phase
	MaxIterations     int    `toml:"max_iterations"`     // Suggested max iterations (0 = no limit)
}

// Dependencies describes what the plugin needs to run.
type Dependencies struct {
	Binaries   []string `toml:"binaries"`   // Required executables in PATH
	Packages   []string `toml:"packages"`   // Required system packages
	APIKeys    []string `toml:"api_keys"`   // Required API keys (for documentation)
	MinVersion string   `toml:"min_version"` // Minimum plugin version for dependencies
}

// Capabilities describes what operations the plugin can perform.
type Capabilities struct {
	CanModifyFiles     bool `toml:"can_modify_files"`     // Can the plugin write files?
	CanExecuteCommands bool `toml:"can_execute_commands"` // Can the plugin run shell commands?
	CanAccessNetwork   bool `toml:"can_access_network"`   // Can the plugin make network requests?
	CanAccessEnv       bool `toml:"can_access_env"`       // Can the plugin read environment variables?
}

// Request represents a JSON-RPC request to a plugin.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a JSON-RPC response from a plugin.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError represents a JSON-RPC error.
type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// AgentRunParams are parameters for an agent plugin's "run" method.
type AgentRunParams struct {
	Prompt  string                 `json:"prompt"`
	Context map[string]interface{} `json:"context"`
}

// AgentResult is the result returned by an agent plugin.
type AgentResult struct {
	TaskID   string   `json:"task_id"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Files    []string `json:"files,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

// WorkflowRunParams are parameters for a workflow plugin's "run" method.
type WorkflowRunParams struct {
	Config    map[string]interface{} `json:"config"`
	WorkDir   string                 `json:"work_dir"`
	TodoFile  string                 `json:"todo_file"`
	UserPrompt string                `json:"user_prompt,omitempty"`
}

// WorkflowResult is the result returned by a workflow plugin.
type WorkflowResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// ExecutorConfig holds configuration for executing a plugin.
type ExecutorConfig struct {
	// Timeout is the maximum time to wait for plugin execution.
	// If zero, a default timeout is used.
	Timeout time.Duration

	// WorkDir is the working directory for plugin execution.
	WorkDir string

	// EnvVars are additional environment variables to pass to the plugin.
	EnvVars []string

	// Stdin is the input to pass to the plugin.
	// If nil, no input is provided.
	Stdin []byte

	// TimeoutHandler is called when a timeout occurs.
	// If nil, the default behavior is to return a timeout error.
	TimeoutHandler func(*Plugin) error
}

// IsAgent returns true if the plugin is an agent plugin.
func (p *Plugin) IsAgent() bool {
	return p.Category == PluginCategoryAgent
}

// IsWorkflow returns true if the plugin is a workflow plugin.
func (p *Plugin) IsWorkflow() bool {
	return p.Category == PluginCategoryWorkflow
}

// GetAgentType returns the agent type if this is an agent plugin.
func (p *Plugin) GetAgentType() string {
	if p.Manifest == nil || p.Manifest.Agent == nil {
		return ""
	}
	return p.Manifest.Agent.Type
}

// GetWorkflowType returns the workflow type if this is a workflow plugin.
func (p *Plugin) GetWorkflowType() string {
	if p.Manifest == nil || p.Manifest.Workflow == nil {
		return ""
	}
	return p.Manifest.Workflow.Type
}

// SupportsCapability returns true if the plugin has the specified capability.
func (p *Plugin) SupportsCapability(cap string) bool {
	if p.Manifest == nil || p.Manifest.Capabilities == nil {
		return false
	}

	switch cap {
	case "modify_files":
		return p.Manifest.Capabilities.CanModifyFiles
	case "execute_commands":
		return p.Manifest.Capabilities.CanExecuteCommands
	case "access_network":
		return p.Manifest.Capabilities.CanAccessNetwork
	case "access_env":
		return p.Manifest.Capabilities.CanAccessEnv
	}
	return false
}

// GetTimeout returns the configured timeout for the plugin, or the default if not set.
func (p *Plugin) GetTimeout(defaultTimeout time.Duration) time.Duration {
	if p.Config == nil {
		return defaultTimeout
	}

	if timeoutStr, ok := p.Config["timeout"].(string); ok {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			return d
		}
	}

	return defaultTimeout
}

// String returns a string representation of the plugin.
func (p *Plugin) String() string {
	scope := p.Scope.String()
	if p.Version != "" {
		return p.Name + "@" + p.Version + " (" + scope + ")"
	}
	return p.Name + " (" + scope + ")"
}
