# M6 — Agent Sharpness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make era sharper at finishing real tasks: bumped caps + per-task `--budget` profiles, smarter egress allowlist, reply-to-continue conversation threading, mid-run progress DMs, `/ask` read-only shortcut, `/stats` activity command.

**Architecture:** Six linear phases (AG → AL), each with its own live gate. Two new bidirectional surfaces: runner stdout streams `PROGRESS <json>` lines (consumed by orchestrator), and Telegram `reply_to_message_id` is detected to thread tasks. Three new SQLite columns (one per migration). One Telegram client API change (`SendMessage` returns `(int64, error)`). Zero changes to runner image, sidecar boot, or systemd unit.

**Tech Stack:** Go 1.25, SQLite via modernc.org/sqlite, sqlc v2, go-telegram-bot-api/v5, Docker. No new external dependencies.

**Spec:** `docs/superpowers/specs/2026-04-25-m6-agent-sharpness-design.md`.

**Testing philosophy:** Strict TDD. Fail-first tests before implementation. `go test -race -count=1 ./...` green before every commit. Per-phase smoke scripts kept for regression. Live Telegram smokes at every phase gate. Subagent-driven execution. We do not build blindly.

**Prerequisites (check before starting):**
- `sqlc` v2 installed (`which sqlc`).
- M5 CI pipeline green; pushes to master auto-deploy.
- Live VPS at `era@178.105.44.3` reachable.
- Telegram bot live and DMing your account.

---

## File Structure

```
migrations/
├── 0007_budget_profile.sql              CREATE (AG)
├── 0008_completion_message_id.sql       CREATE (AI)
└── 0009_read_only.sql                   CREATE (AK)

queries/tasks.sql                        MODIFY (AG, AI, AK, AL) — 8+ new sqlc queries

internal/db/
├── repo.go                              MODIFY — wrappers for new queries
└── (sqlc-regenerated)                   tasks.sql.go + models.go

internal/queue/
├── queue.go                             MODIFY (AG, AI, AJ, AK) — CreateTask signature; ProgressNotifier; CreateAskTask
├── budget.go                            CREATE (AG) — Profile + ParseBudgetFlag
├── budget_test.go                       CREATE (AG)
├── reply_compose.go                     CREATE (AI) — ComposeReplyPrompt
├── reply_compose_test.go                CREATE (AI)
├── stats.go                             CREATE (AL) — Stats + Queue.Stats
└── stats_test.go                        CREATE (AL)

internal/runner/
├── docker.go                            MODIFY (AG, AJ, AK) — RunInput cap fields + ReadOnly; ProgressCallback; streamToWithProgress
└── docker_test.go                       MODIFY — new field/path tests

internal/telegram/
├── client.go                            MODIFY (AI) — SendMessage returns (int64, error)
├── client_test.go                       MODIFY (AI) — FakeClient updated
├── handler.go                           MODIFY (AG, AI, AK, AL) — --budget parse; reply detection; /ask, /stats
└── handler_test.go                      MODIFY — new command coverage; stubOps updates

cmd/runner/
├── main.go                              MODIFY (AJ, AK) — wire onProgress callback; ERA_READ_ONLY path
├── pi.go                                MODIFY (AJ) — progressFunc param
├── pi_test.go                           MODIFY (AJ) — progressFunc tests
├── result.go                            MODIFY (AJ) — writeProgress + runProgress type
└── result_test.go                       MODIFY (AJ)

cmd/orchestrator/
└── main.go                              MODIFY (AI, AJ) — tgNotifier rename + repo + progressMsgs; NotifyProgress; wire ProgressNotifier; NewHandler new args

cmd/sidecar/
├── allowlist.go                         MODIFY (AH) — new static hosts + PI_EGRESS_EXTRA parser
└── allowlist_test.go                    MODIFY (AH)

scripts/smoke/
├── phase_ag_caps.sh                     CREATE
├── phase_ah_allowlist.sh                CREATE
├── phase_ai_reply.sh                    CREATE
├── phase_aj_progress.sh                 CREATE
├── phase_ak_ask.sh                      CREATE
└── phase_al_stats.sh                    CREATE

.env.example                             MODIFY (AG) — bumped defaults
deploy/env.template                      MODIFY (AG) — bumped defaults
README.md                                MODIFY (Final)
```

---

# Phase AG — Caps + budget profiles

**Goal:** Three named profiles (`quick`/`default`/`deep`); `/task --budget=NAME ...` overrides per-task; runner reads per-task caps via env vars; `RetryTask` inherits original profile + target_repo.

## Task AG-1: Migration 0007 + sqlc query + repo wrapper

**Files:**
- Create: `migrations/0007_budget_profile.sql`
- Modify: `queries/tasks.sql`
- Regenerate: `internal/db/tasks.sql.go` via sqlc
- Modify: `internal/db/repo.go`

- [ ] **Step 1: Write the migration.**

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN budget_profile TEXT NOT NULL DEFAULT 'default';

-- +goose Down
SELECT 1;
```

- [ ] **Step 2: Add sqlc queries.**

Append to `queries/tasks.sql`:

```sql
-- name: SetBudgetProfile :exec
UPDATE tasks SET budget_profile = ? WHERE id = ?;
```

Also modify the existing `CreateTask` query — the simplest path: leave CreateTask alone (default value handles new tasks), and use `SetBudgetProfile` to override. Profile default of `'default'` from the migration is the happy path.

Actually cleaner: extend `CreateTask` to accept profile too, since AG-3 changes `Queue.CreateTask` signature. Update the existing query:

```sql
-- name: CreateTask :one
INSERT INTO tasks (description, status, target_repo, budget_profile)
VALUES (?, 'queued', ?, ?)
RETURNING *;
```

Old callers passed 2 args (desc, target_repo). New takes 3. Cascades through repo wrapper.

- [ ] **Step 3: Regenerate sqlc.**

```
sqlc generate
```

Confirm `internal/db/tasks.sql.go` has updated `CreateTaskParams` (now includes `BudgetProfile string`) and new `SetBudgetProfile` method on Queries. `Task` struct in `models.go` has `BudgetProfile string`.

- [ ] **Step 4: Update repo wrapper.**

In `internal/db/repo.go`, locate the existing `CreateTask` wrapper. Update signature:

```go
func (r *Repo) CreateTask(ctx context.Context, desc, targetRepo, profile string) (Task, error) {
    return r.q.CreateTask(ctx, CreateTaskParams{
        Description:   desc,
        TargetRepo:    targetRepo,
        BudgetProfile: profile,
    })
}

func (r *Repo) SetBudgetProfile(ctx context.Context, id int64, profile string) error {
    return r.q.SetBudgetProfile(ctx, SetBudgetProfileParams{
        BudgetProfile: profile,
        ID:            id,
    })
}
```

- [ ] **Step 5: Verify compiles.**

```
go build ./...
```

Expected: errors at every existing `repo.CreateTask(ctx, desc, target)` 2-arg call site. We'll fix them in AG-3. For now, check the regenerated code is syntactically correct:

```
go vet ./internal/db/...
```

- [ ] **Step 6: Commit.**

```bash
git add migrations/0007_budget_profile.sql queries/tasks.sql internal/db/
git commit -m "feat(db): migration 0007 tasks.budget_profile + extend CreateTask + SetBudgetProfile"
```

## Task AG-2: `internal/queue/budget.go` Profile + ParseBudgetFlag

**Files:**
- Create: `internal/queue/budget.go`
- Create: `internal/queue/budget_test.go`

- [ ] **Step 1: Write failing tests.**

```go
package queue_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/vaibhav0806/era/internal/queue"
)

func TestProfiles_KnownNames(t *testing.T) {
    require.Contains(t, queue.Profiles, "quick")
    require.Contains(t, queue.Profiles, "default")
    require.Contains(t, queue.Profiles, "deep")
    require.Equal(t, 60, queue.Profiles["default"].MaxIter)
    require.Equal(t, 20, queue.Profiles["default"].MaxCents)
    require.Equal(t, 1800, queue.Profiles["default"].MaxWallSec)
}

func TestParseBudgetFlag_NoFlag(t *testing.T) {
    profile, desc := queue.ParseBudgetFlag("build something")
    require.Equal(t, "default", profile)
    require.Equal(t, "build something", desc)
}

func TestParseBudgetFlag_DeepFlag(t *testing.T) {
    profile, desc := queue.ParseBudgetFlag("--budget=deep build a complex thing")
    require.Equal(t, "deep", profile)
    require.Equal(t, "build a complex thing", desc)
}

func TestParseBudgetFlag_QuickFlag(t *testing.T) {
    profile, desc := queue.ParseBudgetFlag("--budget=quick foo")
    require.Equal(t, "quick", profile)
    require.Equal(t, "foo", desc)
}

func TestParseBudgetFlag_UnknownProfile(t *testing.T) {
    profile, desc := queue.ParseBudgetFlag("--budget=hyperultra do thing")
    require.Equal(t, "default", profile)
    require.Equal(t, "--budget=hyperultra do thing", desc, "unknown profile preserved in desc")
}

func TestParseBudgetFlag_NoSpaceAfter(t *testing.T) {
    profile, desc := queue.ParseBudgetFlag("--budget=deep")
    require.Equal(t, "default", profile)
    require.Equal(t, "--budget=deep", desc, "malformed (no trailing desc) — preserve")
}

func TestParseBudgetFlag_LeadingWhitespace(t *testing.T) {
    profile, desc := queue.ParseBudgetFlag("   --budget=deep do thing")
    require.Equal(t, "deep", profile)
    require.Equal(t, "do thing", desc)
}

func TestParseBudgetFlag_FlagInMiddle_NotMatched(t *testing.T) {
    // Flag must be the FIRST token; later --budget= in description is preserved.
    profile, desc := queue.ParseBudgetFlag("build a thing --budget=deep should be in desc")
    require.Equal(t, "default", profile)
    require.Equal(t, "build a thing --budget=deep should be in desc", desc)
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestProfiles_KnownNames ./internal/queue/
```

Expected: FAIL — package undefined.

- [ ] **Step 3: Implement.**

`internal/queue/budget.go`:

```go
package queue

import "strings"

type Profile struct {
    Name       string
    MaxIter    int
    MaxCents   int
    MaxWallSec int
}

// Profiles defines the three named budget presets. Caps are independent —
// a profile exceeding any one cap is considered exceeded.
var Profiles = map[string]Profile{
    "quick":   {"quick", 20, 5, 600},     // 10 min, 20 iters, $0.05
    "default": {"default", 60, 20, 1800}, // 30 min, 60 iters, $0.20
    "deep":    {"deep", 120, 100, 3600},  // 60 min, 120 iters, $1.00
}

// ParseBudgetFlag strips a leading `--budget=NAME` token from desc.
// Returns (profileName, cleanedDesc). Unknown profile names fall back to
// "default" with the description preserved as-is.
func ParseBudgetFlag(desc string) (string, string) {
    desc = strings.TrimSpace(desc)
    if !strings.HasPrefix(desc, "--budget=") {
        return "default", desc
    }
    end := strings.IndexByte(desc, ' ')
    if end < 0 {
        return "default", desc // malformed; no trailing space, preserve
    }
    name := strings.TrimPrefix(desc[:end], "--budget=")
    if _, ok := Profiles[name]; !ok {
        return "default", desc // unknown name, preserve whole desc
    }
    return name, strings.TrimSpace(desc[end+1:])
}
```

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestProfiles ./internal/queue/
go test -race -run TestParseBudgetFlag ./internal/queue/
```

- [ ] **Step 5: Commit.**

```bash
git add internal/queue/budget.go internal/queue/budget_test.go
git commit -m "feat(queue): Profile + ParseBudgetFlag for --budget=NAME parsing"
```

## Task AG-3: Update `Queue.CreateTask` signature + cascade

**Files:**
- Modify: `internal/queue/queue.go` (CreateTask + RetryTask)
- Modify: `internal/telegram/handler.go` (Ops interface + /task path)
- Modify: `internal/telegram/handler_test.go` (stubOps signature)
- Modify: `internal/queue/queue_test.go` + `queue_run_test.go` if they call CreateTask
- Modify: any other callers (grep)

- [ ] **Step 1: Find all callers.**

```
grep -rn "\.CreateTask(" --include="*.go" .
```

Expected callers: handler.go (`h.ops.CreateTask`), queue.go (`q.repo.CreateTask` from inside `Queue.CreateTask` and `RetryTask`), and tests.

- [ ] **Step 2: Update Queue.CreateTask.**

Change:

```go
func (q *Queue) CreateTask(ctx context.Context, desc, targetRepo string) (int64, error) {
    t, err := q.repo.CreateTask(ctx, desc, targetRepo)
    ...
}
```

to:

```go
func (q *Queue) CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error) {
    if profile == "" {
        profile = "default"
    }
    t, err := q.repo.CreateTask(ctx, desc, targetRepo, profile)
    ...
}
```

- [ ] **Step 3: Update Ops interface in handler.go.**

```go
type Ops interface {
    CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error)
    ...
}
```

Update handler.go `/task` route to parse the budget flag and pass profile:

```go
case strings.HasPrefix(text, "/task"):
    body := strings.TrimSpace(strings.TrimPrefix(text, "/task"))
    if body == "" {
        return h.client.SendMessage(ctx, u.ChatID, "usage: /task [--budget=quick|default|deep] [owner/repo] <description>")
    }
    repo, desc := parseTaskArgs(body)
    profile, desc := queue.ParseBudgetFlag(desc)
    id, err := h.ops.CreateTask(ctx, desc, repo, profile)
    if err != nil {
        return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
    }
    if repo != "" {
        return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d queued (repo: %s, profile: %s)", id, repo, profile))
    }
    return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d queued (profile: %s)", id, profile))
```

Add `"github.com/vaibhav0806/era/internal/queue"` import to handler.go if not present.

**Wait:** does parseTaskArgs already strip a budget flag? No — parseTaskArgs only extracts repo prefix. Order must be: `parseTaskArgs(body)` first to strip repo, THEN `ParseBudgetFlag(desc)` on the remainder. But if user types `/task --budget=deep vaibhav0806/foo build X`, the budget flag appears BEFORE the repo. That breaks our parser order.

**Fix:** parse budget first, then repo. The budget flag is a leading token unconditionally; repo prefix is whatever remains:

```go
profile, body := queue.ParseBudgetFlag(body)
repo, desc := parseTaskArgs(body)
```

Order matters. Update accordingly.

- [ ] **Step 4: Update Queue.RetryTask to inherit profile + target_repo.**

Locate `RetryTask` (~line 484):

```go
func (q *Queue) RetryTask(ctx context.Context, id int64) (int64, error) {
    orig, err := q.repo.GetTask(ctx, id)
    if err != nil { return 0, err }
    newTask, err := q.repo.CreateTask(ctx, orig.Description, "", "default")  // OLD
    ...
}
```

Replace the `CreateTask` line to inherit:

```go
newTask, err := q.repo.CreateTask(ctx, orig.Description, orig.TargetRepo, orig.BudgetProfile)
```

Both fields now exist on Task.

- [ ] **Step 5: Update test stubs.**

In `internal/telegram/handler_test.go`, `stubOps.CreateTask` signature:

```go
func (s *stubOps) CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error) {
    s.lastDesc = desc
    s.lastRepo = targetRepo
    s.lastProfile = profile
    return s.nextID, s.err
}
```

Add `lastProfile string` field.

In `internal/queue/queue_test.go` and `queue_run_test.go`, every `q.CreateTask(ctx, ...)` becomes `q.CreateTask(ctx, ..., "default")`. Use `"default"` as the third arg unless the test specifically wants another profile.

- [ ] **Step 6: Verify build + tests.**

```
go build ./...
go test -race -count=1 ./...
```

If there are compile errors, fix the call sites. Common ones: `queue.New(...)` test helpers that internally seed tasks via `repo.CreateTask` — bump those to 4-arg too.

- [ ] **Step 7: Commit.**

```bash
git add internal/queue/queue.go internal/telegram/handler.go internal/telegram/handler_test.go internal/queue/queue_test.go internal/queue/queue_run_test.go internal/queue/queue_pr_test.go internal/queue/queue_reject_test.go internal/queue/queue_approve_test.go internal/queue/queue_cancel_test.go
git commit -m "feat(queue): CreateTask + RetryTask carry budget_profile; handler parses --budget flag"
```

(The `git add` list is broad — if you find more files via `git status` after the build fix, add them too.)

## Task AG-4: Runner per-task cap overrides

**Files:**
- Modify: `internal/runner/docker.go` (RunInput, buildDockerArgs)
- Modify: `internal/runner/docker_test.go`
- Modify: `internal/runner/adapter.go` (resolve profile → caps, populate RunInput)
- Modify: `internal/queue/queue.go` (Runner interface adds optional caps; pass through)

The cleanest path: orchestrator looks up the profile from the task DB row, populates `RunInput.MaxIter/MaxCents/MaxWallSec`, runner emits per-container env overrides.

- [ ] **Step 1: Add fields to RunInput.**

In `internal/runner/docker.go`, find `type RunInput struct`. Add three fields (zero = use Docker struct default):

```go
type RunInput struct {
    TaskID        int64
    Repo          string
    Description   string
    GitHubToken   string
    ContainerName string
    MaxIter       int  // M6 AG: per-task override; 0 = use d.MaxIterations
    MaxCents      int  // 0 = use d.MaxCostCents
    MaxWallSec    int  // 0 = use d.MaxWallSeconds
}
```

- [ ] **Step 2: Update `buildDockerArgs` to emit per-task overrides.**

In `Docker.buildDockerArgs(in RunInput)`, find the existing `-e ERA_MAX_ITERATIONS=...` lines. Currently they read from `d.MaxIterations` etc. Change to honor RunInput overrides:

```go
maxIter := d.MaxIterations
if in.MaxIter > 0 { maxIter = in.MaxIter }
maxCents := d.MaxCostCents
if in.MaxCents > 0 { maxCents = in.MaxCents }
maxWall := d.MaxWallSeconds
if in.MaxWallSec > 0 { maxWall = in.MaxWallSec }

args = append(args,
    "-e", fmt.Sprintf("ERA_MAX_ITERATIONS=%d", maxIter),
    "-e", fmt.Sprintf("ERA_MAX_COST_CENTS=%d", maxCents),
    "-e", fmt.Sprintf("ERA_MAX_WALL_SECONDS=%d", maxWall),
)
```

- [ ] **Step 3: Write failing test.**

In `internal/runner/docker_test.go`:

```go
func TestBuildDockerArgs_PerTaskCapsOverrideDefaults(t *testing.T) {
    d := &Docker{
        Image:          "test:v1",
        MaxIterations:  30,
        MaxCostCents:   5,
        MaxWallSeconds: 600,
    }
    in := RunInput{
        TaskID:     1,
        Repo:       "o/r",
        Description: "x",
        MaxIter:    120,
        MaxCents:   100,
        MaxWallSec: 3600,
    }
    args := d.BuildDockerArgs(in)
    requireEnvSet(t, args, "ERA_MAX_ITERATIONS=120")
    requireEnvSet(t, args, "ERA_MAX_COST_CENTS=100")
    requireEnvSet(t, args, "ERA_MAX_WALL_SECONDS=3600")
}

func TestBuildDockerArgs_ZeroFieldsFallBackToDocker(t *testing.T) {
    d := &Docker{
        Image:          "test:v1",
        MaxIterations:  60,
        MaxCostCents:   20,
        MaxWallSeconds: 1800,
    }
    in := RunInput{TaskID: 1, Repo: "o/r"}
    args := d.BuildDockerArgs(in)
    requireEnvSet(t, args, "ERA_MAX_ITERATIONS=60")
    requireEnvSet(t, args, "ERA_MAX_COST_CENTS=20")
    requireEnvSet(t, args, "ERA_MAX_WALL_SECONDS=1800")
}

// helper
func requireEnvSet(t *testing.T, args []string, want string) {
    t.Helper()
    for i, a := range args {
        if a == "-e" && i+1 < len(args) && args[i+1] == want {
            return
        }
    }
    t.Fatalf("expected %s in args; got: %v", want, args)
}
```

- [ ] **Step 4: Verify fail then pass.**

```
go test -race -run TestBuildDockerArgs_ ./internal/runner/
```

Expected: first FAIL, then after Step 2 passes.

- [ ] **Step 5: Update QueueAdapter to populate RunInput caps.**

In `internal/runner/adapter.go`, locate `QueueAdapter.Run`. Currently signature includes taskID, desc, ghToken, repo. Need to know the task's profile to look up caps. Two approaches:

**A.** Change adapter signature to accept caps.
**B.** Change Queue.Runner interface to pass caps.

Plan picks **A** but cleanly — extend the `runner.Runner` interface defined in `internal/queue/queue.go`:

```go
type Runner interface {
    Run(ctx context.Context, taskID int64, description string, ghToken string, repo string,
        maxIter, maxCents, maxWallSec int) (branch, summary string, tokens int64, costCents int, audits []audit.Entry, err error)
}
```

Yes that's a wide interface. Acceptable for one milestone — alternative is a Caps struct, but six int args isn't ugly enough to warrant a struct.

In `internal/queue/queue.go` `RunNext`, before calling `q.runner.Run(...)`, look up profile:

```go
profile := queue.Profiles[t.BudgetProfile]
if profile.Name == "" {
    profile = queue.Profiles["default"] // unknown stored profile; safe fallback
}
branch, summary, tokens, costCents, audits, runErr := q.runner.Run(
    ctx, t.ID, t.Description, ghToken, effectiveRepo,
    profile.MaxIter, profile.MaxCents, profile.MaxWallSec,
)
```

Update `QueueAdapter.Run` to take + pass through:

```go
func (q *QueueAdapter) Run(ctx context.Context, taskID int64, description, ghToken, repo string,
    maxIter, maxCents, maxWallSec int) (string, string, int64, int, []audit.Entry, error) {
    name := fmt.Sprintf("era-runner-%d-%d", taskID, time.Now().UnixNano())
    if q.running != nil {
        q.running.Register(taskID, name)
        defer q.running.Deregister(taskID)
    }
    out, err := q.D.Run(ctx, RunInput{
        TaskID:        taskID,
        Repo:          repo,
        Description:   description,
        GitHubToken:   ghToken,
        ContainerName: name,
        MaxIter:       maxIter,
        MaxCents:      maxCents,
        MaxWallSec:    maxWallSec,
    })
    ...
}
```

Test stubs (`fakeRunner` in queue_run_test.go) gain the three new params + capture them:

```go
func (f *fakeRunner) Run(ctx context.Context, taskID int64, desc, ghToken, repo string, maxIter, maxCents, maxWallSec int) (string, string, int64, int, []audit.Entry, error) {
    f.calls++
    f.lastID = taskID
    f.lastMaxIter = maxIter
    f.lastMaxCents = maxCents
    f.lastMaxWallSec = maxWallSec
    ...
}
```

Add `lastMaxIter, lastMaxCents, lastMaxWallSec int` fields to fakeRunner.

- [ ] **Step 6: Test cap-passthrough end-to-end via Queue.RunNext.**

Add to `queue_run_test.go`:

```go
func TestQueue_RunNext_DeepProfilePassesThroughCaps(t *testing.T) {
    ctx := context.Background()
    fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
    q, repo := newRunQueue(t, fr)
    _, err := repo.CreateTask(ctx, "x", "", "deep")
    require.NoError(t, err)
    _, err = q.RunNext(ctx)
    require.NoError(t, err)
    require.Equal(t, 120, fr.lastMaxIter)
    require.Equal(t, 100, fr.lastMaxCents)
    require.Equal(t, 3600, fr.lastMaxWallSec)
}
```

- [ ] **Step 7: Verify + commit.**

```
go test -race -count=1 ./...
git add internal/runner/docker.go internal/runner/docker_test.go internal/runner/adapter.go internal/queue/queue.go internal/queue/queue_run_test.go
git commit -m "feat(runner,queue): per-task cap overrides resolved from budget profile"
```

## Task AG-5: Bump default env caps

**Files:**
- Modify: `.env.example`
- Modify: `deploy/env.template`

- [ ] **Step 1: Update .env.example.**

Find lines:
```
PI_MAX_ITERATIONS=30
PI_MAX_COST_CENTS=5
PI_MAX_WALL_SECONDS=3600
```
Replace:
```
PI_MAX_ITERATIONS=60
PI_MAX_COST_CENTS=20
PI_MAX_WALL_SECONDS=1800
```

- [ ] **Step 2: Update deploy/env.template.**

Same change, same lines.

- [ ] **Step 3: Verify the live VPS .env reflects the bump.**

Note in commit body: "Live VPS /etc/era/env still has the old values; manual update needed via ssh era@... and edit. CI deploys don't touch /etc/era/env."

For the M6 live gate, the live env values should be updated. Plan in AG-8 includes a manual ssh step.

- [ ] **Step 4: Commit.**

```bash
git add .env.example deploy/env.template
git commit -m "feat(env): bump default caps (60 iter / 20 cents / 30 min) to match M6 default profile"
```

## Task AG-6: Live VPS env update + Phase AG smoke + gate

**Files:**
- Create: `scripts/smoke/phase_ag_caps.sh`

- [ ] **Step 1: Update live /etc/era/env.**

```
ssh era@178.105.44.3 'sed -i \
    -e "s/^PI_MAX_ITERATIONS=.*/PI_MAX_ITERATIONS=60/" \
    -e "s/^PI_MAX_COST_CENTS=.*/PI_MAX_COST_CENTS=20/" \
    -e "s/^PI_MAX_WALL_SECONDS=.*/PI_MAX_WALL_SECONDS=1800/" \
    /etc/era/env'
ssh era@178.105.44.3 'grep PI_MAX /etc/era/env'
```

Expected three lines printing the new values.

- [ ] **Step 2: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AG smoke: budget profile lib + flag parser + CreateTask cascade.
set -euo pipefail
go test -race -count=1 -run 'TestProfiles_|TestParseBudgetFlag_|TestQueue_RunNext_DeepProfilePassesThroughCaps|TestBuildDockerArgs_PerTaskCaps' \
    ./internal/queue/... ./internal/runner/... > /dev/null
echo "OK: phase AG — caps + budget profiles unit tests green"
```

- [ ] **Step 3: Make executable + run.**

```
chmod +x scripts/smoke/phase_ag_caps.sh
bash scripts/smoke/phase_ag_caps.sh
```

- [ ] **Step 4: Push to master (CI auto-deploys).**

```
git push origin master
gh run watch --exit-status
```

Expected: green test, green deploy.

- [ ] **Step 5: Live gate via Telegram.**

```
/task --budget=deep vaibhav0806/trying-something add a single file BUDGET_TEST.md with one line saying "budget=deep test"
```

Expected DM: `task #N queued (repo: vaibhav0806/trying-something, profile: deep)`. Task completes (Pi has 120 iter ceiling now). PR opens.

Verify caps actually applied — DB row:
```
ssh era@178.105.44.3 'sqlite3 /opt/era/pi-agent.db "SELECT id, budget_profile FROM tasks ORDER BY id DESC LIMIT 1;"'
```

Expected output: `<id>|deep`.

- [ ] **Step 6: Commit smoke script.**

```bash
git add scripts/smoke/phase_ag_caps.sh
git commit -m "docs(smoke): phase AG caps + budget profiles"
```

---

# Phase AH — Egress allowlist expansion

**Goal:** Add ten common dev hosts to the static allowlist; add `PI_EGRESS_EXTRA` env var for per-deployment additions without recompile.

## Task AH-1: Static allowlist additions + tests

**Files:**
- Modify: `cmd/sidecar/allowlist.go`
- Modify: `cmd/sidecar/allowlist_test.go`

- [ ] **Step 1: Write failing tests.**

In `cmd/sidecar/allowlist_test.go`, in the existing `TestAllowlist_StaticHostsAllowed` (or equivalent), add assertions:

```go
require.True(t, a.allowed("crates.io"))
require.True(t, a.allowed("static.crates.io"))
require.True(t, a.allowed("index.crates.io"))
require.True(t, a.allowed("registry.yarnpkg.com"))
require.True(t, a.allowed("cdn.jsdelivr.net"))
require.True(t, a.allowed("cdnjs.cloudflare.com"))
require.True(t, a.allowed("unpkg.com"))
require.True(t, a.allowed("fonts.googleapis.com"))
require.True(t, a.allowed("fonts.gstatic.com"))
require.True(t, a.allowed("services.gradle.org"))
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestAllowlist_StaticHostsAllowed ./cmd/sidecar/
```

Expected: fails on the new lines.

- [ ] **Step 3: Add to staticHosts.**

In `cmd/sidecar/allowlist.go`, locate the `staticHosts` slice or var. Append a new section:

```go
// M6 AH: common dev ecosystem hosts
"crates.io", "static.crates.io", "index.crates.io",
"registry.yarnpkg.com",
"cdn.jsdelivr.net", "cdnjs.cloudflare.com", "unpkg.com",
"fonts.googleapis.com", "fonts.gstatic.com",
"services.gradle.org",
```

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestAllowlist_ ./cmd/sidecar/
```

- [ ] **Step 5: Commit.**

```bash
git add cmd/sidecar/allowlist.go cmd/sidecar/allowlist_test.go
git commit -m "feat(sidecar): allowlist 10 common dev ecosystem hosts (crates, yarn, jsdelivr, fonts, gradle)"
```

## Task AH-2: `PI_EGRESS_EXTRA` env var parser

**Files:**
- Modify: `cmd/sidecar/allowlist.go`
- Modify: `cmd/sidecar/allowlist_test.go`

- [ ] **Step 1: Write failing tests.**

```go
func TestPIEgressExtra_AppendsHosts(t *testing.T) {
    t.Setenv("PI_EGRESS_EXTRA", "foo.example.com,bar.example.org")
    a := newAllowlist()
    require.True(t, a.allowed("foo.example.com"))
    require.True(t, a.allowed("bar.example.org"))
}

func TestPIEgressExtra_EmptyWhitespaceSkipped(t *testing.T) {
    t.Setenv("PI_EGRESS_EXTRA", "foo.example.com, ,  bar.example.org , ")
    a := newAllowlist()
    require.True(t, a.allowed("foo.example.com"))
    require.True(t, a.allowed("bar.example.org"))
    // No empty-string host accidentally allowed
    require.False(t, a.allowed(""))
}

func TestPIEgressExtra_Unset_NoChange(t *testing.T) {
    t.Setenv("PI_EGRESS_EXTRA", "")
    a := newAllowlist()
    require.False(t, a.allowed("notinlist.example.com"))
}
```

(`newAllowlist()` is whatever the existing constructor is — match the actual function name.)

- [ ] **Step 2: Verify fail.**

```
go test -run TestPIEgressExtra ./cmd/sidecar/
```

- [ ] **Step 3: Implement.**

In `cmd/sidecar/allowlist.go`, in the constructor (or the package init that builds the allowlist):

```go
// M6 AH: PI_EGRESS_EXTRA appends comma-separated hosts at runtime.
if extra := os.Getenv("PI_EGRESS_EXTRA"); extra != "" {
    for _, h := range strings.Split(extra, ",") {
        h = strings.TrimSpace(h)
        if h != "" {
            staticHosts = append(staticHosts, h)
        }
    }
}
```

Add `"os"` and `"strings"` to imports if not present. Note: if the allowlist construction is done at package init time, the env var must be read there; if it's per-call, read it once and cache. Match the existing code's lifecycle.

- [ ] **Step 4: Verify pass + full regression.**

```
go test -race -count=1 ./...
```

- [ ] **Step 5: Commit.**

```bash
git add cmd/sidecar/allowlist.go cmd/sidecar/allowlist_test.go
git commit -m "feat(sidecar): PI_EGRESS_EXTRA env appends comma-separated hosts at runtime"
```

## Task AH-3: Phase AH smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_ah_allowlist.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AH smoke: new static hosts + PI_EGRESS_EXTRA parser unit tests.
set -euo pipefail
go test -race -count=1 -run 'TestAllowlist_StaticHostsAllowed|TestPIEgressExtra_' \
    ./cmd/sidecar/... > /dev/null
echo "OK: phase AH — egress allowlist expansion all unit tests green"
```

- [ ] **Step 2: Run + commit.**

```
chmod +x scripts/smoke/phase_ah_allowlist.sh
bash scripts/smoke/phase_ah_allowlist.sh
git add scripts/smoke/phase_ah_allowlist.sh
git commit -m "docs(smoke): phase AH egress allowlist"
```

- [ ] **Step 3: Push to master (CI deploys).**

```
git push origin master
gh run watch --exit-status
```

- [ ] **Step 4: Live gate.**

A task that uses one of the new hosts. Rust is the cleanest test:
```
/task --budget=deep vaibhav0806/ad-smoke create a hello-world Rust binary. Use cargo new (in a temp dir, then move files into root). main.rs should print "phase AH ok". Include Cargo.toml. Run cargo build to confirm it works.
```

Expected: completes, PR opens. Audit log shows `crates.io` / `static.crates.io` calls succeeded (200, not 403). Verify:

```
ssh era@178.105.44.3 'sqlite3 /opt/era/pi-agent.db "SELECT payload FROM events WHERE task_id=(SELECT MAX(id) FROM tasks) AND kind='"'"'http_request'"'"';"' | grep -E "crates|jsdelivr" | head -5
```

Expected: 200 status entries for crates hosts.

---

# Phase AI — Reply-to-continue threading

**Goal:** Migration 0008 + completion message ID stored on every completion DM. Telegram client `SendMessage` returns `(int64, error)`. Handler detects replies and threads original task context into a new queued task.

## Task AI-1: Migration 0008 + sqlc queries + repo wrappers

**Files:**
- Create: `migrations/0008_completion_message_id.sql`
- Modify: `queries/tasks.sql`
- Modify: `internal/db/repo.go`
- Regenerate: sqlc

- [ ] **Step 1: Write the migration.**

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN completion_message_id INTEGER;

-- +goose Down
SELECT 1;
```

- [ ] **Step 2: Add sqlc queries.**

Append to `queries/tasks.sql`:

```sql
-- name: SetCompletionMessageID :exec
UPDATE tasks SET completion_message_id = ? WHERE id = ?;

-- name: GetTaskByCompletionMessageID :one
SELECT * FROM tasks WHERE completion_message_id = ? LIMIT 1;
```

- [ ] **Step 3: Regenerate.**

```
sqlc generate
```

Confirm `tasks.sql.go` has `SetCompletionMessageID` + `GetTaskByCompletionMessageID` methods. The `Task` struct has `CompletionMessageID sql.NullInt64`.

- [ ] **Step 4: Add repo wrappers.**

```go
func (r *Repo) SetCompletionMessageID(ctx context.Context, id, msgID int64) error {
    return r.q.SetCompletionMessageID(ctx, SetCompletionMessageIDParams{
        CompletionMessageID: sql.NullInt64{Int64: msgID, Valid: true},
        ID:                  id,
    })
}

func (r *Repo) GetTaskByCompletionMessageID(ctx context.Context, msgID int64) (Task, error) {
    return r.q.GetTaskByCompletionMessageID(ctx, sql.NullInt64{Int64: msgID, Valid: true})
}
```

(Exact param struct names depend on sqlc output.)

- [ ] **Step 5: Add a unit test.**

In `internal/db/repo_test.go`, add:

```go
func TestRepo_CompletionMessageID_RoundTrip(t *testing.T) {
    repo, cleanup := newTestRepo(t)
    defer cleanup()
    task, err := repo.CreateTask(context.Background(), "test", "", "default")
    require.NoError(t, err)
    require.NoError(t, repo.SetCompletionMessageID(context.Background(), task.ID, 12345))
    got, err := repo.GetTaskByCompletionMessageID(context.Background(), 12345)
    require.NoError(t, err)
    require.Equal(t, task.ID, got.ID)
}
```

- [ ] **Step 6: Verify + commit.**

```
go test -race ./internal/db/
git add migrations/0008_completion_message_id.sql queries/tasks.sql internal/db/
git commit -m "feat(db): migration 0008 tasks.completion_message_id + reply lookup query"
```

## Task AI-2: `Telegram.Client.SendMessage` signature change

**Files:**
- Modify: `internal/telegram/client.go`
- Modify: `internal/telegram/client_test.go`
- Modify: every caller — grep first

- [ ] **Step 1: Find all callers.**

```
grep -rn "\.SendMessage(" --include="*.go" .
```

Expected files: `cmd/orchestrator/main.go`, `internal/telegram/handler.go`, `internal/telegram/client_test.go`, `internal/digest/` (if it sends DMs).

Capture the list — each will need updating in this same commit.

- [ ] **Step 2: Update interface.**

In `internal/telegram/client.go`:

```go
type Client interface {
    SendMessage(ctx context.Context, chatID int64, text string) (int64, error)
    SendMessageWithButtons(ctx context.Context, chatID int64, text string, buttons [][]InlineButton) (messageID int, err error)
    EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error
}
```

- [ ] **Step 3: Update realClient impl.**

```go
func (c *realClient) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
    msg := tgbotapi.NewMessage(chatID, text)
    sent, err := c.bot.Send(msg)
    if err != nil {
        return 0, err
    }
    return int64(sent.MessageID), nil
}
```

- [ ] **Step 4: Update FakeClient.**

```go
func (f *FakeClient) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.nextMsgID++
    f.sentMessages = append(f.sentMessages, FakeSentMessage{
        ChatID: chatID, Text: text, MessageID: f.nextMsgID,
    })
    return f.nextMsgID, nil
}
```

(Add `nextMsgID int64` and update `FakeSentMessage` with `MessageID int64` field. Existing tests asserting on `f.sentMessages[i].Text` still work; tests asserting on `MessageID` are new.)

- [ ] **Step 5: Update existing FakeClient tests.**

In `client_test.go`, find `TestFakeClient_SendMessage` (or equivalent). Update to assert the new return shape:

```go
func TestFakeClient_SendMessage(t *testing.T) {
    f := NewFakeClient()
    msgID, err := f.SendMessage(context.Background(), 1, "hello")
    require.NoError(t, err)
    require.Greater(t, msgID, int64(0))
    require.Len(t, f.sentMessages, 1)
    require.Equal(t, "hello", f.sentMessages[0].Text)
}
```

- [ ] **Step 6: Mechanically fix every caller.**

Cascade fixes (each line: original → replacement):

In `internal/telegram/handler.go`, every:
```go
return h.client.SendMessage(ctx, u.ChatID, "...")
```
becomes:
```go
_, err := h.client.SendMessage(ctx, u.ChatID, "...")
return err
```

In `cmd/orchestrator/main.go`, in tgNotifier methods — same mechanical fix. Where currently:
```go
if err := n.client.SendMessage(ctx, n.chatID, msg); err != nil { ... }
```
becomes:
```go
_, err := n.client.SendMessage(ctx, n.chatID, msg)
if err != nil { ... }
```

NOTE: `tgNotifier.NotifyCompleted` will be the ONE caller in AI-3 that captures the message ID instead of discarding. For now, just `_, err :=` and capture in AI-3.

In `internal/digest/...` (if applicable) — mechanical too.

- [ ] **Step 7: Build + test.**

```
go build ./...
go test -race -count=1 ./...
```

- [ ] **Step 8: Commit.**

```bash
git add internal/telegram/client.go internal/telegram/client_test.go internal/telegram/handler.go cmd/orchestrator/main.go internal/digest/
git commit -m "refactor(telegram): SendMessage returns (int64, error); cascade _, err := through callers"
```

## Task AI-3: tgNotifier rename `repo` → `sandboxRepo` + add `repo *db.Repo` + store completion msg ID

**Files:**
- Modify: `cmd/orchestrator/main.go`

- [ ] **Step 1: Find current tgNotifier.**

```
grep -n "type tgNotifier\|n\.repo\|repo:.*cfg.GitHubSandboxRepo" cmd/orchestrator/main.go
```

The struct (currently around line 174) has `client, chatID, repo string`. Construction site (around line 111-115) passes `repo: cfg.GitHubSandboxRepo`. Methods read `n.repo` for URL fallback in `NotifyCompleted`.

- [ ] **Step 2: Rename + add fields.**

```go
type tgNotifier struct {
    client       telegram.Client
    chatID       int64
    sandboxRepo  string                  // renamed from `repo`
    repo         *db.Repo                // new: for SetCompletionMessageID
    progressMsgs sync.Map                // added later in AJ; declare here so Notify methods can use
}
```

Add `"sync"` and `"github.com/vaibhav0806/era/internal/db"` imports if not present.

- [ ] **Step 3: Update construction.**

In main(), the `q.SetNotifier(&tgNotifier{...})` line:

```go
q.SetNotifier(&tgNotifier{
    client:      client,
    chatID:      cfg.TelegramAllowedUserID,
    sandboxRepo: cfg.GitHubSandboxRepo,
    repo:        repo,  // new
})
```

- [ ] **Step 4: Replace `n.repo` → `n.sandboxRepo` in methods.**

```
grep -n "n\.repo" cmd/orchestrator/main.go
```

Inside `NotifyCompleted`:
```go
if repo == "" {
    repo = n.sandboxRepo  // was: n.repo
}
```

- [ ] **Step 5: Capture msg ID in NotifyCompleted; persist via repo.**

In `tgNotifier.NotifyCompleted`, replace the `_, err := n.client.SendMessage(...)` with:

```go
msgID, err := n.client.SendMessage(ctx, n.chatID, msg)
if err != nil {
    slog.Error("notify completed", "err", err, "task", id)
    return
}
if err := n.repo.SetCompletionMessageID(ctx, id, msgID); err != nil {
    slog.Warn("set completion message id", "err", err, "task", id)
}
```

- [ ] **Step 6: Verify build.**

```
go build ./...
go test -race -count=1 ./...
```

- [ ] **Step 7: Commit.**

```bash
git add cmd/orchestrator/main.go
git commit -m "refactor(orchestrator): rename tgNotifier.repo → sandboxRepo; add db.Repo + persist completion msg id"
```

## Task AI-4: ComposeReplyPrompt helper

**Files:**
- Create: `internal/queue/reply_compose.go`
- Create: `internal/queue/reply_compose_test.go`

- [ ] **Step 1: Write failing tests.**

```go
package queue_test

import (
    "database/sql"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/vaibhav0806/era/internal/db"
    "github.com/vaibhav0806/era/internal/queue"
)

func TestComposeReplyPrompt_HappyPath(t *testing.T) {
    orig := db.Task{
        ID:          5,
        Description: "build a URL shortener",
        BranchName:  sql.NullString{String: "agent/5/foo", Valid: true},
        PrNumber:    sql.NullInt64{Int64: 12, Valid: true},
        Summary:     sql.NullString{String: "I built it", Valid: true},
        Status:      "completed",
    }
    out := queue.ComposeReplyPrompt(orig, "now add tests")
    require.Contains(t, out, "task #5")
    require.Contains(t, out, "build a URL shortener")
    require.Contains(t, out, "agent/5/foo")
    require.Contains(t, out, "#12")
    require.Contains(t, out, "I built it")
    require.Contains(t, out, "now add tests")
}

func TestComposeReplyPrompt_NoBranchNoSummary(t *testing.T) {
    orig := db.Task{
        ID:          7,
        Description: "what is in main.go",
        Status:      "completed",
    }
    out := queue.ComposeReplyPrompt(orig, "tell me more")
    require.Contains(t, out, "task #7")
    require.Contains(t, out, "tell me more")
    require.NotContains(t, out, "branch")
    require.NotContains(t, out, "PR")
}

func TestComposeReplyPrompt_FailedTask(t *testing.T) {
    orig := db.Task{
        ID:          9,
        Description: "thing that broke",
        Status:      "failed",
        Error:       sql.NullString{String: "exit status 137", Valid: true},
    }
    out := queue.ComposeReplyPrompt(orig, "try again with smaller scope")
    require.Contains(t, out, "failed")
    require.Contains(t, out, "exit status 137")
    require.Contains(t, out, "try again")
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestComposeReplyPrompt_ ./internal/queue/
```

- [ ] **Step 3: Implement.**

`internal/queue/reply_compose.go`:

```go
package queue

import (
    "fmt"
    "strings"

    "github.com/vaibhav0806/era/internal/db"
)

// ComposeReplyPrompt builds the prompt for a reply-threaded task.
// Non-transitive: orig is the task the user replied to, not a chain.
func ComposeReplyPrompt(orig db.Task, replyBody string) string {
    var b strings.Builder
    fmt.Fprintf(&b, "You previously completed task #%d: %q\n", orig.ID, orig.Description)
    if orig.BranchName.Valid && orig.BranchName.String != "" {
        fmt.Fprintf(&b, "You made changes on branch %s.\n", orig.BranchName.String)
    }
    if orig.PrNumber.Valid {
        fmt.Fprintf(&b, "The pull request is #%d.\n", orig.PrNumber.Int64)
    }
    if orig.Summary.Valid && strings.TrimSpace(orig.Summary.String) != "" {
        fmt.Fprintf(&b, "\nSummary of what you did:\n%s\n", orig.Summary.String)
    }
    if orig.Status == "failed" && orig.Error.Valid {
        fmt.Fprintf(&b, "\nThat task failed with: %s\n", orig.Error.String)
    }
    fmt.Fprintf(&b, "\nNow the user has a follow-up: %s", replyBody)
    return b.String()
}
```

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestComposeReplyPrompt_ ./internal/queue/
```

- [ ] **Step 5: Commit.**

```bash
git add internal/queue/reply_compose.go internal/queue/reply_compose_test.go
git commit -m "feat(queue): ComposeReplyPrompt threads original task into follow-up prompt"
```

## Task AI-5: Handler struct gains repo + sandboxRepo fields

**Files:**
- Modify: `internal/telegram/handler.go`
- Modify: `cmd/orchestrator/main.go`
- Modify: `internal/telegram/handler_test.go`

- [ ] **Step 1: Update Handler struct + NewHandler.**

In `internal/telegram/handler.go`:

```go
type Handler struct {
    client      Client
    ops         Ops
    repo        *db.Repo  // new in AI — for GetTaskByCompletionMessageID
    sandboxRepo string    // new in AI — for reply-DM fallback when target_repo empty
}

func NewHandler(c Client, ops Ops, repo *db.Repo, sandboxRepo string) *Handler {
    return &Handler{client: c, ops: ops, repo: repo, sandboxRepo: sandboxRepo}
}
```

Add `"github.com/vaibhav0806/era/internal/db"` import.

- [ ] **Step 2: Update construction in main.go.**

In `cmd/orchestrator/main.go`, the line:
```go
handler := telegram.NewHandler(client, q)
```
becomes:
```go
handler := telegram.NewHandler(client, q, repo, cfg.GitHubSandboxRepo)
```

- [ ] **Step 3: Update test stubs.**

In `internal/telegram/handler_test.go`, every `NewHandler(c, ops)` call gains two args:

```go
h := NewHandler(c, ops, nil, "vaibhav0806/sandbox")
```

(`nil` for repo is fine in tests that don't exercise reply paths; reply-specific tests will set up an in-memory db.Repo.)

- [ ] **Step 4: Verify build.**

```
go build ./...
go test -race -count=1 ./...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/telegram/handler.go internal/telegram/handler_test.go cmd/orchestrator/main.go
git commit -m "feat(telegram): Handler gains repo + sandboxRepo for reply-threading"
```

## Task AI-6: Handler routes reply messages

**Files:**
- Modify: `internal/telegram/handler.go`
- Modify: `internal/telegram/handler_test.go`
- Modify: `internal/telegram/client.go` (Update struct may need ReplyToMessageID if not already present)

- [ ] **Step 1: Confirm Update struct has ReplyToMessageID.**

```
grep -n "type Update\|ReplyToMessageID" internal/telegram/*.go
```

If absent, add `ReplyToMessageID int` field to `Update`. The realClient's update polling already gets this from `tgbotapi.Update.Message.ReplyToMessage.MessageID`. Map it through.

- [ ] **Step 2: Write failing tests.**

In `internal/telegram/handler_test.go`:

```go
func TestHandler_ReplyToUnknownMessage_DMsNotFound(t *testing.T) {
    ctx := context.Background()
    f := NewFakeClient()
    h := NewHandler(f, &stubOps{}, newInMemRepo(t), "vaibhav0806/sandbox")
    err := h.Handle(ctx, Update{
        ChatID: 1,
        Text:   "now add tests",
        ReplyToMessageID: 99999, // nothing in DB matches
    })
    require.NoError(t, err)
    require.Len(t, f.sentMessages, 1)
    require.Contains(t, f.sentMessages[0].Text, "couldn't find")
}

func TestHandler_ReplyToKnownMessage_QueuesThreadedTask(t *testing.T) {
    ctx := context.Background()
    repo := newInMemRepo(t)
    // Seed a task with completion_message_id=12345
    task, err := repo.CreateTask(ctx, "build a thing", "vaibhav0806/foo", "default")
    require.NoError(t, err)
    require.NoError(t, repo.SetCompletionMessageID(ctx, task.ID, 12345))

    f := NewFakeClient()
    ops := &stubOps{nextID: 99}
    h := NewHandler(f, ops, repo, "vaibhav0806/sandbox")
    err = h.Handle(ctx, Update{
        ChatID: 1,
        Text:   "now add tests",
        ReplyToMessageID: 12345,
    })
    require.NoError(t, err)
    require.Equal(t, "vaibhav0806/foo", ops.lastRepo)
    require.Contains(t, ops.lastDesc, "previously completed task #")
    require.Contains(t, ops.lastDesc, "now add tests")
    require.Contains(t, f.sentMessages[0].Text, "task #99 queued")
    require.Contains(t, f.sentMessages[0].Text, "reply to #")
}

func TestHandler_ReplyWithCommandPrefix_FallsThroughToCommand(t *testing.T) {
    // /list is a command — prefix wins over reply detection
    ctx := context.Background()
    f := NewFakeClient()
    h := NewHandler(f, &stubOps{}, newInMemRepo(t), "vaibhav0806/sandbox")
    err := h.Handle(ctx, Update{
        ChatID: 1,
        Text:   "/list",
        ReplyToMessageID: 12345,
    })
    require.NoError(t, err)
    // Should NOT DM "couldn't find" — should run /list path
    require.NotContains(t, f.sentMessages[0].Text, "couldn't find")
}
```

`newInMemRepo(t)` is a helper — if not present, write it (see existing pattern in `internal/db/repo_test.go`).

- [ ] **Step 3: Verify fail.**

```
go test -run 'TestHandler_Reply' ./internal/telegram/
```

- [ ] **Step 4: Implement reply detection.**

In `internal/telegram/handler.go` `Handle` method, BEFORE the existing prefix routing:

```go
func (h *Handler) Handle(ctx context.Context, u Update) error {
    // M6 AI: reply-to-continue. A non-command reply threads a follow-up task.
    if u.ReplyToMessageID != 0 && !strings.HasPrefix(u.Text, "/") {
        return h.handleReply(ctx, u)
    }
    // ... existing prefix routes ...
}

func (h *Handler) handleReply(ctx context.Context, u Update) error {
    orig, err := h.repo.GetTaskByCompletionMessageID(ctx, int64(u.ReplyToMessageID))
    if errors.Is(err, sql.ErrNoRows) {
        _, err := h.client.SendMessage(ctx, u.ChatID,
            "sorry, couldn't find the task you're replying to")
        return err
    }
    if err != nil {
        return fmt.Errorf("get task by message id: %w", err)
    }
    prompt := queue.ComposeReplyPrompt(orig, u.Text)
    targetRepo := orig.TargetRepo
    if targetRepo == "" {
        targetRepo = h.sandboxRepo
    }
    id, err := h.ops.CreateTask(ctx, prompt, targetRepo, "default")
    if err != nil {
        return fmt.Errorf("queue reply task: %w", err)
    }
    _, err = h.client.SendMessage(ctx, u.ChatID,
        fmt.Sprintf("task #%d queued (reply to #%d, repo: %s)", id, orig.ID, targetRepo))
    return err
}
```

Add `"errors"`, `"database/sql"`, `"github.com/vaibhav0806/era/internal/queue"`, `"github.com/vaibhav0806/era/internal/db"` to imports if not present.

- [ ] **Step 5: Verify pass + full regression.**

```
go test -race -count=1 ./...
```

- [ ] **Step 6: Commit.**

```bash
git add internal/telegram/handler.go internal/telegram/handler_test.go internal/telegram/client.go
git commit -m "feat(telegram): handleReply threads follow-up tasks via reply_to_message_id"
```

## Task AI-7: Phase AI smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_ai_reply.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AI smoke: completion_message_id round-trip + ComposeReplyPrompt + handler reply detection.
set -euo pipefail
go test -race -count=1 -run 'TestRepo_CompletionMessageID_|TestComposeReplyPrompt_|TestHandler_Reply|TestFakeClient_SendMessage' \
    ./internal/db/... ./internal/queue/... ./internal/telegram/... > /dev/null
echo "OK: phase AI — reply threading all unit tests green"
```

- [ ] **Step 2: Run + commit + push.**

```
chmod +x scripts/smoke/phase_ai_reply.sh
bash scripts/smoke/phase_ai_reply.sh
git add scripts/smoke/phase_ai_reply.sh
git commit -m "docs(smoke): phase AI reply threading"
git push origin master
gh run watch --exit-status
```

- [ ] **Step 3: Live gate.**

Step 3a: complete a task to get a completion DM:
```
/task vaibhav0806/ad-smoke add a file REPLY_TEST.md saying "phase AI"
```
Wait for completion DM. Save the DM message somewhere visible (it's the target you'll reply to).

Step 3b: long-press the completion DM in Telegram → Reply → type:
```
now add another file REPLY_TEST_2.md saying "thread"
```
Send.

Expected:
- Reply DM: `task #N queued (reply to #M, repo: vaibhav0806/ad-smoke)`
- New task #N completes — Pi's prompt includes the original task's context (you can verify by looking at the orchestrator log briefly: `ssh era@... 'sudo journalctl -u era -n 30'`)
- PR opens with the second file added on a NEW branch

Step 3c: reply to a non-era message (or a very old DM):
- Expected: "sorry, couldn't find the task you're replying to"

---

# Phase AJ — Mid-run progress DMs

**Goal:** Runner emits `PROGRESS <json>` lines on each tool_execution_end. Orchestrator scanner forwards to Queue; Queue notifies tgNotifier; tgNotifier sends one message per task and edits it on subsequent updates.

## Task AJ-1: Runner result.go writeProgress + runProgress type

**Files:**
- Modify: `cmd/runner/result.go`
- Modify: `cmd/runner/result_test.go`

- [ ] **Step 1: Write failing test.**

```go
func TestWriteProgress_EmitsJSONLine(t *testing.T) {
    var buf bytes.Buffer
    writeProgress(&buf, runProgress{
        Iter: 7, Action: "write", Tokens: 8200, CostCents: 3,
    })
    line := buf.String()
    require.True(t, strings.HasPrefix(line, "PROGRESS "))
    require.True(t, strings.HasSuffix(line, "\n"))
    payload := strings.TrimSuffix(strings.TrimPrefix(line, "PROGRESS "), "\n")
    var got runProgress
    require.NoError(t, json.Unmarshal([]byte(payload), &got))
    require.Equal(t, 7, got.Iter)
    require.Equal(t, "write", got.Action)
    require.Equal(t, int64(8200), got.Tokens)
    require.Equal(t, 3, got.CostCents)
}
```

- [ ] **Step 2: Verify fail, implement, verify pass.**

In `cmd/runner/result.go`:

```go
type runProgress struct {
    Iter      int    `json:"iter"`
    Action    string `json:"action"`
    Tokens    int64  `json:"tokens_cum"`
    CostCents int    `json:"cost_cents_cum"`
}

func writeProgress(w io.Writer, p runProgress) {
    payload, err := json.Marshal(p)
    if err != nil {
        return // best-effort
    }
    fmt.Fprintf(w, "PROGRESS %s\n", payload)
}
```

```
go test -race -run TestWriteProgress ./cmd/runner/
```

- [ ] **Step 3: Commit.**

```bash
git add cmd/runner/result.go cmd/runner/result_test.go
git commit -m "feat(runner): writeProgress emits PROGRESS <json> line"
```

## Task AJ-2: Pi event loop calls progressFunc on tool_execution_end

**Files:**
- Modify: `cmd/runner/pi.go`
- Modify: `cmd/runner/pi_test.go`

- [ ] **Step 1: Write failing test.**

In `pi_test.go`:

```go
func TestRunPi_FiresProgressOnToolExecution(t *testing.T) {
    jsonl := strings.Join([]string{
        `{"type":"tool_execution_end","tool":"read"}`,
        `{"type":"tool_execution_end","tool":"write"}`,
        `{"type":"agent_end"}`,
    }, "\n")
    p := &fakePi{stdout: strings.NewReader(jsonl)}

    var got []struct {
        iter   int
        action string
    }
    onProgress := func(iter int, action string, tokens int64, cost float64) {
        got = append(got, struct {
            iter   int
            action string
        }{iter, action})
    }
    _, err := runPi(context.Background(), p, nopObserver{}, onProgress)
    require.NoError(t, err)
    require.Len(t, got, 2)
    require.Equal(t, 1, got[0].iter)
    require.Equal(t, "read", got[0].action)
    require.Equal(t, 2, got[1].iter)
    require.Equal(t, "write", got[1].action)
}

func TestRunPi_NilProgressIsSafe(t *testing.T) {
    jsonl := `{"type":"tool_execution_end","tool":"x"}` + "\n" + `{"type":"agent_end"}`
    p := &fakePi{stdout: strings.NewReader(jsonl)}
    _, err := runPi(context.Background(), p, nopObserver{}, nil)
    require.NoError(t, err)
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestRunPi_FiresProgress ./cmd/runner/
```

- [ ] **Step 3: Update runPi signature + implementation.**

```go
type progressFunc func(iter int, action string, tokens int64, costUSD float64)

func runPi(ctx context.Context, p piProcess, obs eventObserver, onProgress progressFunc) (*runSummary, error) {
    // ... existing setup unchanged ...
    for sc.Scan() {
        // ... existing event decode unchanged ...
        switch e.Type {
        case "message_end":
            // existing
        case "tool_execution_end":
            summary.ToolUseCount++
            if onProgress != nil {
                onProgress(summary.ToolUseCount, e.Tool, summary.TotalTokens, summary.TotalCostUSD)
            }
        case "error":
            // existing
        }
        // ... rest unchanged ...
    }
    // ... rest unchanged ...
}
```

Update existing call site in `cmd/runner/main.go` to pass `nil` for now (AJ-3 wires the real callback).

- [ ] **Step 4: Verify all runPi tests pass (existing tests now have nil onProgress).**

```
go test -race -count=1 ./cmd/runner/
```

If existing tests call `runPi(ctx, p, obs)` with 3 args, they need to become `runPi(ctx, p, obs, nil)`. Quick sed fix or manual updates.

- [ ] **Step 5: Commit.**

```bash
git add cmd/runner/pi.go cmd/runner/pi_test.go cmd/runner/main.go
git commit -m "feat(runner): runPi gains progressFunc callback fired on tool_execution_end"
```

## Task AJ-3: Wire onProgress in `cmd/runner/main.go`

**Files:**
- Modify: `cmd/runner/main.go`

- [ ] **Step 1: Replace the `nil` from AJ-2 with a real callback.**

In `cmd/runner/main.go`, locate the `runPi(ctx, p, c, nil)` call (added in AJ-2). Replace:

```go
onProgress := func(iter int, action string, tokens int64, costUSD float64) {
    writeProgress(os.Stdout, runProgress{
        Iter:      iter,
        Action:    action,
        Tokens:    tokens,
        CostCents: int(math.Round(costUSD * 100)),
    })
}
summary, piErr := runPi(ctx, p, c, onProgress)
```

`math` should already be imported. If not, add.

- [ ] **Step 2: Verify build + tests.**

```
go build ./...
go test -race -count=1 ./cmd/runner/
```

- [ ] **Step 3: Commit.**

```bash
git add cmd/runner/main.go
git commit -m "feat(runner): wire writeProgress as runPi onProgress callback"
```

## Task AJ-4: docker.go ProgressEvent + streamToWithProgress + Docker.Run signature

**Files:**
- Modify: `internal/runner/docker.go`
- Modify: `internal/runner/docker_test.go`

- [ ] **Step 1: Add ProgressEvent type + ProgressCallback type.**

```go
type ProgressEvent struct {
    Iter      int    `json:"iter"`
    Action    string `json:"action"`
    Tokens    int64  `json:"tokens_cum"`
    CostCents int    `json:"cost_cents_cum"`
}

type ProgressCallback func(ev ProgressEvent)
```

- [ ] **Step 2: Add streamToWithProgress.**

Replace existing `streamTo` invocations with a wrapped version. New helper:

```go
func streamToWithProgress(mu *sync.Mutex, r io.Reader, combined *strings.Builder, wg *sync.WaitGroup, onProgress ProgressCallback) {
    defer wg.Done()
    sc := bufio.NewScanner(r)
    sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
    for sc.Scan() {
        line := sc.Text()
        mu.Lock()
        combined.WriteString(line)
        combined.WriteString("\n")
        mu.Unlock()
        if onProgress != nil && strings.HasPrefix(line, "PROGRESS ") {
            payload := strings.TrimPrefix(line, "PROGRESS ")
            var ev ProgressEvent
            if err := json.Unmarshal([]byte(payload), &ev); err == nil {
                onProgress(ev)
            }
        }
    }
}
```

(Keep the old `streamTo` if other callers exist; otherwise replace it with the progress-aware variant. Check via grep.)

- [ ] **Step 3: Add tests.**

```go
func TestStreamToWithProgress_FiresCallback(t *testing.T) {
    input := strings.Join([]string{
        "regular log line",
        `PROGRESS {"iter":1,"action":"read","tokens_cum":100,"cost_cents_cum":0}`,
        `PROGRESS {"iter":2,"action":"write","tokens_cum":500,"cost_cents_cum":1}`,
        `RESULT {"branch":"x","summary":"y","tokens":500,"cost_cents":1}`,
    }, "\n")

    var mu sync.Mutex
    var combined strings.Builder
    var wg sync.WaitGroup
    wg.Add(1)
    var got []ProgressEvent
    onProgress := func(ev ProgressEvent) { got = append(got, ev) }
    go streamToWithProgress(&mu, strings.NewReader(input), &combined, &wg, onProgress)
    wg.Wait()

    require.Len(t, got, 2)
    require.Equal(t, 1, got[0].Iter)
    require.Equal(t, "read", got[0].Action)
    require.Equal(t, 2, got[1].Iter)
    require.Contains(t, combined.String(), "RESULT")
}

func TestStreamToWithProgress_MalformedJSON_Ignored(t *testing.T) {
    input := `PROGRESS {bad json` + "\n" + `RESULT {"branch":"x","summary":"y","tokens":0,"cost_cents":0}`
    var mu sync.Mutex
    var combined strings.Builder
    var wg sync.WaitGroup
    wg.Add(1)
    called := 0
    onProgress := func(ev ProgressEvent) { called++ }
    go streamToWithProgress(&mu, strings.NewReader(input), &combined, &wg, onProgress)
    wg.Wait()
    require.Equal(t, 0, called)
}
```

- [ ] **Step 4: Update Docker.Run signature.**

```go
func (d *Docker) Run(ctx context.Context, in RunInput, onProgress ProgressCallback) (*RunOutput, error) {
    // ... existing setup ...
    go streamToWithProgress(&mu, stdout, &combined, &wg, onProgress)
    go streamToWithProgress(&mu, stderr, &combined, &wg, nil) // stderr doesn't carry PROGRESS
    // ... rest unchanged ...
}
```

- [ ] **Step 5: Update QueueAdapter.Run to accept + pass through callback.**

In `internal/runner/adapter.go`:

```go
func (q *QueueAdapter) Run(ctx context.Context, taskID int64, description, ghToken, repo string,
    maxIter, maxCents, maxWallSec int, onProgress ProgressCallback) (string, string, int64, int, []audit.Entry, error) {
    // ... name + register unchanged ...
    out, err := q.D.Run(ctx, RunInput{...}, onProgress)
    // ... rest unchanged ...
}
```

- [ ] **Step 6: Update Queue.Runner interface.**

In `internal/queue/queue.go`:

```go
type Runner interface {
    Run(ctx context.Context, taskID int64, description, ghToken, repo string,
        maxIter, maxCents, maxWallSec int, onProgress runner.ProgressCallback) (string, string, int64, int, []audit.Entry, error)
}
```

Add `"github.com/vaibhav0806/era/internal/runner"` import to queue.go (NOT a circular import — queue imports runner, runner doesn't import queue).

In `RunNext`, build a callback:

```go
progressCB := func(ev runner.ProgressEvent) {
    if q.progressNotifier != nil {
        q.progressNotifier.NotifyProgress(ctx, t.ID, ProgressEvent{
            Iter: ev.Iter, Action: ev.Action,
            Tokens: ev.Tokens, CostCents: ev.CostCents,
        })
    }
}
branch, summary, tokens, costCents, audits, runErr := q.runner.Run(
    ctx, t.ID, t.Description, ghToken, effectiveRepo,
    profile.MaxIter, profile.MaxCents, profile.MaxWallSec, progressCB,
)
```

(Define `ProgressEvent` and `ProgressNotifier` in queue.go — see AJ-5.)

Update `fakeRunner.Run` signature in tests to match.

- [ ] **Step 7: Verify build + tests.**

```
go build ./...
go test -race -count=1 ./...
```

- [ ] **Step 8: Commit.**

```bash
git add internal/runner/docker.go internal/runner/docker_test.go internal/runner/adapter.go internal/queue/queue.go internal/queue/queue_run_test.go
git commit -m "feat(runner,queue): streamToWithProgress + Docker.Run/QueueAdapter/Queue.Runner gain ProgressCallback"
```

## Task AJ-5: Queue ProgressNotifier interface + ProgressEvent (queue-side)

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_run_test.go`

- [ ] **Step 1: Add types + interface + setter.**

In `internal/queue/queue.go`:

```go
// ProgressEvent is the queue-layer counterpart to runner.ProgressEvent.
// Two types intentional — runner can't import queue (would be circular).
type ProgressEvent struct {
    Iter      int
    Action    string
    Tokens    int64
    CostCents int
}

type ProgressNotifier interface {
    NotifyProgress(ctx context.Context, taskID int64, ev ProgressEvent)
}

func (q *Queue) SetProgressNotifier(p ProgressNotifier) { q.progressNotifier = p }
```

Add `progressNotifier ProgressNotifier` field to Queue struct.

- [ ] **Step 2: Write failing test.**

```go
type fakeProgressNotifier struct {
    mu     sync.Mutex
    events []struct {
        TaskID int64
        Ev     queue.ProgressEvent
    }
}
func (f *fakeProgressNotifier) NotifyProgress(ctx context.Context, taskID int64, ev queue.ProgressEvent) {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.events = append(f.events, struct {
        TaskID int64
        Ev     queue.ProgressEvent
    }{taskID, ev})
}

func TestQueue_RunNext_FiresProgress(t *testing.T) {
    ctx := context.Background()
    fr := &fakeRunner{
        branch:  "agent/1/x",
        summary: "ok",
        progressEvents: []runner.ProgressEvent{
            {Iter: 1, Action: "read", Tokens: 200, CostCents: 0},
            {Iter: 2, Action: "write", Tokens: 500, CostCents: 1},
        },
    }
    q, repo := newRunQueue(t, fr)
    pn := &fakeProgressNotifier{}
    q.SetProgressNotifier(pn)
    _, err := repo.CreateTask(ctx, "x", "", "default")
    require.NoError(t, err)
    _, err = q.RunNext(ctx)
    require.NoError(t, err)
    require.Len(t, pn.events, 2)
    require.Equal(t, "read", pn.events[0].Ev.Action)
    require.Equal(t, "write", pn.events[1].Ev.Action)
}
```

For this test to work, `fakeRunner` needs a `progressEvents []runner.ProgressEvent` field, and its `Run` method must invoke `onProgress(ev)` for each event:

```go
func (f *fakeRunner) Run(ctx context.Context, taskID int64, desc, ghToken, repo string,
    maxIter, maxCents, maxWallSec int, onProgress runner.ProgressCallback) (string, string, int64, int, []audit.Entry, error) {
    for _, ev := range f.progressEvents {
        if onProgress != nil { onProgress(ev) }
    }
    return f.branch, f.summary, f.tokens, f.costCents, f.audits, f.err
}
```

- [ ] **Step 3: Verify fail then pass.**

```
go test -race -run TestQueue_RunNext_FiresProgress ./internal/queue/
```

- [ ] **Step 4: Commit.**

```bash
git add internal/queue/queue.go internal/queue/queue_run_test.go
git commit -m "feat(queue): ProgressNotifier interface + ProgressEvent + fakeRunner emits"
```

## Task AJ-6: tgNotifier.NotifyProgress + EditMessageText cast

**Files:**
- Modify: `cmd/orchestrator/main.go`

- [ ] **Step 1: Add NotifyProgress method on tgNotifier.**

```go
func (n *tgNotifier) NotifyProgress(ctx context.Context, id int64, ev queue.ProgressEvent) {
    body := fmt.Sprintf("task #%d · iter %d · %s · $%.3f",
        id, ev.Iter, ev.Action, float64(ev.CostCents)/100.0)
    if existing, ok := n.progressMsgs.Load(id); ok {
        msgID := existing.(int64)
        if err := n.client.EditMessageText(ctx, n.chatID, int(msgID), body); err != nil {
            slog.Warn("edit progress", "err", err, "task", id)
        }
        return
    }
    msgID, err := n.client.SendMessage(ctx, n.chatID, body)
    if err != nil {
        slog.Warn("send progress", "err", err, "task", id)
        return
    }
    n.progressMsgs.Store(id, msgID)
}
```

The `progressMsgs sync.Map` field was added to tgNotifier struct in AI-3. If not, add it now.

- [ ] **Step 2: Wire ProgressNotifier on Queue.**

In `main()`, after `q.SetNotifier(...)`:

```go
q.SetProgressNotifier(notifier) // tgNotifier implements both interfaces
```

(`tgNotifier` satisfies `ProgressNotifier` thanks to NotifyProgress method.)

- [ ] **Step 3: Verify build + tests.**

```
go build ./...
go test -race -count=1 ./...
```

- [ ] **Step 4: Commit.**

```bash
git add cmd/orchestrator/main.go
git commit -m "feat(orchestrator): tgNotifier.NotifyProgress edits pinned per-task message"
```

## Task AJ-7: Phase AJ smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_aj_progress.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AJ smoke: writeProgress + runPi callback + streamToWithProgress + Queue ProgressNotifier.
set -euo pipefail
go test -race -count=1 -run 'TestWriteProgress_|TestRunPi_FiresProgress|TestRunPi_NilProgress|TestStreamToWithProgress_|TestQueue_RunNext_FiresProgress' \
    ./cmd/runner/... ./internal/runner/... ./internal/queue/... > /dev/null
echo "OK: phase AJ — progress DM pipeline all unit tests green"
```

- [ ] **Step 2: Commit + push.**

```
chmod +x scripts/smoke/phase_aj_progress.sh
bash scripts/smoke/phase_aj_progress.sh
git add scripts/smoke/phase_aj_progress.sh
git commit -m "docs(smoke): phase AJ progress DMs"
git push origin master
gh run watch --exit-status
```

- [ ] **Step 3: Live gate.**

Run a multi-step task:
```
/task vaibhav0806/ad-smoke create a tiny Go HTTP server in main.go (just a /health endpoint), Makefile with build+test targets, README.md, and go.mod
```

Watch Telegram during execution. Expected:
- A pinned "task #N · iter 1 · read · $0.000" message appears within seconds
- Same message edits with each new iter as Pi works
- Final completion DM is a NEW message with the PR link
- The progress message stays in chat history as a trace

- [ ] **Step 4: Verify in DB.**

No new DB columns to check. Just confirm task completed with a non-trivial `tool_use_count` event log:
```
ssh era@178.105.44.3 'sqlite3 /opt/era/pi-agent.db "SELECT id, status, tokens_used FROM tasks ORDER BY id DESC LIMIT 1;"'
```

---

# Phase AK — `/ask` read-only shortcut

**Goal:** Atomic `CreateAskTask` (migration 0009 + sqlc query). Handler `/ask` route. Runner respects `ERA_READ_ONLY=1` to skip CommitAndPush + restrict Pi tools.

## Task AK-1: Migration 0009 + atomic CreateAskTask sqlc query

**Files:**
- Create: `migrations/0009_read_only.sql`
- Modify: `queries/tasks.sql`
- Modify: `internal/db/repo.go`

- [ ] **Step 1: Migration.**

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN read_only INTEGER NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
```

- [ ] **Step 2: Atomic CreateAskTask query.**

Append to `queries/tasks.sql`:

```sql
-- name: CreateAskTask :one
INSERT INTO tasks (description, target_repo, budget_profile, read_only, status)
VALUES (?, ?, 'quick', 1, 'queued')
RETURNING *;
```

- [ ] **Step 3: Regenerate sqlc.**

```
sqlc generate
```

- [ ] **Step 4: Repo wrapper.**

```go
func (r *Repo) CreateAskTask(ctx context.Context, desc, targetRepo string) (Task, error) {
    return r.q.CreateAskTask(ctx, CreateAskTaskParams{
        Description: desc,
        TargetRepo:  targetRepo,
    })
}
```

- [ ] **Step 5: Test round-trip.**

```go
func TestRepo_CreateAskTask_AtomicReadOnlyQuick(t *testing.T) {
    repo, cleanup := newTestRepo(t)
    defer cleanup()
    task, err := repo.CreateAskTask(context.Background(), "what is in main.go", "vaibhav0806/foo")
    require.NoError(t, err)
    require.Equal(t, int64(1), task.ReadOnly)
    require.Equal(t, "quick", task.BudgetProfile)
    require.Equal(t, "queued", task.Status)
    require.Equal(t, "vaibhav0806/foo", task.TargetRepo)
}
```

- [ ] **Step 6: Commit.**

```bash
git add migrations/0009_read_only.sql queries/tasks.sql internal/db/
git commit -m "feat(db): migration 0009 tasks.read_only + atomic CreateAskTask sqlc query"
```

## Task AK-2: Queue.CreateAskTask wrapper

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_test.go` (or new file)

- [ ] **Step 1: Add method.**

```go
func (q *Queue) CreateAskTask(ctx context.Context, desc, targetRepo string) (int64, error) {
    task, err := q.repo.CreateAskTask(ctx, desc, targetRepo)
    if err != nil {
        return 0, fmt.Errorf("create ask task: %w", err)
    }
    return task.ID, nil
}
```

- [ ] **Step 2: Test.**

```go
func TestQueue_CreateAskTask_ReturnsID(t *testing.T) {
    ctx := context.Background()
    q, _ := newRunQueue(t, &fakeRunner{})
    id, err := q.CreateAskTask(ctx, "what is in foo", "owner/repo")
    require.NoError(t, err)
    require.Greater(t, id, int64(0))
}
```

- [ ] **Step 3: Verify + commit.**

```bash
git add internal/queue/queue.go internal/queue/queue_test.go
git commit -m "feat(queue): CreateAskTask wraps atomic db.CreateAskTask"
```

## Task AK-3: Handler `/ask` route + Ops interface

**Files:**
- Modify: `internal/telegram/handler.go`
- Modify: `internal/telegram/handler_test.go`

- [ ] **Step 1: Extend Ops interface.**

```go
type Ops interface {
    CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error)
    CreateAskTask(ctx context.Context, desc, targetRepo string) (int64, error)
    // ... existing methods unchanged
}
```

Update `stubOps` in handler_test.go to implement the new method.

- [ ] **Step 2: Add /ask route.**

```go
case strings.HasPrefix(text, "/ask "):
    args := strings.TrimSpace(strings.TrimPrefix(text, "/ask "))
    repo, desc := parseAskArgs(args)
    if repo == "" {
        _, err := h.client.SendMessage(ctx, u.ChatID,
            "usage: /ask <owner>/<repo> <question>")
        return err
    }
    id, err := h.ops.CreateAskTask(ctx, desc, repo)
    if err != nil {
        _, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
        return err
    }
    _, err = h.client.SendMessage(ctx, u.ChatID,
        fmt.Sprintf("task #%d queued (ask, repo: %s)", id, repo))
    return err
```

`parseAskArgs` is similar to `parseTaskArgs` but REQUIRES the repo prefix (no sandbox fallback). Define near `parseTaskArgs`:

```go
var askRepoPattern = regexp.MustCompile(`^([\w.-]+/[\w.-]+)\s+(.+)$`)

func parseAskArgs(args string) (repo, desc string) {
    m := askRepoPattern.FindStringSubmatch(strings.TrimSpace(args))
    if len(m) != 3 {
        return "", ""
    }
    return m[1], m[2]
}
```

- [ ] **Step 3: Tests.**

```go
func TestHandler_AskCommand_QueuesReadOnlyTask(t *testing.T) {
    ctx := context.Background()
    f := NewFakeClient()
    ops := &stubOps{nextID: 42}
    h := NewHandler(f, ops, nil, "vaibhav0806/sandbox")
    err := h.Handle(ctx, Update{
        ChatID: 1,
        Text:   "/ask vaibhav0806/foo what is in main.go",
    })
    require.NoError(t, err)
    require.Equal(t, "vaibhav0806/foo", ops.lastAskRepo)
    require.Equal(t, "what is in main.go", ops.lastAskDesc)
    require.Contains(t, f.sentMessages[0].Text, "task #42 queued (ask")
}

func TestHandler_AskWithoutRepo_DMsUsage(t *testing.T) {
    ctx := context.Background()
    f := NewFakeClient()
    h := NewHandler(f, &stubOps{}, nil, "vaibhav0806/sandbox")
    err := h.Handle(ctx, Update{ChatID: 1, Text: "/ask just a question"})
    require.NoError(t, err)
    require.Contains(t, f.sentMessages[0].Text, "usage: /ask")
}
```

`stubOps` needs `CreateAskTask` plus `lastAskRepo` and `lastAskDesc` fields.

- [ ] **Step 4: Update help message.**

In the unknown-command fallback, add `/ask`:

```go
return h.client.SendMessage(ctx, u.ChatID, "unknown command. try /task, /ask, /status, /list, /cancel, /retry, /stats")
```

(/stats added in AL — leave for AL or include now; including now is fine since handler.go gets touched again.)

- [ ] **Step 5: Verify + commit.**

```
go test -race -count=1 ./...
git add internal/telegram/handler.go internal/telegram/handler_test.go
git commit -m "feat(telegram): /ask <repo> <question> route + parseAskArgs + stubOps update"
```

## Task AK-4: Runner ERA_READ_ONLY path

**Files:**
- Modify: `internal/runner/docker.go` (RunInput.ReadOnly + buildDockerArgs)
- Modify: `internal/runner/adapter.go` (resolve task.ReadOnly to RunInput)
- Modify: `internal/queue/queue.go` (Runner interface adds readOnly bool)
- Modify: `cmd/runner/main.go` (read ERA_READ_ONLY env, restrict tools, skip commit)
- Modify: `internal/runner/docker_test.go`

- [ ] **Step 1: Add RunInput.ReadOnly + buildDockerArgs emission.**

```go
type RunInput struct {
    // ... existing fields ...
    ReadOnly bool  // M6 AK: skip commit, restrict tools
}
```

In buildDockerArgs:

```go
if in.ReadOnly {
    args = append(args, "-e", "ERA_READ_ONLY=1")
}
```

- [ ] **Step 2: Test.**

```go
func TestBuildDockerArgs_ReadOnlyEmitsEnv(t *testing.T) {
    d := &Docker{Image: "test:v1"}
    in := RunInput{TaskID: 1, Repo: "o/r", ReadOnly: true}
    args := d.BuildDockerArgs(in)
    requireEnvSet(t, args, "ERA_READ_ONLY=1")
}

func TestBuildDockerArgs_NotReadOnlyOmitsEnv(t *testing.T) {
    d := &Docker{Image: "test:v1"}
    in := RunInput{TaskID: 1, Repo: "o/r"}
    args := d.BuildDockerArgs(in)
    for i, a := range args {
        if a == "-e" && i+1 < len(args) && strings.HasPrefix(args[i+1], "ERA_READ_ONLY") {
            t.Fatalf("ERA_READ_ONLY should be absent when ReadOnly=false; args: %v", args)
        }
    }
}
```

- [ ] **Step 3: Update Runner interface signature.**

```go
type Runner interface {
    Run(ctx context.Context, taskID int64, description, ghToken, repo string,
        maxIter, maxCents, maxWallSec int, readOnly bool, onProgress runner.ProgressCallback) (string, string, int64, int, []audit.Entry, error)
}
```

QueueAdapter.Run gains `readOnly bool` arg, populates `RunInput.ReadOnly`.

In `Queue.RunNext`, look up `t.ReadOnly`:

```go
readOnly := t.ReadOnly == 1
branch, summary, tokens, costCents, audits, runErr := q.runner.Run(
    ctx, t.ID, t.Description, ghToken, effectiveRepo,
    profile.MaxIter, profile.MaxCents, profile.MaxWallSec, readOnly, progressCB,
)
```

`fakeRunner.Run` updated to match.

- [ ] **Step 4: Runner main.go ERA_READ_ONLY path.**

In `cmd/runner/main.go`, AT THE TOP after Pi config setup:

```go
readOnly := os.Getenv("ERA_READ_ONLY") == "1"

piTools := "read,write,edit,grep,find,ls,bash"
if readOnly {
    piTools = "read,grep,find,ls"
}
```

Update the `newRealPi(...)` call (or wherever Pi is invoked with `--tools`) to use `piTools`.

After `runPi` returns, BEFORE the existing commit-or-no-changes switch:

```go
if readOnly {
    writeResult(os.Stdout, runResult{
        Branch:    "",
        Summary:   finalSummary(summary, piErr),
        Tokens:    tokens,
        CostCents: int(math.Round(costUSD * 100)),
    })
    if piErr != nil {
        return piErr
    }
    return nil
}
// existing pre-commit test gate + CommitAndPush switch
```

- [ ] **Step 5: Verify + commit.**

```
go test -race -count=1 ./...
git add internal/runner/docker.go internal/runner/docker_test.go internal/runner/adapter.go internal/queue/queue.go internal/queue/queue_run_test.go cmd/runner/main.go
git commit -m "feat(runner): ERA_READ_ONLY=1 skips commit + restricts Pi tools"
```

## Task AK-5: Phase AK smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_ak_ask.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AK smoke: CreateAskTask atomicity + handler /ask route + buildDockerArgs ReadOnly.
set -euo pipefail
go test -race -count=1 -run 'TestRepo_CreateAskTask_|TestQueue_CreateAskTask_|TestHandler_AskCommand_|TestHandler_AskWithoutRepo_|TestBuildDockerArgs_ReadOnly|TestBuildDockerArgs_NotReadOnly' \
    ./internal/db/... ./internal/queue/... ./internal/telegram/... ./internal/runner/... > /dev/null
echo "OK: phase AK — /ask read-only all unit tests green"
```

- [ ] **Step 2: Commit + push + live gate.**

```
chmod +x scripts/smoke/phase_ak_ask.sh
bash scripts/smoke/phase_ak_ask.sh
git add scripts/smoke/phase_ak_ask.sh
git commit -m "docs(smoke): phase AK /ask read-only"
git push origin master
gh run watch --exit-status
```

Live test:
```
/ask vaibhav0806/trying-something what does main.go do, in 3 sentences?
```

Expected DM (within ~30s): `task #N completed` with prose response. NO branch pushed (verify on GitHub — no new branch). DB row:
```
ssh era@178.105.44.3 'sqlite3 /opt/era/pi-agent.db "SELECT id, status, read_only, budget_profile, branch_name FROM tasks ORDER BY id DESC LIMIT 1;"'
```
Expected: `<id>|completed|1|quick|` (no branch_name).

---

# Phase AL — `/stats` command

**Goal:** Four sqlc queries × three periods. Single Telegram DM with a 3-column table.

## Task AL-1: 4 sqlc stats queries + repo wrappers

**Files:**
- Modify: `queries/tasks.sql`
- Modify: `internal/db/repo.go`

- [ ] **Step 1: Append queries.**

```sql
-- name: CountTasksByStatusSince :many
SELECT status, COUNT(*) AS count FROM tasks WHERE created_at >= ? GROUP BY status;

-- name: SumTokensSince :one
SELECT COALESCE(SUM(tokens_used), 0)::INTEGER AS total FROM tasks WHERE created_at >= ?;

-- name: SumCostCentsSince :one
SELECT COALESCE(SUM(cost_cents), 0)::INTEGER AS total FROM tasks WHERE created_at >= ?;

-- name: CountQueuedTasks :one
SELECT COUNT(*) AS count FROM tasks WHERE status = 'queued';
```

(SQLite doesn't support `::INTEGER` type cast — drop it. `SUM` returns the column type; for INTEGER columns it stays INTEGER. Plain `COALESCE(SUM(...), 0)` is fine.)

Final form:

```sql
-- name: SumTokensSince :one
SELECT COALESCE(SUM(tokens_used), 0) AS total FROM tasks WHERE created_at >= ?;

-- name: SumCostCentsSince :one
SELECT COALESCE(SUM(cost_cents), 0) AS total FROM tasks WHERE created_at >= ?;
```

- [ ] **Step 2: Regenerate.**

```
sqlc generate
```

Check the generated types: `CountTasksByStatusSinceRow` (with `Status string, Count int64` fields) and the Sum* methods returning `int64`.

- [ ] **Step 3: Repo wrappers.**

```go
func (r *Repo) CountTasksByStatusSince(ctx context.Context, since time.Time) ([]CountTasksByStatusSinceRow, error) {
    return r.q.CountTasksByStatusSince(ctx, since)
}

func (r *Repo) SumTokensSince(ctx context.Context, since time.Time) (int64, error) {
    v, err := r.q.SumTokensSince(ctx, since)
    return v, err
}

func (r *Repo) SumCostCentsSince(ctx context.Context, since time.Time) (int64, error) {
    v, err := r.q.SumCostCentsSince(ctx, since)
    return v, err
}

func (r *Repo) CountQueuedTasks(ctx context.Context) (int64, error) {
    return r.q.CountQueuedTasks(ctx)
}
```

(Adjust types per the actual sqlc output — Sum may return `interface{}` if SQLite needs a coercion.)

- [ ] **Step 4: Commit.**

```bash
git add queries/tasks.sql internal/db/
git commit -m "feat(db): stats sqlc queries (CountTasksByStatusSince, SumTokens, SumCostCents, CountQueuedTasks)"
```

## Task AL-2: Queue.Stats method + tests

**Files:**
- Create: `internal/queue/stats.go`
- Create: `internal/queue/stats_test.go`

- [ ] **Step 1: Write failing tests.**

```go
package queue_test

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestStats_EmptyDB_ReturnsZeros(t *testing.T) {
    ctx := context.Background()
    q, _ := newRunQueue(t, &fakeRunner{})
    s, err := q.Stats(ctx)
    require.NoError(t, err)
    require.Equal(t, 0, s.Last24h.TasksTotal)
    require.Equal(t, 0, s.Last7d.TasksTotal)
    require.Equal(t, 0, s.Last30d.TasksTotal)
    require.Equal(t, 0, s.PendingQueue)
}

func TestStats_MixedStatuses_CountsSuccessRate(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    // Seed: 2 completed, 1 failed in last 24h
    seed := func(status string, tokens int, cents int) {
        task, err := repo.CreateTask(ctx, "x", "", "default")
        require.NoError(t, err)
        require.NoError(t, repo.SetStatus(ctx, task.ID, status))
        require.NoError(t, repo.CompleteTask(ctx, task.ID, "br", "s", int64(tokens), int64(cents)))
    }
    seed("completed", 100, 1)
    seed("completed", 200, 2)
    seed("failed", 50, 0)

    s, err := q.Stats(ctx)
    require.NoError(t, err)
    require.Equal(t, 3, s.Last24h.TasksTotal)
    require.Equal(t, 2, s.Last24h.TasksOK)
    require.Equal(t, int64(350), s.Last24h.Tokens)
    require.Equal(t, int64(3), s.Last24h.CostCents)
}

func TestPeriodStats_SuccessRate(t *testing.T) {
    p := PeriodStatsZero()
    require.Equal(t, 0.0, p.SuccessRate())
    p2 := PeriodStatsFor(10, 7)
    require.InDelta(t, 0.7, p2.SuccessRate(), 0.001)
}
```

(`PeriodStatsZero()` and `PeriodStatsFor(...)` are test helpers. Use them OR construct the struct literal inline.)

- [ ] **Step 2: Implement.**

`internal/queue/stats.go`:

```go
package queue

import (
    "context"
    "time"
)

type PeriodStats struct {
    TasksTotal int
    TasksOK    int
    Tokens     int64
    CostCents  int64
}

func (p PeriodStats) SuccessRate() float64 {
    if p.TasksTotal == 0 {
        return 0
    }
    return float64(p.TasksOK) / float64(p.TasksTotal)
}

type Stats struct {
    Last24h, Last7d, Last30d PeriodStats
    PendingQueue             int
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
    var s Stats
    targets := []*PeriodStats{&s.Last24h, &s.Last7d, &s.Last30d}
    durs := []time.Duration{24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}
    now := time.Now().UTC()

    for i, d := range durs {
        since := now.Add(-d)
        rows, err := q.repo.CountTasksByStatusSince(ctx, since)
        if err != nil {
            return s, err
        }
        for _, r := range rows {
            targets[i].TasksTotal += int(r.Count)
            if r.Status == "completed" || r.Status == "approved" {
                targets[i].TasksOK += int(r.Count)
            }
        }
        toks, err := q.repo.SumTokensSince(ctx, since)
        if err != nil {
            return s, err
        }
        targets[i].Tokens = toks
        cost, err := q.repo.SumCostCentsSince(ctx, since)
        if err != nil {
            return s, err
        }
        targets[i].CostCents = cost
    }
    pending, err := q.repo.CountQueuedTasks(ctx)
    if err != nil {
        return s, err
    }
    s.PendingQueue = int(pending)
    return s, nil
}
```

- [ ] **Step 3: Verify + commit.**

```
go test -race -count=1 -run TestStats_ ./internal/queue/
git add internal/queue/stats.go internal/queue/stats_test.go
git commit -m "feat(queue): Stats() aggregates 24h/7d/30d task counts, tokens, cost, queue depth"
```

## Task AL-3: Handler `/stats` route + formatStatsDM

**Files:**
- Modify: `internal/telegram/handler.go`
- Modify: `internal/telegram/handler_test.go`

- [ ] **Step 1: Extend Ops interface.**

```go
type Ops interface {
    // ... existing ...
    Stats(ctx context.Context) (queue.Stats, error)
}
```

(`stubOps.Stats` returns canned data in tests.)

- [ ] **Step 2: Add /stats route + formatter.**

```go
case text == "/stats":
    s, err := h.ops.Stats(ctx)
    if err != nil {
        _, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
        return err
    }
    _, err = h.client.SendMessage(ctx, u.ChatID, formatStatsDM(s))
    return err
```

Add at the bottom of handler.go (or in a helper file):

```go
func formatStatsDM(s queue.Stats) string {
    return fmt.Sprintf(
`era stats
────────────
            24h    7d     30d
tasks:      %-6d %-6d %-d
success:    %-6s %-6s %s
tokens:     %-6s %-6s %s
cost:       %-6s %-6s %s
queue: %d pending`,
        s.Last24h.TasksTotal, s.Last7d.TasksTotal, s.Last30d.TasksTotal,
        pctStr(s.Last24h.SuccessRate()), pctStr(s.Last7d.SuccessRate()), pctStr(s.Last30d.SuccessRate()),
        kStr(s.Last24h.Tokens), kStr(s.Last7d.Tokens), kStr(s.Last30d.Tokens),
        costStr(s.Last24h.CostCents), costStr(s.Last7d.CostCents), costStr(s.Last30d.CostCents),
        s.PendingQueue,
    )
}

func pctStr(x float64) string  { return fmt.Sprintf("%.0f%%", x*100) }
func kStr(n int64) string      {
    if n < 1000 { return fmt.Sprintf("%d", n) }
    return fmt.Sprintf("%dk", n/1000)
}
func costStr(c int64) string   { return fmt.Sprintf("$%.2f", float64(c)/100.0) }
```

- [ ] **Step 3: Test.**

```go
func TestHandler_StatsCommand_SendsFormattedDM(t *testing.T) {
    ctx := context.Background()
    f := NewFakeClient()
    ops := &stubOps{
        statsResult: queue.Stats{
            Last24h:      queue.PeriodStats{TasksTotal: 5, TasksOK: 4, Tokens: 1500, CostCents: 8},
            Last7d:       queue.PeriodStats{TasksTotal: 20, TasksOK: 17, Tokens: 8500, CostCents: 75},
            Last30d:      queue.PeriodStats{TasksTotal: 80, TasksOK: 65, Tokens: 41000, CostCents: 320},
            PendingQueue: 0,
        },
    }
    h := NewHandler(f, ops, nil, "vaibhav0806/sandbox")
    err := h.Handle(ctx, Update{ChatID: 1, Text: "/stats"})
    require.NoError(t, err)
    require.Len(t, f.sentMessages, 1)
    body := f.sentMessages[0].Text
    require.Contains(t, body, "era stats")
    require.Contains(t, body, "tasks:")
    require.Contains(t, body, "5")
    require.Contains(t, body, "80")
    require.Contains(t, body, "queue: 0 pending")
}
```

`stubOps` gains `statsResult queue.Stats` and a `Stats(ctx) (queue.Stats, error)` method that returns it.

- [ ] **Step 4: Verify + commit.**

```
go test -race -count=1 ./...
git add internal/telegram/handler.go internal/telegram/handler_test.go
git commit -m "feat(telegram): /stats command DMs 3-column 24h/7d/30d summary table"
```

## Task AL-4: Phase AL smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_al_stats.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AL smoke: stats queries + Queue.Stats + handler /stats route.
set -euo pipefail
go test -race -count=1 -run 'TestStats_|TestPeriodStats_|TestHandler_StatsCommand_' \
    ./internal/queue/... ./internal/telegram/... > /dev/null
echo "OK: phase AL — /stats command all unit tests green"
```

- [ ] **Step 2: Commit + push + live gate.**

```
chmod +x scripts/smoke/phase_al_stats.sh
bash scripts/smoke/phase_al_stats.sh
git add scripts/smoke/phase_al_stats.sh
git commit -m "docs(smoke): phase AL /stats command"
git push origin master
gh run watch --exit-status
```

Live test from Telegram:
```
/stats
```

Expected: 3-column table reflecting recent activity (M6 phase live gates produced ~10-20 tasks).

---

# Final — README + tag m6-release

## Task F-1: README M6 status + roadmap row

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Prepend M6 status section.**

Before the existing `## Status: Milestone 5 — polish + safety` heading, insert:

```markdown
## Status: Milestone 6 — agent sharpness

M6 makes era's agent loop sharper at finishing real tasks and easier to direct in flight:

- **Per-task budget profiles.** `quick` (20 iter / $0.05 / 10 min), `default` (60 / $0.20 / 30 min), `deep` (120 / $1.00 / 60 min). Override per task via `/task --budget=deep <desc>`. Defaults bumped from M0's 30/$0.05/60min, which was too tight for real side projects.
- **Smarter egress allowlist.** Added crates.io, jsdelivr/cdnjs/unpkg, fonts.googleapis.com, services.gradle.org, etc. New `PI_EGRESS_EXTRA` env var appends comma-separated hosts at runtime.
- **Reply-to-continue.** Reply to a completion DM in Telegram → era queues a new task with the original task's branch, PR, and summary woven into the prompt. Migration 0008 stores the completion message ID per task.
- **Mid-run progress DMs.** While Pi is working, era pins a message that edits per tool action: `task #N · iter 7 · write · $0.008`. Final completion DM is a new message — the progress one stays as a trace.
- **`/ask <repo> <question>`.** Read-only shortcut. Same runner image + iptables sandbox, but Pi runs with `read,grep,find,ls` only, no commit, no push. ~15-30s to a prose answer in DM.
- **`/stats`.** 3-column 24h/7d/30d summary: tasks, success rate, tokens, cost, queue depth.

Telegram client API change: `SendMessage` now returns `(int64, error)` so the orchestrator can persist completion message IDs for reply lookups.

Everything from M5 still applies.
```

- [ ] **Step 2: Update roadmap.**

Find the roadmap list. Move `← you are here` from M5 to a new M6 row:

```markdown
- **M5 — polish + safety**: GitHub Actions CI + auto-deploy, offsite B2 backups, pre-commit test gate, PR approval feedback (label + comment), runner tooling bake, wildcarded sudoers, Bearer auth normalization
- **M6 — agent sharpness** ← you are here: per-task budget profiles + bumped caps, smarter egress allowlist, reply-to-continue, mid-run progress DMs, /ask read-only, /stats activity summary
```

- [ ] **Step 3: Commit.**

```bash
git add README.md
git commit -m "docs(readme): M6 shipped — agent sharpness"
```

## Task F-2: Final regression + tag + push

- [ ] **Step 1: Full regression.**

```
go test -race -count=1 ./...
```

All packages green.

- [ ] **Step 2: Run all M6 phase smokes locally.**

```
for f in scripts/smoke/phase_a{g,h,i,j,k,l}_*.sh; do
    echo "--- $f ---"
    bash "$f"
done
```

Each prints OK.

- [ ] **Step 3: Tag.**

```bash
git tag m6-release
git push origin master
git push origin m6-release
```

- [ ] **Step 4: Final live end-to-end via CI.**

The push triggers CI. Watch:
```
gh run watch --exit-status
```

Expected: green test + green deploy → era.service restarts on VPS → Telegram bot responds to a `/stats`.

- [ ] **Step 5: Empty marker commit.**

```bash
git commit --allow-empty -m "phase(M6): milestone complete — m6-release tagged + pushed + deployed"
```

---

## Post-M6 followups (NOT in this plan)

Deferred to M7+ (per spec §2):

- Dedicated read-only sandbox image (only if `/ask` latency is a real complaint)
- HTTP-based progress transport (only if stdout pipe limits surface)
- Richer telegram_messages audit table
- Transitive reply chains
- `/stats` with top-cost tasks breakdown
- PR auto-merge on approve
- Natural-language repo parsing (alias table)
- Long-answer Telegram file attachments
- Scheduled / recurring tasks
- Task chaining (`/task A depends-on B`)
- Multi-repo fan-out
- Dev/prod bot split
- sqlc drift check in CI
- Reconcile cleaning up orphan progress messages
- Rate-limiting progress edits
- `/ask --budget=<x>` variants
- Alpine CDN allowlist expansion
- Java/PHP/Ruby toolchain bake into runner image
