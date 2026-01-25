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

// PluginCreateCommand handles creating a new plugin skeleton.
type PluginCreateCommand struct {
	config  *config.Config
	pluginType string
	outputDir string
	author  string
	license string
	description string
}

// NewPluginCreateCommand creates a new plugin create command.
func NewPluginCreateCommand(cfg *config.Config) *PluginCreateCommand {
	return &PluginCreateCommand{
		config: cfg,
	}
}

// Run executes the plugin create command.
func (c *PluginCreateCommand) Run(ctx context.Context, args []string) error {
	// Parse flags (simplified - in production use proper flag parsing)
	if len(args) == 0 {
		return c.printUsage()
	}

	pluginName := args[0]

	// Parse options
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--type" && i+1 < len(args):
			i++
			c.pluginType = args[i]
		case arg == "--output" && i+1 < len(args):
			i++
			c.outputDir = args[i]
		case arg == "--author" && i+1 < len(args):
			i++
			c.author = args[i]
		case arg == "--license" && i+1 < len(args):
			i++
			c.license = args[i]
		case arg == "--description" && i+1 < len(args):
			i++
			c.description = args[i]
		case arg == "--help" || arg == "-h":
			return c.printUsage()
		}
	}

	// Default to agent type if not specified
	if c.pluginType == "" {
		c.pluginType = "agent"
	}

	// Validate plugin type
	if c.pluginType != "agent" && c.pluginType != "workflow" {
		return fmt.Errorf("invalid plugin type: %s (must be 'agent' or 'workflow')", c.pluginType)
	}

	// Set default license
	if c.license == "" {
		c.license = "MIT"
	}

	// Create the plugin skeleton
	return c.create(pluginName)
}

// printUsage prints usage information.
func (c *PluginCreateCommand) printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: looper plugin create <name> [options]

Creates a new plugin skeleton with the given name.

Arguments:
  name                  Name of the plugin to create

Options:
  --type <type>         Type of plugin: 'agent' or 'workflow' (default: agent)
  --output <dir>        Output directory (default: current directory)
  --author <name>       Plugin author name
  --license <name>      License name (default: MIT)
  --description <text>  Plugin description
  --help, -h            Show this help message

Examples:
  looper plugin create my-agent
  looper plugin create my-workflow --type workflow
  looper plugin create my-agent --author "John Doe" --output ./plugins
`)
	return nil
}

// create creates the plugin skeleton.
func (c *PluginCreateCommand) create(pluginName string) error {
	// Normalize plugin name
	pluginName = plugin.NormalizePluginName(pluginName)

	// Determine output directory
	outputDir := c.outputDir
	if outputDir == "" {
		outputDir = "."
	}
	pluginDir := filepath.Join(outputDir, pluginName)

	// Check if directory already exists
	if _, err := os.Stat(pluginDir); err == nil {
		return fmt.Errorf("directory %s already exists", pluginDir)
	}

	// Create manifest
	var manifest *plugin.Manifest
	if c.pluginType == "agent" {
		manifest = plugin.AgentManifestForType(pluginName)
	} else {
		manifest = plugin.WorkflowManifestForType(pluginName)
	}

	// Set additional metadata
	manifest.Plugin.Author = c.author
	manifest.Plugin.License = c.license
	if c.description != "" {
		manifest.Description = c.description
	}

	// Create plugin directory structure
	if err := c.createDirectoryStructure(pluginDir, manifest); err != nil {
		return fmt.Errorf("creating directory structure: %w", err)
	}

	// Write manifest
	if err := plugin.WriteManifest(pluginDir, manifest); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	// Create binary stub
	if err := c.createBinaryStub(pluginDir, manifest); err != nil {
		return fmt.Errorf("creating binary stub: %w", err)
	}

	// Create README
	if err := c.createReadme(pluginDir, manifest); err != nil {
		return fmt.Errorf("creating README: %w", err)
	}

	fmt.Printf("Plugin %q created successfully at %s\n", pluginName, pluginDir)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. cd %s\n", pluginDir)
	fmt.Printf("  2. Edit the manifest (looper-plugin.toml) if needed\n")
	fmt.Printf("  3. Implement the plugin binary in bin/%s\n", pluginName)
	fmt.Printf("  4. Test: looper plugin validate .\n")
	fmt.Printf("  5. Install: looper plugin install .\n")

	return nil
}

// createDirectoryStructure creates the plugin directory structure.
func (c *PluginCreateCommand) createDirectoryStructure(pluginDir string, manifest *plugin.Manifest) error {
	// Create bin directory
	binDir := filepath.Join(pluginDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	return nil
}

// createBinaryStub creates a stub binary file.
func (c *PluginCreateCommand) createBinaryStub(pluginDir string, manifest *plugin.Manifest) error {
	binPath := filepath.Join(pluginDir, "bin", filepath.Base(manifest.Plugin.Binary))

	// Determine if this is a Go plugin (check if we should create a .go file)
	// For now, create a simple shell script stub

	var content string
	if manifest.Category == "agent" {
		content = fmt.Sprintf("#!/bin/sh\n# %s agent plugin\n# This is a stub - implement your agent here\n\n# For JSON-RPC communication, read request from stdin and write response to stdout\n# Request format: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"run\",\"params\":{...}}\n# Response format: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{...}}\n\n# Read the request\nread -r request\n\n# TODO: Implement your agent logic here\n# Parse the request, execute the agent, and return a response\n\n# Example response (for agent):\necho '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"task_id\":\"T001\",\"status\":\"done\",\"summary\":\"Task completed\"}}'\n", manifest.Name)
	} else {
		content = fmt.Sprintf("#!/bin/sh\n# %s workflow plugin\n# This is a stub - implement your workflow here\n\n# For JSON-RPC communication, read request from stdin and write response to stdout\n# Request format: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"run\",\"params\":{...}}\n# Response format: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{...}}\n\n# Read the request\nread -r request\n\n# TODO: Implement your workflow logic here\n# Parse the request, execute the workflow, and return a response\n\n# Example response (for workflow):\necho '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"success\":true,\"message\":\"Workflow completed\"}}'\n", manifest.Name)
	}

	if err := os.WriteFile(binPath, []byte(content), 0755); err != nil {
		return err
	}

	return nil
}

// createReadme creates a README file for the plugin.
func (c *PluginCreateCommand) createReadme(pluginDir string, manifest *plugin.Manifest) error {
	// Build description string
	description := manifest.Description
	if description == "" {
		description = "A looper plugin"
	}

	// Start building content
	var content string
	content += fmt.Sprintf("# %s Plugin\n\n%s\n\n", manifest.Name, description)

	content += "## Installation\n\n"
	content += fmt.Sprintf("```bash\nlooper plugin install %s\n```\n\n", manifest.Name)

	content += "## Usage\n\n"
	content += fmt.Sprintf("This is a %s plugin.\n\n", manifest.Category)

	if manifest.Category == "agent" {
		content += fmt.Sprintf("Use it by specifying the agent type in your looper configuration:\n\n```toml\n[roles]\niter = \"%s\"\n```\n\n", manifest.Name)
	} else {
		content += fmt.Sprintf("Use it by specifying the workflow in your looper configuration:\n\n```toml\nworkflow = \"%s\"\n```\n\n", manifest.Name)
	}

	content += "## Development\n\n"
	content += fmt.Sprintf("Edit the binary in %s to implement your plugin logic.\n\n", filepath.Base(manifest.Plugin.Binary))
	content += "The plugin communicates via JSON-RPC over stdin/stdout.\n\n"

	content += "### Agent Plugin Protocol\n\n"
	content += "**Request:**\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": 1,\n  \"method\": \"run\",\n  \"params\": {\n    \"prompt\": \"...\",\n    \"context\": {...}\n  }\n}\n```\n\n"
	content += "**Response:**\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": 1,\n  \"result\": {\n    \"task_id\": \"T001\",\n    \"status\": \"done\",\n    \"summary\": \"...\",\n    \"files\": [...],\n    \"blockers\": []\n  }\n}\n```\n\n"

	content += "### Workflow Plugin Protocol\n\n"
	content += "**Request:**\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": 1,\n  \"method\": \"run\",\n  \"params\": {\n    \"config\": {...},\n    \"work_dir\": \"...\",\n    \"todo_file\": \"...\"\n  }\n}\n```\n\n"
	content += "**Response:**\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": 1,\n  \"result\": {\n    \"success\": true,\n    \"message\": \"...\"\n  }\n}\n```\n\n"

	content += fmt.Sprintf("## License\n\n%s\n", manifest.Plugin.License)

	readmePath := filepath.Join(pluginDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(content), 0644); err != nil {
		return err
	}

	return nil
}
