package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

// runResult tracks the outcome of a single run in a multi-run session.
type runResult struct {
	spec    RunSpec
	repeat  int
	summary *EvalSummary
	err     error
}

// MultiRunConfig is persisted as multi-run-config.json in the umbrella directory.
type MultiRunConfig struct {
	Specs     []RunSpec    `json:"specs"`
	Shared    SharedConfig `json:"shared"`
	Repeat    int          `json:"repeat"`
	CreatedAt string       `json:"created_at"`
}

// MultiRunState tracks per-run status for resume support.
type MultiRunState struct {
	ID     string         `json:"id"`
	Repeat int            `json:"repeat"`
	Specs  []RunSpec      `json:"specs"`
	Runs   []MultiRunItem `json:"runs"`
}

// MultiRunItem represents one run entry in the state file.
type MultiRunItem struct {
	SpecIndex int    `json:"spec_index"`
	Repeat    int    `json:"repeat"`
	Dir       string `json:"dir"`
	Status    string `json:"status"` // "completed", "interrupted", "pending"
}

// RepeatStats holds statistical aggregation across repeated runs of the same config.
type RepeatStats struct {
	Config              RunSpec            `json:"config"`
	Runs                int                `json:"runs"`
	PassRates           []float64          `json:"pass_rates"`
	MeanPassRate        float64            `json:"mean_pass_rate"`
	StdDevPassRate      float64            `json:"stddev_pass_rate"`
	MinPassRate         float64            `json:"min_pass_rate"`
	MaxPassRate         float64            `json:"max_pass_rate"`
	MeanWeightedScore   float64            `json:"mean_weighted_score"`
	StdDevWeightedScore float64            `json:"stddev_weighted_score"`
	MinWeightedScore    float64            `json:"min_weighted_score"`
	MaxWeightedScore    float64            `json:"max_weighted_score"`
	MeanDuration        float64            `json:"mean_duration_seconds"`
	TaskConsistency     map[string]float64 `json:"task_consistency"`
}

// Comparison holds a side-by-side comparison of multiple eval runs.
type Comparison struct {
	Runs       []ComparisonRun              `json:"runs"`
	TaskMatrix map[string]map[string]string `json:"task_matrix"`
	BestRun    string                       `json:"best_run"`
	BestScore  float64                      `json:"best_weighted_score"`
}

// ComparisonRun is one entry in a comparison table.
type ComparisonRun struct {
	ID                  string  `json:"id"`
	Agent               string  `json:"agent"`
	Model               string  `json:"model"`
	Reasoning           string  `json:"reasoning,omitempty"`
	PassRate            float64 `json:"pass_rate"`
	WeightedPassRate    float64 `json:"weighted_pass_rate"`
	WeightedScore       float64 `json:"weighted_score"`
	Passed              int     `json:"passed"`
	Failed              int     `json:"failed"`
	Total               int     `json:"total"`
	Duration            float64 `json:"duration_seconds"`
	IntegrityViolations int     `json:"integrity_violations"`
}

// broadcastOrSplit splits a comma-separated string into N values.
// If the input has 1 element, it is broadcast to all N slots.
// If it has N elements, they are used as-is.
// If it has 0 elements (empty string), all slots are empty.
// Otherwise, returns an error.
func broadcastOrSplit(value string, n int, flagName string) ([]string, error) {
	if value == "" {
		return make([]string, n), nil
	}
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 1 {
		result := make([]string, n)
		for i := range result {
			result[i] = parts[0]
		}
		return result, nil
	}
	if len(parts) != n {
		return nil, fmt.Errorf("--%s has %d values but --agent has %d (must be 1 or %d)", flagName, len(parts), n, n)
	}
	return parts, nil
}

// sanitizeModel replaces characters that are problematic in directory names.
func sanitizeModel(model string) string {
	return strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(model)
}

// multiRunSubdir returns the subdirectory path for a specific run within the umbrella.
func multiRunSubdir(umbrella string, spec RunSpec, specIdx, rep, totalRepeats int) string {
	name := spec.Agent
	if spec.Model != "" {
		name += "-" + sanitizeModel(spec.Model)
	}
	if totalRepeats > 1 {
		return filepath.Join(umbrella, name, fmt.Sprintf("run-%d", rep))
	}
	return filepath.Join(umbrella, name)
}

// writeMultiRunConfig persists the multi-run configuration to the umbrella directory.
func writeMultiRunConfig(umbrellaDir string, specs []RunSpec, shared SharedConfig, repeat int) {
	cfg := MultiRunConfig{
		Specs:     specs,
		Shared:    shared,
		Repeat:    repeat,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(filepath.Join(umbrellaDir, "multi-run-config.json"), data, 0o644)
}

// updateMultiRunState writes the current state of all runs to multi-run-state.json.
func updateMultiRunState(umbrellaDir string, results []runResult, specs []RunSpec, repeat int, interrupted bool) {
	state := MultiRunState{
		ID:     filepath.Base(umbrellaDir),
		Repeat: repeat,
		Specs:  specs,
	}

	// Build a set of completed runs.
	completed := make(map[string]bool)
	for _, rr := range results {
		key := fmt.Sprintf("%s-%d", rr.spec.Agent, rr.repeat)
		if rr.summary != nil || rr.err != nil {
			completed[key] = true
		}
	}

	for specIdx, spec := range specs {
		for rep := 1; rep <= repeat; rep++ {
			dir := multiRunSubdir("", spec, specIdx, rep, repeat)
			key := fmt.Sprintf("%s-%d", spec.Agent, rep)
			status := "pending"
			if completed[key] {
				status = "completed"
			}
			state.Runs = append(state.Runs, MultiRunItem{
				SpecIndex: specIdx,
				Repeat:    rep,
				Dir:       dir,
				Status:    status,
			})
		}
	}

	// If interrupted, mark the last non-completed run as interrupted.
	if interrupted {
		for i := len(state.Runs) - 1; i >= 0; i-- {
			if state.Runs[i].Status == "pending" {
				// Find the first pending — the one before it might be interrupted.
				break
			}
		}
		// Mark the last completed as interrupted if it had an error or was partial.
		for i := range state.Runs {
			if state.Runs[i].Status == "pending" && i > 0 {
				// The run just before the first pending might be the interrupted one.
				if i > 0 && state.Runs[i-1].Status == "completed" {
					// Check if it was actually interrupted.
					for _, rr := range results {
						if rr.spec.Agent == specs[state.Runs[i-1].SpecIndex].Agent &&
							rr.repeat == state.Runs[i-1].Repeat &&
							rr.err != nil {
							state.Runs[i-1].Status = "interrupted"
						}
					}
				}
				break
			}
		}
	}

	data, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(filepath.Join(umbrellaDir, "multi-run-state.json"), data, 0o644)
}

// isMultiRunDir checks if a directory is a multi-run umbrella directory.
func isMultiRunDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "multi-run-config.json"))
	return err == nil
}

// resumeMultiRun resumes a multi-run session from its umbrella directory.
func resumeMultiRun(resumeDir string) error {
	// Load multi-run config.
	cfgData, err := os.ReadFile(filepath.Join(resumeDir, "multi-run-config.json"))
	if err != nil {
		return fmt.Errorf("reading multi-run config: %w", err)
	}
	var mrCfg MultiRunConfig
	if err := json.Unmarshal(cfgData, &mrCfg); err != nil {
		return fmt.Errorf("parsing multi-run config: %w", err)
	}

	// Load state.
	stateData, err := os.ReadFile(filepath.Join(resumeDir, "multi-run-state.json"))
	if err != nil {
		return fmt.Errorf("reading multi-run state: %w", err)
	}
	var state MultiRunState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return fmt.Errorf("parsing multi-run state: %w", err)
	}

	// Restore shared config globals for runner creation.
	shared := mrCfg.Shared
	evalTier = shared.Tier
	evalDifficulty = shared.Difficulty
	evalLang = shared.Lang
	evalTasks = shared.Tasks
	evalTimeout = shared.Timeout
	evalParallel = shared.Parallel
	evalKeepWorkspaces = shared.KeepWorkspaces
	evalUseMCPTools = shared.UseMCPTools
	evalDisableMCP = shared.DisableMCP
	evalNoSandbox = shared.NoSandbox
	evalLegacy = shared.Legacy

	// Create runner.
	r, err := newRunnerFromConfig()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	if shared.Legacy {
		r.LegacyHiddenTests = true
	}

	// Load and filter tasks.
	allTasks, err := r.ListTasks()
	if err != nil {
		return fmt.Errorf("listing tasks: %w", err)
	}
	allTasks = filterTasksForShared(allTasks, shared)
	if len(allTasks) == 0 {
		return fmt.Errorf("no tasks match the specified filters")
	}

	evalSandboxActive = initSandbox()

	if restoreFn, err := protectTasksDir(); err != nil {
		logger.Warn("failed to protect tasks directory", "error", err)
	} else if restoreFn != nil {
		defer restoreFn()
	}

	interruptCtx, interruptCancel := setupInterruptHandler()
	defer interruptCancel()

	timestamp := time.Now().Format("2006-01-02T150405")

	var allSummaries []runResult
	for _, item := range state.Runs {
		if item.Status == "completed" {
			// Load existing summary.
			runDir := filepath.Join(resumeDir, item.Dir)
			prevSummary, err := loadPreviousSummary(runDir)
			if err != nil {
				logger.Warn("failed to load completed run summary", "dir", runDir, "error", err)
			}
			allSummaries = append(allSummaries, runResult{
				spec:    mrCfg.Specs[item.SpecIndex],
				repeat:  item.Repeat,
				summary: prevSummary,
			})
			continue
		}

		if checkInterrupted(interruptCtx) {
			updateMultiRunState(resumeDir, allSummaries, mrCfg.Specs, mrCfg.Repeat, true)
			printMultiRunResumeCommand(resumeDir)
			return nil
		}

		spec := mrCfg.Specs[item.SpecIndex]
		runDir := filepath.Join(resumeDir, item.Dir)

		// For interrupted runs, use single-run resume logic.
		var isResuming bool
		var previousResults []EvalResult
		var completedTasks map[string]bool
		var prevAttestation *EvalAttestation
		var runCfg *RunConfig

		if item.Status == "interrupted" {
			runCfg, err = loadRunConfig(runDir)
			if err == nil {
				isResuming = true
				completedTasks, _ = findCompletedTasks(runDir)
				prevSummary, _ := loadPreviousSummary(runDir)
				if prevSummary != nil {
					previousResults = prevSummary.Results
				}
				prevAttestation, _ = loadPreviousAttestation(runDir)
			}
		}

		summary, _, runErr := evalRunSingle(
			interruptCtx, spec, shared, allTasks, allTasks,
			runDir, timestamp, r, isResuming,
			previousResults, completedTasks, prevAttestation, runCfg,
		)
		rr := runResult{spec: spec, repeat: item.Repeat, summary: summary, err: runErr}
		allSummaries = append(allSummaries, rr)
		updateMultiRunState(resumeDir, allSummaries, mrCfg.Specs, mrCfg.Repeat, false)
	}

	// Regenerate comparison and repeat stats.
	if len(mrCfg.Specs) > 1 {
		var summaries []EvalSummary
		for _, rr := range allSummaries {
			if rr.summary != nil {
				summaries = append(summaries, *rr.summary)
			}
		}
		if len(summaries) > 1 {
			comparison := generateComparison(summaries)
			writeComparisonJSON(resumeDir, comparison)
			writeComparisonMarkdown(resumeDir, comparison)
		}
	}
	if mrCfg.Repeat > 1 {
		writeRepeatStats(resumeDir, mrCfg.Specs, allSummaries, mrCfg.Repeat)
	}

	fmt.Printf("\n Multi-run results saved to: %s\n\n", resumeDir)
	return nil
}

// printMultiRunResumeCommand prints the command to resume a multi-run session.
func printMultiRunResumeCommand(umbrellaDir string) {
	fmt.Println()
	fmt.Println("\033[33m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
	fmt.Println("\033[33m ⚠ Multi-run interrupted. To resume:\033[0m")
	fmt.Printf("   ./sanity eval --resume %s\n", umbrellaDir)
	fmt.Println("\033[33m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
	fmt.Println()
}

// generateComparison creates a side-by-side comparison of multiple eval summaries.
func generateComparison(summaries []EvalSummary) Comparison {
	c := Comparison{
		TaskMatrix: make(map[string]map[string]string),
	}

	for _, s := range summaries {
		id := s.Agent
		if s.Model != "" && s.Model != "unknown" {
			id += "/" + s.Model
		}

		run := ComparisonRun{
			ID:                  id,
			Agent:               s.Agent,
			Model:               s.Model,
			Reasoning:           s.Reasoning,
			PassRate:            s.PassRate,
			WeightedPassRate:    s.WeightedPassRate,
			WeightedScore:       s.WeightedScore,
			Passed:              s.Passed,
			Failed:              s.Failed,
			Total:               s.Total,
			Duration:            s.Duration,
			IntegrityViolations: s.IntegrityViolations,
		}
		c.Runs = append(c.Runs, run)

		if run.WeightedScore > c.BestScore {
			c.BestScore = run.WeightedScore
			c.BestRun = id
		}

		for _, r := range s.Results {
			if c.TaskMatrix[r.Task] == nil {
				c.TaskMatrix[r.Task] = make(map[string]string)
			}
			if r.Passed {
				c.TaskMatrix[r.Task][id] = "✅"
			} else {
				c.TaskMatrix[r.Task][id] = "❌"
			}
		}
	}

	return c
}

// writeComparisonJSON writes comparison.json to the umbrella directory.
func writeComparisonJSON(dir string, c Comparison) {
	data, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "comparison.json"), data, 0o644)
}

// writeComparisonMarkdown writes comparison-report.md to the umbrella directory.
func writeComparisonMarkdown(dir string, c Comparison) {
	f, err := os.Create(filepath.Join(dir, "comparison-report.md"))
	if err != nil {
		return
	}
	defer f.Close()
	writeComparisonReport(f, c)
}

// writeComparisonReport writes a human-readable comparison report.
func writeComparisonReport(w io.Writer, c Comparison) {
	fmt.Fprintf(w, "### Agent Comparison\n\n")

	// Summary table.
	fmt.Fprintf(w, "| Agent | Model | Pass Rate | Weighted Score | Passed | Failed | Duration |\n")
	fmt.Fprintf(w, "|-------|-------|-----------|----------------|--------|--------|----------|\n")
	for _, r := range c.Runs {
		dur := formatDuration(r.Duration)
		best := ""
		if r.ID == c.BestRun {
			best = " 🏆"
		}
		fmt.Fprintf(w, "| %s%s | %s | %.1f%% | %.2f | %d | %d | %s |\n",
			r.Agent, best, r.Model, r.PassRate, r.WeightedScore, r.Passed, r.Failed, dur)
	}
	fmt.Fprintln(w)

	// Task matrix.
	if len(c.TaskMatrix) > 0 && len(c.Runs) > 0 {
		fmt.Fprintf(w, "### Task Matrix\n\n")
		fmt.Fprintf(w, "| Task |")
		for _, r := range c.Runs {
			fmt.Fprintf(w, " %s |", r.ID)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "|------|")
		for range c.Runs {
			fmt.Fprintf(w, "------|")
		}
		fmt.Fprintln(w)

		// Sort tasks for deterministic output.
		var tasks []string
		for t := range c.TaskMatrix {
			tasks = append(tasks, t)
		}
		sort.Strings(tasks)

		for _, t := range tasks {
			fmt.Fprintf(w, "| %s |", t)
			for _, r := range c.Runs {
				status := c.TaskMatrix[t][r.ID]
				if status == "" {
					status = "—"
				}
				fmt.Fprintf(w, " %s |", status)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
}

// writeRepeatStats computes and writes repeat statistics for each config.
func writeRepeatStats(umbrellaDir string, specs []RunSpec, results []runResult, repeat int) {
	var allStats []RepeatStats

	for _, spec := range specs {
		var summaries []*EvalSummary
		for _, rr := range results {
			if rr.spec.Agent == spec.Agent && rr.spec.Model == spec.Model &&
				rr.spec.Reasoning == spec.Reasoning && rr.summary != nil {
				summaries = append(summaries, rr.summary)
			}
		}
		if len(summaries) == 0 {
			continue
		}
		allStats = append(allStats, computeRepeatStats(spec, summaries))
	}

	// Write JSON.
	data, _ := json.MarshalIndent(allStats, "", "  ")
	_ = os.WriteFile(filepath.Join(umbrellaDir, "repeat-stats.json"), data, 0o644)

	// Write Markdown.
	f, err := os.Create(filepath.Join(umbrellaDir, "repeat-report.md"))
	if err != nil {
		return
	}
	defer f.Close()

	for _, stats := range allStats {
		label := stats.Config.Agent
		if stats.Config.Model != "" {
			label += " / " + stats.Config.Model
		}
		fmt.Fprintf(f, "### Repeat Analysis — %s (%d runs)\n\n", label, stats.Runs)
		fmt.Fprintf(f, "| Metric | Mean | Std Dev | Min | Max |\n")
		fmt.Fprintf(f, "|--------|------|---------|-----|-----|\n")
		fmt.Fprintf(f, "| Pass Rate | %.1f%% | ±%.1f%% | %.1f%% | %.1f%% |\n",
			stats.MeanPassRate, stats.StdDevPassRate, stats.MinPassRate, stats.MaxPassRate)
		fmt.Fprintf(f, "| Weighted Score | %.2f | ±%.2f | %.2f | %.2f |\n",
			stats.MeanWeightedScore, stats.StdDevWeightedScore, stats.MinWeightedScore, stats.MaxWeightedScore)
		fmt.Fprintf(f, "| Duration | %s | — | — | — |\n", formatDuration(stats.MeanDuration))
		fmt.Fprintln(f)

		// Task consistency sorted by flakiness.
		if len(stats.TaskConsistency) > 0 {
			fmt.Fprintf(f, "### Task Consistency (sorted by flakiness)\n\n")
			fmt.Fprintf(f, "| Task | Pass Rate | Status |\n")
			fmt.Fprintf(f, "|------|-----------|--------|\n")

			type taskRate struct {
				task string
				rate float64
			}
			var sorted []taskRate
			for t, rate := range stats.TaskConsistency {
				sorted = append(sorted, taskRate{t, rate})
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].rate < sorted[j].rate
			})

			for _, tr := range sorted {
				status := "✅ Stable"
				if tr.rate < 50 {
					status = "❌ Unreliable"
				} else if tr.rate < 100 {
					status = "⚠️ Flaky"
				}
				fmt.Fprintf(f, "| %s | %.0f%% | %s |\n", tr.task, tr.rate, status)
			}
			fmt.Fprintln(f)
		}
	}
}

// computeRepeatStats computes statistical aggregation across repeated runs.
func computeRepeatStats(spec RunSpec, summaries []*EvalSummary) RepeatStats {
	var passRates, weightedScores, durations []float64
	taskPassCounts := make(map[string]int)
	taskTotal := make(map[string]int)

	for _, s := range summaries {
		passRates = append(passRates, s.PassRate)
		weightedScores = append(weightedScores, s.WeightedScore)
		durations = append(durations, s.Duration)
		for _, r := range s.Results {
			taskTotal[r.Task]++
			if r.Passed {
				taskPassCounts[r.Task]++
			}
		}
	}

	taskConsistency := make(map[string]float64)
	for tk, total := range taskTotal {
		taskConsistency[tk] = float64(taskPassCounts[tk]) / float64(total) * 100.0
	}

	return RepeatStats{
		Config:              spec,
		Runs:                len(summaries),
		PassRates:           passRates,
		MeanPassRate:        mean(passRates),
		StdDevPassRate:      stddev(passRates),
		MinPassRate:         minVal(passRates),
		MaxPassRate:         maxVal(passRates),
		MeanWeightedScore:   mean(weightedScores),
		StdDevWeightedScore: stddev(weightedScores),
		MinWeightedScore:    minVal(weightedScores),
		MaxWeightedScore:    maxVal(weightedScores),
		MeanDuration:        mean(durations),
		TaskConsistency:     taskConsistency,
	}
}

// filterTasksForShared applies shared config filters to a task list.
func filterTasksForShared(allTasks []*task.Task, shared SharedConfig) []*task.Task {
	result := allTasks

	if shared.Tasks != "" {
		tokens := strings.Split(shared.Tasks, ",")
		var selected []*task.Task
		seen := make(map[string]bool)
		for _, tok := range tokens {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			t, err := task.ResolveRef(result, tok)
			if err != nil {
				continue
			}
			if !seen[t.ID()] {
				seen[t.ID()] = true
				selected = append(selected, t)
			}
		}
		result = selected
	}

	if shared.Lang != "" {
		lang, err := task.ParseLanguage(shared.Lang)
		if err == nil {
			var filtered []*task.Task
			for _, t := range result {
				if t.Language == lang {
					filtered = append(filtered, t)
				}
			}
			result = filtered
		}
	}

	if shared.Difficulty != "" {
		want := make(map[string]bool)
		for _, tok := range strings.Split(shared.Difficulty, ",") {
			tok = strings.TrimSpace(tok)
			if tok != "" {
				want[tok] = true
			}
		}
		var filtered []*task.Task
		for _, t := range result {
			if want[t.Difficulty] {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	if shared.Tier != "" && shared.Tier != "all" {
		var filtered []*task.Task
		for _, t := range result {
			if t.Tier == shared.Tier {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	return result
}

// newRunnerFromConfig creates a new runner using the global config.
func newRunnerFromConfig() (*runner.Runner, error) {
	return runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
}

// Math helpers.

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stddev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := mean(vals)
	sum := 0.0
	for _, v := range vals {
		sum += (v - m) * (v - m)
	}
	return math.Sqrt(sum / float64(len(vals)))
}

func minVal(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxVal(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
