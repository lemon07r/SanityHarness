package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/result"
)

var showJSON bool

var showCmd = &cobra.Command{
	Use:   "show <session-path>",
	Short: "Display session results",
	Long: `Shows the results of a previous evaluation session.

Example:
  sanity show sessions/bank-account-2024-12-30T143022
  sanity show sessions/bank-account-2024-12-30T143022 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionPath := args[0]

		// Load result.json
		resultPath := filepath.Join(sessionPath, "result.json")
		data, err := os.ReadFile(resultPath)
		if err != nil {
			return fmt.Errorf("reading session: %w", err)
		}

		var session result.Session
		if err := json.Unmarshal(data, &session); err != nil {
			return fmt.Errorf("parsing session: %w", err)
		}

		if showJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(session)
		}

		// Display formatted output
		return displaySession(&session, sessionPath)
	},
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "output as JSON")
}

func displaySession(session *result.Session, path string) error {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf(" SESSION: %s\n", session.ID)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	fmt.Printf(" Status:    %s %s\n", result.StatusEmoji[session.Status], strings.ToUpper(string(session.Status)))
	fmt.Printf(" Task:      %s/%s\n", session.Language, session.TaskSlug)
	fmt.Printf(" Attempts:  %d\n", len(session.Attempts))
	fmt.Printf(" Duration:  %s\n", session.TotalTime.Round(1e6)) // Round to milliseconds
	fmt.Printf(" Started:   %s\n", session.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf(" Completed: %s\n", session.CompletedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	fmt.Println(" ─────────────────────────────────────────────────────────")
	fmt.Println(" ATTEMPTS")
	fmt.Println(" ─────────────────────────────────────────────────────────")

	for _, attempt := range session.Attempts {
		status := "❌"
		if attempt.Passed {
			status = "✅"
		}

		fmt.Printf("\n Attempt %d: %s (exit: %d, duration: %s)\n",
			attempt.Number, status, attempt.ExitCode,
			attempt.Duration.Round(1e6))

		if len(attempt.ErrorSummary) > 0 && !attempt.Passed {
			fmt.Println(" Errors:")
			for _, e := range attempt.ErrorSummary {
				fmt.Printf("   • %s\n", e)
			}
		}
	}

	fmt.Println()
	fmt.Println(" ─────────────────────────────────────────────────────────")
	fmt.Println(" FILES")
	fmt.Println(" ─────────────────────────────────────────────────────────")
	fmt.Printf(" Report:    %s/report.md\n", path)
	fmt.Printf(" Result:    %s/result.json\n", path)
	fmt.Printf(" Workspace: %s/workspace/\n", path)
	fmt.Printf(" Logs:      %s/logs/\n", path)
	fmt.Println()

	return nil
}
