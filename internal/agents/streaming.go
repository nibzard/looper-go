package agents

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

func sendErr(errs chan<- error, err error) {
	if err == nil || errs == nil {
		return
	}
	select {
	case errs <- err:
	default:
	}
}

func sendString(ch chan<- string, value string) {
	if ch == nil || value == "" {
		return
	}
	select {
	case ch <- value:
	default:
	}
}

func writeAssistantLine(logWriter LogWriter, line string) error {
	return logWriter.Write(LogEvent{
		Type:      "assistant_message",
		Timestamp: time.Now().UTC(),
		Content:   line,
	})
}

func recordSummary(ctx context.Context, logWriter LogWriter, summaries chan *Summary, summary *Summary) error {
	if summary == nil {
		return nil
	}
	sendSummary(ctx, summaries, summary)
	if err := logWriter.Write(LogEvent{
		Type:      "summary",
		Timestamp: time.Now().UTC(),
		Summary:   summary,
	}); err != nil {
		return fmt.Errorf("write log event: %w", err)
	}
	return nil
}

func logRawEvent(logWriter LogWriter, raw map[string]any, line, assistantText string) error {
	eventType := classifyEventType(raw)
	event := LogEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}

	if eventType == "assistant_message" {
		if assistantText != "" {
			event.Content = assistantText
		} else {
			event.Content = line
		}
	} else {
		event.Content = line
	}

	if tool := extractToolName(raw); tool != "" {
		event.Tool = tool
	}
	if cmd, ok := extractCommand(raw); ok {
		event.Command = cmd
	}
	if exitCode, ok := extractExitCode(raw); ok {
		event.ExitCode = exitCode
	}

	if err := logWriter.Write(event); err != nil {
		return fmt.Errorf("write log event: %w", err)
	}
	return nil
}

func streamWithStderr(
	ctx context.Context,
	stdout, stderr io.Reader,
	logWriter LogWriter,
	summaries chan *Summary,
	errs chan error,
	lastMessages chan string,
	stdoutFn func() (string, error),
) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		lastMessage, err := stdoutFn()
		sendErr(errs, err)
		sendString(lastMessages, lastMessage)
	}()

	go func() {
		defer wg.Done()
		streamStderr(ctx, stderr, logWriter, errs)
	}()

	go func() {
		wg.Wait()
		close(summaries)
		close(errs)
		if lastMessages != nil {
			close(lastMessages)
		}
	}()
}

// streamStderr streams stderr lines to the log writer as error events.
// This is shared between codex and Claude agents.
func streamStderr(ctx context.Context, stderr io.Reader, logWriter LogWriter, errs chan<- error) {
	scanner := newScanner(stderr)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			_ = logWriter.Write(LogEvent{
				Type:      "error",
				Timestamp: time.Now().UTC(),
				Content:   line,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		sendErr(errs, fmt.Errorf("stderr scanner error: %w", err))
	}
}

// newScanner creates a buffered scanner with consistent settings.
func newScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, ScanBufferSize), MaxScanTokenSize)
	return scanner
}
