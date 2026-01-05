package result

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	t.Parallel()

	cfg := SessionConfig{
		Timeout:     30,
		MaxAttempts: 5,
		WatchMode:   true,
		Image:       "test-image:latest",
	}

	session := NewSession("test-task", "go", cfg)

	if session.TaskSlug != "test-task" {
		t.Errorf("TaskSlug = %q, want test-task", session.TaskSlug)
	}
	if session.Language != "go" {
		t.Errorf("Language = %q, want go", session.Language)
	}
	if session.Status != StatusFail {
		t.Errorf("Status = %q, want fail (default)", session.Status)
	}
	if len(session.Attempts) != 0 {
		t.Errorf("Attempts = %d, want 0", len(session.Attempts))
	}
	if session.Config.Timeout != 30 {
		t.Errorf("Config.Timeout = %d, want 30", session.Config.Timeout)
	}
	if session.FinalCode == nil {
		t.Error("FinalCode should not be nil")
	}

	// ID should contain language, slug, and timestamp
	if !strings.Contains(session.ID, "go") || !strings.Contains(session.ID, "test-task") {
		t.Errorf("ID = %q, should contain language and slug", session.ID)
	}
}

func TestAddAttempt(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{MaxAttempts: 5})

	// Add failing attempt
	session.AddAttempt(1, 100*time.Millisecond, "error output", []string{"Error 1"})

	if len(session.Attempts) != 1 {
		t.Fatalf("Attempts = %d, want 1", len(session.Attempts))
	}
	if session.Attempts[0].Number != 1 {
		t.Errorf("Attempt.Number = %d, want 1", session.Attempts[0].Number)
	}
	if session.Attempts[0].Passed {
		t.Error("Attempt should not be passed")
	}
	if session.Status != StatusFail {
		t.Errorf("Status should remain fail after failed attempt")
	}

	// Add passing attempt
	session.AddAttempt(0, 50*time.Millisecond, "success output", nil)

	if len(session.Attempts) != 2 {
		t.Fatalf("Attempts = %d, want 2", len(session.Attempts))
	}
	if session.Attempts[1].Number != 2 {
		t.Errorf("Attempt.Number = %d, want 2", session.Attempts[1].Number)
	}
	if !session.Attempts[1].Passed {
		t.Error("Attempt should be passed")
	}
	if session.Status != StatusPass {
		t.Errorf("Status = %q, want pass after successful attempt", session.Status)
	}
}

func TestComplete(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{})
	time.Sleep(10 * time.Millisecond)
	session.Complete()

	if session.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
	if session.TotalTime <= 0 {
		t.Error("TotalTime should be positive")
	}
}

func TestLastAttempt(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{})

	// No attempts yet
	if session.LastAttempt() != nil {
		t.Error("LastAttempt should be nil when no attempts")
	}

	session.AddAttempt(1, time.Second, "output", nil)
	session.AddAttempt(0, time.Second, "output", nil)

	last := session.LastAttempt()
	if last == nil {
		t.Fatal("LastAttempt should not be nil")
	}
	if last.Number != 2 {
		t.Errorf("LastAttempt.Number = %d, want 2", last.Number)
	}
}

func TestPassed(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{})
	if session.Passed() {
		t.Error("new session should not be passed")
	}

	session.Status = StatusPass
	if !session.Passed() {
		t.Error("session with StatusPass should be passed")
	}

	session.Status = StatusTimeout
	if session.Passed() {
		t.Error("session with StatusTimeout should not be passed")
	}
}

func TestSessionDir(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{})
	dir := session.SessionDir("/base")

	if !strings.HasPrefix(dir, "/base/") {
		t.Errorf("SessionDir = %q, should start with /base/", dir)
	}
	if !strings.Contains(dir, session.ID) {
		t.Errorf("SessionDir = %q, should contain session ID", dir)
	}
}

func TestSave(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()

	session := NewSession("test", "typescript", SessionConfig{
		Timeout:     30,
		MaxAttempts: 5,
		Image:       "test:latest",
	})
	session.AddAttempt(1, time.Second, "error output", []string{"Error 1"})
	session.AddAttempt(0, time.Second, "success output", nil)
	session.Complete()
	session.FinalCode["test.ts"] = "console.log('hello');"

	if err := session.Save(baseDir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	sessionDir := session.SessionDir(baseDir)

	// Check result.json exists and is valid
	resultPath := filepath.Join(sessionDir, "result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("reading result.json: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parsing result.json: %v", err)
	}
	if loaded.TaskSlug != "test" {
		t.Errorf("loaded TaskSlug = %q, want test", loaded.TaskSlug)
	}
	if len(loaded.Attempts) != 2 {
		t.Errorf("loaded Attempts = %d, want 2", len(loaded.Attempts))
	}

	// Check report.md exists
	reportPath := filepath.Join(sessionDir, "report.md")
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("report.md should exist: %v", err)
	}

	// Check logs directory and attempt logs
	logsDir := filepath.Join(sessionDir, "logs")
	if _, err := os.Stat(logsDir); err != nil {
		t.Errorf("logs dir should exist: %v", err)
	}

	log1Path := filepath.Join(logsDir, "attempt-1.log")
	if _, err := os.Stat(log1Path); err != nil {
		t.Errorf("attempt-1.log should exist: %v", err)
	}

	// Note: Workspace files are no longer written by Save() since the runner
	// creates the workspace inside the session directory. The FinalCode field
	// in result.json still contains the solution code for programmatic access.
	if loaded.FinalCode["test.ts"] != "console.log('hello');" {
		t.Errorf("FinalCode[test.ts] = %q, want console.log('hello');", loaded.FinalCode["test.ts"])
	}
}

func TestGenerateMarkdown(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{
		Timeout:     30,
		MaxAttempts: 5,
		Image:       "test:latest",
	})
	session.AddAttempt(1, time.Second, "error output", []string{"Error 1", "Error 2"})
	session.Complete()

	md := session.GenerateMarkdown()

	// Check for key sections
	if !strings.Contains(md, "# SanityHarness Report") {
		t.Error("markdown should contain report header")
	}
	if !strings.Contains(md, "test") {
		t.Error("markdown should contain task slug")
	}
	if !strings.Contains(md, "Error 1") {
		t.Error("markdown should contain error summary")
	}
	if !strings.Contains(md, "Attempt 1") {
		t.Error("markdown should contain attempt info")
	}
}

func TestFormatTerminal(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{MaxAttempts: 5})
	session.AddAttempt(1, time.Second, "error output", []string{"Error 1"})

	output := FormatTerminal(session, session.LastAttempt(), false)

	if !strings.Contains(output, "SANITY HARNESS") {
		t.Error("output should contain header")
	}
	if !strings.Contains(output, "test") {
		t.Error("output should contain task slug")
	}
	if !strings.Contains(output, "FAIL") {
		t.Error("output should contain FAIL status")
	}
}

func TestFormatFinalResult(t *testing.T) {
	t.Parallel()

	session := NewSession("test", "go", SessionConfig{})
	session.Status = StatusPass
	session.AddAttempt(0, time.Second, "output", nil)
	session.Complete()

	output := FormatFinalResult(session)

	if !strings.Contains(output, "FINAL RESULT") {
		t.Error("output should contain final result header")
	}
	if !strings.Contains(output, "PASSED") {
		t.Error("output should contain PASSED")
	}
}
