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

# Log directory (supports ~ expansion)
log_dir = "~/.looper"

# Maximum loop iterations
max_iterations = 50

# Iteration schedule: codex, claude, odd-even, round-robin
schedule = "codex"

# Apply summaries back to task file
apply_summary = true

# Initialize git repo if missing
git_init = true

# Hook command to run after each iteration
# hook_command = "/path/to/hook.sh"

# Delay between iterations (seconds)
loop_delay_seconds = 0

# [agents.codex]
# binary = "codex"
# model = ""

# [agents.claude]
# binary = "claude"
# model = ""
`
}
