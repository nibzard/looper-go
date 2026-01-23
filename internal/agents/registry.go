// Package agents defines Codex and Claude runners.
package agents

// Registry holds registered agent types and their factories.
var Registry = map[AgentType]AgentFactory{}

// RegisterAgent registers an agent type with its factory.
// This allows external code to register new agent types (e.g., opencode, ampcode).
func RegisterAgent(agentType AgentType, factory AgentFactory) {
	Registry[agentType] = factory
}

// init registers the built-in agent types.
func init() {
	RegisterAgent(AgentTypeCodex, func(cfg Config) (Agent, error) {
		return NewCodexAgent(cfg), nil
	})
	RegisterAgent(AgentTypeClaude, func(cfg Config) (Agent, error) {
		return NewClaudeAgent(cfg), nil
	})
}

// IsAgentTypeRegistered returns true if the agent type is registered.
func IsAgentTypeRegistered(agentType string) bool {
	_, ok := Registry[AgentType(agentType)]
	return ok
}

// RegisteredAgentTypes returns a list of all registered agent types.
func RegisteredAgentTypes() []string {
	types := make([]string, 0, len(Registry))
	for t := range Registry {
		types = append(types, string(t))
	}
	return types
}
