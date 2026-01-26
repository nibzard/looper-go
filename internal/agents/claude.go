package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// safePrefix returns a safe prefix of a string for logging.
func safePrefix(s string) string {
	const maxLen = 200
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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
	isInitEvent := false           // Track if we're processing an init event

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

		// DEBUG: Log raw decoded structure BEFORE any processing
		rawType, typeOK := raw["type"]
		_ = logWriter.Write(LogEvent{
			Type:      "debug",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("[DEBUG-POST-DECODE] raw type=%v (typeOK=%v), raw=%+v", rawType, typeOK, raw),
		})
		fmt.Fprintf(os.Stderr, "DEBUG-POST-DECODE: raw type=%v (typeOK=%v)\n", rawType, typeOK)

		// Reset init event flag for each new event
		isInitEvent = false

		// Serialize back to JSON for logging
		data, _ := json.Marshal(raw)
		line := string(data)

		if err := logRawEvent(logWriter, raw, line, extractClaudeEventText(raw)); err != nil {
			return "", err
		}

		// Check for assistant_message events with plain text content
		// These often contain the final summary JSON
		// Note: The stream-json format uses type "assistant" not "assistant_message"
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
				// DEBUG: Log that we found an assistant_message with content
				_ = logWriter.Write(LogEvent{
					Type:      "debug",
					Timestamp: time.Now().UTC(),
					Content:   fmt.Sprintf("[DEBUG] Found assistant with text content length %d, prefix: %s", len(content), safePrefix(content)),
				})

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
							_ = logWriter.Write(LogEvent{
								Type:      "debug",
								Timestamp: time.Now().UTC(),
								Content:   fmt.Sprintf("[DEBUG] Found valid summary JSON: %s", jsonStr),
							})
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
		// Skip init events - they have nested message structures that falsely trigger full message detection
		if !sawFullMessage && !isInitEvent {
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

	// Prefer the assistant_message content if it was captured
	_ = logWriter.Write(LogEvent{
		Type:      "debug",
		Timestamp: time.Now().UTC(),
		Content:   fmt.Sprintf("[DEBUG] After loop: lastAssistantContent=%q, sawFullMessage=%v, lastMessageBuf.Len=%d",
			lastAssistantContent, sawFullMessage, lastMessageBuf.Len()),
	})
	if lastAssistantContent != "" {
		if summary, ok := parseSummaryFromText(lastAssistantContent); ok {
			_ = logWriter.Write(LogEvent{
				Type:      "debug",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("[DEBUG] Parsed summary from assistant_message: %+v", summary),
			})
			if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
				return "", err
			}
			return lastAssistantContent, nil
		} else {
			_ = logWriter.Write(LogEvent{
				Type:      "debug",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("[DEBUG] parseSummaryFromText FAILED for lastAssistantContent: %s", safePrefix(lastAssistantContent)),
			})
		}
	} else {
		_ = logWriter.Write(LogEvent{
			Type:      "debug",
			Timestamp: time.Now().UTC(),
			Content:   "[DEBUG] lastAssistantContent is EMPTY, falling back to accumulated content",
		})
	}

	// Fall back to accumulated message content
	if lastMessageBuf.Len() > 0 {
		content := lastMessageBuf.String()
		_ = logWriter.Write(LogEvent{
			Type:      "debug",
			Timestamp: time.Now().UTC(),
			Content:   fmt.Sprintf("[DEBUG] Parsing summary from accumulated content (len=%d, prefix=%s)", len(content), safePrefix(content)),
		})
		if summary, ok := parseSummaryFromText(content); ok {
			_ = logWriter.Write(LogEvent{
				Type:      "debug",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("[DEBUG] Successfully parsed summary: %+v", summary),
			})
			if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
				return "", err
			}
		}
	}

	return lastMessageBuf.String(), nil
}
