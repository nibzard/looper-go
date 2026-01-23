// Package agents provides console logging with charmbracelet/log.
package agents

import (
	"io"
	"os"

	"github.com/charmbracelet/log"
)

// ConsoleLogOptions holds configuration for console logging.
type ConsoleLogOptions struct {
	Level           log.Level
	Formatter       log.Formatter
	ReportTimestamp bool
	ReportCaller    bool
	Prefix          string
}

// DefaultConsoleLogOptions returns default options for console logging.
func DefaultConsoleLogOptions() ConsoleLogOptions {
	return ConsoleLogOptions{
		Level:           log.InfoLevel,
		Formatter:       log.TextFormatter,
		ReportTimestamp: false,
		ReportCaller:    false,
		Prefix:          "looper",
	}
}

// ConsoleLogWriter implements LogWriter using charmbracelet/log for
// colorful, leveled, human-readable console output.
type ConsoleLogWriter struct {
	logger *log.Logger
}

// NewConsoleLogWriter creates a new console log writer with the given options.
func NewConsoleLogWriter(opts ConsoleLogOptions) *ConsoleLogWriter {
	logger := log.NewWithOptions(os.Stdout, log.Options{
		Level:           opts.Level,
		Formatter:       opts.Formatter,
		ReportTimestamp: opts.ReportTimestamp,
		ReportCaller:    opts.ReportCaller,
		Prefix:          opts.Prefix,
	})
	return &ConsoleLogWriter{logger: logger}
}

// NewConsoleLogWriterWithLogger creates a new console log writer with a custom logger.
// This is useful for testing or when you want to redirect output.
func NewConsoleLogWriterWithLogger(logger *log.Logger) *ConsoleLogWriter {
	return &ConsoleLogWriter{logger: logger}
}

// Write writes a log event to the console using charmbracelet/log.
func (c *ConsoleLogWriter) Write(event LogEvent) error {
	msg := formatMessage(event)
	fields := c.extractFields(event)

	switch event.Type {
	case "error":
		c.logger.Error(msg, fields...)
	case "command", "summary":
		c.logger.Info(msg, fields...)
	case "tool", "assistant_message", "agent_output", "debug":
		c.logger.Debug(msg, fields...)
	default:
		c.logger.Debug(msg, fields...)
	}
	return nil
}

// extractFields extracts structured fields from a LogEvent for charmbracelet/log.
func (c *ConsoleLogWriter) extractFields(event LogEvent) []any {
	var fields []any
	if event.Tool != "" {
		fields = append(fields, "tool", event.Tool)
	}
	if len(event.Command) > 0 {
		fields = append(fields, "command", event.Command)
	}
	if event.ExitCode != 0 {
		fields = append(fields, "exit_code", event.ExitCode)
	}
	if event.Summary != nil && event.Summary.TaskID != "" {
		fields = append(fields, "task_id", event.Summary.TaskID)
		if event.Summary.Status != "" {
			fields = append(fields, "status", event.Summary.Status)
		}
		if event.Summary.Summary != "" {
			fields = append(fields, "summary", event.Summary.Summary)
		}
	}
	return fields
}

// formatMessage formats a log message from a LogEvent.
func formatMessage(event LogEvent) string {
	if event.Content != "" {
		return event.Content
	}
	switch event.Type {
	case "command":
		if len(event.Command) > 0 {
			return "Running command"
		}
		return "Command"
	case "summary":
		if event.Summary != nil {
			if event.Summary.Status == "done" {
				return "Task completed"
			}
			return "Summary received"
		}
		return "Summary"
	case "tool":
		if event.Tool != "" {
			return "Using tool"
		}
		return "Tool"
	case "error":
		return "Error"
	case "assistant_message":
		return "Assistant message"
	default:
		return event.Type
	}
}

// ParseLogLevel parses a string log level to a charmbracelet/log Level.
func ParseLogLevel(level string) log.Level {
	switch level {
	case "debug":
		return log.DebugLevel
	case "info":
		return log.InfoLevel
	case "warn", "warning":
		return log.WarnLevel
	case "error":
		return log.ErrorLevel
	case "fatal":
		return log.FatalLevel
	default:
		return log.InfoLevel
	}
}

// ParseLogFormatter parses a string formatter name to a charmbracelet/log Formatter.
func ParseLogFormatter(format string) log.Formatter {
	switch format {
	case "json":
		return log.JSONFormatter
	case "logfmt":
		return log.LogfmtFormatter
	default:
		return log.TextFormatter
	}
}

// parseLogLevel parses a string log level to a charmbracelet/log Level.
// Deprecated: Use ParseLogLevel instead.
func parseLogLevel(level string) log.Level {
	return ParseLogLevel(level)
}

// parseLogFormatter parses a string formatter name to a charmbracelet/log Formatter.
// Deprecated: Use ParseLogFormatter instead.
func parseLogFormatter(format string) log.Formatter {
	return ParseLogFormatter(format)
}

// NewConsoleLogWriterFromConfig creates a ConsoleLogWriter from string configuration values.
// This is useful when loading config from TOML or environment variables.
func NewConsoleLogWriterFromConfig(level, format string, timestamps, caller bool, prefix string) *ConsoleLogWriter {
	opts := ConsoleLogOptions{
		Level:           ParseLogLevel(level),
		Formatter:       ParseLogFormatter(format),
		ReportTimestamp: timestamps,
		ReportCaller:    caller,
		Prefix:          prefix,
	}
	return NewConsoleLogWriter(opts)
}

// NewTestConsoleLogWriter creates a console log writer that writes to a specific writer
// for testing purposes. It uses minimal formatting for easier test assertions.
func NewTestConsoleLogWriter(w io.Writer) *ConsoleLogWriter {
	logger := log.NewWithOptions(w, log.Options{
		Level:           log.DebugLevel,
		Formatter:       log.TextFormatter,
		ReportTimestamp: false,
		ReportCaller:    false,
	})
	return &ConsoleLogWriter{logger: logger}
}
