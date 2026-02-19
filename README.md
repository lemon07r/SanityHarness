# SanityHarness

![sanity-banner](https://github.com/user-attachments/assets/b0f8572d-a5fc-4b39-959a-c573e421af17)

[![CI](https://github.com/lemon07r/sanityharness/actions/workflows/ci.yml/badge.svg)](https://github.com/lemon07r/sanityharness/actions/workflows/ci.yml)
[![Go 1.25+](https://img.shields.io/github/go-mod/go-version/lemon07r/sanityharness)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/lemon07r/sanityharness)](https://github.com/lemon07r/sanityharness/releases)

A lightweight evaluation harness for coding agents that runs high-signal, compact but challenging problems in isolated Docker containers. Evaluate agents across 26 tasks in 6 languages with weighted scoring, integrity verification, and detailed reporting.

> **Note:** All evaluation results obtained before version `v1.6.0` cannot be compared to results obtained on or after `v1.6.0` due to a critical fix in how hidden tests are handled.

<!-- Add demo GIF/screenshot here -->

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Available Tasks](#available-tasks)
- [Configuration](#configuration)
- [Agents](#agents)
- [How It Works](#how-it-works)
- [Output](#output)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Isolated Execution**: Each task runs in a dedicated Docker container
- **Multi-Language Support**: Go, Rust, TypeScript, Kotlin, Dart, and Zig (26 tasks)
- **19 Built-in Agents**: Gemini, Claude, OpenCode, Codex, Goose, and more
- **Weighted Scoring**: Empirically-derived difficulty factors for fair comparison
- **BLAKE3 Verification**: Cryptographic integrity checks for submissions
- **Watch Mode**: Automatically re-run tests on file changes
- **Hidden Tests**: Additional validation applied only during eval
- **Parallel Eval**: Run multiple tasks concurrently with `--parallel`
- **Agent Sandboxing**: Bubblewrap isolation restricts agents to their workspace
- **Persistent Caches**: Speed up builds with `.sanity-cache/` mounts

## Quick Start

### Prerequisites

- Go 1.25+
- Docker (running daemon)
- [bubblewrap](https://github.com/containers/bubblewrap) (optional, for agent sandboxing)

### Installation

```bash
git clone https://github.com/lemon07r/sanityharness.git
cd sanityharness
make tools    # Install dev tools (first-time only)
make build    # Build the CLI
```

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | | Config file path (default: `./sanity.toml`) |
| `--tasks-dir` | | External tasks directory |
| `--verbose` | `-v` | Enable debug logging |

## Usage

### List Tasks

```bash
./sanity list                        # List all tasks
./sanity list --json                 # JSON output
./sanity list --language go          # Filter by language
./sanity list --tier core            # Filter by tier
./sanity list --difficulty hard      # Filter by difficulty
```

### Initialize Workspace

```bash
./sanity init go/bank-account        # Create workspace with stub files
./sanity init go/bank-account -o ./my-dir
```

### Run a Task

```bash
./sanity run go/bank-account         # Run tests once
./sanity run go/bank-account --watch # Re-run on file changes
./sanity run go/bank-account -w ./my-impl --timeout 60
```

### Evaluate an Agent

```bash
./sanity eval --agent gemini                          # Evaluate against core tasks
./sanity eval --agent gemini --model gemini-3-pro     # Specify model
./sanity eval --agent gemini --tier all --parallel 4  # All tasks, 4 concurrent
./sanity eval --agent gemini --dry-run                # Preview without running
./sanity eval --agent droid --reasoning high          # Set reasoning effort
./sanity eval --agent gemini --use-mcp-tools          # Enable MCP tools
./sanity eval --agent opencode --disable-mcp          # Disable MCP tools / currently only supported for opencode
./sanity eval --agent opencode --keep-workspaces      # Keep workspaces for debugging
./sanity eval --agent gemini --no-sandbox             # Disable bubblewrap sandbox
./sanity eval --resume ./eval-results/gemini-...      # Resume interrupted eval
```

### View Results

```bash
./sanity show sessions/go-bank-account-2026-01-15T143022-a1b2c3d4
./sanity show sessions/go-bank-account-2026-01-15T143022-a1b2c3d4 --json
```

### Verify Submission

```bash
./sanity verify ./eval-results/gemini-2026-01-07T120000
```

### Clean Up

```bash
./sanity clean                # Interactive cleanup
./sanity clean --all --force  # Clean everything
```

### Version

```bash
./sanity version              # Show version, commit, build date
```

### Task References

Tasks can be referenced as:
- **Canonical**: `<language>/<slug>` (e.g., `go/bank-account`) - always unambiguous
- **Bare slug**: `bank-account` - works if unique across languages

## Available Tasks

26 tasks across 6 languages with varying difficulty:

| Language | Tasks | Tiers | Difficulty |
|----------|-------|-------|------------|
| Go | 6 | 4 core, 2 extended | Hard - Expert |
| Rust | 6 | 4 core, 2 extended | Hard - Expert |
| TypeScript | 5 | 4 core, 1 extended | Hard |
| Kotlin | 3 | 3 extended | Hard |
| Dart | 3 | 3 extended | Hard |
| Zig | 3 | 3 extended | Hard - Expert |

See [docs/TASKS.md](docs/TASKS.md) for complete task listings and metadata.

## Configuration

Create `sanity.toml` in your project root (see `sanity.toml.example`):

```toml
[harness]
max_attempts = 10
default_timeout = 60
session_dir = "sessions"

[docker]
go_image = "ghcr.io/lemon07r/sanity-go:latest"
auto_pull = true
```

Config files are searched in order:
1. `./sanity.toml`
2. `~/.sanity.toml`
3. `~/.config/sanity/config.toml`

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for all options.

## Agents

### Built-in Agents

| Agent | Description |
|-------|-------------|
| `gemini` | Google Gemini CLI |
| `kilocode` | Kilo Code CLI |
| `opencode` | OpenCode CLI |
| `claude` | Anthropic Claude Code |
| `codex` | OpenAI Codex CLI |
| `kimi` | Moonshot Kimi CLI |
| `crush` | Crush CLI |
| `copilot` | GitHub Copilot CLI |
| `droid` | Factory Droid CLI |
| `iflow` | iFlow CLI |
| `qwen` | Qwen Code CLI |
| `amp` | Sourcegraph Amp CLI (modes: `smart`, `rush`) |
| `codebuff` | Codebuff CLI (modes: `max`, `lite`) |
| `vibe` | Mistral Vibe CLI |
| `goose` | Block Goose CLI |
| `junie` | JetBrains Junie CLI |
| `ccs` | Claude Code Switch (profiles: `agy`, `glm`, `kimi`, etc.) |
| `cline` | Cline CLI |
| `pi` | Pi CLI |

### Custom Agents

```toml
[agents.my-agent]
command = "/path/to/my-agent"
args = ["--auto-approve", "{prompt}"]
model_flag = "-m"
env = { API_KEY = "xxx" }
```

See [docs/CONFIGURATION.md#agent-configuration](docs/CONFIGURATION.md#agent-configuration) for full schema.

> **Sandbox note:** `sanity eval` runs agents inside a [bubblewrap](https://github.com/containers/bubblewrap) sandbox where `$HOME` is read-only. All dot-directories under `$HOME` (e.g. `~/.my-agent/`) are automatically writable, so most agents work out of the box. For non-dot directories, add them to `sanity.toml` under `[sandbox] writable_dirs`. Use `--no-sandbox` to disable.

## How It Works

1. **Container Strategy**: Containers run `sleep infinity`; commands execute via `docker exec` for fast reuse
2. **Workspace Mounting**: Your code is mounted at `/workspace` in the container
3. **User Permissions**: Runs as your host UID:GID to avoid root-owned files
4. **Cache Persistence**: Language caches mount from `.sanity-cache/` for faster builds
5. **Embedded Tasks**: Task files are compiled into the binary for zero-dependency distribution

## Output

### Session Output

Each `sanity run` creates:

```
sessions/<session-id>/
├── result.json      # Structured results
├── report.md        # Markdown summary
├── logs/            # Per-attempt logs
└── workspace/       # Final code
```

### Eval Output

Each `sanity eval` creates:

```
eval-results/<agent>-<timestamp>/
├── summary.json       # Complete results with weighted scores
├── attestation.json   # BLAKE3 hashes for verification
├── report.md          # Human-readable report
├── submission.json    # Leaderboard format
├── run-config.json    # Config for resume capability
└── <task>/
    ├── agent.log      # Agent output during task execution
    └── validation.log # Test runner output from validation
```

**Resume interrupted evals:** If interrupted (CTRL+C), the harness saves partial results and prints a resume command. Use `./sanity eval --resume <dir>` to continue.

See [docs/SCORING.md](docs/SCORING.md) for scoring details and output schemas.

## Architecture

```
sanityharness/
├── cmd/sanity/          # CLI entry point
├── internal/
│   ├── cli/             # Cobra commands
│   ├── config/          # TOML configuration
│   ├── errors/          # Error summarization
│   ├── result/          # Session/attempt types
│   ├── runner/          # Docker execution
│   └── task/            # Task loading
├── tasks/               # Embedded task files
└── containers/          # Dockerfiles
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for architecture details.

## Contributing

Contributions are welcome! Please see [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) for guidelines.

Quick start:
```bash
make pre-commit  # Run before committing
make test        # Run tests
```

## License

MIT License
