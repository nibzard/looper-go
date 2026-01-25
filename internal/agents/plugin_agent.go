// Package agents provides agent implementations for looper-go.
package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginAgent implements Agent using a loaded plugin.
type PluginAgent struct {
	plugin *plugin.Plugin
	cfg    Config
}

// NewPluginAgent creates a new plugin-based agent.
func NewPluginAgent(p *plugin.Plugin, cfg Config) (*PluginAgent, error) {
	if p == nil {
		return nil, fmt.Errorf("plugin is nil")
	}

	if !p.IsAgent() {
		return nil, fmt.Errorf("plugin %q is not an agent plugin", p.Name)
	}

	// Apply plugin configuration to the agent config
	// Plugin configs from looper.toml override defaults
	if p.Config != nil {
		if timeout, ok := p.Config["timeout"].(string); ok {
			if d, err := time.ParseDuration(timeout); err == nil {
				cfg.Timeout = d
			}
		}
		if workDir, ok := p.Config["work_dir"].(string); ok {
			cfg.WorkDir = workDir
		}
		if binary, ok := p.Config["binary"].(string); ok {
			cfg.Binary = binary
		}
		if model, ok := p.Config["model"].(string); ok {
			cfg.Model = model
		}
		if reasoning, ok := p.Config["reasoning"].(string); ok {
			cfg.Reasoning = reasoning
		}
	}

	return &PluginAgent{
		plugin: p,
		cfg:    cfg,
	}, nil
}

// Run executes the agent plugin.
func (a *PluginAgent) Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	logWriter = normalizeLogWriter(logWriter)

	executor := plugin.NewExecutor(a.plugin)

	// Log the execution
	if err := logWriter.Write(LogEvent{
		Type:      "plugin_start",
		Timestamp: time.Now().UTC(),
		Content:   fmt.Sprintf("Running plugin agent: %s", a.plugin.Name),
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Execute with timeout
	timeout := a.cfg.Timeout
	if timeout == 0 {
		timeout = a.plugin.GetTimeout(DefaultTimeout)
	}

	var pluginResult *plugin.AgentResult
	var err error

	// Use streaming if the plugin supports it
	if a.plugin.Manifest != nil && a.plugin.Manifest.Agent != nil && a.plugin.Manifest.Agent.SupportsStreaming {
		// Get a writer for streaming
		writer := newLogWriterWriter(logWriter)
		pluginResult, err = executor.StreamExecute(ctx, prompt, writer)
	} else {
		// Get a writer for non-streaming execution
		writer := newLogWriterWriter(logWriter)
		pluginResult, err = plugin.ExecuteAgentWithTimeout(ctx, a.plugin, prompt, timeout, writer)
	}

	// Log completion
	if err != nil {
		_ = logWriter.Write(LogEvent{
			Type:      "plugin_error",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("Plugin agent failed: %s", err),
		})
		return nil, err
	}

	if err := logWriter.Write(LogEvent{
		Type:      "plugin_complete",
		Timestamp: time.Now().UTC(),
		Content:   fmt.Sprintf("Plugin agent completed: %s", a.plugin.Name),
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Convert plugin.AgentResult to agents.Summary
	summary := &Summary{
		TaskID:   pluginResult.TaskID,
		Status:   pluginResult.Status,
		Summary:  pluginResult.Summary,
		Files:    pluginResult.Files,
		Blockers: pluginResult.Blockers,
	}

	return summary, nil
}

// logWriterWriter adapts LogWriter to io.Writer for plugin execution.
type logWriterWriter struct {
	logWriter LogWriter
}

func newLogWriterWriter(lw LogWriter) *logWriterWriter {
	return &logWriterWriter{logWriter: lw}
}

func (w *logWriterWriter) Write(p []byte) (n int, err error) {
	// Write as content log events
	if err := w.logWriter.Write(LogEvent{
		Type:      "plugin_output",
		Timestamp: time.Now().UTC(),
		Content:   string(p),
	}); err != nil {
		return 0, err
	}
	return len(p), nil
}

// GetPlugin returns the underlying plugin.
func (a *PluginAgent) GetPlugin() *plugin.Plugin {
	return a.plugin
}

// GetConfig returns the agent configuration.
func (a *PluginAgent) GetConfig() Config {
	return a.cfg
}

// PluginAgentFactory creates an AgentFactory from a plugin.
func PluginAgentFactory(p *plugin.Plugin) AgentFactory {
	return func(cfg Config) (Agent, error) {
		return NewPluginAgent(p, cfg)
	}
}

// RegisterAgentPlugin registers an agent plugin with the agent registry.
func RegisterAgentPlugin(p *plugin.Plugin) error {
	if p == nil {
		return fmt.Errorf("plugin is nil")
	}

	if !p.IsAgent() {
		return fmt.Errorf("plugin %q is not an agent plugin", p.Name)
	}

	agentType := AgentType(p.GetAgentType())
	if agentType == "" {
		return fmt.Errorf("plugin %q has no agent type", p.Name)
	}

	// Register with the global registry
	RegisterAgent(agentType, PluginAgentFactory(p))

	return nil
}

// LoadAndRegisterAgentPlugins loads agent plugins from the plugin registry
// and registers them with the agent registry.
func LoadAndRegisterAgentPlugins() error {
	registry := plugin.GetRegistry()

	if !registry.IsInitialized() {
		// Registry not initialized, nothing to load
		return nil
	}

	for _, p := range registry.ListAgents() {
		if err := RegisterAgentPlugin(p); err != nil {
			// Log warning but continue loading other plugins
			// TODO: add proper logging
			continue
		}
	}

	return nil
}

// GetPluginForAgentType returns a plugin for the given agent type, if one exists.
func GetPluginForAgentType(agentType string) *plugin.Plugin {
	registry := plugin.GetRegistry()

	if !registry.IsInitialized() {
		return nil
	}

	return registry.GetByAgentType(agentType)
}

// IsAgentTypePlugin returns true if the agent type is provided by a plugin.
func IsAgentTypePlugin(agentType string) bool {
	return GetPluginForAgentType(agentType) != nil
}
