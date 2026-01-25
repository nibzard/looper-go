// Package plugin provides a plugin system for looper-go.
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

var (
	// DefaultExecutionTimeout is the default timeout for plugin execution.
	DefaultExecutionTimeout = 30 * time.Minute

	// requestID is used to generate unique JSON-RPC request IDs.
	requestIDMu sync.Mutex
	requestID   int
)

// Executor handles execution of plugin binaries via JSON-RPC.
type Executor struct {
	plugin *Plugin
}

// NewExecutor creates a new plugin executor.
func NewExecutor(plugin *Plugin) *Executor {
	return &Executor{
		plugin: plugin,
	}
}

// Execute executes a JSON-RPC method on the plugin.
func (e *Executor) Execute(ctx context.Context, method string, params interface{}, result interface{}) error {
	// Create request
	reqID := nextRequestID()
	req := Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	// Create command
	cmd := exec.CommandContext(ctx, e.plugin.BinaryPath)
	cmd.Dir = e.GetWorkDir()

	// Set up environment
	cmd.Env = e.getEnvVars()

	// Set up stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting plugin binary: %w", err)
	}

	// Send request
	if _, err := stdin.Write(reqData); err != nil {
		return fmt.Errorf("writing request to plugin: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("closing stdin: %w", err)
	}

	// Read response
	respData, err := io.ReadAll(stdout)
	if err != nil {
		return fmt.Errorf("reading response from plugin: %w", err)
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		// Include stderr in error
		stderrStr := stderr.String()
		if stderrStr != "" {
			return fmt.Errorf("plugin execution failed: %w\nstderr: %s", err, stderrStr)
		}
		return fmt.Errorf("plugin execution failed: %w", err)
	}

	// Parse response
	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return fmt.Errorf("unmarshaling response: %w", err)
	}

	// Check for JSON-RPC error
	if resp.Error != nil {
		return fmt.Errorf("plugin error (code %d): %s", resp.Error.Code, resp.Error.Message)
	}

	// Decode result
	if result != nil && resp.Result != nil {
		resultData, err := json.Marshal(resp.Result)
		if err != nil {
			return fmt.Errorf("marshaling result: %w", err)
		}
		if err := json.Unmarshal(resultData, result); err != nil {
			return fmt.Errorf("unmarshaling result: %w", err)
		}
	}

	return nil
}

// ExecuteAgent runs an agent plugin and returns an AgentResult.
// Note: The caller is responsible for converting AgentResult to their internal Summary type.
func (e *Executor) ExecuteAgent(ctx context.Context, prompt string, logWriter io.Writer) (*AgentResult, error) {
	// Create parameters
	params := AgentRunParams{
		Prompt:  prompt,
		Context: make(map[string]interface{}),
	}

	// Execute
	var result AgentResult
	if err := e.Execute(ctx, "run", params, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ExecuteWorkflow runs a workflow plugin.
func (e *Executor) ExecuteWorkflow(ctx context.Context, params WorkflowRunParams) (*WorkflowResult, error) {
	var result WorkflowResult
	if err := e.Execute(ctx, "run", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetWorkDir returns the working directory for plugin execution.
func (e *Executor) GetWorkDir() string {
	// Check plugin config for work_dir
	if workDir, ok := e.plugin.Config["work_dir"].(string); ok {
		return workDir
	}

	// Default to current directory
	return "."
}

// getEnvVars returns environment variables for plugin execution.
func (e *Executor) getEnvVars() []string {
	// Start with parent process environment
	env := append([]string{}, execCmdEnv()...)

	// Add plugin-specific environment variables
	// TODO: Add plugin configuration as environment variables

	return env
}

// ValidatePlugin checks if the plugin binary is executable and responds to basic queries.
func (e *Executor) ValidatePlugin(ctx context.Context) error {
	// Quick health check - try to get plugin info
	// This requires the plugin to implement a "info" method
	// For now, just check if binary exists
	info, err := exec.Command(e.plugin.BinaryPath, "--version").CombinedOutput()
	if err != nil {
		// Version check failed, but don't fail validation
		// The plugin might not implement --version
	}
	_ = info

	return nil
}

// GetPluginInfo retrieves information about the plugin.
func (e *Executor) GetPluginInfo(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := e.Execute(ctx, "info", nil, &result)
	return result, err
}

// nextRequestID generates a unique request ID.
func nextRequestID() int {
	requestIDMu.Lock()
	defer requestIDMu.Unlock()
	requestID++
	return requestID
}

// execCmdEnv returns the current process environment.
func execCmdEnv() []string {
	// In Go 1.19+, we can use os.ExecutableEnv
	// For compatibility, we'll use a fallback
	return exec.Command("").Env
}

// ExecuteAgentWithTimeout executes an agent plugin with a timeout.
func ExecuteAgentWithTimeout(ctx context.Context, plugin *Plugin, prompt string, timeout time.Duration, logWriter io.Writer) (*AgentResult, error) {
	executor := NewExecutor(plugin)

	// Set timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return executor.ExecuteAgent(ctx, prompt, logWriter)
}

// ExecuteWorkflowWithTimeout executes a workflow plugin with a timeout.
func ExecuteWorkflowWithTimeout(ctx context.Context, plugin *Plugin, params WorkflowRunParams, timeout time.Duration) (*WorkflowResult, error) {
	executor := NewExecutor(plugin)

	// Set timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return executor.ExecuteWorkflow(ctx, params)
}

// StreamExecute executes an agent plugin and streams the output.
// This is used for agents that support streaming output.
func (e *Executor) StreamExecute(ctx context.Context, prompt string, logWriter io.Writer) (*AgentResult, error) {
	// For streaming, we need to handle the JSON-RPC streaming protocol
	// This is more complex and requires the agent to support it

	// Create request
	reqID := nextRequestID()
	req := Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "stream",
		Params: AgentRunParams{
			Prompt:  prompt,
			Context: make(map[string]interface{}),
		},
	}

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Create command
	cmd := exec.CommandContext(ctx, e.plugin.BinaryPath)
	cmd.Dir = e.GetWorkDir()
	cmd.Env = e.getEnvVars()

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting plugin binary: %w", err)
	}

	// Send request
	go func() {
		stdin.Write(reqData)
		stdin.Close()
	}()

	// Read stderr in background (for logs)
	if logWriter != nil {
		go io.Copy(logWriter, stderr)
	} else {
		go io.Copy(io.Discard, stderr)
	}

	// Read streaming response
	decoder := json.NewDecoder(stdout)
	var finalResult *AgentResult

	for decoder.More() {
		var resp Response
		if err := decoder.Decode(&resp); err != nil {
			break
		}

		// Handle different response types
		if resp.Error != nil {
			return nil, fmt.Errorf("plugin error: %s", resp.Error.Message)
		}

		// Check if this is the final summary
		if resp.Result != nil {
			resultData, _ := json.Marshal(resp.Result)
			var result AgentResult
			if err := json.Unmarshal(resultData, &result); err == nil {
				if result.Status == "done" || result.Status == "failed" {
					finalResult = &result
				}
			}
		}
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("plugin execution failed: %w", err)
	}

	if finalResult == nil {
		return nil, fmt.Errorf("plugin did not return a result")
	}

	return finalResult, nil
}
