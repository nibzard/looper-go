// Package parsers provides a plugin-based system for parsing agent output.
// It supports external Python/JavaScript parsers and bundled parsers.
package parsers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Summary represents the parsed summary from agent output.
// This is a simplified version of the agents.Summary type to avoid import cycles.
type Summary struct {
	TaskID   string   `json:"task_id"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Files    []string `json:"files,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

// Parser extracts a summary from raw agent output.
type Parser interface {
	// Parse extracts a summary from raw agent output.
	// Returns the summary, an error if parsing failed, or ErrSummaryMissing if no summary found.
	Parse(ctx context.Context, output string) (*Summary, error)
}

// ParserFactory creates a Parser from a config string.
type ParserFactory func(config string) (Parser, error)

// Registry holds registered parser types and their factories.
var Registry = map[string]ParserFactory{
	"python": newPythonParser,
	"js":     newJSParser,
	"builtin": func(config string) (Parser, error) {
		// Built-in parsers are referenced by name
		if config == "" {
			return nil, fmt.Errorf("builtin parser name required")
		}
		return &BuiltinParser{name: config}, nil
	},
}

// BuiltinParser uses the built-in Go parsing logic (no external script).
type BuiltinParser struct {
	name string
}

func (p *BuiltinParser) Parse(ctx context.Context, output string) (*Summary, error) {
	// Built-in parsing delegates to the existing Go logic
	// This is handled at a higher level by returning the raw output
	return nil, fmt.Errorf("builtin parser %q: use built-in Go parsing", p.name)
}

// ErrSummaryMissing indicates the parser completed without finding a summary.
// This is distinct from a parsing error - it means the output was valid but contained no summary.
var ErrSummaryMissing = fmt.Errorf("no summary found in output")

// ParserConfig represents a parser configuration from the TOML file.
// Examples:
//   - "claude_parser.py"      -> Python parser
//   - "parsers/codex.js"      -> JavaScript parser
//   - "~/.looper/parsers/custom.py" -> User-level Python parser
//   - "builtin:claude"        -> Built-in Go parser
type ParserConfig string

// Parser creates a Parser from the config.
// It searches for parser scripts in:
// 1. Absolute path (if starts with / or ~/)
// 2. ./looper-parsers/ (project-level)
// 3. ~/.looper/parsers/ (user-level)
// 4. Bundled parsers
func (pc ParserConfig) Parser(workDir string) (Parser, error) {
	config := string(pc)
	if config == "" {
		return nil, nil // No parser configured
	}

	// Check for builtin: prefix
	if strings.HasPrefix(config, "builtin:") {
		name := strings.TrimPrefix(config, "builtin:")
		if name == "" {
			return nil, fmt.Errorf("builtin parser name required after builtin:")
		}
		return Registry["builtin"](name)
	}

	// Expand ~ prefix
	if strings.HasPrefix(config, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("expand home directory: %w", err)
		}
		config = filepath.Join(home, config[2:])
	}

	// Absolute path or already expanded
	if filepath.IsAbs(config) {
		return createParserFromPath(config)
	}

	// Check project-level parsers
	projectPath := filepath.Join(workDir, "looper-parsers", config)
	if info, err := os.Stat(projectPath); err == nil && !info.IsDir() {
		return createParserFromPath(projectPath)
	}

	// Check user-level parsers
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".looper", "parsers", config)
		if info, err := os.Stat(userPath); err == nil && !info.IsDir() {
			return createParserFromPath(userPath)
		}
	}

	// Check bundled parsers
	if bundled := getBundledParserPath(config); bundled != "" {
		return Registry["builtin"](config)
	}

	// Not found - treat as relative path
	return createParserFromPath(config)
}

func createParserFromPath(path string) (Parser, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py":
		return Registry["python"](path)
	case ".js":
		return Registry["js"](path)
	default:
		return nil, fmt.Errorf("unsupported parser extension: %s (supported: .py, .js)", ext)
	}
}

// getBundledParserPath checks if a parser name refers to a bundled parser.
func getBundledParserPath(name string) string {
	bundledParsers := map[string]bool{
		"claude_parser.py": true,
		"codex_parser.py":  true,
		"opencode_parser.py": true,
	}
	if bundledParsers[name] {
		return name
	}
	return ""
}

// ParseJSONSummary is a helper for parsers that return JSON summaries.
func ParseJSONSummary(output string) (*Summary, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, ErrSummaryMissing
	}

	var summary Summary
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		return nil, fmt.Errorf("unmarshal summary: %w", err)
	}

	if !summaryHasContent(summary) {
		return nil, ErrSummaryMissing
	}

	return &summary, nil
}

func summaryHasContent(summary Summary) bool {
	return summary.TaskID != "" ||
		summary.Status != "" ||
		summary.Summary != "" ||
		len(summary.Files) > 0 ||
		len(summary.Blockers) > 0
}

// ParserResult represents the result of parsing agent output.
type ParserResult struct {
	Summary *Summary
	Error   error
}

// ParseWithParser uses a parser to extract a summary from agent output.
// If parser is nil, returns ErrSummaryMissing to indicate fallback to built-in parsing.
func ParseWithParser(ctx context.Context, parser Parser, output string) (*Summary, error) {
	if parser == nil {
		return nil, ErrSummaryMissing
	}
	return parser.Parse(ctx, output)
}
