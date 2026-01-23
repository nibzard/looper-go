package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

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
	spec := agentSpec{
		name: "codex",
		buildArgs: func(cfg Config, prompt string) []string {
			args := []string{"exec", "--json"}
			if cfg.Model != "" {
				args = append(args, "-m", cfg.Model)
			}
			if cfg.Reasoning != "" {
				args = append(args, "-c", "model_reasoning_effort="+cfg.Reasoning)
			}
			args = append(args, cfg.Args...)
			if cfg.LastMessagePath != "" {
				args = append(args, "--output-last-message", cfg.LastMessagePath)
			}
			return append(args, "-")
		},
		setupStdin: func(cmd *exec.Cmd, prompt string) {
			cmd.Stdin = strings.NewReader(ensurePromptTerminator(prompt))
		},
		streamOutput: func(ctx context.Context, stdout, stderr io.Reader, logWriter LogWriter) agentStreamResult {
			summaries, errs := a.streamOutput(ctx, stdout, stderr, logWriter)
			return agentStreamResult{summaries: summaries, errs: errs}
		},
		postProcess: func(cfg Config, summary *Summary, logWriter LogWriter) (*Summary, error) {
			if summary != nil {
				return summary, nil
			}
			if cfg.LastMessagePath != "" {
				if parsed, ok := parseSummaryFromFile(cfg.LastMessagePath); ok {
					_ = logWriter.Write(LogEvent{
						Type:      "summary",
						Timestamp: time.Now().UTC(),
						Summary:   parsed,
					})
					return parsed, nil
				}
			}
			return nil, nil
		},
	}
	return runAgent(ctx, a.cfg, prompt, logWriter, spec)
}

// streamOutput streams stdout and stderr from the codex process.
func (a *codexAgent) streamOutput(
	ctx context.Context,
	stdout, stderr io.Reader,
	logWriter LogWriter,
) (<-chan *Summary, <-chan error) {
	summaries := make(chan *Summary, 1)
	errs := make(chan error, 10)

	streamWithStderr(ctx, stdout, stderr, logWriter, summaries, errs, nil, func() (string, error) {
		scanner := newScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if err := a.processLine(ctx, line, logWriter, summaries); err != nil {
				sendErr(errs, err)
			}
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %w", err)
		}
		return "", nil
	})

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
		return writeAssistantLine(logWriter, line)
	}

	text := extractTextFromMessage(rawData)
	if err := logRawEvent(logWriter, rawData, line, text); err != nil {
		return err
	}

	summary, ok := parseSummaryFromRaw(rawData)
	if !ok && text != "" {
		summary, ok = parseSummaryFromText(text)
	}
	if ok {
		if err := recordSummary(ctx, logWriter, summaries, summary); err != nil {
			return err
		}
	}

	return nil
}
