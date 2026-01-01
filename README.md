# SanityHarness

A lightweight evaluation harness for coding agents that runs "Compact Hard Problems" in isolated Docker containers.

## Features

- **Isolated Execution**: Each task runs in a fresh Docker container
- **Multi-Language Support**: Go, Rust, and TypeScript tasks
- **Watch Mode**: Automatically re-run tests when files change
- **Session Tracking**: JSON and Markdown reports for each evaluation run
- **Error Summarization**: Language-specific error extraction and summarization

## Quick Start

### Prerequisites

- Go 1.23+
- Docker (running daemon)

### Installation

```bash
# Clone the repository
git clone https://github.com/lemon07r/sanityharness.git
cd sanityharness

# Build the CLI
go build -o sanity ./cmd/sanity
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

## Available Tasks

### Go (4 tasks)
- **bank-account** - Concurrent bank account with mutex synchronization
- **dining-philosophers** - Classic concurrency problem solving
- **parallel-letter-frequency** - Parallel text processing with goroutines
- **react** - Reactive spreadsheet cells with callbacks

### Rust (4 tasks)
- **circular-buffer** - Generic circular buffer with ownership
- **doubly-linked-list** - Unsafe Rust linked list implementation
- **macros** - Declarative macro creation
- **parallel-letter-frequency** - Multi-threaded text processing

### TypeScript (2 tasks)
- **forth** - Stack-based language interpreter
- **react** - Reactive cell system with dependencies

## Configuration

Create a `sanity.toml` file in your project root:

```toml
[harness]
max_attempts = 10
default_timeout = 60
session_dir = "sessions"

[docker]
auto_pull = true

[docker.images]
go = "ghcr.io/lemon07r/sanity-go:latest"
rust = "ghcr.io/lemon07r/sanity-rust:latest"
typescript = "ghcr.io/lemon07r/sanity-ts:latest"
```

## Architecture

```
sanityharness/
├── cmd/sanity/          # CLI entry point
├── internal/
│   ├── cli/             # Cobra commands
│   ├── config/          # TOML configuration
│   ├── errors/          # Error summarization
│   ├── result/          # Session and result types
│   ├── runner/          # Docker execution
│   └── task/            # Task loading
├── tasks/               # Embedded task files
│   ├── go/
│   ├── rust/
│   └── typescript/
└── containers/          # Dockerfiles
```

## Session Output

Each run creates a session directory with:

- `result.json` - Structured results with attempts, timing, and final code
- `result.md` - Human-readable Markdown summary

Example session structure:
```
sessions/
└── bank-account-2024-12-30T143022/
    ├── result.json
    └── result.md
```

## Development

### Running from External Tasks Directory

For development, you can use an external tasks directory:

```bash
./sanity list --tasks-dir ./my-tasks
./sanity run my-task --tasks-dir ./my-tasks
```

### Building Container Images

```bash
docker build -f containers/Dockerfile-go -t ghcr.io/lemon07r/sanity-go:latest .
docker build -f containers/Dockerfile-rust -t ghcr.io/lemon07r/sanity-rust:latest .
docker build -f containers/Dockerfile-ts -t ghcr.io/lemon07r/sanity-ts:latest .
```

## Known Issues

### Docker File Permissions

Files created by tests inside Docker containers (e.g., Rust's `target/` directory) may be owned by root, making them difficult to delete. To clean up:

```bash
# Use Docker to remove files created by containers
docker run --rm -v $(pwd):/workspace alpine rm -rf /workspace/my-workspace
```

## License

MIT License
