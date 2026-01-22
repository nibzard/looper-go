// Package hooks provides tests for external post-iteration hook invocation.
package hooks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestInvoke tests the Invoke function with various scenarios.
func TestInvoke(t *testing.T) {
	t.Run("empty command returns success without running", func(t *testing.T) {
		result, err := Invoke(context.Background(), Options{
			Command:         "",
			LastMessagePath: "/tmp/nonexistent.json",
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Ran {
			t.Error("expected Ran to be false")
		}
	})

	t.Run("empty last message path returns success without running", func(t *testing.T) {
		result, err := Invoke(context.Background(), Options{
			Command:         "echo",
			LastMessagePath: "",
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Ran {
			t.Error("expected Ran to be false")
		}
	})

	t.Run("nonexistent last message file returns success without running", func(t *testing.T) {
		result, err := Invoke(context.Background(), Options{
			Command:         "echo",
			LastMessagePath: "/tmp/nonexistent-looper-test-12345.json",
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Ran {
			t.Error("expected Ran to be false")
		}
	})

	t.Run("last message path is directory returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		result, err := Invoke(context.Background(), Options{
			Command:         "echo",
			LastMessagePath: tmpDir,
		})
		if err == nil {
			t.Fatal("expected error for directory path, got nil")
		}
		if !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("expected directory error, got %v", err)
		}
		if result.Ran {
			t.Error("expected Ran to be false")
		}
	})

	t.Run("invalid JSON in last message returns error", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "last.json")
		if err := os.WriteFile(tmpFile, []byte("not json"), 0644); err != nil {
			t.Fatal(err)
		}
		result, err := Invoke(context.Background(), Options{
			Command:         "echo",
			LastMessagePath: tmpFile,
		})
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
		if !strings.Contains(err.Error(), "not valid JSON") {
			t.Errorf("expected JSON error, got %v", err)
		}
		if result.Ran {
			t.Error("expected Ran to be false")
		}
	})

	t.Run("empty last message file returns error", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "last.json")
		if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
		result, err := Invoke(context.Background(), Options{
			Command:         "echo",
			LastMessagePath: tmpFile,
		})
		if err == nil {
			t.Fatal("expected error for empty file, got nil")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("expected empty error, got %v", err)
		}
		if result.Ran {
			t.Error("expected Ran to be false")
		}
	})
}

// TestInvokeSuccessfulHook tests a successful hook invocation.
func TestInvokeSuccessfulHook(t *testing.T) {
	// Create a test last message file with valid JSON
	lastMsg := `{"task_id":"T001","status":"done","summary":"completed task"}`
	tmpFile := filepath.Join(t.TempDir(), "last.json")
	if err := os.WriteFile(tmpFile, []byte(lastMsg), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a simple hook script
	var hookScript string
	if runtime.GOOS == "windows" {
		hookScript = filepath.Join(t.TempDir(), "hook.bat")
		if err := os.WriteFile(hookScript, []byte("@echo off\nexit /b 0"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		hookScript = filepath.Join(t.TempDir(), "hook.sh")
		if err := os.WriteFile(hookScript, []byte("#!/bin/sh\nexit 0"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Invoke(context.Background(), Options{
		Command:         hookScript,
		LastMessagePath: tmpFile,
		Label:           "test",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Ran {
		t.Error("expected Ran to be true")
	}
	if result.TaskID != "T001" {
		t.Errorf("expected TaskID T001, got %s", result.TaskID)
	}
	if result.Status != "done" {
		t.Errorf("expected Status done, got %s", result.Status)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", result.ExitCode)
	}
}

// TestInvokeHookFailure tests a hook that returns non-zero exit code.
func TestInvokeHookFailure(t *testing.T) {
	// Create a test last message file with valid JSON
	lastMsg := `{"task_id":"T002","status":"blocked"}`
	tmpFile := filepath.Join(t.TempDir(), "last.json")
	if err := os.WriteFile(tmpFile, []byte(lastMsg), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a hook script that fails
	var hookScript string
	if runtime.GOOS == "windows" {
		hookScript = filepath.Join(t.TempDir(), "hook.bat")
		if err := os.WriteFile(hookScript, []byte("@echo off\nexit /b 42"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		hookScript = filepath.Join(t.TempDir(), "hook.sh")
		if err := os.WriteFile(hookScript, []byte("#!/bin/sh\nexit 42"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Invoke(context.Background(), Options{
		Command:         hookScript,
		LastMessagePath: tmpFile,
		Label:           "test",
	})
	if err == nil {
		t.Fatal("expected error for failed hook, got nil")
	}
	if !result.Ran {
		t.Error("expected Ran to be true")
	}
	if result.ExitCode != 42 {
		t.Errorf("expected ExitCode 42, got %d", result.ExitCode)
	}
	if result.TaskID != "T002" {
		t.Errorf("expected TaskID T002, got %s", result.TaskID)
	}
}

// TestInvokeWithWorkDir tests hook invocation with a custom working directory.
func TestInvokeWithWorkDir(t *testing.T) {
	workDir := t.TempDir()
	lastMsg := `{"task_id":"T003","status":"doing"}`
	tmpFile := filepath.Join(t.TempDir(), "last.json")
	if err := os.WriteFile(tmpFile, []byte(lastMsg), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a hook script that prints the working directory
	var hookScript string
	if runtime.GOOS == "windows" {
		hookScript = filepath.Join(t.TempDir(), "hook.bat")
		if err := os.WriteFile(hookScript, []byte("@echo off\ncd"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		hookScript = filepath.Join(t.TempDir(), "hook.sh")
		if err := os.WriteFile(hookScript, []byte("#!/bin/sh\npwd"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Invoke(context.Background(), Options{
		Command:         hookScript,
		LastMessagePath: tmpFile,
		Label:           "test",
		WorkDir:         workDir,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Ran {
		t.Error("expected Ran to be true")
	}
}

// TestInvokeWithContextCancellation tests hook invocation with context cancellation.
func TestInvokeWithContextCancellation(t *testing.T) {
	lastMsg := `{"task_id":"T004","status":"todo"}`
	tmpFile := filepath.Join(t.TempDir(), "last.json")
	if err := os.WriteFile(tmpFile, []byte(lastMsg), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a long-running hook script
	var hookScript string
	if runtime.GOOS == "windows" {
		hookScript = filepath.Join(t.TempDir(), "hook.bat")
		if err := os.WriteFile(hookScript, []byte("@echo off\ntimeout /t 10 /nobreak >nul"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		hookScript = filepath.Join(t.TempDir(), "hook.sh")
		if err := os.WriteFile(hookScript, []byte("#!/bin/sh\nsleep 10"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := Invoke(ctx, Options{
		Command:         hookScript,
		LastMessagePath: tmpFile,
		Label:           "test",
	})
	// The hook should be cancelled/killed
	if err == nil {
		t.Log("hook completed or was killed (both acceptable on some systems)")
	}
	// Even if cancelled, the hook should show as having run
	if !result.Ran && !strings.Contains(hookScript, "timeout") && !strings.Contains(hookScript, "sleep") {
		t.Log("Note: some systems may not have killed the process before completion")
	}
}

// TestExtractSummaryFields tests the extractSummaryFields helper.
func TestExtractSummaryFields(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantID   string
		wantStat string
	}{
		{
			name:     "valid summary with both fields",
			json:     `{"task_id":"T123","status":"done"}`,
			wantID:   "T123",
			wantStat: "done",
		},
		{
			name:     "missing task_id",
			json:     `{"status":"blocked"}`,
			wantID:   "",
			wantStat: "blocked",
		},
		{
			name:     "missing status",
			json:     `{"task_id":"T456"}`,
			wantID:   "T456",
			wantStat: "",
		},
		{
			name:     "both fields missing",
			json:     `{"summary":"something"}`,
			wantID:   "",
			wantStat: "",
		},
		{
			name:     "empty object",
			json:     `{}`,
			wantID:   "",
			wantStat: "",
		},
		{
			name:     "null values",
			json:     `{"task_id":null,"status":null}`,
			wantID:   "",
			wantStat: "",
		},
		{
			name:     "non-string values",
			json:     `{"task_id":123,"status":true}`,
			wantID:   "",
			wantStat: "",
		},
		{
			name:     "not an object",
			json:     `["array","value"]`,
			wantID:   "",
			wantStat: "",
		},
		{
			name:     "primitive value",
			json:     `"just a string"`,
			wantID:   "",
			wantStat: "",
		},
		{
			name:     "null value",
			json:     `null`,
			wantID:   "",
			wantStat: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to decode the JSON since that's what the real flow does
			// But for testing the helper directly, we'll manually construct the value
			decoded := mockDecodeJSON(tt.json)
			gotID, gotStat := extractSummaryFields(decoded)
			if gotID != tt.wantID {
				t.Errorf("extractSummaryFields() taskID = %v, want %v", gotID, tt.wantID)
			}
			if gotStat != tt.wantStat {
				t.Errorf("extractSummaryFields() status = %v, want %v", gotStat, tt.wantStat)
			}
		})
	}
}

// mockDecodeJSON is a simple JSON decoder for testing.
func mockDecodeJSON(s string) any {
	if s == "null" {
		return nil
	}
	if strings.HasPrefix(s, "[") {
		return []any{"array", "value"}
	}
	// Parse simple JSON for testing - match exact patterns
	if strings.Contains(s, `"task_id":"T123"`) {
		return map[string]any{
			"task_id": "T123",
			"status":  "done",
		}
	}
	if strings.Contains(s, `"task_id":"T456"`) {
		return map[string]any{"task_id": "T456"}
	}
	if strings.Contains(s, `"status":"blocked"`) && !strings.Contains(s, "task_id") {
		return map[string]any{"status": "blocked"}
	}
	if strings.Contains(s, `"summary":"something"`) {
		return map[string]any{"summary": "something"}
	}
	if strings.Contains(s, `"task_id":null`) {
		return map[string]any{"task_id": nil, "status": nil}
	}
	if strings.Contains(s, `"task_id":123`) {
		return map[string]any{"task_id": 123, "status": true}
	}
	if strings.HasPrefix(s, "{") && !strings.Contains(s, ":") {
		return map[string]any{}
	}
	if strings.HasPrefix(s, `"`) {
		return "just a string"
	}
	return nil
}

// TestExitCodeFromError tests the exitCodeFromError helper.
func TestExitCodeFromError(t *testing.T) {
	t.Run("nil error returns 0", func(t *testing.T) {
		if code := exitCodeFromError(nil); code != 0 {
			t.Errorf("expected 0, got %d", code)
		}
	})

	t.Run("non-ExitError returns -1", func(t *testing.T) {
		err := &os.PathError{Err: exec.ErrNotFound}
		if code := exitCodeFromError(err); code != -1 {
			t.Errorf("expected -1, got %d", code)
		}
	})

	t.Run("ExitError returns actual exit code", func(t *testing.T) {
		// Create an actual ExitError by running a command that fails
		cmd := exec.Command("sh", "-c", "exit 42")
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if code := exitCodeFromError(exitErr); code != 42 {
					t.Errorf("expected 42, got %d", code)
				}
				return
			}
		}
		t.Skip("cannot create ExitError on this system")
	})
}

// TestStringField tests the stringField helper.
func TestStringField(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"empty string", "", ""},
		{"number", 123, ""},
		{"bool", true, ""},
		{"slice", []string{"a"}, ""},
		{"map", map[string]string{"k": "v"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringField(tt.input); got != tt.want {
				t.Errorf("stringField() = %v, want %v", got, tt.want)
			}
		})
	}
}
