package parallel

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/loop"
	"github.com/nibzard/looper-go/internal/prompts"
	"github.com/nibzard/looper-go/internal/todo"
)

// TaskExecutor wraps a loop to execute tasks with support for multiple agents.
type TaskExecutor struct {
	loop      *loop.Loop
	maxAgents int
	cfg       *config.Config
}

// NewTaskExecutor creates a new task executor.
func NewTaskExecutor(l *loop.Loop, maxAgents int, cfg *config.Config) *TaskExecutor {
	return &TaskExecutor{
		loop:      l,
		maxAgents: maxAgents,
		cfg:       cfg,
	}
}

// Execute executes a task using the configured agent strategy.
// If maxAgents > 1, multiple agents are run for consensus.
// Otherwise, a single agent is used.
func (e *TaskExecutor) Execute(ctx context.Context, task *todo.Task, agentType string, iter int) (*agents.Summary, error) {
	if e.maxAgents > 1 {
		return e.executeMultiple(ctx, task, agentType, iter)
	}
	return e.executeSingle(ctx, task, agentType, iter)
}

// executeSingle runs a single agent for the task.
func (e *TaskExecutor) executeSingle(ctx context.Context, task *todo.Task, agentType string, iter int) (*agents.Summary, error) {
	label := fmt.Sprintf("%s-iter-%d", task.ID, iter)
	promptData := prompts.NewData(
		e.loop.TodoPath(),
		e.loop.SchemaPath(),
		e.loop.WorkDir(),
		prompts.Task{
			ID:     task.ID,
			Title:  task.Title,
			Status: string(task.Status),
		},
		iter,
		agentType,
		time.Now(),
	)
	prompt, err := e.loop.RenderPrompt(prompts.IterationPrompt, promptData)
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	logWriter := e.loop.MultiLogWriter()
	return e.loop.RunAgent(ctx, agentType, prompt, label, logWriter)
}

// executeMultiple runs multiple agents for the task and combines results.
// This is useful for consensus/voting patterns where multiple agents
// provide different perspectives on the same task.
//
// The consensus strategy implemented:
// - Status: majority vote (if tie, prefer done > blocked > skipped)
// - Files: union of all files from all summaries
// - Blockers: union of all blockers from all summaries
// - Summary text: concatenated with agent attribution
func (e *TaskExecutor) executeMultiple(ctx context.Context, task *todo.Task, baseAgentType string, iter int) (*agents.Summary, error) {
	// For multi-agent consensus, we run each agent and collect results
	// The current implementation uses the same agent type multiple times
	// Future extensions could support different agent types

	label := fmt.Sprintf("%s-iter-%d", task.ID, iter)
	promptData := prompts.NewData(
		e.loop.TodoPath(),
		e.loop.SchemaPath(),
		e.loop.WorkDir(),
		prompts.Task{
			ID:     task.ID,
			Title:  task.Title,
			Status: string(task.Status),
		},
		iter,
		baseAgentType,
		time.Now(),
	)

	var summaries []*agents.Summary
	var errors []error

	for i := 0; i < e.maxAgents; i++ {
		agentLabel := fmt.Sprintf("%s-agent-%d", label, i)

		prompt, err := e.loop.RenderPrompt(prompts.IterationPrompt, promptData)
		if err != nil {
			return nil, fmt.Errorf("render prompt for agent %d: %w", i, err)
		}

		logWriter := e.loop.MultiLogWriter()
		summary, err := e.loop.RunAgent(ctx, baseAgentType, prompt, agentLabel, logWriter)
		if err != nil {
			errors = append(errors, fmt.Errorf("agent %d: %w", i, err))
			continue
		}
		summaries = append(summaries, summary)
	}

	// Combine results using consensus
	if len(summaries) == 0 {
		return nil, fmt.Errorf("all %d agents failed: %v", e.maxAgents, errors)
	}

	return e.consensusSummaries(summaries), nil
}

// consensusSummaries combines multiple agent summaries into a single consensus result.
// It uses majority voting for status, union for files/blockers, and concatenates summaries.
func (e *TaskExecutor) consensusSummaries(summaries []*agents.Summary) *agents.Summary {
	if len(summaries) == 1 {
		return summaries[0]
	}

	// Count status votes for majority decision
	statusVotes := make(map[string]int)
	for _, s := range summaries {
		statusVotes[s.Status]++
	}

	// Find majority status (prefer done > blocked > skipped on ties)
	consensusStatus := e.majorityStatus(statusVotes)

	// Collect all unique files using a set
	filesSet := make(map[string]struct{})
	for _, s := range summaries {
		for _, f := range s.Files {
			filesSet[f] = struct{}{}
		}
	}
	// Convert to sorted slice for deterministic output
	consensusFiles := slices.Collect(maps.Keys(filesSet))
	slices.Sort(consensusFiles)

	// Collect all unique blockers using a set
	blockersSet := make(map[string]struct{})
	for _, s := range summaries {
		for _, b := range s.Blockers {
			blockersSet[b] = struct{}{}
		}
	}
	consensusBlockers := slices.Collect(maps.Keys(blockersSet))
	slices.Sort(consensusBlockers)

	// Build consensus summary text with agent attribution
	var summaryParts []string
	for i, s := range summaries {
		if s.Summary != "" {
			summaryParts = append(summaryParts, fmt.Sprintf("[Agent %d] %s", i, s.Summary))
		}
	}

	consensusSummaryText := ""
	if len(summaryParts) > 0 {
		if len(summaryParts) == 1 {
			// Single agent provided summary, use it directly
			consensusSummaryText = summaries[0].Summary
		} else {
			// Multiple agents provided input, concatenate with clear separation
			consensusSummaryText = fmt.Sprintf("Multi-agent consensus (%d agents):\n%s",
				len(summaries),
				fmt.Sprintf("%s", summaryParts))
		}
	}

	return &agents.Summary{
		TaskID:   summaries[0].TaskID,
		Status:   consensusStatus,
		Summary:  consensusSummaryText,
		Files:    consensusFiles,
		Blockers: consensusBlockers,
	}
}

// majorityStatus determines the consensus status from vote counts.
// On ties, prefers done > blocked > skipped.
func (e *TaskExecutor) majorityStatus(votes map[string]int) string {
	if len(votes) == 0 {
		return "skipped"
	}

	maxVotes := 0
	var candidates []string

	// Find all statuses with maximum votes
	for status, count := range votes {
		if count > maxVotes {
			maxVotes = count
			candidates = []string{status}
		} else if count == maxVotes {
			candidates = append(candidates, status)
		}
	}

	// If we have a clear winner, return it
	if len(candidates) == 1 {
		return candidates[0]
	}

	// Tie-breaker: prefer done > blocked > skipped
	tieOrder := []string{"done", "blocked", "skipped"}
	for _, preferred := range tieOrder {
		if slices.Contains(candidates, preferred) {
			return preferred
		}
	}

	// Fallback: return first candidate (shouldn't reach here with valid statuses)
	return candidates[0]
}
