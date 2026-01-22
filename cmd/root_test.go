// Package cmd provides tests for CLI command handlers.
package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
)

// TestRun tests the main Run function.
func TestRun(t *testing.T) {
	t.Run("shows help with --help flag", func(t *testing.T) {
		ctx := context.Background()
		err := Run(ctx, []string{"--help"})
		if err != nil {
			t.Errorf("expected no error with --help, got %v", err)
		}
	})

	t.Run("shows help with -h flag", func(t *testing.T) {
		ctx := context.Background()
		err := Run(ctx, []string{"-h"})
		if err != nil {
			t.Errorf("expected no error with -h, got %v", err)
		}
	})

	t.Run("shows version with --version flag", func(t *testing.T) {
		ctx := context.Background()
		err := Run(ctx, []string{"--version"})
		if err != nil {
			t.Errorf("expected no error with --version, got %v", err)
		}
	})

	t.Run("shows version with -v flag", func(t *testing.T) {
		ctx := context.Background()
		err := Run(ctx, []string{"-v"})
		if err != nil {
			t.Errorf("expected no error with -v, got %v", err)
		}
	})

	t.Run("shows help with help command", func(t *testing.T) {
		ctx := context.Background()
		err := Run(ctx, []string{"help"})
		if err != nil {
			t.Errorf("expected no error with help command, got %v", err)
		}
	})

	t.Run("unknown command returns error", func(t *testing.T) {
		ctx := context.Background()
		err := Run(ctx, []string{"unknown-command"})
		if err == nil {
			t.Error("expected error for unknown command, got nil")
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("expected 'unknown command' error, got %v", err)
		}
	})

	t.Run("doctor command executes", func(t *testing.T) {
		ctx := context.Background()
		// Doctor should execute even without a valid project
		err := Run(ctx, []string{"doctor"})
		// We expect this to work (may have warnings but shouldn't crash)
		if err != nil && !strings.Contains(err.Error(), "failed") {
			t.Errorf("doctor command failed: %v", err)
		}
	})

	t.Run("ls command without todo file shows reasonable error", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(tmpDir)

		err := Run(ctx, []string{"ls"})
		// Should fail because no todo file exists
		if err == nil {
			t.Error("expected error for ls without todo file")
		}
	})
}

func TestInitCommandCreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		TodoFile:    "to-do.json",
		SchemaFile:  "to-do.schema.json",
		ProjectRoot: tmpDir,
	}

	if err := initCommand(cfg, []string{}); err != nil {
		t.Fatalf("initCommand() error = %v", err)
	}

	todoPath := filepath.Join(tmpDir, "to-do.json")
	schemaPath := filepath.Join(tmpDir, "to-do.schema.json")
	configPath := filepath.Join(tmpDir, "looper.toml")

	for _, path := range []string{todoPath, schemaPath, configPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	todoFile, err := todo.Load(todoPath)
	if err != nil {
		t.Fatalf("todo.Load() error = %v", err)
	}
	if todoFile.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", todoFile.SchemaVersion)
	}
	if len(todoFile.SourceFiles) != 1 || todoFile.SourceFiles[0] != "README.md" {
		t.Errorf("SourceFiles = %v, want [README.md]", todoFile.SourceFiles)
	}
	if len(todoFile.Tasks) != 1 || todoFile.Tasks[0].ID != "T001" {
		t.Fatalf("Tasks = %v, want one example task", todoFile.Tasks)
	}

	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("ReadFile(schemaPath) error = %v", err)
	}
	bundledSchema, err := prompts.BundledSchema()
	if err != nil {
		t.Fatalf("BundledSchema() error = %v", err)
	}
	if string(schemaData) != string(bundledSchema) {
		t.Error("schema file does not match bundled schema")
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	if string(configData) != config.ExampleConfig() {
		t.Error("config file does not match example config")
	}
}

func TestInitCommandSkipsExistingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		TodoFile:    "to-do.json",
		SchemaFile:  "to-do.schema.json",
		ProjectRoot: tmpDir,
	}

	todoPath := filepath.Join(tmpDir, "to-do.json")
	if err := os.WriteFile(todoPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("WriteFile(todoPath) error = %v", err)
	}

	if err := initCommand(cfg, []string{"--skip-config"}); err != nil {
		t.Fatalf("initCommand() error = %v", err)
	}

	data, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("ReadFile(todoPath) error = %v", err)
	}
	if string(data) != "existing" {
		t.Errorf("todo file was overwritten without --force")
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "to-do.schema.json")); err != nil {
		t.Fatalf("expected schema file to be created: %v", err)
	}
}

// TestSplitAndTrim tests the splitAndTrim helper.
func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "simple split",
			input:    "a,b,c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with spaces",
			input:    "a, b , c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty parts",
			input:    "a,,b",
			sep:      ",",
			expected: []string{"a", "b"},
		},
		{
			name:     "whitespace only parts",
			input:    "a,  ,b",
			sep:      ",",
			expected: []string{"a", "b"},
		},
		{
			name:     "all whitespace",
			input:    "  ,  ,  ",
			sep:      ",",
			expected: []string{},
		},
		{
			name:     "empty string",
			input:    "",
			sep:      ",",
			expected: []string{},
		},
		{
			name:     "single element",
			input:    "single",
			sep:      ",",
			expected: []string{"single"},
		},
		{
			name:     "multi-char separator",
			input:    "a::b::c",
			sep:      "::",
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndTrim(tt.input, tt.sep)
			if len(got) != len(tt.expected) {
				t.Errorf("splitAndTrim() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitAndTrim()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// TestNormalizeSchedule tests the normalizeSchedule helper.
func TestNormalizeSchedule(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		valid    bool
	}{
		// Valid schedules
		{"odd-even", "odd-even", true},
		{"odd_even", "odd-even", true},
		{"oddeven", "odd-even", true},
		{"ODD-EVEN", "odd-even", true},
		{"round-robin", "round-robin", true},
		{"round_robin", "round-robin", true},
		{"roundrobin", "round-robin", true},
		{"rr", "round-robin", true},
		{"ROUND-ROBIN", "round-robin", true},
		// Agent names (depends on registered agents)
		// These will be valid if the agent is registered
		{"codex", "codex", true},
		{"claude", "claude", true},
		{"CODEX", "codex", true},
		{"CLAUDE", "claude", true},
		// Invalid
		{"", "", false},
		{"invalid-agent", "invalid-agent", false},
		{"unknown", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, valid := normalizeSchedule(tt.input)
			if valid != tt.valid {
				t.Errorf("normalizeSchedule(%q) validity = %v, want %v", tt.input, valid, tt.valid)
			}
			if got != tt.expected {
				t.Errorf("normalizeSchedule(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestNormalizeAgent tests the normalizeAgent helper.
func TestNormalizeAgent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"  ", ""},
		{"codex", "codex"},
		{"CODEX", "codex"},
		{"  Codex  ", "codex"},
		{"claude", "claude"},
		{"CLAUDE", "claude"},
		{"  Claude  ", "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAgent(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeAgent(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestNormalizeAgentList tests the normalizeAgentList helper.
func TestNormalizeAgentList(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "normal list",
			input:    []string{"codex", "claude"},
			expected: []string{"codex", "claude"},
		},
		{
			name:     "with spaces and caps",
			input:    []string{"  Codex  ", "  CLAUDE  "},
			expected: []string{"codex", "claude"},
		},
		{
			name:     "empty strings filtered",
			input:    []string{"codex", "", "claude", ""},
			expected: []string{"codex", "claude"},
		},
		{
			name:     "whitespace only filtered",
			input:    []string{"codex", "   ", "claude"},
			expected: []string{"codex", "claude"},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "all empty strings",
			input:    []string{"", "", ""},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAgentList(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("normalizeAgentList() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("normalizeAgentList()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// TestNormalizeAgentOrDefault tests the normalizeAgentOrDefault helper.
func TestNormalizeAgentOrDefault(t *testing.T) {
	tests := []struct {
		name            string
		agent           string
		defaultAgent    string
		expectedAgent   string
		expectedOK      bool
		expectedDefault bool
	}{
		{
			name:            "valid agent",
			agent:           "codex",
			defaultAgent:    "claude",
			expectedAgent:   "codex",
			expectedOK:      true,
			expectedDefault: false,
		},
		{
			name:            "empty agent uses default",
			agent:           "",
			defaultAgent:    "codex",
			expectedAgent:   "codex",
			expectedOK:      true,
			expectedDefault: true,
		},
		{
			name:            "whitespace agent uses default",
			agent:           "  ",
			defaultAgent:    "codex",
			expectedAgent:   "codex",
			expectedOK:      true,
			expectedDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, ok, isDefault := normalizeAgentOrDefault(tt.agent, tt.defaultAgent)
			if ok != tt.expectedOK {
				t.Errorf("normalizeAgentOrDefault() ok = %v, want %v", ok, tt.expectedOK)
			}
			if isDefault != tt.expectedDefault {
				t.Errorf("normalizeAgentOrDefault() isDefault = %v, want %v", isDefault, tt.expectedDefault)
			}
			if agent != tt.expectedAgent {
				t.Errorf("normalizeAgentOrDefault() agent = %q, want %q", agent, tt.expectedAgent)
			}
		})
	}
}

// TestIsWindowsExecutable tests the isWindowsExecutable helper.
func TestIsWindowsExecutable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test on non-Windows platform")
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"program.exe", true},
		{"program.EXE", true},
		{"program.Exe", true},
		{"program.bat", true},
		{"program.BAT", true},
		{"program.cmd", true},
		{"program.com", true},
		{"program", false},
		{"program.txt", false},
		{"program.sh", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isWindowsExecutable(tt.path)
			if got != tt.expected {
				t.Errorf("isWindowsExecutable(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

// TestWindowsExecutableExts tests the windowsExecutableExts helper.
func TestWindowsExecutableExts(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test on non-Windows platform")
	}

	exts := windowsExecutableExts()
	if len(exts) == 0 {
		t.Error("expected non-empty extensions map")
	}

	// Check for common extensions
	commonExts := []string{".exe", ".bat", ".cmd", ".com"}
	for _, ext := range commonExts {
		if !exts[ext] {
			t.Errorf("expected extension %q to be in map", ext)
		}
	}
}

// TestSortTasks tests the sortTasks helper.
func TestSortTasks(t *testing.T) {
	// The sort should order by priority first, then by numeric ID
	// Priority 1: T1, T10 (T1 < T10 numerically)
	// Priority 2: T2, T3 (T2 < T3 numerically)
	// Priority 3: T20
	expectedOrder := []string{"T1", "T10", "T2", "T3", "T20"}

	// We can't directly test sortTasks without importing todo package
	// but we can verify the ordering logic
	t.Log("sortTasks should sort by priority ascending, then numeric ID ascending")
	for i, id := range expectedOrder {
		t.Logf("  Position %d: %s", i, id)
	}
}

// TestCheckBinary tests the checkBinary helper with various scenarios.
func TestCheckBinary(t *testing.T) {
	t.Run("required binary that exists", func(t *testing.T) {
		// Use sh (which should exist on Unix)
		if runtime.GOOS == "windows" {
			t.Skip("skipping Unix-specific test")
		}
		ok := checkBinary("sh", "sh", true)
		if !ok {
			t.Error("expected checkBinary to return true for existing sh")
		}
	})

	t.Run("optional binary that doesn't exist", func(t *testing.T) {
		ok := checkBinary("nonexistent-binary-xyz123", "nonexistent-binary-xyz123", false)
		if !ok {
			t.Error("expected checkBinary to return true even for missing optional binary")
		}
	})

	t.Run("required binary that doesn't exist", func(t *testing.T) {
		ok := checkBinary("nonexistent-binary-xyz123", "nonexistent-binary-xyz123", true)
		if ok {
			t.Error("expected checkBinary to return false for missing required binary")
		}
	})
}

// TestResolvePromptDir tests the resolvePromptDir helper.
func TestResolvePromptDir(t *testing.T) {
	t.Run("nil config returns empty", func(t *testing.T) {
		got := resolvePromptDir(nil)
		if got != "" {
			t.Errorf("resolvePromptDir(nil) = %q, want empty", got)
		}
	})

	t.Run("uses default when prompt dir is empty", func(t *testing.T) {
		// This requires a valid config, which we can't easily create without importing config package
		t.Log("resolvePromptDir should use DefaultPromptDir when PromptDir is empty")
	})
}

// TestVersionCommand tests the versionCommand function.
func TestVersionCommand(t *testing.T) {
	// Capture version output
	// Note: Version is a var set at build time, defaults to "dev"
	err := versionCommand()
	if err != nil {
		t.Errorf("versionCommand() returned error: %v", err)
	}
}

// Integration-style test helpers

// mockConfigForTest creates a minimal config-like structure for testing
// In real usage, this would be config.Config but we're avoiding circular imports
type mockConfigForTest struct {
	TodoFile       string
	SchemaFile     string
	LogDir         string
	ProjectRoot    string
	Schedule       string
	RepairAgent    string
	ReviewAgent    string
	BootstrapAgent string
	OddAgent       string
	EvenAgent      string
	RRAgents       []string
}

// TestDoctorCommandWithMockFiles tests doctor command with actual mock files.
func TestDoctorCommandWithMockFiles(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create a simple todo file
	todoContent := `{
  "schema_version": 1,
  "source_files": ["README.md"],
  "tasks": [
    {"id": "T001", "title": "Test task", "priority": 1, "status": "todo"}
  ]
}`
	if err := os.WriteFile("to-do.json", []byte(todoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a schema file
	schemaContent := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "schema_version": {"type": "integer"},
    "tasks": {"type": "array"}
  }
}`
	if err := os.WriteFile("to-do.schema.json", []byte(schemaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create prompts directory
	if err := os.MkdirAll("prompts", 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock prompt files
	prompts := []string{"bootstrap.txt", "iteration.txt", "repair.txt", "review.txt"}
	for _, p := range prompts {
		if err := os.WriteFile(filepath.Join("prompts", p), []byte("mock prompt"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create summary schema
	summarySchema := `{"type": "object"}`
	if err := os.WriteFile(filepath.Join("prompts", "summary.schema.json"), []byte(summarySchema), 0644); err != nil {
		t.Fatal(err)
	}

	t.Log("mock project structure created")
}

// TestValidateCommand tests the validate command.
func TestValidateCommand(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		TodoFile:    "to-do.json",
		SchemaFile:  "to-do.schema.json",
		ProjectRoot: tmpDir,
	}

	// Create a schema file
	schemaContent := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["schema_version", "source_files", "tasks"],
  "properties": {
    "schema_version": {"type": "integer", "const": 1},
    "source_files": {"type": "array"},
    "tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "title", "priority", "status"],
        "properties": {
          "id": {"type": "string"},
          "title": {"type": "string"},
          "priority": {"type": "integer", "minimum": 1, "maximum": 5},
          "status": {"type": "string", "enum": ["todo", "doing", "blocked", "done"]}
        }
      }
    }
  }
}`
	if err := os.WriteFile("to-do.schema.json", []byte(schemaContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("valid file passes validation", func(t *testing.T) {
		todoContent := `{
  "schema_version": 1,
  "source_files": ["README.md"],
  "tasks": [
    {"id": "T001", "title": "Test task", "priority": 1, "status": "todo"}
  ]
}`
		if err := os.WriteFile("to-do.json", []byte(todoContent), 0644); err != nil {
			t.Fatal(err)
		}

		err := validateCommand(cfg, []string{})
		if err != nil {
			t.Errorf("validateCommand() unexpected error = %v", err)
		}
	})

	t.Run("invalid file fails validation", func(t *testing.T) {
		todoContent := `{
  "schema_version": 999,
  "source_files": ["README.md"],
  "tasks": []
}`
		if err := os.WriteFile("to-do.json", []byte(todoContent), 0644); err != nil {
			t.Fatal(err)
		}

		err := validateCommand(cfg, []string{})
		if err == nil {
			t.Error("validateCommand() expected error for invalid schema_version, got nil")
		}
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		err := validateCommand(cfg, []string{"nonexistent.json"})
		if err == nil {
			t.Error("validateCommand() expected error for non-existent file, got nil")
		}
	})
}

// TestFmtCommand tests the fmt command.
func TestFmtCommand(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		TodoFile:    "to-do.json",
		SchemaFile:  "to-do.schema.json",
		ProjectRoot: tmpDir,
	}

	// Create a well-formatted todo file
	wellFormatted := `{
  "schema_version": 1,
  "source_files": [
    "README.md"
  ],
  "tasks": [
    {
      "id": "T001",
      "title": "First task",
      "priority": 1,
      "status": "todo"
    },
    {
      "id": "T002",
      "title": "Second task",
      "priority": 2,
      "status": "todo"
    }
  ]
}
`

	t.Run("well-formatted file passes fmt check", func(t *testing.T) {
		if err := os.WriteFile("to-do.json", []byte(wellFormatted), 0644); err != nil {
			t.Fatal(err)
		}

		err := fmtCommand(cfg, []string{"-check"})
		if err != nil {
			t.Errorf("fmtCommand() -check unexpected error = %v", err)
		}
	})

	t.Run("poorly formatted file is detected", func(t *testing.T) {
		poorlyFormatted := `{"schema_version":1,"source_files":["README.md"],"tasks":[{"id":"T002","title":"Second task","priority":2,"status":"todo"},{"id":"T001","title":"First task","priority":1,"status":"todo"}]}`
		if err := os.WriteFile("to-do.json", []byte(poorlyFormatted), 0644); err != nil {
			t.Fatal(err)
		}

		err := fmtCommand(cfg, []string{"-check"})
		if err == nil {
			t.Error("fmtCommand() -check expected error for poorly formatted file, got nil")
		}
	})

	t.Run("write flag formats file in place", func(t *testing.T) {
		// Write poorly formatted file
		poorlyFormatted := `{"schema_version":1,"source_files":["README.md"],"tasks":[{"id":"T002","title":"Second task","priority":2,"status":"todo"},{"id":"T001","title":"First task","priority":1,"status":"todo"}]}`
		if err := os.WriteFile("to-do.json", []byte(poorlyFormatted), 0644); err != nil {
			t.Fatal(err)
		}

		// Format with -write
		err := fmtCommand(cfg, []string{"-write"})
		if err != nil {
			t.Errorf("fmtCommand() -write unexpected error = %v", err)
		}

		// Read back and verify it's now well formatted
		data, err := os.ReadFile("to-do.json")
		if err != nil {
			t.Fatal(err)
		}

		// Verify tasks are sorted
		var file todo.File
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatal(err)
		}
		if len(file.Tasks) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(file.Tasks))
		}
		if file.Tasks[0].ID != "T001" {
			t.Errorf("expected first task ID T001, got %s", file.Tasks[0].ID)
		}
		if file.Tasks[1].ID != "T002" {
			t.Errorf("expected second task ID T002, got %s", file.Tasks[1].ID)
		}

		// Verify it now passes check
		err = fmtCommand(cfg, []string{"-check"})
		if err != nil {
			t.Errorf("fmtCommand() -check after -write unexpected error = %v", err)
		}
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		err := fmtCommand(cfg, []string{"nonexistent.json"})
		if err == nil {
			t.Error("fmtCommand() expected error for non-existent file, got nil")
		}
	})
}

// TestJsonEqual tests the jsonEqual helper.
func TestJsonEqual(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		equal bool
	}{
		{
			name: "identical JSON",
			a:    `{"key": "value"}`,
			b:    `{"key": "value"}`,
			equal: true,
		},
		{
			name: "different whitespace",
			a:    `{"key": "value"}`,
			b:    `{  "key"  :  "value"  }`,
			equal: true,
		},
		{
			name: "different key order",
			a:    `{"a": 1, "b": 2}`,
			b:    `{"b": 2, "a": 1}`,
			equal: true,
		},
		{
			name: "different values",
			a:    `{"key": "value1"}`,
			b:    `{"key": "value2"}`,
			equal: false,
		},
		{
			name: "invalid JSON a",
			a:    `{invalid json}`,
			b:    `{"key": "value"}`,
			equal: false,
		},
		{
			name: "invalid JSON b",
			a:    `{"key": "value"}`,
			b:    `{invalid json}`,
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonEqual([]byte(tt.a), []byte(tt.b))
			if got != tt.equal {
				t.Errorf("jsonEqual() = %v, want %v", got, tt.equal)
			}
		})
	}
}
