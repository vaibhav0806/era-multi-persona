# Hackathon Submission Writeups

Three submissions, three angles. Same project, retargeted per track. Paste the relevant block into each platform's form. Update `<DEMO_VIDEO_URL>` after upload.

---

## 0G Track 2 — Best Autonomous Agents, Swarms & iNFT Innovations ($7.5k)

### Project name
era-multi-persona

### One-liner
A coding agent powered by a multi-persona swarm — every task fans out to a planner, coder, and reviewer, each minted as an ERC-7857 iNFT, each invocation logged on-chain.

### Description (paste verbatim)

era-multi-persona is a Telegram-driven coding agent that runs every task through a 3-persona swarm — planner, coder, reviewer — with **sealed inference receipts on 0G Compute**, **evolving per-persona memory on 0G Storage**, and **on-chain invocation events on a forked ERC-7857 iNFT contract** deployed to 0G Galileo testnet.

Every persona invocation produces three artifacts:

1. A `brain.Receipt` with sha256 input/output hashes, sealed flag, model name, timestamp.
2. An append-only entry on 0G Storage (`zg_log`) with the receipt + persona memory delta.
3. An on-chain `Invocation(tokenId, receiptHash, ts)` event from `EraPersonaINFT.recordInvocation` against the persona's iNFT token.

Custom personas can be minted live via Telegram: `/persona-mint <name> <prompt>` mints a new iNFT, uploads the prompt to 0G storage, registers an ENS subname, and inserts a SQLite row. Subsequent tasks can use the persona via `/task --persona=<name> ...`. The persona's iNFT token ID is what shows up in the on-chain Invocation events for tasks driven by that persona.

**iNFT contract on 0G Galileo:** `0x33847c5500C2443E2f3BBf547d9b069B334c3D16` ([chainscan](https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16))

**Verifiable on-chain proof:**
- 5+ minted persona tokens (planner=0, coder=1, reviewer=2 + 2 user-minted)
- 20+ `Invocation` events from real `/task` runs
- ~$0.05 ZG total spent across mints + invocations

### Tech stack

Go 1.25, Foundry (Solidity 0.8.24, OpenZeppelin v5.6.1), 0G Storage SDK, 0G Compute (sealed inference via `Zg-Res-Key` TEE attestation header), abigen v1.17.2, SQLite + goose migrations, Docker, Telegram Bot API, GitHub App.

### What makes this novel

- **Sealed inference + on-chain receipts.** Most agent demos are "an LLM in a loop." We're closer to "an LLM whose every output is a sealed-attested receipt with a content hash, written to decentralized storage and acknowledged on-chain."
- **Real product, not a toy.** era already shipped through M0–M6 (~6 weeks of pre-hackathon work) as a real personal coding agent opening real GitHub PRs. The fork (M7-A → M7-G) layered the swarm + 0G + iNFT + ENS on top.
- **Live persona minting.** Judges can `/persona-mint` a persona during the demo and use it in the next task — `Invocation` events fire against the new tokenID, not the default coder. The minting is on-stage on-chain.
- **TDD discipline throughout.** Every milestone has a brainstormed spec → reviewed plan → strict TDD execution → live testnet gate. 13 milestones, all tagged. Specs and plans committed in `docs/superpowers/`.

### Video demo
<DEMO_VIDEO_URL>

### Repo
https://github.com/vaibhav0806/era-multi-persona

### Twitter / X
[your handle]

---

## 0G Track 1 — Best Agent Framework, Tooling & Core Extensions ($7.5k)

### Project name
era-brain (the SDK powering era-multi-persona)

### One-liner
A standalone Go SDK for building multi-persona agents on 0G — five interfaces, plug-and-play providers for storage, compute, and on-chain identity.

### Description (paste verbatim)

`era-brain` is a `go get`-able Go module at `github.com/vaibhav0806/era-multi-persona/era-brain` that abstracts the 0G stack into five interfaces:

- **`Persona`** — anything that takes input, produces output + a receipt
- **`MemoryProvider`** — KV + append-log semantics; impls: SQLite, 0G `zg_kv`, 0G `zg_log`, and a write-both-read-cache-first `dual` wrapper
- **`LLMProvider`** — chat-completion abstraction; impls: OpenRouter (fallback) and 0G Compute (sealed inference, TEE-attested receipts)
- **`INFTRegistry`** — mint + recordInvocation; impl: `zg_7857` (Go client over abigen bindings for our forked ERC-7857 contract)
- **`IdentityResolver`** — ENS subname registration + text record read/write; impl: `identity/ens` (Sepolia NameWrapper + PublicResolver)

The orchestrator (era) imports the SDK. So can any other agent — the SDK is **product-independent**. We provide a working `examples/coding-agent/` that demonstrates the 3-persona flow against real OpenRouter + 0G testnet.

The SDK enforces strict separation between the *intent layer* (what each persona does, in their system prompt) and the *infrastructure layer* (how the call gets made, where memory lives, what gets logged on-chain). Swap any provider out — testing uses the same code path against `simulated.Backend` + in-memory KV.

**SDK README:** [era-brain/README.md](https://github.com/vaibhav0806/era-multi-persona/blob/master/era-brain/README.md) *(if not present, see the package godoc)*

### Tech stack

Go 1.25, go-ethereum v1.17.2 (`abigen`, `bind`, `ethclient`), simulated.Backend for unit tests, 0G Storage Go client, 0G Compute (HTTP + TEE attestation), Foundry-compiled mock contracts for ENS test fixtures.

### What makes this a real framework

- **Separate Go module.** Lives in its own `go.mod` inside the monorepo. Reusable as `go get github.com/vaibhav0806/era-multi-persona/era-brain`.
- **Five clean interfaces.** Each impl is ~100-300 lines. Each impl has unit tests + a build-tagged live test.
- **Test infrastructure included.** `simulated.Backend` for chain interactions, in-memory KV stubs for storage, mock contracts compiled in Foundry for ENS fixtures. Anyone forking the SDK can extend it without touching production-grade SDK plumbing.
- **Documented deviations from 0G upstream.** The SDK's package comments list 5+ places where the 0G Storage Go SDK differs from its docs (module path is `0gfoundation/0g-storage-client`, not `0glabs`; Batcher uses in-place mutation; etc). Future SDK consumers don't have to rediscover them.

### Video demo
<DEMO_VIDEO_URL>

### Repo
https://github.com/vaibhav0806/era-multi-persona

### Twitter / X
[your handle]

---

## ENS — Best Integration for AI Agents ($2.5k)

### Project name
era-multi-persona — per-persona ENS subnames

### One-liner
Every AI persona in our coding agent has its own ENS subname under `vaibhav-era.eth`, with text records pointing at the persona's iNFT token + 0G storage URI. Live-resolved at every task DM render.

### Description (paste verbatim)

In era-multi-persona, every persona — whether one of the 3 defaults (planner/coder/reviewer) or a user-minted custom persona — gets a Sepolia ENS subname under the parent `vaibhav-era.eth`:

- `planner.vaibhav-era.eth` → token #0
- `coder.vaibhav-era.eth` → token #1
- `reviewer.vaibhav-era.eth` → token #2
- `rustacean.vaibhav-era.eth` → token #4 (custom mint, "Rust-only" persona)
- `pythonic.vaibhav-era.eth` → token #5 (custom mint, "idiomatic Python" persona)

Each subname has 4 text records:
- `inft_addr` → the deployed iNFT contract address (`0x33847c...3D16` on 0G Galileo)
- `inft_token_id` → the persona's tokenID
- `zg_storage_uri` → 0G storage URI for the persona's system prompt blob
- `description` → first 60 chars of the prompt (for /personas listing)

**The integration is real, not decorative.** Every Telegram task DM ends with a `personas:` footer that performs **live ENS resolution at DM-render time** for the personas used in *that specific task*. If you `/task --persona=rustacean ...`, the footer resolves `rustacean.vaibhav-era.eth` and shows token #4, NOT the default coder. Edit a text record on Sepolia via `cast send` and the next task's DM reflects the change without restarting the orchestrator.

When a user mints a new persona via `/persona-mint <name> <prompt>`, the orchestrator dynamically registers `<name>.vaibhav-era.eth` via `NameWrapper.setSubnodeRecord` in the same transaction batch as the iNFT mint. ENS subname creation is a first-class action of `/persona-mint`.

**Parent name:** [vaibhav-era.eth](https://sepolia.app.ens.domains/vaibhav-era.eth) (registered + wrapped on Sepolia, owner `0x6DB1508Deeb45E0194d4716349622806672f6Ac2`)

### Verifiable on-chain proof

Type any of these into [sepolia.app.ens.domains](https://sepolia.app.ens.domains):
- planner.vaibhav-era.eth
- coder.vaibhav-era.eth
- reviewer.vaibhav-era.eth
- rustacean.vaibhav-era.eth
- pythonic.vaibhav-era.eth

Each shows the 4 text records. Cross-check via `cast call` on Sepolia PublicResolver `0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5`.

### Tech stack

Go 1.25, go-ethereum v1.17.2, abigen v1.17.2, NameWrapper at `0x0635513f179D50A207757E05759CbD106d7dFcE8`, PublicResolver at `0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5`, ENSIP-1 namehash implementation in Go (no third-party deps).

### Video demo
<DEMO_VIDEO_URL>

### Repo
https://github.com/vaibhav0806/era-multi-persona

### Twitter / X
[your handle]

---

## Submission tips

1. **Same demo video link** in all 3. Don't make 3 different videos.
2. **Customize the description.** Each track's writeup above is already retargeted — paste verbatim, change `<DEMO_VIDEO_URL>` and `<your handle>`.
3. **Test the on-chain links** before submitting. If chainscan-galileo is down, screenshot the explorer and paste it in the description as a fallback.
4. **Submit to the easiest one first.** Use the first submission as a dry run for content; tweak language for #2 and #3.
5. **Don't oversell.** Judges can smell hype. The on-chain proof is the moat — let it speak.
