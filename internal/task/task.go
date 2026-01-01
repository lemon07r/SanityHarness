// Package task provides task definition and loading for SanityHarness.
package task

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Language represents a supported programming language.
type Language string

const (
	Go         Language = "go"
	Rust       Language = "rust"
	TypeScript Language = "typescript"
)

// Task represents a single evaluation task.
type Task struct {
	Slug        string     `toml:"slug"`
	Name        string     `toml:"name"`
	Language    Language   `toml:"language"`
	Difficulty  string     `toml:"difficulty"`
	Description string     `toml:"description"`
	Timeout     int        `toml:"timeout,omitempty"`
	Files       TaskFiles  `toml:"files"`
	Validation  Validation `toml:"validation"`
}

// TaskFiles specifies the files that make up a task.
type TaskFiles struct {
	Stub    []string `toml:"stub"`
	Test    []string `toml:"test"`
	Support []string `toml:"support,omitempty"`
}

// Validation specifies how to validate a task solution.
type Validation struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

// AllFiles returns all files associated with this task.
func (t *Task) AllFiles() []string {
	files := make([]string, 0, len(t.Files.Stub)+len(t.Files.Test)+len(t.Files.Support))
	files = append(files, t.Files.Stub...)
	files = append(files, t.Files.Test...)
	files = append(files, t.Files.Support...)
	return files
}

// ValidationCommand returns the full command to run for validation.
func (t *Task) ValidationCommand() []string {
	cmd := []string{t.Validation.Command}
	cmd = append(cmd, t.Validation.Args...)
	return cmd
}

// Loader handles loading tasks from embedded or external sources.
type Loader struct {
	embeddedFS  embed.FS
	externalDir string
}

// NewLoader creates a new task loader.
// If externalDir is provided, it takes precedence over embedded tasks.
func NewLoader(embeddedFS embed.FS, externalDir string) *Loader {
	return &Loader{
		embeddedFS:  embeddedFS,
		externalDir: externalDir,
	}
}

// LoadAll loads all available tasks.
func (l *Loader) LoadAll() ([]*Task, error) {
	if l.externalDir != "" {
		return l.loadFromDir(l.externalDir)
	}
	return l.loadFromEmbed()
}

// Load loads a specific task by slug.
func (l *Loader) Load(slug string) (*Task, error) {
	tasks, err := l.LoadAll()
	if err != nil {
		return nil, err
	}

	for _, t := range tasks {
		if t.Slug == slug {
			return t, nil
		}
	}

	return nil, fmt.Errorf("task not found: %s", slug)
}

// LoadByLanguage loads all tasks for a specific language.
func (l *Loader) LoadByLanguage(lang Language) ([]*Task, error) {
	all, err := l.LoadAll()
	if err != nil {
		return nil, err
	}

	var filtered []*Task
	for _, t := range all {
		if t.Language == lang {
			filtered = append(filtered, t)
		}
	}

	return filtered, nil
}

// loadFromEmbed loads tasks from the embedded filesystem.
func (l *Loader) loadFromEmbed() ([]*Task, error) {
	var tasks []*Task

	languages := []string{"go", "rust", "typescript"}
	for _, lang := range languages {
		langDir := lang // The embed is from tasks/, so paths are relative to that
		entries, err := fs.ReadDir(l.embeddedFS, langDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("reading %s tasks: %w", lang, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			taskPath := filepath.Join(langDir, entry.Name(), "task.toml")
			data, err := l.embeddedFS.ReadFile(taskPath)
			if err != nil {
				continue
			}

			var task Task
			if err := toml.Unmarshal(data, &task); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", taskPath, err)
			}

			tasks = append(tasks, &task)
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Language != tasks[j].Language {
			return tasks[i].Language < tasks[j].Language
		}
		return tasks[i].Slug < tasks[j].Slug
	})

	return tasks, nil
}

// loadFromDir loads tasks from an external directory.
func (l *Loader) loadFromDir(dir string) ([]*Task, error) {
	var tasks []*Task

	languages := []string{"go", "rust", "typescript"}
	for _, lang := range languages {
		langDir := filepath.Join(dir, lang)
		entries, err := os.ReadDir(langDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			taskPath := filepath.Join(langDir, entry.Name(), "task.toml")
			var task Task
			if _, err := toml.DecodeFile(taskPath, &task); err != nil {
				continue
			}

			tasks = append(tasks, &task)
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Language != tasks[j].Language {
			return tasks[i].Language < tasks[j].Language
		}
		return tasks[i].Slug < tasks[j].Slug
	})

	return tasks, nil
}

// GetTaskDir returns the directory path for a task.
// For embedded tasks, this returns the path relative to the embedded FS root.
// For external tasks, this returns the absolute filesystem path.
func (l *Loader) GetTaskDir(task *Task) string {
	if l.externalDir != "" {
		return filepath.Join(l.externalDir, string(task.Language), task.Slug)
	}
	// Embedded paths are relative to the tasks/ directory (where embed.go lives)
	return filepath.Join(string(task.Language), task.Slug)
}

// ReadTaskFile reads a file from a task's directory.
func (l *Loader) ReadTaskFile(task *Task, filename string) ([]byte, error) {
	taskDir := l.GetTaskDir(task)
	// Use forward slashes for embed.FS paths (cross-platform)
	filePath := taskDir + "/" + filename

	if l.externalDir != "" {
		absPath := filepath.Join(taskDir, filename)
		return os.ReadFile(absPath)
	}

	return l.embeddedFS.ReadFile(filePath)
}

// ParseLanguage converts a string to a Language type.
func ParseLanguage(s string) (Language, error) {
	switch strings.ToLower(s) {
	case "go":
		return Go, nil
	case "rust":
		return Rust, nil
	case "typescript", "ts":
		return TypeScript, nil
	default:
		return "", fmt.Errorf("unknown language: %s", s)
	}
}

// String returns the string representation of a Language.
func (l Language) String() string {
	return string(l)
}

// Extension returns the primary file extension for a language.
func (l Language) Extension() string {
	switch l {
	case Go:
		return ".go"
	case Rust:
		return ".rs"
	case TypeScript:
		return ".ts"
	default:
		return ""
	}
}
