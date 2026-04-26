# M7-D.2 — iNFT Go Client + era Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire era's orchestrator to call `recordInvocation(tokenId, receiptHash)` on the deployed `EraPersonaINFT` contract after each persona LLM run. Real `/task` via Telegram produces 2 on-chain `Invocation` events (planner + reviewer) per task, visible on the 0G Galileo explorer when filtered by the contract address.

**Architecture:** Five linear phases. Phase 0 generates abigen Go bindings from the deployed contract's ABI (extracted via `jq` from forge's combined artifact JSON). Phase 1 wraps the bindings in a `zg_7857.Provider` impl satisfying `era-brain/inft.Registry`. Phase 2 wires the provider into era's queue with a `INFTProvider` seam interface (mirrors how `swarm` is wired). Phase 3 wires orchestrator construction conditional on env vars. Phase 4 is the live Telegram gate.

**Tech Stack:** Go 1.25, go-ethereum's `abigen` + `ethclient` + `accounts/abi/bind` + `ethclient/simulated` (for unit tests), the deployed iNFT contract at `0x33847c5500C2443E2f3BBf547d9b069B334c3D16` on 0G Galileo (chainID 16602). No new external dependencies.

**Spec:** `docs/superpowers/specs/2026-04-27-m7d-inft-design.md`. All §-references below point at the spec.

**Testing philosophy:** Strict TDD. Failing test first, run, verify FAIL, write minimal Go, run, verify PASS, commit. `go test -race -count=1 ./...` from repo root green at every commit. Live testnet gate at the end (Phase 4). `zg_live`-tagged tests skip in CI; only run when env vars present.

**Prerequisites (check before starting):**
- M7-D.1 done (tag `m7d1-done`).
- iNFT contract deployed at `0x33847c5500C2443E2f3BBf547d9b069B334c3D16` (verified on 0G explorer).
- `.env` populated with `PI_ZG_PRIVATE_KEY`, `PI_ZG_EVM_RPC`, `PI_ZG_INFT_CONTRACT_ADDRESS`.
- `jq` installed (`brew install jq` on macOS).
- `abigen` installed — comes with go-ethereum; check `which abigen`. If missing: `go install github.com/ethereum/go-ethereum/cmd/abigen@v1.17.2` (matches the version in era-brain's go.mod).
- Existing era + era-brain Go tests still pass (non-regression baseline).

---

## File Structure

```
Makefile                                                            MODIFY (Phase 0) — add `make abigen` target

era-brain/inft/zg_7857/                                             CREATE (Phases 0, 1)
├── zg_7857.go                                                      CREATE (Phase 1) — Provider impl
├── zg_7857_test.go                                                 CREATE (Phase 1) — 3 unit tests via simulated.Backend
├── zg_7857_live_test.go                                            CREATE (Phase 1) — //go:build zg_live
└── bindings/
    └── era_persona_inft.go                                         CREATE (Phase 0) — abigen output, committed

internal/queue/queue.go                                             MODIFY (Phase 2) — INFTProvider interface, Queue.inft + SetINFT, RunNext calls
internal/queue/queue_run_test.go                                    MODIFY (Phase 2) — stubINFT helper, new tests

cmd/orchestrator/main.go                                            MODIFY (Phase 3) — env-conditional zg_7857.New + SetINFT + boot log
```

No changes to era-brain's existing packages (brain, llm, memory). The `inft.Registry` interface stub from M7-A.2 stays untouched — Phase 1's Provider just satisfies it.

---

## Phase 0: Generate abigen bindings

**Files:**
- Create: `era-brain/inft/zg_7857/bindings/era_persona_inft.go` (abigen output)
- Modify: `Makefile` (era root) — add `abigen` target for regeneration

The Foundry build at `contracts/out/EraPersonaINFT.sol/EraPersonaINFT.json` is a combined artifact (ABI + bytecode + AST). abigen wants a bare ABI array. Workflow: extract ABI with `jq`, pipe to abigen.

### Step 0.1: Verify jq + abigen are installed

```bash
which jq && jq --version
which abigen && abigen --version
```

Expected: both print versions. If missing:
```bash
brew install jq
go install github.com/ethereum/go-ethereum/cmd/abigen@v1.17.2
```

(`@v1.17.2` matches era-brain's go.mod go-ethereum version. Check `era-brain/go.mod` for the actual version pinned.)

### Step 0.2: Build the contract first (so the artifact exists)

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era/contracts
forge build
ls out/EraPersonaINFT.sol/EraPersonaINFT.json
```

Expected: file exists.

### Step 0.3: Extract ABI + run abigen

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
mkdir -p era-brain/inft/zg_7857/bindings
jq '.abi' contracts/out/EraPersonaINFT.sol/EraPersonaINFT.json > /tmp/era_inft.abi
abigen --abi /tmp/era_inft.abi --pkg bindings --type EraPersonaINFT \
  --out era-brain/inft/zg_7857/bindings/era_persona_inft.go
```

Expected: `era-brain/inft/zg_7857/bindings/era_persona_inft.go` created. Should be ~600-800 lines of generated Go code with structs `EraPersonaINFT`, `EraPersonaINFTCaller`, `EraPersonaINFTTransactor`, etc.

### Step 0.4: Verify generated bindings compile

```bash
cd era-brain
go build ./inft/zg_7857/bindings/...
```

Expected: exit 0. The generated code uses `github.com/ethereum/go-ethereum/...` packages already in era-brain's go.mod (transitively from M7-B.1's 0G Storage SDK).

If `go build` fails with "cannot find package", check that go-ethereum is in era-brain/go.mod — if not, run `go get github.com/ethereum/go-ethereum@latest` from era-brain/.

### Step 0.5: Add `make abigen` target to Makefile

Open the era repo root's `Makefile`. Find the existing targets (build, test, etc.) and add:

```make
.PHONY: abigen
abigen:  ## Regenerate iNFT abigen bindings from contracts/out
	@command -v jq >/dev/null || { echo "ERROR: jq not installed (brew install jq)"; exit 1; }
	@command -v abigen >/dev/null || { echo "ERROR: abigen not installed (go install github.com/ethereum/go-ethereum/cmd/abigen@latest)"; exit 1; }
	cd contracts && forge build
	mkdir -p era-brain/inft/zg_7857/bindings
	jq '.abi' contracts/out/EraPersonaINFT.sol/EraPersonaINFT.json > /tmp/era_inft.abi
	abigen --abi /tmp/era_inft.abi --pkg bindings --type EraPersonaINFT \
	  --out era-brain/inft/zg_7857/bindings/era_persona_inft.go
	@echo "Bindings regenerated."
```

(Use real tabs not spaces in Makefile recipes.)

### Step 0.6: Verify make target works

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
make abigen
```

Expected: forge build runs, jq extracts ABI, abigen writes the file (idempotent — same content as before).

### Step 0.7: Run all era + era-brain tests (no regression)

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
cd era-brain
go vet ./...
go test -race -count=1 ./...
```

Both green.

### Step 0.8: Commit

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add era-brain/inft/zg_7857/bindings/ Makefile
git commit -m "phase(M7-D.2.0): abigen bindings for EraPersonaINFT + Makefile target"
git tag m7d2-0-abigen
```

No Co-Authored-By per `~/.claude/CLAUDE.md`.

---

## Phase 1: zg_7857.Provider impl + tests

**Files:**
- Create: `era-brain/inft/zg_7857/zg_7857.go`
- Create: `era-brain/inft/zg_7857/zg_7857_test.go` (3 unit tests via `simulated.Backend`)
- Create: `era-brain/inft/zg_7857/zg_7857_live_test.go` (build-tag `zg_live`)

`Provider` satisfies `era-brain/inft.Registry`. The interface has `Mint`, `Lookup`, `RecordInvocation`. M7-D.2 only implements `RecordInvocation`; `Mint` and `Lookup` return `ErrNotImplemented`.

### 1A: Failing unit test for RecordInvocation

- [ ] **Step 1.1: Write the failing test**

`era-brain/inft/zg_7857/zg_7857_test.go`:

```go
package zg_7857_test

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

	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857/bindings"
)

// deployContractOnSim deploys EraPersonaINFT to a simulated chain w/ deployer
// as initial owner, mints one token to deployer (tokenId 0), and returns
// the simulated backend, contract instance, deployer auth, and deployer key.
func deployContractOnSim(t *testing.T) (*simulated.Backend, *bindings.EraPersonaINFT, *bind.TransactOpts, *ecdsa.PrivateKey, common.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	deployer := crypto.PubkeyToAddress(key.PublicKey)
	alloc := types.GenesisAlloc{
		deployer: {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))}, // 100 ETH
	}
	backend := simulated.NewBackend(alloc)
	t.Cleanup(func() { _ = backend.Close() })

	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(1337)) // simulated chainID
	require.NoError(t, err)

	addr, _, contract, err := bindings.DeployEraPersonaINFT(auth, backend.Client(), deployer)
	require.NoError(t, err)
	backend.Commit()

	// Mint tokenId 0 to deployer so RecordInvocation has something to record against.
	tx, err := contract.Mint(auth, deployer, "ipfs://test")
	require.NoError(t, err)
	backend.Commit()
	rc, err := bind.WaitMined(context.Background(), backend.Client(), tx)
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, rc.Status)

	return backend, contract, auth, key, addr
}

func TestProvider_RecordInvocation_HappyPath(t *testing.T) {
	// We can't construct zg_7857.Provider against a simulated backend because
	// it dials via ethclient.Dial(URL). Instead, this test verifies that the
	// abigen contract.RecordInvocation call (which Provider wraps) emits the
	// Invocation event correctly when called directly. Provider's role is
	// just packaging this call w/ tx signing — verified by the live test.
	backend, contract, auth, _, addr := deployContractOnSim(t)
	_ = addr

	var receiptHash [32]byte
	copy(receiptHash[:], []byte("0123456789abcdef0123456789abcdef")) // 32 bytes

	tx, err := contract.RecordInvocation(auth, big.NewInt(0), receiptHash)
	require.NoError(t, err)
	backend.Commit()

	rc, err := bind.WaitMined(context.Background(), backend.Client(), tx)
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, rc.Status)

	// Verify Invocation event was emitted.
	logs, err := contract.FilterInvocation(&bind.FilterOpts{Start: 0, End: nil}, []*big.Int{big.NewInt(0)}, [][32]byte{receiptHash}, nil)
	require.NoError(t, err)
	defer logs.Close()
	require.True(t, logs.Next(), "should have one Invocation log")
	require.Equal(t, big.NewInt(0), logs.Event.TokenId)
	require.Equal(t, receiptHash, logs.Event.ReceiptHash)
}

func TestProvider_RecordInvocation_HexDecodeError(t *testing.T) {
	// Provider takes a hex string; if not 32 bytes after decoding, it should error.
	// We construct a Provider against a real RPC URL but never call it (just New).
	// Actually, we can't easily test this without a live RPC. Skip and rely on the
	// hex-decode error path being exercised in the live test or via a smaller unit
	// that decodes the string standalone.
	//
	// Easier: define an exported helper TestDecodeReceiptHash that the test calls.
	short := "abc"
	_, err := zg_7857.DecodeReceiptHash(short)
	require.Error(t, err, "non-32-byte hex should error")

	wrongLen := "00112233445566778899aabbccddeeff" // 16 bytes only
	_, err = zg_7857.DecodeReceiptHash(wrongLen)
	require.Error(t, err)

	good := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff" // 32 bytes
	hash, err := zg_7857.DecodeReceiptHash(good)
	require.NoError(t, err)
	require.Equal(t, byte(0x00), hash[0])
	require.Equal(t, byte(0xff), hash[31])
}

func TestProvider_MintAndLookupReturnNotImplemented(t *testing.T) {
	// M7-D.2 scope: Mint and Lookup return ErrNotImplemented (deferred to M7-D.3+).
	// We test this without standing up a real backend — just construct via New
	// against a localhost URL that won't be hit (the methods early-return).
	p, err := zg_7857.New(zg_7857.Config{
		ContractAddress: "0x0000000000000000000000000000000000000001",
		EVMRPCURL:       "http://127.0.0.1:1", // unused for these methods
		PrivateKey:      "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ChainID:         16602,
	})
	if err != nil {
		// New() may fail to dial — that's OK for this test; we want to assert
		// that IF the provider constructed, Mint/Lookup return ErrNotImplemented.
		// If New itself errors on dial, skip the rest.
		t.Skipf("New errored on dial (expected against unreachable RPC): %v", err)
	}
	defer p.Close()

	_, err = p.Mint(context.Background(), "planner", "ipfs://x")
	require.ErrorIs(t, err, zg_7857.ErrNotImplemented)

	_, err = p.Lookup(context.Background(), "0xabc", "planner")
	require.ErrorIs(t, err, zg_7857.ErrNotImplemented)
}
```

- [ ] **Step 1.2: Run, verify FAIL**

```bash
mkdir -p era-brain/inft/zg_7857
# (test file written in Step 1.1 lives there; impl file does not exist yet)
cd era-brain
go test ./inft/zg_7857/... -v 2>&1 | head -40
```

Expected: build failure mentioning `undefined: zg_7857.New`, `undefined: zg_7857.Config`, `undefined: zg_7857.DecodeReceiptHash`, or `undefined: zg_7857.ErrNotImplemented`. Exit code non-zero. If output says "no Go files in …" or "matched no packages" exit 0, the test file isn't placed correctly — re-check the path.

### 1B: Implement Provider

- [ ] **Step 1.3: Write zg_7857.go**

`era-brain/inft/zg_7857/zg_7857.go`:

```go
// Package zg_7857 is an inft.Registry impl wrapping abigen bindings for the
// EraPersonaINFT contract deployed on 0G Galileo testnet.
//
// M7-D.2 scope: only RecordInvocation is implemented. Mint and Lookup return
// ErrNotImplemented (deferred to M7-D.3+).
package zg_7857

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/vaibhav0806/era-multi-persona/era-brain/inft"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857/bindings"
)

// ErrNotImplemented is returned by Mint and Lookup in M7-D.2 scope.
var ErrNotImplemented = errors.New("zg_7857: not implemented in M7-D.2 (deferred)")

// Config holds the Provider setup.
type Config struct {
	ContractAddress string // 0x... — deployed EraPersonaINFT address
	EVMRPCURL       string // e.g. https://evmrpc-testnet.0g.ai
	PrivateKey      string // hex (with or without 0x prefix)
	ChainID         int64  // 0G Galileo testnet = 16602
}

// Provider implements inft.Registry on top of abigen bindings.
type Provider struct {
	cfg      Config
	client   *ethclient.Client
	contract *bindings.EraPersonaINFT
	auth     *bind.TransactOpts
	privKey  *ecdsa.PrivateKey
}

var _ inft.Registry = (*Provider)(nil)

// New constructs a Provider. Caller must Close.
func New(cfg Config) (*Provider, error) {
	client, err := ethclient.Dial(cfg.EVMRPCURL)
	if err != nil {
		return nil, fmt.Errorf("zg_7857 dial: %w", err)
	}

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("zg_7857 priv key: %w", err)
	}

	contract, err := bindings.NewEraPersonaINFT(common.HexToAddress(cfg.ContractAddress), client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("zg_7857 bind contract: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(cfg.ChainID))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("zg_7857 auth: %w", err)
	}

	return &Provider{cfg: cfg, client: client, contract: contract, auth: auth, privKey: privKey}, nil
}

// Close releases the underlying ethclient.
func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

// RecordInvocation submits a tx logging the persona invocation. tokenID is a
// decimal string ("0", "1", "2"); receiptHashHex is a 64-char hex string
// (sha256 digest from brain.ReceiptHash). Returns nil on tx submission success;
// errors are non-fatal in queue.RunNext caller (logged via slog.Warn).
func (p *Provider) RecordInvocation(ctx context.Context, tokenID string, receiptHashHex string) error {
	tokenIDBig, ok := new(big.Int).SetString(tokenID, 10)
	if !ok {
		return fmt.Errorf("zg_7857 invalid tokenID %q", tokenID)
	}
	hash, err := DecodeReceiptHash(receiptHashHex)
	if err != nil {
		return fmt.Errorf("zg_7857 decode receiptHash: %w", err)
	}

	// Shallow copy of auth so we can override Context per call. Signer (pointer)
	// stays shared — fine because era's queue serializes tasks today.
	auth := *p.auth
	auth.Context = ctx

	if _, err := p.contract.RecordInvocation(&auth, tokenIDBig, hash); err != nil {
		return fmt.Errorf("zg_7857 recordInvocation tx: %w", err)
	}
	return nil
}

// Mint returns ErrNotImplemented in M7-D.2 scope.
func (p *Provider) Mint(_ context.Context, _, _ string) (inft.Persona, error) {
	return inft.Persona{}, ErrNotImplemented
}

// Lookup returns ErrNotImplemented in M7-D.2 scope.
func (p *Provider) Lookup(_ context.Context, _, _ string) (inft.Persona, error) {
	return inft.Persona{}, ErrNotImplemented
}

// DecodeReceiptHash converts a hex string (with or without 0x prefix) into
// a [32]byte. Returns error if the decoded byte length isn't 32.
func DecodeReceiptHash(hexStr string) ([32]byte, error) {
	var hash [32]byte
	raw, err := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	if err != nil {
		return hash, fmt.Errorf("hex decode: %w", err)
	}
	if len(raw) != 32 {
		return hash, fmt.Errorf("expected 32 bytes, got %d", len(raw))
	}
	copy(hash[:], raw)
	return hash, nil
}
```

- [ ] **Step 1.4: Run unit tests, verify PASS**

```bash
cd era-brain
go test -race ./inft/zg_7857/...
```

Expected: 3 tests pass (HappyPath via simulated backend, HexDecodeError, MintAndLookupNotImplemented).

If `TestProvider_MintAndLookupReturnNotImplemented` skips (because `New` errors on dial), that's fine — the live test exercises the full flow.

### 1C: Live integration test

- [ ] **Step 1.5: Write build-tagged live test**

`era-brain/inft/zg_7857/zg_7857_live_test.go`:

```go
//go:build zg_live

package zg_7857_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
)

func TestProvider_LiveTestnet_RecordInvocation(t *testing.T) {
	contractAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	privKey := os.Getenv("PI_ZG_PRIVATE_KEY")
	if contractAddr == "" || rpc == "" || privKey == "" {
		t.Skip("PI_ZG_INFT_CONTRACT_ADDRESS / PI_ZG_EVM_RPC / PI_ZG_PRIVATE_KEY required")
	}

	p, err := zg_7857.New(zg_7857.Config{
		ContractAddress: contractAddr,
		EVMRPCURL:       rpc,
		PrivateKey:      privKey,
		ChainID:         16602, // 0G Galileo
	})
	require.NoError(t, err)
	t.Cleanup(p.Close)

	// 32-byte hex (sha256-shaped) — content doesn't matter for the live test.
	receiptHashHex := strings.Repeat("ab", 32)
	require.Len(t, receiptHashHex, 64)

	// tokenId 0 = planner (minted at deploy time per M7-D.1).
	err = p.RecordInvocation(context.Background(), "0", receiptHashHex)
	require.NoError(t, err)
}
```

- [ ] **Step 1.6: Run live test against testnet**

```bash
cd era-brain
set -a; source ../.env; set +a
go test -tags zg_live -v -run TestProvider_LiveTestnet ./inft/zg_7857/... 2>&1 | tee /tmp/m7d2-live.log
grep -E "^--- PASS: TestProvider_LiveTestnet_RecordInvocation" /tmp/m7d2-live.log
```

Expected: `--- PASS:` line printed; the grep at the end exits 0. PASS in ~10-30 seconds. Cost ~0.0005 ZG per call. (`-run` alone exits 0 on no-match, so the explicit grep gates this step.)

If FAIL:
- "execution reverted" → check ACL (caller wallet must be contract owner OR token holder; deployer wallet is both — should pass).
- Auth errors → bearer/key issues; verify env vars.
- "no contract code at given address" → contract address typo.

### 1D: Sanity sweep

- [ ] **Step 1.7: Run all era-brain tests**

```bash
go vet ./...
go test -race -count=1 ./...   # without zg_live tag
```

Expected: green.

- [ ] **Step 1.8: Run all era root tests (no regression)**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

- [ ] **Step 1.9: Commit**

```bash
git add era-brain/inft/zg_7857/
git commit -m "phase(M7-D.2.1): zg_7857.Provider — RecordInvocation wired; Mint/Lookup deferred"
git tag m7d2-1-provider
```

---

## Phase 2: Queue wiring — INFTProvider seam + RecordInvocation calls

**Files:**
- Modify: `internal/queue/queue.go` — INFTProvider interface, Queue.inft + SetINFT, RunNext calls
- Modify: `internal/queue/queue_run_test.go` — stubINFT helper, 2 new tests

### 2A: Failing test for queue-side recordInvocation propagation

- [ ] **Step 2.1: Read existing queue.go to find ReviewArgs construction**

```bash
grep -n "swarm.ReviewArgs\|q.swarm.Plan\|q.swarm.Review\|plannerReceipt\|reviewerReceipt" /Users/vaibhav/Documents/projects/era-multi-persona/era/internal/queue/queue.go | head -10
```

Expected: hits at the existing `q.swarm.Plan(...)` and `q.swarm.Review(...)` call sites inside `RunNext`. `plannerReceipt` and `reviewerReceipt` are existing local variables.

- [ ] **Step 2.2: Read queue_run_test.go for existing stub patterns**

```bash
grep -n "stubSwarm\|q.SetSwarm\|q.SetUserID\|TestRunNext_" /Users/vaibhav/Documents/projects/era-multi-persona/era/internal/queue/queue_run_test.go | head -15
```

Find the `stubSwarm` definition + the existing `TestRunNext_PassesPlannerSealedToReviewer` test (M7-C.2.2) for boilerplate to mirror.

- [ ] **Step 2.3: Add failing tests to queue_run_test.go**

Append to `internal/queue/queue_run_test.go`:

```go
type stubINFT struct {
	mu             sync.Mutex
	calls          []stubINFTCall
	failOnFirst    bool // when true, first RecordInvocation call returns error
}

type stubINFTCall struct {
	tokenID         string
	receiptHashHex  string
}

func (s *stubINFT) RecordInvocation(_ context.Context, tokenID, receiptHashHex string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, stubINFTCall{tokenID: tokenID, receiptHashHex: receiptHashHex})
	if s.failOnFirst && len(s.calls) == 1 {
		return errors.New("stubINFT: simulated first-call failure")
	}
	return nil
}

func TestRunNext_RecordsInvocationForPlannerAndReviewer(t *testing.T) {
	fr := &fakeRunner{branch: "agent/1/ok", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	stub := &stubSwarm{
		planText:       "1. step",
		plannerSealed:  true,
		reviewDecision: swarm.DecisionApprove,
	}
	q.SetSwarm(stub)
	q.SetUserID("u")

	inftStub := &stubINFT{}
	q.SetINFT(inftStub)

	_, err := repo.CreateTask(context.Background(), "do thing", "owner/repo", "default")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed)

	inftStub.mu.Lock()
	defer inftStub.mu.Unlock()
	require.Len(t, inftStub.calls, 2, "expected planner + reviewer invocations recorded")
	require.Equal(t, "0", inftStub.calls[0].tokenID, "planner tokenID = 0")
	require.Equal(t, "2", inftStub.calls[1].tokenID, "reviewer tokenID = 2")
	require.Len(t, inftStub.calls[0].receiptHashHex, 64, "receipt hash should be 64-char hex")
	require.Len(t, inftStub.calls[1].receiptHashHex, 64)
}

func TestRunNext_INFTFailureDoesNotBlockTask(t *testing.T) {
	fr := &fakeRunner{branch: "agent/1/ok", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	stub := &stubSwarm{
		planText:       "1. step",
		plannerSealed:  true,
		reviewDecision: swarm.DecisionApprove,
	}
	q.SetSwarm(stub)
	q.SetUserID("u")

	inftStub := &stubINFT{failOnFirst: true}
	q.SetINFT(inftStub)

	_, err := repo.CreateTask(context.Background(), "do thing", "owner/repo", "default")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err, "INFT failure must not block task completion")
	require.True(t, processed)

	// Both calls were attempted (first errored, second still ran).
	inftStub.mu.Lock()
	defer inftStub.mu.Unlock()
	require.Len(t, inftStub.calls, 2, "second call should attempt despite first failure")
}

func TestRunNext_NoINFTProviderSkipsRecording(t *testing.T) {
	fr := &fakeRunner{branch: "agent/1/ok", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	stub := &stubSwarm{
		planText:       "1. step",
		plannerSealed:  true,
		reviewDecision: swarm.DecisionApprove,
	}
	q.SetSwarm(stub)
	q.SetUserID("u")

	// q.SetINFT(...) NOT called — provider stays nil.

	_, err := repo.CreateTask(context.Background(), "do thing", "owner/repo", "default")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed, "task should complete with no INFTProvider")
}
```

(`newRunQueue(t, fr)` returns `(*queue.Queue, *db.Repo)` — verified at `internal/queue/queue_run_test.go:66`. If `errors` or `sync` aren't already imported, add them to the test file's imports.)

- [ ] **Step 2.4: Run, verify FAIL**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go test ./internal/queue/ -run "RecordsInvocation|INFTFailureDoesNotBlock|NoINFTProviderSkips" -count=1
```

Expected: `q.SetINFT undefined`, `INFTProvider undefined` compile errors.

### 2B: Implement INFTProvider interface + Queue wiring

- [ ] **Step 2.5: Add INFTProvider interface + Queue field**

In `internal/queue/queue.go`, near the existing `Swarm` interface:

```go
// INFTProvider is the queue's view of the iNFT registry: just RecordInvocation.
// Defined here so queue tests can inject a stub without pulling the full
// era-brain.inft.Registry interface (Mint/Lookup are out of M7-D.2 scope).
type INFTProvider interface {
	RecordInvocation(ctx context.Context, tokenID, receiptHashHex string) error
}
```

Add to Queue struct:
```go
type Queue struct {
	// ... existing fields ...
	inft INFTProvider
}
```

Add the setter near `SetSwarm`:
```go
// SetINFT attaches an iNFT registry to this Queue. When set, RunNext records
// an Invocation event per persona LLM run after each successful Plan/Review.
// Failures are non-fatal — logged via slog.Warn.
func (q *Queue) SetINFT(p INFTProvider) { q.inft = p }
```

### 2C: Add token ID constants + RecordInvocation calls in RunNext

- [ ] **Step 2.6: Add tokenID constants at top of queue.go**

Verify (should print nothing):
```bash
grep -n "plannerTokenID\|reviewerTokenID" internal/queue/queue.go
```

If empty (it should be — pre-check confirms no prior definition), add:

```go
const (
	plannerTokenID  = "0"
	reviewerTokenID = "2"
	// coder tokenID 1 skipped — Pi-in-Docker is unsealed per M7-C scope;
	// no LLMPersona receipt to record.
)
```

If the grep returns hits (someone added them in a previous milestone), reuse the existing constants and skip this step. Names match spec §2 line 258-263.

- [ ] **Step 2.7: Modify RunNext to call RecordInvocation**

In `internal/queue/queue.go`, find the post-`q.swarm.Plan(...)` block (where `plannerReceipt` is set on success). Add immediately after:

```go
if perr == nil && q.inft != nil {
	hash := brain.ReceiptHash(plannerReceipt)
	if recErr := q.inft.RecordInvocation(ctx, plannerTokenID, hash); recErr != nil {
		slog.Warn("inft recordInvocation failed (planner)",
			"task_id", t.ID, "tokenID", plannerTokenID, "err", recErr)
	}
}
```

Find the post-`q.swarm.Review(...)` block (where `reviewerReceipt` is set on success). Add immediately after:

```go
if rerr == nil && q.inft != nil {
	hash := brain.ReceiptHash(reviewerReceipt)
	if recErr := q.inft.RecordInvocation(ctx, reviewerTokenID, hash); recErr != nil {
		slog.Warn("inft recordInvocation failed (reviewer)",
			"task_id", t.ID, "tokenID", reviewerTokenID, "err", recErr)
	}
}
```

Both blocks gate on `q.inft != nil` — preserves M7-C.2 baseline when iNFT isn't wired.

`brain.ReceiptHash` returns 64-char hex (no `0x` prefix); zg_7857.DecodeReceiptHash handles both — no transformation needed.

### 2D: Run + commit

- [ ] **Step 2.8: Run, verify PASS**

```bash
go test -race ./internal/queue/ -count=1
```

Expected: 3 new tests pass + all existing queue tests still pass.

- [ ] **Step 2.9: Run all tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./...
```

Both green.

- [ ] **Step 2.10: Commit**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add internal/queue/
git commit -m "phase(M7-D.2.2): queue wiring — INFTProvider interface; RunNext records Invocation per persona"
git tag m7d2-2-queue-wiring
```

---

## Phase 3: Orchestrator wiring

**Files:**
- Modify: `cmd/orchestrator/main.go` — env-conditional zg_7857.New + q.SetINFT + boot log

### Step 3.1: Read existing main.go for the env-var-conditional pattern

```bash
grep -n "zgEnabled\|zgComputeEnabled\|q.SetSwarm\|q.SetUserID" cmd/orchestrator/main.go | head
```

Find the existing `if zgEnabled() { ... }` and `if zgComputeEnabled() { ... }` blocks for the wiring pattern to mirror.

### Step 3.2: Add `zgINFTEnabled` helper

Near the existing `zgEnabled()` and `zgComputeEnabled()`:

```go
// zgINFTEnabled returns true when the iNFT contract address is configured
// AND a private key is available for tx signing.
func zgINFTEnabled() bool {
	return os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS") != "" &&
		os.Getenv("PI_ZG_PRIVATE_KEY") != ""
}
```

### Step 3.3: Add iNFT provider construction after q.SetSwarm + q.SetUserID

```go
// (After existing q.SetSwarm(sw); q.SetUserID(...) calls)

if zgINFTEnabled() {
	inftProv, err := zg_7857.New(zg_7857.Config{
		ContractAddress: os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS"),
		EVMRPCURL:       os.Getenv("PI_ZG_EVM_RPC"),
		PrivateKey:      os.Getenv("PI_ZG_PRIVATE_KEY"),
		ChainID:         16602, // 0G Galileo testnet
	})
	if err != nil {
		fail(fmt.Errorf("zg_7857 provider: %w", err))
	}
	defer inftProv.Close()
	q.SetINFT(inftProv)
	slog.Info("0G iNFT registry wired",
		"contract", os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS"))
}
```

Add the import at the top of main.go:
```go
"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
```

### Step 3.4: Build, verify compile

```bash
go build ./...
```

Expected: exit 0.

### Step 3.5: Run all tests + vet

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

### Step 3.6: Commit

```bash
git add cmd/orchestrator/main.go
git commit -m "phase(M7-D.2.3): orchestrator wires zg_7857 iNFT provider when PI_ZG_INFT_CONTRACT_ADDRESS set"
git tag m7d2-3-orchestrator-wiring
```

---

## Phase 4: Live gate — real Telegram /task

**Files:** none modified. Verification only.

### Step 4.1: Build orchestrator binary

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go build -o bin/orchestrator ./cmd/orchestrator
ls -lh bin/orchestrator
```

### Step 4.2: Stop VPS M6

```bash
ssh era@178.105.44.3 sudo systemctl stop era
ssh era@178.105.44.3 systemctl is-active era
```

Expected: second command prints `inactive` (and exit code 3). Don't proceed until confirmed — otherwise the VPS bot may steal the Telegram update meant for local.

### Step 4.3: Start local orchestrator

```bash
set -a; source .env; set +a
./bin/orchestrator
```

**Expected NEW boot line (vs M7-C.2):**
- `INFO 0G iNFT registry wired contract=0x33847c5500C2443E2f3BBf547d9b069B334c3D16`

If absent → `.env` doesn't have `PI_ZG_INFT_CONTRACT_ADDRESS` populated, OR private key missing.

### Step 4.4: Send a /task via Telegram

```
/task add a /version endpoint that returns the current commit SHA
```

### Step 4.5: Watch the orchestrator stdout

Expected event sequence (compared to M7-C.2 baseline):
- 4 0G Storage Set/receipt blocks (planner audit + planner KV + reviewer audit + reviewer KV) — M7-B.3 baseline
- **NEW: 2 more `Set tx params` + `Transaction receipt` blocks** for the iNFT `recordInvocation` calls (planner + reviewer)
- No `inft recordInvocation failed` warnings (would indicate ACL issue or RPC error)
- Telegram completion DM unchanged from M7-C.2

Total 0G txs per task: **6** (was 4 in M7-B.3 / M7-C.2). Cost ~0.001 ZG total (~0.0005 per recordInvocation tx).

### Step 4.6: Verify Invocation events on 0G explorer

Open in browser:
```
https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16#events
```

(URL may vary; if there's no `#events` tab, navigate to the contract address page → "Logs" or "Events" tab.)

Expected: 2 new `Invocation` events with topics:
- `tokenId = 0` (planner) and `tokenId = 2` (reviewer)
- `receiptHash` = the hash of each persona's brain.Receipt

Cross-check: dump recent events via cast. Compute a from-block ~50 blocks back so we capture the just-emitted logs:

```bash
LATEST=$(cast block-number --rpc-url $PI_ZG_EVM_RPC)
FROM=$((LATEST - 50))
cast logs --address 0x33847c5500C2443E2f3BBf547d9b069B334c3D16 \
  --from-block $FROM --to-block latest \
  --rpc-url $PI_ZG_EVM_RPC | tail -40
```

Should show 2 logs with the `Invocation` event topic. (Using `--from-block latest` returns only the single most-recent block, usually empty.)

### Step 4.7: Cross-check receipts in era-brain.db

```bash
sqlite3 ./era-brain.db "SELECT val FROM entries WHERE is_kv = 0 AND namespace LIKE 'audit/%' ORDER BY seq DESC LIMIT 2"
```

Expected: planner + reviewer receipts with `Sealed=true`. Their `OutputHash` (or whatever brain.ReceiptHash hashes — see brain/receipt.go) should match the `receiptHash` topic in the on-chain events.

### Step 4.8: Restart VPS M6

```bash
ssh era@178.105.44.3 sudo systemctl start era
```

**Don't skip.** Production bot stays offline until you do.

### Step 4.9: Stop local orchestrator (Ctrl-C)

### Step 4.10: Replay tests

```bash
go vet ./... && go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./...
```

Both green.

### Step 4.11: Tag M7-D.2 done

```bash
git tag m7d2-done
```

(no commit — Phase 4 is verification only)

---

## Live gate summary (M7-D.2 acceptance)

When this milestone is done:

1. `go build ./...` from repo root succeeds.
2. `go test -race -count=1 ./...` from repo root green; no regression.
3. Real `/task` on a real repo:
   - Orchestrator startup logs show `0G iNFT registry wired ...`.
   - 6 0G txs total per task (4 Storage from M7-B.3 + 2 iNFT recordInvocation from M7-D.2).
   - 2 `Invocation` events visible on 0G explorer with planner+reviewer tokenIds + receipt hashes.
   - Telegram DM unchanged shape from M7-C.2.
4. Without `PI_ZG_INFT_CONTRACT_ADDRESS` env var, orchestrator falls back to no-iNFT mode — M7-C.2 baseline preserved.
5. VPS M6 era is restarted after the live gate.

---

## Out of scope (deferred)

- **`Mint` and `Lookup` Provider methods.** Return `ErrNotImplemented`; M7-D.3+.
- **`/persona-mint` Telegram command.** Requires Mint impl; defer.
- **Coder persona on-chain receipts.** Pi remains unsealed in M7-C scope.
- **Royalty splits / marketplace integration.** Out of M7-D entirely.
- **iNFT URI dereferencing during /task.** era never fetches the metadata URL; judges click it externally.
- **Audit-log event kinds for iNFT failures** (`inft_recorded`, `inft_failed`). Cuts-list candidate; defer.

---

## Risks + cuts list (in order if slipping)

1. **abigen produces a binding that doesn't compile.** Recovery: regenerate via `make abigen`; check go-ethereum version in era-brain/go.mod matches the abigen tool version.
2. **Live test in Phase 1 reverts** because contract ACL rejects the caller. Recovery: confirm `cast call <CONTRACT> "owner()(address)" --rpc-url $PI_ZG_EVM_RPC` returns the same wallet as `PI_ZG_PRIVATE_KEY`'s address.
3. **`bind.NewKeyedTransactorWithChainID` rejects the chainID** because 0G Galileo's chainID isn't standard EVM. Recovery: chainID is 16602 (confirmed M7-C.2.1 working); shouldn't fail. If it does, check `eth_chainId` RPC result.
4. **6 txs/task drains the wallet faster than expected.** Recovery: monitor via `cast balance`; faucet up if approaching empty. At ~0.001 ZG/task, 1 ZG covers ~1000 tasks.
5. **`q.SetINFT` cascade breaks an existing test** because nil-guard is missing somewhere. Recovery: ensure all RunNext code paths gate on `q.inft != nil`. The two new blocks already do; existing code paths shouldn't reference `q.inft` at all.

---

## Notes for implementer

- The abigen-generated `bindings/era_persona_inft.go` is a LARGE file (600-800 lines). Don't read it carefully — just verify it compiles. Trust abigen.
- `go-ethereum`'s `simulated.Backend` (NOT `SimulatedBackend` — the type was renamed in v1.14+) is at `github.com/ethereum/go-ethereum/ethclient/simulated`. Construct with `simulated.NewBackend(alloc, opts...)`. The era-brain/go.mod already pins `v1.17.2` (added during M7-B.1 0G Storage SDK).
- The `auth := *p.auth` shallow copy in `RecordInvocation` only protects the `Context` field; pointer fields like `Signer` are shared. era's queue serializes tasks today so this is fine. If concurrent tasks are ever added, build a fresh `bind.NewKeyedTransactorWithChainID` per call instead.
- `brain.ReceiptHash` returns 64-char hex (no `0x` prefix). zg_7857.DecodeReceiptHash strips `0x` if present, so callers don't need to think about it.
- Token IDs hardcoded: planner=0, coder=1, reviewer=2 — must match the Foundry deploy script's mint order. M7-D.1.3's deploy script mints in this order.
- For Phase 4's live gate, the user needs to be aware that 6 testnet txs per task is real cost — about 0.001 ZG. With 1 ZG funded earlier (M7-C.1.0's transfer-fund), that covers many tasks; not a concern for hackathon scope.
