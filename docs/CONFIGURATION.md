# Configuration

SanityHarness is configured through TOML files and command-line flags.

## Config File Locations

Configuration files are searched in the following order (first found wins):

1. `./sanity.toml` (current directory)
2. `~/.sanity.toml` (home directory)
3. `~/.config/sanity/config.toml` (XDG config directory)

You can also specify a config file explicitly:

```bash
./sanity --config /path/to/config.toml list
```

## Harness Configuration

### [harness] Section

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `session_dir` | string | `"./sessions"` | Directory for session output |
| `default_timeout` | int | `30` | Default validation timeout in seconds |
| `max_attempts` | int | `5` | Maximum validation attempts per run |
| `output_format` | string | `"all"` | Output format: `json`, `human`, or `all` |

Example:

```toml
[harness]
session_dir = "./sessions"
default_timeout = 60
max_attempts = 10
output_format = "all"
```

### [docker] Section

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `go_image` | string | `ghcr.io/lemon07r/sanity-go:latest` | Go container image |
| `rust_image` | string | `ghcr.io/lemon07r/sanity-rust:latest` | Rust container image |
| `typescript_image` | string | `ghcr.io/lemon07r/sanity-ts:latest` | TypeScript container image |
| `kotlin_image` | string | `ghcr.io/lemon07r/sanity-kotlin:latest` | Kotlin container image |
| `dart_image` | string | `ghcr.io/lemon07r/sanity-dart:latest` | Dart container image |
| `zig_image` | string | `ghcr.io/lemon07r/sanity-zig:latest` | Zig container image |
| `auto_pull` | bool | `true` | Automatically pull missing images |

Example:

```toml
[docker]
go_image = "ghcr.io/lemon07r/sanity-go:latest"
rust_image = "ghcr.io/lemon07r/sanity-rust:latest"
typescript_image = "ghcr.io/lemon07r/sanity-ts:latest"
kotlin_image = "ghcr.io/lemon07r/sanity-kotlin:latest"
dart_image = "ghcr.io/lemon07r/sanity-dart:latest"
zig_image = "ghcr.io/lemon07r/sanity-zig:latest"
auto_pull = true
```

## Agent Configuration

SanityHarness supports 15 built-in coding agents and allows custom agent definitions.

### Built-in Agents

| Agent | Command | Args Pattern | Model Flag | Reasoning Flag |
|-------|---------|--------------|------------|----------------|
| `gemini` | `gemini` | `--yolo {prompt}` | `--model` (before) | - |
| `kilocode` | `kilocode` | `--auto --yolo --mode code {prompt}` | `--model` (before) | - |
| `opencode` | `opencode` | `run {prompt}` | `-m` (after) | - |
| `claude` | `claude` | `-p --dangerously-skip-permissions {prompt}` | `--model` (before) | - |
| `codex` | `codex` | `exec --dangerously-bypass-approvals-and-sandbox {prompt}` | `-m` (before) | `-c model_reasoning_effort={value}` (before) |
| `kimi` | `kimi` | `--yolo -c {prompt}` | `-m` (before) | - |
| `crush` | `crush` | `run {prompt}` | - | - |
| `copilot` | `copilot` | `--allow-all-tools -i {prompt}` | `--model` (before) | - |
| `droid` | `droid` | `exec --skip-permissions-unsafe {prompt}` | `-m` (after) | `-r` (after) |
| `iflow` | `iflow` | `--yolo -p {prompt}` | `-m` (before) | - |
| `qwen` | `qwen` | `--yolo {prompt}` | `-m` (before) | - |
| `amp` | `amp` | `--dangerously-allow-all -x {prompt}` | `-m` (before) | - |
| `codebuff` | `codebuff` | `{prompt}` | `--{value}` (before) | - |
| `vibe` | `vibe` | `--prompt {prompt}` | - | - |
| `goose` | `goose` | `run --no-session -t {prompt}` | `--model` (after) | - |
| `ccs` | `ccs` | `-p --dangerously-skip-permissions {prompt}` | `{value}` (before) | `--thinking` (before) |

### Custom Agent Schema

Define custom agents in your `sanity.toml`:

```toml
[agents.my-agent]
command = "/path/to/my-agent"         # Binary name or path (required)
args = ["--auto-approve", "{prompt}"] # Arguments with {prompt} placeholder (required)
model_flag = "-m"                     # Flag for specifying model (optional)
model_flag_position = "before"        # "before" (default) or "after" args
reasoning_flag = "-r"                 # Flag for reasoning effort (optional)
reasoning_flag_position = "after"     # "before" (default) or "after" args
env = { API_KEY = "xxx" }             # Environment variables (optional)
```

### Overriding Built-in Agents

You can override built-in agents to change their default behavior:

```toml
# Use a specific model by default for gemini
[agents.gemini]
command = "gemini"
args = ["--yolo", "--model", "gemini-3-pro", "{prompt}"]

# Add custom environment variables to opencode
[agents.opencode]
command = "opencode"
args = ["run", "{prompt}"]
model_flag = "-m"
model_flag_position = "after"
env = { CUSTOM_VAR = "value" }
```

### Placeholder Syntax

#### `{prompt}` Placeholder

The `{prompt}` placeholder in `args` is replaced with the task prompt:

```toml
args = ["--execute", "{prompt}"]
# Becomes: --execute "Implement the bank-account task..."
```

#### `{value}` Placeholder

The `{value}` placeholder in `model_flag` or `reasoning_flag` allows inline substitution:

```toml
# Simple flag (value as separate argument)
model_flag = "-m"
# Result: -m gemini-3-pro

# Placeholder format (value inline)
model_flag = "--model={value}"
# Result: --model=gemini-3-pro

# Mode-based flags (for agents like codebuff)
model_flag = "--{value}"
# With --model max, becomes: --max
```

### Model Flag Position

The `model_flag_position` determines where the model flag is inserted:

- `"before"` (default): Model flag appears before `args`
- `"after"`: Model flag appears after `args`

```bash
# model_flag_position = "before"
my-agent -m gemini-3-pro --auto-approve "prompt"

# model_flag_position = "after"  
my-agent --auto-approve "prompt" -m gemini-3-pro
```

### Reasoning Effort

Some agents support configurable reasoning/thinking effort levels.

#### Supported Agents

| Agent | Levels | Flag Format |
|-------|--------|-------------|
| `droid` | `off`, `none`, `low`, `medium`, `high` | `-r <level>` |
| `codex` | `minimal`, `low`, `medium`, `high`, `xhigh` | `-c model_reasoning_effort=<level>` |

#### Usage

```bash
./sanity eval --agent droid --reasoning high
./sanity eval --agent codex --reasoning medium
./sanity eval --agent droid --reasoning high --model claude-opus-4-5-20251101
```

#### Configuration

```toml
# Simple flag (value as separate argument)
[agents.droid]
reasoning_flag = "-r"
reasoning_flag_position = "after"
# Result: droid exec ... -r high

# Placeholder format (value inline)
[agents.codex]
reasoning_flag = "-c model_reasoning_effort={value}"
reasoning_flag_position = "before"
# Result: codex -c model_reasoning_effort=high exec ...
```

The reasoning flag is only passed when you specify `--reasoning <level>` on the command line.

### MCP Tools Control

For agents with MCP (Model Context Protocol) tools, you can control their behavior:

#### Enable MCP Tool Usage

The `--use-mcp-tools` flag injects instructions encouraging the agent to leverage its available tools:

```bash
./sanity eval --agent gemini --use-mcp-tools
```

This appends guidance to the agent prompt that:
- Encourages proactive use of MCP tools for file reading and code search
- Advises against guessing at implementation details
- Prioritizes using tools to gather context over making assumptions

#### Disable MCP Tools

The `--disable-mcp` flag disables MCP tools entirely for agents that support it:

```bash
./sanity eval --agent opencode --disable-mcp
```

Currently supported agents:
- `opencode`: Sets `OPENCODE_CONFIG_CONTENT={"tools":{"*_*":false}}` to disable all MCP server tools

This is useful for benchmarking agents without external tool access.

### Environment Variables

Custom environment variables can be set per agent:

```toml
[agents.my-agent]
command = "my-agent"
args = ["{prompt}"]
env = { 
    API_KEY = "sk-xxx",
    DEBUG = "true",
    CUSTOM_ENDPOINT = "https://api.example.com"
}
```

These are merged with the process environment when the agent is invoked.

## Cache Configuration

SanityHarness maintains persistent caches in `.sanity-cache/` to speed up repeated runs.

### Cache Mount Locations

| Language | Host Path | Container Path |
|----------|-----------|----------------|
| Go | `.sanity-cache/go/gocache` | `/tmp/sanity-go-build-cache` |
| Go | `.sanity-cache/go/gomodcache` | `/tmp/sanity-go-mod-cache` |
| Rust | `.sanity-cache/rust/cargo-home` | `/tmp/sanity-cargo-home` |
| Rust | `.sanity-cache/rust/cargo-target` | `/tmp/sanity-cargo-target` |
| TypeScript | `.sanity-cache/typescript/npm-cache` | `/tmp/sanity-npm-cache` |
| Kotlin | `.sanity-cache/kotlin/gradle-home` | `/tmp/sanity-gradle-home` |
| Dart | `.sanity-cache/dart/pub-cache` | `/tmp/sanity-pub-cache` |
| Zig | `.sanity-cache/zig/zig-cache` | `/tmp/.zig-cache` |

### Cache Behavior

- Caches are created automatically on first run
- They persist across runs for faster builds
- The `.sanity-cache/` directory is gitignored
- Safe to delete at any time (will be recreated)

### Environment Variables in Containers

Each container has environment variables set to redirect caches to `/tmp`:

```bash
# Go
GOCACHE=/tmp/sanity-go-build-cache
GOMODCACHE=/tmp/sanity-go-mod-cache

# Rust
CARGO_HOME=/tmp/sanity-cargo-home
CARGO_TARGET_DIR=/tmp/sanity-cargo-target

# And similar for other languages...
```

This prevents caches from being written to the workspace directory.

## Complete Configuration Example

```toml
[harness]
session_dir = "./sessions"
default_timeout = 60
max_attempts = 10
output_format = "all"

[docker]
go_image = "ghcr.io/lemon07r/sanity-go:latest"
rust_image = "ghcr.io/lemon07r/sanity-rust:latest"
typescript_image = "ghcr.io/lemon07r/sanity-ts:latest"
kotlin_image = "ghcr.io/lemon07r/sanity-kotlin:latest"
dart_image = "ghcr.io/lemon07r/sanity-dart:latest"
zig_image = "ghcr.io/lemon07r/sanity-zig:latest"
auto_pull = true

# Override gemini to use a specific model
[agents.gemini]
command = "gemini"
args = ["--yolo", "--model", "gemini-3-pro", "{prompt}"]

# Add a custom agent
[agents.my-custom-agent]
command = "/usr/local/bin/my-agent"
args = ["--auto-approve", "--format", "json", "{prompt}"]
model_flag = "-m"
model_flag_position = "before"
reasoning_flag = "-r"
reasoning_flag_position = "after"
env = { MY_API_KEY = "xxx", DEBUG = "true" }
```
