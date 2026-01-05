package runner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches a directory for file changes.
type Watcher struct {
	dir      string
	debounce time.Duration
	onChange func()
	logger   *slog.Logger
}

// NewWatcher creates a new file watcher.
func NewWatcher(dir string, debounce time.Duration, onChange func(), logger *slog.Logger) *Watcher {
	return &Watcher{
		dir:      dir,
		debounce: debounce,
		onChange: onChange,
		logger:   logger,
	}
}

// Watch starts watching for file changes and blocks until context is cancelled.
func (w *Watcher) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer func() { _ = watcher.Close() }()

	// Add the directory
	if err := watcher.Add(w.dir); err != nil {
		return fmt.Errorf("watching directory %s: %w", w.dir, err)
	}

	// Also watch subdirectories
	if err := w.addSubdirs(watcher, w.dir); err != nil {
		w.logger.Warn("failed to watch some subdirectories", "error", err)
	}

	var debounceTimer *time.Timer
	var timerMu sync.Mutex
	stopped := false

	// Helper to safely stop the timer
	stopTimer := func() {
		timerMu.Lock()
		defer timerMu.Unlock()
		stopped = true
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}

	for {
		select {
		case <-ctx.Done():
			stopTimer()
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				stopTimer()
				return nil
			}

			// Filter for relevant events
			if !w.isRelevantEvent(event) {
				continue
			}

			w.logger.Debug("file change detected", "file", event.Name, "op", event.Op.String())

			// Debounce: reset timer on each event
			timerMu.Lock()
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			if !stopped {
				debounceTimer = time.AfterFunc(w.debounce, func() {
					timerMu.Lock()
					defer timerMu.Unlock()
					if !stopped {
						w.onChange()
					}
				})
			}
			timerMu.Unlock()

		case err, ok := <-watcher.Errors:
			if !ok {
				stopTimer()
				return nil
			}
			w.logger.Error("watcher error", "error", err)
		}
	}
}

// isRelevantEvent checks if a file event should trigger a rebuild.
func (w *Watcher) isRelevantEvent(event fsnotify.Event) bool {
	// Only care about writes and creates
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
		return false
	}

	// Ignore hidden files and directories
	name := filepath.Base(event.Name)
	if strings.HasPrefix(name, ".") {
		return false
	}

	// Ignore common non-source files
	ext := filepath.Ext(event.Name)
	ignoredExts := map[string]bool{
		".swp": true, ".swo": true, ".swn": true, // Vim
		".tmp": true, ".bak": true,
		".log": true,
	}
	return !ignoredExts[ext]
}

// addSubdirs recursively adds subdirectories to the watcher.
func (w *Watcher) addSubdirs(watcher *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") && path != dir {
				return filepath.SkipDir
			}

			if err := watcher.Add(path); err != nil {
				w.logger.Debug("failed to watch directory", "path", path, "error", err)
			}
		}

		return nil
	})
}
