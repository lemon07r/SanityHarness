// Package runner provides the main task runner orchestration.
package runner

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/mount"

	"github.com/lemon07r/sanityharness/internal/config"
	errsummary "github.com/lemon07r/sanityharness/internal/errors"
	"github.com/lemon07r/sanityharness/internal/result"
	"github.com/lemon07r/sanityharness/internal/task"
)

// Runner orchestrates task execution.
type Runner struct {
	cfg        *config.Config
	taskLoader *task.Loader
	docker     *DockerClient
	logger     *slog.Logger
}

// NewRunner creates a new runner.
func NewRunner(cfg *config.Config, tasksFS embed.FS, tasksDir string, logger *slog.Logger) (*Runner, error) {
	docker, err := NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &Runner{
		cfg:        cfg,
		taskLoader: task.NewLoader(tasksFS, tasksDir),
		docker:     docker,
		logger:     logger,
	}, nil
}

// ResolveTaskRef resolves a task reference, which can be either a bare slug (if unambiguous)
// or a canonical ID in the form "<language>/<slug>".
func (r *Runner) ResolveTaskRef(ref string) (*task.Task, error) {
	tasks, err := r.ListTasks()
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}

	t, err := task.ResolveRef(tasks, ref)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// Close cleans up runner resources.
func (r *Runner) Close() error {
	return r.docker.Close()
}

func (r *Runner) cacheMountsForLanguage(lang task.Language) ([]mount.Mount, error) {
	// Cache directory lives alongside the workspace/session directories.
	// It is safe to delete at any time; it only improves performance.
	var mounts []mount.Mount

	ensureMount := func(hostRel, containerPath string) error {
		hostAbs, err := filepath.Abs(hostRel)
		if err != nil {
			return fmt.Errorf("resolving cache dir %s: %w", hostRel, err)
		}
		if err := os.MkdirAll(hostAbs, 0755); err != nil {
			return fmt.Errorf("creating cache dir %s: %w", hostAbs, err)
		}
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: hostAbs,
			Target: containerPath,
		})
		return nil
	}

	switch lang {
	case task.Go:
		if err := ensureMount(filepath.Join(".sanity-cache", "go", "gocache"), "/tmp/sanity-go-build-cache"); err != nil {
			return nil, err
		}
		if err := ensureMount(filepath.Join(".sanity-cache", "go", "gomodcache"), "/tmp/sanity-go-mod-cache"); err != nil {
			return nil, err
		}

	case task.Rust:
		if err := ensureMount(filepath.Join(".sanity-cache", "rust", "cargo-home"), "/tmp/sanity-cargo-home"); err != nil {
			return nil, err
		}
		if err := ensureMount(filepath.Join(".sanity-cache", "rust", "cargo-target"), "/tmp/sanity-cargo-target"); err != nil {
			return nil, err
		}

	case task.TypeScript:
		if err := ensureMount(filepath.Join(".sanity-cache", "typescript", "npm-cache"), "/tmp/sanity-npm-cache"); err != nil {
			return nil, err
		}

	case task.Kotlin:
		if err := ensureMount(filepath.Join(".sanity-cache", "kotlin", "gradle-home"), "/tmp/sanity-gradle-home"); err != nil {
			return nil, err
		}

	case task.Dart:
		if err := ensureMount(filepath.Join(".sanity-cache", "dart", "pub-cache"), "/tmp/sanity-pub-cache"); err != nil {
			return nil, err
		}

	case task.Zig:
		if err := ensureMount(filepath.Join(".sanity-cache", "zig", "zig-cache"), "/tmp/.zig-cache"); err != nil {
			return nil, err
		}
	}

	return mounts, nil
}

// RunOptions configures a task run.
type RunOptions struct {
	TaskSlug     string
	Task         *task.Task // If set, use this task directly instead of loading by slug
	WatchMode    bool
	MaxAttempts  int
	Timeout      int
	OutputDir    string
	WorkspaceDir string

	// ValidationCommand overrides the task's default validation command when set.
	// The first element is the command, followed by args.
	ValidationCommand []string
}

// Run executes a task and returns the session result.
func (r *Runner) Run(ctx context.Context, opts RunOptions) (*result.Session, error) {
	// Load the task (or use provided one)
	var t *task.Task
	var err error
	if opts.Task != nil {
		t = opts.Task
	} else {
		t, err = r.taskLoader.Load(opts.TaskSlug)
		if err != nil {
			return nil, fmt.Errorf("loading task: %w", err)
		}
	}

	// Set defaults
	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = r.cfg.Harness.MaxAttempts
	}
	if opts.Timeout == 0 {
		if t.Timeout > 0 {
			opts.Timeout = t.Timeout
		} else {
			opts.Timeout = r.cfg.Harness.DefaultTimeout
		}
	}
	if opts.OutputDir == "" {
		opts.OutputDir = r.cfg.Harness.SessionDir
	}

	// Get image for language
	imageName := r.cfg.ImageForLanguage(string(t.Language))
	if imageName == "" {
		return nil, fmt.Errorf("no image configured for language: %s", t.Language)
	}

	// Ensure image is available
	r.logger.Info("ensuring container image", "image", imageName)
	if err := r.docker.EnsureImage(ctx, imageName, r.cfg.Docker.AutoPull); err != nil {
		return nil, fmt.Errorf("ensuring image: %w", err)
	}

	// Create session first so we can put workspace inside session directory
	session := result.NewSession(t.Slug, string(t.Language), result.SessionConfig{
		Timeout:     opts.Timeout,
		MaxAttempts: opts.MaxAttempts,
		WatchMode:   opts.WatchMode,
		Image:       imageName,
	})

	// Determine workspace directory - now inside the session folder
	var workspaceDir string
	if opts.WorkspaceDir != "" {
		// Explicit workspace provided (e.g., from eval command)
		workspaceDir = opts.WorkspaceDir
	} else {
		// Default: put workspace inside session directory
		workspaceDir = filepath.Join(session.SessionDir(opts.OutputDir), "workspace")
	}
	workspaceDir, err = filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving workspace path: %w", err)
	}

	// Ensure workspace exists with task files
	if err := r.ensureWorkspace(t, workspaceDir); err != nil {
		return nil, fmt.Errorf("setting up workspace: %w", err)
	}

	// Create container
	r.logger.Info("creating container", "workspace", workspaceDir)
	containerUser := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	containerEnv := []string{"HOME=/tmp"}

	cacheMounts, err := r.cacheMountsForLanguage(t.Language)
	if err != nil {
		return nil, err
	}
	switch t.Language {
	case task.Rust:
		containerEnv = append(containerEnv,
			"CARGO_TARGET_DIR=/tmp/sanity-cargo-target",
			"CARGO_HOME=/tmp/sanity-cargo-home",
		)
	case task.Go:
		containerEnv = append(containerEnv,
			"GOCACHE=/tmp/sanity-go-build-cache",
			"GOMODCACHE=/tmp/sanity-go-mod-cache",
		)
	case task.TypeScript:
		containerEnv = append(containerEnv,
			"npm_config_cache=/tmp/sanity-npm-cache",
		)
	case task.Kotlin:
		containerEnv = append(containerEnv,
			"GRADLE_USER_HOME=/tmp/sanity-gradle-home",
		)
	case task.Dart:
		containerEnv = append(containerEnv,
			"PUB_CACHE=/tmp/sanity-pub-cache",
		)
	}
	containerID, err := r.docker.CreateContainer(ctx, ContainerConfig{
		Image:        imageName,
		WorkspaceDir: workspaceDir,
		Name:         fmt.Sprintf("sanity-%s-%s-%d", t.Language, t.Slug, time.Now().UnixNano()),
		User:         containerUser,
		Env:          containerEnv,
		Mounts:       cacheMounts,
	})
	if err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}
	defer func() {
		r.logger.Debug("cleaning up container", "id", containerID[:12])
		_ = r.docker.RemoveContainer(context.Background(), containerID, true)
	}()

	// Start container
	if err := r.docker.StartContainer(ctx, containerID); err != nil {
		return nil, fmt.Errorf("starting container: %w", err)
	}

	// Create error summarizer
	summarizer := errsummary.NewSummarizer(string(t.Language))

	// Touch stub files to invalidate build cache (prevents false positives from stale cached binaries).
	// This is necessary because Cargo uses mtime-based fingerprinting - if an agent doesn't modify
	// the stub file, Cargo may reuse a cached binary from a previous successful run.
	if err := r.touchStubFiles(workspaceDir, t); err != nil {
		r.logger.Warn("failed to touch stub files", "error", err)
	}

	// Run validation
	if opts.WatchMode {
		err = r.runWatchMode(ctx, t, containerID, session, summarizer, workspaceDir, opts)
	} else {
		err = r.runSingle(ctx, t, containerID, session, summarizer, opts)
	}

	// Complete session
	session.Complete()

	// Capture final code
	if err := r.captureWorkspace(workspaceDir, t, session); err != nil {
		r.logger.Warn("failed to capture workspace", "error", err)
	}

	// Save session
	if saveErr := session.Save(opts.OutputDir); saveErr != nil {
		r.logger.Error("failed to save session", "error", saveErr)
	}

	return session, err
}

// runSingle runs a single validation attempt.
func (r *Runner) runSingle(ctx context.Context, t *task.Task, containerID string, session *result.Session, summarizer *errsummary.Summarizer, opts RunOptions) error {
	cmd := t.ValidationCommand()
	if len(opts.ValidationCommand) > 0 {
		cmd = opts.ValidationCommand
	}

	execResult, err := r.docker.Exec(ctx, containerID, cmd, "/workspace", time.Duration(opts.Timeout)*time.Second)
	if err != nil {
		session.Status = result.StatusError
		return fmt.Errorf("executing validation: %w", err)
	}

	errorSummary := summarizer.Summarize(execResult.Combined)
	session.AddAttempt(execResult.ExitCode, execResult.Duration, execResult.Combined, errorSummary)

	// Print result
	fmt.Print(result.FormatTerminal(session, session.LastAttempt(), false))

	return nil
}

// runWatchMode runs validation in watch mode.
func (r *Runner) runWatchMode(ctx context.Context, t *task.Task, containerID string, session *result.Session, summarizer *errsummary.Summarizer, workspaceDir string, opts RunOptions) error {
	// Initial run
	if err := r.runAttempt(ctx, t, containerID, session, summarizer, opts); err != nil {
		return err
	}

	// If passed on first attempt, we're done
	if session.Passed() {
		return nil
	}

	// Setup watcher
	attemptCh := make(chan struct{}, 1)
	watcher := NewWatcher(workspaceDir, 200*time.Millisecond, func() {
		select {
		case attemptCh <- struct{}{}:
		default:
		}
	}, r.logger)

	// Start watching in background
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	go func() {
		if err := watcher.Watch(watchCtx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.Error("watcher error", "error", err)
		}
	}()

	// Wait for changes and run attempts
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-attemptCh:
			if len(session.Attempts) >= opts.MaxAttempts {
				r.logger.Info("max attempts reached", "attempts", len(session.Attempts))
				return nil
			}

			if err := r.runAttempt(ctx, t, containerID, session, summarizer, opts); err != nil {
				return err
			}

			if session.Passed() {
				return nil
			}
		}
	}
}

// runAttempt runs a single validation attempt and updates the session.
func (r *Runner) runAttempt(ctx context.Context, t *task.Task, containerID string, session *result.Session, summarizer *errsummary.Summarizer, opts RunOptions) error {
	r.logger.Debug("running validation attempt", "attempt", len(session.Attempts)+1)

	cmd := t.ValidationCommand()
	if len(opts.ValidationCommand) > 0 {
		cmd = opts.ValidationCommand
	}

	execResult, err := r.docker.Exec(ctx, containerID, cmd, "/workspace", time.Duration(opts.Timeout)*time.Second)
	if err != nil {
		session.Status = result.StatusTimeout
		return fmt.Errorf("executing validation: %w", err)
	}

	errorSummary := summarizer.Summarize(execResult.Combined)
	session.AddAttempt(execResult.ExitCode, execResult.Duration, execResult.Combined, errorSummary)

	// Print result
	fmt.Print(result.FormatTerminal(session, session.LastAttempt(), true))

	return nil
}

// ensureWorkspace creates the workspace directory and copies task files.
func (r *Runner) ensureWorkspace(t *task.Task, dir string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Check if workspace already has files (don't overwrite)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory: %w", err)
	}
	if len(entries) > 0 {
		r.logger.Debug("workspace already exists, not overwriting", "dir", dir)
		return nil
	}

	return r.copyTaskFiles(t, dir, t.VisibleFiles())
}

// captureWorkspace reads the workspace files into the session.
func (r *Runner) captureWorkspace(dir string, t *task.Task, session *result.Session) error {
	var captureErrors []string
	for _, filename := range t.Files.Stub {
		// Use the stripped filename when reading from workspace
		destFilename := task.StripTxtExtension(filename)
		path := filepath.Join(dir, destFilename)
		content, err := os.ReadFile(path)
		if err != nil {
			r.logger.Warn("failed to read stub file", "file", destFilename, "error", err)
			captureErrors = append(captureErrors, destFilename)
			continue
		}
		session.FinalCode[destFilename] = string(content)
	}
	if len(captureErrors) > 0 {
		return fmt.Errorf("failed to capture %d file(s): %v", len(captureErrors), captureErrors)
	}
	return nil
}

// copyTaskFiles copies task files to the destination directory.
// This is a helper used by ensureWorkspace, InitWorkspace, and InitWorkspaceForTask.
func (r *Runner) copyTaskFiles(t *task.Task, destDir string, files []string) error {
	for _, filename := range files {
		content, err := r.taskLoader.ReadTaskFile(t, filename)
		if err != nil {
			return fmt.Errorf("reading task file %s: %w", filename, err)
		}

		// Strip .txt extension for workspace files
		destFilename := task.StripTxtExtension(filename)
		destPath := filepath.Join(destDir, destFilename)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", destFilename, err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing file %s: %w", destFilename, err)
		}
	}
	return nil
}

// touchStubFiles updates the modification time of all stub files in the workspace.
// This invalidates Cargo's mtime-based build cache, forcing recompilation of user code.
// Without this, if an agent crashes without modifying the stub, Cargo may reuse a
// cached binary from a previous successful run, causing false positives.
func (r *Runner) touchStubFiles(workspaceDir string, t *task.Task) error {
	now := time.Now()
	for _, filename := range t.Files.Stub {
		destFilename := task.StripTxtExtension(filename)
		path := filepath.Join(workspaceDir, destFilename)
		if err := os.Chtimes(path, now, now); err != nil {
			return fmt.Errorf("touching stub file %s: %w", destFilename, err)
		}
	}
	return nil
}

// InitWorkspaceForTask initializes a workspace for a specific task object.
func (r *Runner) InitWorkspaceForTask(t *task.Task, outputDir string) error {
	if outputDir == "" {
		outputDir = t.Slug
	}

	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Create directory
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Check if already has files
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("reading directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("directory is not empty: %s", absDir)
	}

	return r.copyTaskFiles(t, absDir, t.VisibleFiles())
}

// ListTasks returns all available tasks.
func (r *Runner) ListTasks() ([]*task.Task, error) {
	return r.taskLoader.LoadAll()
}

// ListTasksByLanguage returns tasks filtered by language.
func (r *Runner) ListTasksByLanguage(lang task.Language) ([]*task.Task, error) {
	return r.taskLoader.LoadByLanguage(lang)
}
