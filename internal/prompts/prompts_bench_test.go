package prompts

import (
	"os"
	"testing"
	"time"
)

// BenchmarkNewData benchmarks prompt data creation.
func BenchmarkNewData(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewData(
			"to-do.json",
			"to-do.schema.json",
			"/work/dir",
			Task{ID: "T001", Title: "Test Task", Status: "todo"},
			42,
			"codex",
			time.Now(),
		)
	}
}

// BenchmarkNewDataForBootstrap benchmarks bootstrap prompt data creation.
func BenchmarkNewDataForBootstrap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewDataForBootstrap(
			"to-do.json",
			"to-do.schema.json",
			"/work/dir",
			"build a CLI tool",
			time.Now(),
		)
	}
}

// BenchmarkNewDataForRepair benchmarks repair prompt data creation.
func BenchmarkNewDataForRepair(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewDataForRepair(
			"to-do.json",
			"to-do.schema.json",
			"/work/dir",
			"validation failed: missing task id",
			time.Now(),
		)
	}
}

// BenchmarkRenderer_Render benchmarks template rendering.
func BenchmarkRenderer_Render(b *testing.B) {
	// Create a temporary directory with a test prompt
	tmpDir := b.TempDir()
	store := &Store{dir: tmpDir}

	// Create a simple test prompt template using iteration.txt as the name
	promptContent := `You are working on: {{.SelectedTask.ID}} - {{.SelectedTask.Title}}
Status: {{.SelectedTask.Status}}
Todo file: {{.TodoPath}}
Iteration: {{.Iteration}}
Schedule: {{.Schedule}}
Time: {{.Now}}
`
	promptPath := tmpDir + "/iteration.txt"
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		b.Fatalf("Failed to create test prompt: %v", err)
	}

	renderer := NewRenderer(store)
	data := NewData(
		"to-do.json",
		"to-do.schema.json",
		"/work/dir",
		Task{ID: "T001", Title: "Test Task", Status: "todo"},
		42,
		"codex",
		time.Now(),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := renderer.Render(IterationPrompt, data)
		if err != nil {
			b.Fatalf("Render failed: %v", err)
		}
	}
}

// BenchmarkRenderer_RenderLarge benchmarks rendering with larger templates.
func BenchmarkRenderer_RenderLarge(b *testing.B) {
	tmpDir := b.TempDir()
	store := &Store{dir: tmpDir}

	// Create a larger prompt template (simulating real prompts)
	largePrompt := `You are in a deterministic RALF loop with fresh context each run.
Selected task for this iteration:
- id: {{.SelectedTask.ID}}
- title: {{.SelectedTask.Title}}
- status: {{.SelectedTask.Status}}

You must work on this exact task id. Do not switch tasks.
Todo file path: {{.TodoPath}}
Schema file path: {{.SchemaPath}}
Work directory: {{.WorkDir}}
Iteration number: {{.Iteration}}
Schedule: {{.Schedule}}
Current time: {{.Now}}

Return only a JSON object with the task result.
`

	promptPath := tmpDir + "/review.txt"
	if err := os.WriteFile(promptPath, []byte(largePrompt), 0644); err != nil {
		b.Fatalf("Failed to create test prompt: %v", err)
	}

	renderer := NewRenderer(store)
	data := NewData(
		"to-do.json",
		"to-do.schema.json",
		"/work/dir",
		Task{ID: "T001", Title: "Test Task", Status: "todo"},
		42,
		"review",
		time.Now(),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := renderer.Render(ReviewPrompt, data)
		if err != nil {
			b.Fatalf("Render failed: %v", err)
		}
	}
}

// BenchmarkValidateRequired benchmarks required variable validation.
func BenchmarkValidateRequired(b *testing.B) {
	data := Data{
		TodoPath:     "to-do.json",
		SchemaPath:   "to-do.schema.json",
		WorkDir:      "/work/dir",
		SelectedTask: Task{ID: "T001", Title: "Test", Status: "todo"},
		Iteration:    42,
		Schedule:     "codex",
		Now:          time.Now().UTC().Format(time.RFC3339),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := validateRequired(IterationPrompt, data)
		if err != nil {
			b.Fatalf("validateRequired failed: %v", err)
		}
	}
}

// BenchmarkStore_Load benchmarks prompt loading from disk.
func BenchmarkStore_Load(b *testing.B) {
	tmpDir := b.TempDir()
	store := &Store{dir: tmpDir}

	// Create test prompt file
	promptContent := "Test prompt content for benchmarking"
	promptPath := tmpDir + "/bench.txt"
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		b.Fatalf("Failed to create test prompt: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Load("bench.txt")
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
	}
}
