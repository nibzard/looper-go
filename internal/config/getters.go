package config

import (
	"github.com/nibzard/looper-go/internal/utils"
)

// IterSchedule returns the agent for a given iteration number.
// Looks up roles["iter"] or returns empty string if not configured.
func (c *Config) IterSchedule(iter int) string {
	return c.Roles.GetAgent("iter")
}

// GetReviewAgent returns the agent to use for the review pass.
// Looks up roles["review"] or returns empty string if not configured.
func (c *Config) GetReviewAgent() string {
	return c.Roles.GetAgent("review")
}

// GetBootstrapAgent returns the agent to use for bootstrap operations.
// Looks up roles["bootstrap"] or returns empty string if not configured.
func (c *Config) GetBootstrapAgent() string {
	return c.Roles.GetAgent("bootstrap")
}

// GetRepairAgent returns the agent to use for repair operations.
// Looks up roles["repair"] or returns empty string if not configured.
func (c *Config) GetRepairAgent() string {
	return c.Roles.GetAgent("repair")
}

// GetAgentBinary returns the binary path for the given agent type.
// It checks both custom agents and built-in defaults, then falls back to the agent name.
func (c *Config) GetAgentBinary(agentType string) string {
	agentType = utils.NormalizeAgentName(agentType)
	if agentType == "" {
		return ""
	}
	if agent := c.Agents.GetAgent(agentType); agent.Binary != "" {
		return agent.Binary
	}
	if binary, ok := DefaultAgentBinaries()[agentType]; ok {
		return binary
	}
	return agentType
}

// GetAgentModel returns the model for the given agent type.
// It checks both custom agents and built-in agents.
func (c *Config) GetAgentModel(agentType string) string {
	agentType = utils.NormalizeAgentName(agentType)
	if agentType == "" {
		return ""
	}
	return c.Agents.GetAgent(agentType).Model
}

// GetAgentReasoning returns the reasoning effort for the given agent type.
// It checks both custom agents and built-in agents.
func (c *Config) GetAgentReasoning(agentType string) string {
	agentType = utils.NormalizeAgentName(agentType)
	if agentType == "" {
		return ""
	}
	return c.Agents.GetAgent(agentType).Reasoning
}

// GetAgentArgs returns extra args for the given agent type.
func (c *Config) GetAgentArgs(agentType string) []string {
	agentType = utils.NormalizeAgentName(agentType)
	if agentType == "" {
		return nil
	}
	args := c.Agents.GetAgent(agentType).Args
	if len(args) == 0 {
		return nil
	}
	copied := make([]string, len(args))
	copy(copied, args)
	return copied
}

// GetAgentPromptFormat returns the prompt format for the given agent type.
// It checks both custom agents and built-in agents.
// Returns "stdin" if not configured (the traditional format for codex/claude).
func (c *Config) GetAgentPromptFormat(agentType string) PromptFormat {
	agentType = utils.NormalizeAgentName(agentType)
	if agentType == "" {
		return PromptFormatStdin
	}
	agent := c.Agents.GetAgent(agentType)
	if agent.PromptFormat == "" {
		// Default: codex uses stdin, claude uses arg
		if agentType == "claude" {
			return PromptFormatArg
		}
		return PromptFormatStdin
	}
	return agent.PromptFormat
}

// GetAgentParser returns the parser script path for the given agent type.
// It checks both custom agents and built-in agents.
// Returns empty string if not configured (use built-in Go parsing).
func (c *Config) GetAgentParser(agentType string) string {
	agentType = utils.NormalizeAgentName(agentType)
	if agentType == "" {
		return ""
	}
	return c.Agents.GetAgent(agentType).Parser
}
