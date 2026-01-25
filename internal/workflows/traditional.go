package workflows

import (
	"context"
	"fmt"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/loop"
)

const (
	// WorkflowTypeTraditional is the traditional looper loop.
	WorkflowTypeTraditional WorkflowType = "traditional"
)

func init() {
	Register(WorkflowTypeTraditional, NewTraditionalLoopFactory())
}

// NewTraditionalLoopFactory creates a factory for the traditional workflow.
// The todoFile parameter is ignored - the Loop handles its own todo file loading.
func NewTraditionalLoopFactory() WorkflowFactory {
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

		return &TraditionalLoop{
			cfg:     cfgTyped,
			workDir: workDir,
		}, nil
	}
}

// TraditionalLoop wraps the existing looper Loop as a workflow.
type TraditionalLoop struct {
	cfg     *config.Config
	workDir string
	loop    *loop.Loop
}

// Run executes the traditional looper loop.
func (t *TraditionalLoop) Run(ctx context.Context) error {
	if t.loop == nil {
		l, err := loop.New(t.cfg, t.workDir)
		if err != nil {
			return fmt.Errorf("create loop: %w", err)
		}
		t.loop = l
	}
	return t.loop.Run(ctx)
}

// Description returns a description of the traditional workflow.
func (t *TraditionalLoop) Description() string {
	return "Traditional iterative loop with review and repair"
}
