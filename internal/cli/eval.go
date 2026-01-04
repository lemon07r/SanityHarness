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
	"time"

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
	evalTimeout        int
	evalOutputDir      string
	evalKeepWorkspaces bool
)

// EvalResult holds the result of evaluating a single task.
type EvalResult struct {
	Task         string  `json:"task"`
	Language     string  `json:"language"`
	Passed       bool    `json:"passed"`
	Attempts     int     `json:"attempts"`
	Duration     float64 `json:"duration_seconds"`
	Error        string  `json:"error,omitempty"`
	WorkspaceDir string  `json:"-"` // Not serialized, used for cleanup
}

// EvalSummary holds the overall evaluation summary.
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
		defer func() { _ = r.Close() }()

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
		Task:     t.ID(),
		Language: string(t.Language),
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
	agentErr := cmd.Run()
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
			validationCmd = append(validationCmd, stripTxtExtension(filename))
		}
	}

	session, err := r.Run(ctx, runner.RunOptions{
		Task:              t, // Pass task directly to avoid slug collision
		WorkspaceDir:      workspaceDir,
		Timeout:           timeout,
		MaxAttempts:       1,
		ValidationCommand: validationCmd,
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

ENVIRONMENT:
- Tests run automatically in a Docker container with %s toolchain pre-installed
- You do NOT need to run tests yourself - just write the code
- Do NOT search for or install language toolchains/SDKs

YOUR TASK:
1. Read the stub file (contains function signatures with panic()/todo!() placeholders)
2. Read the test file to understand expected behavior and edge cases
3. Implement all functions, replacing placeholders with working code
4. Handle all edge cases shown in the tests
5. Ensure thread-safety if the tests use concurrent operations

RULES:
- ONLY edit the stub/solution source file(s)
- Do NOT modify test files or support files (go.mod, Cargo.toml, pubspec.yaml, build.zig)
- You may add new helper source files if needed
- Evaluation fails if you modify protected files`,
		t.Name, t.Language, t.Language)
}

func stripTxtExtension(filename string) string {
	if strings.HasSuffix(filename, ".txt") {
		return strings.TrimSuffix(filename, ".txt")
	}
	return filename
}

func detectModifiedTaskFiles(loader *task.Loader, t *task.Task, workspaceDir string) ([]string, error) {
	var modified []string
	for _, filename := range append(append([]string{}, t.Files.Test...), t.Files.Support...) {
		want, err := loader.ReadTaskFile(t, filename)
		if err != nil {
			return nil, fmt.Errorf("reading canonical %s: %w", filename, err)
		}

		workspacePath := filepath.Join(workspaceDir, stripTxtExtension(filename))
		got, err := os.ReadFile(workspacePath)
		if err != nil {
			modified = append(modified, stripTxtExtension(filename))
			continue
		}
		if !bytes.Equal(got, want) {
			modified = append(modified, stripTxtExtension(filename))
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

		destFilename := stripTxtExtension(filename)
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
	evalCmd.Flags().IntVar(&evalTimeout, "timeout", 120, "timeout per task in seconds")
	evalCmd.Flags().StringVar(&evalOutputDir, "output", "", "output directory for results")
	evalCmd.Flags().BoolVar(&evalKeepWorkspaces, "keep-workspaces", false, "keep workspace directories after evaluation")
}
