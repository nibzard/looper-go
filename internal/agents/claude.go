package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

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
	spec := agentSpec{
		name: "claude",
		buildArgs: func(cfg Config, prompt string) []string {
			args := []string{"--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
			if cfg.Model != "" {
				args = append(args, "--model", cfg.Model)
			}
			args = append(args, cfg.Args...)
			return append(args, "-p", prompt)
		},
		streamOutput: func(ctx context.Context, stdout, stderr io.Reader, logWriter LogWriter) agentStreamResult {
			summaries, errs, lastMessages := a.streamOutput(ctx, stdout, stderr, logWriter)
			return agentStreamResult{summaries: summaries, errs: errs, lastMessage: lastMessages}
		},
	}
	return runAgent(ctx, a.cfg, prompt, logWriter, spec)
}

// streamOutput streams stdout and stderr from the claude process.
func (a *claudeAgent) streamOutput(
	ctx context.Context,
	stdout, stderr io.Reader,
	logWriter LogWriter,
) (<-chan *Summary, <-chan error, <-chan string) {
	summaries := make(chan *Summary, 1)
	errs := make(chan error, 10)
	lastMessages := make(chan string, 1)

	streamWithStderr(ctx, stdout, stderr, logWriter, summaries, errs, lastMessages, func() (string, error) {
		return a.processStreamJSON(ctx, stdout, logWriter, summaries)
	})

	return summaries, errs, lastMessages
}

// processStreamJSON processes Claude's stream-json format.
// The format is NDJSON (newline-delimited JSON) with various event types.
func (a *claudeAgent) processStreamJSON(
	ctx context.Context,
	r io.Reader,
	logWriter LogWriter,
	summaries chan *Summary,
) (string, error) {
	decoder := json.NewDecoder(r)

	var lastMessageBuf bytes.Buffer
	sawFullMessage := false
	var lastAssistantContent string // Track last assistant_message content

	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("decode json: %w", err)
		}

		// Serialize back to JSON for logging
		data, _ := json.Marshal(raw)
		line := string(data)

		if err := logRawEvent(logWriter, raw, line, extractClaudeEventText(raw)); err != nil {
			return "", err
		}

		// Check for assistant_message events with plain text content
		// These often contain the final summary JSON
		if msgType, ok := raw["type"].(string); ok && msgType == "assistant_message" {
			if content, ok := raw["content"].(string); ok && content != "" {
				// First, try to unescape the content if it's a JSON-encoded string
				// (e.g., "{\"task_id\":...}" -> {"task_id":...})
				var unescapedContent string
				if err := json.Unmarshal([]byte(content), &unescapedContent); err == nil {
					content = unescapedContent
				}

				// Try to extract JSON from the content (handles markdown code blocks)
				if jsonStr := extractJSON(content); jsonStr != "" {
					// Check if it's actually a summary by validating fields
					var rawObj map[string]any
					if err := json.Unmarshal([]byte(jsonStr), &rawObj); err == nil {
						if _, hasTaskID := rawObj["task_id"]; hasTaskID {
							if _, hasStatus := rawObj["status"]; hasStatus {
								// This is a valid summary JSON - save it
								lastAssistantContent = jsonStr
							}
						}
					}
				}
				// Always accumulate non-JSON content if we haven't seen a full message
				if lastAssistantContent == "" && !sawFullMessage {
					lastMessageBuf.WriteString(content)
				}
			}
		}

		// Look for full message events from the stream
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

	// Prefer the assistant_message content if it was captured
	if lastAssistantContent != "" {
		if summary, ok := parseSummaryFromText(lastAssistantContent); ok {
			_ = logWriter.Write(LogEvent{
				Type:      "debug",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("Parsed summary from assistant_message: %+v", summary),
			})
			if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
				return "", err
			}
			return lastAssistantContent, nil
		}
	}

	// Fall back to accumulated message content
	if lastMessageBuf.Len() > 0 {
		content := lastMessageBuf.String()
		_ = logWriter.Write(LogEvent{
			Type:      "debug",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("Parsing summary from accumulated content (len=%d)", len(content)),
		})
		if summary, ok := parseSummaryFromText(content); ok {
			_ = logWriter.Write(LogEvent{
				Type:      "debug",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("Successfully parsed summary: %+v", summary),
			})
			if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
				return "", err
			}
		}
	}

	return lastMessageBuf.String(), nil
}
