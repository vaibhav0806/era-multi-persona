# era

A personal agent orchestrator that runs tasks via Telegram, executes them in disposable Docker containers, and reports back with a pushed git branch. See [FEATURE.md](./FEATURE.md) for the full vision and design principles.

The name reflects the intent: this is the chapter where the typing gets delegated and the focus shifts to describing and reviewing. M0 lays down the chassis; later milestones swap in a real coding agent, network allowlisting, and approval gates.

## Status: Milestone 2 — security hardening

M2 wraps Pi in a hardened sandbox. A second Go binary (`cmd/sidecar`) runs alongside the runner inside the container as the only process with internet access (enforced via iptables with uid-based rules). Pi's HTTP traffic is forced through the sidecar via `HTTPS_PROXY`. The sidecar exposes exactly what Pi needs:

- **`/llm/*`** — OpenRouter passthrough with auth injection. Pi's `OPENROUTER_API_KEY` is a dummy; the real key lives only in the sidecar and is injected on forward.
- **`/search`** — Tavily-backed web search. Returned URL hosts get a short-lived permit so `/fetch` can retrieve them.
- **`/fetch?url=...`** — fetches pages from allowlisted or search-permitted hosts, with a content-type filter.
- **`/credentials/git`** — git credential helper returning short-lived GitHub App installation tokens (1hr TTL).

Classic PATs are **removed from every production code path.** The orchestrator uses a [GitHub App](https://docs.github.com/en/apps) to mint fresh per-task installation tokens via JWT; the token flows through env → sidecar → git credential helper → git push. Leaked tokens expire within the hour.

Every HTTP request the container makes is logged to the `events` table as a `http_request` event — an audit trail you can `sqlite3` into for any task.

**What's proven by passing tests + live smoke:**
- [x] Container egress locked: non-allowlisted hosts return 403 at the sidecar / TCP RST at iptables
- [x] Runner's OS env contains neither OpenRouter key nor GitHub token
- [x] Pi's LLM calls route through `/llm/*` (verified via audit log)
- [x] Git operations route through `/credentials/git` (verified via audit log)
- [x] GitHub App tokens minted fresh per task, cached within TTL, refreshed when stale

**What M2 still does NOT have** (deferred to M3):
- No prompt-injection diff-scan (reward-hacking detection was deferred; the runner's system prompt includes untrusted-content guidance but there's no post-hoc diff gate)
- No approval gates or inline Telegram buttons
- No EOD digest

**Rule of thumb for M2:** still use a throwaway sandbox repo. The security improvements mean a compromised Pi cannot read your secrets or exfil to arbitrary hosts — but Pi's `bash` tool still runs freely inside the container, and M2 has no diff-scan gate on reward-hacking patterns.

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
| `/task <description>` | Queue a task. Bot replies with the task id. |
| `/status <id>` | Report the current status of a task. |
| `/list` | Show the 10 most recent tasks. |

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

- **M0 — plumbing** ← you are here
- **M1 — real agent**: Pi + OpenRouter (Kimi K2.5/K2.6), per-task token + 1h timeout caps
- **M2 — security**: network allowlist per container, secret proxy sidecar, untrusted-content tags, diff-scan reward-hacking guards, GitHub App installation tokens
- **M3 — approvals + digest**: inline Telegram approval buttons, approval state machine, EOD digest generator
