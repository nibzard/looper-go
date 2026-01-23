package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// findConfigFile looks for config files in the following order:
// 1. User config directory (~/.looper/looper.toml or OS-specific config dir)
// 2. Project directory (./looper.toml or ./.looper.toml)
// Returns the first config file found, or empty string if none exist.
//
// Deprecated: Use findUserConfigFile and findProjectConfigFile separately
// for proper merge order. This is kept for backwards compatibility.
func findConfigFile() string {
	// First check for user-level config
	if userCfg := findUserConfigFile(); userCfg != "" {
		return userCfg
	}

	// Then check for project config in current directory
	return findProjectConfigFile()
}

// findProjectConfigFile looks for a config file in the current directory.
func findProjectConfigFile() string {
	// Check for looper.toml in current directory
	names := []string{"looper.toml", ".looper.toml"}
	for _, name := range names {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

// findUserConfigFile looks for a user-level config file.
// Checks ~/.looper/looper.toml first, then falls back to OS-specific
// config directories if ~/.looper doesn't exist.
func findUserConfigFile() string {
	// First try ~/.looper/looper.toml
	home, err := os.UserHomeDir()
	if err == nil {
		userConfigPath := filepath.Join(home, ".looper", "looper.toml")
		if _, err := os.Stat(userConfigPath); err == nil {
			return userConfigPath
		}
	}

	// If ~/.looper doesn't exist, try OS-specific config directories
	if cfgDir := osUserConfigDir(); cfgDir != "" {
		userConfigPath := filepath.Join(cfgDir, "looper", "looper.toml")
		if _, err := os.Stat(userConfigPath); err == nil {
			return userConfigPath
		}
	}

	return ""
}

// osUserConfigDir returns the OS-specific user config directory.
// Returns empty string if the directory cannot be determined.
func osUserConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		// On Windows, use %APPDATA%\looper
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return appdata
		}
	case "darwin":
		// On macOS, use ~/Library/Application Support
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, "Library", "Application Support")
		}
	case "linux", "openbsd", "freebsd", "netbsd":
		// On Linux/BSD, respect XDG_CONFIG_HOME or use ~/.config
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return xdg
		}
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".config")
		}
	}
	return ""
}

// setDefaults applies default values to the config.
func setDefaults(cfg *Config) {
	cfg.TodoFile = DefaultTodoFile
	cfg.SchemaFile = "to-do.schema.json"
	cfg.LogDir = DefaultLogDir
	cfg.MaxIterations = DefaultMaxIterations
	cfg.ApplySummary = DefaultApplySummary
	cfg.GitInit = true
	cfg.LoopDelaySeconds = 0

	// Logging defaults
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}

	// Set default roles
	if cfg.Roles == nil {
		cfg.Roles = make(RolesConfig)
	}
	cfg.Roles["iter"] = "claude"
	cfg.Roles["review"] = "claude"
	cfg.Roles["repair"] = "claude"
	cfg.Roles["bootstrap"] = "claude"

	// Default agent binaries with parsers
	cfg.Agents.SetAgent("codex", Agent{
		Binary:      DefaultAgentBinaries()["codex"],
		PromptFormat: PromptFormatStdin,
		Parser:      "codex_parser.py",
	})
	cfg.Agents.SetAgent("claude", Agent{
		Binary:      DefaultAgentBinaries()["claude"],
		PromptFormat: PromptFormatArg,
		Parser:      "claude_parser.py",
	})
}

// GetConfigFile returns the active config file path (project or user).
func (cws *ConfigWithSources) GetConfigFile() string {
	for _, source := range cws.Sources {
		if source == SourceProjFile {
			projectConfigFile := findProjectConfigFile()
			if projectConfigFile != "" {
				return projectConfigFile
			}
		}
	}
	for _, source := range cws.Sources {
		if source == SourceUserFile {
			userConfigFile := findUserConfigFile()
			if userConfigFile != "" {
				return userConfigFile
			}
		}
	}
	return ""
}
