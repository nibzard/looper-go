// Package config tests configuration loading.
package config

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	if cfg.TodoFile != DefaultTodoFile {
		t.Errorf("TodoFile: got %q, want %q", cfg.TodoFile, DefaultTodoFile)
	}
	if cfg.MaxIterations != DefaultMaxIterations {
		t.Errorf("MaxIterations: got %d, want %d", cfg.MaxIterations, DefaultMaxIterations)
	}
	if cfg.Schedule != "codex" {
		t.Errorf("Schedule: got %q, want codex", cfg.Schedule)
	}
	if cfg.ApplySummary != true {
		t.Errorf("ApplySummary: got %v, want true", cfg.ApplySummary)
	}
}

func TestIterSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		iter     int
		want     string
	}{
		{"codex always", "codex", 1, "codex"},
		{"codex iter 2", "codex", 2, "codex"},
		{"claude always", "claude", 1, "claude"},
		{"odd-even odd", "odd-even", 1, "codex"},
		{"odd-even even", "odd-even", 2, "claude"},
		{"odd-even odd 3", "odd-even", 3, "codex"},
		{"odd-even alias", "odd_even", 2, "claude"},
		{"round-robin 1", "round-robin", 1, "claude"},
		{"round-robin 2", "round-robin", 2, "codex"},
		{"round-robin alias", "round_robin", 1, "claude"},
		{"custom agent schedule", "opencode", 1, "opencode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Schedule: tt.schedule}
			got := cfg.IterSchedule(tt.iter)
			if got != tt.want {
				t.Errorf("IterSchedule(%d): got %q, want %q", tt.iter, got, tt.want)
			}
		})
	}
}

func TestReviewAgent(t *testing.T) {
	cfg := &Config{}
	if got := cfg.GetReviewAgent(); got != "codex" {
		t.Errorf("GetReviewAgent: got %q, want codex", got)
	}
}

func TestBootstrapAgent(t *testing.T) {
	cfg := &Config{}
	if got := cfg.GetBootstrapAgent(); got != "codex" {
		t.Errorf("GetBootstrapAgent: got %q, want codex", got)
	}

	cfg.BootstrapAgent = "claude"
	if got := cfg.GetBootstrapAgent(); got != "claude" {
		t.Errorf("GetBootstrapAgent: got %q, want claude", got)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save original env
	origTodo := os.Getenv("LOOPER_TODO")
	origMaxIter := os.Getenv("LOOPER_MAX_ITERATIONS")
	origIterSchedule := os.Getenv("LOOPER_ITER_SCHEDULE")
	origSchedule := os.Getenv("LOOPER_SCHEDULE")

	defer func() {
		if origTodo != "" {
			os.Setenv("LOOPER_TODO", origTodo)
		} else {
			os.Unsetenv("LOOPER_TODO")
		}
		if origMaxIter != "" {
			os.Setenv("LOOPER_MAX_ITERATIONS", origMaxIter)
		} else {
			os.Unsetenv("LOOPER_MAX_ITERATIONS")
		}
		if origIterSchedule != "" {
			os.Setenv("LOOPER_ITER_SCHEDULE", origIterSchedule)
		} else {
			os.Unsetenv("LOOPER_ITER_SCHEDULE")
		}
		if origSchedule != "" {
			os.Setenv("LOOPER_SCHEDULE", origSchedule)
		} else {
			os.Unsetenv("LOOPER_SCHEDULE")
		}
	}()

	// Set test env vars
	os.Setenv("LOOPER_TODO", "custom-todo.json")
	os.Setenv("LOOPER_MAX_ITERATIONS", "100")
	os.Setenv("LOOPER_ITER_SCHEDULE", "claude")
	os.Setenv("LOOPER_SCHEDULE", "codex")

	cfg := &Config{}
	setDefaults(cfg)
	loadFromEnv(cfg)

	if cfg.TodoFile != "custom-todo.json" {
		t.Errorf("TodoFile: got %q, want custom-todo.json", cfg.TodoFile)
	}
	if cfg.MaxIterations != 100 {
		t.Errorf("MaxIterations: got %d, want 100", cfg.MaxIterations)
	}
	if cfg.Schedule != "claude" {
		t.Errorf("Schedule: got %q, want claude", cfg.Schedule)
	}
}

func TestLoadWithSourcesFlags(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", "")
	t.Setenv("LOOPER_PROMPT_MODE", "")
	t.Setenv("LOOPER_TODO", "")
	t.Setenv("LOOPER_MAX_ITERATIONS", "")
	t.Setenv("LOOPER_ITER_SCHEDULE", "")
	t.Setenv("LOOPER_SCHEDULE", "")

	fs := flag.NewFlagSet("looper", flag.ContinueOnError)
	cws, err := LoadWithSources(fs, []string{"--todo", "custom.json", "--max-iterations", "123", "--schedule", "claude"})
	if err != nil {
		t.Fatalf("LoadWithSources: %v", err)
	}

	wantTodo := filepath.Join(tmpDir, "custom.json")
	if cws.Config.TodoFile != wantTodo {
		t.Errorf("TodoFile: got %q, want %q", cws.Config.TodoFile, wantTodo)
	}
	if cws.Config.MaxIterations != 123 {
		t.Errorf("MaxIterations: got %d, want 123", cws.Config.MaxIterations)
	}
	if cws.Config.Schedule != "claude" {
		t.Errorf("Schedule: got %q, want claude", cws.Config.Schedule)
	}
	if cws.Sources["todo_file"] != SourceFlag {
		t.Errorf("Source todo_file: got %q, want %q", cws.Sources["todo_file"], SourceFlag)
	}
	if cws.Sources["max_iterations"] != SourceFlag {
		t.Errorf("Source max_iterations: got %q, want %q", cws.Sources["max_iterations"], SourceFlag)
	}
	if cws.Sources["schedule"] != SourceFlag {
		t.Errorf("Source schedule: got %q, want %q", cws.Sources["schedule"], SourceFlag)
	}
}

func TestLoadFromEnvAgents(t *testing.T) {
	t.Setenv("CODEX_REASONING", "low")
	t.Setenv("CODEX_REASONING_EFFORT", "high")
	t.Setenv("CODEX_ARGS", "arg1,arg2")
	t.Setenv("CLAUDE_ARGS", "arg3,arg4")

	cfg := &Config{}
	setDefaults(cfg)
	loadFromEnv(cfg)

	codexAgent := cfg.Agents.GetAgent("codex")
	if codexAgent.Reasoning != "high" {
		t.Errorf("codex.Reasoning: got %q, want high", codexAgent.Reasoning)
	}
	if want := []string{"arg1", "arg2"}; !reflect.DeepEqual(codexAgent.Args, want) {
		t.Errorf("codex.Args: got %v, want %v", codexAgent.Args, want)
	}

	claudeAgent := cfg.Agents.GetAgent("claude")
	if want := []string{"arg3", "arg4"}; !reflect.DeepEqual(claudeAgent.Args, want) {
		t.Errorf("claude.Args: got %v, want %v", claudeAgent.Args, want)
	}
}

func TestLoadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "looper.toml")

	content := []byte(`todo_file = "custom.json"
max_iterations = 25
schedule = "claude"
`)
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	if err := loadConfigFile(cfg, configFile); err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}

	if cfg.TodoFile != "custom.json" {
		t.Errorf("TodoFile: got %q, want custom.json", cfg.TodoFile)
	}
	if cfg.MaxIterations != 25 {
		t.Errorf("MaxIterations: got %d, want 25", cfg.MaxIterations)
	}
	if cfg.Schedule != "claude" {
		t.Errorf("Schedule: got %q, want claude", cfg.Schedule)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~", home},
		{"/absolute/path", "/absolute/path"},
		{"relative", "relative"},
	}
	if runtime.GOOS == "windows" {
		t.Setenv("LOOPER_TEST_HOME", home)
		tests = append(tests, struct {
			input string
			want  string
		}{
			input: `~\test`,
			want:  filepath.Join(home, "test"),
		}, struct {
			input string
			want  string
		}{
			input: `%LOOPER_TEST_HOME%\logs`,
			want:  filepath.Join(home, "logs"),
		})
	} else {
		tests = append(tests, struct {
			input string
			want  string
		}{
			input: `~\test`,
			want:  `~\test`,
		})
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.want {
				t.Errorf("expandPath(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	args := []string{
		"--todo", "flag-todo.json",
		"--max-iterations", "75",
		"--schedule", "odd-even",
		"--rr-agents", "claude,codex",
		"--codex-reasoning", "high",
		"--codex-args", "arg1,arg2",
		"--claude-args", "arg3,arg4",
	}

	if err := parseFlags(cfg, fs, args); err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.TodoFile != "flag-todo.json" {
		t.Errorf("TodoFile: got %q, want flag-todo.json", cfg.TodoFile)
	}
	if cfg.MaxIterations != 75 {
		t.Errorf("MaxIterations: got %d, want 75", cfg.MaxIterations)
	}
	if cfg.Schedule != "odd-even" {
		t.Errorf("Schedule: got %q, want odd-even", cfg.Schedule)
	}
	if len(cfg.RRAgents) != 2 || cfg.RRAgents[0] != "claude" || cfg.RRAgents[1] != "codex" {
		t.Errorf("RRAgents: got %v, want [claude codex]", cfg.RRAgents)
	}
	codexAgent := cfg.Agents.GetAgent("codex")
	if codexAgent.Reasoning != "high" {
		t.Errorf("codex.Reasoning: got %q, want high", codexAgent.Reasoning)
	}
	if want := []string{"arg1", "arg2"}; !reflect.DeepEqual(codexAgent.Args, want) {
		t.Errorf("codex.Args: got %v, want %v", codexAgent.Args, want)
	}
	claudeAgent := cfg.Agents.GetAgent("claude")
	if want := []string{"arg3", "arg4"}; !reflect.DeepEqual(claudeAgent.Args, want) {
		t.Errorf("claude.Args: got %v, want %v", claudeAgent.Args, want)
	}
}

func TestBoolFromString(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := boolFromString(tt.input)
			if got != tt.want {
				t.Errorf("boolFromString(%q): got %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestAgentConfig tests the AgentConfig methods.
func TestAgentConfig(t *testing.T) {
	t.Run("GetAgent returns configured agents", func(t *testing.T) {
		cfg := AgentConfig{
			"codex":  {Binary: "codex-bin", Model: "gpt-5"},
			"claude": {Binary: "claude-bin", Model: "claude-4"},
		}

		codexAgent := cfg.GetAgent("codex")
		if codexAgent.Binary != "codex-bin" {
			t.Errorf("GetAgent(codex).Binary = %q, want codex-bin", codexAgent.Binary)
		}
		if codexAgent.Model != "gpt-5" {
			t.Errorf("GetAgent(codex).Model = %q, want gpt-5", codexAgent.Model)
		}

		claudeAgent := cfg.GetAgent("claude")
		if claudeAgent.Binary != "claude-bin" {
			t.Errorf("GetAgent(claude).Binary = %q, want claude-bin", claudeAgent.Binary)
		}
		if claudeAgent.Model != "claude-4" {
			t.Errorf("GetAgent(claude).Model = %q, want claude-4", claudeAgent.Model)
		}
	})

	t.Run("GetAgent returns empty for unknown types", func(t *testing.T) {
		cfg := AgentConfig{}
		agent := cfg.GetAgent("unknown")
		if agent.Binary != "" {
			t.Errorf("GetAgent(unknown).Binary = %q, want empty string", agent.Binary)
		}
		if agent.Model != "" {
			t.Errorf("GetAgent(unknown).Model = %q, want empty string", agent.Model)
		}
	})

	t.Run("SetAgent updates agents", func(t *testing.T) {
		cfg := AgentConfig{}
		cfg.SetAgent("codex", Agent{Binary: "my-codex", Model: "my-model"})

		codexAgent := cfg.GetAgent("codex")
		if codexAgent.Binary != "my-codex" {
			t.Errorf("SetAgent(codex) - Binary = %q, want my-codex", codexAgent.Binary)
		}
		if codexAgent.Model != "my-model" {
			t.Errorf("SetAgent(codex) - Model = %q, want my-model", codexAgent.Model)
		}

		cfg.SetAgent("codex", Agent{Binary: "override-codex", Model: "override-model"})
		codexAgent = cfg.GetAgent("codex")
		if codexAgent.Binary != "override-codex" {
			t.Errorf("SetAgent(codex) override - Binary = %q, want override-codex", codexAgent.Binary)
		}
	})

	t.Run("SetAgent adds custom agents", func(t *testing.T) {
		var cfg AgentConfig
		cfg.SetAgent("custom-agent", Agent{Binary: "custom-bin", Model: "custom-model"})

		if cfg == nil {
			t.Fatal("SetAgent(custom) should initialize Agents map")
		}
		customAgent := cfg.GetAgent("custom-agent")
		if customAgent.Binary != "custom-bin" {
			t.Errorf("SetAgent(custom) - Binary = %q, want custom-bin", customAgent.Binary)
		}
		if customAgent.Model != "custom-model" {
			t.Errorf("SetAgent(custom) - Model = %q, want custom-model", customAgent.Model)
		}
	})
}

// TestConfigGetAgentBinaryAndModel tests the Config methods for getting agent config.
func TestConfigGetAgentBinaryAndModel(t *testing.T) {
	t.Run("built-in agents", func(t *testing.T) {
		cfg := &Config{
			Agents: AgentConfig{
				"codex":  {Binary: "codex-bin", Model: "gpt-5"},
				"claude": {Binary: "claude-bin", Model: "claude-4"},
			},
		}

		if cfg.GetAgentBinary("codex") != "codex-bin" {
			t.Errorf("GetAgentBinary(codex) = %q, want codex-bin", cfg.GetAgentBinary("codex"))
		}
		if cfg.GetAgentModel("codex") != "gpt-5" {
			t.Errorf("GetAgentModel(codex) = %q, want gpt-5", cfg.GetAgentModel("codex"))
		}

		if cfg.GetAgentBinary("claude") != "claude-bin" {
			t.Errorf("GetAgentBinary(claude) = %q, want claude-bin", cfg.GetAgentBinary("claude"))
		}
		if cfg.GetAgentModel("claude") != "claude-4" {
			t.Errorf("GetAgentModel(claude) = %q, want claude-4", cfg.GetAgentModel("claude"))
		}
	})

	t.Run("custom agents", func(t *testing.T) {
		cfg := &Config{
			Agents: AgentConfig{
				"opencode": {Binary: "opencode-bin", Model: "opencode-model"},
			},
		}

		if cfg.GetAgentBinary("opencode") != "opencode-bin" {
			t.Errorf("GetAgentBinary(opencode) = %q, want opencode-bin", cfg.GetAgentBinary("opencode"))
		}
		if cfg.GetAgentModel("opencode") != "opencode-model" {
			t.Errorf("GetAgentModel(opencode) = %q, want opencode-model", cfg.GetAgentModel("opencode"))
		}
	})

	t.Run("fallback to agent name when not configured", func(t *testing.T) {
		cfg := &Config{
			Agents: AgentConfig{},
		}

		if cfg.GetAgentBinary("opencode") != "opencode" {
			t.Errorf("GetAgentBinary(opencode) = %q, want opencode", cfg.GetAgentBinary("opencode"))
		}
	})
}

// TestLoadConfigFileWithCustomAgents tests loading config with custom agents.
func TestLoadConfigFileWithCustomAgents(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "looper.toml")

	content := []byte(`
todo_file = "custom.json"

[agents.codex]
binary = "my-codex"
model = "gpt-5"
reasoning = "medium"
args = ["arg1", "arg2"]

[agents.claude]
binary = "my-claude"
model = "claude-4"
args = ["arg3", "arg4"]

[agents.opencode]
binary = "opencode"
model = "opencode-model"
args = ["arg5"]
`)
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	setDefaults(cfg)
	if err := loadConfigFile(cfg, configFile); err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}

	// Check built-in agents
	codexAgent := cfg.Agents.GetAgent("codex")
	if codexAgent.Binary != "my-codex" {
		t.Errorf("codex.Binary: got %q, want my-codex", codexAgent.Binary)
	}
	if codexAgent.Model != "gpt-5" {
		t.Errorf("codex.Model: got %q, want gpt-5", codexAgent.Model)
	}
	if codexAgent.Reasoning != "medium" {
		t.Errorf("codex.Reasoning: got %q, want medium", codexAgent.Reasoning)
	}
	if want := []string{"arg1", "arg2"}; !reflect.DeepEqual(codexAgent.Args, want) {
		t.Errorf("codex.Args: got %v, want %v", codexAgent.Args, want)
	}

	claudeAgent := cfg.Agents.GetAgent("claude")
	if claudeAgent.Binary != "my-claude" {
		t.Errorf("claude.Binary: got %q, want my-claude", claudeAgent.Binary)
	}
	if claudeAgent.Model != "claude-4" {
		t.Errorf("claude.Model: got %q, want claude-4", claudeAgent.Model)
	}
	if want := []string{"arg3", "arg4"}; !reflect.DeepEqual(claudeAgent.Args, want) {
		t.Errorf("claude.Args: got %v, want %v", claudeAgent.Args, want)
	}

	// Check custom agent
	opencodeAgent := cfg.Agents.GetAgent("opencode")
	if opencodeAgent.Binary != "opencode" {
		t.Errorf("opencode.Binary: got %q, want opencode", opencodeAgent.Binary)
	}
	if opencodeAgent.Model != "opencode-model" {
		t.Errorf("opencode.Model: got %q, want opencode-model", opencodeAgent.Model)
	}
	if want := []string{"arg5"}; !reflect.DeepEqual(opencodeAgent.Args, want) {
		t.Errorf("opencode.Args: got %v, want %v", opencodeAgent.Args, want)
	}

	// Test GetAgent methods
	if cfg.GetAgentBinary("codex") != "my-codex" {
		t.Errorf("GetAgentBinary(codex): got %q, want my-codex", cfg.GetAgentBinary("codex"))
	}
	if cfg.GetAgentBinary("opencode") != "opencode" {
		t.Errorf("GetAgentBinary(opencode): got %q, want opencode", cfg.GetAgentBinary("opencode"))
	}
}

// TestFindUserConfigFile tests user config file discovery.
func TestFindUserConfigFile(t *testing.T) {
	t.Run("findUserConfigFile prefers ~/.looper/looper.toml", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot get home directory")
		}

		userCfgDir := filepath.Join(home, ".looper")
		userCfgFile := filepath.Join(userCfgDir, "looper.toml")

		// Create directory if it doesn't exist
		if err := os.MkdirAll(userCfgDir, 0755); err != nil {
			t.Skipf("Cannot create .looper directory: %v", err)
		}
		defer os.RemoveAll(userCfgDir)

		// Create user config file
		if err := os.WriteFile(userCfgFile, []byte("max_iterations = 100"), 0644); err != nil {
			t.Fatal(err)
		}

		found := findUserConfigFile()
		if found != userCfgFile {
			t.Errorf("findUserConfigFile() = %q, want %q", found, userCfgFile)
		}
	})

	t.Run("findUserConfigFile returns empty when no config exists", func(t *testing.T) {
		// Ensure no ~/.looper/looper.toml exists
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot get home directory")
		}

		userCfgFile := filepath.Join(home, ".looper", "looper.toml")
		os.Remove(userCfgFile)

		// Also check OS-specific config dir doesn't interfere
		cfgDir := osUserConfigDir()
		if cfgDir != "" {
			os.RemoveAll(filepath.Join(cfgDir, "looper"))
		}

		found := findUserConfigFile()
		if found != "" {
			t.Errorf("findUserConfigFile() = %q, want empty string", found)
		}
	})
}

// TestFindProjectConfigFile tests project config file discovery.
func TestFindProjectConfigFile(t *testing.T) {
	t.Run("findProjectConfigFile finds looper.toml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "looper.toml")

		if err := os.WriteFile(configFile, []byte("max_iterations = 50"), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to temp directory
		origWd, _ := os.Getwd()
		defer os.Chdir(origWd)
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatal(err)
		}

		found := findProjectConfigFile()
		if found != "looper.toml" {
			t.Errorf("findProjectConfigFile() = %q, want looper.toml", found)
		}
	})

	t.Run("findProjectConfigFile finds .looper.toml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, ".looper.toml")

		if err := os.WriteFile(configFile, []byte("max_iterations = 50"), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to temp directory
		origWd, _ := os.Getwd()
		defer os.Chdir(origWd)
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatal(err)
		}

		found := findProjectConfigFile()
		if found != ".looper.toml" {
			t.Errorf("findProjectConfigFile() = %q, want .looper.toml", found)
		}
	})

	t.Run("findProjectConfigFile prefers looper.toml over .looper.toml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile1 := filepath.Join(tmpDir, "looper.toml")
		configFile2 := filepath.Join(tmpDir, ".looper.toml")

		if err := os.WriteFile(configFile1, []byte("max_iterations = 50"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configFile2, []byte("max_iterations = 25"), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to temp directory
		origWd, _ := os.Getwd()
		defer os.Chdir(origWd)
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatal(err)
		}

		found := findProjectConfigFile()
		if found != "looper.toml" {
			t.Errorf("findProjectConfigFile() = %q, want looper.toml", found)
		}
	})

	t.Run("findProjectConfigFile returns empty when no config exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Change to temp directory (no config files)
		origWd, _ := os.Getwd()
		defer os.Chdir(origWd)
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatal(err)
		}

		found := findProjectConfigFile()
		if found != "" {
			t.Errorf("findProjectConfigFile() = %q, want empty string", found)
		}
	})
}

// TestConfigMergeOrder tests that user config is loaded first, then project config overrides.
func TestConfigMergeOrder(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	// Create user config
	userCfgDir := filepath.Join(home, ".looper")
	if err := os.MkdirAll(userCfgDir, 0755); err != nil {
		t.Skipf("Cannot create .looper directory: %v", err)
	}
	defer os.RemoveAll(userCfgDir)

	userCfgFile := filepath.Join(userCfgDir, "looper.toml")
	userCfgContent := []byte(`# User config
max_iterations = 100
schedule = "claude"
`)
	if err := os.WriteFile(userCfgFile, userCfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create project config
	tmpDir := t.TempDir()
	projectCfgFile := filepath.Join(tmpDir, "looper.toml")
	projectCfgContent := []byte(`# Project config (overrides user config)
max_iterations = 25  # This should override the user config
todo_file = "project-tasks.json"
`)
	if err := os.WriteFile(projectCfgFile, projectCfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Change to temp directory
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := Load(nil, []string{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// User config sets schedule to claude
	if cfg.Schedule != "claude" {
		t.Errorf("Schedule from user config: got %q, want claude", cfg.Schedule)
	}

	// Project config overrides max_iterations
	if cfg.MaxIterations != 25 {
		t.Errorf("MaxIterations from project config: got %d, want 25", cfg.MaxIterations)
	}

	// Project config sets todo_file (path is made absolute by finalizeConfig)
	wantTodoFile := filepath.Join(tmpDir, "project-tasks.json")
	if cfg.TodoFile != wantTodoFile {
		t.Errorf("TodoFile from project config: got %q, want %q", cfg.TodoFile, wantTodoFile)
	}
}

// TestOSUserConfigDir tests OS-specific config directory detection.
func TestOSUserConfigDir(t *testing.T) {
	// Just test that the function returns something non-empty
	// The exact path depends on the OS and environment
	dir := osUserConfigDir()
	if dir == "" {
		// On some systems or test environments, this might be empty
		// which is acceptable (fallback to ~/.looper)
		t.Skip("osUserConfigDir() returned empty (acceptable fallback)")
	}

	// Verify the path is absolute
	if !filepath.IsAbs(dir) {
		t.Errorf("osUserConfigDir() = %q, want absolute path", dir)
	}
}
