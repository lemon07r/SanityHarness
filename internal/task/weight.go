// Package task provides task definition and loading for SanityHarness.
package task

import (
	"bytes"
)

// WeightVersion identifies the scoring methodology version for attestation.
const WeightVersion = "1.0"

// Weight holds computed difficulty factors for a task.
type Weight struct {
	Base            float64 `json:"base"`
	TestComplexity  float64 `json:"test_complexity"`
	HiddenTestRatio float64 `json:"hidden_test_ratio"`
	TimeoutFactor   float64 `json:"timeout_factor"`
	TierBonus       float64 `json:"tier_bonus"`
}

// ComputeWeight calculates a task's difficulty weight based on objective factors.
// The weight is computed from:
//   - Test line count (more tests = more edge cases to handle)
//   - Hidden test presence and ratio (indicates intentional traps)
//   - Agent timeout override (task author expected difficulty)
//   - Tier (extended tier generally contains harder tasks)
//
// Returns a Weight struct with the base score and component factors.
func ComputeWeight(t *Task, testContent, hiddenTestContent []byte) Weight {
	w := Weight{
		Base: 1.0,
	}

	// Factor 1: Test complexity (more tests = more edge cases)
	testLines := countLines(testContent)
	hiddenLines := countLines(hiddenTestContent)
	totalTestLines := testLines + hiddenLines

	// Normalize: 200 lines = 0.5 bonus, capped at 0.5
	w.TestComplexity = min(float64(totalTestLines)/200.0, 0.5)
	w.Base += w.TestComplexity

	// Factor 2: Hidden test presence and ratio
	if len(t.Files.HiddenTest) > 0 && totalTestLines > 0 {
		w.Base += 0.3 // Base bonus for having hidden tests
		w.HiddenTestRatio = float64(hiddenLines) / float64(totalTestLines)
		w.Base += w.HiddenTestRatio * 0.4 // Up to 0.4 additional based on ratio
	}

	// Factor 3: Extended timeout signals author-expected difficulty
	defaultTimeout := 120
	if t.AgentTimeout > defaultTimeout {
		w.TimeoutFactor = float64(t.AgentTimeout-defaultTimeout) / 180.0 * 0.3
		w.Base += w.TimeoutFactor
	}

	// Factor 4: Tier bonus (extended tier generally harder)
	if t.Tier == "extended" {
		w.TierBonus = 0.2
		w.Base += w.TierBonus
	}

	return w
}

// countLines counts the number of newlines in content.
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	return bytes.Count(content, []byte{'\n'}) + 1
}

// ResultStatus represents the outcome status of a task evaluation.
type ResultStatus string

const (
	StatusPass               ResultStatus = "pass"
	StatusPartialPass        ResultStatus = "partial_pass"
	StatusFail               ResultStatus = "fail"
	StatusIntegrityViolation ResultStatus = "integrity_violation"
	StatusError              ResultStatus = "error"
)

// DetermineStatus computes the result status from pass/timeout/error state.
func DetermineStatus(passed, agentTimedOut bool, errorMsg string) ResultStatus {
	if errorMsg != "" {
		if contains(errorMsg, "modified task files") {
			return StatusIntegrityViolation
		}
		return StatusError
	}
	if passed {
		if agentTimedOut {
			return StatusPartialPass
		}
		return StatusPass
	}
	return StatusFail
}

// contains checks if s contains substr (simple helper to avoid strings import).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr) >= 0
}

// searchString finds substr in s, returns index or -1.
func searchString(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}

// ScoreResult computes the weighted score for a task result.
func ScoreResult(passed, agentTimedOut bool, errorMsg string, weight Weight) float64 {
	status := DetermineStatus(passed, agentTimedOut, errorMsg)

	switch status {
	case StatusPass:
		return weight.Base
	case StatusPartialPass:
		return weight.Base * 0.7 // 30% reduction for timeout
	case StatusIntegrityViolation:
		return -0.5 // Penalty
	default:
		return 0.0
	}
}
