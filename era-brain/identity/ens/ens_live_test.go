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
		ChainID:    11155111,
	})
	require.NoError(t, err)
	t.Cleanup(p.Close)

	require.Equal(t, parentName, p.ParentName())

	require.NoError(t, p.EnsureSubname(context.Background(), "planner"))

	val := "live-test-" + strings.Repeat("x", 8)
	require.NoError(t, p.SetTextRecord(context.Background(), "planner", "live_test_marker", val))

	read, err := p.ReadTextRecord(context.Background(), "planner", "live_test_marker")
	require.NoError(t, err)
	require.Equal(t, val, read, "round-trip text record should match")
}
