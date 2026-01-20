package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/lemon07r/sanityharness/internal/config"
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

func TestBuildAgentCommandDisableMCP(t *testing.T) {
	t.Parallel()

	agentCfg := &config.AgentConfig{
		Command: "opencode",
		Args:    []string{"run", "{prompt}"},
	}

	ctx := context.Background()

	// Test with disableMCP=true for opencode - should inject OPENCODE_CONFIG_CONTENT
	cmd := buildAgentCommand(ctx, agentCfg, "test prompt", "", "", true, "opencode")

	found := false
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "OPENCODE_CONFIG_CONTENT=") {
			found = true
			if !strings.Contains(env, `"tools"`) {
				t.Error("expected tools config in OPENCODE_CONFIG_CONTENT")
			}
			if !strings.Contains(env, `"*_*":false`) {
				t.Error("expected *_* glob pattern to disable MCP tools")
			}
			break
		}
	}
	if !found {
		t.Error("expected OPENCODE_CONFIG_CONTENT to be set when disableMCP=true for opencode")
	}

	// Test with disableMCP=true for non-opencode agent - should not inject
	cmd2 := buildAgentCommand(ctx, agentCfg, "test prompt", "", "", true, "gemini")
	for _, env := range cmd2.Env {
		if strings.HasPrefix(env, "OPENCODE_CONFIG_CONTENT=") {
			t.Error("should not set OPENCODE_CONFIG_CONTENT for non-opencode agents")
		}
	}

	// Test with disableMCP=false for opencode - should not inject
	cmd3 := buildAgentCommand(ctx, agentCfg, "test prompt", "", "", false, "opencode")
	for _, env := range cmd3.Env {
		if strings.HasPrefix(env, "OPENCODE_CONFIG_CONTENT=") {
			t.Error("should not set OPENCODE_CONFIG_CONTENT when disableMCP=false")
		}
	}
}
