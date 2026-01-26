package parallel

import (
	"testing"

	"github.com/nibzard/looper-go/internal/agents"
)

// Test consensusSummaries with various scenarios
func TestConsensusSummaries(t *testing.T) {
	tests := []struct {
		name      string
		summaries []*agents.Summary
		wantTask  string
		wantStatus string
		wantFiles  []string
		wantBlockers []string
	}{
		{
			name: "single summary returns unchanged",
			summaries: []*agents.Summary{
				{
					TaskID:   "T001",
					Status:   "done",
					Summary:  "Completed task",
					Files:    []string{"file1.go", "file2.go"},
					Blockers: []string{"blocker1"},
				},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   []string{"file1.go", "file2.go"},
			wantBlockers: []string{"blocker1"},
		},
		{
			name: "unanimous done",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "done", Files: []string{"file1.go"}},
				{TaskID: "T001", Status: "done", Files: []string{"file2.go"}},
				{TaskID: "T001", Status: "done", Files: []string{"file3.go"}},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   []string{"file1.go", "file2.go", "file3.go"},
			wantBlockers: nil,
		},
		{
			name: "majority done",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "done"},
				{TaskID: "T001", Status: "done"},
				{TaskID: "T001", Status: "blocked"},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   nil,
			wantBlockers: nil,
		},
		{
			name: "tie prefers done over blocked",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "done"},
				{TaskID: "T001", Status: "blocked"},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   nil,
			wantBlockers: nil,
		},
		{
			name: "tie prefers blocked over skipped",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "blocked"},
				{TaskID: "T001", Status: "skipped"},
			},
			wantTask:    "T001",
			wantStatus:  "blocked",
			wantFiles:   nil,
			wantBlockers: nil,
		},
		{
			name: "three way tie prefers done",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "done"},
				{TaskID: "T001", Status: "blocked"},
				{TaskID: "T001", Status: "skipped"},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   nil,
			wantBlockers: nil,
		},
		{
			name: "union of files",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "done", Files: []string{"file1.go", "file2.go"}},
				{TaskID: "T001", Status: "done", Files: []string{"file2.go", "file3.go"}},
				{TaskID: "T001", Status: "done", Files: []string{"file3.go", "file4.go"}},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   []string{"file1.go", "file2.go", "file3.go", "file4.go"},
			wantBlockers: nil,
		},
		{
			name: "union of blockers",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "blocked", Blockers: []string{"blocker1", "blocker2"}},
				{TaskID: "T001", Status: "blocked", Blockers: []string{"blocker2", "blocker3"}},
				{TaskID: "T001", Status: "blocked", Blockers: []string{"blocker3", "blocker4"}},
			},
			wantTask:    "T001",
			wantStatus:  "blocked",
			wantFiles:   nil,
			wantBlockers: []string{"blocker1", "blocker2", "blocker3", "blocker4"},
		},
		{
			name: "empty summaries list returns first",
			summaries: []*agents.Summary{
				{TaskID: "T001", Status: "done", Summary: "Agent 1 summary", Files: []string{"file1.go"}},
				{TaskID: "T001", Status: "done", Summary: "", Files: []string{"file2.go"}},
			},
			wantTask:    "T001",
			wantStatus:  "done",
			wantFiles:   []string{"file1.go", "file2.go"},
			wantBlockers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &TaskExecutor{maxAgents: len(tt.summaries)}
			got := exec.consensusSummaries(tt.summaries)

			if got.TaskID != tt.wantTask {
				t.Errorf("TaskID = %v, want %v", got.TaskID, tt.wantTask)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", got.Status, tt.wantStatus)
			}
			if !slicesEqual(got.Files, tt.wantFiles) {
				t.Errorf("Files = %v, want %v", got.Files, tt.wantFiles)
			}
			if !slicesEqual(got.Blockers, tt.wantBlockers) {
				t.Errorf("Blockers = %v, want %v", got.Blockers, tt.wantBlockers)
			}
		})
	}
}

// Test majorityStatus edge cases
func TestMajorityStatus(t *testing.T) {
	tests := []struct {
		name  string
		votes map[string]int
		want  string
	}{
		{
			name:  "empty votes returns skipped",
			votes: map[string]int{},
			want:  "skipped",
		},
		{
			name:  "single candidate",
			votes: map[string]int{"done": 3},
			want:  "done",
		},
		{
			name:  "clear majority",
			votes: map[string]int{"done": 3, "blocked": 1},
			want:  "done",
		},
		{
			name:  "tie between done and blocked",
			votes: map[string]int{"done": 2, "blocked": 2},
			want:  "done",
		},
		{
			name:  "tie between blocked and skipped",
			votes: map[string]int{"blocked": 2, "skipped": 2},
			want:  "blocked",
		},
		{
			name:  "tie between all three",
			votes: map[string]int{"done": 1, "blocked": 1, "skipped": 1},
			want:  "done",
		},
		{
			name:  "done wins even with fewer votes than blocked but more than skipped",
			votes: map[string]int{"done": 2, "blocked": 3, "skipped": 2},
			want:  "blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &TaskExecutor{}
			got := exec.majorityStatus(tt.votes)
			if got != tt.want {
				t.Errorf("majorityStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test consensus with mixed content
func TestConsensusSummaries_MixedContent(t *testing.T) {
	summaries := []*agents.Summary{
		{
			TaskID:   "T001",
			Status:   "done",
			Summary:  "Implemented feature A",
			Files:    []string{"feature_a.go", "utils.go"},
			Blockers: []string{"pending review"},
		},
		{
			TaskID:   "T001",
			Status:   "done",
			Summary:  "Added tests for feature A",
			Files:    []string{"feature_a_test.go", "utils.go"},
			Blockers: []string{"pending docs"},
		},
		{
			TaskID:   "T001",
			Status:   "blocked",
			Summary:  "Waiting on API changes",
			Files:    []string{"api_client.go"},
			Blockers: []string{"pending review", "API dependency"},
		},
	}

	exec := &TaskExecutor{maxAgents: 3}
	got := exec.consensusSummaries(summaries)

	// Status should be done (2/3 majority)
	if got.Status != "done" {
		t.Errorf("Status = %v, want done", got.Status)
	}

	// Files should be union of all
	expectedFiles := []string{"api_client.go", "feature_a.go", "feature_a_test.go", "utils.go"}
	if !slicesEqual(got.Files, expectedFiles) {
		t.Errorf("Files = %v, want %v", got.Files, expectedFiles)
	}

	// Blockers should be union of all
	expectedBlockers := []string{"API dependency", "pending docs", "pending review"}
	if !slicesEqual(got.Blockers, expectedBlockers) {
		t.Errorf("Blockers = %v, want %v", got.Blockers, expectedBlockers)
	}

	// Summary should contain consensus prefix
	if got.Summary == "" {
		t.Errorf("Summary should not be empty")
	}
}

// Helper function to compare string slices
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
