# era

*ephemeral runtime agent.*

A personal agent orchestrator that runs tasks via Telegram, executes them in disposable Docker containers, and reports back with a pushed git branch. See [FEATURE.md](./FEATURE.md) for the full vision and design principles.

*era* = **e**phemeral **r**untime **a**gent. Every task spawns a fresh disposable Docker container, does its work, and exits; no state lives longer than a single task run. The name also reflects the intent: this is the chapter where the typing gets delegated and the focus shifts to describing and reviewing. M0 lays down the chassis; later milestones swap in a real coding agent, network allowlisting, and approval gates.

## Status: Milestone 5 — polish + safety

M5 sharpens the production era install that shipped in M4 and adds the safety rails around it:

- **CI replaces `make deploy`.** Push to master → GitHub Actions runs `go vet` + `gofmt -l` + `go test -race` + `go build` + a curated subset of `scripts/smoke/phase_*.sh` → on green, ssh's to the VPS, `git pull`, rebuilds, restarts systemd. `make deploy` stays as an emergency manual path.
- **Offsite backups.** Nightly SQLite dump now also pushes to a private Backblaze B2 bucket with 30-day retention. `/var/backups/era/` keeps 7 days locally for fast restore.
- **Pre-commit tests.** Runner detects a top-level `Makefile` with a `test` target in the cloned repo and runs `make test` before `git commit`. Failure aborts the push and DMs the test output — no broken code lands in a PR.
- **PR approval feedback.** Tapping Approve in Telegram now adds an `era-approved` label + posts an "Approved via era" issue comment on the GitHub PR. Tapping Reject posts a comment with the diff-scan findings before closing + deleting the branch. The PR page reflects era's decision.
- **Runner tooling baked in.** Python, Rust, Go toolchains + common build dependencies (`build-base`, `openssl-dev`, `libffi-dev`, sqlite-dev, utilities like `rg` and `fd`) are now pre-installed in the runner image. Tasks that need `pip install` or `cargo build` no longer hit the egress allowlist trying to reach Alpine's CDN. `storage.googleapis.com` also allowlisted so Go module tarball downloads complete cleanly.
- **Looser VPS sudoers + NotifyFailed truncation.** era user now has wildcarded `NOPASSWD` for any `systemctl * era` and `journalctl -u era *` — fixes the `--no-pager` friction from M4. Long docker-log failure DMs now truncate under Telegram's 4096-char cap instead of getting silently dropped.
- **Cleanup.** `internal/githubpr` auth header normalized to `Bearer` (matching siblings); removed dangling env template keys; gitignored stray build artifacts.

Everything from M4 still applies.

## Status: Milestone 4 — deployment, PRs, mid-run cancel, read-only answers

M4 shipped four things:

1. **Deployment to a Hetzner CAX11 VPS.** Era now runs 24/7 from `era@178.105.44.3` under systemd, not the user's laptop. One-shot `deploy/install.sh` bootstraps a fresh Ubuntu 24.04 box: installs docker + Go 1.25, creates a non-root `era` user, enables UFW + unattended-upgrades, drops in a hardened systemd unit, schedules a nightly SQLite backup with 7-day retention, disables root SSH once the era user's key is verified. Code updates go out via `make deploy VPS_HOST=...`.
2. **PR creation on every completed task.** The orchestrator opens a GitHub PR after push via a new `internal/githubpr/` client. Clean tasks DM the PR URL; flagged tasks DM the same PR URL alongside the inline diff + Approve/Reject buttons. **Approve** leaves the PR open (never auto-merges — the user merges manually). **Reject** closes the PR first, then deletes the branch. Base branch is auto-detected via the repo's `default_branch` API, not hardcoded.
3. **Mid-run `/cancel` via docker kill.** A `/cancel <id>` against a running task kills the Docker container in <2s. Orchestrator observes the kill (via a `RunningSet` flag written before `docker kill`), transitions the task to `cancelled` instead of `failed`, and DMs "cancelled mid-run". A startup reconcile sweeps any orphan `running` tasks to `failed` with a `reconciled_failed` event, so a restart-during-deploy doesn't leave ghosts.
4. **Pi's real prose in completion DMs.** The runner's RESULT line was space-delimited key=value, which mangled spaces in Pi's assistant text. Switched it to `RESULT <json>`. Runner now captures the last assistant text block from Pi's `message_end` events and surfaces it as the summary — both for committed tasks and read-only "what does the README say?" queries that don't commit anything. DMs are truncated rune-safely at 3500 bytes with a "(N bytes truncated)" footer.

Everything from M3.5 still applies — multi-repo per task (`/task owner/repo <desc>`), approvals, EOD digest, `/retry`, diff-scan, iptables egress lockdown, GitHub App tokens, audit log.

## Status: Milestone 3.5 — multi-repo per task

M3.5 lets a single orchestrator drive tasks across any repo the GitHub App is installed on. Send `/task vaibhav0806/my-side-project add a README` and era clones that repo, runs the agent, and pushes a branch there — completion DM links to the right repo. No orchestrator restart needed to switch targets. Tasks without a repo prefix still run on the sandbox default.

Everything else from M3 still applies — approvals, EOD digest, `/cancel`, `/retry`, diff-scan, iptables egress lockdown, GitHub App tokens, audit log.

## Status: Milestone 3 — approvals + EOD digest

M3 closes the human-in-the-loop gap. Every completed task is scanned for reward-hacking patterns (removed tests, `.skip` directives, weakened assertions, deleted test files). Clean tasks auto-complete as before. Flagged tasks transition to `needs_review` and the orchestrator sends a Telegram DM with the findings, an inline diff preview, a GitHub compare link, and **inline Approve / Reject buttons**:

- **Approve** → task stays at `approved`; the pushed branch remains on GitHub for you to review + merge.
- **Reject** → task transitions to `rejected` and the orchestrator deletes the branch via the GitHub App API. No residue on the sandbox repo.

At **11 PM IST** by default (`PI_DIGEST_TIME_UTC=17:30`, configurable), era sends an **end-of-day digest** summarizing the previous 24h of tasks: counts by status, total tokens, total cost, per-task list.

Two small quality-of-life commands:
- **`/cancel <id>`** — cancels a queued task before it starts. (Running tasks hit their wall-clock cap naturally — docker-kill is M4+.)
- **`/retry <id>`** — clones any prior task's description into a new queued task. Useful when a task fails or you want the same thing done again.

Everything from M2 still applies: containerized agent, iptables-locked egress, Tavily-backed search, OpenRouter passthrough, GitHub App tokens, audit log.

**What M3 adds (in tests and live):**
- Diff-scan rule engine catches `removed_test`, `skip_directive`, `weakened_assertion`, `deleted_test_file` patterns across Go / Python / JS test conventions
- GitHub compare API client fetches the diff for every pushed branch
- Telegram client supports inline keyboards + callback queries; the handler routes button taps into the approval state machine
- `approvals` table (dormant since M0) now records every approve/reject decision
- Deterministic EOD digest renderer + cron-style scheduler goroutine

**What M3 still does NOT have** (deferred to M4+):
- No running-task cancellation (docker kill)
- No VPS deployment helper
- No PR creation
- No option-A-style pre-push approval (runner commits + pushes unconditionally; diff-scan gates AFTER push)

Full roadmap and implementation plan: [`docs/superpowers/plans/`](./docs/superpowers/plans/).

## Prerequisites

- Go 1.22+ (`brew install go`)
- Docker (`brew install --cask docker`)
- A Telegram bot token (from [@BotFather](https://t.me/BotFather)) and your numeric user ID (message [@userinfobot](https://t.me/userinfobot))
- A throwaway GitHub repo (e.g. `<you>/era-sandbox` or any sandbox repo you own) with a `README.md` committed
- A [GitHub App](https://github.com/settings/apps/new) installed on your sandbox repo with `Contents: Read and write` + `Metadata: Read-only` permissions. Note the App ID, download the private key (.pem), and note the Installation ID from the install URL.
- An [OpenRouter](https://openrouter.ai) account + API key with at least a few dollars of credit
- A [Tavily](https://tavily.com) API key (free tier: 1000 queries/mo) for the sidecar's `/search` endpoint

## Setup

```bash
git clone git@github.com:vaibhav0806/era.git
cd era
cp .env.example .env
# Edit .env and fill in all six required values (PI_OPENROUTER_API_KEY is M1)

make docker-runner    # builds bin/era-runner-linux + era-runner:m1 image (~600MB)
make build            # builds bin/orchestrator
./bin/orchestrator
```

On startup you should see:
```
... OK   0001_init.sql (xx ms)
... OK   0002_add_cost_columns.sql (xx ms)
... goose: successfully migrated database to version: 2
... INFO orchestrator ready version=... db_path=... sandbox_repo=...
```

## Telegram commands

Send these to your bot:

| Command | Effect |
|---------|--------|
| `/task <description>` | Queue a task on the default sandbox repo. |
| `/task <owner>/<repo> <description>` | Queue a task on any repo your GitHub App is installed on. |
| `/status <id>` | Report the current status of a task. |
| `/list` | Show the 10 most recent tasks. |
| `/cancel <id>` | Cancel a queued (not-yet-started) task. |
| `/retry <id>` | Clone a prior task's description into a new queued task. |

When a task completes, the bot sends a message with the branch name and a link to the branch on GitHub. When a task fails, the bot sends the error.

## M0 security notes — read before running

M0 is deliberately insecure for simplicity. **Only point the orchestrator at a throwaway sandbox repo**.

- **No network allowlist.** The runner container has default Docker bridged networking. It can reach any host. A compromised or malicious agent could exfiltrate anything in its environment, including the PAT.
- **PAT is in the agent's environment.** The container receives `PI_GITHUB_PAT` as an env var, so any code running inside the container can read it. M2 moves the PAT behind a sidecar proxy so the container never sees it.
- **No prompt-injection protections.** The dummy runner doesn't read untrusted content, so M0 is safe from this — but as soon as a real agent is introduced (M1), prompt-injection protections (M2) must be in place before pointing it at any repo with untrusted content.
- **Wide Telegram bot permissions.** The bot token grants full control over the bot's chats. If leaked, anyone can drive it. Rotate via @BotFather / `/revoke` if that happens. The orchestrator silently drops messages from any user ID other than `PI_TELEGRAM_ALLOWED_USER_ID`, which is a weak but simple trust boundary.
- **One task at a time.** M0 processes tasks serially. No concurrency, no resource limits beyond Docker defaults.

**Rule of thumb for M0:** assume the sandbox repo can be destroyed or corrupted at any time; do not point the orchestrator at anything you care about.

## Development

```bash
make test         # unit + integration tests
make test-race    # with race detector
make lint         # go vet
make fmt          # go fmt + goimports

# End-to-end test (requires .env + Docker + sandbox repo, creates real branch):
set -a; source .env; set +a
go test -tags e2e -v -timeout 3m ./internal/e2e/...
```

## Layout

```
cmd/orchestrator/      # main entrypoint
internal/config/       # env-var config
internal/db/           # SQLite + sqlc queries
internal/telegram/     # bot client + command handler
internal/queue/        # task lifecycle (create, claim, run, notify)
internal/runner/       # Docker wrapper + adapter to queue.Runner
internal/e2e/          # end-to-end test (build tag: e2e)
migrations/            # goose SQL + embed package
queries/               # sqlc input SQL
docker/runner/         # Dockerfile + entrypoint for the M0 dummy runner
scripts/smoke/         # manual smoke-test reference scripts
docs/superpowers/plans # implementation plans (M0 and beyond)
```

## Roadmap

- **M0 — plumbing**: SQLite persistence, Telegram loop, Docker runner, dummy agent
- **M1 — real agent**: Pi + OpenRouter (Kimi K2.5/K2.6), per-task token + 1h timeout caps
- **M2 — security**: network allowlist per container, secret proxy sidecar, untrusted-content tags, diff-scan reward-hacking guards, GitHub App installation tokens
- **M3 — approvals + digest**: inline Telegram approval buttons, approval state machine, EOD digest generator
- **M3.5 — multi-repo**: per-task `target_repo`, `/task <owner>/<repo> <desc>` syntax
- **M4 — deployment + PRs + cancel + prose**: Hetzner VPS via `deploy/install.sh` + `make deploy`, PR-per-task on clean/flagged paths, mid-run `/cancel` via docker kill, Pi's actual text in DMs
- **M5 — polish + safety** ← you are here: GitHub Actions CI + auto-deploy, offsite B2 backups, pre-commit test gate, PR approval feedback (label + comment), runner tooling bake (Python/Rust/Go + build deps), wildcarded sudoers, Bearer auth normalization
