package workflows

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/logging"
	"github.com/nibzard/looper-go/internal/todo"
)

const (
	// WorkflowTypeIncidentTriage is the incident triage workflow.
	WorkflowTypeIncidentTriage WorkflowType = "incident-triage"
)

func init() {
	Register(WorkflowTypeIncidentTriage, NewIncidentTriageLoopFactory())
}

// NewIncidentTriageLoopFactory creates a factory for the incident triage workflow.
func NewIncidentTriageLoopFactory() WorkflowFactory {
	return func(cfg interface{}, workDir string, todoFile interface{}) (Workflow, error) {
		// Handle nil config for description purposes
		var cfgTyped *config.Config
		if cfg != nil {
			var ok bool
			cfgTyped, ok = cfg.(*config.Config)
			if !ok {
				return nil, fmt.Errorf("expected *config.Config, got %T", cfg)
			}
		}

		// todoFile parameter is ignored for incident-triage workflow

		settings := getConfigSettings(cfgTyped, WorkflowTypeIncidentTriage)

		return &IncidentTriageLoop{
			cfg:            cfgTyped,
			workDir:        workDir,
			severityLevels: GetStringSlice(settings, "severity_levels", []string{"critical", "high", "medium", "low"}),
			autoAssign:     GetBool(settings, "auto_assign", true),
			notifySlack:    GetBool(settings, "notify_slack", false),
			slackWebhook:   GetString(settings, "slack_webhook", ""),
		}, nil
	}
}

// IncidentTriageLoop implements incident classification, assignment, and notification.
type IncidentTriageLoop struct {
	cfg            *config.Config
	workDir        string
	severityLevels []string
	autoAssign     bool
	notifySlack    bool
	slackWebhook   string
	todoFile       *todo.File
}

// Run executes the incident triage workflow.
func (i *IncidentTriageLoop) Run(ctx context.Context) error {
	// Load todo file
	todoPath := i.cfg.TodoFile
	if todoPath == "" {
		todoPath = "to-do.json"
	}
	if todoPath, err := filepath.Abs(todoPath); err == nil {
		// Try absolute path first
	} else {
		todoPath = filepath.Join(i.workDir, todoPath)
	}

	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return fmt.Errorf("load todo file: %w", err)
	}
	i.todoFile = todoFile

	// Find all incident tasks
	incidents := i.getIncidentTasks()
	if len(incidents) == 0 {
		fmt.Println("No incidents to triage.")
		return nil
	}

	fmt.Printf("Triaging %d incidents\n", len(incidents))

	for _, incident := range incidents {
		fmt.Printf("\n=== Incident: %s ===\n", incident.ID)

		// 1. Classify severity
		severity, err := i.classifySeverity(ctx, incident)
		if err != nil {
			fmt.Printf("Error classifying severity: %v\n", err)
			continue
		}
		fmt.Printf("Severity: %s\n", severity)

		// Update incident with severity
		incident.Tags = append(incident.Tags, "severity:"+severity)

		// 2. Assign if auto-assign is enabled
		if i.autoAssign {
			assignee := i.getAssigneeForSeverity(severity)
			fmt.Printf("Assigned to: %s\n", assignee)
			incident.Tags = append(incident.Tags, "assignee:"+assignee)
		}

		// 3. Notify if enabled
		if i.notifySlack && i.slackWebhook != "" {
			if err := i.notifySlackWebhook(incident, severity); err != nil {
				fmt.Printf("Error sending Slack notification: %v\n", err)
			} else {
				fmt.Println("Slack notification sent")
			}
		}

		// Mark as triaged
		incident.Status = todo.StatusDone
	}

	// Save updated todo file
	return todoFile.Save(todoPath)
}

// Description returns a description of the incident triage workflow.
func (i *IncidentTriageLoop) Description() string {
	return "Incident classification, assignment, and notification"
}

// classifySeverity uses an agent to classify incident severity.
func (i *IncidentTriageLoop) classifySeverity(ctx context.Context, incident *todo.Task) (string, error) {
	// Use the review agent for classification
	agentType := i.cfg.GetReviewAgent()
	if agentType == "" {
		agentType = i.cfg.IterSchedule(1)
	}

	// Build agent config
	agentCfg := agents.Config{
		Binary: i.cfg.GetAgentBinary(agentType),
		Model:  i.cfg.GetAgentModel(agentType),
		WorkDir: i.workDir,
	}

	// Create agent
	agent, err := agents.NewAgent(agents.AgentType(agentType), agentCfg)
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}

	// Build prompt
	prompt := i.buildClassificationPrompt(incident)

	// Create log writer
	runLogger, err := logging.NewRunLogger(i.cfg.LogDir, i.workDir)
	if err != nil {
		return "", fmt.Errorf("create logger: %w", err)
	}
	defer runLogger.Close()
	logWriter := agents.NewIOStreamLogWriter(runLogger.Writer())

	// Run agent
	summary, err := agent.Run(ctx, prompt, logWriter)
	if err != nil {
		return "", err
	}

	// Extract severity from summary
	severity := i.extractSeverityFromSummary(summary.Summary)
	if severity == "" {
		// Default to medium if not found
		return "medium", nil
	}

	return severity, nil
}

// buildClassificationPrompt creates a prompt for severity classification.
func (i *IncidentTriageLoop) buildClassificationPrompt(incident *todo.Task) string {
	return fmt.Sprintf(`Classify the severity of this incident.

Title: %s
Description: %s

Available severity levels:
- %s

Respond with ONLY the severity level (one word).`, incident.Title, incident.Description, strings.Join(i.severityLevels, ", "))
}

// extractSeverityFromSummary extracts the severity level from agent output.
func (i *IncidentTriageLoop) extractSeverityFromSummary(summary string) string {
	summaryLower := strings.ToLower(summary)
	for _, level := range i.severityLevels {
		if strings.Contains(summaryLower, strings.ToLower(level)) {
			return level
		}
	}
	return ""
}

// getAssigneeForSeverity returns the assignee for a given severity level.
func (i *IncidentTriageLoop) getAssigneeForSeverity(severity string) string {
	// Simple assignment rules - could be made configurable
	switch severity {
	case "critical":
		return "oncall-senior"
	case "high":
		return "oncall"
	case "medium":
		return "team-backend"
	case "low":
		return "backlog"
	default:
		return "unassigned"
	}
}

// notifySlackWebhook sends a notification to Slack.
func (i *IncidentTriageLoop) notifySlackWebhook(incident *todo.Task, severity string) error {
	// Use curl to send webhook notification
	message := fmt.Sprintf("*Incident %s*: %s (Severity: %s)", incident.ID, incident.Title, severity)

	cmd := exec.Command("curl", "-X", "POST", i.slackWebhook,
		"-H", "Content-Type: application/json",
		"-d", fmt.Sprintf(`{"text": %q}`, message))
	cmd.Dir = i.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("curl: %w: %s", err, string(output))
	}

	return nil
}

// getIncidentTasks returns all tasks with "incident" tag.
func (i *IncidentTriageLoop) getIncidentTasks() []*todo.Task {
	var incidents []*todo.Task
	for idx := range i.todoFile.Tasks {
		task := &i.todoFile.Tasks[idx]
		for _, tag := range task.Tags {
			if tag == "incident" {
				incidents = append(incidents, task)
				break
			}
		}
	}
	return incidents
}
