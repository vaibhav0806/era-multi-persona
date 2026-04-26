# M7-A.5 — Swarm Runner Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `era-brain` into era's existing `/task` pipeline so every real task runs through a planner-Pi-reviewer swarm. Planner produces a plan from the task description (LLM call); plan is injected into Pi's task context; Pi runs as the *coder* persona inside its existing Docker sandbox (unchanged); reviewer (LLM call) consumes Pi's diff via the GitHub compare API and produces a critique + approve/flag decision. All three persona receipts surface in the completion DM and land in the era-brain audit log.

**Architecture: Option 2 — orchestrator-side swarm.** Spec §3 says "container hosts the swarm"; we deviate. The orchestrator process owns the planner and reviewer LLM calls; the container still runs Pi unchanged. Pi *is* the coder persona's tool-loop engine. Three reasons:

1. No runner Docker image rebuild — saves ~hour of image churn per phase.
2. Planner & reviewer don't need network sandboxing (LLM calls only, no exfil risk).
3. Reviewer naturally consumes the GitHub compare diff (already wired in M3 via `internal/githubcompare`).

Future M7-C (0G Compute sealed inference) works identically — sealed inference is just an HTTP endpoint the LLM provider hits; orchestrator-side calls work fine.

**Tech Stack:** Go 1.25, era-brain SDK (now living at `era-brain/` in the same repo as a separate Go module), modernc.org/sqlite (already era's), existing Telegram + GitHub App + diff-scan plumbing.

**Spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` §3 — with the orchestrator-side-swarm deviation explicitly approved by user.

**Testing philosophy:** Strict TDD. Failing test → run, verify FAIL → minimal impl → run, verify PASS → commit. `go test -race -count=1 ./...` from repo root green at every commit. Live Telegram gate at the end. Subagent-driven execution.

**Prerequisites (check before starting):**
- M7-A complete (tag `m7a-done` exists at commit `a75190c`).
- `era-brain/examples/coding-agent` runs successfully against real OpenRouter (Task 6 live gate passed).
- `.env` at repo root has `PI_OPENROUTER_API_KEY` populated.
- Existing era M6 tests at repo root pass: `go test -race ./...`.
- Hetzner VPS deployment from M5 still working (we won't touch deploy in this milestone, but the live gate runs against the VPS).

---

## File Structure

```
go.mod                                          MODIFY (Task 1) — require + replace era-brain
go.sum                                          MODIFY (Task 1)

internal/swarm/                                 CREATE (Task 2)
├── swarm.go                                    CREATE — Swarm struct + Plan + Review methods
├── swarm_test.go                               CREATE — TDD coverage with fake LLM
├── personas.go                                 CREATE — system prompts (planner/reviewer)
└── inject.go                                   CREATE — InjectPlan helper for runner task description

cmd/orchestrator/
├── main.go                                     MODIFY (Task 3, 5) — wire swarm + extend tgNotifier
└── (existing)                                  unchanged elsewhere

internal/queue/
├── queue.go                                    MODIFY (Task 4) — Queue.swarm field + RunNext changes; CompletedArgs struct
└── queue_run_test.go                           MODIFY (Task 4) — verify RunNext threads planner/reviewer

internal/runner/
└── (none)                                      unchanged — Pi flow stays exactly the same
```

**Important:** the existing era binary's module path stays `github.com/vaibhav0806/era`. We do NOT rename to `github.com/vaibhav0806/era-multi-persona` in this milestone — that's a churn-heavy refactor and yields zero hackathon value. The era-brain dep is added via `require` + local `replace` directive.

---

## Task 1: Wire era-brain dependency into era's go.mod

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

The era-brain Go module lives at `era-brain/` inside this same repo. We add it as a versioned dependency (`v0.0.0-unpublished`) with a local `replace` directive pointing at `./era-brain`. This is the standard Go monorepo pattern.

- [ ] **Step 1.1: Add the dependency**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go mod edit -require=github.com/vaibhav0806/era-multi-persona/era-brain@v0.0.0-unpublished
go mod edit -replace=github.com/vaibhav0806/era-multi-persona/era-brain=./era-brain
```

- [ ] **Step 1.2: Add a smoke import to verify the wiring**

Temporarily add a blank import in `cmd/orchestrator/main.go` (top of imports):

```go
import (
    _ "github.com/vaibhav0806/era-multi-persona/era-brain/brain"
    // ... existing imports
)
```

Then run:

```bash
go mod tidy
go build ./...
```

Expected: `go.sum` populated with era-brain (and its transitive deps via the replace). `go build` exits 0.

If you see `cannot find module providing package github.com/vaibhav0806/era-multi-persona/era-brain/brain`, the replace directive isn't pointed at the right relative path. Verify `replace` line in `go.mod` is exactly `replace github.com/vaibhav0806/era-multi-persona/era-brain => ./era-brain`.

- [ ] **Step 1.3: Remove the smoke import** (Task 2 will add real imports)

Revert the blank import added in 1.2. Just `go.mod` + `go.sum` should remain changed.

- [ ] **Step 1.4: Verify era M6 tests still pass**

```bash
go test -race ./...
```

Expected: green. era-brain dep is declared but no era code imports it yet, so behavior is identical to before.

- [ ] **Step 1.5: Commit**

```bash
git add go.mod go.sum
git commit -m "phase(M7-A.5.1): require era-brain via local replace directive"
git tag m7a5-1-deps
```

---

## Task 2: New `internal/swarm/` package

**Files:**
- Create: `internal/swarm/swarm.go`
- Create: `internal/swarm/swarm_test.go`
- Create: `internal/swarm/personas.go`
- Create: `internal/swarm/inject.go`

The swarm package wraps `era-brain.brain.LLMPersona` with era-specific glue: planner and reviewer system prompts, plan-injection into Pi's task description, diff-scan output piping into the reviewer. It exposes two methods used by the queue:

- `Swarm.Plan(ctx, taskDesc) (PlanResult, error)` — runs planner LLMPersona; returns the plan text + receipt.
- `Swarm.Review(ctx, taskDesc, planText, diffText, diffScanFindings) (ReviewResult, error)` — runs reviewer LLMPersona; returns critique + decision (`approve` / `flag`) + receipt.

We do NOT use `brain.Brain.Run([planner, coder, reviewer])` here because the coder isn't an LLMPersona — it's Pi-in-Docker, called via the existing `runner.Run`. The orchestration sequence is queue-level, not brain-level. brain.Brain comes back into play in M7-D when reviewer needs to see all 3 receipts on 0G Log.

### 2A: Swarm types + Plan method

- [ ] **Step 2.1: Write the failing test for Swarm.Plan**

`internal/swarm/swarm_test.go`:
```go
package swarm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era/internal/swarm"
)

type fakeLLM struct {
	resp string
}

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	return llm.Response{Text: f.resp + "(sys=" + req.SystemPrompt[:min(len(req.SystemPrompt), 20)] + ")", Model: "test-m", Sealed: false}, nil
}

func min(a, b int) int { if a < b { return a }; return b }

func TestSwarm_Plan_ProducesPlanWithReceipt(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "1. step one\n2. step two\n3. step three"}
	reviewerLLM := &fakeLLM{resp: "no issues found\nDECISION: approve"}
	s := swarm.New(swarm.Config{
		PlannerLLM:  plannerLLM,
		ReviewerLLM: reviewerLLM,
	})

	res, err := s.Plan(context.Background(), swarm.PlanArgs{
		TaskID:          "t1",
		TaskDescription: "add JWT auth",
	})
	require.NoError(t, err)
	require.Contains(t, res.PlanText, "step one")
	require.Equal(t, "planner", res.Receipt.Persona)
	require.Equal(t, "test-m", res.Receipt.Model)
	require.False(t, res.Receipt.Sealed)
	require.NotEmpty(t, res.Receipt.InputHash)
}
```

- [ ] **Step 2.2: Run, verify FAIL**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go test ./internal/swarm/...
```

Expected: package not found.

- [ ] **Step 2.3: Implement Swarm + Plan**

`internal/swarm/swarm.go`:
```go
// Package swarm wraps era-brain.LLMPersona with era-specific glue:
// planner runs before Pi, reviewer runs after Pi sees the diff. Pi itself
// is the coder persona's tool-loop engine and is not part of this package.
package swarm

import (
	"context"
	"fmt"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// Config configures a Swarm.
type Config struct {
	PlannerLLM  llm.Provider
	ReviewerLLM llm.Provider
	Memory      memory.Provider // optional; when set, receipts append to audit log
	Now         func() time.Time
}

// Swarm orchestrates planner + reviewer LLM calls. Coder is Pi-in-Docker,
// invoked by the queue between Plan and Review.
type Swarm struct {
	planner  *brain.LLMPersona
	reviewer *brain.LLMPersona
}

// New constructs a Swarm with the planner and reviewer personas wired up.
func New(cfg Config) *Swarm {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Swarm{
		planner: brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "planner",
			SystemPrompt: PlannerSystemPrompt,
			LLM:          cfg.PlannerLLM,
			Memory:       cfg.Memory,
			Now:          cfg.Now,
		}),
		reviewer: brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "reviewer",
			SystemPrompt: ReviewerSystemPrompt,
			LLM:          cfg.ReviewerLLM,
			Memory:       cfg.Memory,
			Now:          cfg.Now,
		}),
	}
}

// PlanArgs is the input to Plan.
type PlanArgs struct {
	TaskID          string
	UserID          string
	TaskDescription string
}

// PlanResult is what Plan returns.
type PlanResult struct {
	PlanText string
	Receipt  brain.Receipt
}

// Plan runs the planner persona and returns the plan text plus receipt.
func (s *Swarm) Plan(ctx context.Context, args PlanArgs) (PlanResult, error) {
	out, err := s.planner.Run(ctx, brain.Input{
		TaskID:          args.TaskID,
		UserID:          args.UserID,
		TaskDescription: args.TaskDescription,
	})
	if err != nil {
		return PlanResult{}, fmt.Errorf("swarm.Plan: %w", err)
	}
	return PlanResult{PlanText: out.Text, Receipt: out.Receipt}, nil
}
```

- [ ] **Step 2.4: Add the planner system prompt**

`internal/swarm/personas.go`:
```go
package swarm

// PlannerSystemPrompt is the planner persona's system prompt.
// Adapted from era-brain/examples/coding-agent — shaped for era's actual
// task descriptions (which target real GitHub repos, not synthetic tasks).
const PlannerSystemPrompt = `You are the PLANNER persona for era, an autonomous coding agent.

Given a coding task, produce a numbered step list (3-7 steps) describing what code changes are needed. The CODER persona will execute your plan against a real Git repository — it has read/write/edit/run tool access and will figure out exact file paths from the repo state. Your job is to give the coder clear intent, ordering, and acceptance criteria.

Be specific about behaviors and likely files (e.g. "add a /healthz handler in the existing HTTP router", not "fix the server"). Do not write code yet. Output ONLY the numbered list — no preamble, no postscript.`

// ReviewerSystemPrompt is the reviewer persona's system prompt.
// Reviewer sees: original task description, planner's plan, the unified diff
// produced by the coder (Pi), and the diff-scan finding list (rule names + files).
const ReviewerSystemPrompt = `You are the REVIEWER persona for era, an autonomous coding agent.

You will see (1) the original task description, (2) the planner's step list, (3) the coder's actual unified diff, and (4) any diff-scan findings (e.g. "removed_test", "skip_directive", "weakened_assertion"). Critique the diff against the plan and the task. Flag:

(a) deviations from the plan
(b) test removals, skips, or weakened assertions
(c) anything that looks like it would not compile, run, or pass tests
(d) any diff-scan finding that the coder did not justify

End your output with EXACTLY one line: "DECISION: approve" or "DECISION: flag". Use "approve" only if you would land this diff yourself; use "flag" if a human should look before merging.`
```

- [ ] **Step 2.5: Run, verify PASS**

```bash
go test -race ./internal/swarm/...
```

Expected: 1 PASS (Plan test).

### 2B: Review method

- [ ] **Step 2.6: Write the failing test for Swarm.Review**

Append to `internal/swarm/swarm_test.go`:
```go
func TestSwarm_Review_DecisionApprove(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "no issues found\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	res, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:          "t1",
		TaskDescription: "task",
		PlanText:        "1. step",
		DiffText:        "diff --git a/x b/x\n+hello",
	})
	require.NoError(t, err)
	require.Equal(t, swarm.DecisionApprove, res.Decision)
	require.Contains(t, strings.ToLower(res.CritiqueText), "no issues")
	require.Equal(t, "reviewer", res.Receipt.Persona)
}

func TestSwarm_Review_DecisionFlag(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "the diff removes a test\nDECISION: flag"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	res, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:   "t1",
		PlanText: "p",
		DiffText: "d",
	})
	require.NoError(t, err)
	require.Equal(t, swarm.DecisionFlag, res.Decision)
}

func TestSwarm_Review_NoExplicitDecisionDefaultsToFlag(t *testing.T) {
	// Defensive: if reviewer omits the DECISION line, treat as flag (don't auto-approve).
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "looks fine I guess"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	res, err := s.Review(context.Background(), swarm.ReviewArgs{TaskID: "t1", PlanText: "p", DiffText: "d"})
	require.NoError(t, err)
	require.Equal(t, swarm.DecisionFlag, res.Decision)
}
```

- [ ] **Step 2.7: Run, verify FAIL**

```bash
go test ./internal/swarm/...
```

Expected: `undefined: swarm.DecisionApprove`, `undefined: ReviewArgs`.

- [ ] **Step 2.8: Implement Review**

Append to `internal/swarm/swarm.go`:
```go
// Decision is the reviewer persona's verdict on the diff.
type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionFlag    Decision = "flag"
)

// ReviewArgs is the input to Review.
type ReviewArgs struct {
	TaskID            string
	UserID            string
	TaskDescription   string
	PlanText          string
	DiffText          string
	DiffScanFindings  []string // human-readable rule names; e.g. ["removed_test (foo_test.go)"]
}

// ReviewResult is what Review returns.
type ReviewResult struct {
	CritiqueText string
	Decision     Decision
	Receipt      brain.Receipt
}

// Review runs the reviewer persona on the coder's diff. Returns the critique,
// decision (approve | flag — flag is the safe default), and receipt.
func (s *Swarm) Review(ctx context.Context, args ReviewArgs) (ReviewResult, error) {
	out, err := s.reviewer.Run(ctx, brain.Input{
		TaskID:          args.TaskID,
		UserID:          args.UserID,
		TaskDescription: args.TaskDescription,
		PriorOutputs: []brain.Output{
			{PersonaName: "planner", Text: args.PlanText},
			{PersonaName: "coder", Text: composeCoderOutput(args.DiffText, args.DiffScanFindings)},
		},
	})
	if err != nil {
		return ReviewResult{}, fmt.Errorf("swarm.Review: %w", err)
	}
	return ReviewResult{
		CritiqueText: out.Text,
		Decision:     parseDecision(out.Text),
		Receipt:      out.Receipt,
	}, nil
}

func composeCoderOutput(diff string, findings []string) string {
	out := "Diff:\n" + diff
	if len(findings) > 0 {
		out += "\n\nDiff-scan findings:\n"
		for _, f := range findings {
			out += "- " + f + "\n"
		}
	}
	return out
}

func parseDecision(text string) Decision {
	// Look for the literal "DECISION: approve" line. Anything else (including
	// "DECISION: flag" or no decision line at all) maps to flag — safe default.
	for _, line := range splitLines(text) {
		l := trimLower(line)
		if l == "decision: approve" {
			return DecisionApprove
		}
	}
	return DecisionFlag
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func trimLower(s string) string {
	out := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r = r + 32
		}
		out += string(r)
	}
	for len(out) > 0 && (out[0] == ' ' || out[0] == '\t' || out[0] == '\r') {
		out = out[1:]
	}
	for len(out) > 0 && (out[len(out)-1] == ' ' || out[len(out)-1] == '\t' || out[len(out)-1] == '\r') {
		out = out[:len(out)-1]
	}
	return out
}
```

(The verbose `splitLines` / `trimLower` are stdlib-light to avoid adding `strings` to the imports if you'd prefer; but feel free to use `strings.Split` and `strings.ToLower` + `strings.TrimSpace` if you find that cleaner — match era's existing style.)

- [ ] **Step 2.9: Run, verify PASS**

```bash
go test -race ./internal/swarm/...
```

Expected: 4 PASS (Plan + 3 Review variants).

### 2C: InjectPlan helper

- [ ] **Step 2.10: Write the failing test**

Append to `swarm_test.go`:
```go
func TestSwarm_InjectPlan_PrependsPlanToTaskDescription(t *testing.T) {
	out := swarm.InjectPlan("add JWT auth", "1. add middleware\n2. add login")
	require.Contains(t, out, "add JWT auth")
	require.Contains(t, out, "1. add middleware")
	// Plan should appear AFTER the task description (so the task is the lede; plan is context).
	taskIdx := strings.Index(out, "add JWT auth")
	planIdx := strings.Index(out, "1. add middleware")
	require.True(t, taskIdx < planIdx, "task should appear before plan")
}
```

- [ ] **Step 2.11: Run, verify FAIL**

```bash
go test ./internal/swarm/...
```

Expected: `undefined: swarm.InjectPlan`.

- [ ] **Step 2.12: Implement InjectPlan**

`internal/swarm/inject.go`:
```go
package swarm

// InjectPlan composes the task description Pi sees: the original user-facing
// task first, followed by the planner's step list as additional context.
//
// Pi reads ERA_TASK_DESCRIPTION verbatim — there is no separate "plan" env
// var. We thread the plan through by appending it to the description.
func InjectPlan(taskDesc, planText string) string {
	if planText == "" {
		return taskDesc
	}
	return taskDesc + "\n\n--- Planner step list (from the planner persona; treat as guidance, not literal commands) ---\n" + planText
}
```

- [ ] **Step 2.13: Run, verify PASS**

```bash
go test -race ./internal/swarm/...
```

Expected: 5 PASS.

### 2D: Sanity sweep

- [ ] **Step 2.14: Run all era tests + vet**

```bash
go vet ./... && go test -race ./...
```

Expected: all green from repo root.

- [ ] **Step 2.15: Commit**

```bash
git add internal/swarm/
git commit -m "phase(M7-A.5.2): internal/swarm package — Plan, Review, InjectPlan"
git tag m7a5-2-swarm
```

---

## Task 3: Wire OpenRouter LLMProvider + memory provider into orchestrator

**Files:**
- Modify: `cmd/orchestrator/main.go`

The orchestrator currently constructs Pi/runner/queue/etc. We add:
1. An `era-brain.openrouter.Provider` for the planner+reviewer LLM calls (uses the same `PI_OPENROUTER_API_KEY` Pi already uses).
2. An `era-brain.memory.sqlite.Provider` for persona audit log (uses a separate `data/era-brain.db` so it doesn't collide with era's main DB schema).
3. A `swarm.Swarm` constructed with the above.
4. `Queue.SetSwarm(swarm)` after queue construction.

We pick the `openai/gpt-4o-mini` default model — same as the M7-A live gate. Model is overridable via `PI_BRAIN_PLANNER_MODEL` and `PI_BRAIN_REVIEWER_MODEL` env vars (both default to `openai/gpt-4o-mini`).

- [ ] **Step 3.1: Read `cmd/orchestrator/main.go` to find the right insertion points**

Read the file. Find:
- Where Pi config is loaded from env (look for `PI_OPENROUTER_API_KEY`).
- Where `queue.New` is called.
- Where the orchestrator's main loop (`queue.RunNext`) is invoked.

You'll add the swarm construction between the runner construction and the queue construction.

- [ ] **Step 3.2: Add env-var loading for brain models**

Find the existing env-loading block in `main.go`. Add (matching existing style):

```go
plannerModel := os.Getenv("PI_BRAIN_PLANNER_MODEL")
if plannerModel == "" {
    plannerModel = "openai/gpt-4o-mini"
}
reviewerModel := os.Getenv("PI_BRAIN_REVIEWER_MODEL")
if reviewerModel == "" {
    reviewerModel = "openai/gpt-4o-mini"
}
```

- [ ] **Step 3.3: Construct OpenRouter providers + memory + swarm**

After the env-loading and before `queue.New`, add:

```go
// era-brain integration: planner + reviewer LLMs and audit memory.
plannerLLM := openrouter.New(openrouter.Config{
    APIKey:       cfg.OpenRouterAPIKey, // existing field used by the Pi sidecar
    DefaultModel: plannerModel,
})
reviewerLLM := openrouter.New(openrouter.Config{
    APIKey:       cfg.OpenRouterAPIKey,
    DefaultModel: reviewerModel,
})
brainDBPath := filepath.Join(filepath.Dir(cfg.DBPath), "era-brain.db") // sibling of era.db
brainMem, err := brainsqlite.Open(brainDBPath)
if err != nil {
    log.Fatalf("open era-brain memory: %v", err)
}
defer brainMem.Close()
sw := swarm.New(swarm.Config{
    PlannerLLM:  plannerLLM,
    ReviewerLLM: reviewerLLM,
    Memory:      brainMem,
})
```

Add the imports:
```go
"path/filepath"

"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
brainsqlite "github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
"github.com/vaibhav0806/era/internal/swarm"
```

(Adjust `cfg.OpenRouterAPIKey` and `cfg.DBPath` to match the actual field names in era's config — read `internal/config/` if uncertain. The names are illustrative; preserve era's existing config struct.)

- [ ] **Step 3.4: Pass swarm to queue**

After `queue.New(...)`, add:
```go
q.SetSwarm(sw)
```

(`SetSwarm` is added in Task 4.)

- [ ] **Step 3.5: Build, verify compile errors only on `SetSwarm`**

```bash
go build ./...
```

Expected: error `q.SetSwarm undefined (type *queue.Queue has no field or method SetSwarm)`. That's correct — Task 4 adds it. Don't fix here. Move to Task 4.

DO NOT commit yet; this build is broken intentionally and the commit comes in Task 4 once the queue side lands.

---

## Task 4: Queue integration — RunNext calls planner before runner.Run, reviewer after

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_run_test.go`

Changes:

1. New `Queue.swarm *swarm.Swarm` field + `SetSwarm` setter.
2. Define a small `Swarm` interface in queue.go (`Plan(ctx, swarm.PlanArgs) (swarm.PlanResult, error)` + `Review(ctx, swarm.ReviewArgs) (swarm.ReviewResult, error)`). Queue tests will need to import `internal/swarm` (for the args/result types) and `era-brain/brain` (for `Receipt` in fake return values) — that's fine. Import chain is queue → swarm → brain → llm; no cycle. The Swarm interface in queue.go is purely for stubbability of the methods, not to avoid pulling in the swarm package types.

3. Modify `RunNext`: before the `q.runner.Run` call, call `q.swarm.Plan(...)`. Inject plan into task description via `swarm.InjectPlan`. After `runner.Run` succeeds, fetch the diff via `q.compare.Fetch(...)` (existing M3 path), call `q.swarm.Review(...)`, override the existing diff-scan-driven approval branch:

   - If swarm reviewer returns `DecisionApprove` AND existing diff-scan returns `findings == nil` → notify Completed (clean).
   - If swarm reviewer returns `DecisionFlag` OR existing diff-scan finds something → notify NeedsReview (existing M3 flow). **Reviewer's critique attaches as the body of the approval DM; existing diff-scan findings remain visible.**

4. New `CompletedArgs` struct (mirroring existing `NeedsReviewArgs`) carrying persona breakdown to the Notifier. Update the `Notifier.NotifyCompleted` interface method to take this struct instead of the current 7-arg shape (small breaking change inside the same milestone — acceptable per project's "atomic cascade commits when an interface widens" rule).

### 4A: Define Swarm interface + Queue field

- [ ] **Step 4.1: Write the failing test for SetSwarm + RunNext threading planner**

Modify `internal/queue/queue_run_test.go` (look at existing TestRunNext_* tests for the established pattern; add a new test):

```go
type stubSwarm struct {
	plannedDesc   string
	planText      string
	reviewedDiff  string
	reviewDecision swarm.Decision
}

func (s *stubSwarm) Plan(_ context.Context, args swarm.PlanArgs) (swarm.PlanResult, error) {
	s.plannedDesc = args.TaskDescription
	return swarm.PlanResult{
		PlanText: s.planText,
		Receipt: brain.Receipt{Persona: "planner", Model: "stub", Sealed: false, TimestampUnix: 1},
	}, nil
}

func (s *stubSwarm) Review(_ context.Context, args swarm.ReviewArgs) (swarm.ReviewResult, error) {
	s.reviewedDiff = args.DiffText
	return swarm.ReviewResult{
		CritiqueText: "ok",
		Decision:     s.reviewDecision,
		Receipt:      brain.Receipt{Persona: "reviewer", Model: "stub", Sealed: false, TimestampUnix: 2},
	}, nil
}

func TestRunNext_PlannerInjectedIntoRunnerDescription(t *testing.T) {
	// existing test scaffolding — see other RunNext tests for the boilerplate.
	// Key assertions:
	// 1. stubSwarm.plannedDesc == original task description (planner sees raw task)
	// 2. runner.Run was called with a description containing both the original task AND the planner step list
	// 3. After runner.Run succeeds, stubSwarm.reviewedDiff is non-empty (reviewer ran)
	// 4. NotifyCompleted received persona receipts in args
}
```

(The test scaffolding for existing RunNext tests is in `queue_run_test.go`. Mirror an existing test like `TestRunNext_HappyPath_LandsBranch` and add the swarm-related assertions.)

- [ ] **Step 4.2: Run, verify FAIL**

```bash
go test ./internal/queue/...
```

Expected: undefined symbols.

- [ ] **Step 4.3: Add Swarm interface + Queue field**

In `internal/queue/queue.go`, near the existing `Runner` interface (around line 40):

```go
// Swarm is the queue's view of the era-brain swarm: planner before runner.Run,
// reviewer after. Defined here (not imported from internal/swarm) so queue tests
// can inject a stub without pulling in the LLM stack.
type Swarm interface {
	Plan(ctx context.Context, args swarm.PlanArgs) (swarm.PlanResult, error)
	Review(ctx context.Context, args swarm.ReviewArgs) (swarm.ReviewResult, error)
}
```

Add `"github.com/vaibhav0806/era/internal/swarm"` to the imports.

In the `Queue` struct, add `swarm Swarm`:
```go
type Queue struct {
	// ... existing fields
	swarm Swarm
}
```

Add the setter (next to `SetNotifier`):
```go
func (q *Queue) SetSwarm(s Swarm) { q.swarm = s }
```

### 4B: Modify RunNext to call Plan + Review around runner.Run

- [ ] **Step 4.4: Modify RunNext to call Plan before runner.Run**

Around line 230 (where `q.runner.Run` is called), insert:

```go
// Plan: run planner persona before the container starts.
var planText string
var plannerReceipt brain.Receipt
if q.swarm != nil {
    pr, perr := q.swarm.Plan(ctx, swarm.PlanArgs{
        TaskID:          fmt.Sprintf("%d", t.ID),
        UserID:          fmt.Sprintf("%d", t.OwnerUserID), // or whatever era already has
        TaskDescription: t.Description,
    })
    if perr != nil {
        // Planner failure shouldn't block the task — log and continue.
        _ = q.repo.AppendEvent(ctx, t.ID, "planner_failed", quoteJSON(perr.Error()))
    } else {
        planText = pr.PlanText
        plannerReceipt = pr.Receipt
        _ = q.repo.AppendEvent(ctx, t.ID, "planner_ok", quoteJSON(planText))
    }
}

effectiveDesc := swarm.InjectPlan(t.Description, planText)
```

Add `"github.com/vaibhav0806/era-multi-persona/era-brain/brain"` to imports for the `Receipt` type.

Replace `t.Description` in the existing `q.runner.Run(...)` call with `effectiveDesc`.

- [ ] **Step 4.5: Modify RunNext to call Review after runner.Run succeeds**

After `q.runner.Run` succeeds (i.e. inside the `runErr == nil` path), and after `q.repo.CompleteTask(...)` — but BEFORE the existing diff-scan + notify branch — insert:

```go
// Review: fetch diff (best-effort), run reviewer persona.
var reviewerReceipt brain.Receipt
var reviewCritique string
reviewDecision := swarm.DecisionApprove // default to approve when no swarm wired (preserves M0 behavior)

if q.swarm != nil && branch != "" {
    var diffText string
    if q.compare != nil {
        // Fetch diff (best-effort). DiffSource.Compare returns []diffscan.FileDiff
        // (the same shape M3's diff-scan path consumes — see queue.go:296). We render
        // it back to a string for the reviewer with renderDiffText (helper added below).
        if files, derr := q.compare.Compare(ctx, effectiveRepo, base, branch); derr == nil {
            diffText = renderDiffText(files)
        } else {
            _ = q.repo.AppendEvent(ctx, t.ID, "diff_fetch_failed", quoteJSON(derr.Error()))
        }
    }
    rr, rerr := q.swarm.Review(ctx, swarm.ReviewArgs{
        TaskID:          fmt.Sprintf("%d", t.ID),
        TaskDescription: t.Description, // original, not effectiveDesc — reviewer sees the user's task, not the plan-augmented prompt
        PlanText:        planText,
        DiffText:        diffText,
        // DiffScanFindings filled later in this same step
    })
    if rerr != nil {
        _ = q.repo.AppendEvent(ctx, t.ID, "reviewer_failed", quoteJSON(rerr.Error()))
        reviewDecision = swarm.DecisionFlag // safe default on reviewer failure
    } else {
        reviewerReceipt = rr.Receipt
        reviewCritique = rr.CritiqueText
        reviewDecision = rr.Decision
        _ = q.repo.AppendEvent(ctx, t.ID, "reviewer_ok", quoteJSON(string(reviewDecision)))
    }
}
```

(Field names like `df.RawDiff` are illustrative — read `internal/githubcompare/` to find the actual shape; the existing M3 diff-scan path already uses it, so just match that consumer.)

- [ ] **Step 4.6: Update the notify-completed / notify-needs-review branch**

The existing M3 diff-scan flow already sets a `findings` slice from running diff-scan rules on the diff. After your reviewer-call insertion, the decision is now: notify Completed only if BOTH `len(findings) == 0` AND `reviewDecision == swarm.DecisionApprove`. Otherwise notify NeedsReview, attaching `reviewCritique` to the existing approval-DM body.

Find the existing branch that distinguishes clean vs flagged. Pseudocode:

```go
clean := len(findings) == 0 && reviewDecision == swarm.DecisionApprove
if clean {
    if q.notifier != nil {
        q.notifier.NotifyCompleted(ctx, queue.CompletedArgs{
            TaskID:    t.ID,
            Repo:      effectiveRepo,
            Branch:    branch,
            PRURL:     prURL,
            Summary:   summary,
            Tokens:    tokens,
            CostCents: costCents,
            Receipts: []brain.Receipt{
                plannerReceipt, synthCoderReceipt(), reviewerReceipt,
            },
            PlannerPlan:        planText,
            ReviewerCritique:   reviewCritique,
            ReviewerDecision:   string(reviewDecision),
        })
    }
} else {
    // existing NotifyNeedsReview path, with new fields:
    args.ReviewerCritique = reviewCritique
    args.PlannerPlan = planText
    args.Receipts = []brain.Receipt{plannerReceipt, synthCoderReceipt(), reviewerReceipt}
    q.notifier.NotifyNeedsReview(ctx, args)
}
```

The "coder receipt synthesized below" is a new helper: since Pi doesn't natively produce a receipt, we synthesize a placeholder one. Add at file scope:

```go
func synthCoderReceipt() brain.Receipt {
    // Pi runs inside the container; era-brain has no view of its prompt or
    // diff body. M7-A.5 emits a placeholder receipt so the swarm metadata
    // shape is consistent (3 entries: planner, coder, reviewer). M7-B
    // can extend Pi's RESULT json to include real InputHash/OutputHash;
    // M7-D's iNFT recordInvocation skips coder receipts where Sealed=false
    // and hashes are empty, so this placeholder doesn't pollute on-chain state.
    return brain.Receipt{
        Persona:       "coder",
        Model:         "", // Pi's model lives in container env; not surfaced here
        InputHash:     "",
        OutputHash:    "",
        Sealed:        false,
        TimestampUnix: time.Now().Unix(),
    }
}
```

Add `"time"` to queue.go imports if not already present.

Also add the diff-rendering helper at file scope:

```go
// renderDiffText composes a unified-diff-shaped string from []diffscan.FileDiff
// for the reviewer persona. Lossy (no @@ context lines), but enough for the
// reviewer LLM to spot test removals, weakened assertions, and plan deviations.
func renderDiffText(files []diffscan.FileDiff) string {
    var b strings.Builder
    for _, f := range files {
        fmt.Fprintf(&b, "--- %s\n+++ %s\n", f.Path, f.Path)
        for _, line := range f.Removed {
            b.WriteString("-")
            b.WriteString(line)
            b.WriteString("\n")
        }
        for _, line := range f.Added {
            b.WriteString("+")
            b.WriteString(line)
            b.WriteString("\n")
        }
    }
    return b.String()
}
```

Field names (`f.Path`, `f.Removed`, `f.Added`) are illustrative — read `internal/diffscan/` to find the actual field names; `[]diffscan.FileDiff` already flows through the M3 diff-scan path so the shape is established. Match the existing field accessors.

Add `"strings"` to queue.go imports.

### 4C: Add CompletedArgs struct and update Notifier interface

- [ ] **Step 4.7: Define CompletedArgs and update the Notifier interface**

Near `NeedsReviewArgs` in `queue.go`:

```go
// CompletedArgs bundles the completion-DM payload so we can extend persona
// breakdown without touching the Notifier signature again. Mirrors NeedsReviewArgs shape.
type CompletedArgs struct {
    TaskID           int64
    Repo             string
    Branch           string
    PRURL            string
    Summary          string
    Tokens           int64
    CostCents        int
    Receipts         []brain.Receipt // [planner, coder, reviewer] in order
    PlannerPlan      string
    ReviewerCritique string
    ReviewerDecision string // "approve" or "flag"
}
```

Update `Notifier`:
```go
type Notifier interface {
    NotifyCompleted(ctx context.Context, args CompletedArgs)
    NotifyFailed(ctx context.Context, taskID int64, reason string)
    NotifyNeedsReview(ctx context.Context, args NeedsReviewArgs)
    NotifyCancelled(ctx context.Context, taskID int64)
}
```

This is a breaking interface change — update `cmd/orchestrator/main.go`'s `tgNotifier.NotifyCompleted` accordingly (Task 5 covers the impl side).

### 4D: Verify build

- [ ] **Step 4.8: Run, verify the queue tests still pass after impl**

```bash
go test -race ./internal/queue/...
```

Expected: existing tests pass (with their fakes updated for the new NotifyCompleted shape). The new `TestRunNext_PlannerInjectedIntoRunnerDescription` passes.

**Cascade list (must update in this same commit):**

The existing `internal/queue/queue_run_test.go` has:
- `fakeNotifier.NotifyCompleted(ctx, id, repo, b, prURL, s, t, c)` impl — change to `NotifyCompleted(ctx, queue.CompletedArgs)`.
- `var _ queue.Notifier = (*fakeNotifier)(nil)` compile-time assertion — will catch any drift.
- `completedArgs` test struct — extend to hold the new fields, or replace its usage with `queue.CompletedArgs` directly.
- ~4 test functions assert on `n.completed[…]` fields — update to read from the new struct shape.

`grep -n "NotifyCompleted\|completedArgs\|n\.completed" internal/queue/queue_run_test.go` to find every site. Update them all in this commit.

- [ ] **Step 4.9: Don't commit yet — Task 5 ties off the orchestrator side**

If `go build ./...` from repo root errors on `cmd/orchestrator/main.go` (because tgNotifier.NotifyCompleted still has the old signature), that's expected. Task 5 fixes it.

---

## Task 5: Update tgNotifier — render persona breakdown in completion DM

**Files:**
- Modify: `cmd/orchestrator/main.go`

The existing `tgNotifier.NotifyCompleted` takes 8 args. Replace with the `CompletedArgs` struct from Task 4. Render the planner plan + reviewer decision in the DM body (compact — Telegram has a 4096-char cap; era already truncates summaries).

- [ ] **Step 5.1: Update tgNotifier.NotifyCompleted signature**

In `cmd/orchestrator/main.go`, change:
```go
func (n *tgNotifier) NotifyCompleted(ctx context.Context, id int64, repo, branch, prURL, summary string, tokens int64, costCents int) {
```
to:
```go
func (n *tgNotifier) NotifyCompleted(ctx context.Context, a queue.CompletedArgs) {
```

Update the body to read fields from `a` instead of positional args. Keep the existing DM format but add a compact persona footer:

```go
// Existing body composition uses id/repo/branch/prURL/summary/tokens/costCents.
// Read those from a.TaskID, a.Repo, a.Branch, a.PRURL, a.Summary, a.Tokens, a.CostCents.

// NEW: persona footer (only when receipts present).
if len(a.Receipts) >= 1 {
    body += fmt.Sprintf("\n\n— planner: %.80s%s",
        truncate(a.PlannerPlan, 80),
        ellipsisIfTrunc(a.PlannerPlan, 80),
    )
}
if a.ReviewerDecision != "" {
    body += fmt.Sprintf("\n— reviewer: %s%s",
        a.ReviewerDecision,
        ifNotEmpty(a.ReviewerCritique, " — "+truncate(a.ReviewerCritique, 120)),
    )
}
```

(Helpers `truncate`, `ellipsisIfTrunc`, `ifNotEmpty` may already exist in main.go; if not, add them inline. era's existing DM code uses rune-safe truncation — `internal/queue/Truncate` is already exported. Use that.)

- [ ] **Step 5.2: Update the compile-time assertion at the bottom of main.go**

```go
var _ queue.Notifier = (*tgNotifier)(nil)
```

This was already there. Now that `Notifier.NotifyCompleted` signature changed, the compile-time check will catch any drift.

- [ ] **Step 5.3: Build the whole repo**

```bash
go build ./...
```

Expected: zero errors. Tasks 4 + 5 together close the breaking interface change.

- [ ] **Step 5.4: Run all tests**

```bash
go vet ./... && go test -race ./...
```

Expected: green. If any test in `internal/queue/queue_run_test.go` had a fake Notifier using the old NotifyCompleted shape, update it to the new struct shape.

- [ ] **Step 5.5: Commit (Tasks 4 + 5 together — atomic cascade)**

```bash
git add internal/queue/ cmd/orchestrator/main.go
git commit -m "phase(M7-A.5.4-5): RunNext threads planner+reviewer; CompletedArgs carries persona breakdown; tgNotifier renders it"
git tag m7a5-45-queue-notifier
```

---

## Task 6: Live gate — real /task on real repo

No code changes. This is the integration test that proves the milestone.

- [ ] **Step 6.1: Build + deploy locally**

```bash
go build -o bin/orchestrator ./cmd/orchestrator
```

Run the binary locally — do NOT deploy to VPS yet. The VPS has M6's binary; this run is to validate before pushing.

- [ ] **Step 6.2: Source .env and run**

```bash
set -a; source .env; set +a
./bin/orchestrator
```

Expected boot log lines:
- migration runs OK
- "orchestrator ready version=… db_path=… sandbox_repo=…"
- (NEW) "era-brain memory db: data/era-brain.db" or similar — confirm the brain DB is opened.

- [ ] **Step 6.3: Send a /task via Telegram**

From your phone or Telegram desktop, send to your bot:

```
/task add a /healthz endpoint to the existing HTTP server, returning 200 OK with body "ok"
```

(Replace with your actual sandbox repo's command form if M6 needed `--repo` / a per-task `<owner>/<repo>` prefix. Match how you've been using era day-to-day.)

- [ ] **Step 6.4: Watch the orchestrator stdout**

Expected event sequence:
1. `task_id=<N> created`
2. `planner_ok` event (planner produced a plan)
3. Pi container spawn + tool-loop progress as before
4. `pr_opened` event (PR URL)
5. `reviewer_ok` event (reviewer decision)
6. `completed` event

- [ ] **Step 6.5: Verify the Telegram completion DM**

Expected DM shape:
- Branch link
- PR link
- Summary
- (NEW) `— planner: 1. add /healthz handler ...` (truncated)
- (NEW) `— reviewer: approve` (or `flag`, depending on Pi's diff)

If reviewer flagged, you'll get the existing Approve/Reject inline buttons instead — that's still success, just the flagged path.

- [ ] **Step 6.6: Verify the era-brain audit log**

```bash
sqlite3 data/era-brain.db "SELECT seq, namespace, length(val) FROM entries WHERE is_kv = 0 ORDER BY seq"
```

Expected: 2 log entries per task — one from planner, one from reviewer — both under `audit/<task_id>` namespace.

(Coder receipt is NOT in era-brain audit log because Pi doesn't write to the era-brain memory provider. Pi's audit lives in era's main DB as before.)

- [ ] **Step 6.7: Verify M6 product still works**

Send a second `/task` and verify the existing flow (branch + PR + DM) works end-to-end — the planner + reviewer additions shouldn't have regressed anything.

- [ ] **Step 6.8: Commit + tag M7-A.5 done**

If you made any small fixes during the live gate (typos, env names, etc.), commit them. Otherwise:

```bash
git tag m7a5-done
```

(no commit needed if Task 6 was pure verification).

---

## Live gate summary (M7-A.5 acceptance)

When this milestone is done:

1. `go test -race ./...` from repo root — green. era M6 tests untouched. New tests in `internal/swarm/` and `internal/queue/` pass.
2. Real `/task` on a real repo:
   - Planner persona produces a step list (visible in `planner_ok` event + Telegram DM footer).
   - Pi runs as before, edits files, pushes a branch.
   - Reviewer persona produces a critique + decision.
   - Clean tasks (reviewer=approve, no diff-scan findings) auto-complete with persona footer in DM.
   - Flagged tasks (reviewer=flag OR diff-scan findings) hit the existing M3 NeedsReview Approve/Reject DM, with reviewer critique attached.
3. era-brain audit log (`data/era-brain.db`) has 2 receipts per task (planner + reviewer) under `audit/<task_id>`.
4. Existing Hetzner VPS deployment is untouched — we run locally for the live gate; M5 CI auto-deploys when we push.

---

## Out of scope (deferred to M7-B and beyond)

- Coder persona writing its own receipt with InputHash/OutputHash from Pi's actual prompt + response. Requires Pi to emit a receipt-shaped JSON line. Defer to M7-B or later.
- Persona memory (KV) reads — planner doesn't read prior plans yet. M7-B introduces 0G KV and the persona memory loop.
- iNFT `recordInvocation` calls per receipt — M7-D. The receipts collected here will be the ones M7-D records on-chain.
- Per-persona model assignment (planner=GLM-5-FP8, coder=qwen3.6-plus, etc) — M7-C wires this once 0G Compute is online. M7-A.5 uses gpt-4o-mini for both planner and reviewer.
- ENS subnames in completion DM — M7-E.
- Web dashboard / activity feed — stretch / M7-F.
