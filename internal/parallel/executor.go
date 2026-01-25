package parallel

import (
	"context"
	"fmt"
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

	// Combine results
	if len(summaries) == 0 {
		return nil, fmt.Errorf("all %d agents failed: %v", e.maxAgents, errors)
	}

	// If we have at least one successful result, use it
	// In a more sophisticated implementation, we could:
	// - Vote on the best result
	// - Merge summaries from multiple agents
	// - Use the most common result
	return summaries[0], nil
}
