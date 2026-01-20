# Contributing to SanityHarness

Thank you for your interest in contributing to SanityHarness! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/sanityharness.git
   cd sanityharness
   ```
3. Set up the development environment:
   ```bash
   make tools    # Install dev tools
   make deps     # Download dependencies
   make build    # Build the CLI
   ```

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed setup instructions.

## Reporting Issues

### Bug Reports

When reporting bugs, please include:

- SanityHarness version (`./sanity version`)
- Go version (`go version`)
- Docker version (`docker --version`)
- Operating system and architecture
- Steps to reproduce the issue
- Expected vs actual behavior
- Relevant log output or error messages

### Feature Requests

For feature requests, please describe:

- The problem you're trying to solve
- Your proposed solution
- Alternative approaches you've considered
- Whether you'd be willing to implement it

## Making Changes

### Branch Naming

Use descriptive branch names with prefixes:

- `feature/` - New features (e.g., `feature/add-python-support`)
- `fix/` - Bug fixes (e.g., `fix/container-timeout-handling`)
- `docs/` - Documentation changes (e.g., `docs/improve-readme`)
- `refactor/` - Code refactoring (e.g., `refactor/simplify-runner`)

### Development Workflow

1. Create a feature branch:
   ```bash
   git checkout -b feature/my-feature
   ```

2. Make your changes

3. Run quality checks:
   ```bash
   make pre-commit
   ```

4. Run the full test suite:
   ```bash
   make test
   ```

5. Commit your changes:
   ```bash
   git add .
   git commit -m "Add feature X"
   ```

6. Push to your fork:
   ```bash
   git push origin feature/my-feature
   ```

7. Open a Pull Request

### Commit Messages

Write clear, concise commit messages:

- Use imperative mood ("Add feature" not "Added feature")
- Keep the first line under 50 characters
- Add detail in the body if needed

Good examples:
- `Add Python language support`
- `Fix container timeout handling for slow networks`
- `Update README with new configuration options`

Bad examples:
- `Fixed stuff`
- `WIP`
- `Updates`

## Code Style

### Import Order

Organize imports into three groups, separated by blank lines:

```go
import (
    "context"              // 1. Standard library
    "fmt"

    "github.com/spf13/cobra"  // 2. External dependencies

    "github.com/lemon07r/sanityharness/internal/config"  // 3. Internal packages
)
```

### Error Handling

Wrap errors with context using `fmt.Errorf`:

```go
if err := doSomething(); err != nil {
    return fmt.Errorf("doing something: %w", err)
}
```

### Documentation

- Add package documentation: `// Package runner provides...`
- Document exported functions: `// NewRunner creates a new runner with the given configuration.`
- Write complete sentences starting with the name being documented

### Formatting

Use `goimports` for formatting (automatically sorts imports):

```bash
make fmt
```

## Pre-Commit Checklist

Before submitting a PR, ensure:

- [ ] Code compiles: `make build`
- [ ] Code is formatted: `make fmt`
- [ ] Linting passes: `make lint`
- [ ] Tests pass: `make test`
- [ ] Pre-commit checks pass: `make pre-commit`

Or run all checks at once:

```bash
make check && make test
```

## Pull Request Process

1. **Ensure CI passes**: All GitHub Actions checks must be green
2. **Update documentation**: If you've added features or changed behavior
3. **Add tests**: For new features or bug fixes
4. **Request review**: Tag maintainers for review

### PR Title Format

Use a clear, descriptive title:

- `Add: Python language support`
- `Fix: Container timeout handling`
- `Docs: Update configuration guide`
- `Refactor: Simplify runner package`

### PR Description

Include in your PR description:

- What changes were made
- Why the changes were necessary
- How to test the changes
- Any breaking changes or migration steps

## Testing

### Running Tests

```bash
# All tests with race detection
make test

# Quick tests (skip integration)
make test-short

# Specific package
go test -v ./internal/runner

# Specific test
go test -v -run TestName ./internal/runner
```

### Writing Tests

- Use table-driven tests with `t.Run()`
- Place test files alongside code (`*_test.go`)
- Cover both happy path and error cases
- Use race detection (`-race` flag)

Example:

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "hello", "HELLO", false},
        {"empty input", "", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Something(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Something() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Something() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Adding New Tasks

To add a new task:

1. Create the task directory:
   ```
   tasks/<language>/<slug>/
   ├── task.toml
   ├── <slug>.go.txt        # Stub file
   └── <slug>_test.go.txt   # Test file
   ```

2. Define `task.toml`:
   ```toml
   slug = "my-task"
   name = "My Task"
   language = "go"
   tier = "core"
   difficulty = "hard"
   description = "..."
   
   [files]
   stub = ["my_task.go.txt"]
   test = ["my_task_test.go.txt"]
   support = ["go.mod.txt"]
   
   [validation]
   command = "go"
   args = ["test", "-race", "-v", "./..."]
   ```

3. Rebuild the binary (task files are embedded):
   ```bash
   make build
   ```

4. Test the task:
   ```bash
   ./sanity init <language>/my-task
   ./sanity run <language>/my-task
   ```

## Adding New Agents

To add a built-in agent, update the agent registry in `internal/cli/eval.go`.

For custom agents, users can configure them in `sanity.toml`:

```toml
[agents.my-agent]
command = "my-agent"
args = ["--auto", "{prompt}"]
model_flag = "-m"
```

See [CONFIGURATION.md](CONFIGURATION.md#agent-configuration) for details.

## Questions?

If you have questions:

1. Check existing [issues](https://github.com/lemon07r/sanityharness/issues)
2. Open a new issue with the "question" label
3. Reach out to maintainers

Thank you for contributing!
