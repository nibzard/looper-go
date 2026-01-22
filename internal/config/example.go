// Package config provides configuration loading and management.
package config

// ExampleConfig returns an example configuration showing all available options.
func ExampleConfig() string {
	return `# Looper configuration file
# Paths can be overridden by environment variables or CLI flags

# Task file (relative to project root)
todo_file = "to-do.json"

# Schema file (auto-generated if missing)
schema_file = "to-do.schema.json"

# Log directory (supports ~ expansion and %VAR% on Windows)
log_dir = "~/.looper"

# Maximum loop iterations
max_iterations = 50

# Iteration schedule: codex, claude, odd-even, round-robin, or any registered agent type
schedule = "codex"

# Agent for repair operations (any registered agent type)
repair_agent = "codex"

# Agent for review pass (any registered agent type, default: codex)
# review_agent = "codex"

# Agent for bootstrap operations (any registered agent type, default: codex)
# bootstrap_agent = "codex"

# Apply summaries back to task file
apply_summary = true

# Initialize git repo if missing
git_init = true

# Hook command to run after each iteration
# hook_command = "/path/to/hook.sh"

# Delay between iterations (seconds)
loop_delay_seconds = 0

# Built-in agent configuration
[agents.codex]
binary = "codex"
model = ""

[agents.claude]
binary = "claude"
model = ""

# Custom agents can be added under the agents.agents map
# For example, to use a custom agent named "opencode":
# [agents.agents.opencode]
# binary = "opencode"
# model = "custom-model"
`
}
