package cli

import (
	"bytes"
	"context"
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
	Task         string  `json:"task"`
	Language     string  `json:"language"`
	Tier         string  `json:"tier,omitempty"`
	Difficulty   string  `json:"difficulty,omitempty"`
	Passed       bool    `json:"passed"`
	Attempts     int     `json:"attempts"`
	Duration     float64 `json:"duration_seconds"`
	AgentTime    float64 `json:"agent_duration_seconds,omitempty"`
	ValidateTime float64 `json:"validation_duration_seconds,omitempty"`
	PromptChars  int     `json:"prompt_chars,omitempty"`
	Error        string  `json:"error,omitempty"`
	WorkspaceDir string  `json:"-"` // Not serialized, used for cleanup
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
	Agent        string                   `json:"agent"`
	Model        string                   `json:"model,omitempty"`
	Timestamp    string                   `json:"timestamp"`
	Tier         string                   `json:"tier,omitempty"`
	Difficulty   string                   `json:"difficulty,omitempty"`
	Parallel     int                      `json:"parallel,omitempty"`
	Results      []EvalResult             `json:"results"`
	Passed       int                      `json:"passed"`
	Failed       int                      `json:"failed"`
	Total        int                      `json:"total"`
	PassRate     float64                  `json:"pass_rate"`
	Duration     float64                  `json:"duration_seconds,omitempty"`
	AgentTime    float64                  `json:"agent_duration_seconds,omitempty"`
	ValidateTime float64                  `json:"validation_duration_seconds,omitempty"`
	PromptChars  int                      `json:"prompt_chars,omitempty"`
	ByLanguage   map[string]EvalAggregate `json:"by_language,omitempty"`
	ByTier       map[string]EvalAggregate `json:"by_tier,omitempty"`
	ByDifficulty map[string]EvalAggregate `json:"by_difficulty,omitempty"`
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate an agent against all tasks",
	Long: `Runs all (or selected) tasks against a coding agent and reports results.

Supported agents:
  gemini    - Gemini CLI (requires 'gemini' in PATH)
  opencode  - OpenCode CLI (requires 'opencode' in PATH)

Examples:
  sanity eval --agent gemini
  sanity eval --agent gemini --model gemini-3-pro-preview
  sanity eval --agent opencode
  sanity eval --agent gemini --lang go
  sanity eval --agent gemini --tasks bank-account,react
  sanity eval --agent gemini --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Dry-run mode doesn't require agent to be installed
		if !evalDryRun {
			if evalAgent == "" {
				return fmt.Errorf("--agent is required (gemini or opencode)")
			}

			// Validate agent
			switch evalAgent {
			case "gemini", "opencode":
				// OK
			default:
				return fmt.Errorf("unknown agent: %s (supported: gemini, opencode)", evalAgent)
			}

			// Check agent is installed
			if _, err := exec.LookPath(evalAgent); err != nil {
				return fmt.Errorf("%s not found in PATH", evalAgent)
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

			addAgg(byLanguage, r.Language, r)
			if r.Tier != "" {
				addAgg(byTier, r.Tier, r)
			}
			if r.Difficulty != "" {
				addAgg(byDifficulty, r.Difficulty, r)
			}
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
			Agent:        evalAgent,
			Model:        evalModel,
			Timestamp:    timestamp,
			Tier:         evalTier,
			Difficulty:   evalDifficulty,
			Parallel:     parallel,
			Results:      results,
			Passed:       passed,
			Failed:       failed,
			Total:        total,
			PassRate:     passRate,
			Duration:     totalDuration,
			AgentTime:    totalAgentTime,
			ValidateTime: totalValidateTime,
			PromptChars:  totalPromptChars,
			ByLanguage:   finalize(byLanguage),
			ByTier:       finalize(byTier),
			ByDifficulty: finalize(byDifficulty),
		}

		summaryPath := filepath.Join(evalOutputDir, "summary.json")
		summaryData, _ := json.MarshalIndent(summary, "", "  ")
		if err := os.WriteFile(summaryPath, summaryData, 0644); err != nil {
			logger.Warn("failed to save summary", "error", err)
		} else {
			fmt.Printf(" Results saved to: %s\n", summaryPath)
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
	var cmd *exec.Cmd
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

	switch agent {
	case "gemini":
		args := []string{"--yolo"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, prompt)
		cmd = exec.CommandContext(agentCtx, "gemini", args...)

	case "opencode":
		// OpenCode with prompt flag
		cmd = exec.CommandContext(agentCtx, "opencode", "-p", prompt)
	}

	cmd.Dir = workspaceDir
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
		logger.Debug("agent timed out", "timeout", agentTimeout)
	}
	if agentErr != nil {
		logger.Debug("agent returned error", "error", agentErr)
		// Don't fail yet - the tests will determine success
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

func init() {
	evalCmd.Flags().StringVar(&evalAgent, "agent", "", "agent to evaluate (gemini, opencode)")
	evalCmd.Flags().StringVar(&evalModel, "model", "", "model to use (for gemini)")
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
