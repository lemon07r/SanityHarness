package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	evalResume         string
)

// Quota retry configuration.
const (
	quotaMaxRetries  = 3
	quotaRetryDelay1 = 10 * time.Second
	quotaRetryDelay2 = 30 * time.Second
	quotaRetryDelay3 = 60 * time.Second

	// Stop eval early after this many consecutive quota-exhausted tasks.
	// This allows resuming later when quota resets, rather than wasting time
	// on tasks that will all fail immediately.
	quotaExhaustedStopThreshold = 3
)

// Patterns indicating recoverable rate limit errors (worth retrying).
var recoverablePatterns = []string{
	"429",
	"rate limit",
	"rate_limit",
	"ratelimit",
	"too many requests",
	"overload",
	"503",
	"502",
	"529",
	"temporarily unavailable",
	"try again later",
	"service unavailable",
}

// Patterns indicating non-recoverable quota errors (skip retries).
var nonRecoverablePatterns = []string{
	"reset after",
	"monthly",
	"billing",
	"authentication failed",
	"unauthorized",
	"forbidden",
	"invalid api key",
	"exhausted your capacity",
	"exceeded your current quota",
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
	CleanPasses         int                      `json:"clean_passes,omitempty"`
	PartialPasses       int                      `json:"partial_passes,omitempty"`
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
	QuotaAffectedTasks  int                      `json:"quota_affected_tasks,omitempty"`
	TotalQuotaRetries   int                      `json:"total_quota_retries,omitempty"`
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
		// Track if we're resuming a previous run.
		var isResuming bool
		var previousResults []EvalResult
		var completedTasks map[string]bool
		var runCfg *RunConfig
		var timestamp string

		// Handle resume mode: load config and apply settings.
		var prevAttestation *EvalAttestation
		if evalResume != "" {
			var err error
			runCfg, err = loadRunConfig(evalResume)
			if err != nil {
				return fmt.Errorf("loading resume config: %w", err)
			}
			applyRunConfig(runCfg)
			evalOutputDir = evalResume
			isResuming = true

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

		// Dry-run mode doesn't require agent to be installed
		if !evalDryRun {
			if evalAgent == "" {
				return fmt.Errorf("--agent is required (use --help to see available agents)")
			}

			// Validate agent exists in config
			agentCfg := cfg.GetAgent(evalAgent)
			if agentCfg == nil {
				available := strings.Join(cfg.ListAgents(), ", ")
				return fmt.Errorf("unknown agent: %s (available: %s)", evalAgent, available)
			}

			// Check agent binary is installed
			if _, err := exec.LookPath(agentCfg.Command); err != nil {
				return fmt.Errorf("agent %q binary %q not found in PATH", evalAgent, agentCfg.Command)
			}
		}

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		// If the user specified another selector, default tier should not hide tasks.
		tierChanged := cmd.Flags().Changed("tier")
		if !tierChanged && (evalLang != "" || evalTasks != "" || evalDifficulty != "") {
			evalTier = "all"
		}

		switch evalTier {
		case "", "core", "extended", "all":
			// OK
		default:
			return fmt.Errorf("invalid --tier %q (valid: core, extended, all)", evalTier)
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
		if evalDryRun {
			fmt.Println()
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println(" SANITY HARNESS - Dry Run")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println()
			if evalAgent != "" {
				fmt.Printf(" Agent:      %s\n", evalAgent)
			}
			if evalModel != "" {
				fmt.Printf(" Model:      %s\n", evalModel)
			}
			if evalTier != "" {
				fmt.Printf(" Tier:       %s\n", evalTier)
			}
			if evalDifficulty != "" {
				fmt.Printf(" Difficulty: %s\n", evalDifficulty)
			}
			fmt.Printf(" Tasks:      %d\n", len(allTasks))
			fmt.Println()
			fmt.Println(" Tasks that would be executed:")
			fmt.Println("─────────────────────────────────────────────────────────────")
			for i, t := range allTasks {
				timeout := evalTimeout
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

		// Create output directory
		if evalOutputDir == "" {
			evalOutputDir = filepath.Join("eval-results", fmt.Sprintf("%s-%s", evalAgent, timestamp))
		}
		if err := os.MkdirAll(evalOutputDir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// For resume mode: filter out completed tasks and clean incomplete dirs.
		var tasksToRun []*task.Task
		totalTaskCount := len(allTasks)
		if isResuming {
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
			if err := cleanIncompleteTaskDirs(evalOutputDir, completedTasks, allTasks); err != nil {
				return fmt.Errorf("cleaning incomplete tasks: %w", err)
			}

			// Filter out completed tasks.
			for _, t := range allTasks {
				taskSlug := string(t.Language) + "/" + t.Slug
				if !completedTasks[taskSlug] {
					tasksToRun = append(tasksToRun, t)
				}
			}

			if len(tasksToRun) == 0 {
				fmt.Println("\n All tasks already completed. Nothing to resume.")
				return nil
			}
		} else {
			tasksToRun = allTasks
			// Save run config for new runs (enables resume).
			if err := saveRunConfig(evalOutputDir, allTasks); err != nil {
				return fmt.Errorf("saving run config: %w", err)
			}
		}

		// Set up interrupt handler for graceful shutdown.
		interrupted := setupInterruptHandler()
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
		fmt.Printf(" Agent:   %s\n", evalAgent)
		if evalModel != "" {
			fmt.Printf(" Model:   %s\n", evalModel)
		}
		if evalTier != "" {
			fmt.Printf(" Tier:    %s\n", evalTier)
		}
		if evalDifficulty != "" {
			fmt.Printf(" Difficulty: %s\n", evalDifficulty)
		}
		if evalParallel > 1 {
			fmt.Printf(" Parallel: %d\n", evalParallel)
		}
		if isResuming {
			fmt.Printf(" Tasks:   %d remaining of %d total\n", len(tasksToRun), totalTaskCount)
		} else {
			fmt.Printf(" Tasks:   %d\n", len(tasksToRun))
		}
		fmt.Printf(" Output:  %s\n", evalOutputDir)
		fmt.Println()

		// Run tasks
		results := make([]EvalResult, 0, len(tasksToRun))
		passed, failed := 0, 0

		parallel := evalParallel
		if parallel <= 0 {
			parallel = 1
		}

		if parallel == 1 {
			consecutiveQuotaExhausted := 0
			for i, t := range tasksToRun {
				// Check for interrupt before starting next task.
				if checkInterrupted(interrupted) {
					wasInterrupted = true
					fmt.Println("\n\033[33m⚠ Interrupt received. Saving partial results...\033[0m")
					break
				}

				fmt.Println("─────────────────────────────────────────────────────────────")
				fmt.Printf(" [%d/%d] %s\n", i+1, len(tasksToRun), t.ID())
				fmt.Println("─────────────────────────────────────────────────────────────")

				result := runTaskWithAgent(r, t, evalAgent, evalModel, evalOutputDir, evalTimeout)
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
				if !evalKeepWorkspaces && result.WorkspaceDir != "" {
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
						res := runTaskWithAgent(r, j.t, evalAgent, evalModel, evalOutputDir, evalTimeout)
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
				collected[jr.idx] = jr.r
				seen++

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

				if !evalKeepWorkspaces && jr.r.WorkspaceDir != "" {
					if err := os.RemoveAll(jr.r.WorkspaceDir); err != nil {
						logger.Debug("failed to cleanup workspace", "dir", jr.r.WorkspaceDir, "error", err)
					}
				}

				// Check for interrupt after each result.
				shouldStop := checkInterrupted(interrupted)
				stopReason := "Interrupt received"

				// Also stop if we hit consecutive quota exhaustion threshold
				if !shouldStop && consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
					shouldStop = true
					stopReason = fmt.Sprintf("Quota exhausted for %d consecutive tasks", consecutiveQuotaExhausted)
				}

				if shouldStop {
					wasInterrupted = true
					fmt.Printf("\n\033[33m⚠ %s. Waiting for in-flight tasks...\033[0m\n", stopReason)
					close(stopSending)
					// Drain remaining results from in-flight tasks.
					for jr := range jobResults {
						collected[jr.idx] = jr.r
						if jr.r.Passed {
							passed++
						} else {
							failed++
						}
						if !evalKeepWorkspaces && jr.r.WorkspaceDir != "" {
							_ = os.RemoveAll(jr.r.WorkspaceDir)
						}
					}
					break collectLoop
				}
			}
			// Only include results that were actually run.
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
		fmt.Printf(" Agent:     %s\n", evalAgent)
		if evalModel != "" {
			fmt.Printf(" Model:     %s\n", evalModel)
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
		var cleanPasses int
		var partialPasses int
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
			switch r.Status {
			case task.StatusPass:
				cleanPasses++
			case task.StatusPartialPass:
				partialPasses++
			case task.StatusIntegrityViolation:
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
		model := evalModel
		if model == "" {
			model = "unknown"
		}

		summary := EvalSummary{
			Agent:               evalAgent,
			Model:               model,
			Reasoning:           evalReasoning,
			Timestamp:           timestamp,
			Tier:                evalTier,
			Difficulty:          evalDifficulty,
			Parallel:            parallel,
			Results:             results,
			Passed:              passed,
			Failed:              failed,
			Total:               total,
			PassRate:            passRate,
			WeightedScore:       totalWeightedScore,
			MaxPossibleScore:    maxPossibleScore,
			WeightedPassRate:    weightedPassRate,
			CleanPasses:         cleanPasses,
			PartialPasses:       partialPasses,
			IntegrityViolations: integrityViolations,
			Duration:            totalDuration,
			AgentTime:           totalAgentTime,
			ValidateTime:        totalValidateTime,
			PromptChars:         totalPromptChars,
			ByLanguage:          finalize(byLanguage),
			ByTier:              finalize(byTier),
			ByDifficulty:        finalize(byDifficulty),
			UseMCPTools:         evalUseMCPTools,
			DisableMCP:          evalDisableMCP,
			QuotaAffectedTasks:  quotaAffectedTasks,
			TotalQuotaRetries:   totalQuotaRetries,
		}

		summaryPath := filepath.Join(evalOutputDir, "summary.json")
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
			evalAgent, evalModel, timestamp, totalDuration,
			results, evalOutputDir, loader, allTasks, newlyRunTasks, prevTasks,
		)
		if err != nil {
			logger.Warn("failed to generate attestation", "error", err)
		} else {
			attestationPath := filepath.Join(evalOutputDir, "attestation.json")
			attestationData, _ := json.MarshalIndent(attestation, "", "  ")
			if err := os.WriteFile(attestationPath, attestationData, 0644); err != nil {
				logger.Warn("failed to save attestation", "error", err)
			} else {
				fmt.Printf(" Attestation saved to: %s\n", attestationPath)
			}
		}

		// Generate human-readable report.md
		reportMd := generateEvalReport(summary, attestation)
		reportPath := filepath.Join(evalOutputDir, "report.md")
		if err := os.WriteFile(reportPath, []byte(reportMd), 0644); err != nil {
			logger.Warn("failed to save report", "error", err)
		} else {
			fmt.Printf(" Report saved to: %s\n", reportPath)
		}

		// Generate leaderboard submission file
		submission := generateLeaderboardSubmission(summary, attestation)
		submissionData, _ := json.MarshalIndent(submission, "", "  ")
		submissionPath := filepath.Join(evalOutputDir, "submission.json")
		if err := os.WriteFile(submissionPath, submissionData, 0644); err != nil {
			logger.Warn("failed to save submission", "error", err)
		} else {
			fmt.Printf(" Submission saved to: %s\n", submissionPath)
		}

		fmt.Println()

		// If interrupted, print resume command.
		if wasInterrupted {
			printResumeCommand(evalOutputDir)
		}

		return nil
	},
}

func runTaskWithAgent(r *runner.Runner, t *task.Task, agent, model, outputDir string, timeout int) EvalResult {
	start := time.Now()
	result := EvalResult{
		Task:       t.ID(),
		Language:   string(t.Language),
		Tier:       t.Tier,
		Difficulty: t.Difficulty,
	}

	loader := task.NewLoader(tasks.FS, tasksDir)

	// Create workspace for this task - use language prefix to avoid slug collisions
	workspaceName := fmt.Sprintf("%s-%s", t.Language, t.Slug)
	workspaceDir := filepath.Join(outputDir, workspaceName)
	result.WorkspaceDir = workspaceDir // Track for cleanup

	if err := r.InitWorkspaceForTask(t, workspaceDir); err != nil {
		result.Error = fmt.Sprintf("init failed: %v", err)
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Build agent command
	prompt := buildAgentPrompt(t, evalUseMCPTools)
	result.PromptChars = utf8.RuneCountInString(prompt)

	agentTimeout := time.Duration(timeout) * time.Second
	if agentTimeout <= 0 {
		agentTimeout = 120 * time.Second
	}
	// Use task-specific timeout if set
	if t.AgentTimeout > 0 {
		agentTimeout = time.Duration(t.AgentTimeout) * time.Second
	}

	// Get agent configuration
	agentCfg := cfg.GetAgent(agent)
	if agentCfg == nil {
		result.Error = fmt.Sprintf("unknown agent: %s", agent)
		result.Duration = time.Since(start).Seconds()
		return result
	}

	agentLogPath := filepath.Join(workspaceDir, "agent.log")

	// Execute agent with quota retry support
	agentResult := executeAgentWithRetries(t, agentCfg, prompt, model, workspaceDir, agentLogPath, agentTimeout, agent)
	result.AgentTime = agentResult.totalTime
	result.AgentTimedOut = agentResult.timedOut
	result.QuotaRetries = agentResult.quotaRetries
	result.QuotaExhausted = agentResult.quotaExhausted

	// Preserve agent log in eval output directory for debugging
	copyAgentLog(agentLogPath, outputDir, t)

	// Ensure the agent didn't modify task-owned files.
	modified, err := detectModifiedTaskFiles(loader, t, workspaceDir)
	if err != nil {
		result.Error = fmt.Sprintf("integrity check failed: %v", err)
		result.Duration = time.Since(start).Seconds()
		return result
	}
	if len(modified) > 0 {
		sort.Strings(modified)
		result.Error = fmt.Sprintf("modified task files (disallowed): %s", strings.Join(modified, ", "))
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Add hidden tests (not shown to the agent) before validation.
	if err := writeTaskFilesToWorkspace(loader, t, workspaceDir, t.HiddenTestFiles()); err != nil {
		result.Error = fmt.Sprintf("writing hidden tests: %v", err)
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Run sanity harness to validate
	ctx := context.Background()
	if timeout == 0 {
		timeout = 120
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
		Timeout:           timeout,
		MaxAttempts:       1,
		ValidationCommand: validationCmd,
	})
	result.ValidateTime = time.Since(validateStart).Seconds()

	result.Duration = time.Since(start).Seconds()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Passed = session.Passed()
	result.Attempts = len(session.Attempts)

	// Save validation output to validation.log
	if len(session.Attempts) > 0 {
		lastAttempt := session.Attempts[len(session.Attempts)-1]
		taskOutputDir := filepath.Join(outputDir, fmt.Sprintf("%s-%s", t.Language, t.Slug))
		validationLogPath := filepath.Join(taskOutputDir, "validation.log")
		if err := os.MkdirAll(taskOutputDir, 0755); err == nil {
			_ = os.WriteFile(validationLogPath, []byte(lastAttempt.RawOutput), 0644)
		}
	}

	// Compute task weight and score
	weight := task.ComputeWeight(t)
	result.Weight = weight.Base
	result.Status = task.DetermineStatus(result.Passed, result.AgentTimedOut, result.Error)
	result.WeightedScore = task.ScoreResult(result.Passed, result.AgentTimedOut, result.Error, weight)

	return result
}

// agentExecutionResult holds the outcome of agent execution with retries.
type agentExecutionResult struct {
	totalTime      float64
	timedOut       bool
	quotaRetries   int
	quotaExhausted bool
}

// executeAgentWithRetries runs the agent command with quota-aware retry logic.
func executeAgentWithRetries(
	t *task.Task,
	agentCfg *config.AgentConfig,
	prompt, model, workspaceDir, agentLogPath string,
	agentTimeout time.Duration,
	agent string,
) agentExecutionResult {
	var result agentExecutionResult

	for attempt := 0; attempt <= quotaMaxRetries; attempt++ {
		// On retry, wait and log
		if attempt > 0 {
			delay := getRetryDelay(attempt)
			logger.Info("quota error detected, retrying",
				"task", t.ID(),
				"attempt", attempt,
				"delay", delay)
			time.Sleep(delay)
			result.quotaRetries = attempt
		}

		// Run single attempt
		attemptResult := runAgentAttempt(agentCfg, prompt, model, workspaceDir, agentLogPath, agentTimeout, agent, attempt)
		result.totalTime += attemptResult.duration
		result.timedOut = attemptResult.timedOut

		// Check for quota errors and decide whether to retry
		hasError, isRecoverable := detectQuotaError(agentLogPath)
		if !hasError {
			break // No quota error, we're done
		}
		if !isRecoverable {
			result.quotaExhausted = true
			logger.Debug("non-recoverable quota error, skipping retries", "task", t.ID())
			break
		}
		if attempt == quotaMaxRetries {
			result.quotaExhausted = true
			logger.Debug("max quota retries reached", "task", t.ID(), "retries", quotaMaxRetries)
			break
		}
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
	agentCfg *config.AgentConfig,
	prompt, model, workspaceDir, agentLogPath string,
	agentTimeout time.Duration,
	agent string,
	attempt int,
) agentAttemptResult {
	var result agentAttemptResult

	agentCtx, cancel := context.WithTimeout(context.Background(), agentTimeout)
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

// copyAgentLog copies the agent log to the eval output directory.
func copyAgentLog(agentLogPath, outputDir string, t *task.Task) {
	if _, err := os.Stat(agentLogPath); err == nil {
		agentLogDst := filepath.Join(outputDir, fmt.Sprintf("%s-%s", t.Language, t.Slug), "agent.log")
		if err := os.MkdirAll(filepath.Dir(agentLogDst), 0755); err == nil {
			if srcData, err := os.ReadFile(agentLogPath); err == nil {
				_ = os.WriteFile(agentLogDst, srcData, 0644)
			}
		}
	}
}

func buildAgentPrompt(t *task.Task, useMCPTools bool) string {
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
- Tests run automatically in a Docker container with a %s toolchain pre-installed.
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
- Evaluation fails if you modify protected files.`,
		t.Name, t.Language, t.Tier, t.Difficulty, t.Description,
		strings.Join(stubFiles, ", "), strings.Join(testFiles, ", "),
		t.Language)

	// Append MCP tools section if enabled
	if useMCPTools {
		prompt += `

MCP TOOLS:
You have access to MCP (Model Context Protocol) tools. Use them proactively:
- Use file reading tools to examine stub files and test files thoroughly
- Use code search tools to find patterns, helper functions, or related implementations
- Use any available analysis tools to understand the codebase structure
- Prefer using tools to gather context over making assumptions

Do NOT guess at implementation details that tools can help you discover.`
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
	CleanPasses         int `json:"clean_passes"`
	PartialPasses       int `json:"partial_passes"`
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
		CleanPasses:         summary.CleanPasses,
		PartialPasses:       summary.PartialPasses,
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
	fmt.Fprintf(sb, "| Timestamp | %s |\n", summary.Timestamp)
	fmt.Fprintf(sb, "| Pass Rate | **%.1f%%** (%d/%d) |\n", summary.PassRate, summary.Passed, summary.Total)
	fmt.Fprintf(sb, "| Weighted Pass Rate | **%.1f%%** |\n", summary.WeightedPassRate)
	fmt.Fprintf(sb, "| Weighted Score | %.2f / %.2f |\n", summary.WeightedScore, summary.MaxPossibleScore)
	fmt.Fprintf(sb, "| Duration | %.1fs |\n", summary.Duration)
	sb.WriteString("\n")
}

func writeReportQuality(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Quality Breakdown\n\n")
	fmt.Fprintf(sb, "- **Clean Passes**: %d\n", summary.CleanPasses)
	fmt.Fprintf(sb, "- **Partial Passes** (timed out but passed): %d\n", summary.PartialPasses)
	fmt.Fprintf(sb, "- **Integrity Violations** (modified test files): %d\n", summary.IntegrityViolations)
	fmt.Fprintf(sb, "- **Failures**: %d\n", summary.Failed-summary.IntegrityViolations-summary.PartialPasses)
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
	case r.Status == task.StatusPartialPass:
		return "⚠️", "PARTIAL"
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

// getRetryDelay returns the delay for the given retry attempt (1-indexed).
func getRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return quotaRetryDelay1
	case 2:
		return quotaRetryDelay2
	case 3:
		return quotaRetryDelay3
	default:
		return quotaRetryDelay3
	}
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

// setupInterruptHandler creates a channel that receives interrupt signals.
func setupInterruptHandler() chan os.Signal {
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
	return interrupted
}

// checkInterrupted checks if an interrupt signal has been received (non-blocking).
func checkInterrupted(interrupted chan os.Signal) bool {
	select {
	case <-interrupted:
		return true
	default:
		return false
	}
}

// printResumeCommand prints the command to resume an interrupted eval.
func printResumeCommand(outputDir string) {
	fmt.Printf("\n\033[33m⚠ Evaluation interrupted. To resume, run:\033[0m\n")
	fmt.Printf("  ./sanity eval --resume %s\n\n", outputDir)
}

func init() {
	evalCmd.Flags().StringVar(&evalAgent, "agent", "", "agent to evaluate (see --help for list)")
	evalCmd.Flags().StringVar(&evalModel, "model", "", "model to use (e.g., gemini-2.5-pro or google/gemini-2.5-flash)")
	evalCmd.Flags().StringVar(&evalReasoning, "reasoning", "", "reasoning effort level (e.g., off, none, low, medium, high)")
	evalCmd.Flags().StringVar(&evalTasks, "tasks", "", "comma-separated list of task slugs")
	evalCmd.Flags().StringVar(&evalLang, "lang", "", "filter by language (go, rust, typescript)")
	evalCmd.Flags().StringVar(&evalTier, "tier", "core", "filter by tier (core, extended, all)")
	evalCmd.Flags().StringVar(&evalDifficulty, "difficulty", "", "filter by difficulty (comma-separated)")
	evalCmd.Flags().IntVar(&evalTimeout, "timeout", 120, "timeout per task in seconds")
	evalCmd.Flags().IntVar(&evalParallel, "parallel", 1, "run up to N tasks in parallel")
	evalCmd.Flags().StringVar(&evalOutputDir, "output", "", "output directory for results")
	evalCmd.Flags().BoolVar(&evalKeepWorkspaces, "keep-workspaces", false, "keep workspace directories after evaluation")
	evalCmd.Flags().BoolVar(&evalDryRun, "dry-run", false, "show what tasks would be run without executing")
	evalCmd.Flags().BoolVar(&evalUseMCPTools, "use-mcp-tools", false, "inject MCP tool usage instructions into agent prompt")
	evalCmd.Flags().BoolVar(&evalDisableMCP, "disable-mcp", false, "disable MCP tools for agents that support it (currently: opencode)")
	evalCmd.Flags().StringVar(&evalResume, "resume", "", "resume eval from existing output directory")
}
