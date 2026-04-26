# M7-E — ENS Subname Resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire era's orchestrator to register 3 ENS subnames (`planner`/`coder`/`reviewer` under `vaibhav-era.eth` on Sepolia) at startup with text records pointing at the iNFT contract, and have the Telegram completion DM include a "personas:" footer with live-resolved ENS data per task.

**Architecture:** Five linear phases. Phase 0 generates abigen bindings from minimal mock Solidity contracts whose ABIs match the real ENS NameWrapper + PublicResolver subset we use (`setSubnodeRecord`, `ownerOf`, `setText`, `text`). Phase 1 wraps the bindings in `era-brain/identity/ens.Provider` satisfying `era-brain/identity.Resolver`. Phase 2 adds an `ENSResolver` interface + footer helper to `tgNotifier` in `cmd/orchestrator/main.go`. Phase 3 wires env-conditional `ens.New` + per-persona sync at boot. Phase 4 is the live Telegram gate.

**Tech Stack:** Go 1.25, go-ethereum's `abigen` + `ethclient` + `accounts/abi/bind` + `ethclient/simulated` (for unit tests), Foundry for compiling mock contracts, ENS NameWrapper at `0x0635513f179D50A207757E05759CbD106d7dFcE8` and PublicResolver at `0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5` on Sepolia (chainID 11155111). No new external dependencies.

**Spec:** `docs/superpowers/specs/2026-04-27-m7e-ens-design.md`. All §-references below point at the spec.

**Testing philosophy:** Strict TDD. Failing test first, run, verify FAIL, write minimal Go, run, verify PASS, commit. `go test -race -count=1 ./...` from repo root green at every commit. Live testnet gate at the end (Phase 4). `ens_live`-tagged tests skip in CI; only run when env vars present.

**Prerequisites (check before starting):**
- M7-D.2 done (tag `m7d2-done`).
- `vaibhav-era.eth` registered + wrapped on Sepolia ENS to the `PI_ZG_PRIVATE_KEY` wallet.
- Pre-flight check passes:
  ```bash
  cast call 0x0635513f179D50A207757E05759CbD106d7dFcE8 \
    'ownerOf(uint256)(address)' \
    $(cast namehash vaibhav-era.eth) \
    --rpc-url $PI_ENS_RPC
  ```
  must return signer address (not `0x0...0`).
- Sepolia wallet funded with ≥ 0.01 ETH (Google Cloud / Alchemy faucet).
- `.env` populated with `PI_ENS_RPC`, `PI_ENS_PARENT_NAME=vaibhav-era.eth`. `PI_ZG_PRIVATE_KEY` reused.
- `jq` + `abigen v1.17.2` + `forge` installed (already verified during M7-D milestones).
- Existing era + era-brain Go tests still pass (non-regression baseline).

---

## File Structure

```
contracts/test/                                                     CREATE (Phase 0)
├── MockNameWrapper.sol                                             CREATE — minimal subset matching real ENS NameWrapper ABI
└── MockPublicResolver.sol                                          CREATE — minimal subset matching real ENS PublicResolver ABI

Makefile                                                            MODIFY (Phase 0) — add `make abigen-ens` target

era-brain/identity/ens/                                             CREATE (Phases 0, 1)
├── ens.go                                                          CREATE (Phase 1) — Provider impl + namehash
├── ens_test.go                                                     CREATE (Phase 1) — unit tests via simulated.Backend + mock contracts
├── ens_live_test.go                                                CREATE (Phase 1) — //go:build ens_live
└── bindings/
    ├── name_wrapper.go                                             CREATE (Phase 0) — abigen output for MockNameWrapper
    └── public_resolver.go                                          CREATE (Phase 0) — abigen output for MockPublicResolver

cmd/orchestrator/main.go                                            MODIFY (Phases 2, 3) — ENSResolver interface, tgNotifier.ens field, ensFooter helper, ensEnabled() + boot wiring
cmd/orchestrator/notifier_ens_test.go                               CREATE (Phase 2) — unit tests for ensFooter rendering
```

No changes to `internal/queue/`, `era-brain/inft/`, or any contract under `contracts/src/`. The `identity.Resolver` interface stub from M7-A.2 stays untouched — Phase 1's Provider just satisfies it.

---

## Phase 0: Mock Solidity contracts + abigen bindings

**Files:**
- Create: `contracts/test/MockNameWrapper.sol`
- Create: `contracts/test/MockPublicResolver.sol`
- Create: `era-brain/identity/ens/bindings/name_wrapper.go` (abigen output)
- Create: `era-brain/identity/ens/bindings/public_resolver.go` (abigen output)
- Modify: `Makefile` — add `make abigen-ens` target

The real Sepolia NameWrapper + PublicResolver are 2k+ LoC with deep inheritance. Deploying them in `simulated.Backend` for unit tests is a half-day yak-shave. Instead, write minimal mock contracts whose ABI matches the subset we call (`setSubnodeRecord`, `ownerOf`, `setText`, `text`). Generate Go bindings from the mocks. The bindings work against both the simulated mock and the real Sepolia contracts (same ABI, same call signatures).

### Step 0.1: Verify forge + jq + abigen

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
which forge && forge --version
which jq && jq --version
which abigen && abigen --version
```

Expected: all three print versions. If `abigen` missing: `go install github.com/ethereum/go-ethereum/cmd/abigen@v1.17.2`.

### Step 0.2: Write `MockNameWrapper.sol`

`contracts/test/MockNameWrapper.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

// MockNameWrapper — minimal stand-in for ENS NameWrapper used in unit tests.
// Implements ONLY the subset of methods era-brain/identity/ens.Provider calls:
// - setSubnodeRecord(parentNode, label, owner, resolver, ttl, fuses, expiry)
// - ownerOf(tokenId)
//
// Function signatures (parameter types + names) MUST match the real
// NameWrapper at 0x0635513f179D50A207757E05759CbD106d7dFcE8 on Sepolia, so
// abigen output works against both.
contract MockNameWrapper {
    // tokenId = uint256(node) where node is the ENS namehash bytes32.
    mapping(uint256 => address) private _owners;

    // Mint helper for tests: register parentNode as owned by msg.sender.
    function testMint(bytes32 parentNode, address to) external {
        _owners[uint256(parentNode)] = to;
    }

    function ownerOf(uint256 tokenId) external view returns (address) {
        return _owners[tokenId];
    }

    function setSubnodeRecord(
        bytes32 parentNode,
        string calldata label,
        address owner,
        address /* resolver */,
        uint64 /* ttl */,
        uint32 /* fuses */,
        uint64 /* expiry */
    ) external returns (bytes32 node) {
        require(_owners[uint256(parentNode)] == msg.sender, "MockNameWrapper: not owner of parent");
        node = keccak256(abi.encodePacked(parentNode, keccak256(bytes(label))));
        _owners[uint256(node)] = owner;
        return node;
    }
}
```

### Step 0.3: Write `MockPublicResolver.sol`

`contracts/test/MockPublicResolver.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

// MockPublicResolver — minimal stand-in for ENS PublicResolver. Implements
// only setText / text. Function signatures match the real PublicResolver at
// 0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5 on Sepolia.
contract MockPublicResolver {
    mapping(bytes32 => mapping(string => string)) private _texts;

    function setText(bytes32 node, string calldata key, string calldata value) external {
        _texts[node][key] = value;
    }

    function text(bytes32 node, string calldata key) external view returns (string memory) {
        return _texts[node][key];
    }
}
```

### Step 0.4: Build the mock contracts

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era/contracts
forge build
ls out/MockNameWrapper.sol/MockNameWrapper.json
ls out/MockPublicResolver.sol/MockPublicResolver.json
```

Expected: both artifact files exist.

If `forge build` fails with "linter mixed-case variable" warnings: those are warnings (exit 0), proceed. If it fails with errors: re-check the .sol files for typos.

### Step 0.5: Add `abigen-ens` target to Makefile

Open the era root's `Makefile`. After the existing `abigen` target (the iNFT one from M7-D.2), add:

```make
.PHONY: abigen-ens
abigen-ens: ## Regenerate ENS abigen bindings from contracts/test mocks (ABIs match real Sepolia NameWrapper + PublicResolver subsets we use)
	@command -v jq >/dev/null || { echo "ERROR: jq not installed"; exit 1; }
	@command -v abigen >/dev/null || { echo "ERROR: abigen not installed (go install github.com/ethereum/go-ethereum/cmd/abigen@v1.17.2)"; exit 1; }
	cd contracts && forge build
	mkdir -p era-brain/identity/ens/bindings
	jq '.abi' contracts/out/MockNameWrapper.sol/MockNameWrapper.json > /tmp/era_namewrapper.abi
	jq -r '.bytecode.object' contracts/out/MockNameWrapper.sol/MockNameWrapper.json | sed 's/^0x//' > /tmp/era_namewrapper.bin
	abigen --abi /tmp/era_namewrapper.abi --bin /tmp/era_namewrapper.bin --pkg bindings --type NameWrapper \
	  --out era-brain/identity/ens/bindings/name_wrapper.go
	jq '.abi' contracts/out/MockPublicResolver.sol/MockPublicResolver.json > /tmp/era_resolver.abi
	jq -r '.bytecode.object' contracts/out/MockPublicResolver.sol/MockPublicResolver.json | sed 's/^0x//' > /tmp/era_resolver.bin
	abigen --abi /tmp/era_resolver.abi --bin /tmp/era_resolver.bin --pkg bindings --type PublicResolver \
	  --out era-brain/identity/ens/bindings/public_resolver.go
	@echo "ENS bindings regenerated."
```

(Use real tabs, not spaces, in Makefile recipes.)

### Step 0.6: Run `make abigen-ens`

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
make abigen-ens
```

Expected output ends with `ENS bindings regenerated.` Both `era-brain/identity/ens/bindings/name_wrapper.go` and `public_resolver.go` exist.

### Step 0.7: Verify generated bindings compile

```bash
cd era-brain
go build ./identity/ens/bindings/...
```

Expected: exit 0.

Sanity-check Deploy + ABI methods exist:
```bash
grep -E "DeployNameWrapper\b|func.*NameWrapper.*SetSubnodeRecord\b|func.*NameWrapper.*OwnerOf\b" identity/ens/bindings/name_wrapper.go | head
grep -E "DeployPublicResolver\b|func.*PublicResolver.*SetText\b|func.*PublicResolver.*Text\b" identity/ens/bindings/public_resolver.go | head
```

Expected: each grep returns hits. If `Deploy*` is missing, the Makefile didn't pass `--bin` correctly — re-check Step 0.5.

### Step 0.8: Run all era + era-brain tests (no regression)

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
cd era-brain
go vet ./...
go test -race -count=1 ./...
```

Both green.

### Step 0.9: Commit

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add contracts/test/ era-brain/identity/ens/bindings/ Makefile
git commit -m "phase(M7-E.0): mock NameWrapper + PublicResolver contracts + abigen bindings + Makefile target"
git tag m7e-0-bindings
```

NO `Co-Authored-By` trailer per `~/.claude/CLAUDE.md`. NO `--author`.

---

## Phase 1: `ens.Provider` impl + tests

**Files:**
- Create: `era-brain/identity/ens/ens.go`
- Create: `era-brain/identity/ens/ens_test.go` (4 unit tests via simulated.Backend + mock contracts)
- Create: `era-brain/identity/ens/ens_live_test.go` (build-tag `ens_live`)

`Provider` satisfies `era-brain/identity.Resolver` (existing interface stub from M7-A.2). The interface defines whatever signatures M7-A.2 stubbed — verify by reading `era-brain/identity/provider.go` first; if the Resolver interface signature differs from `New/Close/EnsureSubname/SetTextRecord/ReadTextRecord/ParentName`, adapt.

### 1A: Failing unit test

- [ ] **Step 1.1: Replace the M7-A.2 `identity.Resolver` stub with the granular API**

The existing `era-brain/identity/provider.go` declares a stub `Resolver` interface with `Resolve(name) (Resolution, error)` + `RegisterSubname(parent, label, res)`. That stub was never satisfied by anything (M7-A.2 just defined it as a placeholder). M7-E.1 is the real impl, and the granular API in this plan (`EnsureSubname`, `SetTextRecord`, `ReadTextRecord`, `ParentName`) is more useful for testing + reuse. **Rewrite the interface now**:

```bash
cat > era-brain/identity/provider.go <<'EOF'
// Package identity defines the Resolver interface for persona name → metadata
// lookup + subname management. Reference impl in identity/ens lands in M7-E.
package identity

import "context"

// Resolver registers ENS subnames + reads/writes their text records.
// Implementations: identity/ens.Provider (M7-E.1).
type Resolver interface {
	// EnsureSubname registers <label>.<parent> if not already owned by signer.
	// Idempotent.
	EnsureSubname(ctx context.Context, label string) error

	// SetTextRecord overwrites a text record. Idempotent: skips tx if value matches.
	SetTextRecord(ctx context.Context, label, key, value string) error

	// ReadTextRecord returns "" with nil error when key is unset.
	ReadTextRecord(ctx context.Context, label, key string) (string, error)

	// ParentName returns the configured parent ENS name, e.g. "vaibhav-era.eth".
	ParentName() string
}
EOF
```

Verify nothing else in era-brain references the old interface or `Resolution` struct:
```bash
grep -rn "identity.Resolver\|identity.Resolution" era-brain/ | grep -v "identity/provider.go"
```
Expected: zero hits (the M7-A.2 stub had no consumers). If any consumer exists, adapt or report.

Run tests + vet to confirm the rewrite compiles:
```bash
cd era-brain
go vet ./identity/...
go test -race ./identity/...
```
Both green (the package has no tests yet — `go test` exits 0 with "no test files").

- [ ] **Step 1.2: Write the failing test**

`era-brain/identity/ens/ens_test.go`:

```go
package ens_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/require"

	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens"
	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens/bindings"
)

// deployMocksOnSim deploys MockNameWrapper + MockPublicResolver on a simulated
// chain and mints `parentName` as owned by deployer.
func deployMocksOnSim(t *testing.T, parentName string) (
	*simulated.Backend,
	*bindings.NameWrapper, common.Address,
	*bindings.PublicResolver, common.Address,
	*bind.TransactOpts, *ecdsa.PrivateKey,
) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	deployer := crypto.PubkeyToAddress(key.PublicKey)
	alloc := types.GenesisAlloc{
		deployer: {Balance: new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))},
	}
	backend := simulated.NewBackend(alloc)
	t.Cleanup(func() { _ = backend.Close() })

	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(1337))
	require.NoError(t, err)

	nwAddr, _, nw, err := bindings.DeployNameWrapper(auth, backend.Client())
	require.NoError(t, err)
	backend.Commit()

	resAddr, _, res, err := bindings.DeployPublicResolver(auth, backend.Client())
	require.NoError(t, err)
	backend.Commit()

	// "Mint" parentName to deployer.
	parentNode, err := ens.Namehash(parentName)
	require.NoError(t, err)
	tx, err := nw.TestMint(auth, parentNode, deployer)
	require.NoError(t, err)
	backend.Commit()
	rc, err := bind.WaitMined(context.Background(), backend.Client(), tx)
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, rc.Status)

	return backend, nw, nwAddr, res, resAddr, auth, key
}

// newTestProvider constructs a Provider against the given simulated mocks.
// It uses the test override fields on Config to point at the mock addresses
// and reuses the existing simulated client (does NOT call ethclient.Dial).
func newTestProvider(t *testing.T, parentName string, key *ecdsa.PrivateKey, nwAddr, resAddr common.Address, backend *simulated.Backend) *ens.Provider {
	t.Helper()
	keyHex := common.Bytes2Hex(crypto.FromECDSA(key))
	p, err := ens.NewWithClient(ens.Config{
		ParentName:         parentName,
		PrivateKey:         keyHex,
		ChainID:            1337,
		NameWrapperAddress: nwAddr.Hex(),
		ResolverAddress:    resAddr.Hex(),
	}, backend.Client())
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

func TestNamehash_Vectors(t *testing.T) {
	cases := []struct {
		name string
		hex  string // expected namehash hex (no 0x prefix)
	}{
		{"", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"eth", "93cdeb708b7545dc668eb9280176169d1c33cfd8ed6f04690a0bcc88a93fc4ae"},
		{"foo.eth", "de9b09fd7c5f901e23a3f19fecc54828e9c848539801e86591bd9801b019f84f"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, err := ens.Namehash(c.name)
			require.NoError(t, err)
			require.Equal(t, c.hex, common.Bytes2Hex(h[:]))
		})
	}
}

func TestProvider_EnsureSubname_RegistersOnce(t *testing.T) {
	parentName := "vaibhav-era.eth"
	backend, nw, nwAddr, _, resAddr, auth, key := deployMocksOnSim(t, parentName)
	_ = nw
	_ = auth

	p := newTestProvider(t, parentName, key, nwAddr, resAddr, backend)

	// First call: subnode does not exist; expect a tx + commit.
	require.NoError(t, p.EnsureSubname(context.Background(), "planner"))
	backend.Commit()

	// Verify ownership of subnode is now the deployer.
	plannerNode, err := ens.Namehash("planner." + parentName)
	require.NoError(t, err)
	owner, err := nw.OwnerOf(&bind.CallOpts{}, new(big.Int).SetBytes(plannerNode[:]))
	require.NoError(t, err)
	deployer := crypto.PubkeyToAddress(key.PublicKey)
	require.Equal(t, deployer, owner, "subnode should be owned by signer")

	// Second call: idempotent — should NOT submit a new tx.
	require.NoError(t, p.EnsureSubname(context.Background(), "planner"))
	// (No backend.Commit needed; nothing was sent.)
}

func TestProvider_SetAndReadTextRecord(t *testing.T) {
	parentName := "vaibhav-era.eth"
	backend, _, nwAddr, _, resAddr, _, key := deployMocksOnSim(t, parentName)
	p := newTestProvider(t, parentName, key, nwAddr, resAddr, backend)

	require.NoError(t, p.EnsureSubname(context.Background(), "planner"))
	backend.Commit()

	// Empty before any write.
	v, err := p.ReadTextRecord(context.Background(), "planner", "inft_addr")
	require.NoError(t, err)
	require.Equal(t, "", v)

	// First write.
	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "inft_addr", "0xABCDEF"))
	backend.Commit()

	v, err = p.ReadTextRecord(context.Background(), "planner", "inft_addr")
	require.NoError(t, err)
	require.Equal(t, "0xABCDEF", v)

	// Idempotent: setting same value should NOT emit a tx (no Commit needed).
	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "inft_addr", "0xABCDEF"))

	// Overwrite with different value.
	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "inft_addr", "0x123456"))
	backend.Commit()
	v, err = p.ReadTextRecord(context.Background(), "planner", "inft_addr")
	require.NoError(t, err)
	require.Equal(t, "0x123456", v)
}

func TestProvider_ParentNameAndConfig(t *testing.T) {
	parentName := "vaibhav-era.eth"
	backend, _, nwAddr, _, resAddr, _, key := deployMocksOnSim(t, parentName)
	p := newTestProvider(t, parentName, key, nwAddr, resAddr, backend)
	require.Equal(t, parentName, p.ParentName())
}
```

- [ ] **Step 1.3: Run, verify FAIL**

```bash
mkdir -p era-brain/identity/ens
cd era-brain
go test ./identity/ens/... -v 2>&1 | head -40
```

Expected: build failure listing `undefined: ens.Provider`, `undefined: ens.Config`, `undefined: ens.NewWithClient`, `undefined: ens.Namehash`. Exit non-zero.

### 1B: Implement Provider + Namehash

- [ ] **Step 1.4: Write `ens.go`**

`era-brain/identity/ens/ens.go`:

```go
// Package ens is an identity.Resolver impl wrapping abigen bindings for the
// ENS NameWrapper + PublicResolver contracts on Sepolia. ABIs come from
// minimal mock contracts under contracts/test/ but match the real Sepolia
// contracts' subset of methods we use, so the same Go code works against
// both simulated.Backend (unit tests) and the real Sepolia chain (live test
// + production).
package ens

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens/bindings"
)

// Sepolia constants — used when Config.NameWrapperAddress / Config.ResolverAddress
// are empty (production). Override in tests with the simulated mock addresses.
const (
	SepoliaNameWrapper    = "0x0635513f179D50A207757E05759CbD106d7dFcE8"
	SepoliaPublicResolver = "0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5"
)

type Config struct {
	ParentName string
	RPCURL     string
	PrivateKey string
	ChainID    int64

	// Optional address overrides for tests. Leave empty in production.
	NameWrapperAddress string
	ResolverAddress    string
}

// ContractClient is the subset of *ethclient.Client + simulated.Client we need.
// Both satisfy bind.ContractBackend and bind.DeployBackend; in tests we pass a
// simulated.Client, in prod a *ethclient.Client.
type ContractClient interface {
	bind.ContractBackend
	bind.DeployBackend
}

type Provider struct {
	cfg        Config
	parentNode [32]byte

	client       ContractClient
	dialedClient *ethclient.Client // non-nil iff we own the dial; nil in tests

	nameWrapper *bindings.NameWrapper
	resolver    *bindings.PublicResolver
	auth        *bind.TransactOpts
	signer      common.Address
}

// New dials cfg.RPCURL and constructs a Provider. Caller must Close.
func New(cfg Config) (*Provider, error) {
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("ens dial: %w", err)
	}
	p, err := newWithBackend(cfg, client)
	if err != nil {
		client.Close()
		return nil, err
	}
	p.dialedClient = client
	return p, nil
}

// NewWithClient is a test entry point: skip dial, use the provided client
// (simulated.Client or *ethclient.Client). Production callers use New.
func NewWithClient(cfg Config, client ContractClient) (*Provider, error) {
	return newWithBackend(cfg, client)
}

func newWithBackend(cfg Config, client ContractClient) (*Provider, error) {
	parentNode, err := Namehash(cfg.ParentName)
	if err != nil {
		return nil, fmt.Errorf("ens namehash parent: %w", err)
	}

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("ens priv key: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(cfg.ChainID))
	if err != nil {
		return nil, fmt.Errorf("ens auth: %w", err)
	}

	nwAddrHex := cfg.NameWrapperAddress
	if nwAddrHex == "" {
		nwAddrHex = SepoliaNameWrapper
	}
	resAddrHex := cfg.ResolverAddress
	if resAddrHex == "" {
		resAddrHex = SepoliaPublicResolver
	}

	nw, err := bindings.NewNameWrapper(common.HexToAddress(nwAddrHex), client)
	if err != nil {
		return nil, fmt.Errorf("ens bind name_wrapper: %w", err)
	}
	res, err := bindings.NewPublicResolver(common.HexToAddress(resAddrHex), client)
	if err != nil {
		return nil, fmt.Errorf("ens bind resolver: %w", err)
	}

	signer := pubkeyAddr(privKey)

	return &Provider{
		cfg:         cfg,
		parentNode:  parentNode,
		client:      client,
		nameWrapper: nw,
		resolver:    res,
		auth:        auth,
		signer:      signer,
	}, nil
}

func pubkeyAddr(k *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(k.PublicKey)
}

func (p *Provider) Close() {
	if p.dialedClient != nil {
		p.dialedClient.Close()
	}
}

func (p *Provider) ParentName() string { return p.cfg.ParentName }

// EnsureSubname registers `<label>.<parent>` if not already owned by the signer.
// Idempotent: returns nil without sending a tx when the subnode already
// resolves to the signer in NameWrapper.
//
// Calls NameWrapper.setSubnodeRecord with expiry = max-uint64 (passing 0
// reverts on Sepolia NameWrapper when the parent has any fuses burned;
// max-uint64 lets the contract clamp to the parent's expiry internally).
func (p *Provider) EnsureSubname(ctx context.Context, label string) error {
	subnode, err := p.subnameNode(label)
	if err != nil {
		return err
	}
	tokenID := new(big.Int).SetBytes(subnode[:])

	owner, err := p.nameWrapper.OwnerOf(&bind.CallOpts{Context: ctx}, tokenID)
	if err != nil {
		return fmt.Errorf("ens ownerOf %s: %w", label, err)
	}
	if owner == p.signer {
		return nil // already registered to us — idempotent skip
	}

	auth := *p.auth
	auth.Context = ctx

	resAddrHex := p.cfg.ResolverAddress
	if resAddrHex == "" {
		resAddrHex = SepoliaPublicResolver
	}
	tx, err := p.nameWrapper.SetSubnodeRecord(
		&auth,
		p.parentNode,
		label,
		p.signer,
		common.HexToAddress(resAddrHex),
		uint64(0),    // ttl
		uint32(0),    // fuses
		^uint64(0),   // expiry — sentinel "use parent's"; NameWrapper clamps internally
	)
	if err != nil {
		return fmt.Errorf("ens setSubnodeRecord %s: %w", label, err)
	}
	_ = tx
	return nil
}

// SetTextRecord overwrites a text record. Idempotent: returns nil without
// sending a tx if the on-chain value already equals `value`.
func (p *Provider) SetTextRecord(ctx context.Context, label, key, value string) error {
	subnode, err := p.subnameNode(label)
	if err != nil {
		return err
	}
	current, err := p.resolver.Text(&bind.CallOpts{Context: ctx}, subnode, key)
	if err != nil {
		return fmt.Errorf("ens read text %s.%s: %w", label, key, err)
	}
	if current == value {
		return nil
	}

	auth := *p.auth
	auth.Context = ctx
	if _, err := p.resolver.SetText(&auth, subnode, key, value); err != nil {
		return fmt.Errorf("ens setText %s.%s: %w", label, key, err)
	}
	return nil
}

// ReadTextRecord returns the text record value, or "" with nil error when unset.
func (p *Provider) ReadTextRecord(ctx context.Context, label, key string) (string, error) {
	subnode, err := p.subnameNode(label)
	if err != nil {
		return "", err
	}
	v, err := p.resolver.Text(&bind.CallOpts{Context: ctx}, subnode, key)
	if err != nil {
		return "", fmt.Errorf("ens read text %s.%s: %w", label, key, err)
	}
	return v, nil
}

func (p *Provider) subnameNode(label string) ([32]byte, error) {
	if label == "" {
		return [32]byte{}, errors.New("ens: empty label")
	}
	return Namehash(label + "." + p.cfg.ParentName)
}

// Namehash computes the ENS namehash of `name` per ENSIP-1.
// Empty string → bytes32(0). Otherwise recursive keccak256 of (parent || keccak256(label)).
func Namehash(name string) ([32]byte, error) {
	var node [32]byte
	if name == "" {
		return node, nil
	}
	labels := strings.Split(name, ".")
	for i := len(labels) - 1; i >= 0; i-- {
		if labels[i] == "" {
			return node, fmt.Errorf("ens: empty label in %q", name)
		}
		labelHash := crypto.Keccak256([]byte(labels[i]))
		concat := append(node[:], labelHash...)
		next := crypto.Keccak256(concat)
		copy(node[:], next)
	}
	return node, nil
}
```

- [ ] **Step 1.5: Run unit tests, verify PASS**

```bash
cd era-brain
go test -race ./identity/ens/...
```

Expected: 4 tests pass (`Namehash_Vectors`, `Provider_EnsureSubname_RegistersOnce`, `Provider_SetAndReadTextRecord`, `Provider_ParentNameAndConfig`).

If `Namehash_Vectors` fails on a specific case: the expected hex strings in the test were copied from the ENS spec — recompute via `cast namehash <name>` to verify the test fixtures.

If `Provider_EnsureSubname_RegistersOnce` fails with "not owner of parent": the `TestMint` helper in `MockNameWrapper.sol` wasn't called correctly — re-check `deployMocksOnSim`.

### 1C: Live integration test

- [ ] **Step 1.6: Write build-tagged live test**

`era-brain/identity/ens/ens_live_test.go`:

```go
//go:build ens_live

package ens_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens"
)

func TestProvider_LiveSepolia_EnsureAndSetText(t *testing.T) {
	parentName := os.Getenv("PI_ENS_PARENT_NAME")
	rpc := os.Getenv("PI_ENS_RPC")
	privKey := os.Getenv("PI_ZG_PRIVATE_KEY")
	if parentName == "" || rpc == "" || privKey == "" {
		t.Skip("PI_ENS_PARENT_NAME / PI_ENS_RPC / PI_ZG_PRIVATE_KEY required")
	}

	p, err := ens.New(ens.Config{
		ParentName: parentName,
		RPCURL:     rpc,
		PrivateKey: privKey,
		ChainID:    11155111, // Sepolia
	})
	require.NoError(t, err)
	t.Cleanup(p.Close)

	require.Equal(t, parentName, p.ParentName())

	// EnsureSubname is idempotent — safe to call repeatedly.
	require.NoError(t, p.EnsureSubname(context.Background(), "planner"))

	// Round-trip a text record. Use a unique value so we know the write happened.
	val := "live-test-" + strings.Repeat("x", 8)
	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "live_test_marker", val))

	read, err := p.ReadTextRecord(context.Background(), "planner", "live_test_marker")
	require.NoError(t, err)
	require.Equal(t, val, read, "round-trip text record should match")
}
```

- [ ] **Step 1.7: Run live test against Sepolia (USES REAL ETH ~0.001-0.002)**

```bash
cd era-brain
set -a; source ../.env; set +a
go test -tags ens_live -v -run TestProvider_LiveSepolia ./identity/ens/... 2>&1 | tee /tmp/m7e-live.log
grep -E "^--- PASS: TestProvider_LiveSepolia_EnsureAndSetText" /tmp/m7e-live.log
```

Expected: `--- PASS:` printed; final grep exits 0.

If FAIL:
- "execution reverted" + parent-name-related → run pre-flight `cast call ... ownerOf $(cast namehash $PI_ENS_PARENT_NAME) ...` — must return signer. If 0x0, parent name not wrapped.
- "insufficient funds" → faucet up the wallet (Google Cloud / Alchemy Sepolia).
- "no contract code" → wrong address const for NameWrapper or PublicResolver — check `ens.go` constants vs ENS docs.
- Timeout / RPC 429 → public RPC throttling; switch `PI_ENS_RPC` to a different provider (Alchemy works well for Sepolia).

### 1D: Sanity sweep

- [ ] **Step 1.8: Run all era-brain tests (no `ens_live` tag)**

```bash
cd era-brain
go vet ./...
go test -race -count=1 ./...
```

Both green.

- [ ] **Step 1.9: Run all era root tests (no regression)**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
```

Both green.

- [ ] **Step 1.10: Commit**

```bash
git add era-brain/identity/ens/
git commit -m "phase(M7-E.1): ens.Provider — EnsureSubname + SetTextRecord + ReadTextRecord; Namehash helper; Sepolia + simulated.Backend"
git tag m7e-1-provider
```

---

## Phase 2: tgNotifier ENS footer + ENSResolver interface

**Files:**
- Modify: `cmd/orchestrator/main.go` — `ENSResolver` interface, `tgNotifier.ens` field, `ensFooter` helper, append footer in `NotifyCompleted` + `NotifyNeedsReview`
- Create: `cmd/orchestrator/notifier_ens_test.go` — unit tests for `ensFooter`

**Spec deviation (intentional):** Spec §3 placed `ENSResolver` + footer rendering inside `internal/queue/queue.go` (mirroring `INFTProvider` seam). This plan relocates them to `cmd/orchestrator/main.go`'s `tgNotifier` because the actual DM render code already lives there — `queue.go` only composes `CompletedArgs`/`NeedsReviewArgs` structs and hands them to the `Notifier` interface. Threading a queue-side ENSResolver through the notifier boundary buys nothing; putting reads where the rendering happens is simpler and keeps the queue clean. Document this in the commit message.

The DM rendering lives in `tgNotifier` (`cmd/orchestrator/main.go:284-380`). Phase 2 adds an `ens` field and a helper that reads the 3 personas' text records and formats a footer string. Production wires the real Provider in Phase 3; Phase 2 tests use a stub.

### 2A: Failing test for ensFooter

- [ ] **Step 2.1: Read the existing tgNotifier shape**

```bash
grep -n "type tgNotifier struct\|func (n \*tgNotifier) NotifyCompleted\|func (n \*tgNotifier) NotifyNeedsReview" cmd/orchestrator/main.go
```

Confirm: the struct has fields `client`, `chatID`, `sandboxRepo`, `repo`, `progressMsgs`. We're adding one more: `ens ENSResolver`.

- [ ] **Step 2.2: Write the failing test**

`cmd/orchestrator/notifier_ens_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubENS struct {
	parent string
	// Per (label, key) lookup. Returning "" with nil error = unset.
	values  map[string]string // key = "label:textKey"
	failKey string            // when set, ReadTextRecord returns error for this key
}

func (s *stubENS) ParentName() string { return s.parent }

func (s *stubENS) ReadTextRecord(_ context.Context, label, key string) (string, error) {
	if s.failKey != "" && key == s.failKey {
		return "", errStub
	}
	return s.values[label+":"+key], nil
}

var errStub = stubErr("stub: simulated read error")

type stubErr string

func (e stubErr) Error() string { return string(e) }

func TestEnsFooter_RendersAllThreePersonas(t *testing.T) {
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":      "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"planner:inft_token_id":  "0",
			"coder:inft_addr":        "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"coder:inft_token_id":    "1",
			"reviewer:inft_addr":     "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"reviewer:inft_token_id": "2",
		},
	}
	footer := ensFooter(context.Background(), stub)
	require.Contains(t, footer, "personas:")
	require.Contains(t, footer, "planner.vaibhav-era.eth")
	require.Contains(t, footer, "coder.vaibhav-era.eth")
	require.Contains(t, footer, "reviewer.vaibhav-era.eth")
	require.Contains(t, footer, "token #0")
	require.Contains(t, footer, "token #1")
	require.Contains(t, footer, "token #2")
	require.Contains(t, footer, "0x33847c")
	// Footer should start with a newline so it appends cleanly to existing body.
	require.True(t, strings.HasPrefix(footer, "\n\n"), "footer should start with double newline for separation")
}

func TestEnsFooter_NilResolverReturnsEmpty(t *testing.T) {
	footer := ensFooter(context.Background(), nil)
	require.Equal(t, "", footer)
}

func TestEnsFooter_ReadFailureReturnsEmpty(t *testing.T) {
	stub := &stubENS{
		parent:  "vaibhav-era.eth",
		values:  map[string]string{}, // anything fine
		failKey: "inft_addr",         // any read of inft_addr errors
	}
	footer := ensFooter(context.Background(), stub)
	require.Equal(t, "", footer, "any read failure should drop the entire footer to avoid partial DMs")
}

func TestEnsFooter_PartialDataReturnsEmpty(t *testing.T) {
	// One persona's records are missing — footer should be skipped entirely.
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":     "0xabc",
			"planner:inft_token_id": "0",
			// coder + reviewer records absent → empty strings → drop footer
		},
	}
	footer := ensFooter(context.Background(), stub)
	require.Equal(t, "", footer)
}
```

- [ ] **Step 2.3: Run, verify FAIL**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go test ./cmd/orchestrator/ -run TestEnsFooter -count=1 -v 2>&1 | head -30
```

Expected: build failure mentioning `undefined: ensFooter`, `undefined: ENSResolver`. Exit non-zero.

### 2B: Implement ENSResolver + ensFooter

- [ ] **Step 2.4: Add ENSResolver interface + tgNotifier.ens field + ensFooter helper**

In `cmd/orchestrator/main.go`, near the `tgNotifier` struct definition (around line 284):

```go
// ENSResolver is the notifier's view of the ENS provider — only the read +
// parent-name calls. Defined here so tests can inject a stub without pulling
// the full era-brain.identity.Resolver interface (writes are out of scope
// for the DM render path).
type ENSResolver interface {
	ReadTextRecord(ctx context.Context, label, key string) (string, error)
	ParentName() string
}
```

Modify `type tgNotifier struct` to add `ens ENSResolver // may be nil`:

```go
type tgNotifier struct {
	client       telegram.Client
	chatID       int64
	sandboxRepo  string
	repo         *db.Repo
	progressMsgs sync.Map
	ens          ENSResolver // may be nil — set by main when ENS is wired
}
```

Add the `ensFooter` helper — placed below the existing `tgNotifier` methods or near the message-formatting helpers:

```go
// ensFooter renders the "personas:" section appended to completion / review DMs.
// Returns "" when ens is nil OR any single read fails OR any persona's records
// are missing (partial data → empty string, never partial footer).
func ensFooter(ctx context.Context, ens ENSResolver) string {
	if ens == nil {
		return ""
	}
	type row struct{ label, addr, tokenID string }
	rows := make([]row, 0, 3)
	for _, label := range []string{"planner", "coder", "reviewer"} {
		addr, err := ens.ReadTextRecord(ctx, label, "inft_addr")
		if err != nil {
			return ""
		}
		tokenID, err := ens.ReadTextRecord(ctx, label, "inft_token_id")
		if err != nil {
			return ""
		}
		if addr == "" || tokenID == "" {
			return ""
		}
		rows = append(rows, row{label: label, addr: addr, tokenID: tokenID})
	}
	var b strings.Builder
	b.WriteString("\n\npersonas:")
	parent := ens.ParentName()
	for _, r := range rows {
		// Truncate addr to first 10 chars for readability: "0x33847c5500..."
		shortAddr := r.addr
		if len(shortAddr) > 12 {
			shortAddr = shortAddr[:12] + "…"
		}
		fmt.Fprintf(&b, "\n  %s.%s → token #%s (%s)", r.label, parent, r.tokenID, shortAddr)
	}
	return b.String()
}
```

Append `+ ensFooter(ctx, n.ens)` to the `body` in **both** `NotifyCompleted` and `NotifyNeedsReview` immediately before the `n.client.SendMessage` / `SendMessageWithButtons` call.

For `NotifyCompleted` (around line 318), change:
```go
	body += fmt.Sprintf("\n\ntokens: %d · cost: $%.4f", a.Tokens, float64(a.CostCents)/100)
	// ... (existing planner/reviewer lines)
```
to:
```go
	body += fmt.Sprintf("\n\ntokens: %d · cost: $%.4f", a.Tokens, float64(a.CostCents)/100)
	// ... (existing planner/reviewer lines)
	body += ensFooter(ctx, n.ens)
```

For `NotifyNeedsReview`, find `body := formatNeedsReviewMessage(a)` and modify:
```go
	body := formatNeedsReviewMessage(a) + ensFooter(ctx, n.ens)
```

(Read the file to confirm exact insertion points; `body` is a local string that gets sent.)

### 2C: Verify

- [ ] **Step 2.5: Run, verify PASS**

```bash
go test ./cmd/orchestrator/ -run TestEnsFooter -count=1 -v
```

Expected: 4 tests pass.

- [ ] **Step 2.6: Run all era tests (no regression)**

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

All green.

- [ ] **Step 2.7: Commit**

```bash
git add cmd/orchestrator/
git commit -m "phase(M7-E.2): tgNotifier ENS footer — ENSResolver interface + ensFooter helper appended to completion + needs_review DMs"
git tag m7e-2-notifier-footer
```

---

## Phase 3: Orchestrator boot wiring

**Files:**
- Modify: `cmd/orchestrator/main.go` — `ensEnabled()` helper, env-conditional `ens.New` + `syncPersonaENS` loop + `n.ens` assignment

### Step 3.1: Read existing env-helper pattern

```bash
grep -n "zgEnabled\|zgComputeEnabled\|zgINFTEnabled\|q.SetSwarm\|q.Reconcile\|notifier := \|q.SetNotifier\|tgNotifier{" cmd/orchestrator/main.go | head
```

**Insertion point matters.** In `main.go`:
- Existing `if zgINFTEnabled() { ... }` block (lines ~185-196) runs BEFORE `q.Reconcile(ctx)` (line 201).
- `notifier := &tgNotifier{...}` is constructed at line ~211 (AFTER `Reconcile`).
- `q.SetNotifier(notifier)` follows shortly after.

The ENS block needs to assign `notifier.ens = ensProv`, so it MUST go AFTER `notifier := &tgNotifier{...}` is constructed. Place the ENS block **immediately after** the `notifier := &tgNotifier{...}` literal and **before** `q.SetNotifier(notifier)` (or wherever the notifier first gets used). Nothing in `syncPersonaENS` needs to run before `Reconcile` — the order doesn't affect queue replay logic.

### Step 3.2: Add `ensEnabled` helper

Near existing `zgEnabled()` / `zgComputeEnabled()` / `zgINFTEnabled()`:

```go
// ensEnabled returns true when the ENS parent name + Sepolia RPC are configured
// AND a private key is available for tx signing.
func ensEnabled() bool {
	return os.Getenv("PI_ENS_RPC") != "" &&
		os.Getenv("PI_ENS_PARENT_NAME") != "" &&
		os.Getenv("PI_ZG_PRIVATE_KEY") != ""
}
```

### Step 3.3: Add zg-storage URI constants

Near the top of `main.go` (or just before the boot wiring), add the 3 hardcoded persona metadata URIs (per spec §3 + §8 — committed under `contracts/metadata/` from M7-D.1):

```go
const (
	plannerZGURI  = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/planner.json"
	coderZGURI    = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/coder.json"
	reviewerZGURI = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/reviewer.json"
)
```

Verify the file paths exist in `contracts/metadata/`:
```bash
ls contracts/metadata/
```

If filenames differ (e.g., `planner-persona.json`), adjust the constants to match.

### Step 3.4: Add the env-conditional wiring block

Insert IMMEDIATELY AFTER the `notifier := &tgNotifier{...}` construction and BEFORE the queue starts processing (i.e., before `q.SetNotifier(notifier)` if that's the next call, or before any goroutine that uses the notifier):

```go
if ensEnabled() {
	ensProv, err := ens.New(ens.Config{
		ParentName: os.Getenv("PI_ENS_PARENT_NAME"),
		RPCURL:     os.Getenv("PI_ENS_RPC"),
		PrivateKey: os.Getenv("PI_ZG_PRIVATE_KEY"),
		ChainID:    11155111, // Sepolia
	})
	if err != nil {
		// ENS is decorative; Sepolia public RPC flakes more than 0G's RPC.
		// Log + continue without ENS instead of aborting boot.
		slog.Error("ens disabled — boot continues without ENS", "err", err)
	} else {
		defer ensProv.Close()

		inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
		for _, p := range []struct{ label, tokenID, zgURI string }{
			{"planner", "0", plannerZGURI},
			{"coder", "1", coderZGURI},
			{"reviewer", "2", reviewerZGURI},
		} {
			if err := syncPersonaENS(ctx, ensProv, p.label, p.tokenID, inftAddr, p.zgURI); err != nil {
				slog.Warn("ens sync failed", "label", p.label, "err", err)
			}
		}
		notifier.ens = ensProv // wire into tgNotifier so DM footer can read records
		slog.Info("ENS resolver wired", "parent", os.Getenv("PI_ENS_PARENT_NAME"))
	}
}
```

The `notifier.ens = ...` assignment happens after the notifier is constructed — that's the whole reason this block can't go above `notifier := &tgNotifier{...}`. Confirm by reading the surrounding code; if the notifier variable is named differently (e.g., `n` instead of `notifier`), adapt accordingly.

### Step 3.5: Implement `syncPersonaENS`

Add as a top-level helper in `main.go`:

```go
// syncPersonaENS registers the subname and writes 3 text records for a single
// persona. Each step is independently idempotent — re-running this on a fully
// synced subname produces 0 on-chain txs (just 4 RPC reads).
func syncPersonaENS(ctx context.Context, p *ens.Provider, label, tokenID, inftAddr, zgURI string) error {
	if err := p.EnsureSubname(ctx, label); err != nil {
		return fmt.Errorf("ensureSubname: %w", err)
	}
	if err := p.SetTextRecord(ctx, label, "inft_addr", inftAddr); err != nil {
		return fmt.Errorf("set inft_addr: %w", err)
	}
	if err := p.SetTextRecord(ctx, label, "inft_token_id", tokenID); err != nil {
		return fmt.Errorf("set inft_token_id: %w", err)
	}
	if err := p.SetTextRecord(ctx, label, "zg_storage_uri", zgURI); err != nil {
		return fmt.Errorf("set zg_storage_uri: %w", err)
	}
	return nil
}
```

### Step 3.6: Add the `ens` import

At the top of `main.go` add:
```go
"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens"
```

### Step 3.7: Build, verify compile

```bash
go build ./...
```

Expected: exit 0.

### Step 3.8: Run all tests + vet (no regression)

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green.

### Step 3.9: Commit

```bash
git add cmd/orchestrator/main.go
git commit -m "phase(M7-E.3): orchestrator wires ens.Provider when PI_ENS_RPC + PI_ENS_PARENT_NAME set; syncs 3 personas at boot; non-fatal on Sepolia failure"
git tag m7e-3-orchestrator-wiring
```

---

## Phase 4: Live gate — real Telegram /task

**Files:** none modified. Verification only.

### Step 4.1: Pre-flight — confirm parent name is owned + wrapped

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
cast call 0x0635513f179D50A207757E05759CbD106d7dFcE8 \
  'ownerOf(uint256)(address)' \
  $(cast namehash $PI_ENS_PARENT_NAME) \
  --rpc-url $PI_ENS_RPC
```

Expected: returns the signer address (the wallet derived from `PI_ZG_PRIVATE_KEY`). To find that address:
```bash
cast wallet address $PI_ZG_PRIVATE_KEY
```

If `ownerOf` returns `0x0000000000000000000000000000000000000000`: the name is not wrapped. Open https://sepolia.app.ens.domains/$PI_ENS_PARENT_NAME and click "Wrap" → confirm tx. Wait for it to land, re-run the cast call.

### Step 4.2: Confirm Sepolia balance ≥ 0.01 ETH

```bash
cast balance $(cast wallet address $PI_ZG_PRIVATE_KEY) --rpc-url $PI_ENS_RPC --ether
```

Expected: ≥ 0.01. If lower, faucet at https://cloud.google.com/application/web3/faucet/ethereum/sepolia.

### Step 4.3: Build orchestrator binary

```bash
go build -o bin/orchestrator ./cmd/orchestrator
ls -lh bin/orchestrator
```

### Step 4.4: Stop VPS bot

```bash
ssh era@178.105.44.3 sudo systemctl stop era
ssh era@178.105.44.3 systemctl is-active era
```

Expect `inactive` from the second command (and exit code 3). Don't proceed until confirmed.

### Step 4.5: Start local orchestrator

```bash
set -a; source .env; set +a
./bin/orchestrator
```

**Expected NEW boot lines (vs M7-D.2):**
- `INFO ENS resolver wired parent=vaibhav-era.eth`
- 3 Sepolia txs may be visible in stdout if subnames not yet registered (first boot only). Subsequent boots: 0 Sepolia txs (idempotent).

If `ens disabled — boot continues without ENS` appears: env vars wrong OR Sepolia RPC unreachable. Boot continues; ENS footer will be absent in DMs.

### Step 4.6: Send a /task via Telegram

```
/task add a /version endpoint that returns the current commit SHA
```

### Step 4.7: Watch the orchestrator stdout + Telegram

Expected (additive to M7-D.2):
- 6 0G txs per task (4 storage + 2 iNFT) — unchanged from M7-D.2
- Possibly 1-3 Sepolia eth_call requests during DM render (small log volume; visible at debug level if any)
- Telegram completion DM ends with:
  ```
  personas:
    planner.vaibhav-era.eth → token #0 (0x33847c5500…)
    coder.vaibhav-era.eth → token #1 (0x33847c5500…)
    reviewer.vaibhav-era.eth → token #2 (0x33847c5500…)
  ```

### Step 4.8: Verify subnames on the ENS app

Open in browser:
```
https://sepolia.app.ens.domains/planner.vaibhav-era.eth
```

Expected: page shows the subname with 3 text records (`inft_addr`, `inft_token_id`, `zg_storage_uri`). Repeat for `coder.` and `reviewer.`.

Cross-check via `cast`:
```bash
cast call 0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5 \
  'text(bytes32,string)(string)' \
  $(cast namehash planner.$PI_ENS_PARENT_NAME) \
  inft_addr \
  --rpc-url $PI_ENS_RPC
```
Expected: `0x33847c5500C2443E2f3BBf547d9b069B334c3D16`.

### Step 4.9: Verify "real resolution work" — edit on chain, see DM update without restart

(Optional but the strongest evidence for the prize criterion.)

Manually setText a different value via `cast send` for `planner`'s `inft_addr`:
```bash
cast send 0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5 \
  'setText(bytes32,string,string)' \
  $(cast namehash planner.$PI_ENS_PARENT_NAME) \
  inft_addr \
  "0xDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF" \
  --rpc-url $PI_ENS_RPC \
  --private-key $PI_ZG_PRIVATE_KEY
```

Send another `/task`. The DM footer should show the new `0xDEADBE…` for planner (proves footer values are read live, not cached).

Restore the original value before continuing (or leave the orchestrator's idempotent reconcile to fix it on next restart):
```bash
cast send 0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5 \
  'setText(bytes32,string,string)' \
  $(cast namehash planner.$PI_ENS_PARENT_NAME) \
  inft_addr \
  "0x33847c5500C2443E2f3BBf547d9b069B334c3D16" \
  --rpc-url $PI_ENS_RPC \
  --private-key $PI_ZG_PRIVATE_KEY
```

### Step 4.10: Restart VPS

```bash
ssh era@178.105.44.3 sudo systemctl start era
```

**Don't skip.** Production bot stays offline until you do.

### Step 4.11: Stop local orchestrator (Ctrl-C)

### Step 4.12: Replay tests

```bash
go vet ./... && go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./...
```

Both green.

### Step 4.13: Tag M7-E done

```bash
git tag m7e-done
```

(no commit — Phase 4 is verification only)

---

## Live gate summary (M7-E acceptance)

When this milestone is done:

1. `go build ./...` from repo root succeeds.
2. `go test -race -count=1 ./...` from repo root green; no regression.
3. Real `/task` on a real repo:
   - Orchestrator boot logs show `ENS resolver wired parent=vaibhav-era.eth`.
   - Telegram completion DM ends with a `personas:` footer listing all 3 subnames + their resolved tokens + addresses.
   - DM footer values come from a live Sepolia read at DM-render time (Step 4.9 confirms this).
4. The 3 subnames are visible at https://sepolia.app.ens.domains/planner.vaibhav-era.eth (and `coder.`, `reviewer.`) with 3 text records each.
5. Without ENS env vars OR with Sepolia RPC unreachable, orchestrator falls back to no-ENS mode — M7-D.2 baseline DM unchanged.
6. Repeated boots produce 0 Sepolia txs (per-key idempotency: writes only fire when on-chain value differs).
7. VPS M6 era is restarted after the live gate.

---

## Out of scope (deferred)

- **`/personas` Telegram command.** Defer to M7-F if pursued.
- **ENS Most Creative track web page** (subscriber to PublicResolver `TextChanged` events). Stretch only — track separately if budget after M7-E.
- **Reverse resolution (address → ENS name).** Not needed for prize criterion.
- **Custom wildcard resolver contract.** Master spec mentioned wildcards; we chose explicit subnames per spec §8.
- **Mainnet ENS.** Sepolia only.
- **Audit-log event kinds for ENS read/write failures** (`ens_recorded`, `ens_read_failed`). slog only.

---

## Risks + cuts list (in order if slipping)

1. **Identity.Resolver interface from M7-A.2 has different method names than spec assumes.** Recovery: adapt Provider's exported method names to satisfy the existing interface; rename in tests accordingly. Should be ~5 min.
2. **Real Sepolia NameWrapper has fuse semantics that reject our `setSubnodeRecord` call.** Recovery: the `expiry=max-uint64` mitigation should handle it (NameWrapper clamps to parent's expiry). If still reverts, fall back to legacy `ENS.setSubnodeOwner` + `ENS.setResolver` two-tx path (re-abigen ENSRegistry; ~1 hour).
3. **Public Sepolia RPC throttles or 429s during live test.** Recovery: switch `PI_ENS_RPC` to Alchemy free-tier (`https://eth-sepolia.g.alchemy.com/v2/<key>`); retry.
4. **`Namehash` test vectors are wrong.** Recovery: regenerate via `cast namehash <name>`; common bug is empty-label handling at the start of recursion.
5. **Bindings package name conflicts** (`bindings` is also used by `era-brain/inft/zg_7857/bindings/`). Both packages live in different directories with different import paths, so this is fine as long as imports are explicit. Don't rename.
6. **DM footer pushes message past Telegram's 4096-char cap** for very long task summaries. Recovery: footer is ≤200 chars; existing `truncateForTelegram` budget (~3500 chars) leaves headroom. Not a real risk in practice.
7. **Idempotent EnsureSubname misses a stale resolver** (e.g., subnode owned by us but resolver pointing somewhere else). Recovery: spec accepts this — first-time write fixes; the "owner == us" check is the strongest signal we have without more reads. Cuts-list: add a resolver-equality check if it bites.

---

## Notes for implementer

- The abigen-generated `bindings/name_wrapper.go` and `bindings/public_resolver.go` are LARGE files. Don't read carefully — verify they compile.
- `simulated.NewBackend` API matches what M7-D.2 used (`simulated.NewBackend(alloc)` + `backend.Client()` + `backend.Commit()`).
- `Provider.dialedClient` is the ownership flag for closing the underlying ethclient — only `New` sets it, not `NewWithClient`. This avoids the test from accidentally closing the simulated backend's client.
- `Namehash` per ENSIP-1: empty string → bytes32(0); recursive `keccak256(parent || keccak256(label))`.
- For Phase 4 live gate: the 3 ENS subname registrations on first boot cost ~0.0015 Sepolia ETH each (3 setSubnodeRecord) + ~0.001 ETH (9 setText). With ≥ 0.01 ETH funded, room for many demo runs.
- Token IDs hardcoded: planner=0, coder=1, reviewer=2 — must match M7-D.1 mint order.
- Idempotency runs at TWO levels: `EnsureSubname` checks owner before tx; `SetTextRecord` reads value before tx. Re-running boot with all subnames synced does 0 txs and ~12 RPC reads (cheap, fast).
- The `n.ens = ensProv` line in Phase 3 assumes `n` is the `tgNotifier` variable in scope. If `main()` constructs the notifier later than expected, move the assignment to wherever the notifier is built.
