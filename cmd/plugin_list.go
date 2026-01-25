// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginListCommand handles listing installed plugins.
type PluginListCommand struct {
	config     *config.Config
	showAll    bool
	category   string
	verbose    bool
}

// NewPluginListCommand creates a new plugin list command.
func NewPluginListCommand(cfg *config.Config) *PluginListCommand {
	return &PluginListCommand{
		config: cfg,
	}
}

// Run executes the plugin list command.
func (c *PluginListCommand) Run(ctx context.Context, args []string) error {
	// Parse options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--all":
			c.showAll = true
		case "--verbose", "-v":
			c.verbose = true
		case "--category":
			if i+1 < len(args) {
				i++
				c.category = args[i]
			}
		case "--help", "-h":
			return c.printUsage()
		}
	}

	// Initialize registry
	registry, err := initializePluginRegistry(c.config)
	if err != nil {
		return err
	}

	// Get all plugins
	plugins := registry.List()

	// Filter by category if specified
	if c.category != "" {
		category := plugin.PluginCategory(c.category)
		var filtered []*plugin.Plugin
		for _, p := range plugins {
			if p.Category == category {
				filtered = append(filtered, p)
			}
		}
		plugins = filtered
	}

	// Sort plugins by name
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	// Print plugins
	if len(plugins) == 0 {
		fmt.Println("No plugins installed")
		return nil
	}

	fmt.Printf("Found %d plugin(s):\n\n", len(plugins))

	for _, p := range plugins {
		c.printPlugin(p)
	}

	return nil
}

// printPlugin prints information about a single plugin.
func (c *PluginListCommand) printPlugin(p *plugin.Plugin) {
	// Format: name@version (scope) - category
	scope := p.Scope.String()
	version := p.Version
	if version == "" {
		version = "unknown"
	}

	fmt.Printf("  %s@%s (%s) - %s", p.Name, version, scope, p.Category)

	if c.verbose {
		// Add more details
		fmt.Printf("\n")

		// Description
		if p.Manifest != nil && p.Manifest.Description != "" {
			fmt.Printf("    Description: %s\n", p.Manifest.Description)
		}

		// Author
		if p.Manifest != nil && p.Manifest.Plugin.Author != "" {
			fmt.Printf("    Author: %s\n", p.Manifest.Plugin.Author)
		}

		// Type-specific info
		if p.IsAgent() {
			fmt.Printf("    Agent Type: %s\n", p.GetAgentType())
		} else if p.IsWorkflow() {
			fmt.Printf("    Workflow Type: %s\n", p.GetWorkflowType())
		}

		// Path
		fmt.Printf("    Path: %s\n", p.Path)

		// Binary
		fmt.Printf("    Binary: %s\n", p.BinaryPath)
	} else {
		fmt.Printf("\n")
	}
}

// printUsage prints usage information.
func (c *PluginListCommand) printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: looper plugin list [options]

List all installed plugins.

Options:
  --all                 Show all plugins including built-ins
  --category <cat>      Filter by category (agent, workflow)
  --verbose, -v         Show detailed information
  --help, -h            Show this help message

Examples:
  looper plugin list
  looper plugin list --verbose
  looper plugin list --category agent
`)
	return nil
}
