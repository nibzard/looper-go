package parallel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
)

// TaskResult represents the result of executing a task.
type TaskResult struct {
	TaskID   string
	Summary  *agents.Summary
	Error    error
	Duration time.Duration
}

// WorkerPool manages concurrent task execution with bounded concurrency.
type WorkerPool struct {
	maxWorkers int
	semaphore  chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	results    []TaskResult
	errors     []error
	failFast   bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewWorkerPool creates a new worker pool with bounded concurrency.
// If maxWorkers is 0, unlimited workers are allowed (bounded by available tasks).
// If failFast is true, the context will be cancelled on the first error.
func NewWorkerPool(ctx context.Context, maxWorkers int, failFast bool) *WorkerPool {
	ctx, cancel := context.WithCancel(ctx)
	return &WorkerPool{
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
		failFast:   failFast,
		ctx:        ctx,
		cancel:     cancel,
		results:    make([]TaskResult, 0),
	}
}

// Submit submits a task for execution. If the pool is at capacity,
// this will block until a worker becomes available or the context is cancelled.
func (p *WorkerPool) Submit(taskID string, fn func() (*agents.Summary, error)) {
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Acquire semaphore slot
		if p.maxWorkers > 0 {
			select {
			case p.semaphore <- struct{}{}:
				defer func() { <-p.semaphore }()
			case <-p.ctx.Done():
				return
			}
		}

		// Check if we should still run (fail-fast or cancelled)
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		start := time.Now()
		summary, err := fn()
		duration := time.Since(start)

		result := TaskResult{
			TaskID:   taskID,
			Summary:  summary,
			Error:    err,
			Duration: duration,
		}

		p.mu.Lock()
		defer p.mu.Unlock()

		p.results = append(p.results, result)
		if err != nil {
			p.errors = append(p.errors, fmt.Errorf("%s: %w", taskID, err))
			if p.failFast {
				p.cancel()
			}
		}
	}()
}

// Wait waits for all submitted tasks to complete and returns the results.
// If failFast was enabled and an error occurred, the context may have been
// cancelled and some tasks may not have completed.
func (p *WorkerPool) Wait() ([]TaskResult, []error) {
	p.wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()

	// Cancel the context to clean up
	p.cancel()

	results := make([]TaskResult, len(p.results))
	copy(results, p.results)

	errors := make([]error, len(p.errors))
	copy(errors, p.errors)

	return results, errors
}

// Results returns a snapshot of current results without waiting.
// This is safe to call from multiple goroutines.
func (p *WorkerPool) Results() []TaskResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	results := make([]TaskResult, len(p.results))
	copy(results, p.results)
	return results
}

// Errors returns a snapshot of current errors without waiting.
// This is safe to call from multiple goroutines.
func (p *WorkerPool) Errors() []error {
	p.mu.Lock()
	defer p.mu.Unlock()

	errors := make([]error, len(p.errors))
	copy(errors, p.errors)
	return errors
}

// Cancel cancels all pending work in the pool.
func (p *WorkerPool) Cancel() {
	p.cancel()
}
