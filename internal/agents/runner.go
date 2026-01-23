package agents

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// runAgent executes an agent using the provided spec.
// This consolidates the common run logic for all agent types.
func runAgent(ctx context.Context, cfg Config, prompt string, logWriter LogWriter, spec agentSpec) (*Summary, error) {
	logWriter = normalizeLogWriter(logWriter)

	// Build agent-specific arguments
	args := spec.buildArgs(cfg, prompt)

	// Apply timeout
	var cancel context.CancelFunc
	ctx, cancel = applyTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, cfg.Binary, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	if spec.setupStdin != nil {
		spec.setupStdin(cmd, prompt)
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
		return nil, fmt.Errorf("start %s: %w", spec.name, err)
	}

	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Stream output
	streamResult := spec.streamOutput(ctx, stdout, stderr, logWriter)

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
	summary := collectSummary(streamResult.summaries)
	var outputErrs []error
	for e := range streamResult.errs {
		outputErrs = append(outputErrs, e)
	}
	var lastMessage string
	if streamResult.lastMessage != nil {
		for msg := range streamResult.lastMessage {
			if msg != "" {
				lastMessage = msg
			}
		}
	}

	// Agent-specific post-processing
	if spec.postProcess != nil {
		postSummary, err := spec.postProcess(cfg, summary, logWriter)
		if err != nil {
			return nil, err
		}
		if postSummary != nil {
			summary = postSummary
		}
	}

	// Handle last-message file for Claude
	if cfg.LastMessagePath != "" && lastMessage != "" {
		if err := writeLastMessageFile(cfg.LastMessagePath, lastMessage, summary); err != nil {
			_ = logWriter.Write(LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("write last message: %v", err),
			})
		}
	}

	// Handle errors
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			_ = logWriter.Write(LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("%s timeout after %s", spec.name, cfg.Timeout),
			})
			return nil, fmt.Errorf("%s timeout after %s", spec.name, cfg.Timeout)
		}
		if len(outputErrs) > 0 {
			return nil, fmt.Errorf("%s failed: %w (output errors: %v)", spec.name, runErr, outputErrs)
		}
		return nil, fmt.Errorf("%s failed: %w", spec.name, runErr)
	}

	if summary == nil {
		return nil, fmt.Errorf("%s: %w", spec.name, ErrSummaryMissing)
	}

	return summary, nil
}

// collectSummary extracts the last summary from the channel.
func collectSummary(summaries <-chan *Summary) *Summary {
	var summary *Summary
	for s := range summaries {
		summary = s
	}
	return summary
}

// agentSpec captures agent-specific behavior.
type agentSpec struct {
	name         string
	buildArgs    func(cfg Config, prompt string) []string
	setupStdin   func(cmd *exec.Cmd, prompt string)
	streamOutput func(ctx context.Context, stdout, stderr io.Reader, logWriter LogWriter) agentStreamResult
	postProcess  func(cfg Config, summary *Summary, logWriter LogWriter) (*Summary, error)
}

// agentStreamResult holds streaming outputs.
type agentStreamResult struct {
	summaries   <-chan *Summary
	errs        <-chan error
	lastMessage <-chan string
}

func normalizeConfig(agentType AgentType, cfg Config) Config {
	if cfg.Binary == "" {
		switch agentType {
		case AgentTypeCodex:
			cfg.Binary = "codex"
		case AgentTypeClaude:
			cfg.Binary = "claude"
		default:
			if agentType != "" {
				cfg.Binary = string(agentType)
			}
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
