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
- Repairs invalid task files via configured agent (any registered agent).
- Runs one task per iteration (doing > todo > blocked).
- When tasks are exhausted, runs a review pass; appends a final
  `project-done` marker task if no new work is found.
- Supports multiple iteration schedules (agent name, odd-even, round-robin).
- Enforces JSON output from the model and logs JSONL per run.
- Optionally applies model summaries back into `to-do.json`.

## Install

### From source (Unix/Linux/macOS)

```bash
make install
```

The binary is installed to `~/.local/bin/looper` by default.

### From source (Windows)

On Windows, build the binary directly with Go:

```powershell
go build -o looper.exe ./cmd/looper
```

Or use the provided build script:

```powershell
go build -ldflags "-X main.Version=dev" -o bin/looper.exe ./cmd/looper
```

The Makefile and install/uninstall scripts are Unix-only; on Windows use `go build`
or `go install` and place `looper.exe` on your PATH.

The binary can be placed anywhere on your PATH. A common location is `%USERPROFILE%\bin` or a directory added to your PATH.

To add to PATH (PowerShell):

```powershell
$env:PATH += ";$env:USERPROFILE\bin"
# To make permanent, add to System Environment Variables
```

### Uninstall

**Unix/Linux/macOS:**
```bash
make uninstall
```

**Windows:**
Simply delete the binary file.

### Homebrew (Unix/Linux/macOS only)

```bash
brew install nibzard/tap/looper
```

## Usage

```bash
# Initialize a new project
looper init

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

# Run a release workflow via the agent
looper push
looper push --agent claude
looper push -y

# Validate task file against schema
looper validate
looper validate path/to/tasks.json
looper validate --schema custom-schema.json

# Format task file with stable ordering
looper fmt
looper fmt -w      # Write formatted file back to disk
looper fmt -check  # Check if file is formatted
looper fmt -d      # Display diffs of formatting changes
```

## Iteration Schedules

Looper supports multiple iteration schedules for balancing agent usage:

- `--schedule codex` (default) - All iterations use Codex
- `--schedule claude` - All iterations use Claude
- `--schedule <agent>` - All iterations use any registered agent type
- `--schedule odd-even` - Odd iterations use one agent, even use another
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

**Note:** The final review pass uses the configured review agent (defaults to Codex if not set).

### Step Agents

Agents for specific loop steps can be configured independently:

```bash
looper run --repair-agent claude
looper run --review-agent claude
looper run --bootstrap-agent claude
```

- **Repair agent** - Used for repairing invalid task files (default: `codex`)
- **Review agent** - Used for the review pass when no tasks are found (default: `codex`)
- **Bootstrap agent** - Used for creating the initial task file (default: `codex`)

## Initializing a Project

The `looper init` command scaffolds the initial project files:

```bash
looper init
```

This creates:
- `to-do.json` - Task file with one example task
- `to-do.schema.json` - JSON Schema for validation
- `looper.toml` - Configuration file with defaults

### Init Options

```bash
# Overwrite existing files
looper init --force

# Skip creating looper.toml (use defaults or existing config)
looper init --skip-config

# Specify custom file paths
looper init --todo my-tasks.json --schema my-schema.json --config my-config.toml
```

### What Gets Created

**to-do.json** - Contains a minimal task structure:
```json
{
  "schema_version": 1,
  "project": {
    "root": "."
  },
  "source_files": ["README.md"],
  "tasks": [
    {
      "id": "T001",
      "title": "Example: Add project documentation",
      "description": "Create a README.md file documenting the project setup and usage.",
      "reference": "README.md",
      "priority": 1,
      "status": "todo"
    }
  ]
}
```

**looper.toml** - Contains commented configuration with all available options.

## Configuration

Looper uses a configuration hierarchy (later entries override earlier):

1. Built-in defaults
2. User config file (`~/.looper/looper.toml` or OS-specific config directory)
3. Project config file (`looper.toml` or `.looper.toml` in the current directory)
4. Environment variables
5. CLI flags

### Config File Locations

**User-level config** (for global defaults):
- `~/.looper/looper.toml` (preferred)
- OS-specific config directories (if `~/.looper` doesn't exist):
  - **Windows**: `%APPDATA%\looper\looper.toml`
  - **macOS**: `~/Library/Application Support/looper/looper.toml`
  - **Linux/BSD**: `$XDG_CONFIG_HOME/looper/looper.toml` or `~/.config/looper/looper.toml`

**Project-level config** (overrides user config):
- `./looper.toml` (preferred)
- `./.looper.toml`

### Config File

Create `looper.toml` (or `.looper.toml`) in your project directory:

```toml
# Paths
todo_file = "to-do.json"
schema_file = "to-do.schema.json"
log_dir = "~/.looper"

# Loop settings
max_iterations = 50
schedule = "codex"  # agent name|odd-even|round-robin
repair_agent = "codex"  # any registered agent type
# review_agent = "codex"  # any registered agent type (default: codex)
# bootstrap_agent = "codex"  # any registered agent type (default: codex)

# Odd-even schedule options
odd_agent = "codex"   # any registered agent type
even_agent = "claude" # any registered agent type

# Round-robin schedule options
rr_agents = ["claude", "codex"]  # any registered agent types

# Agents
[agents.codex]
binary = "codex"
model = ""
# reasoning = "medium"  # Optional: low, medium, or high reasoning effort
# args = ["--flag", "value"]  # Optional extra args to pass to codex

[agents.claude]
binary = "claude"
model = ""
# args = ["--flag", "value"]  # Optional extra args to pass to claude

# Custom agents can be registered and configured
# To add a custom agent, register it via init() in your code
# and configure it under the agents map:
# [agents.opencode]
# binary = "opencode"
# model = "custom-model"

# Output
apply_summary = true

# Git (currently no-op in Go CLI)
git_init = true

# Hooks
hook_command = "/path/to/hook.sh"

# Delay between iterations
loop_delay_seconds = 0
```

Looper reads the config file from the current working directory (not the todo file directory).

### Environment Variables

- `LOOPER_TODO` - Task file path (default: `to-do.json`)
- `LOOPER_SCHEMA` - Schema file path
- `LOOPER_BASE_DIR` / `LOOPER_LOG_DIR` - Log directory (supports `~`, `$HOME`, or `%USERPROFILE%` on Windows)
- `LOOPER_MAX_ITERATIONS` - Maximum iterations
- `LOOPER_ITER_SCHEDULE` / `LOOPER_SCHEDULE` - Iteration schedule (agent name or odd-even/round-robin)
- `LOOPER_REPAIR_AGENT` - Agent for repair operations
- `LOOPER_REVIEW_AGENT` - Agent for review pass (default: codex)
- `LOOPER_BOOTSTRAP_AGENT` - Agent for bootstrap (default: codex)
- `LOOPER_ITER_ODD_AGENT` - Agent for odd iterations
- `LOOPER_ITER_EVEN_AGENT` - Agent for even iterations
- `LOOPER_ITER_RR_AGENTS` - Comma-separated agent list for round-robin
- `LOOPER_APPLY_SUMMARY` - Apply summaries to task file (1/0)
- `LOOPER_GIT_INIT` - Accepted but currently unused by the Go CLI (1/0)
- `LOOPER_HOOK` - Hook command to run after each iteration
- `LOOPER_LOOP_DELAY` - Delay between iterations (seconds)
- `LOOPER_PROMPT_DIR` - Prompt directory override (dev only, requires `LOOPER_PROMPT_MODE=dev`)
- `LOOPER_PRINT_PROMPT` - Print rendered prompts (1/0, dev only)
- `CODEX_BIN` / `CLAUDE_BIN` - Agent binary paths (on Windows, use `codex.exe` / `claude.exe`)
- `CODEX_MODEL` / `CLAUDE_MODEL` - Model selection
- `CODEX_REASONING` / `CODEX_REASONING_EFFORT` - Codex reasoning effort (e.g., "low", "medium", "high")
- `CODEX_ARGS` - Comma-separated extra args for Codex (e.g., `--flag,value`)
- `CLAUDE_ARGS` - Comma-separated extra args for Claude (e.g., `--flag,value`)

### CLI Flags

Global flags (place before the subcommand):
- `--todo`, `--schema`, `--log-dir`
- `--codex-bin`, `--claude-bin`, `--codex-model`, `--claude-model`, `--codex-reasoning`, `--codex-args`, `--claude-args`

Run flags (use with `run`):
- `--max-iterations`, `--schedule`, `--odd-agent`, `--even-agent`, `--rr-agents`
- `--repair-agent`, `--review-agent`, `--bootstrap-agent`
- `--apply-summary`, `--git-init`, `--hook`, `--loop-delay`

Run `looper help` for the full list. Dev-only flags appear when `LOOPER_PROMPT_MODE=dev` is set.

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
      "description": "Create a comprehensive README documenting the project setup and usage.",
      "reference": "README.md",
      "priority": 1,
      "status": "todo"
    }
  ]
}
```

### Task Fields

- `id` (required) - Unique task identifier (e.g., "T1", "T002")
- `title` (required) - Concise, actionable summary
- `description` (optional) - Detailed explanation of what the task involves
- `reference` (optional) - Relevant file paths, URLs, or documentation links
- `priority` (required) - 1-5, where 1 is highest priority
- `status` (required) - One of: "todo", "doing", "blocked", "done"
- `details` (optional) - Additional implementation notes or constraints
- `steps` (optional) - Array of specific sub-steps for complex tasks
- `blockers` (optional) - Array of blocking reasons when status is "blocked"
- `tags` (optional) - Array of category labels (e.g., "cli", "agents")
- `files` (optional) - Array of files this task will modify
- `depends_on` (optional) - Array of task IDs this task depends on
- `created_at` (optional) - ISO 8601 timestamp
- `updated_at` (optional) - ISO 8601 timestamp

## Logs and Output

Logs are stored per project under:

**Unix/Linux/macOS:**
```
~/.looper/<project>-<hash>/
```

**Windows:**
```
%USERPROFILE%\.looper\<project>-<hash>\
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

## Release Workflow

The `looper push` command runs a release workflow via an AI agent:

```bash
looper push [options]
```

The push command will:

1. Verify git is available and you're in a git repository
2. Check for `gh` (GitHub CLI) and verify authentication
3. Offer to create a GitHub repository if no remote exists and `gh` is available
4. Run an agent with instructions to follow the release-runbook skill at `skills/release-runbook/SKILL.md`
5. Execute the release workflow: run tests, bump version, tag, push, create GitHub release, update Homebrew formula

**Options:**
- `--agent <agent>` - Agent to use for release workflow (default: codex)
- `-y` - Skip confirmation prompts

**Example:**

```bash
# Run the release workflow with codex
looper push

# Run with claude and skip confirmation
looper push --agent claude -y
```

The agent will follow the workflow steps defined in `skills/release-runbook/SKILL.md`, which includes:

- Checking repo state and verifying tools
- Determining the appropriate version bump
- Running tests
- Updating version references in files
- Committing release changes
- Creating and pushing git tags
- Creating GitHub releases
- Updating Homebrew formulas (if present)

The output is logged to the log directory with label `push` for traceability.

## Git Behavior

Looper does not auto-initialize git repositories. Run `git init` yourself if you need one.
When git is available, Looper uses it to resolve the project root for log grouping.

## Dev Mode (Prompt Development)

For prompt development and debugging, set `LOOPER_PROMPT_MODE=dev` to enable:

- `--prompt-dir` - Override the prompt directory
- `--print-prompt` - Print rendered prompts before running

Example:

```bash
LOOPER_PROMPT_MODE=dev looper run --print-prompt --prompt-dir ./my-prompts
```

**Note:** Dev mode is intentionally hidden from normal usage and not shown in help output unless enabled.
You can also set `LOOPER_PROMPT_DIR` and `LOOPER_PRINT_PROMPT=1` instead of flags.

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
- `internal/agents` - Agent registry and runners
- `internal/logging` - JSONL logging and tailing
- `internal/hooks` - Hook invocation

### Agent Registry

Looper uses a registry-based agent system that allows custom agent types to be registered and used throughout the system. Built-in agents (`codex`, `claude`) are registered at initialization, but additional agent types can be registered programmatically.

To register a custom agent type, use the `agents.RegisterAgent()` function:

```go
import "github.com/nibzard/looper-go/internal/agents"

// Define an agent type
const AgentTypeOpenCode agents.AgentType = "opencode"

// Register the agent factory
agents.RegisterAgent(AgentTypeOpenCode, func(cfg agents.Config) (agents.Agent, error) {
    return &myOpenCodeAgent{cfg: cfg}, nil
})
```

Once registered, the agent type can be used in schedules, step agents (repair/review/bootstrap), and configuration files. The `doctor` command will automatically validate and show registered agent types.

## Migration from Shell Version

See [MIGRATION.md](MIGRATION.md) for details on migrating from the previous shell script implementation.
