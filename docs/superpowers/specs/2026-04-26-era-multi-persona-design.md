# era-multi-persona — design spec

> Status: brainstormed and approved 2026-04-26. Implementation plans live in `docs/superpowers/plans/`.
> Companion vision doc: top-level `FEATURE.md`.

## §1 — Vision & scope

`era-multi-persona` is the 0G hackathon fork of [era](https://github.com/vaibhav0806/era). Same product spine (Telegram-driven coding agent, disposable Docker, push branch + open PR), reframed as a multi-persona swarm: every task runs through three sealed-inference personas — **planner → coder → reviewer** — on 0G Compute, with persona memory + audit log on 0G Storage, and personas minted as iNFTs (ERC-7857) addressable via ENS subnames.

Hackathon submission targets:

- **0G Track 2** — Best Autonomous Agents, Swarms & iNFT Innovations ($7.5k) — primary.
- **0G Track 1** — Best Agent Framework, Tooling & Core Extensions ($7.5k) — via standalone `era-brain` Go SDK + reference example agents.
- **ENS Best Integration for AI Agents** ($2.5k) — per-persona subnames doing real resolution work.
- **ENS Most Creative Use** ($2.5k) — stretch only, via activity feed page.

**Skip:** Uniswap, KeeperHub, Gensyn AXL. Wrong primitive — era is a coding agent, not an on-chain value mover.

Total target pool: ~$12.5k base, ~$15k with stretch. **2-week build budget.**

## §2 — Architecture

Three layers:

**Layer 1 — `era-brain` SDK (separate repo).** Pure Go module. Defines:

- `Persona` interface — `Run(ctx, input) (output, receipt)`
- `Brain` — orchestrates a chain of personas, threads memory between them
- `MemoryProvider` interface — impls: `sqlite`, `zg_kv`, `zg_log`
- `LLMProvider` interface — impls: `zg_compute` (sealed), `openrouter`
- `INFTRegistry` interface — impl: `zg_7857` (forked ERC-7857)
- `IdentityResolver` interface — impl: `ens`

Examples: `coding-agent/` (era integration), `audit-agent/` (smart-contract review), `chat-agent/` (minimal "hello brain").

**Layer 2 — era orchestrator (this repo).** Existing Go binary. Imports `era-brain`. Replaces internal monolithic Pi loop with `Brain.Run([planner, coder, reviewer])`. Telegram bot, queue, Docker spawn, GitHub App, diff-scan, deploy scripts — unchanged from M6.

**Layer 3 — On-chain.** Forked ERC-7857 contract on 0G Chain testnet. ENS subnames on Sepolia (or 0G's L2 if subname support is live) pointing at iNFT contract addr + 0G Storage URI for persona memory blob.

### Data flow per task

```
Telegram /task → orchestrator queues → spawns container with persona snapshot
  → container clones repo
  → Brain.Run(ctx, input, [planner, coder, reviewer]):
      planner (sealed GLM-5-FP8) → plan + receipt₁ → 0G Log + iNFT.recordInvocation
      coder   (sealed qwen3.6+)  → diff  + receipt₂ → 0G Log + iNFT.recordInvocation
      reviewer(sealed qwen3.6+)  → decision + receipt₃ → 0G Log + iNFT.recordInvocation
  → orchestrator: push branch + open PR + DM(branch, PR, ENS subnames, 0G Log URI)
```

Each persona invocation = one event-row in SQLite (hot index) + one append in 0G Log (canonical) + one on-chain `Invocation` event.

## §3 — Components (detail)

### `era-brain` SDK package layout

```
era-brain/
├── brain/         # Brain + Persona orchestration
├── persona/       # Persona impl helpers (system prompt + tool loop)
├── memory/
│   ├── sqlite/    # local cache
│   ├── zg_kv/     # mutable persona memory
│   └── zg_log/    # append-only audit
├── llm/
│   ├── zg_compute/  # 0G sealed inference
│   └── openrouter/  # existing era path
├── inft/
│   └── zg_7857/   # ERC-7857 contract client
├── identity/
│   └── ens/       # ENS resolver + subname writer
└── examples/
    ├── coding-agent/
    ├── audit-agent/
    └── chat-agent/
```

### era orchestrator changes

- New `internal/swarm/` — wraps `brain.Brain` w/ era-specific glue: persona system-prompt loaders, diff-scan invocation between coder→reviewer, Telegram progress DMs reflecting current persona.
- Existing `internal/runner/` Docker spawn — unchanged shape; container now hosts the swarm instead of single Pi process.
- New `internal/persona/` — reads persona registry: which iNFT token IDs are owned by user, which ENS subnames resolve, which system prompts load. Cached, refreshed per task.
- New migrations:
  - `0009_personas.sql` — `persona_id, inft_token_id, ens_name, system_prompt_uri, default_model, created_at`
  - `0010_inference_receipts.sql` — `task_id, persona_id, receipt_hash, model, sealed (bool), zg_log_uri, ts`
- New Telegram commands: `/personas`, `/persona-mint <name> <prompt>`, `/task --pipeline=<...>`.

### On-chain

- `contracts/` directory — Foundry repo. Forked ERC-7857 with:
  - Per-token metadata URI pointing to 0G Storage system-prompt blob.
  - Royalty splits: creator cut + holder cut; configurable per token.
  - `recordInvocation(tokenId, sealedReceiptHash)` emitting `Invocation(tokenId, receiptHash, ts)` event.
- ENS: parent name `<user>-era.eth` is **pre-registered manually by the user** before the demo (one-time, ~5 min in the ENS app). Orchestrator owns the wildcard resolver contract; on persona mint writes subname text records: `inft_addr`, `inft_token_id`, `zg_storage_uri`. Orchestrator does not register the parent name itself — keeps M7-E scoped to resolver writes only.

## §4 — Error handling, testing, security

### Error handling

- **0G Compute sealed call fails** → fall back to OpenRouter for that persona. Receipt marked `sealed=false` in DB + log. Reviewer sees the unsealed flag in critique input ("coder ran unsealed; verify carefully"). Demo-time: still useful, doesn't block task.
- **0G Storage write fails** → SQLite remains authoritative for hot path. Task continues. Background retry queue flushes pending writes; surface failures in `/stats`.
- **iNFT `recordInvocation` tx fails** → task continues. Log to events table with `inft_record_failed`. Background reconciler retries. We don't gate task completion on chain settlement.
- **Persona disagreement (reviewer flags)** → existing era flow: Telegram DM with Approve/Reject buttons (M3).
- **Container OOM / network drop** → existing M4 startup-reconcile sweeps orphans to `failed`.

### Testing strategy (TDD per project philosophy)

- **Unit tests** — table-driven for each `era-brain` interface impl. Mock `LLMProvider` for brain orchestration tests so CI doesn't hit real 0G.
- **Integration tests** — SQLite + 0G testnet. Each phase ends with `go test -race ./...` green.
- **Live gate per phase** — real Telegram task hitting real 0G testnet. Smoke scripts in `scripts/smoke/phase_*.sh` extending the existing pattern.
- **Solidity tests** — Foundry. `forge test` in CI.
- **E2E (build tag)** — `internal/e2e/` extends to verify full task → 3 receipts on 0G Log → ENS resolves → PR opens.

### Security

Original era's three threats (prompt injection, reward hacking, push-credential blast radius) still apply. Two additions:

- **Sealed-inference receipt forgery.** The receipt is only as trustworthy as 0G Compute's own attestation chain — the `LLMProvider` impl runs in the same container as the persona, so "the provider writes the receipt" is a code-boundary, not a trust-boundary, claim. The actual mitigation is the **reviewer cross-check**: reviewer persona fetches receipts from 0G Log (not from coder's claim) and re-verifies the attestation hash against 0G Compute's published verifier. We do not invent a forgery defense beyond what 0G Compute provides; if 0G's attestation is broken, every team's submission has the same hole.
- **Hot-wallet compromise.** Orchestrator holds a hot wallet for minting + recording. Mitigation: small balance only (gas-budget), funded just-in-time. Loss of wallet = loss of mint capability for that orchestrator instance, not loss of past data.

Reward-hacking guard now stacks: existing diff-scan rules + sealed-inference receipt for the coder + reviewer persona's critique + reviewer's own sealed receipt.

iptables egress allowlist additions: 0G testnet RPC, 0G Storage gateway, 0G Compute endpoint, ENS gateway.

## §5 — Milestones + time budget

Per the project process philosophy: each milestone splits into ~5-7 phases; each phase ends with `go test -race ./...` green + replay every prior phase smoke + live Telegram gate + tagged commit. TDD throughout: failing test first, verify fail, implement minimal, verify pass, commit.

### M7-A — `era-brain` skeleton (~2 days)

New repo `era-brain`. Interfaces (`Persona`, `Brain`, `MemoryProvider`, `LLMProvider`, `INFTRegistry`, `IdentityResolver`). SQLite + OpenRouter impls (existing era stack ported behind new boundary). era orchestrator imports brain, replaces monolithic Pi loop with `Brain.Run([planner, coder, reviewer])` using OpenRouter + SQLite still — no 0G yet.

**Live gate:** real `/task` lands a PR via the new abstraction. Validates: refactor didn't break existing product.

### M7-B — 0G Storage integration (~3 days)

`zg_kv` + `zg_log` memory providers in `era-brain/memory/`. Per-persona memory writes per task. Audit log writes per receipt. Migration `0010_inference_receipts`.

**Live gate:** `/task` → 0G testnet log shows three append events; persona memory blob retrievable via 0G Storage gateway URL.

### M7-C — 0G Compute sealed inference (~2 days)

`zg_compute` LLM provider with sealed-inference receipts. Per-persona model config (planner=GLM-5-FP8, coder+reviewer=qwen3.6-plus). Fallback to OpenRouter on sealed failure; receipt marked `sealed=false`.

**Live gate:** `/task` with all-0G provider config; verify three receipt hashes recorded; force a 0G failure, verify fallback fires and reviewer sees unsealed flag.

### M7-D — iNFT contract + minting (~3 days)

Fork 0G ERC-7857 template. Add royalty splits + `recordInvocation`. Foundry tests. Deploy to 0G testnet. Go client in `era-brain/inft/zg_7857/`. Orchestrator mints three defaults on first startup; mint is **idempotent** — checks the iNFT contract for existing `(creator, name)` tuples before submitting tx, so repeated dev runs do not produce duplicate token IDs. `/persona-mint` Telegram command for custom personas (also idempotent on `(creator, name)`). Hook `recordInvocation` into brain post-call.

**Live gate:** `/task` triggers three on-chain events visible on 0G explorer; mint a custom persona; use it in next `/task`.

### M7-E — ENS subnames (~2 days)

Orchestrator owns wildcard resolver for `<user>-era.eth`. On persona mint → write subname text records (iNFT addr, token ID, 0G Storage URI). On task complete → reviewer's DM includes resolved ENS names.

**Live gate:** `dig` / ENS app shows resolved subnames; clicking through reaches iNFT contract + memory blob.

### M7-F — Polish, examples, demo (~2-3 days)

`era-brain/examples/audit-agent/` + `chat-agent/`. README arch diagram. 3-min demo video. Submission form.

**Live gate:** end-to-end demo dry-run.

### Sum: 14-17 days

2 weeks = 14 days. Tight. Cuts in order if slipping:

1. ENS Creative track activity feed (already stretch).
2. `chat-agent/` example (ship two examples not three).
3. `/persona-mint` user-defined personas (ship only three defaults; iNFT story still works).
4. Per-persona model split (single sealed model for all three).
5. ENS subnames entirely (keep Track 1 + Track 2 only).

Order: cut farthest from primary tracks first.

## §6 — Decisions log (Q&A from brainstorming)

| Q | Choice | Rationale |
|---|---|---|
| Q1 — Multi-persona shape | D: internal swarm + iNFT-minted personas, no AXL | 2-week budget; AXL adds cross-node complexity for marginal prize fit |
| Q2 — Persona scope | C: ship 3 archetypes minted as defaults + allow user-mintable custom | Day-1 working iNFT demo + extends to "dynamic upgrades, royalty splits" prize criterion |
| Q3 — Demo task domain | A: keep era's coding-agent flow | Reuses 6 milestones of existing infra; sealed-inference + iNFT layered on real product, not toy |
| Q4 — Memory architecture | B: 0G Log audit + 0G KV persona memory | Hits "evolving persistent memory" criterion verbatim; SQLite stays authoritative for hot path |
| Q5 — Track 1 framework framing | C: full SDK in separate repo with examples | Real `go get`-able framework for Track 1 prize; same build qualifies for both 0G tracks |
| Q6 — iNFT contract | B: fork + customize 0G template (royalty splits + invocation receipts) | Matches "ownership, composability, monetization" wording from prize; ~2 days well spent |
| Q7 — ENS scope | A: per-persona subname only | Functional ENS work hits Best Integration prize cleanly; activity feed is stretch |
| Q8 — Default personas | A: planner / coder / reviewer | Classic split; build chaining mechanism so user-defined pipelines are small follow-on |
| Q9 — Sealed inference model | C: provider-agnostic LLMProvider; ship 0G Compute + OpenRouter impls | Strongest framework story; per-persona model defaults (planner=GLM-5-FP8, coder+reviewer=qwen3.6-plus) |
| Q10 — Demo medium | A+C: Telegram screen-recording + last 30s of CLI showing `era-brain` example | Serves both tracks in one 3-min video; no dashboard-build distraction |

## §7 — Out of scope

- Cross-node persona communication (Gensyn AXL).
- Persona marketplaces / iNFT discovery beyond a user's own owned set.
- Self-improving personas that rewrite their own system prompts.
- Web UI beyond Telegram (demo uses 0G explorer + ENS resolver + GitHub PR + CLI).
- Multi-user. Single allow-listed user per orchestrator instance.
- Mainnet. Testnet only for the hackathon.
