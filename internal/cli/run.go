package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/result"
	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/tasks"
)

var (
	runWatch       bool
	runMaxAttempts int
	runTimeout     int
	runOutput      string
	runWorkspace   string
)

var runCmd = &cobra.Command{
	Use:   "run <task>",
	Short: "Run evaluation for a task",
	Long: `Executes the validation tests for a task in an isolated Docker container.

In watch mode (--watch), the harness monitors the workspace for file changes
and automatically re-runs validation after each change.

Examples:
  sanity run bank-account
  sanity run bank-account --watch
  sanity run bank-account --watch --max-attempts 10
  sanity run bank-account -w ./my-workspace`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskSlug := args[0]

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer r.Close()

		// Setup context with cancellation
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nReceived interrupt, stopping...")
			cancel()
		}()

		// Run the task
		session, err := r.Run(ctx, runner.RunOptions{
			TaskSlug:     taskSlug,
			WatchMode:    runWatch,
			MaxAttempts:  runMaxAttempts,
			Timeout:      runTimeout,
			OutputDir:    runOutput,
			WorkspaceDir: runWorkspace,
		})

		// Print final result
		if session != nil {
			fmt.Print(result.FormatFinalResult(session))
			fmt.Printf(" Session saved to: %s\n\n", session.SessionDir(cfg.Harness.SessionDir))
		}

		if err != nil {
			if ctx.Err() != nil {
				return nil // Graceful shutdown
			}
			return err
		}

		// Exit with appropriate code
		if session != nil && !session.Passed() {
			os.Exit(1)
		}

		return nil
	},
}

func init() {
	runCmd.Flags().BoolVar(&runWatch, "watch", false, "watch mode: re-run on file changes")
	runCmd.Flags().IntVar(&runMaxAttempts, "max-attempts", 0, "maximum attempts (default from config)")
	runCmd.Flags().IntVar(&runTimeout, "timeout", 0, "timeout per attempt in seconds (default from config)")
	runCmd.Flags().StringVar(&runOutput, "output", "", "session output directory (default from config)")
	runCmd.Flags().StringVarP(&runWorkspace, "workspace", "w", "", "workspace directory (default: ./<task-slug>)")
}
