// Package looperdir provides constants and utilities for the .looper directory structure.
package looperdir

import "path/filepath"

const (
	// Dir is the name of the looper state directory.
	Dir = ".looper"

	// DefaultTodoFile is the default todo file name (inside .looper).
	DefaultTodoFile = "todo.json"

	// DefaultSchemaFile is the default schema file name (inside .looper).
	DefaultSchemaFile = "todo.schema.json"

	// DefaultConfigFile is the default config file name (inside .looper).
	DefaultConfigFile = "looper.toml"
)

// TodoPath returns the full path to the todo file within a work directory.
func TodoPath(workDir string) string {
	return joinPath(workDir, DefaultTodoFile)
}

// SchemaPath returns the full path to the schema file within a work directory.
func SchemaPath(workDir string) string {
	return joinPath(workDir, DefaultSchemaFile)
}

// ConfigPath returns the full path to the config file within a work directory.
func ConfigPath(workDir string) string {
	return joinPath(workDir, DefaultConfigFile)
}

// DirPath returns the full path to the .looper directory within a work directory.
func DirPath(workDir string) string {
	if workDir == "." || workDir == "" {
		return Dir
	}
	return workDir + string(filepath.Separator) + Dir
}

func joinPath(workDir, file string) string {
	if workDir == "." || workDir == "" {
		return Dir + string(filepath.Separator) + file
	}
	return workDir + string(filepath.Separator) + Dir + string(filepath.Separator) + file
}
