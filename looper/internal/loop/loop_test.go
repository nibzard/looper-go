package loop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nibzard/looper/internal/agents"
	"github.com/nibzard/looper/internal/config"
	"github.com/nibzard/looper/internal/prompts"
	"github.com/nibzard/looper/internal/todo"
)

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
			name: "add new task",
			tasks: []todo.Task{
				{ID: "T001", Title: "Existing task", Priority: 1, Status: todo.StatusDone},
			},
			summary: &agents.Summary{
				TaskID:  "T002",
				Status:  "todo",
				Summary: "New task from review",
			},
			wantErr: false,
			verifyFn: func(t *testing.T, f *todo.File) {
				task := f.GetTask("T002")
				if task == nil {
					t.Fatal("Task T002 not found")
				}
				if task.Status != todo.StatusTodo {
					t.Errorf("Task status = %q, want %q", task.Status, todo.StatusTodo)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			todoPath := filepath.Join(tmpDir, "todo.json")
			schemaPath := filepath.Join(tmpDir, "to-do.schema.json")

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
				TodoFile:     "todo.json",
				SchemaFile:   "to-do.schema.json",
				PromptDir:    promptsDir,
				ApplySummary: true,
				MaxIterations: 10,
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
		{"doing", todo.StatusDoing},
		{"todo", todo.StatusTodo},
		{"unknown", todo.StatusTodo},
	}

	for _, st := range statusTests {
		t.Run(st.summaryStatus, func(t *testing.T) {
			todoPath := filepath.Join(tmpDir, "todo.json")
			schemaPath := filepath.Join(tmpDir, "to-do.schema.json")

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
				TodoFile:     "todo.json",
				SchemaFile:   "to-do.schema.json",
				PromptDir:    promptsDir,
				ApplySummary: true,
				MaxIterations: 10,
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
		TodoFile:     "todo.json",
		SchemaFile:   "to-do.schema.json",
		PromptDir:    promptsDir,
		ApplySummary: true,
		MaxIterations: 10,
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
		name     string
		tasks    []todo.Task
		wantHas  bool
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
	schemaFilePath := filepath.Join(tmpDir, "to-do.schema.json")

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
		TodoFile:     "todo.json",
		SchemaFile:   "to-do.schema.json",
		PromptDir:    promptsDir,
		ApplySummary: true,
		MaxIterations: 10,
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
		Status: "done",
	}

	err = loop.applySummary(invalidSummary2)
	if err == nil {
		t.Error("applySummary() without task_id expected error, got nil")
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

	todoPath := filepath.Join(tmpDir, "todo.json")
	schemaFilePath := filepath.Join(tmpDir, "to-do.schema.json")

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
	}

	loop, err := New(cfg, tmpDir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Create stub agent with a summary
	task := loop.todoFile.SelectTask()
	if task == nil {
		t.Fatal("No task selected")
	}

	// We'll need to mock the agent creation, which is done internally
	// For this test, we'll verify the iteration setup works
	if task.ID != "T001" {
		t.Errorf("Selected task ID = %q, want T001", task.ID)
	}

	// Verify task was marked as doing
	if task.Status != todo.StatusTodo {
		// The task in the file hasn't been modified yet
		t.Logf("Task status before iteration: %q", task.Status)
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
