// Package plugin provides a plugin system for looper-go.
package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// CapabilityType represents a type of capability that can be checked or enforced.
type CapabilityType string

const (
	// CapabilityModifyFiles allows plugins to create/modify files.
	CapabilityModifyFiles CapabilityType = "modify_files"

	// CapabilityExecuteCommands allows plugins to run shell commands.
	CapabilityExecuteCommands CapabilityType = "execute_commands"

	// CapabilityAccessNetwork allows plugins to make network requests.
	CapabilityAccessNetwork CapabilityType = "access_network"

	// CapabilityAccessEnv allows plugins to read environment variables.
	CapabilityAccessEnv CapabilityType = "access_env"

	// CapabilityReadFiles allows plugins to read files from the filesystem.
	CapabilityReadFiles CapabilityType = "read_files"
)

// PermissionLevel represents the permission level for a capability.
type PermissionLevel int

const (
	// PermissionDenied means the capability is explicitly denied.
	PermissionDenied PermissionLevel = iota

	// PermissionPrompt means the user should be prompted before allowing.
	PermissionPrompt

	// PermissionGranted means the capability is allowed.
	PermissionGranted
)

// String returns the string representation of the permission level.
func (p PermissionLevel) String() string {
	switch p {
	case PermissionDenied:
		return "denied"
	case PermissionPrompt:
		return "prompt"
	case PermissionGranted:
		return "granted"
	default:
		return "unknown"
	}
}

// CapabilityRequest represents a request for a capability.
type CapabilityRequest struct {
	// Plugin is the plugin requesting the capability.
	Plugin *Plugin

	// Capability is the capability being requested.
	Capability CapabilityType

	// Context provides additional context about the request.
	// For example, for file operations, this might include the file path.
	Context map[string]string

	// Reason is why the plugin needs this capability.
	Reason string
}

// CapabilityManager manages plugin capabilities and permissions.
type CapabilityManager struct {
	// permissions holds the permission level for each plugin's capabilities.
	// Key format: "pluginName:capabilityType"
	permissions map[string]PermissionLevel

	// mu protects concurrent access to permissions.
	mu sync.RWMutex

	// promptUser is called when permission level is PermissionPrompt.
	// If nil, defaults to denying the capability.
	promptUser func(*CapabilityRequest) (bool, error)

	// logCallback is called for capability checks (for audit logging).
	logCallback func(plugin *Plugin, capability CapabilityType, granted bool)
}

// NewCapabilityManager creates a new capability manager.
func NewCapabilityManager() *CapabilityManager {
	return &CapabilityManager{
		permissions: make(map[string]PermissionLevel),
		promptUser:  nil, // Will default to denial
		logCallback: nil,
	}
}

// SetPermission sets the permission level for a plugin's capability.
func (m *CapabilityManager) SetPermission(pluginName string, capability CapabilityType, level PermissionLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.permissionKey(pluginName, capability)
	m.permissions[key] = level
}

// GetPermission gets the permission level for a plugin's capability.
func (m *CapabilityManager) GetPermission(pluginName string, capability CapabilityType) PermissionLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.permissionKey(pluginName, capability)
	if level, ok := m.permissions[key]; ok {
		return level
	}

	// Check if the plugin manifest declares this capability
	// If the plugin doesn't declare it, we should probably prompt
	return PermissionPrompt
}

// CheckCapability checks if a plugin has permission to use a capability.
// Returns true if granted, false if denied.
func (m *CapabilityManager) CheckCapability(ctx context.Context, plugin *Plugin, capability CapabilityType) (bool, error) {
	return m.CheckCapabilityWithContext(ctx, plugin, capability, nil)
}

// CheckCapabilityWithContext checks capability with additional context.
func (m *CapabilityManager) CheckCapabilityWithContext(ctx context.Context, plugin *Plugin, capability CapabilityType, context map[string]string) (bool, error) {
	// First, check the plugin manifest
	if !m.pluginHasCapability(plugin, capability) {
		// Plugin doesn't declare this capability - deny by default
		m.logCallbackIfSet(plugin, capability, false)
		return false, fmt.Errorf("plugin %q does not declare capability %q", plugin.Name, capability)
	}

	// Get permission level
	level := m.GetPermission(plugin.Name, capability)

	switch level {
	case PermissionGranted:
		m.logCallbackIfSet(plugin, capability, true)
		return true, nil

	case PermissionDenied:
		m.logCallbackIfSet(plugin, capability, false)
		return false, fmt.Errorf("capability %q denied for plugin %q", capability, plugin.Name)

	case PermissionPrompt:
		// Prompt the user
		if m.promptUser == nil {
			// No prompt handler - deny by default
			m.logCallbackIfSet(plugin, capability, false)
			return false, fmt.Errorf("capability %q requires permission for plugin %q (no prompt handler)", capability, plugin.Name)
		}

		req := &CapabilityRequest{
			Plugin:     plugin,
			Capability: capability,
			Context:    context,
		}

		granted, err := m.promptUser(req)
		m.logCallbackIfSet(plugin, capability, granted)

		if err != nil {
			return false, err
		}

		// Remember the user's choice
		newLevel := PermissionDenied
		if granted {
			newLevel = PermissionGranted
		}
		m.SetPermission(plugin.Name, capability, newLevel)

		return granted, nil

	default:
		m.logCallbackIfSet(plugin, capability, false)
		return false, fmt.Errorf("unknown permission level for plugin %q capability %q", plugin.Name, capability)
	}
}

// pluginHasCapability checks if a plugin's manifest declares a capability.
func (m *CapabilityManager) pluginHasCapability(plugin *Plugin, capability CapabilityType) bool {
	if plugin.Manifest == nil || plugin.Manifest.Capabilities == nil {
		return false
	}

	caps := plugin.Manifest.Capabilities

	switch capability {
	case CapabilityModifyFiles:
		return caps.CanModifyFiles
	case CapabilityExecuteCommands:
		return caps.CanExecuteCommands
	case CapabilityAccessNetwork:
		return caps.CanAccessNetwork
	case CapabilityAccessEnv:
		return caps.CanAccessEnv
	case CapabilityReadFiles:
		// Read is typically implied by modify
		return caps.CanModifyFiles || caps.CanAccessEnv
	default:
		return false
	}
}

// permissionKey creates the map key for permissions.
func (m *CapabilityManager) permissionKey(pluginName string, capability CapabilityType) string {
	return fmt.Sprintf("%s:%s", pluginName, capability)
}

// SetPromptUser sets the callback for prompting the user about permissions.
func (m *CapabilityManager) SetPromptUser(fn func(*CapabilityRequest) (bool, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promptUser = fn
}

// SetLogCallback sets the callback for logging capability checks.
func (m *CapabilityManager) SetLogCallback(fn func(*Plugin, CapabilityType, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logCallback = fn
}

// logCallbackIfSet calls the log callback if it's set.
func (m *CapabilityManager) logCallbackIfSet(plugin *Plugin, capability CapabilityType, granted bool) {
	if m.logCallback != nil {
		m.logCallback(plugin, capability, granted)
	}
}

// Reset clears all permission settings.
func (m *CapabilityManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.permissions = make(map[string]PermissionLevel)
}

// ExportPermissions exports all permissions as a map.
func (m *CapabilityManager) ExportPermissions() map[string]PermissionLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]PermissionLevel, len(m.permissions))
	for k, v := range m.permissions {
		result[k] = v
	}
	return result
}

// ImportPermissions imports permissions from a map.
func (m *CapabilityManager) ImportPermissions(permissions map[string]PermissionLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.permissions = make(map[string]PermissionLevel)
	for k, v := range permissions {
		m.permissions[k] = v
	}
}

// RestrictedCommandBuilder builds shell commands with capability checks.
type RestrictedCommandBuilder struct {
	manager *CapabilityManager
	plugin  *Plugin
}

// NewRestrictedCommandBuilder creates a new restricted command builder.
func NewRestrictedCommandBuilder(manager *CapabilityManager, plugin *Plugin) *RestrictedCommandBuilder {
	return &RestrictedCommandBuilder{
		manager: manager,
		plugin:  plugin,
	}
}

// Command creates a command with capability checks.
// Returns an error if the plugin doesn't have permission to execute commands.
func (b *RestrictedCommandBuilder) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	// Check if plugin can execute commands
	granted, err := b.manager.CheckCapabilityWithContext(ctx, b.plugin, CapabilityExecuteCommands, map[string]string{
		"command": name,
		"args":    strings.Join(args, " "),
	})

	if err != nil {
		return nil, err
	}

	if !granted {
		return nil, fmt.Errorf("plugin %q does not have permission to execute commands", b.plugin.Name)
	}

	cmd := exec.CommandContext(ctx, name, args...)
	return cmd, nil
}

// RestrictedFileWriter provides file writing with capability checks.
type RestrictedFileWriter struct {
	manager *CapabilityManager
	plugin  *Plugin
	baseDir string // Optional base directory for file operations
}

// NewRestrictedFileWriter creates a new restricted file writer.
func NewRestrictedFileWriter(manager *CapabilityManager, plugin *Plugin, baseDir string) *RestrictedFileWriter {
	return &RestrictedFileWriter{
		manager: manager,
		plugin:  plugin,
		baseDir: baseDir,
	}
}

// WriteFile writes a file with capability checks.
func (w *RestrictedFileWriter) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	// Resolve path relative to baseDir if set
	fullPath := path
	if w.baseDir != "" && !filepath.IsAbs(path) {
		fullPath = filepath.Join(w.baseDir, path)
	}

	// Check if plugin can modify files
	granted, err := w.manager.CheckCapabilityWithContext(ctx, w.plugin, CapabilityModifyFiles, map[string]string{
		"file": fullPath,
	})

	if err != nil {
		return err
	}

	if !granted {
		return fmt.Errorf("plugin %q does not have permission to modify file %q", w.plugin.Name, path)
	}

	return os.WriteFile(fullPath, data, perm)
}

// ReadFile reads a file with capability checks.
func (w *RestrictedFileWriter) ReadFile(ctx context.Context, path string) ([]byte, error) {
	// Resolve path relative to baseDir if set
	fullPath := path
	if w.baseDir != "" && !filepath.IsAbs(path) {
		fullPath = filepath.Join(w.baseDir, path)
	}

	// Read capability - check if plugin can read files
	// For now, we use the same check as modify (reading is implied)
	granted, err := w.manager.CheckCapabilityWithContext(ctx, w.plugin, CapabilityReadFiles, map[string]string{
		"file": fullPath,
	})

	if err != nil {
		return nil, err
	}

	if !granted {
		return nil, fmt.Errorf("plugin %q does not have permission to read file %q", w.plugin.Name, path)
	}

	return os.ReadFile(fullPath)
}

// Global capability manager instance
var globalCapabilityManager *CapabilityManager
var capabilityManagerOnce sync.Once

// GetGlobalCapabilityManager returns the global capability manager.
func GetGlobalCapabilityManager() *CapabilityManager {
	capabilityManagerOnce.Do(func() {
		globalCapabilityManager = NewCapabilityManager()
	})
	return globalCapabilityManager
}
