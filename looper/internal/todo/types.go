// Package todo parses, validates, and updates task files.
package todo

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

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
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Priority  int        `json:"priority"`
	Status    Status     `json:"status"`
	Details   string     `json:"details,omitempty"`
	Steps     []string   `json:"steps,omitempty"`
	Blockers  []string   `json:"blockers,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Files     []string   `json:"files,omitempty"`
	DependsOn []string   `json:"depends_on,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
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
	Valid     bool
	Errors    []error
	Warnings  []string
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

	// Read schema file
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		// Schema file not found - not an error, just fall back
		result.Warnings = append(result.Warnings, fmt.Sprintf("schema file not found: %s", schemaPath))
		return result
	}

	// Parse the schema
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("invalid schema file: %v", err))
		return result
	}

	// Mark that we're using schema validation
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

	// Perform schema validation
	var fileObj map[string]interface{}
	if err := json.Unmarshal(fileData, &fileObj); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Path: "",
			Err:  fmt.Errorf("failed to unmarshal file for validation: %w", err),
		})
		return result
	}

	// Validate against schema
	validateObjAgainstSchema(fileObj, schema, "", result)

	return result
}

// validateObjAgainstSchema recursively validates an object against a JSON schema.
func validateObjAgainstSchema(obj, schema map[string]interface{}, path string, result *ValidationResult) {
	// Check required fields
	if required, ok := schema["required"].([]interface{}); ok {
		for _, req := range required {
			if field, ok := req.(string); ok {
				if _, exists := obj[field]; !exists {
					result.Valid = false
					result.Errors = append(result.Errors, &ValidationError{
						Path: joinPath(path, field),
						Err:  fmt.Errorf("missing required field"),
					})
				}
			}
		}
	}

	// Check additionalProperties (should be false for our schema)
	if addProps, ok := schema["additionalProperties"].(bool); ok && !addProps {
		// Get allowed properties from schema
		allowedProps := make(map[string]bool)
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			for key := range properties {
				allowedProps[key] = true
			}
		}
		// Check for extra properties
		for key := range obj {
			if !allowedProps[key] {
				result.Valid = false
				result.Errors = append(result.Errors, &ValidationError{
					Path: joinPath(path, key),
					Err:  fmt.Errorf("additional property not allowed"),
				})
			}
		}
	}

	// Validate properties
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for key, propSchema := range properties {
			propSchemaMap, ok := propSchema.(map[string]interface{})
			if !ok {
				continue
			}
			if value, exists := obj[key]; exists {
				validateValueAgainstSchema(value, propSchemaMap, joinPath(path, key), result)
			}
		}
	}
}

// validateValueAgainstSchema validates a value against a schema.
func validateValueAgainstSchema(value interface{}, schema map[string]interface{}, path string, result *ValidationResult) {
	// Check const constraint
	if constVal, ok := schema["const"]; ok {
		if value != constVal {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Path: path,
				Err:  fmt.Errorf("expected %v, got %v", constVal, value),
			})
			return
		}
	}

	// Check type constraint
	typeStr, _ := schema["type"].(string)
	switch typeStr {
	case "object":
		obj, ok := value.(map[string]interface{})
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Path: path,
				Err:  fmt.Errorf("expected object, got %T", value),
			})
			return
		}
		validateObjAgainstSchema(obj, schema, path, result)

	case "array":
		arr, ok := value.([]interface{})
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Path: path,
				Err:  fmt.Errorf("expected array, got %T", value),
			})
			return
		}
		// Validate array items
		if itemsSchema, ok := schema["items"].(map[string]interface{}); ok {
			for i, item := range arr {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				if itemObj, ok := item.(map[string]interface{}); ok {
					validateObjAgainstSchema(itemObj, itemsSchema, itemPath, result)
				}
			}
		}

	case "string":
		if _, ok := value.(string); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Path: path,
				Err:  fmt.Errorf("expected string, got %T", value),
			})
		}
		// Check enum constraint
		if enum, ok := schema["enum"].([]interface{}); ok {
			found := false
			for _, e := range enum {
				if value == e {
					found = true
					break
				}
			}
			if !found {
				result.Valid = false
				result.Errors = append(result.Errors, &ValidationError{
					Path: path,
					Err:  fmt.Errorf("value must be one of %v, got %v", enum, value),
				})
			}
		}

	case "integer":
		// JSON numbers are float64 in Go
		if _, ok := value.(float64); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Path: path,
				Err:  fmt.Errorf("expected integer, got %T", value),
			})
		} else {
			// Check minimum constraint
			if min, ok := schema["minimum"].(float64); ok {
				if val, ok := value.(float64); ok && val < min {
					result.Valid = false
					result.Errors = append(result.Errors, &ValidationError{
						Path: path,
						Err:  fmt.Errorf("must be >= %v, got %v", min, val),
					})
				}
			}
			// Check maximum constraint
			if max, ok := schema["maximum"].(float64); ok {
				if val, ok := value.(float64); ok && val > max {
					result.Valid = false
					result.Errors = append(result.Errors, &ValidationError{
						Path: path,
						Err:  fmt.Errorf("must be <= %v, got %v", max, val),
					})
				}
			}
		}
	}
}

// joinPath joins JSON path segments.
func joinPath(base, segment string) string {
	if base == "" {
		return segment
	}
	return base + "." + segment
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
