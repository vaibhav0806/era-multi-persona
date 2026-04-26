# era-multi-persona

> 0G hackathon fork of [era](https://github.com/vaibhav0806/era) — a personal Telegram-driven coding agent reframed as a multi-persona swarm running on the 0G stack.

Same product spine as upstream era: a coding agent that clones a repo, writes code, pushes a branch, opens a PR — driven from Telegram. The fork swaps in a multi-persona pipeline (planner → coder → reviewer), shared persistent memory + audit log on 0G Storage, and (later) sealed-inference receipts on 0G Compute, iNFT-minted personas (ERC-7857), and ENS subnames per persona.

See [FEATURE.md](./FEATURE.md) for the full vision and design principles.

## Hackathon submission targets

| Track | Prize | Status |
|---|---|---|
| 0G Best Autonomous Agents, Swarms & iNFT | $7.5k | swarm + receipts on 0G ✅ — iNFT pending M7-D |
| 0G Best Agent Framework, Tooling & Core Extensions | $7.5k | `era-brain` SDK shipped (M7-A) ✅ |
| ENS Best Integration for AI Agents | $2.5k | pending M7-E |
| ENS Most Creative Use *(stretch)* | $2.5k | pending |

Skip: Uniswap, KeeperHub, Gensyn AXL — wrong primitive for a coding agent. Forced integration < focused submission.

---

## Status: Milestone 7-B.2 — orchestrator wired to 0G Storage

M7-B.2 wires real `/task` to the dual memory provider — every persona's audit-log receipt is written to BOTH 0G testnet AND local SQLite, with graceful degradation when 0G writes fail.

- **`memory/dual` provider in production.** `cmd/orchestrator/main.go` constructs `dual.New(sqlite_cache, zg_composite_primary)` whenever `PI_ZG_PRIVATE_KEY/PI_ZG_EVM_RPC/PI_ZG_INDEXER_RPC` env vars are present. Fall-back to sqlite-only when missing — M7-A.5 behavior preserved as the default.
- **Reviewer prompt cap.** `internal/swarm/composeCoderOutput` truncates the diff fed to the reviewer at 30k chars (~7.5k tokens). Big diffs (lockfile regenerations, large refactors) used to blow past gpt-4o-mini's 128k token window with `openrouter 400`; now they don't.
- **Resilience proof in production.** A reverted 0G transaction during the live gate (`Transaction execution failed`) was caught by `dual.Provider.onPrimaryError`, surfaced as a `slog.Warn`, and did NOT block the task. Cache held the canonical record; reviewer continued; PR landed.
- **5 testnet transactions on the wallet** ([Galileo block explorer](https://chainscan-galileo.0g.ai/)) prove writes are real, not theoretical.

Everything from M7-B.1 still applies.

## Status: Milestone 7-B.1 — `era-brain` 0G Storage SDK

M7-B.1 added 0G Storage as a `memory.Provider` impl in the `era-brain` SDK:

- **`memory/zg_kv`** — KV semantics on 0G KV streams. `Provider` + `LiveOps` (SDK wrapper) + `kvOps` interface seam for unit tests.
- **`memory/zg_log`** — append-only log on 0G KV streams using sequence-numbered keys (`000001`, `000002`, ...). Reuses `zg_kv.LiveOps` so SDK init is shared.
- **`memory/dual`** — write-both, read-cache-first wrapper. Cache failure is fatal (broken local DB); primary failure is non-fatal (logged via optional `OnPrimaryError` hook, task continues).
- **5 SDK deviations from documented templates** captured in `scripts/zg-smoke/zg-smoke.go` header comments — module path is `0gfoundation/0g-storage-client` (not `0glabs`), Batcher is in-place mutation not builder, indexer lives in its own package, etc. Read those before extending.
- Testnet endpoints: EVM RPC `https://evmrpc-testnet.0g.ai`, Indexer Turbo `https://indexer-storage-testnet-turbo.0g.ai`. The KV node URL is currently best-effort — read paths gracefully degrade to cache when no working KV node is configured.

## Status: Milestone 7-A.5 — swarm runner integration

M7-A.5 wires the `era-brain` swarm into era's existing `/task` pipeline:

- **Orchestrator-side swarm.** `internal/swarm/` calls planner (LLM) before container spawn, reviewer (LLM) after Pi finishes — Pi remains the coder persona's tool-loop engine inside Docker. Spec §3 said "container hosts the swarm"; we deviated for 2-week velocity (no Docker image rebuild needed; reviewer naturally consumes the GitHub-compare diff that's already wired).
- **CompletedArgs cascade.** `Notifier.NotifyCompleted` widened from positional args to a `CompletedArgs` struct carrying persona breakdown (planner plan, reviewer decision/critique, 3 receipts). All callers updated atomically.
- **Telegram persona DM.** Completion DM gets `— planner: <step list>` and `— reviewer: approve|flag — <critique>` footers. Needs-review DM gets the same persona context above the existing Approve/Reject keyboard.
- **Per-task gas budget.** Default OpenRouter model `openai/gpt-4o-mini` for both planner + reviewer (~$0.001 per task). Override via `PI_BRAIN_PLANNER_MODEL` / `PI_BRAIN_REVIEWER_MODEL`.

## Status: Milestone 7-A — `era-brain` SDK skeleton

M7-A stood up a `go get`-able Go SDK at `era-brain/` (separate Go module inside this monorepo) with the six core abstractions powering everything that follows:

- **Five interfaces** — `Persona`, `MemoryProvider`, `LLMProvider`, `INFTRegistry` *(impl in M7-D)*, `IdentityResolver` *(impl in M7-E)*.
- **`Brain.Run(personas...)` orchestrator** — sequential persona chain with output threading, receipt accumulation, fail-fast on first persona error.
- **`LLMPersona`** — concrete `Persona` impl that composes a prompt from (system prompt + prior outputs), calls the LLM, computes input/output hashes via length-prefixed encoding (collision-resistant for M7-D's on-chain receipt logging), writes a `Receipt` to the audit log.
- **Reference impls** — `memory/sqlite` (production-grade with `SetMaxOpenConns(1)` for concurrent safety + `is_kv` flag for dual KV/Log on a single table), `llm/openrouter` (OpenAI-compatible HTTP client with sealed=false; sealed=true arrives in M7-C).
- **Working example** — `era-brain/examples/coding-agent` demonstrates the 3-persona flow against real OpenRouter; live gate verified planner numbered list + coder unified diff + reviewer DECISION line.

---

## Prerequisites

- Go 1.22+ (`brew install go`)
- Docker (`brew install --cask docker`)
- A Telegram bot token (from [@BotFather](https://t.me/BotFather)) and your numeric user ID (message [@userinfobot](https://t.me/userinfobot))
- A throwaway GitHub repo (e.g. `<you>/era-sandbox`) with a `README.md` committed
- A [GitHub App](https://github.com/settings/apps/new) installed on your sandbox repo with `Contents: Read and write` + `Metadata: Read-only` permissions. Note the App ID, download the private key (.pem), and note the Installation ID from the install URL.
- An [OpenRouter](https://openrouter.ai) account + API key with at least a few dollars of credit
- A [Tavily](https://tavily.com) API key (free tier: 1000 queries/mo) for the sidecar's `/search` endpoint
- **(M7-B onward)** A 0G testnet wallet (`cast wallet new` or any EVM-compatible private key) funded from the [0G testnet faucet](https://docs.0g.ai/developer-hub/testnet/testnet-overview).

## Setup

```bash
git clone git@github.com:vaibhav0806/era-multi-persona.git
cd era-multi-persona/era   # repo's working dir
cp .env.example .env
# Edit .env and fill in the required values. PI_ZG_* are optional —
# orchestrator falls back to sqlite-only when missing (M7-A.5 baseline).

make docker-runner    # builds bin/era-runner-linux + era-runner image
make build            # builds bin/orchestrator
./bin/orchestrator
```

Expected boot lines:

```
... goose: successfully migrated database to version: 9
... INFO github app token source configured app_id=... installation_id=...
INFO[0000] Selecting nodes ...                     (only when PI_ZG_* set)
INFO[0000] Selected Nodes... ips="[...]"
... INFO 0G storage wired indexer=https://... kv_node_set=true|false
... INFO orchestrator ready version=... db_path=... sandbox_repo=...
... INFO digest scheduled fires_at_utc=...
```

## Telegram commands

| Command | Effect |
|---------|--------|
| `/task <description>` | Queue a task on the default sandbox repo. |
| `/task <owner>/<repo> <description>` | Queue a task on any repo your GitHub App is installed on. |
| `/task --budget=quick\|default\|deep <desc>` | Override the per-task budget profile. |
| `/status <id>` | Report the current status of a task. |
| `/list` | Show the 10 most recent tasks. |
| `/cancel <id>` | Cancel a queued (not-yet-started) task. |
| `/retry <id>` | Clone a prior task's description into a new queued task. |
| `/ask <repo> <question>` | Read-only short answer (no commit, no push). |
| `/stats` | 24h/7d/30d activity summary. |

A successful `/task` produces a Telegram completion DM with branch + PR URL + the planner step list (truncated to 200 chars) + reviewer decision. A flagged `/task` produces a needs-review DM with diff preview + Approve/Reject inline buttons.

## Development

```bash
make test         # unit + integration tests for era root module
make test-race    # with race detector
make lint         # go vet
make fmt          # go fmt + goimports

# era-brain SDK has its own go.mod
cd era-brain && go test -race ./...

# Live 0G testnet tests (build-tagged so CI skips them)
set -a; source ../.env; set +a
go test -tags zg_live ./memory/zg_kv/... ./memory/zg_log/...

# era's full e2e (requires .env + Docker + sandbox repo, creates real branch)
cd .. && set -a; source .env; set +a
go test -tags e2e -v -timeout 3m ./internal/e2e/...
```

## Layout

```
cmd/orchestrator/             main entrypoint — wires queue, runner, swarm, dual provider
cmd/runner/, cmd/sidecar/     in-container Pi loop + secret-proxy sidecar
internal/config/              env-var config
internal/db/                  SQLite + sqlc queries (era's main DB at ./pi-agent.db)
internal/telegram/            bot client + command handler
internal/queue/               task lifecycle (create, claim, run, notify); CompletedArgs/NeedsReviewArgs
internal/swarm/               era-brain swarm wrapper — Plan, Review, InjectPlan, planner+reviewer prompts
internal/runner/              Docker wrapper + adapter to queue.Runner
internal/audit/               tool-call audit log (era's main DB)
internal/diffscan/            reward-hacking pattern detection
internal/digest/              EOD digest generator
internal/githubapp/           GitHub App installation token source
internal/githubcompare/       PR diff fetch (consumed by both diffscan + reviewer)
internal/githubpr/            PR open/close/label/comment
internal/githubbranch/        branch delete on reject
internal/e2e/                 end-to-end test (build tag: e2e)
internal/budget/              per-task budget profiles
internal/replyprompt/         reply-to-continue prompt composition
internal/stats/               /stats query types
internal/progress/            mid-run progress event types

era-brain/                    standalone Go SDK — separate go.mod, go-get-able as
                              github.com/vaibhav0806/era-multi-persona/era-brain
era-brain/brain/              Brain orchestrator + Persona interface + LLMPersona
era-brain/memory/             MemoryProvider interface + impls
era-brain/memory/sqlite/      reference impl (used as cache by dual)
era-brain/memory/zg_kv/       0G KV streams (M7-B.1) — KV semantics
era-brain/memory/zg_log/      0G KV streams (M7-B.1) — Log semantics via sequence-numbered keys
era-brain/memory/dual/        write-both, read-cache-first wrapper
era-brain/llm/                LLMProvider interface + impls
era-brain/llm/openrouter/     OpenRouter HTTP client
era-brain/inft/               INFTRegistry interface (impl in M7-D)
era-brain/identity/           IdentityResolver interface (impl in M7-E)
era-brain/examples/coding-agent/  3-persona reference example w/ optional --zg-live

migrations/                   goose SQL + embed package
queries/                      sqlc input SQL
docker/runner/                Dockerfile + entrypoint for the Pi-based runner
scripts/smoke/                manual smoke-test reference scripts (M0+)
scripts/zg-smoke/             0G Storage SDK verification script (M7-B.1)
deploy/install.sh             one-shot Hetzner VPS bootstrap (M4+)
docs/superpowers/specs/       brainstormed design docs per milestone
docs/superpowers/plans/       implementation plans per milestone
```

## Roadmap

**Hackathon fork (M7-A onward):**

- **M7-A** — `era-brain` SDK skeleton: 5 interfaces + Brain orchestrator + sqlite + openrouter impls + coding-agent example
- **M7-A.5** — orchestrator-side swarm: `internal/swarm/` wraps `Brain`; planner-before-container + reviewer-after-PR; persona DM
- **M7-B.1** — `era-brain` 0G Storage SDK: zg_kv + zg_log + dual providers; live testnet writes
- **M7-B.2** ← *you are here*: production `/task` writes audit log to dual(sqlite, 0G); resilient on primary failures
- **M7-B.3** *(planned)* — persona KV reads: `LLMPersona` reads prior memory before LLM call → "evolving memory" criterion lands
- **M7-C** *(planned)* — 0G Compute sealed inference as `LLMProvider` impl; receipts flip `Sealed=true`; reviewer cross-checks the attestation
- **M7-D** *(planned)* — fork ERC-7857; deploy iNFT contract; mint 3 default personas + `/persona-mint` for custom; `recordInvocation(tokenId, receiptHash)` per persona run
- **M7-E** *(planned)* — ENS subname registration + wildcard resolver: `coder.<user>-era.eth` resolves to iNFT addr + 0G Storage URI for memory blob
- **M7-F** *(planned)* — polish, examples, demo video, hackathon submission

**Pre-fork upstream era milestones (M0-M6):** see [the original era repo](https://github.com/vaibhav0806/era) for full M0-M6 status sections. Summary:

- **M0** — plumbing: SQLite persistence, Telegram loop, Docker runner, dummy agent
- **M1** — real agent: Pi + OpenRouter (Kimi K2.5/K2.6), per-task token + 1h timeout caps
- **M2** — security: network allowlist per container, secret proxy sidecar, untrusted-content tags, diff-scan reward-hacking guards, GitHub App installation tokens
- **M3** — approvals + digest: inline Telegram approval buttons, EOD digest
- **M3.5** — multi-repo per task (`/task <owner>/<repo> <desc>`)
- **M4** — Hetzner VPS deploy + PR-per-task + mid-run `/cancel` + Pi prose in DMs
- **M5** — GitHub Actions CI + auto-deploy, offsite B2 backups, pre-commit test gate, runner tooling baked in
- **M6** — agent sharpness: budget profiles, smarter egress, reply-to-continue, mid-run progress DMs, `/ask`, `/stats`

## Security notes — read before running

The original M0 caveats (network allowlist, prompt injection, push credential blast radius) all apply. M2 hardened most of them; M5 added pre-commit test gating. The hackathon fork additionally introduces:

- **Hot wallet for 0G testnet.** `PI_ZG_PRIVATE_KEY` lives in `.env`. Use a wallet with **only enough ZG for gas** — never put real funds in it. Loss of the wallet = loss of mint capability for that orchestrator instance, NOT loss of past data (0G Storage is content-addressed; iNFTs already minted are owned by their respective wallets).
- **0G failures are non-fatal.** When 0G writes fail (testnet hiccups, indexer down, transaction reverts), `dual.Provider.onPrimaryError` logs via `slog.Warn` and the task continues. The cache (SQLite) holds the canonical local record. Don't treat 0G as load-bearing for task completion.
- **Same Telegram-allowlist rule as M0.** The orchestrator silently drops messages from any user ID other than `PI_TELEGRAM_ALLOWED_USER_ID`.

**Rule of thumb:** still a personal tool. Sandbox repo only.
