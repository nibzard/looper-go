// Package logging writes JSONL logs and tail output.
package logging

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// FindLogDir finds the log directory for a given work directory.
func FindLogDir(baseDir, workDir string) (string, error) {
	if baseDir == "" {
		return "", fmt.Errorf("log base dir is empty")
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

	return logDir, nil
}

// FindLatestLog finds the latest JSONL log file in a directory.
func FindLatestLog(logDir string) (string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read log dir: %w", err)
	}

	var latest string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Join(logDir, name)
		}
	}

	return latest, nil
}

// TailLog tails a log file to a writer, optionally following.
func TailLog(w io.Writer, path string, n int, follow bool) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	// If n > 0, seek to show only last n lines
	if n > 0 {
		if err := tailSeek(file, n); err != nil {
			return fmt.Errorf("seek to tail position: %w", err)
		}
	}

	if follow {
		return tailFollow(w, file)
	}

	// Just dump the rest of the file
	_, err = io.Copy(w, file)
	return err
}

// tailSeek seeks to a position that shows approximately the last n lines.
func tailSeek(file *os.File, n int) error {
	const avgLineLength = 100

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	size := stat.Size()
	if size < avgLineLength*int64(n) {
		// File is small enough, just read from start
		_, err = file.Seek(0, io.SeekStart)
		return err
	}

	// Seek back from end
	offset := size - int64(n*avgLineLength)
	if offset < 0 {
		offset = 0
	}
	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}

	// Discard partial first line
	buf := make([]byte, 1)
	_, err = file.Read(buf)
	if err != nil {
		return err
	}
	for {
		if buf[0] == '\n' {
			break
		}
		_, err := file.Read(buf)
		if err != nil {
			break
		}
	}

	return nil
}

// tailFollow follows a file like tail -f.
func tailFollow(w io.Writer, file *os.File) error {
	// First, copy existing content
	if _, err := io.Copy(w, file); err != nil {
		return err
	}

	// Then follow for new content
	for {
		_, err := io.Copy(w, file)
		if err != nil {
			return err
		}

		// Wait briefly before checking for more data
		time.Sleep(100 * time.Millisecond)

		// Check if more data is available
		var buf [1]byte
		_, err = file.Read(buf[:])
		if err != nil {
			if err == io.EOF {
				continue
			}
			return err
		}
		// We read a byte, write it and continue copying
		if _, err := w.Write(buf[:]); err != nil {
			return err
		}
	}
}

// LogRun represents a single log run with its associated files.
type LogRun struct {
	RunID            string
	ModTime          time.Time
	Files            []string
	LastMessageFiles []string
}

// FindLogRuns finds all log runs in a directory, grouped by run ID.
func FindLogRuns(logDir string) ([]LogRun, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("read log dir: %w", err)
	}

	// Group files by run ID
	runMap := make(map[string]*LogRun)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Extract run ID from filename
		// Format: <timestamp>-<pid>.jsonl or <timestamp>-<pid>-<label>.last.json
		runID, isLast := extractRunID(name)
		if runID == "" {
			continue
		}

		fullPath := filepath.Join(logDir, name)

		if _, exists := runMap[runID]; !exists {
			runMap[runID] = &LogRun{
				RunID:            runID,
				ModTime:          info.ModTime(),
				Files:            []string{},
				LastMessageFiles: []string{},
			}
		}

		run := runMap[runID]

		// Update mod time if this file is newer
		if info.ModTime().After(run.ModTime) {
			run.ModTime = info.ModTime()
		}

		if isLast {
			run.LastMessageFiles = append(run.LastMessageFiles, fullPath)
		} else {
			run.Files = append(run.Files, fullPath)
		}
	}

	// Convert to slice and sort by mod time descending
	runs := make([]LogRun, 0, len(runMap))
	for _, run := range runMap {
		runs = append(runs, *run)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].ModTime.After(runs[j].ModTime)
	})

	return runs, nil
}

// extractRunID extracts the run ID from a log filename.
// Returns the run ID and whether this is a last message file.
func extractRunID(filename string) (string, bool) {
	// Check for .jsonl files (main log files)
	if strings.HasSuffix(filename, ".jsonl") {
		base := strings.TrimSuffix(filename, ".jsonl")
		// Run ID format: timestamp-pid
		// We just use the whole base as the run ID
		return base, false
	}

	// Check for .last.json files (last message files)
	if strings.HasSuffix(filename, ".last.json") {
		base := strings.TrimSuffix(filename, ".last.json")
		// Format: timestamp-time-pid-label (e.g., 20060102-150405-12345-run)
		// Extract timestamp-time-pid part (first three hyphen-separated parts)
		parts := strings.Split(base, "-")
		if len(parts) >= 3 {
			// Rejoin the first three parts (timestamp, time, and pid)
			runID := strings.Join(parts[:3], "-")
			return runID, true
		}
	}

	return "", false
}
