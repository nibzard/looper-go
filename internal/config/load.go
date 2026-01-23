package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Load loads configuration from multiple sources in priority order:
// 1. Defaults
// 2. User config file (~/.looper/looper.toml or OS-specific config dir)
// 3. Project config file (looper.toml or .looper.toml in current directory)
// 4. Environment variables
// 5. CLI flags
func Load(fs *flag.FlagSet, args []string) (*Config, error) {
	cfg := &Config{}

	// 1. Set defaults
	setDefaults(cfg)

	// 2. Try to load from user config file
	userConfigFile := findUserConfigFile()
	if userConfigFile != "" {
		if err := loadConfigFile(cfg, userConfigFile); err != nil {
			return nil, fmt.Errorf("loading user config file %s: %w", userConfigFile, err)
		}
	}

	// 3. Try to load from project config file (overrides user config)
	projectConfigFile := findProjectConfigFile()
	if projectConfigFile != "" {
		if err := loadConfigFile(cfg, projectConfigFile); err != nil {
			return nil, fmt.Errorf("loading project config file %s: %w", projectConfigFile, err)
		}
	}

	// 4. Override from environment
	loadFromEnv(cfg)

	// 5. Parse CLI flags (they override everything)
	if err := parseFlags(cfg, fs, args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	// 6. Compute derived values
	if err := finalizeConfig(cfg); err != nil {
		return nil, fmt.Errorf("finalizing config: %w", err)
	}

	return cfg, nil
}

// LoadWithSources loads configuration and tracks the source of each value.
// Returns ConfigWithSources containing the config and a map of field names to their sources.
func LoadWithSources(fs *flag.FlagSet, args []string) (*ConfigWithSources, error) {
	sources := make(map[string]ConfigSource)
	cfg := &Config{}

	// 1. Set defaults (all fields start with default source)
	setDefaults(cfg)
	for _, field := range configFields() {
		sources[field] = SourceDefault
	}

	// 2. Try to load from user config file
	userConfigFile := findUserConfigFile()
	if userConfigFile != "" {
		if err := loadConfigFileWithSources(cfg, userConfigFile, sources, SourceUserFile); err != nil {
			return nil, fmt.Errorf("loading user config file %s: %w", userConfigFile, err)
		}
	}

	// 3. Try to load from project config file (overrides user config)
	projectConfigFile := findProjectConfigFile()
	if projectConfigFile != "" {
		if err := loadConfigFileWithSources(cfg, projectConfigFile, sources, SourceProjFile); err != nil {
			return nil, fmt.Errorf("loading project config file %s: %w", projectConfigFile, err)
		}
	}

	// 4. Override from environment
	loadFromEnvWithSources(cfg, sources)

	// 5. Parse CLI flags (they override everything)
	if err := parseFlagsWithSources(cfg, fs, args, sources); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	// 6. Compute derived values
	if err := finalizeConfig(cfg); err != nil {
		return nil, fmt.Errorf("finalizing config: %w", err)
	}

	return &ConfigWithSources{
		Config:  cfg,
		Sources: sources,
	}, nil
}

// configFields returns the list of configurable field names for source tracking.
func configFields() []string {
	return []string{
		"todo_file",
		"schema_file",
		"log_dir",
		"max_iterations",
		"roles",
		"apply_summary",
		"git_init",
		"hook_command",
		"loop_delay_seconds",
		"codex_binary",
		"codex_model",
		"codex_reasoning",
		"codex_args",
		"claude_binary",
		"claude_model",
		"claude_args",
	}
}

// loadConfigFile loads TOML config from the given file.
func loadConfigFile(cfg *Config, path string) error {
	_, err := toml.DecodeFile(path, cfg)
	return err
}

// loadConfigFileWithSources loads TOML config and updates source tracking.
func loadConfigFileWithSources(cfg *Config, path string, sources map[string]ConfigSource, source ConfigSource) error {
	// Create a temporary config to see what values are in the file
	tempCfg := &Config{}
	if _, err := toml.DecodeFile(path, tempCfg); err != nil {
		return err
	}

	// Track non-default values
	if tempCfg.TodoFile != "" && tempCfg.TodoFile != DefaultTodoFile {
		setSource(cfg, &cfg.TodoFile, tempCfg.TodoFile, sources, "todo_file", source)
	}
	if tempCfg.SchemaFile != "" && tempCfg.SchemaFile != "to-do.schema.json" {
		setSource(cfg, &cfg.SchemaFile, tempCfg.SchemaFile, sources, "schema_file", source)
	}
	if tempCfg.LogDir != "" && tempCfg.LogDir != DefaultLogDir {
		setSource(cfg, &cfg.LogDir, tempCfg.LogDir, sources, "log_dir", source)
	}
	if tempCfg.MaxIterations != 0 && tempCfg.MaxIterations != DefaultMaxIterations {
		setSource(cfg, &cfg.MaxIterations, tempCfg.MaxIterations, sources, "max_iterations", source)
	}
	if tempCfg.ApplySummary != DefaultApplySummary {
		setSource(cfg, &cfg.ApplySummary, tempCfg.ApplySummary, sources, "apply_summary", source)
	}
	if tempCfg.GitInit != true {
		setSource(cfg, &cfg.GitInit, tempCfg.GitInit, sources, "git_init", source)
	}
	if tempCfg.HookCommand != "" {
		setSource(cfg, &cfg.HookCommand, tempCfg.HookCommand, sources, "hook_command", source)
	}
	if tempCfg.LoopDelaySeconds != 0 {
		setSource(cfg, &cfg.LoopDelaySeconds, tempCfg.LoopDelaySeconds, sources, "loop_delay_seconds", source)
	}

	// Handle agent configs
	mergeAgentSources(cfg, tempCfg, sources, source, "codex", DefaultAgentBinaries()["codex"])
	mergeAgentSources(cfg, tempCfg, sources, source, "claude", DefaultAgentBinaries()["claude"])

	return nil
}

// finalizeConfig computes derived values and validates paths.
func finalizeConfig(cfg *Config) error {
	// Expand ~ in paths
	cfg.LogDir = expandPath(cfg.LogDir)

	// Determine project root
	if cfg.ProjectRoot == "" {
		// Use current working directory
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		cfg.ProjectRoot = wd
	}

	// Make paths absolute if they're relative
	if !filepath.IsAbs(cfg.TodoFile) {
		cfg.TodoFile = filepath.Join(cfg.ProjectRoot, cfg.TodoFile)
	}
	if !filepath.IsAbs(cfg.SchemaFile) {
		cfg.SchemaFile = filepath.Join(cfg.ProjectRoot, cfg.SchemaFile)
	}

	return nil
}
