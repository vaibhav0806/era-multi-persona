# M7-D — iNFT Contract + era Integration Design Spec

> Status: brainstormed and approved 2026-04-27. Implementation plans to follow.
> Companion vision doc: top-level `FEATURE.md`. Parent design: `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md`.

## §1 — Vision & scope

M7-D ships an **iNFT contract** representing era's personas as on-chain assets, fulfilling 0G Track 2's "iNFT-minted agents" prize criterion + Track 1's "infrastructure primitives". The contract is an **ERC-7857-inspired minimal subset** — full ERC-7857 oracle complexity (encrypted-metadata transfer, clone(), authorizeUsage(), TEE/ZKP) is documented as a roadmap migration, not implemented for the hackathon.

Two sub-milestones:

- **M7-D.1** — Solidity contract: `EraPersonaINFT` (ERC-721 base + per-token URI + custom `recordInvocation` event). Foundry project under `contracts/`. Foundry tests + deploy script that mints 3 default personas (planner/coder/reviewer) at deploy time. JSON metadata files committed under `contracts/metadata/` and pointed at via raw GitHub URLs.

- **M7-D.2** — Go client + era integration: `era-brain/inft/zg_7857/` package wrapping abigen bindings + `INFTRegistry` interface impl. era's `queue.RunNext` calls `recordInvocation(tokenId, receiptHash)` per persona run after the existing audit-log write. Live gate: real `/task` produces on-chain `Invocation` events on 0G explorer.

**Skip (deferred to M7-D.3+ or post-hackathon):**
- Encrypted-metadata transfer flow (full ERC-7857 oracle integration)
- `clone()` and `authorizeUsage()` functions
- TEE/ZKP oracle wiring
- Royalty splits (ERC-2981)
- User-mintable personas via `/persona-mint` Telegram command
- 0G Storage native blob URI for metadata (raw GitHub URL is the hackathon-pragmatic choice)
- System-prompt URI dereferencing inside era

## §2 — Architecture

Three layers across the two sub-milestones.

### Layer 1 — Solidity contract (M7-D.1)

New top-level dir `contracts/` (Foundry project; own `foundry.toml`; separate from era's Go code):

```
contracts/
├── foundry.toml                 — project config; remappings to OpenZeppelin
├── lib/openzeppelin-contracts/  — git submodule (forge install)
├── src/
│   └── EraPersonaINFT.sol       — the contract
├── test/
│   └── EraPersonaINFT.t.sol     — Foundry tests (Solidity)
├── script/
│   └── Deploy.s.sol             — deploys + mints 3 default personas
└── metadata/                    — committed JSON files
    ├── planner.json
    ├── coder.json
    └── reviewer.json
```

`EraPersonaINFT.sol` (~120 lines):
- Inherits OpenZeppelin's `ERC721`, `Ownable`.
- `mint(address to, string memory uri) returns (uint256 tokenId)` — only owner. Auto-incrementing tokenId. Stores `_tokenURIs[tokenId] = uri`. Emits standard `Transfer` event.
- `tokenURI(uint256 tokenId) returns (string)` override — returns the stored URI.
- `recordInvocation(uint256 tokenId, bytes32 receiptHash)` — only owner OR token holder. Validates tokenId exists. Emits `Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts)` (block.timestamp filled in).
- `totalSupply()` view for convenience.

Deploy script flow (`forge script Deploy.s.sol --broadcast --rpc-url $PI_ZG_EVM_RPC --legacy`):
1. Read `PI_ZG_PRIVATE_KEY` from env via `vm.envUint(...)`. (Confirmed: forge-std accepts `0x`-prefixed hex strings — the underlying revm parser handles both forms; no normalization needed.)
2. Deploy `EraPersonaINFT(deployer)`.
3. Mint planner → tokenId 0 → URI `https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/planner.json`.
4. Same for coder (1) and reviewer (2).
5. Console.log contract address + 3 tokenIds for env-var setup.

### Layer 2 — era-brain Go client (M7-D.2)

`era-brain/inft/zg_7857/` package replaces the interface-only `era-brain/inft/provider.go` stub from M7-A.2:

```
era-brain/inft/zg_7857/
├── zg_7857.go                   — Provider impl wrapping abigen bindings
├── zg_7857_test.go              — unit tests via *simulated.Backend (constructed via simulated.NewBackend(alloc, opts...))
├── zg_7857_live_test.go         — //go:build zg_live — hits real testnet contract
└── bindings/
    └── era_persona_inft.go      — abigen output (committed; regenerated via `make abigen`)
```

Public API:

```go
type Config struct {
    ContractAddress string  // 0x...
    EVMRPCURL       string  // https://evmrpc-testnet.0g.ai
    PrivateKey      string  // hex (0x prefix tolerated and stripped)
    ChainID         int64   // 0G Galileo testnet = 16602
}

type Provider struct { ... }

func New(cfg Config) (*Provider, error)
func (p *Provider) Close()

// RecordInvocation submits a tx logging the persona invocation. Returns the
// tx hash on success. Errors are wrapped with context.
func (p *Provider) RecordInvocation(ctx context.Context, tokenID string, receiptHashHex string) error

// Mint and Lookup return ErrNotImplemented in M7-D.2 (deferred to M7-D.3+).
func (p *Provider) Mint(ctx context.Context, name, uri string) (inft.Persona, error)
func (p *Provider) Lookup(ctx context.Context, ownerAddr, name string) (inft.Persona, error)
```

Compile-time interface check: `var _ inft.Registry = (*Provider)(nil)`.

### Layer 3 — era integration (M7-D.2)

- **`cmd/orchestrator/main.go`** — env vars: `PI_ZG_INFT_CONTRACT_ADDRESS` (new) + reuses existing `PI_ZG_PRIVATE_KEY` + `PI_ZG_EVM_RPC`. When the contract address is set AND private key is present, construct `zg_7857.New(...)` and inject into queue via `q.SetINFT(prov)`. `defer prov.Close()`.

- **`internal/queue/queue.go`** — Queue gains `inft INFTProvider` field (parallels how `swarm` is wired). After each successful persona LLM call (planner + reviewer), call `q.inft.RecordInvocation(ctx, tokenID, brain.ReceiptHash(receipt))`. Errors warn-only via `slog.Warn`, never block the task.

- Token IDs hardcoded as constants in queue.go: `plannerTokenID = "0"`, `reviewerTokenID = "2"`. Coder (`tokenID = "1"`) skipped because Pi remains unsealed in M7-C scope — no LLMPersona receipt to record.

- Receipt hash → bytes32 conversion happens inside `Provider.RecordInvocation` (hex-decode the 64-char string from `brain.ReceiptHash` into `[32]byte`).

## §3 — Components (detail)

### `contracts/src/EraPersonaINFT.sol`

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "openzeppelin-contracts/contracts/token/ERC721/ERC721.sol";
import "openzeppelin-contracts/contracts/access/Ownable.sol";

/// @title EraPersonaINFT
/// @notice ERC-7857-inspired minimal iNFT for era's coding-agent personas.
contract EraPersonaINFT is ERC721, Ownable {
    uint256 private _nextTokenId;
    mapping(uint256 => string) private _tokenURIs;

    event Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts);

    constructor(address initialOwner) ERC721("Era Persona iNFT", "ERAINFT") Ownable(initialOwner) {}

    function mint(address to, string memory uri) external onlyOwner returns (uint256 tokenId) {
        tokenId = _nextTokenId++;
        _safeMint(to, tokenId);
        _tokenURIs[tokenId] = uri;
    }

    function tokenURI(uint256 tokenId) public view override returns (string memory) {
        _requireOwned(tokenId);
        return _tokenURIs[tokenId];
    }

    function recordInvocation(uint256 tokenId, bytes32 receiptHash) external {
        require(_ownerOf(tokenId) != address(0), "EraPersonaINFT: token does not exist");
        require(
            msg.sender == owner() || msg.sender == _ownerOf(tokenId),
            "EraPersonaINFT: not authorized"
        );
        emit Invocation(tokenId, receiptHash, block.timestamp);
    }

    function totalSupply() external view returns (uint256) {
        return _nextTokenId;
    }
}
```

### `contracts/test/EraPersonaINFT.t.sol`

~10 Foundry tests:
- `testMintByOwner` — owner can mint; tokenId increments; tokenURI stored.
- `testMintByNonOwnerReverts` — onlyOwner enforcement.
- `testTokenURIRevertsForNonExistent`.
- `testRecordInvocationByOwner` — emits Invocation event w/ correct fields (use `vm.expectEmit`).
- `testRecordInvocationByTokenHolder` — non-owner who holds the token can record.
- `testRecordInvocationByStrangerReverts`.
- `testRecordInvocationForNonExistentTokenReverts`.
- `testTransferUpdatesHolderACLForRecord` — after `safeTransferFrom`, new holder can call recordInvocation.
- `testTotalSupplyIncrements`.
- `testEventFieldsExact` — assert `Invocation` event includes block.timestamp.

### `contracts/script/Deploy.s.sol`

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/EraPersonaINFT.sol";

contract Deploy is Script {
    function run() external {
        uint256 deployerKey = vm.envUint("PI_ZG_PRIVATE_KEY");
        address deployer = vm.addr(deployerKey);
        string memory baseURL = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/";

        vm.startBroadcast(deployerKey);
        EraPersonaINFT inft = new EraPersonaINFT(deployer);
        uint256 plannerId  = inft.mint(deployer, string.concat(baseURL, "planner.json"));
        uint256 coderId    = inft.mint(deployer, string.concat(baseURL, "coder.json"));
        uint256 reviewerId = inft.mint(deployer, string.concat(baseURL, "reviewer.json"));
        vm.stopBroadcast();

        console.log("Contract address:", address(inft));
        console.log("Planner tokenId:", plannerId);
        console.log("Coder tokenId:", coderId);
        console.log("Reviewer tokenId:", reviewerId);
    }
}
```

### `contracts/metadata/planner.json` (similar shape for coder, reviewer)

```json
{
  "name": "Planner",
  "description": "Era's planner persona — drafts step lists for coding tasks.",
  "system_prompt_url": "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/internal/swarm/personas.go",
  "persona_role": "planner",
  "version": 1,
  "created_by": "vaibhav0806/era-multi-persona"
}
```

(Optional `image` field; skip if no asset.)

### `era-brain/inft/zg_7857/zg_7857.go`

Wraps abigen bindings. Key methods: `New`, `Close`, `RecordInvocation`. `Mint` and `Lookup` return `ErrNotImplemented` for M7-D.2 scope.

Imports: `github.com/ethereum/go-ethereum/{accounts/abi/bind, common, crypto, ethclient}`. PrivateKey hex stripped of optional `0x` prefix via `strings.TrimPrefix`.

`RecordInvocation` flow:
1. Parse `tokenID` (decimal string) into `*big.Int`.
2. Decode `receiptHashHex` (hex string) into `[32]byte` — error if not 32 bytes.
3. Call `p.contract.RecordInvocation(p.auth, tokenIDBig, hash)` from abigen bindings.
4. Wrap any error with `"recordInvocation tx: %w"`.

### era integration (`cmd/orchestrator/main.go` + `internal/queue/queue.go`)

`main.go`:

```go
inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
if inftAddr != "" && os.Getenv("PI_ZG_PRIVATE_KEY") != "" {
    inftProv, err := zg_7857.New(zg_7857.Config{
        ContractAddress: inftAddr,
        EVMRPCURL:       os.Getenv("PI_ZG_EVM_RPC"),
        PrivateKey:      os.Getenv("PI_ZG_PRIVATE_KEY"),
        ChainID:         16602,
    })
    if err != nil { fail(fmt.Errorf("inft provider: %w", err)) }
    defer inftProv.Close()
    q.SetINFT(inftProv)
    slog.Info("0G iNFT registry wired", "contract", inftAddr)
}
```

`queue/queue.go`:
- New `INFTProvider` interface (1 method): `RecordInvocation(ctx, tokenID, receiptHashHex string) error`.
- `Queue.inft INFTProvider` field; `SetINFT(p)` setter.
- In `RunNext` after `q.swarm.Plan(...)` succeeds → `if q.inft != nil { q.inft.RecordInvocation(ctx, plannerTokenID, brain.ReceiptHash(plannerReceipt)) }` w/ slog.Warn on error.
- Same after `q.swarm.Review(...)` → `reviewerTokenID = "2"`.

Token ID constants at top of queue.go:
```go
const (
    plannerTokenID  = "0"
    reviewerTokenID = "2"
    // coder tokenID 1 skipped — Pi-in-Docker is unsealed in M7-C scope
)
```

## §4 — Error handling, testing, security

### Error handling

- Contract revert (insufficient gas, ACL fail, non-existent token) → tx returns error → `slog.Warn("inft recordInvocation failed", ...)` → task continues. Never blocks `/task` completion.
- RPC timeout / network blip → tx submission errors → same warn-only.
- Receipt hash decode fail (corrupt hex) → wrap + log; skip the call.
- Contract address misconfigured → `New()` succeeds (just dials RPC) but `RecordInvocation` fails on first tx with "execution reverted" → warn fires repeatedly; user investigates.
- iNFT provider unset (`q.inft == nil`) → guard skips entirely; preserves M7-C.2 baseline.

### Testing

**Solidity (Foundry):** ~10 tests in `EraPersonaINFT.t.sol` covering mint, ACL, recordInvocation, transfer-then-record, event field correctness. Run via `forge test -vv`.

**Go unit (era-brain/inft/zg_7857):** abigen bindings tested via go-ethereum's simulated chain (`github.com/ethereum/go-ethereum/ethclient/*simulated.Backend (constructed via simulated.NewBackend(alloc, opts...))`). Deploy contract to sim chain, call `RecordInvocation`, assert event log shape. ~3 tests.

**Go live integration (build-tag `zg_live`):** hits real testnet contract. One test verifies a single `RecordInvocation` against a known tokenId, confirming tx mines. Cost ~0.0005 ZG per call.

**era integration:** queue test extends existing `stubSwarm` pattern with `stubINFT` capturing `lastRecordTokenID` and `lastReceiptHash`. Assertion: after RunNext, both planner+reviewer's tokenIDs were passed.

**Live gate (M7-D.2 final):** real Telegram `/task`. Verify (1) `0G iNFT registry wired` boot line, (2) `Invocation` events appear on 0G explorer when filtering by contract address + recent blocks, (3) tx hashes captured in stdout.

### Security

- **Hot wallet exposure expanded.** Deployer wallet is contract owner + only minter + recordInvocation authority for non-transferred tokens. Loss of wallet = loss of mint capability for all 3 default tokens. Mitigation: same as M7-B/C — testnet hot wallet w/ small balance only. Document deployer key as the iNFT admin in README.
- **recordInvocation spam.** `onlyOwner OR tokenHolder` ACL prevents random callers from polluting the event log. If wallet leaked, attacker could spam events but not transfer tokens (`safeTransferFrom` requires the holder's signature).
- **No marketplace integration.** Tokens are transferable but no marketplace is wired. Hackathon scope; documented as roadmap.
- **Contract upgradeability.** Not implemented. If a bug surfaces, redeploy + remint to a new contract address. Trade-off: simpler code, breaks demo continuity if it happens. Acceptable for hackathon.
- **Reentrancy.** `recordInvocation` is event-only — no state change beyond emit, no external calls. Reentrancy-safe.
- **Front-running.** N/A — Invocation events are informational; nothing economic on the line.

## §5 — Milestones (phases)

### M7-D.1 — Solidity contract + Foundry tests + testnet deploy

| Phase | Tag | What |
|---|---|---|
| D.1.0 | `m7d1-0-foundry-init` | New `contracts/` dir with `foundry.toml`, OpenZeppelin git submodule via `forge install`, `.gitignore` for `out/` `cache/` `broadcast/`. `forge test` runs (zero tests yet, exits 0). |
| D.1.1 | `m7d1-1-mint-acl` | TDD: `EraPersonaINFT.sol` w/ ERC-721 base + `mint(to, uri)` (onlyOwner) + `tokenURI` override. 5 Foundry tests pass. |
| D.1.2 | `m7d1-2-record-invocation` | TDD: `recordInvocation(tokenId, receiptHash)` + `Invocation` event + ACL (owner or holder) + non-existent token revert. 5 more Foundry tests pass. |
| D.1.3 | `m7d1-3-deploy-script` | `Deploy.s.sol` reads `PI_ZG_PRIVATE_KEY` from env, deploys + mints 3 default personas. `metadata/{planner,coder,reviewer}.json` committed. |
| D.1.4 | `m7d1-done` | LIVE GATE: `forge script Deploy.s.sol --broadcast --rpc-url $PI_ZG_EVM_RPC --legacy` deploys to 0G testnet. Capture contract address + 3 tokenIds. Verify on 0G Galileo explorer: contract code visible, 3 Transfer events, ownership = deployer. Add `PI_ZG_INFT_CONTRACT_ADDRESS` to `.env.example`. |

**Estimated effort:** ~1.5 days subagent work.

### M7-D.2 — era-brain Go client + era integration

| Phase | Tag | What |
|---|---|---|
| D.2.0 | `m7d2-0-abigen` | Generate `era-brain/inft/zg_7857/bindings/era_persona_inft.go` via abigen. Forge's `out/EraPersonaINFT.sol/EraPersonaINFT.json` is a combined artifact, so the `make abigen` target must extract the ABI first: `jq '.abi' contracts/out/EraPersonaINFT.sol/EraPersonaINFT.json > /tmp/era_inft.abi && abigen --abi /tmp/era_inft.abi --pkg bindings --type EraPersonaINFT --out era-brain/inft/zg_7857/bindings/era_persona_inft.go`. Commit the bindings file. |
| D.2.1 | `m7d2-1-provider` | TDD: `era-brain/inft/zg_7857/zg_7857.go` w/ `Provider` impl + `RecordInvocation`. 3 unit tests via `*simulated.Backend (constructed via simulated.NewBackend(alloc, opts...))`. Build-tagged live test against real testnet. |
| D.2.2 | `m7d2-2-queue-wiring` | era integration: `INFTProvider` interface in queue.go; `Queue.inft` + `SetINFT` setter; `RunNext` calls `RecordInvocation` after each persona LLM call. Cascade `stubINFT` in queue_run_test.go. |
| D.2.3 | `m7d2-3-orchestrator-wiring` | `cmd/orchestrator/main.go` constructs `zg_7857.New(...)` when `PI_ZG_INFT_CONTRACT_ADDRESS` is set + calls `q.SetINFT(prov)`. Boot log confirms wiring. |
| D.2.4 | `m7d2-done` | LIVE GATE: real Telegram `/task` produces 2 on-chain `Invocation` events (planner + reviewer; coder skipped per M7-C). `cast logs` filtering on contract addr shows the events. Tx hashes recorded. |

**Estimated effort:** ~1.5 days subagent work.

**Total M7-D:** ~3 days.

## §6 — Decisions log (Q&A from brainstorming)

| Q | Choice | Rationale |
|---|---|---|
| Q1 — Scope split | A: two sub-milestones (D.1 contract + D.2 Go integration) | Each ships a self-contained artifact; matches the M7-B/C cadence. |
| Q2 — Reference vs roll-our-own | A: roll our own minimal ERC-721 + iNFT semantics | Hackathon scope wins beat compliance theater; ~120 lines vs 500+ unfamiliar fork. |
| Q3 — Tooling | A: Foundry | Smaller dependency surface; faster iteration; Solidity tests; user already has `forge` + `cast` installed. |
| Q4 — Mint timing | A: deploy-time mint in Foundry script | Static defaults need static mint; avoids Go-side mint code; user-mint deferred to M7-D.3+. |
| Q5 — tokenURI scheme | A: raw GitHub URLs of committed JSON files | Real, clickable, audit-trailable; 0G Storage native is a roadmap item. |
| Q6 — Royalty splits | A: skip | No marketplace integrated; royalty splits are demo-theater without one. |

## §7 — Out of scope (deferred)

Per spec §1 + Q-decisions:

- **Encrypted-metadata transfer flow** (full ERC-7857 oracle integration with TEE/ZKP).
- **`clone()` and `authorizeUsage()` functions.**
- **Royalty splits** (ERC-2981).
- **User-mintable personas** via `/persona-mint` Telegram command. M7-D.3+ scope.
- **0G Storage native blob URI** for metadata. Migration is a post-hackathon roadmap item.
- **Coder persona on-chain receipts.** Pi-in-Docker remains unsealed per M7-C scope.
- **Contract upgradeability** (proxy patterns).
- **Marketplace integration / external tradability flows.**

## §8 — Cuts list (in order if slipping)

1. `Mint`/`Lookup` impls on Provider — keep returning `ErrNotImplemented`; defer entirely.
2. Live integration test in D.2.1 (`zg_live` tag) — defer to D.2.4's live gate.
3. Solidity event-fields-exact tests (the cosmetic ones) — keep just the core ACL + emit tests.
4. `recordInvocation` ACL allowing token holders (not just owner) — simplify to `onlyOwner` only. Hackathon scope: deployer is always the holder, never transfers. Saves 1 test, 4 lines of contract code.
5. Coder tokenID `Invocation` events — already deferred (Pi unsealed scope).

---
