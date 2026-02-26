package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
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
		SkippedExternalTasks:            3,
		WeightedScore:                   10.5,
		MaxPossibleScore:                20.5,
		Timeout:                         600,
		Parallel:                        4,
		UseMCPTools:                     false,
		DisableMCP:                      false,
		Sandbox:                         false,
		Legacy:                          false,
		QuotaAffectedTasks:              0,
		AuthAffectedTasks:               1,
		InfraAffectedTasks:              2,
		TotalQuotaRetries:               0,
		TotalInfraRetries:               3,
		TotalSelfTestCommands:           17,
		TotalToolchainInstallAttempts:   2,
		TotalOutOfWorkspaceReadAttempts: 3,
		SkillsUsageRate:                 38.5,
		TotalSkillsUsageSignals:         5,
		TasksWithSelfTesting:            9,
		TasksWithToolchainInstall:       1,
		TasksWithOutOfWorkspaceReads:    2,
		TasksWithSkillsUsage:            10,
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
	if submission.AuthAffectedTasks != 1 {
		t.Fatalf("auth_affected_tasks = %d, want 1", submission.AuthAffectedTasks)
	}
	if submission.InfraAffectedTasks != 2 {
		t.Fatalf("infra_affected_tasks = %d, want 2", submission.InfraAffectedTasks)
	}
	if submission.TotalQuotaRetries != 0 {
		t.Fatalf("total_quota_retries = %d, want 0", submission.TotalQuotaRetries)
	}
	if submission.TotalInfraRetries != 3 {
		t.Fatalf("total_infra_retries = %d, want 3", submission.TotalInfraRetries)
	}
	if submission.SkippedExternalTasks != 3 {
		t.Fatalf("skipped_external_tasks = %d, want 3", submission.SkippedExternalTasks)
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
	if submission.SkillsUsageRate != 38.5 {
		t.Fatalf("skills_usage_rate = %v, want 38.5", submission.SkillsUsageRate)
	}
	if submission.TotalSkillsUsageSignals != 5 {
		t.Fatalf("total_skills_usage_signals = %d, want 5", submission.TotalSkillsUsageSignals)
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
	if submission.TasksWithSkillsUsage != 10 {
		t.Fatalf("tasks_with_skills_usage = %d, want 10", submission.TasksWithSkillsUsage)
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
		SkippedExternalTasks:            0,
		PassRate:                        0,
		UseMCPTools:                     false,
		DisableMCP:                      false,
		Sandbox:                         false,
		Legacy:                          false,
		QuotaAffectedTasks:              0,
		AuthAffectedTasks:               0,
		InfraAffectedTasks:              0,
		TotalQuotaRetries:               0,
		TotalInfraRetries:               0,
		TotalSelfTestCommands:           0,
		TotalToolchainInstallAttempts:   0,
		TotalOutOfWorkspaceReadAttempts: 0,
		SkillsUsageRate:                 0,
		TotalSkillsUsageSignals:         0,
		TasksWithSelfTesting:            0,
		TasksWithToolchainInstall:       0,
		TasksWithOutOfWorkspaceReads:    0,
		TasksWithSkillsUsage:            0,
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
		`"auth_affected_tasks":0`,
		`"infra_affected_tasks":0`,
		`"skipped_external_tasks":0`,
		`"total_quota_retries":0`,
		`"total_infra_retries":0`,
		`"total_self_test_commands":0`,
		`"total_toolchain_install_attempts":0`,
		`"total_out_of_workspace_read_attempts":0`,
		`"skills_usage_rate":0`,
		`"total_skills_usage_signals":0`,
		`"tasks_with_self_testing":0`,
		`"tasks_with_toolchain_install":0`,
		`"tasks_with_out_of_workspace_reads":0`,
		`"tasks_with_skills_usage":0`,
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

func TestHashFilesReturnsEmptyWhenNoFilesPresent(t *testing.T) {
	t.Parallel()

	hash, found, err := hashFiles([]string{
		filepath.Join(t.TempDir(), "missing-a"),
		filepath.Join(t.TempDir(), "missing-b"),
	})
	if err != nil {
		t.Fatalf("hashFiles() error = %v", err)
	}
	if found {
		t.Fatal("hashFiles() found = true, want false")
	}
	if hash != "" {
		t.Fatalf("hashFiles() hash = %q, want empty", hash)
	}
}

func TestWriteIntegrityViolationArtifacts(t *testing.T) {
	t.Parallel()

	loader := task.NewLoader(tasks.FS, tasksDir)
	taskDef, err := loader.Load("flow-processor")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}

	taskOutputDir := t.TempDir()
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	modifiedPath := filepath.Join(workspaceDir, "build.gradle.kts")
	if err := os.WriteFile(modifiedPath, []byte("plugins { kotlin(\"jvm\") version \"9.9.9\" }"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}

	err = writeIntegrityViolationArtifacts(
		taskOutputDir,
		loader,
		taskDef,
		workspaceDir,
		[]string{"build.gradle.kts"},
		"modified task files (disallowed): build.gradle.kts",
	)
	if err != nil {
		t.Fatalf("writeIntegrityViolationArtifacts() error = %v", err)
	}

	reportPath := filepath.Join(taskOutputDir, "integrity.json")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read integrity report: %v", err)
	}

	var report integrityArtifactReport
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("unmarshal integrity report: %v", err)
	}
	if report.Task != "kotlin/flow-processor" {
		t.Fatalf("report task = %q, want kotlin/flow-processor", report.Task)
	}
	if len(report.Files) != 1 {
		t.Fatalf("report files len = %d, want 1", len(report.Files))
	}
	entry := report.Files[0]
	if entry.Path != "build.gradle.kts" {
		t.Fatalf("entry path = %q, want build.gradle.kts", entry.Path)
	}
	if !entry.ExpectedExists || !entry.ActualExists {
		t.Fatalf("expected_exists=%v actual_exists=%v, want true/true", entry.ExpectedExists, entry.ActualExists)
	}
	if entry.ExpectedHash == "" || entry.ActualHash == "" {
		t.Fatalf("expected both hashes to be populated, got expected=%q actual=%q", entry.ExpectedHash, entry.ActualHash)
	}

	expectedArtifact := filepath.Join(taskOutputDir, filepath.FromSlash(entry.ExpectedArtifact))
	actualArtifact := filepath.Join(taskOutputDir, filepath.FromSlash(entry.ActualArtifact))
	diffArtifact := filepath.Join(taskOutputDir, filepath.FromSlash(entry.DiffArtifact))
	if _, err := os.Stat(expectedArtifact); err != nil {
		t.Fatalf("expected artifact missing: %v", err)
	}
	if _, err := os.Stat(actualArtifact); err != nil {
		t.Fatalf("actual artifact missing: %v", err)
	}
	diffData, err := os.ReadFile(diffArtifact)
	if err != nil {
		t.Fatalf("diff artifact missing: %v", err)
	}
	if len(diffData) == 0 {
		t.Fatal("diff artifact is empty")
	}
}
