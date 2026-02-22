// Package result provides result handling, session management, and output formatting.
package result

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Status represents the final status of a task run.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusTimeout Status = "timeout"
	StatusError   Status = "error"
)

// StatusEmoji maps status values to their emoji representations.
var StatusEmoji = map[Status]string{
	StatusPass:    "✅",
	StatusFail:    "❌",
	StatusTimeout: "⏱️",
	StatusError:   "⚠️",
}

// Session represents a complete evaluation session.
type Session struct {
	ID          string            `json:"id"`
	TaskSlug    string            `json:"task_slug"`
	Language    string            `json:"language"`
	Status      Status            `json:"status"`
	Attempts    []Attempt         `json:"attempts"`
	TotalTime   time.Duration     `json:"total_time_ns"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at"`
	FinalCode   map[string]string `json:"final_code,omitempty"`
	Config      SessionConfig     `json:"config"`
}

// SessionConfig captures the configuration used for a session.
type SessionConfig struct {
	Timeout     int    `json:"timeout"`
	MaxAttempts int    `json:"max_attempts"`
	WatchMode   bool   `json:"watch_mode"`
	Image       string `json:"image"`
}

// Attempt represents a single validation attempt.
type Attempt struct {
	Number       int           `json:"number"`
	ExitCode     int           `json:"exit_code"`
	Passed       bool          `json:"passed"`
	Duration     time.Duration `json:"duration_ns"`
	ErrorSummary []string      `json:"error_summary,omitempty"`
	RawOutput    string        `json:"raw_output"`
	Timestamp    time.Time     `json:"timestamp"`
}

// NewSession creates a new session with the given parameters.
func NewSession(taskSlug, language string, cfg SessionConfig) *Session {
	now := time.Now()
	// Add random suffix to prevent ID collisions
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	randSuffix := hex.EncodeToString(randBytes)
	id := fmt.Sprintf("%s-%s-%s-%s", language, taskSlug, now.Format("2006-01-02T150405"), randSuffix)

	return &Session{
		ID:        id,
		TaskSlug:  taskSlug,
		Language:  language,
		Status:    StatusFail,
		Attempts:  make([]Attempt, 0),
		StartedAt: now,
		Config:    cfg,
		FinalCode: make(map[string]string),
	}
}

// AddAttempt adds a new attempt to the session.
func (s *Session) AddAttempt(exitCode int, duration time.Duration, output string, errorSummary []string) {
	attempt := Attempt{
		Number:       len(s.Attempts) + 1,
		ExitCode:     exitCode,
		Passed:       exitCode == 0,
		Duration:     duration,
		ErrorSummary: errorSummary,
		RawOutput:    output,
		Timestamp:    time.Now(),
	}

	s.Attempts = append(s.Attempts, attempt)

	if attempt.Passed {
		s.Status = StatusPass
	}
}

// Complete finalizes the session.
func (s *Session) Complete() {
	s.CompletedAt = time.Now()
	s.TotalTime = s.CompletedAt.Sub(s.StartedAt)
}

// LastAttempt returns the most recent attempt, or nil if none.
func (s *Session) LastAttempt() *Attempt {
	if len(s.Attempts) == 0 {
		return nil
	}
	return &s.Attempts[len(s.Attempts)-1]
}

// Passed returns true if the session passed.
func (s *Session) Passed() bool {
	return s.Status == StatusPass
}

// SessionDir returns the directory path for storing session data.
func (s *Session) SessionDir(baseDir string) string {
	return filepath.Join(baseDir, s.ID)
}

// Save writes the session data to disk.
// If the workspace is already inside the session directory (the default),
// this only writes result.json, report.md, and attempt logs.
func (s *Session) Save(baseDir string) error {
	dir := s.SessionDir(baseDir)

	// Create directories
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0755); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	// Write result.json
	resultJSON, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), resultJSON, 0644); err != nil {
		return fmt.Errorf("writing result.json: %w", err)
	}

	// Write report.md
	report := s.GenerateMarkdown()
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(report), 0644); err != nil {
		return fmt.Errorf("writing report.md: %w", err)
	}

	// Write attempt logs
	for _, attempt := range s.Attempts {
		logFile := filepath.Join(dir, "logs", fmt.Sprintf("attempt-%d.log", attempt.Number))
		if err := os.WriteFile(logFile, []byte(attempt.RawOutput), 0644); err != nil {
			return fmt.Errorf("writing attempt log: %w", err)
		}
	}

	// Note: Workspace files are no longer copied here since the workspace
	// is now created inside the session directory by the runner.
	// The FinalCode field in result.json still contains the stub files for
	// programmatic access.

	return nil
}

// GenerateMarkdown generates a human-readable markdown report.
func (s *Session) GenerateMarkdown() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# SanityHarness Report: %s\n\n", s.TaskSlug)
	fmt.Fprintf(&sb, "**Status:** %s %s\n\n", StatusEmoji[s.Status], strings.ToUpper(string(s.Status)))
	fmt.Fprintf(&sb, "**Language:** %s\n\n", s.Language)
	fmt.Fprintf(&sb, "**Started:** %s\n\n", s.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "**Completed:** %s\n\n", s.CompletedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "**Total Time:** %s\n\n", s.TotalTime.Round(time.Millisecond))
	fmt.Fprintf(&sb, "**Attempts:** %d/%d\n\n", len(s.Attempts), s.Config.MaxAttempts)

	sb.WriteString("---\n\n")
	sb.WriteString("## Attempts\n\n")

	for _, attempt := range s.Attempts {
		status := "❌ FAIL"
		if attempt.Passed {
			status = "✅ PASS"
		}

		fmt.Fprintf(&sb, "### Attempt %d - %s\n\n", attempt.Number, status)
		fmt.Fprintf(&sb, "- **Exit Code:** %d\n", attempt.ExitCode)
		fmt.Fprintf(&sb, "- **Duration:** %s\n", attempt.Duration.Round(time.Millisecond))
		fmt.Fprintf(&sb, "- **Time:** %s\n\n", attempt.Timestamp.Format(time.RFC3339))

		if len(attempt.ErrorSummary) > 0 {
			sb.WriteString("**Error Summary:**\n\n")
			for _, err := range attempt.ErrorSummary {
				fmt.Fprintf(&sb, "- %s\n", err)
			}
			sb.WriteString("\n")
		}

		sb.WriteString("<details>\n<summary>Raw Output</summary>\n\n```\n")
		sb.WriteString(attempt.RawOutput)
		sb.WriteString("\n```\n</details>\n\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("## Configuration\n\n")
	fmt.Fprintf(&sb, "- **Timeout:** %ds\n", s.Config.Timeout)
	fmt.Fprintf(&sb, "- **Max Attempts:** %d\n", s.Config.MaxAttempts)
	fmt.Fprintf(&sb, "- **Watch Mode:** %v\n", s.Config.WatchMode)
	fmt.Fprintf(&sb, "- **Image:** %s\n", s.Config.Image)

	return sb.String()
}

// FormatTerminal returns a formatted string for terminal output.
func FormatTerminal(session *Session, attempt *Attempt, watchMode bool) string {
	if session == nil || attempt == nil {
		return ""
	}

	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(&sb, " SANITY HARNESS                    %s (%s)\n", session.TaskSlug, session.Language)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("\n")

	// Attempt info
	fmt.Fprintf(&sb, " Attempt %d/%d                                    ⏱  %s\n",
		attempt.Number, session.Config.MaxAttempts,
		attempt.Duration.Round(time.Millisecond))
	sb.WriteString(" ─────────────────────────────────────────────────────────\n")

	// Status
	if attempt.Passed {
		sb.WriteString(" ✓ PASS\n")
	} else {
		fmt.Fprintf(&sb, " ✗ FAIL (exit code %d)\n", attempt.ExitCode)
	}
	sb.WriteString("\n")

	// Error summary
	if len(attempt.ErrorSummary) > 0 && !attempt.Passed {
		sb.WriteString(" Error Summary:\n")
		for _, err := range attempt.ErrorSummary {
			fmt.Fprintf(&sb, "   • %s\n", err)
		}
		sb.WriteString("\n")
	}

	// Watch mode hint
	if watchMode && !attempt.Passed {
		sb.WriteString(" Watching for changes... (Ctrl+C to stop)\n")
	}

	sb.WriteString("\n")

	return sb.String()
}

// FormatFinalResult returns a formatted summary for the end of a session.
func FormatFinalResult(session *Session) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(" FINAL RESULT\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("\n")

	if session.Passed() {
		sb.WriteString(" ✓ PASSED\n")
	} else {
		fmt.Fprintf(&sb, " ✗ %s\n", strings.ToUpper(string(session.Status)))
	}

	sb.WriteString("\n")
	fmt.Fprintf(&sb, " Task:      %s\n", session.TaskSlug)
	fmt.Fprintf(&sb, " Attempts:  %d\n", len(session.Attempts))
	fmt.Fprintf(&sb, " Duration:  %s\n", session.TotalTime.Round(time.Millisecond))
	fmt.Fprintf(&sb, " Session:   %s\n", session.ID)
	sb.WriteString("\n")

	return sb.String()
}
