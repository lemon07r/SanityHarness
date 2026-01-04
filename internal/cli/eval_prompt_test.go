package cli

import (
	"strings"
	"testing"

	"github.com/lemon07r/sanityharness/internal/task"
)

func TestBuildAgentPromptIncludesKeyInfo(t *testing.T) {
	t.Parallel()

	tt := &task.Task{
		Slug:        "demo",
		Name:        "Demo Task",
		Language:    task.Go,
		Tier:        "core",
		Difficulty:  "hard",
		Description: "Implement the thing.",
		Files: task.TaskFiles{
			Stub: []string{"demo.go.txt"},
			Test: []string{"demo_test.go.txt"},
		},
	}

	prompt := buildAgentPrompt(tt)

	for _, s := range []string{
		"Description: " + tt.Description,
		"Tier:",
		"Difficulty:",
		"Stub/solution files: demo.go",
		"Test files:          demo_test.go",
	} {
		if !strings.Contains(prompt, s) {
			t.Fatalf("prompt missing %q\n\nPrompt:\n%s", s, prompt)
		}
	}

	if strings.Contains(prompt, ".txt") {
		t.Fatalf("prompt should not include .txt filenames\n\nPrompt:\n%s", prompt)
	}
}
