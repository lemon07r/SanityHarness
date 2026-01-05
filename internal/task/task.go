// Package task provides task definition and loading for SanityHarness.
package task

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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
	Kotlin     Language = "kotlin"
	Dart       Language = "dart"
	Zig        Language = "zig"
)

// Task represents a single evaluation task.
type Task struct {
	Slug         string     `json:"slug"                    toml:"slug"`
	Name         string     `json:"name"                    toml:"name"`
	Language     Language   `json:"language"                toml:"language"`
	Tier         string     `json:"tier,omitempty"          toml:"tier,omitempty"`
	Difficulty   string     `json:"difficulty"              toml:"difficulty"`
	Description  string     `json:"description"             toml:"description"`
	Timeout      int        `json:"timeout,omitempty"       toml:"timeout,omitempty"`
	AgentTimeout int        `json:"agent_timeout,omitempty" toml:"agent_timeout,omitempty"`
	Files        TaskFiles  `json:"files"                   toml:"files"`
	Validation   Validation `json:"validation"              toml:"validation"`
}

// ID returns the canonical task identifier in the form "<language>/<slug>".
func (t *Task) ID() string {
	return fmt.Sprintf("%s/%s", t.Language, t.Slug)
}

// TaskFiles specifies the files that make up a task.
type TaskFiles struct {
	Stub       []string `json:"stub"                  toml:"stub"`
	Test       []string `json:"test"                  toml:"test"`
	HiddenTest []string `json:"hidden_test,omitempty" toml:"hidden_test,omitempty"`
	Support    []string `json:"support,omitempty"     toml:"support,omitempty"`
}

// Validation specifies how to validate a task solution.
type Validation struct {
	Command string   `json:"command" toml:"command"`
	Args    []string `json:"args"    toml:"args"`
}

// AllFiles returns all files associated with this task.
func (t *Task) AllFiles() []string {
	files := make([]string, 0, len(t.Files.Stub)+len(t.Files.Test)+len(t.Files.Support))
	files = append(files, t.Files.Stub...)
	files = append(files, t.Files.Test...)
	files = append(files, t.Files.Support...)
	return files
}

// HiddenTestFiles returns the hidden test files for this task.
func (t *Task) HiddenTestFiles() []string {
	return t.Files.HiddenTest
}

// ValidationCommand returns the full command to run for validation.
func (t *Task) ValidationCommand() []string {
	cmd := []string{t.Validation.Command}
	cmd = append(cmd, t.Validation.Args...)
	return cmd
}

// Validate checks that required task fields are present.
func (t *Task) Validate() error {
	if t.Slug == "" {
		return errors.New("task slug is required")
	}
	if t.Language == "" {
		return errors.New("task language is required")
	}
	if t.Validation.Command == "" {
		return errors.New("task validation command is required")
	}
	if len(t.Files.Stub) == 0 {
		return fmt.Errorf("task %s has no stub files", t.Slug)
	}
	if len(t.Files.Test) == 0 {
		return fmt.Errorf("task %s has no test files", t.Slug)
	}
	return nil
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

	var matches []*Task
	for _, t := range tasks {
		if t.Slug == slug {
			matches = append(matches, t)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("task not found: %s", slug)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, 0, len(matches))
		for _, t := range matches {
			ids = append(ids, t.ID())
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("task slug %q is ambiguous; use one of: %s", slug, strings.Join(ids, ", "))
	}
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

	languages := []string{"go", "rust", "typescript", "kotlin", "dart", "zig"}
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

			taskPath := path.Join(langDir, entry.Name(), "task.toml")
			data, err := l.embeddedFS.ReadFile(taskPath)
			if err != nil {
				continue
			}

			var task Task
			if err := toml.Unmarshal(data, &task); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", taskPath, err)
			}
			if task.Tier == "" {
				task.Tier = "core"
			}
			if err := task.Validate(); err != nil {
				return nil, fmt.Errorf("invalid task %s: %w", taskPath, err)
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

	languages := []string{"go", "rust", "typescript", "kotlin", "dart", "zig"}
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
				continue // Skip unparseable tasks in external dir
			}
			if task.Tier == "" {
				task.Tier = "core"
			}
			if err := task.Validate(); err != nil {
				continue // Skip invalid tasks in external dir
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
	return path.Join(string(task.Language), task.Slug)
}

// ReadTaskFile reads a file from a task's directory.
func (l *Loader) ReadTaskFile(task *Task, filename string) ([]byte, error) {
	taskDir := l.GetTaskDir(task)

	if l.externalDir != "" {
		absPath := filepath.Join(taskDir, filename)
		return os.ReadFile(absPath)
	}

	filePath := path.Join(taskDir, filename)
	return l.embeddedFS.ReadFile(filePath)
}

// ParseTaskID parses a canonical task identifier in the form "<language>/<slug>".
// Returns ok=false if the input is not in task ID form.
func ParseTaskID(s string) (lang Language, slug string, ok bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	parsedLang, err := ParseLanguage(parts[0])
	if err != nil {
		return "", "", false
	}

	return parsedLang, parts[1], true
}

// ResolveRef resolves a task reference which can be either:
//   - canonical ID: "<language>/<slug>"
//   - bare slug: "<slug>" (must be unambiguous within tasks)
func ResolveRef(tasks []*Task, ref string) (*Task, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("task reference is empty")
	}

	if lang, slug, ok := ParseTaskID(ref); ok {
		for _, t := range tasks {
			if t.Language == lang && t.Slug == slug {
				return t, nil
			}
		}
		return nil, fmt.Errorf("task not found: %s/%s", lang, slug)
	}

	var matches []*Task
	for _, t := range tasks {
		if t.Slug == ref {
			matches = append(matches, t)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("task not found: %s", ref)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, 0, len(matches))
		for _, t := range matches {
			ids = append(ids, t.ID())
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("task slug %q is ambiguous; use one of: %s", ref, strings.Join(ids, ", "))
	}
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
	case "kotlin", "kt":
		return Kotlin, nil
	case "dart":
		return Dart, nil
	case "zig":
		return Zig, nil
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
	case Kotlin:
		return ".kt"
	case Dart:
		return ".dart"
	case Zig:
		return ".zig"
	default:
		return ""
	}
}

// StripTxtExtension removes the .txt suffix from a filename if present.
// Task files are stored with .txt extension in the embedded FS to prevent
// language toolchains from treating them as source files.
func StripTxtExtension(filename string) string {
	if strings.HasSuffix(filename, ".txt") {
		return strings.TrimSuffix(filename, ".txt")
	}
	return filename
}
