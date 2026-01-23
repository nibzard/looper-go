package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/parsers"
)

// genericAgent implements Agent for any agent type using external parsers.
// It runs the agent binary and uses a parser script to extract the summary.
type genericAgent struct {
	cfg     Config
	name    string
	parser  parsers.Parser
}

// NewGenericAgent creates a new generic agent that uses external parsers.
// The parser field in cfg must be set.
func NewGenericAgent(name string, cfg Config) (Agent, error) {
	if cfg.Parser == "" {
		return nil, fmt.Errorf("agent %s: parser is required (set Parser field in config)", name)
	}

	// Create parser
	parserConfig := parsers.ParserConfig(cfg.Parser)
	p, err := parserConfig.Parser(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("create parser for %s: %w", name, err)
	}

	return &genericAgent{
		cfg:    normalizeConfig(AgentType(name), cfg),
		name:   name,
		parser: p,
	}, nil
}

// Run executes the generic agent.
func (a *genericAgent) Run(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	return a.runWithParser(ctx, prompt, logWriter)
}

func (a *genericAgent) runWithParser(ctx context.Context, prompt string, logWriter LogWriter) (*Summary, error) {
	logWriter = normalizeLogWriter(logWriter)

	// Build agent-specific arguments based on prompt format
	args := a.cfg.Args
	if a.cfg.PromptFormat == config.PromptFormatArg {
		// Pass prompt as argument
		args = append(args, prompt)
	}

	// Apply timeout
	var cancel context.CancelFunc
	ctx, cancel = applyTimeout(ctx, a.cfg.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, a.cfg.Binary, args...)
	if a.cfg.WorkDir != "" {
		cmd.Dir = a.cfg.WorkDir
	}
	// Set up stdin if prompt format is stdin
	if a.cfg.PromptFormat == config.PromptFormatStdin || a.cfg.PromptFormat == "" {
		cmd.Stdin = strings.NewReader(ensurePromptTerminator(prompt))
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
		return nil, fmt.Errorf("start %s: %w", a.name, err)
	}

	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Read all output
	outputBuf := new(bytes.Buffer)
	errs := make(chan error, 10)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := newScanner(stdout)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				outputBuf.WriteString(line)
				outputBuf.WriteString("\n")
				_ = logWriter.Write(LogEvent{
					Type:      "agent_output",
					Timestamp: time.Now().UTC(),
					Content:   line,
				})
			}
		}
		if err := scanner.Err(); err != nil {
			sendErr(errs, fmt.Errorf("stdout scanner error: %w", err))
		}
	}()

	go func() {
		defer wg.Done()
		streamStderr(ctx, stderr, logWriter, errs)
	}()

	// Wait for command to finish
	wg.Wait()
	runErr := cmd.Wait()
	exitCode := exitCodeFromError(runErr)

	// Collect any errors
	var outputErrs []error
	for e := range errs {
		outputErrs = append(outputErrs, e)
	}

	if err := logWriter.Write(LogEvent{
		Type:      "command",
		Timestamp: time.Now().UTC(),
		Command:   cmd.Args,
		ExitCode:  exitCode,
	}); err != nil {
		return nil, fmt.Errorf("write log event: %w", err)
	}

	// Use the parser to extract summary
	parserSummary, parseErr := a.parser.Parse(ctx, outputBuf.String())
	if parseErr != nil {
		if errors.Is(parseErr, parsers.ErrSummaryMissing) {
			// Parser didn't find a summary - this is expected for some agents
			// Try fallback parsing
			_ = logWriter.Write(LogEvent{
				Type:      "debug",
				Timestamp: time.Now().UTC(),
				Content:   "parser did not find summary, trying fallback",
			})
			// For now, just return ErrSummaryMissing
			return nil, ErrSummaryMissing
		}
		return nil, fmt.Errorf("parse output: %w", parseErr)
	}

	// Convert parsers.Summary to agents.Summary
	summary := &Summary{
		TaskID:   parserSummary.TaskID,
		Status:   parserSummary.Status,
		Summary:  parserSummary.Summary,
		Files:    parserSummary.Files,
		Blockers: parserSummary.Blockers,
	}

	// Handle run errors
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			_ = logWriter.Write(LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   fmt.Sprintf("%s timeout after %s", a.name, a.cfg.Timeout),
			})
			return nil, fmt.Errorf("%s timeout after %s", a.name, a.cfg.Timeout)
		}
		if len(outputErrs) > 0 {
			return nil, fmt.Errorf("%s failed: %w (output errors: %v)", a.name, runErr, outputErrs)
		}
		return nil, fmt.Errorf("%s failed: %w", a.name, runErr)
	}

	if summary == nil {
		return nil, fmt.Errorf("%s: %w", a.name, ErrSummaryMissing)
	}

	return summary, nil
}
