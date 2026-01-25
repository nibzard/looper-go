// Package main is the JSON-RPC wrapper for Amp Code agent.
// This wrapper translates between looper's JSON-RPC protocol and Amp's CLI.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
)

// Version is the plugin version.
var Version = "1.0.0"

// JSON-RPC 2.0 types
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *ErrorObj   `json:"error,omitempty"`
}

type ErrorObj struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Agent params
type AgentRunParams struct {
	Prompt  string                 `json:"prompt"`
	Context map[string]interface{} `json:"context"`
}

// Agent result
type AgentResult struct {
	TaskID   string   `json:"task_id"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Files    []string `json:"files,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

// Plugin info result
type PluginInfo struct {
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Binary   string            `json:"binary"`
	Features map[string]bool   `json:"features"`
	AmpStatus string           `json:"amp_status,omitempty"`
}

// requestID counter
var requestID int64

func nextID() int {
	return int(atomic.AddInt64(&requestID, 1))
}

func main() {
	// Read request from stdin
	var req Request
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&req); err != nil {
		sendError(0, -32700, "Parse error: "+err.Error())
		os.Exit(1)
	}

	var result interface{}
	var err error

	switch req.Method {
	case "run":
		result, err = handleRun(req)
	case "stream":
		result, err = handleStream(req)
	case "info":
		result, err = handleInfo(req)
	default:
		err = fmt.Errorf("unknown method: %s", req.Method)
	}

	if err != nil {
		sendError(req.ID, -32603, err.Error())
		os.Exit(1)
	}

	sendResult(req.ID, result)
}

// handleRun handles the "run" method - non-streaming execution
func handleRun(req Request) (*AgentResult, error) {
	var params AgentRunParams
	if err := unmarshalParams(req.Params, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// Build amp command with non-interactive flags
	// --dangerously-allow-all is the equivalent of Claude's --dangerously-skip-permissions
	args := []string{
		"--execute", params.Prompt,
		"--dangerously-allow-all",
	}

	// Execute amp
	cmd := exec.Command("amp", args...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("amp execution failed: %w\nOutput: %s", err, string(output))
	}

	// Parse Amp's output to extract summary
	return parseAmpOutput(string(output)), nil
}

// handleStream handles the "stream" method - streaming execution
func handleStream(req Request) (*AgentResult, error) {
	var params AgentRunParams
	if err := unmarshalParams(req.Params, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// Build amp command with streaming flags
	// Note: Amp doesn't have --model flag. Models are selected via mode (smart/rush/large)
	// which is configured via command palette in interactive use, not CLI flags.
	args := []string{
		"--execute", params.Prompt,
		"--stream-json",
		"--dangerously-allow-all",
	}

	cmd := exec.Command("amp", args...)

	// Set up pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start amp: %w", err)
	}

	// Stream events from stdout
	scanner := bufio.NewScanner(stdout)
	var finalResult *AgentResult
	var accumulatedContent strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse each NDJSON line
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Not JSON, treat as content
			fmt.Println(line) // Forward to looper
			accumulatedContent.WriteString(line)
			accumulatedContent.WriteString("\n")
			continue
		}

		// Forward event to looper (as NDJSON)
		jsonData, _ := json.Marshal(event)
		fmt.Println(string(jsonData))

		// Extract final result from the event
		if result := extractResultFromEvent(event, &accumulatedContent); result != nil {
			finalResult = result
		}
	}

	// Drain stderr to logs
	stderrScanner := bufio.NewScanner(stderr)
	for stderrScanner.Scan() {
		// Prefix stderr lines to distinguish from stdout events
		fmt.Fprintf(os.Stderr, "[amp stderr] %s\n", stderrScanner.Text())
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("amp wait: %w", err)
	}

	// If we didn't get a proper result, create one from accumulated content
	if finalResult == nil {
		finalResult = &AgentResult{
			TaskID:  generateTaskID(),
			Status:  "done",
			Summary: accumulatedContent.String(),
		}
	}

	return finalResult, nil
}

// handleInfo handles the "info" method - returns plugin information
func handleInfo(req Request) (*PluginInfo, error) {
	// Get amp version
	cmd := exec.Command("amp", "--version")
	_, err := cmd.CombinedOutput()
	ampVersion := "unknown"
	if err == nil {
		ampVersion = "installed"
	}

	return &PluginInfo{
		Name:    "amp",
		Version: Version,
		Binary:  "amp",
		Features: map[string]bool{
			"streaming":   true,
			"tools":       true,
			"mcp":         true,
			"multi_model": true,
		},
		AmpStatus: ampVersion,
	}, nil
}

// parseAmpOutput parses Amp's output to extract a summary.
// Amp can output plain text or JSON (from --stream-json)
func parseAmpOutput(output string) *AgentResult {
	output = strings.TrimSpace(output)

	// Try to parse as JSON first
	var result AgentResult
	if err := json.Unmarshal([]byte(output), &result); err == nil {
		if result.Status != "" {
			return &result
		}
	}

	// Try to extract JSON from markdown code blocks
	if jsonStr := extractJSON(output); jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
			if result.Status != "" {
				return &result
			}
		}
	}

	// Fall back to plain text result
	return &AgentResult{
		TaskID:  generateTaskID(),
		Status:  "done",
		Summary: output,
	}
}

// extractResultFromEvent extracts an AgentResult from an Amp stream event.
func extractResultFromEvent(event map[string]interface{}, content *strings.Builder) *AgentResult {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "result":
		// Final result event from Amp
		if subtype, _ := event["subtype"].(string); subtype == "success" {
			return &AgentResult{
				TaskID:  extractString(event, "session_id"),
				Status:  "done",
				Summary: extractString(event, "result"),
			}
		} else if subtype == "error" {
			return &AgentResult{
				TaskID:  extractString(event, "session_id"),
				Status:  "failed",
				Summary: extractString(event, "error"),
			}
		}

	case "assistant":
		// Assistant message - might contain the final answer
		if message, ok := event["message"].(map[string]interface{}); ok {
			if messageContent, ok := message["content"].([]interface{}); ok {
				for _, c := range messageContent {
					if contentBlock, ok := c.(map[string]interface{}); ok {
						if contentType, _ := contentBlock["type"].(string); contentType == "text" {
							if text, _ := contentBlock["text"].(string); text != "" {
								// Check if it's a JSON summary
								if result := tryParseSummary(text); result != nil {
									return result
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// tryParseSummary attempts to parse text as an AgentResult JSON.
func tryParseSummary(text string) *AgentResult {
	// Try direct JSON parse
	var result AgentResult
	if err := json.Unmarshal([]byte(text), &result); err == nil {
		if result.Status != "" && result.TaskID != "" {
			return &result
		}
	}

	// Try extracting from code block
	if jsonStr := extractJSON(text); jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
			if result.Status != "" {
				return &result
			}
		}
	}

	return nil
}

// extractJSON extracts JSON from markdown code blocks.
func extractJSON(s string) string {
	// Look for ```json ... ``` blocks
	start := strings.Index(s, "```json")
	if start == -1 {
		start = strings.Index(s, "```")
		if start == -1 {
			return ""
		}
		start += 3 // Skip ```
	} else {
		start += 7 // Skip ```json
	}

	end := strings.Index(s[start:], "```")
	if end == -1 {
		return ""
	}

	candidate := strings.TrimSpace(s[start : start+end])
	// Verify it looks like JSON
	if strings.HasPrefix(candidate, "{") || strings.HasPrefix(candidate, "[") {
		return candidate
	}

	return ""
}

// extractString extracts a string value from a nested map.
func extractString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// generateTaskID generates a unique task ID.
func generateTaskID() string {
	return fmt.Sprintf("T-%d", nextID())
}

// sendResult sends a successful JSON-RPC response.
func sendResult(id int, result interface{}) {
	json.NewEncoder(os.Stdout).Encode(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

// sendError sends a JSON-RPC error response.
func sendError(id int, code int, message string) {
	json.NewEncoder(os.Stdout).Encode(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObj{
			Code:    code,
			Message: message,
		},
	})
}

// unmarshalParams unmarshals params into the target.
func unmarshalParams(params interface{}, target interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
