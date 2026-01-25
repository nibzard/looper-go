// Package cmd implements the CLI command structure for looper.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/plugin"
)

// PluginInstallCommand handles plugin installation.
type PluginInstallCommand struct {
	config    *config.Config
	scopeStr  string
	force     bool
	skipValidation bool
}

// NewPluginInstallCommand creates a new plugin install command.
func NewPluginInstallCommand(cfg *config.Config) *PluginInstallCommand {
	return &PluginInstallCommand{
		config:   cfg,
		scopeStr: "user", // Default to user scope
	}
}

// Run executes the plugin install command.
func (c *PluginInstallCommand) Run(ctx context.Context, args []string) error {
	// Parse flags (simplified - in production use proper flag parsing)
	if len(args) == 0 {
		return fmt.Errorf("missing plugin source\n\nUsage: looper plugin install <url|path> [options]")
	}

	source := args[0]

	// Parse options
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--scope" && i+1 < len(args):
			i++
			c.scopeStr = args[i]
		case arg == "--force":
			c.force = true
		case arg == "--skip-validation":
			c.skipValidation = true
		case arg == "--help" || arg == "-h":
			return c.printUsage()
		}
	}

	// Validate scope
	var scope plugin.PluginScope
	switch strings.ToLower(c.scopeStr) {
	case "user":
		scope = plugin.ScopeUser
	case "project":
		scope = plugin.ScopeProject
	default:
		return fmt.Errorf("invalid scope: %s (must be 'user' or 'project')", c.scopeStr)
	}

	// Install the plugin
	return c.install(ctx, source, scope)
}

// printUsage prints usage information.
func (c *PluginInstallCommand) printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: looper plugin install <url|path> [options]

Install a plugin from a git URL or local path.

Arguments:
  url|path              Git URL or local path to the plugin

Options:
  --scope <scope>       Installation scope: 'user' (~/.looper/plugins) or 'project' (.looper/plugins)
  --force               Force installation even if plugin already exists
  --skip-validation     Skip plugin validation
  --help, -h            Show this help message

Examples:
  looper plugin install https://github.com/user/looper-claude-plugin
  looper plugin install ./path/to/plugin
  looper plugin install ./my-plugin --scope project
`)
	return nil
}

// install installs a plugin from a source URL or path.
func (c *PluginInstallCommand) install(ctx context.Context, source string, scope plugin.PluginScope) error {
	// Initialize registry
	registry, err := initializePluginRegistry(c.config)
	if err != nil {
		return err
	}

	// Determine source type
	var tempDir string
	var cleanupRequired bool

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "git@") {
		// Git URL
		tempDir, cleanupRequired, err = c.cloneGitRepo(ctx, source)
		if err != nil {
			return fmt.Errorf("cloning git repository: %w", err)
		}
		defer func() {
			if cleanupRequired {
				os.RemoveAll(tempDir)
			}
		}()
	} else {
		// Local path
		tempDir = source
		cleanupRequired = false

		// Resolve to absolute path
		if !filepath.IsAbs(tempDir) {
			abs, err := filepath.Abs(tempDir)
			if err != nil {
				return fmt.Errorf("resolving absolute path: %w", err)
			}
			tempDir = abs
		}

		// Check if directory exists
		if _, err := os.Stat(tempDir); err != nil {
			return fmt.Errorf("accessing plugin directory: %w", err)
		}
	}

	// Parse manifest to get plugin name
	manifest, err := plugin.ParseManifest(tempDir)
	if err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	pluginName := manifest.Name

	// Check if plugin already exists
	if existing, ok := registry.Get(pluginName); ok && !c.force {
		return fmt.Errorf("plugin %q already installed at %s\nUse --force to reinstall", pluginName, existing.Path)
	}

	// Validate plugin unless skipped
	if !c.skipValidation {
		validator := plugin.NewValidator()
		result := validator.ValidatePlugin(tempDir)

		if !result.Valid {
			return fmt.Errorf("plugin validation failed:\n%s", plugin.FormatValidationResult(pluginName, result))
		}

		if len(result.Warnings) > 0 {
			fmt.Printf("Warnings:\n")
			for _, w := range result.Warnings {
				fmt.Printf("  - %s\n", w)
			}
		}
	}

	// Determine target directory
	var targetDir string
	if scope == plugin.ScopeProject {
		if err := registry.EnsureProjectPluginsDir(); err != nil {
			return fmt.Errorf("ensuring project plugins directory: %w", err)
		}
		targetDir = filepath.Join(registry.ProjectPluginsDir(), pluginName)
	} else {
		if err := registry.EnsureUserPluginsDir(); err != nil {
			return fmt.Errorf("ensuring user plugins directory: %w", err)
		}
		targetDir = filepath.Join(registry.UserPluginsDir(), pluginName)
	}

	// If installing from a temp directory, copy to target
	if cleanupRequired {
		if err := c.copyPluginDir(tempDir, targetDir); err != nil {
			return fmt.Errorf("copying plugin: %w", err)
		}
	} else if tempDir != targetDir {
		// Local path but different from target - copy it
		if err := c.copyPluginDir(tempDir, targetDir); err != nil {
			return fmt.Errorf("copying plugin: %w", err)
		}
	}

	// Load the plugin from target directory
	// Create a new plugin instance with the correct scope
	loadedPlugin := &plugin.Plugin{
		Name:      manifest.Name,
		Version:   manifest.Version,
		Category:  plugin.PluginCategory(manifest.Category),
		Manifest:  manifest,
		Path:      targetDir,
		Scope:     scope,
		Config:    make(map[string]any),
	}

	// Get binary path
	binaryPath, err := plugin.GetBinaryPath(targetDir, manifest)
	if err != nil {
		return fmt.Errorf("getting binary path: %w", err)
	}
	loadedPlugin.BinaryPath = binaryPath

	// Register the plugin
	if err := registry.Register(loadedPlugin); err != nil {
		return fmt.Errorf("registering plugin: %w", err)
	}

	fmt.Printf("Plugin %q installed successfully to %s\n", pluginName, targetDir)
	return nil
}

// cloneGitRepo clones a git repository to a temporary directory.
func (c *PluginInstallCommand) cloneGitRepo(ctx context.Context, url string) (string, bool, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "looper-plugin-")
	if err != nil {
		return "", false, fmt.Errorf("creating temp directory: %w", err)
	}

	// Clone the repository
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, tempDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return "", false, fmt.Errorf("git clone failed: %w", err)
	}

	return tempDir, true, nil
}

// copyPluginDir copies a plugin directory to a target location.
func (c *PluginInstallCommand) copyPluginDir(src, dst string) error {
	// Remove target if it exists (for --force)
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("removing existing directory: %w", err)
		}
	}

	// Create target directory
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Copy all files
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, info.Mode())
	})
}
