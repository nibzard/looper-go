# Notes for Claude (AI Assistant)

## Testing Guidelines

### Agent Selection

**Always use the `claude` binary when running actual looper commands or integration tests.**

Unless the user explicitly specifies to use `codex`, default to `claude` for all testing scenarios.

**Rationale:**
- `claude` is the primary agent used in this codebase
- `codex` may have different behaviors/output formats
- Consistency in testing ensures reliable results

### When Running Looper

When you need to run or test the looper binary:

```bash
# Use claude by default
looper run

# Or explicitly set the agent
LOOPER_ITER_SCHEDULE=claude looper run

# Only use codex when explicitly requested
# LOOPER_ITER_SCHEDULE=codex looper run
```

### Unit Tests

Go unit tests use a custom `test-stub` agent type for simplicity. This avoids the complexity of Claude's stream-json format.

```go
cfg := &config.Config{
    Schedule:    "test-stub",
    ReviewAgent: "test-stub",
}
cfg.Agents.SetAgent("test-stub", config.Agent{Binary: stubPath})
```

The `test-stub` agent type is registered in `internal/loop/loop_test.go` and expects simple JSON output:

```bash
# Stub scripts output plain JSON
printf '{"task_id":"T001","status":"done","summary":"completed"}\n'
```

### Integration Testing

For integration testing with real agents, use `claude`:

```bash
# Run with claude
LOOPER_ITER_SCHEDULE=claude looper run

# Or set in looper.toml
schedule = "claude"
```

### Available Agents

- **claude** - Primary agent, use by default
- **codex** - Secondary agent, only when explicitly requested
- **test-stub** - Unit test helper (registered only in tests)

Do NOT use "codex" unless explicitly testing codex-specific behavior.
