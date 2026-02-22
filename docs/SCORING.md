# Scoring System

SanityHarness uses a weighted scoring system (v2.1) based on empirically-derived difficulty factors calibrated from agent performance analysis.

## Overview

The scoring system accounts for:
- Task completion status
- Task difficulty (language rarity, esoteric features, novel algorithms)
- Integrity of test/support files
- Agent timeout behavior

## Result Statuses

Each task result has one of the following statuses:

| Status | Description |
|--------|-------------|
| `pass` | Tests passed, agent completed within timeout |
| `partial_pass` | Tests passed, but agent timed out (solution was already correct; scores same as `pass`) |
| `fail` | Tests failed |
| `integrity_violation` | Agent modified protected files (tests or support files) |
| `error` | Execution error (container failure, validation error, etc.) |

## Scoring Rules

| Status | Score |
|--------|-------|
| Clean pass (`pass`) | 100% of task weight |
| Partial pass (`partial_pass`) | 100% of task weight |
| Fail (`fail`) | 0 points |
| Integrity violation (`integrity_violation`) | -0.25 penalty |
| Error (`error`) | 0 points |

### Examples

- Task with weight 1.2:
  - Clean pass: 1.2 points
  - Fail: 0 points
  - Integrity violation: -0.25 points

## Task Weight Formula

Task weights range from 1.0 to 1.5 and are calculated as:

```
weight = 1.0
       + lang_rarity * 0.5
       + esoteric_feature * 0.8
       + novel_algorithm * 0.6
       + edge_case_density * 0.4
       + novel_problem * 0.2
```

The result is capped at 1.5.

### Difficulty Factors

| Factor | Description | Example Values |
|--------|-------------|----------------|
| **Language rarity** | Languages less common in training data | Dart=0.4, Kotlin=0.3, Zig=0.2 |
| **Esoteric features** | Advanced language-specific features | comptime=0.5, macros=0.5, isolates=0.4 |
| **Novel algorithms** | Implementing algorithms from scratch | regex from scratch=0.4, parser=0.2 |
| **Edge case density** | Complex error handling, streaming, concurrency | Varies by task |
| **Novel problems** | Less documented patterns vs classic problems | Varies by task |

### Weight Examples

| Task | Language | Factors | Weight |
|------|----------|---------|--------|
| `bank-account` | Go | Standard concurrency | 1.0 |
| `comptime-json` | Zig | Zig rarity + comptime | 1.5 |
| `isolate-pool` | Dart | Dart rarity + isolates | 1.4 |
| `macros` | Rust | Esoteric (macros) | 1.4 |
| `regex-lite` | Rust | Novel algorithm | 1.24 |

## Session Output

Each `sanity run` creates a session directory:

```
sessions/<lang>-<slug>-<timestamp>-<random>/
├── result.json      # Structured results with attempts, timing, final code
├── report.md        # Human-readable Markdown summary
├── logs/
│   ├── attempt-1.log
│   └── attempt-2.log
└── workspace/       # Final code snapshot
    ├── solution.go
    └── go.mod
```

### Session ID Format

Session IDs include:
- Language prefix
- Task slug
- ISO timestamp
- 8-character random hex suffix (prevents collisions)

Example: `go-bank-account-2026-01-15T143022-a1b2c3d4`

### result.json Schema

```json
{
  "task": "go/bank-account",
  "status": "pass",
  "attempts": 2,
  "total_duration_ms": 4523,
  "final_attempt": {
    "number": 2,
    "exit_code": 0,
    "duration_ms": 2341,
    "output": "...",
    "error_summary": null
  },
  "all_attempts": [
    {
      "number": 1,
      "exit_code": 1,
      "duration_ms": 2182,
      "output": "...",
      "error_summary": "undefined: Account"
    },
    {
      "number": 2,
      "exit_code": 0,
      "duration_ms": 2341,
      "output": "...",
      "error_summary": null
    }
  ]
}
```

## Eval Output

Each `sanity eval` creates an output directory:

```
eval-results/<agent>-<timestamp>/
├── summary.json       # Complete results with weighted scores
├── attestation.json   # BLAKE3 hashes for verification
├── report.md          # Human-readable Markdown report
├── submission.json    # Compact format for leaderboard
├── run-config.json    # Original run configuration (resume + audit)
└── <lang>-<slug>/
    ├── agent.log      # Agent output (includes HARNESS timeout footer on agent timeout)
    └── validation.log # Validation output (always includes HARNESS footer)
```

### summary.json Schema

```json
{
  "agent": "gemini",
  "model": "gemini-3-flash-preview",
  "reasoning": "high",
  "use_mcp_tools": false,
  "disable_mcp": false,
  "timestamp": "2026-01-07T052902",
  "harness_version": "abc123",
  "weight_version": "2.0",
  
  "total": 26,
  "passed": 13,
  "failed": 12,
  "errors": 1,
  
  "integrity_violations": 0,
  
  "pass_rate": 50.0,
  "weighted_pass_rate": 45.5,
  "weighted_score": 15.12,
  "max_possible_score": 33.29,
  
  "by_language": {
    "go": { "passed": 3, "failed": 3, "total": 6, "pass_rate": 50.0 }
  },
  "by_tier": {
    "core": { "passed": 8, "failed": 4, "total": 12, "pass_rate": 66.7 },
    "extended": { "passed": 5, "failed": 8, "total": 14, "pass_rate": 35.7 }
  },
  "by_difficulty": {
    "hard": { "passed": 12, "failed": 10, "total": 22, "pass_rate": 54.5 },
    "expert": { "passed": 1, "failed": 3, "total": 4, "pass_rate": 25.0 }
  },
  
  "results": [
    {
      "task": "go/bank-account",
      "status": "pass",
      "weight": 1.0,
      "score": 1.0,
      "duration_ms": 45000,
      "attempts": 1
    }
  ]
}
```

Notes:
- `timeout`, `parallel`, `use_mcp_tools`, `disable_mcp`, `sandbox`, `legacy`,
  `quota_affected_tasks`, and `total_quota_retries` are always emitted.
- Per-task `results[]` include explicit retry/infra metadata fields.

### attestation.json Schema

```json
{
  "harness_version": "abc123",
  "weight_version": "2.0",
  "timestamp": "2026-01-07T05:29:02Z",
  
  "tasks_hash": "blake3:a1b2c3d4...",
  "results_hash": "blake3:e5f6g7h8...",
  
  "task_hashes": {
    "go/bank-account": "blake3:...",
    "go/react": "blake3:..."
  },
  "solution_hashes": {
    "go/bank-account": "blake3:...",
    "go/react": "blake3:..."
  }
}
```

The attestation includes:
- **task_hash**: Hash of task files (stub + test + support) - detects task modifications
- **solution_hash**: Hash of solution files after agent run
- **tasks_hash**: Combined hash of all task hashes
- **results_hash**: Hash of the results JSON array

### submission.json Schema

Optimized for leaderboard submissions:

```json
{
  "agent": "gemini",
  "model": "gemini-3-flash-preview",
  "reasoning": "high",
  "use_mcp_tools": false,
  "disable_mcp": false,
  "timestamp": "2026-01-07T052902",
  
  "pass_rate": 50.0,
  "weighted_pass_rate": 45.5,
  "passed": 13,
  "failed": 13,
  "total": 26,
  
  "weighted_score": 15.12,
  "max_possible_score": 33.29,
  
  "integrity_violations": 0,
  
  "by_language": {
    "go": { "passed": 3, "failed": 3, "total": 6, "pass_rate": 50.0 }
  },
  
  "harness_version": "abc123",
  "weight_version": "2.0",
  "tasks_hash": "blake3:...",
  "results_hash": "blake3:..."
}
```

Notes:
- `submission.json` includes run metadata and audit counters:
  `timeout`, `parallel`, `quota_affected_tasks`, and `total_quota_retries`.
- Configuration booleans (`use_mcp_tools`, `disable_mcp`, `sandbox`, `legacy`)
  are always emitted as explicit booleans.

### report.md Format

The Markdown report includes:
- Summary table (agent, model, timestamp, pass rate, weighted score)
- Results table with status icons
- Breakdowns by language, tier, and difficulty
- Links to individual task logs

## Verification

Verify the integrity of an eval submission:

```bash
./sanity verify ./eval-results/gemini-2026-01-07T120000
```

Verification checks:
1. **Results hash**: Ensures `summary.json` wasn't modified after generation
2. **Task hashes**: Ensures task files match the embedded version
3. **Version compatibility**: Checks harness version matches

### Verification Output

```
Verifying submission: gemini-2026-01-07T120000

[PASS] Results hash matches
[PASS] All 26 task hashes match embedded tasks
[PASS] Harness version compatible

Submission verified successfully.
```

### Verification Failures

```
[FAIL] Results hash mismatch
       Expected: blake3:a1b2c3d4...
       Got:      blake3:x9y8z7w6...

[WARN] Task hash mismatch for go/bank-account
       Submission may have used modified task files

[WARN] Harness version mismatch
       Submission: v1.0.0
       Current:    v1.1.0
```

## Error Summarization

SanityHarness extracts human-readable error summaries from test output using language-specific regex patterns:

| Language | Example Errors |
|----------|----------------|
| Go | Race conditions, deadlocks, type mismatches, undefined symbols, panics |
| Rust | Borrow checker (E0382, E0499, E0502, E0597), trait bounds, panics |
| TypeScript | TS2322 (type assignment), TS2339 (missing property), TS2304 (undefined) |
| Kotlin | Unresolved references, type mismatches, null safety, exceptions |
| Dart | Method not found, type mismatches, null safety, missing implementations |
| Zig | Compilation errors, type mismatches, panics, unreachable code |

Error summaries appear in:
- Session `result.json` (per attempt)
- Terminal output during `sanity run`
- Eval `report.md` for failed tasks
