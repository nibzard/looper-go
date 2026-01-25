// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginUninstallCommand handles plugin uninstallation.
type PluginUninstallCommand struct {
	config *config.Config
	force  bool
}

// NewPluginUninstallCommand creates a new plugin uninstall command.
func NewPluginUninstallCommand(cfg *config.Config) *PluginUninstallCommand {
	return &PluginUninstallCommand{
		config: cfg,
	}
}

// Run executes the plugin uninstall command.
func (c *PluginUninstallCommand) Run(ctx context.Context, args []string) error {
	// Parse options
	if len(args) == 0 {
		return c.printUsage()
	}

	pluginName := args[0]

	// Parse options
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--force":
			c.force = true
		case "--help", "-h":
			return c.printUsage()
		}
	}

	// Uninstall the plugin
	return c.uninstall(pluginName)
}

// printUsage prints usage information.
func (c *PluginUninstallCommand) printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: looper plugin uninstall <name> [options]

Uninstall a plugin by name.

Arguments:
  name                  Name of the plugin to uninstall

Options:
  --force               Force removal without confirmation
  --help, -h            Show this help message

Examples:
  looper plugin uninstall my-agent
  looper plugin uninstall my-agent --force
`)
	return nil
}

// uninstall removes a plugin.
func (c *PluginUninstallCommand) uninstall(pluginName string) error {
	// Initialize registry
	registry, err := initializePluginRegistry(c.config)
	if err != nil {
		return err
	}

	// Check if plugin exists
	p, ok := registry.Get(pluginName)
	if !ok {
		return fmt.Errorf("plugin %q not found", pluginName)
	}

	// Check if it's a built-in plugin
	if p.Scope == plugin.ScopeBuiltin {
		return fmt.Errorf("cannot uninstall built-in plugin %q", pluginName)
	}

	// Confirm unless force is set
	if !c.force {
		fmt.Printf("Are you sure you want to uninstall plugin %q? [y/N] ", pluginName)
		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if response != "y" && response != "Y" {
			fmt.Println("Uninstall cancelled")
			return nil
		}
	}

	// Get the plugin path before unregistering
	pluginPath := p.Path

	// Unregister from registry
	if err := registry.UninstallPlugin(pluginName); err != nil {
		return fmt.Errorf("unregistering plugin: %w", err)
	}

	// Remove the plugin directory
	if err := os.RemoveAll(pluginPath); err != nil {
		return fmt.Errorf("removing plugin directory: %w", err)
	}

	fmt.Printf("Plugin %q uninstalled successfully\n", pluginName)
	return nil
}
