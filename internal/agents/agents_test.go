// Package agents tests for Codex and Claude runners.
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestExtractJSON tests the extractJSON function.
func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"task_id":"T1","status":"done"}`,
			expected: `{"task_id":"T1","status":"done"}`,
		},
		{
			name:     "JSON in markdown code block with json tag",
			input:    "```json\n{\"task_id\":\"T1\",\"status\":\"done\"}\n```",
			expected: `{"task_id":"T1","status":"done"}`,
		},
		{
			name:     "JSON in markdown code block without tag",
			input:    "```\n{\"task_id\":\"T1\",\"status\":\"done\"}\n```",
			expected: `{"task_id":"T1","status":"done"}`,
		},
		{
			name:     "JSON embedded in text",
			input:    "Some text here\n{\"task_id\":\"T1\",\"status\":\"done\"}\nMore text",
			expected: `{"task_id":"T1","status":"done"}`,
		},
		{
			name:     "no JSON",
			input:    "Just plain text with no JSON",
			expected: "",
		},
		{
			name:     "nested objects",
			input:    `{"outer":{"inner":"value"}}`,
			expected: `{"outer":{"inner":"value"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			if result != tt.expected {
				t.Errorf("extractJSON() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestIOStreamLogWriter tests the IOStreamLogWriter.
func TestIOStreamLogWriter(t *testing.T) {
	var buf bytes.Buffer
	writer := NewIOStreamLogWriter(&buf)

	event := LogEvent{
		Type:      "test",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Content:   "test message",
	}

	err := writer.Write(event)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Check that output is valid JSON
	var got LogEvent
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Failed to parse output as JSON: %v", err)
	}

	if got.Type != event.Type {
		t.Errorf("Type = %q, want %q", got.Type, event.Type)
	}
	if got.Content != event.Content {
		t.Errorf("Content = %q, want %q", got.Content, event.Content)
	}
}

// TestIOStreamLogWriterIndent tests the SetIndent method.
func TestIOStreamLogWriterIndent(t *testing.T) {
	var buf bytes.Buffer
	writer := NewIOStreamLogWriter(&buf)
	writer.SetIndent("  ")

	event := LogEvent{
		Type:      "test",
		Timestamp: time.Now().UTC(),
		Content:   "test message",
	}

	err := writer.Write(event)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "  ") {
		t.Errorf("Output does not start with indent: %q", output)
	}
}

// TestMultiLogWriter tests the MultiLogWriter.
func TestMultiLogWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	writer1 := NewIOStreamLogWriter(&buf1)
	writer2 := NewIOStreamLogWriter(&buf2)
	multi := NewMultiLogWriter(writer1, writer2)

	event := LogEvent{
		Type:      "test",
		Timestamp: time.Now().UTC(),
		Content:   "test message",
	}

	err := multi.Write(event)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Both writers should have received the event
	if buf1.Len() == 0 {
		t.Error("Writer 1 received no output")
	}
	if buf2.Len() == 0 {
		t.Error("Writer 2 received no output")
	}

	// Both should be valid JSON
	var got1, got2 LogEvent
	if err := json.Unmarshal(buf1.Bytes(), &got1); err != nil {
		t.Errorf("Failed to parse writer 1 output as JSON: %v", err)
	}
	if err := json.Unmarshal(buf2.Bytes(), &got2); err != nil {
		t.Errorf("Failed to parse writer 2 output as JSON: %v", err)
	}

	if got1.Content != event.Content {
		t.Errorf("Writer 1 Content = %q, want %q", got1.Content, event.Content)
	}
	if got2.Content != event.Content {
		t.Errorf("Writer 2 Content = %q, want %q", got2.Content, event.Content)
	}
}

// TestNullLogWriter tests the NullLogWriter.
func TestNullLogWriter(t *testing.T) {
	writer := NullLogWriter{}

	event := LogEvent{
		Type:      "test",
		Timestamp: time.Now().UTC(),
		Content:   "test message",
	}

	err := writer.Write(event)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

// TestValidateBinary tests the ValidateBinary function.
func TestValidateBinary(t *testing.T) {
	tmpDir := t.TempDir()
	if runtime.GOOS == "windows" {
		execPath := filepath.Join(tmpDir, "test_exec.exe")
		if err := os.WriteFile(execPath, []byte("fake exe"), 0644); err != nil {
			t.Fatalf("Failed to create test executable: %v", err)
		}

		if err := ValidateBinary(execPath); err != nil {
			t.Errorf("ValidateBinary() on valid executable error = %v", err)
		}

		if err := ValidateBinary(filepath.Join(tmpDir, "missing.exe")); err == nil {
			t.Error("ValidateBinary() on non-existent file expected error, got nil")
		}

		nonExecPath := filepath.Join(tmpDir, "non_exec.txt")
		if err := os.WriteFile(nonExecPath, []byte("not executable"), 0644); err != nil {
			t.Fatalf("Failed to create non-executable file: %v", err)
		}
		if err := ValidateBinary(nonExecPath); err == nil {
			t.Error("ValidateBinary() on non-executable file expected error, got nil")
		}
		return
	}

	execPath := filepath.Join(tmpDir, "test_exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test\n"), 0755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	if err := ValidateBinary(execPath); err != nil {
		t.Errorf("ValidateBinary() on valid executable error = %v", err)
	}

	if err := ValidateBinary("/nonexistent/path/to/binary"); err == nil {
		t.Error("ValidateBinary() on non-existent file expected error, got nil")
	}

	nonExecPath := filepath.Join(tmpDir, "non_exec")
	if err := os.WriteFile(nonExecPath, []byte("not executable"), 0644); err != nil {
		t.Fatalf("Failed to create non-executable file: %v", err)
	}

	if err := ValidateBinary(nonExecPath); err == nil {
		t.Error("ValidateBinary() on non-executable file expected error, got nil")
	}
}

// TestFindAgentBinary tests the FindAgentBinary function.
func TestFindAgentBinary(t *testing.T) {
	// Test with a binary that should exist (sh, ls, etc.)
	if path, err := FindAgentBinary(AgentTypeCodex); err != nil {
		// Codex might not be installed, that's ok
		t.Logf("Codex binary not found (expected if not installed): %v", err)
	} else if path == "" {
		t.Error("FindAgentBinary() returned empty path without error")
	}

	// Test with unknown agent type
	if _, err := FindAgentBinary("unknown"); err == nil {
		t.Error("FindAgentBinary() with unknown type expected error, got nil")
	}

	// Test with a shell built-in that exists
	if path, err := exec.LookPath("sh"); err != nil {
		t.Logf("Shell not found (unexpected): %v", err)
	} else if path == "" {
		t.Error("exec.LookPath() returned empty path without error")
	}
}

// TestSummary tests Summary marshaling/unmarshaling.
func TestSummary(t *testing.T) {
	summary := Summary{
		TaskID:   "T001",
		Status:   "done",
		Summary:  "Test completed successfully",
		Files:    []string{"file1.go", "file2.go"},
		Blockers: []string{"blocker1"},
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got Summary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.TaskID != summary.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, summary.TaskID)
	}
	if got.Status != summary.Status {
		t.Errorf("Status = %q, want %q", got.Status, summary.Status)
	}
	if got.Summary != summary.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, summary.Summary)
	}
	if len(got.Files) != len(summary.Files) {
		t.Errorf("Files length = %d, want %d", len(got.Files), len(summary.Files))
	}
}

// TestCodexAgentConfig tests creating a Codex agent with config.
func TestCodexAgentConfig(t *testing.T) {
	cfg := Config{
		Binary:  "codex",
		Model:   "gpt-5",
		Args:    []string{"--arg1", "value1"},
		Timeout: 5 * time.Minute,
		WorkDir: "/tmp",
	}

	agent := NewCodexAgent(cfg)
	if agent == nil {
		t.Fatal("NewCodexAgent() returned nil")
	}

	// Type assertion to check it's the right type
	if _, ok := agent.(*codexAgent); !ok {
		t.Error("NewCodexAgent() did not return *codexAgent")
	}
}

// TestClaudeAgentConfig tests creating a Claude agent with config.
func TestClaudeAgentConfig(t *testing.T) {
	cfg := Config{
		Binary:  "claude",
		Model:   "claude-4",
		Args:    []string{"--arg1", "value1"},
		Timeout: 10 * time.Minute,
		WorkDir: "/tmp",
	}

	agent := NewClaudeAgent(cfg)
	if agent == nil {
		t.Fatal("NewClaudeAgent() returned nil")
	}

	// Type assertion to check it's the right type
	if _, ok := agent.(*claudeAgent); !ok {
		t.Error("NewClaudeAgent() did not return *claudeAgent")
	}
}

// TestNewAgent tests the NewAgent factory function.
func TestNewAgent(t *testing.T) {
	cfg := Config{
		Binary:  "test",
		Timeout: DefaultTimeout,
	}

	tests := []struct {
		name      string
		agentType AgentType
		wantErr   bool
	}{
		{
			name:      "codex agent",
			agentType: AgentTypeCodex,
			wantErr:   false,
		},
		{
			name:      "claude agent",
			agentType: AgentTypeClaude,
			wantErr:   false,
		},
		{
			name:      "unknown agent type",
			agentType: "unknown",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := NewAgent(tt.agentType, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAgent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && agent == nil {
				t.Error("NewAgent() returned nil agent")
			}
		})
	}
}

// TestLogEventJSON tests LogEvent JSON marshaling.
func TestLogEventJSON(t *testing.T) {
	event := LogEvent{
		Type:      "assistant_message",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Content:   "test message",
		Tool:      "test_tool",
		Command:   []string{"echo", "test"},
		ExitCode:  0,
		Summary: &Summary{
			TaskID:  "T001",
			Status:  "done",
			Summary: "completed",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got LogEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Type != event.Type {
		t.Errorf("Type = %q, want %q", got.Type, event.Type)
	}
	if got.Content != event.Content {
		t.Errorf("Content = %q, want %q", got.Content, event.Content)
	}
	if got.Tool != event.Tool {
		t.Errorf("Tool = %q, want %q", got.Tool, event.Tool)
	}
	if got.Summary.TaskID != event.Summary.TaskID {
		t.Errorf("Summary.TaskID = %q, want %q", got.Summary.TaskID, event.Summary.TaskID)
	}
}

// stubAgentProcess creates a stub process that outputs JSON lines.
// This is used for integration testing.
func stubAgentProcess(t *testing.T, output string) *exec.Cmd {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "stub.sh")

	// Create a stub script that outputs the given content
	script := "#!/bin/sh\necho '" + output + "'\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create stub script: %v", err)
	}

	return exec.Command(scriptPath)
}

// TestCodexAgentTimeout tests that the codex agent respects timeout.
func TestCodexAgentTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	t.Skip("timeout test requires actual binary or more complex stub")

	// This would need a stub that sleeps longer than the timeout
	// For now, we skip this test
}

// TestProcessLine tests the processLine method for codex agent.
func TestProcessLine(t *testing.T) {
	agent := NewCodexAgent(Config{}).(*codexAgent)

	tests := []struct {
		name        string
		line        string
		wantSummary bool
	}{
		{
			name:        "plain text",
			line:        "just some text",
			wantSummary: false,
		},
		{
			name:        "valid JSON with task_id",
			line:        `{"task_id":"T001","status":"done","summary":"test"}`,
			wantSummary: true,
		},
		{
			name:        "valid JSON with summary field",
			line:        `{"summary":"test output"}`,
			wantSummary: true,
		},
		{
			name:        "assistant message with summary JSON",
			line:        `{"type":"assistant_message","message":{"content":[{"type":"text","text":"{\"task_id\":\"T1\",\"status\":\"done\"}"}]}}`,
			wantSummary: true,
		},
		{
			name:        "JSON without summary",
			line:        `{"type":"tool_use","tool":"test"}`,
			wantSummary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logWriter := NewIOStreamLogWriter(&buf)
			summaries := make(chan *Summary, 10)
			ctx := context.Background()

			err := agent.processLine(ctx, tt.line, logWriter, summaries)
			if err != nil {
				t.Errorf("processLine() error = %v", err)
			}

			// Check if summary was sent
			select {
			case summary := <-summaries:
				if !tt.wantSummary {
					t.Errorf("Unexpected summary received: %+v", summary)
				}
			default:
				if tt.wantSummary {
					t.Error("Expected summary but none received")
				}
			}
		})
	}
}

func TestClaudeProcessStreamJSONSummaryFromDelta(t *testing.T) {
	agent := NewClaudeAgent(Config{}).(*claudeAgent)
	stream := strings.Join([]string{
		`{"type":"content_block_start","content_block":{"type":"text","text":"{\"task_id\":\"T1\","}}`,
		`{"type":"content_block_delta","delta":{"text":"\"status\":\"done\",\"summary\":\"ok\"}"}}`,
	}, "\n")

	summaries := make(chan *Summary, 1)
	lastMsg, err := agent.processStreamJSON(context.Background(), strings.NewReader(stream), NullLogWriter{}, summaries)
	if err != nil {
		t.Fatalf("processStreamJSON() error = %v", err)
	}

	// Verify we got a last message (the accumulated text)
	if lastMsg == "" {
		t.Error("processStreamJSON() returned empty last message")
	}

	select {
	case summary := <-summaries:
		if summary.Status != "done" {
			t.Fatalf("summary.Status = %q, want %q", summary.Status, "done")
		}
	default:
		t.Fatal("Expected summary but none received")
	}
}

func TestCodexAgentRunReadsPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "stub.sh")
	script := "#!/bin/sh\ninput=$(cat)\nif [ -z \"$input\" ]; then\n  echo '{\"task_id\":\"T1\",\"status\":\"blocked\",\"summary\":\"missing prompt\"}'\nelse\n  echo '{\"task_id\":\"T1\",\"status\":\"done\",\"summary\":\"got prompt\"}'\nfi\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create stub script: %v", err)
	}

	agent := NewCodexAgent(Config{
		Binary:  scriptPath,
		Timeout: 5 * time.Second,
	})

	summary, err := agent.Run(context.Background(), "hello", NullLogWriter{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if summary.Status != "done" {
		t.Fatalf("summary.Status = %q, want %q", summary.Status, "done")
	}
}

// TestValidateSummaryMinimal tests the minimal summary validation (without schema).
func TestValidateSummaryMinimal(t *testing.T) {
	tests := []struct {
		name    string
		summary *Summary
		wantErr bool
	}{
		{
			name: "valid done summary",
			summary: &Summary{
				TaskID:  "T001",
				Status:  "done",
				Summary: "Task completed",
			},
			wantErr: false,
		},
		{
			name: "valid blocked summary",
			summary: &Summary{
				TaskID:   "T001",
				Status:   "blocked",
				Summary:  "Blocked by dependency",
				Blockers: []string{"waiting for T002"},
			},
			wantErr: false,
		},
		{
			name: "valid skipped summary",
			summary: &Summary{
				TaskID:  "T001",
				Status:  "skipped",
				Summary: "Task not applicable",
			},
			wantErr: false,
		},
		{
			name: "summary with files",
			summary: &Summary{
				TaskID:  "T001",
				Status:  "done",
				Summary: "Completed",
				Files:   []string{"file1.go", "file2.go"},
			},
			wantErr: false,
		},
		{
			name: "minimal valid summary - just status",
			summary: &Summary{
				Status: "done",
			},
			wantErr: false,
		},
		{
			name: "invalid status",
			summary: &Summary{
				TaskID:  "T001",
				Status:  "invalid_status",
				Summary: "Test",
			},
			wantErr: true,
		},
		{
			name:    "empty summary",
			summary: &Summary{},
			wantErr: true,
		},
		{
			name:    "nil summary",
			summary: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSummary(tt.summary, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSummary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateSummaryWithSchema tests summary validation with actual schema file.
func TestValidateSummaryWithSchema(t *testing.T) {
	// Create a temporary schema file
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "summary.schema.json")
	schemaContent := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title": "Test Summary Schema",
		"type": "object",
		"additionalProperties": false,
		"required": ["task_id", "status"],
		"properties": {
			"task_id": { "type": ["string", "null"] },
			"status": { "type": "string", "enum": ["done", "blocked", "skipped"] },
			"summary": { "type": "string" },
			"files": { "type": "array", "items": { "type": "string" } },
			"blockers": { "type": "array", "items": { "type": "string" } }
		}
	}`

	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("Failed to create schema file: %v", err)
	}

	tests := []struct {
		name    string
		summary *Summary
		wantErr bool
	}{
		{
			name: "valid summary with all fields",
			summary: &Summary{
				TaskID:   "T001",
				Status:   "done",
				Summary:  "Completed successfully",
				Files:    []string{"file1.go"},
				Blockers: []string{},
			},
			wantErr: false,
		},
		{
			name: "valid minimal summary",
			summary: &Summary{
				TaskID: "T001",
				Status: "blocked",
			},
			wantErr: false,
		},
		{
			name: "empty task_id passes schema validation",
			summary: &Summary{
				TaskID: "", // Empty string is valid per schema ["string", "null"]
				Status: "done",
			},
			wantErr: false, // Schema allows empty string as valid string
		},
		{
			name: "missing required status",
			summary: &Summary{
				TaskID: "T001",
			},
			wantErr: true,
		},
		{
			name: "invalid status value",
			summary: &Summary{
				TaskID: "T001",
				Status: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSummary(tt.summary, schemaPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSummary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && err != nil {
				// Check if it's a validation error with good formatting
				var valErr *SummaryValidationError
				if !errors.As(err, &valErr) {
					t.Errorf("Expected SummaryValidationError, got %T", err)
				}
			}
		})
	}
}

// TestSummaryValidationError tests the SummaryValidationError type.
func TestSummaryValidationError(t *testing.T) {
	err := &SummaryValidationError{
		Path:    "status",
		Message: "invalid value",
	}

	expected := "summary validation failed at status: invalid value"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}

	// Test without path
	err2 := &SummaryValidationError{
		Message: "general error",
	}

	expected2 := "summary validation failed: general error"
	if err2.Error() != expected2 {
		t.Errorf("Error() without path = %q, want %q", err2.Error(), expected2)
	}
}

// TestAgentRegistry tests the agent registry functionality.
func TestAgentRegistry(t *testing.T) {
	// Save the original registry to restore it after the test
	originalRegistry := make(map[AgentType]AgentFactory)
	for k, v := range Registry {
		originalRegistry[k] = v
	}
	defer func() {
		// Clear and restore the registry
		for k := range Registry {
			delete(Registry, k)
		}
		for k, v := range originalRegistry {
			Registry[k] = v
		}
	}()

	// Test that built-in agents are registered
	t.Run("builtin agents registered", func(t *testing.T) {
		if !IsAgentTypeRegistered("codex") {
			t.Error("codex agent should be registered")
		}
		if !IsAgentTypeRegistered("claude") {
			t.Error("claude agent should be registered")
		}
	})

	// Test registering a custom agent
	t.Run("register custom agent", func(t *testing.T) {
		customType := AgentType("custom")
		if IsAgentTypeRegistered(string(customType)) {
			t.Error("custom agent should not be registered yet")
		}

		// Register a custom agent factory
		factoryCalled := false
		RegisterAgent(customType, func(cfg Config) (Agent, error) {
			factoryCalled = true
			return nil, nil
		})

		if !IsAgentTypeRegistered(string(customType)) {
			t.Error("custom agent should be registered after RegisterAgent call")
		}

		// Create an agent using the registry
		cfg := Config{}
		agent, err := NewAgent(customType, cfg)
		if err != nil {
			t.Errorf("NewAgent() error = %v", err)
		}
		if agent != nil {
			t.Error("factory should return nil agent")
		}
		if !factoryCalled {
			t.Error("factory was not called")
		}
	})

	// Test that RegisteredAgentTypes includes all registered types
	t.Run("registered agent types", func(t *testing.T) {
		types := RegisteredAgentTypes()

		// Should at least include codex and claude
		hasCodex := false
		hasClaude := false
		hasCustom := false
		for _, t := range types {
			if t == "codex" {
				hasCodex = true
			}
			if t == "claude" {
				hasClaude = true
			}
			if t == "custom" {
				hasCustom = true
			}
		}
		if !hasCodex {
			t.Error("RegisteredAgentTypes() should include codex")
		}
		if !hasClaude {
			t.Error("RegisteredAgentTypes() should include claude")
		}
		if !hasCustom {
			t.Error("RegisteredAgentTypes() should include custom")
		}
	})

	// Test NewAgent with unregistered type
	t.Run("NewAgent with unregistered type", func(t *testing.T) {
		_, err := NewAgent("unregistered", Config{})
		if err == nil {
			t.Error("NewAgent() with unregistered type should return error")
		}
		// Error message should mention the registered types
		if !strings.Contains(err.Error(), "unknown agent type") {
			t.Errorf("Error message should mention unknown agent type, got: %v", err)
		}
	})
}

// TestFindAgentBinaryWithCustomType tests FindAgentBinary with custom agent types.
func TestFindAgentBinaryWithCustomType(t *testing.T) {
	// Test with built-in types
	t.Run("built-in types use default binary names", func(t *testing.T) {
		if _, err := FindAgentBinary(AgentTypeCodex); err != nil {
			// Codex might not be installed, that's ok
			t.Logf("Codex binary not found (expected if not installed): %v", err)
		}

		if _, err := FindAgentBinary(AgentTypeClaude); err != nil {
			// Claude might not be installed, that's ok
			t.Logf("Claude binary not found (expected if not installed): %v", err)
		}
	})

	// Test with custom agent type (should use the type name as binary name)
	t.Run("custom type uses type name as binary", func(t *testing.T) {
		customType := AgentType("nonexistent-custom-agent")
		_, err := FindAgentBinary(customType)
		if err == nil {
			t.Error("FindAgentBinary() with nonexistent custom binary should return error")
		}
		// Error should mention the custom binary name
		if !strings.Contains(err.Error(), string(customType)) {
			t.Errorf("Error should mention the custom binary name, got: %v", err)
		}
	})
}

// TestStreamStderr tests the shared streamStderr helper.
func TestStreamStderr(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantEvents int
	}{
		{
			name:       "single error line",
			stderr:     "error: something went wrong",
			wantEvents: 1,
		},
		{
			name:       "multiple error lines",
			stderr:     "error: line 1\nerror: line 2\nerror: line 3",
			wantEvents: 3,
		},
		{
			name:       "blank lines are ignored",
			stderr:     "error: line 1\n  \nerror: line 2",
			wantEvents: 2,
		},
		{
			name:       "empty input",
			stderr:     "",
			wantEvents: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var buf bytes.Buffer
			logWriter := NewIOStreamLogWriter(&buf)
			errs := make(chan error, 10)

			go streamStderr(ctx, strings.NewReader(tt.stderr), logWriter, errs)
			time.Sleep(10 * time.Millisecond) // Give goroutine time to finish

			// Count logged events
			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			eventCount := 0
			for _, line := range lines {
				if line == "" {
					continue
				}
				var event LogEvent
				if err := json.Unmarshal([]byte(line), &event); err == nil && event.Type == "error" {
					eventCount++
				}
			}

			if eventCount != tt.wantEvents {
				t.Errorf("got %d error events, want %d", eventCount, tt.wantEvents)
			}
		})
	}
}

// TestNewScanner tests the newScanner helper.
func TestNewScanner(t *testing.T) {
	input := "line1\nline2\nline3\n"
	scanner := newScanner(strings.NewReader(input))

	lines := []string{}
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}

	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestLogRawEvent(t *testing.T) {
	tests := []struct {
		name          string
		raw           map[string]any
		line          string
		assistantText string
		wantType      string
		wantContent   string
		wantTool      string
	}{
		{
			name:          "assistant message prefers assistant text",
			raw:           map[string]any{"type": "assistant_message"},
			line:          `{"type":"assistant_message"}`,
			assistantText: "hello",
			wantType:      "assistant_message",
			wantContent:   "hello",
		},
		{
			name:        "assistant message falls back to line",
			raw:         map[string]any{"type": "assistant_message"},
			line:        "raw line",
			wantType:    "assistant_message",
			wantContent: "raw line",
		},
		{
			name:          "tool events use line content",
			raw:           map[string]any{"type": "tool_use", "name": "tester"},
			line:          `{"type":"tool_use","name":"tester"}`,
			assistantText: "ignored",
			wantType:      "tool",
			wantContent:   `{"type":"tool_use","name":"tester"}`,
			wantTool:      "tester",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logWriter := NewIOStreamLogWriter(&buf)

			if err := logRawEvent(logWriter, tt.raw, tt.line, tt.assistantText); err != nil {
				t.Fatalf("logRawEvent() error = %v", err)
			}

			var event LogEvent
			if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &event); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if event.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", event.Type, tt.wantType)
			}
			if event.Content != tt.wantContent {
				t.Errorf("Content = %q, want %q", event.Content, tt.wantContent)
			}
			if event.Tool != tt.wantTool {
				t.Errorf("Tool = %q, want %q", event.Tool, tt.wantTool)
			}
		})
	}
}
