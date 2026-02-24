package cli

import (
	"math"
	"testing"
	"time"

	"github.com/lemon07r/sanityharness/internal/task"
)

func TestFinalizeEvalResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      EvalResult
		weight     task.Weight
		wantStatus task.ResultStatus
		wantScore  float64
		wantClass  FailureClass
	}{
		{
			name: "validation_error_sets_error_status_and_zero_score",
			input: EvalResult{
				Passed: false,
				Error:  "executing validation: exec timed out after 2m0s",
			},
			weight:     task.Weight{Base: 1.5},
			wantStatus: task.StatusError,
			wantScore:  0.0,
			wantClass:  FailureClassValidationTimeout,
		},
		{
			name: "integrity_violation_sets_penalty_score",
			input: EvalResult{
				Passed: false,
				Error:  "modified task files (disallowed): test.go",
			},
			weight:     task.Weight{Base: 1.4},
			wantStatus: task.StatusIntegrityViolation,
			wantScore:  -0.25,
			wantClass:  FailureClassIntegrity,
		},
		{
			name: "agent_timeout_with_pass_is_partial_pass",
			input: EvalResult{
				Passed:        true,
				AgentTimedOut: true,
			},
			weight:     task.Weight{Base: 1.2},
			wantStatus: task.StatusPartialPass,
			wantScore:  1.2,
			wantClass:  FailureClassNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.input
			start := time.Now().Add(-time.Second)

			finalizeEvalResult(&result, start, tt.weight)

			if result.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", result.Status, tt.wantStatus)
			}
			if math.Abs(result.WeightedScore-tt.wantScore) > 1e-9 {
				t.Fatalf("weighted_score = %v, want %v", result.WeightedScore, tt.wantScore)
			}
			if result.FailureClass != tt.wantClass {
				t.Fatalf("failure_class = %q, want %q", result.FailureClass, tt.wantClass)
			}
			if result.Duration <= 0 {
				t.Fatalf("duration must be > 0, got %v", result.Duration)
			}
		})
	}
}
