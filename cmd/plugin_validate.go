// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginValidateCommand handles plugin validation.
type PluginValidateCommand struct {
	config    *config.Config
	strict    bool
	quick     bool
	verbose   bool
}

// NewPluginValidateCommand creates a new plugin validate command.
func NewPluginValidateCommand(cfg *config.Config) *PluginValidateCommand {
	return &PluginValidateCommand{
		config: cfg,
	}
}

// Run executes the plugin validate command.
func (c *PluginValidateCommand) Run(ctx context.Context, args []string) error {
	// Parse flags
	if len(args) == 0 {
		return fmt.Errorf("missing plugin path\n\nUsage: looper plugin validate <path> [options]")
	}

	pluginPath := args[0]

	// Parse options
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--strict":
			c.strict = true
		case "--quick":
			c.quick = true
		case "--verbose", "-v":
			c.verbose = true
		case "--help", "-h":
			return c.printUsage()
		}
	}

	// Validate the plugin
	return c.validate(pluginPath)
}

// printUsage prints usage information.
func (c *PluginValidateCommand) printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: looper plugin validate <path> [options]

Validates a plugin manifest and binary.

Arguments:
  path                  Path to the plugin directory

Options:
  --strict              Enable strict validation mode
  --quick               Skip binary validation (manifest only)
  --verbose, -v         Show detailed validation results
  --help, -h            Show this help message

Examples:
  looper plugin validate ./my-plugin
  looper plugin validate ./my-plugin --strict --verbose
  looper plugin validate ./my-plugin --quick
`)
	return nil
}

// validate validates a plugin.
func (c *PluginValidateCommand) validate(pluginPath string) error {
	// Resolve to absolute path
	if !filepath.IsAbs(pluginPath) {
		abs, err := filepath.Abs(pluginPath)
		if err != nil {
			return fmt.Errorf("resolving absolute path: %w", err)
		}
		pluginPath = abs
	}

	// Create validator
	validator := plugin.NewValidator()
	validator.StrictMode = c.strict
	validator.SkipBinaryCheck = c.quick

	// Validate
	result := validator.ValidatePlugin(pluginPath)

	// Get plugin name from manifest (if available)
	pluginName := filepath.Base(pluginPath)
	if result.Valid {
		// Try to get the actual name from manifest
		if manifest, err := plugin.ParseManifest(pluginPath); err == nil {
			pluginName = manifest.Name
		}
	}

	// Print results
	if c.verbose || !result.Valid {
		fmt.Println(plugin.FormatValidationResult(pluginName, result))
	} else {
		// Simple output for valid case
		if result.Valid {
			fmt.Printf("Plugin %q is valid\n", pluginName)
			if len(result.Warnings) > 0 {
				fmt.Printf("\nWarnings:\n")
				for _, w := range result.Warnings {
					fmt.Printf("  - %s\n", w)
				}
			}
		}
	}

	// Return error if invalid
	if !result.Valid {
		return fmt.Errorf("plugin validation failed")
	}

	return nil
}
