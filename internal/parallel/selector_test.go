package parallel

import (
	"testing"
	"time"

	"github.com/nibzard/looper-go/internal/config"
	"github.com/nibzard/looper-go/internal/todo"
)

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func makeTask(id, title string, priority int, status todo.Status, dependsOn []string) todo.Task {
	return todo.Task{
		ID:        id,
		Title:     title,
		Priority:  priority,
		Status:    status,
		DependsOn: dependsOn,
		CreatedAt: timePtr(mustParseTime("2024-01-01T00:00:00Z")),
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestNewTaskSelector(t *testing.T) {
	file := &todo.File{}
	selector := NewTaskSelector(file, config.StrategyPriority)

	if selector == nil {
		t.Fatal("NewTaskSelector returned nil")
	}
	if selector.file != file {
		t.Error("file not set correctly")
	}
	if selector.strategy != config.StrategyPriority {
		t.Error("strategy not set correctly")
	}
}

func TestTaskSelector_GetReadyTasks(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tests := []struct {
		name     string
		tasks    []todo.Task
		expected []string // IDs of ready tasks
	}{
		{
			name: "all tasks ready",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
				{ID: "T002", Title: "Task 2", Priority: 2, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
			},
			expected: []string{"T001", "T002"},
		},
		{
			name: "task with dependency blocked",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusDone, CreatedAt: timePtr(now)},
				{ID: "T002", Title: "Task 2", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T001"}, CreatedAt: timePtr(now)},
				{ID: "T003", Title: "Task 3", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T002"}, CreatedAt: timePtr(now)},
			},
			expected: []string{"T002"}, // T003 depends on T002 which is not done
		},
		{
			name: "done tasks not ready",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusDone, CreatedAt: timePtr(now)},
				{ID: "T002", Title: "Task 2", Priority: 2, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
			},
			expected: []string{"T002"},
		},
		{
			name: "blocked tasks with satisfied dependencies",
			tasks: []todo.Task{
				{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusDone, CreatedAt: timePtr(now)},
				{ID: "T002", Title: "Task 2", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T001"}, CreatedAt: timePtr(now)},
				{ID: "T003", Title: "Task 3", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
			},
			expected: []string{"T002", "T003"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &todo.File{Tasks: tt.tasks}
			selector := NewTaskSelector(file, config.StrategyPriority)

			ready := selector.getReadyTasks()

			if len(ready) != len(tt.expected) {
				t.Fatalf("expected %d ready tasks, got %d", len(tt.expected), len(ready))
			}

			readyIDs := make(map[string]bool)
			for _, task := range ready {
				readyIDs[task.ID] = true
			}

			for _, expectedID := range tt.expected {
				if !readyIDs[expectedID] {
					t.Errorf("expected task %s to be ready", expectedID)
				}
			}
		})
	}
}

func TestTaskSelector_SortByPriority(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tasks := []*todo.Task{
		{ID: "T003", Title: "Task 3", Priority: 3, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T002a", Title: "Task 2a", Priority: 2, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T010", Title: "Task 10", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T002b", Title: "Task 2b", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
	}

	file := &todo.File{}
	selector := NewTaskSelector(file, config.StrategyPriority)
	selector.sortByPriority(tasks)

	// Should be sorted by priority, then by numeric ID
	expectedOrder := []string{"T001", "T002b", "T010", "T002a", "T003"}
	for i, task := range tasks {
		if task.ID != expectedOrder[i] {
			t.Errorf("position %d: expected %s, got %s", i, expectedOrder[i], task.ID)
		}
	}
}

func TestTaskSelector_SortByDependency(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tasks := []*todo.Task{
		{ID: "T001", Title: "Task 1", Priority: 2, Status: todo.StatusTodo, DependsOn: []string{}, CreatedAt: timePtr(now)},
		{ID: "T002", Title: "Task 2", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T001"}, CreatedAt: timePtr(now)},
		{ID: "T003", Title: "Task 3", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T001", "T002"}, CreatedAt: timePtr(now)},
	}

	file := &todo.File{}
	selector := NewTaskSelector(file, config.StrategyDependency)
	selector.sortByDependency(tasks)

	// T001 should be first (blocks 2 tasks), T002 second (blocks 1 task), T003 last (blocks 0)
	expectedOrder := []string{"T001", "T002", "T003"}
	for i, task := range tasks {
		if task.ID != expectedOrder[i] {
			t.Errorf("position %d: expected %s, got %s", i, expectedOrder[i], task.ID)
		}
	}
}

func TestTaskSelector_SortByMixed(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tasks := []*todo.Task{
		// Priority 1 tasks
		{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{}, CreatedAt: timePtr(now)},
		{ID: "T002", Title: "Task 2", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T001"}, CreatedAt: timePtr(now)},
		// Priority 2 tasks
		{ID: "T003", Title: "Task 3", Priority: 2, Status: todo.StatusTodo, DependsOn: []string{}, CreatedAt: timePtr(now)},
		{ID: "T004", Title: "Task 4", Priority: 2, Status: todo.StatusTodo, DependsOn: []string{"T003"}, CreatedAt: timePtr(now)},
	}

	file := &todo.File{}
	selector := NewTaskSelector(file, config.StrategyMixed)
	selector.sortByMixed(tasks)

	// Within priority 1: T001 (blocks T002) then T002
	// Within priority 2: T003 (blocks T004) then T004
	expectedOrder := []string{"T001", "T002", "T003", "T004"}
	for i, task := range tasks {
		if task.ID != expectedOrder[i] {
			t.Errorf("position %d: expected %s, got %s", i, expectedOrder[i], task.ID)
		}
	}
}

func TestTaskSelector_SelectTasks(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tasks := []todo.Task{
		{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T002", Title: "Task 2", Priority: 2, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T003", Title: "Task 3", Priority: 3, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T004", Title: "Task 4", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T005", Title: "Task 5", Priority: 2, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
	}

	file := &todo.File{Tasks: tasks}
	selector := NewTaskSelector(file, config.StrategyPriority)

	t.Run("selects all when n is large", func(t *testing.T) {
		selected := selector.SelectTasks(100)
		if len(selected) != 5 {
			t.Errorf("expected 5 tasks, got %d", len(selected))
		}
	})

	t.Run("selects n when n is smaller", func(t *testing.T) {
		selected := selector.SelectTasks(3)
		if len(selected) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(selected))
		}
		// Should select priority 1 tasks first
		if selected[0].ID != "T001" && selected[0].ID != "T004" {
			t.Errorf("expected first task to have priority 1, got %s with priority %d", selected[0].ID, selected[0].Priority)
		}
	})

	t.Run("selects all when n is 0", func(t *testing.T) {
		selected := selector.SelectTasks(0)
		if len(selected) != 5 {
			t.Errorf("expected 5 tasks (0 means unlimited), got %d", len(selected))
		}
	})

	t.Run("returns nil when no ready tasks", func(t *testing.T) {
		doneFile := &todo.File{Tasks: []todo.Task{
			{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusDone, CreatedAt: timePtr(now)},
		}}
		doneSelector := NewTaskSelector(doneFile, config.StrategyPriority)

		selected := doneSelector.SelectTasks(10)
		if selected != nil && len(selected) != 0 {
			t.Errorf("expected no tasks, got %d", len(selected))
		}
	})
}

func TestTaskSelector_CountReady(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tasks := []todo.Task{
		{ID: "T001", Title: "Task 1", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T002", Title: "Task 2", Priority: 1, Status: todo.StatusDone, CreatedAt: timePtr(now)},
		{ID: "T003", Title: "Task 3", Priority: 1, Status: todo.StatusTodo, DependsOn: []string{"T002"}, CreatedAt: timePtr(now)},
	}

	file := &todo.File{Tasks: tasks}
	selector := NewTaskSelector(file, config.StrategyPriority)

	count := selector.CountReady()
	// T001 is ready, T002 is done, T003 depends on T002 (done) so it's ready too
	if count != 2 {
		t.Errorf("expected 2 ready tasks, got %d", count)
	}
}

func TestTaskSelector_Strategies(t *testing.T) {
	now := mustParseTime("2024-01-01T00:00:00Z")

	tasks := []todo.Task{
		{ID: "T003", Title: "Low priority task", Priority: 3, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T001", Title: "High priority task", Priority: 1, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
		{ID: "T002", Title: "Medium priority task", Priority: 2, Status: todo.StatusTodo, CreatedAt: timePtr(now)},
	}

	file := &todo.File{Tasks: tasks}

	t.Run("priority strategy", func(t *testing.T) {
		selector := NewTaskSelector(file, config.StrategyPriority)
		selected := selector.SelectTasks(10)

		if selected[0].ID != "T001" {
			t.Errorf("priority strategy: expected T001 first, got %s", selected[0].ID)
		}
	})

	t.Run("dependency strategy", func(t *testing.T) {
		selector := NewTaskSelector(file, config.StrategyDependency)
		selected := selector.SelectTasks(10)

		// All have no dependencies, so should be sorted by priority as tie-break
		if selected[0].ID != "T001" {
			t.Errorf("dependency strategy: expected T001 first, got %s", selected[0].ID)
		}
	})

	t.Run("mixed strategy", func(t *testing.T) {
		selector := NewTaskSelector(file, config.StrategyMixed)
		selected := selector.SelectTasks(10)

		// Should prioritize by priority first
		if selected[0].ID != "T001" {
			t.Errorf("mixed strategy: expected T001 first, got %s", selected[0].ID)
		}
	})

	t.Run("default strategy", func(t *testing.T) {
		selector := NewTaskSelector(file, "invalid")
		selected := selector.SelectTasks(10)

		// Should fall back to priority
		if selected[0].ID != "T001" {
			t.Errorf("default strategy: expected T001 first, got %s", selected[0].ID)
		}
	})
}
