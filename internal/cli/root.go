// Package cli provides the command-line interface for SanityHarness.
package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/config"
)

var (
	cfgFile  string
	tasksDir string
	verbose  bool
	cfg      *config.Config
	logger   *slog.Logger
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "sanity",
	Short: "Lightweight evaluation harness for coding agents",
	Long: `SanityHarness is a lightweight, fast evaluation harness for coding agents.

It runs "Compact Hard Problems" in isolated Docker containers, providing
high-signal feedback for testing agent capabilities in Go, Rust, TypeScript,
Kotlin, Dart, and Zig.

Features:
  - Fast execution via container reuse (<10 seconds per task)
  - Watch mode for tight feedback loops
  - File-based agent interface (works with any agent)
  - Error summarization per language`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for commands that don't need it
		if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "version" {
			return nil
		}

		// Setup logger
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}))

		// Load config
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		return nil
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Check for exitError to get specific exit code
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./sanity.toml)")
	rootCmd.PersistentFlags().StringVar(&tasksDir, "tasks-dir", "", "external tasks directory (for development)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Add subcommands
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(compareCmd)
	rootCmd.AddCommand(batchCmd)
}

// Version information (set by build flags).
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sanity version %s\n", Version)
		fmt.Printf("  commit: %s\n", Commit)
		fmt.Printf("  built:  %s\n", BuildDate)
	},
}
