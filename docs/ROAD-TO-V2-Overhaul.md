# Road to v2.x.x Overhaul: What's Changed Since v1.6.1

If you want the short version: `1.7.x` is a fairness and reliability release. We reduced information leakage during eval, made infrastructure failures easier to reason about, and tightened task contracts where hidden requirements were too implicit. `1.8.x` adds multi-run orchestration for statistical rigor and cross-agent comparison.

## Why we changed anything

By the time we reached `v1.6.1`, we had enough evaluation history to see recurring issues:
- some outcomes were influenced by workspace visibility rather than pure task solving,
- transient agent or infrastructure problems were being mixed with normal failures,
- and a few tasks required behavior that was technically tested but not clearly stated.

`1.7.x` is the pass where we cleaned those up. `1.8.x` builds on that foundation to make multi-run evaluation a first-class workflow.

## What changed in evaluation behavior

The biggest shift is workspace isolation during `sanity eval`.

Agents now run in isolated temporary workspaces under `/tmp`, and then the harness copies the resulting code back into `eval-results` for validation. In practical terms, agents cannot inspect sibling tasks, prior eval outputs, or their own running log stream while solving.

This is the key fairness change in `v1.7.0`. It removes a class of accidental side-channel advantages and makes comparisons more defensible.

We also moved `agent.log` placement so it lives in task output directories (`eval-results/.../<task>/agent.log`) rather than inside the active workspace.

## What changed in failure handling

Another major improvement is how we handle infra-style failures.

In older behavior, empty or broken agent runs could look like regular task failures. In `1.7.x` we detect these cases explicitly, retry appropriately, and keep resume flow clear. The end result is that pass/fail numbers better reflect model behavior, not random execution flakiness.

Resume messaging and bookkeeping were also improved so interrupted or infra-affected runs are easier to continue safely.

## What changed in timeouts, retries, and scoring

SanityHarness evaluates coding ability, not provider infrastructure quality. Several defaults were recalibrated to stop penalizing agents for slow or flaky providers:

- **Default agent timeout increased from 120s to 600s.** Many agents need 3–5 minutes for harder tasks, especially with slower providers. Timeouts caused by provider latency no longer cut into scores.
- **Partial pass scoring penalty removed.** A correct solution produced by a timed-out agent now scores 100% of the task weight, the same as a clean pass. The `partial_pass` status is still recorded for diagnostics, but it no longer reduces the score. `WeightVersion` bumped to `"2.1"`.
- **Quota retries increased from 3 to 5** with longer exponential backoff (30s → 60s → 120s → 240s → 480s). The consecutive quota-exhausted stop threshold also moved from 3 to 5 to avoid premature eval termination during bursty rate limits.
- **Infrastructure failure retries separated from quota retries.** Infra failures (empty agent logs, provider connection drops) now have their own independent budget of 5 retries with backoff (15s → 30s → 60s → 120s → 240s). A provider that drops connections no longer eats into the quota retry budget.
- **`CleanPasses` and `PartialPasses` removed from output.** With partial pass scoring equalized, these metrics were noise. They have been removed from `summary.json`, `submission.json`, and `report.md`.

## What changed in prompts and test stability

We updated eval prompts to include explicit toolchain versions. That was a practical fix for frequent version mismatch mistakes, especially in ecosystems with fast API churn.

We also removed machine-dependent wall-clock assertions from hidden tests in `rust/regex-lite`. The task is still challenging, but now runtime pressure is enforced by harness/container timeouts instead of host-specific timing thresholds.

## What changed in task specs

Several low-pass tasks were failing for reasons that looked more like under-specified requirements than true capability gaps.  
For those tasks, we tightened textual contracts in stubs and comments so hidden expectations are inferable without giving away implementation details.

Tasks updated in this pass:
- `tasks/typescript/promise-pool/promise_pool.ts`
- `tasks/dart/future-pool/lib/future_pool.dart`
- `tasks/typescript/csv-lite/csv.ts`
- `tasks/rust/macros/lib.rs`
- `tasks/kotlin/channel-multiplexer/src/main/kotlin/ChannelMultiplexer.kt`
- `tasks/dart/reactive-cache/lib/reactive_cache.dart`
- `tasks/dart/isolate-pool/lib/isolate_pool.dart`
- `tasks/kotlin/flow-processor/src/main/kotlin/FlowProcessor.kt`
- `tasks/zig/arena-allocator/arena.zig`
- `tasks/zig/comptime-json/json.zig`

The intent here was high-signal evaluation: remove "mind-reading" requirements, but do not turn tasks into copy-paste exercises.

## What changed in MCP tools mode

The `--use-mcp-tools` prompt was rewritten to be minimal and neutral. The previous prompt included workflow coaching (read files, search code, don't guess) that could inflate scores independent of actual MCP tool usage. The new prompt is a single sentence nudging the agent to use its MCP tools proactively, without teaching problem-solving strategy. This makes with-vs-without comparisons fairer.

A new `mcp_prompt` config field was added to `AgentConfig`, allowing per-agent MCP tool guidance (e.g., telling Gemini to use `@web` search). This is appended under an `AGENT-SPECIFIC TOOLS:` header when `--use-mcp-tools` is set.

## What changed in v1.8.0 — multi-run orchestration

Single eval runs are useful for quick checks, but meaningful comparison requires repetition and side-by-side analysis. `1.8.0` adds three complementary features that share a common orchestration layer:

**`--repeat N` flag.** Run the same eval configuration N times to measure variance. Each repeat gets its own subdirectory under a multi-run parent, and the harness produces aggregate statistics (mean, stddev, min, max) for pass rate and weighted score across repeats.

**Comma-separated multi-agent.** Quick A/B comparison from the CLI without batch files. `--agent codex,opencode --model gpt-5.2,kimi-k2.5` broadcasts shared flags across agents and produces a multi-run directory with per-agent subdirectories and a comparison summary. The `--repeat` flag composes with multi-agent: `--agent codex,opencode --repeat 3` produces 6 total runs.

**`sanity batch --config runs.toml`.** For complex multi-run configurations with per-run overrides, shared defaults, and repeat support. The batch file is TOML with a `[shared]` section for defaults and `[[runs]]` entries for per-run overrides.

**`sanity compare <dir1> <dir2> ...`.** Load summaries from multiple eval result directories and produce a side-by-side comparison table. Works with any combination of single-run and multi-run directories.

The internal change that enables all of this is the extraction of the monolithic `evalCmd.RunE` into a reusable `evalRunSingle()` function. The top-level `RunE` is now a thin orchestration wrapper that parses multi-agent specs, manages the multi-run directory structure, and delegates individual runs.

Multi-run directories use a `multi-run-config.json` config and `multi-run-state.json` for resume support. Interrupted multi-runs can be resumed with `--resume`, which detects completed sub-runs and continues from where it left off.

Phase 5 (parallel runs with `--parallel-runs`) is deferred to a future release.

## What changed in v1.8.0-alpha.3 — ARM readiness

This pre-release focuses on making the harness reliable on `arm64` hosts while keeping behavior explicit when image architecture is mismatched.

- **Early image platform validation in the runner.** `EnsureImage` now inspects the local/pulled image platform and fails fast with a clear error if the image does not match the host architecture, instead of failing later at container create/run time.
- **Zig container is now architecture-aware.** The Zig Dockerfile now selects the correct upstream tarball for `amd64` (`x86_64`) and `arm64` (`aarch64`) builds.
- **Docker image publishing now includes both Linux architectures.** The Docker workflow now builds and pushes `linux/amd64` and `linux/arm64` images for all six runtime images (`go`, `rust`, `typescript`, `kotlin`, `dart`, `zig`) using Buildx + QEMU.

Operationally, this means ARM users get deterministic behavior:
- either a matching image runs normally,
- or the harness exits with an actionable platform mismatch message telling you to publish/build the needed architecture or override image config.

## What changed in v1.8.0-alpha.4 — output contracts and auditability

This pre-release tightened output invariants so resume/verification tooling and downstream parsers can rely on stable files even in edge cases.

- `agent.log` now gets a deterministic `HARNESS:` timeout footer when the agent times out.
- `validation.log` is now always written and always ends with a `HARNESS:` footer (`command`, `exit_code`, `duration_seconds`, `timed_out`, optional `run_error`), even when raw validator output is empty.
- `run-config.json`, `summary.json`, and `submission.json` now emit key boolean/counter fields explicitly even when false/zero (for example `use_mcp_tools`, `disable_mcp`, `no_sandbox`, `legacy`, and retry counters). This avoids schema drift across runs.
- Validation commands were normalized for several tasks so task contracts, execution behavior, and log metadata stay aligned.

The net effect is better auditability and fewer ambiguous "empty log" outcomes.

## What changed in v1.8.0-alpha.5 and alpha.6 — sandbox hardening

The sandbox model moved from a narrow writable-dir override to an explicit compatibility allowlist plus stricter masking.

- Added `[sandbox] shared_readwrite_dirs` and `[sandbox] shared_readonly_dirs` to configuration, with broad defaults covering common auth/cache/toolchain locations.
- Bubblewrap setup now masks non-allowlisted top-level directories under `$HOME`, then bind-mounts allowlisted paths with explicit read/write or read-only intent.
- Denylist masking was expanded for sensitive host paths (including `tasks`, `eval-results`, and `sessions`), with extra entries configurable via `[sandbox] readable_denylist`.
- Fixed a symlink canonicalization regression in denylist handling so masked paths remain masked even when traversed through symlinked parents.

This keeps agents functional (expected config/cache access still works) while reducing exposure to unrelated host data.

## What changed after v1.8.0-alpha.6 — failure taxonomy, telemetry, and resumable externals

Recent `1.8.x` work focused on separating model failures from provider/auth/infra failures and making that visible in outputs.

- Added `FailureClass` to per-task results: `none`, `quota_recoverable`, `quota_exhausted`, `auth`, `infra`, `integrity`, `validation_error`, `validation_timeout`.
- External failures (`auth`, `quota_exhausted`, `infra`) are now treated as resumable skips: they are excluded from pass/fail scoring for that run, task artifacts are cleaned, and the harness prints a `--resume` command to retry only those tasks.
- Added early-stop protection when quota exhaustion repeats: after 5 consecutive `quota_exhausted` outcomes, eval stops and preserves resumable state instead of burning more requests.
- Summary/submission/report outputs now include richer counters (`auth_affected_tasks`, `infra_affected_tasks`, `total_infra_retries`, plus quota counters) and report-level failure-class breakdown tables.
- Added behavior telemetry from agent logs (self-test command count, toolchain install attempts, out-of-workspace read attempts) with confidence flags so analysis can distinguish strict parsing from fallback heuristics.

The practical outcome is cleaner benchmarking: weighted scores reflect task-solving ability, while external instability is tracked separately and recoverably.

## What changed in v1.8.2 — prompt integration and workspace cleanup

Two focused improvements to eval ergonomics:

- **MCP tool guidance integrated into task prompt sections.** Instead of appending a separate `MCP TOOLS:` block, MCP guidance is now woven into the existing `ENVIRONMENT`, `YOUR TASK`, `IMPORTANT`, and `RULES` sections when `--use-mcp-tools` is enabled. This produces a more natural prompt structure and makes MCP tool usage a first-class instruction rather than an afterthought. Agent-specific `mcp_prompt` text is no longer appended separately.
- **Workspace cleanup preserves eval artifacts.** Previously, `--keep-workspaces=false` (the default) removed the entire workspace directory after validation, which also deleted `agent.log`, `validation.log`, and integrity artifacts since the workspace dir doubles as the task output dir. Cleanup now selectively removes only source files while preserving harness-produced outputs (`agent.log`, `validation.log`, `integrity.json`, `integrity-files/`, `integrity-diff/`).

## What changed in v1.8.4 — Agent Skills prompting and telemetry

This release focuses on making `--use-skills` behavior measurable and auditable.

- **Stronger skills prompt contract.** The eval prompt now explicitly instructs agents to load at least one relevant skill (when available) and make the first skill call before writing code.
- **Per-task skills telemetry.** `results[]` now records `skills_used` and `skills_usage_signals` so skill adoption can be analyzed per task.
- **Run-level skills metrics.** `summary.json`, `submission.json`, and `report.md` now include:
  - `skills_usage_rate`
  - `total_skills_usage_signals`
  - `tasks_with_skills_usage`
- **OpenCode-compatible skill signal detection.** Telemetry parsing now detects common OpenCode skill markers like `Skill "..."` activations and explicit `firecrawl ...` command usage, in addition to file-path based skill artifact signals.

Practical impact: A/B runs with and without `--use-skills` can now show an observable usage delta instead of only a mode toggle.

## What changed in v1.8.5 — Agent Skills workspace integration and prompt rewrite

This release makes `--use-skills` actually trigger skill usage by agents, rather than just injecting a prompt hint.

- **Skills copied into task workspaces.** When `--use-skills` is enabled, the harness now copies skill directories from `~/.agents/skills/` into each task workspace under `.agents/skills/`. Previously, skills were only referenced in the prompt but not present in the workspace, so agents had no way to discover or read `SKILL.md` files.
- **Agent-agnostic skills prompt.** The skills prompt was rewritten to be fully generic: it tells the agent that skills exist in `.agents/skills/`, to read the `SKILL.md` files, and to execute skill commands directly in the terminal. No specific skill names, capabilities, or workflows are mentioned — the `SKILL.md` content drives agent behavior. This replaces the previous `activate_skill` wording that referenced a tool only available in Codex.
- **Toolchain search telemetry.** Agent behavior metrics now track `toolchain_search_attempts` and `tasks_with_toolchain_search` separately from `toolchain_install_attempts`, distinguishing agents that search for compilers (e.g., `find / -name dart`) from those that try to install them.
- **Skills path whitelisting.** The sandbox and out-of-workspace penalty system now whitelists `.agents/skills/` paths so agents can read skill files without triggering penalties.

Practical impact: Agents with `--use-skills` now actually discover and execute skill tools (e.g., `firecrawl search ...`) instead of ignoring them. Early testing shows ~27% of tasks triggering real skill commands, up from 0% in all previous runs.

## Compatibility and comparing old runs

`1.7.x` is intentionally not identical to `v1.6.1` behavior. If you are comparing against historical leaderboard-era runs, use legacy mode:

```bash
./sanity eval --legacy
```

Use default mode for current evaluations. Use legacy mode only when you need apples-to-apples historical comparison. 

## Commit range

This document covers:
- baseline: `v1.6.1`
- through: current `HEAD` on `main`

Notable commits in range (chronological):
- `a3a2758`
- `c69c19e`
- `5e972ea`
- `f192caf`
- `3567905`
- `ba25c9a`
- `a17ac5c`
- `9c0e0b4`
- `2e63c8a`
- `0dab2c5`
- `6a67453`
- `7e2d943`
- `6e6993f`
- `8b8f55d`
- `6f61a69`
- `d027b85`
- `1b26446`
- `14efc13`
- `f9c3a14`
- `09f44ee`
- `62f8538`
- `097d088`
