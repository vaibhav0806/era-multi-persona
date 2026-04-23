# M5 — Polish & Safety Design

**Date:** 2026-04-24
**Status:** design, pre-implementation
**Predecessor:** M4 (deployment + PRs + mid-run cancel + read-only answers)

## 1. Goal

Make era iterate faster and safer on its now-production Hetzner install, closer to the real GitHub workflow, without regressing the security posture built in M2–M4. Seven self-contained chunks:

1. Cleanup batch (Bearer auth, env template, gitignore).
2. Looser wildcarded sudoers for the `era` user.
3. Pre-bake language toolchains into the runner image (Node, Python, Rust + build deps) so Pi stops hitting blocked Alpine CDN.
4. PR approval feedback on GitHub (label + review on approve; comment on reject).
5. Pre-commit test gate in the runner when the repo has a `Makefile` with a `test` target.
6. Offsite nightly backups to Backblaze B2 with 30-day retention.
7. GitHub Actions CI: push to master runs a test matrix; on green, auto-deploys to the VPS.

## 2. Non-goals (deferred to M6+)

- `/ask` read-only shortcut — safety regression; revisit later.
- Natural-language repo parsing.
- Option-A pre-push approval (diff-scan BEFORE push).
- Long-answer Telegram file-upload attachments.
- PR auto-merge on approve.
- Dev/prod bot split.
- Task chaining / scheduled tasks.
- Task stats dashboard / `/stats` command.
- Egress allowlist expansion (Alpine CDN, arbitrary hosts).
- Java/PHP/Ruby/Kotlin toolchains (bake-on-demand later).
- sqlc drift check in CI.
- Cross-arch build verification in CI.
- Prometheus / metrics export.
- Slack / email alerts beyond GitHub Actions default.

## 3. Architecture overview

Zero orchestrator-core changes. The existing `internal/queue`, `cmd/orchestrator`, `cmd/runner` packages stay structurally unchanged. M5 touches:

- `internal/githubpr/` — new `ApprovePR`, `AddLabel`, `AddComment` methods; auth header `token` → `Bearer`.
- `internal/queue/queue.go` — `ApproveTask` and `RejectTask` gain calls to the new `githubpr` methods.
- `cmd/runner/` — new `pretest.go` with `HasMakefileTest` + `RunMakefileTest`; `git.go` hooks it between Pi's exit and `CommitAndPush`.
- `docker/runner/Dockerfile` — one new `RUN apk add` layer with Node/Python/Rust toolchains + build deps + utilities.
- `deploy/` — new `sudoers-era`, new `rclone.conf.template`, extended `era-backup.sh`, updated `install.sh`.
- `.github/workflows/ci.yml` — brand new workflow.

### 3.1 Deploy flow change

| | M4 (current) | M5 (target) |
|---|---|---|
| Trigger | `make deploy` from Mac | `git push` from Mac |
| Pipeline | rsync → ssh → build → restart | GitHub Actions runs test gate; on green, ssh era@VPS → `git pull` → build → restart |
| Fallback | — | `make deploy` retained as emergency/manual redeploy path |

### 3.2 Security boundary (unchanged)

- Runner container: still iptables-locked to the M2 allowlist. No Alpine CDN added. Pre-baked tools prevent Pi needing `apk add` at task time.
- VPS SSH: still era-user-only (root locked since M4-Y-6). New CI keypair is a separate key on era's `authorized_keys`; existing Mac key unaffected.
- GitHub App perms: unchanged. `Pull requests: Read and write` covers reviews, labels, and issue comments (labels/comments use the Issues API but PRs are Issues in GitHub's model).
- B2 app key: scoped to the `era-backups` bucket with write-only permission. Separate read-only key for restore, minted manually when needed.

### 3.3 Data flow additions

```
Nightly 03:00 UTC (cron on VPS)
   └── era-backup.sh
         ├── sqlite3 .backup → /tmp + gzip → /var/backups/era/pi-agent-YYYYMMDD.db.gz  (existing, local, 7d retention)
         └── rclone copy → b2:era-backups/                                              (new, offsite, 30d lifecycle)

Push to master (Mac)
   └── GitHub Actions
         ├── job: test      (go vet + gofmt + go test -race + go build + phase smokes)
         └── job: deploy    (needs test; ssh era@VPS → git pull → make build + docker-runner → systemctl restart)
```

## 4. Per-chunk mechanics

### 4.1 Chunk 1 — Cleanup batch (phase Z)

Three trivial tweaks, separate commits.

**4.1.a `internal/githubpr` Bearer auth.** In `client.go:newReq`, change:

```go
req.Header.Set("Authorization", "token "+tok)
```

to:

```go
req.Header.Set("Authorization", "Bearer "+tok)
```

Update `client_test.go` assertions (`require.Equal(t, "token ghs_test", ...)` → `"Bearer ghs_test"`). Matches the convention in `internal/githubbranch/delete.go:48` and `internal/githubcompare/compare.go:52`.

**4.1.b Drop unused env var.** Remove the `PI_GITHUB_APP_PRIVATE_KEY_PATH=/etc/era/github-app.pem` line from `deploy/env.template`. Config loader doesn't read it; era uses `PI_GITHUB_APP_PRIVATE_KEY` (base64 inline). Leaving it in the template is misleading.

**4.1.c .gitignore verification.** `/runner` and `/sidecar` were added in M3.5. Verify they're present and that no stray VPS-synced binaries slip into commits. No change if already correct.

### 4.2 Chunk 2 — Looser sudoers (phase AA)

New file `deploy/sudoers-era`:

```
era ALL=(root) NOPASSWD: /usr/bin/systemctl * era, /usr/bin/systemctl * era *, /usr/bin/journalctl -u era, /usr/bin/journalctl -u era *
```

Four entries. Still narrow (only the `era` unit). Covers all the flag combinations that broke the strict M4 rule (`sudo systemctl status era --no-pager`, `sudo journalctl -u era -n 40`).

`deploy/install.sh` change: replace the inline heredoc with:

```bash
install -m 440 /opt/era/deploy/sudoers-era /etc/sudoers.d/era
visudo -c -f /etc/sudoers.d/era || { echo "bad sudoers"; exit 1; }
```

One-time live push for the running VPS. The narrow existing sudoers doesn't let era install a new sudoers file, so we need a brief root-SSH window. Three explicit ordered steps (plan writes these out in Phase AA; do not skip the re-disable):

1. Re-enable root SSH via Hetzner web console: console → Rescue/Console → run `sed -i -E 's/^PermitRootLogin no/PermitRootLogin yes/' /etc/ssh/sshd_config && systemctl reload ssh`.
2. From Mac: `scp deploy/sudoers-era root@178.105.44.3:/tmp/sudoers-era`. Then `ssh root@178.105.44.3 'install -m 440 /tmp/sudoers-era /etc/sudoers.d/era && visudo -c -f /etc/sudoers.d/era && rm /tmp/sudoers-era'`.
3. **Immediately** re-disable root SSH: `ssh root@178.105.44.3 'bash /opt/era/deploy/disable-root-ssh.sh'`. Verify `ssh root@...` fails and `ssh era@...` still works before closing the phase.

After phase AA ships, all future deploys use the widened sudoers and no longer need the console.

### 4.3 Chunk 3 — Runner tooling bake (phase AB)

`docker/runner/Dockerfile` gets one new `RUN` block (inserted after the base-image `apk add` but before the Go toolchain install, so Go's known-good state layer stays cached on rebuild):

```dockerfile
RUN apk add --no-cache \
    # language toolchains
    nodejs npm python3 py3-pip py3-virtualenv rust cargo \
    # native build deps
    build-base musl-dev pkgconf openssl-dev libffi-dev python3-dev sqlite-dev zlib-dev \
    # utilities
    sqlite tar gzip unzip tree ripgrep fd \
    && rm -rf /var/cache/apk/*
```

Alpine package naming verifies during implementation:
- `ripgrep` provides the `rg` binary (asserted by smoke).
- `fd` provides the `fd` binary on Alpine 3.19+ (older versions called it `fd-find`). If `fd` fails, fallback to `fd-find`.
- Everything else is stable across recent Alpine versions.

New smoke script `scripts/smoke/phase_ab_tooling.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
docker run --rm era-runner:m2 sh -c '
    node --version
    npm --version
    python3 --version
    pip3 --version
    cargo --version
    rustc --version
    rg --version
    fd --version
    tree --version
' > /dev/null
echo "OK: phase AB — runner tooling bake all versions present"
```

Image size: current ~600MB → estimated ~1.2-1.5GB. Build time: ~1-2 min → ~3-5 min on cache miss. First build after merge is the slowest; subsequent builds cached.

Live gate: re-run the failed URL-shortener task (`build a complete URL shortener in Go ... Vite react frontend` — the one that hit blocked Alpine CDN in M5 brainstorm). Should succeed.

### 4.4 Chunk 4 — PR approval feedback (phase AC)

Three new methods on `internal/githubpr.Client`:

```go
// ApprovePR submits an APPROVED review on the PR with an optional body.
// Endpoint: POST /repos/{owner}/{repo}/pulls/{number}/reviews
// Body: {"event":"APPROVE","body":"<body>"}
func (c *Client) ApprovePR(ctx context.Context, repo string, number int, body string) error

// AddLabel adds a label to a PR (PRs are issues in GitHub's model for labels).
// Endpoint: POST /repos/{owner}/{repo}/issues/{number}/labels
// Body: {"labels":["<label>"]}
func (c *Client) AddLabel(ctx context.Context, repo string, number int, label string) error

// AddComment posts a plain issue comment on a PR (again, PRs-are-issues).
// Endpoint: POST /repos/{owner}/{repo}/issues/{number}/comments
// Body: {"body":"<body>"}
func (c *Client) AddComment(ctx context.Context, repo string, number int, body string) error
```

Each is a thin HTTP wrapper following the same pattern as existing `Create` / `Close` / `DefaultBranch`. Each uses `c.newReq(...)` helper for auth + headers. All three tolerate 404/403 by returning the error — caller logs it as an event but does not block the state machine.

`PRCreator` interface in `internal/queue/queue.go` expands from 3 methods to 6:

```go
type PRCreator interface {
    Create(ctx, args) (*githubpr.PR, error)
    Close(ctx, repo string, number int) error
    DefaultBranch(ctx, repo string) (string, error)
    ApprovePR(ctx, repo string, number int, body string) error
    AddLabel(ctx, repo string, number int, label string) error
    AddComment(ctx, repo string, number int, body string) error
}
```

Existing fakes in `queue_pr_test.go` and `queue_reject_test.go` are extended with the three new methods, defaulting to nil-error no-ops. Any test not explicitly asserting on the new methods just ignores them.

**Queue wiring — ApproveTask:**

```go
func (q *Queue) ApproveTask(ctx context.Context, id int64) error {
    task, err := q.repo.GetTask(ctx, id)
    if err != nil { return err }
    switch task.Status {
    case "approved":
        return nil // idempotent, preserved
    case "needs_review":
        // fall through
    default:
        return fmt.Errorf("cannot approve task in state %q", task.Status)
    }

    effectiveRepo := task.TargetRepo
    if effectiveRepo == "" { effectiveRepo = q.repoFQN }

    if task.PrNumber.Valid && q.prCreator != nil {
        n := int(task.PrNumber.Int64)
        if err := q.prCreator.AddLabel(ctx, effectiveRepo, n, "era-approved"); err != nil {
            _ = q.repo.AppendEvent(ctx, id, "pr_label_error", quoteJSON(err.Error()))
        } else {
            _ = q.repo.AppendEvent(ctx, id, "pr_labeled", "{}")
        }
        if err := q.prCreator.ApprovePR(ctx, effectiveRepo, n, "Approved via era Telegram bot."); err != nil {
            _ = q.repo.AppendEvent(ctx, id, "pr_review_error", quoteJSON(err.Error()))
        } else {
            _ = q.repo.AppendEvent(ctx, id, "pr_reviewed_approved", "{}")
        }
    }

    if err := q.repo.SetStatus(ctx, id, "approved"); err != nil {
        return err
    }
    _ = q.repo.AppendEvent(ctx, id, "approved", "{}")
    return nil
}
```

**Queue wiring — RejectTask:** insert comment BEFORE the existing Close + DeleteBranch sequence.

```go
// ... existing idempotency + status check ...

if task.PrNumber.Valid && q.prCreator != nil {
    n := int(task.PrNumber.Int64)
    body := rejectionCommentBody(task, findings) // helper in queue/reject_body.go
    if err := q.prCreator.AddComment(ctx, effectiveRepo, n, body); err != nil {
        _ = q.repo.AppendEvent(ctx, id, "pr_comment_error", quoteJSON(err.Error()))
    } else {
        _ = q.repo.AppendEvent(ctx, id, "pr_commented_rejected", "{}")
    }
    // existing Close call:
    if err := q.prCreator.Close(ctx, effectiveRepo, n); err != nil { ... }
}
// existing DeleteBranch call unchanged
```

`rejectionCommentBody` composes from the findings stored at diffscan time. If findings aren't retrievable from the DB (no diffscan event for this task), the body is just `"✗ Rejected via era Telegram bot. Branch deleted."`.

**Label name:** `era-approved`. Auto-created by GitHub on first use (since ~2018).

**GitHub App permission check:** `Pull requests: Read and write` already grants `pulls/:n/reviews` POST. Labels and comments use the Issues API, which is covered by `Issues: write` OR `Pull requests: write` (PRs are issues). Verified — no permission update needed.

Tests:
- `internal/githubpr/client_test.go` — new `TestApprovePR_*`, `TestAddLabel_*`, `TestAddComment_*` using `httptest.Server` fakes asserting endpoints, bodies, and headers.
- `internal/queue/queue_approve_test.go` — new file: approve invokes AddLabel then ApprovePR in order; PR-label error is logged as event but doesn't block status transition; null pr_number skips GH calls entirely.
- `internal/queue/queue_reject_test.go` — extend: reject posts AddComment BEFORE Close; comment body includes findings if available; PR-comment error is logged as event but doesn't block Close.

### 4.5 Chunk 5 — Pre-commit test (phase AD)

New file `cmd/runner/pretest.go`:

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"
    "time"
)

// makefileTestTarget matches lines like "test:", "test :", or "test: deps".
// We look for an exact "test" target name, not substrings.
var makefileTestTarget = regexp.MustCompile(`(?m)^test\s*:`)

// HasMakefileTest returns true if workspace/Makefile exists and contains a
// `test` target at the start of a line.
func HasMakefileTest(workspace string) bool {
    f, err := os.Open(filepath.Join(workspace, "Makefile"))
    if err != nil { return false }
    defer f.Close()
    sc := bufio.NewScanner(f)
    for sc.Scan() {
        if makefileTestTarget.MatchString(sc.Text()) {
            return true
        }
    }
    return false
}

// RunMakefileTest runs `make test` in workspace with a 10-minute timeout.
// Returns combined stdout+stderr and any error.
func RunMakefileTest(ctx context.Context, workspace string) (string, error) {
    cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
    defer cancel()
    cmd := exec.CommandContext(cctx, "make", "test")
    cmd.Dir = workspace
    out, err := cmd.CombinedOutput()
    if cctx.Err() == context.DeadlineExceeded {
        return string(out), fmt.Errorf("pre-commit test exceeded 10-minute cap")
    }
    return string(out), err
}
```

`cmd/runner/git.go` hook — between Pi exit and `CommitAndPush`:

```go
// ...after Pi exits cleanly, before CommitAndPush...
if HasMakefileTest(workspace) {
    out, testErr := RunMakefileTest(ctx, workspace)
    if testErr != nil {
        slog.Warn("pre-commit test failed", "err", testErr, "out_len", len(out))
        writeResult(os.Stdout, runResult{
            Branch:    "",
            Summary:   "tests_failed: " + truncate(out, 2000),
            Tokens:    tokens,
            CostCents: int(math.Round(costUSD * 100)),
        })
        return fmt.Errorf("pre-commit test failed: %w", testErr)
    }
}
// ... existing CommitAndPush call ...
```

Non-zero runner exit → orchestrator's `RunNext` sees the error → marks task `failed` → DMs the summary. The summary (`tests_failed: <output>`) is truncated to 2000 chars before JSON-encoding via the existing T-0 RESULT pipeline, then truncated again for Telegram DM via `truncateForTelegram(3500)`.

**Do NOT wrap the truncated output in `sanitize()`.** `sanitize` maps whitespace to underscores — a leftover from the pre-T-0 space-delimited RESULT line. With the JSON pipeline, newlines + tabs in test output survive the round-trip cleanly, which is what you want for readable failure DMs.

Tests:
- `cmd/runner/pretest_test.go` — `HasMakefileTest` over fixture workspaces (no Makefile, Makefile-without-test, Makefile-with-test, Makefile-with-test-as-phony); `RunMakefileTest` with a tiny fake Makefile that `@echo ok` → returns empty error, or `@exit 1` → returns error with output.
- `cmd/runner/git_test.go` (extend) — full CommitAndPush path skips when no Makefile; runs and commits when Makefile test passes; runs, fails, and aborts before commit when Makefile test fails.
- Phase smoke: `scripts/smoke/phase_ad_pretest.sh` runs the unit tests for detector + runner.

**Edge cases:**

- Makefile has `.PHONY: test` but no actual `test:` recipe — `make test` fails with "no rule to make target test" → we treat as test failure. That's correct behavior: the target was DECLARED but not IMPLEMENTED, so the user's intent was a test gate.
- Makefile has `test-unit` and `test-integration` but no `test` — `HasMakefileTest` returns false (we only match `^test:`). Task proceeds without gating. Correct; we only gate when the conventional `test` target exists.
- Test target exists and passes but exits with stderr warnings — `make test` returns 0 → we proceed. Correct.

### 4.6 Chunk 6 — Offsite backup (phase AE)

`deploy/era-backup.sh` extended:

```bash
#!/usr/bin/env bash
# Nightly SQLite backup. Local + offsite (B2).
set -euo pipefail
DB=/opt/era/pi-agent.db
OUTDIR=/var/backups/era
STAMP=$(date +%Y%m%d)
TMP=$(mktemp)
trap "rm -f $TMP" EXIT

# --- 1. Local dump (unchanged from M4) ---
sqlite3 "$DB" ".backup $TMP"
gzip -c "$TMP" > "$OUTDIR/pi-agent-$STAMP.db.gz"
chown era:era "$OUTDIR/pi-agent-$STAMP.db.gz"
find "$OUTDIR" -name 'pi-agent-*.db.gz' -mtime +7 -delete

# --- 2. Offsite push (new) ---
if [ -f /etc/era/rclone.conf ] && command -v rclone >/dev/null 2>&1; then
    rclone --config=/etc/era/rclone.conf copy \
        "$OUTDIR/pi-agent-$STAMP.db.gz" b2:era-backups/ \
        --log-level INFO 2>&1 | tee -a /var/log/era-backup.log
else
    echo "$(date -Is) rclone/config missing; skipping offsite push" >> /var/log/era-backup.log
fi
```

Graceful degradation: if rclone isn't installed or config is absent, local backup still runs and logs the skip.

New `deploy/rclone.conf.template` (placeholders only, ships in repo):

```ini
[b2]
type = b2
account = YOUR_B2_ACCOUNT_ID
key = YOUR_B2_APPLICATION_KEY
```

`deploy/install.sh` updates:
- apt list adds `rclone`.
- Template installation: `install -m 600 -o era -g era /opt/era/deploy/rclone.conf.template /etc/era/rclone.conf.template`.
- Install script echoes a new step in the post-install instructions: "cp `/etc/era/rclone.conf.template` to `/etc/era/rclone.conf` and fill in real B2 creds".

Manual one-time B2 setup (user does, documented in plan):
1. Sign up at https://www.backblaze.com/b2/.
2. Create bucket `era-backups`, type Private, default server-side encryption ON.
3. Create application key: name `era-vps-write`, scope `era-backups` bucket, capabilities `writeFiles` + `listFiles` (listFiles needed for rclone to not re-upload identical files).
4. Bucket lifecycle rule: "Keep only the last 30 days".
5. Paste `keyID` and `applicationKey` into `/etc/era/rclone.conf` on VPS.
6. Test manually: `sudo -u era rclone --config=/etc/era/rclone.conf copy /var/backups/era/pi-agent-*.db.gz b2:era-backups/` → check B2 console.

Gate:
- Unit: none (shell script, no unit-test framework).
- Live: manual first-run + next 03:00 UTC cron produces a dated object in B2 bucket.

### 4.7 Chunk 7 — GitHub Actions CI (phase AF)

New `.github/workflows/ci.yml`:

```yaml
name: ci

on:
  push:
    branches: [master]
  workflow_dispatch:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - name: go vet
        run: go vet ./...
      - name: gofmt check
        run: |
          bad="$(gofmt -l .)"
          if [ -n "$bad" ]; then
            echo "unformatted files:"
            echo "$bad"
            exit 1
          fi
      - name: test
        run: go test -race -count=1 ./...
      - name: build
        run: go build ./...
      - name: phase smokes
        run: |
          for f in scripts/smoke/phase_*.sh; do
            echo "--- $f ---"
            bash "$f"
          done

  deploy:
    needs: test
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/master' && github.event_name == 'push'
    steps:
      - name: SSH setup
        env:
          DEPLOY_SSH_KEY: ${{ secrets.DEPLOY_SSH_KEY }}
        run: |
          mkdir -p ~/.ssh
          echo "$DEPLOY_SSH_KEY" > ~/.ssh/id_ed25519
          chmod 600 ~/.ssh/id_ed25519
          ssh-keyscan -H 178.105.44.3 >> ~/.ssh/known_hosts 2>/dev/null
      - name: Deploy
        run: |
          ssh era@178.105.44.3 'cd /opt/era && \
            git pull origin master && \
            export PATH=/usr/local/go/bin:$PATH && \
            make build && \
            make docker-runner && \
            sudo systemctl restart era && \
            sudo systemctl status era'
```

**Manual prerequisites** (user does, documented in plan):

1. Generate CI keypair on Mac:
   ```
   ssh-keygen -t ed25519 -f ~/.ssh/era_ci -C "era-ci@github-actions" -N ""
   ```
2. Install public key on VPS:
   ```
   cat ~/.ssh/era_ci.pub | ssh era@178.105.44.3 'cat >> ~/.ssh/authorized_keys'
   ```
3. Store private key as GitHub Actions secret:
   ```
   gh secret set DEPLOY_SSH_KEY < ~/.ssh/era_ci
   ```
4. Convert VPS `/opt/era` from rsync'd tree to git checkout (ONE TIME):
   ```
   ssh era@178.105.44.3 'cd /opt/era && git init && \
       git remote add origin https://github.com/vaibhav0806/era.git && \
       git fetch && \
       git checkout -B master origin/master'
   ```
   Safe because `.env` lives in `/etc/era/`, `pi-agent.db` is `.gitignore`'d.

**Two-stage commit of ci.yml to avoid "first CI run auto-deploys itself":**

- First commit: `ci.yml` with `deploy` job guarded by `if: false` — tests only.
- Push, confirm test gate is reliable across 2-3 commits.
- Second commit: remove the `if: false`, deploy goes live.

Gate:
- Empty commit pushed to master → Actions "test" job runs green → "deploy" job (once enabled) ssh's to VPS → era.service restart visible in `journalctl -u era`.
- `workflow_dispatch` manual trigger: runs test job (deploy job skips due to `if: github.event_name == 'push'`). Good for re-running after a flake.

## 5. Phase order

| Phase | Chunk | Rationale |
|-------|-------|-----------|
| **Z** | Cleanup batch (§4.1) | Smallest, warms up the M5 branch; zero runtime risk. |
| **AA** | Looser sudoers (§4.2) | Fixes the friction hit twice during M4 + M5 brainstorm. Must ship BEFORE CI (AF) so CI's `sudo systemctl restart era` works. |
| **AB** | Runner tooling bake (§4.3) | Fixes the Pi failure seen on URL-shortener task #23. Live gate = retry that task. |
| **AC** | PR approval feedback (§4.4) | Safe additive feature; extends `githubpr` API. Lower risk than AD's runner hook. |
| **AD** | Pre-commit test (§4.5) | Needs AC's new `githubpr` pattern established but doesn't depend on it. Biggest code-path change. |
| **AE** | Offsite backup (§4.6) | Ship before AF so CI deploys assume backups exist. |
| **AF** | GitHub Actions CI (§4.7) | Ties everything together. CI smoke matrix re-runs phase Z-AE smokes. |

Each phase commits cleanly (`go test -race -count=1 ./...` green), has its own smoke script where applicable, and a live gate before the next phase begins.

## 6. Testing philosophy

TDD for every new function. Fail-first tests before implementation. `go test -race -count=1 ./...` green before every commit. Per-phase smoke scripts kept as regression guards. Live Telegram / GitHub / VPS round-trip smoke at every phase gate — no phase is "done" until the live test passes. Subagent-driven execution: fresh implementer per task, two-stage review (spec compliance + code quality) after each. Same cadence as M4.

**We are cautious. We are serious. We are productive. We do not build blindly.**

## 7. Risk log

1. **CI deploy job has broad blast radius.** A single bad auto-deploy takes prod down. Mitigations: comprehensive test matrix (go test + vet + fmt + smokes + build) before deploy job runs. `workflow_dispatch` manual lets you run tests-only. Rollback plan in a new `deploy/ROLLBACK.md`: `ssh era@VPS; cd /opt/era; git reset --hard <last-good-sha>; make build; sudo systemctl restart era`.

2. **VPS `/opt/era` isn't a git checkout currently** (rsync'd in M4). Phase AF plan step requires `git init` + `git fetch` + `git checkout -B master`. Low risk because: (a) `.env` lives in `/etc/era/`, outside the checkout; (b) `pi-agent.db` is `.gitignore`'d; (c) any local modifications get stashed by git's checkout behavior, recoverable via `git stash pop`. If something goes wrong, fallback is delete+clone+restore.

3. **Runner image growth to ~1.5GB.** Doubles disk + build time. On the 40GB VPS this is ~5% of disk; not an issue. Build time ~3-5 min on cache miss; cache hit after first rebuild.

4. **CI keypair leak.** If `DEPLOY_SSH_KEY` secret leaks, attacker gets era-user shell. Mitigations: narrow sudoers (only `era` unit); rotation is 3 commands (`ssh-keygen` + `gh secret set` + remove old pubkey). Document in `deploy/ROTATE_CI_KEY.md`.

5. **GitHub secondary rate limits on PR reviews API.** For era's ~5 PRs/day, nowhere near the limit. If a burst happens, `ApprovePR` returns error → logged as `pr_review_error` event, task still transitions to `approved`. Graceful.

6. **`make test` inside runner can be slow.** 10-minute `context.WithTimeout` caps wall-clock cost. Tests that exceed fail with "pre-commit test exceeded 10-minute cap".

7. **B2 app key in `/etc/era/rclone.conf` on VPS.** Same threat model as existing `/etc/era/env` and `github-app.pem`. Mode 600, owner era. Compromise of era user gets all three. Accepted trust boundary.

8. **One-time sudoers update on live VPS pre-phase-AA needs root SSH or cloud console.** Plan documents using Hetzner's web console or temporarily re-enabling root SSH for the copy, then locking back down.

## 8. Open questions (resolved here; plan implements)

- `rg` vs `ripgrep`: Alpine pkg `ripgrep`, binary `rg`. Dockerfile uses `apk add ripgrep`; smoke asserts `rg --version`.
- `fd` vs `fd-find`: Alpine 3.19+ pkg is `fd`. Fallback to `fd-find` on older Alpine if `apk add fd` fails during implementation.
- `make test` exit semantics: any non-zero exit treated as test failure.
- Label creation: GitHub auto-creates labels that don't pre-exist when AddLabel references them.
- Two-stage CI commit (disabled → enabled): documented, prevents self-auto-deploy.

## 9. Deliverables

Per-phase, committed to master incrementally:

- Phase Z: 3 commits (Bearer auth, env template, gitignore).
- Phase AA: 2-3 commits (sudoers-era file, install.sh update, live push via recovery channel).
- Phase AB: 2 commits (Dockerfile bake, phase smoke script).
- Phase AC: 4-5 commits (3 new githubpr methods, Queue wiring for approve, Queue wiring for reject, test fixtures).
- Phase AD: 3-4 commits (pretest.go, git.go hook, test updates, phase smoke script).
- Phase AE: 3 commits (backup script extension, rclone template, install.sh update).
- Phase AF: 2 commits (ci.yml with deploy disabled, ci.yml with deploy enabled).

After all phases land:
- `git tag m5-release`
- `git push origin master m5-release`
- README update: M5 status block + roadmap table row.

Estimated sub-tasks: ~35-45. Smaller than M4 in total code surface — the substantive code work is Chunks 4 (PR approval) and 5 (pre-commit test); everything else is configuration, shell scripts, and Docker.
