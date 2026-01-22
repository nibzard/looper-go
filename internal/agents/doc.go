// Package agents defines Codex and Claude runners.
//
// The package provides an Agent interface for running AI agent commands
// with streaming log output and timeout support.
//
// Agent types are registered via RegisterAgent. Built-ins include:
//   - codex: Runs Codex CLI with --json flag
//   - claude: Runs Claude CLI with --output-format stream-json
//
// Custom agent types can be registered to integrate additional CLIs.
//
// Log events are streamed to a LogWriter implementation, allowing real-time
// observability of agent execution. Each event includes a type, timestamp,
// and relevant content.
//
// Default timeout is 30 minutes, configurable per agent.
package agents
