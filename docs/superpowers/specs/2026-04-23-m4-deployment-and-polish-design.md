# M4 — Deployment & Polish Design

**Date:** 2026-04-23
**Status:** design, pre-implementation
**Predecessor:** M3.5 (multi-repo per task)

## 1. Goal

Close the loop between era and the user's actual day-to-day workflow. M4 does four things:

1. **Deploy era to a Hetzner CAX11** so it runs 24/7 without the user's laptop.
2. **Surface Pi's actual text output** in DMs — today the summary is a useless `ok_tokens=X_cost=Y` or `no_changes` string.
3. **Cancel running tasks instantly** via `docker kill`, not wait for the 60-min wall-clock cap.
4. **Open a PR for every completed task**, so the user reviews a proper diff instead of a bare branch.

No behavior change that breaks M3.5. No auto-merge. No new milestones deferred beyond what's listed.

## 2. Non-goals (deferred to M5+)

- GitHub Actions CI or push-based deploys. Deploy stays a local `make deploy` target.
- Offsite backups (S3, Backblaze, etc.). Local nightly SQLite dump only.
- Natural-language repo parsing. Users must still prefix with `/task owner/repo …`.
- Option-A pre-push approval. Runner still commits and pushes unconditionally; approval gates after push.
- Multi-bot (dev/prod split). One bot, one host.
- Per-task observability dashboard. `journalctl -u era -f` is the observability story.
- Auto-merge on approve. Approve leaves the PR open; the user merges manually.

## 3. Deployment

### 3.1 Target

Hetzner CAX11 in Nuremberg (NBG1): 2 vCPU ARM64 Ampere, 4 GB RAM, 40 GB NVMe, Ubuntu 24.04. Provisioned 2026-04-23, public IPv4 `178.105.44.3`. Cost: $6.09/mo USD ($5.49 server + $0.60 IPv4). Root SSH with the user's ed25519 key.

Rationale: Railway (which the user already pays for) cannot host era because era's safety model depends on spawning sibling Docker containers with iptables uid-owner egress rules. Railway gives you no Docker socket access and no nested containerization. Hetzner VPS keeps M2's security model intact.

### 3.2 On-host layout

```
/etc/era/
├── env                         mode 600, owner era:era. KEY=value env file.
└── github-app.pem              mode 600, owner era:era.

/opt/era/                       owner era:era. Source checkout.
├── bin/orchestrator            aarch64 linux, built on the box.
├── migrations/                 (embedded in binary; kept for dev)
└── pi-agent.db                 SQLite. Backed up nightly.

/var/backups/era/
└── pi-agent-YYYYMMDD.db.gz     7 daily dumps (sqlite3 .backup | gzip).

/etc/systemd/system/
└── era.service                 User=era, Group=era, restart=on-failure,
                                EnvironmentFile=/etc/era/env,
                                NoNewPrivileges=true, ProtectSystem=strict,
                                ReadWritePaths=/opt/era /var/backups/era.

/etc/cron.d/era-backup          03:00 nightly dump + prune >7d.
```

Docker is host-native. The `era` user is in the `docker` group. Orchestrator shells out to `docker run` unchanged from today.

### 3.3 Install flow

One-shot `deploy/install.sh` run as root on a fresh VPS. Idempotent — re-runnable safely.

1. `apt update && apt install -y docker.io docker-buildx golang-go make git rsync sqlite3 ufw`
2. Create `era` user, add to `docker` group, copy `/root/.ssh/authorized_keys` to `/home/era/.ssh/authorized_keys` (mode 600, owner era:era).
3. `mkdir -p /opt/era /etc/era /var/backups/era`, set ownership.
4. UFW: allow 22/tcp in, deny everything else inbound, enable.
5. Enable `unattended-upgrades`.
6. Drop in `deploy/era.service` + `deploy/era-backup.cron` → `systemctl daemon-reload && systemctl enable era`.
7. Print next steps: the user manually scp's `.env` + `github-app.pem` to `/etc/era/`, then tests ssh-as-era works, then runs `deploy/disable-root-ssh.sh` as a separate step.

### 3.4 Deploy flow

Local `make deploy` from the user's Mac:

```
rsync -az --delete \
    --exclude bin/ --exclude pi-agent.db --exclude node_modules/ --exclude .env \
    ./  era@178.105.44.3:/opt/era/
ssh era@178.105.44.3 '
    cd /opt/era
    make build                   # go build -o bin/orchestrator ./cmd/orchestrator
    make docker-runner           # rebuild runner image (cached if unchanged)
    sudo systemctl restart era
    sudo systemctl status era --no-pager
'
```

Fast path: no Dockerfile changes = image not rebuilt (docker build cache). Systemd restart is <1s.

### 3.5 State migration (one-time)

The Mac currently runs orchestrator + holds `.env` + GH App PEM + `pi-agent.db`. Cutover is manual:

1. On the Mac: stop orchestrator process (`kill <pid>`).
2. `scp .env era@178.105.44.3:/etc/era/env`
3. `scp ~/Downloads/era-orchestrator.*.private-key.pem era@178.105.44.3:/etc/era/github-app.pem`
4. `scp pi-agent.db era@178.105.44.3:/opt/era/pi-agent.db`
5. On the VPS: `sudo chown era:era /etc/era/env /etc/era/github-app.pem /opt/era/pi-agent.db && sudo chmod 600 /etc/era/env /etc/era/github-app.pem` — scp lands files as the uploading user (era) but chmod resets to 600 to match §3.2's declared permissions.
6. Update `PI_GITHUB_APP_PRIVATE_KEY_PATH` in the VPS's `/etc/era/env` to `/etc/era/github-app.pem`.
7. `ssh era@178.105.44.3 'sudo systemctl start era'`
8. Verify `/list` in Telegram returns prior task history.
9. After confirming era user SSH works: `ssh root@178.105.44.3 bash /opt/era/deploy/disable-root-ssh.sh`.

SQLite is **scp'd, not rsync'd** — atomic copy-then-move avoids partial-file read during ongoing writes. But orchestrator is already stopped on the Mac by step 1, so the DB is quiescent anyway.

### 3.6 Hardening baseline

- UFW: `ufw allow 22/tcp`, `ufw default deny incoming`, `ufw default allow outgoing`, `ufw enable`.
- SSH: password auth disabled (Ubuntu 24.04 default), root login disabled **after** era-user SSH is confirmed (staged).
- `era` user: non-root, in `docker` group, runs the service. Can admin via sudo (added to sudoers with NOPASSWD for `systemctl restart era` and `systemctl status era` only — nothing else).
- `unattended-upgrades`: enabled, security patches auto-applied.
- Nightly SQLite backup: `/etc/cron.d/era-backup` runs `sqlite3 /opt/era/pi-agent.db '.backup /tmp/era.bak' && gzip < /tmp/era.bak > /var/backups/era/pi-agent-$(date +\%Y\%m\%d).db.gz && rm /tmp/era.bak && find /var/backups/era -mtime +7 -delete` at 03:00 UTC.

## 4. Read-only answer path

### 4.1 Problem

`cmd/runner/main.go:84` uses `piSummary()` which returns `"ok_tokens=X_cost=Y"` for committed tasks; line 93 hard-codes `"no_changes"` for uncommitted. Pi's actual text output is never captured. `/task <repo> what does X do?` returns a useless DM.

Confirmed during M3.5 smoke: task #8 (`what is this repo about?`) came back with summary literally `no_changes` — agent's answer lost.

### 4.2 Change

Pi emits `message_end` events with an assistant-role `content[]` array of `{type:"text", text:"..."}` blocks. Runner currently parses only `usage` / `stopReason` from those events, ignoring `content`.

**`cmd/runner/events.go`** — extend `piEvent.Message`:

```go
Role    string `json:"role,omitempty"`
Content []struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
} `json:"content,omitempty"`
```

**`cmd/runner/pi.go`** — in the event-stream switch, on `message_end` with `role=="assistant"`, concat all `content[].text` blocks into `summary.LastText` (overwriting each time, so the LAST assistant message wins). Add `LastText string` to `runSummary`.

**`cmd/runner/main.go`** — replace both `piSummary(...)` calls and the `"no_changes"` literal with `summary.LastText`. Defensive fallback: if `LastText == ""` (Pi emitted zero assistant text, e.g., exited on tool loop without a final message), use `"(no final message from pi)"`. Keep the `aborted_<reason>` string for hard errors.

**`cmd/orchestrator/main.go`** — new helper `truncateForTelegram(s, 3500)` used inside `NotifyCompleted` and `NotifyNeedsReview` to cap the summary field in the DM. Appends `\n…(N chars truncated)` footer when over. Budget 3500 leaves ~600 chars for the header/PR link/cost line within Telegram's 4096 cap.

**No DB migration.** `tasks.summary` is `TEXT`, accepts long strings.

### 4.3 Tests

- `cmd/runner/events_test.go` — parse fixture JSONL line (real Pi output copied into a testdata file), assert text blocks extracted.
- `cmd/runner/pi_test.go` — stream of 3 events ending in `message_end` with assistant text, assert `summary.LastText` set. Stream of 2 consecutive `message_end`s, assert second text wins. Stream with only tool events, assert `LastText == ""`.
- `cmd/orchestrator/truncate_test.go` — table test: under budget → passthrough; exactly at budget → passthrough; over budget → footer with correct count; unicode-safe (multi-byte chars not split mid-rune).
- `cmd/runner/main_test.go` — golden JSON test for the three cases: commits+LastText, no-commits+LastText, no-commits+no-LastText.

### 4.4 Phase gate

After tests green, live Telegram smoke:
- `/task vaibhav0806/trying-something what does the README say?` → DM shows prose (not `no_changes`).
- `/task vaibhav0806/trying-something add a file FOO.md with content bar` → DM shows Pi's completion summary (not `ok_tokens=…`).

## 5. Running-task cancel

### 5.1 Problem

`internal/queue/queue.go` `CancelTask` only accepts status `queued`. Mid-run cancel returns "cannot cancel". User must wait for the 60-min wall-clock cap.

### 5.2 Mechanic

**Container registry.** New type in `internal/queue/running.go`:

```go
type runningSet struct {
    mu     sync.Mutex
    m      map[int64]string // taskID → containerName
    killed map[int64]bool   // set by Kill(), read by RunNext
}

func (r *runningSet) Register(taskID int64, containerName string)
func (r *runningSet) Deregister(taskID int64)
func (r *runningSet) Get(taskID int64) (string, bool)
func (r *runningSet) MarkKilled(taskID int64)
func (r *runningSet) WasKilled(taskID int64) bool
```

Owned by `Queue`. Single instance.

**Runner invocation.** `internal/runner/docker.go` currently runs without `--name`. Change: generate `containerName := fmt.Sprintf("era-runner-%d-%d", taskID, rand)` once per run, pass to `docker run --name <name>`. Register in `runningSet` BEFORE `cmd.Run()`, Deregister in a `defer`.

**Kill interface.**

```go
// internal/queue/killer.go
type ContainerKiller interface {
    Kill(ctx context.Context, containerName string) error
}
```

Real impl: `exec.Command("docker", "kill", name)`. Fake impl for tests records calls.

**CancelTask extended.**

```go
func (q *Queue) CancelTask(ctx context.Context, id int64) error {
    t, err := q.repo.GetTask(ctx, id)
    if err != nil { return err }
    switch t.Status {
    case "queued":
        return q.repo.SetStatus(ctx, id, "cancelled") // existing path
    case "running":
        name, ok := q.running.Get(id)
        if !ok {
            return errors.New("task is running but container not yet registered; retry shortly")
        }
        q.running.MarkKilled(id)       // flag BEFORE kill so RunNext's race is safe
        if err := q.killer.Kill(ctx, name); err != nil {
            return fmt.Errorf("docker kill: %w", err)
        }
        // Don't set status here. RunNext observes cmd.Wait() error, checks
        // WasKilled, and writes status=cancelled itself. Single writer.
        return nil
    default:
        return fmt.Errorf("cannot cancel task in state %q", t.Status)
    }
}
```

**RunNext observes cancellation.** After `runner.Run` returns error, branch on `q.running.WasKilled(t.ID)`:

```go
if runErr != nil {
    if q.running.WasKilled(t.ID) {
        _ = q.repo.AppendEvent(ctx, t.ID, "cancelled", "{}")
        _ = q.repo.SetStatus(ctx, t.ID, "cancelled")
        if q.notifier != nil { q.notifier.NotifyCancelled(ctx, t.ID) }
        return true, nil
    }
    // existing fail path...
}
```

**Notifier gets NotifyCancelled.** `tgNotifier.NotifyCancelled` sends `"task #%d cancelled mid-run"`.

**Orchestrator restart reconcile.** On startup (`cmd/orchestrator/main.go` after DB open), call `queue.Reconcile(ctx)` which:

```sql
UPDATE tasks SET status='failed', error='orchestrator restart, task state lost'
WHERE status='running';
```

And appends a `reconciled_failed` event per affected task. Prevents ghost `running` rows from a mid-task deploy.

**No DB migration.** `cancelled` is already in the status CHECK enum from M0.

### 5.3 Edge cases

- `/cancel` before `docker run` has started (~10ms window): returns "retry shortly". User retries.
- `/cancel` after completion: falls through to default branch, returns "cannot cancel task in state 'completed'". Acceptable.
- `/cancel` twice: first succeeds, second finds map empty (deregistered), same acceptable error.
- Orchestrator kill/crash mid-run: in-memory map lost; restart reconcile sweeps `running → failed`. Container may still be running on the host; next `make deploy` cycle will restart docker daemon if kernel-level, but a stale container is harmless — it will eventually hit its 60-min cap and exit cleanly (no writes because it was never pushed anyway).

### 5.4 Tests

- `internal/queue/running_test.go` — Register/Deregister/Get/MarkKilled/WasKilled under goroutine load (go test -race).
- `internal/queue/queue_cancel_test.go` — table: cancel queued, cancel running-with-container, cancel running-without-container, cancel completed, cancel twice.
- `internal/queue/queue_run_test.go` — extend: fake runner returns error + running.killed[id]=true preset → RunNext writes status=cancelled, calls NotifyCancelled, NOT NotifyFailed.
- `internal/queue/reconcile_test.go` — seed DB with `running` row, call Reconcile, assert `failed` with correct reason + event row.
- `internal/runner/docker_test.go` — verify `--name era-runner-<id>-<rand>` in built argv; integration behind `DOCKER_E2E=1` env gate.

### 5.5 Phase gate

Live Telegram: submit `/task vaibhav0806/trying-something slow task, sleep 60` or similar. While running, `/cancel <id>`. Assert DM "cancelled mid-run" within 2s, no branch on GitHub, DB `status='cancelled'`, events log shows `cancelled`.

## 6. PR creation

### 6.1 Change

Flow delta:

```
OLD (M3.5):
  runner pushes branch → diff-scan →
    clean:   NotifyCompleted(branch URL)
    flagged: NotifyNeedsReview(compare URL + buttons)

NEW (M4):
  runner pushes branch → orchestrator opens PR → diff-scan →
    clean:   NotifyCompleted(PR URL)
    flagged: NotifyNeedsReview(PR URL + inline diff + buttons)

  Approve  → task.status = approved. PR stays open. User merges manually.
  Reject   → Close PR → delete branch → task.status = rejected.
```

Never auto-merges. Explicit policy.

### 6.2 New package: `internal/githubpr/`

```go
type Client struct {
    tokens TokenSource
    http   *http.Client
}

type CreateArgs struct {
    Repo, Head, Base, Title, Body string
}
type PR struct { Number int; URL string; HTMLURL string }

func (c *Client) Create(ctx, args) (*PR, error)
func (c *Client) Close(ctx, repo string, number int) error
func (c *Client) DefaultBranch(ctx, repo string) (string, error)
```

Thin HTTPS wrappers:
- `POST /repos/{owner}/{repo}/pulls` → returns PR with number + html_url.
- `PATCH /repos/{owner}/{repo}/pulls/{number}` body `{"state":"closed"}`.
- `GET /repos/{owner}/{repo}` → `default_branch` field.

Same `TokenSource` abstraction as `internal/githubbranch/` + `internal/githubcompare/`. Each call mints a fresh installation token via the existing App flow.

### 6.3 PR title and body

```
Title: [era] {first 60 chars of task description}
Body:
  Task #{id}
  Branch: {branch}
  Tokens: {tokens}  Cost: ${cost:.4f}

  ---
  {Pi's LastText — truncated to 2500 chars in the PR body}

  ---
  Generated by era. Do not merge without reading the diff.
```

### 6.4 Queue wiring (`internal/queue/queue.go`)

New dep:

```go
type PRCreator interface {
    Create(ctx context.Context, args githubpr.CreateArgs) (*githubpr.PR, error)
    Close(ctx context.Context, repo string, number int) error
    DefaultBranch(ctx context.Context, repo string) (string, error)
}
```

Injected into `Queue` (same pattern as `branchDeleter`, `compare`, `notifier`). Nil-checked so unit tests without a PR client still work.

In `RunNext`, after successful commit/push:

```go
base, err := q.prCreator.DefaultBranch(ctx, effectiveRepo)
if err != nil {
    _ = q.repo.AppendEvent(ctx, t.ID, "default_branch_fallback", quoteJSON(err.Error()))
    base = "main"
}
pr, prErr := q.prCreator.Create(ctx, githubpr.CreateArgs{
    Repo:  effectiveRepo,
    Head:  branch,
    Base:  base,
    Title: "[era] " + truncate(t.Description, 60),
    Body:  composePRBody(t.ID, branch, summary.LastText, tokens, costCents),
})
var prURL string
var prNumber int
if prErr != nil {
    _ = q.repo.AppendEvent(ctx, t.ID, "pr_create_error", quoteJSON(prErr.Error()))
    prURL = fmt.Sprintf("https://github.com/%s/tree/%s", effectiveRepo, branch) // fallback
} else {
    prNumber = pr.Number
    prURL = pr.HTMLURL
    _ = q.repo.AppendEvent(ctx, t.ID, "pr_opened", prPayload(pr))
    _ = q.repo.SetPRNumber(ctx, t.ID, int64(prNumber))
}
```

Diff-scan then runs with the same `base` (fixes the hardcoded `"main"` at queue.go:186).

**PR-create failure semantics.** If `prCreator.Create` fails, the task still proceeds through the rest of the pipeline: diff-scan runs, status can become `completed` or `needs_review` normally, and `prURL` is the branch tree URL passed to the notifier. The task does not move to `needs_review` because of the PR failure itself — `needs_review` is a diff-scan outcome, not a GitHub-API outcome. Reject path §6.6 tolerates a null `pr_number` (skips `Close`, still deletes branch).

**Default-branch fallback safety.** The `base = "main"` fallback is correct for every repo the user currently targets (sandbox and trying-something both default to main). If a future target repo defaults to something else AND `DefaultBranch` also fails, the PR would open against a non-existent `main` — GitHub's API returns 422, which is caught by the same `prErr` path (falls back to branch URL). No silent wrong-base PR.

### 6.5 DB migration 0006

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN pr_number INTEGER;
-- +goose Down
SELECT 1;
```

New sqlc query `SetPRNumber(id int64, pr_number int64)` — takes a plain int64 because `pr_number` is always assigned once on successful PR creation and never cleared. Column type is `INTEGER NULL` (SQLite default), so sqlc generates the Go field as `sql.NullInt64` for reads. The `SELECT 1;` down-migration matches migrations 0004 and 0005's pattern — goose requires at least one SQL statement under the `-- +goose Down` directive, even for no-op rollbacks.

### 6.6 Reject path change (`internal/queue/queue.go`)

```go
func (q *Queue) RejectTask(ctx context.Context, id int64) error {
    t, err := q.repo.GetTask(ctx, id)
    if err != nil { return err }
    if t.Status != "needs_review" {
        return fmt.Errorf("cannot reject task in state %q", t.Status)
    }
    // 1. Close PR first (so GitHub doesn't auto-close dangling PR on branch delete).
    if t.PRNumber.Valid && q.prCreator != nil {
        if err := q.prCreator.Close(ctx, t.TargetRepo, int(t.PRNumber.Int64)); err != nil {
            _ = q.repo.AppendEvent(ctx, id, "pr_close_error", quoteJSON(err.Error()))
            // Continue — don't block state machine on GH hiccups.
        } else {
            _ = q.repo.AppendEvent(ctx, id, "pr_closed", "{}")
        }
    }
    // 2. Delete branch.
    if t.BranchName.Valid && q.branchDeleter != nil {
        if err := q.branchDeleter.DeleteBranch(ctx, t.TargetRepo, t.BranchName.String); err != nil {
            _ = q.repo.AppendEvent(ctx, id, "branch_delete_error", quoteJSON(err.Error()))
        } else {
            _ = q.repo.AppendEvent(ctx, id, "branch_deleted", "{}")
        }
    }
    // 3. Transition task regardless — the user's intent was reject.
    if err := q.repo.SetStatus(ctx, id, "rejected"); err != nil {
        return fmt.Errorf("set status: %w", err)
    }
    _ = q.repo.AppendEvent(ctx, id, "rejected", "{}")
    return nil
}
```

### 6.7 Notifier signature changes

```go
type Notifier interface {
    NotifyCompleted(ctx, taskID int64, repo, branch, prURL, summary string, tokens int64, costCents int)
    NotifyFailed(ctx, taskID int64, reason string)
    NotifyNeedsReview(ctx, args NeedsReviewArgs) // PRURL replaces CompareURL in args
    NotifyCancelled(ctx, taskID int64)
}
```

`tgNotifier` message bodies use the PR URL.

### 6.8 GitHub App permission prerequisite

**Manual step the user must do before phase V ships:** visit https://github.com/settings/apps/era-orchestrator → Permissions → repository permissions → change `Pull requests` from `No access` to `Read and write` → save → accept the permission update on the installation. Takes ~30s. Plan will call this out as a blocking prerequisite, not a code task.

### 6.9 Tests

- `internal/githubpr/client_test.go` — `httptest.Server` fake returning canned JSON. Assert Create POSTs correct body, parses number/url/html_url. Close PATCHes state=closed. DefaultBranch reads `default_branch`.
- `internal/githubpr/client_test.go` — auth: injects `Authorization: token ghs_…` from a fake TokenSource.
- `internal/queue/queue_pr_test.go` — fake PRCreator; successful Run → Create called with right args, pr_number stored in DB, NotifyCompleted receives PR HTMLURL.
- `internal/queue/queue_pr_test.go` — PR creation fails → task completes anyway, event `pr_create_error` logged, DM falls back to branch URL.
- `internal/queue/queue_pr_test.go` — DefaultBranch fails → falls back to "main", PR still created with base=main.
- `internal/queue/queue_reject_test.go` — on reject with pr_number set, PRCreator.Close called BEFORE branchDeleter.Delete; order asserted.
- `internal/queue/queue_reject_test.go` — on reject with pr_number null (fallback case), branchDeleter called but PRCreator.Close NOT called.

### 6.10 Phase gate

Live, in order:
1. `/task vaibhav0806/trying-something add file PR_TEST.md with content hello` → DM contains `https://github.com/…/pull/N`, clicking opens the PR, showing diff + body formatted correctly.
2. Submit a task crafted to trip diff-scan (e.g. "delete test file X") → DM is needs_review with PR link + inline diff; tap Approve → PR stays open on GitHub, task status flips to approved.
3. Submit another diff-scan tripper → tap Reject → PR shows as closed on GitHub, branch deleted, task status `rejected`.

## 7. Phases

Each phase is its own commit (or tight series), each gated by `go test -race -count=1 ./...` plus a phase smoke script before moving on.

| Phase | Scope | Gate |
|-------|-------|------|
| **T** | Read-only answer path (§4): events parser, LastText, truncation, notifier uses LastText | Full regression green + `scripts/smoke/phase_t_readonly.sh` + live `/task <repo> what does the README say?` returns prose |
| **U** | `internal/githubpr/` package (§6.2), no queue wiring | `go test ./internal/githubpr/... -race` green |
| **V** | Queue wires PRCreator + migration 0006 + Notifier signature change + reject closes PR (§6.4–6.7) | Full regression + `scripts/smoke/phase_v_prs.sh` + live clean-task PR + live reject path |
| **W** | Running-task cancel (§5): runningSet, --name, killer, CancelTask, NotifyCancelled, Reconcile | Full regression + `scripts/smoke/phase_w_cancel.sh` + live slow-task /cancel within 2s |
| **X** | Deploy artifacts: `deploy/install.sh`, `deploy/era.service`, `deploy/era-backup.cron`, `deploy/env.template`, `deploy/disable-root-ssh.sh`, `Makefile` `deploy` target | Install script runs idempotently on VPS (era user created, service enabled, UFW on, docker installed); no era running yet |
| **Y** | Live migration + cutover (§3.5): kill Mac orchestrator, scp state, start on VPS, verify, disable root SSH | Live `/list` from VPS shows prior history + live `/task` round-trip works + `journalctl -u era` clean |

## 8. Testing philosophy

TDD for every new function. Fail-first tests, then minimal code to pass. Full `go test -race -count=1 ./...` green before any commit. Phase smoke scripts kept for future regression. Manual Telegram smoke at every phase gate — no phase is "done" until the live round-trip passes. We are cautious, we are serious, we test everything before moving ahead.

## 9. Risk log

1. **GH App permission update** (§6.8) is a manual prerequisite. Plan will surface as first-task blocker for phase V.
2. **Root-SSH disable lockout.** Install script verifies era user's authorized_keys is present and prints a "TEST SSH AS era NOW" prompt. Disable is a separate script run only after user confirms era SSH works.
3. **SQLite copy during live writes.** Mitigated by stopping the Mac orchestrator before scp. Step 1 of cutover, explicit.
4. **ARM/x86 drift.** Mac (arm64) and VPS (arm64) match; cross-check with `GOARCH=amd64 go test ./...` locally before shipping phase X to catch any architecture assumptions.
5. **Ghost running tasks** after crash/deploy. Reconcile on startup (§5.2) sweeps `running → failed`.

## 10. Out-of-scope items explicitly listed here so future-you doesn't add them to M4

- Push-based CI deploys
- S3/Backblaze offsite backups
- `/ask`-style read-only command without container spin-up
- Multi-turn follow-ups on read-only answers
- Attaching long summaries as Telegram file uploads
- PR merge queue / auto-merge
- Pre-push approval gate (Option A)
- Secret-store integration (1Password CLI, Infisical, Doppler)
