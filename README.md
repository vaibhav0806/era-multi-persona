# era-multi-persona

> A multi-persona coding agent swarm running on the [0G](https://0g.ai) intelligent L1, with iNFT-minted personas (ERC-7857) addressable via [ENS](https://ens.domains) subnames.

Telegram in → multi-persona swarm runs on 0G Compute (sealed inference) → diffs land as a GitHub PR. Every persona invocation produces a sealed inference receipt, a 0G Storage append-log entry, and an on-chain iNFT `Invocation` event. New personas can be minted live via `/persona-mint <name> <prompt>`; once minted, they're usable in subsequent tasks via `/task --persona=<name> ...`.

**Built on top of [era](https://github.com/vaibhav0806/era)** — an existing personal Telegram-driven coding agent (M0–M6, ~6 weeks of pre-hackathon work). The fork (M7-A through M7-G) adds the multi-persona swarm + 0G + ENS layers.

---

## 🎯 Hackathon submissions (3 tracks, $17.5k target)

| Track | Prize | Status | Proof |
|---|---|---|---|
| **0G Best Autonomous Agents, Swarms & iNFT** | $7.5k | ✅ shipped | iNFT contract [`0x33847c...3D16`](https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16) on 0G Galileo, 5+ minted personas, `Invocation` events per task |
| **0G Best Agent Framework, Tooling & Core Extensions** | $7.5k | ✅ shipped | Standalone [`era-brain`](./era-brain/) Go SDK — `go get github.com/vaibhav0806/era-multi-persona/era-brain` |
| **ENS Best Integration for AI Agents** | $2.5k | ✅ shipped | Per-persona subnames on Sepolia: [planner](https://sepolia.app.ens.domains/planner.vaibhav-era.eth), [coder](https://sepolia.app.ens.domains/coder.vaibhav-era.eth), [reviewer](https://sepolia.app.ens.domains/reviewer.vaibhav-era.eth), [pythonic](https://sepolia.app.ens.domains/pythonic.vaibhav-era.eth) — live-resolved in every task DM |

---

## 🔍 Verify on-chain (judges, click here)

**0G Galileo testnet** (chainID 16602):
- iNFT contract: [`0x33847c5500C2443E2f3BBf547d9b069B334c3D16`](https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16)
- `Invocation(tokenId, receiptHash, ts)` events fire after every persona run — one event per persona per task
- Mint events fire from `/persona-mint`

**Sepolia ENS** (parent `vaibhav-era.eth`, owner `0x6DB1508Deeb45E0194d4716349622806672f6Ac2`):
- [planner.vaibhav-era.eth](https://sepolia.app.ens.domains/planner.vaibhav-era.eth) → token #0
- [coder.vaibhav-era.eth](https://sepolia.app.ens.domains/coder.vaibhav-era.eth) → token #1
- [reviewer.vaibhav-era.eth](https://sepolia.app.ens.domains/reviewer.vaibhav-era.eth) → token #2
- [rustacean.vaibhav-era.eth](https://sepolia.app.ens.domains/rustacean.vaibhav-era.eth) → token #4 (custom mint)
- [pythonic.vaibhav-era.eth](https://sepolia.app.ens.domains/pythonic.vaibhav-era.eth) → token #5 (custom mint)

Each subname has 4 text records: `inft_addr`, `inft_token_id`, `zg_storage_uri`, `description` — readable via `cast call $RESOLVER 'text(bytes32,string)(string)' $(cast namehash <name>.vaibhav-era.eth) <key>`.

**Sample PR opened by `/task --persona=pythonic`:** [pi-agent-sandbox#9](https://github.com/vaibhav0806/pi-agent-sandbox/pull/9).

---

## 🧠 Architecture

```
                    ┌────────────────┐
                    │   Telegram     │  /task --persona=rustacean ...
                    └───────┬────────┘
                            ▼
                    ┌────────────────┐
                    │  orchestrator  │  era root — queue, runner, swarm wrapper
                    └───┬────────┬───┘
                        │        │
              ┌─────────┘        └──────────┐
              ▼                             ▼
       ┌─────────────┐                ┌──────────┐
       │  era-brain  │                │ Pi-Docker│  coder slot — task-isolated container
       │     SDK     │                └──────────┘  (description prefix = persona prompt)
       └──┬─────┬─┬──┘
          │     │ │
          │     │ └──────────┐
          ▼     ▼            ▼
   ┌─────────┐ ┌────┐   ┌─────────┐
   │planner  │ │mem │   │reviewer │  brain.LLMPersona instances
   │(sealed) │ │evol│   │(sealed) │  → call 0G Compute (qwen-2.5-7b-instruct)
   └────┬────┘ └────┘   └────┬────┘  → produce brain.Receipt (sha256 of canonical fields)
        │      0G KV          │
        │   namespace=         │
        │   <persona-name>     │
        │                      │
        ▼                      ▼
  ┌─────────────────────────────────────┐
  │ 0G Storage  (zg_log audit + zg_kv mem) │  primary (canonical)
  │ SQLite      (dual provider cache)      │  cache (hot read)
  └─────────────────────────────────────┘
        │
        ▼
  ┌──────────────┐         ┌─────────────────┐
  │ EraPersonaINFT│ ─────▶  │  Invocation     │  on-chain receipt per persona run
  │ (0G Galileo)  │         │  events         │
  └──────────────┘         └─────────────────┘
        │
        ▼
  ┌────────────────────────────────────────┐
  │ ENS subname (Sepolia NameWrapper)      │  live-resolved in every task DM
  │   <persona>.vaibhav-era.eth            │  → token_id + iNFT addr + 0G URI
  └────────────────────────────────────────┘
```

### Three layers

1. **`era-brain` Go SDK** (separate `go.mod`, `go-get`-able). Defines `Persona`, `Brain`, `MemoryProvider`, `LLMProvider`, `INFTRegistry`, `IdentityResolver`. Concrete impls: SQLite + OpenRouter + 0G Storage (`zg_kv`/`zg_log`/`dual`) + 0G Compute (sealed inference) + ERC-7857 Go client (`zg_7857`) + ENS resolver (`identity/ens`). Reusable beyond era.
2. **era orchestrator** (this repo). Telegram bot, queue, Docker runner, GitHub App. Imports `era-brain`. Replaces M0–M6's monolithic Pi loop with `Brain.Run([planner, coder, reviewer])`.
3. **On-chain.** `EraPersonaINFT` (forked ERC-7857) on 0G Galileo. ENS subnames on Sepolia (parent name pre-registered + wrapped manually).

### Per-task flow

```
Telegram /task --persona=rustacean ...
  ↓
orchestrator queues + claims task
  ↓
era-brain.Brain.Run:
  planner   → 0G Compute (sealed) → brain.Receipt → 0G Storage append + iNFT.recordInvocation
  coder     → Pi-Docker (prompt prefix=rustacean's stored prompt) → diff
            → iNFT.recordInvocation against rustacean's tokenID
  reviewer  → 0G Compute (sealed) → brain.Receipt → 0G Storage append + iNFT.recordInvocation
  ↓
ENS lookup (Sepolia) for each persona's subname → live text records
  ↓
Telegram DM: PR URL + planner plan + reviewer decision + personas footer
              (live-resolved at DM-render time — judges can verify by clicking)
```

---

## 🎬 Quick demo (5 minutes for judges)

1. **Mint a custom persona live:**
   ```
   /persona-mint pythonic You write clean Pythonic code. Type hints everywhere, dataclasses over dicts, pathlib over os.path.
   ```
   Bot replies with: token #N, [chainscan link](https://chainscan-galileo.0g.ai/) (verify mint tx), [ENS link](https://sepolia.app.ens.domains/) (verify subname), 0G storage URI for the prompt blob.

2. **Use it in a task:**
   ```
   /task --persona=pythonic add a /healthz endpoint that returns 200 OK
   ```
   Watch orchestrator stdout for:
   - 4 0G Storage txs (planner audit + planner KV + reviewer audit + reviewer KV)
   - 3 0G iNFT `recordInvocation` txs (planner=token #0, pythonic=token #N, reviewer=token #2)
   - GitHub PR opens
   - Telegram DM ends with a `personas:` footer:
     ```
     personas:
       planner.vaibhav-era.eth → token #0 (0x33847c5500…)
       pythonic.vaibhav-era.eth → token #5 (0x33847c5500…)
       reviewer.vaibhav-era.eth → token #2 (0x33847c5500…)
     ```

3. **List minted personas:**
   ```
   /personas
   ```
   Returns the full list (defaults + custom).

The `personas:` DM footer values are **fetched from Sepolia at DM-render time** — not cached, not hardcoded. Edit a text record on Sepolia and the next task's DM reflects the change without restarting the orchestrator.

---

## 🛠 Tech stack

- **Go 1.25** (era + era-brain SDK)
- **0G Galileo testnet** — chain ID 16602, RPC `https://evmrpc-testnet.0g.ai`
- **0G Storage** — `zg_kv` (KV streams) + `zg_log` (sequence-numbered append-only log)
- **0G Compute** — sealed inference via TEE-signed responses (`Zg-Res-Key` header), model `qwen/qwen-2.5-7b-instruct`
- **Foundry** — `EraPersonaINFT` contract (ERC-721 + Ownable + Invocation event), OpenZeppelin v5.6.1, solc 0.8.24
- **abigen v1.17.2** — Go bindings for `EraPersonaINFT`, `MockNameWrapper`, `MockPublicResolver`
- **Sepolia ENS** — NameWrapper at `0x0635513f179D50A207757E05759CbD106d7dFcE8`, PublicResolver at `0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5`
- **SQLite + goose migrations** — local hot path; 0G is canonical
- **Telegram Bot API** — single-user gate (`PI_TELEGRAM_ALLOWED_USER_ID`)
- **GitHub App** — installation token source for branch push + PR open
- **Docker** — task-isolated container per `/task` (Pi-in-Docker as the coder slot)
- **OpenRouter** — fallback for sealed inference failures

---

## 🚀 Quickstart

### Prerequisites

- Go 1.25, Docker, jq, Foundry (`forge` + `cast`)
- Telegram bot token + your numeric user ID
- A throwaway sandbox GitHub repo + GitHub App with `Contents: Read/write` + `Metadata: Read-only` permissions
- OpenRouter API key (small balance for fallback)
- 0G Galileo testnet wallet with ≥ 0.05 ZG ([faucet](https://docs.0g.ai/developer-hub/testnet/testnet-overview))
- Sepolia ENS parent name registered + wrapped to the same wallet ([sepolia.app.ens.domains](https://sepolia.app.ens.domains))
- Sepolia wallet funded with ≥ 0.05 ETH ([Google Cloud faucet](https://cloud.google.com/application/web3/faucet/ethereum/sepolia))

### Setup

```bash
git clone git@github.com:vaibhav0806/era-multi-persona.git
cd era-multi-persona/era
cp .env.example .env
# Fill in the env vars. PI_ZG_*, PI_ENS_*, and PI_ZG_INFT_CONTRACT_ADDRESS unlock
# the hackathon features. PI_GITHUB_APP_*, PI_TELEGRAM_*, PI_OPENROUTER_API_KEY
# are required for the base /task functionality.

make abigen          # iNFT contract bindings
make abigen-ens      # ENS bindings (mock contracts → bindings)
make docker-runner   # Pi-in-Docker image
make build           # bin/orchestrator
./bin/orchestrator
```

Pre-flight check (verify the parent name is wrapped):
```bash
cast call 0x0635513f179D50A207757E05759CbD106d7dFcE8 \
  'ownerOf(uint256)(address)' \
  $(cast namehash $PI_ENS_PARENT_NAME) \
  --rpc-url $PI_ENS_RPC
```
Should return your signer address. `0x0...0` = name not wrapped — wrap via the ENS UI.

### Expected boot

```
goose: successfully migrated database to version: 12
INFO github app token source configured
INFO 0G storage wired indexer=https://indexer-storage-testnet-turbo.0g.ai
INFO 0G Compute sealed inference wired model=qwen/qwen-2.5-7b-instruct
INFO 0G iNFT registry wired contract=0x33847c5500C2443E2f3BBf547d9b069B334c3D16
INFO ENS resolver wired parent=vaibhav-era.eth
INFO personas reconciled
INFO orchestrator ready
```

---

## 🧪 Testing

```bash
make test                                     # full unit + integration suite (era root)
make test-race                                # with race detector
cd era-brain && go test -race ./...           # SDK tests

# live testnet tests (build-tagged, skip in CI)
set -a; source .env; set +a
go test -tags zg_live ./era-brain/inft/zg_7857/...        # iNFT mint + recordInvocation
go test -tags ens_live ./era-brain/identity/ens/...       # ENS subname write+read
go test -tags zg_live ./era-brain/memory/zg_kv/...        # 0G Storage KV
```

---

## 📜 Milestones (proof of work)

Each milestone is a tagged commit, with a spec at `docs/superpowers/specs/` and a plan at `docs/superpowers/plans/`. All shipped via brainstorm → spec → plan → execute, strict TDD.

### Hackathon fork (M7-*)

| Milestone | Tag | What |
|---|---|---|
| M7-A | `m7a-done` | `era-brain` SDK skeleton: 5 interfaces + Brain orchestrator + sqlite + openrouter |
| M7-A.5 | `m7a5-done` | Orchestrator-side swarm wrapper (planner-before, reviewer-after); CompletedArgs cascade; Telegram persona DM |
| M7-B.1 | `m7b1-done` | `zg_kv` + `zg_log` + `dual` 0G Storage providers; live testnet writes |
| M7-B.2 | `m7b2-done` | Production `/task` writes audit log to dual(sqlite, 0G); resilient on primary failures |
| M7-B.3 | `m7b3-done` | Persona evolving memory: `LLMPersona` reads prior memory before LLM call, prepends to prompt, writes updated memory |
| M7-C.1 | `m7c1-done` | `zg_compute` LLM provider with sealed-inference receipts via TEE-signed responses |
| M7-C.2 | `m7c2-done` | Orchestrator wires sealed inference; receipts flip `Sealed=true`; reviewer cross-checks |
| M7-D.1 | `m7d1-done` | `EraPersonaINFT` Foundry contract deployed to 0G Galileo; 3 default personas minted at deploy time |
| M7-D.2 | `m7d2-done` | Go iNFT client (`zg_7857`); `recordInvocation` per persona per task |
| M7-E | `m7e-done` | ENS subname registration + live-resolved DM footer; `tgNotifier.ensFooter` queries Sepolia at render time |
| M7-F | `m7f-done` | `/persona-mint <name> <prompt>` + `/task --persona=<name>` + `/personas` |
| M7-F.6 | `m7f6-done` | SQLite prompt cache + 0G fallback for `/task --persona=` (fixes 0G KV testnet flakiness) |
| M7-G | `m7g-done` | Polish: paginate Transfer scan, auto-backfill empty prompts, slog.Warn on parse failures |

### Pre-fork upstream era (M0-M6)

See [the upstream era repo](https://github.com/vaibhav0806/era). Briefly:

- **M0** — plumbing (SQLite, Telegram loop, Docker runner, dummy agent)
- **M1** — real agent (Pi + OpenRouter, budget caps)
- **M2** — security (network allowlist, secret proxy sidecar, diff-scan reward-hacking guards, GitHub App)
- **M3** — Telegram approval buttons + EOD digest
- **M3.5** — multi-repo per task
- **M4** — Hetzner VPS deploy + PR-per-task + mid-run cancel
- **M5** — CI + auto-deploy + offsite backups + pre-commit gate
- **M6** — agent sharpness: budget profiles, smarter egress, reply-to-continue, mid-run progress, `/ask`, `/stats`

---

## 🗺 Repo layout

```
cmd/orchestrator/          main entrypoint — wires queue, runner, swarm, all 0G/ENS providers
cmd/runner/, cmd/sidecar/  in-container Pi loop + secret-proxy sidecar
internal/queue/            task lifecycle + INFTProvider, ENSWriter, PersonaRegistry, PromptStorage seams
internal/swarm/            era-brain swarm wrapper (planner + reviewer brain personas; coder = Pi)
internal/telegram/         bot client + command handler (/task, /persona-mint, /personas, ...)
internal/db/               SQLite + sqlc; personas + tasks tables
internal/persona/          shared Persona type to break import cycles
internal/runner/           Docker wrapper + adapter to queue.Runner
internal/diffscan/         reward-hacking pattern detection
internal/digest/           EOD digest generator
internal/githubapp/        GitHub App installation token source

era-brain/                 standalone Go SDK (separate go.mod)
  brain/                   Brain orchestrator + Persona interface + LLMPersona
  memory/                  MemoryProvider impls: sqlite, zg_kv, zg_log, dual
  llm/                     LLMProvider impls: openrouter, zg_compute (sealed)
  inft/                    INFTRegistry interface + zg_7857 impl (ERC-721 client via abigen)
  identity/                Resolver interface + ens impl (NameWrapper + PublicResolver)
  storage/zg_storage/      0G prompt blob upload/fetch (M7-F)
  examples/coding-agent/   3-persona reference example

contracts/                 Foundry repo
  src/EraPersonaINFT.sol   forked ERC-7857 (mint, tokenURI, recordInvocation)
  test/                    forge tests + minimal mock ENS contracts for Go test fixtures

migrations/                goose SQL migrations (0001 → 0012)
docs/superpowers/specs/    brainstormed design docs per milestone
docs/superpowers/plans/    implementation plans per milestone
```

---

## 🔒 Security notes

Original M0 caveats (network allowlist, prompt injection guards, push credential blast radius) all apply. M2 hardened most of them; M5 added pre-commit test gating. The hackathon fork additionally introduces:

- **Hot wallet for 0G Galileo + Sepolia.** `PI_ZG_PRIVATE_KEY` is the same key for both chains. Lives in `.env`. Use a wallet with **only enough testnet ETH/ZG for gas** — never put real funds in it. Loss of the wallet = loss of mint capability for that orchestrator instance, NOT loss of past data (data on 0G Storage is content-addressed; iNFTs already minted are owned by their respective wallets and on-chain forever).
- **All on-chain writes are best-effort.** When 0G or Sepolia writes fail (testnet hiccups, RPC throttling, transaction reverts), `dual.Provider.onPrimaryError` / `slog.Warn` handles it — task completion never depends on chain liveness. The cache (SQLite) holds the canonical local record.
- **Sealed inference receipt forgery.** The receipt is only as trustworthy as 0G Compute's own attestation chain. The orchestrator runs the `LLMProvider` impl in the same process as the persona, so "the provider writes the receipt" is a code-boundary, not a trust-boundary, claim. Mitigation: the **reviewer cross-checks** the receipt hash against 0G Compute's published verifier. If 0G's attestation is broken, every team's submission has the same hole.
- **Single-user Telegram allowlist.** Bot silently drops messages from any user other than `PI_TELEGRAM_ALLOWED_USER_ID`. Necessary because `/persona-mint` costs real (testnet) ETH and `/task` costs real LLM tokens.

**Rule of thumb:** still a personal tool. Sandbox repo only.

---

## 🙏 Acknowledgements

- [0G Labs](https://0g.ai) for the intelligent L1 infrastructure (Storage, Compute, Chain)
- [ENS](https://ens.domains) for the canonical Web3 identity layer
- [OpenRouter](https://openrouter.ai) for the model fallback path
- [Foundry](https://book.getfoundry.sh) for Solidity tooling
- [go-ethereum](https://geth.ethereum.org) for `abigen` and the Ethereum client libraries
- [Pi](https://github.com/anthropics/pi-mcp-server) — the in-container coding agent that powers the coder slot

## License

MIT.
