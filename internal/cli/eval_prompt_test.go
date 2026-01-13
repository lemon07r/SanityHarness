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

	prompt := buildAgentPrompt(tt, false)

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

func TestBuildAgentPromptWithMCPTools(t *testing.T) {
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

	// Test without MCP tools
	promptWithoutMCP := buildAgentPrompt(tt, false)
	if strings.Contains(promptWithoutMCP, "MCP TOOLS:") {
		t.Fatalf("prompt without MCP tools should not contain MCP section\n\nPrompt:\n%s", promptWithoutMCP)
	}

	// Test with MCP tools
	promptWithMCP := buildAgentPrompt(tt, true)
	for _, s := range []string{
		"MCP TOOLS:",
		"Model Context Protocol",
		"file reading tools",
		"code search tools",
		"Do NOT guess at implementation details",
	} {
		if !strings.Contains(promptWithMCP, s) {
			t.Fatalf("prompt with MCP tools missing %q\n\nPrompt:\n%s", s, promptWithMCP)
		}
	}
}
