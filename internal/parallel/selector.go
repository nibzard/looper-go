package parallel

import (
	"sort"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/todo"
)

// TaskSelector selects multiple tasks based on a strategy.
type TaskSelector struct {
	file     *todo.File
	strategy config.ParallelStrategy
}

// NewTaskSelector creates a new task selector.
func NewTaskSelector(file *todo.File, strategy config.ParallelStrategy) *TaskSelector {
	return &TaskSelector{
		file:     file,
		strategy: strategy,
	}
}

// SelectTasks selects up to n tasks based on the configured strategy.
// It returns tasks that are ready to execute (dependencies satisfied)
// ordered according to the strategy.
func (s *TaskSelector) SelectTasks(n int) []*todo.Task {
	// Get all ready tasks (dependencies satisfied)
	readyTasks := s.getReadyTasks()

	if len(readyTasks) == 0 {
		return nil
	}

	// Sort according to strategy
	s.sortTasks(readyTasks)

	// Return up to n tasks
	if n > 0 && n < len(readyTasks) {
		return readyTasks[:n]
	}
	return readyTasks
}

// getReadyTasks returns all tasks whose dependencies are satisfied and are not done.
func (s *TaskSelector) getReadyTasks() []*todo.Task {
	var ready []*todo.Task
	for i := range s.file.Tasks {
		task := &s.file.Tasks[i]
		// Only include tasks that are not done and have satisfied dependencies
		if task.Status != todo.StatusDone && s.file.DependenciesSatisfied(task) {
			ready = append(ready, task)
		}
	}
	return ready
}

// sortTasks sorts tasks according to the configured strategy.
func (s *TaskSelector) sortTasks(tasks []*todo.Task) {
	switch s.strategy {
	case config.StrategyPriority:
		s.sortByPriority(tasks)
	case config.StrategyDependency:
		s.sortByDependency(tasks)
	case config.StrategyMixed:
		s.sortByMixed(tasks)
	default:
		s.sortByPriority(tasks)
	}
}

// sortByPriority sorts by priority (lower first), then by ID for ties.
func (s *TaskSelector) sortByPriority(tasks []*todo.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		return todo.CompareIDs(tasks[i].ID, tasks[j].ID)
	})
}

// sortByDependency sorts to maximize parallel execution by prioritizing
// tasks that unblock other tasks.
func (s *TaskSelector) sortByDependency(tasks []*todo.Task) {
	// Count how many tasks each task blocks
	blocksCount := make(map[string]int)
	for _, task := range tasks {
		for _, depID := range task.DependsOn {
			blocksCount[depID]++
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		// Tasks that block more other tasks should run first
		blocksI := blocksCount[tasks[i].ID]
		blocksJ := blocksCount[tasks[j].ID]
		if blocksI != blocksJ {
			return blocksI > blocksJ
		}
		// Tie-break by priority
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		// Final tie-break by ID
		return todo.CompareIDs(tasks[i].ID, tasks[j].ID)
	})
}

// sortByMixed balances priority and dependency awareness.
func (s *TaskSelector) sortByMixed(tasks []*todo.Task) {
	// First, group by priority
	priorityGroups := make(map[int][]*todo.Task)
	for _, task := range tasks {
		priorityGroups[task.Priority] = append(priorityGroups[task.Priority], task)
	}

	// Sort priorities
	priorities := make([]int, 0, len(priorityGroups))
	for p := range priorityGroups {
		priorities = append(priorities, p)
	}
	sort.Ints(priorities)

	// Within each priority, sort by dependency
	var result []*todo.Task
	for _, p := range priorities {
		group := priorityGroups[p]
		// Create a temporary selector with dependency strategy
		tempSelector := &TaskSelector{
			file:     s.file,
			strategy: config.StrategyDependency,
		}
		tempSelector.sortByDependency(group)
		result = append(result, group...)
	}

	// Copy back to original slice
	for i, task := range result {
		tasks[i] = task
	}
}

// CountReady returns the number of tasks ready to execute.
func (s *TaskSelector) CountReady() int {
	return len(s.getReadyTasks())
}
