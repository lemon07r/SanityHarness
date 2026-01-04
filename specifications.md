# Project Context

## Purpose

SanityHarness is a lightweight, fast, and high-signal evaluation harness for coding agents. Unlike heavy benchmarks like SWE-bench (10-45 minutes per task, $5+ per run), SanityHarness targets "Compact Hard Problems" that execute in <30 seconds at ~$0.15 per run.

The goal is a simple, easy-to-use testing suite that works with **any coding agent** with minimal setup:

1. Agent writes code to a workspace directory
2. Harness detects changes, runs validation in isolated containers
3. Results saved to session folder (JSON + markdown report)

### Core Value Proposition

- **Speed**: <10 seconds per task execution via `docker exec` reuse
- **Simplicity**: Single binary, auto-pulls container images, file-based agent interface
- **High Signal**: Tasks that discriminate between pattern-matching and true reasoning
- **Multi-Language**: Go, Rust, TypeScript support from day one

## Tech Stack

- **Language**: Go 1.25+
- **CLI Framework**: `github.com/spf13/cobra`
- **Configuration**: `github.com/BurntSushi/toml`
- **Docker SDK**: `github.com/docker/docker`
- **File Watching**: `github.com/fsnotify/fsnotify`
- **Logging**: `log/slog` (standard library)

## Project Conventions

### Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` and `goimports` for formatting
- Use `go vet` for static analysis
- Use `go lint` (and add linters) for code quality
- Explicit error handling (no panics except in main)
- Variable naming: short but descriptive (`ctx`, `cfg`, `sess`)
- Package naming: lowercase, single-word, no underscores

### Architecture Patterns

- **cmd/** for CLI entry points
- **internal/** for private packages (not importable externally)
- **tasks/** for bundled task definitions
- **Interfaces for testability** - especially Docker and filesystem operations
- **Context propagation** - use `context.Context` for cancellation/timeouts
- **Flat structure initially** - avoid premature abstraction

### Project Layout

```
sanityharness/
├── cmd/
│   └── sanity/
│       └── main.go                 # CLI entry point
├── internal/
│   ├── cli/                        # Cobra commands
│   │   ├── root.go                 # Root command + global flags
│   │   ├── run.go                  # sanity run
│   │   ├── list.go                 # sanity list
│   │   └── init.go                 # sanity init
│   ├── config/                     # Configuration
│   │   └── config.go               # TOML loading, defaults
│   ├── task/                       # Task system
│   │   ├── task.go                 # Task struct, validation
│   │   ├── loader.go               # Discovery from tasks/
│   │   └── language.go             # Language-specific configs
│   ├── runner/                     # Execution engine
│   │   ├── runner.go               # Main orchestration
│   │   ├── session.go              # Session lifecycle
│   │   ├── docker.go               # Container management
│   │   └── watcher.go              # fsnotify file watching
│   ├── result/                     # Results
│   │   ├── result.go               # Result structs
│   │   ├── session.go              # Session folder management
│   │   ├── json.go                 # JSON output
│   │   ├── markdown.go             # Markdown report
│   │   └── terminal.go             # Human-readable terminal output
│   └── errors/                     # Error handling
│       └── summarizer.go           # Regex-based error extraction
├── tasks/                          # Bundled tasks (10 total)
│   ├── go/                         # 4 Go tasks
│   ├── rust/                       # 4 Rust tasks
│   └── typescript/                 # 2 TypeScript tasks
├── containers/                     # Dockerfiles (for CI builds)
│   ├── Dockerfile.go
│   ├── Dockerfile.rust
│   └── Dockerfile.ts
├── .github/
│   └── workflows/
│       └── docker.yml              # Build + push to GHCR
├── sessions/                       # Output (git-ignored)
├── go.mod
├── go.sum
├── sanity.toml                     # Default config
└── README.md
```

### Testing Strategy

- **Table-driven tests** using `t.Run()` for subtests
- **Test files alongside code** (`*_test.go` next to implementation)
- **Race detector enabled**: `go test -race ./...`
- **Integration tests** with Docker (skippable via build tags)
- **Cover happy path + key error cases**

### Git Workflow

- **Branch prefixes**: `feature/`, `fix/`, `docs/`
- **Commit messages**: Imperative mood, concise (<50 chars summary)
  - Example: "Add container timeout handling"
- **No force pushes** to main
- **PRs require passing CI** before merge

## Domain Context

### What is a "Compact Hard Problem"?

Tasks that are computationally lightweight (<30 seconds execution) but logically dense, serving as high-signal discriminators of agent capability. Examples:

- **Concurrency puzzles**: Dining Philosophers, Bank Account with race conditions
- **Memory safety challenges**: Rust borrow checker, lifetimes
- **Type system navigation**: TypeScript generics, interface design

### The Compiler as Adversary

Unlike Python where errors manifest at runtime, Go/Rust/TypeScript provide strict compile-time contracts. An agent that can interpret and resolve compiler errors demonstrates causal reasoning beyond pattern matching.

### Agent Interface

Agents interact via file-based workflow:

1. `sanity init <task>` creates workspace with stub files
2. Agent modifies the stub files
3. `sanity run <task> --watch` detects changes and runs validation
4. Results written to session folder

This works with **any agent** - no special API integration required.

### Watch Mode (Feedback Loop)

The ability for an agent to "think, write, fail, think, write, pass" in a tight loop is what separates a coding agent from a simple LLM chat. Watch mode enables:

1. Agent writes code
2. Harness detects change via fsnotify
3. Runs validation via `docker exec` (fast!)
4. Outputs error summary
5. Agent reads error, fixes code
6. Repeat until pass

### Container Strategy

**Per-session container reuse** for performance:

```
Session Start:  docker run -d ghcr.io/lemon07r/sanity-go:1.25 sleep infinity
Attempt 1:      docker exec <id> go test -race ./...  (fast!)
Attempt 2:      docker exec <id> go test -race ./...  (fast!)
Session End:    docker rm -f <id>
```

**Auto-pull from GHCR**:

- Check if image exists locally
- If not, pull with progress bar
- No local Dockerfile builds required
- Single binary distribution works

### Error Summarization

Regex-based extraction per language (deterministic, offline-capable):

- Go: Race conditions, type mismatches, undefined symbols
- Rust: Borrow checker errors (E0382, E0499, etc.)
- TypeScript: Type errors (TS2322, TS2339, etc.)

## Important Constraints

- **Minimal dependencies** - Standard library preferred where possible
- **No external database** - All state is file-based (session folders)
- **Container isolation required** - All task execution in Docker
- **Default 30-second timeout** per task, configurable
- **Hermetic execution** - Same container image = same results
- **Bundled tasks only (MVP)** - 10 high-quality tasks, no custom task support

## External Dependencies

### Required

- **Docker Engine** - For container execution
- **Go 1.25+** - For building the harness

### Container Images (Auto-Pulled)

| Image | Base | Pre-installed |
|-------|------|---------------|
| `ghcr.io/lemon07r/sanity-go:1.25` | `golang:1.25-alpine` | `golangci-lint` |
| `ghcr.io/lemon07r/sanity-rust:1.75` | `rust:1.75-alpine` | `clippy`, `miri` |
| `ghcr.io/lemon07r/sanity-ts:20` | `node:20-alpine` | `typescript`, `tsx` |

## CLI Interface

```bash
# List available tasks
sanity list                          # All tasks
sanity list --language go            # Filter by language
sanity list --json                   # JSON output

# Initialize workspace
sanity init bank-account             # Creates ./bank-account/
sanity init bank-account -o ./work   # Custom output

# Run evaluation
sanity run bank-account              # Single attempt
sanity run bank-account --watch      # Watch mode (continuous)
sanity run bank-account --max-attempts 5

# Output format
sanity run bank-account --output json
sanity run bank-account --output human
sanity run bank-account --output all   # Default

# View session
sanity show sessions/2024-12-30-143022-bank-account
```

## Bundled Tasks (22 Total)

### Go (6 tasks)

| Task | Difficulty | Tier |
|------|------------|------|
| `go/bank-account` | Hard | core |
| `go/dining-philosophers` | Hard | core |
| `go/errgroup-limit` | Hard | core |
| `go/parallel-letter-frequency` | Hard | core |
| `go/react` | Hard | extended |
| `go/singleflight` | Expert | extended |

### Rust (6 tasks)

| Task | Difficulty | Tier |
|------|------------|------|
| `rust/circular-buffer` | Hard | core |
| `rust/doubly-linked-list` | Expert | extended |
| `rust/generational-arena` | Hard | extended |
| `rust/macros` | Hard | core |
| `rust/parallel-letter-frequency` | Hard | core |
| `rust/regex-lite` | Hard | core |

### TypeScript (4 tasks)

| Task | Difficulty | Tier |
|------|------------|------|
| `typescript/forth` | Hard | core |
| `typescript/glob` | Hard | core |
| `typescript/promise-pool` | Hard | core |
| `typescript/react` | Hard | extended |

### Kotlin (2 tasks)

| Task | Difficulty | Tier |
|------|------------|------|
| `kotlin/channel-multiplexer` | Hard | extended |
| `kotlin/flow-processor` | Hard | extended |

### Dart (2 tasks)

| Task | Difficulty | Tier |
|------|------------|------|
| `dart/isolate-pool` | Hard | extended |
| `dart/reactive-cache` | Hard | extended |

### Zig (2 tasks)

| Task | Difficulty | Tier |
|------|------------|------|
| `zig/arena-allocator` | Hard | extended |
| `zig/comptime-json` | Expert | extended |

## Task Authoring Guidelines

### Hidden Tests Policy

- Hidden tests must only rely on public APIs declared in the task stub (`[files].stub`).
- If a new public function/type is required, add it to the stub (and update the task description) before asserting it in hidden tests.
- Hidden tests should be deterministic: avoid wall-clock timing thresholds and goroutine/thread counting.
- Hidden tests must not require solution-side interfaces/types that are defined only in the test file.

## Session Output Format

Each run creates a session folder:

```
sessions/2024-12-30-143022-bank-account/
├── result.json             # Machine-readable
├── report.md               # Human-readable markdown
├── metadata.json           # Session config, timing
├── workspace/              # Agent's final code
└── logs/
    ├── attempt-1.log
    ├── attempt-2.log
    └── container.log
```

## Configuration (sanity.toml)

```toml
[harness]
session_dir = "./sessions"
default_timeout = 30
max_attempts = 5
output_format = "all"  # json, human, all

[docker]
go_image = "ghcr.io/lemon07r/sanity-go:1.25"
rust_image = "ghcr.io/lemon07r/sanity-rust:1.75"
typescript_image = "ghcr.io/lemon07r/sanity-ts:20"
auto_pull = true
```
