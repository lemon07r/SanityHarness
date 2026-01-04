package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var (
	cleanForce      bool
	cleanWorkspaces bool
	cleanSessions   bool
	cleanEval       bool
	cleanAll        bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up workspace directories and other generated files",
	Long: `Remove workspace directories created by 'sanity init' or 'sanity run',
session directories, and eval results.

By default, shows what would be deleted and asks for confirmation.
Use --force to skip confirmation.

Examples:
  sanity clean                    # Interactive cleanup of workspaces
  sanity clean --workspaces       # Clean only workspace directories
  sanity clean --sessions         # Clean only session directories  
  sanity clean --eval             # Clean only eval-results
  sanity clean --all              # Clean everything
  sanity clean --force            # Skip confirmation prompts`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to workspaces if no specific flag is set
		if !cleanWorkspaces && !cleanSessions && !cleanEval && !cleanAll {
			cleanWorkspaces = true
		}

		if cleanAll {
			cleanWorkspaces = true
			cleanSessions = true
			cleanEval = true
		}

		var toDelete []string

		// Find workspace directories
		if cleanWorkspaces {
			workspaces, err := findWorkspaceDirectories()
			if err != nil {
				return fmt.Errorf("finding workspaces: %w", err)
			}
			toDelete = append(toDelete, workspaces...)
		}

		// Find session directories
		if cleanSessions {
			if info, err := os.Stat("sessions"); err == nil && info.IsDir() {
				toDelete = append(toDelete, "sessions")
			}
		}

		// Find eval-results directories
		if cleanEval {
			if info, err := os.Stat("eval-results"); err == nil && info.IsDir() {
				toDelete = append(toDelete, "eval-results")
			}
		}

		if len(toDelete) == 0 {
			fmt.Println("Nothing to clean.")
			return nil
		}

		// Show what will be deleted
		fmt.Println("The following directories will be deleted:")
		fmt.Println()
		for _, dir := range toDelete {
			fmt.Printf("  %s\n", dir)
		}
		fmt.Println()

		// Confirm unless --force
		if !cleanForce {
			fmt.Print("Delete these directories? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		// Delete directories
		deleted := 0
		for _, dir := range toDelete {
			if err := os.RemoveAll(dir); err != nil {
				fmt.Printf("  Failed to delete %s: %v\n", dir, err)
			} else {
				fmt.Printf("  Deleted %s\n", dir)
				deleted++
			}
		}

		fmt.Printf("\nCleaned up %d directories.\n", deleted)
		return nil
	},
}

// findWorkspaceDirectories finds workspace directories in the current directory
// by matching against known task slugs.
func findWorkspaceDirectories() ([]string, error) {
	// Load all tasks to get their slugs
	loader := task.NewLoader(tasks.FS, "")
	allTasks, err := loader.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("loading tasks: %w", err)
	}

	// Build a set of patterns to match
	patterns := make(map[string]bool)
	for _, t := range allTasks {
		// Match both "<lang>-<slug>" and "<slug>" patterns
		patterns[fmt.Sprintf("%s-%s", t.Language, t.Slug)] = true
		patterns[t.Slug] = true
	}

	// Scan current directory for matching directories
	var workspaces []string
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories and known project directories
		if strings.HasPrefix(name, ".") {
			continue
		}
		if isProjectDirectory(name) {
			continue
		}
		// Check if this matches a workspace pattern
		if patterns[name] {
			workspaces = append(workspaces, name)
		}
	}

	// Also look for directories matching "<lang>-*" patterns
	langPrefixes := []string{"go-", "rust-", "typescript-", "kotlin-", "dart-", "zig-"}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, prefix := range langPrefixes {
			if strings.HasPrefix(name, prefix) && !contains(workspaces, name) {
				// Check if the slug part matches a known task
				slug := strings.TrimPrefix(name, prefix)
				for _, t := range allTasks {
					if t.Slug == slug {
						workspaces = append(workspaces, name)
						break
					}
				}
			}
		}
	}

	return workspaces, nil
}

// isProjectDirectory returns true if the name is a known project directory.
func isProjectDirectory(name string) bool {
	projectDirs := map[string]bool{
		"bin":          true,
		"cmd":          true,
		"containers":   true,
		"internal":     true,
		"tasks":        true,
		"sessions":     true,
		"eval-results": true,
		"vendor":       true,
		"node_modules": true,
	}
	return projectDirs[name]
}

// contains checks if a slice contains a string.
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func init() {
	cleanCmd.Flags().BoolVar(&cleanForce, "force", false, "skip confirmation prompts")
	cleanCmd.Flags().BoolVar(&cleanWorkspaces, "workspaces", false, "clean workspace directories")
	cleanCmd.Flags().BoolVar(&cleanSessions, "sessions", false, "clean sessions directory")
	cleanCmd.Flags().BoolVar(&cleanEval, "eval", false, "clean eval-results directory")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "clean everything")
}
