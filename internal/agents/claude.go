package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
	var lastAssistantContent string // Track last assistant content with summary JSON

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

		// Check for assistant events with text content
		// These may contain the final summary JSON
		// Note: The stream-json format uses type "assistant"
		if msgType, ok := raw["type"].(string); ok && msgType == "assistant" {
			// Extract text content from message.content array
			// The content is an array like: [{type:"text", text:"..."}, {type:"tool_use", ...}]
			var contentBuilder strings.Builder
			if message, ok := raw["message"].(map[string]any); ok {
				if contentArray, ok := message["content"].([]any); ok {
					for _, item := range contentArray {
						if contentItem, ok := item.(map[string]any); ok {
							if itemType, ok := contentItem["type"].(string); ok && itemType == "text" {
								if text, ok := contentItem["text"].(string); ok {
									contentBuilder.WriteString(text)
								}
							}
						}
					}
				}
			}

			content := contentBuilder.String()
			if content != "" {
				// Try to extract JSON from the content (handles markdown code blocks)
				if jsonStr := extractJSON(content); jsonStr != "" {
					// Check if it's actually a summary by validating fields
					var rawObj map[string]any
					if err := json.Unmarshal([]byte(jsonStr), &rawObj); err == nil {
						_, hasTaskID := rawObj["task_id"]
						_, hasStatus := rawObj["status"]
						if hasTaskID && hasStatus {
							// This is a valid summary JSON - save it
							lastAssistantContent = jsonStr
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
				// Check that this is actual text content, not just a JSON structure
				// Init events return JSON that starts with "{" and contains type/message fields
				isJSONStruct := strings.HasPrefix(full, "{") && (strings.Contains(full, "\"type\":") || strings.Contains(full, "\"message\":"))
				if !isJSONStruct {
					lastMessageBuf.Reset()
					lastMessageBuf.WriteString(full)
					sawFullMessage = true
				}
			} else if delta := extractClaudeStreamDelta(raw); delta != "" {
				lastMessageBuf.WriteString(delta)
			}
		}
	}

	// Prefer the assistant content if it was captured
	if lastAssistantContent != "" {
		if summary, ok := parseSummaryFromText(lastAssistantContent); ok {
			if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
				return "", err
			}
			return lastAssistantContent, nil
		}
	}

	// Fall back to accumulated message content
	if lastMessageBuf.Len() > 0 {
		content := lastMessageBuf.String()
		if summary, ok := parseSummaryFromText(content); ok {
			if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
				return "", err
			}
		}
	}

	return lastMessageBuf.String(), nil
}
