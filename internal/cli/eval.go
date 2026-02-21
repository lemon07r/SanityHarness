package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/zeebo/blake3"

	"github.com/lemon07r/sanityharness/internal/config"
	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var (
	evalAgent string
	evalModel string
	// TODO(consistency): Consider passing evalReasoning explicitly through the call
	// stack (runTaskWithAgent -> executeAgentWithRetries -> runAgentAttempt) to match
	// the pattern used for model. Currently safe since it's read-only after CLI parse.
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
	evalRepeat         int
)

// Quota retry configuration.
const (
	quotaMaxRetries  = 5
	quotaRetryDelay1 = 30 * time.Second
	quotaRetryDelay2 = 60 * time.Second
	quotaRetryDelay3 = 120 * time.Second
	quotaRetryDelay4 = 240 * time.Second
	quotaRetryDelay5 = 480 * time.Second

	// Threshold for considering an agent log as an infra failure (empty or near-empty).
	infraFailureLogThreshold = 10 // bytes

	// Stop eval early after this many consecutive quota-exhausted tasks.
	// This allows resuming later when quota resets, rather than wasting time
	// on tasks that will all fail immediately.
	quotaExhaustedStopThreshold = 5
)

// Infra failure retry configuration (separate from quota retries).
const (
	infraMaxRetries  = 5
	infraRetryDelay1 = 15 * time.Second
	infraRetryDelay2 = 30 * time.Second
	infraRetryDelay3 = 60 * time.Second
	infraRetryDelay4 = 120 * time.Second
	infraRetryDelay5 = 240 * time.Second
)

// Patterns indicating recoverable rate limit errors (worth retrying).
// These use contextual phrases to avoid false positives from bare numbers
// appearing in durations (e.g. "0.503s"), UUIDs, git hashes, line numbers, etc.
var recoverablePatterns = []string{
	"rate limit",
	"rate_limit",
	"ratelimit",
	"too many requests",
	"overload",
	"temporarily unavailable",
	"try again later",
	"please try again",
	"service unavailable",
	"server error",
	"bad gateway",
	"http 429",
	"http 502",
	"http 503",
	"http 529",
	"status 429",
	"status 502",
	"status 503",
	"status 529",
	"error 429",
	"error 502",
	"error 503",
	"error 529",
	"code 429",
	"code 502",
	"code 503",
	"code 529",
	"429 too many",
	"502 bad gateway",
	"503 service",
	"529 ",
	"capacity limit",
}

// Patterns indicating non-recoverable quota errors (skip retries).
var nonRecoverablePatterns = []string{
	"quota reset",
	"daily quota",
	"monthly quota",
	"billing",
	"authentication failed",
	"unauthorized",
	"forbidden",
	"invalid api key",
	"api key invalid",
	"exhausted your capacity",
	"exceeded your current quota",
	"subscription limit",
}

// EvalResult holds the result of evaluating a single task.
type EvalResult struct {
	Task           string            `json:"task"`
	Language       string            `json:"language"`
	Tier           string            `json:"tier,omitempty"`
	Difficulty     string            `json:"difficulty,omitempty"`
	Passed         bool              `json:"passed"`
	AgentTimedOut  bool              `json:"agent_timed_out,omitempty"`
	Status         task.ResultStatus `json:"status,omitempty"`
	Attempts       int               `json:"attempts"`
	Duration       float64           `json:"duration_seconds"`
	AgentTime      float64           `json:"agent_duration_seconds,omitempty"`
	ValidateTime   float64           `json:"validation_duration_seconds,omitempty"`
	PromptChars    int               `json:"prompt_chars,omitempty"`
	Error          string            `json:"error,omitempty"`
	Weight         float64           `json:"weight,omitempty"`
	WeightedScore  float64           `json:"weighted_score,omitempty"`
	QuotaRetries   int               `json:"quota_retries,omitempty"`
	QuotaExhausted bool              `json:"quota_exhausted,omitempty"`
	InfraFailure   bool              `json:"infra_failure,omitempty"`
	WorkspaceDir   string            `json:"-"` // Not serialized, used for cleanup
}

// EvalAggregate summarizes results for a group (language, tier, difficulty).
type EvalAggregate struct {
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Total        int     `json:"total"`
	PassRate     float64 `json:"pass_rate"`
	Duration     float64 `json:"duration_seconds"`
	AgentTime    float64 `json:"agent_duration_seconds"`
	ValidateTime float64 `json:"validation_duration_seconds"`
}

// EvalSummary holds the overall evaluation summary.
type EvalSummary struct {
	Agent               string                   `json:"agent"`
	Model               string                   `json:"model,omitempty"`
	Reasoning           string                   `json:"reasoning,omitempty"`
	Timestamp           string                   `json:"timestamp"`
	Tier                string                   `json:"tier,omitempty"`
	Difficulty          string                   `json:"difficulty,omitempty"`
	Parallel            int                      `json:"parallel,omitempty"`
	Results             []EvalResult             `json:"results"`
	Passed              int                      `json:"passed"`
	Failed              int                      `json:"failed"`
	Total               int                      `json:"total"`
	PassRate            float64                  `json:"pass_rate"`
	WeightedScore       float64                  `json:"weighted_score,omitempty"`
	MaxPossibleScore    float64                  `json:"max_possible_score,omitempty"`
	WeightedPassRate    float64                  `json:"weighted_pass_rate,omitempty"`
	IntegrityViolations int                      `json:"integrity_violations,omitempty"`
	Duration            float64                  `json:"duration_seconds,omitempty"`
	AgentTime           float64                  `json:"agent_duration_seconds,omitempty"`
	ValidateTime        float64                  `json:"validation_duration_seconds,omitempty"`
	PromptChars         int                      `json:"prompt_chars,omitempty"`
	ByLanguage          map[string]EvalAggregate `json:"by_language,omitempty"`
	ByTier              map[string]EvalAggregate `json:"by_tier,omitempty"`
	ByDifficulty        map[string]EvalAggregate `json:"by_difficulty,omitempty"`
	UseMCPTools         bool                     `json:"use_mcp_tools,omitempty"`
	DisableMCP          bool                     `json:"disable_mcp,omitempty"`
	Sandbox             bool                     `json:"sandbox,omitempty"`
	Legacy              bool                     `json:"legacy,omitempty"`
	QuotaAffectedTasks  int                      `json:"quota_affected_tasks,omitempty"`
	TotalQuotaRetries   int                      `json:"total_quota_retries,omitempty"`
}

// RunSpec defines a single eval run's configuration.
type RunSpec struct {
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
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

// RunConfig stores the original eval configuration for resume capability.
type RunConfig struct {
	Agent          string   `json:"agent"`
	Model          string   `json:"model,omitempty"`
	Reasoning      string   `json:"reasoning,omitempty"`
	Tier           string   `json:"tier,omitempty"`
	Difficulty     string   `json:"difficulty,omitempty"`
	Lang           string   `json:"lang,omitempty"`
	Tasks          string   `json:"tasks,omitempty"`
	Timeout        int      `json:"timeout"`
	Parallel       int      `json:"parallel"`
	UseMCPTools    bool     `json:"use_mcp_tools,omitempty"`
	DisableMCP     bool     `json:"disable_mcp,omitempty"`
	NoSandbox      bool     `json:"no_sandbox,omitempty"`
	Legacy         bool     `json:"legacy,omitempty"`
	KeepWorkspaces bool     `json:"keep_workspaces,omitempty"`
	TaskList       []string `json:"task_list"`
	CreatedAt      string   `json:"created_at"`
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate an agent against all tasks",
	Long: `Runs all (or selected) tasks against a coding agent and reports results.

Built-in agents:
  gemini    - Google Gemini CLI
  kilocode  - Kilo Code CLI
  opencode  - OpenCode CLI
  claude    - Anthropic Claude Code
  codex     - OpenAI Codex CLI
  kimi      - Moonshot Kimi CLI
  crush     - Crush CLI
  copilot   - GitHub Copilot CLI
  droid     - Factory Droid CLI
  iflow     - iFlow CLI
  qwen      - Qwen Code CLI
  amp       - Sourcegraph Amp CLI
  codebuff  - Codebuff CLI
  vibe      - Mistral Vibe CLI
  goose     - Block Goose CLI
  junie     - JetBrains Junie CLI
  ccs       - Claude Code Switch
  cline     - Cline CLI
  pi        - Pi CLI

Custom agents can be configured in sanity.toml under [agents.<name>].

Examples:
  sanity eval --agent gemini
  sanity eval --agent kilocode
  sanity eval --agent gemini --model gemini-2.5-pro
  sanity eval --agent opencode --model google/gemini-2.5-flash
  sanity eval --agent claude --lang go
  sanity eval --agent my-custom-agent --tasks bank-account,react
  sanity eval --agent gemini --dry-run
  sanity eval --resume ./eval-results/gemini-2026-01-19T192910`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Apply config defaults for flags not explicitly set.
		if !cmd.Flags().Changed("timeout") && evalTimeout == 0 {
			if cfg != nil && cfg.Harness.DefaultTimeout > 0 {
				evalTimeout = cfg.Harness.DefaultTimeout
			} else {
				evalTimeout = 600
			}
		}

		if evalRepeat < 1 {
			evalRepeat = 1
		}

		shared := SharedConfig{
			Tier: evalTier, Difficulty: evalDifficulty, Lang: evalLang,
			Tasks: evalTasks, Timeout: evalTimeout, Parallel: evalParallel,
			KeepWorkspaces: evalKeepWorkspaces, UseMCPTools: evalUseMCPTools,
			DisableMCP: evalDisableMCP, NoSandbox: evalNoSandbox,
			Legacy: evalLegacy, DryRun: evalDryRun,
		}

		// Track if we're resuming a previous run.
		var isResuming bool
		var previousResults []EvalResult
		var completedTasks map[string]bool
		var runCfg *RunConfig
		var timestamp string

		// Handle resume mode: load config and apply settings.
		var prevAttestation *EvalAttestation
		if evalResume != "" {
			// Check if this is a multi-run directory.
			if isMultiRunDir(evalResume) {
				return resumeMultiRun(evalResume)
			}

			var err error
			runCfg, err = loadRunConfig(evalResume)
			if err != nil {
				return fmt.Errorf("loading resume config: %w", err)
			}
			applyRunConfig(runCfg)
			evalOutputDir = evalResume
			isResuming = true

			// Re-build shared from restored globals.
			shared = SharedConfig{
				Tier: evalTier, Difficulty: evalDifficulty, Lang: evalLang,
				Tasks: evalTasks, Timeout: evalTimeout, Parallel: evalParallel,
				KeepWorkspaces: evalKeepWorkspaces, UseMCPTools: evalUseMCPTools,
				DisableMCP: evalDisableMCP, NoSandbox: evalNoSandbox,
				Legacy: evalLegacy, DryRun: evalDryRun,
			}

			completedTasks, err = findCompletedTasks(evalOutputDir)
			if err != nil {
				return fmt.Errorf("finding completed tasks: %w", err)
			}

			prevSummary, err := loadPreviousSummary(evalOutputDir)
			if err != nil {
				return fmt.Errorf("loading previous results: %w", err)
			}
			if prevSummary != nil {
				previousResults = prevSummary.Results
				timestamp = prevSummary.Timestamp
			}

			// Load previous attestation to preserve hashes of tasks whose workspaces are gone.
			prevAttestation, err = loadPreviousAttestation(evalOutputDir)
			if err != nil {
				logger.Warn("failed to load previous attestation", "error", err)
			}
		}

		if timestamp == "" {
			timestamp = time.Now().Format("2006-01-02T150405")
		}

		// Parse comma-separated agent/model/reasoning for multi-agent support.
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

		// Dry-run mode doesn't require agent to be installed.
		if !evalDryRun {
			for _, spec := range specs {
				if spec.Agent == "" {
					return fmt.Errorf("--agent is required (use --help to see available agents)")
				}
				agentCfg := cfg.GetAgent(spec.Agent)
				if agentCfg == nil {
					available := strings.Join(cfg.ListAgents(), ", ")
					return fmt.Errorf("unknown agent: %s (available: %s)", spec.Agent, available)
				}
				if _, err := exec.LookPath(agentCfg.Command); err != nil {
					return fmt.Errorf("agent %q binary %q not found in PATH", spec.Agent, agentCfg.Command)
				}
			}
		}

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		if shared.Legacy {
			r.LegacyHiddenTests = true
			logger.Info("legacy mode enabled: hidden tests exposed to agent (pre-v1.6.0 behavior)")
		}

		// If the user specified another selector, default tier should not hide tasks.
		tierChanged := cmd.Flags().Changed("tier")
		if !tierChanged && (shared.Lang != "" || shared.Tasks != "" || shared.Difficulty != "") {
			shared.Tier = "all"
			evalTier = "all"
		}

		switch shared.Tier {
		case "", "core", "extended", "all":
			// OK
		default:
			return fmt.Errorf("invalid --tier %q (valid: core, extended, all)", shared.Tier)
		}

		// Get tasks to run
		allTasks, err := r.ListTasks()
		if err != nil {
			return fmt.Errorf("listing tasks: %w", err)
		}

		// Filter by specific tasks if specified
		if evalTasks != "" {
			tokens := strings.Split(evalTasks, ",")
			var selected []*task.Task
			seen := make(map[string]bool)
			for _, tok := range tokens {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				t, err := task.ResolveRef(allTasks, tok)
				if err != nil {
					return fmt.Errorf("resolving task %q: %w", tok, err)
				}
				if !seen[t.ID()] {
					seen[t.ID()] = true
					selected = append(selected, t)
				}
			}
			allTasks = selected
		}

		// Filter by language if specified
		if evalLang != "" {
			lang, err := task.ParseLanguage(evalLang)
			if err != nil {
				return err
			}
			var filtered []*task.Task
			for _, t := range allTasks {
				if t.Language == lang {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		// Filter by difficulty if specified
		if evalDifficulty != "" {
			want := make(map[string]bool)
			for _, tok := range strings.Split(evalDifficulty, ",") {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				want[tok] = true
			}
			var filtered []*task.Task
			for _, t := range allTasks {
				if want[t.Difficulty] {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		// Filter by tier if specified
		if evalTier != "" && evalTier != "all" {
			var filtered []*task.Task
			for _, t := range allTasks {
				if t.Tier == evalTier {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		if len(allTasks) == 0 {
			return fmt.Errorf("no tasks match the specified filters")
		}

		// Dry-run mode: print what would be executed and exit
		if shared.DryRun {
			fmt.Println()
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println(" SANITY HARNESS - Dry Run")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println()
			for _, spec := range specs {
				if spec.Agent != "" {
					fmt.Printf(" Agent:      %s\n", spec.Agent)
				}
				if spec.Model != "" {
					fmt.Printf(" Model:      %s\n", spec.Model)
				}
				if spec.Reasoning != "" {
					fmt.Printf(" Reasoning:  %s\n", spec.Reasoning)
				}
			}
			if shared.Tier != "" {
				fmt.Printf(" Tier:       %s\n", shared.Tier)
			}
			if shared.Difficulty != "" {
				fmt.Printf(" Difficulty: %s\n", shared.Difficulty)
			}
			if evalRepeat > 1 {
				fmt.Printf(" Repeat:     %d\n", evalRepeat)
			}
			fmt.Printf(" Tasks:      %d\n", len(allTasks))
			fmt.Println()
			fmt.Println(" Tasks that would be executed:")
			fmt.Println("─────────────────────────────────────────────────────────────")
			for i, t := range allTasks {
				timeout := shared.Timeout
				if t.AgentTimeout > 0 {
					timeout = t.AgentTimeout
				}
				fmt.Printf(" %3d. %-35s [%s, %s, %ds]\n",
					i+1, t.ID(), t.Tier, t.Difficulty, timeout)
			}
			fmt.Println("─────────────────────────────────────────────────────────────")
			fmt.Println()
			return nil
		}

		// Detect sandbox availability.
		evalSandboxActive = initSandbox()

		// Protect the tasks/ directory from agent modification during eval.
		if restoreFn, err := protectTasksDir(); err != nil {
			logger.Warn("failed to protect tasks directory", "error", err)
		} else if restoreFn != nil {
			defer restoreFn()
		}

		// Set up interrupt handler for graceful shutdown.
		interruptCtx, interruptCancel := setupInterruptHandler()
		defer interruptCancel()

		if isMultiRun {
			// Multi-run mode: create umbrella directory and orchestrate runs.
			var umbrellaDir string
			if evalOutputDir != "" {
				umbrellaDir = evalOutputDir
			} else if len(specs) == 1 {
				// Single-agent repeat: use normal naming.
				umbrellaDir = filepath.Join("eval-results", fmt.Sprintf("%s-%s", specs[0].Agent, timestamp))
			} else {
				umbrellaDir = filepath.Join("eval-results", fmt.Sprintf("multi-%s", timestamp))
			}
			if err := os.MkdirAll(umbrellaDir, 0o755); err != nil {
				return fmt.Errorf("creating umbrella directory: %w", err)
			}

			writeMultiRunConfig(umbrellaDir, specs, shared, evalRepeat)

			var allSummaries []runResult
			for specIdx, spec := range specs {
				for rep := 1; rep <= evalRepeat; rep++ {
					if checkInterrupted(interruptCtx) {
						updateMultiRunState(umbrellaDir, allSummaries, specs, evalRepeat, true)
						printMultiRunResumeCommand(umbrellaDir)
						return nil
					}

					runDir := multiRunSubdir(umbrellaDir, spec, specIdx, rep, evalRepeat)
					summary, _, err := evalRunSingle(
						interruptCtx, spec, shared, allTasks, allTasks,
						runDir, timestamp, r, false, nil, nil, nil, nil,
					)
					rr := runResult{spec: spec, repeat: rep, summary: summary}
					if err != nil {
						logger.Warn("run failed", "agent", spec.Agent, "repeat", rep, "error", err)
						rr.err = err
					}
					allSummaries = append(allSummaries, rr)
					updateMultiRunState(umbrellaDir, allSummaries, specs, evalRepeat, false)
				}
			}

			// Generate comparison if multiple specs.
			if len(specs) > 1 {
				var summaries []EvalSummary
				for _, rr := range allSummaries {
					if rr.summary != nil {
						summaries = append(summaries, *rr.summary)
					}
				}
				if len(summaries) > 1 {
					comparison := generateComparison(summaries)
					writeComparisonJSON(umbrellaDir, comparison)
					writeComparisonMarkdown(umbrellaDir, comparison)
				}
			}

			// Generate repeat stats if repeating.
			if evalRepeat > 1 {
				writeRepeatStats(umbrellaDir, specs, allSummaries, evalRepeat)
			}

			fmt.Printf("\n Multi-run results saved to: %s\n\n", umbrellaDir)
			return nil
		}

		// Single run — unchanged behavior.
		spec := specs[0]

		// Create output directory.
		if evalOutputDir == "" {
			evalOutputDir = filepath.Join("eval-results", fmt.Sprintf("%s-%s", spec.Agent, timestamp))
		}

		_, _, err = evalRunSingle(
			interruptCtx, spec, shared, allTasks, allTasks,
			evalOutputDir, timestamp, r, isResuming,
			previousResults, completedTasks, prevAttestation, runCfg,
		)
		return err
	},
}


// evalRunSingle executes a single eval run for one agent/model/reasoning combination.
// It handles output directory creation, task execution, aggregation, and output file writing.
func evalRunSingle(
	interruptCtx context.Context,
	spec RunSpec,
	shared SharedConfig,
	allTasks []*task.Task,
	tasksToRun []*task.Task,
	outputDir string,
	timestamp string,
	r *runner.Runner,
	isResuming bool,
	previousResults []EvalResult,
	completedTasks map[string]bool,
	prevAttestation *EvalAttestation,
	runCfg *RunConfig,
) (*EvalSummary, *EvalAttestation, error) {
	// Set globals that sub-functions (runTaskWithAgent, runAgentAttempt, etc.) read.
	evalAgent = spec.Agent
	evalModel = spec.Model
	evalReasoning = spec.Reasoning
	evalUseMCPTools = shared.UseMCPTools
	evalDisableMCP = shared.DisableMCP
	evalLegacy = shared.Legacy
	evalKeepWorkspaces = shared.KeepWorkspaces

	// Create output directory.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("creating output directory: %w", err)
	}

	// For resume mode: filter out completed tasks and clean incomplete dirs.
	totalTaskCount := len(allTasks)
	if isResuming && runCfg != nil {
		// Build task map for ordering from run config.
		taskMap := make(map[string]*task.Task)
		for _, t := range allTasks {
			taskMap[string(t.Language)+"/"+t.Slug] = t
		}

		// Restore original task order from run config.
		var orderedTasks []*task.Task
		var missingTasks []string
		for _, slug := range runCfg.TaskList {
			if t, ok := taskMap[slug]; ok {
				orderedTasks = append(orderedTasks, t)
			} else {
				missingTasks = append(missingTasks, slug)
			}
		}
		if len(missingTasks) > 0 {
			logger.Warn("some tasks from original run not found in current build",
				"missing", missingTasks,
				"count", len(missingTasks))
			fmt.Printf(" Warning: %d task(s) from original run not found: %v\n",
				len(missingTasks), missingTasks)
		}
		allTasks = orderedTasks

		// Clean up incomplete task directories.
		if err := cleanIncompleteTaskDirs(outputDir, completedTasks, allTasks); err != nil {
			return nil, nil, fmt.Errorf("cleaning incomplete tasks: %w", err)
		}

		// Filter out completed tasks.
		tasksToRun = nil
		for _, t := range allTasks {
			taskSlug := string(t.Language) + "/" + t.Slug
			if !completedTasks[taskSlug] {
				tasksToRun = append(tasksToRun, t)
			}
		}

		if len(tasksToRun) == 0 {
			fmt.Println("\n All tasks already completed. Nothing to resume.")
			return nil, nil, nil
		}
	} else {
		// Save run config for new runs (enables resume).
		if err := saveRunConfig(outputDir, allTasks); err != nil {
			return nil, nil, fmt.Errorf("saving run config: %w", err)
		}
	}

	var wasInterrupted bool

	// Print header
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if isResuming {
		fmt.Println(" SANITY HARNESS - Agent Evaluation (RESUMING)")
	} else {
		fmt.Println(" SANITY HARNESS - Agent Evaluation")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf(" Agent:   %s\n", spec.Agent)
	if spec.Model != "" {
		fmt.Printf(" Model:   %s\n", spec.Model)
	}
	if shared.Tier != "" {
		fmt.Printf(" Tier:    %s\n", shared.Tier)
	}
	if shared.Difficulty != "" {
		fmt.Printf(" Difficulty: %s\n", shared.Difficulty)
	}
	if shared.Parallel > 1 {
		fmt.Printf(" Parallel: %d\n", shared.Parallel)
	}
	if evalSandboxActive {
		fmt.Println(" Sandbox: enabled (bwrap)")
	}
	if isResuming {
		fmt.Printf(" Tasks:   %d remaining of %d total\n", len(tasksToRun), totalTaskCount)
	} else {
		fmt.Printf(" Tasks:   %d\n", len(tasksToRun))
	}
	fmt.Printf(" Output:  %s\n", outputDir)
	fmt.Println()

	// Run tasks
	results := make([]EvalResult, 0, len(tasksToRun))
	passed, failed := 0, 0
	var infraFailedTasks []string // Tasks that failed due to infra issues (excluded from results)

	parallel := shared.Parallel
	if parallel <= 0 {
		parallel = 1
	}

	if parallel == 1 {
		consecutiveQuotaExhausted := 0
		for i, t := range tasksToRun {
			// Check for interrupt before starting next task.
			if checkInterrupted(interruptCtx) {
				wasInterrupted = true
				fmt.Println("\n\033[33m⚠ Interrupt received. Saving partial results...\033[0m")
				break
			}

			fmt.Println("─────────────────────────────────────────────────────────────")
			fmt.Printf(" [%d/%d] %s\n", i+1, len(tasksToRun), t.ID())
			fmt.Println("─────────────────────────────────────────────────────────────")

			result := runTaskWithAgent(interruptCtx, r, t, spec.Agent, spec.Model, outputDir, shared.Timeout)

			// Infra failures are excluded from results so they can be resumed later.
			if result.InfraFailure {
				fmt.Printf(" ⚠ INFRA FAILURE — will be skipped (resumable)\n")
				infraFailedTasks = append(infraFailedTasks, t.ID())
				// Delete workspace so resume picks it up as incomplete.
				if result.WorkspaceDir != "" {
					_ = os.RemoveAll(result.WorkspaceDir)
				}
				// Also remove the task output dir (agent.log copy).
				taskOutputDir := filepath.Join(outputDir, fmt.Sprintf("%s-%s", t.Language, t.Slug))
				_ = os.RemoveAll(taskOutputDir)
				consecutiveQuotaExhausted++
				if consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
					wasInterrupted = true
					fmt.Printf("\n\033[33m⚠ Infra/quota failures for %d consecutive tasks. Stopping early to allow resume.\033[0m\n", consecutiveQuotaExhausted)
					break
				}
				fmt.Println()
				continue
			}

			results = append(results, result)

			if result.Passed {
				fmt.Printf(" ✓ PASSED (%.2fs)\n", result.Duration)
				passed++
				consecutiveQuotaExhausted = 0 // Reset counter on success
			} else {
				fmt.Printf(" ✗ FAILED (%.2fs)\n", result.Duration)
				if result.Error != "" {
					fmt.Printf("   Error: %s\n", result.Error)
				}
				failed++

				// Track consecutive quota exhaustion
				if result.QuotaExhausted {
					consecutiveQuotaExhausted++
					if consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
						wasInterrupted = true
						fmt.Printf("\n\033[33m⚠ Quota exhausted for %d consecutive tasks. Stopping early to allow resume.\033[0m\n", consecutiveQuotaExhausted)
						break
					}
				} else {
					consecutiveQuotaExhausted = 0 // Reset on non-quota failure
				}
			}

			// Clean up workspace unless --keep-workspaces is set
			if !shared.KeepWorkspaces && result.WorkspaceDir != "" {
				if err := os.RemoveAll(result.WorkspaceDir); err != nil {
					logger.Debug("failed to cleanup workspace", "dir", result.WorkspaceDir, "error", err)
				}
			}

			fmt.Println()
		}
	} else {
		type job struct {
			idx int
			t   *task.Task
		}
		type jobResult struct {
			idx int
			r   EvalResult
		}

		jobs := make(chan job)
		jobResults := make(chan jobResult)
		stopSending := make(chan struct{})

		var wg sync.WaitGroup
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					res := runTaskWithAgent(interruptCtx, r, j.t, spec.Agent, spec.Model, outputDir, shared.Timeout)
					jobResults <- jobResult{idx: j.idx, r: res}
				}
			}()
		}

		// Producer goroutine: sends jobs, stops on interrupt.
		go func() {
			for i, t := range tasksToRun {
				select {
				case <-stopSending:
					// Interrupt received, stop sending new jobs.
					close(jobs)
					wg.Wait()
					close(jobResults)
					return
				case jobs <- job{idx: i, t: t}:
				}
			}
			close(jobs)
			wg.Wait()
			close(jobResults)
		}()

		collected := make([]EvalResult, len(tasksToRun))
		seen := 0
		consecutiveQuotaExhausted := 0
	collectLoop:
		for jr := range jobResults {
			seen++

			// Infra failures are excluded from results so they can be resumed later.
			if jr.r.InfraFailure {
				fmt.Printf(" [%d/%d] %s ⚠ INFRA FAILURE — will be skipped (resumable)\n", seen, len(tasksToRun), jr.r.Task)
				infraFailedTasks = append(infraFailedTasks, jr.r.Task)
				if jr.r.WorkspaceDir != "" {
					_ = os.RemoveAll(jr.r.WorkspaceDir)
				}
				// Remove task output dir (agent.log copy).
				parts := strings.SplitN(jr.r.Task, "/", 2)
				if len(parts) == 2 {
					taskOutputDir := filepath.Join(outputDir, parts[0]+"-"+parts[1])
					_ = os.RemoveAll(taskOutputDir)
				}
				consecutiveQuotaExhausted++
			} else {
				collected[jr.idx] = jr.r

				status := "FAILED"
				if jr.r.Passed {
					status = "PASSED"
				}
				fmt.Printf(" [%d/%d] %s %s (%.2fs)\n", seen, len(tasksToRun), jr.r.Task, status, jr.r.Duration)
				if !jr.r.Passed && jr.r.Error != "" {
					fmt.Printf("   Error: %s\n", jr.r.Error)
				}

				if jr.r.Passed {
					passed++
					consecutiveQuotaExhausted = 0 // Reset counter on success
				} else {
					failed++
					// Track consecutive quota exhaustion
					if jr.r.QuotaExhausted {
						consecutiveQuotaExhausted++
					} else {
						consecutiveQuotaExhausted = 0 // Reset on non-quota failure
					}
				}

				if !shared.KeepWorkspaces && jr.r.WorkspaceDir != "" {
					if err := os.RemoveAll(jr.r.WorkspaceDir); err != nil {
						logger.Debug("failed to cleanup workspace", "dir", jr.r.WorkspaceDir, "error", err)
					}
				}
			}

			// Check for interrupt after each result.
			shouldStop := checkInterrupted(interruptCtx)
			stopReason := "Interrupt received"

			// Also stop if we hit consecutive quota exhaustion threshold
			if !shouldStop && consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
				shouldStop = true
				stopReason = fmt.Sprintf("Quota/infra failures for %d consecutive tasks", consecutiveQuotaExhausted)
			}

			if shouldStop {
				wasInterrupted = true
				fmt.Printf("\n\033[33m⚠ %s. Waiting for in-flight tasks...\033[0m\n", stopReason)
				close(stopSending)
				// Drain remaining results from in-flight tasks.
				for jr := range jobResults {
					if jr.r.InfraFailure {
						infraFailedTasks = append(infraFailedTasks, jr.r.Task)
						if jr.r.WorkspaceDir != "" {
							_ = os.RemoveAll(jr.r.WorkspaceDir)
						}
						parts := strings.SplitN(jr.r.Task, "/", 2)
						if len(parts) == 2 {
							taskOutputDir := filepath.Join(outputDir, parts[0]+"-"+parts[1])
							_ = os.RemoveAll(taskOutputDir)
						}
					} else {
						collected[jr.idx] = jr.r
						if jr.r.Passed {
							passed++
						} else {
							failed++
						}
						if !shared.KeepWorkspaces && jr.r.WorkspaceDir != "" {
							_ = os.RemoveAll(jr.r.WorkspaceDir)
						}
					}
				}
				break collectLoop
			}
		}
		// Only include results that were actually run (excluding infra failures).
		for _, r := range collected {
			if r.Task != "" {
				results = append(results, r)
			}
		}
	}

	// If resuming, merge with previous results.
	if isResuming && len(previousResults) > 0 {
		// Build a set of task IDs from new results.
		newResultTasks := make(map[string]bool)
		for _, r := range results {
			newResultTasks[r.Task] = true
		}
		// Prepend previous results that aren't in the new results.
		var merged []EvalResult
		for _, r := range previousResults {
			if !newResultTasks[r.Task] {
				merged = append(merged, r)
				if r.Passed {
					passed++
				} else {
					failed++
				}
			}
		}
		results = append(merged, results...)
	}

	// Sort results to match allTasks order
	taskOrder := make(map[string]int)
	for i, t := range allTasks {
		taskOrder[t.ID()] = i
	}
	sort.Slice(results, func(i, j int) bool {
		return taskOrder[results[i].Task] < taskOrder[results[j].Task]
	})

	// Recompute status and weighted score for all results.
	// Previous results loaded from summary.json during resume may lack
	// these fields (they were never set due to a defer/named-return bug).
	taskWeights := make(map[string]task.Weight)
	for _, t := range allTasks {
		taskWeights[t.ID()] = task.ComputeWeight(t)
	}
	for i := range results {
		r := &results[i]
		w, ok := taskWeights[r.Task]
		if ok {
			r.Weight = w.Base
		}
		r.Status = task.DetermineStatus(r.Passed, r.AgentTimedOut, r.Error)
		r.WeightedScore = task.ScoreResult(r.Passed, r.AgentTimedOut, r.Error, w)
	}

	// Calculate pass rate
	total := passed + failed
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}

	// Print summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" EVALUATION SUMMARY")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf(" Agent:     %s\n", spec.Agent)
	if spec.Model != "" {
		fmt.Printf(" Model:     %s\n", spec.Model)
	}
	fmt.Printf(" Passed:    %d\n", passed)
	fmt.Printf(" Failed:    %d\n", failed)
	fmt.Printf(" Total:     %d\n", total)
	fmt.Printf(" Pass Rate: %.1f%%\n", passRate)
	fmt.Println()

	// Save summary
	// Aggregate stats
	byLanguage := make(map[string]EvalAggregate)
	byTier := make(map[string]EvalAggregate)
	byDifficulty := make(map[string]EvalAggregate)

	var totalDuration float64
	var totalAgentTime float64
	var totalValidateTime float64
	var totalPromptChars int
	var totalWeightedScore float64
	var maxPossibleScore float64
	var integrityViolations int

	addAgg := func(m map[string]EvalAggregate, key string, r EvalResult) {
		agg := m[key]
		if r.Passed {
			agg.Passed++
		} else {
			agg.Failed++
		}
		agg.Total++
		agg.Duration += r.Duration
		agg.AgentTime += r.AgentTime
		agg.ValidateTime += r.ValidateTime
		m[key] = agg
	}

	for _, r := range results {
		totalDuration += r.Duration
		totalAgentTime += r.AgentTime
		totalValidateTime += r.ValidateTime
		totalPromptChars += r.PromptChars
		totalWeightedScore += r.WeightedScore
		maxPossibleScore += r.Weight

		// Count by status
		if r.Status == task.StatusIntegrityViolation {
			integrityViolations++
		}

		addAgg(byLanguage, r.Language, r)
		if r.Tier != "" {
			addAgg(byTier, r.Tier, r)
		}
		if r.Difficulty != "" {
			addAgg(byDifficulty, r.Difficulty, r)
		}
	}

	// Calculate weighted pass rate
	weightedPassRate := 0.0
	if maxPossibleScore > 0 {
		weightedPassRate = totalWeightedScore / maxPossibleScore * 100
	}

	finalize := func(m map[string]EvalAggregate) map[string]EvalAggregate {
		for k, v := range m {
			if v.Total > 0 {
				v.PassRate = float64(v.Passed) / float64(v.Total) * 100
			}
			m[k] = v
		}
		return m
	}

	// Aggregate quota statistics
	var quotaAffectedTasks int
	var totalQuotaRetries int
	for _, r := range results {
		if r.QuotaExhausted {
			quotaAffectedTasks++
		}
		totalQuotaRetries += r.QuotaRetries
	}

	// Default model to "unknown" if not specified
	model := spec.Model
	if model == "" {
		model = "unknown"
	}

	summary := EvalSummary{
		Agent:               spec.Agent,
		Model:               model,
		Reasoning:           spec.Reasoning,
		Timestamp:           timestamp,
		Tier:                shared.Tier,
		Difficulty:          shared.Difficulty,
		Parallel:            parallel,
		Results:             results,
		Passed:              passed,
		Failed:              failed,
		Total:               total,
		PassRate:            passRate,
		WeightedScore:       totalWeightedScore,
		MaxPossibleScore:    maxPossibleScore,
		WeightedPassRate:    weightedPassRate,
		IntegrityViolations: integrityViolations,
		Duration:            totalDuration,
		AgentTime:           totalAgentTime,
		ValidateTime:        totalValidateTime,
		PromptChars:         totalPromptChars,
		ByLanguage:          finalize(byLanguage),
		ByTier:              finalize(byTier),
		ByDifficulty:        finalize(byDifficulty),
		UseMCPTools:         shared.UseMCPTools,
		DisableMCP:          shared.DisableMCP,
		Sandbox:             evalSandboxActive,
		Legacy:              shared.Legacy,
		QuotaAffectedTasks:  quotaAffectedTasks,
		TotalQuotaRetries:   totalQuotaRetries,
	}

	summaryPath := filepath.Join(outputDir, "summary.json")
	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, summaryData, 0644); err != nil {
		logger.Warn("failed to save summary", "error", err)
	} else {
		fmt.Printf(" Results saved to: %s\n", summaryPath)
	}

	// Generate attestation for verification
	loader := task.NewLoader(tasks.FS, tasksDir)
	var prevTasks map[string]AttestationTask
	if prevAttestation != nil {
		prevTasks = prevAttestation.Tasks
	}
	// Build set of tasks that were newly run in this session
	newlyRunTasks := make(map[string]bool)
	for _, t := range tasksToRun {
		newlyRunTasks[t.ID()] = true
	}
	attestation, err := generateAttestation(
		spec.Agent, spec.Model, timestamp, totalDuration,
		results, outputDir, loader, allTasks, newlyRunTasks, prevTasks,
	)
	if err != nil {
		logger.Warn("failed to generate attestation", "error", err)
	} else {
		attestationPath := filepath.Join(outputDir, "attestation.json")
		attestationData, _ := json.MarshalIndent(attestation, "", "  ")
		if err := os.WriteFile(attestationPath, attestationData, 0644); err != nil {
			logger.Warn("failed to save attestation", "error", err)
		} else {
			fmt.Printf(" Attestation saved to: %s\n", attestationPath)
		}
	}

	// Generate human-readable report.md
	reportMd := generateEvalReport(summary, attestation)
	reportPath := filepath.Join(outputDir, "report.md")
	if err := os.WriteFile(reportPath, []byte(reportMd), 0644); err != nil {
		logger.Warn("failed to save report", "error", err)
	} else {
		fmt.Printf(" Report saved to: %s\n", reportPath)
	}

	// Generate leaderboard submission file
	submission := generateLeaderboardSubmission(summary, attestation)
	submissionData, _ := json.MarshalIndent(submission, "", "  ")
	submissionPath := filepath.Join(outputDir, "submission.json")
	if err := os.WriteFile(submissionPath, submissionData, 0644); err != nil {
		logger.Warn("failed to save submission", "error", err)
	} else {
		fmt.Printf(" Submission saved to: %s\n", submissionPath)
	}

	fmt.Println()

	// Report infra failures and provide resume command.
	if len(infraFailedTasks) > 0 {
		fmt.Println("\033[33m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
		fmt.Printf("\033[33m ⚠ %d task(s) skipped due to infrastructure failures:\033[0m\n", len(infraFailedTasks))
		for _, t := range infraFailedTasks {
			fmt.Printf("   • %s\n", t)
		}
		fmt.Println()
		fmt.Println(" These tasks were not counted in the results above.")
		fmt.Println(" To retry them, run:")
		fmt.Printf("   ./sanity eval --resume %s\n", outputDir)
		fmt.Println("\033[33m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
		fmt.Println()
	}

	// If interrupted, print resume command.
	if wasInterrupted {
		printResumeCommand(outputDir)
	}

	return &summary, attestation, nil

}

func runTaskWithAgent(ctx context.Context, r *runner.Runner, t *task.Task, agent, model, outputDir string, timeout int) (result EvalResult) {
	start := time.Now()
	weight := task.ComputeWeight(t)
	result = EvalResult{
		Task:       t.ID(),
		Language:   string(t.Language),
		Tier:       t.Tier,
		Difficulty: t.Difficulty,
		Weight:     weight.Base,
	}
	defer finalizeEvalResult(&result, start, weight)

	loader := task.NewLoader(tasks.FS, tasksDir)

	// Create workspace for this task - use language prefix to avoid slug collisions
	workspaceName := fmt.Sprintf("%s-%s", t.Language, t.Slug)
	workspaceDir := filepath.Join(outputDir, workspaceName)
	result.WorkspaceDir = workspaceDir // Track for cleanup

	// Create an isolated temp workspace for the agent so it cannot read
	// other eval results or sibling task directories. After the agent
	// finishes, files are copied back to the real workspace for validation.
	agentWorkDir, err := os.MkdirTemp("", fmt.Sprintf("sanity-eval-%s-%s-*", t.Language, t.Slug))
	if err != nil {
		result.Error = fmt.Sprintf("creating temp workspace: %v", err)
		return result
	}
	defer func() { _ = os.RemoveAll(agentWorkDir) }()

	if err := r.InitWorkspaceForTask(t, agentWorkDir); err != nil {
		result.Error = fmt.Sprintf("init failed: %v", err)
		return result
	}

	// Get agent configuration
	agentCfg := cfg.GetAgent(agent)
	if agentCfg == nil {
		result.Error = fmt.Sprintf("unknown agent: %s", agent)
		return result
	}

	// Build agent command
	prompt := buildAgentPrompt(t, evalUseMCPTools, agentCfg.MCPPrompt)
	result.PromptChars = utf8.RuneCountInString(prompt)

	agentTimeout := time.Duration(timeout) * time.Second
	if agentTimeout <= 0 {
		agentTimeout = 600 * time.Second
	}
	// Apply per-agent default timeout as a floor (for agents with buffered output)
	if agentCfg.DefaultTimeout > 0 {
		agentMin := time.Duration(agentCfg.DefaultTimeout) * time.Second
		if agentTimeout < agentMin {
			agentTimeout = agentMin
		}
	}
	// Use task-specific timeout if set
	if t.AgentTimeout > 0 {
		agentTimeout = time.Duration(t.AgentTimeout) * time.Second
	}

	// Place agent.log in the task output directory (eval-results/<run>/<lang>-<slug>/).
	// This is outside the agent's temp workspace so the agent cannot read it.
	taskOutputDir := filepath.Join(outputDir, workspaceName)
	if err := os.MkdirAll(taskOutputDir, 0755); err != nil {
		result.Error = fmt.Sprintf("creating task output dir: %v", err)
		return result
	}
	agentLogPath := filepath.Join(taskOutputDir, "agent.log")

	// Execute agent in the isolated temp workspace
	workspaceReadyAt := time.Now()
	agentResult := executeAgentWithRetries(ctx, t, agentCfg, prompt, model, agentWorkDir, agentLogPath, agentTimeout, agent, workspaceReadyAt)
	result.AgentTime = agentResult.totalTime
	result.AgentTimedOut = agentResult.timedOut
	result.QuotaRetries = agentResult.quotaRetries
	result.QuotaExhausted = agentResult.quotaExhausted
	result.InfraFailure = agentResult.infraFailure

	// If the agent never produced output (infra failure), skip validation entirely.
	// The task will be excluded from results so it can be resumed later.
	if agentResult.infraFailure {
		result.Error = "infra failure: agent produced no output after retries"
		return result
	}

	// Ensure the agent didn't modify task-owned files.
	modified, err := detectModifiedTaskFiles(loader, t, agentWorkDir)
	if err != nil {
		result.Error = fmt.Sprintf("integrity check failed: %v", err)
		return result
	}
	if len(modified) > 0 {
		sort.Strings(modified)
		result.Error = fmt.Sprintf("modified task files (disallowed): %s", strings.Join(modified, ", "))
		return result
	}

	// Copy agent's work from temp workspace to the real workspace for validation.
	if err := copyDirContents(agentWorkDir, workspaceDir); err != nil {
		result.Error = fmt.Sprintf("copying agent workspace: %v", err)
		return result
	}

	// Add hidden tests (not shown to the agent) before validation.
	// In legacy mode, hidden tests are already in the workspace from init.
	if !evalLegacy {
		if err := writeTaskFilesToWorkspace(loader, t, workspaceDir, t.HiddenTestFiles()); err != nil {
			result.Error = fmt.Sprintf("writing hidden tests: %v", err)
			return result
		}
	}

	// Run sanity harness to validate.
	// Use at least 120s for validation regardless of the agent timeout,
	// since some tasks (e.g. Dart isolate tests) need more time to run.
	validationTimeout := timeout
	if validationTimeout < 120 {
		validationTimeout = 120
	}

	validationCmd := []string(nil)
	if t.Language == task.TypeScript && len(t.HiddenTestFiles()) > 0 {
		validationCmd = append([]string{}, t.ValidationCommand()...)
		for _, filename := range t.HiddenTestFiles() {
			validationCmd = append(validationCmd, task.StripTxtExtension(filename))
		}
	}

	validateStart := time.Now()
	session, err := r.Run(ctx, runner.RunOptions{
		Task:              t, // Pass task directly to avoid slug collision
		WorkspaceDir:      workspaceDir,
		Timeout:           validationTimeout,
		MaxAttempts:       1,
		ValidationCommand: validationCmd,
	})
	result.ValidateTime = time.Since(validateStart).Seconds()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Passed = session.Passed()
	result.Attempts = len(session.Attempts)

	// Save validation output to validation.log
	if len(session.Attempts) > 0 {
		lastAttempt := session.Attempts[len(session.Attempts)-1]
		validationLogPath := filepath.Join(taskOutputDir, "validation.log")
		_ = os.WriteFile(validationLogPath, []byte(lastAttempt.RawOutput), 0644)
	}

	return result
}

// finalizeEvalResult ensures status/score fields are populated for all return paths.
func finalizeEvalResult(result *EvalResult, start time.Time, weight task.Weight) {
	result.Duration = time.Since(start).Seconds()
	result.Status = task.DetermineStatus(result.Passed, result.AgentTimedOut, result.Error)
	result.WeightedScore = task.ScoreResult(result.Passed, result.AgentTimedOut, result.Error, weight)
}

// agentExecutionResult holds the outcome of agent execution with retries.
type agentExecutionResult struct {
	totalTime      float64
	timedOut       bool
	quotaRetries   int
	quotaExhausted bool
	infraRetries   int
	infraFailure   bool // true when agent produced no output after all retries
}

// executeAgentWithRetries runs the agent command with quota-aware retry logic.
// It also detects infra failures (empty/near-empty agent logs) and retries
// with aggressive backoff.
// workspaceReadyAt is the time when workspace setup completed (before the agent
// started); it is used to distinguish harness-written files from agent-written
// files when detecting infra failures.
func executeAgentWithRetries(
	ctx context.Context,
	t *task.Task,
	agentCfg *config.AgentConfig,
	prompt, model, workspaceDir, agentLogPath string,
	agentTimeout time.Duration,
	agent string,
	workspaceReadyAt time.Time,
) agentExecutionResult {
	var result agentExecutionResult
	var quotaAttempts, infraAttempts int
	var lastRetryType string // "quota" or "infra"

	for {
		totalAttempt := quotaAttempts + infraAttempts

		// On retry, wait (interruptible) and log.
		if totalAttempt > 0 {
			var delay time.Duration
			if lastRetryType == "infra" {
				delay = getInfraRetryDelay(infraAttempts)
			} else {
				delay = getRetryDelay(quotaAttempts)
			}
			logger.Info("retrying agent execution",
				"task", t.ID(),
				"attempt", totalAttempt,
				"type", lastRetryType,
				"delay", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
			}
		}

		// Check for interrupt before starting attempt.
		if ctx.Err() != nil {
			break
		}

		// Run single attempt.
		attemptResult := runAgentAttempt(ctx, agentCfg, prompt, model, workspaceDir, agentLogPath, agentTimeout, agent, totalAttempt)
		result.totalTime += attemptResult.duration
		result.timedOut = attemptResult.timedOut

		// Check for quota errors first.
		hasError, isRecoverable := detectQuotaError(agentLogPath)
		if hasError {
			if !isRecoverable {
				result.quotaExhausted = true
				logger.Debug("non-recoverable quota error, skipping retries", "task", t.ID())
				break
			}
			quotaAttempts++
			result.quotaRetries = quotaAttempts
			if quotaAttempts >= quotaMaxRetries {
				result.quotaExhausted = true
				logger.Debug("max quota retries reached", "task", t.ID(), "retries", quotaMaxRetries)
				break
			}
			lastRetryType = "quota"
			continue
		}

		// Check for infra failures (no quota error detected).
		if isInfraFailure(agentLogPath, workspaceDir, workspaceReadyAt) {
			infraAttempts++
			result.infraRetries = infraAttempts
			if infraAttempts >= infraMaxRetries {
				result.quotaExhausted = true
				result.infraFailure = true
				logger.Debug("max infra retries reached", "task", t.ID(), "retries", infraMaxRetries)
				break
			}
			lastRetryType = "infra"
			continue
		}

		// Success — no quota error, no infra failure.
		break
	}

	return result
}

// agentAttemptResult holds the outcome of a single agent attempt.
type agentAttemptResult struct {
	duration float64
	timedOut bool
}

// runAgentAttempt executes a single agent command attempt.
func runAgentAttempt(
	ctx context.Context,
	agentCfg *config.AgentConfig,
	prompt, model, workspaceDir, agentLogPath string,
	agentTimeout time.Duration,
	agent string,
	attempt int,
) agentAttemptResult {
	var result agentAttemptResult

	agentCtx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()

	cmd := buildAgentCommand(agentCtx, agentCfg, prompt, model, evalReasoning, evalDisableMCP, agent)
	cmd.Dir = workspaceDir

	// Use /dev/null for stdin to prevent TTY issues with agents that use Ink/React
	devNull, err := os.Open(os.DevNull)
	if err == nil {
		cmd.Stdin = devNull
		defer func() { _ = devNull.Close() }()
	}

	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil

	// Open log file: create on first attempt, append on retry
	logFile := openAgentLogFile(agentLogPath, attempt)
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer func() {
			_ = logFile.Sync()
			_ = logFile.Close()
		}()
	}

	// Wrap in bubblewrap sandbox if enabled.
	if evalSandboxActive {
		var extraDirs []string
		if cfg != nil {
			extraDirs = cfg.Sandbox.WritableDirs
		}
		cmd = wrapCommandWithSandbox(agentCtx, cmd, extraDirs)
	}

	// Run agent in its own process group so we can kill the entire tree on
	// timeout or interrupt, preventing orphaned child processes.
	setupProcessGroup(cmd)

	// Run agent
	agentStart := time.Now()
	agentErr := cmd.Run()
	result.duration = time.Since(agentStart).Seconds()

	// Check for timeout
	if errors.Is(agentCtx.Err(), context.DeadlineExceeded) {
		result.timedOut = true
		logger.Debug("agent timed out", "timeout", agentTimeout)
	}
	if agentErr != nil {
		logger.Debug("agent returned error", "error", agentErr)
	}

	return result
}

// openAgentLogFile opens the agent log file for writing.
func openAgentLogFile(agentLogPath string, attempt int) *os.File {
	var logFile *os.File
	var err error

	if attempt == 0 {
		logFile, err = os.Create(agentLogPath)
	} else {
		logFile, err = os.OpenFile(agentLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			separator := fmt.Sprintf("\n\n=== RETRY %d (after %v delay) ===\n\n", attempt, getRetryDelay(attempt))
			_, _ = logFile.WriteString(separator)
		}
	}

	if err != nil {
		return nil
	}
	return logFile
}

// toolchainInfo returns a human-readable toolchain description for the given language.
func toolchainInfo(lang task.Language) string {
	switch lang {
	case task.Go:
		return "Go 1.25"
	case task.Rust:
		return "Rust 1.83 (stable)"
	case task.Zig:
		return "Zig 0.13.0"
	case task.Dart:
		return "Dart 3.3 SDK"
	case task.TypeScript:
		return "Node.js 20 with TypeScript (tsx)"
	case task.Kotlin:
		return "Kotlin (JDK 21, Gradle 8.5)"
	default:
		return string(lang)
	}
}

func buildAgentPrompt(t *task.Task, useMCPTools bool, mcpPrompt string) string {
	stubFiles := make([]string, 0, len(t.Files.Stub))
	for _, f := range t.Files.Stub {
		stubFiles = append(stubFiles, task.StripTxtExtension(f))
	}
	testFiles := make([]string, 0, len(t.Files.Test))
	for _, f := range t.Files.Test {
		testFiles = append(testFiles, task.StripTxtExtension(f))
	}

	prompt := fmt.Sprintf(`You are solving a coding task called "%s".

TASK INFO:
- Language:    %s
- Tier:        %s
- Difficulty:  %s
- Description: %s

FILES TO READ:
- Stub/solution files: %s
- Test files:          %s

ENVIRONMENT:
- Tests run automatically in a Docker container.
- Toolchain: %s
- You do NOT need to run tests yourself.
- Do NOT search for or install language toolchains/SDKs.

YOUR TASK:
1. Read the stub file(s) (function signatures with panic()/todo!/Unimplemented placeholders).
2. Read the visible test file(s) to understand expected behavior and edge cases.
3. Implement the stub file(s), replacing placeholders with working code.
4. Ensure your solution handles edge cases and performance constraints.
5. Ensure thread-safety if the tests use concurrent operations.

IMPORTANT:
- There may be hidden tests that check additional edge cases for the same public API.

RULES:
- ONLY edit the stub/solution source file(s).
- Do NOT modify test files or support files.
- You may add new helper source files if needed.
- Evaluation fails if you modify protected files.
- Do NOT navigate to parent directories or read files outside the workspace.`,
		t.Name, t.Language, t.Tier, t.Difficulty, t.Description,
		strings.Join(stubFiles, ", "), strings.Join(testFiles, ", "),
		toolchainInfo(t.Language))

	// Append MCP tools section if enabled
	if useMCPTools {
		prompt += `

MCP TOOLS:
You have access to MCP tools. Carefully assess what they do and how they can be used as effectively as possible, then use them as proactively as you can wherever and whenever most suitable.`
		if mcpPrompt != "" {
			prompt += "\n\nAGENT-SPECIFIC TOOLS:\n" + mcpPrompt
		}
	}

	return prompt
}

func detectModifiedTaskFiles(loader *task.Loader, t *task.Task, workspaceDir string) ([]string, error) {
	var modified []string
	for _, filename := range append(append([]string{}, t.Files.Test...), t.Files.Support...) {
		want, err := loader.ReadTaskFile(t, filename)
		if err != nil {
			return nil, fmt.Errorf("reading canonical %s: %w", filename, err)
		}

		workspacePath := filepath.Join(workspaceDir, task.StripTxtExtension(filename))
		got, err := os.ReadFile(workspacePath)
		if err != nil {
			modified = append(modified, task.StripTxtExtension(filename))
			continue
		}
		if !bytes.Equal(got, want) {
			modified = append(modified, task.StripTxtExtension(filename))
		}
	}
	return modified, nil
}

func writeTaskFilesToWorkspace(loader *task.Loader, t *task.Task, workspaceDir string, files []string) error {
	for _, filename := range files {
		content, err := loader.ReadTaskFile(t, filename)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filename, err)
		}

		destFilename := task.StripTxtExtension(filename)
		destPath := filepath.Join(workspaceDir, destFilename)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", destFilename, err)
		}
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destFilename, err)
		}
	}
	return nil
}

// copyDirContents recursively copies all files and directories from src to dst.
// It preserves directory structure and file permissions.
func copyDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(destPath, data, info.Mode().Perm())
	})
}

// buildAgentCommand creates an exec.Cmd for the given agent configuration.
// It handles prompt placeholder substitution, model flag positioning, reasoning flag, and environment variables.
// If disableMCP is true and the agent supports it, MCP tools will be disabled via environment variables.
func buildAgentCommand(ctx context.Context, agentCfg *config.AgentConfig, prompt, model, reasoning string, disableMCP bool, agentName string) *exec.Cmd {
	var args []string

	// Determine model flag position (default to "before")
	modelPosition := agentCfg.ModelFlagPosition
	if modelPosition == "" {
		modelPosition = "before"
	}

	// Determine reasoning flag position (default to "before")
	reasoningPosition := agentCfg.ReasoningFlagPosition
	if reasoningPosition == "" {
		reasoningPosition = "before"
	}

	// Add model flag if specified (before position)
	// If ModelFlag contains {value}, substitute it; otherwise append as separate arg
	if model != "" && agentCfg.ModelFlag != "" && modelPosition == "before" {
		if strings.Contains(agentCfg.ModelFlag, "{value}") {
			// Format: "--{value}" -> "--max" (single arg with value substituted)
			args = append(args, strings.ReplaceAll(agentCfg.ModelFlag, "{value}", model))
		} else {
			// Format: "-m" "model-name" (two separate args)
			args = append(args, agentCfg.ModelFlag, model)
		}
	}

	// Add reasoning flag if specified (before position)
	// If ReasoningFlag contains {value}, substitute it; otherwise append as separate arg
	if reasoning != "" && agentCfg.ReasoningFlag != "" && reasoningPosition == "before" {
		if strings.Contains(agentCfg.ReasoningFlag, "{value}") {
			// Format: "-c key={value}" -> "-c key=high"
			args = append(args, strings.ReplaceAll(agentCfg.ReasoningFlag, "{value}", reasoning))
		} else {
			// Format: "-r" "high"
			args = append(args, agentCfg.ReasoningFlag, reasoning)
		}
	}

	// Process args, replacing {prompt} placeholder
	for _, arg := range agentCfg.Args {
		if arg == "{prompt}" {
			args = append(args, prompt)
		} else {
			args = append(args, arg)
		}
	}

	// Add model flag if specified (after position)
	// If ModelFlag contains {value}, substitute it; otherwise append as separate arg
	if model != "" && agentCfg.ModelFlag != "" && modelPosition == "after" {
		if strings.Contains(agentCfg.ModelFlag, "{value}") {
			// Format: "--{value}" -> "--max" (single arg with value substituted)
			args = append(args, strings.ReplaceAll(agentCfg.ModelFlag, "{value}", model))
		} else {
			// Format: "-m" "model-name" (two separate args)
			args = append(args, agentCfg.ModelFlag, model)
		}
	}

	// Add reasoning flag if specified (after position)
	// If ReasoningFlag contains {value}, substitute it; otherwise append as separate arg
	if reasoning != "" && agentCfg.ReasoningFlag != "" && reasoningPosition == "after" {
		if strings.Contains(agentCfg.ReasoningFlag, "{value}") {
			// Format: "-c key={value}" -> "-c key=high"
			args = append(args, strings.ReplaceAll(agentCfg.ReasoningFlag, "{value}", reasoning))
		} else {
			// Format: "-r" "high"
			args = append(args, agentCfg.ReasoningFlag, reasoning)
		}
	}

	cmd := exec.CommandContext(ctx, agentCfg.Command, args...)
	cmd.Env = buildAgentEnv(agentCfg.Env, disableMCP, agentName)

	return cmd
}

// wrapCommandWithSandbox wraps an exec.Cmd in a bubblewrap sandbox.
// The sandbox restricts filesystem access so the agent can only write to the
// workspace directory and /tmp. The rest of the filesystem (including $HOME)
// is mounted read-only. Network access is preserved for LLM API calls.
func wrapCommandWithSandbox(ctx context.Context, cmd *exec.Cmd, extraWritableDirs []string) *exec.Cmd {
	bwrapArgs := buildSandboxArgs(cmd.Dir, extraWritableDirs)
	bwrapArgs = append(bwrapArgs, "--", cmd.Path)
	bwrapArgs = append(bwrapArgs, cmd.Args[1:]...)

	wrapped := exec.CommandContext(ctx, "bwrap", bwrapArgs...)
	wrapped.Env = cmd.Env
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr

	return wrapped
}

// buildSandboxArgs constructs the bubblewrap arguments for filesystem isolation.
func buildSandboxArgs(workspaceDir string, extraWritableDirs []string) []string {
	homeDir, _ := os.UserHomeDir()

	var args []string

	// Mount system directories read-only.
	for _, dir := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/run"} {
		if _, err := os.Stat(dir); err == nil {
			args = append(args, "--ro-bind", dir, dir)
		}
	}

	// Mount $HOME read-only so agents can access their own configs and binaries.
	args = append(args, "--ro-bind", homeDir, homeDir)

	// Allow agents to write to all dot-directories (hidden dirs) under $HOME.
	// Instead of maintaining a per-agent whitelist, we enumerate existing
	// dot-directories and mount them all writable. This covers XDG dirs
	// (.cache, .config, .local), language toolchains (.npm, .cargo, .rustup),
	// and any agent-specific directories (.claude, .gemini, .junie, etc.)
	// without needing updates when new agents are added.
	// Non-dot directories like "go" must still be listed explicitly.
	explicitWritableDirs := make([]string, 0, 1+len(extraWritableDirs))
	explicitWritableDirs = append(explicitWritableDirs, "go")

	// Merge user-configured writable dirs from [sandbox] config.
	explicitWritableDirs = append(explicitWritableDirs, extraWritableDirs...)

	// Mount all existing dot-directories under $HOME as writable.
	if entries, err := os.ReadDir(homeDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), ".") {
				absDir := filepath.Join(homeDir, e.Name())
				args = append(args, "--bind", absDir, absDir)
			}
		}
	}

	// Mount explicitly listed non-dot directories.
	for _, dir := range explicitWritableDirs {
		absDir := filepath.Join(homeDir, dir)
		if _, err := os.Stat(absDir); err == nil {
			args = append(args, "--bind", absDir, absDir)
		}
	}

	// /tmp for temporary files.
	args = append(args, "--tmpfs", "/tmp")

	// Workspace is the only persistent writable directory outside $HOME.
	// Mounted after --tmpfs /tmp so it takes precedence when the workspace
	// is inside /tmp (which it is during eval for isolation).
	args = append(args, "--bind", workspaceDir, workspaceDir)

	// Required virtual filesystems.
	args = append(args, "--dev", "/dev")
	args = append(args, "--proc", "/proc")

	// Namespace isolation: new mount/pid/user/ipc/uts namespaces, but keep network.
	args = append(args, "--unshare-all", "--share-net")

	// Kill agent if harness dies.
	args = append(args, "--die-with-parent")

	// Set working directory to workspace.
	args = append(args, "--chdir", workspaceDir)

	return args
}

// getOpenCodeConfigPaths returns the possible paths for the OpenCode config file.
// It checks XDG_CONFIG_HOME first, then falls back to ~/.config/opencode/.
func getOpenCodeConfigPaths() []string {
	var paths []string

	// Check XDG_CONFIG_HOME first (follows XDG Base Directory spec)
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		paths = append(paths,
			filepath.Join(xdgConfig, "opencode", "opencode.jsonc"),
			filepath.Join(xdgConfig, "opencode", "opencode.json"),
		)
	}

	// Fall back to ~/.config/opencode/
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc"),
			filepath.Join(homeDir, ".config", "opencode", "opencode.json"),
		)
	}

	return paths
}

// readOpenCodeConfig reads and parses the OpenCode config file.
// Returns nil if the config file doesn't exist or can't be parsed.
func readOpenCodeConfig() map[string]any {
	for _, configPath := range getOpenCodeConfigPaths() {
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue // File doesn't exist or can't be read
		}

		// Strip JSONC comments (// and /* */) for parsing
		// This is a simple approach that handles most cases
		data = stripJSONComments(data)

		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			continue // Invalid JSON, try next path
		}

		return config
	}

	return nil
}

// stripJSONComments removes single-line (//) and multi-line (/* */) comments from JSON.
// This allows parsing JSONC (JSON with Comments) files.
//
//nolint:gocognit // State machine for parsing requires multiple conditionals
func stripJSONComments(data []byte) []byte {
	var result bytes.Buffer
	inString := false
	inSingleComment := false
	inMultiComment := false

	for i := 0; i < len(data); i++ {
		c := data[i]

		// Handle string context (don't strip comments inside strings)
		if !inSingleComment && !inMultiComment {
			if c == '"' && (i == 0 || data[i-1] != '\\') {
				inString = !inString
			}
		}

		if inString {
			result.WriteByte(c)
			continue
		}

		// Handle single-line comments
		if inSingleComment {
			if c == '\n' {
				inSingleComment = false
				result.WriteByte(c) // Keep newline for line counting
			}
			continue
		}

		// Handle multi-line comments
		if inMultiComment {
			if c == '*' && i+1 < len(data) && data[i+1] == '/' {
				inMultiComment = false
				i++ // Skip the '/'
			}
			continue
		}

		// Check for comment start
		if c == '/' && i+1 < len(data) {
			if data[i+1] == '/' {
				inSingleComment = true
				i++ // Skip the second '/'
				continue
			}
			if data[i+1] == '*' {
				inMultiComment = true
				i++ // Skip the '*'
				continue
			}
		}

		result.WriteByte(c)
	}

	return result.Bytes()
}

// deepMergeJSON performs a deep merge of two JSON-like maps.
// Values from 'override' take precedence over 'base'.
// Nested maps are merged recursively; other values are replaced.
func deepMergeJSON(base, override map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}

	result := make(map[string]any)

	// Copy base values
	for k, v := range base {
		result[k] = v
	}

	// Merge override values
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, merge recursively
			baseMap, baseIsMap := baseVal.(map[string]any)
			overrideMap, overrideIsMap := v.(map[string]any)
			if baseIsMap && overrideIsMap {
				result[k] = deepMergeJSON(baseMap, overrideMap)
				continue
			}
		}
		// Otherwise, override takes precedence
		result[k] = v
	}

	return result
}

// buildOpenCodeMCPDisableConfig creates the OPENCODE_CONFIG_CONTENT value
// by merging the user's existing config with the MCP disable settings.
func buildOpenCodeMCPDisableConfig() string {
	// The override config that disables all MCP tools
	// MCP tools are registered as "servername_toolname", so "*_*" matches all
	mcpDisable := map[string]any{
		"tools": map[string]any{
			"*_*": false,
		},
	}

	// Try to read the user's existing config
	userConfig := readOpenCodeConfig()

	var finalConfig map[string]any
	if userConfig != nil {
		// Merge user config with MCP disable (MCP disable takes precedence)
		finalConfig = deepMergeJSON(userConfig, mcpDisable)
	} else {
		// No user config found, just use MCP disable
		finalConfig = mcpDisable
	}

	// Serialize to JSON
	data, err := json.Marshal(finalConfig)
	if err != nil {
		// Fallback to minimal config if marshaling fails
		return `{"tools":{"*_*":false}}`
	}

	return string(data)
}

// buildAgentEnv creates the environment variable slice for an agent command.
// It merges the agent's configured env vars with any runtime injections (like MCP disable).
func buildAgentEnv(agentEnv map[string]string, disableMCP bool, agentName string) []string {
	if len(agentEnv) == 0 && !disableMCP {
		return nil
	}

	env := os.Environ()
	for k, v := range agentEnv {
		env = append(env, k+"="+v)
	}

	// Inject MCP disable config for OpenCode
	// Merges user's existing config with the MCP disable settings to preserve
	// custom models, plugins, and other configuration while disabling MCP tools
	if disableMCP && agentName == "opencode" {
		configContent := buildOpenCodeMCPDisableConfig()
		env = append(env, "OPENCODE_CONFIG_CONTENT="+configContent)
	}

	return env
}

// EvalAttestation provides cryptographic verification of eval results.
type EvalAttestation struct {
	Version   string                     `json:"version"`
	Harness   AttestationHarness         `json:"harness"`
	Eval      AttestationEval            `json:"eval"`
	Tasks     map[string]AttestationTask `json:"tasks"`
	Integrity AttestationIntegrity       `json:"integrity"`
}

// AttestationHarness contains harness version information.
type AttestationHarness struct {
	Version       string `json:"version"`
	BuildDate     string `json:"build_date"`
	WeightVersion string `json:"weight_version,omitempty"`
}

// AttestationEval contains evaluation metadata.
type AttestationEval struct {
	Agent     string  `json:"agent"`
	Model     string  `json:"model,omitempty"`
	Timestamp string  `json:"timestamp"`
	Duration  float64 `json:"duration_seconds"`
}

// AttestationTask contains per-task verification data.
type AttestationTask struct {
	TaskHash     string  `json:"task_hash"`
	SolutionHash string  `json:"solution_hash,omitempty"`
	Passed       bool    `json:"passed"`
	Duration     float64 `json:"duration_seconds"`
}

// AttestationIntegrity contains aggregate hashes for verification.
type AttestationIntegrity struct {
	TasksHash   string `json:"tasks_hash"`
	ResultsHash string `json:"results_hash"`
}

// hashBytes returns the BLAKE3 hash of data as a prefixed hex string.
func hashBytes(data []byte) string {
	h := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(h[:])
}

// hashFiles returns the BLAKE3 hash of multiple files concatenated.
func hashFiles(paths []string) (string, error) {
	hasher := blake3.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip missing files
		}
		_, _ = hasher.Write(data)
	}
	sum := hasher.Sum(nil)
	return "blake3:" + hex.EncodeToString(sum), nil
}

// generateAttestation creates an attestation for the eval run.
// newlyRunTasks contains task IDs that were executed in this session.
// previousTasks contains attestation data from a previous run (for resume).
func generateAttestation(
	agent, model, timestamp string,
	totalDuration float64,
	results []EvalResult,
	outputDir string,
	loader *task.Loader,
	allTasks []*task.Task,
	newlyRunTasks map[string]bool,
	previousTasks map[string]AttestationTask,
) (*EvalAttestation, error) {
	attestation := &EvalAttestation{
		Version: "1",
		Harness: AttestationHarness{
			Version:       Version,
			BuildDate:     BuildDate,
			WeightVersion: task.WeightVersion,
		},
		Eval: AttestationEval{
			Agent:     agent,
			Model:     model,
			Timestamp: timestamp,
			Duration:  totalDuration,
		},
		Tasks: make(map[string]AttestationTask),
	}

	// Build a map of tasks by ID for quick lookup
	taskMap := make(map[string]*task.Task)
	for _, t := range allTasks {
		taskMap[t.ID()] = t
	}

	// Compute task hashes
	var allTaskHashes []byte
	for _, r := range results {
		t := taskMap[r.Task]
		if t == nil {
			continue
		}

		// If this task was NOT newly run and we have previous attestation data, use it.
		if !newlyRunTasks[r.Task] {
			if prev, ok := previousTasks[r.Task]; ok && prev.SolutionHash != "" {
				attestation.Tasks[r.Task] = prev
				allTaskHashes = append(allTaskHashes, []byte(prev.TaskHash)...)
				continue
			}
		}

		// Hash task files (stub + test + support)
		var taskFileContents []byte
		for _, f := range append(append(t.Files.Stub, t.Files.Test...), t.Files.Support...) {
			if content, err := loader.ReadTaskFile(t, f); err == nil {
				taskFileContents = append(taskFileContents, content...)
			}
		}
		taskHash := hashBytes(taskFileContents)

		// Hash solution files if they exist
		var solutionHash string
		workspaceDir := filepath.Join(outputDir, fmt.Sprintf("%s-%s", t.Language, t.Slug))
		solutionPaths := make([]string, 0, len(t.Files.Stub))
		for _, f := range t.Files.Stub {
			solutionPaths = append(solutionPaths, filepath.Join(workspaceDir, task.StripTxtExtension(f)))
		}
		if hash, err := hashFiles(solutionPaths); err == nil {
			solutionHash = hash
		}

		attestation.Tasks[r.Task] = AttestationTask{
			TaskHash:     taskHash,
			SolutionHash: solutionHash,
			Passed:       r.Passed,
			Duration:     r.Duration,
		}

		allTaskHashes = append(allTaskHashes, []byte(taskHash)...)
	}

	// Compute integrity hashes
	attestation.Integrity.TasksHash = hashBytes(allTaskHashes)

	// Hash the results JSON
	resultsJSON, _ := json.Marshal(results)
	attestation.Integrity.ResultsHash = hashBytes(resultsJSON)

	return attestation, nil
}

// LeaderboardSubmission is a compact format for submitting results to a leaderboard website.
type LeaderboardSubmission struct {
	// Identity
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Timestamp string `json:"timestamp"`

	// Core metrics
	PassRate         float64 `json:"pass_rate"`
	WeightedPassRate float64 `json:"weighted_pass_rate"`
	Passed           int     `json:"passed"`
	Failed           int     `json:"failed"`
	Total            int     `json:"total"`

	// Weighted scoring
	WeightedScore    float64 `json:"weighted_score"`
	MaxPossibleScore float64 `json:"max_possible_score"`

	// Quality metrics
	IntegrityViolations int `json:"integrity_violations"`

	// Per-language breakdown
	ByLanguage map[string]LeaderboardLanguageStats `json:"by_language"`

	// Timing
	TotalDurationSec float64 `json:"total_duration_seconds"`
	AgentDurationSec float64 `json:"agent_duration_seconds"`

	// Verification
	HarnessVersion string `json:"harness_version"`
	WeightVersion  string `json:"weight_version"`
	TasksHash      string `json:"tasks_hash"`
	ResultsHash    string `json:"results_hash"`

	// Configuration
	UseMCPTools bool `json:"use_mcp_tools,omitempty"`
	DisableMCP  bool `json:"disable_mcp,omitempty"`
	Sandbox     bool `json:"sandbox,omitempty"`
	Legacy      bool `json:"legacy,omitempty"`
}

// LeaderboardLanguageStats contains per-language metrics for the leaderboard.
type LeaderboardLanguageStats struct {
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Total    int     `json:"total"`
	PassRate float64 `json:"pass_rate"`
}

// generateLeaderboardSubmission creates a compact submission file for leaderboard websites.
func generateLeaderboardSubmission(summary EvalSummary, attestation *EvalAttestation) LeaderboardSubmission {
	submission := LeaderboardSubmission{
		Agent:               summary.Agent,
		Model:               summary.Model,
		Reasoning:           summary.Reasoning,
		Timestamp:           summary.Timestamp,
		PassRate:            summary.PassRate,
		WeightedPassRate:    summary.WeightedPassRate,
		Passed:              summary.Passed,
		Failed:              summary.Failed,
		Total:               summary.Total,
		WeightedScore:       summary.WeightedScore,
		MaxPossibleScore:    summary.MaxPossibleScore,
		IntegrityViolations: summary.IntegrityViolations,
		TotalDurationSec:    summary.Duration,
		AgentDurationSec:    summary.AgentTime,
		ByLanguage:          make(map[string]LeaderboardLanguageStats),
	}

	// Add verification data from attestation
	if attestation != nil {
		submission.HarnessVersion = attestation.Harness.Version
		submission.WeightVersion = attestation.Harness.WeightVersion
		submission.TasksHash = attestation.Integrity.TasksHash
		submission.ResultsHash = attestation.Integrity.ResultsHash
	}

	// Add configuration flags
	submission.UseMCPTools = summary.UseMCPTools
	submission.DisableMCP = summary.DisableMCP
	submission.Sandbox = summary.Sandbox
	submission.Legacy = summary.Legacy

	// Convert language stats
	for lang, agg := range summary.ByLanguage {
		submission.ByLanguage[lang] = LeaderboardLanguageStats{
			Passed:   agg.Passed,
			Failed:   agg.Failed,
			Total:    agg.Total,
			PassRate: agg.PassRate,
		}
	}

	return submission
}

// generateEvalReport creates a human-readable Markdown report for the evaluation.
func generateEvalReport(summary EvalSummary, attestation *EvalAttestation) string {
	var sb strings.Builder

	sb.WriteString("# Evaluation Report\n\n")
	writeReportSummary(&sb, summary)
	writeReportQuality(&sb, summary)
	writeReportByLanguage(&sb, summary)
	writeReportByTier(&sb, summary)
	writeReportTaskResults(&sb, summary)
	writeReportErrors(&sb, summary)
	writeReportVerification(&sb, attestation)
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("*Generated by SanityHarness on %s*\n", summary.Timestamp))

	return sb.String()
}

func writeReportSummary(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	fmt.Fprintf(sb, "| Agent | **%s** |\n", summary.Agent)
	if summary.Model != "" {
		fmt.Fprintf(sb, "| Model | %s |\n", summary.Model)
	}
	if summary.Reasoning != "" {
		fmt.Fprintf(sb, "| Reasoning Effort | %s |\n", summary.Reasoning)
	}
	if summary.UseMCPTools {
		sb.WriteString("| MCP Tools Mode | Yes |\n")
	}
	if summary.DisableMCP {
		sb.WriteString("| MCP Disabled | Yes |\n")
	}
	if summary.Sandbox {
		sb.WriteString("| Sandbox | Yes |\n")
	}
	if summary.Legacy {
		sb.WriteString("| Legacy Mode | Yes |\n")
	}
	fmt.Fprintf(sb, "| Timestamp | %s |\n", summary.Timestamp)
	fmt.Fprintf(sb, "| Pass Rate | **%.1f%%** (%d/%d) |\n", summary.PassRate, summary.Passed, summary.Total)
	fmt.Fprintf(sb, "| Weighted Pass Rate | **%.1f%%** |\n", summary.WeightedPassRate)
	fmt.Fprintf(sb, "| Weighted Score | %.2f / %.2f |\n", summary.WeightedScore, summary.MaxPossibleScore)
	fmt.Fprintf(sb, "| Duration | %.1fs |\n", summary.Duration)
	sb.WriteString("\n")
}

func writeReportQuality(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Quality Breakdown\n\n")
	fmt.Fprintf(sb, "- **Integrity Violations** (modified test files): %d\n", summary.IntegrityViolations)
	fmt.Fprintf(sb, "- **Failures**: %d\n", summary.Failed-summary.IntegrityViolations)
	sb.WriteString("\n")
}

func writeReportByLanguage(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Results by Language\n\n")
	sb.WriteString("| Language | Passed | Failed | Total | Pass Rate |\n")
	sb.WriteString("|----------|--------|--------|-------|-----------|\n")
	languages := make([]string, 0, len(summary.ByLanguage))
	for lang := range summary.ByLanguage {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	for _, lang := range languages {
		agg := summary.ByLanguage[lang]
		fmt.Fprintf(sb, "| %s | %d | %d | %d | %.1f%% |\n",
			lang, agg.Passed, agg.Failed, agg.Total, agg.PassRate)
	}
	sb.WriteString("\n")
}

func writeReportByTier(sb *strings.Builder, summary EvalSummary) {
	if len(summary.ByTier) == 0 {
		return
	}
	sb.WriteString("## Results by Tier\n\n")
	sb.WriteString("| Tier | Passed | Failed | Total | Pass Rate |\n")
	sb.WriteString("|------|--------|--------|-------|-----------|\n")
	for _, tier := range []string{"core", "extended"} {
		if agg, ok := summary.ByTier[tier]; ok {
			fmt.Fprintf(sb, "| %s | %d | %d | %d | %.1f%% |\n",
				tier, agg.Passed, agg.Failed, agg.Total, agg.PassRate)
		}
	}
	sb.WriteString("\n")
}

func writeReportTaskResults(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Task Results\n\n")
	sb.WriteString("| Task | Status | Weight | Score | Duration |\n")
	sb.WriteString("|------|--------|--------|-------|----------|\n")
	for _, r := range summary.Results {
		statusIcon, status := getResultStatusDisplay(r)
		fmt.Fprintf(sb, "| %s | %s %s | %.2f | %.2f | %.1fs |\n",
			r.Task, statusIcon, status, r.Weight, r.WeightedScore, r.Duration)
	}
	sb.WriteString("\n")
}

func getResultStatusDisplay(r EvalResult) (icon, text string) {
	switch {
	case r.Status == task.StatusIntegrityViolation:
		return "🚫", "VIOLATION"
	case r.Passed:
		return "✅", "PASS"
	default:
		return "❌", "FAIL"
	}
}

func writeReportErrors(sb *strings.Builder, summary EvalSummary) {
	hasErrors := false
	for _, r := range summary.Results {
		if r.Error != "" {
			hasErrors = true
			break
		}
	}
	if !hasErrors {
		return
	}
	sb.WriteString("## Errors\n\n")
	for _, r := range summary.Results {
		if r.Error != "" {
			fmt.Fprintf(sb, "### %s\n\n", r.Task)
			fmt.Fprintf(sb, "```\n%s\n```\n\n", r.Error)
		}
	}
}

func writeReportVerification(sb *strings.Builder, attestation *EvalAttestation) {
	if attestation == nil {
		return
	}
	sb.WriteString("## Verification\n\n")
	fmt.Fprintf(sb, "- **Harness Version**: %s\n", attestation.Harness.Version)
	fmt.Fprintf(sb, "- **Weight Version**: %s\n", attestation.Harness.WeightVersion)
	fmt.Fprintf(sb, "- **Tasks Hash**: `%s`\n", attestation.Integrity.TasksHash)
	fmt.Fprintf(sb, "- **Results Hash**: `%s`\n", attestation.Integrity.ResultsHash)
	sb.WriteString("\n")
}

// detectQuotaError checks if agent log contains rate limit or quota errors.
// Returns (hasError, isRecoverable) where hasError indicates if any quota/rate
// limit pattern was found, and isRecoverable indicates if the error is transient.
func detectQuotaError(logPath string) (bool, bool) {
	content, err := os.ReadFile(logPath)
	if err != nil {
		return false, false
	}

	lower := strings.ToLower(string(content))

	// Check for non-recoverable patterns first
	for _, pattern := range nonRecoverablePatterns {
		if strings.Contains(lower, pattern) {
			return true, false // Error found, NOT recoverable
		}
	}

	// Check for recoverable patterns
	for _, pattern := range recoverablePatterns {
		if strings.Contains(lower, pattern) {
			return true, true // Error found, IS recoverable
		}
	}

	return false, false
}

// getRetryDelay returns the delay for the given quota retry attempt (1-indexed).
func getRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return quotaRetryDelay1
	case 2:
		return quotaRetryDelay2
	case 3:
		return quotaRetryDelay3
	case 4:
		return quotaRetryDelay4
	default:
		return quotaRetryDelay5
	}
}

// getInfraRetryDelay returns the delay for the given infra retry attempt (1-indexed).
func getInfraRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return infraRetryDelay1
	case 2:
		return infraRetryDelay2
	case 3:
		return infraRetryDelay3
	case 4:
		return infraRetryDelay4
	default:
		return infraRetryDelay5
	}
}

// isInfraFailure checks if the agent log indicates an infrastructure failure
// (empty or near-empty output suggesting the provider never responded).
// It strips retry separator lines and whitespace to avoid false negatives
// when the log contains only retry markers but no actual agent output.
// If workspaceDir is non-empty, it also checks whether the agent modified any
// files in the workspace — agents that write to files but produce no stdout
// (e.g. droid, cline) are NOT infra failures.
func isInfraFailure(logPath, workspaceDir string, workspaceReadyAt time.Time) bool {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return true // No log file at all is an infra failure
	}

	// Strip retry separator lines (e.g., "=== RETRY 1 (after 30s delay) ===")
	// and whitespace to check if there's any real agent output.
	var meaningful []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("=== RETRY ")) && bytes.HasSuffix(trimmed, []byte("===")) {
			continue
		}
		meaningful = append(meaningful, trimmed...)
	}

	if len(meaningful) >= infraFailureLogThreshold {
		return false // Agent produced meaningful output
	}

	// Agent produced no/minimal stdout. Check if it modified workspace files.
	// Some agents (droid, cline) legitimately produce no stdout but write files.
	if workspaceDir != "" && hasModifiedFiles(workspaceDir, workspaceReadyAt) {
		return false // Agent wrote files — not an infra failure
	}

	return true
}

// hasModifiedFiles checks if any files in the workspace directory were modified
// after the given cutoff time. This detects agents that write to files but
// produce no stdout output, while ignoring files written by the harness during
// workspace setup.
func hasModifiedFiles(dir string, cutoff time.Time) bool {
	found := false

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip agent.log — it is created by the harness (not the agent) inside
		// the workspace dir, so its presence does not indicate agent activity.
		if filepath.Base(path) == "agent.log" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})

	return found
}

// saveRunConfig saves the eval configuration for resume capability.
func saveRunConfig(outputDir string, allTasks []*task.Task) error {
	taskList := make([]string, len(allTasks))
	for i, t := range allTasks {
		taskList[i] = string(t.Language) + "/" + t.Slug
	}

	runCfg := RunConfig{
		Agent:          evalAgent,
		Model:          evalModel,
		Reasoning:      evalReasoning,
		Tier:           evalTier,
		Difficulty:     evalDifficulty,
		Lang:           evalLang,
		Tasks:          evalTasks,
		Timeout:        evalTimeout,
		Parallel:       evalParallel,
		UseMCPTools:    evalUseMCPTools,
		DisableMCP:     evalDisableMCP,
		NoSandbox:      evalNoSandbox,
		Legacy:         evalLegacy,
		KeepWorkspaces: evalKeepWorkspaces,
		TaskList:       taskList,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(runCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling run config: %w", err)
	}

	return os.WriteFile(filepath.Join(outputDir, "run-config.json"), data, 0o644)
}

// loadRunConfig loads the eval configuration from a resume directory.
func loadRunConfig(resumeDir string) (*RunConfig, error) {
	data, err := os.ReadFile(filepath.Join(resumeDir, "run-config.json"))
	if err != nil {
		return nil, fmt.Errorf("reading run config: %w", err)
	}

	var runCfg RunConfig
	if err := json.Unmarshal(data, &runCfg); err != nil {
		return nil, fmt.Errorf("parsing run config: %w", err)
	}

	return &runCfg, nil
}

// applyRunConfig applies the loaded run config to global eval variables.
func applyRunConfig(runCfg *RunConfig) {
	evalAgent = runCfg.Agent
	evalModel = runCfg.Model
	evalReasoning = runCfg.Reasoning
	evalTier = runCfg.Tier
	evalDifficulty = runCfg.Difficulty
	evalLang = runCfg.Lang
	evalTasks = runCfg.Tasks
	evalTimeout = runCfg.Timeout
	evalParallel = runCfg.Parallel
	evalUseMCPTools = runCfg.UseMCPTools
	evalDisableMCP = runCfg.DisableMCP
	evalNoSandbox = runCfg.NoSandbox
	evalLegacy = runCfg.Legacy
	evalKeepWorkspaces = runCfg.KeepWorkspaces
}

// findCompletedTasks returns a set of task slugs that have validation.log files.
func findCompletedTasks(outputDir string) (map[string]bool, error) {
	completed := make(map[string]bool)

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("reading output directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if validation.log exists in this task directory.
		validationLog := filepath.Join(outputDir, entry.Name(), "validation.log")
		if _, err := os.Stat(validationLog); err == nil {
			// Directory name format is "language-slug", convert to "language/slug".
			name := entry.Name()
			if idx := strings.Index(name, "-"); idx > 0 {
				taskSlug := name[:idx] + "/" + name[idx+1:]
				completed[taskSlug] = true
			}
		}
	}

	return completed, nil
}

// loadPreviousSummary loads results from a previous eval run for merging.
func loadPreviousSummary(outputDir string) (*EvalSummary, error) {
	summaryPath := filepath.Join(outputDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No previous results.
		}
		return nil, fmt.Errorf("reading summary: %w", err)
	}

	var summary EvalSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("parsing summary: %w", err)
	}

	return &summary, nil
}

// loadPreviousAttestation loads attestation from a previous eval run.
func loadPreviousAttestation(outputDir string) (*EvalAttestation, error) {
	attestationPath := filepath.Join(outputDir, "attestation.json")
	data, err := os.ReadFile(attestationPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No previous attestation.
		}
		return nil, fmt.Errorf("reading attestation: %w", err)
	}

	var attestation EvalAttestation
	if err := json.Unmarshal(data, &attestation); err != nil {
		return nil, fmt.Errorf("parsing attestation: %w", err)
	}

	return &attestation, nil
}

// cleanIncompleteTaskDirs removes task directories that don't have validation.log.
func cleanIncompleteTaskDirs(outputDir string, completed map[string]bool, allTasks []*task.Task) error {
	for _, t := range allTasks {
		taskSlug := string(t.Language) + "/" + t.Slug
		if completed[taskSlug] {
			continue
		}

		// Task is incomplete, remove its directory if it exists.
		taskDir := filepath.Join(outputDir, string(t.Language)+"-"+t.Slug)
		if _, err := os.Stat(taskDir); err == nil {
			if err := os.RemoveAll(taskDir); err != nil {
				return fmt.Errorf("removing incomplete task dir %s: %w", taskDir, err)
			}
		}
	}

	return nil
}

// setupInterruptHandler creates a context that is cancelled on interrupt signals.
// The returned cancel function should be deferred to clean up signal handling.
func setupInterruptHandler() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()
	return ctx, cancel
}

// checkInterrupted checks if an interrupt signal has been received.
func checkInterrupted(ctx context.Context) bool {
	return ctx.Err() != nil
}

// printResumeCommand prints the command to resume an interrupted eval.
func printResumeCommand(outputDir string) {
	fmt.Printf("\n\033[33m⚠ Evaluation interrupted. To resume, run:\033[0m\n")
	fmt.Printf("  ./sanity eval --resume %s\n\n", outputDir)
}

// initSandbox checks if bubblewrap sandboxing should be enabled.
// Returns true if sandbox is active (bwrap found and not disabled).
func initSandbox() bool {
	if evalNoSandbox {
		logger.Info("sandbox disabled via --no-sandbox")
		return false
	}

	if _, err := exec.LookPath("bwrap"); err != nil {
		logger.Warn("bubblewrap (bwrap) not found, running agents without sandbox")
		return false
	}

	return true
}

// protectTasksDir makes the tasks/ directory read-only to prevent agents from
// modifying embedded task source files during evaluation. Returns a restore
// function that re-enables write permissions, or nil if protection was not needed.
func protectTasksDir() (restore func(), err error) {
	tasksPath := "tasks"
	info, err := os.Stat(tasksPath)
	if err != nil || !info.IsDir() {
		return nil, nil // No on-disk tasks/ directory; nothing to protect
	}

	absTasksPath, err := filepath.Abs(tasksPath)
	if err != nil {
		return nil, fmt.Errorf("resolving tasks path: %w", err)
	}

	// Collect original permissions so we can restore them exactly.
	type entry struct {
		path string
		mode fs.FileMode
	}
	var origPerms []entry

	err = filepath.WalkDir(absTasksPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		origPerms = append(origPerms, entry{path: path, mode: info.Mode()})

		// Remove write bits for user, group, and other.
		newMode := info.Mode() &^ 0222
		if newMode != info.Mode() {
			return os.Chmod(path, newMode)
		}
		return nil
	})
	if err != nil {
		// Best-effort: try to restore anything we already changed.
		for _, e := range origPerms {
			_ = os.Chmod(e.path, e.mode)
		}
		return nil, fmt.Errorf("protecting tasks directory: %w", err)
	}

	restore = func() {
		// Restore in reverse order so directories are restored after their contents.
		for i := len(origPerms) - 1; i >= 0; i-- {
			_ = os.Chmod(origPerms[i].path, origPerms[i].mode)
		}
	}
	return restore, nil
}

func init() {
	evalCmd.Flags().StringVar(&evalAgent, "agent", "", "agent to evaluate (see --help for list)")
	evalCmd.Flags().StringVar(&evalModel, "model", "", "model to use (e.g., gemini-2.5-pro or google/gemini-2.5-flash)")
	evalCmd.Flags().StringVar(&evalReasoning, "reasoning", "", "reasoning effort level (e.g., off, none, low, medium, high)")
	evalCmd.Flags().StringVar(&evalTasks, "tasks", "", "comma-separated list of task slugs")
	evalCmd.Flags().StringVar(&evalLang, "lang", "", "filter by language (go, rust, typescript)")
	evalCmd.Flags().StringVar(&evalTier, "tier", "core", "filter by tier (core, extended, all)")
	evalCmd.Flags().StringVar(&evalDifficulty, "difficulty", "", "filter by difficulty (comma-separated)")
	evalCmd.Flags().IntVar(&evalTimeout, "timeout", 0, "timeout per task in seconds (default from config)")
	evalCmd.Flags().IntVar(&evalParallel, "parallel", 1, "run up to N tasks in parallel")
	evalCmd.Flags().StringVar(&evalOutputDir, "output", "", "output directory for results")
	evalCmd.Flags().BoolVar(&evalKeepWorkspaces, "keep-workspaces", false, "keep workspace directories after evaluation")
	evalCmd.Flags().BoolVar(&evalDryRun, "dry-run", false, "show what tasks would be run without executing")
	evalCmd.Flags().BoolVar(&evalUseMCPTools, "use-mcp-tools", false, "inject MCP tool usage instructions into agent prompt")
	evalCmd.Flags().BoolVar(&evalDisableMCP, "disable-mcp", false, "disable MCP tools for agents that support it (currently: opencode)")
	evalCmd.Flags().BoolVar(&evalNoSandbox, "no-sandbox", false, "disable bubblewrap sandbox for agent processes")
	evalCmd.Flags().BoolVar(&evalLegacy, "legacy", false, "expose hidden tests to agent during workspace init (pre-v1.6.0 behavior)")
	evalCmd.Flags().StringVar(&evalResume, "resume", "", "resume eval from existing output directory")
	evalCmd.Flags().IntVar(&evalRepeat, "repeat", 1, "repeat each configuration N times for statistical analysis")
}
