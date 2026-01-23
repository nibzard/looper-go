// Package agents provides tests for console logging.
package agents

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

// TestConsoleLogWriter_Write tests the Write method with various log event types.
func TestConsoleLogWriter_Write(t *testing.T) {
	tests := []struct {
		name       string
		event      LogEvent
		wantLevel  string
		wantMsg    string
		wantFields []string
	}{
		{
			name: "error event",
			event: LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   "something went wrong",
			},
			wantLevel: "ERRO",
			wantMsg:   "something went wrong",
		},
		{
			name: "command event",
			event: LogEvent{
				Type:      "command",
				Timestamp: time.Now().UTC(),
				Command:   []string{"git", "status"},
			},
			wantLevel:  "INFO",
			wantMsg:    "Running command",
			wantFields: []string{"command"},
		},
		{
			name: "summary event with task completed",
			event: LogEvent{
				Type:      "summary",
				Timestamp: time.Now().UTC(),
				Summary: &Summary{
					TaskID:  "T001",
					Status:  "done",
					Summary: "completed the task",
				},
			},
			wantLevel:  "INFO",
			wantMsg:    "Task completed",
			wantFields: []string{"task_id", "status", "summary"},
		},
		{
			name: "tool event",
			event: LogEvent{
				Type:      "tool",
				Timestamp: time.Now().UTC(),
				Tool:      "bash",
			},
			wantLevel:  "DEBU",
			wantMsg:    "Using tool",
			wantFields: []string{"tool"},
		},
		{
			name: "assistant_message event",
			event: LogEvent{
				Type:      "assistant_message",
				Timestamp: time.Now().UTC(),
				Content:   "Thinking about the problem...",
			},
			wantLevel: "DEBU",
			wantMsg:   "Thinking about the problem...",
		},
		{
			name: "command with exit code",
			event: LogEvent{
				Type:      "command",
				Timestamp: time.Now().UTC(),
				Command:   []string{"false"},
				ExitCode:  1,
			},
			wantLevel:  "INFO",
			wantMsg:    "Running command",
			wantFields: []string{"command", "exit_code"},
		},
		{
			name: "unknown event type",
			event: LogEvent{
				Type:      "unknown_type",
				Timestamp: time.Now().UTC(),
				Content:   "some content",
			},
			wantLevel: "DEBU",
			wantMsg:   "some content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := NewTestConsoleLogWriter(&buf)

			err := writer.Write(tt.event)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			output := buf.String()

			// Check that the log level is present
			if !strings.Contains(output, tt.wantLevel) {
				t.Errorf("Expected output to contain level %q, got: %s", tt.wantLevel, output)
			}

			// Check that the message is present
			if !strings.Contains(output, tt.wantMsg) {
				t.Errorf("Expected output to contain message %q, got: %s", tt.wantMsg, output)
			}

			// Check for expected fields
			for _, field := range tt.wantFields {
				if !strings.Contains(output, field) {
					t.Errorf("Expected output to contain field %q, got: %s", field, output)
				}
			}
		})
	}
}

// TestConsoleLogWriter_extractFields tests the extractFields method.
func TestConsoleLogWriter_extractFields(t *testing.T) {
	tests := []struct {
		name       string
		event      LogEvent
		wantFields map[string]string
	}{
		{
			name: "tool field",
			event: LogEvent{
				Type:      "tool",
				Timestamp: time.Now().UTC(),
				Tool:      "bash",
			},
			wantFields: map[string]string{"tool": "bash"},
		},
		{
			name: "command field",
			event: LogEvent{
				Type:      "command",
				Timestamp: time.Now().UTC(),
				Command:   []string{"ls", "-la"},
			},
			wantFields: map[string]string{"command": "[ls -la]"},
		},
		{
			name: "exit code field",
			event: LogEvent{
				Type:      "command",
				Timestamp: time.Now().UTC(),
				ExitCode:  1,
			},
			wantFields: map[string]string{"exit_code": "1"},
		},
		{
			name: "summary fields",
			event: LogEvent{
				Type:      "summary",
				Timestamp: time.Now().UTC(),
				Summary: &Summary{
					TaskID:  "T001",
					Status:  "done",
					Summary: "All done",
				},
			},
			wantFields: map[string]string{
				"task_id": "T001",
				"status":  "done",
				"summary": "All done",
			},
		},
		{
			name: "no fields",
			event: LogEvent{
				Type:      "assistant_message",
				Timestamp: time.Now().UTC(),
				Content:   "hello",
			},
			wantFields: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := NewConsoleLogWriter(ConsoleLogOptions{})
			fields := writer.extractFields(tt.event)

			// Convert fields slice to map for easier comparison
			fieldMap := make(map[string]string)
			for i := 0; i < len(fields); i += 2 {
				if i+1 < len(fields) {
					key, ok := fields[i].(string)
					if !ok {
						continue
					}
					var val string
					switch v := fields[i+1].(type) {
					case string:
						val = v
					case []string:
						val = "[" + strings.Join(v, " ") + "]"
					case int:
						val = fmt.Sprintf("%d", v)
					}
					fieldMap[key] = val
				}
			}

			for key, wantVal := range tt.wantFields {
				gotVal, ok := fieldMap[key]
				if !ok {
					t.Errorf("extractFields() missing field %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("extractFields() field %q = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

// TestFormatMessage tests the formatMessage function.
func TestFormatMessage(t *testing.T) {
	tests := []struct {
		name string
		event LogEvent
		want string
	}{
		{
			name: "content takes precedence",
			event: LogEvent{
				Type:    "command",
				Content: "custom message",
			},
			want: "custom message",
		},
		{
			name: "command with command array",
			event: LogEvent{
				Type:    "command",
				Command: []string{"git", "status"},
			},
			want: "Running command",
		},
		{
			name: "command without command array",
			event: LogEvent{
				Type: "command",
			},
			want: "Command",
		},
		{
			name: "summary with done status",
			event: LogEvent{
				Type: "summary",
				Summary: &Summary{
					Status: "done",
				},
			},
			want: "Task completed",
		},
		{
			name: "summary without done status",
			event: LogEvent{
				Type: "summary",
				Summary: &Summary{
					Status: "blocked",
				},
			},
			want: "Summary received",
		},
		{
			name: "summary nil",
			event: LogEvent{
				Type:    "summary",
				Summary: nil,
			},
			want: "Summary",
		},
		{
			name: "tool with tool name",
			event: LogEvent{
				Type: "tool",
				Tool: "bash",
			},
			want: "Using tool",
		},
		{
			name: "tool without tool name",
			event: LogEvent{
				Type: "tool",
			},
			want: "Tool",
		},
		{
			name: "error",
			event: LogEvent{
				Type: "error",
			},
			want: "Error",
		},
		{
			name: "assistant_message",
			event: LogEvent{
				Type: "assistant_message",
			},
			want: "Assistant message",
		},
		{
			name: "unknown type",
			event: LogEvent{
				Type: "unknown_type",
			},
			want: "unknown_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMessage(tt.event)
			if got != tt.want {
				t.Errorf("formatMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseLogLevel tests the parseLogLevel function.
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name string
		level string
		want log.Level
	}{
		{"debug", "debug", log.DebugLevel},
		{"info", "info", log.InfoLevel},
		{"warn", "warn", log.WarnLevel},
		{"warning", "warning", log.WarnLevel},
		{"error", "error", log.ErrorLevel},
		{"fatal", "fatal", log.FatalLevel},
		{"unknown defaults to info", "unknown", log.InfoLevel},
		{"empty defaults to info", "", log.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLogLevel(tt.level)
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

// TestParseLogFormatter tests the parseLogFormatter function.
func TestParseLogFormatter(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   log.Formatter
	}{
		{"json", "json", log.JSONFormatter},
		{"logfmt", "logfmt", log.LogfmtFormatter},
		{"text", "text", log.TextFormatter},
		{"unknown defaults to text", "unknown", log.TextFormatter},
		{"empty defaults to text", "", log.TextFormatter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLogFormatter(tt.format)
			if got != tt.want {
				t.Errorf("parseLogFormatter(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

// TestNewConsoleLogWriterFromConfig tests creation from config strings.
func TestNewConsoleLogWriterFromConfig(t *testing.T) {
	writer := NewConsoleLogWriterFromConfig("debug", "json", true, false, "test")
	if writer == nil {
		t.Fatal("NewConsoleLogWriterFromConfig() returned nil")
	}
	if writer.logger == nil {
		t.Fatal("NewConsoleLogWriterFromConfig() logger is nil")
	}
}

// TestDefaultConsoleLogOptions tests the default options.
func TestDefaultConsoleLogOptions(t *testing.T) {
	opts := DefaultConsoleLogOptions()

	if opts.Level != log.InfoLevel {
		t.Errorf("DefaultConsoleLogOptions() Level = %v, want %v", opts.Level, log.InfoLevel)
	}
	if opts.Formatter != log.TextFormatter {
		t.Errorf("DefaultConsoleLogOptions() Formatter = %v, want %v", opts.Formatter, log.TextFormatter)
	}
	if opts.ReportTimestamp {
		t.Error("DefaultConsoleLogOptions() ReportTimestamp = true, want false")
	}
	if opts.ReportCaller {
		t.Error("DefaultConsoleLogOptions() ReportCaller = true, want false")
	}
	if opts.Prefix != "looper" {
		t.Errorf("DefaultConsoleLogOptions() Prefix = %q, want \"looper\"", opts.Prefix)
	}
}

// TestConsoleLogWriter_LevelMapping tests that log event types map to correct levels.
func TestConsoleLogWriter_LevelMapping(t *testing.T) {
	tests := []struct {
		name           string
		eventType      string
		wantContains   string
		dontWantContains []string
	}{
		{
			name:         "error maps to ERROR",
			eventType:    "error",
			wantContains: "ERRO",
		},
		{
			name:         "command maps to INFO",
			eventType:    "command",
			wantContains: "INFO",
		},
		{
			name:         "summary maps to INFO",
			eventType:    "summary",
			wantContains: "INFO",
		},
		{
			name:         "tool maps to DEBUG",
			eventType:    "tool",
			wantContains: "DEBU",
		},
		{
			name:         "assistant_message maps to DEBUG",
			eventType:    "assistant_message",
			wantContains: "DEBU",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := NewTestConsoleLogWriter(&buf)

			event := LogEvent{
				Type:      tt.eventType,
				Timestamp: time.Now().UTC(),
				Content:   "test message",
			}

			_ = writer.Write(event)
			output := buf.String()

			if !strings.Contains(output, tt.wantContains) {
				t.Errorf("Expected output to contain %q for event type %q, got: %s", tt.wantContains, tt.eventType, output)
			}
		})
	}
}

// TestNewConsoleLogWriterWithLogger tests creating a writer with a custom logger.
func TestNewConsoleLogWriterWithLogger(t *testing.T) {
	customLogger := log.NewWithOptions(os.Stderr, log.Options{
		Level: log.DebugLevel,
	})
	writer := NewConsoleLogWriterWithLogger(customLogger)

	if writer == nil {
		t.Fatal("NewConsoleLogWriterWithLogger() returned nil")
	}
	if writer.logger != customLogger {
		t.Error("NewConsoleLogWriterWithLogger() did not use the provided logger")
	}
}
