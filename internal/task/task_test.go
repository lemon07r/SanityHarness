package task

import (
	"testing"
)

func TestParseTaskID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		ok   bool
		lang Language
		slug string
	}{
		{name: "canonical", in: "go/bank-account", ok: true, lang: Go, slug: "bank-account"},
		{name: "canonical whitespace", in: "  rust/regex-lite  ", ok: true, lang: Rust, slug: "regex-lite"},
		{name: "missing slug", in: "go/", ok: false},
		{name: "missing lang", in: "/bank-account", ok: false},
		{name: "unknown lang", in: "python/foo", ok: false},
		{name: "too many slashes", in: "go/a/b", ok: false},
		{name: "no slash", in: "bank-account", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lang, slug, ok := ParseTaskID(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if lang != tc.lang {
				t.Fatalf("lang=%q, want %q", lang, tc.lang)
			}
			if slug != tc.slug {
				t.Fatalf("slug=%q, want %q", slug, tc.slug)
			}
		})
	}
}

func TestResolveRef(t *testing.T) {
	t.Parallel()

	tasks := []*Task{
		{Slug: "react", Language: Go},
		{Slug: "react", Language: TypeScript},
		{Slug: "bank-account", Language: Go},
	}

	t.Run("canonical id", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveRef(tasks, "go/react")
		if err != nil {
			t.Fatalf("ResolveRef error: %v", err)
		}
		if got.Language != Go || got.Slug != "react" {
			t.Fatalf("got %s, want go/react", got.ID())
		}
	})

	t.Run("bare slug unambiguous", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveRef(tasks, "bank-account")
		if err != nil {
			t.Fatalf("ResolveRef error: %v", err)
		}
		if got.Language != Go || got.Slug != "bank-account" {
			t.Fatalf("got %s, want go/bank-account", got.ID())
		}
	})

	t.Run("bare slug ambiguous", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveRef(tasks, "react")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "task slug \"react\" is ambiguous; use one of: go/react, typescript/react"
		if err.Error() != want {
			t.Fatalf("error=%q, want %q", err.Error(), want)
		}
	})

	t.Run("empty ref", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveRef(tasks, "")
		if err == nil {
			t.Fatalf("expected error for empty ref")
		}
	})

	t.Run("whitespace ref", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveRef(tasks, "   ")
		if err == nil {
			t.Fatalf("expected error for whitespace ref")
		}
	})

	t.Run("not found canonical", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveRef(tasks, "rust/unknown")
		if err == nil {
			t.Fatalf("expected error for not found task")
		}
	})

	t.Run("not found bare", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveRef(tasks, "unknown-slug")
		if err == nil {
			t.Fatalf("expected error for not found task")
		}
	})
}

func TestParseLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Language
		wantErr bool
	}{
		{name: "go lowercase", input: "go", want: Go},
		{name: "go uppercase", input: "GO", want: Go},
		{name: "rust", input: "rust", want: Rust},
		{name: "typescript", input: "typescript", want: TypeScript},
		{name: "ts alias", input: "ts", want: TypeScript},
		{name: "kotlin", input: "kotlin", want: Kotlin},
		{name: "kt alias", input: "kt", want: Kotlin},
		{name: "dart", input: "dart", want: Dart},
		{name: "zig", input: "zig", want: Zig},
		{name: "unknown", input: "python", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseLanguage(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLanguage(%q) error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseLanguage(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestLanguageExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		lang Language
		want string
	}{
		{Go, ".go"},
		{Rust, ".rs"},
		{TypeScript, ".ts"},
		{Kotlin, ".kt"},
		{Dart, ".dart"},
		{Zig, ".zig"},
		{Language("unknown"), ""},
	}

	for _, tc := range tests {
		t.Run(string(tc.lang), func(t *testing.T) {
			t.Parallel()

			got := tc.lang.Extension()
			if got != tc.want {
				t.Fatalf("Language(%q).Extension() = %q, want %q", tc.lang, got, tc.want)
			}
		})
	}
}

func TestLanguageString(t *testing.T) {
	t.Parallel()

	if Go.String() != "go" {
		t.Fatalf("Go.String() = %q, want %q", Go.String(), "go")
	}
}

func TestTaskID(t *testing.T) {
	t.Parallel()

	task := &Task{Slug: "bank-account", Language: Go}
	if task.ID() != "go/bank-account" {
		t.Fatalf("Task.ID() = %q, want %q", task.ID(), "go/bank-account")
	}
}

func TestTaskAllFiles(t *testing.T) {
	t.Parallel()

	task := &Task{
		Files: TaskFiles{
			Stub:    []string{"main.go"},
			Test:    []string{"main_test.go"},
			Support: []string{"go.mod"},
		},
	}

	files := task.AllFiles()
	if len(files) != 3 {
		t.Fatalf("AllFiles() returned %d files, want 3", len(files))
	}

	expected := []string{"main.go", "main_test.go", "go.mod"}
	for i, want := range expected {
		if files[i] != want {
			t.Fatalf("AllFiles()[%d] = %q, want %q", i, files[i], want)
		}
	}
}

func TestTaskHiddenTestFiles(t *testing.T) {
	t.Parallel()

	task := &Task{
		Files: TaskFiles{
			HiddenTest: []string{"hidden_test.go"},
		},
	}

	files := task.HiddenTestFiles()
	if len(files) != 1 || files[0] != "hidden_test.go" {
		t.Fatalf("HiddenTestFiles() = %v, want [hidden_test.go]", files)
	}
}

func TestTaskValidationCommand(t *testing.T) {
	t.Parallel()

	task := &Task{
		Validation: Validation{
			Command: "go",
			Args:    []string{"test", "-race", "./..."},
		},
	}

	cmd := task.ValidationCommand()
	expected := []string{"go", "test", "-race", "./..."}
	if len(cmd) != len(expected) {
		t.Fatalf("ValidationCommand() returned %d args, want %d", len(cmd), len(expected))
	}
	for i, want := range expected {
		if cmd[i] != want {
			t.Fatalf("ValidationCommand()[%d] = %q, want %q", i, cmd[i], want)
		}
	}
}

func TestTaskValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		task    Task
		wantErr bool
	}{
		{
			name: "valid task",
			task: Task{
				Slug:     "test-task",
				Language: Go,
				Files: TaskFiles{
					Stub: []string{"main.go"},
					Test: []string{"main_test.go"},
				},
				Validation: Validation{Command: "go", Args: []string{"test"}},
			},
			wantErr: false,
		},
		{
			name: "missing slug",
			task: Task{
				Language: Go,
				Files: TaskFiles{
					Stub: []string{"main.go"},
					Test: []string{"main_test.go"},
				},
				Validation: Validation{Command: "go"},
			},
			wantErr: true,
		},
		{
			name: "missing language",
			task: Task{
				Slug: "test",
				Files: TaskFiles{
					Stub: []string{"main.go"},
					Test: []string{"main_test.go"},
				},
				Validation: Validation{Command: "go"},
			},
			wantErr: true,
		},
		{
			name: "missing validation command",
			task: Task{
				Slug:     "test",
				Language: Go,
				Files: TaskFiles{
					Stub: []string{"main.go"},
					Test: []string{"main_test.go"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing stub files",
			task: Task{
				Slug:     "test",
				Language: Go,
				Files: TaskFiles{
					Test: []string{"main_test.go"},
				},
				Validation: Validation{Command: "go"},
			},
			wantErr: true,
		},
		{
			name: "missing test files",
			task: Task{
				Slug:     "test",
				Language: Go,
				Files: TaskFiles{
					Stub: []string{"main.go"},
				},
				Validation: Validation{Command: "go"},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.task.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
