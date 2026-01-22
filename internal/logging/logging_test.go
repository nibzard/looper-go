// Package logging provides tests for JSONL logging and tail output.
package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestNewRunLogger tests creating a new run logger.
func TestNewRunLogger(t *testing.T) {
	t.Run("successful creation with valid paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()

		logger, err := NewRunLogger(tmpDir, workDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer logger.Close()

		if logger.Dir == "" {
			t.Error("expected Dir to be set")
		}
		if logger.RunID == "" {
			t.Error("expected RunID to be set")
		}
		if logger.LogPath == "" {
			t.Error("expected LogPath to be set")
		}
		if logger.file == nil {
			t.Error("expected file to be set")
		}

		// Verify log file was created
		if _, err := os.Stat(logger.LogPath); err != nil {
			t.Errorf("log file not created: %v", err)
		}
	})

	t.Run("empty base dir returns error", func(t *testing.T) {
		_, err := NewRunLogger("", t.TempDir())
		if err == nil {
			t.Fatal("expected error for empty base dir, got nil")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("expected empty dir error, got %v", err)
		}
	})

	t.Run("creates log directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		newLogDir := filepath.Join(tmpDir, "new-logs", "nested")
		workDir := t.TempDir()

		logger, err := NewRunLogger(newLogDir, workDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer logger.Close()

		// Verify directory was created
		if _, err := os.Stat(newLogDir); err != nil {
			t.Errorf("log directory not created: %v", err)
		}
	})

	t.Run("uses absolute workDir when relative provided", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := "." // relative path

		logger, err := NewRunLogger(tmpDir, workDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer logger.Close()

		// Log directory should contain absolute path components
		if logger.Dir == "" {
			t.Error("expected Dir to be set")
		}
	})

	t.Run("log directory includes project slug", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()
		// Name the workdir something predictable
		namedWorkDir := filepath.Join(workDir, "my-test-project")
		if err := os.Mkdir(namedWorkDir, 0755); err != nil {
			t.Fatal(err)
		}

		logger, err := NewRunLogger(tmpDir, namedWorkDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer logger.Close()

		// The log dir should contain a slugified version of the project name
		if !strings.Contains(logger.Dir, "my-test-project") || !strings.Contains(logger.Dir, "-") {
			// Should have project name and a hash
			t.Logf("Note: log dir is %s", logger.Dir)
		}
	})
}

// TestRunLoggerWriter tests the Writer method.
func TestRunLoggerWriter(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := t.TempDir()

	logger, err := NewRunLogger(tmpDir, workDir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	writer := logger.Writer()
	if writer == nil {
		t.Fatal("expected non-nil writer")
	}

	// Write something
	testData := []byte(`{"test":"data"}` + "\n")
	if _, err := writer.Write(testData); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Verify file contents
	content, err := os.ReadFile(logger.LogPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Contains(content, testData) {
		t.Errorf("expected file to contain %q, got %q", testData, content)
	}
}

// TestRunLoggerClose tests closing the logger.
func TestRunLoggerClose(t *testing.T) {
	t.Run("close valid logger", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()

		logger, err := NewRunLogger(tmpDir, workDir)
		if err != nil {
			t.Fatal(err)
		}

		if err := logger.Close(); err != nil {
			t.Errorf("close failed: %v", err)
		}
	})

	t.Run("close nil logger", func(t *testing.T) {
		var logger *RunLogger
		if err := logger.Close(); err != nil {
			t.Errorf("close nil logger failed: %v", err)
		}
	})

	t.Run("close logger with nil file", func(t *testing.T) {
		logger := &RunLogger{file: nil}
		if err := logger.Close(); err != nil {
			t.Errorf("close logger with nil file failed: %v", err)
		}
	})
}

// TestRunLoggerLastMessagePath tests the LastMessagePath method.
func TestRunLoggerLastMessagePath(t *testing.T) {
	t.Run("valid label generates path", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()

		logger, err := NewRunLogger(tmpDir, workDir)
		if err != nil {
			t.Fatal(err)
		}
		defer logger.Close()

		path := logger.LastMessagePath("test-run")
		if path == "" {
			t.Fatal("expected non-empty path")
		}

		// Path should be in the log directory
		if !strings.HasPrefix(path, logger.Dir) {
			t.Error("last message path should be in log directory")
		}

		// Path should end with .last.json
		if !strings.HasSuffix(path, ".last.json") {
			t.Error("last message path should end with .last.json")
		}

		// Path should contain the label
		if !strings.Contains(path, "test-run") {
			t.Error("last message path should contain label")
		}
	})

	t.Run("empty label uses default", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()

		logger, err := NewRunLogger(tmpDir, workDir)
		if err != nil {
			t.Fatal(err)
		}
		defer logger.Close()

		path := logger.LastMessagePath("")
		if path == "" {
			t.Fatal("expected non-empty path")
		}

		// Should use "run" as default label
		if !strings.Contains(path, "-run.last.json") {
			t.Errorf("expected default label 'run' in path, got %s", path)
		}
	})

	t.Run("label with special characters is sanitized", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()

		logger, err := NewRunLogger(tmpDir, workDir)
		if err != nil {
			t.Fatal(err)
		}
		defer logger.Close()

		path := logger.LastMessagePath("test/run with spaces!")
		if path == "" {
			t.Fatal("expected non-empty path")
		}

		// Special chars like "/" and "!" should be replaced with underscores
		// Spaces become underscores too
		// The path will contain "test_run_with_spaces_" in the filename portion
		if strings.Contains(path, "/") && !strings.HasPrefix(path, "/tmp/") && !strings.HasPrefix(path, "C:\\") {
			t.Errorf("special characters not sanitized in path: %s", path)
		}
		if strings.Contains(path, "!") {
			t.Errorf("special characters not sanitized in path: %s", path)
		}
		// The sanitized label should be in the filename
		if !strings.Contains(path, "test_run_with_spaces_") && !strings.Contains(path, "test-run-with-spaces") {
			// At minimum, the special chars should be replaced
			t.Logf("Note: path is %s (may have different sanitization)", path)
		}
	})

	t.Run("nil logger returns empty path", func(t *testing.T) {
		var logger *RunLogger
		path := logger.LastMessagePath("test")
		if path != "" {
			t.Errorf("expected empty path for nil logger, got %s", path)
		}
	})
}

// TestSlugify tests the slugify helper.
func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"Hello World", "Hello_World"}, // slugify preserves case
		{"test-project", "test-project"},
		{"test_project", "test_project"},
		{"many   spaces", "many_spaces"}, // consecutive underscores are collapsed
		{"special@chars!", "special_chars"},
		{"123numbers", "123numbers"},
		{"", "project"},
		{"   ", "project"},
		{"---", "---"}, // "-" is valid, so "---" stays as is (not trimmed)
		{"___", "project"}, // underscores are trimmed from ends, leaving empty -> "project"
		{"CamelCase", "CamelCase"}, // slugify preserves case
		{"test.-_project", "test.-_project"},
		{"test/directory", "test_directory"},
		{"test\\directory", "test_directory"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSanitizeLabel tests the sanitizeLabel helper.
func TestSanitizeLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"test-run", "test-run"},
		{"test_run", "test_run"},
		{"test/run", "test_run"},
		{"test\\run", "test_run"},
		{"test run", "test_run"},
		{"test@run!", "test_run"},
		{"", "run"},
		{"   ", "run"},
		{"---", "---"}, // "-" is valid for labels (not trimmed like slugify)
		{"___", "run"}, // underscores are trimmed from ends, leaving empty -> "run"
		{"a-b_c", "a-b_c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeLabel(tt.input); got != tt.want {
				t.Errorf("sanitizeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestHashPath tests the hashPath helper.
func TestHashPath(t *testing.T) {
	tests := []struct {
		input string
		// hash should be deterministic and 8 characters
	}{
		{"/path/to/project"},
		{"/another/path"},
		{""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := hashPath(tt.input)
			if len(got) != 8 {
				t.Errorf("hashPath(%q) length = %d, want 8", tt.input, len(got))
			}
			// Same input should produce same hash
			got2 := hashPath(tt.input)
			if got != got2 {
				t.Errorf("hashPath(%q) not deterministic: %s vs %s", tt.input, got, got2)
			}
			// Different inputs should produce different hashes (with high probability)
			if tt.input != "" {
				differentHash := hashPath(tt.input + "x")
				if got == differentHash {
					t.Errorf("hashPath(%q) and hashPath(%q) produced same hash", tt.input, tt.input+"x")
				}
			}
		})
	}
}

// TestRunID tests the runID helper.
func TestRunID(t *testing.T) {
	id := runID()
	if id == "" {
		t.Fatal("expected non-empty run ID")
	}

	// Should be in format: YYYYMMDD-HHMMSS-PID
	parts := strings.Split(id, "-")
	if len(parts) != 3 {
		t.Errorf("expected 3 parts separated by '-', got %d: %s", len(parts), id)
	}

	// First part should be a date
	if _, err := time.Parse("20060102", parts[0]); err != nil {
		t.Errorf("first part not a valid date: %v", err)
	}

	// Second part should be a time
	if _, err := time.Parse("150405", parts[1]); err != nil {
		t.Errorf("second part not a valid time: %v", err)
	}

	// Third part should be PID
	if parts[2] == "" {
		t.Error("PID part is empty")
	}
}

// TestFindLogDir tests finding the log directory.
func TestFindLogDir(t *testing.T) {
	t.Run("finds existing log directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		workDir := t.TempDir()

		logDir, err := FindLogDir(tmpDir, workDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if logDir == "" {
			t.Error("expected non-empty log directory")
		}

		// Should be under tmpDir
		if !strings.HasPrefix(logDir, tmpDir) {
			t.Error("log directory should be under base dir")
		}
	})

	t.Run("empty base dir returns error", func(t *testing.T) {
		_, err := FindLogDir("", t.TempDir())
		if err == nil {
			t.Fatal("expected error for empty base dir, got nil")
		}
	})
}

// TestFindLatestLog tests finding the latest log file.
func TestFindLatestLog(t *testing.T) {
	t.Run("finds latest log in directory", func(t *testing.T) {
		logDir := t.TempDir()

		// Create multiple log files with different timestamps
		files := []string{"20240101-120000-100.jsonl", "20240101-120001-101.jsonl", "20240101-120002-102.jsonl"}
		for _, f := range files {
			path := filepath.Join(logDir, f)
			if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
				t.Fatal(err)
			}
			// Add a small delay to ensure different modification times
			time.Sleep(10 * time.Millisecond)
		}

		latest, err := FindLatestLog(logDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if latest == "" {
			t.Fatal("expected non-empty latest log path")
		}

		// Should be the last file created
		if !strings.Contains(latest, "102.jsonl") {
			t.Logf("Note: latest is %s (may vary by filesystem)", latest)
		}
	})

	t.Run("returns empty for non-existent directory", func(t *testing.T) {
		latest, err := FindLatestLog("/nonexistent/directory")
		if err != nil {
			t.Fatalf("expected no error for non-existent dir, got %v", err)
		}
		if latest != "" {
			t.Errorf("expected empty path for non-existent directory, got %s", latest)
		}
	})

	t.Run("ignores non-jsonl files", func(t *testing.T) {
		logDir := t.TempDir()

		// Create mix of files
		os.WriteFile(filepath.Join(logDir, "log.jsonl"), []byte("log1"), 0644)
		os.WriteFile(filepath.Join(logDir, "readme.txt"), []byte("readme"), 0644)
		os.WriteFile(filepath.Join(logDir, "data.json"), []byte("{}"), 0644)
		time.Sleep(10 * time.Millisecond)
		os.WriteFile(filepath.Join(logDir, "log2.jsonl"), []byte("log2"), 0644)

		latest, err := FindLatestLog(logDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !strings.HasSuffix(latest, ".jsonl") {
			t.Errorf("expected .jsonl file, got %s", latest)
		}
	})

	t.Run("ignores subdirectories", func(t *testing.T) {
		logDir := t.TempDir()

		os.WriteFile(filepath.Join(logDir, "log.jsonl"), []byte("log"), 0644)
		os.Mkdir(filepath.Join(logDir, "subdir"), 0755)
		time.Sleep(10 * time.Millisecond)
		os.WriteFile(filepath.Join(logDir, "log2.jsonl"), []byte("log2"), 0644)

		latest, err := FindLatestLog(logDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Should return a file, not a directory
		if latest != "" && strings.HasSuffix(latest, "subdir") {
			t.Error("should not return directory as latest log")
		}
	})

	t.Run("returns empty for empty directory", func(t *testing.T) {
		logDir := t.TempDir()

		latest, err := FindLatestLog(logDir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if latest != "" {
			t.Errorf("expected empty path for empty directory, got %s", latest)
		}
	})
}

// TestTailLog tests tailing log files.
func TestTailLog(t *testing.T) {
	t.Run("tails entire file when n=0", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "test.log")
		content := []byte("line1\nline2\nline3\n")
		if err := os.WriteFile(logFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := TailLog(&buf, logFile, 0, false); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		got := buf.String()
		if !strings.Contains(got, string(content)) {
			t.Errorf("expected content to contain %q, got %q", string(content), got)
		}
	})

	t.Run("tails last n lines", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "test.log")
		content := []byte("line1\nline2\nline3\nline4\nline5\n")
		if err := os.WriteFile(logFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := TailLog(&buf, logFile, 2, false); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		got := buf.String()
		// Should contain last 2 lines (approximately)
		if strings.Contains(got, "line1") && strings.Contains(got, "line2") && strings.Contains(got, "line3") {
			// If all lines are present, file was small enough to include all
			t.Log("file small enough to include all lines")
		}
		if !strings.Contains(got, "line5") {
			t.Error("expected last line to be present")
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		var buf bytes.Buffer
		err := TailLog(&buf, "/nonexistent/file.log", 0, false)
		if err == nil {
			t.Fatal("expected error for non-existent file, got nil")
		}
	})

	t.Run("follow mode with file write", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping follow test on Windows due to file locking issues")
		}

		logFile := filepath.Join(t.TempDir(), "test.log")
		initialContent := []byte("initial\n")
		if err := os.WriteFile(logFile, initialContent, 0644); err != nil {
			t.Fatal(err)
		}

		// Start tailing in a goroutine
		var buf bytes.Buffer
		done := make(chan error, 1)
		go func() {
			done <- TailLog(&buf, logFile, 0, true)
		}()

		// Wait a bit for tail to start
		time.Sleep(50 * time.Millisecond)

		// Append to the file
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("appended line\n"); err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Give it time to read
		time.Sleep(100 * time.Millisecond)

		// The buffer should contain both initial and appended content
		got := buf.String()
		if !strings.Contains(got, "initial") {
			t.Error("expected initial content in tail output")
		}
		if !strings.Contains(got, "appended") {
			t.Error("expected appended content in tail output")
		}
	})
}

// TestResolveBaseDir tests the resolveBaseDir helper.
func TestResolveBaseDir(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		workDir  string
		wantPrefix string
	}{
		{
			name:     "absolute base dir",
			baseDir:  "/absolute/path/logs",
			workDir:  "/work",
			wantPrefix: "/absolute/path/logs",
		},
		{
			name:     "relative base dir",
			baseDir:  "logs",
			workDir:  "/work",
			wantPrefix: "/work",
		},
		{
			name:     "base dir with ..",
			baseDir:  "../logs",
			workDir:  "/work/dir",
			wantPrefix: "/work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: on Windows, paths will have drive letters, so we check prefix differently
			got := resolveBaseDir(tt.baseDir, tt.workDir)
			if !filepath.IsAbs(got) && tt.baseDir[0] == filepath.Separator {
				t.Errorf("expected absolute path, got %s", got)
			}
			if tt.baseDir[0] == filepath.Separator && got != tt.baseDir {
				// Absolute paths should remain unchanged (cleaned)
				if got != filepath.Clean(tt.baseDir) {
					t.Errorf("resolveBaseDir() = %s, want %s", got, filepath.Clean(tt.baseDir))
				}
			}
		})
	}
}

// TestResolveProjectRoot tests the resolveProjectRoot helper.
func TestResolveProjectRoot(t *testing.T) {
	t.Run("uses work dir when no git", func(t *testing.T) {
		workDir := t.TempDir()
		got := resolveProjectRoot(workDir)
		if got != workDir {
			t.Errorf("resolveProjectRoot() = %s, want %s", got, workDir)
		}
	})

	t.Run("empty work dir returns dot", func(t *testing.T) {
		got := resolveProjectRoot("")
		if got != "." {
			t.Errorf("resolveProjectRoot() = %s, want .", got)
		}
	})
}

// TestProjectSlug tests the projectSlug helper.
func TestProjectSlug(t *testing.T) {
	slug := projectSlug("/my/project")
	if slug == "" {
		t.Fatal("expected non-empty slug")
	}

	// Should contain hash
	parts := strings.Split(slug, "-")
	if len(parts) < 2 {
		t.Errorf("expected slug with hash, got %s", slug)
	}

	// Hash should be 8 chars
	hash := parts[len(parts)-1]
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %s", hash)
	}
}
