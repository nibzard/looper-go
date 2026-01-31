// Package coreplugins provides bundled core plugins that ship with looper.
// These plugins are embedded in the binary and auto-extracted on first run.
package coreplugins

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	// bundleMutex protects bundle extraction.
	bundleMutex sync.Mutex

	// extracted indicates whether core plugins have been extracted.
	extracted bool
)

// Manifest represents the manifest data for a core plugin.
// This is a simplified version that doesn't import the plugin package
// to avoid import cycles.
type Manifest struct {
	Name        string
	Version     string
	Category    string
	Description string
	Binary      string
	Author      string
	Homepage    string
	License     string
	MinLooperVersion string

	// Agent-specific fields
	AgentType          string
	SupportsStreaming  bool
	SupportsTools      bool
	SupportsMCP        bool
	DefaultPromptFormat string

	// Workflow-specific fields
	WorkflowType      string
	SupportsParallel  bool
	SupportsReview    bool
	MaxIterations     int

	// Capabilities
	CanModifyFiles     bool
	CanExecuteCommands bool
	CanAccessNetwork   bool
	CanAccessEnv       bool
}

// Extract extracts core plugins to the user plugins directory.
// This is called automatically on first run if the plugins don't exist.
func Extract(userPluginsDir string) error {
	bundleMutex.Lock()
	defer bundleMutex.Unlock()

	// Check if already extracted
	if extracted {
		return nil
	}

	// Create user plugins directory if it doesn't exist
	if err := os.MkdirAll(userPluginsDir, 0755); err != nil {
		return fmt.Errorf("creating user plugins directory: %w", err)
	}

	// Extract each core plugin
	if err := extractClaudePlugin(userPluginsDir); err != nil {
		return fmt.Errorf("extracting claude plugin: %w", err)
	}

	if err := extractCodexPlugin(userPluginsDir); err != nil {
		return fmt.Errorf("extracting codex plugin: %w", err)
	}

	if err := extractTraditionalPlugin(userPluginsDir); err != nil {
		return fmt.Errorf("extracting traditional plugin: %w", err)
	}

	extracted = true
	return nil
}

// EnsureExtracted ensures core plugins are extracted.
// Returns true if extraction was performed, false if already existed.
func EnsureExtracted(userPluginsDir string) (bool, error) {
	bundleMutex.Lock()
	defer bundleMutex.Unlock()

	// Check if already extracted
	if extracted {
		return false, nil
	}

	// Check if core plugins already exist on disk
	claudePath := filepath.Join(userPluginsDir, "claude", "README.md")
	if _, err := os.Stat(claudePath); err == nil {
		// Core plugins already exist
		extracted = true
		return false, nil
	}

	// Create user plugins directory if it doesn't exist
	if err := os.MkdirAll(userPluginsDir, 0755); err != nil {
		return false, fmt.Errorf("creating user plugins directory: %w", err)
	}

	// Extract each core plugin (inline to avoid deadlock from calling Extract which also acquires lock)
	if err := extractClaudePlugin(userPluginsDir); err != nil {
		return false, fmt.Errorf("extracting claude plugin: %w", err)
	}

	if err := extractCodexPlugin(userPluginsDir); err != nil {
		return false, fmt.Errorf("extracting codex plugin: %w", err)
	}

	if err := extractTraditionalPlugin(userPluginsDir); err != nil {
		return false, fmt.Errorf("extracting traditional plugin: %w", err)
	}

	extracted = true
	return true, nil
}

// GetCoreManifests returns the manifests for all core plugins.
// These can be used to register the plugins with the plugin registry.
func GetCoreManifests() map[string]*Manifest {
	return map[string]*Manifest{
		"claude":      claudeManifest(),
		"codex":       codexManifest(),
		"traditional": traditionalManifest(),
	}
}

// IsCorePlugin returns true if the plugin name is a core plugin.
func IsCorePlugin(name string) bool {
	switch name {
	case "claude", "codex", "traditional":
		return true
	default:
		return false
	}
}

// CorePluginNames returns a list of core plugin names.
func CorePluginNames() []string {
	return []string{"claude", "codex", "traditional"}
}
