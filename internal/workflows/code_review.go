package workflows

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nibzard/looper-go/internal/agents"
	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/logging"
)

const (
	// WorkflowTypeCodeReview is the code review workflow.
	WorkflowTypeCodeReview WorkflowType = "code-review"
)

func init() {
	Register(WorkflowTypeCodeReview, NewCodeReviewLoopFactory())
}

// NewCodeReviewLoopFactory creates a factory for the code review workflow.
func NewCodeReviewLoopFactory() WorkflowFactory {
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

		// todoFile parameter is ignored for code-review workflow

		settings := getConfigSettings(cfgTyped, WorkflowTypeCodeReview)

		return &CodeReviewLoop{
			cfg:             cfgTyped,
			workDir:         workDir,
			diffPath:        GetString(settings, "diff_path", "."),
			stages:          GetStringSlice(settings, "review_stages", []string{"analyze", "security", "style"}),
			requireApproval: GetBool(settings, "require_approval", true),
			approvalFile:    GetString(settings, "approval_file", ".looper/approval.txt"),
		}, nil
	}
}

// CodeReviewLoop implements a multi-stage code review workflow.
type CodeReviewLoop struct {
	cfg             *config.Config
	workDir         string
	diffPath        string
	stages          []string
	requireApproval bool
	approvalFile    string
}

// Run executes the code review workflow.
func (c *CodeReviewLoop) Run(ctx context.Context) error {
	// 1. Get the diff
	diff, err := c.getDiff()
	if err != nil {
		return fmt.Errorf("get diff: %w", err)
	}

	if diff == "" {
		fmt.Println("No changes to review.")
		return nil
	}

	// 2. Run through review stages
	for _, stage := range c.stages {
		fmt.Printf("\n=== Stage: %s ===\n", stage)
		if err := c.runReviewStage(ctx, stage, diff); err != nil {
			return fmt.Errorf("stage %s: %w", stage, err)
		}
	}

	// 3. Handle approval if required
	if c.requireApproval {
		fmt.Println("\n=== Approval Required ===")
		fmt.Printf("Review the output above. Create approval file to continue:\n")
		fmt.Printf("  echo 'approved' > %s\n", c.approvalFile)
		return c.waitForApproval(ctx)
	}

	return nil
}

// Description returns a description of the code review workflow.
func (c *CodeReviewLoop) Description() string {
	return "Multi-stage code review with optional approval"
}

// getDiff gets the git diff of changes.
func (c *CodeReviewLoop) getDiff() (string, error) {
	// Check if we're in a git repo
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found: %w", err)
	}

	// Check if .git exists
	gitDir := filepath.Join(c.workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return "", nil // Not an error, just no diff available
	}

	// Get diff
	cmd := exec.Command(gitPath, "diff", "HEAD")
	cmd.Dir = c.workDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}

	diff := strings.TrimSpace(string(output))
	if diff == "" {
		return "", nil
	}

	return diff, nil
}

// runReviewStage runs a single review stage with an agent.
func (c *CodeReviewLoop) runReviewStage(ctx context.Context, stage string, diff string) error {
	// Determine which agent to use for this stage
	agentType := c.getAgentForStage(stage)

	// Build agent config
	agentCfg := agents.Config{
		Binary: c.cfg.GetAgentBinary(agentType),
		Model:  c.cfg.GetAgentModel(agentType),
		WorkDir: c.workDir,
	}

	// Create agent
	agent, err := agents.NewAgent(agents.AgentType(agentType), agentCfg)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	// Build prompt for this stage
	prompt := c.buildPrompt(stage, diff)

	// Create log writer
	runLogger, err := logging.NewRunLogger(c.cfg.LogDir, c.workDir)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer runLogger.Close()
	logWriter := agents.NewIOStreamLogWriter(runLogger.Writer())

	// Run agent
	_, err = agent.Run(ctx, prompt, logWriter)
	return err
}

// getAgentForStage returns the agent type for a given stage.
func (c *CodeReviewLoop) getAgentForStage(stage string) string {
	// Check if there's a stage-specific agent config
	key := fmt.Sprintf("agent_%s", stage)
	if agent := c.cfg.GetAgentBinary(key); agent != "" {
		return key
	}
	// Default to iter agent
	return c.cfg.IterSchedule(1)
}

// buildPrompt creates the prompt for a review stage.
func (c *CodeReviewLoop) buildPrompt(stage string, diff string) string {
	var stageInstructions string
	switch stage {
	case "analyze":
		stageInstructions = "Analyze the code changes for:"
		stageInstructions += "\n- Overall code quality and structure"
		stageInstructions += "\n- Potential bugs or issues"
		stageInstructions += "\n- Performance considerations"
		stageInstructions += "\n- Testing coverage"
	case "security":
		stageInstructions = "Review the code for security issues:"
		stageInstructions += "\n- SQL injection, XSS, CSRF vulnerabilities"
		stageInstructions += "\n- Authentication and authorization issues"
		stageInstructions += "\n- Sensitive data handling"
		stageInstructions += "\n- Dependency vulnerabilities"
	case "style":
		stageInstructions = "Review the code for style and consistency:"
		stageInstructions += "\n- Code formatting and readability"
		stageInstructions += "\n- Naming conventions"
		stageInstructions += "\n- Comments and documentation"
		stageInstructions += "\n- Idiomatic Go patterns"
	default:
		stageInstructions = fmt.Sprintf("Review the code for %s-related issues.", stage)
	}

	return fmt.Sprintf(`You are reviewing a code change. Below is the git diff.

%s

Focus your review on:
%s

Provide a clear, actionable summary of any issues found.
`, diff, stageInstructions)
}

// waitForApproval waits for the user to create an approval file.
func (c *CodeReviewLoop) waitForApproval(ctx context.Context) error {
	approvalPath := filepath.Join(c.workDir, c.approvalFile)

	// Check if approval already exists
	if _, err := os.Stat(approvalPath); err == nil {
		content, _ := os.ReadFile(approvalPath)
		if strings.TrimSpace(string(content)) == "approved" {
			fmt.Println("Already approved. Continuing...")
			return os.Remove(approvalPath)
		}
	}

	// Wait for approval
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := os.Stat(approvalPath); err == nil {
				content, _ := os.ReadFile(approvalPath)
				if strings.TrimSpace(string(content)) == "approved" {
					fmt.Println("Approval received. Continuing...")
					return os.Remove(approvalPath)
				}
			}
		}
	}
}

// getConfigSettings gets workflow settings from config.
func getConfigSettings(cfg *config.Config, workflowType WorkflowType) map[string]any {
	if cfg == nil || cfg.WorkflowConfigs == nil {
		return nil
	}
	return cfg.WorkflowConfigs[string(workflowType)]
}
