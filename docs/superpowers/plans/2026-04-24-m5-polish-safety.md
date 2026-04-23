# M5 — Polish & Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make era iterate faster and safer on its production Hetzner install — cleanup the rough edges, loosen sudoers, bake language toolchains into the runner, surface approval state on GitHub, gate pre-commits on tests, push backups offsite, and replace `make deploy` with GitHub Actions CI.

**Architecture:** Seven linear phases (Z → AA → AB → AC → AD → AE → AF), each with its own live gate. No orchestrator-core refactoring. Code changes concentrated in `internal/githubpr/`, `internal/queue/queue.go`, `cmd/runner/`, `docker/runner/Dockerfile`, `deploy/`, and a new `.github/workflows/ci.yml`.

**Tech Stack:** Go 1.25, SQLite via modernc.org/sqlite, Alpine Linux for runner base, GitHub Actions, Backblaze B2, rclone. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-04-24-m5-polish-safety-design.md`.

**Testing philosophy:** Strict TDD. Fail-first tests before implementation. `go test -race -count=1 ./...` green before every commit. Per-phase smoke scripts kept for regression. Live gates at every phase. We do not build blindly.

**Prerequisites (check before starting):**

- `sqlc` v2 installed (unchanged from M4).
- Docker running locally for `make docker-runner` rebuilds.
- SSH access as `era@178.105.44.3` (M4-verified).
- Hetzner web console login (Phase AA one-time console flip).
- B2 account created + bucket `era-backups` + app key + lifecycle rule (Phase AE manual prereq, documented in task AE-3 below).
- GitHub Actions secret write permission on the repo (Phase AF manual prereq, documented in task AF-2 below).

---

## File Structure

```
.github/workflows/
└── ci.yml                        CREATE (phase AF) — test gate + auto-deploy on push to master

docker/runner/
└── Dockerfile                    MODIFY (phase AB) — apk add Node/Python/Rust + build deps + utils

cmd/runner/
├── pretest.go                    CREATE (phase AD) — HasMakefileTest + RunMakefileTest
├── pretest_test.go               CREATE (phase AD) — unit tests + fixtures
├── git.go                        MODIFY (phase AD) — hook pre-commit test before CommitAndPush
├── git_test.go                   MODIFY (phase AD) — skip/pass/fail paths
└── testdata/
    ├── makefile_with_test/       CREATE (phase AD) — fixture
    ├── makefile_no_test/         CREATE (phase AD) — fixture
    └── no_makefile/              CREATE (phase AD) — fixture

internal/githubpr/
├── client.go                     MODIFY (phase Z + AC) — Bearer auth; add ApprovePR, AddLabel, AddComment
└── client_test.go                MODIFY (phase Z + AC) — Bearer header assertions; new method tests

internal/queue/
├── queue.go                      MODIFY (phase AC) — PRCreator interface expansion; ApproveTask + RejectTask wiring
├── queue_pr_test.go              MODIFY (phase AC) — fakePRCreator gains 3 no-op methods
├── queue_approve_test.go         CREATE (phase AC) — ApproveTask calls Label + Review in order; error handling
├── queue_reject_test.go          MODIFY (phase AC) — RejectTask posts Comment BEFORE Close
└── reject_body.go                CREATE (phase AC) — rejectionCommentBody helper + tests

deploy/
├── env.template                  MODIFY (phase Z) — drop unused PI_GITHUB_APP_PRIVATE_KEY_PATH
├── sudoers-era                   CREATE (phase AA) — wildcarded sudoers rule file
├── install.sh                    MODIFY (phase AA + AE + AF) — install sudoers-era from file; add rclone; git-init hint
├── era-backup.sh                 MODIFY (phase AE) — append rclone push to B2
├── rclone.conf.template          CREATE (phase AE) — B2 app-key placeholders
├── ROLLBACK.md                   CREATE (phase AF) — git-reset rollback runbook
└── ROTATE_CI_KEY.md              CREATE (phase AF) — CI keypair rotation runbook

scripts/smoke/
├── phase_z_cleanup.sh            CREATE — githubpr Bearer + env template + gitignore checks
├── phase_ab_tooling.sh           CREATE — runner image contains expected binaries
├── phase_ac_approval.sh          CREATE — githubpr new methods + queue approve/reject
├── phase_ad_pretest.sh           CREATE — pre-commit test detector + runner
├── phase_ae_backup.sh            CREATE — era-backup.sh dry-run invocation
└── phase_af_ci.sh                CREATE — ci.yml YAML validity check

.gitignore                        VERIFY (phase Z) — /runner, /sidecar entries present

README.md                         MODIFY (final) — M5 status + roadmap row
Makefile                          (no changes in M5; kept as emergency manual deploy path)
```

---

# Phase Z — Cleanup batch

**Goal:** Three trivial tweaks that clear out small drift accumulated across M2-M4. Zero runtime behavior change.

## Task Z-1: `internal/githubpr` auth header `token` → `Bearer`

**Files:**
- Modify: `internal/githubpr/client.go` (one line in `newReq`)
- Modify: `internal/githubpr/client_test.go` (all `require.Equal` on Authorization header)

- [ ] **Step 1: Write the failing test assertion.**

Locate the test that asserts the auth header in `internal/githubpr/client_test.go`. Search for `"token ghs_test"`. Change each occurrence to `"Bearer ghs_test"`. Example:

```go
require.Equal(t, "Bearer ghs_test", r.Header.Get("Authorization"))
```

Do this for every `httptest.Server` handler in the file (TestDefaultBranch, TestCreate_PostsCorrectBody, TestClose_PatchesStateClosed — at minimum).

- [ ] **Step 2: Verify tests fail.**

```
go test -run Test ./internal/githubpr/
```

Expected: FAIL — current impl sends `token ghs_test`, tests now expect `Bearer`.

- [ ] **Step 3: Flip the auth scheme.**

In `internal/githubpr/client.go`, in the `newReq` method, change:

```go
req.Header.Set("Authorization", "token "+tok)
```

to:

```go
req.Header.Set("Authorization", "Bearer "+tok)
```

- [ ] **Step 4: Verify tests pass.**

```
go test -race ./internal/githubpr/
```

Expected: all PASS.

- [ ] **Step 5: Full regression.**

```
go test -race -count=1 ./...
```

All packages green.

- [ ] **Step 6: Commit.**

```bash
git add internal/githubpr/client.go internal/githubpr/client_test.go
git commit -m "refactor(githubpr): Bearer auth matches sibling packages"
```

## Task Z-2: Drop unused `PI_GITHUB_APP_PRIVATE_KEY_PATH` from env template

**Files:**
- Modify: `deploy/env.template`

- [ ] **Step 1: Remove the line.**

Open `deploy/env.template`, locate:

```
PI_GITHUB_APP_PRIVATE_KEY_PATH=/etc/era/github-app.pem
```

Delete that line. The adjacent `PI_GITHUB_APP_INSTALLATION_ID=` and `PI_GITHUB_SANDBOX_REPO=` stay. No other changes.

- [ ] **Step 2: Sanity check.**

```
grep PI_GITHUB_APP_PRIVATE_KEY_PATH deploy/env.template
```

Expected: no output (grep exits 1).

- [ ] **Step 3: Commit.**

```bash
git add deploy/env.template
git commit -m "deploy(env): drop unused PI_GITHUB_APP_PRIVATE_KEY_PATH; base64 inline is canonical"
```

## Task Z-3: Verify `.gitignore` has stray-binary entries

**Files:**
- Read: `.gitignore`

- [ ] **Step 1: Check.**

```
grep -E "^/(runner|sidecar)$" .gitignore
```

Expected output (order may vary):
```
/runner
/sidecar
```

If both present → Z-3 is a no-op, skip to Z-4.

If absent, add them after the `/bin/` + `/orchestrator` block:

```
# Stray top-level binaries
/runner
/sidecar
```

- [ ] **Step 2: Commit (only if modified).**

```bash
git add .gitignore
git commit -m "chore(gitignore): lock in /runner and /sidecar exclusions"
```

## Task Z-4: Phase Z smoke + gate

**Files:**
- Create: `scripts/smoke/phase_z_cleanup.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase Z smoke: Bearer auth is in place, env template drift is cleared,
# gitignore protects stray binaries.
set -euo pipefail

# Bearer assertion in githubpr tests still green
go test -race -count=1 -run 'TestDefaultBranch|TestCreate_|TestClose_' \
    ./internal/githubpr/... > /dev/null

# env template is clean
if grep -q PI_GITHUB_APP_PRIVATE_KEY_PATH deploy/env.template; then
    echo "FAIL: PI_GITHUB_APP_PRIVATE_KEY_PATH still present in env.template"
    exit 1
fi

# gitignore protects stray binaries
grep -qE '^/runner$'  .gitignore || { echo "FAIL: /runner not in .gitignore"; exit 1; }
grep -qE '^/sidecar$' .gitignore || { echo "FAIL: /sidecar not in .gitignore"; exit 1; }

echo "OK: phase Z — cleanup batch all checks green"
```

- [ ] **Step 2: Make executable + run.**

```
chmod +x scripts/smoke/phase_z_cleanup.sh
bash scripts/smoke/phase_z_cleanup.sh
```

Expected: `OK: phase Z — cleanup batch all checks green`.

- [ ] **Step 3: Full regression.**

```
go test -race -count=1 ./...
```

- [ ] **Step 4: Commit smoke.**

```bash
git add scripts/smoke/phase_z_cleanup.sh
git commit -m "docs(smoke): phase Z cleanup batch"
```

---

# Phase AA — Looser sudoers

**Goal:** Replace the strict-match sudoers rule with wildcarded patterns so `sudo systemctl status era --no-pager`, `sudo journalctl -u era -n 40`, etc. all work without password.

**One-time live VPS step needed** (task AA-3). Plan includes explicit ordered instructions for re-enabling root SSH briefly via Hetzner console, pushing the new sudoers file, and re-locking. Follow them exactly.

## Task AA-1: Create `deploy/sudoers-era`

**Files:**
- Create: `deploy/sudoers-era`

- [ ] **Step 1: Write the file.**

```
# era user sudo rules — used by M5 install.sh.
# Wildcards cover all subcommands on the era unit and trailing flags.
era ALL=(root) NOPASSWD: /usr/bin/systemctl * era, /usr/bin/systemctl * era *, /usr/bin/journalctl -u era, /usr/bin/journalctl -u era *
```

The four comma-separated entries correspond to:
- `systemctl <verb> era` — restart/start/stop/reload era (no extra flags)
- `systemctl <verb> era <flags>` — `status era --no-pager`, `show era --property=ActiveState`
- `journalctl -u era` — bare
- `journalctl -u era <flags>` — `-n 40 --no-pager -f`

- [ ] **Step 2: Validate syntax locally.**

```
visudo -c -f deploy/sudoers-era
```

Expected: `deploy/sudoers-era: parsed OK`.

If `visudo` isn't on the Mac, skip — install.sh runs `visudo -c` on the VPS.

- [ ] **Step 3: Commit.**

```bash
git add deploy/sudoers-era
git commit -m "deploy(sudoers): wildcarded sudoers rule for era user"
```

## Task AA-2: Update `deploy/install.sh` to install from file

**Files:**
- Modify: `deploy/install.sh`

- [ ] **Step 1: Replace inline heredoc.**

Locate the block:

```bash
log "sudoers entry for era (limited to restart/status era)"
cat > /etc/sudoers.d/era <<'EOF'
era ALL=(root) NOPASSWD: /usr/bin/systemctl restart era, /usr/bin/systemctl status era, /usr/bin/systemctl start era, /usr/bin/systemctl stop era, /usr/bin/journalctl -u era
EOF
chmod 440 /etc/sudoers.d/era
```

Replace with:

```bash
log "sudoers entry for era (from deploy/sudoers-era)"
install -m 440 /opt/era/deploy/sudoers-era /etc/sudoers.d/era
visudo -c -f /etc/sudoers.d/era >/dev/null || { echo "sudoers validation failed"; exit 1; }
```

- [ ] **Step 2: Shell syntax check.**

```
bash -n deploy/install.sh
```

Expected: no output.

- [ ] **Step 3: Commit.**

```bash
git add deploy/install.sh
git commit -m "deploy(install): source sudoers-era from file, validate with visudo"
```

## Task AA-3: Live VPS push (one-time, manual)

**Manual task — no code commit. Follow the three steps exactly; skipping the re-disable leaves the VPS with root-SSH open.**

- [ ] **Step 1: Re-enable root SSH via Hetzner web console.**

Open https://console.hetzner.com → your project → era server → Console tab.

In the console (login as root via web terminal — no SSH needed):

```
sed -i -E 's/^PermitRootLogin no/PermitRootLogin yes/' /etc/ssh/sshd_config
systemctl reload ssh
```

Verify from Mac: `ssh root@178.105.44.3 whoami` → returns `root`.

- [ ] **Step 2: Push + install sudoers-era.**

From Mac:

```
scp deploy/sudoers-era root@178.105.44.3:/tmp/sudoers-era
ssh root@178.105.44.3 'install -m 440 /tmp/sudoers-era /etc/sudoers.d/era && visudo -c -f /etc/sudoers.d/era && rm /tmp/sudoers-era'
```

Expected: `/etc/sudoers.d/era: parsed OK`.

- [ ] **Step 3: Re-disable root SSH.**

From Mac:

```
ssh root@178.105.44.3 'bash /opt/era/deploy/disable-root-ssh.sh'
```

Expected: `root ssh disabled. test: ssh root@<ip> should fail, ssh era@<ip> should succeed.`

Verify:
```
ssh -o BatchMode=yes -o ConnectTimeout=5 root@178.105.44.3 'whoami' 2>&1 | grep -q "Permission denied"   # must fail
ssh era@178.105.44.3 'whoami'   # must return "era"
```

If root SSH still works, the reload hasn't taken effect — re-run the disable script.

- [ ] **Step 4: Verify wildcarded sudoers works.**

```
ssh era@178.105.44.3 'sudo systemctl status era --no-pager'
ssh era@178.105.44.3 'sudo journalctl -u era -n 5 --no-pager'
```

Both should succeed without password prompts. If either fails with "a password is required", the sudoers file didn't install correctly — redo steps 1-3.

- [ ] **Step 5: No commit from this task.**

Nothing code-side changed. Log completion in the phase AA gate commit message (AA-4 below).

## Task AA-4: Phase AA gate + record live verification

**Files:**
- No code changes. This task only tags the phase complete in the git log.

- [ ] **Step 1: Empty commit to mark phase boundary + record the manual verification.**

```bash
git commit --allow-empty -m "phase(AA): sudoers widened on live VPS; verified no-password systemctl + journalctl"
```

No smoke script — sudoers widening is infrastructure, not code. Live verification from AA-3 step 4 is the gate.

---

# Phase AB — Runner tooling bake

**Goal:** Add language toolchains (Node already present; add Python, Rust, Go) + build deps + utilities into the runner image so Pi never needs `apk add` at task time.

## Task AB-1: Dockerfile apk additions

**Files:**
- Modify: `docker/runner/Dockerfile`

- [ ] **Step 1: Add the new RUN layer.**

After the existing `RUN apk add` line (line 5, base utilities), insert a new `RUN` block. The final file should look like:

```dockerfile
FROM node:22-alpine

# git for the runner; bash + wget for the entrypoint script; iptables for
# Phase K (Phase J doesn't use it yet but installing now avoids a rebuild).
RUN apk add --no-cache git bash ca-certificates coreutils curl iptables wget

# M5 tooling bake: common language toolchains + native build deps + utilities.
# Keeps egress locked down (no runtime apk fetches) and unblocks Python/Rust/Go
# tasks that would otherwise try to reach dl-cdn.alpinelinux.org (not in allowlist).
RUN apk add --no-cache \
    # language toolchains (node+npm come from the base image)
    python3 py3-pip py3-virtualenv rust cargo go \
    # native build dependencies
    build-base musl-dev pkgconf openssl-dev libffi-dev python3-dev sqlite-dev zlib-dev \
    # utilities
    sqlite tar gzip unzip tree ripgrep fd \
    && rm -rf /var/cache/apk/*

ARG PI_VERSION=latest
RUN npm install -g @mariozechner/pi-coding-agent@${PI_VERSION} \
    && pi --version > /pi-version.txt

# ... rest of file unchanged ...
```

Leave all layers below `ARG PI_VERSION` untouched. Keep the tooling-bake layer BEFORE the Pi install so Pi version bumps don't invalidate the tooling cache layer.

- [ ] **Step 2: Rebuild image locally.**

```
make docker-runner
```

Expected: build succeeds. First run pulls Alpine packages (~90s); watch for any "package not found" errors.

If `fd` fails with "package not found" on the Alpine version in use, try `fd-find` instead (older Alpine calls it that). Update the Dockerfile accordingly and document the fallback in the commit message.

- [ ] **Step 3: Commit.**

```bash
git add docker/runner/Dockerfile
git commit -m "feat(runner): bake Python/Rust/Go toolchains + build deps + utilities into runner image"
```

## Task AB-2: Image-contents smoke script

**Files:**
- Create: `scripts/smoke/phase_ab_tooling.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AB smoke: runner image contains the expected language toolchains +
# utilities. Tool verification is a simple "does the binary exist and print
# a version" check — deep correctness is Pi's job at task time.
set -euo pipefail

IMAGE="${IMAGE:-era-runner:m2}"

# Tools we expect to be present after phase AB bake.
TOOLS=(
    "node --version"
    "npm --version"
    "python3 --version"
    "pip3 --version"
    "cargo --version"
    "rustc --version"
    "go version"
    "rg --version"
    "fd --version"
    "tree --version"
    "sqlite3 --version"
)

for cmd in "${TOOLS[@]}"; do
    if ! docker run --rm "$IMAGE" sh -c "$cmd" > /dev/null 2>&1; then
        echo "FAIL: '$cmd' did not succeed inside $IMAGE"
        docker run --rm "$IMAGE" sh -c "$cmd" 2>&1 | head -3
        exit 1
    fi
done

echo "OK: phase AB — runner image tooling bake verified (${#TOOLS[@]} binaries)"
```

- [ ] **Step 2: Make executable + run.**

```
chmod +x scripts/smoke/phase_ab_tooling.sh
bash scripts/smoke/phase_ab_tooling.sh
```

Expected: `OK: phase AB — runner image tooling bake verified (11 binaries)`.

If any tool fails, investigate — the Dockerfile edit may have typos or Alpine package names may differ. Fix AB-1 and rerun.

- [ ] **Step 3: Commit smoke.**

```bash
git add scripts/smoke/phase_ab_tooling.sh
git commit -m "docs(smoke): phase AB runner tooling bake"
```

## Task AB-3: Live gate — retry failed URL-shortener task

**Files:**
- No code changes. Manual live test only.

- [ ] **Step 1: Push new runner image to live VPS.**

On Mac:
```
make deploy VPS_HOST=era@178.105.44.3
```

Wait for `systemctl restart era` success. Verify `ssh era@178.105.44.3 'sudo systemctl status era'` shows `active (running)` with recent PID.

- [ ] **Step 2: Send the previously-failed task via Telegram.**

```
/task vaibhav0806/trying-something build a complete URL shortener in Go. POST /shorten takes {"url":"..."} returns a 6-char code. GET /:code redirects. GET /stats/:code returns hit count + created_at. Store in SQLite (use modernc.org/sqlite, no cgo). Include a frontend at GET / as a single index.html with vanilla HTML/CSS/JS — form, recent-shortens table, simple modern styling, no frameworks. Include go.mod, README, and a Makefile with run and test targets.
```

- [ ] **Step 3: Verify successful completion.**

Expected: completion DM with a PR link (`https://github.com/vaibhav0806/trying-something/pull/N`). PR contains go.mod, main.go, index.html, Makefile, README.md. Clicking through, code should compile.

If task fails again with another blocked-host error, investigate the audit log:
```
ssh era@178.105.44.3 'sqlite3 /opt/era/pi-agent.db "SELECT payload FROM events WHERE task_id=<N> AND kind='"'"'http_request'"'"';"' | grep status | grep -v 200
```

Identify the blocked host, and decide: (a) add to allowlist if it's a legitimate language registry, or (b) bake the missing tool into the image.

- [ ] **Step 4: Commit gate.**

```bash
git commit --allow-empty -m "phase(AB): runner tooling bake live-verified via URL-shortener task"
```

---

# Phase AC — PR approval feedback

**Goal:** When user taps Approve in Telegram, add an `era-approved` label + submit an APPROVED review on the GitHub PR. When user taps Reject, post a comment BEFORE closing + deleting the branch.

## Task AC-1: `githubpr.Client.ApprovePR` method

**Files:**
- Modify: `internal/githubpr/client.go`
- Modify: `internal/githubpr/client_test.go`

- [ ] **Step 1: Write the failing test.**

In `client_test.go`, add:

```go
func TestApprovePR_PostsApproveEvent(t *testing.T) {
    var gotBody map[string]any
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "POST", r.Method)
        require.Equal(t, "/repos/owner/repo/pulls/42/reviews", r.URL.Path)
        require.Equal(t, "application/json", r.Header.Get("Content-Type"))
        require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
        w.WriteHeader(200)
        _, _ = w.Write([]byte(`{"id":12345,"state":"APPROVED"}`))
    }))
    defer srv.Close()
    c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

    err := c.ApprovePR(context.Background(), "owner/repo", 42, "Approved via era")
    require.NoError(t, err)
    require.Equal(t, "APPROVE", gotBody["event"])
    require.Equal(t, "Approved via era", gotBody["body"])
}

func TestApprovePR_403Error(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        http.Error(w, `{"message":"Resource not accessible by integration"}`, 403)
    }))
    defer srv.Close()
    c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

    err := c.ApprovePR(context.Background(), "o/r", 1, "x")
    require.Error(t, err)
    require.Contains(t, err.Error(), "403")
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestApprovePR_ ./internal/githubpr/
```

Expected: FAIL — method undefined.

- [ ] **Step 3: Implement.**

In `client.go`, add:

```go
// ApprovePR submits an APPROVED review on the given PR. Body is optional prose.
// Requires the GitHub App's "Pull requests: write" permission (covered by the
// existing grant — reviews are a sub-resource of pull requests).
func (c *Client) ApprovePR(ctx context.Context, repo string, number int, body string) error {
    payload, _ := json.Marshal(map[string]string{
        "event": "APPROVE",
        "body":  body,
    })
    req, err := c.newReq(ctx, "POST", fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number), bytes.NewReader(payload))
    if err != nil {
        return err
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return fmt.Errorf("approve pr: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        rb, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("approve pr %s#%d: %d %s", repo, number, resp.StatusCode, string(rb))
    }
    return nil
}
```

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestApprovePR_ ./internal/githubpr/
```

- [ ] **Step 5: Commit.**

```bash
git add internal/githubpr/client.go internal/githubpr/client_test.go
git commit -m "feat(githubpr): ApprovePR submits APPROVED review on PR"
```

## Task AC-2: `githubpr.Client.AddLabel` method

**Files:**
- Modify: `internal/githubpr/client.go`
- Modify: `internal/githubpr/client_test.go`

- [ ] **Step 1: Write failing test.**

```go
func TestAddLabel_PostsToIssuesEndpoint(t *testing.T) {
    var gotBody map[string]any
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "POST", r.Method)
        require.Equal(t, "/repos/owner/repo/issues/42/labels", r.URL.Path)
        require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
        w.WriteHeader(200)
        _, _ = w.Write([]byte(`[{"id":1,"name":"era-approved"}]`))
    }))
    defer srv.Close()
    c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

    err := c.AddLabel(context.Background(), "owner/repo", 42, "era-approved")
    require.NoError(t, err)
    labels, ok := gotBody["labels"].([]any)
    require.True(t, ok)
    require.Equal(t, "era-approved", labels[0])
}
```

- [ ] **Step 2: Verify fail, implement, verify pass, commit.**

```go
// AddLabel attaches a label to a PR. PRs are issues in GitHub's model for
// labels and comments, so this hits the /issues/ endpoint. GitHub auto-creates
// labels that don't pre-exist.
func (c *Client) AddLabel(ctx context.Context, repo string, number int, label string) error {
    payload, _ := json.Marshal(map[string][]string{"labels": {label}})
    req, err := c.newReq(ctx, "POST", fmt.Sprintf("/repos/%s/issues/%d/labels", repo, number), bytes.NewReader(payload))
    if err != nil {
        return err
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return fmt.Errorf("add label: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        rb, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("add label %s#%d %s: %d %s", repo, number, label, resp.StatusCode, string(rb))
    }
    return nil
}
```

```
go test -race ./internal/githubpr/
git add internal/githubpr/client.go internal/githubpr/client_test.go
git commit -m "feat(githubpr): AddLabel attaches label to PR via issues endpoint"
```

## Task AC-3: `githubpr.Client.AddComment` method

**Files:**
- Modify: `internal/githubpr/client.go`
- Modify: `internal/githubpr/client_test.go`

- [ ] **Step 1-5: Test + implement + verify + commit.**

Test:
```go
func TestAddComment_PostsToIssueComments(t *testing.T) {
    var gotBody map[string]any
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "POST", r.Method)
        require.Equal(t, "/repos/owner/repo/issues/42/comments", r.URL.Path)
        require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
        w.WriteHeader(201)
        _, _ = w.Write([]byte(`{"id":1,"body":"rejected"}`))
    }))
    defer srv.Close()
    c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

    err := c.AddComment(context.Background(), "owner/repo", 42, "rejected")
    require.NoError(t, err)
    require.Equal(t, "rejected", gotBody["body"])
}
```

Implementation:
```go
// AddComment posts a plain issue comment on a PR.
func (c *Client) AddComment(ctx context.Context, repo string, number int, body string) error {
    payload, _ := json.Marshal(map[string]string{"body": body})
    req, err := c.newReq(ctx, "POST", fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number), bytes.NewReader(payload))
    if err != nil {
        return err
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return fmt.Errorf("add comment: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 201 {
        rb, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("add comment %s#%d: %d %s", repo, number, resp.StatusCode, string(rb))
    }
    return nil
}
```

```
go test -race ./internal/githubpr/
git add internal/githubpr/client.go internal/githubpr/client_test.go
git commit -m "feat(githubpr): AddComment posts issue comment on PR"
```

## Task AC-4: Expand `PRCreator` interface + fake

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_pr_test.go`

- [ ] **Step 1: Expand interface.**

In `internal/queue/queue.go`, locate `type PRCreator interface` (~line 70). Extend:

```go
type PRCreator interface {
    Create(ctx context.Context, args githubpr.CreateArgs) (*githubpr.PR, error)
    Close(ctx context.Context, repo string, number int) error
    DefaultBranch(ctx context.Context, repo string) (string, error)
    ApprovePR(ctx context.Context, repo string, number int, body string) error
    AddLabel(ctx context.Context, repo string, number int, label string) error
    AddComment(ctx context.Context, repo string, number int, body string) error
}
```

- [ ] **Step 2: Extend fake.**

In `internal/queue/queue_pr_test.go`, `fakePRCreator` struct:

```go
type fakePRCreator struct {
    mu              sync.Mutex
    created         []githubpr.CreateArgs
    createReturns   *githubpr.PR
    createErr       error
    closed          []closedRecord
    closeErr        error
    defaultBranch   string
    defaultBranchEr error
    // AC-4 additions
    approved    []approvedRecord
    approveErr  error
    labeled     []labeledRecord
    labelErr    error
    commented   []commentedRecord
    commentErr  error
}

type approvedRecord  struct{ Repo, Body string; Number int }
type labeledRecord   struct{ Repo, Label string; Number int }
type commentedRecord struct{ Repo, Body string; Number int }

func (f *fakePRCreator) ApprovePR(ctx context.Context, repo string, n int, body string) error {
    f.mu.Lock(); defer f.mu.Unlock()
    f.approved = append(f.approved, approvedRecord{repo, body, n})
    return f.approveErr
}
func (f *fakePRCreator) AddLabel(ctx context.Context, repo string, n int, label string) error {
    f.mu.Lock(); defer f.mu.Unlock()
    f.labeled = append(f.labeled, labeledRecord{repo, label, n})
    return f.labelErr
}
func (f *fakePRCreator) AddComment(ctx context.Context, repo string, n int, body string) error {
    f.mu.Lock(); defer f.mu.Unlock()
    f.commented = append(f.commented, commentedRecord{repo, body, n})
    return f.commentErr
}
```

- [ ] **Step 3: Verify build.**

```
go build ./...
```

Any fake implementing `PRCreator` elsewhere (check with `grep -rn "PRCreator" internal/`) also needs the three new methods. If none beyond `fakePRCreator`, proceed.

- [ ] **Step 4: Commit.**

```bash
git add internal/queue/queue.go internal/queue/queue_pr_test.go
git commit -m "feat(queue): extend PRCreator interface with ApprovePR + AddLabel + AddComment"
```

## Task AC-5: `rejectionCommentBody` helper

**Files:**
- Create: `internal/queue/reject_body.go`
- Create: `internal/queue/reject_body_test.go`

- [ ] **Step 1: Write failing tests.**

```go
package queue_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/vaibhav0806/era/internal/diffscan"
    "github.com/vaibhav0806/era/internal/queue"
)

func TestRejectionCommentBody_WithFindings(t *testing.T) {
    body := queue.RejectionCommentBody([]diffscan.Finding{
        {Rule: "skip_directive", Path: "test_sample.js", Message: "describe.skip()"},
        {Rule: "removed_test", Path: "foo_test.go", Message: "TestFoo removed"},
    })
    require.Contains(t, body, "Rejected via era")
    require.Contains(t, body, "Branch deleted")
    require.Contains(t, body, "skip_directive")
    require.Contains(t, body, "test_sample.js")
    require.Contains(t, body, "removed_test")
}

func TestRejectionCommentBody_NoFindings(t *testing.T) {
    body := queue.RejectionCommentBody(nil)
    require.Contains(t, body, "Rejected via era")
    require.Contains(t, body, "Branch deleted")
    // Empty findings should not produce "Findings:" header
    require.NotContains(t, body, "Findings:")
}
```

- [ ] **Step 2: Implement.**

```go
package queue

import (
    "fmt"
    "strings"

    "github.com/vaibhav0806/era/internal/diffscan"
)

// RejectionCommentBody composes the GitHub PR comment posted before closing
// a rejected task. Findings, when present, are listed for traceability.
func RejectionCommentBody(findings []diffscan.Finding) string {
    var b strings.Builder
    b.WriteString("✗ Rejected via era Telegram bot. Branch deleted.\n")
    if len(findings) > 0 {
        b.WriteString("\nFindings:\n")
        for _, f := range findings {
            fmt.Fprintf(&b, "  • %s (%s): %s\n", f.Rule, f.Path, f.Message)
        }
    }
    return b.String()
}
```

- [ ] **Step 3: Verify + commit.**

```
go test -race ./internal/queue/ -run TestRejectionCommentBody
git add internal/queue/reject_body.go internal/queue/reject_body_test.go
git commit -m "feat(queue): RejectionCommentBody composes PR comment from diffscan findings"
```

## Task AC-6: Wire `ApproveTask` to call Label + Review

**Files:**
- Modify: `internal/queue/queue.go`
- Create: `internal/queue/queue_approve_test.go`

- [ ] **Step 1: Write failing tests.**

In `queue_approve_test.go`:

```go
package queue_test

import (
    "context"
    "errors"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestApproveTask_LabelsAndReviewsPR(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    pc := &fakePRCreator{}
    q.SetPRCreator(pc)

    task, _ := repo.CreateTask(ctx, "x", "owner/repo")
    _ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
    _ = repo.SetPRNumber(ctx, task.ID, 7)
    _ = repo.SetStatus(ctx, task.ID, "needs_review")

    require.NoError(t, q.ApproveTask(ctx, task.ID))

    // Both API calls happened with correct args.
    require.Len(t, pc.labeled, 1)
    require.Equal(t, "owner/repo", pc.labeled[0].Repo)
    require.Equal(t, 7, pc.labeled[0].Number)
    require.Equal(t, "era-approved", pc.labeled[0].Label)

    require.Len(t, pc.approved, 1)
    require.Equal(t, "owner/repo", pc.approved[0].Repo)
    require.Equal(t, 7, pc.approved[0].Number)
    require.Contains(t, pc.approved[0].Body, "Approved via era")

    // Task status flips.
    got, _ := repo.GetTask(ctx, task.ID)
    require.Equal(t, "approved", got.Status)
}

func TestApproveTask_NullPRNumber_SkipsGH(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    pc := &fakePRCreator{}
    q.SetPRCreator(pc)

    task, _ := repo.CreateTask(ctx, "x", "owner/repo")
    _ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
    _ = repo.SetStatus(ctx, task.ID, "needs_review")
    // No SetPRNumber — pr_number stays NULL.

    require.NoError(t, q.ApproveTask(ctx, task.ID))

    require.Len(t, pc.labeled, 0, "must not call GH when pr_number null")
    require.Len(t, pc.approved, 0)

    got, _ := repo.GetTask(ctx, task.ID)
    require.Equal(t, "approved", got.Status)
}

func TestApproveTask_LabelErrorLoggedButNotBlocking(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    pc := &fakePRCreator{labelErr: errors.New("network blip")}
    q.SetPRCreator(pc)

    task, _ := repo.CreateTask(ctx, "x", "owner/repo")
    _ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
    _ = repo.SetPRNumber(ctx, task.ID, 7)
    _ = repo.SetStatus(ctx, task.ID, "needs_review")

    // Status transition must succeed despite label failure.
    require.NoError(t, q.ApproveTask(ctx, task.ID))

    events, _ := repo.ListEvents(ctx, task.ID)
    foundErr := false
    for _, e := range events {
        if e.Kind == "pr_label_error" {
            foundErr = true
        }
    }
    require.True(t, foundErr, "pr_label_error event must be logged")

    got, _ := repo.GetTask(ctx, task.ID)
    require.Equal(t, "approved", got.Status)
}

func TestApproveTask_IdempotentOnAlreadyApproved(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    pc := &fakePRCreator{}
    q.SetPRCreator(pc)

    task, _ := repo.CreateTask(ctx, "x", "owner/repo")
    _ = repo.SetStatus(ctx, task.ID, "approved")

    require.NoError(t, q.ApproveTask(ctx, task.ID))
    require.Len(t, pc.labeled, 0, "already-approved must not re-label")
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestApproveTask_ ./internal/queue/
```

Expected: FAIL — current `ApproveTask` doesn't call Label or Review.

- [ ] **Step 3: Implement.**

In `internal/queue/queue.go`, locate `ApproveTask`. Current implementation likely just flips status. Rewrite:

```go
func (q *Queue) ApproveTask(ctx context.Context, id int64) error {
    task, err := q.repo.GetTask(ctx, id)
    if err != nil {
        return fmt.Errorf("get task: %w", err)
    }
    switch task.Status {
    case "approved":
        return nil // idempotent
    case "needs_review":
        // fall through
    default:
        return fmt.Errorf("cannot approve task in state %q", task.Status)
    }

    effectiveRepo := task.TargetRepo
    if effectiveRepo == "" {
        effectiveRepo = q.repoFQN
    }

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
        return fmt.Errorf("set status: %w", err)
    }
    _ = q.repo.AppendEvent(ctx, id, "approved", "{}")
    return nil
}
```

Check the existing `ApproveTask` first — the structure may differ (variable name `t` vs `task`, event appending conventions). Preserve the existing style.

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestApproveTask_ ./internal/queue/
go test -race -count=1 ./...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/queue/queue.go internal/queue/queue_approve_test.go
git commit -m "feat(queue): ApproveTask labels PR + submits APPROVED review via githubpr"
```

## Task AC-7: Wire `RejectTask` to post Comment before Close

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_reject_test.go`

- [ ] **Step 1: Write failing test.**

Add to `queue_reject_test.go`:

```go
func TestRejectTask_PostsCommentBeforeClose(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    pc := &fakePRCreator{}
    bd := &fakeBranchDeleter{}
    q.SetPRCreator(pc)
    q.SetBranchDeleter(bd)

    task, _ := repo.CreateTask(ctx, "x", "owner/repo")
    _ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
    _ = repo.SetPRNumber(ctx, task.ID, 5)
    _ = repo.SetStatus(ctx, task.ID, "needs_review")

    require.NoError(t, q.RejectTask(ctx, task.ID))

    // Comment posted BEFORE close, both on same PR.
    require.Len(t, pc.commented, 1)
    require.Equal(t, "owner/repo", pc.commented[0].Repo)
    require.Equal(t, 5, pc.commented[0].Number)
    require.Contains(t, pc.commented[0].Body, "Rejected via era")

    require.Len(t, pc.closed, 1)
    require.Equal(t, "owner/repo", pc.closed[0].Repo)
    require.Equal(t, 5, pc.closed[0].Number)
}

func TestRejectTask_CommentErrorDoesNotBlockClose(t *testing.T) {
    ctx := context.Background()
    q, repo := newRunQueue(t, &fakeRunner{})
    pc := &fakePRCreator{commentErr: errors.New("network blip")}
    bd := &fakeBranchDeleter{}
    q.SetPRCreator(pc)
    q.SetBranchDeleter(bd)

    task, _ := repo.CreateTask(ctx, "x", "owner/repo")
    _ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
    _ = repo.SetPRNumber(ctx, task.ID, 5)
    _ = repo.SetStatus(ctx, task.ID, "needs_review")

    require.NoError(t, q.RejectTask(ctx, task.ID))
    require.Len(t, pc.closed, 1, "close must still run even if comment failed")

    events, _ := repo.ListEvents(ctx, task.ID)
    foundErr := false
    for _, e := range events {
        if e.Kind == "pr_comment_error" {
            foundErr = true
        }
    }
    require.True(t, foundErr)
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestRejectTask_PostsCommentBeforeClose ./internal/queue/
```

Expected: FAIL — Comment isn't called yet.

- [ ] **Step 3: Modify `RejectTask` to insert Comment before Close.**

Current path (from M4): GetTask → state check → effectiveRepo → Close PR (if pr_number valid) → Delete branch → SetStatus. M5 adds AddComment before Close.

Locate `RejectTask` in `queue.go`. Replace the Close block with:

```go
// M5: Comment first, so the PR history shows WHY it closed.
if task.PrNumber.Valid && q.prCreator != nil {
    n := int(task.PrNumber.Int64)
    findings := loadFindings(ctx, q.repo, id) // helper below
    commentBody := RejectionCommentBody(findings)
    if err := q.prCreator.AddComment(ctx, effectiveRepo, n, commentBody); err != nil {
        _ = q.repo.AppendEvent(ctx, id, "pr_comment_error", quoteJSON(err.Error()))
    } else {
        _ = q.repo.AppendEvent(ctx, id, "pr_commented_rejected", "{}")
    }
    // existing Close call:
    if err := q.prCreator.Close(ctx, effectiveRepo, n); err != nil {
        _ = q.repo.AppendEvent(ctx, id, "pr_close_error", quoteJSON(err.Error()))
    } else {
        _ = q.repo.AppendEvent(ctx, id, "pr_closed", "{}")
    }
}
// ... existing branch delete + SetStatus unchanged ...
```

Add the `loadFindings` helper in the same file (or `reject_body.go` — the test file is `queue_test`, so pick one and be consistent):

```go
// loadFindings fetches the diffscan_flagged event payload for a task and
// returns the parsed findings. Nil on any error or if no findings event exists.
func loadFindings(ctx context.Context, r *db.Repo, id int64) []diffscan.Finding {
    events, err := r.ListEvents(ctx, id)
    if err != nil {
        return nil
    }
    for _, e := range events {
        if e.Kind == "diffscan_flagged" {
            var findings []diffscan.Finding
            if err := json.Unmarshal([]byte(e.Payload), &findings); err == nil {
                return findings
            }
        }
    }
    return nil
}
```

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestRejectTask_ ./internal/queue/
go test -race -count=1 ./...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/queue/queue.go internal/queue/queue_reject_test.go
git commit -m "feat(queue): RejectTask posts comment with findings BEFORE Close"
```

## Task AC-8: Phase AC smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_ac_approval.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AC smoke: githubpr new methods + queue approve/reject wiring.
set -euo pipefail
go test -race -count=1 -run 'TestApprovePR_|TestAddLabel_|TestAddComment_|TestApproveTask_|TestRejectTask_|TestRejectionCommentBody_' \
    ./internal/githubpr/... ./internal/queue/... > /dev/null
echo "OK: phase AC — PR approval feedback unit tests green"
```

- [ ] **Step 2: Run.**

```
chmod +x scripts/smoke/phase_ac_approval.sh
bash scripts/smoke/phase_ac_approval.sh
```

- [ ] **Step 3: Live gate.**

Deploy to VPS:
```
make deploy VPS_HOST=era@178.105.44.3
```

Trigger a diff-scan flag by sending a task that adds `.skip`:
```
/task vaibhav0806/trying-something create a file phase_ac_test.js with one jest test using describe.skip — intentional scaffold for AC smoke
```

Wait for needs-review DM. Tap **Approve**.

Expected visible change on GitHub PR:
- `era-approved` label appears on the PR.
- A review with "Approved via era Telegram bot." body shows in the Reviews section with a green APPROVED indicator.

Send another flagging task. Tap **Reject**.

Expected:
- An issue comment appears on the PR listing findings.
- PR state flips to "Closed".
- Branch is gone (GitHub's `/branches` page or `git ls-remote`).

- [ ] **Step 4: Commit smoke + mark phase.**

```bash
git add scripts/smoke/phase_ac_approval.sh
git commit -m "docs(smoke): phase AC PR approval feedback"
```

---

# Phase AD — Pre-commit test

**Goal:** If the task's repo has a `Makefile` with a `test` target, runner runs `make test` inside the container before `git commit`. Test failure aborts commit + push, marks task `failed`, DMs the test output.

## Task AD-1: `HasMakefileTest` detector + tests

**Files:**
- Create: `cmd/runner/pretest.go`
- Create: `cmd/runner/pretest_test.go`
- Create: `cmd/runner/testdata/makefile_with_test/Makefile`
- Create: `cmd/runner/testdata/makefile_no_test/Makefile`
- Create: `cmd/runner/testdata/no_makefile/.gitkeep` (empty directory marker)

- [ ] **Step 1: Write fixtures.**

`cmd/runner/testdata/makefile_with_test/Makefile`:
```make
.PHONY: build test

build:
	@echo build

test:
	@echo ok
```

`cmd/runner/testdata/makefile_no_test/Makefile`:
```make
.PHONY: build

build:
	@echo build
```

`cmd/runner/testdata/no_makefile/.gitkeep` — empty file, just creates the directory.

- [ ] **Step 2: Write failing tests.**

`cmd/runner/pretest_test.go`:

```go
package main

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestHasMakefileTest_PresentTarget(t *testing.T) {
    require.True(t, HasMakefileTest("testdata/makefile_with_test"))
}

func TestHasMakefileTest_NoTestTarget(t *testing.T) {
    require.False(t, HasMakefileTest("testdata/makefile_no_test"))
}

func TestHasMakefileTest_NoMakefile(t *testing.T) {
    require.False(t, HasMakefileTest("testdata/no_makefile"))
}

func TestHasMakefileTest_TestAllNotMatched(t *testing.T) {
    // Regex `^test\s*:` must NOT match `test-all:` or `test_unit:`.
    // Write an ephemeral workspace for this check.
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile", []byte("test-all:\n\t@echo x\n"), 0644))
    require.False(t, HasMakefileTest(tmp))
}

func TestHasMakefileTest_CommentedTargetNotMatched(t *testing.T) {
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile", []byte("# test:\n\t@echo x\n"), 0644))
    require.False(t, HasMakefileTest(tmp))
}
```

Import `"os"` at the top.

- [ ] **Step 3: Verify fail.**

```
go test -run TestHasMakefileTest_ ./cmd/runner/
```

Expected: FAIL — `HasMakefileTest` undefined.

- [ ] **Step 4: Implement.**

In `cmd/runner/pretest.go`:

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
    "time"
)

// makefileTestTarget matches a `test:` target at the start of a line,
// optional whitespace allowed between `test` and `:`. Excludes `test-*:`,
// `test_*:`, commented lines, and indented recipe lines.
var makefileTestTarget = regexp.MustCompile(`(?m)^test\s*:`)

// HasMakefileTest returns true iff workspace/Makefile exists and contains a
// `test` target declaration at column 0.
func HasMakefileTest(workspace string) bool {
    f, err := os.Open(filepath.Join(workspace, "Makefile"))
    if err != nil {
        return false
    }
    defer f.Close()
    sc := bufio.NewScanner(f)
    for sc.Scan() {
        if makefileTestTarget.MatchString(sc.Text()) {
            return true
        }
    }
    return false
}
```

- [ ] **Step 5: Verify pass.**

```
go test -race -run TestHasMakefileTest_ ./cmd/runner/
```

- [ ] **Step 6: Commit.**

```bash
git add cmd/runner/pretest.go cmd/runner/pretest_test.go cmd/runner/testdata/
git commit -m "feat(runner): HasMakefileTest detects top-level Makefile test target"
```

## Task AD-2: `RunMakefileTest` runner + tests

**Files:**
- Modify: `cmd/runner/pretest.go`
- Modify: `cmd/runner/pretest_test.go`

- [ ] **Step 1: Write failing tests.**

Add to `pretest_test.go`:

```go
func TestRunMakefileTest_Pass(t *testing.T) {
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile",
        []byte("test:\n\t@echo ok\n"), 0644))
    out, err := RunMakefileTest(context.Background(), tmp)
    require.NoError(t, err)
    require.Contains(t, out, "ok")
}

func TestRunMakefileTest_Fail(t *testing.T) {
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile",
        []byte("test:\n\t@echo fail && exit 1\n"), 0644))
    out, err := RunMakefileTest(context.Background(), tmp)
    require.Error(t, err)
    require.Contains(t, out, "fail")
}

func TestRunMakefileTest_Timeout(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping timeout test in short mode")
    }
    // 11-minute sleep exceeds the 10-min cap. Keep this test opt-in by
    // gating on a short env var — a 10-min CI run is painful.
    if os.Getenv("ERA_TEST_LONG") != "1" {
        t.Skip("set ERA_TEST_LONG=1 to run the 11-minute timeout test")
    }
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile",
        []byte("test:\n\tsleep 660\n"), 0644))
    _, err := RunMakefileTest(context.Background(), tmp)
    require.Error(t, err)
    require.Contains(t, err.Error(), "10-minute")
}
```

- [ ] **Step 2: Verify fail.**

```
go test -run TestRunMakefileTest_ ./cmd/runner/
```

Expected: FAIL — undefined.

- [ ] **Step 3: Implement.**

Add to `pretest.go`:

```go
// RunMakefileTest runs `make test` in workspace with a 10-minute hard cap.
// Returns combined stdout+stderr plus any error. Non-zero exit from make is
// surfaced as error; the output is always returned regardless.
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

- [ ] **Step 4: Verify pass.**

```
go test -race -run TestRunMakefileTest_Pass ./cmd/runner/
go test -race -run TestRunMakefileTest_Fail ./cmd/runner/
```

- [ ] **Step 5: Commit.**

```bash
git add cmd/runner/pretest.go cmd/runner/pretest_test.go
git commit -m "feat(runner): RunMakefileTest runs make test with 10-minute cap"
```

## Task AD-3: Hook pre-commit test into `git.go`

**Files:**
- Modify: `cmd/runner/git.go`
- Modify: `cmd/runner/git_test.go`

- [ ] **Step 1: Locate the hook point.**

Find the `CommitAndPush` call site in `cmd/runner/main.go` (around line 79, post-Pi, pre-result-emit). The hook goes BEFORE `CommitAndPush`.

Actually, looking at the spec's code snippet more carefully: the hook writes the `runResult` and returns an error directly, bypassing the rest of the runner path. This is cleaner than hooking inside `git.go`. Put the hook in `cmd/runner/main.go` where Pi exits and result is emitted.

**Concrete location:** in `cmd/runner/main.go`, after `summary, piErr := runPi(...)` (around line 63) and before the `commitErr := g.CommitAndPush(...)` switch (around line 79).

- [ ] **Step 2: Write failing test.**

In `cmd/runner/git_test.go`, if it exists, extend with integration-level test. Otherwise in `main_test.go`. Need to check what's already there — the existing TestFinalSummary tests might be the right neighborhood.

Write a test that simulates the runner path with a workspace containing a failing Makefile test. Since the runner's main orchestration is hard to unit-test (it calls exec + git + push), use a narrower assertion: a new helper `handlePreCommitTest(workspace, summary, piErr, tokens, costUSD)` that encapsulates the test-gate logic. Test that helper.

`cmd/runner/pretest_test.go` additions:

```go
func TestPreCommitGate_NoMakefileTest_Skipped(t *testing.T) {
    tmp := t.TempDir()
    // No Makefile
    skipped, _ := MaybeRunPreCommitTest(context.Background(), tmp)
    require.True(t, skipped)
}

func TestPreCommitGate_PassingTest_Runs(t *testing.T) {
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile",
        []byte("test:\n\t@echo ok\n"), 0644))
    skipped, runErr := MaybeRunPreCommitTest(context.Background(), tmp)
    require.False(t, skipped)
    require.NoError(t, runErr)
}

func TestPreCommitGate_FailingTest_ReturnsError(t *testing.T) {
    tmp := t.TempDir()
    require.NoError(t, os.WriteFile(tmp+"/Makefile",
        []byte("test:\n\t@echo boom && exit 1\n"), 0644))
    skipped, runErr := MaybeRunPreCommitTest(context.Background(), tmp)
    require.False(t, skipped)
    require.Error(t, runErr)
    require.Contains(t, runErr.Error(), "tests_failed")
}
```

- [ ] **Step 3: Verify fail, implement.**

Add to `pretest.go`:

```go
// MaybeRunPreCommitTest runs `make test` if the workspace has a `test` target.
// Returns (skipped, error). skipped=true means no Makefile test target existed.
// error wraps the test output in a "tests_failed" message suitable for use as
// the runner's summary field.
func MaybeRunPreCommitTest(ctx context.Context, workspace string) (bool, error) {
    if !HasMakefileTest(workspace) {
        return true, nil
    }
    out, err := RunMakefileTest(ctx, workspace)
    if err != nil {
        return false, fmt.Errorf("tests_failed: %s", truncate(out, 2000))
    }
    return false, nil
}
```

`truncate` already exists in `cmd/runner/main.go` (verify name — it may be unexported; if so, reference it by package-internal name).

If `truncate` doesn't exist, add a minimal one:

```go
func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n]
}
```

- [ ] **Step 4: Wire into main.go.**

In `cmd/runner/main.go`, find the block right after `runPi` returns and before `CommitAndPush`:

```go
summary, piErr := runPi(ctx, p, c)
tokens, costUSD, iters := c.Totals()
_ = iters
slog.Info("pi done", ...)

if errors.Is(piErr, errCapExceeded) {
    return piErr
}

// NEW: M5 pre-commit test gate.
_, testErr := MaybeRunPreCommitTest(ctx, workspace)
if testErr != nil {
    writeResult(os.Stdout, runResult{
        Branch:    "",
        Summary:   testErr.Error(),
        Tokens:    tokens,
        CostCents: int(math.Round(costUSD * 100)),
    })
    return testErr
}

commitErr := g.CommitAndPush(ctx, workspace)
// ... existing switch unchanged ...
```

`workspace` variable must be in scope. Check `cmd/runner/main.go` — if it's not called `workspace`, use the actual variable name (`wsDir`, `repo`, etc.).

- [ ] **Step 5: Verify.**

```
go test -race -count=1 ./cmd/runner/
go test -race -count=1 ./...
```

All green. If any existing main_test.go or result_test.go relies on the post-Pi flow without a Makefile in a temp workspace, the `MaybeRunPreCommitTest` call will still skip (no Makefile → skipped=true) and behavior is unchanged.

- [ ] **Step 6: Commit.**

```bash
git add cmd/runner/pretest.go cmd/runner/pretest_test.go cmd/runner/main.go
git commit -m "feat(runner): pre-commit test gate aborts on Makefile test failure"
```

## Task AD-4: Phase AD smoke + live gate

**Files:**
- Create: `scripts/smoke/phase_ad_pretest.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AD smoke: pre-commit test detector + runner unit tests. 10-minute
# timeout test is opt-in via ERA_TEST_LONG=1.
set -euo pipefail
go test -race -count=1 -run 'TestHasMakefileTest_|TestRunMakefileTest_Pass|TestRunMakefileTest_Fail|TestPreCommitGate_' \
    ./cmd/runner/... > /dev/null
echo "OK: phase AD — pre-commit test gate unit tests green"
```

- [ ] **Step 2: Run + regression.**

```
chmod +x scripts/smoke/phase_ad_pretest.sh
bash scripts/smoke/phase_ad_pretest.sh
go test -race -count=1 ./...
```

- [ ] **Step 3: Live gate.**

Rebuild + deploy:
```
make deploy VPS_HOST=era@178.105.44.3
```

Test with a task that will fail tests intentionally:
```
/task vaibhav0806/trying-something create a simple Go package with a Makefile. main.go: package main; func main(){println("hi")}. Makefile: two targets: build runs `go build -o app`; test runs `test -f app && go test ./...`. DO NOT run the build target first — leave the binary absent so tests fail.
```

Expected: task status = `failed`. DM includes `tests_failed: ...` with the make/test output. No branch pushed to GitHub.

Test with a passing test:
```
/task vaibhav0806/trying-something create a simple Go package with a Makefile. main.go: package main; func main(){println("hi")}. main_test.go: one test that passes. Makefile: test target runs `go test ./...`. Include go.mod.
```

Expected: task completes normally with a PR. Pre-commit test passed.

Test with no Makefile:
```
/task vaibhav0806/trying-something add a file NOTES.md with the string "hello"
```

Expected: task completes normally (no test gate triggered since there's no Makefile with test target).

- [ ] **Step 4: Commit smoke.**

```bash
git add scripts/smoke/phase_ad_pretest.sh
git commit -m "docs(smoke): phase AD pre-commit test gate"
```

---

# Phase AE — Offsite backup to B2

**Goal:** Nightly local SQLite dump also pushes to a private Backblaze B2 bucket with 30-day retention. Resilient to VPS loss.

## Task AE-1: Extend `deploy/era-backup.sh` with rclone push

**Files:**
- Modify: `deploy/era-backup.sh`

- [ ] **Step 1: Append offsite stage.**

At the end of the file (after the `find ... -mtime +7 -delete` line), append:

```bash

# --- M5: offsite push to B2 ---
if [ -f /etc/era/rclone.conf ] && command -v rclone >/dev/null 2>&1; then
    rclone --config=/etc/era/rclone.conf copy \
        "$OUTDIR/pi-agent-$STAMP.db.gz" b2:era-backups/ \
        --log-level INFO 2>&1 | tee -a /var/log/era-backup.log
else
    echo "$(date -Is) rclone/config missing; skipping offsite push" >> /var/log/era-backup.log
fi
```

The `if` ensures the script still succeeds (local dump always works) even when rclone or the config isn't installed yet.

- [ ] **Step 2: Shell syntax check.**

```
bash -n deploy/era-backup.sh
```

Expected: no output.

- [ ] **Step 3: Commit.**

```bash
git add deploy/era-backup.sh
git commit -m "deploy(backup): offsite push to B2 via rclone after local dump"
```

## Task AE-2: Create `deploy/rclone.conf.template`

**Files:**
- Create: `deploy/rclone.conf.template`

- [ ] **Step 1: Write template.**

```ini
# era B2 rclone config template. Copy to /etc/era/rclone.conf on the VPS,
# then replace YOUR_B2_ACCOUNT_ID + YOUR_B2_APPLICATION_KEY with real creds
# from https://secure.backblaze.com/app_keys.htm.

[b2]
type = b2
account = YOUR_B2_ACCOUNT_ID
key = YOUR_B2_APPLICATION_KEY
```

- [ ] **Step 2: Commit.**

```bash
git add deploy/rclone.conf.template
git commit -m "deploy(backup): rclone.conf template with B2 placeholders"
```

## Task AE-3: Update `deploy/install.sh` — install rclone + template

**Files:**
- Modify: `deploy/install.sh`

- [ ] **Step 1: Add rclone to apt list.**

Find the `apt-get install -y` line with docker.io + docker-buildx + etc. Add `rclone`:

```bash
DEBIAN_FRONTEND=noninteractive apt-get install -y \
    docker.io docker-buildx make git rsync sqlite3 ufw curl rclone \
    unattended-upgrades apt-listchanges
```

- [ ] **Step 2: Install template.**

After the `install -d ... /etc/era` block (around line 55), add:

```bash
log "install rclone config template"
install -m 600 -o era -g era /opt/era/deploy/rclone.conf.template /etc/era/rclone.conf.template
```

- [ ] **Step 3: Update post-install message.**

Find the `cat <<EOF` at the end of install.sh printing post-install steps. Add a step:

```bash
  4a. cp /etc/era/rclone.conf.template /etc/era/rclone.conf  # then fill in real B2 creds
```

- [ ] **Step 4: Shell check.**

```
bash -n deploy/install.sh
```

- [ ] **Step 5: Commit.**

```bash
git add deploy/install.sh
git commit -m "deploy(install): add rclone + install rclone.conf template"
```

## Task AE-4: Manual B2 setup + live verification

**Manual task — no code commit. Documented steps; execute once.**

- [ ] **Step 1: B2 account + bucket.**

1. Sign up at https://www.backblaze.com/b2/ (free tier, card required even for $0 bill).
2. Navigate to B2 Cloud Storage → Buckets → Create a Bucket.
3. Name: `era-backups`. Type: Private. Object lock: optional (recommended: 14 days). Server-side encryption: enabled.

- [ ] **Step 2: Create app key scoped to bucket.**

1. App Keys → Add a New Application Key.
2. Name: `era-vps-write`.
3. Scope: `era-backups` bucket only.
4. Capabilities: Read + Write + List (rclone uses List for dedup).
5. Save `keyID` and `applicationKey` — they're shown once.

- [ ] **Step 3: Bucket lifecycle rule — 30-day retention.**

1. Buckets → era-backups → Lifecycle Settings → Keep only the last X days.
2. Set to 30 days.

- [ ] **Step 4: Install rclone config on VPS.**

```
ssh era@178.105.44.3 'cp /etc/era/rclone.conf.template /etc/era/rclone.conf && chmod 600 /etc/era/rclone.conf'
```

Then edit the config (use the sudoers we just widened — `ssh era@178.105.44.3` → `vi /etc/era/rclone.conf`, paste account + key values).

Alternative: scp a pre-filled rclone.conf from Mac:
```
# create ~/tmp/rclone.conf locally with real creds
scp ~/tmp/rclone.conf era@178.105.44.3:/etc/era/rclone.conf
ssh era@178.105.44.3 'chmod 600 /etc/era/rclone.conf'
# delete local copy immediately
rm ~/tmp/rclone.conf
```

- [ ] **Step 5: Test upload manually.**

```
ssh era@178.105.44.3 'sudo -u era rclone --config=/etc/era/rclone.conf copy /var/backups/era/pi-agent-*.db.gz b2:era-backups/ --log-level INFO'
```

Expected: rclone logs "Copied" for at least one file. Check B2 web console → era-backups → should list today's backup.

- [ ] **Step 6: Verify cron will push tonight.**

The cron file `/etc/cron.d/era-backup` runs `/opt/era/deploy/era-backup.sh` at 03:00 UTC. After AE-1 ships, next nightly run will also push to B2 via the new stage.

To verify sooner, manually trigger the whole script:
```
ssh era@178.105.44.3 'sudo /opt/era/deploy/era-backup.sh'
```

Expected: `/var/log/era-backup.log` shows both the local dump + rclone copy.

- [ ] **Step 7: Mark phase.**

```bash
git commit --allow-empty -m "phase(AE): offsite backup to B2 verified; nightly cron will push"
```

## Task AE-5: Phase AE smoke

**Files:**
- Create: `scripts/smoke/phase_ae_backup.sh`

- [ ] **Step 1: Smoke runs the backup script in a dry-run locally.**

Since `deploy/era-backup.sh` writes to `/opt/era/pi-agent.db` (VPS path, not Mac), a full local run isn't meaningful. The smoke just validates shell syntax and rclone conf template parseability.

```bash
#!/usr/bin/env bash
# Phase AE smoke: backup script syntax + rclone template validity.
set -euo pipefail

# Shell syntax check
bash -n deploy/era-backup.sh

# rclone template has the expected placeholders
grep -q "YOUR_B2_ACCOUNT_ID"     deploy/rclone.conf.template
grep -q "YOUR_B2_APPLICATION_KEY" deploy/rclone.conf.template

# install.sh includes rclone in the apt list
grep -q "rclone"                  deploy/install.sh

echo "OK: phase AE — backup script + rclone template + install.sh all valid"
```

- [ ] **Step 2: Run + commit.**

```
chmod +x scripts/smoke/phase_ae_backup.sh
bash scripts/smoke/phase_ae_backup.sh
git add scripts/smoke/phase_ae_backup.sh
git commit -m "docs(smoke): phase AE offsite backup"
```

---

# Phase AF — GitHub Actions CI

**Goal:** Push to master → run test matrix → on green, auto-deploy to VPS. `make deploy` retained as emergency manual path.

**Manual prerequisites (done BEFORE any AF task):**
1. Generate CI keypair + install on VPS (AF-1 manual step)
2. Add `DEPLOY_SSH_KEY` secret to GitHub repo (AF-1)
3. Convert `/opt/era` on VPS to a git checkout (AF-2)

## Task AF-1: Manual — CI keypair + GH secret

- [ ] **Step 1: Generate keypair on Mac.**

```
ssh-keygen -t ed25519 -f ~/.ssh/era_ci -C "era-ci@github-actions" -N ""
```

Output: `~/.ssh/era_ci` (private) + `~/.ssh/era_ci.pub` (public).

- [ ] **Step 2: Append public key to VPS's era user.**

```
cat ~/.ssh/era_ci.pub | ssh era@178.105.44.3 'cat >> ~/.ssh/authorized_keys'
```

Verify:
```
ssh -i ~/.ssh/era_ci era@178.105.44.3 'whoami'
```

Must return `era`.

- [ ] **Step 3: Store private key as GitHub Actions secret.**

```
gh secret set DEPLOY_SSH_KEY < ~/.ssh/era_ci
```

Verify via `gh secret list` — should show `DEPLOY_SSH_KEY` with a recent updated timestamp.

- [ ] **Step 4: Track manual completion.**

No commit. Move to AF-2.

## Task AF-2: Manual — convert `/opt/era` to git checkout

- [ ] **Step 1: Confirm current state.**

```
ssh era@178.105.44.3 'cd /opt/era && git status 2>&1'
```

Expected either:
- "fatal: not a git repository" → initial state; continue with AF-2 step 2.
- A clean tree on master → already converted; skip to AF-3.

- [ ] **Step 2: Init + attach remote + fetch + checkout.**

```
ssh era@178.105.44.3 'cd /opt/era && \
    git init && \
    git remote add origin https://github.com/vaibhav0806/era.git && \
    git fetch origin && \
    git checkout -B master origin/master'
```

This overwrites any files that differ from master. Before running, ensure:
- `.env` is at `/etc/era/env` (not inside /opt/era) — confirmed in M4-Y.
- `pi-agent.db` is at `/opt/era/pi-agent.db` — in `.gitignore`, git checkout won't touch it.

- [ ] **Step 3: Verify.**

```
ssh era@178.105.44.3 'cd /opt/era && git status && git log --oneline -3'
```

Expected: clean tree, on master, recent commits listed.

- [ ] **Step 4: Test `git pull` works.**

```
ssh era@178.105.44.3 'cd /opt/era && git pull origin master'
```

Expected: `Already up to date.` Or a fast-forward.

- [ ] **Step 5: Track completion.**

```bash
git commit --allow-empty -m "phase(AF): VPS /opt/era converted to git checkout; CI deploy pipeline ready"
```

## Task AF-3: Create `.github/workflows/ci.yml` with deploy disabled

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write workflow with `if: false` on deploy.**

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
    # M5-AF-3: deploy disabled until we confirm test gate is stable.
    # Re-enable in task AF-5 by removing the `false && ` prefix.
    if: false && github.ref == 'refs/heads/master' && github.event_name == 'push'
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

Note: the phase smokes step includes `phase_ab_tooling.sh` which does `docker run`. CI runners have Docker but the `era-runner:m2` image must exist. To keep CI self-contained, exclude tooling-bake smoke in CI OR pre-build the image in CI. Simplest: exclude Docker-requiring smokes in CI via a naming convention.

Adjust the phase smokes loop:

```yaml
      - name: phase smokes (non-docker)
        run: |
          for f in scripts/smoke/phase_*.sh; do
            case "$f" in
              *phase_ab_tooling.sh|*phase_h_docker.sh|*phase_i_e2e.sh|*phase_k_netlock.sh|*phase_m_secrets.sh)
                echo "SKIP (docker): $f"
                continue
                ;;
            esac
            echo "--- $f ---"
            bash "$f"
          done
```

- [ ] **Step 2: Local validation — yamllint or `act` if installed.**

```
# If yamllint installed:
yamllint .github/workflows/ci.yml

# Otherwise, visual review + GitHub's web UI will lint on first push.
```

- [ ] **Step 3: Commit with deploy disabled.**

```bash
git add .github/workflows/ci.yml
git commit -m "ci(gh-actions): test gate + deploy pipeline (deploy initially disabled)"
```

- [ ] **Step 4: Push to master.**

```
git push origin master
```

Expected: GitHub Actions runs the `test` job. `deploy` job shows `skipped` (due to `if: false`). Test job passes.

Watch: `gh run watch` or https://github.com/vaibhav0806/era/actions.

- [ ] **Step 5: Confirm test gate is reliable across at least one more push.**

Make a trivial change (comment in a Go file, or a README typo fix). Push. Watch CI. Should run green.

## Task AF-4: Create `deploy/ROLLBACK.md` + `deploy/ROTATE_CI_KEY.md`

**Files:**
- Create: `deploy/ROLLBACK.md`
- Create: `deploy/ROTATE_CI_KEY.md`

- [ ] **Step 1: Write ROLLBACK.md.**

```markdown
# Emergency Rollback

If a CI auto-deploy breaks production (era service down, tasks failing unexpectedly):

```
ssh era@178.105.44.3
cd /opt/era
git log --oneline -10                # find last-good SHA
git reset --hard <SHA>
export PATH=/usr/local/go/bin:$PATH
make build
make docker-runner
sudo systemctl restart era
sudo systemctl status era            # confirm active (running)
```

Then fix the bad commit on master (revert or forward-fix) and push normally — CI will redeploy.
```

- [ ] **Step 2: Write ROTATE_CI_KEY.md.**

```markdown
# Rotate the CI Deploy Key

If DEPLOY_SSH_KEY leaks or needs periodic rotation:

1. Generate new keypair on Mac:
   ```
   ssh-keygen -t ed25519 -f ~/.ssh/era_ci_new -C "era-ci-$(date +%Y%m)" -N ""
   ```

2. Append new public key to VPS:
   ```
   cat ~/.ssh/era_ci_new.pub | ssh era@178.105.44.3 'cat >> ~/.ssh/authorized_keys'
   ```

3. Update GitHub secret:
   ```
   gh secret set DEPLOY_SSH_KEY < ~/.ssh/era_ci_new
   ```

4. Trigger a CI deploy (empty commit, or workflow_dispatch) to confirm new key works.

5. Remove the old public key from VPS:
   ```
   ssh era@178.105.44.3
   vi ~/.ssh/authorized_keys        # delete the "era-ci@github-actions" line for the OLD key
   ```

6. Delete old local keypair:
   ```
   shred -u ~/.ssh/era_ci ~/.ssh/era_ci.pub
   mv ~/.ssh/era_ci_new ~/.ssh/era_ci
   mv ~/.ssh/era_ci_new.pub ~/.ssh/era_ci.pub
   ```
```

- [ ] **Step 3: Commit.**

```bash
git add deploy/ROLLBACK.md deploy/ROTATE_CI_KEY.md
git commit -m "docs(deploy): rollback + CI-key rotation runbooks"
```

## Task AF-5: Enable the deploy job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Remove the `if: false` gate.**

Change:
```yaml
    if: false && github.ref == 'refs/heads/master' && github.event_name == 'push'
```

to:
```yaml
    if: github.ref == 'refs/heads/master' && github.event_name == 'push'
```

- [ ] **Step 2: Commit.**

```bash
git add .github/workflows/ci.yml
git commit -m "ci(gh-actions): enable auto-deploy on push to master"
```

- [ ] **Step 3: Push + observe.**

```
git push origin master
```

This push itself triggers CI → tests run → deploy job runs → ssh era@VPS → git pull (pulls this commit, which is now the HEAD) → build → restart.

Watch `gh run watch`. On VPS:
```
ssh era@178.105.44.3 'sudo journalctl -u era -n 30'
```

Expected: era.service restart line with the new binary timestamp. orchestrator ready log appears.

- [ ] **Step 4: Verify end-to-end via Telegram.**

Send a task:
```
/task vaibhav0806/trying-something add CI_TEST.md with one line "auto-deployed by CI"
```

Expected: task completes, PR opens. The orchestrator that handled it is the one CI just deployed.

## Task AF-6: Phase AF smoke

**Files:**
- Create: `scripts/smoke/phase_af_ci.sh`

- [ ] **Step 1: Write smoke.**

```bash
#!/usr/bin/env bash
# Phase AF smoke: ci.yml exists, is valid YAML, has the expected structure.
set -euo pipefail

test -f .github/workflows/ci.yml

# Parse as YAML with python (always available on modern Macs/Linux)
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"

# Has both jobs
grep -qE '^  test:'   .github/workflows/ci.yml
grep -qE '^  deploy:' .github/workflows/ci.yml

# Deploy is no longer if:false
if grep -qE '^\s+if:\s+false' .github/workflows/ci.yml; then
    echo "FAIL: deploy job still gated on 'if: false'"
    exit 1
fi

echo "OK: phase AF — ci.yml valid and deploy enabled"
```

- [ ] **Step 2: Run + commit.**

```
chmod +x scripts/smoke/phase_af_ci.sh
bash scripts/smoke/phase_af_ci.sh
git add scripts/smoke/phase_af_ci.sh
git commit -m "docs(smoke): phase AF CI workflow"
```

- [ ] **Step 3: Push.**

```
git push origin master
```

Watch CI run green end-to-end.

---

# Final — README update + tag

## Task F-1: README M5 status + roadmap

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Prepend M5 status section.**

Before the existing `## Status: Milestone 4 — …` heading, insert:

```markdown
## Status: Milestone 5 — polish + safety

M5 sharpens the production era install that shipped in M4 and adds the safety rails around it:

- **CI replaces `make deploy`.** Push to master → GitHub Actions runs `go vet` + `gofmt -l` + `go test -race` + `go build` + all `scripts/smoke/phase_*.sh` → on green, ssh's to the VPS, `git pull`, rebuilds, restarts systemd. `make deploy` stays as an emergency manual path.
- **Offsite backups.** Nightly SQLite dump now also pushes to a private Backblaze B2 bucket with 30-day retention. `/var/backups/era/` keeps 7 days locally for fast restore.
- **Pre-commit tests.** Runner detects a top-level `Makefile` with a `test` target in the cloned repo and runs `make test` before `git commit`. Failure aborts the push and DMs the test output — no broken code lands in a PR.
- **PR approval feedback.** Tapping Approve in Telegram now adds an `era-approved` label + submits an APPROVED review on the GitHub PR. Tapping Reject posts a comment with the diff-scan findings before closing + deleting the branch. The GitHub PR page reflects era's decision.
- **Runner tooling baked in.** Python, Rust, Go toolchains + common build dependencies (`build-base`, `openssl-dev`, `libffi-dev`, etc.) are now pre-installed in the runner image. Tasks that need `pip install numpy` or `cargo build` no longer hit the egress allowlist trying to reach Alpine's CDN.
- **Looser VPS sudoers.** era user now has wildcarded `NOPASSWD` for any `systemctl * era` and `journalctl -u era *` — fixes the `--no-pager` friction from M4.
- **Cleanup.** `internal/githubpr` auth header normalized to `Bearer` (matching siblings); removed dangling env template keys; gitignored stray build artifacts.

Everything from M4 still applies.
```

- [ ] **Step 2: Update roadmap table.**

Find the roadmap list at the bottom. Change `← you are here` from M4 to M5 and add an M5 row.

- [ ] **Step 3: Commit.**

```bash
git add README.md
git commit -m "docs(readme): M5 shipped — polish + safety"
```

## Task F-2: Final regression + tag + push

- [ ] **Step 1: Full regression.**

```
go test -race -count=1 ./...
```

Expected: all packages green.

- [ ] **Step 2: Run all phase smokes locally.**

```
for f in scripts/smoke/phase_z_*.sh scripts/smoke/phase_a*.sh; do
    echo "--- $f ---"
    bash "$f"
done
```

All should print OK.

- [ ] **Step 3: Tag.**

```bash
git tag m5-release
git push origin master
git push origin m5-release
```

- [ ] **Step 4: Final live end-to-end via CI.**

The push in step 3 triggers CI. Watch it:
```
gh run watch
```

Expected: test job green → deploy job green → era.service restarts on VPS → Telegram bot responds to a test task.

- [ ] **Step 5: Done.**

```bash
git commit --allow-empty -m "phase(M5): milestone complete — m5-release tagged + pushed + deployed"
```

---

## Post-M5 followups (NOT in this plan)

Deferred to M6+:

- `/ask` read-only shortcut (safety regression — requires separate design pass)
- Natural-language repo parsing
- Option-A pre-push approval (diff-scan BEFORE push)
- Long-answer Telegram attachments
- PR auto-merge on approve
- Dev/prod bot split
- Task chaining / scheduled tasks
- `/stats` command / metrics dashboard
- Alpine CDN allowlist expansion (arbitrary `apk add` at runtime)
- Java/PHP/Ruby toolchains (bake-on-demand)
- sqlc drift check in CI
- Cross-arch build verification in CI
- Prometheus / metrics export
- Slack integration
