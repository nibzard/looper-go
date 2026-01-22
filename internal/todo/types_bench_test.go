package todo

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// BenchmarkLoad benchmarks todo file loading and parsing.
func BenchmarkLoad(b *testing.B) {
	content := `{
  "schema_version": 1,
  "source_files": ["README.md"],
  "tasks": [
    {"id": "T001", "title": "Task 1", "priority": 1, "status": "todo"},
    {"id": "T002", "title": "Task 2", "priority": 2, "status": "doing"},
    {"id": "T003", "title": "Task 3", "priority": 3, "status": "done"}
  ]
}`
	tmpDir := b.TempDir()
	todoPath := tmpDir + "/to-do.json"
	if err := os.WriteFile(todoPath, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Load(todoPath)
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
	}
}

// BenchmarkLoadLarge benchmarks todo file loading and parsing with 100 tasks.
func BenchmarkLoadLarge(b *testing.B) {
	// Create a large todo file with 100 tasks
	var tasksJSON string
	for i := 1; i <= 100; i++ {
		status := "todo"
		if i%3 == 0 {
			status = "doing"
		} else if i%5 == 0 {
			status = "done"
		}
		tasksJSON += fmt.Sprintf(`{"id": "T%03d", "title": "Task %d", "priority": %d, "status": "%s"}`,
			i, i, (i%5)+1, status)
		if i < 100 {
			tasksJSON += ","
		}
	}

	content := fmt.Sprintf(`{
  "schema_version": 1,
  "source_files": ["README.md"],
  "tasks": [%s]
}`, tasksJSON)

	tmpDir := b.TempDir()
	todoPath := tmpDir + "/to-do.json"
	if err := os.WriteFile(todoPath, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Load(todoPath)
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
	}
}

// BenchmarkSave benchmarks todo file saving with 2-space indentation.
func BenchmarkSave(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []Task{
			{ID: "T001", Title: "Task 1", Priority: 1, Status: StatusTodo},
			{ID: "T002", Title: "Task 2", Priority: 2, Status: StatusDoing},
			{ID: "T003", Title: "Task 3", Priority: 3, Status: StatusDone},
		},
	}
	tmpDir := b.TempDir()
	todoPath := tmpDir + "/to-do.json"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := f.Save(todoPath); err != nil {
			b.Fatalf("Save failed: %v", err)
		}
	}
}

// BenchmarkSelectTask benchmarks task selection algorithm.
func BenchmarkSelectTask(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(50),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := f.SelectTask()
		if task == nil {
			b.Fatal("SelectTask returned nil")
		}
	}
}

// BenchmarkSelectTaskLarge benchmarks task selection with 500 tasks.
func BenchmarkSelectTaskLarge(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(500),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := f.SelectTask()
		if task == nil {
			b.Fatal("SelectTask returned nil")
		}
	}
}

// BenchmarkValidateMinimal benchmarks minimal validation without schema.
func BenchmarkValidateMinimal(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(50),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := f.Validate(ValidationOptions{SchemaPath: ""})
		if !result.Valid {
			b.Fatal("Validation failed")
		}
	}
}

// BenchmarkGetTask benchmarks task lookup by ID.
func BenchmarkGetTask(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(100),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.GetTask("T050")
	}
}

// BenchmarkSetTaskStatus benchmarks task status update.
func BenchmarkSetTaskStatus(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(100),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskID := fmt.Sprintf("T%03d", (i%100)+1)
		_ = f.SetTaskStatus(taskID, StatusDone)
	}
}

// BenchmarkUpdateTask benchmarks task update via updater function.
func BenchmarkUpdateTask(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(100),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskID := "T050"
		_ = f.UpdateTask(taskID, func(t *Task) {
			t.Details = "Updated details"
		})
	}
}

// BenchmarkDependenciesSatisfied benchmarks dependency checking.
func BenchmarkDependenciesSatisfied(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(100),
	}
	// Add dependencies to some tasks
	for i := range f.Tasks {
		if i > 0 {
			f.Tasks[i].DependsOn = []string{f.Tasks[i-1].ID}
		}
	}

	task := f.Tasks[len(f.Tasks)-1]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.dependenciesSatisfied(&task)
	}
}

// BenchmarkValidateDependencies benchmarks full dependency validation.
func BenchmarkValidateDependencies(b *testing.B) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks:         createTestTasks(100),
	}
	// Add chain dependencies
	for i := range f.Tasks {
		if i > 0 {
			f.Tasks[i].DependsOn = []string{f.Tasks[i-1].ID}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := f.ValidateDependencies()
		if err != nil {
			b.Fatalf("ValidateDependencies failed: %v", err)
		}
	}
}

// BenchmarkIdSortKey benchmarks ID sorting key extraction.
func BenchmarkIdSortKey(b *testing.B) {
	ids := make([]string, 100)
	for i := 0; i < 100; i++ {
		ids[i] = fmt.Sprintf("T%03d", i+1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, id := range ids {
			_ = idSortKey(id)
		}
	}
}

// BenchmarkCompareIDs benchmarks numeric-aware ID comparison.
func BenchmarkCompareIDs(b *testing.B) {
	ids := make([]string, 100)
	for i := 0; i < 100; i++ {
		ids[i] = fmt.Sprintf("T%03d", i+1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 1; j < len(ids); j++ {
			_ = CompareIDs(ids[j-1], ids[j])
		}
	}
}

// Helper function to create test tasks
func createTestTasks(n int) []Task {
	tasks := make([]Task, n)
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		status := StatusTodo
		if i%3 == 0 {
			status = StatusDoing
		} else if i%5 == 0 {
			status = StatusDone
		}
		priority := (i % 5) + 1
		tasks[i] = Task{
			ID:       fmt.Sprintf("T%03d", i+1),
			Title:    fmt.Sprintf("Task %d", i+1),
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
