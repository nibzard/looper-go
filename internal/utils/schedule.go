// Package utils provides shared utility functions used across multiple packages.
package utils

import (
	"strings"
)

// NormalizeSchedule normalizes schedule string values.
// Accepts various aliases for common schedules:
// - "odd_even", "odd-even", "oddeven" -> "odd-even"
// - "round_robin", "round-robin", "roundrobin", "rr" -> "round-robin"
// Returns the normalized schedule string and a boolean indicating if the input was valid.
func NormalizeSchedule(input string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	switch s {
	case "odd_even", "odd-even", "oddeven":
		return "odd-even", true
	case "round_robin", "round-robin", "roundrobin", "rr":
		return "round-robin", true
	default:
		if s == "" {
			return "", false
		}
		// For single agent names, assume valid if not empty
		return s, true
	}
}

// NormalizeAgentName normalizes an agent name by converting to lowercase and trimming whitespace.
// Returns empty string if input is empty after normalization.
func NormalizeAgentName(input string) string {
	return strings.ToLower(strings.TrimSpace(input))
}

// NormalizeAgentList normalizes a list of agent names.
// Empty entries are omitted. Returns nil if the result is empty.
func NormalizeAgentList(agents []string) []string {
	if len(agents) == 0 {
		return nil
	}
	result := make([]string, 0, len(agents))
	for _, agent := range agents {
		normalized := NormalizeAgentName(agent)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}
