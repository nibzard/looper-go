// Package logging writes JSONL logs and tail output.
package logging

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunLogger manages per-run log files and last-message paths.
type RunLogger struct {
	Dir     string
	RunID   string
	LogPath string
	file    *os.File
}

// NewRunLogger creates a per-run log directory and JSONL file.
func NewRunLogger(baseDir, workDir string) (*RunLogger, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("log base dir is empty")
	}

	resolvedWorkDir := workDir
	if resolvedWorkDir == "" {
		resolvedWorkDir = "."
	}
	if abs, err := filepath.Abs(resolvedWorkDir); err == nil {
		resolvedWorkDir = abs
	}

	baseDir = resolveBaseDir(baseDir, resolvedWorkDir)
	projectRoot := resolveProjectRoot(resolvedWorkDir)
	logDir := filepath.Join(baseDir, projectSlug(projectRoot))

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	runID := runID()
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.jsonl", runID))
	file, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	return &RunLogger{
		Dir:     logDir,
		RunID:   runID,
		LogPath: logPath,
		file:    file,
	}, nil
}

// Writer returns the underlying log file writer.
func (r *RunLogger) Writer() *os.File {
	return r.file
}

// Close closes the log file.
func (r *RunLogger) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}

// LastMessagePath returns the path for the last-message JSON file.
func (r *RunLogger) LastMessagePath(label string) string {
	if r == nil {
		return ""
	}
	safeLabel := sanitizeLabel(label)
	return filepath.Join(r.Dir, fmt.Sprintf("%s-%s.last.json", r.RunID, safeLabel))
}

func resolveBaseDir(baseDir, workDir string) string {
	if filepath.IsAbs(baseDir) {
		return filepath.Clean(baseDir)
	}
	return filepath.Clean(filepath.Join(workDir, baseDir))
}

func resolveProjectRoot(workDir string) string {
	if workDir == "" {
		return "."
	}
	if _, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command("git", "-C", workDir, "rev-parse", "--show-toplevel")
		if output, err := cmd.Output(); err == nil {
			root := strings.TrimSpace(string(output))
			if root != "" {
				return root
			}
		}
	}
	return workDir
}

func projectSlug(projectRoot string) string {
	name := filepath.Base(projectRoot)
	slug := slugify(name)
	hash := hashPath(projectRoot)
	return fmt.Sprintf("%s-%s", slug, hash)
}

func slugify(input string) string {
	if strings.TrimSpace(input) == "" {
		return "project"
	}

	var b strings.Builder
	lastUnderscore := false
	for i := 0; i < len(input); i++ {
		c := input[i]
		valid := (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-'
		if !valid {
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		b.WriteByte(c)
		lastUnderscore = false
	}

	slug := strings.Trim(b.String(), "_")
	if slug == "" {
		return "project"
	}
	return slug
}

func sanitizeLabel(input string) string {
	if strings.TrimSpace(input) == "" {
		return "run"
	}

	var b strings.Builder
	for i := 0; i < len(input); i++ {
		c := input[i]
		valid := (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-'
		if !valid {
			b.WriteByte('_')
			continue
		}
		b.WriteByte(c)
	}

	label := strings.Trim(b.String(), "_")
	if label == "" {
		return "run"
	}
	return label
}

func hashPath(input string) string {
	sum := sha1.Sum([]byte(input))
	return hex.EncodeToString(sum[:])[:8]
}

func runID() string {
	return fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102-150405"), os.Getpid())
}
