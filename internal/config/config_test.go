package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	// Verify default values are sensible
	if Default.Harness.SessionDir != "./sessions" {
		t.Errorf("default session dir = %q, want ./sessions", Default.Harness.SessionDir)
	}
	if Default.Harness.DefaultTimeout <= 0 {
		t.Errorf("default timeout = %d, want > 0", Default.Harness.DefaultTimeout)
	}
	if Default.Harness.MaxAttempts <= 0 {
		t.Errorf("default max attempts = %d, want > 0", Default.Harness.MaxAttempts)
	}
	if Default.Docker.AutoPull != true {
		t.Error("default auto pull should be true")
	}
	if len(Default.Sandbox.ReadableDenylist) != 0 {
		t.Errorf("default readable denylist = %v, want empty", Default.Sandbox.ReadableDenylist)
	}
	if len(Default.Sandbox.SharedReadWriteDirs) == 0 {
		t.Error("default shared_readwrite_dirs should not be empty")
	}
	if len(Default.Sandbox.SharedReadOnlyDirs) == 0 {
		t.Error("default shared_readonly_dirs should not be empty")
	}
}

func TestLoadNoFile(t *testing.T) {
	t.Parallel()

	// Load from non-existent directory should return defaults
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(origDir) }()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should get defaults
	if cfg.Harness.SessionDir != Default.Harness.SessionDir {
		t.Errorf("session dir = %q, want %q", cfg.Harness.SessionDir, Default.Harness.SessionDir)
	}
}

func TestLoadExplicitFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.toml")

	content := `
[harness]
session_dir = "./custom-sessions"
default_timeout = 60
max_attempts = 10

[docker]
go_image = "custom-go:latest"
auto_pull = false

[sandbox]
writable_dirs = ["go"]
readable_denylist = ["tasks", "/tmp/secret"]
shared_readwrite_dirs = [".config", ".cache", ".factory"]
shared_readonly_dirs = [".local/bin", "bin"]
		`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Harness.SessionDir != "./custom-sessions" {
		t.Errorf("session dir = %q, want ./custom-sessions", cfg.Harness.SessionDir)
	}
	if cfg.Harness.DefaultTimeout != 60 {
		t.Errorf("timeout = %d, want 60", cfg.Harness.DefaultTimeout)
	}
	if cfg.Harness.MaxAttempts != 10 {
		t.Errorf("max attempts = %d, want 10", cfg.Harness.MaxAttempts)
	}
	if cfg.Docker.GoImage != "custom-go:latest" {
		t.Errorf("go image = %q, want custom-go:latest", cfg.Docker.GoImage)
	}
	if cfg.Docker.AutoPull != false {
		t.Error("auto pull should be false")
	}
	if len(cfg.Sandbox.WritableDirs) != 1 || cfg.Sandbox.WritableDirs[0] != "go" {
		t.Errorf("sandbox writable dirs = %v, want [go]", cfg.Sandbox.WritableDirs)
	}
	if len(cfg.Sandbox.ReadableDenylist) != 2 ||
		cfg.Sandbox.ReadableDenylist[0] != "tasks" ||
		cfg.Sandbox.ReadableDenylist[1] != "/tmp/secret" {
		t.Errorf("sandbox readable denylist = %v, want [tasks /tmp/secret]", cfg.Sandbox.ReadableDenylist)
	}
	if len(cfg.Sandbox.SharedReadWriteDirs) != 3 ||
		cfg.Sandbox.SharedReadWriteDirs[0] != ".config" ||
		cfg.Sandbox.SharedReadWriteDirs[1] != ".cache" ||
		cfg.Sandbox.SharedReadWriteDirs[2] != ".factory" {
		t.Errorf("sandbox shared readwrite dirs = %v, want [.config .cache .factory]", cfg.Sandbox.SharedReadWriteDirs)
	}
	if len(cfg.Sandbox.SharedReadOnlyDirs) != 2 ||
		cfg.Sandbox.SharedReadOnlyDirs[0] != ".local/bin" ||
		cfg.Sandbox.SharedReadOnlyDirs[1] != "bin" {
		t.Errorf("sandbox shared readonly dirs = %v, want [.local/bin bin]", cfg.Sandbox.SharedReadOnlyDirs)
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	t.Parallel()

	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("Load() should error for missing explicit file")
	}
}

func TestImageForLanguage(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Docker: DockerConfig{
			GoImage:         "go-img",
			RustImage:       "rust-img",
			TypeScriptImage: "ts-img",
			KotlinImage:     "kotlin-img",
			DartImage:       "dart-img",
			ZigImage:        "zig-img",
		},
	}

	tests := []struct {
		lang string
		want string
	}{
		{"go", "go-img"},
		{"rust", "rust-img"},
		{"typescript", "ts-img"},
		{"kotlin", "kotlin-img"},
		{"dart", "dart-img"},
		{"zig", "zig-img"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			t.Parallel()
			got := cfg.ImageForLanguage(tc.lang)
			if got != tc.want {
				t.Errorf("ImageForLanguage(%q) = %q, want %q", tc.lang, got, tc.want)
			}
		})
	}
}
