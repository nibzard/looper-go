// Package plugin provides a plugin system for looper-go.
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nibzard/looper-go/internal/looperdir"
)

// PluginDir is the name of the plugins directory within .looper or ~/.looper.
const PluginDir = "plugins"

// Loader handles plugin discovery and loading from user and project directories.
type Loader struct {
	// UserPluginsDir is the path to the user plugins directory (~/.looper/plugins).
	UserPluginsDir string

	// ProjectRoot is the path to the current project root.
	// If empty, project plugins are not loaded.
	ProjectRoot string

	// LoadedPlugins holds all loaded plugins indexed by name.
	LoadedPlugins map[string]*Plugin

	// mu protects concurrent access to LoadedPlugins.
	mu sync.RWMutex
}

// NewLoader creates a new plugin loader.
func NewLoader(projectRoot string) *Loader {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = filepath.Join(os.Getenv("HOME"), ".looper")
	}

	userPluginsDir := filepath.Join(homeDir, ".looper", PluginDir)

	return &Loader{
		UserPluginsDir: userPluginsDir,
		ProjectRoot:    projectRoot,
		LoadedPlugins:  make(map[string]*Plugin),
	}
}

// DiscoverPlugins discovers and loads plugins from user and project directories.
// Project-scoped plugins override user-scoped plugins with the same name.
func (l *Loader) DiscoverPlugins() ([]*Plugin, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// First, load user plugins
	userPlugins, err := l.loadPluginsFromDir(l.UserPluginsDir, ScopeUser)
	if err != nil {
		// Log warning but don't fail - user plugins directory may not exist yet
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading user plugins: %w", err)
		}
		userPlugins = nil
	}

	// Then, load project plugins (these override user plugins)
	var projectPlugins []*Plugin
	if l.ProjectRoot != "" {
		projectPluginsDir := filepath.Join(l.ProjectRoot, looperdir.Dir, PluginDir)
		projectPlugins, err = l.loadPluginsFromDir(projectPluginsDir, ScopeProject)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("loading project plugins: %w", err)
			}
			projectPlugins = nil
		}
	}

	// Merge plugins: user plugins first, then project plugins override
	merged := make(map[string]*Plugin)

	for _, p := range userPlugins {
		merged[p.Name] = p
	}

	for _, p := range projectPlugins {
		merged[p.Name] = p
	}

	// Convert to slice
	result := make([]*Plugin, 0, len(merged))
	for _, p := range merged {
		result = append(result, p)
		l.LoadedPlugins[p.Name] = p
	}

	return result, nil
}

// loadPluginsFromDir loads all plugins from a directory.
func (l *Loader) loadPluginsFromDir(dir string, scope PluginScope) ([]*Plugin, error) {
	// Check if directory exists
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fmt.Errorf("accessing plugins directory: %w", err)
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading plugins directory: %w", err)
	}

	var plugins []*Plugin

	for _, entry := range entries {
		// Skip non-directories
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		pluginPath := filepath.Join(dir, pluginName)

		// Load the plugin
		plugin, err := l.loadPlugin(pluginPath, scope)
		if err != nil {
			// Log warning but continue loading other plugins
			// TODO: add proper logging
			continue
		}

		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

// loadPlugin loads a single plugin from a directory.
func (l *Loader) loadPlugin(pluginDir string, scope PluginScope) (*Plugin, error) {
	// Parse manifest
	manifest, err := ParseManifest(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Get binary path
	binaryPath, err := GetBinaryPath(pluginDir, manifest)
	if err != nil {
		return nil, fmt.Errorf("getting binary path: %w", err)
	}

	// Validate binary exists (optional - may not be built yet)
	// For now, skip this validation to allow development

	plugin := &Plugin{
		Name:      manifest.Name,
		Version:   manifest.Version,
		Category:  PluginCategory(manifest.Category),
		Manifest:  manifest,
		Path:      pluginDir,
		Scope:     scope,
		BinaryPath: binaryPath,
		Config:    make(map[string]any),
	}

	return plugin, nil
}

// GetPlugin returns a loaded plugin by name.
func (l *Loader) GetPlugin(name string) (*Plugin, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	p, ok := l.LoadedPlugins[name]
	return p, ok
}

// GetPluginsByCategory returns all plugins of a specific category.
func (l *Loader) GetPluginsByCategory(category PluginCategory) []*Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*Plugin
	for _, p := range l.LoadedPlugins {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

// GetAgentPlugins returns all agent plugins.
func (l *Loader) GetAgentPlugins() []*Plugin {
	return l.GetPluginsByCategory(PluginCategoryAgent)
}

// GetWorkflowPlugins returns all workflow plugins.
func (l *Loader) GetWorkflowPlugins() []*Plugin {
	return l.GetPluginsByCategory(PluginCategoryWorkflow)
}

// GetPluginByAgentType returns an agent plugin that provides the given agent type.
func (l *Loader) GetPluginByAgentType(agentType string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, p := range l.LoadedPlugins {
		if p.IsAgent() && p.GetAgentType() == agentType {
			return p
		}
	}
	return nil
}

// GetPluginByWorkflowType returns a workflow plugin that provides the given workflow type.
func (l *Loader) GetPluginByWorkflowType(workflowType string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, p := range l.LoadedPlugins {
		if p.IsWorkflow() && p.GetWorkflowType() == workflowType {
			return p
		}
	}
	return nil
}

// Reload rediscover all plugins.
func (l *Loader) Reload() ([]*Plugin, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clear existing plugins
	l.LoadedPlugins = make(map[string]*Plugin)

	// Rediscover
	return l.DiscoverPlugins()
}

// ValidateAll validates all loaded plugins.
func (l *Loader) ValidateAll() []error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var errs []error

	for _, p := range l.LoadedPlugins {
		if err := ValidateBinaryPath(p.BinaryPath); err != nil {
			errs = append(errs, fmt.Errorf("plugin %s: %w", p.Name, err))
		}
	}

	return errs
}

// ProjectPluginsDir returns the path to the project plugins directory.
func (l *Loader) ProjectPluginsDir() string {
	if l.ProjectRoot == "" {
		return ""
	}
	return filepath.Join(l.ProjectRoot, looperdir.Dir, PluginDir)
}

// EnsureProjectPluginsDir creates the project plugins directory if it doesn't exist.
func (l *Loader) EnsureProjectPluginsDir() error {
	dir := l.ProjectPluginsDir()
	if dir == "" {
		return fmt.Errorf("no project root set")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating project plugins directory: %w", err)
	}

	return nil
}

// EnsureUserPluginsDir creates the user plugins directory if it doesn't exist.
func (l *Loader) EnsureUserPluginsDir() error {
	if err := os.MkdirAll(l.UserPluginsDir, 0755); err != nil {
		return fmt.Errorf("creating user plugins directory: %w", err)
	}

	return nil
}
