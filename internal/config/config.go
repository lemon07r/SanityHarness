// Package config provides configuration loading and management for SanityHarness.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all configuration for SanityHarness.
type Config struct {
	Harness HarnessConfig `toml:"harness"`
	Docker  DockerConfig  `toml:"docker"`
}

// HarnessConfig contains harness-specific settings.
type HarnessConfig struct {
	SessionDir     string `toml:"session_dir"`
	DefaultTimeout int    `toml:"default_timeout"`
	MaxAttempts    int    `toml:"max_attempts"`
	OutputFormat   string `toml:"output_format"`
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
		DefaultTimeout: 30,
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
	cfg := Default

	var path string
	if configFile != "" {
		path = configFile
		if _, err := os.Stat(path); err != nil {
			return nil, errors.New("config file not found: " + path)
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
		return nil, errors.New("failed to parse config: " + err.Error())
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
