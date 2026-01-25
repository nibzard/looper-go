// Package coreplugins provides bundled core plugins that ship with looper.
package coreplugins

import (
	"fmt"
	"os"
	"path/filepath"
)

// claudeManifest returns the manifest for the Claude agent plugin.
func claudeManifest() *Manifest {
	return &Manifest{
		Name:        "claude",
		Version:     "1.0.0",
		Category:    "agent",
		Description: "Claude AI agent integration (built-in)",
		Binary:      "claude",
		Author:      "Anthropic",
		Homepage:    "https://github.com/nibzard/looper-go",
		License:     "MIT",
		MinLooperVersion: "0.1.0",
		AgentType:          "claude",
		SupportsStreaming:  true,
		SupportsTools:      true,
		SupportsMCP:        true,
		DefaultPromptFormat: "stdin",
		CanModifyFiles:     true,
		CanExecuteCommands: true,
		CanAccessNetwork:   false,
		CanAccessEnv:       true,
	}
}

// extractClaudePlugin extracts the Claude plugin to the user plugins directory.
// For built-in plugins, this creates a stub that references the built-in implementation.
func extractClaudePlugin(userPluginsDir string) error {
	// For the built-in Claude agent, we don't need to extract anything
	// The agent is already implemented in internal/agents/claude.go
	// We just create a marker file to indicate it's been "extracted"

	pluginDir := filepath.Join(userPluginsDir, "claude")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}

	// Create a README explaining this is a built-in plugin
	readmeContent := fmt.Sprintf("# Claude Agent Plugin (Built-in)\n\n"+
		"This is the built-in Claude agent plugin. It is embedded in the looper binary\n"+
		"and does not require separate installation.\n\n"+
		"## Usage\n\n"+
		"Use the Claude agent by specifying it in your looper configuration:\n\n"+
		"```toml\n"+
		"[roles]\n"+
		"iter = \"claude\"\n"+
		"review = \"claude\"\n"+
		"```\n\n"+
		"## Configuration\n\n"+
		"Configure Claude-specific settings in your looper.toml:\n\n"+
		"```toml\n"+
		"[agents.claude]\n"+
		"binary = \"claude\"  # Path to claude binary\n"+
		"model = \"\"         # Optional: specify model\n"+
		"reasoning = \"\"     # Optional: reasoning effort (low, medium, high)\n"+
		"```\n\n"+
		"This plugin is managed by looper's built-in agent system.\n")

	return os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte(readmeContent), 0644)
}

// Note: The actual Claude agent implementation is in internal/agents/claude.go
// This coreplugins package only provides the manifest and extraction logic
