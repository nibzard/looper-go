// Package config tests configuration loading.
package config

import (
	"flag"
	"os"
	"path/filepath"
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
		{"round-robin 1", "round-robin", 1, "codex"},
		{"round-robin 2", "round-robin", 2, "claude"},
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
	if got := cfg.ReviewAgent(); got != "codex" {
		t.Errorf("ReviewAgent: got %q, want codex", got)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save original env
	origTodo := os.Getenv("LOOPER_TODO")
	origMaxIter := os.Getenv("LOOPER_MAX_ITERATIONS")
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
		if origSchedule != "" {
			os.Setenv("LOOPER_SCHEDULE", origSchedule)
		} else {
			os.Unsetenv("LOOPER_SCHEDULE")
		}
	}()

	// Set test env vars
	os.Setenv("LOOPER_TODO", "custom-todo.json")
	os.Setenv("LOOPER_MAX_ITERATIONS", "100")
	os.Setenv("LOOPER_SCHEDULE", "claude")

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
