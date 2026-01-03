package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/internal/task"
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
		taskRef := args[0]

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		t, err := r.ResolveTaskRef(taskRef)
		if err != nil {
			return err
		}

		outputDir := initOutput
		if outputDir == "" {
			if _, _, ok := task.ParseTaskID(taskRef); ok {
				outputDir = fmt.Sprintf("%s-%s", t.Language, t.Slug)
			} else {
				outputDir = t.Slug
			}
		}

		if err := r.InitWorkspaceForTask(t, outputDir); err != nil {
			return err
		}

		fmt.Printf("Initialized workspace for %s in %s\n", t.ID(), outputDir)
		fmt.Println("\nNext steps:")
		fmt.Printf("  1. Implement the solution in %s\n", outputDir)
		fmt.Printf("  2. Run: sanity run %s\n", t.ID())
		fmt.Printf("     Or with watch mode: sanity run %s --watch\n", t.ID())

		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "", "output directory (default: ./<task-slug>)")
}
