// Package config tests configuration loading.
package config

import (
	"flag"
	"os"
	"path/filepath"
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
		tests = append(tests, struct{ input string; want string }{
			input: `~\test`,
			want:  filepath.Join(home, "test"),
		}, struct{ input string; want string }{
			input: `%LOOPER_TEST_HOME%\logs`,
			want:  filepath.Join(home, "logs"),
		})
	} else {
		tests = append(tests, struct{ input string; want string }{
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
	t.Run("GetAgent returns built-in agents", func(t *testing.T) {
		cfg := AgentConfig{
			Codex:  Agent{Binary: "codex-bin", Model: "gpt-5"},
			Claude: Agent{Binary: "claude-bin", Model: "claude-4"},
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

	t.Run("GetAgent prefers custom agents", func(t *testing.T) {
		cfg := AgentConfig{
			Codex: Agent{Binary: "codex-bin", Model: "gpt-5"},
			Agents: map[string]Agent{
				"codex": {Binary: "custom-codex", Model: "custom-gpt"},
			},
		}

		codexAgent := cfg.GetAgent("codex")
		if codexAgent.Binary != "custom-codex" {
			t.Errorf("GetAgent(codex).Binary = %q, want custom-codex (custom should override built-in)", codexAgent.Binary)
		}
		if codexAgent.Model != "custom-gpt" {
			t.Errorf("GetAgent(codex).Model = %q, want custom-gpt", codexAgent.Model)
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

	t.Run("SetAgent updates built-in agents", func(t *testing.T) {
		cfg := AgentConfig{}
		cfg.SetAgent("codex", Agent{Binary: "my-codex", Model: "my-model"})

		if cfg.Codex.Binary != "my-codex" {
			t.Errorf("SetAgent(codex) - Binary = %q, want my-codex", cfg.Codex.Binary)
		}
		if cfg.Codex.Model != "my-model" {
			t.Errorf("SetAgent(codex) - Model = %q, want my-model", cfg.Codex.Model)
		}

		cfg.SetAgent("claude", Agent{Binary: "my-claude", Model: "claude-model"})
		if cfg.Claude.Binary != "my-claude" {
			t.Errorf("SetAgent(claude) - Binary = %q, want my-claude", cfg.Claude.Binary)
		}
	})

	t.Run("SetAgent adds custom agents", func(t *testing.T) {
		cfg := AgentConfig{}
		cfg.SetAgent("custom-agent", Agent{Binary: "custom-bin", Model: "custom-model"})

		if cfg.Agents == nil {
			t.Fatal("SetAgent(custom) should create Agents map")
		}
		customAgent := cfg.Agents["custom-agent"]
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
				Codex:  Agent{Binary: "codex-bin", Model: "gpt-5"},
				Claude: Agent{Binary: "claude-bin", Model: "claude-4"},
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
				Agents: map[string]Agent{
					"opencode": {Binary: "opencode-bin", Model: "opencode-model"},
				},
			},
		}

		if cfg.GetAgentBinary("opencode") != "opencode-bin" {
			t.Errorf("GetAgentBinary(opencode) = %q, want opencode-bin", cfg.GetAgentBinary("opencode"))
		}
		if cfg.GetAgentModel("opencode") != "opencode-model" {
			t.Errorf("GetAgentModel(opencode) = %q, want opencode-model", cfg.GetAgentModel("opencode"))
		}
	})

	t.Run("custom overrides built-in", func(t *testing.T) {
		cfg := &Config{
			Agents: AgentConfig{
				Codex: Agent{Binary: "codex-bin", Model: "gpt-5"},
				Agents: map[string]Agent{
					"codex": {Binary: "custom-codex", Model: "custom-model"},
				},
			},
		}

		if cfg.GetAgentBinary("codex") != "custom-codex" {
			t.Errorf("GetAgentBinary(codex) = %q, want custom-codex (custom should override)", cfg.GetAgentBinary("codex"))
		}
		if cfg.GetAgentModel("codex") != "custom-model" {
			t.Errorf("GetAgentModel(codex) = %q, want custom-model", cfg.GetAgentModel("codex"))
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

[agents.claude]
binary = "my-claude"
model = "claude-4"

[agents.agents.opencode]
binary = "opencode"
model = "opencode-model"
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
	if cfg.Agents.Codex.Binary != "my-codex" {
		t.Errorf("Codex.Binary: got %q, want my-codex", cfg.Agents.Codex.Binary)
	}
	if cfg.Agents.Codex.Model != "gpt-5" {
		t.Errorf("Codex.Model: got %q, want gpt-5", cfg.Agents.Codex.Model)
	}

	if cfg.Agents.Claude.Binary != "my-claude" {
		t.Errorf("Claude.Binary: got %q, want my-claude", cfg.Agents.Claude.Binary)
	}
	if cfg.Agents.Claude.Model != "claude-4" {
		t.Errorf("Claude.Model: got %q, want claude-4", cfg.Agents.Claude.Model)
	}

	// Check custom agent
	if cfg.Agents.Agents == nil {
		t.Fatal("Agents map should not be nil")
	}
	opencodeAgent := cfg.Agents.Agents["opencode"]
	if opencodeAgent.Binary != "opencode" {
		t.Errorf("opencode.Binary: got %q, want opencode", opencodeAgent.Binary)
	}
	if opencodeAgent.Model != "opencode-model" {
		t.Errorf("opencode.Model: got %q, want opencode-model", opencodeAgent.Model)
	}

	// Test GetAgent methods
	if cfg.GetAgentBinary("codex") != "my-codex" {
		t.Errorf("GetAgentBinary(codex): got %q, want my-codex", cfg.GetAgentBinary("codex"))
	}
	if cfg.GetAgentBinary("opencode") != "opencode" {
		t.Errorf("GetAgentBinary(opencode): got %q, want opencode", cfg.GetAgentBinary("opencode"))
	}
}
