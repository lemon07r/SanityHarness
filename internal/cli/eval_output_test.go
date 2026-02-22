package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateLeaderboardSubmissionIncludesRunMetadata(t *testing.T) {
	t.Parallel()

	summary := EvalSummary{
		Agent:                           "codex",
		Model:                           "gpt-5",
		Timestamp:                       "2026-02-22T010203",
		PassRate:                        50.0,
		WeightedPassRate:                49.0,
		Passed:                          13,
		Failed:                          13,
		Total:                           26,
		WeightedScore:                   10.5,
		MaxPossibleScore:                20.5,
		Timeout:                         600,
		Parallel:                        4,
		UseMCPTools:                     false,
		DisableMCP:                      false,
		Sandbox:                         false,
		Legacy:                          false,
		QuotaAffectedTasks:              0,
		TotalQuotaRetries:               0,
		TotalSelfTestCommands:           17,
		TotalToolchainInstallAttempts:   2,
		TotalOutOfWorkspaceReadAttempts: 3,
		TasksWithSelfTesting:            9,
		TasksWithToolchainInstall:       1,
		TasksWithOutOfWorkspaceReads:    2,
		ByLanguage: map[string]EvalAggregate{
			"go": {Passed: 3, Failed: 3, Total: 6, PassRate: 50.0},
		},
	}

	submission := generateLeaderboardSubmission(summary, nil)

	if submission.Timeout != 600 {
		t.Fatalf("timeout = %d, want 600", submission.Timeout)
	}
	if submission.Parallel != 4 {
		t.Fatalf("parallel = %d, want 4", submission.Parallel)
	}
	if submission.QuotaAffectedTasks != 0 {
		t.Fatalf("quota_affected_tasks = %d, want 0", submission.QuotaAffectedTasks)
	}
	if submission.TotalQuotaRetries != 0 {
		t.Fatalf("total_quota_retries = %d, want 0", submission.TotalQuotaRetries)
	}
	if submission.TotalSelfTestCommands != 17 {
		t.Fatalf("total_self_test_commands = %d, want 17", submission.TotalSelfTestCommands)
	}
	if submission.TotalToolchainInstallAttempts != 2 {
		t.Fatalf("total_toolchain_install_attempts = %d, want 2", submission.TotalToolchainInstallAttempts)
	}
	if submission.TotalOutOfWorkspaceReadAttempts != 3 {
		t.Fatalf("total_out_of_workspace_read_attempts = %d, want 3", submission.TotalOutOfWorkspaceReadAttempts)
	}
	if submission.TasksWithSelfTesting != 9 {
		t.Fatalf("tasks_with_self_testing = %d, want 9", submission.TasksWithSelfTesting)
	}
	if submission.TasksWithToolchainInstall != 1 {
		t.Fatalf("tasks_with_toolchain_install = %d, want 1", submission.TasksWithToolchainInstall)
	}
	if submission.TasksWithOutOfWorkspaceReads != 2 {
		t.Fatalf("tasks_with_out_of_workspace_reads = %d, want 2", submission.TasksWithOutOfWorkspaceReads)
	}
}

func TestRunConfigMarshalIncludesFalseFlags(t *testing.T) {
	t.Parallel()

	cfg := RunConfig{
		Agent:          "codex",
		Timeout:        600,
		Parallel:       1,
		UseMCPTools:    false,
		DisableMCP:     false,
		NoSandbox:      false,
		Legacy:         false,
		KeepWorkspaces: false,
		TaskList:       []string{"go/bank-account"},
		CreatedAt:      "2026-02-22T01:02:03Z",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal run config: %v", err)
	}

	got := string(data)
	for _, field := range []string{
		`"use_mcp_tools":false`,
		`"disable_mcp":false`,
		`"no_sandbox":false`,
		`"legacy":false`,
		`"keep_workspaces":false`,
	} {
		if !strings.Contains(got, field) {
			t.Fatalf("expected run-config json to include %s, got: %s", field, got)
		}
	}
}

func TestEvalSummaryMarshalIncludesZeroAuditFields(t *testing.T) {
	t.Parallel()

	summary := EvalSummary{
		Agent:                           "codex",
		Timestamp:                       "2026-02-22T010203",
		Timeout:                         600,
		Parallel:                        1,
		Results:                         []EvalResult{},
		Passed:                          0,
		Failed:                          0,
		Total:                           0,
		PassRate:                        0,
		UseMCPTools:                     false,
		DisableMCP:                      false,
		Sandbox:                         false,
		Legacy:                          false,
		QuotaAffectedTasks:              0,
		TotalQuotaRetries:               0,
		TotalSelfTestCommands:           0,
		TotalToolchainInstallAttempts:   0,
		TotalOutOfWorkspaceReadAttempts: 0,
		TasksWithSelfTesting:            0,
		TasksWithToolchainInstall:       0,
		TasksWithOutOfWorkspaceReads:    0,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}

	got := string(data)
	for _, field := range []string{
		`"timeout":600`,
		`"parallel":1`,
		`"use_mcp_tools":false`,
		`"disable_mcp":false`,
		`"sandbox":false`,
		`"legacy":false`,
		`"quota_affected_tasks":0`,
		`"total_quota_retries":0`,
		`"total_self_test_commands":0`,
		`"total_toolchain_install_attempts":0`,
		`"total_out_of_workspace_read_attempts":0`,
		`"tasks_with_self_testing":0`,
		`"tasks_with_toolchain_install":0`,
		`"tasks_with_out_of_workspace_reads":0`,
	} {
		if !strings.Contains(got, field) {
			t.Fatalf("expected summary json to include %s, got: %s", field, got)
		}
	}
}

func TestWriteAgentTimeoutFooter(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "agent.log")
	logFile, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	writeAgentTimeoutFooter(logFile, 1, 120*time.Second, 121*time.Second)
	_ = logFile.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "HARNESS: agent timed out") {
		t.Fatalf("expected timeout footer, got: %s", got)
	}
	if !strings.Contains(got, "attempt=2") {
		t.Fatalf("expected attempt index in footer, got: %s", got)
	}
}

func TestWriteValidationLog(t *testing.T) {
	t.Parallel()

	t.Run("empty output still writes footer", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "validation.log")

		writeValidationLog(path, "", []string{"gradle", "test"}, 0, 2*time.Second, false, nil)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read validation log: %v", err)
		}
		got := string(data)
		if !strings.Contains(got, "HARNESS: validation command=") {
			t.Fatalf("expected validation footer, got: %s", got)
		}
		if strings.HasPrefix(got, "\n") {
			t.Fatalf("validation log should not start with newline, got: %q", got)
		}
	})

	t.Run("includes raw output and run error", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "validation.log")

		writeValidationLog(path, "PASS\n", []string{"go", "test", "./..."}, -1, 3*time.Second, true, errors.New("exec timed out"))

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read validation log: %v", err)
		}
		got := string(data)
		if !strings.Contains(got, "PASS") {
			t.Fatalf("expected raw output in validation log, got: %s", got)
		}
		if !strings.Contains(got, "timed_out=true") {
			t.Fatalf("expected timed_out flag in footer, got: %s", got)
		}
		if !strings.Contains(got, `run_error="exec timed out"`) {
			t.Fatalf("expected run_error in footer, got: %s", got)
		}
	})
}
