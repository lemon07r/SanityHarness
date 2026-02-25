package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

// BatchConfig is the top-level structure of a batch TOML file.
type BatchConfig struct {
	Defaults BatchDefaults `toml:"defaults"`
	Runs     []BatchRun    `toml:"runs"`
}

// BatchDefaults holds default settings applied to all runs unless overridden.
type BatchDefaults struct {
	Tier           string `toml:"tier"`
	Difficulty     string `toml:"difficulty"`
	Lang           string `toml:"lang"`
	Tasks          string `toml:"tasks"`
	Timeout        int    `toml:"timeout"`
	Parallel       int    `toml:"parallel"`
	KeepWorkspaces bool   `toml:"keep_workspaces"`
	UseMCPTools    bool   `toml:"use_mcp_tools"`
	UseSkills      bool   `toml:"use_skills"`
	DisableMCP     bool   `toml:"disable_mcp"`
	NoSandbox      bool   `toml:"no_sandbox"`
	Legacy         bool   `toml:"legacy"`
	Repeat         int    `toml:"repeat"`
}

// BatchRun defines a single run entry in the batch config.
type BatchRun struct {
	Agent     string `toml:"agent"`
	Model     string `toml:"model"`
	Reasoning string `toml:"reasoning"`
	Timeout   int    `toml:"timeout"`
	Repeat    int    `toml:"repeat"`
}

var (
	batchConfigFile string
	batchRepeat     int
	batchDryRun     bool
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Run multiple eval configurations from a TOML config file",
	Long: `Execute multiple agent/model configurations defined in a TOML file.
Each run produces its own output directory under a shared umbrella directory.

The TOML file supports defaults that apply to all runs, with per-run overrides.`,
	Example: `  sanity batch --config runs.toml
  sanity batch --config runs.toml --repeat 3
  sanity batch --config runs.toml --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if batchConfigFile == "" {
			return fmt.Errorf("--config is required")
		}

		data, err := os.ReadFile(batchConfigFile)
		if err != nil {
			return fmt.Errorf("reading config file: %w", err)
		}

		var batchCfg BatchConfig
		if err := toml.Unmarshal(data, &batchCfg); err != nil {
			return fmt.Errorf("parsing config file: %w", err)
		}

		if len(batchCfg.Runs) == 0 {
			return fmt.Errorf("no runs defined in config file")
		}

		// Build shared config from defaults.
		defaults := batchCfg.Defaults
		shared := SharedConfig{
			Tier:           defaults.Tier,
			Difficulty:     defaults.Difficulty,
			Lang:           defaults.Lang,
			Tasks:          defaults.Tasks,
			Timeout:        defaults.Timeout,
			Parallel:       defaults.Parallel,
			KeepWorkspaces: defaults.KeepWorkspaces,
			UseMCPTools:    defaults.UseMCPTools,
			UseSkills:      defaults.UseSkills,
			DisableMCP:     defaults.DisableMCP,
			NoSandbox:      defaults.NoSandbox,
			Legacy:         defaults.Legacy,
		}
		if shared.Timeout == 0 {
			if cfg != nil && cfg.Harness.DefaultTimeout > 0 {
				shared.Timeout = cfg.Harness.DefaultTimeout
			} else {
				shared.Timeout = 600
			}
		}

		// Determine repeat count: CLI flag > defaults > 1.
		repeat := 1
		if batchRepeat > 1 {
			repeat = batchRepeat
		} else if defaults.Repeat > 1 {
			repeat = defaults.Repeat
		}

		// Build specs from runs.
		var specs []RunSpec
		var perRunTimeouts []int
		for _, run := range batchCfg.Runs {
			if run.Agent == "" {
				return fmt.Errorf("each run must specify an agent")
			}
			specs = append(specs, RunSpec{
				Agent:     run.Agent,
				Model:     run.Model,
				Reasoning: run.Reasoning,
			})
			timeout := shared.Timeout
			if run.Timeout > 0 {
				timeout = run.Timeout
			}
			perRunTimeouts = append(perRunTimeouts, timeout)
		}

		// Validate all agents.
		if !batchDryRun {
			for _, spec := range specs {
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

		// Dry-run mode.
		if batchDryRun {
			fmt.Println()
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println(" SANITY HARNESS - Batch Dry Run")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println()
			fmt.Printf(" Config:  %s\n", batchConfigFile)
			fmt.Printf(" Runs:    %d\n", len(specs))
			fmt.Printf(" Repeat:  %d\n", repeat)
			fmt.Printf(" Total:   %d\n", len(specs)*repeat)
			fmt.Println()
			for i, spec := range specs {
				fmt.Printf(" %d. Agent: %s", i+1, spec.Agent)
				if spec.Model != "" {
					fmt.Printf(", Model: %s", spec.Model)
				}
				if spec.Reasoning != "" {
					fmt.Printf(", Reasoning: %s", spec.Reasoning)
				}
				fmt.Printf(", Timeout: %ds\n", perRunTimeouts[i])
			}
			fmt.Println()
			return nil
		}

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
		umbrellaDir := filepath.Join("eval-results", fmt.Sprintf("batch-%s", timestamp))
		if err := os.MkdirAll(umbrellaDir, 0o755); err != nil {
			return fmt.Errorf("creating umbrella directory: %w", err)
		}

		writeMultiRunConfig(umbrellaDir, specs, shared, repeat)

		var allSummaries []runResult
		for specIdx, spec := range specs {
			// Apply per-run timeout override.
			runShared := shared
			runShared.Timeout = perRunTimeouts[specIdx]

			for rep := 1; rep <= repeat; rep++ {
				if checkInterrupted(interruptCtx) {
					updateMultiRunState(umbrellaDir, allSummaries, specs, repeat, true)
					printMultiRunResumeCommand(umbrellaDir)
					return nil
				}

				runDir := multiRunSubdir(umbrellaDir, spec, specIdx, rep, repeat)
				summary, _, err := evalRunSingle(
					interruptCtx, spec, runShared, allTasks, allTasks,
					runDir, timestamp, r, false, nil, nil, nil, nil,
				)
				rr := runResult{spec: spec, repeat: rep, summary: summary}
				if err != nil {
					logger.Warn("run failed", "agent", spec.Agent, "repeat", rep, "error", err)
					rr.err = err
				}
				allSummaries = append(allSummaries, rr)
				updateMultiRunState(umbrellaDir, allSummaries, specs, repeat, false)
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

		if repeat > 1 {
			writeRepeatStats(umbrellaDir, specs, allSummaries, repeat)
		}

		fmt.Printf("\n Batch results saved to: %s\n\n", umbrellaDir)
		return nil
	},
}

func init() {
	batchCmd.Flags().StringVar(&batchConfigFile, "config", "", "path to batch TOML config file (required)")
	batchCmd.Flags().IntVar(&batchRepeat, "repeat", 1, "repeat each configuration N times")
	batchCmd.Flags().BoolVar(&batchDryRun, "dry-run", false, "show what would be run without executing")
	_ = batchCmd.MarkFlagRequired("config")
}
