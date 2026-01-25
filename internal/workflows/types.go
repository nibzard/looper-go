// Package workflows provides a pluggable loop execution system.
// The loop itself becomes a workflow, just like agents are pluggable.
package workflows

import (
	"context"
	"fmt"
)

// WorkflowType identifies a workflow type.
type WorkflowType string

// WorkflowFactory creates a Workflow from configuration.
type WorkflowFactory func(cfg interface{}, workDir string, todoFile interface{}) (Workflow, error)

// Workflow defines the entire loop execution strategy.
// Different workflows can have completely different execution mechanics.
type Workflow interface {
	// Run executes the entire workflow until completion.
	Run(context.Context) error

	// Description returns a human-readable description of this workflow.
	Description() string
}

// Config is the base configuration for workflows.
// Each workflow can define its own config schema.
type Config struct {
	// Settings holds workflow-specific configuration.
	// TOML section [workflows.<name>] maps to this.
	Settings map[string]any
}

// Helper functions for reading workflow config settings

// GetString retrieves a string value from settings with a default.
func GetString(settings map[string]any, key string, defaultVal string) string {
	if settings == nil {
		return defaultVal
	}
	val, ok := settings[key]
	if !ok {
		return defaultVal
	}
	if s, ok := val.(string); ok {
		return s
	}
	return defaultVal
}

// GetInt retrieves an int value from settings with a default.
func GetInt(settings map[string]any, key string, defaultVal int) int {
	if settings == nil {
		return defaultVal
	}
	val, ok := settings[key]
	if !ok {
		return defaultVal
	}
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return defaultVal
}

// GetBool retrieves a bool value from settings with a default.
func GetBool(settings map[string]any, key string, defaultVal bool) bool {
	if settings == nil {
		return defaultVal
	}
	val, ok := settings[key]
	if !ok {
		return defaultVal
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return defaultVal
}

// GetStringSlice retrieves a string slice from settings with a default.
func GetStringSlice(settings map[string]any, key string, defaultVal []string) []string {
	if settings == nil {
		return defaultVal
	}
	val, ok := settings[key]
	if !ok {
		return defaultVal
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return defaultVal
}

// GetDuration retrieves a duration string from settings and parses it.
func GetDuration(settings map[string]any, key string, defaultVal string) string {
	return GetString(settings, key, defaultVal)
}

// WorkflowNotFoundError is returned when an unknown workflow type is requested.
type WorkflowNotFoundError struct {
	WorkflowType WorkflowType
}

func (e *WorkflowNotFoundError) Error() string {
	return fmt.Sprintf("workflow %q not found", e.WorkflowType)
}

// IsWorkflowNotFound returns true if the error is a WorkflowNotFoundError.
func IsWorkflowNotFound(err error) bool {
	_, ok := err.(*WorkflowNotFoundError)
	return ok
}
