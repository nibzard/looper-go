// Package plugin provides a plugin system for looper-go.
package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestFilename is the name of the plugin manifest file.
const ManifestFilename = "looper-plugin.toml"

var (
	// ErrManifestNotFound is returned when the manifest file cannot be found.
	ErrManifestNotFound = errors.New("manifest file not found")

	// ErrInvalidManifest is returned when the manifest is invalid.
	ErrInvalidManifest = errors.New("invalid manifest")

	// ErrInvalidCategory is returned when the plugin category is not recognized.
	ErrInvalidCategory = errors.New("invalid plugin category")

	// ErrMissingName is returned when the manifest is missing a name.
	ErrMissingName = errors.New("missing plugin name")

	// ErrMissingVersion is returned when the manifest is missing a version.
	ErrMissingVersion = errors.New("missing plugin version")

	// ErrMissingBinary is returned when the manifest is missing a binary path.
	ErrMissingBinary = errors.New("missing plugin binary path")
)

// ParseManifest reads and parses a plugin manifest from the given directory.
func ParseManifest(pluginDir string) (*Manifest, error) {
	manifestPath := filepath.Join(pluginDir, ManifestFilename)

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrManifestNotFound, manifestPath)
		}
		return nil, fmt.Errorf("accessing manifest: %w", err)
	}

	// Read manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	// Parse TOML
	var manifest Manifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: parsing TOML: %s", ErrInvalidManifest, err)
	}

	// Validate manifest
	if err := ValidateManifest(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// ValidateManifest validates a plugin manifest.
func ValidateManifest(m *Manifest) error {
	// Check required fields
	if m.Name == "" {
		return fmt.Errorf("%w: name", ErrMissingName)
	}

	if m.Version == "" {
		return fmt.Errorf("%w: version", ErrMissingVersion)
	}

	if m.Category == "" {
		return fmt.Errorf("%w: category", ErrInvalidManifest)
	}

	// Validate category
	category := PluginCategory(m.Category)
	switch category {
	case PluginCategoryAgent, PluginCategoryWorkflow:
		// Valid categories
	default:
		return fmt.Errorf("%w: %s", ErrInvalidCategory, m.Category)
	}

	// Validate plugin metadata
	if m.Plugin.Binary == "" {
		return fmt.Errorf("%w: plugin.binary", ErrMissingBinary)
	}

	// Validate category-specific configuration
	switch category {
	case PluginCategoryAgent:
		if m.Agent == nil {
			return fmt.Errorf("%w: agent configuration required for agent plugins", ErrInvalidManifest)
		}
		if m.Agent.Type == "" {
			return fmt.Errorf("%w: agent.type is required", ErrInvalidManifest)
		}

	case PluginCategoryWorkflow:
		if m.Workflow == nil {
			return fmt.Errorf("%w: workflow configuration required for workflow plugins", ErrInvalidManifest)
		}
		if m.Workflow.Type == "" {
			return fmt.Errorf("%w: workflow.type is required", ErrInvalidManifest)
		}
	}

	// Validate plugin name is a valid identifier
	if err := validatePluginName(m.Name); err != nil {
		return err
	}

	return nil
}

// validatePluginName validates that a plugin name is a valid identifier.
// Plugin names should be lowercase alphanumeric with hyphens or underscores.
func validatePluginName(name string) error {
	if name == "" {
		return ErrMissingName
	}

	// Check for invalid characters
	for _, r := range name {
		if !isAlphaNumHyphenUnderscore(r) {
			return fmt.Errorf("%w: invalid plugin name %q (use alphanumeric, hyphens, underscores)", ErrInvalidManifest, name)
		}
	}

	// Name cannot start with a hyphen or underscore
	if name[0] == '-' || name[0] == '_' {
		return fmt.Errorf("%w: plugin name cannot start with hyphen or underscore", ErrInvalidManifest)
	}

	return nil
}

func isAlphaNumHyphenUnderscore(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}

// WriteManifest writes a plugin manifest to the given directory.
func WriteManifest(pluginDir string, manifest *Manifest) error {
	// Validate before writing
	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	// Marshal to TOML
	data, err := toml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	// Write to file
	manifestPath := filepath.Join(pluginDir, ManifestFilename)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

// NormalizePluginName normalizes a plugin name to a canonical form.
// Converts to lowercase and replaces dots/slashes with hyphens.
func NormalizePluginName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	return strings.Trim(name, "-")
}

// IsCompatibleWithVersion checks if the plugin is compatible with the given looper version.
// For now, this does simple string comparison. A full semver implementation could be added.
func IsCompatibleWithVersion(minVersion, currentVersion string) bool {
	if minVersion == "" {
		return true // No minimum version specified
	}

	// For now, assume compatibility if current version is "dev" or empty
	if currentVersion == "" || currentVersion == "dev" {
		return true
	}

	// Simple version check: if minVersion is "0.1.0" and current is "0.2.0", it's compatible
	// This is a simplified check - a full semver library would be better
	minParts := strings.Split(minVersion, ".")
	currParts := strings.Split(currentVersion, ".")

	// Compare major versions
	if len(minParts) > 0 && len(currParts) > 0 {
		if minParts[0] != currParts[0] {
			return minParts[0] < currParts[0]
		}
	}

	return true
}

// GetBinaryPath returns the absolute path to the plugin's binary.
// The binary path in the manifest is relative to the plugin directory.
func GetBinaryPath(pluginDir string, manifest *Manifest) (string, error) {
	binaryRelPath := manifest.Plugin.Binary
	if binaryRelPath == "" {
		return "", ErrMissingBinary
	}

	// Make absolute
	binaryPath := filepath.Join(pluginDir, binaryRelPath)

	// Clean the path
	binaryPath = filepath.Clean(binaryPath)

	return binaryPath, nil
}

// ValidateBinaryPath checks if the plugin binary exists and is executable.
func ValidateBinaryPath(binaryPath string) error {
	info, err := os.Stat(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plugin binary not found: %s", binaryPath)
		}
		return fmt.Errorf("accessing plugin binary: %w", err)
	}

	// Check if it's a regular file
	if info.IsDir() {
		return fmt.Errorf("plugin binary is a directory, not a file: %s", binaryPath)
	}

	// Check executable bit (Unix-like systems)
	// On Windows, this check is less meaningful but doesn't hurt
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("plugin binary is not executable: %s", binaryPath)
	}

	return nil
}

// CreateDefaultManifest creates a default manifest for a new plugin.
func CreateDefaultManifest(name, category string) *Manifest {
	return &Manifest{
		Name:        name,
		Version:     "1.0.0",
		Category:    category,
		Description: "A " + category + " plugin for looper",
		Plugin: PluginMetadata{
			Binary:           "./bin/" + name,
			Author:           "",
			Homepage:         "",
			License:          "MIT",
			MinLooperVersion: "0.1.0",
		},
	}
}

// AgentManifestForType creates an agent manifest for a given agent type.
func AgentManifestForType(agentType string) *Manifest {
	manifest := CreateDefaultManifest(agentType, "agent")
	manifest.Agent = &AgentConfig{
		Type:                agentType,
		SupportsStreaming:   true,
		SupportsTools:       true,
		DefaultPromptFormat: "stdin",
	}
	manifest.Capabilities = &Capabilities{
		CanModifyFiles:     true,
		CanExecuteCommands: true,
		CanAccessNetwork:   false,
		CanAccessEnv:       true,
	}
	return manifest
}

// WorkflowManifestForType creates a workflow manifest for a given workflow type.
func WorkflowManifestForType(workflowType string) *Manifest {
	manifest := CreateDefaultManifest(workflowType, "workflow")
	manifest.Workflow = &WorkflowConfig{
		Type:           workflowType,
		SupportsParallel: false,
		SupportsReview: true,
		MaxIterations:  50,
	}
	return manifest
}
