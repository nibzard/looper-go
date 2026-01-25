// Package internal provides the core adversarial workflow logic.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Workflow implements the adversarial pair workflow.
type Workflow struct {
	CoderAgent     string
	AdversaryAgent string
	MaxReruns      int
	WorkDir        string
	TodoPath       string
}

// TodoFile represents the todo.json structure.
type TodoFile struct {
	SchemaVersion int    `json:"schema_version"`
	Project       struct {
		Name string `json:"name"`
		Root string `json:"root"`
	} `json:"project"`
	SourceFiles []string `json:"source_files"`
	Tasks       []Task   `json:"tasks"`
}

// Task represents a single task in the todo file.
type Task struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Reference   string   `json:"reference,omitempty"`
	Priority    int      `json:"priority"`
	Status      string   `json:"status"`
	Details     string   `json:"details,omitempty"`
	Steps       []string `json:"steps,omitempty"`
	Blockers    []string `json:"blockers,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Files       []string `json:"files,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

// WorkflowResult is the result of workflow execution.
type WorkflowResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// Run executes the adversarial workflow.
func (w *Workflow) Run() (*WorkflowResult, error) {
	// Load todo file
	todoFile, err := w.loadTodoFile()
	if err != nil {
		return nil, fmt.Errorf("loading todo file: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Track results
	completedCount := 0
	blockedCount := 0
	skippedCount := 0

	// Process each pending task
	for i, task := range todoFile.Tasks {
		if task.Status != "todo" {
			continue
		}

		// Check dependencies
		if !w.dependenciesSatisfied(task, todoFile.Tasks) {
			skippedCount++
			continue
		}

		fmt.Printf("\n=== Task %s: %s ===\n", task.ID, task.Title)

		// Run adversarial pair with retry
		var result *AdversarialResult
		var retryErr error
		maxAttempts := w.MaxReruns + 1

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if attempt > 1 {
				fmt.Printf("\n--- Retry %d/%d ---\n", attempt, maxAttempts)
			}

			result, retryErr = w.runAdversarialPair(ctx, &task, attempt)
			if retryErr != nil {
				fmt.Printf("Error: %v\n", retryErr)
				continue
			}

			if result.Approved {
				break // Approved, no need to retry
			}

			if attempt < maxAttempts {
				fmt.Printf("Adversary feedback: %s\n", result.Feedback)
			}
		}

		// Update task status based on result
		if retryErr != nil {
			todoFile.Tasks[i].Status = "blocked"
			todoFile.Tasks[i].Blockers = append(todoFile.Tasks[i].Blockers, retryErr.Error())
			todoFile.Tasks[i].UpdatedAt = time.Now().Format(time.RFC3339)
			blockedCount++
		} else if result.Approved {
			todoFile.Tasks[i].Status = "done"
			todoFile.Tasks[i].UpdatedAt = time.Now().Format(time.RFC3339)
			completedCount++
			fmt.Printf("Task %s: APPROVED - %s\n", task.ID, result.Feedback)
		} else {
			todoFile.Tasks[i].Status = "blocked"
			todoFile.Tasks[i].Blockers = append(todoFile.Tasks[i].Blockers, result.Feedback)
			todoFile.Tasks[i].UpdatedAt = time.Now().Format(time.RFC3339)
			blockedCount++
			fmt.Printf("Task %s: REJECTED after %d attempts - %s\n", task.ID, maxAttempts, result.Feedback)
		}

		// Save todo file after each task
		if err := w.saveTodoFile(todoFile); err != nil {
			return nil, fmt.Errorf("saving todo file: %w", err)
		}
	}

	message := fmt.Sprintf("Completed %d tasks, %d blocked, %d skipped (pending dependencies)",
		completedCount, blockedCount, skippedCount)

	return &WorkflowResult{
		Success: true,
		Message: message,
	}, nil
}

// loadTodoFile reads and parses the todo.json file.
func (w *Workflow) loadTodoFile() (*TodoFile, error) {
	data, err := os.ReadFile(w.TodoPath)
	if err != nil {
		return nil, err
	}

	var todoFile TodoFile
	if err := json.Unmarshal(data, &todoFile); err != nil {
		return nil, err
	}

	return &todoFile, nil
}

// saveTodoFile writes the todo file to disk.
func (w *Workflow) saveTodoFile(todoFile *TodoFile) error {
	data, err := json.MarshalIndent(todoFile, "", "  ")
	if err != nil {
		return err
	}

	// Add trailing newline
	data = append(data, '\n')

	return os.WriteFile(w.TodoPath, data, 0644)
}

// dependenciesSatisfied checks if all task dependencies are complete.
func (w *Workflow) dependenciesSatisfied(task Task, allTasks []Task) bool {
	for _, depID := range task.DependsOn {
		for _, t := range allTasks {
			if t.ID == depID && t.Status != "done" {
				return false
			}
		}
	}
	return true
}

// runAdversarialPair runs the Coder and Adversary agents for a task.
func (w *Workflow) runAdversarialPair(ctx context.Context, task *Task, attempt int) (*AdversarialResult, error) {
	// Build Coder prompt
	coderPrompt := w.buildCoderPrompt(task, attempt)

	// Run Coder agent
	fmt.Printf("\n--- Running Coder (%s) ---\n", w.CoderAgent)
	coderResult, err := w.runAgent(ctx, w.CoderAgent, coderPrompt)
	if err != nil {
		return nil, fmt.Errorf("coder agent failed: %w", err)
	}
	fmt.Printf("Coder summary: %s\n", coderResult.Summary)

	// Build Adversary prompt with Coder's output
	adversaryPrompt := w.buildAdversaryPrompt(task, coderResult, attempt)

	// Run Adversary agent
	fmt.Printf("\n--- Running Adversary (%s) ---\n", w.AdversaryAgent)
	adversaryResult, err := w.runAgent(ctx, w.AdversaryAgent, adversaryPrompt)
	if err != nil {
		return nil, fmt.Errorf("adversary agent failed: %w", err)
	}
	fmt.Printf("Adversary summary: %s\n", adversaryResult.Summary)

	// Parse adversarial verdict
	return w.parseAdversaryResult(adversaryResult.Summary, coderResult)
}

// runAgent executes an agent binary as a subprocess.
func (w *Workflow) runAgent(ctx context.Context, agentType, prompt string) (*AgentResult, error) {
	// Find agent binary
	binary, err := exec.LookPath(agentType)
	if err != nil {
		return nil, fmt.Errorf("agent binary not found: %s: %w", agentType, err)
	}

	// Create command
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = w.WorkDir
	cmd.Stdin = strings.NewReader(prompt)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w: %s", err, string(output))
	}

	// Parse agent summary from output
	return w.parseAgentOutput(string(output)), nil
}

// AgentResult represents the output from an agent.
type AgentResult struct {
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Files    []string `json:"files,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

// parseAgentOutput extracts the summary from agent output.
func (w *Workflow) parseAgentOutput(output string) *AgentResult {
	// Look for JSON summary in the output
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var result AgentResult
			if err := json.Unmarshal([]byte(line), &result); err == nil {
				return &result
			}
		}
	}

	// Fallback: treat last non-empty line as summary
	var lastLine string
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastLine = strings.TrimSpace(lines[i])
			break
		}
	}

	return &AgentResult{
		Status:  "done",
		Summary: lastLine,
	}
}

// AdversarialResult represents the outcome of the adversarial review.
type AdversarialResult struct {
	Approved bool
	Feedback string
}

// buildCoderPrompt creates the prompt for the Coder agent.
func (w *Workflow) buildCoderPrompt(task *Task, attempt int) string {
	var sb strings.Builder

	sb.WriteString("You are the Coder agent. Your task is to implement the following:\n\n")
	sb.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("Title: %s\n", task.Title))

	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	}

	if task.Reference != "" {
		sb.WriteString(fmt.Sprintf("Reference: %s\n", task.Reference))
	}

	if len(task.Steps) > 0 {
		sb.WriteString("\nSteps:\n")
		for i, step := range task.Steps {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step))
		}
	}

	if attempt > 1 {
		sb.WriteString(fmt.Sprintf("\nThis is attempt %d. Incorporate feedback from the previous review.\n", attempt))
	}

	sb.WriteString("\nImplement this task carefully. Your work will be reviewed by an Adversary agent.\n")
	sb.WriteString("\nAfter completing your work, provide a brief summary of what was done.\n")

	return sb.String()
}

// buildAdversaryPrompt creates the prompt for the Adversary agent.
func (w *Workflow) buildAdversaryPrompt(task *Task, coderResult *AgentResult, attempt int) string {
	var sb strings.Builder

	sb.WriteString("You are the Adversary agent. Review the following implementation:\n\n")
	sb.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("Title: %s\n", task.Title))

	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	}

	sb.WriteString(fmt.Sprintf("\nCoder's Summary: %s\n", coderResult.Summary))

	if len(coderResult.Files) > 0 {
		sb.WriteString("\nFiles modified:\n")
		for _, file := range coderResult.Files {
			sb.WriteString(fmt.Sprintf("  - %s\n", file))
		}
	}

	sb.WriteString("\nReview for:\n")
	sb.WriteString("  - Bugs and edge cases\n")
	sb.WriteString("  - Security issues\n")
	sb.WriteString("  - Missing requirements\n")
	sb.WriteString("  - Code quality and maintainability\n")

	sb.WriteString("\nOutput your verdict as one of:\n")
	sb.WriteString("  APPROVED: [reason]\n")
	sb.WriteString("  REJECTED: [specific feedback]\n")

	if attempt > 1 {
		sb.WriteString(fmt.Sprintf("\nThis is review attempt %d.\n", attempt))
	}

	return sb.String()
}

// parseAdversaryResult parses the adversary's verdict.
func (w *Workflow) parseAdversaryResult(summary string, coderResult *AgentResult) (*AdversarialResult, error) {
	upper := strings.ToUpper(summary)

	// Look for approval
	if strings.Contains(upper, "APPROVED") {
		// Extract reason after APPROVED:
		parts := strings.SplitN(upper, "APPROVED:", 2)
		feedback := "Implementation approved"
		if len(parts) > 1 {
			feedback = strings.TrimSpace(parts[1])
		}
		return &AdversarialResult{
			Approved: true,
			Feedback: feedback,
		}, nil
	}

	// Look for rejection
	if strings.Contains(upper, "REJECTED") {
		// Extract reason after REJECTED:
		parts := strings.SplitN(upper, "REJECTED:", 2)
		feedback := "Implementation needs revision"
		if len(parts) > 1 {
			feedback = strings.TrimSpace(parts[1])
		}
		return &AdversarialResult{
			Approved: false,
			Feedback: feedback,
		}, nil
	}

	// Default: treat summary as feedback
	return &AdversarialResult{
		Approved: false,
		Feedback: summary,
	}, nil
}

// getWorkDir returns the working directory for the workflow.
func (w *Workflow) getWorkDir() string {
	if w.WorkDir != "" {
		return w.WorkDir
	}
	// Use todo file directory as default
	return filepath.Dir(w.TodoPath)
}
