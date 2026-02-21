# Road to v2.x.x Overhaul: What's Changed Since v1.6.1

If you want the short version: `1.7.x` is a fairness and reliability release. We reduced information leakage during eval, made infrastructure failures easier to reason about, and tightened task contracts where hidden requirements were too implicit.

## Why we changed anything

By the time we reached `v1.6.1`, we had enough evaluation history to see recurring issues:
- some outcomes were influenced by workspace visibility rather than pure task solving,
- transient agent or infrastructure problems were being mixed with normal failures,
- and a few tasks required behavior that was technically tested but not clearly stated.

`1.7.x` is the pass where we cleaned those up.

## What changed in evaluation behavior

The biggest shift is workspace isolation during `sanity eval`.

Agents now run in isolated temporary workspaces under `/tmp`, and then the harness copies the resulting code back into `eval-results` for validation. In practical terms, agents cannot inspect sibling tasks, prior eval outputs, or their own running log stream while solving.

This is the key fairness change in `v1.7.0`. It removes a class of accidental side-channel advantages and makes comparisons more defensible.

We also moved `agent.log` placement so it lives in task output directories (`eval-results/.../<task>/agent.log`) rather than inside the active workspace.

## What changed in failure handling

Another major improvement is how we handle infra-style failures.

In older behavior, empty or broken agent runs could look like regular task failures. In `1.7.x` we detect these cases explicitly, retry appropriately, and keep resume flow clear. The end result is that pass/fail numbers better reflect model behavior, not random execution flakiness.

Resume messaging and bookkeeping were also improved so interrupted or infra-affected runs are easier to continue safely.

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

Main commits in range:
- `a3a2758`
- `c69c19e`
- `5e972ea`
- `f192caf`
- `3567905`
- `ba25c9a`
