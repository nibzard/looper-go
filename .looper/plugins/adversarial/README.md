# Adversarial Workflow Plugin

An APEX-style adversarial workflow plugin for looper-go that runs each task through a Coder agent followed by an Adversary agent for validation.

## Overview

This plugin implements an adversarial pair workflow where each task is:
1. First implemented by a **Coder** agent (e.g., `claude`)
2. Then reviewed by an **Adversary** agent (e.g., `codex`)
3. Only marked complete when the Adversary approves

This approach helps catch bugs, security issues, and edge cases before tasks are marked as done.

## Installation

```bash
# Build the plugin
cd .looper/plugins/adversarial
go build -o bin/adversarial ./cmd/adversarial

# Verify the plugin is installed
looper plugin list
```

## Configuration

Add to your `looper.toml`:

```toml
workflow = "adversarial"

[workflows.adversarial]
coder_agent = "claude"      # Agent that implements tasks
adversary_agent = "codex"   # Agent that reviews implementations
max_reruns = 2             # Number of retry attempts after rejection
```

## Usage

```bash
# Run with adversarial workflow
looper run

# Or specify workflow explicitly
looper run --workflow adversarial
```

## How It Works

For each task with status `todo`:

1. **Coder Phase**: The Coder agent receives a prompt describing the task
2. **Adversary Phase**: The Adversary agent reviews the Coder's implementation
3. **Verdict**:
   - `APPROVED`: Task is marked `done`
   - `REJECTED`: Task is retried (up to `max_reruns` times)
4. **Final State**:
   - After approval: `done`
   - After exhausting retries: `blocked` with feedback

## Agent Configuration

The plugin uses looper's configured agents. By default:
- Coder: `claude` (the Claude agent)
- Adversary: `codex` (an alternative model for adversarial review)

You can configure which agents to use in `looper.toml`:

```toml
[workflows.adversarial]
coder_agent = "claude"
adversary_agent = "codex"
```

## Prompt Templates

The plugin includes built-in prompt templates for both roles:

- `prompts/coder.txt` - Coder role instructions
- `prompts/adversary.txt` - Adversary role instructions

These templates include:
- Task context (ID, title, description, reference)
- Retry attempt tracking
- Review criteria guidance

## Output Example

```
=== Task T001: Implement user authentication ===

--- Running Coder (claude) ---
Coder summary: Implemented JWT-based authentication with login/logout endpoints

--- Running Adversary (codex) ---
Adversary summary: APPROVED: Implementation is secure and complete

Task T001: APPROVED - Implementation is secure and complete
```

## License

MIT
