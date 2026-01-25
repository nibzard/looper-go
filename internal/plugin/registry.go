// Package plugin provides a plugin system for looper-go.
package plugin

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/nibzard/looper-go/internal/coreplugins"
)

var (
	// globalRegistry is the singleton plugin registry.
	globalRegistry *Registry

	// once ensures the registry is initialized only once.
	once sync.Once
)

// Registry is the global plugin registry.
// It provides a singleton instance for managing plugins across the application.
type Registry struct {
	// loader is the plugin loader.
	loader *Loader

	// plugins holds all registered plugins indexed by name.
	plugins map[string]*Plugin

	// mu protects concurrent access to plugins.
	mu sync.RWMutex

	// initialized indicates whether the registry has been initialized.
	initialized bool
}

// GetRegistry returns the global plugin registry, initializing it if necessary.
func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			plugins: make(map[string]*Plugin),
		}
	})
	return globalRegistry
}

// Initialize initializes the registry with a project root and loads plugins.
func (r *Registry) Initialize(projectRoot string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil // Already initialized
	}

	r.loader = NewLoader(projectRoot)

	// Register built-in core plugins first (these have lowest priority)
	coreManifests := coreplugins.GetCoreManifests()
	for name, cm := range coreManifests {
		// Convert coreplugins.Manifest to Plugin
		manifest := &Manifest{
			Name:        cm.Name,
			Version:     cm.Version,
			Category:    cm.Category,
			Description: cm.Description,
			Plugin: PluginMetadata{
				Binary:           cm.Binary,
				Author:           cm.Author,
				Homepage:         cm.Homepage,
				License:          cm.License,
				MinLooperVersion: cm.MinLooperVersion,
			},
			Capabilities: &Capabilities{
				CanModifyFiles:     cm.CanModifyFiles,
				CanExecuteCommands: cm.CanExecuteCommands,
				CanAccessNetwork:   cm.CanAccessNetwork,
				CanAccessEnv:       cm.CanAccessEnv,
			},
		}

		if cm.Category == "agent" && cm.AgentType != "" {
			manifest.Agent = &AgentConfig{
				Type:                cm.AgentType,
				SupportsStreaming:  cm.SupportsStreaming,
				SupportsTools:      cm.SupportsTools,
				SupportsMCP:        cm.SupportsMCP,
				DefaultPromptFormat: cm.DefaultPromptFormat,
			}
		}

		if cm.Category == "workflow" && cm.WorkflowType != "" {
			manifest.Workflow = &WorkflowConfig{
				Type:             cm.WorkflowType,
				SupportsParallel: cm.SupportsParallel,
				SupportsReview:   cm.SupportsReview,
				MaxIterations:   cm.MaxIterations,
			}
		}

		p := &Plugin{
			Name:      name,
			Version:   cm.Version,
			Category:  PluginCategory(cm.Category),
			Manifest:  manifest,
			Path:      "<builtin>",
			Scope:     ScopeBuiltin,
			Config:    make(map[string]any),
		}
		p.BinaryPath = "<builtin>"

		r.plugins[name] = p
	}

	// Ensure core plugins are extracted to user plugins directory
	// This creates marker files so users know these plugins exist
	// TODO: Re-enable after fixing potential hang
	// if _, err := coreplugins.EnsureExtracted(r.loader.UserPluginsDir); err != nil {
	// 	// Log warning but don't fail - extraction is optional for built-ins
	// }

	// Discover and load user/project plugins (these override built-ins)
	plugins, err := r.loader.DiscoverPlugins()
	if err != nil {
		return fmt.Errorf("discovering plugins: %w", err)
	}

	for _, p := range plugins {
		// User/project plugins override built-in plugins with the same name
		r.plugins[p.Name] = p
	}

	r.initialized = true
	return nil
}

// IsInitialized returns true if the registry has been initialized.
func (r *Registry) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.initialized
}

// Register registers a plugin manually (for built-in or dynamically loaded plugins).
func (r *Registry) Register(plugin *Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if plugin == nil {
		return fmt.Errorf("cannot register nil plugin")
	}

	if plugin.Name == "" {
		return fmt.Errorf("plugin must have a name")
	}

	r.plugins[plugin.Name] = plugin
	return nil
}

// Unregister removes a plugin from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.plugins, name)
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[name]
	return p, ok
}

// GetByAgentType returns an agent plugin that provides the given agent type.
func (r *Registry) GetByAgentType(agentType string) *Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.plugins {
		if p.IsAgent() && p.GetAgentType() == agentType {
			return p
		}
	}
	return nil
}

// GetByWorkflowType returns a workflow plugin that provides the given workflow type.
func (r *Registry) GetByWorkflowType(workflowType string) *Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.plugins {
		if p.IsWorkflow() && p.GetWorkflowType() == workflowType {
			return p
		}
	}
	return nil
}

// List returns all registered plugins.
func (r *Registry) List() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// ListByCategory returns all plugins of a specific category.
func (r *Registry) ListByCategory(category PluginCategory) []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Plugin
	for _, p := range r.plugins {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

// ListAgents returns all agent plugins.
func (r *Registry) ListAgents() []*Plugin {
	return r.ListByCategory(PluginCategoryAgent)
}

// ListWorkflows returns all workflow plugins.
func (r *Registry) ListWorkflows() []*Plugin {
	return r.ListByCategory(PluginCategoryWorkflow)
}

// AgentTypes returns a list of all agent types provided by plugins.
func (r *Registry) AgentTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make(map[string]struct{})
	for _, p := range r.plugins {
		if p.IsAgent() {
			types[p.GetAgentType()] = struct{}{}
		}
	}

	result := make([]string, 0, len(types))
	for t := range types {
		result = append(result, t)
	}
	return result
}

// WorkflowTypes returns a list of all workflow types provided by plugins.
func (r *Registry) WorkflowTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make(map[string]struct{})
	for _, p := range r.plugins {
		if p.IsWorkflow() {
			types[p.GetWorkflowType()] = struct{}{}
		}
	}

	result := make([]string, 0, len(types))
	for t := range types {
		result = append(result, t)
	}
	return result
}

// Reload reloads all plugins from disk.
func (r *Registry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.loader == nil {
		return fmt.Errorf("registry not initialized")
	}

	plugins, err := r.loader.Reload()
	if err != nil {
		return fmt.Errorf("reloading plugins: %w", err)
	}

	// Update registry
	r.plugins = make(map[string]*Plugin)
	for _, p := range plugins {
		r.plugins[p.Name] = p
	}

	return nil
}

// GetLoader returns the plugin loader.
func (r *Registry) GetLoader() *Loader {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loader
}

// UserPluginsDir returns the user plugins directory.
func (r *Registry) UserPluginsDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.loader == nil {
		return ""
	}
	return r.loader.UserPluginsDir
}

// ProjectPluginsDir returns the project plugins directory.
func (r *Registry) ProjectPluginsDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.loader == nil {
		return ""
	}
	return r.loader.ProjectPluginsDir()
}

// ProjectRoot returns the project root.
func (r *Registry) ProjectRoot() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.loader == nil {
		return ""
	}
	return r.loader.ProjectRoot
}

// EnsureProjectPluginsDir creates the project plugins directory if it doesn't exist.
func (r *Registry) EnsureProjectPluginsDir() error {
	r.mu.Lock()
	defer r.mu.RUnlock()

	if r.loader == nil {
		return fmt.Errorf("registry not initialized")
	}

	return r.loader.EnsureProjectPluginsDir()
}

// EnsureUserPluginsDir creates the user plugins directory if it doesn't exist.
func (r *Registry) EnsureUserPluginsDir() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.loader == nil {
		return fmt.Errorf("registry not initialized")
	}

	return r.loader.EnsureUserPluginsDir()
}

// InstallPlugin installs a plugin from a directory to the appropriate location.
func (r *Registry) InstallPlugin(sourceDir string, scope PluginScope) (*Plugin, error) {
	// First, load the plugin to validate it
	manifest, err := ParseManifest(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Determine target directory
	var targetDir string
	if scope == ScopeProject {
		if err := r.EnsureProjectPluginsDir(); err != nil {
			return nil, err
		}
		targetDir = filepath.Join(r.ProjectPluginsDir(), manifest.Name)
	} else {
		if err := r.EnsureUserPluginsDir(); err != nil {
			return nil, err
		}
		targetDir = filepath.Join(r.UserPluginsDir(), manifest.Name)
	}

	// TODO: Implement copying the plugin from sourceDir to targetDir
	// For now, we'll just register it directly
	// In a full implementation, we would:
	// 1. Copy or symlink the plugin directory
	// 2. Run any installation hooks
	// 3. Validate dependencies

	plugin := &Plugin{
		Name:       manifest.Name,
		Version:    manifest.Version,
		Category:   PluginCategory(manifest.Category),
		Manifest:   manifest,
		Path:       targetDir,
		Scope:      scope,
		Config:     make(map[string]any),
	}

	// Get binary path
	binaryPath, err := GetBinaryPath(targetDir, manifest)
	if err != nil {
		return nil, fmt.Errorf("getting binary path: %w", err)
	}
	plugin.BinaryPath = binaryPath

	// Register the plugin
	if err := r.Register(plugin); err != nil {
		return nil, fmt.Errorf("registering plugin: %w", err)
	}

	return plugin, nil
}

// UninstallPlugin removes a plugin from the registry and filesystem.
func (r *Registry) UninstallPlugin(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	// Don't delete built-in plugins
	if p.Scope == ScopeBuiltin {
		return fmt.Errorf("cannot uninstall built-in plugin %q", name)
	}

	// TODO: Implement filesystem cleanup
	// For now, just unregister from registry

	delete(r.plugins, name)
	return nil
}

// ValidateAll validates all registered plugins.
func (r *Registry) ValidateAll() []error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.loader == nil {
		return []error{fmt.Errorf("registry not initialized")}
	}

	return r.loader.ValidateAll()
}

// UpdatePluginConfig updates the configuration for a plugin.
func (r *Registry) UpdatePluginConfig(name string, config map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	if p.Config == nil {
		p.Config = make(map[string]any)
	}

	for k, v := range config {
		p.Config[k] = v
	}

	return nil
}

// GetPluginConfig returns the configuration for a plugin.
func (r *Registry) GetPluginConfig(name string) (map[string]any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[name]
	if !ok {
		return nil, false
	}

	return p.Config, true
}
