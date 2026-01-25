// Package coreplugins provides bundled core plugins that ship with looper.
package coreplugins

import (
	"fmt"
	"os"
	"path/filepath"
)

// codexManifest returns the manifest for the Codex agent plugin.
func codexManifest() *Manifest {
	return &Manifest{
		Name:        "codex",
		Version:     "1.0.0",
		Category:    "agent",
		Description: "Codex AI agent integration (built-in)",
		Binary:      "codex",
		Author:      "OpenAI",
		Homepage:    "https://github.com/nibzard/looper-go",
		License:     "MIT",
		MinLooperVersion: "0.1.0",
		AgentType:          "codex",
		SupportsStreaming:  true,
		SupportsTools:      true,
		SupportsMCP:        false,
		DefaultPromptFormat: "stdin",
		CanModifyFiles:     true,
		CanExecuteCommands: true,
		CanAccessNetwork:   false,
		CanAccessEnv:       true,
	}
}

// extractCodexPlugin extracts the Codex plugin to the user plugins directory.
// For built-in plugins, this creates a stub that references the built-in implementation.
func extractCodexPlugin(userPluginsDir string) error {
	// For the built-in Codex agent, we don't need to extract anything
	// The agent is already implemented in internal/agents/codex.go
	// We just create a marker file to indicate it's been "extracted"

	pluginDir := filepath.Join(userPluginsDir, "codex")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}

	// Create a README explaining this is a built-in plugin
	readmeContent := fmt.Sprintf("# Codex Agent Plugin (Built-in)\n\n"+
		"This is the built-in Codex agent plugin. It is embedded in the looper binary\n"+
		"and does not require separate installation.\n\n"+
		"## Usage\n\n"+
		"Use the Codex agent by specifying it in your looper configuration:\n\n"+
		"```toml\n"+
		"[roles]\n"+
		"iter = \"codex\"\n"+
		"review = \"codex\"\n"+
		"```\n\n"+
		"## Configuration\n\n"+
		"Configure Codex-specific settings in your looper.toml:\n\n"+
		"```toml\n"+
		"[agents.codex]\n"+
		"binary = \"codex\"  # Path to codex binary\n"+
		"model = \"\"         # Optional: specify model\n"+
		"reasoning = \"\"     # Optional: reasoning effort (low, medium, high)\n"+
		"```\n\n"+
		"This plugin is managed by looper's built-in agent system.\n")

	return os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte(readmeContent), 0644)
}

// Note: The actual Codex agent implementation is in internal/agents/codex.go
// This coreplugins package only provides the manifest and extraction logic
