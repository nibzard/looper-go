// Package main provides the adversarial workflow plugin binary.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nibzard/looper-plugin-adversarial/internal"
)

// Request represents a JSON-RPC request from looper.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a JSON-RPC response to looper.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo represents a JSON-RPC error.
type ErrorInfo struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// WorkflowRunParams are parameters for the workflow run method.
type WorkflowRunParams struct {
	Config    map[string]interface{} `json:"config"`
	WorkDir   string                 `json:"work_dir"`
	TodoFile  string                 `json:"todo_file"`
	UserPrompt string                `json:"user_prompt,omitempty"`
}

// WorkflowResult is the result returned by the workflow plugin.
type WorkflowResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func main() {
	// Read JSON-RPC request from stdin
	var req Request
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&req); err != nil {
		writeError(0, -32700, fmt.Sprintf("Parse error: %v", err))
		os.Exit(1)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		writeError(req.ID, -32600, "Invalid JSON-RPC version")
		os.Exit(1)
	}

	// Dispatch method
	switch req.Method {
	case "run":
		handleRun(req)
	case "info":
		handleInfo(req)
	default:
		writeError(req.ID, -32601, "Method not found")
		os.Exit(1)
	}
}

// handleRun handles the "run" method for workflow execution.
func handleRun(req Request) {
	// Parse params
	paramsJSON, _ := json.Marshal(req.Params)
	var params WorkflowRunParams
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		writeError(req.ID, -32602, fmt.Sprintf("Invalid params: %v", err))
		return
	}

	// Extract configuration
	coderAgent := getConfigString(params.Config, "coder_agent", "claude")
	adversaryAgent := getConfigString(params.Config, "adversary_agent", "codex")
	maxReruns := getConfigInt(params.Config, "max_reruns", 2)

	// Create and run workflow
	w := &internal.Workflow{
		CoderAgent:     coderAgent,
		AdversaryAgent: adversaryAgent,
		MaxReruns:      maxReruns,
		WorkDir:        params.WorkDir,
		TodoPath:       params.TodoFile,
	}

	result, err := w.Run()
	if err != nil {
		writeResult(req.ID, WorkflowResult{
			Success: false,
			Message: "Workflow execution failed",
			Error:   err.Error(),
		})
		return
	}

	writeResult(req.ID, result)
}

// handleInfo handles the "info" method for plugin information.
func handleInfo(req Request) {
	info := map[string]interface{}{
		"name":        "adversarial",
		"version":     "0.1.0",
		"description": "APEX-style adversarial workflow with Coder and Adversary agents",
		"author":     "nibzard",
		"capabilities": map[string]bool{
			"can_modify_files":     true,
			"can_execute_commands": true,
			"can_access_network":   false,
			"can_access_env":       true,
		},
	}
	writeResult(req.ID, info)
}

// writeResult writes a successful JSON-RPC response to stdout.
func writeResult(id int, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(resp)
}

// writeError writes a JSON-RPC error response to stdout.
func writeError(id int, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(resp)
}

// getConfigString extracts a string config value with a default.
func getConfigString(config map[string]interface{}, key, defaultValue string) string {
	if config == nil {
		return defaultValue
	}
	if val, ok := config[key].(string); ok && val != "" {
		return val
	}
	return defaultValue
}

// getConfigInt extracts an int config value with a default.
func getConfigInt(config map[string]interface{}, key string, defaultValue int) int {
	if config == nil {
		return defaultValue
	}
	switch v := config[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		// Try to parse string as int
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return defaultValue
}
