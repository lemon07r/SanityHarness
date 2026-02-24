package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var compareOutputFile string

var compareCmd = &cobra.Command{
	Use:   "compare <dir> [dir...]",
	Short: "Compare multiple eval results side-by-side",
	Long: `Compare two or more eval result directories and produce a side-by-side
comparison table showing pass rates, weighted scores, and per-task results.

Supports glob patterns for convenient selection of multiple directories.`,
	Example: `  sanity compare eval-results/*-gemini eval-results/*-codex
  sanity compare ./run-a ./run-b ./run-c
  sanity compare eval-results/multi-2026-02-21T024300/codex-gpt-5.2 eval-results/multi-2026-02-21T024300/opencode-kimi-k2.5`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var summaries []EvalSummary
		for _, dir := range args {
			s, err := loadSummaryFromDir(dir)
			if err != nil {
				return fmt.Errorf("loading summary from %s: %w", dir, err)
			}
			summaries = append(summaries, *s)
		}

		comparison := generateComparison(summaries)

		// Write JSON if output file specified.
		if compareOutputFile != "" {
			writeComparisonJSON(filepath.Dir(compareOutputFile), comparison)
			fmt.Printf(" Comparison saved to: %s\n", compareOutputFile)
		}

		// Always write to stdout.
		fmt.Print(buildComparisonReport(comparison))
		return nil
	},
}

func init() {
	compareCmd.Flags().StringVarP(&compareOutputFile, "output", "o", "", "write comparison JSON to file")
}

// loadSummaryFromDir loads an EvalSummary from a directory's summary.json.
func loadSummaryFromDir(dir string) (*EvalSummary, error) {
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return nil, fmt.Errorf("reading summary.json: %w", err)
	}
	var s EvalSummary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing summary.json: %w", err)
	}
	return &s, nil
}
