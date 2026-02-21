### Multi-Run & Repeat — Finalized Implementation Plan

> **Date:** 2026-02-21
> **Status:** Final
> **Prerequisite:** [Multi-Run Design Proposal](multi-run-proposal.md)
> **Scope:** Comma-separated multi-agent CLI, `batch` subcommand, `--repeat N` flag, cross-run comparison, `compare` command

---

### Table of Contents

1. [Overview](#overview)
2. [New Flags & CLI Surface](#new-flags--cli-surface)
3. [Code Context Reference](#code-context-reference)
4. [Phase 0 — `evalRunSingle()` Extraction](#phase-0--evalrunsingle-extraction)
5. [Phase 1 — `--repeat N` Flag](#phase-1--repeat-n-flag)
6. [Phase 2 — Comma-Separated Multi-Agent](#phase-2--comma-separated-multi-agent)
7. [Phase 3 — `batch` Subcommand](#phase-3--batch-subcommand)
8. [Phase 4 — `compare` Command](#phase-4--compare-command)
9. [Phase 5 — Parallel Runs (Future)](#phase-5--parallel-runs-future)
10. [Output Directory Structures](#output-directory-structures)
11. [Statistical Aggregation for `--repeat`](#statistical-aggregation-for---repeat)
12. [Resume Support](#resume-support)
13. [Testing Strategy](#testing-strategy)

---

### Overview

This plan implements three complementary features that share a common foundation — the extraction of the current monolithic `evalCmd.RunE` into a reusable `evalRunSingle()` function:

| Feature | Use Case | Entry Point |
|---------|----------|-------------|
| `--repeat N` | Run the same config N times for statistical relevance | `sanity eval --agent gemini --repeat 5` |
| Comma-separated multi-agent | Quick A/B comparison from CLI | `sanity eval --agent codex,opencode --model gpt-5.2,kimi-k2.5` |
| `batch` subcommand | Complex multi-run with per-run overrides | `sanity batch --config runs.toml` |

All three produce the same multi-run output structure and share the orchestration layer.

The `--repeat` flag composes with multi-agent: `--agent codex,opencode --repeat 3` produces 6 total runs (2 configs × 3 repeats).

---

### New Flags & CLI Surface

#### `eval` command — new flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repeat` | `int` | `1` | Repeat each configuration N times |
| `--parallel-runs` | `int` | `1` | How many runs to execute simultaneously (future, Phase 5) |

Existing flags `--agent`, `--model`, `--reasoning` gain comma-separated support (Phase 2).

#### `batch` command (new, Phase 3)

```
sanity batch --config runs.toml [--parallel-runs N] [--repeat N]
```

#### `compare` command (new, Phase 4)

```
sanity compare <dir1> <dir2> [dir3...]
sanity compare eval-results/gemini-* eval-results/codex-*
```

---

### Code Context Reference

This section provides the key code locations and signatures an implementing agent needs.

#### File: `internal/cli/eval.go` (2761 lines)

**Package-level flag variables (lines 31–53):**

```go
var (
    evalAgent          string
    evalModel          string
    evalReasoning      string
    evalTasks          string
    evalLang           string
    evalTier           string
    evalDifficulty     string
    evalTimeout        int
    evalOutputDir      string
    evalKeepWorkspaces bool
    evalParallel       int
    evalDryRun         bool
    evalUseMCPTools    bool
    evalDisableMCP     bool
    evalNoSandbox      bool
    evalLegacy         bool
    evalSandboxActive  bool
    evalResume         string
)
```

**Flag registration (lines 2742–2760):**

```go
func init() {
    evalCmd.Flags().StringVar(&evalAgent, "agent", "", "...")
    evalCmd.Flags().StringVar(&evalModel, "model", "", "...")
    evalCmd.Flags().StringVar(&evalReasoning, "reasoning", "", "...")
    // ... all other flags
    evalCmd.Flags().StringVar(&evalResume, "resume", "", "...")
}
```

New flags should be added here following the same pattern.

**Key types (lines 131–222):**

- `EvalResult` (line 131) — per-task result with `Passed`, `Status`, `Duration`, `Weight`, `WeightedScore`, etc.
- `EvalAggregate` (line 161) — summary for a group (language, tier, difficulty).
- `EvalSummary` (line 172) — overall eval summary with `Results []EvalResult`, `PassRate`, `WeightedScore`, aggregates by language/tier/difficulty.
- `RunConfig` (line 205) — serialized eval configuration for resume. New fields (`Repeat`, multi-agent specs) must be added here.

**`RunE` body (lines 261–~1073):** This is the monolithic function that must be extracted. It:
1. Handles resume loading (lines 272–309)
2. Validates agent (lines 316–332)
3. Creates runner (line 334)
4. Filters tasks (lines 358–433)
5. Handles dry-run (lines 436–~500)
6. Sets up output directory and writes `run-config.json` (lines ~500–600)
7. Sets up interrupt handler (lines ~600–620)
8. Runs tasks sequentially or in parallel (lines ~620–900)
9. Computes aggregates and writes output files (lines ~900–1073)

**Key functions:**

| Function | Line | Signature | Purpose |
|----------|------|-----------|---------|
| `runTaskWithAgent` | 1076 | `(ctx, r, t, agent, model, outputDir, timeout) → EvalResult` | Runs one task against one agent |
| `executeAgentWithRetries` | 1256 | `(ctx, t, agentCfg, ...) → (agentDuration, timedOut, quotaRetries, quotaExhausted, infraFailure, error)` | Agent execution with quota retry |
| `runAgentAttempt` | 1348 | `(ctx, agentCfg, ...) → error` | Single agent command execution |
| `buildAgentCommand` | 1600 | `(ctx, agentCfg, prompt, model, reasoning, disableMCP, agentName) → *exec.Cmd` | Constructs agent CLI command |
| `applyRunConfig` | 2538 | `(runCfg *RunConfig)` | Applies loaded config to global vars |
| `setupInterruptHandler` | 2644 | `() → (context.Context, context.CancelFunc)` | SIGINT/SIGTERM handling |
| `initSandbox` | 2672 | `() → bool` | Checks bwrap availability |

**Output file writers (scattered ~2100–2500):**

- `writeReportSummary`, `writeReportQuality`, `writeReportByLanguage`, `writeReportByTier`, `writeReportTaskResults`, `writeReportErrors`, `writeReportVerification` — all take `*strings.Builder` and `EvalSummary`.
- Summary JSON, attestation JSON, submission JSON, and report.md are all written near the end of `RunE`.

**Other files:**

| File | Key Exports |
|------|-------------|
| `internal/config/config.go` | `Config`, `AgentConfig`, `GetAgent()`, `ListAgents()` |
| `internal/runner/runner.go` | `Runner`, `NewRunner()`, `Close()`, `ListTasks()` |
| `internal/task/task.go` | `Task`, `Language`, `ComputeWeight()`, `ResolveRef()` |
| `internal/task/weight.go` | `ComputeWeight()`, weight formula |
| `internal/cli/root.go` | `cfg`, `logger`, `tasksDir` globals, `rootCmd` |

---

### Phase 0 — `evalRunSingle()` Extraction

**Goal:** Extract the `RunE` body into a callable function without changing any behavior.

**This is the critical prerequisite for all subsequent phases.**

#### Step 0.1 — Define `RunSpec` and `SharedConfig`

Add to `eval.go` (near the existing types, after line ~222):

```go
// RunSpec defines a single eval run's configuration.
type RunSpec struct {
    Agent     string
    Model     string
    Reasoning string
}

// SharedConfig holds settings common to all runs.
type SharedConfig struct {
    Tier           string
    Difficulty     string
    Lang           string
    Tasks          string
    Timeout        int
    Parallel       int
    KeepWorkspaces bool
    UseMCPTools    bool
    DisableMCP     bool
    NoSandbox      bool
    Legacy         bool
    DryRun         bool
}
```

#### Step 0.2 — Extract `evalRunSingle()`

Create a function with this signature:

```go
func evalRunSingle(
    ctx context.Context,
    spec RunSpec,
    shared SharedConfig,
    outputDir string,
    timestamp string,
    r *runner.Runner,
    isResuming bool,
    previousResults []EvalResult,
    completedTasks map[string]bool,
    prevAttestation *EvalAttestation,
) (*EvalSummary, *EvalAttestation, error)
```

Move the body of `RunE` (from agent validation through output file writing) into this function. Replace all reads of `evalAgent`, `evalModel`, `evalReasoning` with `spec.Agent`, `spec.Model`, `spec.Reasoning`. Replace reads of shared flags (`evalTimeout`, `evalKeepWorkspaces`, etc.) with `shared.*` fields.

The `RunE` function becomes a thin wrapper:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    shared := SharedConfig{
        Tier: evalTier, Difficulty: evalDifficulty, Lang: evalLang,
        Tasks: evalTasks, Timeout: evalTimeout, Parallel: evalParallel,
        KeepWorkspaces: evalKeepWorkspaces, UseMCPTools: evalUseMCPTools,
        DisableMCP: evalDisableMCP, NoSandbox: evalNoSandbox,
        Legacy: evalLegacy, DryRun: evalDryRun,
    }
    spec := RunSpec{Agent: evalAgent, Model: evalModel, Reasoning: evalReasoning}

    // ... resume handling, runner creation, task filtering (keep in RunE) ...

    summary, _, err := evalRunSingle(ctx, spec, shared, outputDir, timestamp, r, isResuming, previousResults, completedTasks, prevAttestation)
    // ... print final results ...
    return err
}
```

**Key considerations:**
- `runTaskWithAgent` already takes `agent` and `model` as parameters — no change needed.
- `evalReasoning` is read as a global in `executeAgentWithRetries` → `runAgentAttempt` → `buildAgentCommand`. Pass it through the call chain or keep reading the global (since `evalRunSingle` sets it before calling). The cleanest approach: pass `reasoning` as a parameter to `runTaskWithAgent`, which passes it down. But for Phase 0, setting the global before calling is acceptable.
- `evalSandboxActive` is set once and shared — keep as global.
- The `runner.Runner` and task list can be created once in `RunE` and shared across all runs (Phase 2).

#### Step 0.3 — Verify

Run `make test` and `make lint`. The behavior must be identical to before — single-agent eval works exactly as it did.

---

### Phase 1 — `--repeat N` Flag

**Goal:** Allow repeating the same eval configuration N times, with per-repeat subdirectories and aggregated statistics.

#### Step 1.1 — Add flag

```go
var evalRepeat int

// In init():
evalCmd.Flags().IntVar(&evalRepeat, "repeat", 1, "repeat each configuration N times for statistical analysis")
```

Add `Repeat int` to `RunConfig` for resume support.

#### Step 1.2 — Repeat orchestration in `RunE`

When `evalRepeat > 1` (or when multi-agent is active, Phase 2), `RunE` creates a multi-run umbrella directory and loops:

```go
if evalRepeat > 1 || isMultiRun {
    // Create umbrella dir: eval-results/multi-<timestamp>/
    // or eval-results/<agent>-<timestamp>/ for single-agent repeat
    for i := 1; i <= evalRepeat; i++ {
        runDir := filepath.Join(umbrellaDir, fmt.Sprintf("run-%d", i))
        // For multi-agent: runDir = filepath.Join(umbrellaDir, fmt.Sprintf("%s-%s/run-%d", spec.Agent, spec.Model, i))
        summary, att, err := evalRunSingle(ctx, spec, shared, runDir, timestamp, r, ...)
        summaries = append(summaries, summary)
    }
    // Write aggregated stats + comparison
} else {
    // Single run, no repeat — current behavior, no umbrella dir
    evalRunSingle(ctx, spec, shared, outputDir, timestamp, r, ...)
}
```

#### Step 1.3 — Add `RepeatStats` type and aggregation

```go
// RepeatStats holds statistical aggregation across repeated runs of the same config.
type RepeatStats struct {
    Config       RunSpec  `json:"config"`
    Runs         int      `json:"runs"`
    PassRates    []float64 `json:"pass_rates"`
    MeanPassRate float64  `json:"mean_pass_rate"`
    StdDevPassRate float64 `json:"stddev_pass_rate"`
    MinPassRate  float64  `json:"min_pass_rate"`
    MaxPassRate  float64  `json:"max_pass_rate"`
    MeanWeightedScore float64 `json:"mean_weighted_score"`
    StdDevWeightedScore float64 `json:"stddev_weighted_score"`
    MinWeightedScore float64 `json:"min_weighted_score"`
    MaxWeightedScore float64 `json:"max_weighted_score"`
    MeanDuration float64 `json:"mean_duration_seconds"`
    // Per-task consistency: how often each task passed across repeats
    TaskConsistency map[string]float64 `json:"task_consistency"`
}
```

`TaskConsistency` is particularly valuable — it shows which tasks are flaky (pass sometimes, fail others) vs deterministic.

#### Step 1.4 — Write repeat output files

In the umbrella directory, write:
- `repeat-stats.json` — `RepeatStats` for each config
- `repeat-report.md` — human-readable table with mean/stddev/min/max
- `multi-run-config.json` — all specs + repeat count
- `multi-run-state.json` — per-run status for resume

Each repeat subdirectory (`run-1/`, `run-2/`, etc.) contains the standard eval output files (`summary.json`, `report.md`, etc.).

#### Step 1.5 — Aggregation functions

```go
func computeRepeatStats(spec RunSpec, summaries []*EvalSummary) RepeatStats {
    // Collect pass rates and weighted scores
    var passRates, weightedScores, durations []float64
    taskPassCounts := make(map[string]int)
    for _, s := range summaries {
        passRates = append(passRates, s.PassRate)
        weightedScores = append(weightedScores, s.WeightedScore)
        durations = append(durations, s.Duration)
        for _, r := range s.Results {
            if r.Passed {
                taskPassCounts[r.Task]++
            }
        }
    }
    n := float64(len(summaries))
    taskConsistency := make(map[string]float64)
    for task, count := range taskPassCounts {
        taskConsistency[task] = float64(count) / n * 100.0
    }
    return RepeatStats{
        Config: spec, Runs: len(summaries),
        PassRates: passRates,
        MeanPassRate: mean(passRates), StdDevPassRate: stddev(passRates),
        MinPassRate: min(passRates), MaxPassRate: max(passRates),
        MeanWeightedScore: mean(weightedScores), StdDevWeightedScore: stddev(weightedScores),
        MinWeightedScore: min(weightedScores), MaxWeightedScore: max(weightedScores),
        MeanDuration: mean(durations),
        TaskConsistency: taskConsistency,
    }
}

func mean(vals []float64) float64 { /* sum / len */ }
func stddev(vals []float64) float64 { /* sqrt(variance) */ }
// Use math.Sqrt, math.Pow from stdlib. No external deps needed.
```

---

### Phase 2 — Comma-Separated Multi-Agent

**Goal:** `--agent codex,opencode --model gpt-5.2,kimi-k2.5 --reasoning low,medium`

#### Step 2.1 — Parse comma-separated flags

Add a helper function:

```go
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
```

#### Step 2.2 — Build `[]RunSpec` in `RunE`

At the top of `RunE`, after resume handling:

```go
agents := strings.Split(evalAgent, ",")
for i := range agents {
    agents[i] = strings.TrimSpace(agents[i])
}
models, err := broadcastOrSplit(evalModel, len(agents), "model")
if err != nil {
    return err
}
reasonings, err := broadcastOrSplit(evalReasoning, len(agents), "reasoning")
if err != nil {
    return err
}

var specs []RunSpec
for i := range agents {
    specs = append(specs, RunSpec{
        Agent: agents[i], Model: models[i], Reasoning: reasonings[i],
    })
}
isMultiRun := len(specs) > 1 || evalRepeat > 1
```

#### Step 2.3 — Validate all agents before starting

```go
if !shared.DryRun {
    for _, spec := range specs {
        agentCfg := cfg.GetAgent(spec.Agent)
        if agentCfg == nil {
            return fmt.Errorf("unknown agent: %s", spec.Agent)
        }
        if _, err := exec.LookPath(agentCfg.Command); err != nil {
            return fmt.Errorf("agent %q binary %q not found", spec.Agent, agentCfg.Command)
        }
    }
}
```

#### Step 2.4 — Multi-run orchestration loop

```go
if isMultiRun {
    umbrellaDir := filepath.Join("eval-results", fmt.Sprintf("multi-%s", timestamp))
    os.MkdirAll(umbrellaDir, 0o755)

    // Write multi-run-config.json
    writeMultiRunConfig(umbrellaDir, specs, shared, evalRepeat)

    var allSummaries []runResult // {spec, repeatIndex, *EvalSummary}
    for specIdx, spec := range specs {
        for rep := 1; rep <= evalRepeat; rep++ {
            runDir := multiRunSubdir(umbrellaDir, spec, specIdx, rep, evalRepeat)
            summary, _, err := evalRunSingle(ctx, spec, shared, runDir, timestamp, r, false, nil, nil, nil)
            if err != nil { /* log, update state, continue or abort */ }
            allSummaries = append(allSummaries, runResult{spec, rep, summary})
            updateMultiRunState(umbrellaDir, specIdx, rep, "completed")
            if checkInterrupted(ctx) {
                printMultiRunResumeCommand(umbrellaDir)
                break
            }
        }
        if checkInterrupted(ctx) { break }
    }

    // Generate comparison + repeat stats
    writeComparison(umbrellaDir, allSummaries)
    if evalRepeat > 1 {
        writeRepeatStats(umbrellaDir, specs, allSummaries)
    }
} else {
    // Single run — unchanged behavior
}
```

#### Step 2.5 — Subdirectory naming

```go
func multiRunSubdir(umbrella string, spec RunSpec, specIdx, rep, totalRepeats int) string {
    name := spec.Agent
    if spec.Model != "" {
        name += "-" + sanitizeModel(spec.Model)
    }
    if totalRepeats > 1 {
        name += fmt.Sprintf("/run-%d", rep)
    }
    return filepath.Join(umbrella, name)
}

func sanitizeModel(model string) string {
    return strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(model)
}
```

If duplicate `agent-model` names exist (e.g., same agent+model with different reasoning), append the reasoning or a numeric suffix.

---

### Phase 3 — `batch` Subcommand

**Goal:** `sanity batch --config runs.toml`

#### Step 3.1 — Create `internal/cli/batch.go`

```go
package cli

import (
    "github.com/BurntSushi/toml"
    "github.com/spf13/cobra"
)

type BatchConfig struct {
    Defaults BatchDefaults `toml:"defaults"`
    Runs     []BatchRun    `toml:"runs"`
}

type BatchDefaults struct {
    Tier           string `toml:"tier"`
    Difficulty     string `toml:"difficulty"`
    Lang           string `toml:"lang"`
    Tasks          string `toml:"tasks"`
    Timeout        int    `toml:"timeout"`
    Parallel       int    `toml:"parallel"`
    KeepWorkspaces bool   `toml:"keep_workspaces"`
    UseMCPTools    bool   `toml:"use_mcp_tools"`
    NoSandbox      bool   `toml:"no_sandbox"`
    Legacy         bool   `toml:"legacy"`
    Repeat         int    `toml:"repeat"`
}

type BatchRun struct {
    Agent     string `toml:"agent"`
    Model     string `toml:"model"`
    Reasoning string `toml:"reasoning"`
    // Per-run overrides (zero value = use default)
    Timeout   int    `toml:"timeout"`
    Repeat    int    `toml:"repeat"`
}
```

#### Step 3.2 — Parse and convert to `[]RunSpec`

The batch command parses the TOML file, merges per-run overrides with defaults, and constructs the same `[]RunSpec` + `SharedConfig` that the comma-separated parser produces. Then it calls the same orchestration loop.

**Note:** Per-run `Timeout` override requires `SharedConfig` to become per-spec or `RunSpec` to carry optional overrides. Simplest approach: add `TimeoutOverride *int` to `RunSpec` (nil = use shared).

#### Step 3.3 — Register command

In `internal/cli/root.go` (or a new `batch.go`), add `rootCmd.AddCommand(batchCmd)`.

**Note:** The project uses `github.com/BurntSushi/toml` already (check `go.mod`). If not present, use `github.com/pelletier/go-toml/v2` or the same TOML library used for `sanity.toml` config loading.

---

### Phase 4 — `compare` Command

**Goal:** `sanity compare <dir1> <dir2> ...`

#### Step 4.1 — Create `internal/cli/compare.go`

```go
var compareCmd = &cobra.Command{
    Use:   "compare <dir> [dir...]",
    Short: "Compare multiple eval results side-by-side",
    Args:  cobra.MinimumNArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        var summaries []EvalSummary
        for _, dir := range args {
            s, err := loadSummaryFromDir(dir)
            if err != nil { return err }
            summaries = append(summaries, *s)
        }
        comparison := generateComparison(summaries)
        // Write to stdout or --output file
        writeComparisonReport(os.Stdout, comparison)
        return nil
    },
}
```

#### Step 4.2 — Comparison generation (shared with multi-run)

```go
type Comparison struct {
    Runs       []ComparisonRun       `json:"runs"`
    TaskMatrix map[string]map[string]string `json:"task_matrix"`
    BestRun    string                `json:"best_run"`
    BestScore  float64               `json:"best_weighted_score"`
}

type ComparisonRun struct {
    ID                  string  `json:"id"`
    Agent               string  `json:"agent"`
    Model               string  `json:"model"`
    Reasoning           string  `json:"reasoning"`
    PassRate            float64 `json:"pass_rate"`
    WeightedPassRate    float64 `json:"weighted_pass_rate"`
    WeightedScore       float64 `json:"weighted_score"`
    Passed              int     `json:"passed"`
    Failed              int     `json:"failed"`
    Total               int     `json:"total"`
    Duration            float64 `json:"duration_seconds"`
    IntegrityViolations int     `json:"integrity_violations"`
}

func generateComparison(summaries []EvalSummary) Comparison { ... }
func writeComparisonJSON(dir string, c Comparison) error { ... }
func writeComparisonMarkdown(w io.Writer, c Comparison) { ... }
```

The Markdown output includes the side-by-side table and task matrix (✅/❌) as shown in the design proposal.

---

### Phase 5 — Parallel Runs (Future)

Not in initial implementation. When added:

- Add `--parallel-runs N` flag (default 1).
- Wrap the orchestration loop in a semaphore-bounded goroutine pool (same pattern as existing intra-run parallelism in `eval.go` lines ~620–900).
- Guard: `parallel-runs × parallel ≤ 16`.
- Mutex on `multi-run-state.json` writes.

---

### Output Directory Structures

#### Single agent, `--repeat 1` (unchanged)

```
eval-results/
  gemini-2026-02-21T024300/
    summary.json, attestation.json, report.md, submission.json, run-config.json
    go-bank-account/
      agent.log, validation.log
```

#### Single agent, `--repeat 3`

```
eval-results/
  gemini-2026-02-21T024300/           # Umbrella dir
    multi-run-config.json
    multi-run-state.json
    repeat-stats.json
    repeat-report.md
    run-1/
      summary.json, report.md, ...
      go-bank-account/
    run-2/
      summary.json, report.md, ...
    run-3/
      summary.json, report.md, ...
```

**Naming:** For single-agent repeat, the umbrella dir uses the normal `<agent>-<timestamp>` naming (not `multi-`), since it's conceptually one config repeated.

#### Multi-agent, `--repeat 1`

```
eval-results/
  multi-2026-02-21T024300/
    multi-run-config.json
    multi-run-state.json
    comparison.json
    comparison-report.md
    codex-gpt-5.2/
      summary.json, report.md, ...
    opencode-kimi-k2.5/
      summary.json, report.md, ...
```

#### Multi-agent, `--repeat 3`

```
eval-results/
  multi-2026-02-21T024300/
    multi-run-config.json
    multi-run-state.json
    comparison.json               # Compares mean stats across configs
    comparison-report.md
    repeat-stats.json             # Per-config repeat stats
    repeat-report.md
    codex-gpt-5.2/
      run-1/
        summary.json, ...
      run-2/
        summary.json, ...
      run-3/
        summary.json, ...
    opencode-kimi-k2.5/
      run-1/
        summary.json, ...
      run-2/
      run-3/
```

---

### Statistical Aggregation for `--repeat`

#### Per-config stats (`repeat-stats.json`)

```json
[
  {
    "config": {"agent": "gemini", "model": "gemini-2.5-flash", "reasoning": ""},
    "runs": 5,
    "pass_rates": [65.4, 69.2, 65.4, 73.1, 69.2],
    "mean_pass_rate": 68.46,
    "stddev_pass_rate": 3.07,
    "min_pass_rate": 65.4,
    "max_pass_rate": 73.1,
    "mean_weighted_score": 20.15,
    "stddev_weighted_score": 1.23,
    "min_weighted_score": 18.92,
    "max_weighted_score": 21.38,
    "mean_duration_seconds": 1842.5,
    "task_consistency": {
      "go/bank-account": 100.0,
      "go/react": 60.0,
      "rust/forth": 80.0,
      "typescript/grep": 40.0
    }
  }
]
```

#### Repeat report (`repeat-report.md`)

```markdown
### Repeat Analysis — gemini / gemini-2.5-flash (5 runs)

| Metric | Mean | Std Dev | Min | Max |
|--------|------|---------|-----|-----|
| Pass Rate | 68.5% | ±3.1% | 65.4% | 73.1% |
| Weighted Score | 20.15 | ±1.23 | 18.92 | 21.38 |
| Duration | 30m 42s | ±2m 15s | 28m 10s | 33m 05s |

### Task Consistency (sorted by flakiness)

| Task | Pass Rate | Status |
|------|-----------|--------|
| go/bank-account | 100% | ✅ Stable |
| rust/forth | 80% | ⚠️ Flaky |
| go/react | 60% | ⚠️ Flaky |
| typescript/grep | 40% | ❌ Unreliable |
```

#### When combined with multi-agent comparison

The `comparison.json` uses **mean** values from repeat stats instead of single-run values. The comparison report notes the number of repeats and includes confidence indicators.

---

### Resume Support

#### Multi-run state file (`multi-run-state.json`)

```json
{
  "id": "multi-2026-02-21T024300",
  "repeat": 3,
  "specs": [
    {"agent": "codex", "model": "gpt-5.2", "reasoning": "low"},
    {"agent": "opencode", "model": "kimi-k2.5", "reasoning": ""}
  ],
  "runs": [
    {"spec_index": 0, "repeat": 1, "dir": "codex-gpt-5.2/run-1", "status": "completed"},
    {"spec_index": 0, "repeat": 2, "dir": "codex-gpt-5.2/run-2", "status": "completed"},
    {"spec_index": 0, "repeat": 3, "dir": "codex-gpt-5.2/run-3", "status": "interrupted"},
    {"spec_index": 1, "repeat": 1, "dir": "opencode-kimi-k2.5/run-1", "status": "pending"},
    {"spec_index": 1, "repeat": 2, "dir": "opencode-kimi-k2.5/run-2", "status": "pending"},
    {"spec_index": 1, "repeat": 3, "dir": "opencode-kimi-k2.5/run-3", "status": "pending"}
  ]
}
```

#### Resume detection in `RunE`

```go
if evalResume != "" {
    if isMultiRunDir(evalResume) {
        return resumeMultiRun(ctx, evalResume, r)
    }
    // Existing single-run resume logic
}

func isMultiRunDir(dir string) bool {
    _, err := os.Stat(filepath.Join(dir, "multi-run-config.json"))
    return err == nil
}
```

`resumeMultiRun` loads the state file, skips completed runs, delegates interrupted runs to existing single-run resume, and executes pending runs.

#### Graceful shutdown

On SIGINT:
1. Mark current run as `"interrupted"` in state file.
2. Mark remaining runs as `"pending"`.
3. Print: `./sanity eval --resume ./eval-results/multi-2026-02-21T024300`

---

### Testing Strategy

#### Unit tests (no Docker required)

| Test | What it verifies |
|------|-----------------|
| `TestBroadcastOrSplit` | 1-value broadcast, N-value passthrough, count mismatch error, empty string |
| `TestMultiRunSubdir` | Directory naming with/without model, with/without repeat |
| `TestSanitizeModel` | Slash/colon/space replacement |
| `TestComputeRepeatStats` | Mean, stddev, min, max, task consistency from mock summaries |
| `TestGenerateComparison` | Task matrix, best run selection from mock summaries |
| `TestIsMultiRunDir` | Detection of multi-run vs single-run directories |

#### Integration tests (with `--dry-run`)

| Test | What it verifies |
|------|-----------------|
| `TestMultiAgentDryRun` | Comma-separated parsing produces correct specs, dry-run output lists all configs |
| `TestRepeatDryRun` | `--repeat 3` with dry-run shows 3 planned runs |
| `TestBatchDryRun` | Batch config file parsed correctly, dry-run output matches |

#### Resume tests (filesystem only)

| Test | What it verifies |
|------|-----------------|
| `TestMultiRunResume` | Create partial state file, verify resume skips completed, continues interrupted |

#### Math helpers

```go
func TestMean(t *testing.T) {
    tests := []struct{ in []float64; want float64 }{
        {[]float64{1, 2, 3}, 2.0},
        {[]float64{10}, 10.0},
    }
    for _, tt := range tests {
        if got := mean(tt.in); got != tt.want {
            t.Errorf("mean(%v) = %v, want %v", tt.in, got, tt.want)
        }
    }
}
```

---

### Implementation Order Summary

| Phase | Depends On | Effort | Deliverable |
|-------|-----------|--------|-------------|
| **0** | — | Medium (1–2 days) | `evalRunSingle()` extraction, `RunSpec`, `SharedConfig` |
| **1** | Phase 0 | Medium (1–2 days) | `--repeat N`, `RepeatStats`, repeat output files |
| **2** | Phase 0 | Medium (1–2 days) | Comma-separated multi-agent, orchestration loop, comparison output |
| **3** | Phase 2 | Small (0.5–1 day) | `sanity batch` command, TOML config parsing |
| **4** | Phase 2 | Small (0.5 day) | `sanity compare` standalone command |
| **5** | Phase 2 | Medium (1–2 days) | `--parallel-runs`, concurrent orchestration |

**Total estimated effort:** ~5–8 days for Phases 0–4. Phase 5 is deferred.

Phases 1 and 2 can be developed in parallel after Phase 0 is complete, since they both depend on `evalRunSingle()` but don't depend on each other. However, they must be integrated (repeat × multi-agent composition) before merging.
