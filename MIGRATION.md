# Migration Guide: Shell to Go

This document describes the completed migration from the original shell script implementation (`looper.sh`) to the Go binary (`looper`).

## Status: Complete

The Go rewrite is **complete and production-ready**. The shell script version (`looper.sh`) has been fully replaced by the Go binary.

## Breaking Changes

### CLI Command Structure

**Before (shell):**
```bash
./looper.sh                      # Run loop
./looper.sh --ls todo            # List tasks
./looper.sh --tail --follow      # Tail logs
./looper.sh --doctor             # Check dependencies
```

**After (Go):**
```bash
looper                           # Run loop (default)
looper ls todo                   # List tasks
looper tail --follow             # Tail logs
looper doctor                    # Check dependencies
```

The shell script used single-dash flags for everything. The Go CLI uses subcommands with their own flags.

### Flag Changes

| Shell Flag | Go Equivalent | Notes |
|------------|---------------|-------|
| `--ls` | `ls [status]` | Now a subcommand, status is positional or via `--status` |
| `--tail` | `tail` | Now a subcommand, `-f`/`--follow` still works |
| `--doctor` | `doctor` | Now a subcommand |
| `--interleave` | `--schedule odd-even` | Renamed for clarity |
| `--iter-schedule` | `--schedule` | Shorter name |
| `--odd-agent` | `--odd-agent` | Same |
| `--even-agent` | `--even-agent` | Same |
| `--rr-agents` | `--rr-agents` | Same |
| `--repair-agent` | `--repair-agent` | Same |

**Additional Go flags:**
- `--todo`, `--schema`, `--log-dir`
- `--max-iterations`, `--apply-summary`, `--git-init`, `--hook`, `--loop-delay`
- `--codex-bin`, `--claude-bin`, `--codex-model`, `--claude-model`

### Configuration

**New:** Go version supports `looper.toml` or `.looper.toml` in the current directory (shell had none).

**Before:** Shell script relied entirely on environment variables.

**After:** Go version supports:
1. Built-in defaults
2. `looper.toml` or `.looper.toml` in project directory
3. Environment variables
4. CLI flags

### Environment Variables

Most environment variables remain the same. The Go CLI also accepts:

- `LOOPER_LOG_DIR` - Explicit log directory override (alias of `LOOPER_BASE_DIR`)
- `LOOPER_SCHEDULE` - Alias for `LOOPER_ITER_SCHEDULE`
- `LOOPER_REVIEW_AGENT` - Agent for review pass (default: codex)
- `LOOPER_BOOTSTRAP_AGENT` - Agent for bootstrap (default: codex)
- `LOOPER_APPLY_SUMMARY` - Apply summaries to task file (1/0)
- `LOOPER_GIT_INIT` - Accepted but currently unused by the Go CLI (1/0)
- `LOOPER_LOOP_DELAY` - Delay between iterations (seconds)
- `LOOPER_PROMPT_DIR` - Prompt directory override (dev only; requires `LOOPER_PROMPT_MODE=dev`)
- `LOOPER_PRINT_PROMPT` - Print rendered prompts (1/0, dev only)
- `LOOPER_HOOK` - Hook command to run after each iteration

**Renamed/Removed:**
- `CODEX_YOLO`, `CODEX_FULL_AUTO`, `CODEX_PROFILE`, `CODEX_PROGRESS`, `CODEX_ENFORCE_OUTPUT_SCHEMA` - These were internal Codex flags that are now handled by the agent layer.

### Dev Mode Changes

**Before:** `--prompt-dir` was a regular flag.

**After:** Dev-only flags require `LOOPER_PROMPT_MODE=dev` to be set. This prevents accidental exposure of prompt internals.

```bash
# Required to use these flags:
export LOOPER_PROMPT_MODE=dev
looper run --prompt-dir ./my-prompts --print-prompt
```

Dev-only env overrides: `LOOPER_PROMPT_DIR` and `LOOPER_PRINT_PROMPT=1`.

## New Features

### Config File Support

Create `looper.toml` (or `.looper.toml`) in your project:

```toml
schedule = "odd-even"
max_iterations = 100

[agents.codex]
binary = "codex"
model = "gpt-5.2-codex"
```

### Better Doctor Command

The Go `doctor` command is more comprehensive:

- Checks all dependencies
- Validates configuration
- Verifies prompt files
- Tests task file and schema

### Improved Ls Command

```bash
looper ls                    # Group by status
looper ls todo               # Filter by status
looper ls --status doing     # Same, explicit flag
looper ls -v                 # Verbose output with details
```

### Install/Uninstall Scripts

The Go version has native install/uninstall via Makefile and Homebrew (Unix/Linux/macOS):

```bash
make install     # Installs to ~/.local/bin/looper
make uninstall   # Removes the binary
brew install nibzard/tap/looper
```

On Windows, build the binary directly with Go and place `looper.exe` on your PATH.

## Behavior Changes

### Deterministic Round-Robin

The Go version normalizes agent names and schedules more strictly:

- Schedules are normalized: `odd_even` → `odd-even`, `round_robin` → `round-robin`
- Agent names are case-insensitive: `Claude` → `claude`
- Empty agent lists in round-robin now default to `claude,codex` consistently

### Review Agent

**Before:** Shell used whatever agent was configured for iterations.

**After:** Go version uses the configured `review_agent` for the review pass (defaults to Codex if not set). You can configure this via:
- TOML: `review_agent = "claude"`
- Env: `LOOPER_REVIEW_AGENT=claude`
- Flag: `--review-agent claude`

### Log Output

The log format is similar (JSONL), but the Go version may have slight differences in event naming. The overall structure remains compatible.
When git is available, Looper resolves the project root via `git rev-parse --show-toplevel` for log grouping.

### Git Initialization

The Go CLI does not automatically run `git init`. Initialize repositories manually if you need one.

## Migration Steps

### 1. Install the Go Binary

```bash
# From source
cd /path/to/looper-go/looper
make install

# Via Homebrew
brew install nibzard/tap/looper
```

### 2. Update Scripts/Workflows

Replace `looper.sh` calls with `looper`:

**Before:**
```bash
./looper.sh --ls todo
./looper.sh --tail --follow
```

**After:**
```bash
looper ls todo
looper tail --follow
```

### 3. Update Environment Variables

If you used custom env vars, most still work. Additions like `looper.toml` are optional.

### 4. Optional: Create Config File

For projects that need non-default settings:

```bash
cat > looper.toml << 'EOF'
schedule = "odd-even"
max_iterations = 50
repair_agent = "codex"

[agents.codex]
binary = "codex"

[agents.claude]
binary = "claude"
EOF
```

### 5. Verify with Doctor

```bash
looper doctor
```

This will check all dependencies and configuration.

### 6. Run as Normal

```bash
looper
```

## Uninstalling the Shell Version

Once you've verified the Go version works:

```bash
# Remove the old shell script
rm -f looper.sh
rm -f ~/.codex/skills/looper

# Keep your to-do.json and to-do.schema.json - they're compatible
```

## Troubleshooting

### "Command not found: looper"

Ensure `~/.local/bin` is on your PATH (Unix/Linux/macOS):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

On Windows, add the directory containing `looper.exe` (for example `%USERPROFILE%\bin`) to your PATH.

Add this to your `~/.bashrc` or `~/.zshrc`.

### Dev Flags Not Working

Dev-only flags (`--prompt-dir`, `--print-prompt`) require:

```bash
export LOOPER_PROMPT_MODE=dev
```

### Config Not Loading

The Go version looks for `looper.toml` or `.looper.toml` in the current working directory (not the todo file directory). Ensure you run `looper` from the project root.

## Rollback

If you need to revert to the shell version:

1. Restore `looper.sh` from git history or backup
2. Run `make uninstall` to remove the Go binary
3. Use `./looper.sh` as before

**Note:** The `to-do.json` format is compatible between versions.

## Summary of Changes

| Aspect | Shell | Go |
|--------|-------|-----|
| Implementation | Bash script | Compiled Go binary |
| Commands | Flags (`--ls`, `--tail`) | Subcommands (`ls`, `tail`) |
| Config | Env vars only | Defaults + TOML + env + flags |
| Dev mode | Always available | Requires `LOOPER_PROMPT_MODE=dev` |
| Review agent | Follows schedule | Always Codex |
| Install | Manual `install.sh` | Makefile + Homebrew |
| Platform | Unix-like only | Cross-platform |
