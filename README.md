# SanityHarness

A lightweight evaluation harness for coding agents that runs "Compact Hard Problems" in isolated Docker containers.

## Features

- **Isolated Execution**: Each task runs in a Docker container
- **Multi-Language Support**: Go, Rust, TypeScript, Kotlin, Dart, and Zig tasks (22 total)
- **Watch Mode**: Automatically re-run tests when files change
- **Session Tracking**: JSON and Markdown reports with full audit trail
- **Error Summarization**: Language-specific error extraction
- **Hidden Tests**: Tasks can include hidden tests only applied during `sanity eval`
- **Eval Integrity Checks**: Agents cannot modify test/support files

## Quick Start

### Prerequisites

- Go 1.25+
- Docker (running daemon)

### Installation

```bash
# Clone the repository
git clone https://github.com/lemon07r/sanityharness.git
cd sanityharness

# Install development tools (first-time setup)
make tools

# Build the CLI
make build
```

### Usage

#### List Available Tasks

```bash
./sanity list
./sanity list --json
./sanity list --lang go
```

#### Initialize a Workspace

```bash
# Create a workspace directory with task stub files
./sanity init bank-account
./sanity init bank-account -o ./my-workspace
```

#### Run a Task

```bash
# Run tests once
./sanity run bank-account

# Run with watch mode (re-runs on file changes)
./sanity run bank-account --watch

# Specify custom workspace
./sanity run bank-account -w ./my-implementation

# Custom timeout and max attempts
./sanity run bank-account --timeout 120 --max-attempts 5
```

#### View Session Results

```bash
# Show results from a previous run
./sanity show sessions/bank-account-2024-12-30T143022

# Output as JSON
./sanity show sessions/bank-account-2024-12-30T143022 --json
```

#### Evaluate an Agent

```bash
# Supported agents: gemini, opencode
./sanity eval --agent gemini
./sanity eval --agent gemini --model gemini-3-pro-preview
./sanity eval --agent opencode
./sanity eval --agent gemini --lang go
./sanity eval --agent gemini --tasks go/react,typescript/react
./sanity eval --agent gemini --keep-workspaces  # Keep workspaces for debugging
```

#### Clean Up

```bash
# Clean up workspace directories (interactive)
./sanity clean

# Clean specific types
./sanity clean --workspaces       # Workspace directories only
./sanity clean --sessions         # Session directories only
./sanity clean --eval             # Eval results only
./sanity clean --all              # Everything

# Skip confirmation
./sanity clean --all --force
```

### Task References

Tasks can be referenced in two ways:

- **Canonical ID**: `<language>/<slug>` (e.g., `go/bank-account`) - always unambiguous
- **Bare slug**: `bank-account` - works only if the slug is unique across languages

For tasks that exist in multiple languages (e.g., `react` exists in both Go and TypeScript), use the canonical form.

## Available Tasks

### Go (6 tasks)

| Task | Description | Difficulty | Hidden Tests |
|------|-------------|------------|--------------|
| `bank-account` | Concurrent bank account with mutex synchronization | Hard | No |
| `dining-philosophers` | Classic concurrency problem solving | Hard | No |
| `errgroup-limit` | Bounded concurrency group that stops on first error | Hard | Yes |
| `parallel-letter-frequency` | Parallel text processing with goroutines | Hard | No |
| `react` | Reactive spreadsheet cells with callbacks | Hard | No |
| `singleflight` | Deduplicate concurrent calls by key | Expert | Yes |

### Rust (6 tasks)

| Task | Description | Difficulty | Hidden Tests |
|------|-------------|------------|--------------|
| `circular-buffer` | Generic circular buffer with ownership | Hard | No |
| `doubly-linked-list` | Unsafe Rust linked list implementation | Expert | No |
| `generational-arena` | Arena allocator with generational handles | Hard | Yes |
| `macros` | Declarative macro creation | Hard | Yes |
| `parallel-letter-frequency` | Multi-threaded text processing | Hard | No |
| `regex-lite` | Regex matching for `.`, `*` (full-string match) | Hard | Yes |

### TypeScript (4 tasks)

| Task | Description | Difficulty | Hidden Tests |
|------|-------------|------------|--------------|
| `forth` | Stack-based language interpreter | Hard | Yes |
| `glob` | Glob pattern matching (`*`, `?`, escaping) | Hard | Yes |
| `promise-pool` | Promise pool with bounded concurrency | Hard | Yes |
| `react` | Reactive cell system with dependencies | Hard | Yes |

### Kotlin (2 tasks)

| Task | Description | Difficulty | Hidden Tests |
|------|-------------|------------|--------------|
| `channel-multiplexer` | Combine multiple channels with priority support | Hard | Yes |
| `flow-processor` | Composable Kotlin Flow processor with operators | Hard | Yes |

### Dart (2 tasks)

| Task | Description | Difficulty | Hidden Tests |
|------|-------------|------------|--------------|
| `isolate-pool` | Worker pool using Dart isolates | Hard | Yes |
| `reactive-cache` | Reactive cache with TTL and stream subscriptions | Hard | Yes |

### Zig (2 tasks)

| Task | Description | Difficulty | Hidden Tests |
|------|-------------|------------|--------------|
| `arena-allocator` | Custom arena allocator with child arenas | Hard | Yes |
| `comptime-json` | Compile-time JSON schema parsing | Expert | Yes |

## Configuration

Create a `sanity.toml` file in your project root:

```toml
[harness]
max_attempts = 10
default_timeout = 60
session_dir = "sessions"
output_format = "all" # json, human, or all

[docker]
go_image = "ghcr.io/lemon07r/sanity-go:latest"
rust_image = "ghcr.io/lemon07r/sanity-rust:latest"
typescript_image = "ghcr.io/lemon07r/sanity-ts:latest"
kotlin_image = "ghcr.io/lemon07r/sanity-kotlin:latest"
dart_image = "ghcr.io/lemon07r/sanity-dart:latest"
zig_image = "ghcr.io/lemon07r/sanity-zig:latest"
auto_pull = true
```

## Architecture

```
sanityharness/
├── cmd/sanity/          # CLI entry point
├── internal/
│   ├── cli/             # Cobra commands (list, init, run, show, eval, version)
│   ├── config/          # TOML configuration with defaults
│   ├── errors/          # Language-specific error summarization
│   ├── result/          # Session and attempt types, output formatting
│   ├── runner/          # Docker execution, file watching
│   └── task/            # Task loading from embedded/external sources
├── tasks/               # Embedded task files (compiled into binary)
│   ├── go/
│   ├── rust/
│   ├── typescript/
│   ├── kotlin/
│   ├── dart/
│   └── zig/
└── containers/          # Dockerfiles for language runtimes
```

### How It Works

1. **Container Strategy**: Containers run `sleep infinity` and commands execute via `docker exec` for fast reuse
2. **Workspace Mounting**: Your code is mounted at `/workspace` in the container
3. **User Permissions**: Runs as your host UID:GID to avoid root-owned files
4. **Cache Isolation**: Language caches redirect to `/tmp` to keep workspaces clean
5. **Embedded Tasks**: Task files are compiled into the binary for zero-dependency distribution

## Session Output

Each run creates a session directory with:

- `result.json` - Structured results with attempts, timing, and final code
- `report.md` - Human-readable Markdown summary
- `logs/attempt-N.log` - Raw output for each attempt
- `workspace/` - Final code snapshot

Example session structure:
```
sessions/
└── go-bank-account-2024-12-30T143022/
    ├── result.json
    ├── report.md
    ├── logs/
    │   ├── attempt-1.log
    │   └── attempt-2.log
    └── workspace/
        ├── bank_account.go
        └── go.mod
```

## Development

### Makefile

This project uses a production-ready Makefile for all build, test, and development tasks:

```bash
make help               # Show all available targets
make build              # Build binary for current platform
make test               # Run tests with race detection
make lint               # Run golangci-lint
make check              # Run all quality checks (fmt, vet, lint)
make coverage           # Generate HTML coverage report
make build-all          # Cross-compile for Linux/Darwin/Windows
make ci                 # Full CI pipeline (deps, check, test, build)
make pre-commit         # Fast pre-commit checks
```

### First-Time Setup

```bash
make tools              # Install goimports, golangci-lint, govulncheck
make deps               # Download dependencies
```

### Running from External Tasks Directory

For development, you can use an external tasks directory:

```bash
./sanity list --tasks-dir ./my-tasks
./sanity run my-task --tasks-dir ./my-tasks
```

### Building Container Images

```bash
# Build all container images at once
make docker-build

# Or build individually
docker build -f containers/Dockerfile-go -t ghcr.io/lemon07r/sanity-go:latest .
docker build -f containers/Dockerfile-rust -t ghcr.io/lemon07r/sanity-rust:latest .
docker build -f containers/Dockerfile-ts -t ghcr.io/lemon07r/sanity-ts:latest .
docker build -f containers/Dockerfile-kotlin -t ghcr.io/lemon07r/sanity-kotlin:latest .
docker build -f containers/Dockerfile-dart -t ghcr.io/lemon07r/sanity-dart:latest .
docker build -f containers/Dockerfile-zig -t ghcr.io/lemon07r/sanity-zig:latest .
```

## Known Issues

### Docker File Permissions

SanityHarness runs containers as your host UID:GID and redirects language caches/build outputs to `/tmp` to avoid root-owned artifacts in workspaces.

If you still have root-owned files from older runs, you can clean them up with:

```bash
# Use Docker to remove files created by containers
docker run --rm -v $(pwd):/workspace alpine rm -rf /workspace/my-workspace
```

## License

MIT License
