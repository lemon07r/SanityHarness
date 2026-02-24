package cli

import (
	"math"
	"strings"
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

func TestShouldSkipValidationForExternalFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		class           FailureClass
		err             string
		wantSkip        bool
		wantErrContains string
		wantInfra       bool
		wantQuota       bool
	}{
		{
			name:            "infra_failure_is_skipped",
			class:           FailureClassInfra,
			wantSkip:        true,
			wantErrContains: "infra failure",
			wantInfra:       true,
		},
		{
			name:            "auth_failure_is_skipped",
			class:           FailureClassAuth,
			wantSkip:        true,
			wantErrContains: "auth failure",
		},
		{
			name:            "quota_exhausted_is_skipped",
			class:           FailureClassQuotaExhausted,
			wantSkip:        true,
			wantErrContains: "quota failure",
			wantQuota:       true,
		},
		{
			name:     "normal_failure_is_not_skipped",
			class:    FailureClassValidationError,
			wantSkip: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := EvalResult{
				FailureClass: tc.class,
				Error:        tc.err,
			}
			skipped := shouldSkipValidationForExternalFailure(&r)
			if skipped != tc.wantSkip {
				t.Fatalf("shouldSkipValidationForExternalFailure() = %v, want %v", skipped, tc.wantSkip)
			}
			if tc.wantErrContains != "" && !strings.Contains(r.Error, tc.wantErrContains) {
				t.Fatalf("error = %q, want contains %q", r.Error, tc.wantErrContains)
			}
			if r.InfraFailure != tc.wantInfra {
				t.Fatalf("infra_failure = %v, want %v", r.InfraFailure, tc.wantInfra)
			}
			if r.QuotaExhausted != tc.wantQuota {
				t.Fatalf("quota_exhausted = %v, want %v", r.QuotaExhausted, tc.wantQuota)
			}
		})
	}
}
