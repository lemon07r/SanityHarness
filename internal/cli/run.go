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

The workspace is created inside the session directory by default.
Use --workspace to specify an existing workspace or custom location.

In watch mode (--watch), the harness monitors the workspace for file changes
and automatically re-runs validation after each change.

Examples:
  sanity run bank-account
  sanity run bank-account --watch
  sanity run bank-account --watch --max-attempts 10
  sanity run bank-account -w ./my-workspace`,
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

		// Setup context with cancellation
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigCh) // Prevent goroutine leak
		go func() {
			select {
			case <-sigCh:
				fmt.Println("\nReceived interrupt, stopping...")
				cancel()
			case <-ctx.Done():
				// Context cancelled, exit goroutine
			}
		}()

		// Run the task - workspace is created inside session by default
		session, err := r.Run(ctx, runner.RunOptions{
			Task:         t,
			WatchMode:    runWatch,
			MaxAttempts:  runMaxAttempts,
			Timeout:      runTimeout,
			OutputDir:    runOutput,
			WorkspaceDir: runWorkspace, // Empty means session/workspace/
		})

		// Print final result
		if session != nil {
			fmt.Print(result.FormatFinalResult(session))
			outputDir := runOutput
			if outputDir == "" {
				outputDir = cfg.Harness.SessionDir
			}
			fmt.Printf(" Session saved to: %s\n\n", session.SessionDir(outputDir))
		}

		if err != nil {
			if ctx.Err() != nil {
				return nil // Graceful shutdown
			}
			return err
		}

		// Return error to indicate non-zero exit (handled in Execute)
		if session != nil && !session.Passed() {
			return &exitError{code: 1}
		}

		return nil
	},
}

// exitError is a sentinel error for non-zero exit codes.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit code %d", e.code)
}

func init() {
	runCmd.Flags().BoolVar(&runWatch, "watch", false, "watch mode: re-run on file changes")
	runCmd.Flags().IntVar(&runMaxAttempts, "max-attempts", 0, "maximum attempts (default from config)")
	runCmd.Flags().IntVar(&runTimeout, "timeout", 0, "timeout per attempt in seconds (default from config)")
	runCmd.Flags().StringVar(&runOutput, "output", "", "session output directory (default from config)")
	runCmd.Flags().StringVarP(&runWorkspace, "workspace", "w", "", "workspace directory (default: inside session)")
}
