// Package task provides task definition and loading for SanityHarness.
package task

// WeightVersion identifies the scoring methodology version for attestation.
const WeightVersion = "2.0"

// Scoring constants.
const (
	// PartialPassMultiplier is applied to tasks that passed but agent timed out.
	PartialPassMultiplier = 0.75

	// ViolationPenalty is subtracted for integrity violations (modified test files).
	ViolationPenalty = 0.25

	// MaxWeight caps the maximum weight for any task.
	MaxWeight = 1.5
)

// TaskDifficulty holds the empirically-derived difficulty factors for a task.
// These factors were calibrated based on analysis of what makes tasks hard for AI agents:
//   - Language rarity in training data (Dart, Kotlin coroutines > Go, TypeScript)
//   - Esoteric language features (comptime, isolates, macros)
//   - Novel algorithms vs well-known patterns
//   - Edge case density (streaming, concurrency + error handling)
//   - Novel vs classic problems
type TaskDifficulty struct {
	LangRarity      float64 // 0.0-0.4: Dart=0.4, Kotlin=0.3, Zig=0.2, others=0.0
	EsotericFeature float64 // 0.0-0.5: comptime=0.5, macros=0.5, isolates=0.4, etc.
	NovelAlgorithm  float64 // 0.0-0.4: regex from scratch=0.4, parser=0.2
	EdgeCaseDensity float64 // 0.0-0.5: streaming+chunks=0.5, concurrency+errors=0.4
	NovelProblem    float64 // 0.0-0.3: less documented patterns
}

// taskDifficulties contains empirically-derived difficulty factors for each task.
// These were calibrated by analyzing pass/fail patterns across multiple agents.
var taskDifficulties = map[string]TaskDifficulty{
	"dart/future-pool":               {LangRarity: 0.4, EsotericFeature: 0.2, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.2, NovelProblem: 0.1},
	"dart/isolate-pool":              {LangRarity: 0.4, EsotericFeature: 0.4, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.2, NovelProblem: 0.2},
	"dart/reactive-cache":            {LangRarity: 0.4, EsotericFeature: 0.2, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.3, NovelProblem: 0.1},
	"go/bank-account":                {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.1, NovelProblem: 0.0},
	"go/dining-philosophers":         {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.1, NovelProblem: 0.0},
	"go/errgroup-limit":              {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.3, NovelProblem: 0.1},
	"go/parallel-letter-frequency":   {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.1, NovelProblem: 0.0},
	"go/react":                       {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.1, EdgeCaseDensity: 0.2, NovelProblem: 0.0},
	"go/singleflight":                {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.1, EdgeCaseDensity: 0.4, NovelProblem: 0.3},
	"kotlin/channel-multiplexer":     {LangRarity: 0.3, EsotericFeature: 0.3, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.3, NovelProblem: 0.2},
	"kotlin/flow-processor":          {LangRarity: 0.3, EsotericFeature: 0.3, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.2, NovelProblem: 0.2},
	"kotlin/lru-cache":               {LangRarity: 0.1, EsotericFeature: 0.0, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.1, NovelProblem: 0.0},
	"rust/circular-buffer":           {LangRarity: 0.0, EsotericFeature: 0.1, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.1, NovelProblem: 0.0},
	"rust/doubly-linked-list":        {LangRarity: 0.0, EsotericFeature: 0.2, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.2, NovelProblem: 0.0},
	"rust/generational-arena":        {LangRarity: 0.0, EsotericFeature: 0.1, NovelAlgorithm: 0.1, EdgeCaseDensity: 0.2, NovelProblem: 0.1},
	"rust/macros":                    {LangRarity: 0.0, EsotericFeature: 0.5, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.2, NovelProblem: 0.2},
	"rust/parallel-letter-frequency": {LangRarity: 0.0, EsotericFeature: 0.1, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.1, NovelProblem: 0.0},
	"rust/regex-lite":                {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.4, EdgeCaseDensity: 0.3, NovelProblem: 0.2},
	"typescript/csv-lite":            {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.2, EdgeCaseDensity: 0.5, NovelProblem: 0.2},
	"typescript/forth":               {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.2, EdgeCaseDensity: 0.3, NovelProblem: 0.1},
	"typescript/glob":                {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.1, EdgeCaseDensity: 0.2, NovelProblem: 0.0},
	"typescript/promise-pool":        {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.4, NovelProblem: 0.2},
	"typescript/react":               {LangRarity: 0.0, EsotericFeature: 0.0, NovelAlgorithm: 0.1, EdgeCaseDensity: 0.2, NovelProblem: 0.0},
	"zig/arena-allocator":            {LangRarity: 0.2, EsotericFeature: 0.3, NovelAlgorithm: 0.1, EdgeCaseDensity: 0.3, NovelProblem: 0.1},
	"zig/comptime-json":              {LangRarity: 0.2, EsotericFeature: 0.5, NovelAlgorithm: 0.2, EdgeCaseDensity: 0.3, NovelProblem: 0.3},
	"zig/small-vector":               {LangRarity: 0.2, EsotericFeature: 0.2, NovelAlgorithm: 0.0, EdgeCaseDensity: 0.2, NovelProblem: 0.0},
}

// Weight holds the computed difficulty weight for a task.
type Weight struct {
	Base            float64 `json:"base"`
	LangRarity      float64 `json:"lang_rarity"`
	EsotericFeature float64 `json:"esoteric_feature"`
	NovelAlgorithm  float64 `json:"novel_algorithm"`
	EdgeCaseDensity float64 `json:"edge_case_density"`
	NovelProblem    float64 `json:"novel_problem"`
}

// ComputeWeight calculates a task's difficulty weight based on empirical factors.
// The weight formula is:
//
//	base = 1.0
//	     + lang_rarity * 0.5
//	     + esoteric_feature * 0.8
//	     + novel_algorithm * 0.6
//	     + edge_case_density * 0.4
//	     + novel_problem * 0.2
//	weight = min(base, 1.5)  // capped at 1.5
func ComputeWeight(t *Task) Weight {
	taskID := t.ID()
	diff, ok := taskDifficulties[taskID]
	if !ok {
		// Unknown task gets baseline weight
		return Weight{Base: 1.0}
	}

	base := 1.0
	base += diff.LangRarity * 0.5
	base += diff.EsotericFeature * 0.8
	base += diff.NovelAlgorithm * 0.6
	base += diff.EdgeCaseDensity * 0.4
	base += diff.NovelProblem * 0.2

	// Cap at MaxWeight
	if base > MaxWeight {
		base = MaxWeight
	}

	return Weight{
		Base:            base,
		LangRarity:      diff.LangRarity,
		EsotericFeature: diff.EsotericFeature,
		NovelAlgorithm:  diff.NovelAlgorithm,
		EdgeCaseDensity: diff.EdgeCaseDensity,
		NovelProblem:    diff.NovelProblem,
	}
}

// ResultStatus represents the outcome status of a task evaluation.
type ResultStatus string

const (
	StatusPass               ResultStatus = "pass"
	StatusPartialPass        ResultStatus = "partial_pass"
	StatusFail               ResultStatus = "fail"
	StatusIntegrityViolation ResultStatus = "integrity_violation"
	StatusError              ResultStatus = "error"
	StatusInfraFailure       ResultStatus = "infra_failure"
)

// DetermineStatus computes the result status from pass/timeout/error state.
func DetermineStatus(passed, agentTimedOut bool, errorMsg string) ResultStatus {
	if errorMsg != "" {
		if contains(errorMsg, "modified task files") {
			return StatusIntegrityViolation
		}
		if contains(errorMsg, "infra failure") {
			return StatusInfraFailure
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

// contains checks if s contains substr.
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
//
// Scoring rules:
//   - Clean pass: 100% of weight
//   - Partial pass (timeout but correct): 75% of weight
//   - Fail: 0
//   - Integrity violation: -0.25 penalty
func ScoreResult(passed, agentTimedOut bool, errorMsg string, weight Weight) float64 {
	status := DetermineStatus(passed, agentTimedOut, errorMsg)

	switch status {
	case StatusPass:
		return weight.Base
	case StatusPartialPass:
		return weight.Base * PartialPassMultiplier
	case StatusIntegrityViolation:
		return -ViolationPenalty
	default:
		return 0.0
	}
}
