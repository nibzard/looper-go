package config

import (
	"fmt"
	"os"

	"github.com/nibzard/looper-go/internal/utils"
)

// loadFromEnv overrides config from environment variables.
func loadFromEnv(cfg *Config) {
	loadFromEnvHelper(cfg, nil, "")
}

// loadFromEnvWithSources loads environment variables and updates source tracking.
func loadFromEnvWithSources(cfg *Config, sources map[string]ConfigSource) {
	loadFromEnvHelper(cfg, sources, SourceEnv)
}

// loadFromEnvHelper is the shared implementation for env loading.
// If sources is non-nil, it tracks the source of each value.
func loadFromEnvHelper(cfg *Config, sources map[string]ConfigSource, source ConfigSource) {
	setEnv := func(field, value string) {
		if sources != nil {
			sources[field] = source
		}
	}
	setEnvInt := func(field string, value int) {
		if sources != nil {
			sources[field] = source
		}
	}
	setEnvBool := func(field string, value bool) {
		if sources != nil {
			sources[field] = source
		}
	}
	setAgentEnv := func(agentType, field, value string) {
		if sources != nil {
			sources[field] = source
		}
	}
	setAgentArgs := func(agentType, field string, value []string) {
		if sources != nil {
			sources[field] = source
		}
	}
	setAgentReasoning := func(agentType, field string, value string) {
		if sources != nil {
			sources[field] = source
		}
	}

	if v := os.Getenv("LOOPER_TODO"); v != "" {
		cfg.TodoFile = v
		setEnv("todo_file", v)
	}
	if v := os.Getenv("LOOPER_SCHEMA"); v != "" {
		cfg.SchemaFile = v
		setEnv("schema_file", v)
	}
	if v := os.Getenv("LOOPER_BASE_DIR"); v != "" {
		cfg.LogDir = v
		setEnv("log_dir", v)
	}
	if v := os.Getenv("LOOPER_LOG_DIR"); v != "" {
		cfg.LogDir = v
		setEnv("log_dir", v)
	}
	if devModeEnabled() {
		if v := os.Getenv("LOOPER_PROMPT_DIR"); v != "" {
			cfg.PromptDir = v
		}
		if v := os.Getenv("LOOPER_PRINT_PROMPT"); v != "" {
			cfg.PrintPrompt = boolFromString(v)
		}
	}
	if v := os.Getenv("LOOPER_PROMPT"); v != "" {
		cfg.UserPrompt = v
	}
	if v := os.Getenv("LOOPER_MAX_ITERATIONS"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.MaxIterations = i
			setEnvInt("max_iterations", i)
		}
	}
	if v := os.Getenv("LOOPER_APPLY_SUMMARY"); v != "" {
		cfg.ApplySummary = boolFromString(v)
		setEnvBool("apply_summary", cfg.ApplySummary)
	}
	if v := os.Getenv("LOOPER_GIT_INIT"); v != "" {
		cfg.GitInit = boolFromString(v)
		setEnvBool("git_init", cfg.GitInit)
	}
	if v := os.Getenv("LOOPER_HOOK"); v != "" {
		cfg.HookCommand = v
		setEnv("hook_command", v)
	}
	if v := os.Getenv("LOOPER_LOOP_DELAY"); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			cfg.LoopDelaySeconds = i
			setEnvInt("loop_delay_seconds", i)
		}
	}
	if v := os.Getenv("CODEX_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Binary = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentEnv("codex", "codex_binary", v)
	}
	if v := os.Getenv("CLAUDE_BIN"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Binary = v
		cfg.Agents.SetAgent("claude", agent)
		setAgentEnv("claude", "claude_binary", v)
	}
	if v := os.Getenv("CODEX_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Model = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentEnv("codex", "codex_model", v)
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Model = v
		cfg.Agents.SetAgent("claude", agent)
		setAgentEnv("claude", "claude_model", v)
	}
	if v := os.Getenv("CODEX_REASONING"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentReasoning("codex", "codex_reasoning", v)
	}
	if v := os.Getenv("CODEX_REASONING_EFFORT"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Reasoning = v
		cfg.Agents.SetAgent("codex", agent)
		setAgentReasoning("codex", "codex_reasoning", v)
	}
	if v := os.Getenv("CODEX_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("codex")
		agent.Args = utils.SplitAndTrim(v, ",")
		cfg.Agents.SetAgent("codex", agent)
		setAgentArgs("codex", "codex_args", agent.Args)
	}
	if v := os.Getenv("CLAUDE_ARGS"); v != "" {
		agent := cfg.Agents.GetAgent("claude")
		agent.Args = utils.SplitAndTrim(v, ",")
		cfg.Agents.SetAgent("claude", agent)
		setAgentArgs("claude", "claude_args", agent.Args)
	}

	// Logging configuration
	if v := os.Getenv("LOOPER_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
		setEnv("log_level", v)
	}
	if v := os.Getenv("LOOPER_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
		setEnv("log_format", v)
	}
	if v := os.Getenv("LOOPER_LOG_TIMESTAMPS"); v != "" {
		cfg.LogTimestamps = boolFromString(v)
		setEnvBool("log_timestamps", cfg.LogTimestamps)
	}
	if v := os.Getenv("LOOPER_LOG_CALLER"); v != "" {
		cfg.LogCaller = boolFromString(v)
		setEnvBool("log_caller", cfg.LogCaller)
	}
}
