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
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	evalAgent          string
	evalModel          string
	evalTasks          string
	evalLang           string
	evalTier           string
	evalDifficulty     string
	evalTimeout        int
	evalOutputDir      string
	evalKeepWorkspaces bool
	evalParallel       int
	evalDryRun         bool
)

// EvalResult holds the result of evaluating a single task.
type EvalResult struct {
	Task          string            `json:"task"`
	Language      string            `json:"language"`
	Tier          string            `json:"tier,omitempty"`
	Difficulty    string            `json:"difficulty,omitempty"`
	Passed        bool              `json:"passed"`
	AgentTimedOut bool              `json:"agent_timed_out,omitempty"`
	Status        task.ResultStatus `json:"status,omitempty"`
	Attempts      int               `json:"attempts"`
	Duration      float64           `json:"duration_seconds"`
	AgentTime     float64           `json:"agent_duration_seconds,omitempty"`
	ValidateTime  float64           `json:"validation_duration_seconds,omitempty"`
	PromptChars   int               `json:"prompt_chars,omitempty"`
	Error         string            `json:"error,omitempty"`
	Weight        float64           `json:"weight,omitempty"`
	WeightedScore float64           `json:"weighted_score,omitempty"`
	WorkspaceDir  string            `json:"-"` // Not serialized, used for cleanup
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
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate an agent against all tasks",
	Long: `Runs all (or selected) tasks against a coding agent and reports results.

Built-in agents:
  gemini    - Google Gemini CLI
  opencode  - OpenCode CLI
  claude    - Anthropic Claude Code
  codex     - OpenAI Codex CLI
  kimi      - Moonshot Kimi CLI
  crush     - Crush CLI
  copilot   - GitHub Copilot CLI
  droid     - Factory Droid CLI
  iflow     - iFlow CLI
  qwen      - Qwen Code CLI

Custom agents can be configured in sanity.toml under [agents.<name>].

Examples:
  sanity eval --agent gemini
  sanity eval --agent gemini --model gemini-2.5-pro
  sanity eval --agent opencode --model google/gemini-2.5-flash
  sanity eval --agent claude --lang go
  sanity eval --agent my-custom-agent --tasks bank-account,react
  sanity eval --agent gemini --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
		timestamp := time.Now().Format("2006-01-02T150405")
		if evalOutputDir == "" {
			evalOutputDir = filepath.Join("eval-results", fmt.Sprintf("%s-%s", evalAgent, timestamp))
		}
		if err := os.MkdirAll(evalOutputDir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// Print header
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(" SANITY HARNESS - Agent Evaluation")
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
		fmt.Printf(" Tasks:   %d\n", len(allTasks))
		fmt.Printf(" Output:  %s\n", evalOutputDir)
		fmt.Println()

		// Run tasks
		results := make([]EvalResult, 0, len(allTasks))
		passed, failed := 0, 0

		parallel := evalParallel
		if parallel <= 0 {
			parallel = 1
		}

		if parallel == 1 {
			for i, t := range allTasks {
				fmt.Println("─────────────────────────────────────────────────────────────")
				fmt.Printf(" [%d/%d] %s\n", i+1, len(allTasks), t.ID())
				fmt.Println("─────────────────────────────────────────────────────────────")

				result := runTaskWithAgent(r, t, evalAgent, evalModel, evalOutputDir, evalTimeout)
				results = append(results, result)

				if result.Passed {
					fmt.Printf(" ✓ PASSED (%.2fs)\n", result.Duration)
					passed++
				} else {
					fmt.Printf(" ✗ FAILED (%.2fs)\n", result.Duration)
					if result.Error != "" {
						fmt.Printf("   Error: %s\n", result.Error)
					}
					failed++
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

			go func() {
				for i, t := range allTasks {
					jobs <- job{idx: i, t: t}
				}
				close(jobs)
				wg.Wait()
				close(jobResults)
			}()

			collected := make([]EvalResult, len(allTasks))
			seen := 0
			for jr := range jobResults {
				collected[jr.idx] = jr.r
				seen++

				status := "FAILED"
				if jr.r.Passed {
					status = "PASSED"
				}
				fmt.Printf(" [%d/%d] %s %s (%.2fs)\n", seen, len(allTasks), jr.r.Task, status, jr.r.Duration)
				if !jr.r.Passed && jr.r.Error != "" {
					fmt.Printf("   Error: %s\n", jr.r.Error)
				}

				if jr.r.Passed {
					passed++
				} else {
					failed++
				}

				if !evalKeepWorkspaces && jr.r.WorkspaceDir != "" {
					if err := os.RemoveAll(jr.r.WorkspaceDir); err != nil {
						logger.Debug("failed to cleanup workspace", "dir", jr.r.WorkspaceDir, "error", err)
					}
				}
			}
			results = append(results, collected...)
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

		summary := EvalSummary{
			Agent:               evalAgent,
			Model:               evalModel,
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
		attestation, err := generateAttestation(
			evalAgent, evalModel, timestamp, totalDuration,
			results, evalOutputDir, loader, allTasks,
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
	prompt := buildAgentPrompt(t)
	result.PromptChars = utf8.RuneCountInString(prompt)

	agentTimeout := time.Duration(timeout) * time.Second
	if agentTimeout <= 0 {
		agentTimeout = 120 * time.Second
	}
	// Use task-specific timeout if set
	if t.AgentTimeout > 0 {
		agentTimeout = time.Duration(t.AgentTimeout) * time.Second
	}
	agentCtx, cancel := context.WithTimeout(context.Background(), agentTimeout)
	defer cancel()

	// Get agent configuration
	agentCfg := cfg.GetAgent(agent)
	if agentCfg == nil {
		result.Error = fmt.Sprintf("unknown agent: %s", agent)
		result.Duration = time.Since(start).Seconds()
		return result
	}

	cmd := buildAgentCommand(agentCtx, agentCfg, prompt, model)

	cmd.Dir = workspaceDir

	// Use /dev/null for stdin to prevent TTY issues with agents that use Ink/React
	devNull, err := os.Open(os.DevNull)
	if err == nil {
		cmd.Stdin = devNull
		defer func() { _ = devNull.Close() }()
	}

	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil

	// Save agent output to log file
	logFile, err := os.Create(filepath.Join(workspaceDir, "agent.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer func() { _ = logFile.Close() }()
	}

	// Run agent
	agentStart := time.Now()
	agentErr := cmd.Run()
	result.AgentTime = time.Since(agentStart).Seconds()
	if errors.Is(agentCtx.Err(), context.DeadlineExceeded) {
		result.AgentTimedOut = true
		logger.Debug("agent timed out", "timeout", agentTimeout)
	}
	if agentErr != nil {
		logger.Debug("agent returned error", "error", agentErr)
		// Don't fail yet - the tests will determine success
	}

	// Preserve agent log in eval output directory for debugging
	agentLogSrc := filepath.Join(workspaceDir, "agent.log")
	if _, err := os.Stat(agentLogSrc); err == nil {
		agentLogDst := filepath.Join(outputDir, fmt.Sprintf("%s-%s", t.Language, t.Slug), "agent.log")
		if err := os.MkdirAll(filepath.Dir(agentLogDst), 0755); err == nil {
			if srcData, err := os.ReadFile(agentLogSrc); err == nil {
				_ = os.WriteFile(agentLogDst, srcData, 0644)
			}
		}
	}

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

	// Compute task weight and score
	weight := task.ComputeWeight(t)
	result.Weight = weight.Base
	result.Status = task.DetermineStatus(result.Passed, result.AgentTimedOut, result.Error)
	result.WeightedScore = task.ScoreResult(result.Passed, result.AgentTimedOut, result.Error, weight)

	return result
}

func buildAgentPrompt(t *task.Task) string {
	stubFiles := make([]string, 0, len(t.Files.Stub))
	for _, f := range t.Files.Stub {
		stubFiles = append(stubFiles, task.StripTxtExtension(f))
	}
	testFiles := make([]string, 0, len(t.Files.Test))
	for _, f := range t.Files.Test {
		testFiles = append(testFiles, task.StripTxtExtension(f))
	}

	return fmt.Sprintf(`You are solving a coding task called "%s".

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
// It handles prompt placeholder substitution, model flag positioning, and environment variables.
func buildAgentCommand(ctx context.Context, agentCfg *config.AgentConfig, prompt, model string) *exec.Cmd {
	var args []string

	// Determine model flag position (default to "before")
	position := agentCfg.ModelFlagPosition
	if position == "" {
		position = "before"
	}

	// Add model flag if specified (before position)
	if model != "" && agentCfg.ModelFlag != "" && position == "before" {
		args = append(args, agentCfg.ModelFlag, model)
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
	if model != "" && agentCfg.ModelFlag != "" && position == "after" {
		args = append(args, agentCfg.ModelFlag, model)
	}

	cmd := exec.CommandContext(ctx, agentCfg.Command, args...)

	// Add environment variables if specified
	if len(agentCfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range agentCfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	return cmd
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
func generateAttestation(
	agent, model, timestamp string,
	totalDuration float64,
	results []EvalResult,
	outputDir string,
	loader *task.Loader,
	allTasks []*task.Task,
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
		var solutionPaths []string
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

func init() {
	evalCmd.Flags().StringVar(&evalAgent, "agent", "", "agent to evaluate (see --help for list)")
	evalCmd.Flags().StringVar(&evalModel, "model", "", "model to use (e.g., gemini-2.5-pro or google/gemini-2.5-flash)")
	evalCmd.Flags().StringVar(&evalTasks, "tasks", "", "comma-separated list of task slugs")
	evalCmd.Flags().StringVar(&evalLang, "lang", "", "filter by language (go, rust, typescript)")
	evalCmd.Flags().StringVar(&evalTier, "tier", "core", "filter by tier (core, extended, all)")
	evalCmd.Flags().StringVar(&evalDifficulty, "difficulty", "", "filter by difficulty (comma-separated)")
	evalCmd.Flags().IntVar(&evalTimeout, "timeout", 120, "timeout per task in seconds")
	evalCmd.Flags().IntVar(&evalParallel, "parallel", 1, "run up to N tasks in parallel")
	evalCmd.Flags().StringVar(&evalOutputDir, "output", "", "output directory for results")
	evalCmd.Flags().BoolVar(&evalKeepWorkspaces, "keep-workspaces", false, "keep workspace directories after evaluation")
	evalCmd.Flags().BoolVar(&evalDryRun, "dry-run", false, "show what tasks would be run without executing")
}
