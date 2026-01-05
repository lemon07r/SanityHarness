package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var initOutput string

var initCmd = &cobra.Command{
	Use:   "init <task>",
	Short: "Initialize a workspace for a task",
	Long: `Creates a new directory with the task's stub files for manual development.

Use this when you want to work on a task interactively and run tests repeatedly.
The workspace is created in the current directory by default.

To run tests against your workspace:
  sanity run <task> -w <workspace-dir>

Example:
  sanity init bank-account
  sanity init go/bank-account -o ./my-workspace`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskRef := args[0]

		// Load task without creating a Docker client
		loader := task.NewLoader(tasks.FS, tasksDir)
		allTasks, err := loader.LoadAll()
		if err != nil {
			return fmt.Errorf("loading tasks: %w", err)
		}

		t, err := task.ResolveRef(allTasks, taskRef)
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

		// Create workspace directory
		absDir, err := filepath.Abs(outputDir)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}

		if err := os.MkdirAll(absDir, 0755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		// Check if already initialized
		entries, err := os.ReadDir(absDir)
		if err != nil {
			return fmt.Errorf("reading directory: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory %s is not empty", outputDir)
		}

		// Copy task files
		for _, filename := range t.AllFiles() {
			content, err := loader.ReadTaskFile(t, filename)
			if err != nil {
				return fmt.Errorf("reading task file %s: %w", filename, err)
			}

			// Strip .txt extension for workspace files
			destFilename := filename
			if len(destFilename) > 4 && destFilename[len(destFilename)-4:] == ".txt" {
				destFilename = destFilename[:len(destFilename)-4]
			}

			destPath := filepath.Join(absDir, destFilename)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", destFilename, err)
			}
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return fmt.Errorf("writing file %s: %w", destFilename, err)
			}
		}

		fmt.Printf("Initialized workspace for %s in %s\n", t.ID(), outputDir)
		fmt.Println("\nNext steps:")
		fmt.Printf("  1. Implement the solution in %s\n", outputDir)
		fmt.Printf("  2. Run: sanity run %s -w %s\n", t.ID(), outputDir)
		fmt.Printf("     Or with watch mode: sanity run %s -w %s --watch\n", t.ID(), outputDir)

		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "", "output directory (default: ./<task-slug>)")
}
