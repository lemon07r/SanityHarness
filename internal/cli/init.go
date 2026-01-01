package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/tasks"
)

var initOutput string

var initCmd = &cobra.Command{
	Use:   "init <task>",
	Short: "Initialize a workspace for a task",
	Long: `Creates a new directory with the task's stub files for an agent to work on.

Example:
  sanity init bank-account
  sanity init bank-account -o ./my-workspace`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskSlug := args[0]

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer r.Close()

		if err := r.InitWorkspace(taskSlug, initOutput); err != nil {
			return err
		}

		outputDir := initOutput
		if outputDir == "" {
			outputDir = taskSlug
		}

		fmt.Printf("Initialized workspace for %s in ./%s\n", taskSlug, outputDir)
		fmt.Println("\nNext steps:")
		fmt.Printf("  1. Implement the solution in ./%s\n", outputDir)
		fmt.Printf("  2. Run: sanity run %s\n", taskSlug)
		fmt.Printf("     Or with watch mode: sanity run %s --watch\n", taskSlug)

		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "", "output directory (default: ./<task-slug>)")
}
