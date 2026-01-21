// Package ui provides optional terminal interfaces.
package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nibzard/looper/internal/config"
	"github.com/nibzard/looper/internal/loop"
	"github.com/nibzard/looper/internal/todo"
)

// TUIOption configures the TUI behavior.
type TUIOption func(*tuiConfig)

// tuiConfig holds TUI configuration.
type tuiConfig struct {
	runLoop bool
}

// WithRunLoop enables running the loop in the background.
func WithRunLoop(enabled bool) TUIOption {
	return func(c *tuiConfig) {
		c.runLoop = enabled
	}
}

// RunTUI starts the TUI with the given config.
func RunTUI(ctx context.Context, cfg *config.Config, todoPath string, opts ...TUIOption) error {
	c := &tuiConfig{
		runLoop: false,
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.runLoop {
		return runTUIWithLoop(ctx, cfg, todoPath)
	}
	return runTUIViewer(ctx, cfg, todoPath)
}

// runTUIViewer runs a simple TUI viewer (basic terminal UI).
func runTUIViewer(ctx context.Context, cfg *config.Config, todoPath string) error {
	// Clear screen
	fmt.Print("\x1b[2J\x1b[H")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Initial render
	if err := renderTUI(ctx, cfg, todoPath); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Clear screen and re-render
			fmt.Print("\x1b[2J\x1b[H")
			if err := renderTUI(ctx, cfg, todoPath); err != nil {
				return err
			}
		}
	}
}

// renderTUI renders the TUI to stdout.
func renderTUI(ctx context.Context, cfg *config.Config, todoPath string) error {
	// Resolve todo path
	if !filepath.IsAbs(todoPath) {
		todoPath = filepath.Join(cfg.ProjectRoot, todoPath)
	}

	// Load todo file
	todoFile, err := todo.Load(todoPath)
	if err != nil {
		return fmt.Errorf("loading todo file: %w", err)
	}

	// Render header
	renderHeader()

	// Render tasks overview
	renderTasksOverview(todoFile)

	// Render current/next task
	renderCurrentTask(todoFile)

	// Render recent activity
	renderRecentActivity(todoFile)

	// Render config summary
	renderConfigSummary(cfg)

	// Render footer
	renderFooter()

	return nil
}

// renderHeader renders the TUI header.
func renderHeader() {
	title := " Looper TUI "
	border := strings.Repeat("=", len(title)+20)
	fmt.Printf("\x1b[1;37;44m%s\x1b[0m\n", border+title+border)
	fmt.Println()
}

// renderTasksOverview renders the tasks overview section.
func renderTasksOverview(todoFile *todo.File) {
	fmt.Printf("\x1b[1;36m Task Overview \x1b[0m\n\n")

	counts := map[string]int{
		"todo":    0,
		"doing":   0,
		"blocked": 0,
		"done":    0,
	}

	for _, t := range todoFile.Tasks {
		counts[string(t.Status)]++
	}

	fmt.Printf("  \x1b[1;33m%d\x1b[0m Todo    \x1b[2m|\x1b[0m ", counts["todo"])
	fmt.Printf("\x1b[1;33m%d\x1b[0m Doing   \x1b[2m|\x1b[0m ", counts["doing"])
	fmt.Printf("\x1b[1;33m%d\x1b[0m Blocked \x1b[2m|\x1b[0m ", counts["blocked"])
	fmt.Printf("\x1b[1;33m%d\x1b[0m Done\n\n", counts["done"])
}

// renderCurrentTask renders the current or next task.
func renderCurrentTask(todoFile *todo.File) {
	currentTask := todoFile.FindTaskByStatus(todo.StatusDoing)
	if currentTask != nil {
		fmt.Printf("\x1b[1;36m Current Task \x1b[0m\n\n")
		renderTask(currentTask, true)
	} else {
		nextTask := todoFile.SelectTask()
		if nextTask != nil {
			fmt.Printf("\x1b[1;36m Next Task \x1b[0m\n\n")
			renderTask(nextTask, true)
		} else {
			fmt.Printf("\x1b[1;36m All Tasks Done! \x1b[0m\n\n")
			fmt.Println("  No pending tasks remaining.")
		}
	}
	fmt.Println()
}

// renderRecentActivity renders recently completed tasks.
func renderRecentActivity(todoFile *todo.File) {
	fmt.Printf("\x1b[1;36m Recently Completed \x1b[0m\n\n")

	// Sort tasks by updated_at descending
	sortedTasks := make([]todo.Task, len(todoFile.Tasks))
	copy(sortedTasks, todoFile.Tasks)
	sort.Slice(sortedTasks, func(i, j int) bool {
		if sortedTasks[i].UpdatedAt == nil {
			return false
		}
		if sortedTasks[j].UpdatedAt == nil {
			return true
		}
		return sortedTasks[i].UpdatedAt.After(*sortedTasks[j].UpdatedAt)
	})

	doneCount := 0
	for _, t := range sortedTasks {
		if t.Status == todo.StatusDone {
			renderTask(&t, false)
			doneCount++
			if doneCount >= 5 {
				break
			}
		}
	}

	if doneCount == 0 {
		fmt.Println("  No completed tasks yet.")
	}
	fmt.Println()
}

// renderConfigSummary renders the configuration summary.
func renderConfigSummary(cfg *config.Config) {
	fmt.Printf("\x1b[1;36m Configuration \x1b[0m\n\n")
	fmt.Printf("  Schedule:  \x1b[1m%s\x1b[0m\n", cfg.Schedule)
	fmt.Printf("  Max Iters: \x1b[1m%d\x1b[0m\n", cfg.MaxIterations)
	fmt.Printf("  Todo File: %s\n", cfg.TodoFile)
	fmt.Println()
}

// renderFooter renders the TUI footer with help.
func renderFooter() {
	fmt.Printf("\x1b[2mPress Ctrl+C to exit | Refreshing every 1 second\x1b[0m\n")
}

// renderTask renders a single task.
func renderTask(t *todo.Task, verbose bool) {
	statusIcon := " "
	switch t.Status {
	case todo.StatusTodo:
		statusIcon = " "
	case todo.StatusDoing:
		statusIcon = ">"
	case todo.StatusBlocked:
		statusIcon = "!"
	case todo.StatusDone:
		statusIcon = "✓"
	}

	fmt.Printf("  %s \x1b[1m[%s]\x1b[0m (P%d) %s\n", statusIcon, t.ID, t.Priority, t.Title)

	if verbose && t.Details != "" {
		details := t.Details
		if len(details) > 60 {
			details = details[:57] + "..."
		}
		fmt.Printf("      \x1b[2m%s\x1b[0m\n", details)
	}
}

// runTUIWithLoop runs the TUI with the loop executing in background.
func runTUIWithLoop(ctx context.Context, cfg *config.Config, todoPath string) error {
	// Create a channel for loop updates
	statusCh := make(chan loop.Status, 10)

	// Start the loop in a goroutine
	go func() {
		l, err := loop.New(cfg, cfg.ProjectRoot)
		if err != nil {
			statusCh <- loop.Status{Error: err}
			return
		}

		if err := l.RunWithStatus(ctx, statusCh); err != nil {
			statusCh <- loop.Status{Error: err}
		}
		close(statusCh)
	}()

	// Process status updates
	for status := range statusCh {
		if status.Error != nil {
			fmt.Printf("\x1b[1;31mError: %v\x1b[0m\n", status.Error)
			return status.Error
		}
		// Render TUI with status
		fmt.Print("\x1b[2J\x1b[H")
		renderStatus(status)
		renderTUI(ctx, cfg, todoPath)
	}

	return nil
}

// renderStatus renders the current loop status.
func renderStatus(status loop.Status) {
	if status.Message != "" {
		fmt.Printf("\x1b[1;36m[Iter %d]\x1b[0m %s\n\n", status.Iteration, status.Message)
	}
}

// IsTTY returns true if stdout is a terminal.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, _ := f.Stat()
	return (info.Mode() & os.ModeCharDevice) != 0
}
