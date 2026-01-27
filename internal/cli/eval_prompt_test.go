package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestStripJSONComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no comments",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "single line comment",
			input:    "{\"key\": \"value\" // comment\n}",
			expected: "{\"key\": \"value\" \n}",
		},
		{
			name:     "multi line comment",
			input:    `{"key": /* comment */ "value"}`,
			expected: `{"key":  "value"}`,
		},
		{
			name:     "comment-like string in quotes",
			input:    `{"url": "https://example.com"}`,
			expected: `{"url": "https://example.com"}`,
		},
		{
			name:     "double slash in string should not be stripped",
			input:    `{"path": "//server/share"}`,
			expected: `{"path": "//server/share"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stripJSONComments([]byte(tc.input))
			if string(result) != tc.expected {
				t.Errorf("stripJSONComments(%q) = %q, want %q", tc.input, string(result), tc.expected)
			}
		})
	}
}

func TestDeepMergeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     map[string]any
		override map[string]any
		expected map[string]any
	}{
		{
			name:     "nil base",
			base:     nil,
			override: map[string]any{"key": "value"},
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "simple override",
			base:     map[string]any{"a": 1, "b": 2},
			override: map[string]any{"b": 3},
			expected: map[string]any{"a": 1, "b": 3},
		},
		{
			name: "nested merge",
			base: map[string]any{
				"outer": map[string]any{"inner1": 1, "inner2": 2},
			},
			override: map[string]any{
				"outer": map[string]any{"inner2": 3, "inner3": 4},
			},
			expected: map[string]any{
				"outer": map[string]any{"inner1": 1, "inner2": 3, "inner3": 4},
			},
		},
		{
			name: "tools override preserves other keys",
			base: map[string]any{
				"model": "google/gemini-3-flash",
				"provider": map[string]any{
					"google": map[string]any{"models": map[string]any{}},
				},
			},
			override: map[string]any{
				"tools": map[string]any{"*_*": false},
			},
			expected: map[string]any{
				"model": "google/gemini-3-flash",
				"provider": map[string]any{
					"google": map[string]any{"models": map[string]any{}},
				},
				"tools": map[string]any{"*_*": false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := deepMergeJSON(tc.base, tc.override)

			// Compare by marshaling to JSON
			resultJSON, _ := json.Marshal(result)
			expectedJSON, _ := json.Marshal(tc.expected)

			if string(resultJSON) != string(expectedJSON) {
				t.Errorf("deepMergeJSON() = %s, want %s", resultJSON, expectedJSON)
			}
		})
	}
}

func TestBuildOpenCodeMCPDisableConfig(t *testing.T) {
	t.Parallel()

	// This test verifies that the function always includes the MCP disable config
	config := buildOpenCodeMCPDisableConfig()

	if !strings.Contains(config, `"tools"`) {
		t.Error("config should contain tools key")
	}
	if !strings.Contains(config, `"*_*":false`) && !strings.Contains(config, `"*_*": false`) {
		t.Error("config should contain *_* disable pattern")
	}

	// Verify it's valid JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(config), &parsed); err != nil {
		t.Errorf("config should be valid JSON: %v", err)
	}
}

func TestReadOpenCodeConfigWithTempFile(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Create a temp directory to simulate XDG_CONFIG_HOME
	tmpDir := t.TempDir()
	opencodeCfgDir := filepath.Join(tmpDir, "opencode")
	if err := os.MkdirAll(opencodeCfgDir, 0o755); err != nil {
		t.Fatalf("failed to create temp config dir: %v", err)
	}

	// Write a test config
	testConfig := map[string]any{
		"model": "google/test-model",
		"provider": map[string]any{
			"google": map[string]any{
				"models": map[string]any{
					"test-model": map[string]any{"name": "Test Model"},
				},
			},
		},
	}
	configData, _ := json.Marshal(testConfig)
	configPath := filepath.Join(opencodeCfgDir, "opencode.json")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	// Set XDG_CONFIG_HOME to our temp directory
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Cleanup(func() {
		if originalXDG != "" {
			_ = os.Setenv("XDG_CONFIG_HOME", originalXDG)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
	})

	// Read the config
	cfg := readOpenCodeConfig()
	if cfg == nil {
		t.Fatal("expected to read config, got nil")
	}

	if cfg["model"] != "google/test-model" {
		t.Errorf("expected model to be 'google/test-model', got %v", cfg["model"])
	}
}

type agentCommandTestCase struct {
	name         string
	agentCfg     *config.AgentConfig
	prompt       string
	model        string
	reasoning    string
	disableMCP   bool
	agentName    string
	expectedArgs []string
}

func runAgentCommandTestCases(t *testing.T, tests []agentCommandTestCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			cmd := buildAgentCommand(ctx, tc.agentCfg, tc.prompt, tc.model, tc.reasoning, tc.disableMCP, tc.agentName)

			// cmd.Args[0] is the command itself (e.g., "agent"), skip it for comparison
			gotArgs := cmd.Args[1:]
			if !reflect.DeepEqual(gotArgs, tc.expectedArgs) {
				t.Errorf("args mismatch:\n  got:  %v\n  want: %v", gotArgs, tc.expectedArgs)
			}
		})
	}
}

func TestBuildAgentCommand_NoFlags(t *testing.T) {
	t.Parallel()

	runAgentCommandTestCases(t, []agentCommandTestCase{
		{
			name: "no_flags_empty_values",
			agentCfg: &config.AgentConfig{
				Command: "agent",
				Args:    []string{"run", "{prompt}"},
			},
			prompt:       "do the thing",
			expectedArgs: []string{"run", "do the thing"},
		},
	})
}

func TestBuildAgentCommand_ModelFlag(t *testing.T) {
	t.Parallel()

	runAgentCommandTestCases(t, []agentCommandTestCase{
		{
			name: "model_before_standard",
			agentCfg: &config.AgentConfig{
				Command:           "agent",
				Args:              []string{"exec", "{prompt}"},
				ModelFlag:         "-m",
				ModelFlagPosition: "before",
			},
			prompt:       "do the thing",
			model:        "gpt-4",
			expectedArgs: []string{"-m", "gpt-4", "exec", "do the thing"},
		},
		{
			name: "model_after_standard",
			agentCfg: &config.AgentConfig{
				Command:           "agent",
				Args:              []string{"run", "{prompt}"},
				ModelFlag:         "-m",
				ModelFlagPosition: "after",
			},
			prompt:       "do the thing",
			model:        "gpt-4",
			expectedArgs: []string{"run", "do the thing", "-m", "gpt-4"},
		},
		{
			name: "model_before_placeholder",
			agentCfg: &config.AgentConfig{
				Command:           "agent",
				Args:              []string{"{prompt}"},
				ModelFlag:         "--{value}",
				ModelFlagPosition: "before",
			},
			prompt:       "do the thing",
			model:        "max",
			expectedArgs: []string{"--max", "do the thing"},
		},
		{
			name: "model_after_placeholder",
			agentCfg: &config.AgentConfig{
				Command:           "agent",
				Args:              []string{"run", "{prompt}"},
				ModelFlag:         "--mode={value}",
				ModelFlagPosition: "after",
			},
			prompt:       "do the thing",
			model:        "turbo",
			expectedArgs: []string{"run", "do the thing", "--mode=turbo"},
		},
	})
}

func TestBuildAgentCommand_ReasoningFlag(t *testing.T) {
	t.Parallel()

	runAgentCommandTestCases(t, []agentCommandTestCase{
		{
			name: "reasoning_before_standard",
			agentCfg: &config.AgentConfig{
				Command:               "agent",
				Args:                  []string{"exec", "{prompt}"},
				ReasoningFlag:         "-r",
				ReasoningFlagPosition: "before",
			},
			prompt:       "do the thing",
			reasoning:    "high",
			expectedArgs: []string{"-r", "high", "exec", "do the thing"},
		},
		{
			name: "reasoning_after_standard",
			agentCfg: &config.AgentConfig{
				Command:               "agent",
				Args:                  []string{"exec", "{prompt}"},
				ReasoningFlag:         "-r",
				ReasoningFlagPosition: "after",
			},
			prompt:       "do the thing",
			reasoning:    "high",
			expectedArgs: []string{"exec", "do the thing", "-r", "high"},
		},
		{
			name: "reasoning_before_placeholder",
			agentCfg: &config.AgentConfig{
				Command:               "agent",
				Args:                  []string{"exec", "{prompt}"},
				ReasoningFlag:         "-c model_reasoning_effort={value}",
				ReasoningFlagPosition: "before",
			},
			prompt:       "do the thing",
			reasoning:    "high",
			expectedArgs: []string{"-c model_reasoning_effort=high", "exec", "do the thing"},
		},
	})
}

func TestBuildAgentCommand_ModelAndReasoningFlags(t *testing.T) {
	t.Parallel()

	runAgentCommandTestCases(t, []agentCommandTestCase{
		{
			name: "both_before",
			agentCfg: &config.AgentConfig{
				Command:               "agent",
				Args:                  []string{"exec", "{prompt}"},
				ModelFlag:             "-m",
				ModelFlagPosition:     "before",
				ReasoningFlag:         "-r",
				ReasoningFlagPosition: "before",
			},
			prompt:       "do the thing",
			model:        "gpt-4",
			reasoning:    "high",
			expectedArgs: []string{"-m", "gpt-4", "-r", "high", "exec", "do the thing"},
		},
		{
			name: "both_after",
			agentCfg: &config.AgentConfig{
				Command:               "agent",
				Args:                  []string{"exec", "{prompt}"},
				ModelFlag:             "-m",
				ModelFlagPosition:     "after",
				ReasoningFlag:         "-r",
				ReasoningFlagPosition: "after",
			},
			prompt:       "do the thing",
			model:        "gpt-4",
			reasoning:    "high",
			expectedArgs: []string{"exec", "do the thing", "-m", "gpt-4", "-r", "high"},
		},
	})
}

func TestBuildAgentCommand_RealWorldPatterns(t *testing.T) {
	t.Parallel()

	runAgentCommandTestCases(t, []agentCommandTestCase{
		{
			name: "pattern_gemini", // gemini --model X --yolo {prompt}
			agentCfg: &config.AgentConfig{
				Command:           "gemini",
				Args:              []string{"--yolo", "{prompt}"},
				ModelFlag:         "--model",
				ModelFlagPosition: "before",
			},
			prompt:       "implement the feature",
			model:        "gemini-2.5-pro",
			expectedArgs: []string{"--model", "gemini-2.5-pro", "--yolo", "implement the feature"},
		},
		{
			name: "pattern_kilocode", // kilocode --model X --auto --yolo --mode code {prompt}
			agentCfg: &config.AgentConfig{
				Command:           "kilocode",
				Args:              []string{"--auto", "--yolo", "--mode", "code", "{prompt}"},
				ModelFlag:         "--model",
				ModelFlagPosition: "before",
			},
			prompt:       "fix the bug",
			model:        "kilocode-1",
			expectedArgs: []string{"--model", "kilocode-1", "--auto", "--yolo", "--mode", "code", "fix the bug"},
		},
		{
			name: "pattern_droid", // droid exec --skip-permissions-unsafe {prompt} -m X -r Y
			agentCfg: &config.AgentConfig{
				Command:               "droid",
				Args:                  []string{"exec", "--skip-permissions-unsafe", "{prompt}"},
				ModelFlag:             "-m",
				ModelFlagPosition:     "after",
				ReasoningFlag:         "-r",
				ReasoningFlagPosition: "after",
			},
			prompt:       "fix the bug",
			model:        "claude-opus-4",
			reasoning:    "high",
			expectedArgs: []string{"exec", "--skip-permissions-unsafe", "fix the bug", "-m", "claude-opus-4", "-r", "high"},
		},
		{
			name: "pattern_codex", // codex -m X -c effort=Y exec ... {prompt}
			agentCfg: &config.AgentConfig{
				Command:               "codex",
				Args:                  []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "{prompt}"},
				ModelFlag:             "-m",
				ModelFlagPosition:     "before",
				ReasoningFlag:         "-c model_reasoning_effort={value}",
				ReasoningFlagPosition: "before",
			},
			prompt:       "refactor code",
			model:        "o3",
			reasoning:    "high",
			expectedArgs: []string{"-m", "o3", "-c model_reasoning_effort=high", "exec", "--dangerously-bypass-approvals-and-sandbox", "refactor code"},
		},
		{
			name: "pattern_codebuff", // codebuff --max {prompt}
			agentCfg: &config.AgentConfig{
				Command:           "codebuff",
				Args:              []string{"{prompt}"},
				ModelFlag:         "--{value}",
				ModelFlagPosition: "before",
			},
			prompt:       "write tests",
			model:        "max",
			expectedArgs: []string{"--max", "write tests"},
		},
	})
}

func TestBuildAgentCommand_EdgeCases(t *testing.T) {
	t.Parallel()

	runAgentCommandTestCases(t, []agentCommandTestCase{
		{
			name: "empty_position_defaults_to_before",
			agentCfg: &config.AgentConfig{
				Command:           "agent",
				Args:              []string{"run", "{prompt}"},
				ModelFlag:         "-m",
				ModelFlagPosition: "", // Empty should default to "before"
			},
			prompt:       "do the thing",
			model:        "gpt-4",
			expectedArgs: []string{"-m", "gpt-4", "run", "do the thing"},
		},
		{
			name: "model_ignored_when_no_flag_configured",
			agentCfg: &config.AgentConfig{
				Command:   "crush",
				Args:      []string{"run", "{prompt}"},
				ModelFlag: "", // No model flag configured
			},
			prompt:       "do the thing",
			model:        "some-model", // Should be ignored
			expectedArgs: []string{"run", "do the thing"},
		},
		{
			name: "reasoning_ignored_when_no_flag_configured",
			agentCfg: &config.AgentConfig{
				Command:       "agent",
				Args:          []string{"run", "{prompt}"},
				ReasoningFlag: "", // No reasoning flag configured
			},
			prompt:       "do the thing",
			reasoning:    "high", // Should be ignored
			expectedArgs: []string{"run", "do the thing"},
		},
		{
			name: "prompt_in_middle_of_args",
			agentCfg: &config.AgentConfig{
				Command:           "agent",
				Args:              []string{"run", "{prompt}", "--verbose", "--no-confirm"},
				ModelFlag:         "-m",
				ModelFlagPosition: "before",
			},
			prompt:       "do the thing",
			model:        "gpt-4",
			expectedArgs: []string{"-m", "gpt-4", "run", "do the thing", "--verbose", "--no-confirm"},
		},
	})
}
