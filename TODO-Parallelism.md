# Implementation Plan: Explicit Parallelism for Looper-Go

## Overview

Add explicit parallelism to looper-go while maintaining backward compatibility (sequential mode as default). The design introduces a worker pool pattern for concurrent task execution with configurable concurrency limits and output modes.

## Current State (Sequential)

- Main loop in `internal/loop/loop.go:389-420` runs one task at a time
- Task selection is deterministic via `internal/todo/types.go:470-512`
- Agent execution is synchronous in `internal/agents/runner.go`
- No parallel execution infrastructure exists

## Design Approach

### Parallelism Levels (User Choice: Both)
1. **Task-Level Parallelism**: Execute multiple independent tasks concurrently
2. **Agent-Level Parallelism**: Run multiple agents per task for consensus/voting

### Configuration Defaults (User Preferences)
- `MaxAgentsPerTask`: 1 (single agent by default, configurable for multi-agent)
- `OutputMode`: `multiplexed` (interleave output with task/agent ID prefixes)
- `FailFast`: `false` (continue other tasks when one fails)

## Configuration API

Add `ParallelConfig` to `internal/config/types.go`:

```go
type ParallelConfig struct {
    Enabled        bool               `toml:"enabled"`         // Enable parallel execution
    MaxTasks       int                `toml:"max_tasks"`       // Max concurrent tasks (0 = unlimited)
    MaxAgentsPerTask int              `toml:"max_agents_per_task"` // Max agents per task (0 = unlimited)
    Strategy       ParallelStrategy   `toml:"strategy"`        // Task selection strategy
    FailFast       bool               `toml:"fail_fast"`       // Stop all on first failure (default: false)
    OutputMode     ParallelOutputMode `toml:"output_mode"`     // Output handling (default: multiplexed)
}

type ParallelStrategy string
const (
    StrategyPriority    ParallelStrategy = "priority"    // Highest priority first
    StrategyDependency  ParallelStrategy = "dependency"  // Dependency-aware
    StrategyMixed       ParallelStrategy = "mixed"       // Balance both
)

type ParallelOutputMode string
const (
    OutputMultiplexed   ParallelOutputMode = "multiplexed"  // Interleave with prefix
    OutputBuffered      ParallelOutputMode = "buffered"     // Buffer per task
    OutputSummary       ParallelOutputMode = "summary"      // Summary only
)
```

Add to `Config` struct:
```go
Parallel ParallelConfig `toml:"parallel"`
```

## New Package: `internal/parallel`

### Worker Pool (`pool.go`)

```go
type WorkerPool struct {
    maxWorkers int
    semaphore  chan struct{}
    wg         sync.WaitGroup
    mu         sync.Mutex
    results    []TaskResult
    failFast   bool
    ctx        context.Context
    cancel     context.CancelFunc
}

type TaskResult struct {
    TaskID    string
    Summary   *agents.Summary
    Error     error
    Duration  time.Duration
}

func NewWorkerPool(ctx context.Context, maxWorkers int, failFast bool) *WorkerPool
func (p *WorkerPool) Submit(taskID string, fn func() (*agents.Summary, error))
func (p *WorkerPool) Wait() ([]TaskResult, []error)
```

### Task Selector (`selector.go`)

```go
type TaskSelector struct {
    strategy ParallelStrategy
    file     *todo.File
}

func NewTaskSelector(file *todo.File, strategy ParallelStrategy) *TaskSelector
func (s *TaskSelector) SelectTasks(n int) []*todo.Task
```

### Task Executor (`executor.go`)

```go
type TaskExecutor struct {
    loop      *loop.Loop
    maxAgents int
    logWriter agents.LogWriter
}

func NewTaskExecutor(loop *loop.Loop, maxAgents int, logWriter agents.LogWriter) *TaskExecutor
func (e *TaskExecutor) Execute(ctx context.Context, task *todo.Task, agentType string) (*agents.Summary, error)
func (e *TaskExecutor) executeSingle(ctx context.Context, task *todo.Task, agentType string) (*agents.Summary, error)
func (e *TaskExecutor) executeMultiple(ctx context.Context, task *todo.Task, agentType string) (*agents.Summary, error)
```

## Output Multiplexing

Create `internal/agents/multiplex_log_writer.go`:

```go
type MultiplexedLogWriter struct {
    mu     sync.Mutex
    writer io.Writer
}

func NewMultiplexedLogWriter(w io.Writer) *MultiplexedLogWriter
func (m *MultiplexedLogWriter) Write(event LogEvent) error
```

Update `LogEvent` in `internal/agents/log_writer.go` to add:
```go
AgentID   string `json:"agent_id,omitempty"`
TaskID    string `json:"task_id,omitempty"`
Iteration int    `json:"iteration,omitempty"`
```

## Loop Integration

Update `internal/loop/loop.go`:

```go
func (l *Loop) Run(ctx context.Context) error {
    if l.cfg.Parallel.Enabled {
        return l.RunParallel(ctx)
    }
    // Existing sequential logic...
}

func (l *Loop) RunParallel(ctx context.Context) error {
    pool := parallel.NewWorkerPool(ctx, maxWorkers, l.cfg.Parallel.FailFast)
    selector := parallel.NewTaskSelector(l.todoFile, l.cfg.Parallel.Strategy)

    for iter := 1; iter <= l.cfg.MaxIterations; iter++ {
        tasks := selector.SelectTasks(maxWorkers)
        if len(tasks) == 0 {
            // Run review pass
            continue
        }

        for _, task := range tasks {
            pool.Submit(task.ID, func() (*agents.Summary, error) {
                return l.executeTask(ctx, iter, task)
            })
        }

        results, errs := pool.Wait()
        // Process results...
    }
    return nil
}
```

## CLI Flags

Add to `cmd/root.go`:
- `--parallel` - Enable parallel mode
- `--max-tasks` - Max concurrent tasks
- `--max-agents` - Max agents per task
- `--strategy` - Selection strategy (priority|dependency|mixed)
- `--fail-fast` - Stop on first error
- `--output-mode` - Output handling (multiplexed|buffered|summary)

## Implementation Steps

### Phase 1: Configuration
1. Add `ParallelConfig` to `internal/config/types.go`
2. Add CLI flags to `cmd/root.go`
3. Add env var support
4. Update example config

### Phase 2: Core Parallelism
1. Create `internal/parallel` package
2. Implement `WorkerPool` (bounded concurrency)
3. Implement `TaskSelector` (multi-task selection)
4. Implement `TaskExecutor` with both single and multi-agent support
5. Add unit tests

### Phase 3: Output Handling
1. Create `MultiplexedLogWriter`
2. Update `LogEvent` with tracking fields
3. Add concurrent write tests

### Phase 4: Loop Integration
1. Implement `RunParallel()` in `loop.go`
2. Update `Run()` to dispatch based on config
3. Add integration tests

### Phase 5: Testing & Polish
1. Add race condition tests (`-race`)
2. Verify backward compatibility
3. Performance benchmarking

## Critical Files to Modify

| File | Change |
|------|--------|
| `internal/config/types.go` | Add `ParallelConfig` struct and field to `Config` |
| `internal/loop/loop.go` | Add `RunParallel()` method, update `Run()` dispatch |
| `internal/todo/types.go` | Add `SelectMultipleTasks()` method |
| `cmd/root.go` | Add CLI flags for parallelism |
| `internal/agents/log_writer.go` | Add tracking fields to `LogEvent` |

## New Files to Create

| File | Purpose |
|------|---------|
| `internal/parallel/pool.go` | Worker pool with bounded concurrency |
| `internal/parallel/selector.go` | Multi-task selection logic |
| `internal/parallel/executor.go` | Task execution wrapper (single + multi-agent) |
| `internal/parallel/pool_test.go` | Worker pool tests |
| `internal/parallel/selector_test.go` | Selection tests |
| `internal/parallel/executor_test.go` | Executor tests |
| `internal/agents/multiplex_log_writer.go` | Concurrent log output |

## Example Configuration

### Basic Parallelism
```toml
[parallel]
enabled = true
max_tasks = 4
max_agents_per_task = 1   # Set >1 for multi-agent consensus per task
strategy = "priority"
fail_fast = false          # Continue other tasks on failure (default)
output_mode = "multiplexed"  # Interleave output with prefixes (default)
```

### Multi-Agent Consensus
```toml
[parallel]
enabled = true
max_tasks = 4
max_agents_per_task = 3   # Run 3 agents per task for voting/consensus
strategy = "mixed"
output_mode = "summary"   # Show only summaries when using multiple agents
```

CLI equivalent:
```bash
looper run --parallel --max-tasks 4
```

## Verification

1. **Sequential mode unchanged**: Run existing tests, verify no behavioral change when `enabled=false`
2. **Parallel execution**: Create test project with 10 independent tasks, verify concurrent execution
3. **Dependency respect**: Create tasks with dependencies, verify blocked tasks don't run
4. **Output correctness**: Verify log files contain all events with proper prefixes
5. **Graceful shutdown**: Send SIGINT during parallel execution, verify all workers exit cleanly
6. **Race detection**: Run tests with `go test -race ./...`
7. **Multi-agent consensus**: Set `max_agents_per_task=3`, verify voting/consensus logic works

## Backward Compatibility

- Default `enabled = false` preserves sequential behavior
- Existing `looper.toml` files work without modification
- All existing CLI flags unchanged
- Original `Run()` logic preserved
