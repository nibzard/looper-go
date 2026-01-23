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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/loop"
	"github.com/nibzard/looper-go/internal/todo"
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

	if !IsTTY(os.Stdout) {
		return fmt.Errorf("tui requires a TTY")
	}

	if c.runLoop {
		return runTUIWithLoop(ctx, cfg, todoPath)
	}
	return runTUIViewer(ctx, cfg, todoPath)
}

// runTUIViewer runs a simple TUI viewer (basic terminal UI).
func runTUIViewer(ctx context.Context, cfg *config.Config, todoPath string) error {
	model := newTUIModel(cfg, todoPath, nil, false)
	return runProgram(ctx, model)
}

// runTUIWithLoop runs the TUI with the loop executing in background.
func runTUIWithLoop(ctx context.Context, cfg *config.Config, todoPath string) error {
	statusCh := make(chan loop.Status, 16)
	go func() {
		l, err := loop.New(cfg, cfg.ProjectRoot)
		if err != nil {
			statusCh <- loop.Status{Error: err}
			close(statusCh)
			return
		}
		if err := l.RunWithStatus(ctx, statusCh); err != nil {
			// RunWithStatus already reports errors on statusCh.
			return
		}
	}()

	model := newTUIModel(cfg, todoPath, statusCh, true)
	return runProgram(ctx, model)
}

func runProgram(ctx context.Context, model *tuiModel) error {
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(*tuiModel); ok {
		if m.loopErr != nil {
			return m.loopErr
		}
	}
	return nil
}

type tuiModel struct {
	cfg          *config.Config
	todoPath     string
	runLoop      bool
	statusCh     <-chan loop.Status
	loadErr      error
	data         *tuiData
	filteredData *tuiData
	lastStatus   loop.Status
	loopDone     bool
	loopErr      error
	tickInterval time.Duration
	filter       todo.Status // Filter by status
	showHelp     bool        // Show help screen
	showStatus   bool        // Toggle status display
}

type tuiData struct {
	counts       map[todo.Status]int
	currentLabel string
	currentTask  *todo.Task
	allDone      bool
	recent       []todo.Task
}

type tickMsg time.Time

type statusMsg struct {
	status loop.Status
}

type loopDoneMsg struct{}

func newTUIModel(cfg *config.Config, todoPath string, statusCh <-chan loop.Status, runLoop bool) *tuiModel {
	path := todoPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(cfg.ProjectRoot, path)
	}
	return &tuiModel{
		cfg:          cfg,
		todoPath:     path,
		runLoop:      runLoop,
		statusCh:     statusCh,
		tickInterval: time.Second,
	}
}

func (m *tuiModel) Init() tea.Cmd {
	m.refresh()
	cmds := []tea.Cmd{tickCmd(m.tickInterval)}
	if m.runLoop && m.statusCh != nil {
		cmds = append(cmds, waitForStatus(m.statusCh))
	}
	return tea.Batch(cmds...)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r", "f5":
			m.refresh()
			m.applyFilter()
			return m, nil
		case "s":
			m.showStatus = !m.showStatus
			return m, nil
		case "h", "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "1":
			m.filter = todo.StatusTodo
			m.applyFilter()
			return m, nil
		case "2":
			m.filter = todo.StatusDoing
			m.applyFilter()
			return m, nil
		case "3":
			m.filter = todo.StatusBlocked
			m.applyFilter()
			return m, nil
		case "4":
			m.filter = todo.StatusDone
			m.applyFilter()
			return m, nil
		case "0":
			m.filter = ""
			m.filteredData = nil
			return m, nil
		}
	case tickMsg:
		m.refresh()
		return m, tickCmd(m.tickInterval)
	case statusMsg:
		m.lastStatus = msg.status
		if msg.status.Error != nil {
			m.loopErr = msg.status.Error
			return m, tea.Quit
		}
		if msg.status.Status == "done" {
			m.loopDone = true
		}
		if m.runLoop && m.statusCh != nil {
			return m, waitForStatus(m.statusCh)
		}
	case loopDoneMsg:
		m.loopDone = true
	}

	return m, nil
}

func (m *tuiModel) View() string {
	var b strings.Builder
	writeTitle(&b)

	// Show help screen if enabled
	if m.showHelp {
		writeHelp(&b)
		writeFooter(&b, m.tickInterval)
		return b.String()
	}

	if m.runLoop && m.showStatus {
		writeStatusLine(&b, m.lastStatus, m.loopDone)
	}

	// Show filter indicator
	if m.filter != "" {
		b.WriteString(fmt.Sprintf("Filter: %s (0 to clear)\n\n", m.filter))
	}

	if m.loadErr != nil {
		b.WriteString("Error loading todo file:\n")
		b.WriteString("  " + m.loadErr.Error() + "\n\n")
		writeFooter(&b, m.tickInterval)
		return b.String()
	}
	if m.data == nil {
		b.WriteString("Loading...\n\n")
		writeFooter(&b, m.tickInterval)
		return b.String()
	}

	// Use filtered data if filter is active, otherwise use all data
	displayData := m.data
	if m.filteredData != nil {
		displayData = m.filteredData
	}

	writeOverview(&b, displayData)
	writeCurrentTask(&b, displayData)
	writeRecent(&b, displayData)
	writeConfig(&b, m.cfg, m.todoPath)
	writeFooter(&b, m.tickInterval)
	return b.String()
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForStatus(ch <-chan loop.Status) tea.Cmd {
	return func() tea.Msg {
		status, ok := <-ch
		if !ok {
			return loopDoneMsg{}
		}
		return statusMsg{status: status}
	}
}

func (m *tuiModel) refresh() {
	todoFile, err := todo.Load(m.todoPath)
	if err != nil {
		m.loadErr = err
		m.data = nil
		return
	}
	m.loadErr = nil
	m.data = buildTUIData(todoFile)
	m.applyFilter()
}

// applyFilter applies the current filter to the data.
func (m *tuiModel) applyFilter() {
	if m.data == nil || m.filter == "" {
		m.filteredData = nil
		return
	}

	// Create a filtered copy of the data
	filtered := &tuiData{
		counts: map[todo.Status]int{
			todo.StatusTodo:    0,
			todo.StatusDoing:   0,
			todo.StatusBlocked: 0,
			todo.StatusDone:    0,
		},
	}

	// Only show tasks matching the filter
	for status, count := range m.data.counts {
		if status == m.filter {
			filtered.counts[status] = count
		}
	}

	// Set current task if it matches the filter
	if m.data.currentTask != nil && m.data.currentTask.Status == m.filter {
		filtered.currentTask = m.data.currentTask
		filtered.currentLabel = m.data.currentLabel
		filtered.allDone = m.data.allDone
	}

	// Filter recent tasks
	for _, task := range m.data.recent {
		if task.Status == m.filter {
			filtered.recent = append(filtered.recent, task)
		}
	}

	m.filteredData = filtered
}

func buildTUIData(todoFile *todo.File) *tuiData {
	data := &tuiData{
		counts: map[todo.Status]int{
			todo.StatusTodo:    0,
			todo.StatusDoing:   0,
			todo.StatusBlocked: 0,
			todo.StatusDone:    0,
		},
	}

	for _, task := range todoFile.Tasks {
		data.counts[task.Status]++
	}

	if current := todoFile.FindTaskByStatus(todo.StatusDoing); current != nil {
		data.currentLabel = "Current Task"
		data.currentTask = current
	} else if next := todoFile.SelectTask(); next != nil {
		data.currentLabel = "Next Task"
		data.currentTask = next
	} else {
		data.currentLabel = "All Tasks Done"
		data.allDone = true
	}

	sorted := make([]todo.Task, len(todoFile.Tasks))
	copy(sorted, todoFile.Tasks)
	sort.Slice(sorted, func(i, j int) bool {
		left := sorted[i].UpdatedAt
		right := sorted[j].UpdatedAt
		if left == nil && right == nil {
			return false
		}
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.After(*right)
	})

	for _, task := range sorted {
		if task.Status != todo.StatusDone {
			continue
		}
		data.recent = append(data.recent, task)
		if len(data.recent) >= 5 {
			break
		}
	}

	return data
}

func writeTitle(b *strings.Builder) {
	title := "Looper TUI"
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("=", len(title)) + "\n\n")
}

func writeStatusLine(b *strings.Builder, status loop.Status, done bool) {
	if status.Message != "" {
		if status.Iteration > 0 {
			b.WriteString(fmt.Sprintf("[Iter %d] %s\n\n", status.Iteration, status.Message))
		} else {
			b.WriteString(status.Message + "\n\n")
		}
		return
	}
	if done {
		b.WriteString("Loop finished.\n\n")
	}
}

func writeOverview(b *strings.Builder, data *tuiData) {
	b.WriteString("Task Overview\n\n")
	b.WriteString(fmt.Sprintf("  Todo: %d  Doing: %d  Blocked: %d  Done: %d\n\n",
		data.counts[todo.StatusTodo],
		data.counts[todo.StatusDoing],
		data.counts[todo.StatusBlocked],
		data.counts[todo.StatusDone],
	))
}

func writeCurrentTask(b *strings.Builder, data *tuiData) {
	b.WriteString(data.currentLabel + "\n\n")
	if data.allDone {
		b.WriteString("  No pending tasks remaining.\n\n")
		return
	}
	if data.currentTask != nil {
		b.WriteString(formatTask(data.currentTask, true))
		b.WriteString("\n\n")
		return
	}
	b.WriteString("  No task selected.\n\n")
}

func writeRecent(b *strings.Builder, data *tuiData) {
	b.WriteString("Recently Completed\n\n")
	if len(data.recent) == 0 {
		b.WriteString("  No completed tasks yet.\n\n")
		return
	}
	for _, task := range data.recent {
		b.WriteString(formatTask(&task, false))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func writeConfig(b *strings.Builder, cfg *config.Config, todoPath string) {
	b.WriteString("Configuration\n\n")
	b.WriteString(fmt.Sprintf("  Max Iters: %d\n", cfg.MaxIterations))
	b.WriteString(fmt.Sprintf("  Todo File: %s\n\n", todoPath))
}

func writeHelp(b *strings.Builder) {
	b.WriteString("Keyboard Shortcuts\n\n")
	b.WriteString("  q, ctrl+c    Quit\n")
	b.WriteString("  r, F5        Refresh data\n")
	b.WriteString("  s            Toggle status display\n")
	b.WriteString("  h, ?         Toggle this help screen\n")
	b.WriteString("  1            Filter by todo\n")
	b.WriteString("  2            Filter by doing\n")
	b.WriteString("  3            Filter by blocked\n")
	b.WriteString("  4            Filter by done\n")
	b.WriteString("  0            Clear filter\n\n")
}

func writeFooter(b *strings.Builder, interval time.Duration) {
	b.WriteString(fmt.Sprintf("Press h for help | q to quit | Refreshing every %s\n", interval))
}

func formatTask(t *todo.Task, verbose bool) string {
	statusIcon := " "
	switch t.Status {
	case todo.StatusTodo:
		statusIcon = " "
	case todo.StatusDoing:
		statusIcon = ">"
	case todo.StatusBlocked:
		statusIcon = "!"
	case todo.StatusDone:
		statusIcon = "x"
	}

	line := fmt.Sprintf("  %s [%s] (P%d) %s", statusIcon, t.ID, t.Priority, t.Title)
	if !verbose || t.Details == "" {
		return line
	}
	details := t.Details
	if len(details) > 60 {
		details = details[:57] + "..."
	}
	return line + "\n      " + details
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
