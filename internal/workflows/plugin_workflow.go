// Package workflows provides a pluggable loop execution system.
package workflows

import (
	"context"
	"fmt"

	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginWorkflow implements Workflow using a loaded plugin.
type PluginWorkflow struct {
	plugin  *plugin.Plugin
	cfg     interface{}
	workDir string
	todoFile interface{}
}

// NewPluginWorkflow creates a new plugin-based workflow.
func NewPluginWorkflow(p *plugin.Plugin, cfg interface{}, workDir string, todoFile interface{}) (*PluginWorkflow, error) {
	if p == nil {
		return nil, fmt.Errorf("plugin is nil")
	}

	if !p.IsWorkflow() {
		return nil, fmt.Errorf("plugin %q is not a workflow plugin", p.Name)
	}

	return &PluginWorkflow{
		plugin:   p,
		cfg:      cfg,
		workDir:  workDir,
		todoFile: todoFile,
	}, nil
}

// Run executes the plugin workflow.
func (w *PluginWorkflow) Run(ctx context.Context) error {
	executor := plugin.NewExecutor(w.plugin)

	// Convert config to map[string]interface{} if needed
	configMap := make(map[string]interface{})
	if cfgMap, ok := w.cfg.(map[string]interface{}); ok {
		configMap = cfgMap
	} else if w.cfg != nil {
		// Try to get settings from the config
		// This depends on the config structure
		configMap["config"] = w.cfg
	}

	// Apply plugin configuration
	if w.plugin.Config != nil {
		for k, v := range w.plugin.Config {
			configMap[k] = v
		}
	}

	params := plugin.WorkflowRunParams{
		Config:   configMap,
		WorkDir:  w.workDir,
		TodoFile: fmt.Sprintf("%v", w.todoFile),
	}

	result, err := executor.ExecuteWorkflow(ctx, params)
	if err != nil {
		return err
	}

	if !result.Success {
		if result.Error != "" {
			return fmt.Errorf("workflow %q failed: %s", w.plugin.Name, result.Error)
		}
		return fmt.Errorf("workflow %q failed: %s", w.plugin.Name, result.Message)
	}

	return nil
}

// Description returns a description of this workflow.
func (w *PluginWorkflow) Description() string {
	if w.plugin.Manifest != nil && w.plugin.Manifest.Description != "" {
		return fmt.Sprintf("%s (plugin)", w.plugin.Manifest.Description)
	}
	return fmt.Sprintf("Plugin workflow %q", w.plugin.Name)
}

// GetPlugin returns the underlying plugin.
func (w *PluginWorkflow) GetPlugin() *plugin.Plugin {
	return w.plugin
}

// GetConfig returns the workflow configuration.
func (w *PluginWorkflow) GetConfig() interface{} {
	return w.cfg
}

// PluginWorkflowFactory creates a WorkflowFactory from a plugin.
func PluginWorkflowFactory(p *plugin.Plugin) WorkflowFactory {
	return func(cfg interface{}, workDir string, todoFile interface{}) (Workflow, error) {
		return NewPluginWorkflow(p, cfg, workDir, todoFile)
	}
}

// RegisterWorkflowPlugin registers a workflow plugin with the workflow registry.
func RegisterWorkflowPlugin(p *plugin.Plugin) error {
	if p == nil {
		return fmt.Errorf("plugin is nil")
	}

	if !p.IsWorkflow() {
		return fmt.Errorf("plugin %q is not a workflow plugin", p.Name)
	}

	workflowType := WorkflowType(p.GetWorkflowType())
	if workflowType == "" {
		return fmt.Errorf("plugin %q has no workflow type", p.Name)
	}

	// Register with the global registry
	Register(workflowType, PluginWorkflowFactory(p))

	return nil
}

// LoadAndRegisterWorkflowPlugins loads workflow plugins from the plugin registry
// and registers them with the workflow registry.
func LoadAndRegisterWorkflowPlugins() error {
	registry := plugin.GetRegistry()

	if !registry.IsInitialized() {
		// Registry not initialized, nothing to load
		return nil
	}

	for _, p := range registry.ListWorkflows() {
		if err := RegisterWorkflowPlugin(p); err != nil {
			// Log warning but continue loading other plugins
			// TODO: add proper logging
			continue
		}
	}

	return nil
}

// GetPluginForWorkflowType returns a plugin for the given workflow type, if one exists.
func GetPluginForWorkflowType(workflowType string) *plugin.Plugin {
	registry := plugin.GetRegistry()

	if !registry.IsInitialized() {
		return nil
	}

	return registry.GetByWorkflowType(workflowType)
}

// IsWorkflowTypePlugin returns true if the workflow type is provided by a plugin.
func IsWorkflowTypePlugin(workflowType string) bool {
	return GetPluginForWorkflowType(workflowType) != nil
}
