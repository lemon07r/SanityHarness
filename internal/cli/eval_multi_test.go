package cli

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestBroadcastOrSplit(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		n       int
		flag    string
		want    []string
		wantErr bool
	}{
		{"empty broadcasts to empty", "", 3, "model", []string{"", "", ""}, false},
		{"single value broadcasts", "gpt-5", 3, "model", []string{"gpt-5", "gpt-5", "gpt-5"}, false},
		{"matching count passes through", "a,b,c", 3, "model", []string{"a", "b", "c"}, false},
		{"trims whitespace", " a , b , c ", 3, "model", []string{"a", "b", "c"}, false},
		{"count mismatch errors", "a,b", 3, "model", nil, true},
		{"single agent single value", "gpt-5", 1, "model", []string{"gpt-5"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := broadcastOrSplit(tt.value, tt.n, tt.flag)
			if (err != nil) != tt.wantErr {
				t.Errorf("broadcastOrSplit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("broadcastOrSplit() len = %d, want %d", len(got), len(tt.want))
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("broadcastOrSplit()[%d] = %q, want %q", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestSanitizeModel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"gemini-2.5-flash", "gemini-2.5-flash"},
		{"google/gemini-2.5-pro", "google-gemini-2.5-pro"},
		{"model:latest", "model-latest"},
		{"my model name", "my-model-name"},
		{"a/b:c d", "a-b-c-d"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeModel(tt.input); got != tt.want {
				t.Errorf("sanitizeModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMultiRunSubdir(t *testing.T) {
	tests := []struct {
		name         string
		spec         RunSpec
		specIdx      int
		rep          int
		totalRepeats int
		want         string
	}{
		{
			"agent only no repeat",
			RunSpec{Agent: "gemini"}, 0, 1, 1,
			filepath.Join("/umbrella", "gemini"),
		},
		{
			"agent with model no repeat",
			RunSpec{Agent: "codex", Model: "gpt-5.2"}, 0, 1, 1,
			filepath.Join("/umbrella", "codex-gpt-5.2"),
		},
		{
			"agent with model and repeat",
			RunSpec{Agent: "codex", Model: "gpt-5.2"}, 0, 2, 3,
			filepath.Join("/umbrella", "codex-gpt-5.2", "run-2"),
		},
		{
			"model with slash sanitized",
			RunSpec{Agent: "opencode", Model: "google/gemini-2.5-pro"}, 0, 1, 1,
			filepath.Join("/umbrella", "opencode-google-gemini-2.5-pro"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := multiRunSubdir("/umbrella", tt.spec, tt.specIdx, tt.rep, tt.totalRepeats)
			if got != tt.want {
				t.Errorf("multiRunSubdir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		in   []float64
		want float64
	}{
		{[]float64{1, 2, 3}, 2.0},
		{[]float64{10}, 10.0},
		{[]float64{}, 0},
		{[]float64{0, 100}, 50.0},
	}
	for _, tt := range tests {
		if got := mean(tt.in); got != tt.want {
			t.Errorf("mean(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestStddev(t *testing.T) {
	// stddev of [2, 4, 4, 4, 5, 5, 7, 9] = 2.0 (population)
	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	got := stddev(vals)
	if math.Abs(got-2.0) > 0.001 {
		t.Errorf("stddev(%v) = %v, want ~2.0", vals, got)
	}

	// Single value: stddev = 0
	if got := stddev([]float64{5}); got != 0 {
		t.Errorf("stddev([5]) = %v, want 0", got)
	}

	// Empty: stddev = 0
	if got := stddev(nil); got != 0 {
		t.Errorf("stddev(nil) = %v, want 0", got)
	}
}

func TestMinMaxVal(t *testing.T) {
	vals := []float64{3, 1, 4, 1, 5, 9}
	if got := minVal(vals); got != 1 {
		t.Errorf("minVal() = %v, want 1", got)
	}
	if got := maxVal(vals); got != 9 {
		t.Errorf("maxVal() = %v, want 9", got)
	}
	if got := minVal(nil); got != 0 {
		t.Errorf("minVal(nil) = %v, want 0", got)
	}
	if got := maxVal(nil); got != 0 {
		t.Errorf("maxVal(nil) = %v, want 0", got)
	}
}

func TestIsMultiRunDir(t *testing.T) {
	// Create temp dir with multi-run-config.json.
	dir := t.TempDir()
	if isMultiRunDir(dir) {
		t.Error("empty dir should not be multi-run dir")
	}

	_ = os.WriteFile(filepath.Join(dir, "multi-run-config.json"), []byte("{}"), 0o644)
	if !isMultiRunDir(dir) {
		t.Error("dir with multi-run-config.json should be multi-run dir")
	}
}

func TestComputeRepeatStats(t *testing.T) {
	spec := RunSpec{Agent: "test", Model: "m1"}
	summaries := []*EvalSummary{
		{PassRate: 60, WeightedScore: 10, Duration: 100, Results: []EvalResult{
			{Task: "go/a", Passed: true}, {Task: "go/b", Passed: false},
		}},
		{PassRate: 80, WeightedScore: 14, Duration: 120, Results: []EvalResult{
			{Task: "go/a", Passed: true}, {Task: "go/b", Passed: true},
		}},
	}

	stats := computeRepeatStats(spec, summaries)

	if stats.Runs != 2 {
		t.Errorf("Runs = %d, want 2", stats.Runs)
	}
	if stats.MeanPassRate != 70 {
		t.Errorf("MeanPassRate = %v, want 70", stats.MeanPassRate)
	}
	if stats.MinPassRate != 60 {
		t.Errorf("MinPassRate = %v, want 60", stats.MinPassRate)
	}
	if stats.MaxPassRate != 80 {
		t.Errorf("MaxPassRate = %v, want 80", stats.MaxPassRate)
	}
	if stats.TaskConsistency["go/a"] != 100 {
		t.Errorf("TaskConsistency[go/a] = %v, want 100", stats.TaskConsistency["go/a"])
	}
	if stats.TaskConsistency["go/b"] != 50 {
		t.Errorf("TaskConsistency[go/b] = %v, want 50", stats.TaskConsistency["go/b"])
	}
}

func TestGenerateComparison(t *testing.T) {
	summaries := []EvalSummary{
		{
			Agent: "a1", Model: "m1", PassRate: 60, WeightedScore: 10,
			Passed: 3, Failed: 2, Total: 5, Duration: 100,
			Results: []EvalResult{
				{Task: "go/x", Passed: true}, {Task: "go/y", Passed: false},
			},
		},
		{
			Agent: "a2", Model: "m2", PassRate: 80, WeightedScore: 14,
			Passed: 4, Failed: 1, Total: 5, Duration: 90,
			Results: []EvalResult{
				{Task: "go/x", Passed: false}, {Task: "go/y", Passed: true},
			},
		},
	}

	c := generateComparison(summaries)

	if len(c.Runs) != 2 {
		t.Fatalf("Runs = %d, want 2", len(c.Runs))
	}
	if c.BestRun != "a2/m2" {
		t.Errorf("BestRun = %q, want %q", c.BestRun, "a2/m2")
	}
	if c.BestScore != 14 {
		t.Errorf("BestScore = %v, want 14", c.BestScore)
	}
	if c.TaskMatrix["go/x"]["a1/m1"] != "✅" {
		t.Errorf("TaskMatrix[go/x][a1/m1] = %q, want ✅", c.TaskMatrix["go/x"]["a1/m1"])
	}
	if c.TaskMatrix["go/x"]["a2/m2"] != "❌" {
		t.Errorf("TaskMatrix[go/x][a2/m2] = %q, want ❌", c.TaskMatrix["go/x"]["a2/m2"])
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{30, "30s"},
		{90, "1m 30s"},
		{3661, "61m 01s"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.seconds); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}
