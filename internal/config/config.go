// Package config provides configuration loading and management for SanityHarness.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// AgentConfig defines how to invoke a coding agent.
type AgentConfig struct {
	Command               string            `toml:"command"`                 // Binary name or path
	Args                  []string          `toml:"args"`                    // Args with {prompt} placeholder
	ModelFlag             string            `toml:"model_flag"`              // e.g., "--model", "-m"
	ModelFlagPosition     string            `toml:"model_flag_position"`     // "before" or "after" {prompt} in args (default: "before")
	ReasoningFlag         string            `toml:"reasoning_flag"`          // e.g., "-r", "--reasoning-effort"
	ReasoningFlagPosition string            `toml:"reasoning_flag_position"` // "before" or "after" {prompt} in args (default: "before")
	Env                   map[string]string `toml:"env"`                     // Environment variables
	DefaultTimeout        int               `toml:"default_timeout"`         // Per-agent minimum timeout in seconds (overrides harness default if larger)
	MCPPrompt             string            `toml:"mcp_prompt,omitempty"`    // Agent-specific MCP tool guidance (appended when --use-mcp-tools is set)
}

// DefaultAgents provides built-in configurations for popular coding agents.
var DefaultAgents = map[string]AgentConfig{
	"gemini": {
		Command:           "gemini",
		Args:              []string{"--yolo", "{prompt}"},
		ModelFlag:         "--model",
		ModelFlagPosition: "before",
	},
	"kilocode": {
		Command:               "kilocode",
		Args:                  []string{"run", "--auto", "{prompt}"},
		ModelFlag:             "-m",
		ModelFlagPosition:     "after",
		ReasoningFlag:         "--variant",
		ReasoningFlagPosition: "after",
	},
	"opencode": {
		Command:               "opencode",
		Args:                  []string{"run", "{prompt}"},
		ModelFlag:             "-m",
		ModelFlagPosition:     "after",
		ReasoningFlag:         "--variant",
		ReasoningFlagPosition: "after",
		MCPPrompt:             "Your environment includes additional MCP tools. Incorporate them into your workflow the same way you would any built-in command. If an MCP tool can do something more reliably or with better context than your default approach, prefer it.",
	},
	"claude": {
		Command:           "claude",
		Args:              []string{"-p", "--dangerously-skip-permissions", "{prompt}"},
		ModelFlag:         "--model",
		ModelFlagPosition: "before",
	},
	"codex": {
		Command:               "codex",
		Args:                  []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "{prompt}"},
		ModelFlag:             "-m",
		ModelFlagPosition:     "before",
		ReasoningFlag:         "-c model_reasoning_effort={value}", // Reasoning: minimal, low, medium, high, xhigh
		ReasoningFlagPosition: "before",
	},
	"kimi": {
		Command:           "kimi",
		Args:              []string{"--yolo", "-c", "{prompt}"},
		ModelFlag:         "-m",
		ModelFlagPosition: "before",
	},
	"crush": {
		Command:           "crush",
		Args:              []string{"run", "{prompt}"},
		ModelFlag:         "",
		ModelFlagPosition: "",
	},
	"copilot": {
		Command:           "copilot",
		Args:              []string{"--allow-all-tools", "-i", "{prompt}"},
		ModelFlag:         "--model",
		ModelFlagPosition: "before",
	},
	"droid": {
		Command:               "droid",
		Args:                  []string{"exec", "--skip-permissions-unsafe", "{prompt}"},
		ModelFlag:             "-m",
		ModelFlagPosition:     "after",                         // Must be after 'exec' subcommand
		ReasoningFlag:         "-r",                            // Reasoning effort: off, none, low, medium, high
		ReasoningFlagPosition: "after",                         // Must be after 'exec' subcommand
		Env:                   map[string]string{"CI": "true"}, // Disable Ink TTY mode
	},
	"iflow": {
		Command:           "iflow",
		Args:              []string{"--yolo", "-p", "{prompt}"},
		ModelFlag:         "-m",
		ModelFlagPosition: "before",
	},
	"qwen": {
		Command:           "qwen",
		Args:              []string{"--yolo", "{prompt}"},
		ModelFlag:         "-m",
		ModelFlagPosition: "before",
	},
	"amp": {
		Command:           "amp",
		Args:              []string{"--dangerously-allow-all", "-x", "{prompt}"},
		ModelFlag:         "-m",
		ModelFlagPosition: "before",
	},
	"codebuff": {
		Command:           "codebuff",
		Args:              []string{"{prompt}"},
		ModelFlag:         "--{value}",
		ModelFlagPosition: "before",
	},
	"vibe": {
		Command:           "vibe",
		Args:              []string{"--prompt", "{prompt}"},
		ModelFlag:         "",
		ModelFlagPosition: "",
	},
	"goose": {
		Command:           "goose",
		Args:              []string{"run", "--no-session", "-t", "{prompt}"},
		ModelFlag:         "--model",
		ModelFlagPosition: "after",
		Env:               map[string]string{"GOOSE_MODE": "auto"},
	},
	"junie": {
		Command:           "junie",
		Args:              []string{"--skip-update-check", "--task", "{prompt}"},
		ModelFlag:         "--model",
		ModelFlagPosition: "before",
	},
	"ccs": {
		Command:               "ccs",
		Args:                  []string{"--dangerously-skip-permissions", "{prompt}"},
		ModelFlag:             "{value}",
		ModelFlagPosition:     "before",
		ReasoningFlag:         "--thinking",
		ReasoningFlagPosition: "before",
	},
	"cline": {
		Command:           "cline",
		Args:              []string{"task", "--yolo", "--thinking", "{prompt}"},
		ModelFlag:         "-m",
		ModelFlagPosition: "before",
	},
	"pi": {
		Command:               "pi",
		Args:                  []string{"--no-session", "-p", "{prompt}"},
		ModelFlag:             "-m",
		ModelFlagPosition:     "before",
		ReasoningFlag:         "--thinking",
		ReasoningFlagPosition: "before",
		DefaultTimeout:        240, // pi buffers all stdout until tool calls complete; needs more time than streaming agents
	},
}

// Config holds all configuration for SanityHarness.
type Config struct {
	Harness HarnessConfig          `toml:"harness"`
	Docker  DockerConfig           `toml:"docker"`
	Sandbox SandboxConfig          `toml:"sandbox"`
	Agents  map[string]AgentConfig `toml:"agents"`
}

// HarnessConfig contains harness-specific settings.
type HarnessConfig struct {
	SessionDir     string `toml:"session_dir"`
	DefaultTimeout int    `toml:"default_timeout"`
	MaxAttempts    int    `toml:"max_attempts"`
	OutputFormat   string `toml:"output_format"`
}

// SandboxConfig contains bubblewrap sandbox settings.
type SandboxConfig struct {
	WritableDirs        []string `toml:"writable_dirs"`         // Additional $HOME-relative dirs to mount writable
	ReadableDenylist    []string `toml:"readable_denylist"`     // Repo-relative or absolute paths to hide from agents
	SharedReadWriteDirs []string `toml:"shared_readwrite_dirs"` // Broad shared allowlist mounted read/write (home-relative or absolute)
	SharedReadOnlyDirs  []string `toml:"shared_readonly_dirs"`  // Broad shared allowlist mounted read-only (home-relative or absolute)
}

// DockerConfig contains Docker-related settings.
type DockerConfig struct {
	GoImage         string `toml:"go_image"`
	RustImage       string `toml:"rust_image"`
	TypeScriptImage string `toml:"typescript_image"`
	KotlinImage     string `toml:"kotlin_image"`
	DartImage       string `toml:"dart_image"`
	ZigImage        string `toml:"zig_image"`
	AutoPull        bool   `toml:"auto_pull"`
}

// Default configuration values.
var Default = Config{
	Harness: HarnessConfig{
		SessionDir:     "./sessions",
		DefaultTimeout: 600,
		MaxAttempts:    5,
		OutputFormat:   "all",
	},
	Docker: DockerConfig{
		GoImage:         "ghcr.io/lemon07r/sanity-go:latest",
		RustImage:       "ghcr.io/lemon07r/sanity-rust:latest",
		TypeScriptImage: "ghcr.io/lemon07r/sanity-ts:latest",
		KotlinImage:     "ghcr.io/lemon07r/sanity-kotlin:latest",
		DartImage:       "ghcr.io/lemon07r/sanity-dart:latest",
		ZigImage:        "ghcr.io/lemon07r/sanity-zig:latest",
		AutoPull:        true,
	},
	Sandbox: SandboxConfig{
		// Compatibility-focused shared allowlist: keep common auth/config/cache/toolchain
		// paths writable while masking high-risk read locations in the sandbox layer.
		SharedReadWriteDirs: []string{
			".cache",
			".config",
			".local/share",
			".local/state",
			".npm",
			".pnpm-store",
			".bun",
			".cargo",
			".rustup",
			".gradle",
			".pub-cache",
			".dart-tool",
			".claude",
			".gemini",
			".junie",
			".qwen",
			".opencode",
			".codex",
			".kilocode",
			".factory",
			".go",
			"go",
		},
		SharedReadOnlyDirs: []string{
			"bin",
			".local/bin",
			"go/bin",
			".opencode/bin",
			".bun/bin",
			".npm-global",
			".agents",
		},
	},
}

// configPaths returns the list of paths to search for config files.
func configPaths() []string {
	paths := []string{"./sanity.toml"}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".sanity.toml"))
		paths = append(paths, filepath.Join(home, ".config", "sanity", "config.toml"))
	}

	return paths
}

// Load loads configuration from a file or discovers it automatically.
// If configFile is empty, it searches standard locations.
// Returns default config if no file is found.
func Load(configFile string) (*Config, error) {
	cfg := Default // Start with defaults

	var path string
	if configFile != "" {
		path = configFile
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
	} else {
		for _, p := range configPaths() {
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}

	if path == "" {
		return &cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}

	// Ensure critical fields aren't zeroed out by partial config
	if cfg.Harness.SessionDir == "" {
		cfg.Harness.SessionDir = Default.Harness.SessionDir
	}
	if cfg.Harness.DefaultTimeout <= 0 {
		cfg.Harness.DefaultTimeout = Default.Harness.DefaultTimeout
	}
	if cfg.Harness.MaxAttempts <= 0 {
		cfg.Harness.MaxAttempts = Default.Harness.MaxAttempts
	}
	if cfg.Docker.GoImage == "" {
		cfg.Docker.GoImage = Default.Docker.GoImage
	}
	if cfg.Docker.RustImage == "" {
		cfg.Docker.RustImage = Default.Docker.RustImage
	}
	if cfg.Docker.TypeScriptImage == "" {
		cfg.Docker.TypeScriptImage = Default.Docker.TypeScriptImage
	}
	if cfg.Docker.KotlinImage == "" {
		cfg.Docker.KotlinImage = Default.Docker.KotlinImage
	}
	if cfg.Docker.DartImage == "" {
		cfg.Docker.DartImage = Default.Docker.DartImage
	}
	if cfg.Docker.ZigImage == "" {
		cfg.Docker.ZigImage = Default.Docker.ZigImage
	}

	return &cfg, nil
}

// ImageForLanguage returns the Docker image for a given language.
func (c *Config) ImageForLanguage(lang string) string {
	switch lang {
	case "go":
		return c.Docker.GoImage
	case "rust":
		return c.Docker.RustImage
	case "typescript":
		return c.Docker.TypeScriptImage
	case "kotlin":
		return c.Docker.KotlinImage
	case "dart":
		return c.Docker.DartImage
	case "zig":
		return c.Docker.ZigImage
	default:
		return ""
	}
}

// GetAgent returns the agent configuration for the given name.
// User-configured agents take precedence over built-in defaults.
// Returns nil if the agent is not found.
func (c *Config) GetAgent(name string) *AgentConfig {
	// Check user-configured agents first
	if c.Agents != nil {
		if agent, ok := c.Agents[name]; ok {
			return &agent
		}
	}
	// Fall back to built-in defaults
	if agent, ok := DefaultAgents[name]; ok {
		return &agent
	}
	return nil
}

// ListAgents returns all available agent names (built-in + user-configured), sorted.
func (c *Config) ListAgents() []string {
	seen := make(map[string]bool)
	var names []string

	// Add user-configured agents first
	for name := range c.Agents {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	// Add built-in agents
	for name := range DefaultAgents {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	// Sort for consistent output
	sort.Strings(names)

	return names
}
