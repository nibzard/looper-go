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

# Iteration schedule: any registered agent name, odd-even, or round-robin
schedule = "codex"

# Role-based agent configuration (preferred)
# Assign different agents to different loop phases
[roles]
iter = "codex"       # Agent for task iterations (overrides schedule)
review = "codex"     # Agent for review passes
repair = "codex"     # Agent for repair operations
bootstrap = "codex"  # Agent for bootstrap

# Legacy single-step configuration (still supported, use [roles] for clarity)
# repair_agent = "codex"
# review_agent = "codex"
# bootstrap_agent = "codex"

# Apply summaries back to task file
apply_summary = true

# Initialize git repo if missing
git_init = true

# Hook command to run after each iteration
# hook_command = "/path/to/hook.sh"

# Delay between iterations (seconds)
loop_delay_seconds = 0

# Agent configuration (any registered agent type)
[agents.codex]
binary = "codex"
model = ""
# reasoning = "medium"  # Optional: low, medium, or high reasoning effort
# args = ["--flag", "value"]  # Optional extra args to pass to codex

[agents.claude]
binary = "claude"
model = ""
# args = ["--flag", "value"]  # Optional extra args to pass to claude

# Custom agents can be configured under the agents map
# The agent type must be registered via agents.RegisterAgent() in code
# [agents.opencode]
# binary = "opencode"
# model = "custom-model"
`
}
