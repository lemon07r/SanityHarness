# Development Guide

This guide covers building, testing, and developing SanityHarness.

## Prerequisites

- **Go 1.25+**: Required for building and testing
- **Docker**: Running daemon required for task execution
- **golangci-lint v2**: For linting (installed via `make tools`)

## First-Time Setup

```bash
# Clone the repository
git clone https://github.com/lemon07r/sanityharness.git
cd sanityharness

# Install development tools
make tools    # Installs goimports, golangci-lint, govulncheck

# Download dependencies
make deps

# Build the CLI
make build
```

## Makefile Reference

Run `make help` to see all available targets.

### Build Commands

| Command | Description |
|---------|-------------|
| `make build` | Build binary for current platform (`./sanity`) |
| `make build-debug` | Build with debug symbols |
| `make build-all` | Cross-compile for Linux/Darwin/Windows → `bin/` |
| `make build-linux` | Build for Linux/amd64 |
| `make build-darwin` | Build for Darwin/arm64 (Apple Silicon) |
| `make build-windows` | Build for Windows/amd64 |

### Test Commands

| Command | Description |
|---------|-------------|
| `make test` | Run tests with race detection |
| `make test-short` | Skip long-running tests |
| `make coverage` | Generate HTML coverage report |
| `make coverage-open` | Open coverage report in browser |
| `make bench` | Run benchmarks |

### Quality Commands

| Command | Description |
|---------|-------------|
| `make fmt` | Format code with goimports (auto-sorts imports) |
| `make vet` | Run go vet |
| `make lint` | Run golangci-lint |
| `make check` | All quality checks (fmt, vet, lint) |
| `make vuln-check` | Run govulncheck for security vulnerabilities |

### CI Commands

| Command | Description |
|---------|-------------|
| `make pre-commit` | Fast pre-commit checks (fmt, vet, lint, test-short) |
| `make ci` | Full CI pipeline (deps, check, test, build) |
| `make ci-full` | Extended CI (+ vuln-check, coverage, build-all) |
| `make release-dry` | Dry run: clean + build all platforms |
| `make version` | Print version info |

### Utility Commands

| Command | Description |
|---------|-------------|
| `make clean` | Remove build artifacts |
| `make tools` | Install development tools |
| `make deps` | Download Go dependencies |
| `make docker-build` | Build all container images |

## Running Tests

### Unit Tests

```bash
# All tests with race detection
make test

# Specific package
go test -v ./internal/runner

# Specific test by name
go test -v -run TestName ./internal/runner

# Short tests only (skip integration tests)
make test-short
```

### Integration Tests

Integration tests require a running Docker daemon:

```bash
# Run all tests including integration
make test

# Check Docker is available
docker ps
```

### Coverage

```bash
# Generate HTML report
make coverage

# Open in browser
make coverage-open
```

## Building Container Images

Container images are required for task execution.

### Build All Images

```bash
make docker-build
```

### Build Individual Images

```bash
docker build -f containers/Dockerfile-go -t ghcr.io/lemon07r/sanity-go:latest .
docker build -f containers/Dockerfile-rust -t ghcr.io/lemon07r/sanity-rust:latest .
docker build -f containers/Dockerfile-ts -t ghcr.io/lemon07r/sanity-ts:latest .
docker build -f containers/Dockerfile-kotlin -t ghcr.io/lemon07r/sanity-kotlin:latest .
docker build -f containers/Dockerfile-dart -t ghcr.io/lemon07r/sanity-dart:latest .
docker build -f containers/Dockerfile-zig -t ghcr.io/lemon07r/sanity-zig:latest .
```

### Image Auto-Pull

By default, SanityHarness automatically pulls missing images from GHCR. Disable with:

```toml
[docker]
auto_pull = false
```

## Project Architecture

```
sanityharness/
├── cmd/sanity/          # CLI entry point (minimal, calls cli.Execute())
├── internal/
│   ├── cli/             # Cobra commands (list, init, run, show, eval, verify, clean, version)
│   ├── config/          # TOML configuration loading with defaults
│   ├── errors/          # Language-specific error summarization (regex-based)
│   ├── result/          # Session, Attempt types and formatting (JSON, Markdown, terminal)
│   ├── runner/          # Docker execution, task orchestration, file watching
│   └── task/            # Task definition, loading from embedded/external sources, weighted scoring
├── tasks/               # Embedded task files (compiled into binary)
│   ├── go/
│   ├── rust/
│   ├── typescript/
│   ├── kotlin/
│   ├── dart/
│   └── zig/
└── containers/          # Dockerfiles for language runtimes
```

### Key Packages

| Package | Description |
|---------|-------------|
| `cli` | Command-line interface using Cobra |
| `config` | TOML configuration with defaults and merging |
| `errors` | Language-specific error pattern matching |
| `result` | Session/attempt types, JSON/Markdown output |
| `runner` | Docker execution, container lifecycle, file watching |
| `task` | Task loading, filtering, weight calculation |

### Container Strategy

1. Containers run `sleep infinity` as their main process
2. Commands execute via `docker exec` for fast reuse
3. Workspace mounted at `/workspace`
4. Runs as host UID:GID to avoid root-owned files
5. Language caches redirect to `/tmp` and mount from `.sanity-cache/`

### File Watching

Watch mode uses `fsnotify` with:
- 200ms debounce to prevent rapid re-runs
- Recursive subdirectory watching
- Ignores: hidden files, `.swp`, `.tmp`, `.bak`, `.log`

## CI Integration

### Exit Codes

SanityHarness returns appropriate exit codes for CI integration:

| Exit Code | Meaning |
|-----------|---------|
| `0` | All tests passed |
| `1` | One or more tests failed |

### GitHub Actions Example

```yaml
- name: Run SanityHarness
  run: |
    ./sanity eval --agent my-agent --tier core
    
- name: Upload Results
  uses: actions/upload-artifact@v4
  with:
    name: eval-results
    path: eval-results/
```

### Signal Handling

`sanity run` handles SIGINT/SIGTERM gracefully:
- Stops file watching
- Cleans up containers
- Saves partial results

## External Tasks Directory

For development or custom tasks:

```bash
./sanity list --tasks-dir ./my-tasks
./sanity run my-task --tasks-dir ./my-tasks
./sanity eval --agent gemini --tasks-dir ./my-tasks
```

See [TASKS.md](TASKS.md#external-tasks-directory) for directory structure.

## Linting Configuration

This project uses golangci-lint v2 with configuration in `.golangci.yml`.

### Enabled Linters

| Category | Linters |
|----------|---------|
| Core | errcheck, govet, staticcheck, unused, ineffassign |
| Error handling | errorlint (Go 1.13+ error wrapping) |
| Bug detection | bodyclose, durationcheck, noctx, sqlclosecheck, rowserrcheck |
| Code quality | gocritic, unconvert, prealloc |
| Documentation | misspell, godot, predeclared |
| Performance | copyloopvar, intrange |
| Complexity | gocyclo (max 20), gocognit (max 30), nestif (max 5), maintidx |
| Formatting | tagalign |

### Disabled Linters

| Linter | Reason |
|--------|--------|
| gosec | Too many false positives for CLI file operations |
| revive | Unused-parameter warnings conflict with Cobra patterns |
| funlen | Function length limits too restrictive for main logic |
| contextcheck | Conflicts with defer cleanup patterns |

## Troubleshooting

### Docker File Permissions

SanityHarness runs containers as your host UID:GID and redirects caches to `/tmp`. If you have root-owned files from older runs:

```bash
# Use Docker to remove files
docker run --rm -v $(pwd):/workspace alpine rm -rf /workspace/my-workspace
```

### Container Not Starting

1. Ensure Docker daemon is running:
   ```bash
   docker ps
   ```

2. Check for existing containers:
   ```bash
   docker ps -a | grep sanity
   ```

3. Verify images are available:
   ```bash
   docker images | grep sanity
   ```

4. Images auto-pull if missing and `auto_pull = true` (default)

### Build Cache Issues

The `.sanity-cache/` directory can be safely deleted:

```bash
rm -rf .sanity-cache/
```

It will be recreated on the next run.

### golangci-lint Version Mismatch

The project requires golangci-lint built with Go 1.25+:

```bash
# Install from source (recommended)
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2

# Or use make tools
make tools
```

### Test Timeout

For slow systems, increase test timeout:

```bash
go test -timeout 10m ./...
```

## Code Style

### Import Order

Three groups, separated by blank lines:

```go
import (
    "context"              // 1. Standard library
    "fmt"

    "github.com/spf13/cobra"  // 2. External dependencies

    "github.com/lemon07r/sanityharness/internal/config"  // 3. Internal packages
)
```

### Error Handling

Wrap errors with context:

```go
if err := doThing(); err != nil {
    return fmt.Errorf("doing thing: %w", err)
}
```

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Packages | lowercase, single word | `runner`, `config`, `task` |
| Types | PascalCase | `DockerClient`, `SessionConfig` |
| Exported functions | PascalCase | `NewRunner`, `LoadConfig` |
| Unexported functions | camelCase | `parseFlags`, `buildArgs` |
| Variables | short but descriptive | `ctx`, `cfg`, `sess` |
| Custom types | for semantics | `type Language string` |

### Documentation

- Package doc: `// Package runner provides...`
- Exported functions: `// NewRunner creates a new runner with the given configuration.`
- Comments are complete sentences starting with the name being documented
