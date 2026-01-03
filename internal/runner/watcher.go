package runner

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
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
		return err
	}
	defer func() { _ = watcher.Close() }()

	// Add the directory
	if err := watcher.Add(w.dir); err != nil {
		return err
	}

	// Also watch subdirectories
	if err := w.addSubdirs(watcher, w.dir); err != nil {
		w.logger.Warn("failed to watch some subdirectories", "error", err)
	}

	var debounceTimer *time.Timer

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Filter for relevant events
			if !w.isRelevantEvent(event) {
				continue
			}

			w.logger.Debug("file change detected", "file", event.Name, "op", event.Op.String())

			// Debounce: reset timer on each event
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(w.debounce, func() {
				w.onChange()
			})

		case err, ok := <-watcher.Errors:
			if !ok {
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
