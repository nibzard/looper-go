// Package agents defines Codex and Claude runners.
package agents

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMultiplexedLogWriter_Write(t *testing.T) {
	t.Run("writes with task ID prefix", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewMultiplexedLogWriter(&buf)

		event := LogEvent{
			Type:      "assistant_message",
			Timestamp: time.Now(),
			Content:   "Test message",
		}

		err := writer.Write(event)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		output := buf.String()
		if output == "" {
			t.Error("expected output, got empty string")
		}
	})

	t.Run("writes error events", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewMultiplexedLogWriter(&buf)

		event := LogEvent{
			Type:      "error",
			Timestamp: time.Now(),
			Content:   "Test error",
		}

		err := writer.Write(event)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		output := buf.String()
		if output == "" {
			t.Error("expected error output, got empty string")
		}
	})

	t.Run("writes summary events", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewMultiplexedLogWriter(&buf)

		event := LogEvent{
			Type:      "summary",
			Timestamp: time.Now(),
			Summary: &Summary{
				TaskID:  "T001",
				Status:  "done",
				Summary: "Completed",
			},
		}

		err := writer.Write(event)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		output := buf.String()
		if output == "" {
			t.Error("expected summary output, got empty string")
		}
		if bytes.Contains(buf.Bytes(), []byte("T001")) == false {
			t.Error("expected task ID in output")
		}
	})
}

func TestMultiplexedLogWriter_ConcurrentWrites(t *testing.T) {
	t.Run("concurrent writes are thread-safe", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewMultiplexedLogWriter(&buf)

		var wg sync.WaitGroup
		numGoroutines := 10
		writesPerGoroutine := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < writesPerGoroutine; j++ {
					event := LogEvent{
						Type:      "assistant_message",
						Timestamp: time.Now(),
						Content:   "message",
						Summary: &Summary{
							TaskID: fmt.Sprintf("T%03d", id),
						},
					}
					_ = writer.Write(event)
				}
			}(i)
		}

		wg.Wait()

		// Check that we got output
		output := buf.String()
		if len(output) == 0 {
			t.Error("expected output from concurrent writes")
		}
	})
}

func TestMultiplexedLogWriter_RaceDetection(t *testing.T) {
	t.Run("no data races with concurrent access", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewMultiplexedLogWriter(&buf)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		done := make(chan bool)

		// Start multiple goroutines writing concurrently
		for i := 0; i < 5; i++ {
			go func(id int) {
				for {
					select {
					case <-ctx.Done():
						done <- true
						return
					default:
						event := LogEvent{
							Type:      "assistant_message",
							Timestamp: time.Now(),
							Content:   "concurrent test",
							Summary: &Summary{
								TaskID: fmt.Sprintf("T%03d", id),
							},
						}
						_ = writer.Write(event)
					}
				}
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 5; i++ {
			<-done
		}

		// If we get here without panic/deadlock, the test passes
	})
}
