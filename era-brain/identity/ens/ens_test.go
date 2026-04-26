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
		hex  string
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
	backend, nw, nwAddr, _, resAddr, _, key := deployMocksOnSim(t, parentName)
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
}

func TestProvider_SetAndReadTextRecord(t *testing.T) {
	parentName := "vaibhav-era.eth"
	backend, _, nwAddr, _, resAddr, _, key := deployMocksOnSim(t, parentName)
	p := newTestProvider(t, parentName, key, nwAddr, resAddr, backend)

	require.NoError(t, p.EnsureSubname(context.Background(), "planner"))
	backend.Commit()

	v, err := p.ReadTextRecord(context.Background(), "planner", "inft_addr")
	require.NoError(t, err)
	require.Equal(t, "", v)

	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "inft_addr", "0xABCDEF"))
	backend.Commit()

	v, err = p.ReadTextRecord(context.Background(), "planner", "inft_addr")
	require.NoError(t, err)
	require.Equal(t, "0xABCDEF", v)

	// Idempotent — no Commit needed.
	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "inft_addr", "0xABCDEF"))

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
