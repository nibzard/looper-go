package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewStore tests creating a prompt store.
func TestNewStore(t *testing.T) {
	workDir := "/work"
	store := NewStore(workDir, "")
	if store.Dir() != filepath.Join(workDir, "prompts") {
		t.Errorf("NewStore() dir = %q, want %q", store.Dir(), filepath.Join(workDir, "prompts"))
	}

	customDir := "/custom/prompts"
	store = NewStore(workDir, customDir)
	if store.Dir() != customDir {
		t.Errorf("NewStore() with custom dir = %q, want %q", store.Dir(), customDir)
	}
}

// TestStoreLoad tests loading prompt files from disk.
func TestStoreLoad(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a test prompt file
	promptContent := "Test prompt with {{.Variable}}"
	promptPath := filepath.Join(promptsDir, "test.txt")
	if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
		t.Fatalf("Failed to write test prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	content, err := store.Load("test.txt")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if content != promptContent {
		t.Errorf("Load() content = %q, want %q", content, promptContent)
	}

	// Test loading non-existent file
	_, err = store.Load("nonexistent.txt")
	if err == nil {
		t.Error("Load() of non-existent file expected error, got nil")
	}

	// Test loading with empty name
	_, err = store.Load("")
	if err == nil {
		t.Error("Load() with empty name expected error, got nil")
	}
}

// TestNewData tests creating prompt data.
func TestNewData(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	data := NewData("/todo.json", "/schema.json", "/work", Task{
		ID:     "T001",
		Title:  "Test Task",
		Status: "todo",
	}, 5, "codex", now)

	if data.TodoPath != "/todo.json" {
		t.Errorf("TodoPath = %q, want %q", data.TodoPath, "/todo.json")
	}
	if data.SchemaPath != "/schema.json" {
		t.Errorf("SchemaPath = %q, want %q", data.SchemaPath, "/schema.json")
	}
	if data.WorkDir != "/work" {
		t.Errorf("WorkDir = %q, want %q", data.WorkDir, "/work")
	}
	if data.SelectedTask.ID != "T001" {
		t.Errorf("SelectedTask.ID = %q, want %q", data.SelectedTask.ID, "T001")
	}
	if data.SelectedTask.Title != "Test Task" {
		t.Errorf("SelectedTask.Title = %q, want %q", data.SelectedTask.Title, "Test Task")
	}
	if data.Iteration != 5 {
		t.Errorf("Iteration = %d, want %d", data.Iteration, 5)
	}
	if data.Now != now.UTC().Format(time.RFC3339) {
		t.Errorf("Now = %q, want %q", data.Now, "2024-01-01T12:00:00Z")
	}
}

// TestNewDataForRepair tests creating prompt data for repair flow.
func TestNewDataForRepair(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	data := NewDataForRepair("/todo.json", "/schema.json", "/work", "validation failed", now)

	if data.TodoPath != "/todo.json" {
		t.Errorf("TodoPath = %q, want %q", data.TodoPath, "/todo.json")
	}
	if data.SchemaPath != "/schema.json" {
		t.Errorf("SchemaPath = %q, want %q", data.SchemaPath, "/schema.json")
	}
	if data.WorkDir != "/work" {
		t.Errorf("WorkDir = %q, want %q", data.WorkDir, "/work")
	}
	if data.ErrorMessage != "validation failed" {
		t.Errorf("ErrorMessage = %q, want %q", data.ErrorMessage, "validation failed")
	}
	if data.Now != now.UTC().Format(time.RFC3339) {
		t.Errorf("Now = %q, want %q", data.Now, "2024-01-01T12:00:00Z")
	}
}

// TestRenderer tests the prompt renderer with strict missing variable checking.
func TestRenderer(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)

	// Test nil renderer
	nilRenderer := NewRenderer(nil)
	_, err := nilRenderer.Render("test.txt", Data{})
	if err == nil {
		t.Error("Render() with nil renderer expected error, got nil")
	}

	// Test unknown prompt
	_, err = renderer.Render("unknown.txt", Data{})
	if err == nil {
		t.Error("Render() with unknown prompt expected error, got nil")
	}
}

// TestRenderIterationPrompt tests rendering the iteration prompt with all variables.
func TestRenderIterationPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a minimal iteration prompt template
	iterationPrompt := `Goal: complete task {{.SelectedTask.ID}}
Title: {{.SelectedTask.Title}}
Status: {{.SelectedTask.Status}}
WorkDir: {{.WorkDir}}
TodoPath: {{.TodoPath}}
SchemaPath: {{.SchemaPath}}
Iteration: {{.Iteration}}
Schedule: {{.Schedule}}
Now: {{.Now}}
`
	promptPath := filepath.Join(promptsDir, IterationPrompt)
	if err := os.WriteFile(promptPath, []byte(iterationPrompt), 0644); err != nil {
		t.Fatalf("Failed to write iteration prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	data := NewData(
		"/path/to/todo.json",
		"/path/to/schema.json",
		"/work/dir",
		Task{
			ID:     "T001",
			Title:  "Implement feature",
			Status: "doing",
		},
		42,
		"codex",
		now,
	)

	output, err := renderer.Render(IterationPrompt, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Verify all variables were replaced
	if !strings.Contains(output, "task T001") {
		t.Errorf("Output should contain 'task T001', got: %s", output)
	}
	if !strings.Contains(output, "Title: Implement feature") {
		t.Errorf("Output should contain title, got: %s", output)
	}
	if !strings.Contains(output, "Status: doing") {
		t.Errorf("Output should contain status, got: %s", output)
	}
	if !strings.Contains(output, "WorkDir: /work/dir") {
		t.Errorf("Output should contain workdir, got: %s", output)
	}
	if !strings.Contains(output, "TodoPath: /path/to/todo.json") {
		t.Errorf("Output should contain todopath, got: %s", output)
	}
	if !strings.Contains(output, "SchemaPath: /path/to/schema.json") {
		t.Errorf("Output should contain schemapath, got: %s", output)
	}
	if !strings.Contains(output, "Iteration: 42") {
		t.Errorf("Output should contain iteration, got: %s", output)
	}
	if !strings.Contains(output, "Schedule: codex") {
		t.Errorf("Output should contain schedule, got: %s", output)
	}
	if !strings.Contains(output, "2024-01-01T12:00:00Z") {
		t.Errorf("Output should contain timestamp, got: %s", output)
	}
}

// TestRenderMissingRequiredVariable tests that missing required variables cause errors.
func TestRenderMissingRequiredVariable(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a minimal iteration prompt template
	iterationPrompt := `Task {{.SelectedTask.ID}}: {{.SelectedTask.Title}}`
	promptPath := filepath.Join(promptsDir, IterationPrompt)
	if err := os.WriteFile(promptPath, []byte(iterationPrompt), 0644); err != nil {
		t.Fatalf("Failed to write iteration prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)

	tests := []struct {
		name    string
		prompt  string
		data    Data
		wantErr bool
		errMsg  string
	}{
		{
			name:   "valid iteration prompt",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"/work",
				Task{ID: "T001", Title: "Test", Status: "todo"},
				1,
				"codex",
				time.Now(),
			),
			wantErr: false,
		},
		{
			name:   "missing TodoPath",
			prompt: IterationPrompt,
			data: NewData(
				"",
				"/schema.json",
				"/work",
				Task{ID: "T001", Title: "Test", Status: "todo"},
				1,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires TodoPath",
		},
		{
			name:   "missing SchemaPath",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"",
				"/work",
				Task{ID: "T001", Title: "Test", Status: "todo"},
				1,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires SchemaPath",
		},
		{
			name:   "missing WorkDir",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"",
				Task{ID: "T001", Title: "Test", Status: "todo"},
				1,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires WorkDir",
		},
		{
			name:   "missing Task ID",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"/work",
				Task{ID: "", Title: "Test", Status: "todo"},
				1,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires SelectedTask.ID",
		},
		{
			name:   "missing Task Title",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"/work",
				Task{ID: "T001", Title: "", Status: "todo"},
				1,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires SelectedTask.Title",
		},
		{
			name:   "missing Task Status",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"/work",
				Task{ID: "T001", Title: "Test", Status: ""},
				1,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires SelectedTask.Status",
		},
		{
			name:   "invalid Iteration (zero)",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"/work",
				Task{ID: "T001", Title: "Test", Status: "todo"},
				0,
				"codex",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires Iteration > 0",
		},
		{
			name:   "missing Schedule",
			prompt: IterationPrompt,
			data: NewData(
				"/todo.json",
				"/schema.json",
				"/work",
				Task{ID: "T001", Title: "Test", Status: "todo"},
				1,
				"",
				time.Now(),
			),
			wantErr: true,
			errMsg:  "requires Schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderer.Render(tt.prompt, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Error message should contain %q, got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

// TestRenderBootstrapPrompt tests rendering the bootstrap prompt.
func TestRenderBootstrapPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a minimal bootstrap prompt template
	bootstrapPrompt := `Bootstrap: {{.TodoPath}}
Schema: {{.SchemaPath}}
WorkDir: {{.WorkDir}}
{{if .UserPrompt}}User Prompt: {{.UserPrompt}}{{end}}`
	promptPath := filepath.Join(promptsDir, BootstrapPrompt)
	if err := os.WriteFile(promptPath, []byte(bootstrapPrompt), 0644); err != nil {
		t.Fatalf("Failed to write bootstrap prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)

	// Test without user prompt
	data := NewDataForBootstrap(
		"/todo.json",
		"/schema.json",
		"/work",
		"",
		time.Now(),
	)

	output, err := renderer.Render(BootstrapPrompt, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(output, "Bootstrap: /todo.json") {
		t.Errorf("Output should contain todopath, got: %s", output)
	}
	if !strings.Contains(output, "Schema: /schema.json") {
		t.Errorf("Output should contain schemapath, got: %s", output)
	}
	if !strings.Contains(output, "WorkDir: /work") {
		t.Errorf("Output should contain workdir, got: %s", output)
	}
	if strings.Contains(output, "User Prompt:") {
		t.Errorf("Output should not contain user prompt when empty, got: %s", output)
	}

	// Test with user prompt
	dataWithPrompt := NewDataForBootstrap(
		"/todo.json",
		"/schema.json",
		"/work",
		"Build a REST API for task management",
		time.Now(),
	)

	outputWithPrompt, err := renderer.Render(BootstrapPrompt, dataWithPrompt)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(outputWithPrompt, "User Prompt: Build a REST API for task management") {
		t.Errorf("Output should contain user prompt, got: %s", outputWithPrompt)
	}
}

// TestNewDataForBootstrap tests creating prompt data for bootstrap flow.
func TestNewDataForBootstrap(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Test without user prompt
	data := NewDataForBootstrap("/todo.json", "/schema.json", "/work", "", now)

	if data.TodoPath != "/todo.json" {
		t.Errorf("TodoPath = %q, want %q", data.TodoPath, "/todo.json")
	}
	if data.SchemaPath != "/schema.json" {
		t.Errorf("SchemaPath = %q, want %q", data.SchemaPath, "/schema.json")
	}
	if data.WorkDir != "/work" {
		t.Errorf("WorkDir = %q, want %q", data.WorkDir, "/work")
	}
	if data.UserPrompt != "" {
		t.Errorf("UserPrompt = %q, want empty", data.UserPrompt)
	}
	if data.Now != now.UTC().Format(time.RFC3339) {
		t.Errorf("Now = %q, want %q", data.Now, "2024-01-01T12:00:00Z")
	}

	// Test with user prompt
	userPrompt := "Build a web scraper for news articles"
	dataWithPrompt := NewDataForBootstrap("/todo.json", "/schema.json", "/work", userPrompt, now)

	if dataWithPrompt.UserPrompt != userPrompt {
		t.Errorf("UserPrompt = %q, want %q", dataWithPrompt.UserPrompt, userPrompt)
	}
}

// TestRenderRepairPrompt tests rendering the repair prompt with error message.
func TestRenderRepairPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a minimal repair prompt template
	repairPrompt := `Repair: {{.TodoPath}}
Errors: {{.ErrorMessage}}
`
	promptPath := filepath.Join(promptsDir, RepairPrompt)
	if err := os.WriteFile(promptPath, []byte(repairPrompt), 0644); err != nil {
		t.Fatalf("Failed to write repair prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)

	errMsg := "validation failed: invalid task status"
	data := NewDataForRepair("/todo.json", "/schema.json", "/work", errMsg, time.Now())

	output, err := renderer.Render(RepairPrompt, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(output, "Repair: /todo.json") {
		t.Errorf("Output should contain todopath, got: %s", output)
	}
	if !strings.Contains(output, errMsg) {
		t.Errorf("Output should contain error message, got: %s", output)
	}
}

// TestRenderReviewPrompt tests rendering the review prompt.
func TestRenderReviewPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a minimal review prompt template
	reviewPrompt := `Review: {{.TodoPath}}
WorkDir: {{.WorkDir}}
`
	promptPath := filepath.Join(promptsDir, ReviewPrompt)
	if err := os.WriteFile(promptPath, []byte(reviewPrompt), 0644); err != nil {
		t.Fatalf("Failed to write review prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)

	data := NewData(
		"/todo.json",
		"/schema.json",
		"/work",
		Task{},
		5,
		"review",
		time.Now(),
	)

	output, err := renderer.Render(ReviewPrompt, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(output, "Review: /todo.json") {
		t.Errorf("Output should contain todopath, got: %s", output)
	}
	if !strings.Contains(output, "WorkDir: /work") {
		t.Errorf("Output should contain workdir, got: %s", output)
	}
}

// TestRenderPushPrompt tests rendering the push prompt.
func TestRenderPushPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.Mkdir(promptsDir, 0755); err != nil {
		t.Fatalf("Failed to create prompts dir: %v", err)
	}

	// Create a minimal push prompt template
	pushPrompt := `Push: {{.WorkDir}}
Now: {{.Now}}
{{if .HasGH}}GH: yes{{else}}GH: no{{end}}
`
	promptPath := filepath.Join(promptsDir, PushPrompt)
	if err := os.WriteFile(promptPath, []byte(pushPrompt), 0644); err != nil {
		t.Fatalf("Failed to write push prompt: %v", err)
	}

	store := NewStore(tmpDir, "")
	renderer := NewRenderer(store)
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	data := Data{
		WorkDir: "/work",
		Now:     now.UTC().Format(time.RFC3339),
		HasGH:   true,
	}
	output, err := renderer.Render(PushPrompt, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(output, "Push: /work") {
		t.Errorf("Output should contain workdir, got: %s", output)
	}
	if !strings.Contains(output, "Now: 2024-01-02T03:04:05Z") {
		t.Errorf("Output should contain timestamp, got: %s", output)
	}
	if !strings.Contains(output, "GH: yes") {
		t.Errorf("Output should contain GH availability, got: %s", output)
	}

	_, err = renderer.Render(PushPrompt, Data{Now: now.UTC().Format(time.RFC3339)})
	if err == nil || !strings.Contains(err.Error(), "requires WorkDir") {
		t.Errorf("Expected WorkDir error, got: %v", err)
	}

	_, err = renderer.Render(PushPrompt, Data{WorkDir: "/work"})
	if err == nil || !strings.Contains(err.Error(), "requires Now") {
		t.Errorf("Expected Now error, got: %v", err)
	}
}

// TestDefaultPromptDir tests the DefaultPromptDir function.
func TestDefaultPromptDir(t *testing.T) {
	workDir := "/test/work"
	expected := "/test/work/prompts"
	result := DefaultPromptDir(workDir)
	if result != expected {
		t.Errorf("DefaultPromptDir() = %q, want %q", result, expected)
	}
}

// TestBundledSchema tests that the bundled schema is valid JSON.
func TestBundledSchema(t *testing.T) {
	schema, err := BundledSchema()
	if err != nil {
		t.Fatalf("BundledSchema() error = %v", err)
	}
	if len(schema) == 0 {
		t.Error("BundledSchema() returned empty data")
	}
	// Verify it's valid JSON by checking for opening brace
	if string(schema[0]) != "{" {
		t.Errorf("BundledSchema() should start with '{', got %q", string(schema[0]))
	}
}
