# Available Tasks

SanityHarness includes 26 curated tasks across 6 programming languages, designed to test coding agents on challenging problems that require deep language understanding, concurrency handling, and algorithmic thinking.

## Task Reference Formats

Tasks can be referenced in two ways:

- **Canonical ID**: `<language>/<slug>` (e.g., `go/bank-account`) - always unambiguous
- **Bare slug**: `bank-account` - works only if the slug is unique across languages

For tasks that exist in multiple languages (e.g., `react` exists in both Go and TypeScript, `parallel-letter-frequency` exists in both Go and Rust), use the canonical form.

## Tasks by Language

### Go (6 tasks)

| Task | Description | Difficulty | Tier | Hidden Tests |
|------|-------------|------------|------|--------------|
| `bank-account` | Concurrent bank account with mutex synchronization | Hard | core | No |
| `dining-philosophers` | Classic concurrency problem solving | Hard | core | No |
| `errgroup-limit` | Bounded concurrency group that stops on first error | Hard | core | Yes |
| `parallel-letter-frequency` | Parallel text processing with goroutines | Hard | core | Yes |
| `react` | Reactive spreadsheet cells with callbacks | Hard | extended | Yes |
| `singleflight` | Deduplicate concurrent calls by key | Expert | extended | Yes |

### Rust (6 tasks)

| Task | Description | Difficulty | Tier | Hidden Tests |
|------|-------------|------------|------|--------------|
| `circular-buffer` | Generic circular buffer with ownership | Hard | core | No |
| `doubly-linked-list` | Unsafe Rust linked list implementation | Expert | extended | No |
| `generational-arena` | Arena allocator with generational handles | Hard | extended | Yes |
| `macros` | Declarative macro creation | Hard | core | Yes |
| `parallel-letter-frequency` | Multi-threaded text processing | Hard | core | Yes |
| `regex-lite` | Regex matching for `.`, `*` (full-string match) | Hard | core | Yes |

### TypeScript (5 tasks)

| Task | Description | Difficulty | Tier | Hidden Tests |
|------|-------------|------------|------|--------------|
| `csv-lite` | Parse CSV from a stream (quotes/escapes/CRLF) | Hard | core | Yes |
| `forth` | Stack-based language interpreter | Hard | core | Yes |
| `glob` | Glob pattern matching (`*`, `?`, escaping) | Hard | core | Yes |
| `promise-pool` | Promise pool with bounded concurrency | Hard | core | Yes |
| `react` | Reactive cell system with dependencies | Hard | extended | Yes |

### Kotlin (3 tasks)

| Task | Description | Difficulty | Tier | Hidden Tests |
|------|-------------|------------|------|--------------|
| `channel-multiplexer` | Combine multiple channels with priority support | Hard | extended | Yes |
| `flow-processor` | Composable Kotlin Flow processor with operators | Hard | extended | Yes |
| `lru-cache` | Fixed-capacity LRU cache with stable recency ordering | Hard | extended | Yes |

### Dart (3 tasks)

| Task | Description | Difficulty | Tier | Hidden Tests |
|------|-------------|------------|------|--------------|
| `future-pool` | Concurrency-limited async task runner preserving order | Hard | extended | Yes |
| `isolate-pool` | Worker pool using Dart isolates | Hard | extended | Yes |
| `reactive-cache` | Reactive cache with TTL and stream subscriptions | Hard | extended | Yes |

### Zig (3 tasks)

| Task | Description | Difficulty | Tier | Hidden Tests |
|------|-------------|------------|------|--------------|
| `arena-allocator` | Custom arena allocator with child arenas | Hard | extended | Yes |
| `comptime-json` | Compile-time JSON schema parsing | Expert | extended | Yes |
| `small-vector` | SmallVec with inline storage and heap growth | Hard | extended | Yes |

## Task Metadata

### Tiers

Tasks are organized into two tiers:

| Tier | Description | Count |
|------|-------------|-------|
| `core` | Essential benchmark tasks, run by default during eval | 12 |
| `extended` | Additional challenge tasks for comprehensive evaluation | 14 |

Use `--tier all` with `sanity eval` to include extended tasks.

### Difficulty Levels

| Difficulty | Description |
|------------|-------------|
| `hard` | Complex problems requiring deep language knowledge |
| `expert` | Most challenging tasks, often involving unsafe code or advanced features |

### Hidden Tests

Some tasks include hidden test files that are only applied during `sanity eval`. These tests:

- Validate edge cases not covered by visible tests
- Prevent agents from overfitting to visible test cases
- Are overlaid into the workspace before final validation
- Do not affect `sanity run` or `sanity init` commands

## Task Definition Schema

Each task is defined by a `task.toml` file:

```toml
slug = "bank-account"
name = "Bank Account"
language = "go"
tier = "core"                    # core | extended (default: core)
difficulty = "hard"              # hard | expert
description = "Implement a concurrent bank account with mutex synchronization"
timeout = 30                     # Validation timeout in seconds (optional)
agent_timeout = 120              # Agent timeout floor for eval (optional; cannot reduce a higher global timeout)

[files]
stub = ["bank_account.go.txt"]           # Files for agent to implement
test = ["bank_account_test.go.txt"]      # Visible test files
hidden_test = ["hidden_test.go.txt"]     # Hidden tests (eval only, optional)
support = ["go.mod.txt"]                 # Support files (read-only)

[validation]
command = "go"
args = ["test", "-race", "-v", "./..."]
```

### File Conventions

- Task files are stored with `.txt` extension in the embedded FS to prevent toolchain interference
- The `.txt` suffix is automatically stripped when copying to workspace
- Support files are protected during eval (integrity checks prevent modification)

## Filtering Tasks

### By Language

```bash
./sanity list --language go
./sanity list -l rust
./sanity eval --agent gemini --lang typescript
```

### By Tier

```bash
./sanity list --tier core
./sanity list --tier extended
./sanity eval --agent gemini --tier all
```

### By Difficulty

```bash
./sanity list --difficulty hard
./sanity list --difficulty expert
./sanity eval --agent gemini --difficulty hard,expert
```

### Specific Tasks

```bash
./sanity eval --agent gemini --tasks go/react,typescript/react
```

## External Tasks Directory

For development or custom tasks, use the `--tasks-dir` flag:

```bash
./sanity list --tasks-dir ./my-tasks
./sanity run my-task --tasks-dir ./my-tasks
./sanity eval --agent gemini --tasks-dir ./my-tasks
```

The external directory should follow the same structure as the embedded `tasks/` directory:

```
my-tasks/
├── go/
│   └── my-task/
│       ├── task.toml
│       ├── my_task.go.txt
│       └── my_task_test.go.txt
└── rust/
    └── another-task/
        └── ...
```
