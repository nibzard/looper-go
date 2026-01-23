package loop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
)

func init() {
	// Register a test agent type that uses simple output format
	// This allows tests to avoid the complexity of Claude's stream-json format
	agents.RegisterAgent(agents.AgentType("test-stub"), func(cfg agents.Config) (agents.Agent, error) {
		return &testStubAgent{
			binary: cfg.Binary,
		}, nil
	})
}

// testStubAgent is a simple test agent that runs a shell script and parses JSON output.
type testStubAgent struct {
	binary string
}

func (a *testStubAgent) Run(ctx context.Context, prompt string, logWriter agents.LogWriter) (*agents.Summary, error) {
	// Run the stub binary and capture its output
	cmd := exec.CommandContext(ctx, a.binary)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = "" // Use current directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	// Parse the JSON output directly
	var summary agents.Summary
	if err := json.Unmarshal(output, &summary); err != nil {
		return nil, err
	}

	// Handle files and blockers arrays
	if summary.Files == nil {
		summary.Files = []string{}
	}
	if summary.Blockers == nil {
		summary.Blockers = []string{}
	}

	return &summary, nil
}

// TestApplySummary tests applying a summary to a task.
func TestApplySummary(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title": "Summary Schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" },
			"files": { "type": "array", "items": { "type": "string" } },
			"blockers": { "type": "array", "items": { "type": "string" } }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	tests := []struct {
		name     string
		tasks    []todo.Task
		summary  *agents.Summary
		wantErr  bool
		verifyFn func(*testing.T, *todo.File)
	}{
		{
			name: "update existing task to done",
			tasks: []todo.Task{
				{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
			},
			summary: &agents.Summary{
				TaskID:  "T001",
				Status:  "done",
				Summary: "Task completed successfully",
				Files:   []string{"file1.go", "file2.go"},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task == nil {
					t.Fatal("Task T001 not found")
				}
				if task.Status != todo.StatusDone {
					t.Errorf("Task status = %q, want %q", task.Status, todo.StatusDone)
				}
				if task.Details != "Task completed successfully" {
					t.Errorf("Task details = %q, want %q", task.Details, "Task completed successfully")
				}
				if len(task.Files) != 2 {
					t.Errorf("Task files length = %d, want 2", len(task.Files))
				}
			},
		},
		{
			name: "update existing task to blocked with blockers",
			tasks: []todo.Task{
				{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
			},
			summary: &agents.Summary{
				TaskID:   "T001",
				Status:   "blocked",
				Summary:  "Waiting for dependency",
				Blockers: []string{"waiting for T002"},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task == nil {
					t.Fatal("Task T001 not found")
				}
				if task.Status != todo.StatusBlocked {
					t.Errorf("Task status = %q, want %q", task.Status, todo.StatusBlocked)
				}
				if len(task.Blockers) != 1 {
					t.Errorf("Task blockers length = %d, want 1", len(task.Blockers))
				}
			},
		},
		{
			name: "add new task with done status",
			tasks: []todo.Task{
				{ID: "T001", Title: "Existing task", Priority: 1, Status: todo.StatusDone},
			},
			summary: &agents.Summary{
				TaskID:  "T002",
				Status:  "done",
				Summary: "New task from review",
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T002")
				if task == nil {
					t.Fatal("Task T002 not found")
				}
				if task.Status != todo.StatusDone {
					t.Errorf("Task status = %q, want %q", task.Status, todo.StatusDone)
				}
				if task.Title != "New task from review" {
					t.Errorf("Task title = %q, want %q", task.Title, "New task from review")
				}
				if task.Priority != 2 {
					t.Errorf("Task priority = %d, want 2", task.Priority)
				}
			},
		},
		{
			name: "skipped summary does not reset existing task",
			tasks: []todo.Task{
				{
					ID:       "T001",
					Title:    "Test task",
					Priority: 1,
					Status:   todo.StatusDone,
					Details:  "Already done",
					Files:    []string{"file.go"},
					Blockers: []string{"waiting on T002"},
				},
			},
			summary: &agents.Summary{
				TaskID:   "T001",
				Status:   "skipped",
				Summary:  "Ignored summary",
				Files:    []string{"new.go"},
				Blockers: []string{"new blocker"},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task == nil {
					t.Fatal("Task T001 not found")
				}
				if task.Status != todo.StatusDone {
					t.Errorf("Task status = %q, want %q", task.Status, todo.StatusDone)
				}
				if task.Details != "Already done" {
					t.Errorf("Task details = %q, want %q", task.Details, "Already done")
				}
				if len(task.Files) != 1 || task.Files[0] != "file.go" {
					t.Errorf("Task files = %v, want [file.go]", task.Files)
				}
				if len(task.Blockers) != 1 || task.Blockers[0] != "waiting on T002" {
					t.Errorf("Task blockers = %v, want [waiting on T002]", task.Blockers)
				}
			},
		},
		{
			name: "empty task_id is no-op",
			tasks: []todo.Task{
				{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
			},
			summary: &agents.Summary{
				TaskID:  "",
				Status:  "done",
				Summary: "No task",
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task.Status != todo.StatusTodo {
					t.Errorf("Task status should remain todo, got %q", task.Status)
				}
			},
		},
		{
			name: "merge files with existing files",
			tasks: []todo.Task{
				{
					ID:       "T001",
					Title:    "Test task",
					Priority: 1,
					Status:   todo.StatusTodo,
					Files:    []string{"file1.go", "file2.go"},
				},
			},
			summary: &agents.Summary{
				TaskID:  "T001",
				Status:  "done",
				Summary: "Completed",
				Files:   []string{"file2.go", "file3.go"},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task == nil {
					t.Fatal("Task T001 not found")
				}
				wantFiles := []string{"file1.go", "file2.go", "file3.go"}
				if len(task.Files) != len(wantFiles) {
					t.Errorf("Task files length = %d, want %d", len(task.Files), len(wantFiles))
				}
				for i, want := range wantFiles {
					if i >= len(task.Files) || task.Files[i] != want {
						t.Errorf("Task files[%d] = %q, want %q", i, task.Files[i], want)
					}
				}
			},
		},
		{
			name: "merge blockers with existing blockers",
			tasks: []todo.Task{
				{
					ID:       "T001",
					Title:    "Test task",
					Priority: 1,
					Status:   todo.StatusTodo,
					Blockers: []string{"waiting for T002"},
				},
			},
			summary: &agents.Summary{
				TaskID:   "T001",
				Status:   "blocked",
				Summary:  "More blockers",
				Blockers: []string{"waiting for T002", "API change needed"},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task == nil {
					t.Fatal("Task T001 not found")
				}
				wantBlockers := []string{"waiting for T002", "API change needed"}
				if len(task.Blockers) != len(wantBlockers) {
					t.Errorf("Task blockers length = %d, want %d", len(task.Blockers), len(wantBlockers))
				}
				for i, want := range wantBlockers {
					if i >= len(task.Blockers) || task.Blockers[i] != want {
						t.Errorf("Task blockers[%d] = %q, want %q", i, task.Blockers[i], want)
					}
				}
			},
		},
		{
			name: "merge both files and blockers",
			tasks: []todo.Task{
				{
					ID:       "T001",
					Title:    "Test task",
					Priority: 1,
					Status:   todo.StatusTodo,
					Files:    []string{"old.go"},
					Blockers: []string{"old blocker"},
				},
			},
			summary: &agents.Summary{
				TaskID:   "T001",
				Status:   "done",
				Summary:  "Completed",
				Files:    []string{"new.go"},
				Blockers: []string{"new blocker"},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T001")
				if task == nil {
					t.Fatal("Task T001 not found")
				}
				wantFiles := []string{"old.go", "new.go"}
				if len(task.Files) != len(wantFiles) {
					t.Errorf("Task files length = %d, want %d", len(task.Files), len(wantFiles))
				}
				wantBlockers := []string{"old blocker", "new blocker"}
				if len(task.Blockers) != len(wantBlockers) {
					t.Errorf("Task blockers length = %d, want %d", len(task.Blockers), len(wantBlockers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			todoPath := filepath.Join(tmpDir, "todo.json")

			// Create todo file
			todoFile := &todo.File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         tt.tasks,
			}
			if err := todoFile.Save(todoPath); err != nil {
				t.Fatalf("Failed to save todo file: %v", err)
			}

			// Create config
			cfg := &config.Config{
				TodoFile:      "todo.json",
				SchemaFile:    "to-do.schema.json",
				PromptDir:     promptsDir,
				ApplySummary:  true,
				MaxIterations: 10,
				LogDir:        filepath.Join(tmpDir, "logs"),
			}

			// Create loop
			loop, err := New(cfg, tmpDir)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			// Apply summary
			err = loop.applySummary(tt.summary)
			if (err != nil) != tt.wantErr {
				t.Errorf("applySummary() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Reload and verify
			updated, err := todo.Load(todoPath)
			if err != nil {
				t.Fatalf("Failed to reload todo file: %v", err)
			}

			if tt.verifyFn != nil {
				tt.verifyFn(t, updated)
			}
		})
	}
}

// TestApplySummaryStatusMapping tests status string to Status mapping.
func TestApplySummaryStatusMapping(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	statusTests := []struct {
		summaryStatus string
		wantStatus    todo.Status
	}{
		{"done", todo.StatusDone},
		{"blocked", todo.StatusBlocked},
	}

	for _, st := range statusTests {
		t.Run(st.summaryStatus, func(t *testing.T) {
			todoPath := filepath.Join(tmpDir, "todo.json")

			todoFile := &todo.File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks: []todo.Task{
					{ID: "T001", Title: "Test", Priority: 1, Status: todo.StatusTodo},
				},
			}
			if err := todoFile.Save(todoPath); err != nil {
				t.Fatalf("Failed to save todo file: %v", err)
			}

			cfg := &config.Config{
				TodoFile:      "todo.json",
				SchemaFile:    "to-do.schema.json",
				PromptDir:     promptsDir,
				ApplySummary:  true,
				MaxIterations: 10,
				LogDir:        filepath.Join(tmpDir, "logs"),
			}

			loop, err := New(cfg, tmpDir)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			summary := &agents.Summary{
				TaskID:  "T001",
				Status:  st.summaryStatus,
				Summary: "Test",
			}

			if err := loop.applySummary(summary); err != nil {
				t.Fatalf("applySummary() error = %v", err)
			}

			updated, err := todo.Load(todoPath)
			if err != nil {
				t.Fatalf("Failed to reload todo file: %v", err)
			}

			task := updated.GetTask("T001")
			if task.Status != st.wantStatus {
				t.Errorf("Status = %q, want %q", task.Status, st.wantStatus)
			}
		})
	}
}

// TestLoopNew tests creating a new loop instance.
func TestLoopNew(t *testing.T) {
	tmpDir := t.TempDir()

	// Create prompts directory
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Create a valid todo file
	todoPath := filepath.Join(tmpDir, "todo.json")
	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  true,
		MaxIterations: 10,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if loop == nil {
		t.Fatal("New() returned nil")
	}
	if loop.todoFile == nil {
		t.Error("New() todoFile is nil")
	}
	if loop.promptStore == nil {
		t.Error("New() promptStore is nil")
	}
	if loop.renderer == nil {
		t.Error("New() renderer is nil")
	}
}

// TestEnsureSchemaExists tests the schema file creation logic.
func TestEnsureSchemaExists(t *testing.T) {
	t.Run("creates new schema file", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaPath := filepath.Join(tmpDir, "schema.json")

		if err := ensureSchemaExists(schemaPath); err != nil {
			t.Fatalf("ensureSchemaExists() error = %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(schemaPath); err != nil {
			t.Errorf("Schema file was not created: %v", err)
		}
	})

	t.Run("keeps existing schema file", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaPath := filepath.Join(tmpDir, "schema.json")

		// Create an existing schema
		existingContent := `{"existing": "content"}`
		if err := os.WriteFile(schemaPath, []byte(existingContent), 0644); err != nil {
			t.Fatalf("Failed to create existing schema: %v", err)
		}

		if err := ensureSchemaExists(schemaPath); err != nil {
			t.Fatalf("ensureSchemaExists() error = %v", err)
		}

		// Verify content is preserved (will be updated with source_files if missing)
		data, err := os.ReadFile(schemaPath)
		if err != nil {
			t.Fatalf("Failed to read schema: %v", err)
		}
		content := string(data)
		if !contains(content, "existing") {
			t.Error("Existing schema content was not preserved")
		}
	})

	t.Run("returns error for directory path", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaPath := tmpDir // directory, not file

		if err := ensureSchemaExists(schemaPath); err == nil {
			t.Error("ensureSchemaExists() expected error for directory path, got nil")
		}
	})
}

// TestHasProjectDoneMarker tests the project-done marker detection.
func TestHasProjectDoneMarker(t *testing.T) {
	tests := []struct {
		name    string
		tasks   []todo.Task
		wantHas bool
	}{
		{
			name:    "no marker",
			tasks:   []todo.Task{{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo}},
			wantHas: false,
		},
		{
			name: "marker by ID",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo},
				{ID: "PROJECT-DONE", Title: "Done", Priority: 5, Status: todo.StatusDone},
			},
			wantHas: true,
		},
		{
			name: "marker by lowercase ID",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo},
				{ID: "project-done", Title: "Done", Priority: 5, Status: todo.StatusDone},
			},
			wantHas: true,
		},
		{
			name: "marker by tag",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo, Tags: []string{"project-done"}},
			},
			wantHas: true,
		},
		{
			name: "marker by lowercase tag",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo, Tags: []string{"Project-Done"}},
			},
			wantHas: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop := &Loop{
				todoFile: &todo.File{Tasks: tt.tasks},
			}
			if got := loop.hasProjectDoneMarker(); got != tt.wantHas {
				t.Errorf("hasProjectDoneMarker() = %v, want %v", got, tt.wantHas)
			}
		})
	}
}

// TestAddProjectDoneMarker tests adding the project-done marker.
func TestAddProjectDoneMarker(t *testing.T) {
	tmpDir := t.TempDir()
	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusDone},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	loop := &Loop{
		todoFile: todoFile,
		todoPath: todoPath,
	}

	loop.addProjectDoneMarker()

	// Reload and check
	updated, err := todo.Load(todoPath)
	if err != nil {
		t.Fatalf("Failed to reload todo file: %v", err)
	}

	found := false
	for _, task := range updated.Tasks {
		if stringsEqualFold(task.ID, "PROJECT-DONE") {
			found = true
			if task.Title != "Project done: no new tasks" {
				t.Errorf("Marker title = %q, want %q", task.Title, "Project done: no new tasks")
			}
			if task.Priority != 5 {
				t.Errorf("Marker priority = %d, want 5", task.Priority)
			}
			break
		}
	}
	if !found {
		t.Error("Project-done marker was not added")
	}

	// Add again - should not duplicate
	prevCount := len(updated.Tasks)
	loop.addProjectDoneMarker()
	reloaded, _ := todo.Load(todoPath)
	if len(reloaded.Tasks) != prevCount {
		t.Errorf("Marker was duplicated, got %d tasks, want %d", len(reloaded.Tasks), prevCount)
	}
}

// TestLabelFunctions tests the label generation functions.
func TestLabelFunctions(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(int) string
		input    int
		expected string
	}{
		{"iterationLabel", iterationLabel, 1, "iter-1"},
		{"iterationLabel", iterationLabel, 42, "iter-42"},
		{"reviewLabel", reviewLabel, 1, "review-1"},
		{"reviewLabel", reviewLabel, 5, "review-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.expected, func(t *testing.T) {
			result := tt.fn(tt.input)
			if result != tt.expected {
				t.Errorf("%s(%d) = %q, want %q", tt.name, tt.input, result, tt.expected)
			}
		})
	}
}

// TestSummaryApplyWithValidation tests that summary validation is performed.
func TestSummaryApplyWithValidation(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema with strict validation
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" },
			"files": { "type": "array", "items": { "type": "string" } },
			"blockers": { "type": "array", "items": { "type": "string" } }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Test", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  true,
		MaxIterations: 10,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test invalid status (should fail validation)
	invalidSummary := &agents.Summary{
		TaskID:  "T001",
		Status:  "invalid_status",
		Summary: "Test",
	}

	err = loop.applySummary(invalidSummary)
	if err == nil {
		t.Error("applySummary() with invalid status expected error, got nil")
	}

	// Test missing required fields
	invalidSummary2 := &agents.Summary{
		TaskID: "T001",
	}

	err = loop.applySummary(invalidSummary2)
	if err == nil {
		t.Error("applySummary() without status expected error, got nil")
	}
}

// stubAgent is a test double for the Agent interface.
type stubAgent struct {
	summary *agents.Summary
	err     error
}

func (s *stubAgent) Run(ctx context.Context, prompt string, logWriter agents.LogWriter) (*agents.Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.summary, nil
}

// TestRunIteration tests a single iteration with a stub agent.
func TestRunIteration(t *testing.T) {
	t.Setenv("LOOPER_QUIET", "1")
	tmpDir := t.TempDir()

	// Create prompts directory
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Write iteration prompt
	iterationPromptPath := filepath.Join(promptsDir, prompts.IterationPrompt)
	iterationPromptContent := `Task: {{.SelectedTask.ID}}
WorkDir: {{.WorkDir}}
TodoPath: {{.TodoPath}}
SchemaPath: {{.SchemaPath}}
Iteration: {{.Iteration}}
Schedule: {{.Schedule}}
Now: {{.Now}}
`
	if err := os.WriteFile(iterationPromptPath, []byte(iterationPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write iteration prompt: %v", err)
	}

	stubPath := filepath.Join(tmpDir, "stub-test.sh")
	stubScript := `#!/bin/sh
# Read all input to get the prompt
prompt=$(cat)
# Check for any T001 reference in the prompt
if echo "$prompt" | grep -q "T001"; then
  printf '{"task_id":"T001","status":"done","summary":"completed"}\n'
else
  printf '{"task_id":null,"status":"skipped","summary":"no task"}\n'
fi
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("Failed to write stub agent: %v", err)
	}

	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  true,
		MaxIterations: 10,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}
	(&cfg.Roles).SetAgent("iter", "test-stub")
	(&cfg.Roles).SetAgent("review", "test-stub")
	cfg.Agents.SetAgent("test-stub", config.Agent{Binary: stubPath})

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	task := loop.todoFile.SelectTask()
	if task == nil {
		t.Fatal("No task selected")
	}

	if err := loop.runIteration(context.Background(), 1, task); err != nil {
		t.Fatalf("runIteration() error = %v", err)
	}

	updated, err := todo.Load(todoPath)
	if err != nil {
		t.Fatalf("Failed to reload todo file: %v", err)
	}

	updatedTask := updated.GetTask("T001")
	if updatedTask == nil {
		t.Fatal("Task T001 not found after iteration")
	}
	if updatedTask.Status != todo.StatusDone {
		t.Errorf("Task status = %q, want %q", updatedTask.Status, todo.StatusDone)
	}
	if updatedTask.Details != "completed" {
		t.Errorf("Task details = %q, want %q", updatedTask.Details, "completed")
	}
}

// TestStubAgentIntegration tests that the stub agent correctly implements the Agent interface.
func TestStubAgentIntegration(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		agent    *stubAgent
		prompt   string
		wantErr  bool
		verifyFn func(*testing.T, *agents.Summary, error)
	}{
		{
			name: "stub agent returns valid summary",
			agent: &stubAgent{
				summary: &agents.Summary{
					TaskID:  "T001",
					Status:  "done",
					Summary: "Task completed successfully",
					Files:   []string{"file1.go", "file2.go"},
				},
			},
			prompt:  "Complete task T001",
			wantErr: false,
			verifyFn: func(t *testing.T, summary *agents.Summary, err error) {
				if err != nil {
					t.Errorf("Run() unexpected error = %v", err)
				}
				if summary == nil {
					t.Fatal("Run() returned nil summary")
				}
				if summary.TaskID != "T001" {
					t.Errorf("TaskID = %q, want T001", summary.TaskID)
				}
				if summary.Status != "done" {
					t.Errorf("Status = %q, want done", summary.Status)
				}
				if summary.Summary != "Task completed successfully" {
					t.Errorf("Summary = %q, want 'Task completed successfully'", summary.Summary)
				}
				if len(summary.Files) != 2 {
					t.Errorf("Files length = %d, want 2", len(summary.Files))
				}
			},
		},
		{
			name: "stub agent returns error",
			agent: &stubAgent{
				err: errors.New("agent error"),
			},
			prompt:  "Complete task T001",
			wantErr: true,
			verifyFn: func(t *testing.T, summary *agents.Summary, err error) {
				if err == nil {
					t.Error("Run() expected error, got nil")
				}
				if err.Error() != "agent error" {
					t.Errorf("Error = %q, want 'agent error'", err.Error())
				}
				if summary != nil {
					t.Errorf("Run() should return nil summary on error, got %+v", summary)
				}
			},
		},
		{
			name: "stub agent with nil summary",
			agent: &stubAgent{
				summary: nil,
			},
			prompt:  "Complete task T001",
			wantErr: false,
			verifyFn: func(t *testing.T, summary *agents.Summary, err error) {
				if err != nil {
					t.Errorf("Run() unexpected error = %v", err)
				}
				if summary != nil {
					t.Errorf("Run() should return nil summary when configured, got %+v", summary)
				}
			},
		},
		{
			name: "stub agent with blocked status and blockers",
			agent: &stubAgent{
				summary: &agents.Summary{
					TaskID:   "T001",
					Status:   "blocked",
					Summary:  "Waiting for dependency",
					Blockers: []string{"waiting for T002", "API change needed"},
				},
			},
			prompt:  "Complete task T001",
			wantErr: false,
			verifyFn: func(t *testing.T, summary *agents.Summary, err error) {
				if err != nil {
					t.Errorf("Run() unexpected error = %v", err)
				}
				if summary.Status != "blocked" {
					t.Errorf("Status = %q, want blocked", summary.Status)
				}
				if len(summary.Blockers) != 2 {
					t.Errorf("Blockers length = %d, want 2", len(summary.Blockers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a log writer that captures events
			var capturedEvents []agents.LogEvent
			logWriter := &captureLogWriter{
				onWrite: func(event agents.LogEvent) {
					capturedEvents = append(capturedEvents, event)
				},
			}

			summary, err := tt.agent.Run(ctx, tt.prompt, logWriter)

			if tt.verifyFn != nil {
				tt.verifyFn(t, summary, err)
			} else if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// captureLogWriter is a test helper that captures log events.
type captureLogWriter struct {
	onWrite func(agents.LogEvent)
}

func (w *captureLogWriter) Write(event agents.LogEvent) error {
	if w.onWrite != nil {
		w.onWrite(event)
	}
	return nil
}

func (w *captureLogWriter) Close() error {
	return nil
}

func (w *captureLogWriter) SetIndent(string) {}

// TestStubAgentLogWriter tests that the stub agent correctly handles log writers.
func TestStubAgentLogWriter(t *testing.T) {
	ctx := context.Background()
	agent := &stubAgent{
		summary: &agents.Summary{
			TaskID:  "T001",
			Status:  "done",
			Summary: "Complete",
		},
	}

	var logged bool
	logWriter := &captureLogWriter{
		onWrite: func(event agents.LogEvent) {
			logged = true
			if event.Timestamp.IsZero() {
				t.Error("LogEvent timestamp should not be zero")
			}
		},
	}

	_, err := agent.Run(ctx, "test prompt", logWriter)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// The stub agent doesn't write logs, so logged should be false
	// This test verifies the log writer interface is correctly passed
	t.Logf("Log writer called: %v", logged)
}

// TestMergeStrings tests the mergeStrings helper function.
func TestMergeStrings(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		added    []string
		want     []string
	}{
		{
			name:     "merge with empty existing",
			existing: []string{},
			added:    []string{"a", "b"},
			want:     []string{"a", "b"},
		},
		{
			name:     "merge with empty added",
			existing: []string{"a", "b"},
			added:    []string{},
			want:     []string{"a", "b"},
		},
		{
			name:     "merge with no duplicates",
			existing: []string{"a", "b"},
			added:    []string{"c", "d"},
			want:     []string{"a", "b", "c", "d"},
		},
		{
			name:     "merge with duplicates",
			existing: []string{"a", "b"},
			added:    []string{"b", "c"},
			want:     []string{"a", "b", "c"},
		},
		{
			name:     "merge with all duplicates",
			existing: []string{"a", "b"},
			added:    []string{"a", "b"},
			want:     []string{"a", "b"},
		},
		{
			name:     "merge preserves order",
			existing: []string{"b", "a"},
			added:    []string{"c", "a", "d"},
			want:     []string{"b", "a", "c", "d"},
		},
		{
			name:     "merge with duplicate in added",
			existing: []string{"a"},
			added:    []string{"b", "b", "c"},
			want:     []string{"a", "b", "c"},
		},
		{
			name:     "merge with duplicate in existing",
			existing: []string{"a", "a", "b"},
			added:    []string{"c"},
			want:     []string{"a", "b", "c"},
		},
		{
			name:     "merge with both nil",
			existing: nil,
			added:    nil,
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeStrings(tt.existing, tt.added)
			if len(got) != len(tt.want) {
				t.Errorf("mergeStrings() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i, want := range tt.want {
				if i >= len(got) || got[i] != want {
					t.Errorf("mergeStrings()[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func stringsEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca = ca + 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb = cb + 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// TestEnsureGitRepo tests git repository initialization.
func TestEnsureGitRepo(t *testing.T) {
	t.Run("initializes git repo when none exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Verify no .git directory
		gitDir := filepath.Join(tmpDir, ".git")
		if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
			t.Skip("Git repo already exists in temp dir")
		}

		err := ensureGitRepo(tmpDir)
		if err != nil {
			// Git may not be available
			t.Skipf("Git not available: %v", err)
		}

		// Verify .git directory was created
		if info, err := os.Stat(gitDir); err != nil {
			t.Errorf("Git directory not created: %v", err)
		} else if !info.IsDir() {
			t.Error(".git exists but is not a directory")
		}
	})

	t.Run("does nothing when git repo already exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize git repo
		err := ensureGitRepo(tmpDir)
		if err != nil {
			t.Skipf("Git not available: %v", err)
		}

		// Call again - should not error
		err = ensureGitRepo(tmpDir)
		if err != nil {
			t.Errorf("ensureGitRepo() on existing repo error = %v", err)
		}
	})

	t.Run("returns error when .git exists but is not a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitFile := filepath.Join(tmpDir, ".git")
		if err := os.WriteFile(gitFile, []byte("not a directory"), 0644); err != nil {
			t.Fatalf("Failed to create .git file: %v", err)
		}

		err := ensureGitRepo(tmpDir)
		if err == nil {
			t.Error("ensureGitRepo() expected error when .git is a file, got nil")
		}
	})
}

// TestVerifyTodoFileCreated tests the todo file verification.
func TestVerifyTodoFileCreated(t *testing.T) {
	t.Run("returns nil for existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		todoPath := filepath.Join(tmpDir, "todo.json")
		if err := os.WriteFile(todoPath, []byte("{}"), 0644); err != nil {
			t.Fatalf("Failed to create todo file: %v", err)
		}

		if err := verifyTodoFileCreated(todoPath); err != nil {
			t.Errorf("verifyTodoFileCreated() error = %v", err)
		}
	})

	t.Run("returns error when file does not exist", func(t *testing.T) {
		todoPath := "/tmp/nonexistent_todo_file_12345.json"
		err := verifyTodoFileCreated(todoPath)
		if err == nil {
			t.Error("verifyTodoFileCreated() expected error for missing file, got nil")
		}
	})

	t.Run("returns error when path is a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := verifyTodoFileCreated(tmpDir)
		if err == nil {
			t.Error("verifyTodoFileCreated() expected error for directory, got nil")
		}
	})
}

// TestValidationErrorsMessage tests error message formatting.
func TestValidationErrorsMessage(t *testing.T) {
	tests := []struct {
		name   string
		errors []error
		want   string
	}{
		{
			name:   "empty errors returns empty string",
			errors: []error{},
			want:   "",
		},
		{
			name:   "single error",
			errors: []error{errors.New("error 1")},
			want:   "error 1",
		},
		{
			name:   "multiple errors joined by newline",
			errors: []error{errors.New("error 1"), errors.New("error 2")},
			want:   "error 1\nerror 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validationErrorsMessage(tt.errors)
			if got != tt.want {
				t.Errorf("validationErrorsMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMarkTaskBlocked tests marking a task as blocked.
func TestMarkTaskBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	loop := &Loop{
		todoFile: todoFile,
		todoPath: todoPath,
	}

	if err := loop.markTaskBlocked("T001"); err != nil {
		t.Fatalf("markTaskBlocked() error = %v", err)
	}

	// Reload and verify
	updated, err := todo.Load(todoPath)
	if err != nil {
		t.Fatalf("Failed to reload todo file: %v", err)
	}

	task := updated.GetTask("T001")
	if task.Status != todo.StatusBlocked {
		t.Errorf("Task status = %q, want %q", task.Status, todo.StatusBlocked)
	}
}

// TestRenderReviewPrompt tests review prompt rendering.
func TestRenderReviewPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	loop := &Loop{
		renderer: prompts.NewRenderer(prompts.NewStore(tmpDir, "")),
		todoPath: filepath.Join(tmpDir, "todo.json"),
		schemaPath: filepath.Join(tmpDir, "schema.json"),
		workDir: tmpDir,
	}

	prompt := loop.renderReviewPrompt(5)

	// Verify the prompt contains iteration info
	if !contains(prompt, "5") {
		t.Error("Review prompt should contain iteration number")
	}
}

// TestDelayBetweenIterations tests the delay function.
func TestDelayBetweenIterations(t *testing.T) {
	tests := []struct {
		name        string
		delaySec    int
		expectDelay bool
	}{
		{"no delay when zero", 0, false},
		{"no delay when negative", -1, false},
		{"delay when positive", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop := &Loop{cfg: &config.Config{LoopDelaySeconds: tt.delaySec}}

			start := time.Now()
			err := loop.delayBetweenIterations(context.Background())
			elapsed := time.Since(start)

			if err != nil {
				t.Errorf("delayBetweenIterations() error = %v", err)
			}

			// Check if delay occurred (with tolerance)
			if tt.expectDelay && elapsed < 500*time.Millisecond {
				t.Errorf("Expected delay > 500ms, got %v", elapsed)
			}
			if !tt.expectDelay && elapsed > 100*time.Millisecond {
				t.Errorf("Expected no delay, got %v", elapsed)
			}
		})
	}

	t.Run("respects context cancellation", func(t *testing.T) {
		loop := &Loop{cfg: &config.Config{LoopDelaySeconds: 10}}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := loop.delayBetweenIterations(ctx)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})
}

// TestRunReviewAndMaybeFinish tests the review flow.
func TestRunReviewAndMaybeFinish(t *testing.T) {
	t.Setenv("LOOPER_QUIET", "1")
	tmpDir := t.TempDir()

	// Create prompts directory
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Write review prompt
	reviewPromptPath := filepath.Join(promptsDir, prompts.ReviewPrompt)
	reviewPromptContent := `TodoPath: {{.TodoPath}}
SchemaPath: {{.SchemaPath}}
WorkDir: {{.WorkDir}}
Iteration: {{.Iteration}}
`
	if err := os.WriteFile(reviewPromptPath, []byte(reviewPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write review prompt: %v", err)
	}

	stubPath := filepath.Join(tmpDir, "stub-review.sh")
	stubScript := `#!/bin/sh
# Read all input
cat > /dev/null
printf '{"task_id":null,"status":"skipped","summary":"no new tasks"}\n'
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("Failed to write stub agent: %v", err)
	}

	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusDone},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  false, // Don't apply review summary
		MaxIterations: 10,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}
	(&cfg.Roles).SetAgent("iter", "test-stub")
	(&cfg.Roles).SetAgent("review", "test-stub")
	cfg.Agents.SetAgent("test-stub", config.Agent{Binary: stubPath})

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Run review - should add project-done marker since no new tasks
	done, err := loop.runReviewAndMaybeFinish(context.Background(), 1)
	if err != nil {
		t.Fatalf("runReviewAndMaybeFinish() error = %v", err)
	}

	if !done {
		t.Error("Expected done=true when no tasks remain")
	}

	// Verify project-done marker was added
	updated, err := todo.Load(todoPath)
	if err != nil {
		t.Fatalf("Failed to reload todo file: %v", err)
	}

	found := false
	for _, task := range updated.Tasks {
		if stringsEqualFold(task.ID, "PROJECT-DONE") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Project-done marker was not added")
	}
}

// TestRunMainLoop tests the main Run loop.
func TestRunMainLoop(t *testing.T) {
	t.Setenv("LOOPER_QUIET", "1")
	tmpDir := t.TempDir()

	// Create prompts directory
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Write iteration prompt
	iterationPromptPath := filepath.Join(promptsDir, prompts.IterationPrompt)
	iterationPromptContent := `Task: {{.SelectedTask.ID}}
WorkDir: {{.WorkDir}}
`
	if err := os.WriteFile(iterationPromptPath, []byte(iterationPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write iteration prompt: %v", err)
	}

	// Write review prompt
	reviewPromptPath := filepath.Join(promptsDir, prompts.ReviewPrompt)
	reviewPromptContent := `Review pass
`
	if err := os.WriteFile(reviewPromptPath, []byte(reviewPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write review prompt: %v", err)
	}

	stubPath := filepath.Join(tmpDir, "stub-test.sh")
	stubScript := `#!/bin/sh
# Read all input to get the prompt
prompt=$(cat)
status="done"
summary="completed"
case "$prompt" in
  *"Task: T001"*)
    status="done"
    summary="completed"
    ;;
  *"Review pass"*)
    status="skipped"
    summary="no new tasks"
    ;;
esac
printf '{"task_id":"T001","status":"%s","summary":"%s"}\n' "$status" "$summary"
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("Failed to write stub agent: %v", err)
	}

	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  true,
		MaxIterations: 10,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}
	(&cfg.Roles).SetAgent("iter", "test-stub")
	(&cfg.Roles).SetAgent("review", "test-stub")
	cfg.Agents.SetAgent("test-stub", config.Agent{Binary: stubPath})

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Run the loop
	err = loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify task was completed
	updated, err := todo.Load(todoPath)
	if err != nil {
		t.Fatalf("Failed to reload todo file: %v", err)
	}

	task := updated.GetTask("T001")
	if task.Status != todo.StatusDone {
		t.Errorf("Task status = %q, want %q", task.Status, todo.StatusDone)
	}

	// Verify project-done marker was added
	found := false
	for _, task := range updated.Tasks {
		if stringsEqualFold(task.ID, "PROJECT-DONE") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Project-done marker was not added after review")
	}
}

// TestRunWithContextCancellation tests context cancellation.
func TestRunWithContextCancellation(t *testing.T) {
	t.Setenv("LOOPER_QUIET", "1")
	tmpDir := t.TempDir()

	// Create prompts directory
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Write iteration prompt
	iterationPromptPath := filepath.Join(promptsDir, prompts.IterationPrompt)
	iterationPromptContent := `Task: {{.SelectedTask.ID}}
`
	if err := os.WriteFile(iterationPromptPath, []byte(iterationPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write iteration prompt: %v", err)
	}

	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Task", Priority: 1, Status: todo.StatusTodo},
			{ID: "T002", Title: "Task 2", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  false,
		MaxIterations: 100,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Cancel context after first iteration
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = loop.Run(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// TestRunWithStatus tests the RunWithStatus method.
func TestRunWithStatus(t *testing.T) {
	t.Setenv("LOOPER_QUIET", "1")
	tmpDir := t.TempDir()

	// Create prompts directory
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Write summary schema
	schemaPath := filepath.Join(promptsDir, prompts.SummarySchema)
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Write iteration prompt
	iterationPromptPath := filepath.Join(promptsDir, prompts.IterationPrompt)
	iterationPromptContent := `Task: {{.SelectedTask.ID}}
`
	if err := os.WriteFile(iterationPromptPath, []byte(iterationPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write iteration prompt: %v", err)
	}

	// Write review prompt
	reviewPromptPath := filepath.Join(promptsDir, prompts.ReviewPrompt)
	reviewPromptContent := `Review pass
`
	if err := os.WriteFile(reviewPromptPath, []byte(reviewPromptContent), 0644); err != nil {
		t.Fatalf("Failed to write review prompt: %v", err)
	}

	stubPath := filepath.Join(tmpDir, "stub-claude.sh")
	stubScript := `#!/bin/sh
prompt=$(cat)
status="done"
summary="done"
case "$prompt" in
  *"Task: T001"*)
    status="done"
    summary="done"
    ;;
  *"Review pass"*)
    status="skipped"
    summary="no new tasks"
    ;;
esac
printf '{"task_id":"T001","status":"%s","summary":"%s"}\n' "$status" "$summary"
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("Failed to write stub agent: %v", err)
	}

	todoPath := filepath.Join(tmpDir, "todo.json")

	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Test task", Priority: 1, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		t.Fatalf("Failed to save todo file: %v", err)
	}

	cfg := &config.Config{
		TodoFile:      "todo.json",
		SchemaFile:    "to-do.schema.json",
		PromptDir:     promptsDir,
		ApplySummary:  true,
		MaxIterations: 10,
		LogDir:        filepath.Join(tmpDir, "logs"),
	}
	(&cfg.Roles).SetAgent("iter", "test-stub")
	(&cfg.Roles).SetAgent("review", "test-stub")
	cfg.Agents.SetAgent("test-stub", config.Agent{Binary: stubPath})

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	statusCh := make(chan Status, 10)
	doneCh := make(chan error, 1)

	go func() {
		doneCh <- loop.RunWithStatus(context.Background(), statusCh)
	}()

	// Collect status updates
	var statuses []Status
	for status := range statusCh {
		statuses = append(statuses, status)
		if status.Status == "done" || status.Error != nil {
			break
		}
	}

	err = <-doneCh
	if err != nil {
		t.Fatalf("RunWithStatus() error = %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("No status updates received")
	}

	// Verify we got at least one status update
	found := false
	for _, s := range statuses {
		if s.TaskID == "T001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected status update for T001")
	}
}
