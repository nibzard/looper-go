// Package parallel implements task-level and agent-level parallelism for looper.
//
// It provides:
//   - WorkerPool: Bounded concurrency worker pool for parallel task execution
//   - TaskSelector: Multi-task selection based on strategy (priority, dependency, mixed)
//   - TaskExecutor: Task execution wrapper with single and multi-agent support
//
// The package supports both task-level parallelism (multiple tasks concurrently)
// and agent-level parallelism (multiple agents per task for consensus/voting).
package parallel
