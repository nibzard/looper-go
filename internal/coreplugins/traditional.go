// Package coreplugins provides bundled core plugins that ship with looper.
package coreplugins

import (
	"fmt"
	"os"
	"path/filepath"
)

// traditionalManifest returns the manifest for the Traditional workflow plugin.
func traditionalManifest() *Manifest {
	return &Manifest{
		Name:        "traditional",
		Version:     "1.0.0",
		Category:    "workflow",
		Description: "Traditional looper workflow (built-in)",
		Binary:      "traditional",
		Author:      "Looper Team",
		Homepage:    "https://github.com/nibzard/looper-go",
		License:     "MIT",
		MinLooperVersion: "0.1.0",
		WorkflowType:      "traditional",
		SupportsParallel:  false,
		SupportsReview:    true,
		MaxIterations:     50,
		CanModifyFiles:     true,
		CanExecuteCommands: true,
		CanAccessNetwork:   false,
		CanAccessEnv:       true,
	}
}

// extractTraditionalPlugin extracts the Traditional workflow plugin to the user plugins directory.
// For built-in plugins, this creates a stub that references the built-in implementation.
func extractTraditionalPlugin(userPluginsDir string) error {
	// For the built-in Traditional workflow, we don't need to extract anything
	// The workflow is already implemented in internal/loop/loop.go
	// We just create a marker file to indicate it's been "extracted"

	pluginDir := filepath.Join(userPluginsDir, "traditional")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}

	// Create a README explaining this is a built-in plugin
	readmeContent := fmt.Sprintf("# Traditional Workflow Plugin (Built-in)\n\n"+
		"This is the built-in Traditional workflow plugin. It is embedded in the looper binary\n"+
		"and does not require separate installation.\n\n"+
		"## Usage\n\n"+
		"Use the Traditional workflow by specifying it in your looper configuration:\n\n"+
		"```toml\n"+
		"workflow = \"traditional\"\n"+
		"```\n\n"+
		"Or run directly:\n\n"+
		"```bash\n"+
		"looper run\n"+
		"```\n\n"+
		"## Description\n\n"+
		"The Traditional workflow is the classic looper execution model:\n"+
		"1. Bootstrap from project files or user prompt\n"+
		"2. Iterate through tasks until completion\n"+
		"3. Review results\n"+
		"4. Mark as done\n\n"+
		"## Configuration\n\n"+
		"Configure Traditional-specific settings in your looper.toml:\n\n"+
		"```toml\n"+
		"[workflows.traditional]\n"+
		"max_iterations = 50  # Maximum number of iterations\n"+
		"```\n\n"+
		"This plugin is managed by looper's built-in workflow system.\n")

	return os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte(readmeContent), 0644)
}

// Note: The actual Traditional workflow implementation is in internal/loop/loop.go
// This coreplugins package only provides the manifest and extraction logic
