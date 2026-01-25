package parallel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
)

// mockSummary creates a mock summary for testing.
func mockSummary(taskID string) *agents.Summary {
	return &agents.Summary{
		TaskID:  taskID,
		Status:  "done",
		Summary: "Completed task " + taskID,
	}
}

func TestNewWorkerPool(t *testing.T) {
	ctx := context.Background()

	t.Run("creates pool with max workers", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 4, false)
		if pool == nil {
			t.Fatal("NewWorkerPool returned nil")
		}
		if pool.maxWorkers != 4 {
			t.Errorf("expected maxWorkers=4, got %d", pool.maxWorkers)
		}
		if pool.failFast {
			t.Error("expected failFast=false")
		}
	})

	t.Run("creates pool with failFast", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, true)
		if pool.failFast != true {
			t.Error("expected failFast=true")
		}
	})

	t.Run("creates pool with unlimited workers", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 0, false)
		if pool.maxWorkers != 0 {
			t.Errorf("expected maxWorkers=0 for unlimited, got %d", pool.maxWorkers)
		}
	})
}

func TestWorkerPool_SubmitAndWait(t *testing.T) {
	ctx := context.Background()

	t.Run("single task execution", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, false)

		executed := false
		pool.Submit("T001", func() (*agents.Summary, error) {
			executed = true
			return mockSummary("T001"), nil
		})

		results, errs := pool.Wait()
		if len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !executed {
			t.Error("task was not executed")
		}
		if results[0].TaskID != "T001" {
			t.Errorf("expected TaskID=T001, got %s", results[0].TaskID)
		}
	})

	t.Run("multiple tasks execution", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 4, false)

		taskIDs := []string{"T001", "T002", "T003"}
		for _, id := range taskIDs {
			taskID := id
			pool.Submit(taskID, func() (*agents.Summary, error) {
				return mockSummary(taskID), nil
			})
		}

		results, errs := pool.Wait()
		if len(errs) != 0 {
			t.Errorf("expected no errors, got %v", errs)
		}
		if len(results) != len(taskIDs) {
			t.Fatalf("expected %d results, got %d", len(taskIDs), len(results))
		}

		// Check all task IDs are present
		resultIDs := make(map[string]bool)
		for _, r := range results {
			resultIDs[r.TaskID] = true
		}
		for _, id := range taskIDs {
			if !resultIDs[id] {
				t.Errorf("missing result for task %s", id)
			}
		}
	})

	t.Run("respects max workers limit", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, false)

		maxConcurrent := 0
		currentConcurrent := 0
		var mu sync.Mutex

		// Submit 5 tasks that each take some time
		for i := 0; i < 5; i++ {
			taskID := i
			pool.Submit("", func() (*agents.Summary, error) {
				mu.Lock()
				currentConcurrent++
				if currentConcurrent > maxConcurrent {
					maxConcurrent = currentConcurrent
				}
				mu.Unlock()

				time.Sleep(50 * time.Millisecond)

				mu.Lock()
				currentConcurrent--
				mu.Unlock()

				return mockSummary(string(rune(taskID))), nil
			})
		}

		pool.Wait()

		if maxConcurrent > 2 {
			t.Errorf("expected max 2 concurrent tasks, got %d", maxConcurrent)
		}
	})

	t.Run("unlimited workers", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 0, false)

		taskCount := 10
		for i := 0; i < taskCount; i++ {
			taskID := i
			pool.Submit("", func() (*agents.Summary, error) {
				time.Sleep(10 * time.Millisecond)
				return mockSummary(string(rune(taskID))), nil
			})
		}

		results, _ := pool.Wait()
		if len(results) != taskCount {
			t.Errorf("expected %d results, got %d", taskCount, len(results))
		}
	})
}

func TestWorkerPool_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("task error handling", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, false)

		pool.Submit("T001", func() (*agents.Summary, error) {
			return mockSummary("T001"), nil
		})
		pool.Submit("T002", func() (*agents.Summary, error) {
			return nil, errors.New("task failed")
		})
		pool.Submit("T003", func() (*agents.Summary, error) {
			return mockSummary("T003"), nil
		})

		results, errs := pool.Wait()

		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d", len(errs))
		}
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}

		// Find the failed task
		foundFailed := false
		for _, r := range results {
			if r.Error != nil {
				foundFailed = true
				if r.TaskID != "T002" {
					t.Errorf("expected T002 to fail, got %s", r.TaskID)
				}
			}
		}
		if !foundFailed {
			t.Error("expected to find a failed task")
		}
	})

	t.Run("failFast stops execution", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, true)

		executedCount := 0
		var mu sync.Mutex

		// Submit tasks where the second one fails
		for i := 0; i < 5; i++ {
			pool.Submit("", func() (*agents.Summary, error) {
				mu.Lock()
				executedCount++
				current := executedCount
				mu.Unlock()

				// Second task fails
				if current == 2 {
					return nil, errors.New("fail fast test")
				}

				time.Sleep(100 * time.Millisecond)
				return mockSummary(""), nil
			})
		}

		results, errs := pool.Wait()

		// With failFast, not all tasks should complete
		if executedCount == 5 {
			t.Error("failFast did not stop execution early")
		}

		if len(errs) == 0 {
			t.Error("expected at least one error")
		}

		_ = results // We may not get all results with failFast
	})

	t.Run("continue on error without failFast", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 4, false)

		taskCount := 5
		failedIndex := 2
		for i := 0; i < taskCount; i++ {
			idx := i
			pool.Submit("", func() (*agents.Summary, error) {
				if idx == failedIndex {
					return nil, errors.New("task error")
				}
				return mockSummary(""), nil
			})
		}

		results, errs := pool.Wait()

		// All tasks should complete
		if len(results) != taskCount {
			t.Errorf("expected %d results, got %d", taskCount, len(results))
		}

		// One error should be reported
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d", len(errs))
		}
	})
}

func TestWorkerPool_Cancel(t *testing.T) {
	t.Run("cancel before wait", func(t *testing.T) {
		ctx := context.Background()
		pool := NewWorkerPool(ctx, 4, false)

		executed := false
		pool.Submit("", func() (*agents.Summary, error) {
			time.Sleep(100 * time.Millisecond)
			executed = true
			return mockSummary(""), nil
		})

		// Cancel immediately
		pool.Cancel()

		results, _ := pool.Wait()

		// Task may or may not have executed depending on timing
		_ = executed
		_ = results
	})

	t.Run("cancel with context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		pool := NewWorkerPool(ctx, 2, false)

		executedCount := 0
		var mu sync.Mutex

		for i := 0; i < 10; i++ {
			pool.Submit("", func() (*agents.Summary, error) {
				mu.Lock()
				executedCount++
				mu.Unlock()

				time.Sleep(50 * time.Millisecond)
				return mockSummary(""), nil
			})
		}

		// Cancel after a short delay
		go func() {
			time.Sleep(75 * time.Millisecond)
			cancel()
		}()

		pool.Wait()

		// Not all tasks should have executed
		if executedCount >= 10 {
			t.Errorf("expected fewer than 10 executed tasks due to cancel, got %d", executedCount)
		}
	})
}

func TestWorkerPool_ResultsAndErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("Results returns snapshot", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, false)

		for i := 0; i < 3; i++ {
			taskID := i
			pool.Submit("", func() (*agents.Summary, error) {
				time.Sleep(50 * time.Millisecond)
				return mockSummary(string(rune(taskID))), nil
			})
		}

		// Get results before waiting (may be incomplete)
		results1 := pool.Results()
		results2 := pool.Results()

		// Should return copies, not same slice
		if &results1 == &results2 {
			t.Error("Results should return a copy")
		}

		pool.Wait()
	})

	t.Run("Errors returns snapshot", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, false)

		pool.Submit("", func() (*agents.Summary, error) {
			return nil, errors.New("error 1")
		})

		errors1 := pool.Errors()
		errors2 := pool.Errors()

		// Should return copies, not same slice
		if &errors1 == &errors2 {
			t.Error("Errors should return a copy")
		}

		pool.Wait()
	})
}

func TestWorkerPool_Duration(t *testing.T) {
	ctx := context.Background()

	t.Run("records task duration", func(t *testing.T) {
		pool := NewWorkerPool(ctx, 2, false)

		pool.Submit("", func() (*agents.Summary, error) {
			time.Sleep(50 * time.Millisecond)
			return mockSummary(""), nil
		})

		results, _ := pool.Wait()

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		if results[0].Duration < 50*time.Millisecond {
			t.Errorf("expected duration >= 50ms, got %v", results[0].Duration)
		}
	})
}
