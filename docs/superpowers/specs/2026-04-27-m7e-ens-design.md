# M7-E — ENS Subname Resolver Integration Design

**Status:** approved 2026-04-27.
**Parent spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` §M7-E.
**Hackathon prize target:** ENS Best Integration for AI Agents ($2.5k). ENS Most Creative track tracked as stretch only (out of scope for M7-E.1; revisit if budget permits).

## §1 — Goal

Each of the 3 personas minted in M7-D.1 (planner=tokenID 0, coder=1, reviewer=2 on `EraPersonaINFT` at `0x33847c5500C2443E2f3BBf547d9b069B334c3D16`) gets a Sepolia ENS subname under `vaibhav-era.eth`:

- `planner.vaibhav-era.eth`
- `coder.vaibhav-era.eth`
- `reviewer.vaibhav-era.eth`

Each subname has 3 text records: `inft_addr`, `inft_token_id`, `zg_storage_uri`.

**Real resolution work** (the prize criterion): on every `/task` completion, era's reviewer DM includes a "personas:" footer that performs **live ENS resolution at DM-render time** — proves judges can verify the integration by typing the name into sepolia.app.ens.domains.

Time budget: ~2 days. Two phases anticipated (see §7).

## §2 — Architecture

Three components, all on **Sepolia testnet** (separate from 0G Galileo where the iNFT contract lives):

- **`era-brain/identity/ens/`** — Go client wrapping abigen bindings for ENS NameWrapper + PublicResolver. Provides `Provider` struct satisfying the existing `identity.Resolver` interface stub from M7-A.2. Hand-rolled abigen (no third-party deps), mirrors M7-D.2's `zg_7857` package shape.
- **Orchestrator wiring** (`cmd/orchestrator/main.go`) — env-conditional `ensEnabled()` helper. On startup, if enabled, the orchestrator syncs the 3 subnames idempotently — each `SetTextRecord` reads the on-chain value first and skips the tx when it already matches, so partial-write states reconcile on the next boot.
- **Queue / Telegram glue** (`internal/queue/queue.go`) — at task complete, the queue's reviewer-DM path reads the 3 subnames' text records via Sepolia RPC and appends a "personas:" section. Read failures log a warning and skip the footer; the DM still sends.

Networks are fully decoupled: Sepolia chain ID 11155111, RPC URL via `PI_ENS_RPC` env var; 0G Galileo (chain ID 16602, `PI_ZG_EVM_RPC`) untouched. The signing wallet is the **same private key** (`PI_ZG_PRIVATE_KEY`) used for iNFT — funded on both chains.

### Sepolia contract addresses (constants in `ens.go`)

- NameWrapper: `0x0635513f179D50A207757E05759CbD106d7dFcE8`
- PublicResolver: `0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5`
- ENS Registry: `0x00000000000C2E074eC69A0dFb2997BA6C7d2e1e` (used for namehash sanity checks)

### Pre-flight requirement (one-time, manual)

The parent name `vaibhav-era.eth` must be:
1. **Registered** on Sepolia ENS to the signer wallet (the address derived from `PI_ZG_PRIVATE_KEY`).
2. **Wrapped** via NameWrapper before subname creation works (the ENS app does this on registration by default; legacy unwrapped names need a one-click "Wrap" step in the UI).

Pre-flight check before running orchestrator:
```bash
cast call 0x0635513f179D50A207757E05759CbD106d7dFcE8 \
  'ownerOf(uint256)(address)' \
  $(cast namehash vaibhav-era.eth) \
  --rpc-url $PI_ENS_RPC
```
Must return the signer address. `0x0...0` means name not wrapped — wrap via ENS UI before proceeding.

## §3 — Components (detail)

### `era-brain/identity/ens/`

```
era-brain/identity/ens/
├── ens.go                          # Provider impl
├── ens_test.go                     # unit tests via simulated.Backend
├── ens_live_test.go                # //go:build ens_live
└── bindings/
    ├── name_wrapper.go             # abigen output (NameWrapper ABI + bin)
    └── public_resolver.go          # abigen output (PublicResolver ABI + bin)
```

`Provider` API:

```go
type Config struct {
    ParentName string // e.g. "vaibhav-era.eth"
    RPCURL     string // Sepolia RPC URL
    PrivateKey string // hex, with or without 0x prefix
    ChainID    int64  // 11155111 (Sepolia)
}

type Provider struct { /* client, nameWrapper, resolver, auth, parentNode */ }

func New(cfg Config) (*Provider, error)
func (p *Provider) Close()
func (p *Provider) ParentName() string

// Idempotent: reads owner of subnode; if already owned by us with the
// PublicResolver set, returns nil. Otherwise calls
// NameWrapper.setSubnodeRecord(parentNode, label, owner=us,
// resolver=PublicResolver, ttl=0, fuses=0, expiry=type(uint64).max).
//
// expiry MUST be non-zero on Sepolia NameWrapper — passing 0 reverts when
// the parent has any fuses burned. We pass max-uint64 so the subname never
// expires before the parent's own expiry (NameWrapper clamps to parent's
// expiry internally).
func (p *Provider) EnsureSubname(ctx context.Context, label string) error

// SetTextRecord overwrites. Reads first; returns nil if the on-chain value
// already equals the new value.
func (p *Provider) SetTextRecord(ctx context.Context, label, key, value string) error

// ReadTextRecord returns "" (with nil error) when the key is unset.
// Returns error only on RPC/ABI failure.
func (p *Provider) ReadTextRecord(ctx context.Context, label, key string) (string, error)
```

`Provider` satisfies `era-brain/identity.Resolver` (interface stub from M7-A.2).

### Orchestrator changes (`cmd/orchestrator/main.go`)

```go
func ensEnabled() bool {
    return os.Getenv("PI_ENS_RPC") != "" &&
           os.Getenv("PI_ENS_PARENT_NAME") != "" &&
           os.Getenv("PI_ZG_PRIVATE_KEY") != ""
}
```

After the existing `zgINFTEnabled()` block and before `q.Reconcile(ctx)`:

```go
if ensEnabled() {
    ensProv, err := ens.New(ens.Config{
        ParentName: os.Getenv("PI_ENS_PARENT_NAME"),
        RPCURL:     os.Getenv("PI_ENS_RPC"),
        PrivateKey: os.Getenv("PI_ZG_PRIVATE_KEY"),
        ChainID:    11155111,
    })
    if err != nil {
        // Sepolia RPC flakes more often than 0G Galileo and ENS is decorative.
        // Log + skip rather than aborting boot. Behaves identically to the
        // ens-env-vars-absent path.
        slog.Error("ens disabled — boot continues without ENS", "err", err)
        ensProv = nil
    }
    if ensProv != nil { defer ensProv.Close() }

    if ensProv != nil {
        inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
        for _, p := range []struct{ label, tokenID, zgURI string }{
            {"planner",  "0", planerURI},
            {"coder",    "1", coderURI},
            {"reviewer", "2", reviewerURI},
        } {
            if err := syncPersonaENS(ctx, ensProv, p.label, p.tokenID, inftAddr, p.zgURI); err != nil {
                slog.Warn("ens sync failed", "label", p.label, "err", err)
            }
        }
        q.SetENS(ensProv)
        slog.Info("ENS resolver wired", "parent", os.Getenv("PI_ENS_PARENT_NAME"))
    }
}
```

`syncPersonaENS` calls in order: `EnsureSubname(label)` → `SetTextRecord(label, "inft_addr", inftAddr)` → `SetTextRecord(label, "inft_token_id", tokenID)` → `SetTextRecord(label, "zg_storage_uri", zgStorageURI)`. **No outer "skip if already populated" early-return** — `SetTextRecord` is itself idempotent (per-key read-before-write skips the on-chain tx when the value already matches), so partial-write states (e.g., subname exists but `zg_storage_uri` was never written) reconcile on the next boot. 3 extra read calls per persona is free.

The 3 `zg_storage_uri` values (`planerURI`, `coderURI`, `reviewerURI` in the snippet above) are **hardcoded constants** in `main.go` matching the raw GitHub URLs committed in `contracts/metadata/*.json` from M7-D.1. They are static, known at deploy time, in-repo — no runtime call to `iNFT.tokenURI()` needed. If the metadata files are ever moved/renamed, update the constants.

Per-label sync failures are non-fatal — log and continue to next label. Boot-time `ens.New()` failure is **also non-fatal** — log and continue without ENS (same effect as ENS env vars being absent). ENS is a decorative footer; Sepolia RPC flakiness should not crash the orchestrator.

### Queue changes (`internal/queue/queue.go`)

New `ENSResolver` interface (the queue's view, mirrors the `INFTProvider` seam pattern from M7-D.2.2):

```go
type ENSResolver interface {
    ReadTextRecord(ctx context.Context, label, key string) (string, error)
    ParentName() string
}

func (q *Queue) SetENS(r ENSResolver) { q.ens = r }
```

In the existing reviewer-DM rendering path, after computing the existing DM body, if `q.ens != nil`:

```
build "personas:" footer:
  for each label in {planner, coder, reviewer}:
    addr,    _ := q.ens.ReadTextRecord(ctx, label, "inft_addr")
    tokenID, _ := q.ens.ReadTextRecord(ctx, label, "inft_token_id")
    line = fmt.Sprintf("  %s.%s → token #%s (%s)", label, q.ens.ParentName(), tokenID, addr)
  if any line errored: skip ENS footer entirely
  else: append "\n\npersonas:\n" + lines to DM body
```

Read failures from any single label cause the **entire footer** to be skipped (avoids partial/confusing DMs). Task completion + DM send always proceed.

### What stays untouched

- `era-brain/inft/zg_7857/` — done in M7-D.2
- iNFT contract on 0G Galileo — read-only from ENS's perspective
- Telegram approve/reject buttons, queue cascade, swarm pipeline
- Pre-existing migrations
- `.env` semantics for 0G Storage / Compute / iNFT envs

## §4 — Data flow

### Orchestrator boot (first time per fresh `vaibhav-era.eth`)

```
ensEnabled() → ens.New() → for each {planner=0, coder=1, reviewer=2}:
  syncPersonaENS (no outer skip; per-key idempotency only):
    EnsureSubname(label)
      ↳ owner+resolver match? skip
      ↳ else NameWrapper.setSubnodeRecord(parentNode, label, owner=us,
         resolver=PublicResolver, ttl=0, fuses=0, expiry=type(uint64).max)
    SetTextRecord(label, "inft_addr",      "0x33847c...")
      ↳ resolver.text(node, "inft_addr") matches? skip
      ↳ else PublicResolver.setText(...)
    SetTextRecord(label, "inft_token_id",  "0"|"1"|"2")  // same idempotent shape
    SetTextRecord(label, "zg_storage_uri", <hardcoded raw GitHub URL>)  // same
```

Total cost on **first** boot: ~0.0015 Sepolia ETH (3 setSubnodeRecord) + ~0.001 ETH (9 setText if individual; less if multicalled). Subsequent boots: ~12 RPC reads (1 per `EnsureSubname` + 3 per `SetTextRecord` × 3 personas), 0 txs.

### Per task completion (read path)

```
queue.RunNext finishes → reviewer DM body composed → if q.ens != nil:
  for each label:
    ReadTextRecord(label, "inft_addr")
    ReadTextRecord(label, "inft_token_id")
  format "personas:" footer (Sepolia RPC reads, gas-free)
  append to DM body
Telegram DM send (always proceeds; ENS read errors skip footer only)
```

Cost per task: 6 free RPC calls. ~50-200 ms latency added to DM render path.

## §5 — Error handling

| Failure | Behavior |
|---|---|
| ENS env vars absent | orchestrator runs without ENS (M7-D.2 baseline preserved) |
| `ens.New()` fails (RPC unreachable, bad key) | log error, set `ensProv = nil`, boot continues without ENS (decorative feature; Sepolia RPC is flakier than 0G) |
| `EnsureSubname` reverts (e.g., parent not owned by signer) | log error per-label; continue to next persona; don't fail boot |
| `SetTextRecord` reverts | log warn; subname may be partially populated; reconciles next boot |
| `ReadTextRecord` fails at DM-render time (any label) | log warn, skip ENS footer entirely, task DM completes normally |
| Wallet out of Sepolia ETH | per-label warn; reads still work for already-written labels |
| Sepolia RPC partial outage during DM read | skip footer, task succeeds |

**Critical invariant:** task completion (PR creation, status update, primary DM send) **never** depends on Sepolia liveness. ENS is decorative metadata, not on-task path.

## §6 — Security

The ENS hot wallet is the same as the iNFT hot wallet (per Q3 decision). Threat model unchanged from M7-D.2:

- Loss of `PI_ZG_PRIVATE_KEY` = loss of mint capability + ENS write capability for that orchestrator instance. Past data (existing token URIs, existing ENS records) survives — they're on-chain and only setText/setSubnodeRecord can overwrite.
- Sepolia faucet drips small amounts (~0.05 ETH/day from Google Cloud); even worst-case wallet drain is bounded by faucet supply.
- ENS subnames cannot be used to sign on-chain actions — they're text records, not keys. Maliciously rewriting them only confuses the DM footer; iNFT contract on 0G is the source of truth for token ownership.

iptables egress allowlist additions (mirrors M7-B/C/D pattern): Sepolia RPC URL.

## §7 — Implementation phases (~2 days)

Following the project's phased + tagged commit pattern. Each phase ends with `go test -race ./...` green for both modules and a tagged commit.

### M7-E.1 — `ens.Provider` impl (~1 day)

- Phase 0: abigen NameWrapper + PublicResolver. Extend Makefile `abigen` target. Commit + tag `m7e-0-bindings`.
- Phase 1: `ens.Provider` struct + `New` + `Close` + 4 methods. Unit tests via simulated.Backend using **a minimal `MockNameWrapper.sol` + `MockResolver.sol`** (~30 LoC each, implementing only `setSubnodeRecord` / `ownerOf` / `setText` / `text`) committed under `contracts/test/`. Real ENS contracts are ~2k LoC with deep inheritance — deploying them in-fixture is half a day of yak-shaving for no extra signal beyond what the live test gives us. Build-tagged live test against real Sepolia validates the actual contract. Commit + tag `m7e-1-provider`.

### M7-E.2 — Orchestrator + queue wiring + live gate (~1 day)

- Phase 2: queue `ENSResolver` interface + `Queue.SetENS` + DM footer rendering. TDD with stubENS mirroring stubINFT pattern. Commit + tag `m7e-2-queue-wiring`.
- Phase 3: orchestrator `ensEnabled()` + `syncPersonaENS` + boot-time wiring. Compile + regression tests. Commit + tag `m7e-3-orchestrator-wiring`.
- Phase 4: live Telegram `/task` gate. Verify Sepolia subnames exist via sepolia.app.ens.domains. Verify DM footer shows resolved data. Tag `m7e-done`.

## §8 — Out of scope (deferred / cuts)

- **ENS Most Creative track web page.** Track separately; if budget permits after M7-E.2, add a static HTML page subscribing to PublicResolver `TextChanged` events on the parent node. Spec unchanged here.
- **`/personas` Telegram command.** Defer; M7-F if pursued.
- **`/persona-mint` writing ENS subnames at mint time.** `/persona-mint` itself was deferred in M7-D.2 and stays deferred.
- **Reverse resolution (address → ENS name).** Not needed for the prize criterion.
- **Custom wildcard resolver contract.** The original master spec mentioned wildcards; explicit subname registration via NameWrapper.setSubnodeRecord is the simpler path chosen here. Wildcards revisited only if persona count grows beyond 3.
- **Mainnet ENS.** Sepolia only.
- **Audit-log event kinds for ENS write/read failures.** Cuts-list candidate; relies on slog for now.

## §9 — Live gate acceptance criteria (M7-E done)

1. `go build ./...` green; `go test -race -count=1 ./...` green for both modules.
2. Real Telegram `/task` boot logs include `INFO ENS resolver wired parent=vaibhav-era.eth`.
3. The 3 subnames are visible on https://sepolia.app.ens.domains/vaibhav-era.eth, each with 3 text records (`inft_addr`, `inft_token_id`, `zg_storage_uri`).
4. Reviewer Telegram DM contains a `personas:` footer with all 3 labels resolved correctly.
4a. The `personas:` footer values come from a fresh on-chain read at DM-render time — verifiable by `slog.Debug` lines showing 6 Sepolia `eth_call` requests per task in DM render path (or by editing a text record on Sepolia and seeing the next task's DM reflect the change without restart).
5. Without ENS env vars, orchestrator falls back cleanly — M7-D.2 baseline DM unchanged.
6. Repeated boots produce 0 Sepolia txs (idempotent at per-key granularity, not just outer-skip).

## §10 — Risks + cuts list (in order if slipping)

1. **Sepolia ENS NameWrapper has unexpected fuses/permission semantics that block subname registration.** Recovery: fall back to legacy `ENS.setSubnodeOwner` + `ENS.setResolver` two-tx path. Adds ~0.001 ETH/sub. Documented in plan reviewer notes if encountered.
2. **abigen fails on NameWrapper's complex ABI** (it has lots of structs). Recovery: pin abigen to v1.17.2 exactly; if still fails, hand-write minimal ABI JSON containing only the methods we use (`setSubnodeRecord`, `ownerOf`).
3. **Live test gas budget exceeded** because of ENS contract overhead. Recovery: faucet up; faucet drips 0.05 ETH/day, far above any single test cost.
4. **Idempotency check race** — boot N writes records, boot N+1 reads them but RPC returns stale. Recovery: poll RPC for confirmation after boot N's writes (1-block wait); not needed in practice given 12s Sepolia block time + boot-not-immediately-followed-by-restart pattern.
5. **DM rendering becomes too slow** if Sepolia RPC is laggy. Recovery: 5s timeout on each `ReadTextRecord` call in DM path; on timeout, skip footer.
6. **Parent name not actually owned/wrapped by signer wallet.** #1 way the live gate fails. Pre-flight check (mandatory before live gate): `cast call $NAMEWRAPPER 'ownerOf(uint256)(address)' $(cast namehash vaibhav-era.eth) --rpc-url $PI_ENS_RPC` must return signer address. Returning `0x0...0` means name not wrapped — wrap via ENS UI before proceeding.
7. **Hardcoded PublicResolver address bypasses `ENS.resolver(node)` lookup.** If PublicResolver is redeployed on Sepolia (unlikely but possible) or someone unwraps/rewraps a subname pointing at a different resolver, reads return empty silently. Recovery: update constant in `ens.go` (one-line change) and rebuild.
8. **Sepolia chain reorgs / `WaitMined` flakiness.** Public Sepolia RPCs occasionally drop pending txs. Recovery: retry once with same nonce; if still fails, surface as per-label warn (idempotency reconciles next boot).
