# Looper

A deterministic, autonomous loop runner for AI agents (Codex, Claude). It processes exactly
one task per iteration from a JSON backlog, with fresh context each run, and
keeps a JSONL audit log for traceability.

## Quick Start

```bash
make install
looper
```

If `to-do.json` is missing, Looper bootstraps it from project docs, validates it
against `to-do.schema.json`, and repairs it if needed.

## What It Does

- Bootstraps `to-do.json` and `to-do.schema.json` if missing.
- Validates `to-do.json` (using JSON Schema when available).
- Repairs invalid task files via configured agent (Codex or Claude).
- Runs one task per iteration (doing > todo > blocked).
- When tasks are exhausted, runs a review pass; appends a final
  `project-done` marker task if no new work is found.
- Supports multiple iteration schedules (Codex, Claude, odd-even, round-robin).
- Enforces JSON output from the model and logs JSONL per run.
- Optionally applies model summaries back into `to-do.json`.

## Install

### From source

```bash
make install
```

The binary is installed to `~/.local/bin/looper` by default.

### Uninstall

```bash
make uninstall
```

### Homebrew

```bash
brew install nibzard/tap/looper
```

## Usage

```bash
# Run the loop (default command)
looper

# Run with specific todo file
looper run path/to/tasks.json

# Run with specific schedule
looper run --schedule odd-even

# List tasks by status
looper ls
looper ls todo
looper ls --status doing path/to/tasks.json

# Tail the latest log
looper tail
looper tail --follow

# Check dependencies and configuration
looper doctor

# Show version
looper version
```

## Iteration Schedules

Looper supports multiple iteration schedules for balancing agent usage:

- `--schedule codex` (default) - All iterations use Codex
- `--schedule claude` - All iterations use Claude
- `--schedule odd-even` - Odd iterations use Codex, even use Claude
- `--schedule round-robin` - Rotate through a list of agents

### Schedule Options

**Odd-even schedule:**
```bash
looper run --schedule odd-even
looper run --schedule odd-even --odd-agent claude --even-agent codex
```

**Round-robin schedule:**
```bash
# Default: claude,codex pattern
looper run --schedule round-robin

# Custom pattern (2x Claude, then Codex, repeating)
looper run --schedule round-robin --rr-agents claude,claude,codex
```

**Note:** The final review pass always uses Codex, regardless of iteration schedule.

### Repair Agent

The agent used for repair operations can be configured independently:

```bash
looper run --repair-agent claude
```

## Configuration

Looper uses a configuration hierarchy (lower values override higher):

1. Built-in defaults
2. Config file (`looper.toml` in current directory)
3. Environment variables
4. CLI flags

### Config File

Create `looper.toml` in your project directory:

```toml
# Paths
todo_file = "to-do.json"
schema_file = "to-do.schema.json"
log_dir = "~/.looper"

# Loop settings
max_iterations = 50
schedule = "codex"  # codex|claude|odd-even|round-robin
repair_agent = "codex"

# Odd-even schedule options
odd_agent = "codex"   # agent for odd iterations
even_agent = "claude" # agent for even iterations

# Round-robin schedule options
rr_agents = ["claude", "codex"]  # agent rotation list

# Agents
[agents.codex]
binary = "codex"
model = ""

[agents.claude]
binary = "claude"
model = ""

# Output
apply_summary = true

# Git
git_init = true

# Hooks
hook_command = "/path/to/hook.sh"

# Delay between iterations
loop_delay_seconds = 0
```

### Environment Variables

- `LOOPER_TODO` - Task file path (default: `to-do.json`)
- `LOOPER_SCHEMA` - Schema file path
- `LOOPER_BASE_DIR` / `LOOPER_LOG_DIR` - Log directory
- `LOOPER_MAX_ITERATIONS` - Maximum iterations
- `LOOPER_ITER_SCHEDULE` / `LOOPER_SCHEDULE` - Iteration schedule
- `LOOPER_REPAIR_AGENT` - Agent for repair operations
- `LOOPER_ITER_ODD_AGENT` - Agent for odd iterations
- `LOOPER_ITER_EVEN_AGENT` - Agent for even iterations
- `LOOPER_ITER_RR_AGENTS` - Comma-separated agent list for round-robin
- `LOOPER_APPLY_SUMMARY` - Apply summaries to task file (1/0)
- `LOOPER_GIT_INIT` - Initialize git repo if missing (1/0)
- `LOOPER_HOOK` - Hook command to run after each iteration
- `LOOPER_LOOP_DELAY` - Delay between iterations (seconds)
- `CODEX_BIN` / `CLAUDE_BIN` - Agent binary paths
- `CODEX_MODEL` / `CLAUDE_MODEL` - Model selection

### CLI Flags

Run `looper help` or `looper run --help` for a complete list of flags.

## Task File (to-do.json)

`to-do.json` is the source of truth for the loop and must match
`to-do.schema.json`. The loop chooses a single task each iteration:

1) Any task in `doing` (lowest id wins)
2) Otherwise, highest priority `todo` (priority 1 is highest)
3) Otherwise, highest priority `blocked`

Minimal example:

```json
{
  "schema_version": 1,
  "source_files": ["README.md"],
  "tasks": [
    {
      "id": "T1",
      "title": "Add README",
      "priority": 1,
      "status": "todo"
    }
  ]
}
```

## Logs and Output

Logs are stored per project under:

```
~/.looper/<project>-<hash>/
```

Each run produces:

- `<timestamp>-<pid>.jsonl` - Full JSONL trace
- `<timestamp>-<pid>-<label>.last.json` - The last assistant message

`looper tail --follow` watches the latest run in real time.

## Hooks

Set the `hook_command` in config or use `--hook` / `LOOPER_HOOK` to run a script after each iteration.

The hook receives these arguments:

```
<task_id> <status> <last_message_json_path> <label>
```

## Git Behavior

If the project is not a git repo and `git_init` is true, Looper runs `git init`.
If git is unavailable or init fails, the agent runs with `--skip-git-repo-check`.

## Dev Mode (Prompt Development)

For prompt development and debugging, set `LOOPER_PROMPT_MODE=dev` to enable:

- `--prompt-dir` - Override the prompt directory
- `--print-prompt` - Print rendered prompts before running

Example:

```bash
LOOPER_PROMPT_MODE=dev looper run --print-prompt --prompt-dir ./my-prompts
```

**Note:** Dev mode is intentionally hidden from normal usage and not shown in help output unless enabled.

## Building

```bash
make build    # Build for current platform
make test     # Run tests
make smoke    # Run smoke test
```

## Architecture

Looper is written in Go with a clean, testable core:

- `cmd/looper` - CLI entrypoint
- `internal/config` - Configuration loading
- `internal/prompts` - Prompt store and rendering
- `internal/todo` - Task file parsing and validation
- `internal/loop` - Orchestration state machine
- `internal/agents` - Codex/Claude runners
- `internal/logging` - JSONL logging and tailing
- `internal/hooks` - Hook invocation

## Migration from Shell Version

See [MIGRATION.md](MIGRATION.md) for details on migrating from the previous shell script implementation.
