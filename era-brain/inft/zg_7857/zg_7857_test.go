package zg_7857_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/require"

	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857/bindings"
)

func deployContractOnSim(t *testing.T) (*simulated.Backend, *bindings.EraPersonaINFT, *bind.TransactOpts, *ecdsa.PrivateKey, common.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	deployer := crypto.PubkeyToAddress(key.PublicKey)
	alloc := types.GenesisAlloc{
		deployer: {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
	}
	backend := simulated.NewBackend(alloc)
	t.Cleanup(func() { _ = backend.Close() })

	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(1337))
	require.NoError(t, err)

	addr, _, contract, err := bindings.DeployEraPersonaINFT(auth, backend.Client(), deployer)
	require.NoError(t, err)
	backend.Commit()

	tx, err := contract.Mint(auth, deployer, "ipfs://test")
	require.NoError(t, err)
	backend.Commit()
	rc, err := bind.WaitMined(context.Background(), backend.Client(), tx)
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, rc.Status)

	return backend, contract, auth, key, addr
}

func TestProvider_RecordInvocation_HappyPath(t *testing.T) {
	backend, contract, auth, _, addr := deployContractOnSim(t)
	_ = addr

	var receiptHash [32]byte
	copy(receiptHash[:], []byte("0123456789abcdef0123456789abcdef"))

	tx, err := contract.RecordInvocation(auth, big.NewInt(0), receiptHash)
	require.NoError(t, err)
	backend.Commit()

	rc, err := bind.WaitMined(context.Background(), backend.Client(), tx)
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, rc.Status)

	logs, err := contract.FilterInvocation(&bind.FilterOpts{Start: 0, End: nil}, []*big.Int{big.NewInt(0)}, [][32]byte{receiptHash}, nil)
	require.NoError(t, err)
	defer logs.Close()
	require.True(t, logs.Next(), "should have one Invocation log")
	require.Zero(t, logs.Event.TokenId.Cmp(big.NewInt(0)))
	require.Equal(t, receiptHash, logs.Event.ReceiptHash)
}

func TestProvider_RecordInvocation_HexDecodeError(t *testing.T) {
	short := "abc"
	_, err := zg_7857.DecodeReceiptHash(short)
	require.Error(t, err, "non-32-byte hex should error")

	wrongLen := "00112233445566778899aabbccddeeff"
	_, err = zg_7857.DecodeReceiptHash(wrongLen)
	require.Error(t, err)

	good := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	hash, err := zg_7857.DecodeReceiptHash(good)
	require.NoError(t, err)
	require.Equal(t, byte(0x00), hash[0])
	require.Equal(t, byte(0xff), hash[31])
}

func TestProvider_LookupReturnsNotImplemented(t *testing.T) {
	p, err := zg_7857.New(zg_7857.Config{
		ContractAddress: "0x0000000000000000000000000000000000000001",
		EVMRPCURL:       "http://127.0.0.1:1",
		PrivateKey:      "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ChainID:         16602,
	})
	if err != nil {
		t.Skipf("New errored on dial (expected against unreachable RPC): %v", err)
	}
	defer p.Close()

	_, err = p.Lookup(context.Background(), "0xabc", "planner")
	require.ErrorIs(t, err, zg_7857.ErrNotImplemented)
}

func TestProvider_Mint_HappyPath(t *testing.T) {
	backend, contract, auth, key, addr := deployContractOnSim(t)

	keyHex := common.Bytes2Hex(crypto.FromECDSA(key))
	p, err := zg_7857.NewWithClient(zg_7857.Config{
		ContractAddress: addr.Hex(),
		PrivateKey:      keyHex,
		ChainID:         1337,
	}, backend.Client())
	require.NoError(t, err)
	t.Cleanup(p.Close)

	// simulated.Backend doesn't auto-mine — Mint() blocks on bind.WaitMined,
	// so we drive Commit() in parallel with the Mint call.
	done := make(chan struct{})
	go func() {
		// Allow Mint to submit the tx before we commit.
		// A few Commit()s in a row covers any timing skew.
		for i := 0; i < 10; i++ {
			backend.Commit()
		}
		close(done)
	}()

	persona, err := p.Mint(context.Background(), "rustacean", "ipfs://prompt-blob")
	<-done
	require.NoError(t, err)
	require.NotEmpty(t, persona.TokenID, "token ID should be populated from Transfer event")
	require.Equal(t, "rustacean", persona.Name)
	require.Equal(t, "ipfs://prompt-blob", persona.SystemPromptURI)
	require.Equal(t, auth.From.Hex(), persona.OwnerAddr)
	require.NotEmpty(t, persona.MintTxHash, "tx hash should be populated for DM rendering")
	require.Equal(t, addr.Hex(), persona.ContractAddr)

	tokenID, ok := new(big.Int).SetString(persona.TokenID, 10)
	require.True(t, ok)
	owner, err := contract.OwnerOf(&bind.CallOpts{}, tokenID)
	require.NoError(t, err)
	require.Equal(t, auth.From, owner)

	uri, err := contract.TokenURI(&bind.CallOpts{}, tokenID)
	require.NoError(t, err)
	require.Equal(t, "ipfs://prompt-blob", uri)
}

// stubBlockNumber wraps a real ContractClient but overrides BlockNumber to
// claim a high head-block, forcing ScanNewMints to paginate across many chunks.
type stubBlockNumber struct {
	zg_7857.ContractClient
	head uint64
}

func (s *stubBlockNumber) BlockNumber(_ context.Context) (uint64, error) {
	return s.head, nil
}

func TestProvider_ScanNewMints_Paginated(t *testing.T) {
	backend, contract, _, key, addr := deployContractOnSim(t)
	_ = contract

	keyHex := common.Bytes2Hex(crypto.FromECDSA(key))
	p, err := zg_7857.NewWithClient(zg_7857.Config{
		ContractAddress: addr.Hex(),
		PrivateKey:      keyHex,
		ChainID:         1337,
	}, backend.Client())
	require.NoError(t, err)
	t.Cleanup(p.Close)

	// simulated.Backend doesn't auto-mine; drive Commit() in parallel so Mint
	// receipts land. Same pattern as TestProvider_Mint_HappyPath.
	stopCommit := make(chan struct{})
	commitDone := make(chan struct{})
	go func() {
		defer close(commitDone)
		for {
			select {
			case <-stopCommit:
				return
			case <-time.After(50 * time.Millisecond):
				backend.Commit()
			}
		}
	}()

	_, err = p.Mint(context.Background(), "first", "ipfs://1")
	require.NoError(t, err)
	_, err = p.Mint(context.Background(), "second", "ipfs://2")
	require.NoError(t, err)
	close(stopCommit)
	<-commitDone

	// Drive enough commits to push the sim head past several chunks so the
	// pagination loop iterates more than once. simulated.Client.FilterLogs
	// rejects End > actual head, so we must mine real blocks (cheap: pure
	// in-process Commit calls).
	const targetHead = uint64(2500) // forces 3 chunks at scanChunkSize=1000
	currentHead, err := backend.Client().BlockNumber(context.Background())
	require.NoError(t, err)
	for currentHead < targetHead {
		backend.Commit()
		currentHead++
	}

	// Wrap the client so ScanNewMints reads our chosen head, decoupling the
	// test from however many extra blocks Commit produced above.
	stubClient := &stubBlockNumber{
		ContractClient: backend.Client(),
		head:           targetHead,
	}

	pPaginated, err := zg_7857.NewWithClient(zg_7857.Config{
		ContractAddress: addr.Hex(),
		PrivateKey:      keyHex,
		ChainID:         1337,
	}, stubClient)
	require.NoError(t, err)
	t.Cleanup(pPaginated.Close)

	// sinceTokenID=0 — skip token 0 (bootstrap mint from deployContractOnSim)
	// and assert tokens 1+2 are discovered across multiple chunks.
	events, err := pPaginated.ScanNewMints(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, events, 2, "expected both mints discovered across chunks")

	tokenIDs := make(map[string]bool)
	for _, e := range events {
		tokenIDs[e.TokenID] = true
	}
	require.Contains(t, tokenIDs, "1")
	require.Contains(t, tokenIDs, "2")
}
