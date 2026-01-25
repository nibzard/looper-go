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

# Workflow commands
looper workflow list              # List available workflows
looper workflow describe parallel # Describe a specific workflow

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

# Run with a specific workflow
looper run --workflow parallel
looper run --workflow code-review

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

# Show effective configuration
looper config
looper config --json  # Output in JSON format

# Clean up old log runs
looper clean --dry-run              # Show what would be deleted
looper clean --keep 10              # Keep 10 most recent runs
looper clean --age 7d               # Delete logs older than 7 days
looper clean --keep 5 --age 30d     # Keep 5 recent, delete any older than 30 days

# Generate shell completion script
looper completion bash  # Output bash completion to stdout
looper completion zsh   # Output zsh completion to stdout
looper completion fish  # Output fish completion to stdout
looper completion powershell  # Output PowerShell completion to stdout
```

## Shell Completion

Looper provides shell auto-completion for bash, zsh, fish, and PowerShell.

### Installation

**Bash:**
```bash
# For current session
looper completion bash | source

# For persistent completion, add to ~/.bashrc or ~/.bash_profile
echo 'source <(looper completion bash)' >> ~/.bashrc
```

**Zsh:**
```bash
# For current session
looper completion zsh | source

# For persistent completion, add to ~/.zshrc
echo 'source <(looper completion zsh)' >> ~/.zshrc

# Or place the completion script in the zsh completion directory
looper completion zsh > ~/.zsh/completion/_looper
```

**Fish:**
```bash
# For persistent completion, add to fish config directory
looper completion fish > ~/.config/fish/completions/looper.fish
```

**PowerShell:**
```powershell
# For current session
looper completion powershell | Invoke-Expression

# For persistent completion, add to PowerShell profile
looper completion powershell >> $PROFILE
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

### Role-Based Agent Configuration

Looper supports role-based agent configuration that allows you to assign different agents to different phases of the loop. This is more powerful than simple iteration scheduling as it gives you fine-grained control over which agent handles each operation.

#### Available Roles

- **iter** - Agent used for main task iterations (overrides `schedule` when set)
- **review** - Agent used for the review pass when no tasks are found
- **repair** - Agent used for repairing invalid task files
- **bootstrap** - Agent used for creating the initial task file

#### Configuration via Config File

The preferred way to configure roles is via the `[roles]` section in `looper.toml`:

```toml
# Assign different agents to different loop phases
[roles]
iter = "claude"       # Use Claude for task iterations
review = "codex"      # Use Codex for review passes
repair = "claude"     # Use Claude for repair operations
bootstrap = "codex"   # Use Codex for bootstrapping
```

#### Configuration via Environment Variables

Roles can also be set via environment variables:

- `LOOPER_ROLES_ITER` - Agent for iterations
- `LOOPER_ROLES_REVIEW` - Agent for review passes
- `LOOPER_ROLES_REPAIR` - Agent for repair operations
- `LOOPER_ROLES_BOOTSTRAP` - Agent for bootstrap

```bash
export LOOPER_ROLES_ITER="claude"
export LOOPER_ROLES_REVIEW="codex"
looper run
```

#### Configuration via CLI Flags

For one-off overrides, use the step-specific CLI flags:

```bash
looper run --repair-agent claude
looper run --review-agent claude
looper run --bootstrap-agent claude
```

#### Precedence Order

When multiple configuration sources specify the same role, the precedence order is:

1. **CLI flags** (highest priority) - e.g., `--review-agent`
2. **Environment variables** - e.g., `LOOPER_ROLES_REVIEW`
3. **Project config file** - `looper.toml` [roles] section
4. **User config file** - `~/.looper/looper.toml` [roles] section
5. **Legacy config keys** - `repair_agent`, `review_agent`, `bootstrap_agent`
6. **Schedule defaults** - Uses `schedule` setting as fallback for iterations

#### When to Use Role-Based Configuration

**Use role-based configuration when:**
- You want different agents for different phases (e.g., Claude for coding, Codex for review)
- You want to override the schedule for specific operations
- You want a consistent assignment regardless of iteration count

**Use simple schedule when:**
- You want the same agent for all operations
- You want to alternate agents between iterations
- You only need simple round-robin patterns

#### Example Configurations

**Claude for development, Codex for review:**
```toml
[roles]
iter = "claude"
review = "codex"
repair = "claude"
bootstrap = "codex"
```

**Codex for everything except repair:**
```toml
[roles]
iter = "codex"
review = "codex"
bootstrap = "codex"
repair = "claude"  # Use Claude for complex repairs
```

**Use schedule for iterations, override specific roles:**
```toml
schedule = "odd-even"  # Alternate for iterations
odd_agent = "claude"
even_agent = "codex"

[roles]
review = "claude"  # Always use Claude for review
repair = "claude"  # Always use Claude for repair
```

### Step Agents

Agents for specific loop steps can also be configured via CLI flags for one-off overrides:

```bash
looper run --repair-agent claude
looper run --review-agent claude
looper run --bootstrap-agent claude
```

- **Repair agent** - Used for repairing invalid task files (default: `codex`)
- **Review agent** - Used for the review pass when no tasks are found (configurable, defaults to `codex`)
- **Bootstrap agent** - Used for creating the initial task file (default: `codex`)

**Note:** For persistent configuration, prefer the `[roles]` section in `looper.toml` over CLI flags.

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

# Role-based agent configuration (preferred over repair_agent/review_agent/bootstrap_agent)
[roles]
iter = "codex"       # Agent for task iterations (overrides schedule)
review = "codex"     # Agent for review passes
repair = "codex"     # Agent for repair operations
bootstrap = "codex"  # Agent for bootstrap

# Legacy single-step configuration (still supported, use [roles] for clarity)
# repair_agent = "codex"  # any registered agent type
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
# parser = "codex_parser.py"  # Optional: parser for agent output

[agents.claude]
binary = "claude"
model = ""
# args = ["--flag", "value"]  # Optional extra args to pass to claude
# parser = "builtin:claude"  # Optional: parser for agent output

# Custom agents can be registered and configured
# To add a custom agent, register it via init() in your code
# and configure it under the agents map:
# [agents.opencode]
# binary = "opencode"
# model = "custom-model"
# parser = "opencode_parser.py"

# Output
apply_summary = true

# Git
# git_init = true  # Run 'git init' before bootstrap when enabled

# Hooks
hook_command = "/path/to/hook.sh"

# Delay between iterations
loop_delay_seconds = 0

# Logging configuration
log_level = "info"        # Log level: debug, info, warn, error, fatal
log_format = "text"       # Log format: text, json, logfmt
log_timestamps = true     # Show timestamps in log output
log_caller = false        # Show caller location (file:line) in logs

# Parallel task execution (disabled by default for backward compatibility)
[parallel]
enabled = false              # Enable parallel task execution
max_tasks = 4                # Maximum concurrent tasks (0 = unlimited)
max_agents_per_task = 1      # Agents per task for consensus (1 = single agent)
strategy = "priority"         # Task selection: priority|dependency|mixed
fail_fast = false            # Stop all on first failure
output_mode = "multiplexed"   # Output: multiplexed|buffered|summary

# Workflow selection (default: "traditional")
workflow = "traditional"

# Workflow-specific configuration
[workflows.parallel]
max_concurrent = 3      # Max concurrent tasks (default: 3)
fail_fast = false       # Stop on first error (default: false)

[workflows.code-review]
diff_path = "."                    # Path to review (default: ".")
review_stages = ["analyze", "security", "style"]
require_approval = true            # Require manual approval
approval_file = ".looper/approval.txt"

[workflows.incident-triage]
severity_levels = ["critical", "high", "medium", "low"]
auto_assign = true
notify_slack = false
# slack_webhook = "https://hooks.slack.com/services/..."
```

Looper reads the config file from the current working directory (not the todo file directory).

### Advanced Logging Configuration

Looper provides fine-grained control over log output through four configuration options:

#### Log Level (`log_level`)

Controls the verbosity of log messages. Available levels (from most to least verbose):

- **`debug`** - Detailed information for debugging, including all internal operations
- **`info`** (default) - General informational messages about loop progress
- **`warn`** - Warning messages for potential issues
- **`error`** - Error messages only
- **`fatal`** - Critical errors that cause termination

**When to use each level:**
- Use `debug` when troubleshooting issues or developing features
- Use `info` for normal operation (default)
- Use `warn` or `error` to reduce noise in production logs
- Use `fatal` for minimal logging (rarely needed)

```bash
# Via config file
log_level = "debug"

# Via environment variable
export LOOPER_LOG_LEVEL=debug

# Via CLI flag
looper run --log-level debug
```

#### Log Format (`log_format`)

Controls how log messages are formatted:

- **`text`** (default) - Human-readable text format with colors and styling
- **`json`** - Structured JSON output, one log per line (for log aggregation tools)
- **`logfmt`** - Key-value pair format (common in cloud-native systems)

**When to use each format:**
- Use `text` for interactive terminal sessions and development (default)
- Use `json` for log aggregation systems (e.g., ELK, Splunk, CloudWatch)
- Use `logfmt` for systems that expect key-value logging

```bash
# JSON logging for production
log_format = "json"

# Via environment
export LOOPER_LOG_FORMAT=json
```

#### Log Timestamps (`log_timestamps`)

Controls whether timestamps are included in log output:

- **`true`** (default) - Include timestamps in each log message
- **`false` - Omit timestamps for cleaner output

**When to disable timestamps:**
- When logging to a system that adds its own timestamps
- For cleaner local development output
- When timing information is not needed

```bash
# Disable timestamps
log_timestamps = false

# Via environment
export LOOPER_LOG_TIMESTAMPS=0
```

#### Log Caller (`log_caller`)

Controls whether the source location (file and line number) is included:

- **`false`** (default) - Do not show source location
- **`true`** - Show `file:line` for each log message

**When to enable caller information:**
- When debugging to trace where log messages originate
- During development to identify code paths
- Not recommended for production (adds verbosity)

```bash
# Enable caller information for debugging
log_caller = true

# Via environment
export LOOPER_LOG_CALLER=1
```

#### Example Configurations

**Development/Debugging:**
```toml
# Maximum verbosity for troubleshooting
log_level = "debug"
log_format = "text"
log_timestamps = true
log_caller = true
```

**Production with Log Aggregation:**
```toml
# Structured logs for ELK/Splunk
log_level = "info"
log_format = "json"
log_timestamps = false   # Log system adds timestamps
log_caller = false
```

**Minimal Local Output:**
```toml
# Clean output for local development
log_level = "warn"
log_format = "text"
log_timestamps = false
log_caller = false
```

### Environment Variables

- `LOOPER_TODO` - Task file path (default: `to-do.json`)
- `LOOPER_SCHEMA` - Schema file path
- `LOOPER_BASE_DIR` / `LOOPER_LOG_DIR` - Log directory (supports `~`, `$HOME`, or `%USERPROFILE%` on Windows)
- `LOOPER_MAX_ITERATIONS` - Maximum iterations
- `LOOPER_ITER_SCHEDULE` / `LOOPER_SCHEDULE` - Iteration schedule (agent name or odd-even/round-robin)

**Role-based environment variables (preferred):**
- `LOOPER_ROLES_ITER` - Agent for task iterations (overrides schedule when set)
- `LOOPER_ROLES_REVIEW` - Agent for review passes
- `LOOPER_ROLES_REPAIR` - Agent for repair operations
- `LOOPER_ROLES_BOOTSTRAP` - Agent for bootstrap

**Legacy single-step environment variables (still supported):**
- `LOOPER_REPAIR_AGENT` - Agent for repair operations
- `LOOPER_REVIEW_AGENT` - Agent for review pass (default: codex)
- `LOOPER_BOOTSTRAP_AGENT` - Agent for bootstrap (default: codex)

**Schedule options:**
- `LOOPER_ITER_ODD_AGENT` - Agent for odd iterations
- `LOOPER_ITER_EVEN_AGENT` - Agent for even iterations
- `LOOPER_ITER_RR_AGENTS` - Comma-separated agent list for round-robin

**Other settings:**
- `LOOPER_APPLY_SUMMARY` - Apply summaries to task file (1/0)
- `LOOPER_GIT_INIT` - Run 'git init' before bootstrap when enabled (1/0)
- `LOOPER_HOOK` - Hook command to run after each iteration
- `LOOPER_LOOP_DELAY` - Delay between iterations (seconds)
- `LOOPER_PROMPT_DIR` - Prompt directory override (dev only, requires `LOOPER_PROMPT_MODE=dev`)
- `LOOPER_PROMPT` - User prompt for bootstrap (brainstorms tasks from your idea instead of scanning docs)
- `LOOPER_PRINT_PROMPT` - Print rendered prompts (1/0, dev only)

**Logging settings:**
- `LOOPER_LOG_LEVEL` - Log level: `debug`, `info`, `warn`, `error`, `fatal` (default: `info`)
- `LOOPER_LOG_FORMAT` - Log format: `text`, `json`, `logfmt` (default: `text`)
- `LOOPER_LOG_TIMESTAMPS` - Show timestamps in logs (1/0, default: 1)
- `LOOPER_LOG_CALLER` - Show caller location (file:line) in logs (1/0, default: 0)

**Agent settings:**
- `CODEX_BIN` / `CLAUDE_BIN` - Agent binary paths (on Windows, use `codex.exe` / `claude.exe`)
- `CODEX_MODEL` / `CLAUDE_MODEL` - Model selection
- `CODEX_REASONING` / `CODEX_REASONING_EFFORT` - Codex reasoning effort (e.g., "low", "medium", "high")
- `CODEX_ARGS` - Comma-separated extra args for Codex (e.g., `--flag,value`)
- `CLAUDE_ARGS` - Comma-separated extra args for Claude (e.g., `--flag,value`)

### CLI Flags

Global flags (place before the subcommand):
- `--todo`, `--schema`, `--log-dir`
- `--log-level`, `--log-format`, `--log-timestamps`, `--log-caller`
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

## Parallelism

Looper supports explicit parallel task execution while maintaining backward compatibility (sequential mode as default). The parallelism feature provides task-level parallelism (multiple tasks concurrently) and agent-level parallelism (multi-agent consensus per task).

### Enabling Parallelism

Parallelism is **disabled by default** for backward compatibility. Enable it via the `[parallel]` config section or CLI flag:

```toml
[parallel]
enabled = true
max_tasks = 4
```

Or via CLI:

```bash
looper run --parallel --max-tasks 4
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable parallel task execution |
| `max_tasks` | int | `0` | Maximum concurrent tasks (0 = unlimited) |
| `max_agents_per_task` | int | `1` | Agents per task for consensus (>1 = multi-agent) |
| `strategy` | string | `"priority"` | Task selection strategy |
| `fail_fast` | bool | `false` | Stop all on first failure |
| `output_mode` | string | `"multiplexed"` | Output handling mode |

### Task Selection Strategies

The `strategy` option determines how tasks are selected for parallel execution:

- **`priority`** - Select highest priority tasks first (default)
- **`dependency`** - Prioritize tasks that unblock other tasks
- **`mixed`** - Balance priority and dependency awareness

### Output Modes

The `output_mode` option controls how concurrent output is displayed:

- **`multiplexed`** - Interleave output with task/agent ID prefixes (default)
- **`buffered`** - Buffer output per task and display on completion
- **`summary`** - Show only summaries without detailed output

### Environment Variables

- `LOOPER_PARALLEL` - Enable parallel execution (1/0)
- `LOOPER_MAX_TASKS` - Maximum concurrent tasks
- `LOOPER_MAX_AGENTS_PER_TASK` - Agents per task
- `LOOPER_PARALLEL_STRATEGY` - Task selection strategy
- `LOOPER_FAIL_FAST` - Stop on first error (1/0)
- `LOOPER_OUTPUT_MODE` - Output handling mode

### Multi-Agent Consensus

Set `max_agents_per_task > 1` to run multiple agents per task for consensus/voting:

```toml
[parallel]
enabled = true
max_tasks = 2
max_agents_per_task = 3    # Run 3 agents per task for consensus
output_mode = "summary"     # Recommended for multi-agent
```

When using multi-agent consensus:
- Each agent runs independently with the same task
- Results are collected and the first successful result is used
- Use `output_mode = "summary"` to avoid verbose output

### Example Configurations

**Basic parallel execution:**
```toml
[parallel]
enabled = true
max_tasks = 4                # Run up to 4 tasks at once
max_agents_per_task = 1      # Single agent per task
strategy = "priority"         # Select highest priority first
fail_fast = false            # Continue on errors
output_mode = "multiplexed"   # Interleave output with prefixes
```

**Multi-agent consensus:**
```toml
[parallel]
enabled = true
max_tasks = 2
max_agents_per_task = 3      # Run 3 agents per task
strategy = "mixed"
output_mode = "summary"       # Show only summaries
```

**Dependency-aware execution:**
```toml
[parallel]
enabled = true
max_tasks = 0                # Unlimited (bounded by dependencies)
strategy = "dependency"      # Prioritize unblocking tasks
fail_fast = false
```

### Parallelism vs Workflows

Looper offers two parallel execution mechanisms:

1. **`[parallel]` config** - Task-level parallelism within the traditional loop
   - Works with the traditional loop
   - Executes multiple independent tasks concurrently
   - Respects task dependencies
   - Configurable via `[parallel]` section

2. **`parallel` workflow** - A complete workflow implementation
   - Selected via `workflow = "parallel"`
   - Different execution model
   - Configured via `[workflows.parallel]`

Both can be used together or independently. For most use cases, the `[parallel]` config is sufficient.

### When to Use Parallelism

**Use parallelism when:**
- You have many independent tasks
- Tasks have minimal file conflicts
- You want faster completion of large backlogs
- You need multi-agent consensus for critical tasks

**Avoid parallelism when:**
- Tasks frequently modify the same files
- Sequential task ordering is important
- Debugging intermittent issues
- Resource constraints (API rate limits, memory)

## Plugins

Looper supports a plugin system for extending functionality with external agents and workflows. Plugins are external binaries that communicate via JSON-RPC over stdin/stdout.

### Plugin Commands

```bash
# List installed plugins
looper plugin list

# Show plugin details
looper plugin info claude

# Create a new plugin skeleton
looper plugin create my-agent --type agent

# Validate a plugin
looper plugin validate ./my-plugin

# Install a plugin from git URL or local path
looper plugin install https://github.com/user/looper-plugin
looper plugin install ./my-plugin

# Uninstall a plugin
looper plugin uninstall my-agent
```

### Plugin Validation

The `looper plugin validate` command checks a plugin for correctness and completeness:

```bash
# Basic validation
looper plugin validate ./my-plugin

# Strict validation mode (enforces all rules strictly)
looper plugin validate ./my-plugin --strict

# Quick validation (manifest only, skip binary checks)
looper plugin validate ./my-plugin --quick

# Verbose output (show all validation details)
looper plugin validate ./my-plugin --verbose
```

**Validation Rules:**

1. **Manifest Checks**
   - `looper-plugin.toml` must exist in the plugin directory
   - Required fields: `name`, `version`, `category`, `plugin.binary`
   - Category must be `agent` or `workflow`
   - Plugin name must be alphanumeric with hyphens/underscores (cannot start with `-` or `_`)
   - Version should follow semver format (e.g., `1.0.0`)

2. **Category-Specific Configuration**
   - **Agent plugins** must have `[agent]` section with `type` field
   - **Workflow plugins** must have `[workflow]` section with `type` field

3. **Binary Validation**
   - Binary path must exist and be executable
   - Binary should respond to `--version` or `--help` flags
   - Warnings for script-based plugins (`.sh`, `.py`, etc.)

4. **Dependency Checks**
   - Required binaries in `PATH` are verified
   - API key dependencies are documented (warnings only)

5. **Capability Warnings**
   - Warns about dangerous capabilities (`can_execute_commands`, `can_access_network`)

**Validation Output:**

```
Plugin: my-agent
Status: VALID

Warnings:
  - binary does not respond to --version or --help
  - plugin can execute commands (ensure you trust this plugin)
```

### Plugin Creation

The `looper plugin create` command scaffolds a new plugin with all required files:

```bash
# Create an agent plugin (default)
looper plugin create my-agent

# Create a workflow plugin
looper plugin create my-workflow --type workflow

# Specify output directory
looper plugin create my-agent --output ./plugins

# Set metadata
looper plugin create my-agent --author "Your Name" --license MIT --description "My custom agent"
```

**What Gets Created:**

1. **Directory Structure**
   ```
   my-agent/
   ├── looper-plugin.toml    # Plugin manifest
   ├── README.md             # Plugin documentation
   └── bin/
       └── my-agent          # Executable binary stub
   ```

2. **Manifest File** (`looper-plugin.toml`)
   - Pre-populated with name, version, category
   - Category-specific configuration sections
   - Capability declarations
   - Default metadata fields (author, license, etc.)

3. **Binary Stub**
   - Executable shell script placeholder
   - JSON-RPC communication example
   - Response format for plugin type

4. **README.md**
   - Installation instructions
   - Usage examples
   - Plugin protocol documentation

**Example: Creating a Custom Agent**

```bash
# Create the plugin skeleton
looper plugin create my-agent --type agent --author "Jane Doe"

# The command outputs:
# Plugin "my-agent" created successfully at ./my-agent
#
# Next steps:
#   1. cd my-agent
#   2. Edit the manifest (looper-plugin.toml) if needed
#   3. Implement the plugin binary in bin/my-agent
#   4. Test: looper plugin validate .
#   5. Install: looper plugin install .

cd my-agent

# Edit the binary to implement your agent
# The binary must accept JSON-RPC requests via stdin and respond via stdout

# Validate before installing
looper plugin validate .

# Install to project plugins
looper plugin install .

# Now use your agent
looper run --schedule my-agent
```

**Plugin Binary Protocol**

Plugins communicate via JSON-RPC over stdin/stdout:

**Agent Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "run",
  "params": {
    "prompt": "...",
    "context": {...}
  }
}
```

**Agent Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "task_id": "T001",
    "status": "done",
    "summary": "...",
    "files": ["..."],
    "blockers": []
  }
}
```

**Workflow Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "run",
  "params": {
    "config": {...},
    "work_dir": "...",
    "todo_file": "..."
  }
}
```

**Workflow Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "success": true,
    "message": "..."
  }
}
```

### Plugin Locations

Plugins are searched in priority order:

1. **Project plugins** - `.looper/plugins/` (highest priority)
2. **User plugins** - `~/.looper/plugins/`
3. **Built-in plugins** - claude, codex, traditional (lowest priority)

### Plugin Manifest

Plugins require a `looper-plugin.toml` manifest file:

```toml
name = "my-agent"
version = "1.0.0"
category = "agent"
description = "My custom AI agent"

[plugin]
binary = "./bin/my-agent"
author = "Your Name"
homepage = "https://github.com/user/my-plugin"
license = "MIT"
min_looper_version = "0.1.0"

[agent]
type = "my-agent"
supports_streaming = true
supports_tools = true
default_prompt_format = "stdin"

[capabilities]
can_modify_files = true
can_execute_commands = true
can_access_network = false
can_access_env = true
```

### Plugin Categories

- **agent** - AI agent plugins (claude, codex, or custom)
- **workflow** - Workflow plugins (traditional, parallel, or custom)

### Core Plugins Bundle

Looper ships with a bundle of core plugins that are automatically extracted on first run. These core plugins provide the essential functionality out of the box without requiring manual installation.

#### Core Plugin Extraction

On the first run, looper automatically extracts core plugins to `~/.looper/plugins/`:

- **claude** - Claude AI agent integration (built-in)
- **codex** - Codex AI agent integration (built-in)
- **traditional** - Traditional looper workflow (built-in)

The extraction process:
1. Creates `~/.looper/plugins/` if it doesn't exist
2. Extracts each core plugin to its own subdirectory
3. Creates a README.md in each plugin directory with usage instructions

#### Core Plugin vs User Plugin Priority

When plugins with the same name exist in multiple locations, looper uses this priority order:

1. **Project plugins** (`.looper/plugins/`) - highest priority
2. **User plugins** (`~/.looper/plugins/`)
3. **Core plugins** (bundled) - lowest priority

This means you can override a core plugin by installing your own version to either the project or user plugins directory. For example, to use a custom version of the claude plugin:

```bash
# Install to project directory (overrides core plugin for this project only)
looper plugin install ./my-claude-plugin

# Or install to user directory (overrides core plugin globally)
mkdir -p ~/.looper/plugins
cp -r my-claude-plugin ~/.looper/plugins/claude
```

#### Core Plugin Manifests

Core plugins include embedded manifests that define their capabilities. The manifests are used to register plugins with the plugin registry and are available via `GetCoreManifests()`.

Core plugin capabilities:

**Claude Agent:**
- Supports streaming, tools, and MCP (Model Context Protocol)
- Can modify files and execute commands
- Default prompt format: stdin

**Codex Agent:**
- Supports streaming and tools
- Can modify files and execute commands
- Default prompt format: stdin

**Traditional Workflow:**
- Sequential task execution with review passes
- Supports repair operations
- Maximum 50 iterations (configurable)

#### Checking Core Plugin Status

To verify core plugins are installed:

```bash
# List all plugins (includes core plugins)
looper plugin list

# Show info about a specific core plugin
looper plugin info claude
looper plugin info traditional
```

#### Core Plugins in the Architecture

The core plugins bundle is implemented in `internal/coreplugins/`:
- `bundle.go` - Extraction logic and manifest registry
- `claude.go` - Claude agent manifest
- `codex.go` - Codex agent manifest
- `traditional.go` - Traditional workflow manifest

Core plugins are extracted automatically via `EnsureExtracted()` which is called during looper initialization. The extraction is thread-safe and only happens once per session.

### See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) - Detailed architecture documentation
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contributing guidelines

## Workflows

Looper supports a pluggable workflow system that allows you to customize how tasks are executed. Workflows define the execution strategy for processing tasks, from simple iterative loops to complex multi-stage processes.

### Available Workflows

```bash
# List all available workflows
looper workflow list

# Describe a specific workflow
looper workflow describe traditional
```

**Built-in workflows:**

- **traditional** - The default looper loop with sequential task execution, review passes, and repair
- **parallel** - Concurrent task execution with configurable concurrency limits and fail-fast option
- **code-review** - Multi-stage code review workflow with analyze, security, and style stages
- **incident-triage** - Incident classification, assignment, and notification workflow

### Running with a Workflow

Select a workflow via the `--workflow` flag or config file:

```bash
# Run with a specific workflow
looper run --workflow parallel
looper run --workflow code-review
looper run --workflow incident-triage

# Use workflow via config file (looper.toml)
workflow = "parallel"
```

### Workflow Configuration

Each workflow can be configured via `[workflows.<name>]` sections in `looper.toml`. This allows you to customize workflow behavior without modifying code.

#### Configuration Pattern

```toml
# Select workflow
workflow = "parallel"

# Configure parallel workflow
[workflows.parallel]
max_concurrent = 5      # Maximum concurrent tasks
fail_fast = true        # Stop on first error

# Configure code-review workflow
[workflows.code-review]
diff_path = "."         # Path to review (default: ".")
review_stages = ["analyze", "security", "style", "performance"]
require_approval = true
approval_file = ".looper/approval.txt"

# Configure incident-triage workflow
[workflows.incident-triage]
severity_levels = ["critical", "high", "medium", "low"]
auto_assign = true
notify_slack = false
slack_webhook = "https://hooks.slack.com/services/..."
```

#### Workflow-Specific Options

**Parallel Workflow** (`parallel`)

The parallel workflow executes multiple tasks concurrently with bounded concurrency.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `max_concurrent` | int | `3` | Maximum number of tasks to run simultaneously |
| `fail_fast` | bool | `false` | Stop execution immediately if any task fails |

Example:
```toml
[workflows.parallel]
max_concurrent = 5      # Run up to 5 tasks at once
fail_fast = true        # Stop on first error
```

Use cases:
- Large backlogs of independent tasks
- Projects where task isolation is guaranteed
- Faster completion when tasks don't depend on each other

**Code Review Workflow** (`code-review`)

Multi-stage code review with git diff analysis and optional manual approval.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `diff_path` | string | `"."` | Path to run git diff from |
| `review_stages` | array | `["analyze", "security", "style"]` | Review stages to execute |
| `require_approval` | bool | `true` | Require manual approval after review |
| `approval_file` | string | `".looper/approval.txt"` | Path to approval marker file |

Built-in stages:
- `analyze` - Code quality, structure, bugs, performance, testing
- `security` - SQL injection, XSS, auth issues, sensitive data
- `style` - Formatting, naming, comments, idiomatic patterns

Example:
```toml
[workflows.code-review]
diff_path = "src/"                    # Review only src/ directory
review_stages = ["analyze", "security", "style", "performance"]
require_approval = true
approval_file = ".looper/approval.txt"
```

**Custom Stages**: Add custom review stages by defining them in `review_stages` and configuring a stage-specific agent:

```toml
# Add a custom stage agent
[agents.agent_performance]
binary = "claude"
model = ""

[workflows.code-review]
review_stages = ["analyze", "security", "style", "performance"]
```

**Incident Triage Workflow** (`incident-triage`)

Automated incident classification, assignment, and notification.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `severity_levels` | array | `["critical", "high", "medium", "low"]` | Available severity levels |
| `auto_assign` | bool | `true` | Automatically assign based on severity |
| `notify_slack` | bool | `false` | Send Slack notifications |
| `slack_webhook` | string | `""` | Slack webhook URL |

Default assignment rules:
- `critical` → `oncall-senior`
- `high` → `oncall`
- `medium` → `team-backend`
- `low` → `backlog`

Example:
```toml
[workflows.incident-triage]
severity_levels = ["critical", "high", "medium", "low"]
auto_assign = true
notify_slack = true
slack_webhook = "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
```

**Traditional Workflow** (`traditional`)

The default looper workflow with sequential task execution. No specific configuration options - uses standard looper settings like `max_iterations`, `schedule`, `roles`, etc.

#### Discovering Configuration Options

Use `looper workflow describe` to see configuration options for a specific workflow:

```bash
looper workflow describe parallel
looper workflow describe code-review
looper workflow describe incident-triage
```

### Workflow Descriptions

#### Traditional

The default looper workflow with sequential task execution:

- Processes one task per iteration
- Runs review pass when tasks are exhausted
- Supports task repair via agent
- Configurable iteration schedules (agent selection, odd-even, round-robin)

```toml
workflow = "traditional"
```

#### Parallel

Execute tasks concurrently with bounded concurrency:

```toml
workflow = "parallel"

[workflows.parallel]
max_concurrent = 3      # Max concurrent tasks (default: 3)
fail_fast = false       # Stop on first error (default: false)
```

Use parallel workflow for:
- Independent tasks that can run simultaneously
- Faster completion of large task backlogs
- Projects where task isolation is guaranteed

#### Code Review

Multi-stage code review with git diff analysis:

```toml
workflow = "code-review"

[workflows.code-review]
diff_path = "."                    # Path to review (default: ".")
review_stages = ["analyze", "security", "style"]
require_approval = true            # Require manual approval
approval_file = ".looper/approval.txt"
```

Stages:
- **analyze** - Code quality, structure, bugs, performance, testing
- **security** - Security vulnerabilities (SQL injection, XSS, auth issues)
- **style** - Code formatting, naming, comments, idiomatic patterns

Custom stages can be added to `review_stages` with stage-specific agent configuration:

```toml
[agents.agent_performance]
binary = "claude"
model = ""

[workflows.code-review]
review_stages = ["analyze", "security", "style", "performance"]
```

#### Incident Triage

Automated incident classification and assignment:

```toml
workflow = "incident-triage"

[workflows.incident-triage]
severity_levels = ["critical", "high", "medium", "low"]
auto_assign = true
notify_slack = false
slack_webhook = "https://hooks.slack.com/services/..."
```

The workflow:
1. Finds tasks tagged with "incident"
2. Classifies severity using an AI agent
3. Auto-assigns based on severity (oncall-senior, oncall, team-backend, backlog)
4. Optionally sends Slack notifications

Assignment rules:
- **critical** → oncall-senior
- **high** → oncall
- **medium** → team-backend
- **low** → backlog

### Custom Workflow Plugins

Workflows can be implemented as external plugins. Create a workflow plugin:

```bash
looper plugin create my-workflow --type workflow
```

The plugin will be discovered automatically when placed in:
- `.looper/plugins/` (project-level)
- `~/.looper/plugins/` (user-level)

Workflow plugin manifest (`looper-plugin.toml`):

```toml
name = "my-workflow"
version = "1.0.0"
category = "workflow"

[plugin]
binary = "./bin/my-workflow"

[workflow]
type = "my-workflow"
supports_approval = false
```

See the Plugins section for more details on plugin development.

### Workflow System Architecture

The workflow system uses a registry pattern similar to agents:

```go
// Workflow interface
type Workflow interface {
    Run(context.Context) error
    Description() string
}

// Register workflows in init()
func init() {
    workflows.Register(workflows.WorkflowType("my-workflow"), MyWorkflowFactory)
}
```

Built-in workflows are in `internal/workflows/`:
- `types.go` - Core interfaces and types
- `registry.go` - Workflow registration and discovery
- `traditional.go` - Default looper loop
- `parallel.go` - Concurrent execution
- `code_review.go` - Multi-stage code review
- `incident_triage.go` - Incident response
- `plugin_workflow.go` - Plugin workflow wrapper

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

## Log Cleanup

The `looper clean` command removes old log runs by age or count, with dry-run support and clear summaries.

```bash
# Dry run - see what would be deleted
looper clean --dry-run

# Keep only the N most recent runs
looper clean --keep 10

# Delete logs older than a duration
looper clean --age 7d    # 7 days
looper clean --age 24h   # 24 hours
looper clean --age 30m   # 30 minutes

# Combine filters
looper clean --keep 5 --age 30d
```

**Options:**
- `--dry-run` - Show what would be deleted without actually deleting
- `--keep N` - Keep N most recent runs (0 = delete all)
- `--age DURATION` - Delete logs older than duration (e.g., `7d`, `24h`, `30m`)

The command will:
1. Find all log runs in the log directory
2. Group files by run ID (timestamp-pid)
3. Filter based on your criteria
4. Show a summary of what will be deleted
5. Ask for confirmation before deleting (unless using filters or `--dry-run`)

## Git Behavior

Looper can automatically initialize a git repository when `git_init` is enabled:

```bash
looper run --git-init
# Or via config: git_init = true
# Or via env: LOOPER_GIT_INIT=1
```

When `git init` is enabled and git is available, Looper runs `git init` before the bootstrap phase if the current directory is not already a git repository.

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

Looper is written in Go with a clean, modular architecture:

- `cmd/looper` - CLI entrypoint
- `internal/config` - Multi-source configuration loading (TOML, env, flags)
- `internal/prompts` - Prompt store and rendering
- `internal/todo` - Task file types and validation
- `internal/loop` - Orchestration state machine
- `internal/agents` - Modular agent system with registry pattern
- `internal/coreplugins` - Bundled core plugins (claude, codex, traditional) with auto-extraction
- `internal/parsers` - Plugin-based parser system for agent output
- `internal/workflows` - Pluggable workflow system (traditional, parallel, code-review, incident-triage)
- `internal/plugin` - Plugin management and discovery
- `internal/logging` - JSONL logging with charmbracelet/log console output
- `internal/hooks` - Hook invocation
- `internal/utils` - Shared utilities (platform, scheduling)

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed design documentation.

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

### Parser Configuration

Agents can use custom parsers for extracting summaries from their output. Parsers are configured in `looper.toml`:

```toml
[agents.claude]
binary = "claude"
parser = "builtin:claude"  # Use built-in Go parser

[agents.codex]
binary = "codex"
parser = "codex_parser.py"  # Use bundled Python parser

[agents.custom]
binary = "custom-agent"
parser = "~/.looper/parsers/custom.py"  # Use custom parser
```

Parser search paths:
1. Absolute path (if starts with `/` or `~/`)
2. `./looper-parsers/` (project-level)
3. `~/.looper/parsers/` (user-level)
4. Bundled parsers (`claude_parser.py`, `codex_parser.py`, `opencode_parser.py`)

## Migration from Shell Version

See [MIGRATION.md](MIGRATION.md) for details on migrating from the previous shell script implementation.
