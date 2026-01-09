package task

import (
	"testing"
)

func TestComputeWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		task      *Task
		minWeight float64
		maxWeight float64
	}{
		{
			name: "known_task_go_bank_account",
			task: &Task{
				Slug:     "bank-account",
				Language: Go,
			},
			minWeight: 1.0,
			maxWeight: 1.1,
		},
		{
			name: "known_task_dart_isolate_pool",
			task: &Task{
				Slug:     "isolate-pool",
				Language: Dart,
			},
			minWeight: 1.4,
			maxWeight: 1.5,
		},
		{
			name: "known_task_zig_comptime_json",
			task: &Task{
				Slug:     "comptime-json",
				Language: Zig,
			},
			minWeight: 1.4,
			maxWeight: 1.5, // Capped at MaxWeight
		},
		{
			name: "unknown_task_gets_baseline",
			task: &Task{
				Slug:     "unknown-task",
				Language: Go,
			},
			minWeight: 1.0,
			maxWeight: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			weight := ComputeWeight(tt.task)
			if weight.Base < tt.minWeight || weight.Base > tt.maxWeight {
				t.Errorf("ComputeWeight() = %v, want between %v and %v",
					weight.Base, tt.minWeight, tt.maxWeight)
			}
		})
	}
}

func TestComputeWeightCap(t *testing.T) {
	t.Parallel()

	// zig/comptime-json has high factors but should be capped at 1.5
	task := &Task{
		Slug:     "comptime-json",
		Language: Zig,
	}
	weight := ComputeWeight(task)
	if weight.Base > MaxWeight {
		t.Errorf("ComputeWeight() = %v, should be capped at %v", weight.Base, MaxWeight)
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

	weight := Weight{Base: 1.5}

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
			want:   1.5,
		},
		{
			name:          "partial_pass",
			passed:        true,
			agentTimedOut: true,
			want:          1.125, // 1.5 * 0.75
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
			want:     -0.25, // ViolationPenalty
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

func TestScoringConstants(t *testing.T) {
	t.Parallel()

	if PartialPassMultiplier != 0.75 {
		t.Errorf("PartialPassMultiplier = %v, want 0.75", PartialPassMultiplier)
	}
	if ViolationPenalty != 0.25 {
		t.Errorf("ViolationPenalty = %v, want 0.25", ViolationPenalty)
	}
	if MaxWeight != 1.5 {
		t.Errorf("MaxWeight = %v, want 1.5", MaxWeight)
	}
}
