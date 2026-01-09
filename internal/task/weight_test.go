package task

import (
	"testing"
)

func TestComputeWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		task          *Task
		testContent   []byte
		hiddenContent []byte
		minWeight     float64
		maxWeight     float64
	}{
		{
			name: "basic_task_no_hidden_tests",
			task: &Task{
				Slug:     "test-task",
				Language: Go,
				Tier:     "core",
			},
			testContent:   make([]byte, 100), // 100 lines equivalent
			hiddenContent: nil,
			minWeight:     1.0,
			maxWeight:     1.5,
		},
		{
			name: "task_with_hidden_tests",
			task: &Task{
				Slug:     "test-task",
				Language: Go,
				Tier:     "core",
				Files: TaskFiles{
					HiddenTest: []string{"hidden_test.go"},
				},
			},
			testContent:   make([]byte, 100),
			hiddenContent: make([]byte, 100),
			minWeight:     1.5,
			maxWeight:     2.2,
		},
		{
			name: "extended_tier_task",
			task: &Task{
				Slug:     "test-task",
				Language: Go,
				Tier:     "extended",
			},
			testContent:   make([]byte, 100),
			hiddenContent: nil,
			minWeight:     1.2,
			maxWeight:     1.7,
		},
		{
			name: "task_with_extended_timeout",
			task: &Task{
				Slug:         "test-task",
				Language:     Go,
				Tier:         "core",
				AgentTimeout: 300, // 180 seconds over default
			},
			testContent:   make([]byte, 100),
			hiddenContent: nil,
			minWeight:     1.0,
			maxWeight:     1.8,
		},
		{
			name: "complex_task_all_factors",
			task: &Task{
				Slug:         "complex-task",
				Language:     Rust,
				Tier:         "extended",
				AgentTimeout: 240,
				Files: TaskFiles{
					HiddenTest: []string{"hidden_test.rs"},
				},
			},
			testContent:   make([]byte, 200),
			hiddenContent: make([]byte, 100),
			minWeight:     1.8,
			maxWeight:     2.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			weight := ComputeWeight(tt.task, tt.testContent, tt.hiddenContent)
			if weight.Base < tt.minWeight || weight.Base > tt.maxWeight {
				t.Errorf("ComputeWeight() = %v, want between %v and %v",
					weight.Base, tt.minWeight, tt.maxWeight)
			}
		})
	}
}

func TestDetermineStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		passed        bool
		agentTimedOut bool
		errorMsg      string
		want          ResultStatus
	}{
		{
			name:   "clean_pass",
			passed: true,
			want:   StatusPass,
		},
		{
			name:          "partial_pass_with_timeout",
			passed:        true,
			agentTimedOut: true,
			want:          StatusPartialPass,
		},
		{
			name:   "fail",
			passed: false,
			want:   StatusFail,
		},
		{
			name:     "integrity_violation",
			passed:   false,
			errorMsg: "modified task files (disallowed): test.go",
			want:     StatusIntegrityViolation,
		},
		{
			name:     "other_error",
			passed:   false,
			errorMsg: "init failed: something went wrong",
			want:     StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DetermineStatus(tt.passed, tt.agentTimedOut, tt.errorMsg)
			if got != tt.want {
				t.Errorf("DetermineStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScoreResult(t *testing.T) {
	t.Parallel()

	weight := Weight{Base: 2.0}

	tests := []struct {
		name          string
		passed        bool
		agentTimedOut bool
		errorMsg      string
		want          float64
	}{
		{
			name:   "clean_pass",
			passed: true,
			want:   2.0,
		},
		{
			name:          "partial_pass",
			passed:        true,
			agentTimedOut: true,
			want:          1.4, // 2.0 * 0.7
		},
		{
			name:   "fail",
			passed: false,
			want:   0.0,
		},
		{
			name:     "integrity_violation",
			passed:   false,
			errorMsg: "modified task files (disallowed): test.go",
			want:     -0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreResult(tt.passed, tt.agentTimedOut, tt.errorMsg, weight)
			if got != tt.want {
				t.Errorf("ScoreResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content []byte
		want    int
	}{
		{
			name:    "empty",
			content: nil,
			want:    0,
		},
		{
			name:    "single_line",
			content: []byte("hello"),
			want:    1,
		},
		{
			name:    "multiple_lines",
			content: []byte("line1\nline2\nline3"),
			want:    3,
		},
		{
			name:    "trailing_newline",
			content: []byte("line1\nline2\n"),
			want:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := countLines(tt.content)
			if got != tt.want {
				t.Errorf("countLines() = %v, want %v", got, tt.want)
			}
		})
	}
}
