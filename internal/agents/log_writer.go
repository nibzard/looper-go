// Package agents defines Codex and Claude runners.
package agents

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// LogEvent represents a single log event from the agent.
type LogEvent struct {
	// Type is the event type: assistant_message, tool, command, error, summary
	Type string `json:"type"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Content is the message content (for assistant_message and error)
	Content string `json:"content,omitempty"`

	// Tool is the tool name (for tool events)
	Tool string `json:"tool,omitempty"`

	// Command is the command that was run (for command events)
	Command []string `json:"command,omitempty"`

	// ExitCode is the command exit code (for command events)
	ExitCode int `json:"exit_code,omitempty"`

	// Summary is the parsed summary (for summary events)
	Summary *Summary `json:"summary,omitempty"`
}

// LogWriter writes log events.
type LogWriter interface {
	Write(event LogEvent) error
}

// IOStreamLogWriter writes log events to an io.Writer.
type IOStreamLogWriter struct {
	w      io.Writer
	indent string
}

// NewIOStreamLogWriter creates a new log writer that writes to an io.Writer.
func NewIOStreamLogWriter(w io.Writer) *IOStreamLogWriter {
	return &IOStreamLogWriter{w: w}
}

// SetIndent sets the indentation prefix for log output.
func (l *IOStreamLogWriter) SetIndent(indent string) {
	l.indent = indent
}

// Write writes a log event to the underlying writer.
func (l *IOStreamLogWriter) Write(event LogEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal log event: %w", err)
	}
	if l.indent != "" {
		data = append([]byte(l.indent), data...)
	}
	data = append(data, '\n')
	_, err = l.w.Write(data)
	return err
}

// MultiLogWriter writes to multiple log writers.
type MultiLogWriter struct {
	writers []LogWriter
}

// NewMultiLogWriter creates a new multi-log writer.
func NewMultiLogWriter(writers ...LogWriter) *MultiLogWriter {
	return &MultiLogWriter{writers: writers}
}

// Write writes the event to all underlying writers.
func (m *MultiLogWriter) Write(event LogEvent) error {
	var errs []error
	for _, w := range m.writers {
		if err := w.Write(event); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multi-writer errors: %v", errs)
	}
	return nil
}

// NullLogWriter is a no-op log writer.
type NullLogWriter struct{}

// Write does nothing.
func (NullLogWriter) Write(event LogEvent) error {
	return nil
}

type lockedLogWriter struct {
	mu     sync.Mutex
	writer LogWriter
}

func (l *lockedLogWriter) Write(event LogEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writer.Write(event)
}

func normalizeLogWriter(writer LogWriter) LogWriter {
	if writer == nil {
		return NullLogWriter{}
	}
	return &lockedLogWriter{writer: writer}
}
