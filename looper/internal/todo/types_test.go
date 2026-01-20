package todo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "todo.json")

	// Create a test todo file
	now := time.Now().UTC()
	original := &File{
		SchemaVersion: 1,
		Project: &Project{
			Name: "test-project",
			Root: ".",
		},
		SourceFiles: []string{"README.md"},
		Tasks: []Task{
			{
				ID:        "T001",
				Title:     "Test task",
				Priority:  1,
				Status:    StatusTodo,
				CreatedAt: &now,
				UpdatedAt: &now,
			},
		},
	}

	// Save
	if err := original.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify
	if loaded.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", loaded.SchemaVersion, original.SchemaVersion)
	}
	if len(loaded.Tasks) != 1 {
		t.Fatalf("Tasks count: got %d, want 1", len(loaded.Tasks))
	}
	if loaded.Tasks[0].ID != "T001" {
		t.Errorf("Task ID: got %s, want T001", loaded.Tasks[0].ID)
	}
}

func TestValidateMinimal(t *testing.T) {
	tests := []struct {
		name    string
		file    *File
		wantErr bool
	}{
		{
			name: "valid file",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks: []Task{
					{ID: "T001", Title: "Test", Priority: 1, Status: StatusTodo},
				},
			},
			wantErr: false,
		},
		{
			name: "missing schema_version",
			file: &File{
				SourceFiles: []string{"README.md"},
				Tasks:       []Task{{ID: "T001", Title: "Test", Priority: 1, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "wrong schema_version",
			file: &File{
				SchemaVersion: 2,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 1, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "missing source_files",
			file: &File{
				SchemaVersion: 1,
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 1, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "missing tasks",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
			},
			wantErr: true,
		},
		{
			name: "task missing id",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{Title: "Test", Priority: 1, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "task missing title",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Priority: 1, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "task priority out of range",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 0, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "task invalid status",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 1, Status: "invalid"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.file.Validate(ValidationOptions{})
			if result.Valid == tt.wantErr {
				t.Errorf("Validate() valid = %v, want error %v", result.Valid, tt.wantErr)
			}
		})
	}
}

func TestGetTask(t *testing.T) {
	f := &File{
		Tasks: []Task{
			{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
			{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo},
		},
	}

	// Existing task
	task := f.GetTask("T002")
	if task == nil {
		t.Fatal("GetTask(T002) returned nil")
	}
	if task.Title != "Second" {
		t.Errorf("Title: got %s, want Second", task.Title)
	}

	// Non-existing task
	task = f.GetTask("T999")
	if task != nil {
		t.Errorf("GetTask(T999) should return nil, got %+v", task)
	}
}

func TestFindTaskByStatus(t *testing.T) {
	f := &File{
		Tasks: []Task{
			{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
			{ID: "T002", Title: "Second", Priority: 1, Status: StatusDoing},
			{ID: "T003", Title: "Third", Priority: 1, Status: StatusDoing},
		},
	}

	// Find doing - should return first match
	task := f.FindTaskByStatus(StatusDoing)
	if task == nil {
		t.Fatal("FindTaskByStatus(StatusDoing) returned nil")
	}
	if task.ID != "T002" {
		t.Errorf("ID: got %s, want T002", task.ID)
	}

	// No blocked tasks
	task = f.FindTaskByStatus(StatusBlocked)
	if task != nil {
		t.Errorf("FindTaskByStatus(StatusBlocked) should return nil, got %+v", task)
	}
}

func TestSetTaskStatus(t *testing.T) {
	f := &File{
		Tasks: []Task{
			{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
		},
	}

	// Set status
	err := f.SetTaskStatus("T001", StatusDoing)
	if err != nil {
		t.Fatalf("SetTaskStatus failed: %v", err)
	}

	if f.Tasks[0].Status != StatusDoing {
		t.Errorf("Status: got %s, want doing", f.Tasks[0].Status)
	}
	if f.Tasks[0].UpdatedAt == nil {
		t.Error("UpdatedAt should be set")
	}

	// Non-existing task
	err = f.SetTaskStatus("T999", StatusDone)
	if err == nil {
		t.Error("SetTaskStatus for non-existing task should return error")
	}
}

func TestAddTask(t *testing.T) {
	f := &File{
		Tasks: []Task{
			{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
		},
	}

	newTask := Task{
		ID:       "T002",
		Title:    "Second",
		Priority: 1,
		Status:   StatusTodo,
	}

	f.AddTask(newTask)

	if len(f.Tasks) != 2 {
		t.Fatalf("Tasks count: got %d, want 2", len(f.Tasks))
	}
	if f.Tasks[1].ID != "T002" {
		t.Errorf("Added task ID: got %s, want T002", f.Tasks[1].ID)
	}
	if f.Tasks[1].CreatedAt == nil {
		t.Error("CreatedAt should be set")
	}
	if f.Tasks[1].UpdatedAt == nil {
		t.Error("UpdatedAt should be set")
	}
}

func TestUpdateTask(t *testing.T) {
	f := &File{
		Tasks: []Task{
			{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
		},
	}

	// Update existing task
	err := f.UpdateTask("T001", func(t *Task) {
		t.Title = "Updated"
	})
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	if f.Tasks[0].Title != "Updated" {
		t.Errorf("Title: got %s, want Updated", f.Tasks[0].Title)
	}
	if f.Tasks[0].UpdatedAt == nil {
		t.Error("UpdatedAt should be set")
	}

	// Non-existing task
	err = f.UpdateTask("T999", func(t *Task) {})
	if err == nil {
		t.Error("UpdateTask for non-existing task should return error")
	}
}

func TestValidateWithMissingSchema(t *testing.T) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []Task{
			{ID: "T001", Title: "Test", Priority: 1, Status: StatusTodo},
		},
	}

	// Non-existent schema path should fall back to minimal validation
	result := f.Validate(ValidationOptions{
		SchemaPath: "/non/existent/schema.json",
	})

	if !result.Valid {
		t.Errorf("Valid should be true, got false")
	}
	if len(result.Warnings) == 0 {
		t.Error("Expected warnings when schema file not found")
	}
}

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusTodo, "todo"},
		{StatusDoing, "doing"},
		{StatusBlocked, "blocked"},
		{StatusDone, "done"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Status = %s, want %s", tt.status, tt.expected)
			}
		})
	}
}

func TestTaskIsZero(t *testing.T) {
	task := Task{}
	if !task.IsZero() {
		t.Error("Empty task should be zero")
	}

	task.ID = "T001"
	if task.IsZero() {
		t.Error("Task with ID should not be zero")
	}
}

func TestFileOutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "todo.json")

	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []Task{
			{
				ID:       "T001",
				Title:    "Test task",
				Priority: 1,
				Status:   StatusTodo,
			},
		},
	}

	if err := f.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read the file to check formatting
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Check for 2-space indentation
	content := string(data)
	if !containsIndent(content, "  ") {
		t.Error("Expected 2-space indentation")
	}
	// Should not have 4-space indentation
	if containsIndent(content, "    ") && !containsIndent(content, "  ") {
		t.Error("Should use 2-space indent, not 4-space")
	}
}

func containsIndent(content, indent string) bool {
	// Simple check for indentation presence
	for i := 0; i < len(content)-len(indent); i++ {
		if content[i] == '\n' {
			match := true
			for j := 0; j < len(indent); j++ {
				if content[i+1+j] != indent[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
