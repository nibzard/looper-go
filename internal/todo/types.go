// Package todo parses, validates, and updates task files.
package todo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// idSortKey extracts the numeric value from a task ID for sorting.
// For IDs like "T001", "T2", "T10", it returns 1, 2, 10 respectively.
// If the ID doesn't contain a number, it returns -1.
func idSortKey(id string) int {
	// Find the numeric part after the prefix
	i := 0
	for i < len(id) && (id[i] < '0' || id[i] > '9') {
		i++
	}
	if i == len(id) {
		return -1
	}
	numStr := id[i:]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return -1
	}
	return num
}

// CompareIDs returns true if id1 should come before id2 in numeric-aware ordering.
// If both IDs have numeric parts, compares numerically. Otherwise falls back to
// lexicographic comparison.
func CompareIDs(id1, id2 string) bool {
	k1 := idSortKey(id1)
	k2 := idSortKey(id2)
	if k1 >= 0 && k2 >= 0 {
		return k1 < k2
	}
	return id1 < id2
}

// Status represents a task status.
type Status string

const (
	StatusTodo    Status = "todo"
	StatusDoing   Status = "doing"
	StatusBlocked Status = "blocked"
	StatusDone    Status = "done"
)

// Task represents a single task in the todo list.
type Task struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Description string    `json:"description,omitempty"`
	Reference  string     `json:"reference,omitempty"`
	Priority   int        `json:"priority"`
	Status     Status     `json:"status"`
	Details    string     `json:"details,omitempty"`
	Steps      []string   `json:"steps,omitempty"`
	Blockers   []string   `json:"blockers,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Files      []string   `json:"files,omitempty"`
	DependsOn  []string   `json:"depends_on,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

// IsZero returns true if the task is empty (has no ID).
func (t *Task) IsZero() bool {
	return t.ID == ""
}

// File represents the todo file structure.
type File struct {
	SchemaVersion int      `json:"schema_version"`
	Project       *Project `json:"project,omitempty"`
	SourceFiles   []string `json:"source_files"`
	Tasks         []Task   `json:"tasks"`
}

// Project represents project metadata.
type Project struct {
	Name string `json:"name,omitempty"`
	Root string `json:"root,omitempty"`
}

// ValidationError represents a validation error with context.
type ValidationError struct {
	Path string // JSON path to the error location
	Err  error  // Underlying error
}

func (e *ValidationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Err)
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

// ValidationOptions controls validation behavior.
type ValidationOptions struct {
	// SchemaPath is the path to the JSON Schema file.
	// If empty, validation uses only minimal fallback checks.
	SchemaPath string
	// Strict requires all tasks to pass schema validation.
	Strict bool
}

// ValidationResult contains validation results.
type ValidationResult struct {
	Valid      bool
	Errors     []error
	Warnings   []string
	UsedSchema bool // true if JSON Schema validation was performed
}

// Load reads and parses a todo file from path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read todo file: %w", err)
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse todo file: %w", err)
	}

	return &f, nil
}

// Save writes the todo file to path with 2-space indentation.
func (f *File) Save(path string) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todo file: %w", err)
	}

	// Add trailing newline
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write todo file: %w", err)
	}

	return nil
}

// Validate validates the todo file.
func (f *File) Validate(opts ValidationOptions) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   make([]error, 0),
		Warnings: make([]string, 0),
	}

	// Try JSON Schema validation first if schema path is provided
	if opts.SchemaPath != "" {
		schemaResult := validateWithSchema(f, opts.SchemaPath)
		result.UsedSchema = schemaResult.UsedSchema
		if len(schemaResult.Warnings) > 0 {
			result.Warnings = append(result.Warnings, schemaResult.Warnings...)
		}
		if schemaResult.UsedSchema {
			// Schema validation succeeded - use its results
			if !schemaResult.Valid {
				result.Valid = false
				result.Errors = append(result.Errors, schemaResult.Errors...)
			}
			return result
		}
		// Schema validation not available, fall through to minimal checks
		result.Warnings = append(result.Warnings, "JSON Schema validation not available, using minimal checks")
	}

	// Minimal fallback checks
	f.validateMinimal(result)

	return result
}

// validateMinimal performs minimal validation without JSON Schema.
func (f *File) validateMinimal(result *ValidationResult) {
	// Check schema_version
	if f.SchemaVersion != 1 {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Path: "schema_version",
			Err:  fmt.Errorf("expected 1, got %d", f.SchemaVersion),
		})
	}

	// Check source_files exists
	if f.SourceFiles == nil {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Path: "source_files",
			Err:  fmt.Errorf("missing required field"),
		})
	}

	// Check tasks exists
	if f.Tasks == nil {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Path: "tasks",
			Err:  fmt.Errorf("missing required field"),
		})
		return
	}

	// Validate each task
	for i, task := range f.Tasks {
		path := fmt.Sprintf("tasks[%d]", i)
		if err := validateTaskMinimal(&task, path); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, err)
		}
	}
}

// validateTaskMinimal performs minimal task validation.
func validateTaskMinimal(task *Task, path string) *ValidationError {
	if task.ID == "" {
		return &ValidationError{
			Path: path + ".id",
			Err:  fmt.Errorf("missing required field"),
		}
	}

	if task.Title == "" {
		return &ValidationError{
			Path: path + ".title",
			Err:  fmt.Errorf("missing required field"),
		}
	}

	if task.Priority < 1 || task.Priority > 5 {
		return &ValidationError{
			Path: path + ".priority",
			Err:  fmt.Errorf("must be between 1 and 5, got %d", task.Priority),
		}
	}

	switch task.Status {
	case StatusTodo, StatusDoing, StatusBlocked, StatusDone:
		// Valid status
	default:
		return &ValidationError{
			Path: path + ".status",
			Err:  fmt.Errorf("invalid status %q, must be one of: todo, doing, blocked, done", task.Status),
		}
	}

	return nil
}

// validateWithSchema attempts JSON Schema validation.
func validateWithSchema(f *File, schemaPath string) *ValidationResult {
	result := &ValidationResult{
		Valid:      true,
		Errors:     make([]error, 0),
		Warnings:   make([]string, 0),
		UsedSchema: false,
	}

	absPath, err := filepath.Abs(schemaPath)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("invalid schema path: %v", err))
		return result
	}

	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("schema file not found: %s", absPath))
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to read schema file: %v", err))
		}
		return result
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true

	schema, err := compiler.Compile(absPath)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("invalid schema file: %v", err))
		return result
	}

	result.UsedSchema = true

	// Marshal the file back to JSON for validation
	fileData, err := json.Marshal(f)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Path: "",
			Err:  fmt.Errorf("failed to marshal file for validation: %w", err),
		})
		return result
	}

	var fileObj interface{}
	if err := json.Unmarshal(fileData, &fileObj); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Path: "",
			Err:  fmt.Errorf("failed to unmarshal file for validation: %w", err),
		})
		return result
	}

	if err := schema.Validate(fileObj); err != nil {
		result.Valid = false
		appendSchemaErrors(result, err)
	}

	return result
}

func appendSchemaErrors(result *ValidationResult, err error) {
	if err == nil {
		return
	}

	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		result.Errors = append(result.Errors, err)
		return
	}

	collectSchemaErrors(result, ve)
}

func collectSchemaErrors(result *ValidationResult, err *jsonschema.ValidationError) {
	if err == nil {
		return
	}

	if len(err.Causes) == 0 {
		result.Errors = append(result.Errors, &ValidationError{
			Path: jsonPointerToPath(err.InstanceLocation),
			Err:  fmt.Errorf("%s", err.Message),
		})
		return
	}

	for _, cause := range err.Causes {
		collectSchemaErrors(result, cause)
	}
}

func jsonPointerToPath(ptr string) string {
	if ptr == "" {
		return ""
	}
	if strings.HasPrefix(ptr, "#") {
		ptr = strings.TrimPrefix(ptr, "#")
	}
	if strings.HasPrefix(ptr, "/") {
		ptr = ptr[1:]
	}
	if ptr == "" {
		return ""
	}

	parts := strings.Split(ptr, "/")
	path := ""
	for _, part := range parts {
		part = strings.ReplaceAll(part, "~1", "/")
		part = strings.ReplaceAll(part, "~0", "~")
		if part == "" {
			continue
		}
		if idx, err := strconv.Atoi(part); err == nil {
			path += fmt.Sprintf("[%d]", idx)
			continue
		}
		if path == "" {
			path = part
		} else {
			path += "." + part
		}
	}

	return path
}

// GetTask returns a task by ID, or nil if not found.
func (f *File) GetTask(id string) *Task {
	for i := range f.Tasks {
		if f.Tasks[i].ID == id {
			return &f.Tasks[i]
		}
	}
	return nil
}

// FindTaskByStatus returns the first task matching the status,
// or nil if none found.
func (f *File) FindTaskByStatus(status Status) *Task {
	for i := range f.Tasks {
		if f.Tasks[i].Status == status {
			return &f.Tasks[i]
		}
	}
	return nil
}

// SetTaskStatus updates a task's status and sets updated_at.
func (f *File) SetTaskStatus(id string, status Status) error {
	now := time.Now().UTC()
	for i := range f.Tasks {
		if f.Tasks[i].ID == id {
			f.Tasks[i].Status = status
			f.Tasks[i].UpdatedAt = &now
			return nil
		}
	}
	return fmt.Errorf("task %q not found", id)
}

// AddTask appends a new task to the list.
func (f *File) AddTask(task Task) {
	now := time.Now().UTC()
	if task.CreatedAt == nil {
		task.CreatedAt = &now
	}
	task.UpdatedAt = &now
	f.Tasks = append(f.Tasks, task)
}

// UpdateTask updates an existing task by ID.
func (f *File) UpdateTask(id string, updater func(*Task)) error {
	for i := range f.Tasks {
		if f.Tasks[i].ID == id {
			updater(&f.Tasks[i])
			now := time.Now().UTC()
			f.Tasks[i].UpdatedAt = &now
			return nil
		}
	}
	return fmt.Errorf("task %q not found", id)
}

// SelectTask selects the next task to work on using a deterministic algorithm.
// The selection order is:
// 1. Any task with status "doing" (lowest id wins, using numeric-aware ordering)
// 2. Otherwise highest priority "todo" (priority 1 is highest), tie-break by lowest id
// 3. Otherwise highest priority "blocked", tie-break by lowest id
// Returns nil if no tasks are found.
// ID comparison is numeric-aware: T2 sorts before T10.
func (f *File) SelectTask() *Task {
	// First, look for any "doing" task (lowest id wins)
	var selected *Task
	for i := range f.Tasks {
		if f.Tasks[i].Status == StatusDoing {
			if selected == nil || CompareIDs(f.Tasks[i].ID, selected.ID) {
				selected = &f.Tasks[i]
			}
		}
	}
	if selected != nil {
		return selected
	}

	// No "doing" tasks, find highest priority "todo"
	bestPriority := 5 // maximum priority value (lowest priority)
	for i := range f.Tasks {
		if f.Tasks[i].Status == StatusTodo {
			if selected == nil || f.Tasks[i].Priority < bestPriority ||
				(f.Tasks[i].Priority == bestPriority && CompareIDs(f.Tasks[i].ID, selected.ID)) {
				selected = &f.Tasks[i]
				bestPriority = f.Tasks[i].Priority
			}
		}
	}
	if selected != nil {
		return selected
	}

	// No "todo" tasks, find highest priority "blocked"
	bestPriority = 5
	for i := range f.Tasks {
		if f.Tasks[i].Status == StatusBlocked {
			if selected == nil || f.Tasks[i].Priority < bestPriority ||
				(f.Tasks[i].Priority == bestPriority && CompareIDs(f.Tasks[i].ID, selected.ID)) {
				selected = &f.Tasks[i]
				bestPriority = f.Tasks[i].Priority
			}
		}
	}

	return selected
}

// SetTaskDoing marks a task as "doing" and sets updated_at.
// This is a convenience wrapper around SetTaskStatus.
func (f *File) SetTaskDoing(id string) error {
	return f.SetTaskStatus(id, StatusDoing)
}
