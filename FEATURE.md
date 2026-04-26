# FEATURE.md

> Source of truth for **era-multi-persona** — the 0G hackathon fork of era.
> Intentionally more "what and why" than "how". Implementation details live in code.

---

## What this is

**era-multi-persona** is a fork of [era](https://github.com/vaibhav0806/era) reframed as a multi-persona swarm running on the 0G stack. Same product spine: a personal Telegram-driven coding agent that clones a repo, writes code, pushes a branch, and opens a PR. The difference: every task is now executed by three sealed-inference personas — **planner**, **coder**, **reviewer** — each running on 0G Compute, each with its own persistent memory on 0G Storage, each minted as an iNFT (ERC-7857) and addressable via an ENS subname.

This is a hackathon submission. The original era project at [github.com/vaibhav0806/era](https://github.com/vaibhav0806/era) is the production tool I use; era-multi-persona is the experimental track where on-chain primitives become first-class.

## Why

Three things converge here:

1. **The original era was a single Pi process pretending to be many roles.** Splitting into named personas with their own memory and their own sealed-inference receipts is a real architectural improvement, not a hackathon costume.
2. **Sealed inference + diff-scan is a sharper reward-hacking guard than diff-scan alone.** Today, era's diff-scan flags suspicious test-removals after the fact. With sealed inference, the *coder* can't lie about which model wrote the change — every commit comes with a cryptographic receipt. The reviewer persona consumes that receipt as evidence.
3. **iNFTs let "personas" become real artifacts.** A coder persona that has accumulated memory of my style across 200 tasks is genuinely valuable. As an iNFT, it's transferable, composable, and royalty-bearing when others invoke it. This is the first version of era where a persona is a noun.

## Hackathon prize targets

| Track | Prize | How we hit it |
|---|---|---|
| **0G Best Autonomous Agents, Swarms & iNFT** | $7.5k | 3-persona swarm w/ shared 0G Storage memory + iNFT-minted personas w/ royalty splits |
| **0G Best Agent Framework, Tooling & Core Extensions** | $7.5k | Standalone `era-brain` Go SDK w/ swappable memory/LLM/iNFT/identity providers + 3 reference example agents |
| **ENS Best Integration for AI Agents** | $2.5k | Per-persona subnames (`coder.<user>-era.eth`) resolving to iNFT addr + 0G Storage memory URI; functional, not cosmetic |
| ENS Most Creative Use *(stretch)* | $2.5k | Public agent activity feed page resolving user's ENS → task subnames → sealed-inference receipts |

**Skip:** Uniswap, KeeperHub, Gensyn AXL. Wrong primitive — era is a coding agent, not an on-chain value mover. Forced integrations would dilute the submission, not strengthen it.

## Design principles

These extend the original era's principles. The on-chain principles are new.

1. **Cheap to run.** Same target as before — under $5/day of LLM spend even with the agent working full-time. 0G Compute prices stay within budget; OpenRouter is the fallback when sealed endpoints fail.
2. **Human-in-the-loop by default.** The agent never touches external services, external keys, or irreversible actions without asking. Mint operations on the iNFT contract gate on the same Telegram allow-list as everything else.
3. **Transparent.** Every task produces three sealed-inference receipts on 0G Log, three on-chain `Invocation` events on the iNFT contract, and a Telegram DM with the ENS subnames involved. I can grep the entire history without trusting the orchestrator.
4. **Boring infrastructure.** SQLite over Postgres, Docker over Kubernetes, Telegram over custom frontend, Go binary over microservices. SQLite stays as the hot index; 0G Storage is the canonical layer above it.
5. **Disposable workspaces.** Every task runs in a fresh container. No state leaks between tasks. Persona memory lives in 0G KV, not in the container.
6. **Git is the deliverable.** The agent's output is always a branch, a PR, and three sealed-inference receipts pinned to that PR's metadata.
7. **Sealed > unsealed when both are available.** Default to 0G Compute sealed inference. Fall back to OpenRouter only when sealed call fails, and mark the resulting receipt `unsealed` so the reviewer knows.
8. **On-chain failures don't block tasks.** If iNFT recording or 0G Storage write fails, the task still completes. A reconciler retries in the background. The product never blocks waiting on a chain.

## How I interact with it

Telegram, same as before. New commands for the persona system:

| Command | Effect |
|---|---|
| `/task <description>` | Run a task on the default sandbox repo using the default 3-persona pipeline. |
| `/task <owner>/<repo> <description>` | Same, on a specific repo. |
| `/task --pipeline=plan,code,review <repo> <desc>` | Override the pipeline (advanced; default = `plan,code,review`). |
| `/personas` | List my owned personas (3 defaults + any custom-minted), with iNFT token IDs and ENS subnames. |
| `/persona-mint <name> <prompt>` | Mint a new custom persona as an iNFT. Records on 0G Chain, registers ENS subname, uploads system prompt to 0G Storage. |
| `/status <id>`, `/list`, `/cancel <id>`, `/retry <id>`, `/ask <repo> <question>`, `/stats` | Unchanged from era M6. |

When a task completes, the bot DMs: branch name, PR URL, the three ENS subnames involved (one per persona), the 0G Log URI for the audit trail, and a one-line "sealed: 3/3" or "sealed: 2/3 (coder unsealed, OpenRouter fallback)" status.

## The persona system (new core concept)

A **persona** = (system prompt) + (LLM provider config) + (accumulating memory blob on 0G Storage) + (iNFT token on 0G Chain) + (ENS subname).

Three default personas ship with era-multi-persona. They are minted on first orchestrator startup, owned by the user's hot wallet, and subname-registered automatically:

- **planner.\<user\>-era.eth** — reads task description + repo overview, drafts a step list. Default model: GLM-5-FP8 (sealed). Memory: prior plans by task fingerprint.
- **coder.\<user\>-era.eth** — reads the plan, runs the existing era tool loop (read/write/edit/run), commits. Default model: qwen3.6-plus (sealed). Memory: style observations across past tasks for this user.
- **reviewer.\<user\>-era.eth** — reads diff + plan + diff-scan output, produces critique and approve/flag decision. Default model: qwen3.6-plus (sealed). Memory: prior critique patterns.

Custom personas can be minted via `/persona-mint`. Same shape, user-defined system prompt. Royalty splits on the iNFT contract pay the creator a small cut whenever the persona is invoked by anyone — including the creator themselves on later tasks. (Self-royalties are a no-op net-of-gas; the point is third-party invocation if/when personas become tradeable.)

## What the agent can and can't do on its own

**Can do without asking (per persona, gated by container sandbox + iptables):**

- Read, write, edit files inside the workspace
- Run tests, linters, builds, local scripts
- Make local git commits on its own branch
- Call its configured LLM provider (0G Compute or OpenRouter) — receipt is logged regardless
- Read persona memory from 0G KV; append to 0G Log; read iNFT metadata
- Install standard dev dependencies (npm/pnpm/go/cargo from lockfile)

**Must ask first:**

- Push to any remote branch (existing M0 rule)
- Open a PR (existing M4 rule — auto-opens but reviewer flag triggers Approve/Reject DM)
- Mint a new persona iNFT (`/persona-mint` is interactive and confirms before submitting tx)
- Spend money beyond the per-task budget cap (existing M6 rule)

**Never does:**

- Run with network access beyond the iptables allowlist (now extended w/ 0G testnet RPC, 0G Storage gateway, 0G Compute endpoint, ENS gateway)
- Exceed the per-task budget cap, token cap, or wall-clock cap
- Hold the orchestrator's hot wallet private key directly — wallet lives in the sidecar, signs are proxied
- Touch persona memory belonging to another user (KV namespace is keyed on user ID + persona ID + signature)

## Security model

The original era's three threats still apply (prompt injection, reward hacking, push-credential blast radius). Two additions:

**4. Sealed-inference receipt forgery.** A compromised persona could fake a sealed-inference receipt to claim a coder ran on a model it didn't. Mitigated by: receipts are written directly to 0G Log by the LLM provider, not by the persona itself; reviewer fetches receipts from Log, not from coder's claim; mismatch flags the task.

**5. Hot-wallet compromise.** The orchestrator holds a hot wallet for minting + recording. Mitigation: small balance only (gas-budget), funded just-in-time. Loss of wallet = loss of mint capability for that orchestrator instance, but **not** loss of past data — 0G Storage is content-addressed; iNFTs already minted are owned by their respective wallets, not the orchestrator's.

Reward-hacking guard now stacks: existing diff-scan rules + sealed-inference receipt for the coder (so we know which model wrote the change) + reviewer persona's critique + reviewer's own sealed receipt (so the reviewer can't have been silently swapped either).

## Core components (high level)

- **`era-brain` (separate Go SDK).** The framework Track 1 ships. Defines `Persona`, `Brain`, `MemoryProvider`, `LLMProvider`, `INFTRegistry`, `IdentityResolver` interfaces. Ships impls for sqlite + 0G KV + 0G Log (memory), 0G Compute + OpenRouter (LLM), forked-ERC-7857 (iNFT), ENS (identity). Examples directory with three reference agents: `coding-agent` (the era integration), `audit-agent` (smart-contract review toy), `chat-agent` (minimal "hello brain" demo).
- **era orchestrator (this repo).** Existing Go binary. Imports `era-brain`. Replaces internal monolithic Pi loop with `Brain.Run([planner, coder, reviewer])`. Telegram bot, queue, Docker spawn, GitHub App, diff-scan, deploy scripts — unchanged from M6.
- **iNFT contract.** Forked ERC-7857 deployed on 0G Chain. Adds royalty splits and `recordInvocation(tokenId, sealedReceiptHash)` emitting on-chain events.
- **ENS wildcard resolver.** Orchestrator owns `<user>-era.eth`; subnames written automatically on persona mint. Text records: iNFT contract addr, token ID, 0G Storage URI for memory blob.
- **Persona container.** Existing era runner image extended with the brain SDK at `/opt/brain/`. Container loads persona configs from a volume-mounted manifest produced by the orchestrator at task start.

## The task lifecycle

1. I send `/task <repo> <desc>` to Telegram.
2. Orchestrator queues the task and snapshots the persona registry (which iNFTs the user owns + their config).
3. Orchestrator spawns a fresh runner container with the persona snapshot mounted.
4. Container runs `Brain.Run(ctx, input, [planner, coder, reviewer])`:
   - Planner sealed-inference call → receipt on 0G Log + iNFT `recordInvocation` event.
   - Coder reads plan from 0G Log, runs tool loop, commits; receipt(s) on 0G Log + invocation event.
   - Reviewer reads diff + plan + diff-scan output, produces decision; receipt on 0G Log + invocation event.
5. If reviewer approves → orchestrator pushes branch, opens PR, DMs branch + PR URL + 3 ENS subnames + 0G Log URI.
6. If reviewer flags → orchestrator DMs critique + Approve/Reject buttons. User decides; existing M3 flow.
7. Persona memory blobs on 0G KV are updated with what each persona learned this task.
8. End of day → orchestrator sends digest, now including total invocation events and total sealed-vs-unsealed ratio.

## What's out of scope (for now)

- Cross-node persona communication (Gensyn AXL). Personas talk through 0G Storage shared memory inside one orchestrator process.
- Persona marketplaces / iNFT discovery beyond a user's own owned set.
- Self-improving personas that rewrite their own system prompts. Personas mutate memory, not config.
- A web UI beyond Telegram. Demo video uses Telegram + 0G explorer + ENS resolver + GitHub PR + a CLI run of the `era-brain` example agent.
- Multi-user. Each orchestrator instance serves its allow-listed user.
- Mainnet. 0G testnet + ENS Sepolia (or 0G's L2 if subname support is live there) for the duration of the hackathon.

## Success criteria

Hackathon-flavored:

- Demo video shows: a real `/task` running, three sealed-inference receipts visible on 0G explorer, three ENS subnames resolving, an iNFT mint via `/persona-mint`, the `era-brain` example agent running standalone in a CLI.
- Submission qualifies for **0G Track 2 + 0G Track 1 + ENS Best Integration**.
- `era-brain` is `go get`-able from the public repo, has a working README, and the three examples build clean.
- Live deployment of orchestrator on the existing Hetzner VPS, talking to 0G testnet, surviving the demo window.
- Stretch: ENS Creative Use track via the activity feed page.

## Out-of-scope cuts list (in order, if we slip the 2-week budget)

1. ENS Creative track activity feed (already a stretch).
2. Third example agent (`chat-agent`); ship two examples.
3. `/persona-mint` user-defined personas; ship only the three defaults.
4. Per-persona model split; use single sealed model for all three.
5. ENS subnames entirely; keep Track 1 + Track 2 only.

Order: cut farthest from primary tracks first. Each cut is reversible post-hackathon.

## Non-goals that are tempting but we're saying no to

- Building a generic on-chain agent platform. era is still a coding agent for git workflows; the on-chain primitives serve that purpose.
- Forcing a Uniswap or KeeperHub integration to chase prizes. A weak forced integration looks worse than a strong focused one.
- Rewriting the existing iptables-locked sandbox, GitHub App auth, diff-scan, or audit log. They work; we layer on top.
- Mainnet deployment. Testnet only for the hackathon.
- Public marketplace for personas. Personal-tool framing holds; personas are tradeable in principle but no UI for it.
