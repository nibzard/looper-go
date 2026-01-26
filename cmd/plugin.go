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

// PluginCommand is the plugin command handler.
type PluginCommand struct {
	config *config.Config
}

// NewPluginCommand creates a new plugin command.
func NewPluginCommand(cfg *config.Config) *PluginCommand {
	return &PluginCommand{
		config: cfg,
	}
}

// Run executes the plugin command.
func (c *PluginCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return c.printUsage()
	}

	subcommand := args[0]
	subargs := args[1:]

	switch subcommand {
	case "install", "add":
		return c.runInstall(ctx, subargs)
	case "uninstall", "remove", "rm":
		return c.runUninstall(ctx, subargs)
	case "list", "ls":
		return c.runList(ctx, subargs)
	case "info", "show":
		return c.runInfo(ctx, subargs)
	case "create", "new":
		return c.runCreate(ctx, subargs)
	case "validate", "check":
		return c.runValidate(ctx, subargs)
	case "help", "-h", "--help":
		return c.printUsage()
	default:
		return fmt.Errorf("unknown plugin command: %s\n\n%w", subcommand, c.printUsage())
	}
}

// printUsage prints usage information for the plugin command.
func (c *PluginCommand) printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: looper plugin <command> [arguments]

Plugin management commands for looper.

Commands:
  install <url|path>    Install a plugin from a git URL or local path
  uninstall <name>      Uninstall a plugin by name
  list, ls              List all installed plugins
  info, show <name>     Show detailed information about a plugin
  create <name>         Create a new plugin skeleton
  validate <path>       Validate a plugin manifest

Options:
  --help, -h            Show this help message

Examples:
  looper plugin install https://github.com/user/looper-claude-plugin
  looper plugin install ./path/to/plugin
  looper plugin list
  looper plugin info claude
  looper plugin uninstall my-agent
  looper plugin create my-custom-agent --type agent
  looper plugin validate ./my-plugin

For more information about a specific command, run:
  looper plugin <command> --help
`)
	return nil
}

// runInstall executes the plugin install command.
func (c *PluginCommand) runInstall(ctx context.Context, args []string) error {
	cmd := NewPluginInstallCommand(c.config)
	return cmd.Run(ctx, args)
}

// runUninstall executes the plugin uninstall command.
func (c *PluginCommand) runUninstall(ctx context.Context, args []string) error {
	cmd := NewPluginUninstallCommand(c.config)
	return cmd.Run(ctx, args)
}

// runList executes the plugin list command.
func (c *PluginCommand) runList(ctx context.Context, args []string) error {
	cmd := NewPluginListCommand(c.config)
	return cmd.Run(ctx, args)
}

// runInfo executes the plugin info command.
func (c *PluginCommand) runInfo(ctx context.Context, args []string) error {
	cmd := NewPluginInfoCommand(c.config)
	return cmd.Run(ctx, args)
}

// runCreate executes the plugin create command.
func (c *PluginCommand) runCreate(ctx context.Context, args []string) error {
	cmd := NewPluginCreateCommand(c.config)
	return cmd.Run(ctx, args)
}

// runValidate executes the plugin validate command.
func (c *PluginCommand) runValidate(ctx context.Context, args []string) error {
	cmd := NewPluginValidateCommand(c.config)
	return cmd.Run(ctx, args)
}

// initializePluginRegistry initializes the plugin registry if not already initialized.
func initializePluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
	registry := plugin.GetRegistry()

	if registry.IsInitialized() {
		// Even if already initialized, apply plugin configs in case they changed
		applyPluginConfigs(cfg, registry)
		return registry, nil
	}

	projectRoot := cfg.ProjectRoot
	if projectRoot == "" {
		// Try to detect project root from current directory
		if wd, err := os.Getwd(); err == nil {
			// Check if we're in a project with .looper directory
			looperDir := filepath.Join(wd, config.DefaultConfigFile)
			if _, err := os.Stat(looperDir); err == nil {
				// Found .looper directory, use current directory as project root
				projectRoot = wd
			}
		}
	}

	if err := registry.Initialize(projectRoot); err != nil {
		return nil, fmt.Errorf("initializing plugin registry: %w", err)
	}

	// Apply plugin configurations from looper.toml
	applyPluginConfigs(cfg, registry)

	return registry, nil
}

// applyPluginConfigs applies plugin configurations from the config to the registry.
func applyPluginConfigs(cfg *config.Config, registry *plugin.Registry) {
	for pluginName, settings := range cfg.Plugins {
		configMap := make(map[string]any)

		// Convert PluginSettings to map[string]any
		if settings.Timeout != "" {
			configMap["timeout"] = settings.Timeout
		}
		if settings.WorkDir != "" {
			configMap["work_dir"] = settings.WorkDir
		}
		if settings.Binary != "" {
			configMap["binary"] = settings.Binary
		}
		if settings.Model != "" {
			configMap["model"] = settings.Model
		}
		if settings.Reasoning != "" {
			configMap["reasoning"] = settings.Reasoning
		}
		// Note: enabled field is handled separately during plugin lookup

		if err := registry.UpdatePluginConfig(pluginName, configMap); err != nil {
			// Log warning but continue - plugin might not be installed yet
			fmt.Fprintf(os.Stderr, "warning: failed to apply config for plugin %s: %v\n", pluginName, err)
		}
	}
}
