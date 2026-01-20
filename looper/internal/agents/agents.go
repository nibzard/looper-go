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
	"strconv"
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

const maxScanTokenSize = 1024 * 1024

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
	// If zero, DefaultTimeout is used. Use a negative value to disable timeouts.
	Timeout time.Duration

	// WorkDir is the working directory for the agent command.
	WorkDir string

	// LastMessagePath is an optional path to write the last message (codex only).
	LastMessagePath string
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

func normalizeConfig(agentType AgentType, cfg Config) Config {
	if cfg.Binary == "" {
		switch agentType {
		case AgentTypeCodex:
			cfg.Binary = "codex"
		case AgentTypeClaude:
			cfg.Binary = "claude"
		}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return cfg
}

func ensurePromptTerminator(prompt string) string {
	if strings.HasSuffix(prompt, "\n") {
		return prompt
	}
	return prompt + "\n"
}

func applyTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// codexAgent implements Agent for Codex.
type codexAgent struct {
	cfg Config
}

// NewCodexAgent creates a new Codex agent.
func NewCodexAgent(cfg Config) Agent {
	return &codexAgent{cfg: normalizeConfig(AgentTypeCodex, cfg)}
}

// Run executes the Codex agent.
func (a *codexAgent) Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	logWriter = normalizeLogWriter(logWriter)
	cfg := normalizeConfig(AgentTypeCodex, a.cfg)

	// Build arguments
	args := []string{"exec", "--json"}
	if cfg.Model != "" {
		args = append(args, "-m", cfg.Model)
	}
	args = append(args, cfg.Args...)
	if cfg.LastMessagePath != "" {
		args = append(args, "--output-last-message", cfg.LastMessagePath)
	}
	args = append(args, "-")

	// Apply timeout
	var cancel context.CancelFunc
	ctx, cancel = applyTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	cmd.Stdin = strings.NewReader(ensurePromptTerminator(prompt))

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
		_ = logWriter.Write(LogEvent{
			Type:      "error",
			Timestamp: time.Now().UTC(),
			Content:   err.Error(),
		})
		return nil, fmt.Errorf("start codex: %w", err)
	}

	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Stream output
	summaries, errs := a.streamOutput(ctx, stdout, stderr, logWriter)

	// Wait for command to finish
	runErr := cmd.Wait()
	exitCode := exitCodeFromError(runErr)
	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
		ExitCode:  exitCode,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Collect results
	var summary *Summary
	var outputErrs []error

	for s := range summaries {
		summary = s
	}
	for e := range errs {
		outputErrs = append(outputErrs, e)
	}

	if summary == nil && cfg.LastMessagePath != "" {
		if parsed, ok := parseSummaryFromFile(cfg.LastMessagePath); ok {
			summary = parsed
			_ = logWriter.Write(LogEvent{
				Type:      "summary",
				Timestamp: time.Now().UTC(),
				Summary:   summary,
			})
		}
	}

	// Handle errors
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			_ = logWriter.Write(LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("codex timeout after %s", cfg.Timeout),
			})
			return nil, fmt.Errorf("codex timeout after %s", cfg.Timeout)
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
		scanner.Buffer(make([]byte, 0, 64*1024), maxScanTokenSize)
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
		scanner.Buffer(make([]byte, 0, 64*1024), maxScanTokenSize)
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
		if err := scanner.Err(); err != nil {
			select {
			case errs <- fmt.Errorf("stderr scanner error: %w", err):
			default:
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
	if strings.TrimSpace(line) == "" {
		return nil
	}

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

	eventType := classifyEventType(rawData)
	event := LogEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}

	text := extractTextFromMessage(rawData)
	if eventType == "assistant_message" {
		if text != "" {
			event.Content = text
		} else {
			event.Content = line
		}
	} else {
		event.Content = line
	}

	if tool := extractToolName(rawData); tool != "" {
		event.Tool = tool
	}
	if cmd, ok := extractCommand(rawData); ok {
		event.Command = cmd
	}
	if exitCode, ok := extractExitCode(rawData); ok {
		event.ExitCode = exitCode
	}

	if err := logWriter.Write(event); err != nil {
		return fmt.Errorf("write log event: %w", err)
	}

	summary, ok := parseSummaryFromRaw(rawData)
	if !ok && text != "" {
		summary, ok = parseSummaryFromText(text)
	}
	if ok {
		sendSummary(ctx, summaries, summary)
		if err := logWriter.Write(LogEvent{
			Type:      "summary",
			Timestamp: time.Now().UTC(),
			Summary:   summary,
		}); err != nil {
			return fmt.Errorf("write log event: %w", err)
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
	return &claudeAgent{cfg: normalizeConfig(AgentTypeClaude, cfg)}
}

// Run executes the Claude agent.
func (a *claudeAgent) Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	logWriter = normalizeLogWriter(logWriter)
	cfg := normalizeConfig(AgentTypeClaude, a.cfg)

	// Build arguments
	args := []string{"--output-format", "stream-json"}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	args = append(args, cfg.Args...)
	args = append(args, "-p", prompt)

	// Apply timeout
	var cancel context.CancelFunc
	ctx, cancel = applyTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
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
		_ = logWriter.Write(LogEvent{
			Type:      "error",
			Timestamp: time.Now().UTC(),
			Content:   err.Error(),
		})
		return nil, fmt.Errorf("start claude: %w", err)
	}

	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Stream output
	summaries, errs := a.streamOutput(ctx, stdout, stderr, logWriter)

	// Wait for command to finish
	runErr := cmd.Wait()
	exitCode := exitCodeFromError(runErr)
	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
		ExitCode:  exitCode,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

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
			_ = logWriter.Write(LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("claude timeout after %s", cfg.Timeout),
			})
			return nil, fmt.Errorf("claude timeout after %s", cfg.Timeout)
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
		scanner.Buffer(make([]byte, 0, 64*1024), maxScanTokenSize)
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
		if err := scanner.Err(); err != nil {
			select {
			case errs <- fmt.Errorf("stderr scanner error: %w", err):
			default:
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
	sawFullMessage := false

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

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

		eventType := classifyEventType(raw)
		event := LogEvent{
			Type:      eventType,
			Timestamp: time.Now().UTC(),
		}

		content := ""
		if eventType == "assistant_message" {
			content = extractClaudeEventText(raw)
			if content == "" {
				content = line
			}
		} else {
			content = line
		}
		event.Content = content

		if tool := extractToolName(raw); tool != "" {
			event.Tool = tool
		}
		if cmd, ok := extractCommand(raw); ok {
			event.Command = cmd
		}
		if exitCode, ok := extractExitCode(raw); ok {
			event.ExitCode = exitCode
		}

		if err := logWriter.Write(event); err != nil {
			return fmt.Errorf("write log event: %w", err)
		}

		if !sawFullMessage {
			if full := extractClaudeFullMessage(raw); full != "" {
				lastMessageBuf.Reset()
				lastMessageBuf.WriteString(full)
				sawFullMessage = true
			} else if delta := extractClaudeStreamDelta(raw); delta != "" {
				lastMessageBuf.WriteString(delta)
			}
		}
	}

	if lastMessageBuf.Len() > 0 {
		if summary, ok := parseSummaryFromText(lastMessageBuf.String()); ok {
			sendSummary(ctx, summaries, summary)
			if err := logWriter.Write(LogEvent{
				Type:      "summary",
				Timestamp: time.Now().UTC(),
				Summary:   summary,
			}); err != nil {
				return fmt.Errorf("write log event: %w", err)
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

func textFromContent(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			return text
		}
	case []any:
		var parts []string
		for _, item := range v {
			switch typed := item.(type) {
			case string:
				if typed != "" {
					parts = append(parts, typed)
				}
			case map[string]any:
				if text, ok := typed["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func extractTextFromMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if message, ok := raw["message"].(map[string]any); ok {
		if text := textFromContent(message["content"]); text != "" {
			return text
		}
	}
	if text := textFromContent(raw["content"]); text != "" {
		return text
	}
	if text, ok := raw["text"].(string); ok {
		return text
	}
	return ""
}

func extractClaudeFullMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if message, ok := raw["message"].(map[string]any); ok {
		if text := textFromContent(message["content"]); text != "" {
			return text
		}
	}
	return ""
}

func extractClaudeStreamDelta(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	msgType, _ := raw["type"].(string)
	switch msgType {
	case "content_block_delta":
		if delta, ok := raw["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok {
				return text
			}
		}
	case "content_block_start":
		if block, ok := raw["content_block"].(map[string]any); ok {
			if text, ok := block["text"].(string); ok {
				return text
			}
		}
	}
	return ""
}

func extractClaudeEventText(raw map[string]any) string {
	if full := extractClaudeFullMessage(raw); full != "" {
		return full
	}
	if delta := extractClaudeStreamDelta(raw); delta != "" {
		return delta
	}
	return ""
}

func parseSummaryFromText(text string) (*Summary, bool) {
	summaryJSON := extractJSON(text)
	if summaryJSON == "" {
		return nil, false
	}
	var summary Summary
	if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
		return nil, false
	}
	if !summaryHasContent(summary) {
		return nil, false
	}
	return &summary, true
}

func parseSummaryFromFile(path string) (*Summary, bool) {
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, false
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		if summary, ok := parseSummaryFromRaw(raw); ok {
			return summary, true
		}
		if text := extractTextFromMessage(raw); text != "" {
			return parseSummaryFromText(text)
		}
	}
	return parseSummaryFromText(string(data))
}

func parseSummaryFromRaw(raw map[string]any) (*Summary, bool) {
	if raw == nil {
		return nil, false
	}
	if _, ok := raw["task_id"]; ok {
		// continue
	} else if _, ok := raw["status"]; ok {
		// continue
	} else if _, ok := raw["summary"]; ok {
		// continue
	} else if _, ok := raw["files"]; ok {
		// continue
	} else if _, ok := raw["blockers"]; ok {
		// continue
	} else {
		return nil, false
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, false
	}
	if !summaryHasContent(summary) {
		return nil, false
	}
	return &summary, true
}

func summaryHasContent(summary Summary) bool {
	return summary.TaskID != "" ||
		summary.Status != "" ||
		summary.Summary != "" ||
		len(summary.Files) > 0 ||
		len(summary.Blockers) > 0
}

func classifyEventType(raw map[string]any) string {
	if raw == nil {
		return "assistant_message"
	}
	if msgType, ok := raw["type"].(string); ok {
		switch msgType {
		case "tool_use", "tool_result", "tool", "tool_call":
			return "tool"
		case "command":
			return "command"
		case "error":
			return "error"
		}
	}
	if _, ok := raw["command"]; ok {
		return "command"
	}
	if _, ok := raw["tool"]; ok {
		return "tool"
	}
	if _, ok := raw["tool_name"]; ok {
		return "tool"
	}
	return "assistant_message"
}

func extractToolName(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if tool, ok := raw["tool"].(string); ok {
		return tool
	}
	if tool, ok := raw["tool_name"].(string); ok {
		return tool
	}
	if msgType, _ := raw["type"].(string); msgType == "tool_use" || msgType == "tool_result" || msgType == "tool" || msgType == "tool_call" {
		if name, ok := raw["name"].(string); ok {
			return name
		}
		if toolUse, ok := raw["tool_use"].(map[string]any); ok {
			if name, ok := toolUse["name"].(string); ok {
				return name
			}
		}
	}
	return ""
}

func extractCommand(raw map[string]any) ([]string, bool) {
	if raw == nil {
		return nil, false
	}
	value, ok := raw["command"]
	if !ok {
		return nil, false
	}
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil, false
		}
		return []string{typed}, true
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return nil, false
		}
		return parts, true
	}
	return nil, false
}

func extractExitCode(raw map[string]any) (int, bool) {
	if raw == nil {
		return 0, false
	}
	if value, ok := raw["exit_code"]; ok {
		return parseExitCode(value)
	}
	if value, ok := raw["exitCode"]; ok {
		return parseExitCode(value)
	}
	return 0, false
}

func parseExitCode(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed), true
		}
		if parsed, err := typed.Float64(); err == nil {
			return int(parsed), true
		}
	case string:
		if parsed, err := strconv.Atoi(typed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func sendSummary(ctx context.Context, summaries chan *Summary, summary *Summary) {
	if summary == nil {
		return
	}
	select {
	case summaries <- summary:
		return
	default:
	}
	select {
	case <-summaries:
	default:
	}
	select {
	case summaries <- summary:
	case <-ctx.Done():
	}
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

// DefaultTimeout is the default timeout for agents.
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
