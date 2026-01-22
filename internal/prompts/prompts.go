package prompts

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

const (
	BootstrapPrompt = "bootstrap.txt"
	RepairPrompt    = "repair.txt"
	IterationPrompt = "iteration.txt"
	ReviewPrompt    = "review.txt"
	SummarySchema   = "summary.schema.json"
	PushPrompt      = "push.txt"
)

// bundledTodoSchema is the embedded todo schema JSON.
const bundledTodoSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Codex RALF Todo",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "source_files", "tasks"],
  "properties": {
    "schema_version": { "type": "integer", "const": 1 },
    "project": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "name": { "type": "string" },
        "root": { "type": "string" }
      }
    },
    "source_files": {
      "type": "array",
      "items": { "type": "string" }
    },
    "tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "title", "priority", "status"],
        "properties": {
          "id": { "type": "string" },
          "title": { "type": "string", "minLength": 1 },
          "description": { "type": "string" },
          "reference": { "type": "string" },
          "priority": { "type": "integer", "minimum": 1, "maximum": 5 },
          "status": { "type": "string", "enum": ["todo", "doing", "blocked", "done"] },
          "details": { "type": "string" },
          "steps": { "type": "array", "items": { "type": "string" } },
          "blockers": { "type": "array", "items": { "type": "string" } },
          "tags": { "type": "array", "items": { "type": "string" } },
          "files": { "type": "array", "items": { "type": "string" } },
          "depends_on": { "type": "array", "items": { "type": "string" } },
          "created_at": { "type": "string", "format": "date-time" },
          "updated_at": { "type": "string", "format": "date-time" }
        }
      }
    }
  }
}`

// BundledSchema returns the embedded todo schema JSON content.
func BundledSchema() ([]byte, error) {
	return []byte(bundledTodoSchema), nil
}

// DefaultPromptDir returns the default prompt directory for a work dir.
func DefaultPromptDir(workDir string) string {
	return filepath.Join(workDir, "prompts")
}

// Store loads prompt assets from disk.
type Store struct {
	dir string
}

// NewStore creates a prompt store rooted at promptDir, defaulting to workDir/prompts.
func NewStore(workDir, promptDir string) *Store {
	if promptDir == "" {
		promptDir = DefaultPromptDir(workDir)
	}
	return &Store{dir: promptDir}
}

// Dir returns the prompt directory.
func (s *Store) Dir() string {
	return s.dir
}

// Load reads a prompt asset as a string.
func (s *Store) Load(name string) (string, error) {
	if name == "" {
		return "", errors.New("prompt name is empty")
	}
	path := filepath.Join(s.dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt %q: %w", name, err)
	}
	return string(data), nil
}

// Task is the minimal task data needed for prompt rendering.
type Task struct {
	ID     string
	Title  string
	Status string
}

// Data holds prompt template variables.
type Data struct {
	TodoPath     string
	SchemaPath   string
	WorkDir      string
	HasGH        bool
	SelectedTask Task
	Iteration    int
	Schedule     string
	Now          string
	ErrorMessage string // Validation errors for repair prompt
	UserPrompt   string // User-provided prompt for bootstrap
}

// NewData builds prompt data with a UTC timestamp formatted in RFC3339.
func NewData(todoPath, schemaPath, workDir string, selected Task, iteration int, schedule string, now time.Time) Data {
	return Data{
		TodoPath:     todoPath,
		SchemaPath:   schemaPath,
		WorkDir:      workDir,
		SelectedTask: selected,
		Iteration:    iteration,
		Schedule:     schedule,
		Now:          now.UTC().Format(time.RFC3339),
	}
}

// NewDataForBootstrap builds prompt data for bootstrap flow with optional user prompt.
func NewDataForBootstrap(todoPath, schemaPath, workDir, userPrompt string, now time.Time) Data {
	return Data{
		TodoPath:   todoPath,
		SchemaPath: schemaPath,
		WorkDir:    workDir,
		UserPrompt: userPrompt,
		Now:        now.UTC().Format(time.RFC3339),
	}
}

// NewDataForRepair builds prompt data for repair flow with validation errors.
func NewDataForRepair(todoPath, schemaPath, workDir, errMsg string, now time.Time) Data {
	return Data{
		TodoPath:     todoPath,
		SchemaPath:   schemaPath,
		WorkDir:      workDir,
		ErrorMessage: errMsg,
		Now:          now.UTC().Format(time.RFC3339),
	}
}

// Renderer renders templates with strict missing-key behavior.
type Renderer struct {
	store *Store
}

// NewRenderer creates a prompt renderer.
func NewRenderer(store *Store) *Renderer {
	return &Renderer{store: store}
}

// Render loads and renders a prompt template with required variable checks.
func (r *Renderer) Render(name string, data Data) (string, error) {
	if r == nil || r.store == nil {
		return "", errors.New("prompt renderer is not initialized")
	}
	if err := validateRequired(name, data); err != nil {
		return "", err
	}
	raw, err := r.store.Load(name)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse prompt %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt %q: %w", name, err)
	}
	return buf.String(), nil
}

type requiredVar int

const (
	reqTodoPath requiredVar = iota
	reqSchemaPath
	reqWorkDir
	reqIteration
	reqSchedule
	reqNow
	reqTaskID
	reqTaskTitle
	reqTaskStatus
)

var requiredByPrompt = map[string][]requiredVar{
	BootstrapPrompt: {reqTodoPath, reqSchemaPath, reqWorkDir},
	RepairPrompt:    {reqTodoPath, reqSchemaPath},
	IterationPrompt: {reqTodoPath, reqSchemaPath, reqWorkDir, reqIteration, reqSchedule, reqNow, reqTaskID, reqTaskTitle, reqTaskStatus},
	ReviewPrompt:    {reqTodoPath, reqSchemaPath, reqWorkDir},
	PushPrompt:      {reqWorkDir, reqNow},
}

func validateRequired(name string, data Data) error {
	reqs, ok := requiredByPrompt[name]
	if !ok {
		return fmt.Errorf("unknown prompt %q", name)
	}
	for _, req := range reqs {
		switch req {
		case reqTodoPath:
			if data.TodoPath == "" {
				return fmt.Errorf("prompt %q requires TodoPath", name)
			}
		case reqSchemaPath:
			if data.SchemaPath == "" {
				return fmt.Errorf("prompt %q requires SchemaPath", name)
			}
		case reqWorkDir:
			if data.WorkDir == "" {
				return fmt.Errorf("prompt %q requires WorkDir", name)
			}
		case reqIteration:
			if data.Iteration <= 0 {
				return fmt.Errorf("prompt %q requires Iteration > 0", name)
			}
		case reqSchedule:
			if data.Schedule == "" {
				return fmt.Errorf("prompt %q requires Schedule", name)
			}
		case reqNow:
			if data.Now == "" {
				return fmt.Errorf("prompt %q requires Now", name)
			}
		case reqTaskID:
			if data.SelectedTask.ID == "" {
				return fmt.Errorf("prompt %q requires SelectedTask.ID", name)
			}
		case reqTaskTitle:
			if data.SelectedTask.Title == "" {
				return fmt.Errorf("prompt %q requires SelectedTask.Title", name)
			}
		case reqTaskStatus:
			if data.SelectedTask.Status == "" {
				return fmt.Errorf("prompt %q requires SelectedTask.Status", name)
			}
		default:
			return fmt.Errorf("prompt %q has unsupported requirement", name)
		}
	}
	return nil
}
