package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

	prompt := buildAgentPrompt(tt, false, false, "")

	for _, s := range []string{
		"Description: " + tt.Description,
		"Tier:",
		"Difficulty:",
		"Stub/solution files: demo.go",
		"Test files:          demo_test.go",
		"You may run local tests/commands in the workspace while iterating.",
	} {
		if !strings.Contains(prompt, s) {
			t.Fatalf("prompt missing %q\n\nPrompt:\n%s", s, prompt)
		}
	}
	for _, forbidden := range []string{
		"You do NOT need to run tests yourself.",
		"Do NOT search for or install language toolchains/SDKs.",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt should not include %q\n\nPrompt:\n%s", forbidden, prompt)
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
	promptWithoutMCP := buildAgentPrompt(tt, false, false, "")
	for _, forbidden := range []string{
		"You have access to MCP server tools. Review what is available to you before starting work.",
		"1. Use your MCP server tools to help complete your task(s) wherever and whenever applicable.",
		"Prefer your MCP server tools over built-in alternatives if both can accomplish the same step or objective.",
		"You MUST actively use your MCP server tools to assist you with your work. Do NOT ignore them. Make your first MCP server tool call before writing any code.",
		"MCP TOOLS:",
		"AGENT-SPECIFIC TOOLS:",
	} {
		if strings.Contains(promptWithoutMCP, forbidden) {
			t.Fatalf("prompt without MCP tools should not contain %q\n\nPrompt:\n%s", forbidden, promptWithoutMCP)
		}
	}

	// Test with MCP tools
	promptWithMCP := buildAgentPrompt(tt, true, false, "agent-specific text should not appear")
	for _, s := range []string{
		"- You have access to MCP server tools. Review what is available to you before starting work.",
		"1. Use your MCP server tools to help complete your task(s) wherever and whenever applicable.",
		"2. Read the stub file(s) (function signatures with panic()/todo!/Unimplemented placeholders).",
		"6. Ensure thread-safety if the tests use concurrent operations.",
		"- Prefer your MCP server tools over built-in alternatives if both can accomplish the same step or objective.",
		"- You MUST actively use your MCP server tools to assist you with your work. Do NOT ignore them. Make your first MCP server tool call before writing any code.",
	} {
		if !strings.Contains(promptWithMCP, s) {
			t.Fatalf("prompt with MCP tools missing %q\n\nPrompt:\n%s", s, promptWithMCP)
		}
	}
	for _, forbidden := range []string{
		"MCP TOOLS:",
		"AGENT-SPECIFIC TOOLS:",
		"agent-specific text should not appear",
	} {
		if strings.Contains(promptWithMCP, forbidden) {
			t.Fatalf("prompt with MCP tools should not contain %q\n\nPrompt:\n%s", forbidden, promptWithMCP)
		}
	}
}

func TestBuildAgentPromptIncludesToolchainInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lang task.Language
		want string
	}{
		{
			name: "go toolchain mapping",
			lang: task.Go,
			want: "Go 1.25",
		},
		{
			name: "zig toolchain mapping",
			lang: task.Zig,
			want: "Zig 0.13.0",
		},
		{
			name: "unknown language fallback",
			lang: task.Language("customlang"),
			want: "customlang",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tt := &task.Task{
				Slug:        "demo",
				Name:        "Demo Task",
				Language:    tc.lang,
				Tier:        "core",
				Difficulty:  "hard",
				Description: "Implement the thing.",
				Files: task.TaskFiles{
					Stub: []string{"demo.go.txt"},
					Test: []string{"demo_test.go.txt"},
				},
			}

			prompt := buildAgentPrompt(tt, false, false, "")
			wantLine := "- Toolchain: " + tc.want
			if !strings.Contains(prompt, wantLine) {
				t.Fatalf("prompt missing %q\n\nPrompt:\n%s", wantLine, prompt)
			}
		})
	}
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, v := range env {
		if strings.HasPrefix(v, prefix) {
			return strings.TrimPrefix(v, prefix), true
		}
	}
	return "", false
}

func TestBuildAgentCommandDisableMCP(t *testing.T) {
	t.Parallel()

	agentCfg := &config.AgentConfig{
		Command: "opencode",
		Args:    []string{"run", "{prompt}"},
	}

	tests := []struct {
		name           string
		disableMCP     bool
		useMCPTools    bool
		agentName      string
		wantConfig     bool
		wantSubstrings []string
	}{
		{
			name:           "disable_mcp_for_opencode_sets_config",
			disableMCP:     true,
			agentName:      "opencode",
			wantConfig:     true,
			wantSubstrings: []string{`"tools"`, `"*_*":false`},
		},
		{
			name:       "disable_mcp_for_non_opencode_does_not_set_config",
			disableMCP: true,
			agentName:  "gemini",
			wantConfig: false,
		},
		{
			name:           "use_mcp_tools_for_opencode_sets_timeout_config",
			useMCPTools:    true,
			agentName:      "opencode",
			wantConfig:     true,
			wantSubstrings: []string{`"experimental"`, `"mcp_timeout"`, "180000"},
		},
		{
			name:       "neither_flag_for_opencode_does_not_set_config",
			agentName:  "opencode",
			wantConfig: false,
		},
		{
			name:           "both_flags_for_opencode_set_combined_config",
			disableMCP:     true,
			useMCPTools:    true,
			agentName:      "opencode",
			wantConfig:     true,
			wantSubstrings: []string{`"*_*":false`, `"mcp_timeout"`},
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := buildAgentCommand(
				ctx,
				agentCfg,
				"test prompt",
				"",
				"",
				tc.disableMCP,
				tc.useMCPTools,
				tc.agentName,
			)
			configValue, ok := envValue(cmd.Env, "OPENCODE_CONFIG_CONTENT")
			if tc.wantConfig && !ok {
				t.Fatalf(
					"expected OPENCODE_CONFIG_CONTENT to be set (disableMCP=%t useMCPTools=%t agent=%s)",
					tc.disableMCP,
					tc.useMCPTools,
					tc.agentName,
				)
			}
			if !tc.wantConfig && ok {
				t.Fatalf(
					"did not expect OPENCODE_CONFIG_CONTENT to be set (disableMCP=%t useMCPTools=%t agent=%s)",
					tc.disableMCP,
					tc.useMCPTools,
					tc.agentName,
				)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(configValue, want) {
					t.Errorf("expected OPENCODE_CONFIG_CONTENT to include %q, got %q", want, configValue)
				}
			}
		})
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

func TestBuildOpenCodeMCPConfigWithTimeout(t *testing.T) {
	t.Parallel()

	config := buildOpenCodeMCPConfig(false, true)
	if !strings.Contains(config, `"experimental"`) {
		t.Fatal("config should contain experimental key")
	}
	if !strings.Contains(config, `"mcp_timeout"`) {
		t.Fatal("config should contain mcp_timeout")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(config), &parsed); err != nil {
		t.Fatalf("config should be valid JSON: %v", err)
	}

	experimental, ok := parsed["experimental"].(map[string]any)
	if !ok {
		t.Fatalf("expected experimental object, got: %T", parsed["experimental"])
	}

	timeout, ok := experimental["mcp_timeout"].(float64)
	if !ok {
		t.Fatalf("expected mcp_timeout number, got: %T", experimental["mcp_timeout"])
	}

	if int(timeout) != openCodeGlobalMCPTimeoutMS {
		t.Fatalf("expected mcp_timeout=%d, got %d", openCodeGlobalMCPTimeoutMS, int(timeout))
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
	useMCPTools  bool
	agentName    string
	expectedArgs []string
}

func runAgentCommandTestCases(t *testing.T, tests []agentCommandTestCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			cmd := buildAgentCommand(
				ctx,
				tc.agentCfg,
				tc.prompt,
				tc.model,
				tc.reasoning,
				tc.disableMCP,
				tc.useMCPTools,
				tc.agentName,
			)

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
			name: "pattern_kilocode", // kilocode -M X --auto --yolo --mode code {prompt}
			agentCfg: &config.AgentConfig{
				Command:           "kilocode",
				Args:              []string{"--auto", "--yolo", "--mode", "code", "{prompt}"},
				ModelFlag:         "-M",
				ModelFlagPosition: "before",
			},
			prompt:       "fix the bug",
			model:        "kilocode-1",
			expectedArgs: []string{"-M", "kilocode-1", "--auto", "--yolo", "--mode", "code", "fix the bug"},
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

func TestDetectQuotaError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		content         string
		wantHasError    bool
		wantRecoverable bool
	}{
		{
			name:            "no error",
			content:         "agent completed successfully",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "real http 429",
			content:         "error=http 429 Too Many Requests: usage_limit_reached",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "rate limit text",
			content:         "Rate limit exceeded, please try again later",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "503 service unavailable",
			content:         "HTTP 503 Service Unavailable",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "false positive duration 0.503s",
			content:         "Total session time: 4m 1.503s",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive duration_ms 0.429",
			content:         "duration_ms: 0.429788",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive uuid with 503",
			content:         "session id: 019bf26e-a494-73d1-9ce1-6e146f503b01",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive git hash",
			content:         "index 30801fd200a827f0aeb4c5f7a13cbda752e4dd40..b010d97cd7bcc30fb2e838f8fea452975963dcb2",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive line number 502",
			content:         "502:         pub fn items(self: *Self) []T {",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive duration 0.824502",
			content:         "duration_ms: 0.824502",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "non-recoverable billing",
			content:         "billing limit exceeded",
			wantHasError:    true,
			wantRecoverable: false,
		},
		{
			name:            "non-recoverable api key",
			content:         "invalid api key provided",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "too many requests",
			content:         "too many requests, slow down",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "bad gateway",
			content:         "502 Bad Gateway returned from API",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "status 429",
			content:         "received status 429 from server",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "overloaded",
			content:         "server is overloaded, please retry",
			wantHasError:    true,
			wantRecoverable: true,
		},
		{
			name:            "false positive capacity in code description",
			content:         "Uses a buffered channel as a semaphore with capacity equal to the limit",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive try again in algorithm",
			content:         "If first char matches, consume one character and try again",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "false positive capacity in LRU cache",
			content:         "Validates positive capacity\nProper eviction when cache is full",
			wantHasError:    false,
			wantRecoverable: false,
		},
		{
			name:            "real exhausted capacity",
			content:         "You have exhausted your capacity for this billing period",
			wantHasError:    true,
			wantRecoverable: false,
		},
		{
			name:            "real please try again",
			content:         "Service overloaded, please try again in a few minutes",
			wantHasError:    true,
			wantRecoverable: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpFile := filepath.Join(t.TempDir(), "agent.log")
			if err := os.WriteFile(tmpFile, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			hasError, isRecoverable := detectQuotaError(tmpFile)
			if hasError != tc.wantHasError {
				t.Errorf("hasError = %v, want %v", hasError, tc.wantHasError)
			}
			if isRecoverable != tc.wantRecoverable {
				t.Errorf("isRecoverable = %v, want %v", isRecoverable, tc.wantRecoverable)
			}
		})
	}
}

func TestDetectAuthError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		wantAuth bool
	}{
		{
			name:     "authentication failed",
			content:  "Authentication failed. Please login again.",
			wantAuth: true,
		},
		{
			name:     "forbidden",
			content:  "HTTP 403 forbidden from provider",
			wantAuth: true,
		},
		{
			name:     "invalid api key",
			content:  "invalid api key provided",
			wantAuth: true,
		},
		{
			name:     "rate limit is not auth",
			content:  "too many requests, slow down",
			wantAuth: false,
		},
		{
			name:     "normal log",
			content:  "task completed successfully",
			wantAuth: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpFile := filepath.Join(t.TempDir(), "agent.log")
			if err := os.WriteFile(tmpFile, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			got := detectAuthError(tmpFile)
			if got != tc.wantAuth {
				t.Fatalf("detectAuthError() = %v, want %v", got, tc.wantAuth)
			}
		})
	}
}

func TestIsValidationInfraError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "docker daemon unreachable",
			err:  errors.New("ensuring image: Cannot connect to the Docker daemon at unix:///var/run/docker.sock"),
			want: true,
		},
		{
			name: "network timeout",
			err:  errors.New("creating container: dial tcp 10.0.0.1:443: i/o timeout"),
			want: true,
		},
		{
			name: "test failure is not infra",
			err:  errors.New("execution failed for task ':test'"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isValidationInfraError(tc.err)
			if got != tc.want {
				t.Fatalf("isValidationInfraError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsInfraFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		logContent    string
		skipLog       bool // don't create the agent log file at all
		writeFiles    bool // whether to create files in workspace
		writeAgentLog bool // whether to place agent.log in workspace
		wantFailure   bool
	}{
		{
			name:        "no log file",
			skipLog:     true,
			wantFailure: true,
		},
		{
			name:        "empty log",
			logContent:  "",
			writeFiles:  false,
			wantFailure: true,
		},
		{
			name:        "meaningful output",
			logContent:  "Agent completed task successfully with all changes applied",
			wantFailure: false,
		},
		{
			name:        "only retry markers",
			logContent:  "\n\n=== RETRY 1 (after 30s delay) ===\n\n\n\n=== RETRY 2 (after 1m0s delay) ===\n\n",
			wantFailure: true,
		},
		{
			name:        "only retry markers but files modified",
			logContent:  "\n\n=== RETRY 1 (after 30s delay) ===\n\n",
			writeFiles:  true,
			wantFailure: false,
		},
		{
			name:        "empty log but files modified (droid/cline pattern)",
			logContent:  "",
			writeFiles:  true,
			wantFailure: false,
		},
		{
			name:        "small output under threshold",
			logContent:  "err",
			wantFailure: true,
		},
		{
			name:        "small output under threshold but files written",
			logContent:  "err",
			writeFiles:  true,
			wantFailure: false,
		},
		{
			name:        "only harness timeout footer",
			logContent:  "\n\nHARNESS: agent timed out (attempt=1 timeout_seconds=240.000 duration_seconds=240.000)\n",
			wantFailure: true,
		},
		{
			name:        "harness timeout footer but files written",
			logContent:  "\n\nHARNESS: agent timed out (attempt=1 timeout_seconds=240.000 duration_seconds=240.000)\n",
			writeFiles:  true,
			wantFailure: false,
		},
		{
			name:          "empty log with only agent.log in workspace (harness-created)",
			logContent:    "",
			writeAgentLog: true,
			wantFailure:   true, // agent.log inside workspace should be ignored by hasModifiedFiles
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			logPath := filepath.Join(tmpDir, "agent.log")
			workspaceDir := filepath.Join(tmpDir, "workspace")
			if err := os.MkdirAll(workspaceDir, 0755); err != nil {
				t.Fatal(err)
			}

			// Use a cutoff before any file writes so agent-written files are detected.
			workspaceReadyAt := time.Now().Add(-1 * time.Second)

			if !tc.skipLog {
				if err := os.WriteFile(logPath, []byte(tc.logContent), 0644); err != nil {
					t.Fatal(err)
				}
			}

			if tc.writeFiles {
				// Simulate agent writing files to workspace
				if err := os.WriteFile(filepath.Join(workspaceDir, "solution.go"), []byte("package main"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			if tc.writeAgentLog {
				if err := os.WriteFile(filepath.Join(workspaceDir, "agent.log"), []byte(""), 0644); err != nil {
					t.Fatal(err)
				}
			}

			result := isInfraFailure(logPath, workspaceDir, workspaceReadyAt)
			if result != tc.wantFailure {
				t.Errorf("isInfraFailure() = %v, want %v", result, tc.wantFailure)
			}
		})
	}
}

func TestBuildSandboxArgs(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	args := buildSandboxArgs(workspaceDir, "", nil, nil, nil, nil)

	// Verify required arguments are present.
	assertContainsArg := func(flag, value string) {
		t.Helper()
		for i, arg := range args {
			if arg == flag && i+1 < len(args) && args[i+1] == value {
				return
			}
		}
		t.Errorf("expected sandbox args to contain %s %s", flag, value)
	}

	assertContainsFlag := func(flag string) {
		t.Helper()
		for _, arg := range args {
			if arg == flag {
				return
			}
		}
		t.Errorf("expected sandbox args to contain %s", flag)
	}

	// Workspace must be writable (--bind, not --ro-bind).
	assertContainsArg("--bind", workspaceDir)

	// Must have --chdir to workspace.
	assertContainsArg("--chdir", workspaceDir)

	// Must have namespace isolation with network sharing.
	assertContainsFlag("--unshare-all")
	assertContainsFlag("--share-net")
	assertContainsFlag("--die-with-parent")

	// Must have /dev and /proc.
	assertContainsArg("--dev", "/dev")
	assertContainsArg("--proc", "/proc")

	// $HOME must be read-only (--ro-bind).
	homeDir, _ := os.UserHomeDir()
	assertContainsArg("--ro-bind", homeDir)

	// Workspace must NOT appear as --ro-bind (it should be --bind for write access).
	for i, arg := range args {
		if arg == "--ro-bind" && i+1 < len(args) && args[i+1] == workspaceDir {
			t.Error("workspace should not be mounted read-only")
		}
	}
}

func TestWrapCommandWithSandbox(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	ctx := context.Background()

	agentCfg := &config.AgentConfig{
		Command: "echo",
		Args:    []string{"{prompt}"},
	}

	cmd := buildAgentCommand(ctx, agentCfg, "test prompt", "", "", false, false, "test")
	cmd.Dir = workspaceDir

	wrapped := wrapCommandWithSandbox(ctx, cmd, nil, nil, nil, nil)

	// The wrapped command should use bwrap.
	if !strings.HasSuffix(wrapped.Path, "bwrap") {
		// Skip if bwrap is not installed (CI environments).
		if wrapped.Path == "" {
			t.Skip("bwrap not available")
		}
		t.Errorf("expected wrapped command to use bwrap, got %s", wrapped.Path)
	}

	// Args[0] should be "bwrap".
	if wrapped.Args[0] != "bwrap" {
		t.Errorf("expected Args[0] = bwrap, got %s", wrapped.Args[0])
	}

	// The original agent command path should appear after "--".
	foundSeparator := false
	for i, arg := range wrapped.Args {
		if arg == "--" {
			foundSeparator = true
			if i+1 < len(wrapped.Args) {
				if wrapped.Args[i+1] != cmd.Path {
					t.Errorf("expected agent path %s after --, got %s", cmd.Path, wrapped.Args[i+1])
				}
			}
			break
		}
	}
	if !foundSeparator {
		t.Error("expected -- separator in bwrap args")
	}

	// Environment should be preserved.
	if !reflect.DeepEqual(wrapped.Env, cmd.Env) {
		t.Error("expected environment to be preserved in wrapped command")
	}
}

func TestBuildSandboxArgsMasksDenylistedDirs(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	denyDir := filepath.Join(t.TempDir(), "tasks")
	if err := os.MkdirAll(denyDir, 0o755); err != nil {
		t.Fatalf("mkdir deny dir: %v", err)
	}

	args := buildSandboxArgs(workspaceDir, "", nil, nil, nil, []string{denyDir, filepath.Join(t.TempDir(), "missing")})

	foundMask := false
	for i, arg := range args {
		if arg == "--tmpfs" && i+1 < len(args) && args[i+1] == denyDir {
			foundMask = true
			break
		}
	}
	if !foundMask {
		t.Fatalf("expected denylisted directory %s to be masked via --tmpfs", denyDir)
	}
}

func TestBuildSandboxArgsMasksNonAllowlistedHomeDirs(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	for _, rel := range []string{
		".config",
		".factory",
		filepath.Join(".factory", "sessions"),
		"Downloads",
		"Development",
	} {
		if err := os.MkdirAll(filepath.Join(homeDir, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}

	workspaceDir := t.TempDir()
	args := buildSandboxArgs(
		workspaceDir,
		"",
		nil,
		[]string{".config", ".factory"},
		nil,
		nil,
	)

	assertHasMount := func(flag, value string) bool {
		t.Helper()
		for i, arg := range args {
			if arg == flag && i+1 < len(args) && args[i+1] == value {
				return true
			}
		}
		return false
	}

	if !assertHasMount("--bind", filepath.Join(homeDir, ".config")) {
		t.Fatalf("expected .config to be writable-mounted")
	}
	if !assertHasMount("--bind", filepath.Join(homeDir, ".factory")) {
		t.Fatalf("expected .factory to be writable-mounted")
	}
	if !assertHasMount("--tmpfs", filepath.Join(homeDir, "Downloads")) {
		t.Fatalf("expected Downloads to be masked")
	}
	if !assertHasMount("--tmpfs", filepath.Join(homeDir, "Development")) {
		t.Fatalf("expected Development to be masked")
	}
	if !assertHasMount("--tmpfs", filepath.Join(homeDir, ".factory", "sessions")) {
		t.Fatalf("expected .factory/sessions to be masked")
	}
}

func TestBuildSandboxArgsAllowsAgentCommandDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	binDir := filepath.Join(homeDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	commandPath := filepath.Join(binDir, "fake-agent")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}

	workspaceDir := t.TempDir()
	args := buildSandboxArgs(workspaceDir, commandPath, nil, nil, nil, nil)

	hasReadOnlyBind := false
	hasBinMask := false
	for i, arg := range args {
		if arg == "--ro-bind" && i+1 < len(args) && args[i+1] == binDir {
			hasReadOnlyBind = true
		}
		if arg == "--tmpfs" && i+1 < len(args) && args[i+1] == binDir {
			hasBinMask = true
		}
	}
	if !hasReadOnlyBind {
		t.Fatalf("expected command directory %s to be read-only mounted", binDir)
	}
	if hasBinMask {
		t.Fatalf("command directory %s should not be masked", binDir)
	}
}

func TestResolveSandboxDenylistPaths(t *testing.T) {
	origDir, _ := os.Getwd()
	repoRoot := t.TempDir()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	absolutePath := filepath.Join(t.TempDir(), "absolute-secret")
	got := resolveSandboxDenylistPaths([]string{"custom-dir", absolutePath}, "")

	want := []string{
		filepath.Join(repoRoot, "tasks"),
		filepath.Join(repoRoot, "eval-results"),
		filepath.Join(repoRoot, "sessions"),
		filepath.Join(repoRoot, "custom-dir"),
		absolutePath,
	}

	for _, expected := range want {
		found := false
		for _, path := range got {
			if path == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected denylist to include %s, got %v", expected, got)
		}
	}
}

func TestResolveSandboxDenylistPathsIncludesOutputDir(t *testing.T) {
	origDir, _ := os.Getwd()
	repoRoot := t.TempDir()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	outputDir := filepath.Join(t.TempDir(), "custom-output")
	got := resolveSandboxDenylistPaths(nil, outputDir)

	found := false
	for _, path := range got {
		if path == outputDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected denylist to include output dir %s, got %v", outputDir, got)
	}
}

func TestResolveSandboxDenylistPathsCanonicalizesSymlinkRepoRoot(t *testing.T) {
	origDir, _ := os.Getwd()
	realRoot := t.TempDir()
	aliasParent := t.TempDir()
	aliasRoot := filepath.Join(aliasParent, "repo-alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := os.Chdir(aliasRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("PWD", aliasRoot)
	defer func() { _ = os.Chdir(origDir) }()

	got := resolveSandboxDenylistPaths(nil, "")
	want := filepath.Join(realRoot, "tasks")
	unwanted := filepath.Join(aliasRoot, "tasks")

	hasWant := false
	hasUnwanted := false
	for _, path := range got {
		if path == want {
			hasWant = true
		}
		if path == unwanted {
			hasUnwanted = true
		}
	}
	if !hasWant {
		t.Fatalf("expected canonical denylist path %s, got %v", want, got)
	}
	if hasUnwanted {
		t.Fatalf("unexpected symlink denylist path %s in %v", unwanted, got)
	}
}

func TestParseAgentBehaviorMetrics(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "agent.log")
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	content := strings.Join([]string{
		"$ go test ./...",
		"$ cargo test",
		"$ curl -sL https://ziglang.org/download/0.13.0/zig-linux-x86_64-0.13.0.tar.xz | tar xJ",
		"$ ls -la " + workspaceDir,
		"$ find / -name zig -type f 2>/dev/null | head -5",
		"/home/user/project/eval-results/old-run",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	metrics := parseAgentBehaviorMetrics(logPath, workspaceDir)
	if metrics.SelfTestCommands != 2 {
		t.Fatalf("self test commands = %d, want 2", metrics.SelfTestCommands)
	}
	if metrics.ToolchainInstallAttempts != 1 {
		t.Fatalf("toolchain install attempts = %d, want 1", metrics.ToolchainInstallAttempts)
	}
	if metrics.OutOfWorkspaceReads != 1 {
		t.Fatalf("out-of-workspace reads = %d, want 1", metrics.OutOfWorkspaceReads)
	}
	if !metrics.SelfTestCommandsConfident {
		t.Fatal("self-test confidence = false, want true")
	}
	if !metrics.OutOfWorkspaceReadsConfident {
		t.Fatal("out-of-workspace confidence = false, want true")
	}
}

func TestParseAgentBehaviorMetricsFallbackConfidence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "agent.log")
	content := strings.Join([]string{
		"reviewing /home/user/eval-results/run-1",
		"checking /sessions/old",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	metrics := parseAgentBehaviorMetrics(logPath, filepath.Join(tmpDir, "workspace"))
	if metrics.OutOfWorkspaceReads == 0 {
		t.Fatal("out-of-workspace reads = 0, want > 0 from fallback matcher")
	}
	if metrics.OutOfWorkspaceReadsConfident {
		t.Fatal("out-of-workspace confidence = true, want false for fallback parsing")
	}
}
