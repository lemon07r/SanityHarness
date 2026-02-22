package runner

import (
	"errors"
	"testing"
	"time"

	errsummary "github.com/lemon07r/sanityharness/internal/errors"
	"github.com/lemon07r/sanityharness/internal/result"
)

func TestSetSessionStatusFromExecError(t *testing.T) {
	t.Parallel()

	session := result.NewSession("task", "go", result.SessionConfig{})

	setSessionStatusFromExecError(session, errors.New("exec timed out after 10m0s"))
	if session.Status != result.StatusTimeout {
		t.Fatalf("status = %s, want %s", session.Status, result.StatusTimeout)
	}

	setSessionStatusFromExecError(session, errors.New("exec failed"))
	if session.Status != result.StatusError {
		t.Fatalf("status = %s, want %s", session.Status, result.StatusError)
	}
}

func TestRecordExecErrorAttempt(t *testing.T) {
	t.Parallel()

	session := result.NewSession("task", "go", result.SessionConfig{})
	summarizer := errsummary.NewSummarizer("go")
	execResult := &ExecResult{
		ExitCode: -1,
		Combined: "panic: timed out",
		Duration: 2 * time.Second,
	}

	recordExecErrorAttempt(session, summarizer, execResult)

	if len(session.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(session.Attempts))
	}
	got := session.Attempts[0]
	if got.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1", got.ExitCode)
	}
	if got.RawOutput != execResult.Combined {
		t.Fatalf("raw output = %q, want %q", got.RawOutput, execResult.Combined)
	}
	if got.Duration != execResult.Duration {
		t.Fatalf("duration = %s, want %s", got.Duration, execResult.Duration)
	}
}
