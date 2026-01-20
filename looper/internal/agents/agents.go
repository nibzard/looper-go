// Package agents defines Codex and Claude runners.
package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// AgentType represents the type of agent.
type AgentType string

const (
	AgentTypeCodex  AgentType = "codex"
	AgentTypeClaude AgentType = "claude"
)

// Summary is the expected output from an agent run.
type Summary struct {
	TaskID   string   `json:"task_id"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Files    []string `json:"files,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

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

// Agent is the interface for running AI agents.
type Agent interface {
	// Run executes the agent with the given prompt and context.
	// It returns the summary from the agent or an error.
	Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error)
}

// Config holds configuration for an agent.
type Config struct {
	// Binary is the path to the agent binary.
	Binary string

	// Model is the model to use (optional).
	Model string

	// Args are additional arguments to pass to the binary.
	Args []string

	// Timeout is the maximum duration to wait for the agent to complete.
	// If zero, no timeout is enforced.
	Timeout time.Duration

	// WorkDir is the working directory for the agent command.
	WorkDir string
}

// codexAgent implements Agent for Codex.
type codexAgent struct {
	cfg Config
}

// NewCodexAgent creates a new Codex agent.
func NewCodexAgent(cfg Config) Agent {
	return &codexAgent{cfg: cfg}
}

// Run executes the Codex agent.
func (a *codexAgent) Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	// Build arguments
	args := []string{"exec", "--json"}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	args = append(args, a.cfg.Args...)
	args = append(args, "--", prompt)

	// Apply timeout
	if a.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.cfg.Timeout)
		defer cancel()
	}

	// Create command
	cmd := exec.CommandContext(ctx, a.cfg.Binary, args...)
	if a.cfg.WorkDir != "" {
		cmd.Dir = a.cfg.WorkDir
	}

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}

	// Stream output
	summaries, errs := a.streamOutput(ctx, stdout, stderr, logWriter)

	// Wait for command to finish
	runErr := cmd.Wait()

	// Collect results
	var summary *Summary
	var outputErrs []error

	for s := range summaries {
		summary = s
	}
	for e := range errs {
		outputErrs = append(outputErrs, e)
	}

	// Handle errors
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("codex timeout after %s", a.cfg.Timeout)
		}
		if len(outputErrs) > 0 {
			return nil, fmt.Errorf("codex failed: %w (output errors: %v)", runErr, outputErrs)
		}
		return nil, fmt.Errorf("codex failed: %w", runErr)
	}

	if summary == nil {
		return nil, errors.New("codex did not produce a summary")
	}

	return summary, nil
}

// streamOutput streams stdout and stderr from the codex process.
func (a *codexAgent) streamOutput(
	ctx context.Context,
	stdout, stderr io.Reader,
	logWriter LogWriter,
) (<-chan *Summary, <-chan error) {
	summaries := make(chan *Summary, 1)
	errs := make(chan error, 10)

	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout (JSON lines)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if err := a.processLine(ctx, line, logWriter, summaries); err != nil {
				select {
				case errs <- err:
				default:
				}
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case errs <- fmt.Errorf("scanner error: %w", err):
			default:
			}
		}
	}()

	// Stream stderr (plain text errors)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				_ = logWriter.Write(LogEvent{
					Type:      "error",
					Timestamp: time.Now().UTC(),
					Content:   line,
				})
			}
		}
	}()

	go func() {
		wg.Wait()
		close(summaries)
		close(errs)
	}()

	return summaries, errs
}

// processLine processes a single line of JSON output from codex.
func (a *codexAgent) processLine(
	ctx context.Context,
	line string,
	logWriter LogWriter,
	summaries chan *Summary,
) error {
	// Try to parse as JSON
	var rawData map[string]any
	if err := json.Unmarshal([]byte(line), &rawData); err != nil {
		// Not JSON, log as assistant message
		return logWriter.Write(LogEvent{
			Type:      "assistant_message",
			Timestamp: time.Now().UTC(),
			Content:   line,
		})
	}

	// Check for message type
	msgType, _ := rawData["type"].(string)

	// Write raw event to log
	eventType := "assistant_message"
	switch {
	case msgType == "tool_use":
		eventType = "tool"
	case rawData["command"] != nil:
		eventType = "command"
	case rawData["summary"] != nil:
		eventType = "summary"
	}

	if err := logWriter.Write(LogEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Content:   line,
	}); err != nil {
		return fmt.Errorf("write log event: %w", err)
	}

	// Check for summary
	if rawData["task_id"] != nil || rawData["summary"] != nil {
		var summary Summary
		if err := json.Unmarshal([]byte(line), &summary); err == nil {
			select {
			case summaries <- &summary:
			case <-ctx.Done():
			}
		}
	}

	return nil
}

// claudeAgent implements Agent for Claude.
type claudeAgent struct {
	cfg Config
}

// NewClaudeAgent creates a new Claude agent.
func NewClaudeAgent(cfg Config) Agent {
	return &claudeAgent{cfg: cfg}
}

// Run executes the Claude agent.
func (a *claudeAgent) Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	// Build arguments
	args := []string{"--output-format", "stream-json"}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	args = append(args, a.cfg.Args...)
	args = append(args, "--", prompt)

	// Apply timeout
	if a.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.cfg.Timeout)
		defer cancel()
	}

	// Create command
	cmd := exec.CommandContext(ctx, a.cfg.Binary, args...)
	if a.cfg.WorkDir != "" {
		cmd.Dir = a.cfg.WorkDir
	}

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Stream output
	summaries, errs := a.streamOutput(ctx, stdout, stderr, logWriter)

	// Wait for command to finish
	runErr := cmd.Wait()

	// Collect results
	var summary *Summary
	var outputErrs []error

	for s := range summaries {
		summary = s
	}
	for e := range errs {
		outputErrs = append(outputErrs, e)
	}

	// Handle errors
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude timeout after %s", a.cfg.Timeout)
		}
		if len(outputErrs) > 0 {
			return nil, fmt.Errorf("claude failed: %w (output errors: %v)", runErr, outputErrs)
		}
		return nil, fmt.Errorf("claude failed: %w", runErr)
	}

	if summary == nil {
		return nil, errors.New("claude did not produce a summary")
	}

	return summary, nil
}

// streamOutput streams stdout and stderr from the claude process.
func (a *claudeAgent) streamOutput(
	ctx context.Context,
	stdout, stderr io.Reader,
	logWriter LogWriter,
) (<-chan *Summary, <-chan error) {
	summaries := make(chan *Summary, 1)
	errs := make(chan error, 10)

	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout (stream-json format)
	go func() {
		defer wg.Done()
		if err := a.processStreamJSON(ctx, stdout, logWriter, summaries); err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}()

	// Stream stderr (plain text errors)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				_ = logWriter.Write(LogEvent{
					Type:      "error",
					Timestamp: time.Now().UTC(),
					Content:   line,
				})
			}
		}
	}()

	go func() {
		wg.Wait()
		close(summaries)
		close(errs)
	}()

	return summaries, errs
}

// processStreamJSON processes Claude's stream-json format.
// The format is NDJSON (newline-delimited JSON) with various event types.
func (a *claudeAgent) processStreamJSON(
	ctx context.Context,
	r io.Reader,
	logWriter LogWriter,
	summaries chan *Summary,
) error {
	decoder := json.NewDecoder(r)

	var lastMessageBuf bytes.Buffer

	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decode json: %w", err)
		}

		// Serialize back to JSON for logging
		data, _ := json.Marshal(raw)
		line := string(data)

		// Determine event type
		msgType, _ := raw["type"].(string)

		eventType := "assistant_message"
		switch msgType {
		case "tool_use":
			eventType = "tool"
		case "command":
			eventType = "command"
		}

		// Write log event
		if err := logWriter.Write(LogEvent{
			Type:      eventType,
			Timestamp: time.Now().UTC(),
			Content:   line,
		}); err != nil {
			return fmt.Errorf("write log event: %w", err)
		}

		// For assistant messages, accumulate to extract final response
		if msgType == "assistant_message" {
			if content, ok := raw["content"].([]any); ok {
				for _, item := range content {
					if itemMap, ok := item.(map[string]any); ok {
						if itemType, _ := itemMap["type"].(string); itemType == "text" {
							if text, _ := itemMap["text"].(string); text != "" {
								lastMessageBuf.WriteString(text)
							}
						}
					}
				}
			}
		}
	}

	// Try to parse the last message as a summary
	if lastMessageBuf.Len() > 0 {
		lastMessage := lastMessageBuf.String()
		// Look for JSON within the message (often Claude wraps in markdown)
		summaryJSON := extractJSON(lastMessage)
		if summaryJSON != "" {
			var summary Summary
			if err := json.Unmarshal([]byte(summaryJSON), &summary); err == nil {
				select {
				case summaries <- &summary:
				case <-ctx.Done():
				}
			}
		}
	}

	return nil
}

// extractJSON extracts a JSON object from a string.
// It handles markdown code blocks with json language tags.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Check for markdown code block
	if strings.HasPrefix(s, "```json") {
		start := strings.Index(s, "{")
		end := strings.LastIndex(s, "}")
		if start >= 0 && end > start {
			return s[start : end+1]
		}
	}

	// Check for code block without language tag
	if strings.HasPrefix(s, "```") {
		start := strings.Index(s, "{")
		end := strings.LastIndex(s, "}")
		if start >= 0 && end > start {
			return s[start : end+1]
		}
	}

	// Check if the whole string is JSON
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return s
	}

	// Look for first JSON object in the string
	start := strings.Index(s, "{")
	if start >= 0 {
		// Find matching closing brace
		braceCount := 0
		for i := start; i < len(s); i++ {
			switch s[i] {
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					return s[start : i+1]
				}
			}
		}
	}

	return ""
}

// NewAgent creates an agent of the specified type.
func NewAgent(agentType AgentType, cfg Config) (Agent, error) {
	switch agentType {
	case AgentTypeCodex:
		return NewCodexAgent(cfg), nil
	case AgentTypeClaude:
		return NewClaudeAgent(cfg), nil
	default:
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
}

// DefaultTimeout returns the default timeout for agents.
const DefaultTimeout = 30 * time.Minute

// FindAgentBinary finds the agent binary in PATH.
func FindAgentBinary(agentType AgentType) (string, error) {
	var name string
	switch agentType {
	case AgentTypeCodex:
		name = "codex"
	case AgentTypeClaude:
		name = "claude"
	default:
		return "", fmt.Errorf("unknown agent type: %s", agentType)
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("agent binary %q not found in PATH: %w", name, err)
	}
	return path, nil
}

// ValidateBinary checks if a binary exists and is executable.
func ValidateBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("binary not found: %s", path)
		}
		return fmt.Errorf("stat binary: %w", err)
	}

	// Check if it's executable (Unix)
	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("binary is not executable: %s", path)
	}

	return nil
}
