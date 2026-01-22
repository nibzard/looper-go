package loop

import (
	"os"
	"testing"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
)

// BenchmarkMergeStrings benchmarks string slice merging.
func BenchmarkMergeStrings(b *testing.B) {
	existing := []string{"file1.go", "file2.go", "file3.go"}
	added := []string{"file4.go", "file5.go", "file1.go"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mergeStrings(existing, added)
	}
}

// BenchmarkMergeStringsLarge benchmarks merging larger slices.
func BenchmarkMergeStringsLarge(b *testing.B) {
	existing := make([]string, 100)
	for i := 0; i < 100; i++ {
		existing[i] = "existing_file.go"
	}
	added := make([]string, 50)
	for i := 0; i < 50; i++ {
		added[i] = "new_file.go"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mergeStrings(existing, added)
	}
}

// BenchmarkIterationLabel benchmarks iteration label generation.
func BenchmarkIterationLabel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = iterationLabel(i + 1)
	}
}

// BenchmarkReviewLabel benchmarks review label generation.
func BenchmarkReviewLabel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = reviewLabel(i + 1)
	}
}

// BenchmarkApplySummary benchmarks summary application.
func BenchmarkApplySummary(b *testing.B) {
	tmpDir := b.TempDir()
	workDir := tmpDir
	todoPath := tmpDir + "/to-do.json"
	schemaPath := tmpDir + "/to-do.schema.json"

	// Create test todo file
	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo},
			{ID: "T002", Title: "Task 2", Priority: 2, Status: todo.StatusTodo},
		},
	}
	if err := todoFile.Save(todoPath); err != nil {
		b.Fatalf("Failed to create test todo file: %v", err)
	}

	// Create schema file
	schemaContent := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Codex RALF Todo",
  "type": "object",
  "required": ["schema_version", "source_files", "tasks"],
  "properties": {
    "schema_version": {"type": "integer", "const": 1},
    "source_files": {"type": "array", "items": {"type": "string"}},
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
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		b.Fatalf("Failed to create schema file: %v", err)
	}

	// Create summary schema
	promptStore := prompts.NewStore(workDir, "")
	summarySchemaPath := promptStore.Dir() + "/" + prompts.SummarySchema
	if err := os.MkdirAll(promptStore.Dir(), 0755); err != nil {
		b.Fatalf("Failed to create prompt dir: %v", err)
	}
	summarySchemaContent := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Iteration Summary",
  "type": "object",
  "required": ["task_id", "status"],
  "properties": {
    "task_id": {"type": ["string", "null"]},
    "status": {"type": "string", "enum": ["done", "blocked", "skipped"]},
    "summary": {"type": "string"},
    "files": {"type": "array", "items": {"type": "string"}},
    "blockers": {"type": "array", "items": {"type": "string"}}
  }
}`
	if err := os.WriteFile(summarySchemaPath, []byte(summarySchemaContent), 0644); err != nil {
		b.Fatalf("Failed to create summary schema: %v", err)
	}

	cfg := &config.Config{
		LogDir:       tmpDir + "/logs",
		ApplySummary: true,
	}

	loop := &Loop{
		cfg:               cfg,
		todoFile:          todoFile,
		todoPath:          todoPath,
		schemaPath:        schemaPath,
		summarySchemaPath: summarySchemaPath,
		workDir:           workDir,
	}

	summary := &agents.Summary{
		TaskID:   "T001",
		Status:   "done",
		Summary:  "Completed successfully",
		Files:    []string{"file1.go", "file2.go"},
		Blockers: []string{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := loop.applySummary(summary); err != nil {
			b.Fatalf("applySummary failed: %v", err)
		}
		// Reload todo file for next iteration
		todoFile, _ = todo.Load(todoPath)
		loop.todoFile = todoFile
	}
}

// BenchmarkAddProjectDoneMarker benchmarks adding project done marker.
func BenchmarkAddProjectDoneMarker(b *testing.B) {
	tmpDir := b.TempDir()
	workDir := tmpDir
	todoPath := tmpDir + "/to-do.json"

	cfg := &config.Config{
		LogDir: tmpDir + "/logs",
	}

	// Create empty todo file
	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         []todo.Task{},
	}

	loop := &Loop{
		cfg:      cfg,
		todoFile: todoFile,
		todoPath: todoPath,
		workDir:  workDir,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create new todo file for each iteration
		todoFile := &todo.File{
			SchemaVersion: 1,
			SourceFiles:   []string{"README.md"},
			Tasks:         []todo.Task{},
		}
		loop.todoFile = todoFile
		loop.addProjectDoneMarker()
	}
}

// BenchmarkHasProjectDoneMarker benchmarks checking for project done marker.
func BenchmarkHasProjectDoneMarker(b *testing.B) {
	tmpDir := b.TempDir()

	cfg := &config.Config{
		LogDir: tmpDir + "/logs",
	}

	// Create todo file without marker
	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []todo.Task{
			{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo},
		},
	}

	loop := &Loop{
		cfg:      cfg,
		todoFile: todoFile,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loop.hasProjectDoneMarker()
	}
}

// BenchmarkLoadTodoFile benchmarks todo file loading and validation.
func BenchmarkLoadTodoFile(b *testing.B) {
	tmpDir := b.TempDir()
	workDir := tmpDir
	todoPath := tmpDir + "/to-do.json"
	schemaPath := tmpDir + "/to-do.schema.json"

	// Create test todo file
	todoFile := &todo.File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createBenchmarkTasks(20),
	}
	if err := todoFile.Save(todoPath); err != nil {
		b.Fatalf("Failed to create test todo file: %v", err)
	}

	// Create schema file
	schemaContent := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Codex RALF Todo",
  "type": "object",
  "required": ["schema_version", "source_files", "tasks"],
  "properties": {
    "schema_version": {"type": "integer", "const": 1},
    "source_files": {"type": "array", "items": {"type": "string"}},
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
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		b.Fatalf("Failed to create schema file: %v", err)
	}

	cfg := &config.Config{
		LogDir: tmpDir + "/logs",
	}

	promptStore := prompts.NewStore(workDir, "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := loadAndValidateTodo(workDir, todoPath, schemaPath, promptStore, cfg)
		if err != nil {
			b.Fatalf("loadAndValidateTodo failed: %v", err)
		}
	}
}

// Helper function to create benchmark tasks
func createBenchmarkTasks(n int) []todo.Task {
	tasks := make([]todo.Task, n)
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		status := todo.StatusTodo
		if i%3 == 0 {
			status = todo.StatusDoing
		} else if i%5 == 0 {
			status = todo.StatusDone
		}
		priority := (i % 5) + 1
		tasks[i] = todo.Task{
			ID:       "T" + string(rune('0'+i/10)) + string(rune('0'+i%10)),
			Title:    "Task " + string(rune('0'+i)),
			Priority: priority,
			Status:   status,
		}
		if i%2 == 0 {
			tasks[i].CreatedAt = &now
			tasks[i].UpdatedAt = &now
		}
	}
	return tasks
}
