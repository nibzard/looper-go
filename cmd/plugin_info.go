// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"fmt"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginInfoCommand handles showing detailed information about a plugin.
type PluginInfoCommand struct {
	config *config.Config
}

// NewPluginInfoCommand creates a new plugin info command.
func NewPluginInfoCommand(cfg *config.Config) *PluginInfoCommand {
	return &PluginInfoCommand{
		config: cfg,
	}
}

// Run executes the plugin info command.
func (c *PluginInfoCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing plugin name\n\nUsage: looper plugin info <name>")
	}

	pluginName := args[0]

	// Initialize registry
	registry, err := initializePluginRegistry(c.config)
	if err != nil {
		return err
	}

	// Get plugin
	p, ok := registry.Get(pluginName)
	if !ok {
		return fmt.Errorf("plugin %q not found", pluginName)
	}

	// Print plugin info
	c.printPluginInfo(p)

	return nil
}

// printPluginInfo prints detailed information about a plugin.
func (c *PluginInfoCommand) printPluginInfo(p *plugin.Plugin) {
	fmt.Printf("Name: %s\n", p.Name)

	if p.Version != "" {
		fmt.Printf("Version: %s\n", p.Version)
	}

	fmt.Printf("Category: %s\n", p.Category)
	fmt.Printf("Scope: %s\n", p.Scope)

	if p.Manifest != nil {
		if p.Manifest.Description != "" {
			fmt.Printf("Description: %s\n", p.Manifest.Description)
		}

		if p.Manifest.Plugin.Author != "" {
			fmt.Printf("Author: %s\n", p.Manifest.Plugin.Author)
		}

		if p.Manifest.Plugin.Homepage != "" {
			fmt.Printf("Homepage: %s\n", p.Manifest.Plugin.Homepage)
		}

		if p.Manifest.Plugin.License != "" {
			fmt.Printf("License: %s\n", p.Manifest.Plugin.License)
		}

		if p.Manifest.Plugin.MinLooperVersion != "" {
			fmt.Printf("Minimum Looper Version: %s\n", p.Manifest.Plugin.MinLooperVersion)
		}

		// Type-specific information
		if p.IsAgent() && p.Manifest.Agent != nil {
			fmt.Printf("\nAgent Configuration:\n")
			fmt.Printf("  Type: %s\n", p.Manifest.Agent.Type)
			fmt.Printf("  Supports Streaming: %v\n", p.Manifest.Agent.SupportsStreaming)
			fmt.Printf("  Supports Tools: %v\n", p.Manifest.Agent.SupportsTools)
			if p.Manifest.Agent.DefaultPromptFormat != "" {
				fmt.Printf("  Default Prompt Format: %s\n", p.Manifest.Agent.DefaultPromptFormat)
			}
		}

		if p.IsWorkflow() && p.Manifest.Workflow != nil {
			fmt.Printf("\nWorkflow Configuration:\n")
			fmt.Printf("  Type: %s\n", p.Manifest.Workflow.Type)
			fmt.Printf("  Supports Parallel: %v\n", p.Manifest.Workflow.SupportsParallel)
			fmt.Printf("  Supports Review: %v\n", p.Manifest.Workflow.SupportsReview)
			if p.Manifest.Workflow.MaxIterations > 0 {
				fmt.Printf("  Max Iterations: %d\n", p.Manifest.Workflow.MaxIterations)
			}
		}

		// Dependencies
		if p.Manifest.Dependencies != nil {
			fmt.Printf("\nDependencies:\n")
			if len(p.Manifest.Dependencies.Binaries) > 0 {
				fmt.Printf("  Required Binaries:\n")
				for _, bin := range p.Manifest.Dependencies.Binaries {
					fmt.Printf("    - %s\n", bin)
				}
			}
			if len(p.Manifest.Dependencies.APIKeys) > 0 {
				fmt.Printf("  Required API Keys:\n")
				for _, key := range p.Manifest.Dependencies.APIKeys {
					fmt.Printf("    - %s\n", key)
				}
			}
		}

		// Capabilities
		if p.Manifest.Capabilities != nil {
			fmt.Printf("\nCapabilities:\n")
			fmt.Printf("  Can Modify Files: %v\n", p.Manifest.Capabilities.CanModifyFiles)
			fmt.Printf("  Can Execute Commands: %v\n", p.Manifest.Capabilities.CanExecuteCommands)
			fmt.Printf("  Can Access Network: %v\n", p.Manifest.Capabilities.CanAccessNetwork)
			fmt.Printf("  Can Access Environment: %v\n", p.Manifest.Capabilities.CanAccessEnv)
		}
	}

	fmt.Printf("\nPath: %s\n", p.Path)
	fmt.Printf("Binary: %s\n", p.BinaryPath)

	// Configuration
	if len(p.Config) > 0 {
		fmt.Printf("\nConfiguration:\n")
		for k, v := range p.Config {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
}
