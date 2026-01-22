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

func TestValidateWithSchema(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.json")

	// Write a schema file
	schema := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "source_files", "tasks"],
  "properties": {
    "schema_version": {"type": "integer", "const": 1},
    "source_files": {"type": "array"},
    "tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "title", "priority", "status"],
        "properties": {
          "id": {"type": "string"},
          "title": {"type": "string", "minLength": 1},
          "priority": {"type": "integer", "minimum": 1, "maximum": 5},
          "status": {"type": "string", "enum": ["todo", "doing", "blocked", "done"]}
        }
      }
    }
  }
}`
	if err := os.WriteFile(schemaPath, []byte(schema), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	tests := []struct {
		name    string
		file    *File
		wantErr bool
	}{
		{
			name: "valid file with schema",
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
			name: "invalid schema_version",
			file: &File{
				SchemaVersion: 2,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 1, Status: StatusTodo}},
			},
			wantErr: true,
		},
		{
			name: "invalid status enum",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 1, Status: "invalid"}},
			},
			wantErr: true,
		},
		{
			name: "priority out of range",
			file: &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         []Task{{ID: "T001", Title: "Test", Priority: 10, Status: StatusTodo}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.file.Validate(ValidationOptions{
				SchemaPath: schemaPath,
			})
			if result.Valid == tt.wantErr {
				t.Errorf("Validate() valid = %v, want error %v", result.Valid, tt.wantErr)
			}
			if !result.UsedSchema {
				t.Error("Expected UsedSchema to be true")
			}
		})
	}
}

func TestValidateWithSchemaMissingFile(t *testing.T) {
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
	if result.UsedSchema {
		t.Error("UsedSchema should be false when schema file not found")
	}
	if len(result.Warnings) == 0 {
		t.Error("Expected warnings when schema file not found")
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

func TestSelectTask(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []Task
		wantID   string
		wantDesc string
	}{
		{
			name: "select doing task (lowest id wins)",
			tasks: []Task{
				{ID: "T003", Title: "Third task", Priority: 1, Status: StatusDoing},
				{ID: "T001", Title: "First task", Priority: 3, Status: StatusDoing},
				{ID: "T002", Title: "Second task", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T001",
			wantDesc: "doing task T001 selected over T003 (lower id)",
		},
		{
			name: "no doing tasks, select highest priority todo",
			tasks: []Task{
				{ID: "T001", Title: "Priority 3", Priority: 3, Status: StatusTodo},
				{ID: "T002", Title: "Priority 1", Priority: 1, Status: StatusTodo},
				{ID: "T003", Title: "Priority 2", Priority: 2, Status: StatusTodo},
			},
			wantID:   "T002",
			wantDesc: "priority 1 selected",
		},
		{
			name: "same priority todo, lowest id wins",
			tasks: []Task{
				{ID: "T003", Title: "Third task", Priority: 1, Status: StatusTodo},
				{ID: "T001", Title: "First task", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second task", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T001",
			wantDesc: "T001 selected (lowest id among priority 1 todos)",
		},
		{
			name: "no todo or doing, select highest priority blocked",
			tasks: []Task{
				{ID: "T001", Title: "Priority 3 blocked", Priority: 3, Status: StatusBlocked},
				{ID: "T002", Title: "Priority 1 blocked", Priority: 1, Status: StatusBlocked},
			},
			wantID:   "T002",
			wantDesc: "priority 1 blocked selected",
		},
		{
			name: "same priority blocked, lowest id wins",
			tasks: []Task{
				{ID: "T003", Title: "Third blocked", Priority: 2, Status: StatusBlocked},
				{ID: "T001", Title: "First blocked", Priority: 2, Status: StatusBlocked},
				{ID: "T002", Title: "Second blocked", Priority: 2, Status: StatusBlocked},
			},
			wantID:   "T001",
			wantDesc: "T001 selected (lowest id among priority 2 blocked)",
		},
		{
			name: "doing takes priority over todo even with higher priority",
			tasks: []Task{
				{ID: "T005", Title: "Doing task", Priority: 5, Status: StatusDoing},
				{ID: "T001", Title: "Priority 1 todo", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T005",
			wantDesc: "doing always selected first regardless of priority",
		},
		{
			name: "only done tasks returns nil",
			tasks: []Task{
				{ID: "T001", Title: "Done task", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Another done", Priority: 1, Status: StatusDone},
			},
			wantID:   "",
			wantDesc: "no selectable tasks",
		},
		{
			name:     "empty tasks returns nil",
			tasks:    []Task{},
			wantID:   "",
			wantDesc: "no tasks at all",
		},
		{
			name: "complex: doing > todo > blocked",
			tasks: []Task{
				{ID: "T005", Title: "Priority 1 todo", Priority: 1, Status: StatusTodo},
				{ID: "T010", Title: "Priority 1 blocked", Priority: 1, Status: StatusBlocked},
				{ID: "T003", Title: "Priority 3 doing", Priority: 3, Status: StatusDoing},
				{ID: "T001", Title: "Priority 2 todo", Priority: 2, Status: StatusTodo},
			},
			wantID:   "T003",
			wantDesc: "doing selected over higher priority todo/blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         tt.tasks,
			}

			got := f.SelectTask()

			if tt.wantID == "" {
				if got != nil {
					t.Errorf("%s: SelectTask() = %v (%s), want nil", tt.name, got, got.ID)
				}
				return
			}

			if got == nil {
				t.Fatalf("%s: SelectTask() = nil, want task with ID %s (%s)", tt.name, tt.wantID, tt.wantDesc)
			}

			if got.ID != tt.wantID {
				t.Errorf("%s: SelectTask() ID = %s, want %s (%s)", tt.name, got.ID, tt.wantID, tt.wantDesc)
			}
		})
	}
}

func TestIDSortKey(t *testing.T) {
	tests := []struct {
		id    string
		want  int
		desc  string
	}{
		{"T001", 1, "standard T-prefixed ID"},
		{"T2", 2, "short T-prefixed ID"},
		{"T10", 10, "two-digit T-prefixed ID"},
		{"T100", 100, "three-digit T-prefixed ID"},
		{"task-1", 1, "dash-prefixed ID"},
		{"ABC123", 123, "letter prefix with number"},
		{"123", 123, "numeric only ID"},
		{"NOTHING", -1, "no numeric part"},
		{"", -1, "empty string"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := idSortKey(tt.id)
			if got != tt.want {
				t.Errorf("idSortKey(%q) = %d, want %d (%s)", tt.id, got, tt.want, tt.desc)
			}
		})
	}
}

func TestCompareIDs(t *testing.T) {
	tests := []struct {
		id1     string
		id2     string
		wantLT  bool  // true if id1 < id2
		desc    string
	}{
		{"T1", "T2", true, "T1 before T2"},
		{"T2", "T10", true, "T2 before T10 (numeric-aware)"},
		{"T9", "T10", true, "T9 before T10 (numeric-aware)"},
		{"T001", "T2", true, "T001 before T2 (same numeric value, different format)"},
		{"T10", "T2", false, "T10 after T2 (numeric-aware)"},
		{"A1", "A2", true, "A1 before A2"},
		{"A2", "A10", true, "A2 before A10 (numeric-aware)"},
		{"A", "B", true, "non-numeric IDs fall back to lexicographic"},
		{"Z", "A", false, "non-numeric IDs fall back to lexicographic"},
		{"T1", "A1", false, "mixed: T after A lexicographically"},
	}

	for _, tt := range tests {
		t.Run(tt.id1+"_vs_"+tt.id2, func(t *testing.T) {
			got := CompareIDs(tt.id1, tt.id2)
			if got != tt.wantLT {
				t.Errorf("CompareIDs(%q, %q) = %v, want %v (%s)", tt.id1, tt.id2, got, tt.wantLT, tt.desc)
			}
		})
	}
}

func TestSelectTaskNumericIDOrdering(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []Task
		wantID   string
		wantDesc string
	}{
		{
			name: "select doing task with numeric ID ordering (T2 before T10)",
			tasks: []Task{
				{ID: "T10", Title: "Tenth task", Priority: 1, Status: StatusDoing},
				{ID: "T2", Title: "Second task", Priority: 1, Status: StatusDoing},
				{ID: "T1", Title: "First task", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T2",
			wantDesc: "T2 is lowest numeric ID among doing tasks (T2 < T10)",
		},
		{
			name: "select todo task with numeric ID ordering (T2 before T10)",
			tasks: []Task{
				{ID: "T10", Title: "Tenth task", Priority: 1, Status: StatusTodo},
				{ID: "T2", Title: "Second task", Priority: 1, Status: StatusTodo},
				{ID: "T1", Title: "First task", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T1",
			wantDesc: "T1 (lowest numeric ID among same-priority todos)",
		},
		{
			name: "select blocked task with numeric ID ordering",
			tasks: []Task{
				{ID: "T100", Title: "Hundredth blocked", Priority: 2, Status: StatusBlocked},
				{ID: "T20", Title: "Twentieth blocked", Priority: 2, Status: StatusBlocked},
				{ID: "T3", Title: "Third blocked", Priority: 2, Status: StatusBlocked},
			},
			wantID:   "T3",
			wantDesc: "T3 (lowest numeric ID among same-priority blocked: 3 < 20 < 100)",
		},
		{
			name: "mixed ID formats with same numeric value",
			tasks: []Task{
				{ID: "T001", Title: "T001 task", Priority: 1, Status: StatusDoing},
				{ID: "T1", Title: "T1 task", Priority: 1, Status: StatusDoing},
				{ID: "T2", Title: "T2 task", Priority: 1, Status: StatusDoing},
			},
			wantID:   "T001",
			wantDesc: "T001 selected (both T001 and T1 have value 1, T001 < T1 lexicographically)",
		},
		{
			name: "large ID numbers",
			tasks: []Task{
				{ID: "T99", Title: "Ninety-ninth", Priority: 1, Status: StatusTodo},
				{ID: "T100", Title: "Hundredth", Priority: 1, Status: StatusTodo},
				{ID: "T9", Title: "Ninth", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T9",
			wantDesc: "T9 (lowest numeric value: 9 < 99 < 100)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         tt.tasks,
			}

			got := f.SelectTask()

			if tt.wantID == "" {
				if got != nil {
					t.Errorf("%s: SelectTask() = %v (%s), want nil", tt.name, got, got.ID)
				}
				return
			}

			if got == nil {
				t.Fatalf("%s: SelectTask() = nil, want task with ID %s (%s)", tt.name, tt.wantID, tt.wantDesc)
			}

			if got.ID != tt.wantID {
				t.Errorf("%s: SelectTask() ID = %s, want %s (%s)", tt.name, got.ID, tt.wantID, tt.wantDesc)
			}
		})
	}
}

func TestSetTaskDoing(t *testing.T) {
	f := &File{
		SchemaVersion: 1,
		SourceFiles:   []string{"README.md"},
		Tasks: []Task{
			{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
		},
	}

	// Set to doing
	err := f.SetTaskDoing("T001")
	if err != nil {
		t.Fatalf("SetTaskDoing failed: %v", err)
	}

	if f.Tasks[0].Status != StatusDoing {
		t.Errorf("Status: got %s, want doing", f.Tasks[0].Status)
	}
	if f.Tasks[0].UpdatedAt == nil {
		t.Error("UpdatedAt should be set")
	}

	// Non-existing task
	err = f.SetTaskDoing("T999")
	if err == nil {
		t.Error("SetTaskDoing for non-existing task should return error")
	}
}

func TestValidateDependencies(t *testing.T) {
	tests := []struct {
		name        string
		tasks       []Task
		wantErr     error
		description string
	}{
		{
			name: "no dependencies",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo},
			},
			wantErr:     nil,
			description: "tasks without dependencies are valid",
		},
		{
			name: "valid dependencies all done",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
			},
			wantErr:     nil,
			description: "dependency on completed task is valid",
		},
		{
			name: "valid dependencies chain",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002"}},
			},
			wantErr:     nil,
			description: "dependency chain is valid",
		},
		{
			name: "valid multiple dependencies",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001", "T002"}},
			},
			wantErr:     nil,
			description: "multiple dependencies are valid",
		},
		{
			name: "missing dependency",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T999"}},
			},
			wantErr:     &MissingDependencyError{TaskID: "T002", DepID: "T999"},
			description: "dependency on non-existent task is invalid",
		},
		{
			name: "simple cycle",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002"}},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
			},
			wantErr:     &DependencyCycleError{Cycle: []string{"T001", "T002", "T001"}},
			description: "direct cycle is detected",
		},
		{
			name: "long cycle",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo, DependsOn: []string{"T004"}},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002"}},
				{ID: "T004", Title: "Fourth", Priority: 1, Status: StatusTodo, DependsOn: []string{"T003"}},
			},
			wantErr:     &DependencyCycleError{Cycle: []string{"T001", "T004", "T003", "T002", "T001"}},
			description: "longer cycle is detected",
		},
		{
			name: "self cycle",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
			},
			wantErr:     &DependencyCycleError{Cycle: []string{"T001", "T001"}},
			description: "self-dependency is detected as a cycle",
		},
		{
			name: "diamond dependency valid",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T004", Title: "Fourth", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002", "T003"}},
			},
			wantErr:     nil,
			description: "diamond dependency structure is valid (not a cycle)",
		},
		{
			name: "missing dependency in chain",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T999", "T002"}},
			},
			wantErr:     &MissingDependencyError{TaskID: "T003", DepID: "T999"},
			description: "missing dependency in multi-dependency list is detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         tt.tasks,
			}

			err := f.ValidateDependencies()

			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("%s: ValidateDependencies() returned error: %v", tt.description, err)
				}
				return
			}

			if err == nil {
				t.Errorf("%s: ValidateDependencies() returned nil, want error %T", tt.description, tt.wantErr)
				return
			}

			// Check error type
			switch wantErr := tt.wantErr.(type) {
			case *MissingDependencyError:
				gotErr, ok := err.(*MissingDependencyError)
				if !ok {
					t.Errorf("%s: error type is %T, want *MissingDependencyError", tt.description, err)
				} else if gotErr.TaskID != wantErr.TaskID || gotErr.DepID != wantErr.DepID {
					t.Errorf("%s: got %+v, want %+v", tt.description, gotErr, wantErr)
				}
			case *DependencyCycleError:
				gotErr, ok := err.(*DependencyCycleError)
				if !ok {
					t.Errorf("%s: error type is %T, want *DependencyCycleError", tt.description, err)
				} else {
					// For cycle errors, just check that we got a cycle - the exact path may vary
					if len(gotErr.Cycle) < 2 {
						t.Errorf("%s: cycle too short: %v", tt.description, gotErr.Cycle)
					}
					// Verify first and last are the same (cycle property)
					if gotErr.Cycle[0] != gotErr.Cycle[len(gotErr.Cycle)-1] {
						t.Errorf("%s: cycle doesn't loop back: %v", tt.description, gotErr.Cycle)
					}
				}
			default:
				t.Errorf("%s: unexpected wantErr type: %T", tt.description, tt.wantErr)
			}
		})
	}
}

func TestSelectTaskWithDependencies(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []Task
		wantID   string
		wantDesc string
	}{
		{
			name: "select task without dependencies when dependencies exist",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second (depends on T001)", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
			},
			wantID:   "T001",
			wantDesc: "T001 has no dependencies, T002 depends on T001",
		},
		{
			name: "select task after dependency is done",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Second (depends on T001)", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
			},
			wantID:   "T002",
			wantDesc: "T001 is done, T002 can be selected",
		},
		{
			name: "select higher priority task even with dependency",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 2, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusDone},
				{ID: "T003", Title: "Third (depends on T002)", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002"}},
			},
			wantID:   "T003",
			wantDesc: "T003 has priority 1 and satisfied dependencies",
		},
		{
			name: "no tasks with satisfied dependencies",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002"}},
			},
			wantID:   "T001",
			wantDesc: "T001 is the only one with satisfied (empty) dependencies",
		},
		{
			name: "blocked task with unsatisfied dependencies not selected",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 2, Status: StatusTodo},
				{ID: "T002", Title: "Second blocked", Priority: 1, Status: StatusBlocked, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 3, Status: StatusTodo},
			},
			wantID:   "T001",
			wantDesc: "T002 is blocked but depends on T001, T001 selected first",
		},
		{
			name: "multiple dependencies all must be done",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo},
				{ID: "T003", Title: "Third (depends on T001, T002)", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001", "T002"}},
			},
			wantID:   "T002",
			wantDesc: "T003 depends on both T001 and T002, only T001 is done",
		},
		{
			name: "all tasks have unsatisfied dependencies, pick lowest ID among todo",
			tasks: []Task{
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T999"}},
				{ID: "T001", Title: "First", Priority: 1, Status: StatusTodo, DependsOn: []string{"T999"}},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T999"}},
			},
			wantID:   "",
			wantDesc: "all tasks depend on missing T999, none can be selected",
		},
		{
			name: "doing task with satisfied dependencies is selected",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Second doing", Priority: 5, Status: StatusDoing, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T002",
			wantDesc: "T002 depends on done T001 and is selected before todo tasks",
		},
		{
			name: "doing task with unsatisfied dependencies is skipped",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Second doing", Priority: 5, Status: StatusDoing, DependsOn: []string{"T003"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo},
			},
			wantID:   "T003",
			wantDesc: "T002 depends on T003 which is not done, so T003 is selected",
		},
		{
			name: "complex dependency chain with mixed completion",
			tasks: []Task{
				{ID: "T001", Title: "First", Priority: 1, Status: StatusDone},
				{ID: "T002", Title: "Second", Priority: 1, Status: StatusTodo, DependsOn: []string{"T001"}},
				{ID: "T003", Title: "Third", Priority: 1, Status: StatusTodo, DependsOn: []string{"T002"}},
				{ID: "T004", Title: "Fourth", Priority: 2, Status: StatusTodo},
			},
			wantID:   "T002",
			wantDesc: "T002 is next in chain after T001, selected over T004 (priority 1 vs 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				SchemaVersion: 1,
				SourceFiles:   []string{"README.md"},
				Tasks:         tt.tasks,
			}

			got := f.SelectTask()

			if tt.wantID == "" {
				if got != nil {
					t.Errorf("%s: SelectTask() = %v (%s), want nil", tt.name, got, got.ID)
				}
				return
			}

			if got == nil {
				t.Fatalf("%s: SelectTask() = nil, want task with ID %s (%s)", tt.name, tt.wantID, tt.wantDesc)
			}

			if got.ID != tt.wantID {
				t.Errorf("%s: SelectTask() ID = %s, want %s (%s)", tt.name, got.ID, tt.wantID, tt.wantDesc)
			}
		})
	}
}
