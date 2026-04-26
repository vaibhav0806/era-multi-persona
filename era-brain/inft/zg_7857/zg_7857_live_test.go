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
		ChainID:         16602,
	})
	require.NoError(t, err)
	t.Cleanup(p.Close)

	receiptHashHex := strings.Repeat("ab", 32)
	require.Len(t, receiptHashHex, 64)

	err = p.RecordInvocation(context.Background(), "0", receiptHashHex)
	require.NoError(t, err)
}
