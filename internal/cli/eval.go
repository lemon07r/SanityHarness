package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/zeebo/blake3"

	"github.com/lemon07r/sanityharness/internal/config"
	resultpkg "github.com/lemon07r/sanityharness/internal/result"
	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var (
	evalAgent string
	evalModel string
	// TODO(consistency): Consider passing evalReasoning explicitly through the call
	// stack (runTaskWithAgent -> executeAgentWithRetries -> runAgentAttempt) to match
	// the pattern used for model. Currently safe since it's read-only after CLI parse.
	evalReasoning       string
	evalTasks           string
	evalLang            string
	evalTier            string
	evalDifficulty      string
	evalTimeout         int
	evalOutputDir       string
	evalKeepWorkspaces  bool
	evalParallel        int
	evalDryRun          bool
	evalUseMCPTools     bool
	evalUseSkills       bool
	evalDisableMCP      bool
	evalNoSandbox       bool
	evalLegacy          bool
	evalSandboxActive   bool
	evalSandboxDenylist []string
	evalSandboxSharedRW []string
	evalSandboxSharedRO []string
	evalResume          string
	evalRepeat          int
)

// Quota retry configuration.
const (
	quotaMaxRetries  = 5
	quotaRetryDelay1 = 30 * time.Second
	quotaRetryDelay2 = 60 * time.Second
	quotaRetryDelay3 = 120 * time.Second
	quotaRetryDelay4 = 240 * time.Second
	quotaRetryDelay5 = 480 * time.Second

	// Threshold for considering an agent log as an infra failure (empty or near-empty).
	infraFailureLogThreshold = 10 // bytes

	// Stop eval early after this many consecutive quota-exhausted tasks.
	// This allows resuming later when quota resets, rather than wasting time
	// on tasks that will all fail immediately.
	quotaExhaustedStopThreshold = 5
)

// Infra failure retry configuration (separate from quota retries).
const (
	infraMaxRetries  = 5
	infraRetryDelay1 = 15 * time.Second
	infraRetryDelay2 = 30 * time.Second
	infraRetryDelay3 = 60 * time.Second
	infraRetryDelay4 = 120 * time.Second
	infraRetryDelay5 = 240 * time.Second
)

// Patterns indicating recoverable rate limit errors (worth retrying).
// These use contextual phrases to avoid false positives from bare numbers
// appearing in durations (e.g. "0.503s"), UUIDs, git hashes, line numbers, etc.
var recoverablePatterns = []string{
	"rate limit",
	"rate_limit",
	"ratelimit",
	"too many requests",
	"overload",
	"temporarily unavailable",
	"try again later",
	"please try again",
	"service unavailable",
	"server error",
	"bad gateway",
	"http 429",
	"http 502",
	"http 503",
	"http 529",
	"status 429",
	"status 502",
	"status 503",
	"status 529",
	"error 429",
	"error 502",
	"error 503",
	"error 529",
	"code 429",
	"code 502",
	"code 503",
	"code 529",
	"429 too many",
	"502 bad gateway",
	"503 service",
	"529 ",
	"capacity limit",
}

// Patterns indicating non-recoverable quota errors (skip retries).
var nonRecoverableQuotaPatterns = []string{
	"quota reset",
	"daily quota",
	"monthly quota",
	"billing",
	"exhausted your capacity",
	"exceeded your current quota",
	"subscription limit",
}

// Patterns indicating non-recoverable authentication/authorization errors.
var authFailurePatterns = []string{
	"authentication failed",
	"unauthorized",
	"forbidden",
	"invalid api key",
	"api key invalid",
}

// Patterns indicating validation infrastructure/runtime failures (not code/test failures).
var validationInfraErrorPatterns = []string{
	"cannot connect to the docker daemon",
	"error response from daemon",
	"dial tcp",
	"connection refused",
	"i/o timeout",
	"tls handshake timeout",
	"temporary failure in name resolution",
	"no such host",
	"net/http: request canceled",
	"context deadline exceeded while awaiting headers",
	"broken pipe",
	"ensuring image",
	"creating container",
	"starting container",
}

var selfTestCommandPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bgo test\b`),
	regexp.MustCompile(`(?i)\bcargo test\b`),
	regexp.MustCompile(`(?i)\bgradle test\b`),
	regexp.MustCompile(`(?i)\./gradlew test\b`),
	regexp.MustCompile(`(?i)\bzig build test\b`),
	regexp.MustCompile(`(?i)\bdart test\b`),
	regexp.MustCompile(`(?i)\bnpx tsx --test\b`),
	regexp.MustCompile(`(?i)\bnpm test\b`),
	regexp.MustCompile(`(?i)\bpnpm test\b`),
	regexp.MustCompile(`(?i)\byarn test\b`),
	regexp.MustCompile(`(?i)\bbun test\b`),
}

var toolchainInstallPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bapt(?:-get)?\s+install\b`),
	regexp.MustCompile(`(?i)\byum\s+install\b`),
	regexp.MustCompile(`(?i)\bapk\s+add\b`),
	regexp.MustCompile(`(?i)\bbrew\s+install\b`),
	regexp.MustCompile(`(?i)\bpip3?\s+install\b`),
	regexp.MustCompile(`(?i)\bnpm\s+install\s+-g\b`),
	regexp.MustCompile(`(?i)\bcargo\s+install\b`),
	regexp.MustCompile(`(?i)\bgo\s+install\b`),
	regexp.MustCompile(`(?i)\bcurl\b.*ziglang\.org/download`),
	regexp.MustCompile(`(?i)\bwget\b.*ziglang\.org/download`),
}

var outOfWorkspaceReadPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bfind\s+/`),
	regexp.MustCompile(`(?i)\bls\s+-la\s+/`),
	regexp.MustCompile(`(?i)/tasks/`),
	regexp.MustCompile(`(?i)/eval-results/`),
	regexp.MustCompile(`(?i)/sessions/`),
}

var (
	ansiEscapePattern       = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	bashLCPattern           = regexp.MustCompile(`(?i)/usr/bin/bash -lc ['"](.+?)['"]`)
	absolutePathPattern     = regexp.MustCompile(`(^|[\s"'` + "`" + `])(/[^"'` + "`" + `\s;&|]*)`)
	skillActivationPattern  = regexp.MustCompile(`(?i)\bskill\s+"([^"]+)"`)
	firecrawlCommandPattern = regexp.MustCompile(`(?i)(?:^|\s)firecrawl\s+(search|scrape|crawl|map|agent|browser|download)\b`)
	skillArtifactPattern    = regexp.MustCompile(
		`(?i)[^"'` + "`" + `\s]*\.agents/skills/[^"'` + "`" + `\s]*|` +
			`[^"'` + "`" + `\s]*\.codex/skills/[^"'` + "`" + `\s]*|` +
			`[^"'` + "`" + `\s]*skill\.md`,
	)
)

type agentBehaviorMetrics struct {
	SelfTestCommands             int
	SelfTestCommandsConfident    bool
	ToolchainInstallAttempts     int
	OutOfWorkspaceReads          int
	OutOfWorkspaceReadsConfident bool
	SkillsUsed                   bool
	SkillsUsageSignals           int
}

// FailureClass categorizes the root cause of non-successful or degraded runs.
type FailureClass string

const (
	FailureClassNone              FailureClass = "none"
	FailureClassQuotaRecoverable  FailureClass = "quota_recoverable"
	FailureClassQuotaExhausted    FailureClass = "quota_exhausted"
	FailureClassAuth              FailureClass = "auth"
	FailureClassInfra             FailureClass = "infra"
	FailureClassIntegrity         FailureClass = "integrity"
	FailureClassValidationError   FailureClass = "validation_error"
	FailureClassValidationTimeout FailureClass = "validation_timeout"
)

// EvalResult holds the result of evaluating a single task.
type EvalResult struct {
	Task                         string            `json:"task"`
	Language                     string            `json:"language"`
	Tier                         string            `json:"tier,omitempty"`
	Difficulty                   string            `json:"difficulty,omitempty"`
	Passed                       bool              `json:"passed"`
	AgentTimedOut                bool              `json:"agent_timed_out"`
	Status                       task.ResultStatus `json:"status"`
	Attempts                     int               `json:"attempts"`
	Duration                     float64           `json:"duration_seconds"`
	AgentTime                    float64           `json:"agent_duration_seconds,omitempty"`
	ValidateTime                 float64           `json:"validation_duration_seconds,omitempty"`
	PromptChars                  int               `json:"prompt_chars,omitempty"`
	Error                        string            `json:"error,omitempty"`
	FailureClass                 FailureClass      `json:"failure_class"`
	Weight                       float64           `json:"weight,omitempty"`
	WeightedScore                float64           `json:"weighted_score,omitempty"`
	QuotaRetries                 int               `json:"quota_retries"`
	InfraRetries                 int               `json:"infra_retries"`
	QuotaExhausted               bool              `json:"quota_exhausted"`
	InfraFailure                 bool              `json:"infra_failure"`
	SelfTestCommands             int               `json:"self_test_commands"`
	SelfTestCommandsConfident    bool              `json:"self_test_commands_confident"`
	ToolchainInstallAttempts     int               `json:"toolchain_install_attempts"`
	OutOfWorkspaceReadAttempts   int               `json:"out_of_workspace_read_attempts"`
	OutOfWorkspaceReadsConfident bool              `json:"out_of_workspace_read_attempts_confident"`
	SkillsUsed                   bool              `json:"skills_used"`
	SkillsUsageSignals           int               `json:"skills_usage_signals"`
	WorkspaceDir                 string            `json:"-"` // Not serialized, used for cleanup
}

// EvalAggregate summarizes results for a group (language, tier, difficulty).
type EvalAggregate struct {
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Total        int     `json:"total"`
	PassRate     float64 `json:"pass_rate"`
	Duration     float64 `json:"duration_seconds"`
	AgentTime    float64 `json:"agent_duration_seconds"`
	ValidateTime float64 `json:"validation_duration_seconds"`
}

// EvalSummary holds the overall evaluation summary.
type EvalSummary struct {
	Agent                           string                   `json:"agent"`
	Model                           string                   `json:"model,omitempty"`
	Reasoning                       string                   `json:"reasoning,omitempty"`
	Timestamp                       string                   `json:"timestamp"`
	Tier                            string                   `json:"tier,omitempty"`
	Difficulty                      string                   `json:"difficulty,omitempty"`
	Timeout                         int                      `json:"timeout"`
	Parallel                        int                      `json:"parallel"`
	Results                         []EvalResult             `json:"results"`
	Passed                          int                      `json:"passed"`
	Failed                          int                      `json:"failed"`
	Total                           int                      `json:"total"`
	PassRate                        float64                  `json:"pass_rate"`
	WeightedScore                   float64                  `json:"weighted_score,omitempty"`
	MaxPossibleScore                float64                  `json:"max_possible_score,omitempty"`
	WeightedPassRate                float64                  `json:"weighted_pass_rate,omitempty"`
	IntegrityViolations             int                      `json:"integrity_violations,omitempty"`
	Duration                        float64                  `json:"duration_seconds,omitempty"`
	AgentTime                       float64                  `json:"agent_duration_seconds,omitempty"`
	ValidateTime                    float64                  `json:"validation_duration_seconds,omitempty"`
	PromptChars                     int                      `json:"prompt_chars,omitempty"`
	ByLanguage                      map[string]EvalAggregate `json:"by_language,omitempty"`
	ByTier                          map[string]EvalAggregate `json:"by_tier,omitempty"`
	ByDifficulty                    map[string]EvalAggregate `json:"by_difficulty,omitempty"`
	UseMCPTools                     bool                     `json:"use_mcp_tools"`
	UseSkills                       bool                     `json:"use_skills"`
	DisableMCP                      bool                     `json:"disable_mcp"`
	Sandbox                         bool                     `json:"sandbox"`
	Legacy                          bool                     `json:"legacy"`
	QuotaAffectedTasks              int                      `json:"quota_affected_tasks"`
	AuthAffectedTasks               int                      `json:"auth_affected_tasks"`
	InfraAffectedTasks              int                      `json:"infra_affected_tasks"`
	TotalQuotaRetries               int                      `json:"total_quota_retries"`
	TotalInfraRetries               int                      `json:"total_infra_retries"`
	TotalSelfTestCommands           int                      `json:"total_self_test_commands"`
	TotalToolchainInstallAttempts   int                      `json:"total_toolchain_install_attempts"`
	TotalOutOfWorkspaceReadAttempts int                      `json:"total_out_of_workspace_read_attempts"`
	SkillsUsageRate                 float64                  `json:"skills_usage_rate"`
	TotalSkillsUsageSignals         int                      `json:"total_skills_usage_signals"`
	TasksWithSelfTesting            int                      `json:"tasks_with_self_testing"`
	TasksWithToolchainInstall       int                      `json:"tasks_with_toolchain_install"`
	TasksWithOutOfWorkspaceReads    int                      `json:"tasks_with_out_of_workspace_reads"`
	TasksWithSkillsUsage            int                      `json:"tasks_with_skills_usage"`
}

// RunSpec defines a single eval run's configuration.
type RunSpec struct {
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

// SharedConfig holds settings common to all runs.
type SharedConfig struct {
	Tier           string
	Difficulty     string
	Lang           string
	Tasks          string
	Timeout        int
	Parallel       int
	KeepWorkspaces bool
	UseMCPTools    bool
	UseSkills      bool
	DisableMCP     bool
	NoSandbox      bool
	Legacy         bool
	DryRun         bool
}

// RunConfig stores the original eval configuration for resume capability.
type RunConfig struct {
	Agent          string   `json:"agent"`
	Model          string   `json:"model,omitempty"`
	Reasoning      string   `json:"reasoning,omitempty"`
	Tier           string   `json:"tier,omitempty"`
	Difficulty     string   `json:"difficulty,omitempty"`
	Lang           string   `json:"lang,omitempty"`
	Tasks          string   `json:"tasks,omitempty"`
	Timeout        int      `json:"timeout"`
	Parallel       int      `json:"parallel"`
	UseMCPTools    bool     `json:"use_mcp_tools"`
	UseSkills      bool     `json:"use_skills"`
	DisableMCP     bool     `json:"disable_mcp"`
	NoSandbox      bool     `json:"no_sandbox"`
	Legacy         bool     `json:"legacy"`
	KeepWorkspaces bool     `json:"keep_workspaces"`
	TaskList       []string `json:"task_list"`
	CreatedAt      string   `json:"created_at"`
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate an agent against all tasks",
	Long: `Runs all (or selected) tasks against a coding agent and reports results.

Built-in agents:
  gemini    - Google Gemini CLI
  kilocode  - Kilo Code CLI
  opencode  - OpenCode CLI
  claude    - Anthropic Claude Code
  codex     - OpenAI Codex CLI
  kimi      - Moonshot Kimi CLI
  crush     - Crush CLI
  copilot   - GitHub Copilot CLI
  droid     - Factory Droid CLI
  iflow     - iFlow CLI
  qwen      - Qwen Code CLI
  amp       - Sourcegraph Amp CLI
  codebuff  - Codebuff CLI
  vibe      - Mistral Vibe CLI
  goose     - Block Goose CLI
  junie     - JetBrains Junie CLI
  ccs       - Claude Code Switch
  cline     - Cline CLI
  pi        - Pi CLI

Custom agents can be configured in sanity.toml under [agents.<name>].

Examples:
  sanity eval --agent gemini
  sanity eval --agent kilocode
  sanity eval --agent gemini --model gemini-2.5-pro
  sanity eval --agent opencode --model google/gemini-2.5-flash
  sanity eval --agent claude --lang go
  sanity eval --agent my-custom-agent --tasks bank-account,react
  sanity eval --agent gemini --dry-run
  sanity eval --resume ./eval-results/2026-01-19T192910-gemini`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Apply config defaults for flags not explicitly set.
		if !cmd.Flags().Changed("timeout") && evalTimeout == 0 {
			if cfg != nil && cfg.Harness.DefaultTimeout > 0 {
				evalTimeout = cfg.Harness.DefaultTimeout
			} else {
				evalTimeout = 600
			}
		}

		if evalRepeat < 1 {
			evalRepeat = 1
		}

		shared := SharedConfig{
			Tier: evalTier, Difficulty: evalDifficulty, Lang: evalLang,
			Tasks: evalTasks, Timeout: evalTimeout, Parallel: evalParallel,
			KeepWorkspaces: evalKeepWorkspaces, UseMCPTools: evalUseMCPTools,
			UseSkills: evalUseSkills, DisableMCP: evalDisableMCP, NoSandbox: evalNoSandbox,
			Legacy: evalLegacy, DryRun: evalDryRun,
		}

		// Track if we're resuming a previous run.
		var isResuming bool
		var previousResults []EvalResult
		var completedTasks map[string]bool
		var runCfg *RunConfig
		var timestamp string

		// Handle resume mode: load config and apply settings.
		var prevAttestation *EvalAttestation
		if evalResume != "" {
			// Check if this is a multi-run directory.
			if isMultiRunDir(evalResume) {
				return resumeMultiRun(evalResume)
			}

			var err error
			runCfg, err = loadRunConfig(evalResume)
			if err != nil {
				return fmt.Errorf("loading resume config: %w", err)
			}
			applyRunConfig(runCfg)
			evalOutputDir = evalResume
			isResuming = true

			// Re-build shared from restored globals.
			shared = SharedConfig{
				Tier: evalTier, Difficulty: evalDifficulty, Lang: evalLang,
				Tasks: evalTasks, Timeout: evalTimeout, Parallel: evalParallel,
				KeepWorkspaces: evalKeepWorkspaces, UseMCPTools: evalUseMCPTools,
				UseSkills: evalUseSkills, DisableMCP: evalDisableMCP, NoSandbox: evalNoSandbox,
				Legacy: evalLegacy, DryRun: evalDryRun,
			}

			completedTasks, err = findCompletedTasks(evalOutputDir)
			if err != nil {
				return fmt.Errorf("finding completed tasks: %w", err)
			}

			prevSummary, err := loadPreviousSummary(evalOutputDir)
			if err != nil {
				return fmt.Errorf("loading previous results: %w", err)
			}
			if prevSummary != nil {
				previousResults = prevSummary.Results
				timestamp = prevSummary.Timestamp
			}

			// Load previous attestation to preserve hashes of tasks whose workspaces are gone.
			prevAttestation, err = loadPreviousAttestation(evalOutputDir)
			if err != nil {
				logger.Warn("failed to load previous attestation", "error", err)
			}
		}

		if timestamp == "" {
			timestamp = time.Now().Format("2006-01-02T150405")
		}

		// Parse comma-separated agent/model/reasoning for multi-agent support.
		agents := strings.Split(evalAgent, ",")
		for i := range agents {
			agents[i] = strings.TrimSpace(agents[i])
		}
		models, err := broadcastOrSplit(evalModel, len(agents), "model")
		if err != nil {
			return err
		}
		reasonings, err := broadcastOrSplit(evalReasoning, len(agents), "reasoning")
		if err != nil {
			return err
		}

		var specs []RunSpec
		for i := range agents {
			specs = append(specs, RunSpec{
				Agent: agents[i], Model: models[i], Reasoning: reasonings[i],
			})
		}
		isMultiRun := len(specs) > 1 || evalRepeat > 1

		// Dry-run mode doesn't require agent to be installed.
		if !evalDryRun {
			for _, spec := range specs {
				if spec.Agent == "" {
					return fmt.Errorf("--agent is required (use --help to see available agents)")
				}
				agentCfg := cfg.GetAgent(spec.Agent)
				if agentCfg == nil {
					available := strings.Join(cfg.ListAgents(), ", ")
					return fmt.Errorf("unknown agent: %s (available: %s)", spec.Agent, available)
				}
				if _, err := exec.LookPath(agentCfg.Command); err != nil {
					return fmt.Errorf("agent %q binary %q not found in PATH", spec.Agent, agentCfg.Command)
				}
			}
		}

		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		if shared.Legacy {
			r.LegacyHiddenTests = true
			logger.Info("legacy mode enabled: hidden tests exposed to agent (pre-v1.6.0 behavior)")
		}

		// If the user specified another selector, default tier should not hide tasks.
		tierChanged := cmd.Flags().Changed("tier")
		if !tierChanged && (shared.Lang != "" || shared.Tasks != "" || shared.Difficulty != "") {
			shared.Tier = "all"
			evalTier = "all"
		}

		switch shared.Tier {
		case "", "core", "extended", "all":
			// OK
		default:
			return fmt.Errorf("invalid --tier %q (valid: core, extended, all)", shared.Tier)
		}

		// Get tasks to run
		allTasks, err := r.ListTasks()
		if err != nil {
			return fmt.Errorf("listing tasks: %w", err)
		}

		// Filter by specific tasks if specified
		if evalTasks != "" {
			tokens := strings.Split(evalTasks, ",")
			var selected []*task.Task
			seen := make(map[string]bool)
			for _, tok := range tokens {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				t, err := task.ResolveRef(allTasks, tok)
				if err != nil {
					return fmt.Errorf("resolving task %q: %w", tok, err)
				}
				if !seen[t.ID()] {
					seen[t.ID()] = true
					selected = append(selected, t)
				}
			}
			allTasks = selected
		}

		// Filter by language if specified
		if evalLang != "" {
			lang, err := task.ParseLanguage(evalLang)
			if err != nil {
				return err
			}
			var filtered []*task.Task
			for _, t := range allTasks {
				if t.Language == lang {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		// Filter by difficulty if specified
		if evalDifficulty != "" {
			want := make(map[string]bool)
			for _, tok := range strings.Split(evalDifficulty, ",") {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				want[tok] = true
			}
			var filtered []*task.Task
			for _, t := range allTasks {
				if want[t.Difficulty] {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		// Filter by tier if specified
		if evalTier != "" && evalTier != "all" {
			var filtered []*task.Task
			for _, t := range allTasks {
				if t.Tier == evalTier {
					filtered = append(filtered, t)
				}
			}
			allTasks = filtered
		}

		if len(allTasks) == 0 {
			return fmt.Errorf("no tasks match the specified filters")
		}

		// Dry-run mode: print what would be executed and exit
		if shared.DryRun {
			fmt.Println()
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println(" SANITY HARNESS - Dry Run")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println()
			for _, spec := range specs {
				if spec.Agent != "" {
					fmt.Printf(" Agent:      %s\n", spec.Agent)
				}
				if spec.Model != "" {
					fmt.Printf(" Model:      %s\n", spec.Model)
				}
				if spec.Reasoning != "" {
					fmt.Printf(" Reasoning:  %s\n", spec.Reasoning)
				}
			}
			if shared.Tier != "" {
				fmt.Printf(" Tier:       %s\n", shared.Tier)
			}
			if shared.Difficulty != "" {
				fmt.Printf(" Difficulty: %s\n", shared.Difficulty)
			}
			if evalRepeat > 1 {
				fmt.Printf(" Repeat:     %d\n", evalRepeat)
			}
			fmt.Printf(" Tasks:      %d\n", len(allTasks))
			fmt.Println()
			fmt.Println(" Tasks that would be executed:")
			fmt.Println("─────────────────────────────────────────────────────────────")
			for i, t := range allTasks {
				timeout := shared.Timeout
				if t.AgentTimeout > 0 {
					timeout = t.AgentTimeout
				}
				fmt.Printf(" %3d. %-35s [%s, %s, %ds]\n",
					i+1, t.ID(), t.Tier, t.Difficulty, timeout)
			}
			fmt.Println("─────────────────────────────────────────────────────────────")
			fmt.Println()
			return nil
		}

		// Detect sandbox availability.
		evalSandboxActive = initSandbox()
		evalSandboxDenylist = resolveSandboxDenylistPaths(cfg.Sandbox.ReadableDenylist, evalOutputDir)
		evalSandboxSharedRW = append([]string(nil), cfg.Sandbox.SharedReadWriteDirs...)
		evalSandboxSharedRO = append([]string(nil), cfg.Sandbox.SharedReadOnlyDirs...)

		// Protect the tasks/ directory from agent modification during eval.
		if restoreFn, err := protectTasksDir(); err != nil {
			logger.Warn("failed to protect tasks directory", "error", err)
		} else if restoreFn != nil {
			defer restoreFn()
		}

		// Set up interrupt handler for graceful shutdown.
		interruptCtx, interruptCancel := setupInterruptHandler()
		defer interruptCancel()

		if isMultiRun {
			// Multi-run mode: create umbrella directory and orchestrate runs.
			var umbrellaDir string
			if evalOutputDir != "" {
				umbrellaDir = evalOutputDir
			} else if len(specs) == 1 {
				// Single-agent repeat: use normal naming.
				umbrellaDir = filepath.Join("eval-results", fmt.Sprintf("%s-%s", timestamp, specs[0].Agent))
			} else {
				umbrellaDir = filepath.Join("eval-results", fmt.Sprintf("multi-%s", timestamp))
			}
			if err := os.MkdirAll(umbrellaDir, 0o755); err != nil {
				return fmt.Errorf("creating umbrella directory: %w", err)
			}

			writeMultiRunConfig(umbrellaDir, specs, shared, evalRepeat)

			var allSummaries []runResult
			for specIdx, spec := range specs {
				for rep := 1; rep <= evalRepeat; rep++ {
					if checkInterrupted(interruptCtx) {
						updateMultiRunState(umbrellaDir, allSummaries, specs, evalRepeat, true)
						printMultiRunResumeCommand(umbrellaDir)
						return nil
					}

					runDir := multiRunSubdir(umbrellaDir, spec, specIdx, rep, evalRepeat)
					summary, _, err := evalRunSingle(
						interruptCtx, spec, shared, allTasks, allTasks,
						runDir, timestamp, r, false, nil, nil, nil, nil,
					)
					rr := runResult{spec: spec, repeat: rep, summary: summary}
					if err != nil {
						logger.Warn("run failed", "agent", spec.Agent, "repeat", rep, "error", err)
						rr.err = err
					}
					allSummaries = append(allSummaries, rr)
					updateMultiRunState(umbrellaDir, allSummaries, specs, evalRepeat, false)
				}
			}

			// Generate comparison if multiple specs.
			if len(specs) > 1 {
				var summaries []EvalSummary
				for _, rr := range allSummaries {
					if rr.summary != nil {
						summaries = append(summaries, *rr.summary)
					}
				}
				if len(summaries) > 1 {
					comparison := generateComparison(summaries)
					writeComparisonJSON(umbrellaDir, comparison)
					writeComparisonMarkdown(umbrellaDir, comparison)
				}
			}

			// Generate repeat stats if repeating.
			if evalRepeat > 1 {
				writeRepeatStats(umbrellaDir, specs, allSummaries, evalRepeat)
			}

			fmt.Printf("\n Multi-run results saved to: %s\n\n", umbrellaDir)
			return nil
		}

		// Single run — unchanged behavior.
		spec := specs[0]

		// Create output directory.
		if evalOutputDir == "" {
			evalOutputDir = filepath.Join("eval-results", fmt.Sprintf("%s-%s", timestamp, spec.Agent))
		}

		_, _, err = evalRunSingle(
			interruptCtx, spec, shared, allTasks, allTasks,
			evalOutputDir, timestamp, r, isResuming,
			previousResults, completedTasks, prevAttestation, runCfg,
		)
		return err
	},
}

// evalRunSingle executes a single eval run for one agent/model/reasoning combination.
// It handles output directory creation, task execution, aggregation, and output file writing.
func evalRunSingle( //nolint:gocognit,gocyclo,maintidx
	interruptCtx context.Context,
	spec RunSpec,
	shared SharedConfig,
	allTasks []*task.Task,
	tasksToRun []*task.Task,
	outputDir string,
	timestamp string,
	r *runner.Runner,
	isResuming bool,
	previousResults []EvalResult,
	completedTasks map[string]bool,
	prevAttestation *EvalAttestation,
	runCfg *RunConfig,
) (*EvalSummary, *EvalAttestation, error) {
	// Set globals that sub-functions (runTaskWithAgent, runAgentAttempt, etc.) read.
	evalAgent = spec.Agent
	evalModel = spec.Model
	evalReasoning = spec.Reasoning
	evalUseMCPTools = shared.UseMCPTools
	evalUseSkills = shared.UseSkills
	evalDisableMCP = shared.DisableMCP
	evalLegacy = shared.Legacy
	evalKeepWorkspaces = shared.KeepWorkspaces

	// Create output directory.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("creating output directory: %w", err)
	}

	// For resume mode: filter out completed tasks and clean incomplete dirs.
	totalTaskCount := len(allTasks)
	if isResuming && runCfg != nil {
		var err error
		allTasks, tasksToRun, err = prepareResumedTasks(allTasks, runCfg, outputDir, completedTasks)
		if err != nil {
			return nil, nil, err
		}
		if len(tasksToRun) == 0 {
			fmt.Println("\n All tasks already completed. Nothing to resume.")
			return nil, nil, nil
		}
	} else {
		// Save run config for new runs (enables resume).
		if err := saveRunConfig(outputDir, allTasks); err != nil {
			return nil, nil, fmt.Errorf("saving run config: %w", err)
		}
	}

	var wasInterrupted bool

	// Print header
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if isResuming {
		fmt.Println(" SANITY HARNESS - Agent Evaluation (RESUMING)")
	} else {
		fmt.Println(" SANITY HARNESS - Agent Evaluation")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf(" Agent:   %s\n", spec.Agent)
	if spec.Model != "" {
		fmt.Printf(" Model:   %s\n", spec.Model)
	}
	if shared.Tier != "" {
		fmt.Printf(" Tier:    %s\n", shared.Tier)
	}
	if shared.Difficulty != "" {
		fmt.Printf(" Difficulty: %s\n", shared.Difficulty)
	}
	if shared.Parallel > 1 {
		fmt.Printf(" Parallel: %d\n", shared.Parallel)
	}
	if evalSandboxActive {
		fmt.Println(" Sandbox: enabled (bwrap)")
	}
	if isResuming {
		fmt.Printf(" Tasks:   %d remaining of %d total\n", len(tasksToRun), totalTaskCount)
	} else {
		fmt.Printf(" Tasks:   %d\n", len(tasksToRun))
	}
	fmt.Printf(" Output:  %s\n", outputDir)
	fmt.Println()

	// Run tasks
	results := make([]EvalResult, 0, len(tasksToRun))
	passed, failed := 0, 0
	var resumableFailedTasks []string // External failures excluded from results (resumable via --resume)

	parallel := shared.Parallel
	if parallel <= 0 {
		parallel = 1
	}

	if parallel == 1 { //nolint:nestif // Sequential execution loop with deeply interleaved interrupt/quota/progress handling.
		consecutiveQuotaExhausted := 0
		for i, t := range tasksToRun {
			// Check for interrupt before starting next task.
			if checkInterrupted(interruptCtx) {
				wasInterrupted = true
				fmt.Println("\n\033[33m⚠ Interrupt received. Saving partial results...\033[0m")
				break
			}

			fmt.Println("─────────────────────────────────────────────────────────────")
			fmt.Printf(" [%d/%d] %s\n", i+1, len(tasksToRun), t.ID())
			fmt.Println("─────────────────────────────────────────────────────────────")

			result := runTaskWithAgent(interruptCtx, r, t, spec.Agent, spec.Model, outputDir, shared.Timeout)

			// External failures are excluded from results so they can be resumed later.
			if isResumableExternalFailure(result) {
				fmt.Printf(" ⚠ %s — will be skipped (resumable)\n", externalFailureLabel(result.FailureClass))
				resumableFailedTasks = append(resumableFailedTasks, fmt.Sprintf("%s [%s]", t.ID(), result.FailureClass))
				removeTaskArtifactsForResume(outputDir, result)
				if result.FailureClass == FailureClassQuotaExhausted {
					consecutiveQuotaExhausted++
					if consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
						wasInterrupted = true
						fmt.Printf("\n\033[33m⚠ Quota exhausted for %d consecutive tasks. Stopping early to allow resume.\033[0m\n", consecutiveQuotaExhausted)
						break
					}
				} else {
					consecutiveQuotaExhausted = 0
				}
				fmt.Println()
				continue
			}

			results = append(results, result)

			if result.Passed {
				fmt.Printf(" ✓ PASSED (%.2fs)\n", result.Duration)
				passed++
				consecutiveQuotaExhausted = 0 // Reset counter on success
			} else {
				fmt.Printf(" ✗ FAILED (%.2fs)\n", result.Duration)
				if result.Error != "" {
					fmt.Printf("   Error: %s\n", result.Error)
				}
				failed++

				// Track consecutive quota exhaustion
				if result.QuotaExhausted {
					consecutiveQuotaExhausted++
					if consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
						wasInterrupted = true
						fmt.Printf("\n\033[33m⚠ Quota exhausted for %d consecutive tasks. Stopping early to allow resume.\033[0m\n", consecutiveQuotaExhausted)
						break
					}
				} else {
					consecutiveQuotaExhausted = 0 // Reset on non-quota failure
				}
			}

			// Clean up workspace source files unless --keep-workspaces is set.
			// The workspace dir is also the task output dir containing agent.log,
			// validation.log, and integrity artifacts — those must be preserved.
			if !shared.KeepWorkspaces && result.WorkspaceDir != "" {
				cleanupWorkspaceFiles(result.WorkspaceDir)
			}

			fmt.Println()
		}
	} else {
		type job struct {
			idx int
			t   *task.Task
		}
		type jobResult struct {
			idx int
			r   EvalResult
		}

		jobs := make(chan job)
		jobResults := make(chan jobResult)
		stopSending := make(chan struct{})

		var wg sync.WaitGroup
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					res := runTaskWithAgent(interruptCtx, r, j.t, spec.Agent, spec.Model, outputDir, shared.Timeout)
					jobResults <- jobResult{idx: j.idx, r: res}
				}
			}()
		}

		// Producer goroutine: sends jobs, stops on interrupt.
		go func() {
			for i, t := range tasksToRun {
				select {
				case <-stopSending:
					// Interrupt received, stop sending new jobs.
					close(jobs)
					wg.Wait()
					close(jobResults)
					return
				case jobs <- job{idx: i, t: t}:
				}
			}
			close(jobs)
			wg.Wait()
			close(jobResults)
		}()

		collected := make([]EvalResult, len(tasksToRun))
		seen := 0
		consecutiveQuotaExhausted := 0
	collectLoop:
		for jr := range jobResults {
			seen++

			// External failures are excluded from results so they can be resumed later.
			if isResumableExternalFailure(jr.r) {
				fmt.Printf(" [%d/%d] %s ⚠ %s — will be skipped (resumable)\n", seen, len(tasksToRun), jr.r.Task, externalFailureLabel(jr.r.FailureClass))
				resumableFailedTasks = append(resumableFailedTasks, fmt.Sprintf("%s [%s]", jr.r.Task, jr.r.FailureClass))
				removeTaskArtifactsForResume(outputDir, jr.r)
				if jr.r.FailureClass == FailureClassQuotaExhausted {
					consecutiveQuotaExhausted++
				} else {
					consecutiveQuotaExhausted = 0
				}
			} else {
				collected[jr.idx] = jr.r

				status := "FAILED"
				if jr.r.Passed {
					status = "PASSED"
				}
				fmt.Printf(" [%d/%d] %s %s (%.2fs)\n", seen, len(tasksToRun), jr.r.Task, status, jr.r.Duration)
				if !jr.r.Passed && jr.r.Error != "" {
					fmt.Printf("   Error: %s\n", jr.r.Error)
				}

				if jr.r.Passed {
					passed++
					consecutiveQuotaExhausted = 0 // Reset counter on success
				} else {
					failed++
					// Track consecutive quota exhaustion
					if jr.r.QuotaExhausted {
						consecutiveQuotaExhausted++
					} else {
						consecutiveQuotaExhausted = 0 // Reset on non-quota failure
					}
				}

				if !shared.KeepWorkspaces && jr.r.WorkspaceDir != "" {
					cleanupWorkspaceFiles(jr.r.WorkspaceDir)
				}
			}

			// Check for interrupt after each result.
			shouldStop := checkInterrupted(interruptCtx)
			stopReason := "Interrupt received"

			// Also stop if we hit consecutive quota exhaustion threshold.
			if !shouldStop && consecutiveQuotaExhausted >= quotaExhaustedStopThreshold {
				shouldStop = true
				stopReason = fmt.Sprintf("Quota exhaustion for %d consecutive tasks", consecutiveQuotaExhausted)
			}

			if shouldStop {
				wasInterrupted = true
				fmt.Printf("\n\033[33m⚠ %s. Waiting for in-flight tasks...\033[0m\n", stopReason)
				close(stopSending)
				// Drain remaining results from in-flight tasks.
				for jr := range jobResults {
					if isResumableExternalFailure(jr.r) {
						resumableFailedTasks = append(resumableFailedTasks, fmt.Sprintf("%s [%s]", jr.r.Task, jr.r.FailureClass))
						removeTaskArtifactsForResume(outputDir, jr.r)
					} else {
						collected[jr.idx] = jr.r
						if jr.r.Passed {
							passed++
						} else {
							failed++
						}
						if !shared.KeepWorkspaces && jr.r.WorkspaceDir != "" {
							_ = os.RemoveAll(jr.r.WorkspaceDir)
						}
					}
				}
				break collectLoop
			}
		}
		// Only include results that were actually run (excluding resumable external failures).
		for _, r := range collected {
			if r.Task != "" {
				results = append(results, r)
			}
		}
	}

	// If resuming, merge with previous results.
	if isResuming && len(previousResults) > 0 {
		// Build a set of task IDs from new results.
		newResultTasks := make(map[string]bool)
		for _, r := range results {
			newResultTasks[r.Task] = true
		}
		// Prepend previous results that aren't in the new results.
		var merged []EvalResult
		for _, r := range previousResults {
			if !newResultTasks[r.Task] {
				merged = append(merged, r)
				if r.Passed {
					passed++
				} else {
					failed++
				}
			}
		}
		results = append(merged, results...)
	}

	// Sort results to match allTasks order
	taskOrder := make(map[string]int)
	for i, t := range allTasks {
		taskOrder[t.ID()] = i
	}
	sort.Slice(results, func(i, j int) bool {
		return taskOrder[results[i].Task] < taskOrder[results[j].Task]
	})

	// Recompute status and weighted score for all results.
	// Previous results loaded from summary.json during resume may lack
	// these fields (they were never set due to a defer/named-return bug).
	taskWeights := make(map[string]task.Weight)
	for _, t := range allTasks {
		taskWeights[t.ID()] = task.ComputeWeight(t)
	}
	for i := range results {
		r := &results[i]
		w, ok := taskWeights[r.Task]
		if ok {
			r.Weight = w.Base
		}
		if r.FailureClass == "" {
			r.FailureClass = FailureClassNone
			switch {
			case strings.Contains(r.Error, "modified task files"):
				r.FailureClass = FailureClassIntegrity
			case strings.Contains(r.Error, "infra failure"):
				r.FailureClass = FailureClassInfra
			case strings.Contains(strings.ToLower(r.Error), "timed out"):
				r.FailureClass = FailureClassValidationTimeout
			case r.Error != "":
				r.FailureClass = FailureClassValidationError
			case r.QuotaExhausted:
				r.FailureClass = FailureClassQuotaExhausted
			}
		}
		if !r.SelfTestCommandsConfident && r.SelfTestCommands == 0 {
			r.SelfTestCommandsConfident = true
		}
		if !r.OutOfWorkspaceReadsConfident && r.OutOfWorkspaceReadAttempts == 0 {
			r.OutOfWorkspaceReadsConfident = true
		}
		r.Status = task.DetermineStatus(r.Passed, r.AgentTimedOut, r.Error)
		r.WeightedScore = task.ScoreResult(r.Passed, r.AgentTimedOut, r.Error, w)
	}

	// Calculate pass rate
	total := passed + failed
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}

	// Print summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" EVALUATION SUMMARY")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf(" Agent:     %s\n", spec.Agent)
	if spec.Model != "" {
		fmt.Printf(" Model:     %s\n", spec.Model)
	}
	fmt.Printf(" Passed:    %d\n", passed)
	fmt.Printf(" Failed:    %d\n", failed)
	fmt.Printf(" Total:     %d\n", total)
	fmt.Printf(" Pass Rate: %.1f%%\n", passRate)
	fmt.Println()

	// Save summary
	// Aggregate stats
	byLanguage := make(map[string]EvalAggregate)
	byTier := make(map[string]EvalAggregate)
	byDifficulty := make(map[string]EvalAggregate)

	var totalDuration float64
	var totalAgentTime float64
	var totalValidateTime float64
	var totalPromptChars int
	var totalWeightedScore float64
	var maxPossibleScore float64
	var integrityViolations int
	var authAffectedTasks int
	var infraAffectedTasks int
	var totalSelfTestCommands int
	var totalInfraRetries int
	var totalToolchainInstallAttempts int
	var totalOutOfWorkspaceReadAttempts int
	var totalSkillsUsageSignals int
	var tasksWithSelfTesting int
	var tasksWithToolchainInstall int
	var tasksWithOutOfWorkspaceReads int
	var tasksWithSkillsUsage int

	addAgg := func(m map[string]EvalAggregate, key string, r EvalResult) {
		agg := m[key]
		if r.Passed {
			agg.Passed++
		} else {
			agg.Failed++
		}
		agg.Total++
		agg.Duration += r.Duration
		agg.AgentTime += r.AgentTime
		agg.ValidateTime += r.ValidateTime
		m[key] = agg
	}

	for _, r := range results {
		totalDuration += r.Duration
		totalAgentTime += r.AgentTime
		totalValidateTime += r.ValidateTime
		totalPromptChars += r.PromptChars
		totalWeightedScore += r.WeightedScore
		maxPossibleScore += r.Weight
		totalInfraRetries += r.InfraRetries
		totalSelfTestCommands += r.SelfTestCommands
		totalToolchainInstallAttempts += r.ToolchainInstallAttempts
		totalOutOfWorkspaceReadAttempts += r.OutOfWorkspaceReadAttempts
		totalSkillsUsageSignals += r.SkillsUsageSignals
		if r.SelfTestCommands > 0 {
			tasksWithSelfTesting++
		}
		if r.ToolchainInstallAttempts > 0 {
			tasksWithToolchainInstall++
		}
		if r.OutOfWorkspaceReadAttempts > 0 {
			tasksWithOutOfWorkspaceReads++
		}
		if r.SkillsUsed {
			tasksWithSkillsUsage++
		}

		// Count by status
		if r.Status == task.StatusIntegrityViolation {
			integrityViolations++
		}
		if r.FailureClass == FailureClassAuth {
			authAffectedTasks++
		}
		if r.FailureClass == FailureClassInfra {
			infraAffectedTasks++
		}

		addAgg(byLanguage, r.Language, r)
		if r.Tier != "" {
			addAgg(byTier, r.Tier, r)
		}
		if r.Difficulty != "" {
			addAgg(byDifficulty, r.Difficulty, r)
		}
	}

	// Calculate weighted pass rate
	weightedPassRate := 0.0
	if maxPossibleScore > 0 {
		weightedPassRate = totalWeightedScore / maxPossibleScore * 100
	}
	skillsUsageRate := 0.0
	if total > 0 {
		skillsUsageRate = float64(tasksWithSkillsUsage) / float64(total) * 100
	}

	finalize := func(m map[string]EvalAggregate) map[string]EvalAggregate {
		for k, v := range m {
			if v.Total > 0 {
				v.PassRate = float64(v.Passed) / float64(v.Total) * 100
			}
			m[k] = v
		}
		return m
	}

	// Aggregate quota statistics
	var quotaAffectedTasks int
	var totalQuotaRetries int
	for _, r := range results {
		if r.FailureClass == FailureClassQuotaRecoverable || r.FailureClass == FailureClassQuotaExhausted {
			quotaAffectedTasks++
		}
		totalQuotaRetries += r.QuotaRetries
	}

	// Default model to "unknown" if not specified
	model := spec.Model
	if model == "" {
		model = "unknown"
	}

	summary := EvalSummary{
		Agent:                           spec.Agent,
		Model:                           model,
		Reasoning:                       spec.Reasoning,
		Timestamp:                       timestamp,
		Tier:                            shared.Tier,
		Difficulty:                      shared.Difficulty,
		Timeout:                         shared.Timeout,
		Parallel:                        parallel,
		Results:                         results,
		Passed:                          passed,
		Failed:                          failed,
		Total:                           total,
		PassRate:                        passRate,
		WeightedScore:                   totalWeightedScore,
		MaxPossibleScore:                maxPossibleScore,
		WeightedPassRate:                weightedPassRate,
		IntegrityViolations:             integrityViolations,
		Duration:                        totalDuration,
		AgentTime:                       totalAgentTime,
		ValidateTime:                    totalValidateTime,
		PromptChars:                     totalPromptChars,
		ByLanguage:                      finalize(byLanguage),
		ByTier:                          finalize(byTier),
		ByDifficulty:                    finalize(byDifficulty),
		UseMCPTools:                     shared.UseMCPTools,
		UseSkills:                       shared.UseSkills,
		DisableMCP:                      shared.DisableMCP,
		Sandbox:                         evalSandboxActive,
		Legacy:                          shared.Legacy,
		QuotaAffectedTasks:              quotaAffectedTasks,
		AuthAffectedTasks:               authAffectedTasks,
		InfraAffectedTasks:              infraAffectedTasks,
		TotalQuotaRetries:               totalQuotaRetries,
		TotalInfraRetries:               totalInfraRetries,
		TotalSelfTestCommands:           totalSelfTestCommands,
		TotalToolchainInstallAttempts:   totalToolchainInstallAttempts,
		TotalOutOfWorkspaceReadAttempts: totalOutOfWorkspaceReadAttempts,
		SkillsUsageRate:                 skillsUsageRate,
		TotalSkillsUsageSignals:         totalSkillsUsageSignals,
		TasksWithSelfTesting:            tasksWithSelfTesting,
		TasksWithToolchainInstall:       tasksWithToolchainInstall,
		TasksWithOutOfWorkspaceReads:    tasksWithOutOfWorkspaceReads,
		TasksWithSkillsUsage:            tasksWithSkillsUsage,
	}

	summaryPath := filepath.Join(outputDir, "summary.json")
	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, summaryData, 0644); err != nil {
		logger.Warn("failed to save summary", "error", err)
	} else {
		fmt.Printf(" Results saved to: %s\n", summaryPath)
	}

	// Generate attestation for verification
	loader := task.NewLoader(tasks.FS, tasksDir)
	var prevTasks map[string]AttestationTask
	if prevAttestation != nil {
		prevTasks = prevAttestation.Tasks
	}
	// Build set of tasks that were newly run in this session
	newlyRunTasks := make(map[string]bool)
	for _, t := range tasksToRun {
		newlyRunTasks[t.ID()] = true
	}
	attestation, err := generateAttestation(
		spec.Agent, spec.Model, timestamp, totalDuration,
		results, outputDir, loader, allTasks, newlyRunTasks, prevTasks,
	)
	if err != nil {
		logger.Warn("failed to generate attestation", "error", err)
	} else {
		attestationPath := filepath.Join(outputDir, "attestation.json")
		attestationData, _ := json.MarshalIndent(attestation, "", "  ")
		if err := os.WriteFile(attestationPath, attestationData, 0644); err != nil {
			logger.Warn("failed to save attestation", "error", err)
		} else {
			fmt.Printf(" Attestation saved to: %s\n", attestationPath)
		}
	}

	// Generate human-readable report.md
	reportMd := generateEvalReport(summary, attestation)
	reportPath := filepath.Join(outputDir, "report.md")
	if err := os.WriteFile(reportPath, []byte(reportMd), 0644); err != nil {
		logger.Warn("failed to save report", "error", err)
	} else {
		fmt.Printf(" Report saved to: %s\n", reportPath)
	}

	// Generate leaderboard submission file
	submission := generateLeaderboardSubmission(summary, attestation)
	submissionData, _ := json.MarshalIndent(submission, "", "  ")
	submissionPath := filepath.Join(outputDir, "submission.json")
	if err := os.WriteFile(submissionPath, submissionData, 0644); err != nil {
		logger.Warn("failed to save submission", "error", err)
	} else {
		fmt.Printf(" Submission saved to: %s\n", submissionPath)
	}

	fmt.Println()

	// Report resumable external failures and provide resume command.
	if len(resumableFailedTasks) > 0 {
		fmt.Println("\033[33m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
		fmt.Printf("\033[33m ⚠ %d task(s) skipped due to external failures (auth/quota/infra):\033[0m\n", len(resumableFailedTasks))
		for _, t := range resumableFailedTasks {
			fmt.Printf("   • %s\n", t)
		}
		fmt.Println()
		fmt.Println(" These tasks were not counted in the results above.")
		fmt.Println(" To retry them, run:")
		fmt.Printf("   ./sanity eval --resume %s\n", outputDir)
		fmt.Println("\033[33m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
		fmt.Println()
	}

	// If interrupted, print resume command.
	if wasInterrupted {
		printResumeCommand(outputDir)
	}

	return &summary, attestation, nil

}

func runTaskWithAgent(ctx context.Context, r *runner.Runner, t *task.Task, agent, model, outputDir string, timeout int) (result EvalResult) {
	start := time.Now()
	weight := task.ComputeWeight(t)
	result = newEvalResult(t, weight)
	defer finalizeEvalResult(&result, start, weight)

	loader := task.NewLoader(tasks.FS, tasksDir)
	workspaceName, workspaceDir := evalWorkspacePaths(outputDir, t)
	result.WorkspaceDir = workspaceDir

	// Create an isolated temp workspace for the agent so it cannot read
	// other eval results or sibling task directories. After the agent
	// finishes, files are copied back to the real workspace for validation.
	agentWorkDir, err := os.MkdirTemp("", fmt.Sprintf("sanity-eval-%s-%s-*", t.Language, t.Slug))
	if err != nil {
		result.Error = fmt.Sprintf("creating temp workspace: %v", err)
		return result
	}
	defer func() { _ = os.RemoveAll(agentWorkDir) }()

	if err := r.InitWorkspaceForTask(t, agentWorkDir); err != nil {
		result.Error = fmt.Sprintf("init failed: %v", err)
		return result
	}

	if evalUseSkills {
		if homeDir, err := os.UserHomeDir(); err == nil {
			agentSkillsSrc := filepath.Join(homeDir, ".agents", "skills")
			if _, err := os.Stat(agentSkillsSrc); err == nil {
				agentSkillsDest := filepath.Join(agentWorkDir, ".agents", "skills")
				_ = os.MkdirAll(agentSkillsDest, 0755)
				_ = copyDirContents(agentSkillsSrc, agentSkillsDest)
			}
		}
	}

	// Get agent configuration
	agentCfg := cfg.GetAgent(agent)
	if agentCfg == nil {
		result.Error = fmt.Sprintf("unknown agent: %s", agent)
		return result
	}

	// Build agent command
	prompt := buildAgentPrompt(t, evalUseMCPTools, evalUseSkills, agentCfg.MCPPrompt)
	result.PromptChars = utf8.RuneCountInString(prompt)
	agentTimeout := resolveAgentTimeout(timeout, agentCfg.DefaultTimeout, t.AgentTimeout)

	// Place agent.log in the task output directory (eval-results/<run>/<lang>-<slug>/).
	// This is outside the agent's temp workspace so the agent cannot read it.
	taskOutputDir, agentLogPath, validationLogPath, err := ensureEvalTaskOutputPaths(outputDir, workspaceName)
	if err != nil {
		result.Error = fmt.Sprintf("creating task output dir: %v", err)
		return result
	}

	// Execute agent in the isolated temp workspace
	workspaceReadyAt := time.Now()
	agentResult := executeAgentWithRetries(ctx, t, agentCfg, prompt, model, agentWorkDir, agentLogPath, agentTimeout, agent, workspaceReadyAt)
	applyAgentExecutionResult(&result, agentResult, agentLogPath, agentWorkDir)

	// If agent execution failed due auth/quota/infra, skip validation entirely.
	// The task will be excluded from results so it can be resumed later.
	if shouldSkipValidationForExternalFailure(&result) {
		return result
	}

	// Ensure the agent didn't modify task-owned files.
	integrityViolated, err := detectAndRecordIntegrityViolation(
		loader,
		t,
		taskOutputDir,
		agentWorkDir,
		validationLogPath,
		&result,
	)
	if err != nil {
		result.Error = fmt.Sprintf("integrity check failed: %v", err)
		return result
	}
	if integrityViolated {
		return result
	}

	// Copy agent's work from temp workspace to the real workspace for validation.
	if err := copyDirContents(agentWorkDir, workspaceDir); err != nil {
		result.Error = fmt.Sprintf("copying agent workspace: %v", err)
		return result
	}

	if err := writeHiddenTestsIfNeeded(loader, t, workspaceDir); err != nil {
		result.Error = fmt.Sprintf("writing hidden tests: %v", err)
		return result
	}

	validationCmd, effectiveValidationCmd := buildValidationCommands(t)
	validationTimeout := resolveValidationTimeout(timeout)
	session, validateDuration, err := runValidationSession(
		ctx,
		r,
		t,
		workspaceDir,
		validationTimeout,
		validationCmd,
	)
	result.ValidateTime = validateDuration
	if err != nil {
		handleValidationRunError(&result, session, err, validationLogPath, effectiveValidationCmd)
		return result
	}

	applyValidationSessionResult(&result, session)
	writeValidationSessionLog(validationLogPath, effectiveValidationCmd, session)
	return result
}

func newEvalResult(t *task.Task, weight task.Weight) EvalResult {
	return EvalResult{
		Task:       t.ID(),
		Language:   string(t.Language),
		Tier:       t.Tier,
		Difficulty: t.Difficulty,
		Weight:     weight.Base,
	}
}

func evalWorkspacePaths(outputDir string, t *task.Task) (workspaceName, workspaceDir string) {
	workspaceName = fmt.Sprintf("%s-%s", t.Language, t.Slug)
	return workspaceName, filepath.Join(outputDir, workspaceName)
}

func resolveAgentTimeout(timeoutSeconds, defaultSeconds, taskSeconds int) time.Duration {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	if defaultSeconds > 0 {
		defaultTimeout := time.Duration(defaultSeconds) * time.Second
		if timeout < defaultTimeout {
			timeout = defaultTimeout
		}
	}
	if taskSeconds > 0 {
		timeout = time.Duration(taskSeconds) * time.Second
	}
	return timeout
}

// evalOutputFiles lists files and directories produced by the harness in the
// task output directory. These must be preserved when cleaning up workspace
// source files after validation.
var evalOutputFiles = map[string]bool{
	"agent.log":       true,
	"validation.log":  true,
	"integrity.json":  true,
	"integrity-files": true,
	"integrity-diff":  true,
}

// cleanupWorkspaceFiles removes workspace source files from the task output
// directory while preserving eval artifacts (agent.log, validation.log,
// integrity files). The directory itself is kept.
func cleanupWorkspaceFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if evalOutputFiles[e.Name()] {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

func ensureEvalTaskOutputPaths(outputDir, workspaceName string) (taskOutputDir, agentLogPath, validationLogPath string, err error) {
	taskOutputDir = filepath.Join(outputDir, workspaceName)
	if err := os.MkdirAll(taskOutputDir, 0o755); err != nil {
		return "", "", "", err
	}
	return taskOutputDir,
		filepath.Join(taskOutputDir, "agent.log"),
		filepath.Join(taskOutputDir, "validation.log"),
		nil
}

func applyAgentExecutionResult(result *EvalResult, agentResult agentExecutionResult, agentLogPath, workspaceDir string) {
	result.AgentTime = agentResult.totalTime
	result.AgentTimedOut = agentResult.timedOut
	result.QuotaRetries = agentResult.quotaRetries
	result.InfraRetries = agentResult.infraRetries
	result.QuotaExhausted = agentResult.quotaExhausted
	result.InfraFailure = agentResult.infraFailure
	result.FailureClass = agentResult.failureClass

	metrics := parseAgentBehaviorMetrics(agentLogPath, workspaceDir)
	result.SelfTestCommands = metrics.SelfTestCommands
	result.SelfTestCommandsConfident = metrics.SelfTestCommandsConfident
	result.ToolchainInstallAttempts = metrics.ToolchainInstallAttempts
	result.OutOfWorkspaceReadAttempts = metrics.OutOfWorkspaceReads
	result.OutOfWorkspaceReadsConfident = metrics.OutOfWorkspaceReadsConfident
	result.SkillsUsed = metrics.SkillsUsed
	result.SkillsUsageSignals = metrics.SkillsUsageSignals
}

func shouldSkipValidationForExternalFailure(result *EvalResult) bool {
	switch result.FailureClass {
	case FailureClassInfra:
		if result.Error == "" {
			result.Error = "infra failure: agent produced no output after retries"
		}
		result.InfraFailure = true
		return true
	case FailureClassAuth:
		if result.Error == "" {
			result.Error = "auth failure: agent authentication failed"
		}
		return true
	case FailureClassQuotaExhausted:
		if result.Error == "" {
			result.Error = "quota failure: exhausted quota/rate-limit retries"
		}
		result.QuotaExhausted = true
		return true
	default:
		return false
	}
}

func isResumableExternalFailure(result EvalResult) bool {
	switch result.FailureClass {
	case FailureClassInfra, FailureClassAuth, FailureClassQuotaExhausted:
		return true
	default:
		return false
	}
}

func externalFailureLabel(class FailureClass) string {
	switch class {
	case FailureClassAuth:
		return "AUTH FAILURE"
	case FailureClassQuotaExhausted:
		return "QUOTA EXHAUSTED"
	case FailureClassInfra:
		return "INFRA FAILURE"
	default:
		return "EXTERNAL FAILURE"
	}
}

func removeTaskArtifactsForResume(outputDir string, result EvalResult) {
	if result.WorkspaceDir != "" {
		_ = os.RemoveAll(result.WorkspaceDir)
	}
	parts := strings.SplitN(result.Task, "/", 2)
	if len(parts) != 2 {
		return
	}
	taskOutputDir := filepath.Join(outputDir, parts[0]+"-"+parts[1])
	_ = os.RemoveAll(taskOutputDir)
}

func detectAndRecordIntegrityViolation(
	loader *task.Loader,
	t *task.Task,
	taskOutputDir, workspaceDir, validationLogPath string,
	result *EvalResult,
) (bool, error) {
	modified, err := detectModifiedTaskFiles(loader, t, workspaceDir)
	if err != nil {
		return false, err
	}
	if len(modified) == 0 {
		return false, nil
	}

	sort.Strings(modified)
	result.Error = fmt.Sprintf("modified task files (disallowed): %s", strings.Join(modified, ", "))
	result.FailureClass = FailureClassIntegrity

	if err := writeIntegrityViolationArtifacts(taskOutputDir, loader, t, workspaceDir, modified, result.Error); err != nil {
		logger.Warn("failed to write integrity artifacts", "task", t.ID(), "error", err)
	}
	writeValidationLog(
		validationLogPath,
		"",
		t.ValidationCommand(),
		-1,
		0,
		false,
		errors.New("skipped due integrity violation"),
	)
	return true, nil
}

func writeHiddenTestsIfNeeded(loader *task.Loader, t *task.Task, workspaceDir string) error {
	if evalLegacy {
		return nil
	}
	return writeTaskFilesToWorkspace(loader, t, workspaceDir, t.HiddenTestFiles())
}

func resolveValidationTimeout(timeout int) int {
	if timeout < 120 {
		return 120
	}
	return timeout
}

func buildValidationCommands(t *task.Task) (validationCmd, effectiveValidationCmd []string) {
	if t.Language == task.TypeScript && len(t.HiddenTestFiles()) > 0 {
		validationCmd = append([]string{}, t.ValidationCommand()...)
		for _, filename := range t.HiddenTestFiles() {
			validationCmd = append(validationCmd, task.StripTxtExtension(filename))
		}
	}

	effectiveValidationCmd = t.ValidationCommand()
	if len(validationCmd) > 0 {
		effectiveValidationCmd = validationCmd
	}
	return validationCmd, effectiveValidationCmd
}

func runValidationSession(
	ctx context.Context,
	r *runner.Runner,
	t *task.Task,
	workspaceDir string,
	validationTimeout int,
	validationCmd []string,
) (*resultpkg.Session, float64, error) {
	start := time.Now()
	session, err := r.Run(ctx, runner.RunOptions{
		Task:              t, // Pass task directly to avoid slug collision
		WorkspaceDir:      workspaceDir,
		Timeout:           validationTimeout,
		MaxAttempts:       1,
		ValidationCommand: validationCmd,
	})
	return session, time.Since(start).Seconds(), err
}

func handleValidationRunError(
	result *EvalResult,
	session *resultpkg.Session,
	runErr error,
	validationLogPath string,
	effectiveValidationCmd []string,
) {
	applyValidationSessionResult(result, session)
	rawOutput, exitCode, duration := validationErrorEvidence(session, result.ValidateTime)
	timedOut := strings.Contains(strings.ToLower(runErr.Error()), "timed out")

	writeValidationLog(
		validationLogPath,
		rawOutput,
		effectiveValidationCmd,
		exitCode,
		duration,
		timedOut,
		runErr,
	)

	result.Error = runErr.Error()
	if isValidationInfraError(runErr) {
		result.FailureClass = FailureClassInfra
		result.InfraFailure = true
		return
	}
	if timedOut {
		result.FailureClass = FailureClassValidationTimeout
		return
	}
	result.FailureClass = FailureClassValidationError
}

func isValidationInfraError(runErr error) bool {
	if runErr == nil {
		return false
	}
	lower := strings.ToLower(runErr.Error())
	for _, pattern := range validationInfraErrorPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func applyValidationSessionResult(result *EvalResult, session *resultpkg.Session) {
	if session == nil {
		return
	}
	result.Passed = session.Passed()
	result.Attempts = len(session.Attempts)
}

func validationErrorEvidence(session *resultpkg.Session, validateSeconds float64) (rawOutput string, exitCode int, duration time.Duration) {
	rawOutput, exitCode, duration, ok := lastSessionAttempt(session)
	if ok {
		return rawOutput, exitCode, duration
	}
	return "", -1, time.Duration(validateSeconds * float64(time.Second))
}

func writeValidationSessionLog(validationLogPath string, effectiveValidationCmd []string, session *resultpkg.Session) {
	rawOutput, exitCode, duration, ok := lastSessionAttempt(session)
	if !ok {
		writeValidationLog(validationLogPath, "", effectiveValidationCmd, -1, 0, false, nil)
		return
	}
	writeValidationLog(
		validationLogPath,
		rawOutput,
		effectiveValidationCmd,
		exitCode,
		duration,
		exitCode == -1,
		nil,
	)
}

func lastSessionAttempt(session *resultpkg.Session) (rawOutput string, exitCode int, duration time.Duration, ok bool) {
	if session == nil || len(session.Attempts) == 0 {
		return "", 0, 0, false
	}
	last := session.Attempts[len(session.Attempts)-1]
	return last.RawOutput, last.ExitCode, last.Duration, true
}

// finalizeEvalResult ensures status/score fields are populated for all return paths.
func finalizeEvalResult(result *EvalResult, start time.Time, weight task.Weight) {
	result.Duration = time.Since(start).Seconds()
	if result.FailureClass == "" {
		result.FailureClass = FailureClassNone
	}
	if result.FailureClass == FailureClassNone {
		switch {
		case strings.Contains(result.Error, "modified task files"):
			result.FailureClass = FailureClassIntegrity
		case strings.Contains(result.Error, "infra failure"):
			result.FailureClass = FailureClassInfra
		case strings.Contains(strings.ToLower(result.Error), "timed out"):
			result.FailureClass = FailureClassValidationTimeout
		case result.Error != "":
			result.FailureClass = FailureClassValidationError
		}
	}
	result.Status = task.DetermineStatus(result.Passed, result.AgentTimedOut, result.Error)
	result.WeightedScore = task.ScoreResult(result.Passed, result.AgentTimedOut, result.Error, weight)
}

// agentExecutionResult holds the outcome of agent execution with retries.
type agentExecutionResult struct {
	totalTime      float64
	timedOut       bool
	quotaRetries   int
	quotaExhausted bool
	infraRetries   int
	infraFailure   bool // true when agent produced no output after all retries
	failureClass   FailureClass
}

// executeAgentWithRetries runs the agent command with quota-aware retry logic.
// It also detects infra failures (empty/near-empty agent logs) and retries
// with aggressive backoff.
// workspaceReadyAt is the time when workspace setup completed (before the agent
// started); it is used to distinguish harness-written files from agent-written
// files when detecting infra failures.
func executeAgentWithRetries(
	ctx context.Context,
	t *task.Task,
	agentCfg *config.AgentConfig,
	prompt, model, workspaceDir, agentLogPath string,
	agentTimeout time.Duration,
	agent string,
	workspaceReadyAt time.Time,
) agentExecutionResult {
	var result agentExecutionResult
	var quotaAttempts, infraAttempts int
	var lastRetryType string // "quota" or "infra"

	for {
		totalAttempt := quotaAttempts + infraAttempts

		// On retry, wait (interruptible) and log.
		if totalAttempt > 0 {
			var delay time.Duration
			if lastRetryType == "infra" {
				delay = getInfraRetryDelay(infraAttempts)
			} else {
				delay = getRetryDelay(quotaAttempts)
			}
			logger.Info("retrying agent execution",
				"task", t.ID(),
				"attempt", totalAttempt,
				"type", lastRetryType,
				"delay", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
			}
		}

		// Check for interrupt before starting attempt.
		if ctx.Err() != nil {
			break
		}

		// Run single attempt.
		attemptResult := runAgentAttempt(ctx, agentCfg, prompt, model, workspaceDir, agentLogPath, agentTimeout, agent, totalAttempt)
		result.totalTime += attemptResult.duration
		result.timedOut = attemptResult.timedOut

		// Check non-recoverable auth errors first (no retries).
		if detectAuthError(agentLogPath) {
			result.failureClass = FailureClassAuth
			logger.Debug("authentication error, skipping retries", "task", t.ID())
			break
		}

		// Check quota/provider errors.
		hasError, isRecoverable := detectQuotaError(agentLogPath)
		if hasError {
			if !isRecoverable {
				result.quotaExhausted = true
				result.failureClass = FailureClassQuotaExhausted
				logger.Debug("non-recoverable quota error, skipping retries", "task", t.ID())
				break
			}
			quotaAttempts++
			result.quotaRetries = quotaAttempts
			result.failureClass = FailureClassQuotaRecoverable
			if quotaAttempts >= quotaMaxRetries {
				result.quotaExhausted = true
				result.failureClass = FailureClassQuotaExhausted
				logger.Debug("max quota retries reached", "task", t.ID(), "retries", quotaMaxRetries)
				break
			}
			lastRetryType = "quota"
			continue
		}

		// Check for infra failures (no quota error detected).
		if isInfraFailure(agentLogPath, workspaceDir, workspaceReadyAt) {
			infraAttempts++
			result.infraRetries = infraAttempts
			if infraAttempts >= infraMaxRetries {
				result.infraFailure = true
				result.failureClass = FailureClassInfra
				logger.Debug("max infra retries reached", "task", t.ID(), "retries", infraMaxRetries)
				break
			}
			lastRetryType = "infra"
			continue
		}

		// Success — no quota error, no infra failure.
		if result.failureClass == "" {
			result.failureClass = FailureClassNone
		}
		break
	}

	return result
}

// agentAttemptResult holds the outcome of a single agent attempt.
type agentAttemptResult struct {
	duration float64
	timedOut bool
}

// runAgentAttempt executes a single agent command attempt.
func runAgentAttempt(
	ctx context.Context,
	agentCfg *config.AgentConfig,
	prompt, model, workspaceDir, agentLogPath string,
	agentTimeout time.Duration,
	agent string,
	attempt int,
) agentAttemptResult {
	var result agentAttemptResult

	agentCtx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()

	cmd := buildAgentCommand(agentCtx, agentCfg, prompt, model, evalReasoning, evalDisableMCP, evalUseMCPTools, agent)
	cmd.Dir = workspaceDir

	// Use /dev/null for stdin to prevent TTY issues with agents that use Ink/React
	devNull, err := os.Open(os.DevNull)
	if err == nil {
		cmd.Stdin = devNull
		defer func() { _ = devNull.Close() }()
	}

	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil

	// Open log file: create on first attempt, append on retry
	logFile := openAgentLogFile(agentLogPath, attempt)
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer func() {
			_ = logFile.Sync()
			_ = logFile.Close()
		}()
	}

	// Wrap in bubblewrap sandbox if enabled.
	if evalSandboxActive {
		var extraDirs []string
		if cfg != nil {
			extraDirs = cfg.Sandbox.WritableDirs
		}
		cmd = wrapCommandWithSandbox(
			agentCtx,
			cmd,
			extraDirs,
			evalSandboxSharedRW,
			evalSandboxSharedRO,
			evalSandboxDenylist,
		)
	}

	// Run agent in its own process group so we can kill the entire tree on
	// timeout or interrupt, preventing orphaned child processes.
	setupProcessGroup(cmd)

	// Run agent
	agentStart := time.Now()
	agentErr := cmd.Run()
	result.duration = time.Since(agentStart).Seconds()

	// Check for timeout
	if errors.Is(agentCtx.Err(), context.DeadlineExceeded) {
		result.timedOut = true
		logger.Debug("agent timed out", "timeout", agentTimeout)
		writeAgentTimeoutFooter(logFile, attempt, agentTimeout, time.Since(agentStart))
	}
	if agentErr != nil {
		logger.Debug("agent returned error", "error", agentErr)
	}

	return result
}

// openAgentLogFile opens the agent log file for writing.
func openAgentLogFile(agentLogPath string, attempt int) *os.File {
	var logFile *os.File
	var err error

	if attempt == 0 {
		logFile, err = os.Create(agentLogPath)
	} else {
		logFile, err = os.OpenFile(agentLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			separator := fmt.Sprintf("\n\n=== RETRY %d (after %v delay) ===\n\n", attempt, getRetryDelay(attempt))
			_, _ = logFile.WriteString(separator)
		}
	}

	if err != nil {
		return nil
	}
	return logFile
}

// writeAgentTimeoutFooter appends deterministic timeout evidence to the agent log.
func writeAgentTimeoutFooter(logFile *os.File, attempt int, timeout, runDuration time.Duration) {
	if logFile == nil {
		return
	}
	_, _ = fmt.Fprintf(
		logFile,
		"\n\nHARNESS: agent timed out (attempt=%d timeout_seconds=%.3f duration_seconds=%.3f)\n",
		attempt+1,
		timeout.Seconds(),
		runDuration.Seconds(),
	)
	_ = logFile.Sync()
}

// writeValidationLog persists validation output with a machine-readable footer.
func writeValidationLog(path, rawOutput string, command []string, exitCode int, duration time.Duration, timedOut bool, runErr error) {
	var sb strings.Builder
	if rawOutput != "" {
		sb.WriteString(rawOutput)
		if !strings.HasSuffix(rawOutput, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(
		&sb,
		"HARNESS: validation command=%q exit_code=%d duration_seconds=%.3f timed_out=%t\n",
		strings.Join(command, " "),
		exitCode,
		duration.Seconds(),
		timedOut,
	)
	if runErr != nil {
		fmt.Fprintf(&sb, "HARNESS: validation run_error=%q\n", runErr.Error())
	}

	_ = os.WriteFile(path, []byte(sb.String()), 0o644)
}

// toolchainInfo returns a human-readable toolchain description for the given language.
func toolchainInfo(lang task.Language) string {
	switch lang {
	case task.Go:
		return "Go 1.25"
	case task.Rust:
		return "Rust 1.83 (stable)"
	case task.Zig:
		return "Zig 0.13.0"
	case task.Dart:
		return "Dart 3.3 SDK"
	case task.TypeScript:
		return "Node.js 20 with TypeScript (tsx)"
	case task.Kotlin:
		return "Kotlin (JDK 21, Gradle 8.5)"
	default:
		return string(lang)
	}
}

func buildAgentPrompt(t *task.Task, useMCPTools, useSkills bool, mcpPrompt string) string {
	stubFiles := make([]string, 0, len(t.Files.Stub))
	for _, f := range t.Files.Stub {
		stubFiles = append(stubFiles, task.StripTxtExtension(f))
	}
	testFiles := make([]string, 0, len(t.Files.Test))
	for _, f := range t.Files.Test {
		testFiles = append(testFiles, task.StripTxtExtension(f))
	}

	// The generic MCP guidance is injected into existing sections when enabled.
	// Agent-specific MCP text is intentionally ignored to keep this prompt path uniform.
	_ = mcpPrompt

	mcpEnvironmentLine := ""
	mcpImportantLine := ""
	mcpRuleLine := ""
	skillsEnvironmentLine := ""
	skillsImportantLine := ""
	skillsRuleLine := ""
	taskInstructions := `1. Read the stub file(s) (function signatures with panic()/todo!/Unimplemented placeholders).
2. Read the visible test file(s) to understand expected behavior and edge cases.
3. Implement the stub file(s), replacing placeholders with working code.
4. Ensure your solution handles edge cases and performance constraints.
5. Ensure thread-safety if the tests use concurrent operations.`
	if useMCPTools {
		mcpEnvironmentLine = "\n- You have access to MCP server tools. Review what is available to you before starting work."
		taskInstructions = `1. Use your MCP server tools to help complete your task(s) wherever and whenever applicable.
2. Read the stub file(s) (function signatures with panic()/todo!/Unimplemented placeholders).
3. Read the visible test file(s) to understand expected behavior and edge cases.
4. Implement the stub file(s), replacing placeholders with working code.
5. Ensure your solution handles edge cases and performance constraints.
6. Ensure thread-safety if the tests use concurrent operations.`
		mcpImportantLine = "\n- Prefer your MCP server tools over built-in alternatives if both can accomplish the same step or objective."
		mcpRuleLine = "\n- You MUST actively use your MCP server tools to assist you with your work. Do NOT ignore them. Make your first MCP server tool call before writing any code."
	}
	if useSkills {
		skillsEnvironmentLine = "\n- You have access to Agent Skills. Use the 'activate_skill' tool to read their documentation and load their specialized workflows. Do NOT try to read the skill markdown files directly from the filesystem."
		skillsImportantLine = "\n- Load at least one relevant Agent Skill when available, and prefer Agent Skills over manual alternatives if both can accomplish the same step or objective."
		skillsRuleLine = "\n- You MUST actively use your Agent Skills to assist you with your work. Do NOT ignore them. Make your first Agent Skill call before writing any code."
	}

	prompt := fmt.Sprintf(`You are solving a coding task called "%s".

TASK INFO:
- Language:    %s
- Tier:        %s
- Difficulty:  %s
- Description: %s

FILES TO READ:
- Stub/solution files: %s
- Test files:          %s

ENVIRONMENT:
- Final validation runs automatically in a Docker container.
- Toolchain: %s
- You may run local tests/commands in the workspace while iterating.
- Toolchains are preinstalled; extra installs are optional.%s%s

YOUR TASK:
%s

IMPORTANT:
- There may be hidden tests that check additional edge cases for the same public API.%s%s

RULES:
- ONLY edit the stub/solution source file(s).
- Do NOT modify test files or support files.
- You may add new helper source files if needed.
- Evaluation fails if you modify protected files.
- Do NOT navigate to parent directories or read files outside the workspace.%s%s`,
		t.Name, t.Language, t.Tier, t.Difficulty, t.Description,
		strings.Join(stubFiles, ", "), strings.Join(testFiles, ", "),
		toolchainInfo(t.Language), mcpEnvironmentLine, skillsEnvironmentLine, taskInstructions, mcpImportantLine, skillsImportantLine, mcpRuleLine, skillsRuleLine)

	return prompt
}

func detectModifiedTaskFiles(loader *task.Loader, t *task.Task, workspaceDir string) ([]string, error) {
	var modified []string
	for _, filename := range append(append([]string{}, t.Files.Test...), t.Files.Support...) {
		want, err := loader.ReadTaskFile(t, filename)
		if err != nil {
			return nil, fmt.Errorf("reading canonical %s: %w", filename, err)
		}

		workspacePath := filepath.Join(workspaceDir, task.StripTxtExtension(filename))
		got, err := os.ReadFile(workspacePath)
		if err != nil {
			modified = append(modified, task.StripTxtExtension(filename))
			continue
		}
		if !bytes.Equal(got, want) {
			modified = append(modified, task.StripTxtExtension(filename))
		}
	}
	return modified, nil
}

type integrityArtifactReport struct {
	Task      string                  `json:"task"`
	Timestamp string                  `json:"timestamp"`
	Reason    string                  `json:"reason"`
	Files     []integrityArtifactFile `json:"files"`
}

type integrityArtifactFile struct {
	Path             string `json:"path"`
	CanonicalFile    string `json:"canonical_file,omitempty"`
	ExpectedExists   bool   `json:"expected_exists"`
	ActualExists     bool   `json:"actual_exists"`
	ExpectedHash     string `json:"expected_hash,omitempty"`
	ActualHash       string `json:"actual_hash,omitempty"`
	ExpectedArtifact string `json:"expected_artifact,omitempty"`
	ActualArtifact   string `json:"actual_artifact,omitempty"`
	DiffArtifact     string `json:"diff_artifact,omitempty"`
}

//nolint:gocognit // Handles artifact generation across expected/actual/missing file combinations.
func writeIntegrityViolationArtifacts(
	taskOutputDir string,
	loader *task.Loader,
	t *task.Task,
	workspaceDir string,
	modified []string,
	reason string,
) error {
	if err := os.MkdirAll(taskOutputDir, 0o755); err != nil {
		return fmt.Errorf("creating task output dir: %w", err)
	}

	filesRoot := filepath.Join(taskOutputDir, "integrity-files")
	diffRoot := filepath.Join(taskOutputDir, "integrity-diff")
	if err := os.MkdirAll(filesRoot, 0o755); err != nil {
		return fmt.Errorf("creating integrity files dir: %w", err)
	}
	if err := os.MkdirAll(diffRoot, 0o755); err != nil {
		return fmt.Errorf("creating integrity diff dir: %w", err)
	}

	canonicalByWorkspace := make(map[string]string)
	for _, filename := range append(append([]string{}, t.Files.Test...), t.Files.Support...) {
		canonicalByWorkspace[task.StripTxtExtension(filename)] = filename
	}

	report := integrityArtifactReport{
		Task:      t.ID(),
		Timestamp: time.Now().Format(time.RFC3339),
		Reason:    reason,
		Files:     make([]integrityArtifactFile, 0, len(modified)),
	}

	for _, workspaceName := range modified {
		canonicalName := canonicalByWorkspace[workspaceName]
		entry := integrityArtifactFile{
			Path:          workspaceName,
			CanonicalFile: canonicalName,
		}

		expectedBytes := []byte(nil)
		if canonicalName != "" {
			content, err := loader.ReadTaskFile(t, canonicalName)
			if err == nil {
				expectedBytes = content
				entry.ExpectedExists = true
				entry.ExpectedHash = hashBytes(content)
			}
		}

		actualPath := filepath.Join(workspaceDir, workspaceName)
		actualBytes, err := os.ReadFile(actualPath)
		if err == nil {
			entry.ActualExists = true
			entry.ActualHash = hashBytes(actualBytes)
		}

		expectedArtifactAbs := filepath.Join(filesRoot, workspaceName+".expected")
		actualArtifactAbs := filepath.Join(filesRoot, workspaceName+".actual")
		diffArtifactAbs := filepath.Join(diffRoot, workspaceName+".diff")

		if entry.ExpectedExists {
			if err := os.MkdirAll(filepath.Dir(expectedArtifactAbs), 0o755); err != nil {
				return fmt.Errorf("creating expected artifact dir: %w", err)
			}
			if err := os.WriteFile(expectedArtifactAbs, expectedBytes, 0o644); err != nil {
				return fmt.Errorf("writing expected artifact: %w", err)
			}
			if rel, err := filepath.Rel(taskOutputDir, expectedArtifactAbs); err == nil {
				entry.ExpectedArtifact = filepath.ToSlash(rel)
			}
		}

		if entry.ActualExists {
			if err := os.MkdirAll(filepath.Dir(actualArtifactAbs), 0o755); err != nil {
				return fmt.Errorf("creating actual artifact dir: %w", err)
			}
			if err := os.WriteFile(actualArtifactAbs, actualBytes, 0o644); err != nil {
				return fmt.Errorf("writing actual artifact: %w", err)
			}
			if rel, err := filepath.Rel(taskOutputDir, actualArtifactAbs); err == nil {
				entry.ActualArtifact = filepath.ToSlash(rel)
			}
		}

		if err := os.MkdirAll(filepath.Dir(diffArtifactAbs), 0o755); err != nil {
			return fmt.Errorf("creating diff artifact dir: %w", err)
		}
		diffContent := buildIntegrityDiffContent(expectedArtifactAbs, actualArtifactAbs, entry.ExpectedExists, entry.ActualExists)
		if err := os.WriteFile(diffArtifactAbs, []byte(diffContent), 0o644); err != nil {
			return fmt.Errorf("writing diff artifact: %w", err)
		}
		if rel, err := filepath.Rel(taskOutputDir, diffArtifactAbs); err == nil {
			entry.DiffArtifact = filepath.ToSlash(rel)
		}

		report.Files = append(report.Files, entry)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal integrity report: %w", err)
	}
	if err := os.WriteFile(filepath.Join(taskOutputDir, "integrity.json"), data, 0o644); err != nil {
		return fmt.Errorf("writing integrity report: %w", err)
	}

	return nil
}

func buildIntegrityDiffContent(expectedPath, actualPath string, expectedExists, actualExists bool) string {
	switch {
	case expectedExists && actualExists:
		cmd := exec.CommandContext(context.Background(), "diff", "-u", expectedPath, actualPath)
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			return string(out)
		}
		if err != nil {
			return fmt.Sprintf("diff failed: %v\n", err)
		}
		return "no diff output (files may be identical)\n"
	case expectedExists && !actualExists:
		return fmt.Sprintf(
			"--- %s\n+++ %s\n@@\n- expected file exists\n+ actual file missing\n",
			filepath.ToSlash(expectedPath),
			filepath.ToSlash(actualPath),
		)
	case !expectedExists && actualExists:
		return fmt.Sprintf(
			"--- %s\n+++ %s\n@@\n- expected file missing\n+ unexpected actual file exists\n",
			filepath.ToSlash(expectedPath),
			filepath.ToSlash(actualPath),
		)
	default:
		return "both expected and actual files are missing\n"
	}
}

func writeTaskFilesToWorkspace(loader *task.Loader, t *task.Task, workspaceDir string, files []string) error {
	for _, filename := range files {
		content, err := loader.ReadTaskFile(t, filename)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filename, err)
		}

		destFilename := task.StripTxtExtension(filename)
		destPath := filepath.Join(workspaceDir, destFilename)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", destFilename, err)
		}
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destFilename, err)
		}
	}
	return nil
}

// copyDirContents recursively copies all files and directories from src to dst.
// It preserves directory structure and file permissions.
func copyDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(destPath, data, info.Mode().Perm())
	})
}

// buildAgentCommand creates an exec.Cmd for the given agent configuration.
// It handles prompt placeholder substitution, model flag positioning, reasoning flag, and environment variables.
// For OpenCode, disableMCP disables MCP tools and useMCPTools raises the MCP request timeout.
func buildAgentCommand(
	ctx context.Context,
	agentCfg *config.AgentConfig,
	prompt, model, reasoning string,
	disableMCP, useMCPTools bool,
	agentName string,
) *exec.Cmd {
	var args []string

	// Determine model flag position (default to "before")
	modelPosition := agentCfg.ModelFlagPosition
	if modelPosition == "" {
		modelPosition = "before"
	}

	// Determine reasoning flag position (default to "before")
	reasoningPosition := agentCfg.ReasoningFlagPosition
	if reasoningPosition == "" {
		reasoningPosition = "before"
	}

	// Add model flag if specified (before position)
	// If ModelFlag contains {value}, substitute it; otherwise append as separate arg
	if model != "" && agentCfg.ModelFlag != "" && modelPosition == "before" {
		if strings.Contains(agentCfg.ModelFlag, "{value}") {
			// Format: "--{value}" -> "--max" (single arg with value substituted)
			args = append(args, strings.ReplaceAll(agentCfg.ModelFlag, "{value}", model))
		} else {
			// Format: "-m" "model-name" (two separate args)
			args = append(args, agentCfg.ModelFlag, model)
		}
	}

	// Add reasoning flag if specified (before position)
	// If ReasoningFlag contains {value}, substitute it; otherwise append as separate arg
	if reasoning != "" && agentCfg.ReasoningFlag != "" && reasoningPosition == "before" {
		if strings.Contains(agentCfg.ReasoningFlag, "{value}") {
			// Format: "-c key={value}" -> "-c key=high"
			args = append(args, strings.ReplaceAll(agentCfg.ReasoningFlag, "{value}", reasoning))
		} else {
			// Format: "-r" "high"
			args = append(args, agentCfg.ReasoningFlag, reasoning)
		}
	}

	// Process args, replacing {prompt} placeholder
	for _, arg := range agentCfg.Args {
		if arg == "{prompt}" {
			args = append(args, prompt)
		} else {
			args = append(args, arg)
		}
	}

	// Add model flag if specified (after position)
	// If ModelFlag contains {value}, substitute it; otherwise append as separate arg
	if model != "" && agentCfg.ModelFlag != "" && modelPosition == "after" {
		if strings.Contains(agentCfg.ModelFlag, "{value}") {
			// Format: "--{value}" -> "--max" (single arg with value substituted)
			args = append(args, strings.ReplaceAll(agentCfg.ModelFlag, "{value}", model))
		} else {
			// Format: "-m" "model-name" (two separate args)
			args = append(args, agentCfg.ModelFlag, model)
		}
	}

	// Add reasoning flag if specified (after position)
	// If ReasoningFlag contains {value}, substitute it; otherwise append as separate arg
	if reasoning != "" && agentCfg.ReasoningFlag != "" && reasoningPosition == "after" {
		if strings.Contains(agentCfg.ReasoningFlag, "{value}") {
			// Format: "-c key={value}" -> "-c key=high"
			args = append(args, strings.ReplaceAll(agentCfg.ReasoningFlag, "{value}", reasoning))
		} else {
			// Format: "-r" "high"
			args = append(args, agentCfg.ReasoningFlag, reasoning)
		}
	}

	cmd := exec.CommandContext(ctx, agentCfg.Command, args...)
	cmd.Env = buildAgentEnv(agentCfg.Env, disableMCP, useMCPTools, agentName)

	return cmd
}

// wrapCommandWithSandbox wraps an exec.Cmd in a bubblewrap sandbox.
// The sandbox restricts filesystem access so the agent can only write to the
// workspace directory and /tmp. The rest of the filesystem (including $HOME)
// is mounted read-only. Network access is preserved for LLM API calls.
func wrapCommandWithSandbox(
	ctx context.Context,
	cmd *exec.Cmd,
	extraWritableDirs, sharedReadWriteDirs, sharedReadOnlyDirs, readableDenylist []string,
) *exec.Cmd {
	bwrapArgs := buildSandboxArgs(
		cmd.Dir,
		cmd.Path,
		extraWritableDirs,
		sharedReadWriteDirs,
		sharedReadOnlyDirs,
		readableDenylist,
	)
	bwrapArgs = append(bwrapArgs, "--", cmd.Path)
	bwrapArgs = append(bwrapArgs, cmd.Args[1:]...)

	wrapped := exec.CommandContext(ctx, "bwrap", bwrapArgs...)
	wrapped.Env = cmd.Env
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr

	return wrapped
}

// buildSandboxArgs constructs the bubblewrap arguments for filesystem isolation.
func buildSandboxArgs(
	workspaceDir, commandPath string,
	extraWritableDirs, sharedReadWriteDirs, sharedReadOnlyDirs, readableDenylist []string,
) []string {
	homeDir, _ := os.UserHomeDir()

	var args []string

	// Mount system directories read-only.
	for _, dir := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/run"} {
		if _, err := os.Stat(dir); err == nil {
			args = append(args, "--ro-bind", dir, dir)
		}
	}

	// Mount $HOME read-only and expose an explicit broad shared allowlist below.
	args = append(args, "--ro-bind", homeDir, homeDir)

	writableSpecs := make([]string, 0, len(sharedReadWriteDirs)+len(extraWritableDirs)+1)
	writableSpecs = append(writableSpecs, sharedReadWriteDirs...)
	// Backward compatible: writable_dirs remains an explicit additional writable allowlist.
	writableSpecs = append(writableSpecs, extraWritableDirs...)
	writableSpecs = append(writableSpecs, "go")

	readonlySpecs := make([]string, 0, len(sharedReadOnlyDirs)+1)
	readonlySpecs = append(readonlySpecs, sharedReadOnlyDirs...)
	if commandPath != "" {
		readonlySpecs = append(readonlySpecs, filepath.Dir(commandPath))
	}

	writablePaths := resolveSandboxMountPaths(homeDir, writableSpecs)
	readonlyPaths := resolveSandboxMountPaths(homeDir, readonlySpecs)

	writableSet := make(map[string]struct{}, len(writablePaths))
	for _, absPath := range writablePaths {
		if _, exists := writableSet[absPath]; exists {
			continue
		}
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		writableSet[absPath] = struct{}{}
		args = append(args, "--bind", absPath, absPath)
	}

	for _, absPath := range readonlyPaths {
		if _, writable := writableSet[absPath]; writable {
			continue
		}
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		args = append(args, "--ro-bind", absPath, absPath)
	}

	// /tmp for temporary files.
	args = append(args, "--tmpfs", "/tmp")

	// Workspace is the only persistent writable directory outside $HOME.
	// Mounted after --tmpfs /tmp so it takes precedence when the workspace
	// is inside /tmp (which it is during eval for isolation).
	args = append(args, "--bind", workspaceDir, workspaceDir)

	// Mask non-allowlisted top-level home directories to prevent browsing unrelated
	// host data while keeping configured shared directories accessible.
	allowedTopLevel := collectAllowedHomeTopLevel(homeDir, append(writablePaths, readonlyPaths...))
	args = appendSandboxHomeMasks(args, homeDir, workspaceDir, allowedTopLevel)

	// Mask sensitive host directories with empty tmpfs mounts so agents cannot
	// read hidden tests, prior eval outputs, or historical sessions.
	readableDenylist = append(readableDenylist, defaultSandboxSensitiveHomeMasks(homeDir)...)
	args = appendSandboxDenylistMasks(args, workspaceDir, readableDenylist)

	// Required virtual filesystems.
	args = append(args, "--dev", "/dev")
	args = append(args, "--proc", "/proc")

	// Namespace isolation: new mount/pid/user/ipc/uts namespaces, but keep network.
	args = append(args, "--unshare-all", "--share-net")

	// Kill agent if harness dies.
	args = append(args, "--die-with-parent")

	// Set working directory to workspace.
	args = append(args, "--chdir", workspaceDir)

	return args
}

func resolveSandboxMountPaths(homeDir string, specs []string) []string {
	seen := make(map[string]struct{}, len(specs))
	paths := make([]string, 0, len(specs))
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		absPath := spec
		switch {
		case spec == "~":
			absPath = homeDir
		case strings.HasPrefix(spec, "~/"):
			absPath = filepath.Join(homeDir, strings.TrimPrefix(spec, "~/"))
		case !filepath.IsAbs(spec):
			absPath = filepath.Join(homeDir, spec)
		}
		absPath = canonicalizeExistingPath(absPath)
		if _, exists := seen[absPath]; exists {
			continue
		}
		seen[absPath] = struct{}{}
		paths = append(paths, absPath)
	}
	return paths
}

func collectAllowedHomeTopLevel(homeDir string, mountedPaths []string) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, mounted := range mountedPaths {
		rel, err := filepath.Rel(homeDir, mounted)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) == 0 || parts[0] == "" || parts[0] == "." {
			continue
		}
		allowed[parts[0]] = struct{}{}
	}
	return allowed
}

func appendSandboxHomeMasks(args []string, homeDir, workspaceDir string, allowedTopLevel map[string]struct{}) []string {
	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		workspaceAbs = workspaceDir
	}

	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return args
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, allowed := allowedTopLevel[entry.Name()]; allowed {
			continue
		}
		path := filepath.Join(homeDir, entry.Name())
		if path == workspaceAbs || strings.HasPrefix(workspaceAbs, path+string(os.PathSeparator)) {
			continue
		}
		args = append(args, "--tmpfs", path)
	}
	return args
}

func defaultSandboxSensitiveHomeMasks(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".factory", "sessions"),
		filepath.Join(homeDir, ".claude", "projects"),
		filepath.Join(homeDir, ".qwen", "projects"),
		filepath.Join(homeDir, ".junie", "projects"),
		filepath.Join(homeDir, ".local", "share", "Trash"),
	}
}

func appendSandboxDenylistMasks(args []string, workspaceDir string, readableDenylist []string) []string {
	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return args
	}

	for _, rawPath := range readableDenylist {
		if rawPath == "" {
			continue
		}

		denyPath, err := normalizeDenylistPath(rawPath)
		if err != nil {
			continue
		}
		if denyPath == workspaceAbs {
			continue
		}
		if _, err := os.Stat(denyPath); err != nil {
			continue
		}
		args = append(args, "--tmpfs", denyPath)
	}

	return args
}

func normalizeDenylistPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

// getOpenCodeConfigPaths returns the possible paths for the OpenCode config file.
// It checks XDG_CONFIG_HOME first, then falls back to ~/.config/opencode/.
func getOpenCodeConfigPaths() []string {
	var paths []string

	// Check XDG_CONFIG_HOME first (follows XDG Base Directory spec)
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		paths = append(paths,
			filepath.Join(xdgConfig, "opencode", "opencode.jsonc"),
			filepath.Join(xdgConfig, "opencode", "opencode.json"),
		)
	}

	// Fall back to ~/.config/opencode/
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc"),
			filepath.Join(homeDir, ".config", "opencode", "opencode.json"),
		)
	}

	return paths
}

// readOpenCodeConfig reads and parses the OpenCode config file.
// Returns nil if the config file doesn't exist or can't be parsed.
func readOpenCodeConfig() map[string]any {
	for _, configPath := range getOpenCodeConfigPaths() {
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue // File doesn't exist or can't be read
		}

		// Strip JSONC comments (// and /* */) for parsing
		// This is a simple approach that handles most cases
		data = stripJSONComments(data)

		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			continue // Invalid JSON, try next path
		}

		return config
	}

	return nil
}

// stripJSONComments removes single-line (//) and multi-line (/* */) comments from JSON.
// This allows parsing JSONC (JSON with Comments) files.
//
//nolint:gocognit // State machine for parsing requires multiple conditionals
func stripJSONComments(data []byte) []byte {
	var result bytes.Buffer
	inString := false
	inSingleComment := false
	inMultiComment := false

	for i := 0; i < len(data); i++ {
		c := data[i]

		// Handle string context (don't strip comments inside strings)
		if !inSingleComment && !inMultiComment {
			if c == '"' && (i == 0 || data[i-1] != '\\') {
				inString = !inString
			}
		}

		if inString {
			result.WriteByte(c)
			continue
		}

		// Handle single-line comments
		if inSingleComment {
			if c == '\n' {
				inSingleComment = false
				result.WriteByte(c) // Keep newline for line counting
			}
			continue
		}

		// Handle multi-line comments
		if inMultiComment {
			if c == '*' && i+1 < len(data) && data[i+1] == '/' {
				inMultiComment = false
				i++ // Skip the '/'
			}
			continue
		}

		// Check for comment start
		if c == '/' && i+1 < len(data) {
			if data[i+1] == '/' {
				inSingleComment = true
				i++ // Skip the second '/'
				continue
			}
			if data[i+1] == '*' {
				inMultiComment = true
				i++ // Skip the '*'
				continue
			}
		}

		result.WriteByte(c)
	}

	return result.Bytes()
}

// deepMergeJSON performs a deep merge of two JSON-like maps.
// Values from 'override' take precedence over 'base'.
// Nested maps are merged recursively; other values are replaced.
func deepMergeJSON(base, override map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}

	result := make(map[string]any)

	// Copy base values
	for k, v := range base {
		result[k] = v
	}

	// Merge override values
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, merge recursively
			baseMap, baseIsMap := baseVal.(map[string]any)
			overrideMap, overrideIsMap := v.(map[string]any)
			if baseIsMap && overrideIsMap {
				result[k] = deepMergeJSON(baseMap, overrideMap)
				continue
			}
		}
		// Otherwise, override takes precedence
		result[k] = v
	}

	return result
}

const openCodeGlobalMCPTimeoutMS = 180000

// buildOpenCodeMCPDisableConfig creates the OPENCODE_CONFIG_CONTENT value
// by merging the user's existing config with the MCP disable settings.
func buildOpenCodeMCPDisableConfig() string {
	return buildOpenCodeMCPConfig(true, false)
}

// buildOpenCodeMCPConfig creates the OPENCODE_CONFIG_CONTENT value with optional MCP settings.
// It preserves the user's existing OpenCode config and applies runtime overrides.
func buildOpenCodeMCPConfig(disableMCP, useMCPTools bool) string {
	overrides := map[string]any{}

	// MCP tools are registered as "servername_toolname", so "*_*" matches all.
	if disableMCP {
		overrides["tools"] = map[string]any{
			"*_*": false,
		}
	}

	// Increase global MCP timeout for tool-heavy runs.
	if useMCPTools {
		overrides["experimental"] = map[string]any{
			"mcp_timeout": openCodeGlobalMCPTimeoutMS,
		}
	}

	// Try to read the user's existing config.
	userConfig := readOpenCodeConfig()

	var finalConfig map[string]any
	if userConfig != nil {
		// Merge user config with runtime overrides (runtime overrides take precedence).
		finalConfig = deepMergeJSON(userConfig, overrides)
	} else {
		finalConfig = overrides
	}

	// Serialize to JSON.
	data, err := json.Marshal(finalConfig)
	if err != nil {
		if disableMCP && useMCPTools {
			return `{"tools":{"*_*":false},"experimental":{"mcp_timeout":180000}}`
		}
		if disableMCP {
			return `{"tools":{"*_*":false}}`
		}
		return `{"experimental":{"mcp_timeout":180000}}`
	}

	return string(data)
}

// buildAgentEnv creates the environment variable slice for an agent command.
// It merges the agent's configured env vars with any runtime injections.
func buildAgentEnv(agentEnv map[string]string, disableMCP, useMCPTools bool, agentName string) []string {
	needsOpenCodeConfig := agentName == "opencode" && (disableMCP || useMCPTools)
	if len(agentEnv) == 0 && !needsOpenCodeConfig {
		return nil
	}

	env := os.Environ()
	for k, v := range agentEnv {
		env = append(env, k+"="+v)
	}

	// Inject OpenCode config overrides for MCP behavior.
	if needsOpenCodeConfig {
		configContent := buildOpenCodeMCPConfig(disableMCP, useMCPTools)
		env = append(env, "OPENCODE_CONFIG_CONTENT="+configContent)
	}

	return env
}

// EvalAttestation provides cryptographic verification of eval results.
type EvalAttestation struct {
	Version   string                     `json:"version"`
	Harness   AttestationHarness         `json:"harness"`
	Eval      AttestationEval            `json:"eval"`
	Tasks     map[string]AttestationTask `json:"tasks"`
	Integrity AttestationIntegrity       `json:"integrity"`
}

// AttestationHarness contains harness version information.
type AttestationHarness struct {
	Version       string `json:"version"`
	BuildDate     string `json:"build_date"`
	WeightVersion string `json:"weight_version,omitempty"`
}

// AttestationEval contains evaluation metadata.
type AttestationEval struct {
	Agent     string  `json:"agent"`
	Model     string  `json:"model,omitempty"`
	Timestamp string  `json:"timestamp"`
	Duration  float64 `json:"duration_seconds"`
}

// AttestationTask contains per-task verification data.
type AttestationTask struct {
	TaskHash     string  `json:"task_hash"`
	SolutionHash string  `json:"solution_hash,omitempty"`
	Passed       bool    `json:"passed"`
	Duration     float64 `json:"duration_seconds"`
}

// AttestationIntegrity contains aggregate hashes for verification.
type AttestationIntegrity struct {
	TasksHash   string `json:"tasks_hash"`
	ResultsHash string `json:"results_hash"`
}

// hashBytes returns the BLAKE3 hash of data as a prefixed hex string.
func hashBytes(data []byte) string {
	h := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(h[:])
}

// hashFiles returns the BLAKE3 hash of multiple files concatenated.
// If no files were readable, foundAny is false and hash is empty.
func hashFiles(paths []string) (hash string, foundAny bool, err error) {
	hasher := blake3.New()
	found := false
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip missing files
		}
		found = true
		_, _ = hasher.Write(data)
	}
	if !found {
		return "", false, nil
	}
	sum := hasher.Sum(nil)
	return "blake3:" + hex.EncodeToString(sum), true, nil
}

// generateAttestation creates an attestation for the eval run.
// newlyRunTasks contains task IDs that were executed in this session.
// previousTasks contains attestation data from a previous run (for resume).
func generateAttestation(
	agent, model, timestamp string,
	totalDuration float64,
	results []EvalResult,
	outputDir string,
	loader *task.Loader,
	allTasks []*task.Task,
	newlyRunTasks map[string]bool,
	previousTasks map[string]AttestationTask,
) (*EvalAttestation, error) {
	attestation := &EvalAttestation{
		Version: "1",
		Harness: AttestationHarness{
			Version:       Version,
			BuildDate:     BuildDate,
			WeightVersion: task.WeightVersion,
		},
		Eval: AttestationEval{
			Agent:     agent,
			Model:     model,
			Timestamp: timestamp,
			Duration:  totalDuration,
		},
		Tasks: make(map[string]AttestationTask),
	}

	// Build a map of tasks by ID for quick lookup
	taskMap := make(map[string]*task.Task)
	for _, t := range allTasks {
		taskMap[t.ID()] = t
	}

	// Compute task hashes
	var allTaskHashes []byte
	for _, r := range results {
		t := taskMap[r.Task]
		if t == nil {
			continue
		}

		// If this task was NOT newly run and we have previous attestation data, use it.
		if !newlyRunTasks[r.Task] {
			if prev, ok := previousTasks[r.Task]; ok && prev.TaskHash != "" {
				attestation.Tasks[r.Task] = prev
				allTaskHashes = append(allTaskHashes, []byte(prev.TaskHash)...)
				continue
			}
		}

		// Hash task files (stub + test + support)
		var taskFileContents []byte
		for _, f := range append(append(t.Files.Stub, t.Files.Test...), t.Files.Support...) {
			if content, err := loader.ReadTaskFile(t, f); err == nil {
				taskFileContents = append(taskFileContents, content...)
			}
		}
		taskHash := hashBytes(taskFileContents)

		// Hash solution files if they exist
		var solutionHash string
		workspaceDir := filepath.Join(outputDir, fmt.Sprintf("%s-%s", t.Language, t.Slug))
		solutionPaths := make([]string, 0, len(t.Files.Stub))
		for _, f := range t.Files.Stub {
			solutionPaths = append(solutionPaths, filepath.Join(workspaceDir, task.StripTxtExtension(f)))
		}
		if hash, found, err := hashFiles(solutionPaths); err == nil && found {
			solutionHash = hash
		}

		attestation.Tasks[r.Task] = AttestationTask{
			TaskHash:     taskHash,
			SolutionHash: solutionHash,
			Passed:       r.Passed,
			Duration:     r.Duration,
		}

		allTaskHashes = append(allTaskHashes, []byte(taskHash)...)
	}

	// Compute integrity hashes
	attestation.Integrity.TasksHash = hashBytes(allTaskHashes)

	// Hash the results JSON
	resultsJSON, _ := json.Marshal(results)
	attestation.Integrity.ResultsHash = hashBytes(resultsJSON)

	return attestation, nil
}

// LeaderboardSubmission is a compact format for submitting results to a leaderboard website.
type LeaderboardSubmission struct {
	// Identity
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Timestamp string `json:"timestamp"`

	// Core metrics
	PassRate         float64 `json:"pass_rate"`
	WeightedPassRate float64 `json:"weighted_pass_rate"`
	Passed           int     `json:"passed"`
	Failed           int     `json:"failed"`
	Total            int     `json:"total"`

	// Weighted scoring
	WeightedScore    float64 `json:"weighted_score"`
	MaxPossibleScore float64 `json:"max_possible_score"`

	// Quality metrics
	IntegrityViolations int `json:"integrity_violations"`

	// Per-language breakdown
	ByLanguage map[string]LeaderboardLanguageStats `json:"by_language"`

	// Timing
	TotalDurationSec float64 `json:"total_duration_seconds"`
	AgentDurationSec float64 `json:"agent_duration_seconds"`

	// Verification
	HarnessVersion string `json:"harness_version"`
	WeightVersion  string `json:"weight_version"`
	TasksHash      string `json:"tasks_hash"`
	ResultsHash    string `json:"results_hash"`

	// Configuration
	Timeout                         int     `json:"timeout"`
	Parallel                        int     `json:"parallel"`
	UseMCPTools                     bool    `json:"use_mcp_tools"`
	UseSkills                       bool    `json:"use_skills"`
	DisableMCP                      bool    `json:"disable_mcp"`
	Sandbox                         bool    `json:"sandbox"`
	Legacy                          bool    `json:"legacy"`
	QuotaAffectedTasks              int     `json:"quota_affected_tasks"`
	AuthAffectedTasks               int     `json:"auth_affected_tasks"`
	InfraAffectedTasks              int     `json:"infra_affected_tasks"`
	TotalQuotaRetries               int     `json:"total_quota_retries"`
	TotalInfraRetries               int     `json:"total_infra_retries"`
	TotalSelfTestCommands           int     `json:"total_self_test_commands"`
	TotalToolchainInstallAttempts   int     `json:"total_toolchain_install_attempts"`
	TotalOutOfWorkspaceReadAttempts int     `json:"total_out_of_workspace_read_attempts"`
	SkillsUsageRate                 float64 `json:"skills_usage_rate"`
	TotalSkillsUsageSignals         int     `json:"total_skills_usage_signals"`
	TasksWithSelfTesting            int     `json:"tasks_with_self_testing"`
	TasksWithToolchainInstall       int     `json:"tasks_with_toolchain_install"`
	TasksWithOutOfWorkspaceReads    int     `json:"tasks_with_out_of_workspace_reads"`
	TasksWithSkillsUsage            int     `json:"tasks_with_skills_usage"`
}

// LeaderboardLanguageStats contains per-language metrics for the leaderboard.
type LeaderboardLanguageStats struct {
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Total    int     `json:"total"`
	PassRate float64 `json:"pass_rate"`
}

// generateLeaderboardSubmission creates a compact submission file for leaderboard websites.
func generateLeaderboardSubmission(summary EvalSummary, attestation *EvalAttestation) LeaderboardSubmission {
	submission := LeaderboardSubmission{
		Agent:                           summary.Agent,
		Model:                           summary.Model,
		Reasoning:                       summary.Reasoning,
		Timestamp:                       summary.Timestamp,
		PassRate:                        summary.PassRate,
		WeightedPassRate:                summary.WeightedPassRate,
		Passed:                          summary.Passed,
		Failed:                          summary.Failed,
		Total:                           summary.Total,
		WeightedScore:                   summary.WeightedScore,
		MaxPossibleScore:                summary.MaxPossibleScore,
		IntegrityViolations:             summary.IntegrityViolations,
		TotalDurationSec:                summary.Duration,
		AgentDurationSec:                summary.AgentTime,
		Timeout:                         summary.Timeout,
		Parallel:                        summary.Parallel,
		UseMCPTools:                     summary.UseMCPTools,
		UseSkills:                       summary.UseSkills,
		DisableMCP:                      summary.DisableMCP,
		Sandbox:                         summary.Sandbox,
		Legacy:                          summary.Legacy,
		QuotaAffectedTasks:              summary.QuotaAffectedTasks,
		AuthAffectedTasks:               summary.AuthAffectedTasks,
		InfraAffectedTasks:              summary.InfraAffectedTasks,
		TotalQuotaRetries:               summary.TotalQuotaRetries,
		TotalInfraRetries:               summary.TotalInfraRetries,
		TotalSelfTestCommands:           summary.TotalSelfTestCommands,
		TotalToolchainInstallAttempts:   summary.TotalToolchainInstallAttempts,
		TotalOutOfWorkspaceReadAttempts: summary.TotalOutOfWorkspaceReadAttempts,
		SkillsUsageRate:                 summary.SkillsUsageRate,
		TotalSkillsUsageSignals:         summary.TotalSkillsUsageSignals,
		TasksWithSelfTesting:            summary.TasksWithSelfTesting,
		TasksWithToolchainInstall:       summary.TasksWithToolchainInstall,
		TasksWithOutOfWorkspaceReads:    summary.TasksWithOutOfWorkspaceReads,
		TasksWithSkillsUsage:            summary.TasksWithSkillsUsage,
		ByLanguage:                      make(map[string]LeaderboardLanguageStats),
	}

	// Add verification data from attestation
	if attestation != nil {
		submission.HarnessVersion = attestation.Harness.Version
		submission.WeightVersion = attestation.Harness.WeightVersion
		submission.TasksHash = attestation.Integrity.TasksHash
		submission.ResultsHash = attestation.Integrity.ResultsHash
	}

	// Convert language stats
	for lang, agg := range summary.ByLanguage {
		submission.ByLanguage[lang] = LeaderboardLanguageStats{
			Passed:   agg.Passed,
			Failed:   agg.Failed,
			Total:    agg.Total,
			PassRate: agg.PassRate,
		}
	}

	return submission
}

// generateEvalReport creates a human-readable Markdown report for the evaluation.
func generateEvalReport(summary EvalSummary, attestation *EvalAttestation) string {
	var sb strings.Builder

	sb.WriteString("# Evaluation Report\n\n")
	writeReportSummary(&sb, summary)
	writeReportQuality(&sb, summary)
	writeReportBehaviorTelemetry(&sb, summary)
	writeReportByLanguage(&sb, summary)
	writeReportByTier(&sb, summary)
	writeReportTaskResults(&sb, summary)
	writeReportErrors(&sb, summary)
	writeReportVerification(&sb, attestation)
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "*Generated by SanityHarness on %s*\n", summary.Timestamp)

	return sb.String()
}

func writeReportSummary(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	fmt.Fprintf(sb, "| Agent | **%s** |\n", summary.Agent)
	if summary.Model != "" {
		fmt.Fprintf(sb, "| Model | %s |\n", summary.Model)
	}
	if summary.Reasoning != "" {
		fmt.Fprintf(sb, "| Reasoning Effort | %s |\n", summary.Reasoning)
	}
	if summary.UseMCPTools {
		sb.WriteString("| MCP Tools Mode | Yes |\n")
	}
	if summary.UseSkills {
		sb.WriteString("| Skills Mode | Yes |\n")
		fmt.Fprintf(sb, "| Skills Usage Rate | %.1f%% (%d/%d) |\n", summary.SkillsUsageRate, summary.TasksWithSkillsUsage, summary.Total)
	}
	if summary.DisableMCP {
		sb.WriteString("| MCP Disabled | Yes |\n")
	}
	if summary.Sandbox {
		sb.WriteString("| Sandbox | Yes |\n")
	}
	if summary.Legacy {
		sb.WriteString("| Legacy Mode | Yes |\n")
	}
	fmt.Fprintf(sb, "| Timestamp | %s |\n", summary.Timestamp)
	fmt.Fprintf(sb, "| Pass Rate | **%.1f%%** (%d/%d) |\n", summary.PassRate, summary.Passed, summary.Total)
	fmt.Fprintf(sb, "| Weighted Pass Rate | **%.1f%%** |\n", summary.WeightedPassRate)
	fmt.Fprintf(sb, "| Weighted Score | %.2f / %.2f |\n", summary.WeightedScore, summary.MaxPossibleScore)
	fmt.Fprintf(sb, "| Duration | %.1fs |\n", summary.Duration)
	sb.WriteString("\n")
}

func writeReportQuality(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Quality Breakdown\n\n")
	fmt.Fprintf(sb, "- **Integrity Violations** (modified test files): %d\n", summary.IntegrityViolations)
	fmt.Fprintf(sb, "- **Failures**: %d\n", summary.Failed-summary.IntegrityViolations)
	fmt.Fprintf(sb, "- **Quota-affected tasks**: %d\n", summary.QuotaAffectedTasks)
	fmt.Fprintf(sb, "- **Auth-affected tasks**: %d\n", summary.AuthAffectedTasks)
	fmt.Fprintf(sb, "- **Infra-affected tasks**: %d\n", summary.InfraAffectedTasks)

	failureCounts := make(map[FailureClass]int)
	for _, r := range summary.Results {
		if r.FailureClass == FailureClassNone {
			continue
		}
		failureCounts[r.FailureClass]++
	}
	if len(failureCounts) > 0 {
		sb.WriteString("\n| Failure Class | Tasks |\n")
		sb.WriteString("|---------------|-------|\n")
		keys := make([]string, 0, len(failureCounts))
		for class := range failureCounts {
			keys = append(keys, string(class))
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(sb, "| %s | %d |\n", key, failureCounts[FailureClass(key)])
		}
	}
	sb.WriteString("\n")
}

func writeReportBehaviorTelemetry(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Behavior Telemetry\n\n")
	fmt.Fprintf(sb, "- **Total self-test commands**: %d\n", summary.TotalSelfTestCommands)
	fmt.Fprintf(sb, "- **Tasks with self-testing**: %d/%d\n", summary.TasksWithSelfTesting, summary.Total)
	fmt.Fprintf(sb, "- **Total toolchain install attempts**: %d\n", summary.TotalToolchainInstallAttempts)
	fmt.Fprintf(sb, "- **Tasks with toolchain install attempts**: %d/%d\n", summary.TasksWithToolchainInstall, summary.Total)
	fmt.Fprintf(sb, "- **Total out-of-workspace read attempts**: %d\n", summary.TotalOutOfWorkspaceReadAttempts)
	fmt.Fprintf(sb, "- **Tasks with out-of-workspace read attempts**: %d/%d\n", summary.TasksWithOutOfWorkspaceReads, summary.Total)
	fmt.Fprintf(sb, "- **Total Agent Skills usage signals**: %d\n", summary.TotalSkillsUsageSignals)
	fmt.Fprintf(sb, "- **Tasks with Agent Skills usage**: %d/%d (%.1f%%)\n", summary.TasksWithSkillsUsage, summary.Total, summary.SkillsUsageRate)

	hasTaskRows := false
	for _, r := range summary.Results {
		if r.SelfTestCommands > 0 || r.ToolchainInstallAttempts > 0 || r.OutOfWorkspaceReadAttempts > 0 || r.SkillsUsed {
			hasTaskRows = true
			break
		}
	}
	if !hasTaskRows {
		sb.WriteString("\n")
		return
	}

	sb.WriteString("\n| Task | Self Tests | Self Test Conf. | Tool Installs | Out-of-Workspace Reads | Out-of-Workspace Conf. | Skills Used | Skill Signals |\n")
	sb.WriteString("|------|------------|-----------------|---------------|-------------------------|------------------------|-------------|---------------|\n")
	for _, r := range summary.Results {
		if r.SelfTestCommands == 0 && r.ToolchainInstallAttempts == 0 && r.OutOfWorkspaceReadAttempts == 0 && !r.SkillsUsed {
			continue
		}
		fmt.Fprintf(
			sb,
			"| %s | %d | %t | %d | %d | %t | %t | %d |\n",
			r.Task,
			r.SelfTestCommands,
			r.SelfTestCommandsConfident,
			r.ToolchainInstallAttempts,
			r.OutOfWorkspaceReadAttempts,
			r.OutOfWorkspaceReadsConfident,
			r.SkillsUsed,
			r.SkillsUsageSignals,
		)
	}
	sb.WriteString("\n")
}

func writeReportByLanguage(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Results by Language\n\n")
	sb.WriteString("| Language | Passed | Failed | Total | Pass Rate |\n")
	sb.WriteString("|----------|--------|--------|-------|-----------|\n")
	languages := make([]string, 0, len(summary.ByLanguage))
	for lang := range summary.ByLanguage {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	for _, lang := range languages {
		agg := summary.ByLanguage[lang]
		fmt.Fprintf(sb, "| %s | %d | %d | %d | %.1f%% |\n",
			lang, agg.Passed, agg.Failed, agg.Total, agg.PassRate)
	}
	sb.WriteString("\n")
}

func writeReportByTier(sb *strings.Builder, summary EvalSummary) {
	if len(summary.ByTier) == 0 {
		return
	}
	sb.WriteString("## Results by Tier\n\n")
	sb.WriteString("| Tier | Passed | Failed | Total | Pass Rate |\n")
	sb.WriteString("|------|--------|--------|-------|-----------|\n")
	for _, tier := range []string{"core", "extended"} {
		if agg, ok := summary.ByTier[tier]; ok {
			fmt.Fprintf(sb, "| %s | %d | %d | %d | %.1f%% |\n",
				tier, agg.Passed, agg.Failed, agg.Total, agg.PassRate)
		}
	}
	sb.WriteString("\n")
}

func writeReportTaskResults(sb *strings.Builder, summary EvalSummary) {
	sb.WriteString("## Task Results\n\n")
	sb.WriteString("| Task | Status | Weight | Score | Duration |\n")
	sb.WriteString("|------|--------|--------|-------|----------|\n")
	for _, r := range summary.Results {
		statusIcon, status := getResultStatusDisplay(r)
		fmt.Fprintf(sb, "| %s | %s %s | %.2f | %.2f | %.1fs |\n",
			r.Task, statusIcon, status, r.Weight, r.WeightedScore, r.Duration)
	}
	sb.WriteString("\n")
}

func getResultStatusDisplay(r EvalResult) (icon, text string) {
	switch {
	case r.Status == task.StatusIntegrityViolation:
		return "🚫", "VIOLATION"
	case r.Passed:
		return "✅", "PASS"
	default:
		return "❌", "FAIL"
	}
}

func writeReportErrors(sb *strings.Builder, summary EvalSummary) {
	hasErrors := false
	for _, r := range summary.Results {
		if r.Error != "" {
			hasErrors = true
			break
		}
	}
	if !hasErrors {
		return
	}
	sb.WriteString("## Errors\n\n")
	for _, r := range summary.Results {
		if r.Error != "" {
			fmt.Fprintf(sb, "### %s\n\n", r.Task)
			fmt.Fprintf(sb, "```\n%s\n```\n\n", r.Error)
		}
	}
}

func writeReportVerification(sb *strings.Builder, attestation *EvalAttestation) {
	if attestation == nil {
		return
	}
	sb.WriteString("## Verification\n\n")
	fmt.Fprintf(sb, "- **Harness Version**: %s\n", attestation.Harness.Version)
	fmt.Fprintf(sb, "- **Weight Version**: %s\n", attestation.Harness.WeightVersion)
	fmt.Fprintf(sb, "- **Tasks Hash**: `%s`\n", attestation.Integrity.TasksHash)
	fmt.Fprintf(sb, "- **Results Hash**: `%s`\n", attestation.Integrity.ResultsHash)
	sb.WriteString("\n")
}

func parseAgentBehaviorMetrics(logPath, workspaceDir string) agentBehaviorMetrics {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return agentBehaviorMetrics{}
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	commands := extractCommandLines(lines)

	selfTests, selfConfident := countCommandMatches(commands, selfTestCommandPatterns)
	toolchainInstalls, toolchainConfident := countCommandMatches(commands, toolchainInstallPatterns)
	outReads, outReadsConfident := countOutOfWorkspaceReads(commands, workspaceDir)
	skillsSignals := countSkillUsageSignals(lines, commands)

	// Fallback to broad line matching when command extraction fails.
	if !selfConfident {
		selfTests = countMatchingLines(content, selfTestCommandPatterns)
	}
	if !outReadsConfident {
		outReads = countMatchingLines(content, outOfWorkspaceReadPatterns)
	}
	if !toolchainConfident {
		toolchainInstalls = countMatchingLines(content, toolchainInstallPatterns)
	}
	if !selfConfident && selfTests == 0 {
		selfConfident = true
	}
	if !outReadsConfident && outReads == 0 {
		outReadsConfident = true
	}

	return agentBehaviorMetrics{
		SelfTestCommands:             selfTests,
		SelfTestCommandsConfident:    selfConfident,
		ToolchainInstallAttempts:     toolchainInstalls,
		OutOfWorkspaceReads:          outReads,
		OutOfWorkspaceReadsConfident: outReadsConfident,
		SkillsUsed:                   skillsSignals > 0,
		SkillsUsageSignals:           skillsSignals,
	}
}

func countSkillUsageSignals(lines, commands []string) int {
	seen := make(map[string]struct{})

	record := func(text string) {
		for _, ref := range extractSkillArtifactRefs(text) {
			key := "artifact:" + strings.ToLower(ref)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}

		for _, match := range skillActivationPattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			skillName := strings.TrimSpace(match[1])
			if skillName == "" {
				continue
			}
			key := "activation:" + strings.ToLower(skillName)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}

		if firecrawlCommandPattern.MatchString(text) {
			cmd := strings.ToLower(strings.TrimSpace(text))
			key := "firecrawl:" + cmd
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
			}
		}
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(rawLine, ""))
		if line == "" {
			continue
		}
		record(line)
	}
	for _, cmd := range commands {
		record(cmd)
	}

	return len(seen)
}

func extractSkillArtifactRefs(text string) []string {
	matches := skillArtifactPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	refs := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		ref := strings.TrimSpace(match)
		ref = strings.Trim(ref, "\"'`")
		ref = strings.TrimRight(ref, ",.:;)")
		if ref == "" {
			continue
		}
		key := strings.ToLower(ref)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func extractCommandLines(lines []string) []string {
	commands := make([]string, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(rawLine, ""))
		if line == "" {
			continue
		}

		if matches := bashLCPattern.FindStringSubmatch(line); len(matches) == 2 {
			commands = append(commands, strings.TrimSpace(matches[1]))
			continue
		}

		// Supports shell-style logs such as "$ go test ./..." and decorated variants.
		if idx := strings.Index(line, "$ "); idx >= 0 {
			cmd := strings.TrimSpace(line[idx+2:])
			if cmd != "" {
				commands = append(commands, cmd)
			}
		}
	}
	return commands
}

func countCommandMatches(commands []string, patterns []*regexp.Regexp) (int, bool) {
	if len(commands) == 0 {
		return 0, false
	}
	count := 0
	for _, cmd := range commands {
		for _, re := range patterns {
			if re.MatchString(cmd) {
				count++
				break
			}
		}
	}
	return count, true
}

func countOutOfWorkspaceReads(commands []string, workspaceDir string) (int, bool) {
	if len(commands) == 0 {
		return 0, false
	}

	workspaceAbs := workspaceDir
	if workspaceAbs != "" {
		if abs, err := filepath.Abs(workspaceAbs); err == nil {
			workspaceAbs = canonicalizeExistingPath(abs)
		}
	}

	count := 0
	for _, cmd := range commands {
		paths := extractAbsolutePathsFromCommand(cmd)
		if len(paths) == 0 {
			continue
		}
		if commandReadsOutsideWorkspace(paths, workspaceAbs) {
			count++
		}
	}
	return count, true
}

func extractAbsolutePathsFromCommand(cmd string) []string {
	matches := absolutePathPattern.FindAllStringSubmatch(cmd, -1)
	if len(matches) == 0 {
		return nil
	}
	paths := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		p := strings.TrimRight(match[2], ",.:)")
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	return paths
}

func commandReadsOutsideWorkspace(paths []string, workspaceAbs string) bool {
	for _, p := range paths {
		if p == "/" {
			return true
		}
		if workspaceAbs == "" {
			return true
		}
		if p == workspaceAbs || strings.HasPrefix(p, workspaceAbs+string(os.PathSeparator)) {
			continue
		}

		// Whitelist skill directories from being penalized as out-of-workspace
		if strings.Contains(p, "/.agents/") || strings.Contains(p, "/.gemini/skills/") || strings.Contains(p, "/.opencode/skills/") || strings.Contains(p, "/.codex/skills/") || strings.Contains(p, "/.junie/skills/") || strings.Contains(p, "/.qwen/skills/") || strings.Contains(p, "/.kilocode/skills/") || strings.Contains(p, "/.factory/skills/") {
			continue
		}

		// Whitelist standard system executable paths
		if strings.HasPrefix(p, "/usr/bin/") || strings.HasPrefix(p, "/usr/local/bin/") || strings.HasPrefix(p, "/opt/") || strings.HasPrefix(p, "/usr/lib/") {
			continue
		}

		return true
	}
	return false
}

func countMatchingLines(content string, patterns []*regexp.Regexp) int {
	if content == "" {
		return 0
	}
	lines := strings.Split(content, "\n")
	count := 0
	for _, line := range lines {
		for _, re := range patterns {
			if re.MatchString(line) {
				count++
				break
			}
		}
	}
	return count
}

func resolveSandboxDenylistPaths(configured []string, outputDir string) []string {
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil
	}
	repoRoot = canonicalizeExistingPath(repoRoot)

	candidates := []string{
		filepath.Join(repoRoot, "tasks"),
		filepath.Join(repoRoot, "eval-results"),
		filepath.Join(repoRoot, "sessions"),
	}
	if outputDir != "" {
		if filepath.IsAbs(outputDir) {
			candidates = append(candidates, outputDir)
		} else {
			candidates = append(candidates, filepath.Join(repoRoot, outputDir))
		}
	}
	for _, path := range configured {
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			candidates = append(candidates, path)
			continue
		}
		candidates = append(candidates, filepath.Join(repoRoot, path))
	}

	seen := make(map[string]struct{}, len(candidates))
	denylist := make([]string, 0, len(candidates))
	for _, path := range candidates {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		absPath = canonicalizeExistingPath(absPath)
		if _, exists := seen[absPath]; exists {
			continue
		}
		seen[absPath] = struct{}{}
		denylist = append(denylist, absPath)
	}
	return denylist
}

func canonicalizeExistingPath(path string) string {
	if path == "" {
		return path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

// detectAuthError checks if agent log contains auth/authz errors.
func detectAuthError(logPath string) bool {
	content, err := os.ReadFile(logPath)
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(content))
	for _, pattern := range authFailurePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// detectQuotaError checks if agent log contains rate limit or quota errors.
// Returns (hasError, isRecoverable) where hasError indicates if any quota/rate
// limit pattern was found, and isRecoverable indicates if the error is transient.
func detectQuotaError(logPath string) (bool, bool) {
	content, err := os.ReadFile(logPath)
	if err != nil {
		return false, false
	}

	lower := strings.ToLower(string(content))

	// Check for non-recoverable patterns first
	for _, pattern := range nonRecoverableQuotaPatterns {
		if strings.Contains(lower, pattern) {
			return true, false // Error found, NOT recoverable
		}
	}

	// Check for recoverable patterns
	for _, pattern := range recoverablePatterns {
		if strings.Contains(lower, pattern) {
			return true, true // Error found, IS recoverable
		}
	}

	return false, false
}

// getRetryDelay returns the delay for the given quota retry attempt (1-indexed).
func getRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return quotaRetryDelay1
	case 2:
		return quotaRetryDelay2
	case 3:
		return quotaRetryDelay3
	case 4:
		return quotaRetryDelay4
	default:
		return quotaRetryDelay5
	}
}

// getInfraRetryDelay returns the delay for the given infra retry attempt (1-indexed).
func getInfraRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return infraRetryDelay1
	case 2:
		return infraRetryDelay2
	case 3:
		return infraRetryDelay3
	case 4:
		return infraRetryDelay4
	default:
		return infraRetryDelay5
	}
}

// isInfraFailure checks if the agent log indicates an infrastructure failure
// (empty or near-empty output suggesting the provider never responded).
// It strips retry separator lines and whitespace to avoid false negatives
// when the log contains only retry markers but no actual agent output.
// If workspaceDir is non-empty, it also checks whether the agent modified any
// files in the workspace — agents that write to files but produce no stdout
// (e.g. droid, cline) are NOT infra failures.
func isInfraFailure(logPath, workspaceDir string, workspaceReadyAt time.Time) bool {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return true // No log file at all is an infra failure
	}

	// Strip retry separator lines (e.g., "=== RETRY 1 (after 30s delay) ===")
	// and whitespace to check if there's any real agent output.
	var meaningful []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("=== RETRY ")) && bytes.HasSuffix(trimmed, []byte("===")) {
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("HARNESS: agent timed out")) {
			continue
		}
		meaningful = append(meaningful, trimmed...)
	}

	if len(meaningful) >= infraFailureLogThreshold {
		return false // Agent produced meaningful output
	}

	// Agent produced no/minimal stdout. Check if it modified workspace files.
	// Some agents (droid, cline) legitimately produce no stdout but write files.
	if workspaceDir != "" && hasModifiedFiles(workspaceDir, workspaceReadyAt) {
		return false // Agent wrote files — not an infra failure
	}

	return true
}

// hasModifiedFiles checks if any files in the workspace directory were modified
// after the given cutoff time. This detects agents that write to files but
// produce no stdout output, while ignoring files written by the harness during
// workspace setup.
func hasModifiedFiles(dir string, cutoff time.Time) bool {
	found := false

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip agent.log — it is created by the harness (not the agent) inside
		// the workspace dir, so its presence does not indicate agent activity.
		if filepath.Base(path) == "agent.log" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})

	return found
}

// saveRunConfig saves the eval configuration for resume capability.
func saveRunConfig(outputDir string, allTasks []*task.Task) error {
	taskList := make([]string, len(allTasks))
	for i, t := range allTasks {
		taskList[i] = string(t.Language) + "/" + t.Slug
	}

	runCfg := RunConfig{
		Agent:          evalAgent,
		Model:          evalModel,
		Reasoning:      evalReasoning,
		Tier:           evalTier,
		Difficulty:     evalDifficulty,
		Lang:           evalLang,
		Tasks:          evalTasks,
		Timeout:        evalTimeout,
		Parallel:       evalParallel,
		UseMCPTools:    evalUseMCPTools,
		UseSkills:      evalUseSkills,
		DisableMCP:     evalDisableMCP,
		NoSandbox:      evalNoSandbox,
		Legacy:         evalLegacy,
		KeepWorkspaces: evalKeepWorkspaces,
		TaskList:       taskList,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(runCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling run config: %w", err)
	}

	return os.WriteFile(filepath.Join(outputDir, "run-config.json"), data, 0o644)
}

// loadRunConfig loads the eval configuration from a resume directory.
func loadRunConfig(resumeDir string) (*RunConfig, error) {
	data, err := os.ReadFile(filepath.Join(resumeDir, "run-config.json"))
	if err != nil {
		return nil, fmt.Errorf("reading run config: %w", err)
	}

	var runCfg RunConfig
	if err := json.Unmarshal(data, &runCfg); err != nil {
		return nil, fmt.Errorf("parsing run config: %w", err)
	}

	return &runCfg, nil
}

// applyRunConfig applies the loaded run config to global eval variables.
func applyRunConfig(runCfg *RunConfig) {
	evalAgent = runCfg.Agent
	evalModel = runCfg.Model
	evalReasoning = runCfg.Reasoning
	evalTier = runCfg.Tier
	evalDifficulty = runCfg.Difficulty
	evalLang = runCfg.Lang
	evalTasks = runCfg.Tasks
	evalTimeout = runCfg.Timeout
	evalParallel = runCfg.Parallel
	evalUseMCPTools = runCfg.UseMCPTools
	evalUseSkills = runCfg.UseSkills
	evalDisableMCP = runCfg.DisableMCP
	evalNoSandbox = runCfg.NoSandbox
	evalLegacy = runCfg.Legacy
	evalKeepWorkspaces = runCfg.KeepWorkspaces
}

// findCompletedTasks returns a set of task slugs that have validation.log files.
func findCompletedTasks(outputDir string) (map[string]bool, error) {
	completed := make(map[string]bool)

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("reading output directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if validation.log exists in this task directory.
		validationLog := filepath.Join(outputDir, entry.Name(), "validation.log")
		if _, err := os.Stat(validationLog); err == nil {
			// Directory name format is "language-slug", convert to "language/slug".
			name := entry.Name()
			if idx := strings.Index(name, "-"); idx > 0 {
				taskSlug := name[:idx] + "/" + name[idx+1:]
				completed[taskSlug] = true
			}
		}
	}

	return completed, nil
}

// loadPreviousSummary loads results from a previous eval run for merging.
func loadPreviousSummary(outputDir string) (*EvalSummary, error) {
	summaryPath := filepath.Join(outputDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No previous results.
		}
		return nil, fmt.Errorf("reading summary: %w", err)
	}

	var summary EvalSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("parsing summary: %w", err)
	}

	return &summary, nil
}

// loadPreviousAttestation loads attestation from a previous eval run.
func loadPreviousAttestation(outputDir string) (*EvalAttestation, error) {
	attestationPath := filepath.Join(outputDir, "attestation.json")
	data, err := os.ReadFile(attestationPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No previous attestation.
		}
		return nil, fmt.Errorf("reading attestation: %w", err)
	}

	var attestation EvalAttestation
	if err := json.Unmarshal(data, &attestation); err != nil {
		return nil, fmt.Errorf("parsing attestation: %w", err)
	}

	return &attestation, nil
}

// prepareResumedTasks restores task order from the run config, cleans incomplete
// directories, and filters out already-completed tasks for a resumed eval run.
func prepareResumedTasks(
	allTasks []*task.Task,
	runCfg *RunConfig,
	outputDir string,
	completedTasks map[string]bool,
) ([]*task.Task, []*task.Task, error) {
	// Build task map for ordering from run config.
	taskMap := make(map[string]*task.Task)
	for _, t := range allTasks {
		taskMap[string(t.Language)+"/"+t.Slug] = t
	}

	// Restore original task order from run config.
	var orderedTasks []*task.Task
	var missingTasks []string
	for _, slug := range runCfg.TaskList {
		if t, ok := taskMap[slug]; ok {
			orderedTasks = append(orderedTasks, t)
		} else {
			missingTasks = append(missingTasks, slug)
		}
	}
	if len(missingTasks) > 0 {
		logger.Warn("some tasks from original run not found in current build",
			"missing", missingTasks,
			"count", len(missingTasks))
		fmt.Printf(" Warning: %d task(s) from original run not found: %v\n",
			len(missingTasks), missingTasks)
	}

	// Clean up incomplete task directories.
	if err := cleanIncompleteTaskDirs(outputDir, completedTasks, orderedTasks); err != nil {
		return nil, nil, fmt.Errorf("cleaning incomplete tasks: %w", err)
	}

	// Filter out completed tasks.
	var tasksToRun []*task.Task
	for _, t := range orderedTasks {
		taskSlug := string(t.Language) + "/" + t.Slug
		if !completedTasks[taskSlug] {
			tasksToRun = append(tasksToRun, t)
		}
	}

	return orderedTasks, tasksToRun, nil
}

// cleanIncompleteTaskDirs removes task directories that don't have validation.log.
func cleanIncompleteTaskDirs(outputDir string, completed map[string]bool, allTasks []*task.Task) error {
	for _, t := range allTasks {
		taskSlug := string(t.Language) + "/" + t.Slug
		if completed[taskSlug] {
			continue
		}

		// Task is incomplete, remove its directory if it exists.
		taskDir := filepath.Join(outputDir, string(t.Language)+"-"+t.Slug)
		if _, err := os.Stat(taskDir); err == nil {
			if err := os.RemoveAll(taskDir); err != nil {
				return fmt.Errorf("removing incomplete task dir %s: %w", taskDir, err)
			}
		}
	}

	return nil
}

// setupInterruptHandler creates a context that is cancelled on interrupt signals.
// The returned cancel function should be deferred to clean up signal handling.
func setupInterruptHandler() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()
	return ctx, cancel
}

// checkInterrupted checks if an interrupt signal has been received.
func checkInterrupted(ctx context.Context) bool {
	return ctx.Err() != nil
}

// printResumeCommand prints the command to resume an interrupted eval.
func printResumeCommand(outputDir string) {
	fmt.Printf("\n\033[33m⚠ Evaluation interrupted. To resume, run:\033[0m\n")
	fmt.Printf("  ./sanity eval --resume %s\n\n", outputDir)
}

// initSandbox checks if bubblewrap sandboxing should be enabled.
// Returns true if sandbox is active (bwrap found and not disabled).
func initSandbox() bool {
	if evalNoSandbox {
		logger.Info("sandbox disabled via --no-sandbox")
		return false
	}

	if _, err := exec.LookPath("bwrap"); err != nil {
		logger.Warn("bubblewrap (bwrap) not found, running agents without sandbox")
		return false
	}

	return true
}

// protectTasksDir makes the tasks/ directory read-only to prevent agents from
// modifying embedded task source files during evaluation. Returns a restore
// function that re-enables write permissions, or nil if protection was not needed.
func protectTasksDir() (restore func(), err error) {
	tasksPath := "tasks"
	info, err := os.Stat(tasksPath)
	if err != nil || !info.IsDir() {
		return nil, nil // No on-disk tasks/ directory; nothing to protect
	}

	absTasksPath, err := filepath.Abs(tasksPath)
	if err != nil {
		return nil, fmt.Errorf("resolving tasks path: %w", err)
	}

	// Collect original permissions so we can restore them exactly.
	type entry struct {
		path string
		mode fs.FileMode
	}
	var origPerms []entry

	err = filepath.WalkDir(absTasksPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		origPerms = append(origPerms, entry{path: path, mode: info.Mode()})

		// Remove write bits for user, group, and other.
		newMode := info.Mode() &^ 0222
		if newMode != info.Mode() {
			return os.Chmod(path, newMode)
		}
		return nil
	})
	if err != nil {
		// Best-effort: try to restore anything we already changed.
		for _, e := range origPerms {
			_ = os.Chmod(e.path, e.mode)
		}
		return nil, fmt.Errorf("protecting tasks directory: %w", err)
	}

	restore = func() {
		// Restore in reverse order so directories are restored after their contents.
		for i := len(origPerms) - 1; i >= 0; i-- {
			_ = os.Chmod(origPerms[i].path, origPerms[i].mode)
		}
	}
	return restore, nil
}

func init() {
	evalCmd.Flags().StringVar(&evalAgent, "agent", "", "agent to evaluate (see --help for list)")
	evalCmd.Flags().StringVar(&evalModel, "model", "", "model to use (e.g., gemini-2.5-pro or google/gemini-2.5-flash)")
	evalCmd.Flags().StringVar(&evalReasoning, "reasoning", "", "reasoning effort level (e.g., off, none, low, medium, high)")
	evalCmd.Flags().StringVar(&evalTasks, "tasks", "", "comma-separated list of task slugs")
	evalCmd.Flags().StringVar(&evalLang, "lang", "", "filter by language (go, rust, typescript)")
	evalCmd.Flags().StringVar(&evalTier, "tier", "core", "filter by tier (core, extended, all)")
	evalCmd.Flags().StringVar(&evalDifficulty, "difficulty", "", "filter by difficulty (comma-separated)")
	evalCmd.Flags().IntVar(&evalTimeout, "timeout", 0, "timeout per task in seconds (default from config)")
	evalCmd.Flags().IntVar(&evalParallel, "parallel", 1, "run up to N tasks in parallel")
	evalCmd.Flags().StringVar(&evalOutputDir, "output", "", "output directory for results")
	evalCmd.Flags().BoolVar(&evalKeepWorkspaces, "keep-workspaces", false, "keep workspace directories after evaluation")
	evalCmd.Flags().BoolVar(&evalDryRun, "dry-run", false, "show what tasks would be run without executing")
	evalCmd.Flags().BoolVar(&evalUseMCPTools, "use-mcp-tools", false, "inject MCP tool usage instructions into agent prompt")
	evalCmd.Flags().BoolVar(&evalUseSkills, "use-skills", false, "inject Agent Skills usage instructions into agent prompt")
	evalCmd.Flags().BoolVar(&evalDisableMCP, "disable-mcp", false, "disable MCP tools for agents that support it (currently: opencode)")
	evalCmd.Flags().BoolVar(&evalNoSandbox, "no-sandbox", false, "disable bubblewrap sandbox for agent processes")
	evalCmd.Flags().BoolVar(&evalLegacy, "legacy", false, "expose hidden tests to agent during workspace init (pre-v1.6.0 behavior)")
	evalCmd.Flags().StringVar(&evalResume, "resume", "", "resume eval from existing output directory")
	evalCmd.Flags().IntVar(&evalRepeat, "repeat", 1, "repeat each configuration N times for statistical analysis")
}
