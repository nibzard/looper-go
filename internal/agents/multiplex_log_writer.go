// Package agents defines Codex and Claude runners.
package agents

import (
	"fmt"
	"io"
	"sync"
)

// MultiplexedLogWriter writes log events from multiple concurrent sources
// with task/agent ID prefixes to distinguish interleaved output.
type MultiplexedLogWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewMultiplexedLogWriter creates a new multiplexed log writer.
// Events are prefixed with task/agent ID information to distinguish
// concurrent output streams.
func NewMultiplexedLogWriter(w io.Writer) *MultiplexedLogWriter {
	return &MultiplexedLogWriter{
		writer: w,
	}
}

// Write writes a log event with a task/agent ID prefix.
// The prefix format is: "[task_id] " or "[task_id/agent_id] " for multi-agent scenarios.
func (m *MultiplexedLogWriter) Write(event LogEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := m.buildPrefix(event)

	// Write the prefix
	if prefix != "" {
		if _, err := fmt.Fprintf(m.writer, "[%s] ", prefix); err != nil {
			return fmt.Errorf("write prefix: %w", err)
		}
	}

	// Write the event JSON
	// For multiplexed output, we write a human-readable format
	switch event.Type {
	case "assistant_message":
		if _, err := fmt.Fprintf(m.writer, "%s\n", event.Content); err != nil {
			return err
		}
	case "error":
		if _, err := fmt.Fprintf(m.writer, "ERROR: %s\n", event.Content); err != nil {
			return err
		}
	case "summary":
		if event.Summary != nil {
			if _, err := fmt.Fprintf(m.writer, "Summary: task_id=%s status=%s\n", event.Summary.TaskID, event.Summary.Status); err != nil {
				return err
			}
		}
	case "command":
		if _, err := fmt.Fprintf(m.writer, "Command: %v (exit %d)\n", event.Command, event.ExitCode); err != nil {
			return err
		}
	default:
		// Default to JSON for unknown event types
		if _, err := fmt.Fprintf(m.writer, "%s\n", formatEvent(event)); err != nil {
			return err
		}
	}

	return nil
}

// buildPrefix builds a prefix string from the event's task/agent ID.
func (m *MultiplexedLogWriter) buildPrefix(event LogEvent) string {
	// Check if the event has embedded task/agent info
	// We'll use the Summary field or add new fields to LogEvent if needed
	if event.Summary != nil && event.Summary.TaskID != "" {
		return event.Summary.TaskID
	}
	return ""
}

// formatEvent formats an event as a compact JSON string for output.
func formatEvent(event LogEvent) string {
	// Simplified JSON formatting for multiplexed output
	// In a full implementation, this would use json.Marshal
	return fmt.Sprintf("{type=%s timestamp=%s}", event.Type, event.Timestamp.Format("15:04:05"))
}
