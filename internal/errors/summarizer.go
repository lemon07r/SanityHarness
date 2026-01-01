// Package errors provides error summarization for different programming languages.
package errors

import (
	"regexp"
	"strings"
)

// Pattern represents a regex pattern and its human-readable summary.
type Pattern struct {
	Regex   *regexp.Regexp
	Summary string
}

// Summarizer extracts human-readable error summaries from compiler/test output.
type Summarizer struct {
	patterns []Pattern
}

// NewSummarizer creates a summarizer for the given language.
func NewSummarizer(language string) *Summarizer {
	var patterns []Pattern

	switch language {
	case "go":
		patterns = goPatterns
	case "rust":
		patterns = rustPatterns
	case "typescript":
		patterns = tsPatterns
	default:
		patterns = nil
	}

	return &Summarizer{patterns: patterns}
}

// Summarize extracts error summaries from output.
// Returns a slice of human-readable error messages.
func (s *Summarizer) Summarize(output string) []string {
	if len(s.patterns) == 0 {
		return s.fallbackSummary(output)
	}

	var summaries []string
	seen := make(map[string]bool)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		for _, p := range s.patterns {
			if matches := p.Regex.FindStringSubmatch(line); matches != nil {
				summary := p.Summary
				for i, match := range matches[1:] {
					placeholder := "$" + string(rune('1'+i))
					summary = strings.ReplaceAll(summary, placeholder, match)
				}

				if !seen[summary] {
					seen[summary] = true
					summaries = append(summaries, summary)
				}
			}
		}
	}

	if len(summaries) == 0 {
		return s.fallbackSummary(output)
	}

	return summaries
}

// fallbackSummary returns the first few lines of error output when no patterns match.
func (s *Summarizer) fallbackSummary(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var result []string
	for i, line := range lines {
		if i >= 5 {
			break
		}
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "===") && !strings.HasPrefix(line, "---") {
			result = append(result, line)
		}
	}

	return result
}

// Go error patterns
var goPatterns = []Pattern{
	{regexp.MustCompile(`DATA RACE`), "Race condition detected"},
	{regexp.MustCompile(`fatal error: all goroutines are asleep - deadlock`), "Deadlock detected"},
	{regexp.MustCompile(`cannot use (.+) \(.*?\) as (.+)`), "Type mismatch: $1 cannot be used as $2"},
	{regexp.MustCompile(`undefined: (\w+)`), "Undefined: $1"},
	{regexp.MustCompile(`(\w+) declared (and|but) not used`), "Unused variable: $1"},
	{regexp.MustCompile(`cannot assign to (.+)`), "Cannot assign to $1"},
	{regexp.MustCompile(`invalid operation: (.+)`), "Invalid operation: $1"},
	{regexp.MustCompile(`too many arguments in call to (\w+)`), "Too many arguments to $1"},
	{regexp.MustCompile(`not enough arguments in call to (\w+)`), "Not enough arguments to $1"},
	{regexp.MustCompile(`cannot convert (.+) to (.+)`), "Cannot convert $1 to $2"},
	{regexp.MustCompile(`missing return`), "Missing return statement"},
	{regexp.MustCompile(`(\w+) redeclared`), "Redeclared: $1"},
	{regexp.MustCompile(`imported and not used: "(.+)"`), "Unused import: $1"},
	{regexp.MustCompile(`panic: (.+)`), "Panic: $1"},
	{regexp.MustCompile(`FAIL\s+(.+)\s+\[`), "Test failed: $1"},
}

// Rust error patterns
var rustPatterns = []Pattern{
	{regexp.MustCompile(`error\[E0382\]`), "Use of moved value (borrow checker)"},
	{regexp.MustCompile(`error\[E0499\]`), "Cannot borrow as mutable more than once"},
	{regexp.MustCompile(`error\[E0502\]`), "Cannot borrow as mutable while borrowed as immutable"},
	{regexp.MustCompile(`error\[E0597\]`), "Value does not live long enough"},
	{regexp.MustCompile(`error\[E0515\]`), "Cannot return reference to local variable"},
	{regexp.MustCompile(`error\[E0507\]`), "Cannot move out of borrowed content"},
	{regexp.MustCompile(`error\[E0308\]`), "Mismatched types"},
	{regexp.MustCompile(`error\[E0425\]`), "Cannot find value in scope"},
	{regexp.MustCompile(`error\[E0433\]`), "Failed to resolve module/type"},
	{regexp.MustCompile(`error\[E0277\]`), "Trait bound not satisfied"},
	{regexp.MustCompile(`error\[E0599\]`), "Method not found"},
	{regexp.MustCompile(`error\[E0412\]`), "Cannot find type in scope"},
	{regexp.MustCompile(`thread '.+' panicked at (.+)`), "Panic: $1"},
	{regexp.MustCompile(`test .+ \.\.\. FAILED`), "Test failed"},
}

// TypeScript error patterns
var tsPatterns = []Pattern{
	{regexp.MustCompile(`TS2322: Type '(.+?)' is not assignable to type '(.+?)'`), "Type '$1' is not assignable to '$2'"},
	{regexp.MustCompile(`TS2339: Property '(.+?)' does not exist on type '(.+?)'`), "Property '$1' does not exist on type '$2'"},
	{regexp.MustCompile(`TS2345: Argument of type '(.+?)' is not assignable`), "Argument type mismatch: $1"},
	{regexp.MustCompile(`TS2304: Cannot find name '(.+?)'`), "Cannot find name '$1'"},
	{regexp.MustCompile(`TS2551: Property '(.+?)' does not exist.*Did you mean '(.+?)'`), "Property '$1' does not exist, did you mean '$2'?"},
	{regexp.MustCompile(`TS2741: Property '(.+?)' is missing`), "Missing property: $1"},
	{regexp.MustCompile(`TS2739: Type '(.+?)' is missing.*properties`), "Type '$1' is missing required properties"},
	{regexp.MustCompile(`TS2532: Object is possibly 'undefined'`), "Object is possibly undefined"},
	{regexp.MustCompile(`TS2531: Object is possibly 'null'`), "Object is possibly null"},
	{regexp.MustCompile(`TS7006: Parameter '(.+?)' implicitly has an 'any' type`), "Parameter '$1' needs type annotation"},
	{regexp.MustCompile(`Error: (.+)`), "Error: $1"},
	{regexp.MustCompile(`FAIL (.+)`), "Test failed: $1"},
}
