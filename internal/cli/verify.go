package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zeebo/blake3"

	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <eval-dir>",
	Short: "Verify integrity of an eval submission",
	Long: `Verifies the integrity of an eval submission by checking hashes.

This command checks:
  1. Results hash - ensures summary.json wasn't modified after generation
  2. Task hashes - ensures tasks match your embedded version (same harness)

No tests are re-run; this only validates hash integrity.

Examples:
  sanity verify ./eval-results/2026-01-07T120000-gemini
  sanity verify /path/to/submission`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		evalDir := args[0]

		// Load attestation.json
		attestationPath := filepath.Join(evalDir, "attestation.json")
		attestationData, err := os.ReadFile(attestationPath)
		if err != nil {
			return fmt.Errorf("reading attestation.json: %w", err)
		}

		var attestation EvalAttestation
		if err := json.Unmarshal(attestationData, &attestation); err != nil {
			return fmt.Errorf("parsing attestation.json: %w", err)
		}

		// Load summary.json
		summaryPath := filepath.Join(evalDir, "summary.json")
		summaryData, err := os.ReadFile(summaryPath)
		if err != nil {
			return fmt.Errorf("reading summary.json: %w", err)
		}

		var summary EvalSummary
		if err := json.Unmarshal(summaryData, &summary); err != nil {
			return fmt.Errorf("parsing summary.json: %w", err)
		}

		// Print header
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(" SANITY HARNESS - Submission Verification")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		// Show submission info
		fmt.Printf(" Agent:     %s\n", attestation.Eval.Agent)
		if attestation.Eval.Model != "" {
			fmt.Printf(" Model:     %s\n", attestation.Eval.Model)
		}
		fmt.Printf(" Timestamp: %s\n", attestation.Eval.Timestamp)
		fmt.Printf(" Harness:   %s (built %s)\n", attestation.Harness.Version, attestation.Harness.BuildDate)
		fmt.Printf(" Tasks:     %d\n", len(attestation.Tasks))
		fmt.Println()

		passed := 0
		failed := 0
		warnings := 0

		// 1. Verify results hash
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Println(" Verifying Results Integrity")
		fmt.Println("─────────────────────────────────────────────────────────────")

		resultsJSON, _ := json.Marshal(summary.Results)
		computedResultsHash := verifyHashBytes(resultsJSON)

		if computedResultsHash == attestation.Integrity.ResultsHash {
			fmt.Println(" ✓ Results hash matches - summary.json is unmodified")
			passed++
		} else {
			fmt.Println(" ✗ Results hash MISMATCH - summary.json may have been tampered with")
			fmt.Printf("   Expected: %s\n", attestation.Integrity.ResultsHash)
			fmt.Printf("   Got:      %s\n", computedResultsHash)
			failed++
		}
		fmt.Println()

		// 2. Verify task hashes against our embedded tasks
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Println(" Verifying Task Hashes")
		fmt.Println("─────────────────────────────────────────────────────────────")

		loader := task.NewLoader(tasks.FS, tasksDir)
		allTasks, err := loader.LoadAll()
		if err != nil {
			return fmt.Errorf("loading tasks: %w", err)
		}

		// Build task map
		taskMap := make(map[string]*task.Task)
		for _, t := range allTasks {
			taskMap[t.ID()] = t
		}

		taskMatches := 0
		taskMismatches := 0
		taskMissing := 0

		for taskID, taskAttest := range attestation.Tasks {
			t := taskMap[taskID]
			if t == nil {
				fmt.Printf(" ? %s - not found in this harness version\n", taskID)
				taskMissing++
				continue
			}

			// Compute hash of our embedded task files
			var taskFileContents []byte
			for _, f := range append(append(t.Files.Stub, t.Files.Test...), t.Files.Support...) {
				if content, err := loader.ReadTaskFile(t, f); err == nil {
					taskFileContents = append(taskFileContents, content...)
				}
			}
			ourTaskHash := verifyHashBytes(taskFileContents)

			if ourTaskHash == taskAttest.TaskHash {
				taskMatches++
			} else {
				fmt.Printf(" ✗ %s - hash mismatch (different task version)\n", taskID)
				fmt.Printf("     theirs: %s\n", taskAttest.TaskHash)
				fmt.Printf("     ours:   %s\n", ourTaskHash)
				taskMismatches++
			}
		}

		if taskMismatches == 0 && taskMissing == 0 {
			fmt.Printf(" ✓ All %d task hashes match - same task versions used\n", taskMatches)
			passed++
		} else {
			if taskMismatches > 0 {
				fmt.Printf(" ✗ %d task(s) have different hashes\n", taskMismatches)
				failed++
			}
			if taskMissing > 0 {
				fmt.Printf(" ? %d task(s) not found in this harness\n", taskMissing)
				warnings++
			}
			if taskMatches > 0 {
				fmt.Printf(" ✓ %d task(s) match\n", taskMatches)
			}
		}
		fmt.Println()

		// 3. Version check
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Println(" Version Compatibility")
		fmt.Println("─────────────────────────────────────────────────────────────")

		if attestation.Harness.Version == Version {
			fmt.Printf(" ✓ Harness version matches (%s)\n", Version)
			passed++
		} else {
			fmt.Printf(" ! Harness version differs (theirs: %s, yours: %s)\n",
				attestation.Harness.Version, Version)
			fmt.Println("   Task hashes may differ due to version mismatch")
			warnings++
		}
		fmt.Println()

		// Summary
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(" VERIFICATION SUMMARY")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		if failed == 0 {
			fmt.Printf(" ✓ PASSED: %d checks passed", passed)
			if warnings > 0 {
				fmt.Printf(", %d warnings", warnings)
			}
			fmt.Println()
			fmt.Println()
			fmt.Println(" The submission appears to be authentic and unmodified.")
		} else {
			fmt.Printf(" ✗ FAILED: %d checks failed, %d passed", failed, passed)
			if warnings > 0 {
				fmt.Printf(", %d warnings", warnings)
			}
			fmt.Println()
			fmt.Println()
			fmt.Println(" The submission may have been tampered with or uses different task versions.")
		}

		// Show claimed results
		fmt.Println()
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Println(" Claimed Results")
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Printf(" Pass Rate: %.1f%% (%d/%d)\n", summary.PassRate, summary.Passed, summary.Total)
		fmt.Println()

		return nil
	},
}

// verifyHashBytes returns the BLAKE3 hash of data as a prefixed hex string.
func verifyHashBytes(data []byte) string {
	h := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(h[:])
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
