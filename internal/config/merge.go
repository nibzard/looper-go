package config

import (
	"fmt"
	"strings"

	"github.com/nibzard/looper-go/internal/utils"
)

// mergeAgentTables merges agent configuration from TOML tables.
// Handles both direct layout (agents.<name>) and legacy nested format (agents.agents.<name>).
func mergeAgentTables(target AgentConfig, table map[string]interface{}) error {
	for key, value := range table {
		if key == "agents" {
			nested, ok := value.(map[string]interface{})
			if !ok {
				return fmt.Errorf("agents.agents must be a table")
			}
			if err := mergeAgentTables(target, nested); err != nil {
				return err
			}
			continue
		}

		raw, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		agent, err := decodeAgentConfig(raw)
		if err != nil {
			return fmt.Errorf("agent %s: %w", key, err)
		}
		target[utils.NormalizeAgentName(key)] = agent
	}
	return nil
}

// decodeAgentConfig decodes a single agent configuration from raw TOML data.
func decodeAgentConfig(raw map[string]interface{}) (Agent, error) {
	var agent Agent
	if raw == nil {
		return agent, nil
	}
	if v, ok := raw["binary"]; ok {
		binary, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("binary must be a string")
		}
		agent.Binary = binary
	}
	if v, ok := raw["model"]; ok {
		model, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("model must be a string")
		}
		agent.Model = model
	}
	if v, ok := raw["reasoning"]; ok {
		reasoning, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("reasoning must be a string")
		}
		agent.Reasoning = reasoning
	}
	if v, ok := raw["args"]; ok {
		args, err := parseArgsValue(v)
		if err != nil {
			return agent, err
		}
		agent.Args = args
	}
	if v, ok := raw["prompt_format"]; ok {
		promptFormat, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("prompt_format must be a string")
		}
		agent.PromptFormat = PromptFormat(promptFormat)
	}
	if v, ok := raw["parser"]; ok {
		parser, ok := v.(string)
		if !ok {
			return agent, fmt.Errorf("parser must be a string")
		}
		agent.Parser = parser
	}
	return agent, nil
}

// parseArgsValue parses the args field which can be a string array or comma-separated string.
func parseArgsValue(v interface{}) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return filterEmptyArgs(val), nil
	case []interface{}:
		args := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("args must be a string array")
			}
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				args = append(args, trimmed)
			}
		}
		return args, nil
	case string:
		return utils.SplitAndTrim(val, ","), nil
	default:
		return nil, fmt.Errorf("args must be a string or string array")
	}
}

// filterEmptyArgs removes empty strings from args array.
func filterEmptyArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

// mergeAgentSources merges a single agent's config from a file into cfg with source tracking.
func mergeAgentSources(cfg *Config, tempCfg *Config, sources map[string]ConfigSource, source ConfigSource, name, defaultBinary string) {
	agent := tempCfg.Agents.GetAgent(name)
	if agent.Binary != "" {
		if agent.Binary != defaultBinary {
			sources[name+"_binary"] = source
		}
		cfg.Agents.SetAgent(name, agent)
	} else if cfg.Agents.GetAgent(name).Binary == "" {
		cfg.Agents.SetAgent(name, Agent{Binary: defaultBinary})
	}
	if agent.Model != "" {
		sources[name+"_model"] = source
		a := cfg.Agents.GetAgent(name)
		a.Model = agent.Model
		cfg.Agents.SetAgent(name, a)
	}
	if agent.Reasoning != "" {
		sources[name+"_reasoning"] = source
		a := cfg.Agents.GetAgent(name)
		a.Reasoning = agent.Reasoning
		cfg.Agents.SetAgent(name, a)
	}
	if len(agent.Args) > 0 {
		sources[name+"_args"] = source
		a := cfg.Agents.GetAgent(name)
		a.Args = agent.Args
		cfg.Agents.SetAgent(name, a)
	}
}

// setSource is a helper for loadConfigFileWithSources.
func setSource[T any](cfg *Config, field *T, value T, sources map[string]ConfigSource, name string, source ConfigSource) {
	*field = value
	sources[name] = source
}

// boolFromString parses a boolean from a string.
func boolFromString(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}
