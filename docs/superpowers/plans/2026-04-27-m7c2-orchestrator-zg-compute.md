# M7-C.2 — Orchestrator 0G Compute Wiring + Reviewer Cross-Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire era's orchestrator to use `fallback.New(zg_compute, openrouter)` for both planner + reviewer LLM calls when 0G Compute env vars are present. Real `/task` via Telegram produces planner + reviewer receipts with `Sealed=true`. Reviewer's prompt extends to surface planner-sealed flag (lite cross-check) so reviewer can scrutinize unsealed-output runs.

**Architecture:** Single milestone, three phases. Phase 1 wires fallback into `cmd/orchestrator/main.go` (mirrors M7-B.2.1's wiring pattern). Phase 2 extends `internal/swarm.composeCoderOutput` with `planner_sealed:` / `coder_sealed:` lines + updates `ReviewerSystemPrompt`; `internal/queue/queue.go` populates `ReviewArgs.PriorPersonaSealed` from planner receipt. Phase 3 is the live gate — real Telegram `/task` showing `Sealed=true` end-to-end.

**Tech Stack:** Go 1.25. era-brain SDK (M7-C.1 shipped `llm/zg_compute` + `llm/fallback`). 0G Compute testnet bearer-auth working (proven in M7-C.1.0). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-04-26-m7c-sealed-inference-design.md`. §3 "Layer 3 — era app integration" + §5 phases C.2.1-C.2.3.

**Testing philosophy:** Strict TDD. Phase 1 is config wiring (no easily unit-testable surface — mirror M7-B.2.1 cadence). Phase 2 has unit tests on `composeCoderOutput` signature + on RunNext's `PriorPersonaSealed` population via stubSwarm. Phase 3 is the live integration gate. `go test -race -count=1 ./...` from repo root green at every commit.

**Prerequisites (check before starting):**
- M7-C.1 done (tag `m7c1-done`).
- `.env` populated w/ `PI_ZG_COMPUTE_ENDPOINT`, `PI_ZG_COMPUTE_BEARER`, `PI_ZG_COMPUTE_MODEL=qwen/qwen-2.5-7b-instruct`.
- Provider sub-account funded (1 ZG transferred — covered in C.1.0 setup; no additional setup needed).
- Existing era M6 + M7-A.5 + M7-B.2 + M7-B.3 tests green: `go test -race -count=1 ./...`.

---

## File Structure

```
cmd/orchestrator/main.go                MODIFY (Phase 1) — env-var loading; conditional fallback wrap

internal/swarm/swarm.go                 MODIFY (Phase 2) — composeCoderOutput sig change; ReviewArgs.PriorPersonaSealed; Review() passes it through
internal/swarm/personas.go              MODIFY (Phase 2) — ReviewerSystemPrompt appends sealed-flag explanation
internal/swarm/swarm_test.go            MODIFY (Phase 2) — new test asserts composeCoderOutput emits header lines + reviewer prompt contains them

internal/queue/queue.go                 MODIFY (Phase 2) — RunNext populates ReviewArgs.PriorPersonaSealed from plannerReceipt.Sealed
internal/queue/queue_run_test.go        MODIFY (Phase 2) — stubSwarm captures PriorPersonaSealed; test asserts planner Sealed flows through
```

No new files. Three packages touched. No changes to era-brain SDK (M7-C.1 already shipped what's needed).

---

## Phase 1: Wire fallback(zg_compute, openrouter) in main.go

**Files:**
- Modify: `cmd/orchestrator/main.go` — lines 110-156 area (existing era-brain swarm wiring block)

The existing block (post-M7-B.2):

```go
plannerLLM := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: plannerModel})
reviewerLLM := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: reviewerModel})
// ... brainMem setup ...
sw := swarm.New(swarm.Config{
    PlannerLLM:  plannerLLM,
    ReviewerLLM: reviewerLLM,
    Memory:      memProv,
})
```

Becomes:

```go
plannerOR := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: plannerModel})
reviewerOR := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: reviewerModel})

// ... brainMem setup unchanged ...

// Build the LLM providers passed to swarm. Default = OpenRouter alone (M7-B.3 baseline).
// If 0G Compute env vars are present, wrap each persona's LLM with fallback so
// inference tries 0G Compute first, falls back to OpenRouter on error.
var plannerLLM llm.Provider = plannerOR
var reviewerLLM llm.Provider = reviewerOR

if zgComputeEnabled() {
    zgComp := zg_compute.New(zg_compute.Config{
        BearerToken:      os.Getenv("PI_ZG_COMPUTE_BEARER"),
        ProviderEndpoint: os.Getenv("PI_ZG_COMPUTE_ENDPOINT"),
        DefaultModel:     envOrDefault("PI_ZG_COMPUTE_MODEL", "qwen/qwen-2.5-7b-instruct"),
    })
    plannerLLM = fallback.New(zgComp, plannerOR, func(err error) {
        slog.Warn("planner sealed inference fell back to openrouter", "err", err)
    })
    reviewerLLM = fallback.New(zgComp, reviewerOR, func(err error) {
        slog.Warn("reviewer sealed inference fell back to openrouter", "err", err)
    })
    slog.Info("0G Compute sealed inference wired",
        "model", envOrDefault("PI_ZG_COMPUTE_MODEL", "qwen/qwen-2.5-7b-instruct"))
}

sw := swarm.New(swarm.Config{
    PlannerLLM:  plannerLLM,
    ReviewerLLM: reviewerLLM,
    Memory:      memProv,
})
```

Plus a `zgComputeEnabled()` helper at the bottom of main.go (next to existing `zgEnabled`).

### Step 1.1: Read existing wiring block

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
grep -n "plannerLLM\|reviewerLLM\|swarm.New\|zgEnabled\|envOrDefault" cmd/orchestrator/main.go | head -20
```

Expected: existing references to `plannerLLM`, `reviewerLLM`, the swarm.New construction at line ~151, and `zgEnabled`/`envOrDefault` helpers at the bottom.

### Step 1.2: Add the imports

In `cmd/orchestrator/main.go` import block, add:

```go
"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
"github.com/vaibhav0806/era-multi-persona/era-brain/llm/fallback"
"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
```

`os`, `slog`, and the existing `openrouter` import are already present.

### Step 1.3: Add the `zgComputeEnabled` helper

At the bottom of main.go (next to existing `zgEnabled`):

```go
// zgComputeEnabled returns true when all required 0G Compute env vars are present.
// PI_ZG_COMPUTE_MODEL is optional (defaults to qwen/qwen-2.5-7b-instruct).
func zgComputeEnabled() bool {
    return os.Getenv("PI_ZG_COMPUTE_ENDPOINT") != "" &&
        os.Getenv("PI_ZG_COMPUTE_BEARER") != ""
}
```

### Step 1.4: Modify the era-brain swarm wiring block

Find lines around 115-116 where `plannerLLM` and `reviewerLLM` are constructed. Rename them to `plannerOR` and `reviewerOR` (these are the OpenRouter-only providers). Then BEFORE the `sw := swarm.New(...)` call, insert the fallback construction block:

```go
plannerOR := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: plannerModel})
reviewerOR := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: reviewerModel})

// ... existing brainMem + memProv setup unchanged ...

var plannerLLM llm.Provider = plannerOR
var reviewerLLM llm.Provider = reviewerOR

if zgComputeEnabled() {
    zgModel := envOrDefault("PI_ZG_COMPUTE_MODEL", "qwen/qwen-2.5-7b-instruct")
    zgComp := zg_compute.New(zg_compute.Config{
        BearerToken:      os.Getenv("PI_ZG_COMPUTE_BEARER"),
        ProviderEndpoint: os.Getenv("PI_ZG_COMPUTE_ENDPOINT"),
        DefaultModel:     zgModel,
    })
    plannerLLM = fallback.New(zgComp, plannerOR, func(err error) {
        slog.Warn("planner sealed inference fell back to openrouter", "err", err)
    })
    reviewerLLM = fallback.New(zgComp, reviewerOR, func(err error) {
        slog.Warn("reviewer sealed inference fell back to openrouter", "err", err)
    })
    slog.Info("0G Compute sealed inference wired", "model", zgModel)
}

sw := swarm.New(swarm.Config{
    PlannerLLM:  plannerLLM,
    ReviewerLLM: reviewerLLM,
    Memory:      memProv,
})
```

The `swarm.New` call's field references stay as `plannerLLM` and `reviewerLLM` — those are now `llm.Provider` interface values (either bare openrouter when the env vars are absent, or fallback-wrapped when present).

### Step 1.5: Build, verify compile

```bash
go build ./...
```

Expected: exit 0.

If errors:
- "imported and not used" → check that `llm`, `fallback`, `zg_compute` are all referenced.
- type-mismatch on swarm.Config → confirm `PlannerLLM` and `ReviewerLLM` accept `llm.Provider` (they should — they're the interface type).

### Step 1.6: Run all tests

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green. No new tests in this phase. Existing tests must not regress.

### Step 1.7: Commit

```bash
git add cmd/orchestrator/main.go
git commit -m "phase(M7-C.2.1): orchestrator wires fallback(zg_compute, openrouter) for planner+reviewer when env vars set"
git tag m7c2-1-wired
```

---

## Phase 2: Reviewer cross-check — composeCoderOutput + ReviewArgs.PriorPersonaSealed

**Files:**
- Modify: `internal/swarm/swarm.go` — `ReviewArgs` gains field; `composeCoderOutput` sig change; `Review()` passes through
- Modify: `internal/swarm/personas.go` — `ReviewerSystemPrompt` appends sealed-flag explanation
- Modify: `internal/swarm/swarm_test.go` — new test asserts header lines emitted
- Modify: `internal/queue/queue.go` — `RunNext` populates `ReviewArgs.PriorPersonaSealed` from `plannerReceipt.Sealed`
- Modify: `internal/queue/queue_run_test.go` — stubSwarm captures `lastReviewArgs.PriorPersonaSealed`; new test asserts flow

### 2A: composeCoderOutput signature + Review() wiring

- [ ] **Step 2.1: Write failing test for new composeCoderOutput signature**

The existing `composeCoderOutput` is called only from `Swarm.Review()` (line ~119 of swarm.go). After this change, `Review()` must read `args.PriorPersonaSealed["planner"]` and pass it (plus `coderSealed=false` always — Pi is unsealed per spec) into the new params.

Test the end-to-end behavior via swarm.Review (existing test pattern). Append to `internal/swarm/swarm_test.go`:

```go
func TestSwarm_Review_PromptIncludesSealedFlags(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:          "t1",
		TaskDescription: "task",
		PlanText:        "plan",
		DiffText:        "diff",
		PriorPersonaSealed: map[string]bool{
			"planner": true,
		},
	})
	require.NoError(t, err)
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "planner_sealed: true")
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "coder_sealed: false")
}

func TestSwarm_Review_PlannerUnsealedPropagates(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:          "t1",
		TaskDescription: "task",
		PlanText:        "plan",
		DiffText:        "diff",
		PriorPersonaSealed: map[string]bool{
			"planner": false, // fallback fired
		},
	})
	require.NoError(t, err)
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "planner_sealed: false")
}

func TestSwarm_Review_DefaultsBothSealedFalseWhenMapNil(t *testing.T) {
	// Backward-compat: if PriorPersonaSealed is nil (pre-M7-C.2 callers),
	// emit both flags as false. Reviewer treats unknown sealed status as
	// the safe default.
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID: "t1", TaskDescription: "task", PlanText: "plan", DiffText: "diff",
		// PriorPersonaSealed: nil
	})
	require.NoError(t, err)
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "planner_sealed: false")
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "coder_sealed: false")
}
```

- [ ] **Step 2.2: Run, verify FAIL**

```bash
go test ./internal/swarm/ -run Review_PromptIncludesSealedFlags -count=1
```

Expected: tests fail because:
- `ReviewArgs` has no `PriorPersonaSealed` field yet (compile error).
- After adding the field, the assertions on `planner_sealed:` / `coder_sealed:` strings fail because `composeCoderOutput` doesn't emit them yet.

- [ ] **Step 2.3: Add `PriorPersonaSealed` to `ReviewArgs`**

In `internal/swarm/swarm.go`, find the `ReviewArgs` struct (line ~94) and add the field:

```go
type ReviewArgs struct {
	TaskID             string
	UserID             string
	TaskDescription    string
	PlanText           string
	DiffText           string
	DiffScanFindings   []string
	PriorPersonaSealed map[string]bool // NEW (M7-C.2): persona name → was its receipt sealed
}
```

- [ ] **Step 2.4: Modify `composeCoderOutput` signature + impl**

Find `composeCoderOutput` (line ~140 of swarm.go). Change signature + body:

```go
// composeCoderOutput renders the coder's contribution for the reviewer's prompt.
// The leading sealed-flag block (planner_sealed / coder_sealed) was added in
// M7-C.2 so the reviewer can scrutinize unsealed-persona output more strictly.
func composeCoderOutput(diff string, findings []string, plannerSealed, coderSealed bool) string {
	header := fmt.Sprintf("planner_sealed: %t\ncoder_sealed: %t\n\n", plannerSealed, coderSealed)
	if len(diff) > maxDiffChars {
		original := len(diff)
		diff = diff[:maxDiffChars] + fmt.Sprintf("\n[... diff truncated, original was %d chars ...]", original)
	}
	out := header + "Diff:\n" + diff
	if len(findings) > 0 {
		out += "\n\nDiff-scan findings:\n"
		for _, f := range findings {
			out += "- " + f + "\n"
		}
	}
	return out
}
```

- [ ] **Step 2.5: Update `Review()` caller**

Find the `Review()` method (line ~112). The existing call is:

```go
{PersonaName: "coder", Text: composeCoderOutput(args.DiffText, args.DiffScanFindings)},
```

Change to:

```go
{PersonaName: "coder", Text: composeCoderOutput(args.DiffText, args.DiffScanFindings, args.PriorPersonaSealed["planner"], false)},
```

(The `false` for coderSealed is hardcoded because Pi is always unsealed in this milestone. Reading from the map for `"coder"` would also return `false` since queue never populates it — but explicit literal is clearer.)

- [ ] **Step 2.6: Run, verify PASS**

```bash
go test -race ./internal/swarm/ -count=1
```

Expected: 3 new tests pass + all existing swarm tests still pass.

### 2B: ReviewerSystemPrompt update

- [ ] **Step 2.7: Append sealed-flag explanation to ReviewerSystemPrompt**

Open `internal/swarm/personas.go`. Find `ReviewerSystemPrompt` (existing const). Append a paragraph at the end:

```go
const ReviewerSystemPrompt = `... existing system prompt body ...

Note on sealed inference: each persona's output is preceded by ` + "`planner_sealed:`" + ` / ` + "`coder_sealed:`" + ` flags. ` + "`true`" + ` means that persona ran on TEE-attested sealed inference (cryptographic proof of model identity); ` + "`false`" + ` means it ran on unsealed inference (no such proof). When a flag is ` + "`false`" + `, treat that persona's output with extra scrutiny — the cryptographic guarantee is absent.`
```

(Use string concat w/ raw strings to avoid escaping the backticks. If the existing constant uses a different quote style, adapt.)

Optional: read `personas.go` first to see how the constant is defined and pick the cleanest extension style.

### 2C: queue.RunNext populates PriorPersonaSealed

- [ ] **Step 2.8: Read existing RunNext to find ReviewArgs construction**

```bash
grep -n "swarm.ReviewArgs\|plannerReceipt\.Sealed\|q.swarm.Review" internal/queue/queue.go | head
```

Expected: hits at the existing `q.swarm.Review(...)` call site inside `RunNext`. Note `plannerReceipt` is already a local variable.

- [ ] **Step 2.9: Write failing test for queue side**

Modify `internal/queue/queue_run_test.go` to add a new test asserting `PriorPersonaSealed["planner"]` is populated from planner receipt's Sealed flag.

Find the existing `stubSwarm.Plan` impl. Update it to set `Sealed=true` when a test marker is provided. Easiest path: add a new field `plannerSealed bool` to `stubSwarm`:

```go
type stubSwarm struct {
	plannedDesc    string
	planText       string
	plannerSealed  bool // NEW (M7-C.2): set Receipt.Sealed in Plan()
	reviewedDiff   string
	reviewDecision swarm.Decision
	lastPlanArgs   swarm.PlanArgs
	lastReviewArgs swarm.ReviewArgs
}

func (s *stubSwarm) Plan(_ context.Context, args swarm.PlanArgs) (swarm.PlanResult, error) {
	s.lastPlanArgs = args
	s.plannedDesc = args.TaskDescription
	return swarm.PlanResult{
		PlanText: s.planText,
		Receipt:  brain.Receipt{Persona: "planner", Model: "stub", Sealed: s.plannerSealed},
	}, nil
}
```

Then add the new test. **Critical: copy the FULL setup from `TestRunNext_ThreadsUserIDIntoSwarm` verbatim** — without the runner setup that returns a non-empty branch, `RunNext` exits early before calling `swarm.Review` (the `q.swarm != nil && branch != ""` guard).

Concrete shape (copy-paste the queue/runner/notifier/task setup; only the `stubSwarm` config + assertion lines differ from the existing test):

```go
func TestRunNext_PassesPlannerSealedToReviewer(t *testing.T) {
	q, repo, fr, fn := newRunQueue(t) // existing helper used by other RunNext tests
	stub := &stubSwarm{
		planText:       "1. step",
		plannerSealed:  true, // simulate sealed inference returned by zg_compute
		reviewDecision: swarm.DecisionApprove,
	}
	q.SetSwarm(stub)
	q.SetUserID("u")

	// fakeRunner returns a non-empty branch → reviewer block runs
	fr.branch = "agent/1/ok"
	fr.summary = "ok"

	taskID, err := repo.CreateTask(context.Background(), "do thing", "owner/repo", "default")
	require.NoError(t, err)

	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed)
	_ = taskID
	_ = fn

	require.NotNil(t, stub.lastReviewArgs.PriorPersonaSealed)
	require.True(t, stub.lastReviewArgs.PriorPersonaSealed["planner"],
		"planner's Sealed flag should propagate to reviewer args")
	require.False(t, stub.lastReviewArgs.PriorPersonaSealed["coder"],
		"coder is always unsealed (Pi-in-Docker) per M7-C scope")
}

func TestRunNext_PassesUnsealedPlannerToReviewer(t *testing.T) {
	q, repo, fr, _ := newRunQueue(t)
	stub := &stubSwarm{
		planText:       "1. step",
		plannerSealed:  false, // simulate fallback fired
		reviewDecision: swarm.DecisionApprove,
	}
	q.SetSwarm(stub)
	q.SetUserID("u")
	fr.branch = "agent/1/ok"
	fr.summary = "ok"

	_, err := repo.CreateTask(context.Background(), "do thing", "owner/repo", "default")
	require.NoError(t, err)
	_, err = q.RunNext(context.Background())
	require.NoError(t, err)

	require.False(t, stub.lastReviewArgs.PriorPersonaSealed["planner"])
}
```

**Field names to verify** in the existing test scaffolding:
- The helper `newRunQueue(t)` (or whatever existing test setup function returns the queue/repo/fakeRunner/fakeNotifier tuple) — find it in `queue_run_test.go` and use it verbatim.
- `fakeRunner.branch` / `fakeRunner.summary` — verify field names in the existing `fakeRunner` definition.
- `repo.CreateTask` signature — verify the arg order matches the existing fakes/repo wrapper.

If `newRunQueue(t)` doesn't exist by that name, search for an existing test like `TestRunNext_HappyPath_LandsBranch` or `TestRunNext_PlannerInjectedIntoRunnerDescription` — copy its setup boilerplate as the template.

- [ ] **Step 2.10: Run, verify FAIL**

```bash
go test ./internal/queue/ -run PassesPlannerSealed -count=1
```

Expected: `stub.lastReviewArgs.PriorPersonaSealed` is nil because RunNext doesn't populate it yet.

- [ ] **Step 2.11: Modify `RunNext` to populate `PriorPersonaSealed`**

In `internal/queue/queue.go`, find the `q.swarm.Review(...)` call inside `RunNext`. The existing `swarm.ReviewArgs{...}` literal should add a new field:

```go
priorSealed := map[string]bool{
	"planner": plannerReceipt.Sealed,
	"coder":   false, // Pi is always unsealed in M7-C scope
}

rr, rerr := q.swarm.Review(ctx, swarm.ReviewArgs{
	TaskID:             fmt.Sprintf("%d", t.ID),
	UserID:             q.userID,
	TaskDescription:    t.Description,
	PlanText:           planText,
	DiffText:           diffText,
	PriorPersonaSealed: priorSealed,
})
```

(`plannerReceipt` is the existing local variable from the planner's Plan() call earlier in RunNext. The exact placement of the `priorSealed := ...` declaration: just BEFORE the `q.swarm.Review(...)` call.)

- [ ] **Step 2.12: Run, verify PASS**

```bash
go test -race ./internal/queue/ -count=1
```

Expected: 2 new tests pass + all existing queue tests still pass.

### 2D: Sanity sweep + commit

- [ ] **Step 2.13: Run all tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
```

Both green from repo root.

```bash
cd era-brain && go vet ./... && go test -race -count=1 ./...
```

era-brain green (no era-brain code touched in this phase, but verify nothing went sideways).

- [ ] **Step 2.14: Commit**

```bash
git add internal/swarm/ internal/queue/
git commit -m "phase(M7-C.2.2): reviewer cross-check — composeCoderOutput emits sealed flags; RunNext populates PriorPersonaSealed"
git tag m7c2-2-cross-check
```

---

## Phase 3: Live gate — real Telegram /task

**Files:** none modified. Verification only.

### Step 3.1: Build orchestrator

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go build -o bin/orchestrator ./cmd/orchestrator
ls -lh bin/orchestrator
```

Expected: exit 0; binary at `bin/orchestrator`.

### Step 3.2: Stop VPS M6

```bash
ssh era@178.105.44.3 sudo systemctl stop era
```

Wait ~3 sec.

### Step 3.3: Start local orchestrator with full env

```bash
set -a; source .env; set +a
./bin/orchestrator
```

**Expected boot lines (NEW vs M7-B.3):**
- migrations OK (existing era migrations)
- `INFO Selecting nodes ...` (0G Storage SDK)
- `INFO 0G storage wired indexer=...`
- **`INFO 0G Compute sealed inference wired model=qwen/qwen-2.5-7b-instruct`** ← NEW (M7-C.2.1)
- `INFO orchestrator ready ...`

If the "0G Compute sealed inference wired" line is absent, env vars aren't sourced — re-check `.env`.

### Step 3.4: Send a /task via Telegram

From your phone:

```
/task add a /healthz endpoint that returns 200 OK with body "ok"
```

### Step 3.5: Watch the orchestrator stdout

Expected event sequence (compared to M7-B.3 baseline):
- Same 0G Storage tx-params + Transaction-receipt blocks for memory writes (planner audit log + reviewer audit log + planner KV + reviewer KV) — **4 0G Storage writes per task per M7-B.3**.
- **NEW: NO `Set tx params` blocks between persona-LLM-call moments where M7-B.3 used to show OpenRouter calls** — the LLM calls now go to 0G Compute first. Look for lack of fallback-warn lines as the proof.
- If `WARN planner sealed inference fell back to openrouter` or `WARN reviewer sealed inference fell back to openrouter` appears → fallback fired (testnet hiccup). Document the cause.
- Telegram completion DM contains the existing M7-B.3 persona breakdown + reviewer's critique should reference `planner_sealed: true|false` somewhere in the body (the reviewer system prompt instructed it to consider these flags).

### Step 3.6: Verify era-brain.db has Sealed=true on receipts

```bash
sqlite3 ./era-brain.db "SELECT seq, namespace, length(val) FROM entries WHERE is_kv = 0 ORDER BY seq DESC LIMIT 4"
```

Inspect the latest planner audit-log entry:

```bash
sqlite3 ./era-brain.db "SELECT val FROM entries WHERE is_kv = 0 AND namespace LIKE 'audit/%' ORDER BY seq DESC LIMIT 1"
```

Expected: JSON with `"Sealed":true` in the receipt blob (from planner persona, latest task).

Inspect a reviewer entry similarly — it should also be `"Sealed":true`.

If both show `"Sealed":false` → fallback fired for both personas → check stdout for fallback warnings.

### Step 3.7: Restart VPS M6

```bash
ssh era@178.105.44.3 sudo systemctl start era
```

**Don't skip.** Production bot stays offline until you do.

### Step 3.8: Stop local orchestrator (Ctrl-C)

### Step 3.9: Replay tests

```bash
go vet ./... && go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./...
```

Both green.

### Step 3.10: Tag M7-C.2 done

```bash
git tag m7c2-done
```

(no commit — Phase 3 is verification only)

---

## Live gate summary (M7-C.2 acceptance)

When this milestone is done:

1. `go build ./...` from repo root succeeds.
2. `go test -race -count=1 ./...` from repo root green; no regression.
3. Real `/task` on a real repo:
   - Orchestrator startup logs show `0G Compute sealed inference wired ...`.
   - `era-brain.db` audit-log entries for planner + reviewer have `"Sealed":true` in the JSON.
   - Telegram DM unchanged shape from M7-B.3 (planner plan + reviewer decision footer).
   - Optional: reviewer's critique text mentions the sealed flags (this is LLM behavior — may or may not surface; not a blocker).
4. Without `PI_ZG_COMPUTE_*` env vars, orchestrator falls back to OpenRouter-only — M7-B.3 baseline preserved.
5. VPS M6 era is restarted after the live gate.

---

## Out of scope (deferred to M7-D and beyond)

- **Coder persona via 0G Compute.** Pi-in-Docker; bridging Pi → 0G Compute is a separate effort.
- **Cryptographic TEE signature verification.** No Go tooling exists. Sealed=true means "header was present"; honest scope limit (parent spec §4).
- **iNFT recordInvocation per receipt.** M7-D records sealed-receipt hashes on-chain.
- **Audit log new event kinds** (`inference_sealed` / `inference_fell_back`). Cuts-list candidate from spec §5; cheap to add but not blocking.
- **Per-persona model assignment via different env vars.** Testnet has one sealed model; one shared `PI_ZG_COMPUTE_MODEL` is enough.

---

## Risks + cuts list (in order if slipping)

1. **Live gate fails because reviewer's prompt is too long after adding the sealed-flag block.** Recovery: the new header is ~30 chars; negligible. Real risk is testnet latency on long inference calls; if this trips, increase HTTPTimeout in zg_compute.Config.
2. **Fallback fires on every planner call** (testnet provider down). Recovery: dual provider's resilience already handles this — task continues on OpenRouter; receipts show Sealed=false; cross-check still works (reviewer sees `planner_sealed: false`).
3. **Cascade on `composeCoderOutput`'s test usage.** The signature changed. Existing swarm tests that called it (via `Review()`) still work because they don't pass the new `PriorPersonaSealed` field — Go zero-values the new map field as nil, and our impl handles nil correctly (returns `planner_sealed: false`). Verify in Step 2.13.
4. **Audit-log Sealed flag wrong direction.** If receipt JSON shows `Sealed=false` for both personas during the live gate → fallback fired for both → cause is in the orchestrator boot config, not the wiring. Re-verify env vars are sourced.

---

## Notes for implementer

- Phase 1's `var plannerLLM llm.Provider = plannerOR` pattern matters because Go infers types from the right-hand side. Without the explicit type annotation, `plannerLLM` would be `*openrouter.Provider`, and the conditional reassignment to `fallback.New(...)` would fail to compile.
- The `priorSealed` map in queue.go's RunNext is constructed fresh per task — don't try to share it across tasks.
- `composeCoderOutput`'s old signature (2 args) is no longer used anywhere outside `Review()` — the change is fully cascaded by updating the single caller.
- Reviewer's critique TEXT ("approve" / "flag" + reasoning) is unchanged by this milestone. Only the PROMPT going INTO the reviewer changes.
