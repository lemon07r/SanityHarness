package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var (
	evalAgent     string
	evalModel     string
	evalTasks     string
	evalLang      string
	evalTimeout   int
	evalOutputDir string
)

// EvalResult holds the result of evaluating a single task
type EvalResult struct {
	Task     string  `json:"task"`
	Language string  `json:"language"`
	Passed   bool    `json:"passed"`
	Attempts int     `json:"attempts"`
	Duration float64 `json:"duration_seconds"`
	Error    string  `json:"error,omitempty"`
}

// EvalSummary holds the overall evaluation summary
type EvalSummary struct {
	Agent     string       `json:"agent"`
	Model     string       `json:"model,omitempty"`
	Timestamp string       `json:"timestamp"`
	Results   []EvalResult `json:"results"`
	Passed    int          `json:"passed"`
	Failed    int          `json:"failed"`
	Total     int          `json:"total"`
	PassRate  float64      `json:"pass_rate"`
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
  sanity eval --agent gemini --tasks bank-account,react`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer r.Close()

		// Get tasks to run
		allTasks, err := r.ListTasks()
		if err != nil {
			return fmt.Errorf("listing tasks: %w", err)
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

		// Filter by specific tasks if specified
		if evalTasks != "" {
			slugs := strings.Split(evalTasks, ",")
			slugMap := make(map[string]bool)
			for _, s := range slugs {
				slugMap[strings.TrimSpace(s)] = true
			}
			var filtered []*task.Task
			for _, t := range allTasks {
				if slugMap[t.Slug] {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		if len(allTasks) == 0 {
			return fmt.Errorf("no tasks match the specified filters")
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
		fmt.Printf(" Tasks:   %d\n", len(allTasks))
		fmt.Printf(" Output:  %s\n", evalOutputDir)
		fmt.Println()

		// Run each task
		var results []EvalResult
		passed, failed := 0, 0

		for i, t := range allTasks {
			fmt.Println("─────────────────────────────────────────────────────────────")
			fmt.Printf(" [%d/%d] %s (%s)\n", i+1, len(allTasks), t.Slug, t.Language)
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
			fmt.Println()
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
		summary := EvalSummary{
			Agent:     evalAgent,
			Model:     evalModel,
			Timestamp: timestamp,
			Results:   results,
			Passed:    passed,
			Failed:    failed,
			Total:     total,
			PassRate:  passRate,
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
		Task:     t.Slug,
		Language: string(t.Language),
	}

	// Create workspace for this task - use language prefix to avoid slug collisions
	workspaceName := fmt.Sprintf("%s-%s", t.Language, t.Slug)
	workspaceDir := filepath.Join(outputDir, workspaceName)
	if err := r.InitWorkspaceForTask(t, workspaceDir); err != nil {
		result.Error = fmt.Sprintf("init failed: %v", err)
		result.Duration = time.Since(start).Seconds()
		return result
	}

	// Build agent command
	var cmd *exec.Cmd
	prompt := buildAgentPrompt(t)

	switch agent {
	case "gemini":
		args := []string{"--yolo"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, prompt)
		cmd = exec.Command("gemini", args...)

	case "opencode":
		// OpenCode with prompt flag
		cmd = exec.Command("opencode", "-p", prompt)
	}

	cmd.Dir = workspaceDir
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil

	// Save agent output to log file
	logFile, err := os.Create(filepath.Join(workspaceDir, "agent.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer logFile.Close()
	}

	// Run agent
	agentErr := cmd.Run()
	if agentErr != nil {
		logger.Debug("agent returned error", "error", agentErr)
		// Don't fail yet - the tests will determine success
	}

	// Run sanity harness to validate
	ctx := context.Background()
	if timeout == 0 {
		timeout = 120
	}

	session, err := r.Run(ctx, runner.RunOptions{
		Task:         t, // Pass task directly to avoid slug collision
		WorkspaceDir: workspaceDir,
		Timeout:      timeout,
		MaxAttempts:  1,
	})

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
	return fmt.Sprintf(`You are solving a coding task called "%s" in %s.

Read all files in this directory:
- The stub file contains function signatures with panic() or todo!() placeholders
- The test file shows the expected behavior and edge cases

Your job:
1. Understand what each function should do from the tests
2. Implement all functions, replacing panic()/todo!() with working code
3. Handle all edge cases shown in the tests
4. Ensure thread-safety if the tests use concurrent operations

Write your complete implementation to the source file. Do not modify the test file.`,
		t.Name, t.Language)
}

func init() {
	evalCmd.Flags().StringVar(&evalAgent, "agent", "", "agent to evaluate (gemini, opencode)")
	evalCmd.Flags().StringVar(&evalModel, "model", "", "model to use (for gemini)")
	evalCmd.Flags().StringVar(&evalTasks, "tasks", "", "comma-separated list of task slugs")
	evalCmd.Flags().StringVar(&evalLang, "lang", "", "filter by language (go, rust, typescript)")
	evalCmd.Flags().IntVar(&evalTimeout, "timeout", 120, "timeout per task in seconds")
	evalCmd.Flags().StringVar(&evalOutputDir, "output", "", "output directory for results")
}
