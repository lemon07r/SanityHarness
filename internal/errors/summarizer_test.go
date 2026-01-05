package errors

import (
	"strings"
	"testing"
)

func TestNewSummarizer(t *testing.T) {
	t.Parallel()

	languages := []string{"go", "rust", "typescript", "kotlin", "dart", "zig", "unknown"}
	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			t.Parallel()
			s := NewSummarizer(lang)
			if s == nil {
				t.Error("NewSummarizer returned nil")
			}
		})
	}
}

func TestSummarizeGoErrors(t *testing.T) {
	t.Parallel()

	s := NewSummarizer("go")

	tests := []struct {
		name   string
		input  string
		expect string // substring that should appear in summary
	}{
		{
			name:   "race condition",
			input:  "WARNING: DATA RACE\nRead at 0x00c000",
			expect: "Race condition detected",
		},
		{
			name:   "deadlock",
			input:  "fatal error: all goroutines are asleep - deadlock!",
			expect: "Deadlock detected",
		},
		{
			name:   "undefined symbol",
			input:  "undefined: FooBar",
			expect: "Undefined: FooBar",
		},
		{
			name:   "panic",
			input:  "panic: runtime error: index out of range",
			expect: "Panic:",
		},
		{
			name:   "unused variable",
			input:  "x declared but not used",
			expect: "Unused variable: x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := s.Summarize(tc.input)
			if len(result) == 0 {
				t.Fatal("expected non-empty summary")
			}
			found := false
			for _, r := range result {
				if strings.Contains(r, tc.expect) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in summary, got %v", tc.expect, result)
			}
		})
	}
}

func TestSummarizeRustErrors(t *testing.T) {
	t.Parallel()

	s := NewSummarizer("rust")

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "moved value",
			input:  "error[E0382]: use of moved value: `x`",
			expect: "Use of moved value",
		},
		{
			name:   "mutable borrow",
			input:  "error[E0499]: cannot borrow `x` as mutable more than once",
			expect: "Cannot borrow as mutable more than once",
		},
		{
			name:   "mismatched types",
			input:  "error[E0308]: mismatched types",
			expect: "Mismatched types",
		},
		{
			name:   "panic",
			input:  "thread 'main' panicked at 'assertion failed'",
			expect: "Panic:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := s.Summarize(tc.input)
			if len(result) == 0 {
				t.Fatal("expected non-empty summary")
			}
			found := false
			for _, r := range result {
				if strings.Contains(r, tc.expect) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in summary, got %v", tc.expect, result)
			}
		})
	}
}

func TestSummarizeTypeScriptErrors(t *testing.T) {
	t.Parallel()

	s := NewSummarizer("typescript")

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "type not assignable",
			input:  "TS2322: Type 'string' is not assignable to type 'number'",
			expect: "not assignable",
		},
		{
			name:   "property not exist",
			input:  "TS2339: Property 'foo' does not exist on type 'Bar'",
			expect: "does not exist",
		},
		{
			name:   "cannot find name",
			input:  "TS2304: Cannot find name 'xyz'",
			expect: "Cannot find name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := s.Summarize(tc.input)
			if len(result) == 0 {
				t.Fatal("expected non-empty summary")
			}
			found := false
			for _, r := range result {
				if strings.Contains(r, tc.expect) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in summary, got %v", tc.expect, result)
			}
		})
	}
}

func TestSummarizeFallback(t *testing.T) {
	t.Parallel()

	// Unknown language uses fallback
	s := NewSummarizer("unknown")
	result := s.Summarize("line1\nline2\nline3\nline4\nline5\nline6\nline7")

	// Fallback returns first 5 non-empty lines
	if len(result) == 0 {
		t.Error("expected fallback summary")
	}
	if len(result) > 5 {
		t.Errorf("fallback should return at most 5 lines, got %d", len(result))
	}
}

func TestSummarizeDeduplication(t *testing.T) {
	t.Parallel()

	s := NewSummarizer("go")
	input := "undefined: Foo\nundefined: Foo\nundefined: Foo"
	result := s.Summarize(input)

	// Should deduplicate identical errors
	count := 0
	for _, r := range result {
		if strings.Contains(r, "Undefined: Foo") {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected deduplicated errors, got %d occurrences", count)
	}
}
