// Package plugin provides a plugin system for looper-go.
package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Validator performs validation on plugins.
type Validator struct {
	// StrictMode enables stricter validation rules.
	StrictMode bool

	// SkipBinaryCheck skips validation of plugin binaries.
	// Useful for development when binaries aren't built yet.
	SkipBinaryCheck bool

	// LoomerVersion is the current looper version for compatibility checking.
	LooperVersion string
}

// ValidationResult holds the results of plugin validation.
type ValidationResult struct {
	Valid  bool
	Errors []string
	Warnings []string
}

// NewValidator creates a new plugin validator.
func NewValidator() *Validator {
	return &Validator{
		StrictMode:      false,
		SkipBinaryCheck: false,
		LooperVersion:   "dev",
	}
}

// ValidatePlugin validates a plugin from a directory.
func (v *Validator) ValidatePlugin(pluginDir string) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check if directory exists
	if _, err := os.Stat(pluginDir); err != nil {
		if os.IsNotExist(err) {
			result.Errors = append(result.Errors, fmt.Sprintf("plugin directory does not exist: %s", pluginDir))
		} else {
			result.Errors = append(result.Errors, fmt.Sprintf("cannot access plugin directory: %s", err))
		}
		result.Valid = false
		return result
	}

	// Parse manifest
	manifest, err := ParseManifest(pluginDir)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("manifest error: %s", err))
		result.Valid = false
		return result
	}

	// Validate manifest
	v.validateManifest(manifest, result)

	// Get binary path
	binaryPath, err := GetBinaryPath(pluginDir, manifest)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("binary path error: %s", err))
		result.Valid = false
		return result
	}

	// Validate binary
	if !v.SkipBinaryCheck {
		v.validateBinary(binaryPath, manifest, result)
	}

	// Validate dependencies
	v.validateDependencies(manifest, result)

	// Validate capabilities
	v.validateCapabilities(manifest, result)

	return result
}

// validateManifest validates the manifest structure and required fields.
func (v *Validator) validateManifest(manifest *Manifest, result *ValidationResult) {
	// Basic validation already done by ParseManifest
	// Additional checks:

	// Check version format
	if !isValidVersion(manifest.Version) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("version %q does not follow semver format (e.g., 1.0.0)", manifest.Version))
	}

	// Check compatibility
	if !IsCompatibleWithVersion(manifest.Plugin.MinLooperVersion, v.LooperVersion) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("plugin requires looper version %s, current is %s", manifest.Plugin.MinLooperVersion, v.LooperVersion))
	}

	// Category-specific validation
	category := PluginCategory(manifest.Category)
	switch category {
	case PluginCategoryAgent:
		if manifest.Agent == nil {
			result.Errors = append(result.Errors, "agent plugin missing [agent] section")
			result.Valid = false
		} else {
			if manifest.Agent.Type == "" {
				result.Errors = append(result.Errors, "agent plugin missing agent.type")
				result.Valid = false
			}
			// Validate prompt format
			if manifest.Agent.DefaultPromptFormat != "" {
				if manifest.Agent.DefaultPromptFormat != "stdin" && manifest.Agent.DefaultPromptFormat != "arg" {
					result.Warnings = append(result.Warnings, fmt.Sprintf("unknown prompt format %q (should be 'stdin' or 'arg')", manifest.Agent.DefaultPromptFormat))
				}
			}
		}

	case PluginCategoryWorkflow:
		if manifest.Workflow == nil {
			result.Errors = append(result.Errors, "workflow plugin missing [workflow] section")
			result.Valid = false
		} else {
			if manifest.Workflow.Type == "" {
				result.Errors = append(result.Errors, "workflow plugin missing workflow.type")
				result.Valid = false
			}
		}
	}
}

// validateBinary validates the plugin binary.
func (v *Validator) validateBinary(binaryPath string, manifest *Manifest, result *ValidationResult) {
	// Check if binary exists
	if err := ValidateBinaryPath(binaryPath); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("binary validation failed: %s", err))
		result.Valid = false
		return
	}

	// Try to run binary with --help or --version
	// This validates that it's actually executable
	var output []byte
	var err error

	// Try --version first
	output, err = exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		// Try --help
		output, err = exec.Command(binaryPath, "--help").CombinedOutput()
		if err != nil {
			// Log the command output for debugging
			if len(output) > 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("binary output from --version/--help: %s", string(output)))
			}
			// In strict mode, fail if binary doesn't respond to --version or --help
			if v.StrictMode {
				result.Errors = append(result.Errors, fmt.Sprintf("binary does not respond to --version or --help: %s", err))
				result.Valid = false
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("binary does not respond to --version or --help: %s", err))
			}
		}
	}

	// Check binary is not a script (security warning)
	ext := strings.ToLower(filepath.Ext(binaryPath))
	if ext == ".sh" || ext == ".bash" || ext == ".py" || ext == ".rb" || ext == ".pl" {
		result.Warnings = append(result.Warnings, "binary is a script; consider compiling to a native binary for better performance")
	}
}

// validateDependencies validates that plugin dependencies are available.
func (v *Validator) validateDependencies(manifest *Manifest, result *ValidationResult) {
	if manifest.Dependencies == nil {
		return
	}

	// Check for required binaries in PATH
	for _, binary := range manifest.Dependencies.Binaries {
		if _, err := exec.LookPath(binary); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("required binary %q not found in PATH", binary))
		}
	}

	// Note: We can't easily validate API keys here, but we can warn about them
	if len(manifest.Dependencies.APIKeys) > 0 {
		keysStr := strings.Join(manifest.Dependencies.APIKeys, ", ")
		result.Warnings = append(result.Warnings, fmt.Sprintf("plugin requires API keys: %s (ensure these are configured)", keysStr))
	}
}

// validateCapabilities validates plugin capabilities.
func (v *Validator) validateCapabilities(manifest *Manifest, result *ValidationResult) {
	if manifest.Capabilities == nil {
		return
	}

	// Warn about dangerous capabilities
	if manifest.Capabilities.CanExecuteCommands {
		result.Warnings = append(result.Warnings, "plugin can execute commands (ensure you trust this plugin)")
	}

	if manifest.Capabilities.CanAccessNetwork {
		result.Warnings = append(result.Warnings, "plugin can access network (ensure you trust this plugin)")
	}

	// For agent plugins, can_modify_files is typically required
	if manifest.Category == "agent" && !manifest.Capabilities.CanModifyFiles {
		result.Warnings = append(result.Warnings, "agent plugin cannot modify files (this may be intentional)")
	}
}

// isValidVersion checks if a version string follows semver format loosely.
func isValidVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	// Check that we have at least major.minor
	for i := 0; i < 2 && i < len(parts); i++ {
		if parts[i] == "" {
			return false
		}
	}

	return true
}

// ValidatePluginDir validates all plugins in a directory.
func (v *Validator) ValidatePluginDir(pluginDir string) map[string]*ValidationResult {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return map[string]*ValidationResult{
			"": {
				Valid:  false,
				Errors: []string{fmt.Sprintf("cannot read plugin directory: %s", err)},
			},
		}
	}

	results := make(map[string]*ValidationResult)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(pluginDir, name)

		result := v.ValidatePlugin(path)
		results[name] = result
	}

	return results
}

// FormatValidationResult formats a validation result for human display.
func FormatValidationResult(pluginName string, result *ValidationResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Plugin: %s\n", pluginName))

	if result.Valid {
		sb.WriteString("Status: VALID\n")
	} else {
		sb.WriteString("Status: INVALID\n")
	}

	if len(result.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range result.Errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, warn := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", warn))
		}
	}

	return sb.String()
}

// ValidatePluginAt validates a plugin at the given path using the default validator.
func ValidatePluginAt(pluginDir string) error {
	validator := NewValidator()
	result := validator.ValidatePlugin(pluginDir)

	if !result.Valid {
		return fmt.Errorf("plugin validation failed:\n%s", FormatValidationResult(pluginDir, result))
	}

	return nil
}

// ValidatePluginQuick performs quick validation without running the binary.
func ValidatePluginQuick(pluginDir string) error {
	validator := NewValidator()
	validator.SkipBinaryCheck = true
	result := validator.ValidatePlugin(pluginDir)

	if !result.Valid {
		return fmt.Errorf("plugin validation failed:\n%s", FormatValidationResult(pluginDir, result))
	}

	return nil
}
