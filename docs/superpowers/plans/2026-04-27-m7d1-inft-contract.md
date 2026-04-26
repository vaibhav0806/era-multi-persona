# M7-D.1 — iNFT Contract + Foundry Tests + Testnet Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up `contracts/` Foundry project with `EraPersonaINFT.sol` (ERC-721 base + per-token URI + custom `recordInvocation` event), 10+ Foundry tests covering mint/ACL/event-emission, deploy script that mints 3 default personas (planner/coder/reviewer), and a verified testnet deployment with contract address captured in `.env.example`.

**Architecture:** Five linear phases. Phase 0 is Foundry project init w/ OpenZeppelin git submodule. Phases 1-2 build `EraPersonaINFT.sol` TDD-first w/ Foundry's Solidity tests. Phase 3 writes the deploy script. Phase 4 is the live testnet deploy (user-driven — needs gas + manual verification on 0G explorer).

**Tech Stack:** Solidity ^0.8.20, Foundry (forge/cast — already installed), OpenZeppelin v5 contracts (via `forge install` git submodule), 0G Galileo testnet (chainID 16602, RPC `https://evmrpc-testnet.0g.ai`).

**Spec:** `docs/superpowers/specs/2026-04-27-m7d-inft-design.md`. All §-references below point at the spec.

**Testing philosophy:** Strict TDD. Failing test first, run, verify FAIL, write minimal Solidity, run, verify PASS, commit. Foundry's `forge test -vv` is the test runner. Live deploy is the only thing that touches real testnet (Phase 4); contract logic is fully covered by Solidity unit tests before deploy.

**Prerequisites (check before starting):**
- M7-C.2 done (tag `m7c2-done`).
- `forge --version` and `cast --version` work (user already has Foundry installed).
- 0G testnet wallet has ≥0.1 ZG for deployment gas (deploy + 3 mints ≈ ~0.005 ZG total).
- `.env` has `PI_ZG_PRIVATE_KEY` populated.
- Existing era + era-brain Go tests still pass (non-regression baseline).

---

## File Structure

```
contracts/                                      CREATE (Phase 0) — top-level Foundry project
├── foundry.toml                                CREATE (Phase 0) — project config
├── .gitignore                                  CREATE (Phase 0) — ignore out/ cache/ broadcast/
├── lib/openzeppelin-contracts/                 CREATE (Phase 0) — git submodule via forge install
├── src/
│   └── EraPersonaINFT.sol                      CREATE (Phase 1, 2) — the contract; 2 phases (mint+ACL → recordInvocation)
├── test/
│   └── EraPersonaINFT.t.sol                    CREATE (Phase 1, 2) — Foundry tests
├── script/
│   └── Deploy.s.sol                            CREATE (Phase 3) — deploys + mints 3 default personas
└── metadata/                                   CREATE (Phase 3) — committed JSON files
    ├── planner.json
    ├── coder.json
    └── reviewer.json

.env.example                                    MODIFY (Phase 4) — add PI_ZG_INFT_CONTRACT_ADDRESS
```

No changes to existing era code. era-brain stays untouched. M7-D.2 (a separate plan) handles the Go side.

---

## Phase 0: Foundry project init

**Files:**
- Create: `contracts/foundry.toml`
- Create: `contracts/.gitignore`
- Create: `contracts/lib/openzeppelin-contracts/` (via `forge install`)

### Step 0.1: Create the contracts directory

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
mkdir -p contracts
cd contracts
```

### Step 0.2: Initialize Foundry project

```bash
forge init --no-git .
```

`--no-git` because we already have a parent repo. (Foundry doesn't auto-commit by default; no flag needed for that.)

This creates `src/Counter.sol`, `test/Counter.t.sol`, `script/Counter.s.sol`, `lib/forge-std/`, `foundry.toml`, `.gitignore`, `README.md`. We'll delete the Counter scaffolding next.

### Step 0.3: Delete the Counter boilerplate

```bash
rm src/Counter.sol test/Counter.t.sol script/Counter.s.sol README.md
```

### Step 0.4: Install OpenZeppelin v5 contracts

```bash
forge install OpenZeppelin/openzeppelin-contracts
```

This adds `lib/openzeppelin-contracts/` as a git submodule.

### Step 0.5: Configure foundry.toml

Replace the auto-generated `foundry.toml` content with:

```toml
[profile.default]
src = "src"
out = "out"
libs = ["lib"]
# OZ v5.6+ requires ^0.8.24; our contract's pragma is ^0.8.20 (compatible).
# Pinning solc to 0.8.24 satisfies both.
solc = "0.8.24"
optimizer = true
optimizer_runs = 200
remappings = [
    "openzeppelin-contracts/=lib/openzeppelin-contracts/",
    "forge-std/=lib/forge-std/src/",
]

# 0G Galileo testnet
[rpc_endpoints]
zg_testnet = "https://evmrpc-testnet.0g.ai"

# See more config options https://github.com/foundry-rs/foundry/blob/master/crates/config/README.md#all-options
```

### Step 0.6: Verify forge build works against an empty src

```bash
forge build
```

Expected: `No files changed, compilation skipped` OR `Compiling N files with Solc 0.8.20` (depends on cache state). Exit 0.

### Step 0.7: Verify forge test works (no tests yet)

```bash
forge test
```

Expected: `No tests found.` Exit 0.

### Step 0.8: Set up .gitignore

The auto-generated `contracts/.gitignore` should already include `out/`, `cache/`, `broadcast/`. Verify:

```bash
cat .gitignore
```

If missing, add:
```
out/
cache/
broadcast/
```

(Note: `broadcast/` contains deploy artifacts including tx hashes; we want those committed in the broadcasts of the actual deploy. Per Foundry convention they're typically gitignored. We'll commit the relevant artifacts manually after Phase 4.)

### Step 0.9: Commit Phase 0

From era repo root:

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add contracts/
git commit -m "phase(M7-D.1.0): initialize contracts/ Foundry project + OpenZeppelin submodule"
git tag m7d1-0-foundry-init
```

The submodule's `.gitmodules` entry gets created at the repo root (not `contracts/`). Verify:

```bash
cat .gitmodules
```

Should contain:
```
[submodule "contracts/lib/openzeppelin-contracts"]
    path = contracts/lib/openzeppelin-contracts
    url = https://github.com/OpenZeppelin/openzeppelin-contracts
```

If the submodule was added inside `contracts/.gitmodules` instead of repo root, that's also fine — but the commit must include both the `.gitmodules` file and the `contracts/lib/openzeppelin-contracts` pointer.

---

## Phase 1: EraPersonaINFT.sol — mint + ACL + tokenURI

**Files:**
- Create: `contracts/src/EraPersonaINFT.sol` (partial — mint + tokenURI only this phase; recordInvocation comes in Phase 2)
- Create: `contracts/test/EraPersonaINFT.t.sol` (5 tests this phase)

### 1A: Failing tests for mint + tokenURI

- [ ] **Step 1.1: Write the failing tests**

`contracts/test/EraPersonaINFT.t.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "../src/EraPersonaINFT.sol";

contract EraPersonaINFTTest is Test {
    EraPersonaINFT internal inft;
    address internal owner;
    address internal stranger;
    address internal holder;

    function setUp() public {
        owner = address(this); // test contract is deployer
        stranger = makeAddr("stranger");
        holder = makeAddr("holder");
        inft = new EraPersonaINFT(owner);
    }

    // Required because some tests mint to address(this) — _safeMint calls
    // onERC721Received on contract recipients. forge-std's Test contract
    // doesn't implement IERC721Receiver, so we provide it here.
    function onERC721Received(address, address, uint256, bytes calldata)
        external pure returns (bytes4)
    {
        return this.onERC721Received.selector;
    }

    // ---- mint + tokenURI tests (Phase 1) ----

    function testMintByOwner() public {
        uint256 tokenId = inft.mint(owner, "ipfs://planner.json");
        assertEq(tokenId, 0, "first tokenId should be 0");
        assertEq(inft.ownerOf(0), owner);
        assertEq(inft.tokenURI(0), "ipfs://planner.json");
        assertEq(inft.totalSupply(), 1);
    }

    function testMintIncrementsTokenId() public {
        inft.mint(owner, "ipfs://planner.json");
        uint256 second = inft.mint(owner, "ipfs://coder.json");
        assertEq(second, 1, "second tokenId should be 1");
        assertEq(inft.totalSupply(), 2);
    }

    function testMintByNonOwnerReverts() public {
        vm.prank(stranger);
        vm.expectRevert(); // OZ Ownable v5 reverts with OwnableUnauthorizedAccount(address)
        inft.mint(stranger, "ipfs://malicious.json");
    }

    function testTokenURIRevertsForNonExistent() public {
        vm.expectRevert(); // ERC721 v5 reverts with ERC721NonexistentToken(uint256)
        inft.tokenURI(999);
    }

    function testMintToDifferentRecipient() public {
        uint256 tokenId = inft.mint(holder, "ipfs://planner.json");
        assertEq(inft.ownerOf(tokenId), holder, "holder owns the token even though owner minted");
        assertEq(inft.balanceOf(holder), 1);
        assertEq(inft.balanceOf(owner), 0);
    }
}
```

- [ ] **Step 1.2: Run, verify FAIL**

```bash
cd contracts
forge test
```

Expected: error referencing "EraPersonaINFT" not found OR `Error: file not found: ../src/EraPersonaINFT.sol`. The contract doesn't exist yet.

### 1B: Implement EraPersonaINFT.sol — minimum to pass Phase 1 tests

- [ ] **Step 1.3: Write the contract**

`contracts/src/EraPersonaINFT.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "openzeppelin-contracts/contracts/token/ERC721/ERC721.sol";
import "openzeppelin-contracts/contracts/access/Ownable.sol";

/// @title EraPersonaINFT
/// @notice ERC-7857-inspired minimal iNFT for era's coding-agent personas.
///         Each token represents one persona (planner/coder/reviewer + future user-mints).
///         tokenURI points at a JSON blob describing the persona.
/// @dev Out-of-scope vs full ERC-7857: encrypted-metadata transfer, clone(),
///      authorizeUsage(), TEE/ZKP oracles, royalty splits. Roadmap items.
contract EraPersonaINFT is ERC721, Ownable {
    uint256 private _nextTokenId;
    mapping(uint256 => string) private _tokenURIs;

    constructor(address initialOwner) ERC721("Era Persona iNFT", "ERAINFT") Ownable(initialOwner) {}

    /// @notice Mint a new persona iNFT. Only contract owner.
    /// @param to Recipient.
    /// @param uri Metadata URI (raw GitHub URL of persona JSON for hackathon scope).
    /// @return tokenId The newly minted token's ID.
    function mint(address to, string memory uri) external onlyOwner returns (uint256 tokenId) {
        tokenId = _nextTokenId++;
        _safeMint(to, tokenId);
        _tokenURIs[tokenId] = uri;
    }

    /// @notice Get the metadata URI for a token. Reverts on non-existent token.
    function tokenURI(uint256 tokenId) public view override returns (string memory) {
        _requireOwned(tokenId);
        return _tokenURIs[tokenId];
    }

    function totalSupply() external view returns (uint256) {
        return _nextTokenId;
    }
}
```

- [ ] **Step 1.4: Run, verify PASS**

```bash
forge test -vv
```

Expected: 5 tests pass.

If compile errors:
- "imported file not found" → check `foundry.toml` remappings.
- "Function visibility" → check overrides match OZ v5's signature.

- [ ] **Step 1.5: Commit Phase 1**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add contracts/src/EraPersonaINFT.sol contracts/test/EraPersonaINFT.t.sol
git commit -m "phase(M7-D.1.1): EraPersonaINFT mint + tokenURI + ACL — 5 Foundry tests pass"
git tag m7d1-1-mint-acl
```

---

## Phase 2: recordInvocation + Invocation event

**Files:**
- Modify: `contracts/src/EraPersonaINFT.sol` — add recordInvocation function + Invocation event
- Modify: `contracts/test/EraPersonaINFT.t.sol` — append 5 more tests

### 2A: Failing tests for recordInvocation

- [ ] **Step 2.1: Append tests to EraPersonaINFT.t.sol**

Append to the existing `EraPersonaINFTTest` contract (inside the same closing `}` — add new test functions before it):

```solidity
    // ---- recordInvocation tests (Phase 2) ----

    function testRecordInvocationByOwner() public {
        inft.mint(owner, "ipfs://planner.json"); // tokenId 0
        bytes32 receiptHash = keccak256("a receipt");

        vm.expectEmit(true, true, true, false); // topic1, topic2, topic3 indexed; data not asserted
        emit EraPersonaINFT.Invocation(0, receiptHash, block.timestamp);

        inft.recordInvocation(0, receiptHash);
    }

    function testRecordInvocationByTokenHolder() public {
        inft.mint(holder, "ipfs://planner.json"); // tokenId 0, owner = holder
        bytes32 receiptHash = keccak256("a receipt");

        vm.prank(holder); // not contract owner, but token holder
        vm.expectEmit(true, true, true, false);
        emit EraPersonaINFT.Invocation(0, receiptHash, block.timestamp);

        inft.recordInvocation(0, receiptHash);
    }

    function testRecordInvocationByStrangerReverts() public {
        inft.mint(holder, "ipfs://planner.json"); // tokenId 0, owner = holder

        vm.prank(stranger);
        vm.expectRevert(bytes("EraPersonaINFT: not authorized"));
        inft.recordInvocation(0, keccak256("a receipt"));
    }

    function testRecordInvocationForNonExistentTokenReverts() public {
        vm.expectRevert(bytes("EraPersonaINFT: token does not exist"));
        inft.recordInvocation(999, keccak256("a receipt"));
    }

    function testTransferUpdatesHolderACLForRecord() public {
        // 1. mint to holder
        inft.mint(holder, "ipfs://planner.json"); // tokenId 0

        // 2. holder transfers to stranger
        vm.prank(holder);
        inft.safeTransferFrom(holder, stranger, 0);
        assertEq(inft.ownerOf(0), stranger);

        // 3. now stranger (new holder) can record; original holder cannot
        vm.prank(stranger);
        inft.recordInvocation(0, keccak256("post-transfer"));

        vm.prank(holder);
        vm.expectRevert(bytes("EraPersonaINFT: not authorized"));
        inft.recordInvocation(0, keccak256("from previous holder"));

        // 4. contract owner can still record (admin always wins)
        inft.recordInvocation(0, keccak256("admin"));
    }
```

- [ ] **Step 2.2: Run, verify FAIL**

```bash
forge test -vv
```

Expected: tests fail because:
- `EraPersonaINFT.Invocation` event doesn't exist (compile error).
- `inft.recordInvocation(...)` doesn't exist (compile error).

### 2B: Add recordInvocation + Invocation event to the contract

- [ ] **Step 2.3: Modify `contracts/src/EraPersonaINFT.sol`**

Add the event declaration above the constructor:

```solidity
    /// @notice Emitted when an iNFT is invoked (per sealed-inference run).
    /// @param tokenId The persona invoked.
    /// @param receiptHash sha256 of the brain.Receipt (32 bytes).
    /// @param ts block.timestamp at the time of recording.
    event Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts);
```

Add the function after `tokenURI`:

```solidity
    /// @notice Record a persona-invocation event on-chain. Callable by the
    ///         contract owner OR the current token holder.
    /// @param tokenId The persona invoked.
    /// @param receiptHash sha256 of the run's Receipt struct.
    function recordInvocation(uint256 tokenId, bytes32 receiptHash) external {
        require(_ownerOf(tokenId) != address(0), "EraPersonaINFT: token does not exist");
        require(
            msg.sender == owner() || msg.sender == _ownerOf(tokenId),
            "EraPersonaINFT: not authorized"
        );
        emit Invocation(tokenId, receiptHash, block.timestamp);
    }
```

- [ ] **Step 2.4: Run, verify PASS**

```bash
forge test -vv
```

Expected: 10 tests pass (5 from Phase 1 + 5 new).

- [ ] **Step 2.5: Commit Phase 2**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add contracts/src/EraPersonaINFT.sol contracts/test/EraPersonaINFT.t.sol
git commit -m "phase(M7-D.1.2): recordInvocation + Invocation event + ACL — 5 more Foundry tests pass"
git tag m7d1-2-record-invocation
```

---

## Phase 3: Deploy script + persona JSON metadata

**Files:**
- Create: `contracts/script/Deploy.s.sol`
- Create: `contracts/metadata/planner.json`
- Create: `contracts/metadata/coder.json`
- Create: `contracts/metadata/reviewer.json`

No tests in this phase — Forge scripts that broadcast txs aren't unit-testable in the traditional sense. The "test" is Phase 4's live deploy verification.

### 3A: Persona metadata JSON files

- [ ] **Step 3.1: Write `contracts/metadata/planner.json`**

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

- [ ] **Step 3.2: Write `contracts/metadata/coder.json`**

```json
{
  "name": "Coder",
  "description": "Era's coder persona — Pi-in-Docker tool-loop engine producing diffs and commits.",
  "system_prompt_url": "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/cmd/runner/pi.go",
  "persona_role": "coder",
  "version": 1,
  "created_by": "vaibhav0806/era-multi-persona"
}
```

- [ ] **Step 3.3: Write `contracts/metadata/reviewer.json`**

```json
{
  "name": "Reviewer",
  "description": "Era's reviewer persona — critiques diffs and produces approve/flag decisions.",
  "system_prompt_url": "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/internal/swarm/personas.go",
  "persona_role": "reviewer",
  "version": 1,
  "created_by": "vaibhav0806/era-multi-persona"
}
```

### 3B: Deploy script

- [ ] **Step 3.4: Write `contracts/script/Deploy.s.sol`**

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/EraPersonaINFT.sol";

/// @notice Deploys EraPersonaINFT to 0G Galileo testnet and mints 3 default
///         personas (planner=0, coder=1, reviewer=2). Reads PI_ZG_PRIVATE_KEY
///         from env (forge-std vm.envUint accepts 0x-prefixed hex).
///
/// Usage:
///   set -a; source ../.env; set +a
///   forge script script/Deploy.s.sol --broadcast --rpc-url $PI_ZG_EVM_RPC --legacy
contract Deploy is Script {
    function run() external {
        uint256 deployerKey = vm.envUint("PI_ZG_PRIVATE_KEY");
        address deployer = vm.addr(deployerKey);

        // Raw GitHub URL base — points at master branch JSON files we committed
        // under contracts/metadata/.
        string memory baseURL = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/";

        vm.startBroadcast(deployerKey);

        EraPersonaINFT inft = new EraPersonaINFT(deployer);

        uint256 plannerId  = inft.mint(deployer, string.concat(baseURL, "planner.json"));
        uint256 coderId    = inft.mint(deployer, string.concat(baseURL, "coder.json"));
        uint256 reviewerId = inft.mint(deployer, string.concat(baseURL, "reviewer.json"));

        vm.stopBroadcast();

        console.log("=== EraPersonaINFT deployed ===");
        console.log("Contract address:", address(inft));
        console.log("Planner tokenId :", plannerId);
        console.log("Coder tokenId   :", coderId);
        console.log("Reviewer tokenId:", reviewerId);
        console.log("Owner           :", deployer);
        console.log("");
        console.log("Add to .env:");
        console.log(string.concat("PI_ZG_INFT_CONTRACT_ADDRESS=", vm.toString(address(inft))));
    }
}
```

- [ ] **Step 3.5: Build, verify the deploy script compiles**

```bash
cd contracts
forge build
```

Expected: exit 0; `Compiling N files with Solc 0.8.20`.

If compile errors:
- "vm.envUint not found" → forge-std import path wrong; check remappings.
- "string.concat not found" → bump solc to 0.8.20 (string.concat is ≥ 0.8.12).

- [ ] **Step 3.6: Dry-run the deploy locally (no --broadcast)**

```bash
set -a; source ../.env; set +a
forge script script/Deploy.s.sol --rpc-url $PI_ZG_EVM_RPC
```

Expected: `Script ran successfully.` Console.log output shows planned contract address (computed from sender + nonce) and tokenIds 0/1/2. NO transactions broadcast (no `--broadcast` flag).

If the dry-run fails:
- "missing env var PI_ZG_PRIVATE_KEY" → `.env` not sourced; re-source.
- "vm.envUint failed to parse" → key not 0x-prefixed hex; verify .env value.
- RPC errors → `PI_ZG_EVM_RPC` empty or unreachable; check `.env`.

### 3C: Commit Phase 3

- [ ] **Step 3.7: Run all tests + verify**

```bash
forge test -vv
```

Expected: 10 tests still pass (Phase 3 didn't touch the contract or tests).

- [ ] **Step 3.8: Commit**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add contracts/script/Deploy.s.sol contracts/metadata/
git commit -m "phase(M7-D.1.3): Deploy.s.sol mints 3 default personas; metadata JSON committed"
git tag m7d1-3-deploy-script
```

---

## Phase 4: Live testnet deploy + verification

**Files:**
- Modify: `.env.example` — add `PI_ZG_INFT_CONTRACT_ADDRESS=0xYOUR_DEPLOYED_ADDRESS`

**This phase touches real testnet.** ~0.005 ZG of gas burned. The contract address is permanent on-chain.

### 4.1: Verify wallet balance

- [ ] **Step 4.1.1: Check balance is sufficient**

```bash
WALLET_ADDR=0x6DB1508Deeb45E0194d4716349622806672f6Ac2
curl -s -X POST https://evmrpc-testnet.0g.ai \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBalance\",\"params\":[\"$WALLET_ADDR\",\"latest\"],\"id\":1}"
```

Hex result × 10⁻¹⁸ = ZG balance. Need ≥0.1 ZG (deploy + 3 mints). If short, faucet up.

### 4.2: Live deploy

- [ ] **Step 4.2.1: Run the deploy script with --broadcast**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
cd contracts
forge script script/Deploy.s.sol --broadcast --rpc-url $PI_ZG_EVM_RPC --legacy
```

`--legacy` avoids EIP-1559 if the testnet doesn't support it cleanly. If `--legacy` errors, retry without it.

Expected output:
- "Submitting tx for ..." × 4 (deploy + 3 mints)
- "Transaction confirmed" × 4
- Console.log block with contract address + 3 tokenIds
- Final line: "ONCHAIN EXECUTION COMPLETE & SUCCESSFUL"

If a tx fails:
- "insufficient funds" → faucet more ZG, retry.
- "nonce too low" → wait 30 sec for testnet to catch up, retry.
- "execution reverted" — should not happen for a fresh deploy; check whether `Deploy.s.sol` has any logic errors.

### 4.3: Capture deploy outputs

- [ ] **Step 4.3.1: Save the contract address from console.log output**

The script prints `Contract address: 0xABC...`. Save this string — it's the canonical iNFT contract address. M7-D.2 wires it into orchestrator config.

Also capture each tokenId (should be 0, 1, 2 in order — matches `_nextTokenId` auto-increment). Note any tx hashes from the broadcast log.

### 4.4: Verify on 0G Galileo explorer

- [ ] **Step 4.4.1: Open the explorer, verify contract**

Visit `https://chainscan-galileo.0g.ai/address/<CONTRACT_ADDRESS>` (substitute the address from Step 4.3.1).

Verify:
- Contract code is deployed (Solidity version 0.8.20).
- Three `Transfer` events visible (tokenId 0/1/2 transferred from address(0) to deployer wallet).
- Contract owner = deployer (call `owner()` view function in the explorer's "Read Contract" tab if available).

- [ ] **Step 4.4.2: Verify tokenURIs via cast**

```bash
cd contracts
cast call <CONTRACT_ADDRESS> "tokenURI(uint256)(string)" 0 --rpc-url $PI_ZG_EVM_RPC
cast call <CONTRACT_ADDRESS> "tokenURI(uint256)(string)" 1 --rpc-url $PI_ZG_EVM_RPC
cast call <CONTRACT_ADDRESS> "tokenURI(uint256)(string)" 2 --rpc-url $PI_ZG_EVM_RPC
```

Expected: 3 URLs pointing at `.../master/contracts/metadata/{planner,coder,reviewer}.json`.

### 4.5: Update .env.example

- [ ] **Step 4.5.1: Append to `.env.example`**

```
# 0G iNFT contract (M7-D onward)
# Deployed via contracts/script/Deploy.s.sol — see docs/superpowers/specs/2026-04-27-m7d-inft-design.md
PI_ZG_INFT_CONTRACT_ADDRESS=0xREPLACE_WITH_DEPLOYED_ADDRESS
```

(Don't put the actual address in `.env.example` since the file is committed and others may want to deploy their own. The user's `.env` has the real address.)

- [ ] **Step 4.5.2: Add the actual address to your local `.env`** (NOT committed)

```
PI_ZG_INFT_CONTRACT_ADDRESS=0x<the address from Step 4.3.1>
```

### 4.6: Commit + tag M7-D.1 done

- [ ] **Step 4.6.1: Commit broadcast artifacts (optional but useful)**

The Foundry deploy creates `contracts/broadcast/Deploy.s.sol/16602/run-latest.json` with the deploy receipt. By default `broadcast/` is gitignored. We can either:
- Leave it gitignored (typical Foundry convention; deploy receipts are local artifacts).
- Force-add the specific run file as a permanent record.

Recommend **force-add** for the canonical deploy: `git add -f contracts/broadcast/Deploy.s.sol/16602/run-latest.json` so future devs/judges can see the exact deploy tx.

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add .env.example
git add -f contracts/broadcast/Deploy.s.sol/16602/run-latest.json   # canonical deploy record
git commit -m "phase(M7-D.1.4): deploy EraPersonaINFT to 0G Galileo testnet; capture address + canonical broadcast"
git tag m7d1-done
```

(If `broadcast/` is fine to leave gitignored — submission-wise the contract address in `.env.example` + 0G explorer URL are sufficient proof — skip the force-add. Either is acceptable.)

---

## Live gate summary (M7-D.1 acceptance)

When this milestone is done:

1. `forge build` from `contracts/` succeeds.
2. `forge test -vv` from `contracts/` passes 10 tests.
3. Real testnet deploy:
   - Contract address visible on `https://chainscan-galileo.0g.ai/address/<addr>`.
   - 3 Transfer events for tokenIds 0/1/2 with `from=0x0`.
   - Contract owner = deployer wallet.
   - `cast call ... tokenURI(0)` returns the planner URL; same for 1+2.
4. `.env.example` documents `PI_ZG_INFT_CONTRACT_ADDRESS` (placeholder value); user's local `.env` has the real address.
5. Existing era + era-brain Go tests still pass — no regressions (we didn't touch Go code in M7-D.1).

---

## Out of scope (deferred to M7-D.2 and beyond)

- **Go client + abigen bindings** — M7-D.2's Phase 0.
- **era orchestrator wiring** (`q.SetINFT`, `RecordInvocation` calls) — M7-D.2's Phase 2.
- **`recordInvocation` event filtering / verification UI** — judges can use 0G explorer's logs tab; we don't ship our own viewer.
- **`Mint` and `Lookup` Provider methods** — M7-D.3+ scope; M7-D.2 returns `ErrNotImplemented`.
- **Royalty splits** — out per Q6 decision.

---

## Risks + cuts list (in order if slipping)

1. **`forge install OpenZeppelin/openzeppelin-contracts` fails** (network / git error). Recovery: retry; if persistent, install via npm into `node_modules/` and remap `openzeppelin-contracts/=node_modules/@openzeppelin/contracts/` in foundry.toml. Slower path; document if used.
2. **OZ v5 ERC721 `_requireOwned` not exported** (in case OZ ships a v5.x with it private). Recovery: replace with explicit `if (_ownerOf(tokenId) == address(0)) revert ERC721NonexistentToken(tokenId);` using ERC721's custom-error import.
3. **Testnet reverts on deploy** (gas estimation off, or some 0G-specific tx config). Recovery: try `--legacy` / `--gas-price <fixed>` / `--gas-limit 5000000`. If still failing, deploy via cast w/ raw bytecode as fallback.
4. **Faucet drained** during testing. Recovery: wait 24h for refill (0.1 ZG/day cap from M7-C.1.0 experience). Alternative: ask 0G hackathon channels for top-up.
5. **0G Galileo explorer slow to index**. Recovery: poll for ~5 min; if no events visible after 10 min, double-check tx hash via `cast tx <hash> --rpc-url $PI_ZG_EVM_RPC` directly.

---

## Notes for implementer

- This is the project's FIRST Solidity milestone. No prior Solidity files exist. The `contracts/` directory is fully self-contained (own `foundry.toml`, own deps via submodule, own `.gitignore`). The era Go module + era-brain Go module are completely untouched — `go test -race ./...` from repo root must stay green throughout.
- Foundry's TDD is genuinely test-first-friendly: `forge test` runs in ~50ms after the first compile, so the fail→impl→pass loop is fast.
- The `vm.expectEmit` pattern in Phase 2's tests asserts only the indexed topics (we use `(true, true, true, false)` for the 3 indexed event params + non-indexed data). If that's strict enough, great. If we need to assert exact `ts`, swap the third bool to `true` and pass an explicit timestamp via `vm.warp(...)`.
- `vm.envUint` of a 0x-prefixed hex string works — confirmed in spec §3 review. No need to strip the prefix manually.
- The OZ v5 import paths are `openzeppelin-contracts/contracts/...` (the submodule's `contracts/` subdir contains all source). Don't mistype as `openzeppelin-contracts/...`.
- After Phase 4's live deploy, the contract address is permanent. If you discover a bug post-deploy, M7-D.1 redo means re-deploying to a NEW address (gas burn) + updating `.env`. Cheap on testnet but a hassle.
- Do NOT push to GitHub until the `metadata/*.json` URLs resolve (they reference `master` branch). Push order: (a) commit Phase 3 + 4 to local master, (b) push to GitHub, (c) verify the raw URLs return 200, (d) ONLY THEN does the deploy script's tokenURI become resolvable for judges. Suggestion: deploy + verify locally first; push to GitHub last. Or push earlier and deploy AFTER push — judges will hit the live URLs.
