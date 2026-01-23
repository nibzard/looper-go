// Package agents defines Codex and Claude runners.
package agents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nibzard/looper-go/internal/config"
)

// AgentType represents the type of agent.
type AgentType string

const (
	AgentTypeCodex  AgentType = "codex"
	AgentTypeClaude AgentType = "claude"
)

// AgentFactory creates an Agent from a Config.
type AgentFactory func(cfg Config) (Agent, error)

const (
	// MaxScanTokenSize is the maximum token size for the JSON scanner.
	// JSON events can be large (especially tool_use with large inputs).
	// 1MB provides ample headroom for complex tool calls.
	MaxScanTokenSize = 1024 * 1024

	// ScanBufferSize is the buffer size for the scanner.
	// 64KB provides good performance for typical line sizes while
	// keeping memory usage reasonable.
	ScanBufferSize = 64 * 1024

	// DefaultTimeout is the default timeout for agent execution.
	// 30 minutes allows for complex multi-step tasks without
	// hanging indefinitely on errors.
	DefaultTimeout = 30 * time.Minute
)

// Summary is the expected output from an agent run.
type Summary struct {
	TaskID   string   `json:"task_id"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Files    []string `json:"files,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

// SummaryValidationError represents an error in summary validation.
type SummaryValidationError struct {
	Path    string
	Message string
}

func (e *SummaryValidationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("summary validation failed at %s: %s", e.Path, e.Message)
	}
	return fmt.Sprintf("summary validation failed: %s", e.Message)
}

// ErrSummaryMissing indicates the agent completed without returning a summary.
var ErrSummaryMissing = errors.New("agent did not produce a summary")

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

	// Reasoning is the reasoning effort for codex (e.g., "low", "medium", "high").
	Reasoning string

	// Args are additional arguments to pass to the binary.
	Args []string

	// PromptFormat specifies how the prompt is passed to the agent (stdin or arg).
	PromptFormat config.PromptFormat

	// Parser is the parser script path (e.g., "claude_parser.py", "builtin:claude").
	// If empty, uses built-in Go parsing.
	Parser string

	// Timeout is the maximum duration to wait for the agent to complete.
	// If zero, DefaultTimeout is used. Use a negative value to disable timeouts.
	Timeout time.Duration

	// WorkDir is the working directory for the agent command.
	WorkDir string

	// LastMessagePath is an optional path to write the last message (codex only).
	LastMessagePath string
}
