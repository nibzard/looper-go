package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractJSON extracts a JSON object from a string.
// It handles markdown code blocks with json language tags.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Try to unescape if the string contains escaped characters
	// (e.g., when content comes from JSON with escaped newlines)
	if strings.Contains(s, "\\n") || strings.Contains(s, "\\\"") {
		var unescaped string
		if err := json.Unmarshal([]byte(s), &unescaped); err == nil {
			s = unescaped
		}
	}

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
	// Check for content field directly (from assistant_message events in stream-json)
	if content, ok := raw["content"].(string); ok && content != "" {
		return content
	}
	// Also check for result field (from --print mode result events)
	if result, ok := raw["result"].(string); ok && result != "" {
		return result
	}
	return ""
}

func parseSummaryFromText(text string) (*Summary, bool) {
	summaryJSON := extractJSON(text)
	if summaryJSON == "" {
		return nil, false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(summaryJSON), &raw); err != nil {
		// Try unescaping if it might be an escaped JSON string
		if strings.HasPrefix(summaryJSON, "\"") && strings.HasSuffix(summaryJSON, "\"") {
			var unescaped string
			if err := json.Unmarshal([]byte(summaryJSON), &unescaped); err == nil {
				return parseSummaryFromText(unescaped)
			}
		}
		return nil, false
	}
	return parseSummaryFromRaw(raw)
}

func writeLastMessageFile(path, message string, summary *Summary) error {
	if path == "" {
		return nil
	}
	if summary != nil {
		return writeJSONFile(path, summary)
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return nil
	}
	extracted := extractJSON(trimmed)
	if extracted != "" && json.Valid([]byte(extracted)) {
		return os.WriteFile(path, append([]byte(extracted), '\n'), 0644)
	}
	return writeJSONFile(path, map[string]string{"raw": trimmed})
}

func writeJSONFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
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
	// Check if this looks like a summary by checking for known fields.
	// task_id may be present as null, which is valid for skipped summaries.
	_, hasTaskID := raw["task_id"]
	_, hasStatus := raw["status"]
	_, hasSummaryText := raw["summary"]
	_, hasFiles := raw["files"]
	_, hasBlockers := raw["blockers"]

	if !hasTaskID && !hasStatus && !hasSummaryText && !hasFiles && !hasBlockers {
		return nil, false
	}

	normalizeSummaryTaskID(raw)

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

func normalizeSummaryTaskID(raw map[string]any) {
	if raw == nil {
		return
	}
	if _, ok := raw["task_id"]; ok && raw["task_id"] == nil {
		raw["task_id"] = ""
	}
}

func summaryHasContent(summary Summary) bool {
	// A summary is considered to have content if it has:
	// - a task_id (non-empty), or
	// - a valid status (including "skipped" which can have null task_id), or
	// - a summary text, or
	// - files, or
	// - blockers
	// Note: null task_id becomes empty string after JSON unmarshaling,
	// but a summary with status "skipped" and null task_id is valid.
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
