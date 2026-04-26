//go:build zg_live

package zg_kv_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
)

func TestZGKV_LiveTestnet_Write(t *testing.T) {
	priv := os.Getenv("PI_ZG_PRIVATE_KEY")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	indexerURL := os.Getenv("PI_ZG_INDEXER_RPC")
	if priv == "" || rpc == "" || indexerURL == "" {
		t.Skip("PI_ZG_PRIVATE_KEY/PI_ZG_EVM_RPC/PI_ZG_INDEXER_RPC required")
	}

	live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
		PrivateKey: priv,
		EVMRPCURL:  rpc,
		IndexerURL: indexerURL,
		// KVNodeURL omitted — write-only test, reads return ErrKeyNotFound.
	})
	require.NoError(t, err)
	t.Cleanup(live.Close)

	var p memory.Provider = zg_kv.NewWithOps(live)
	ns := fmt.Sprintf("era-brain-live-%d", time.Now().UnixNano())
	key := "smoketest"
	val := []byte(fmt.Sprintf("v-%d", time.Now().UnixNano()))

	require.NoError(t, p.PutKV(context.Background(), ns, key, val))

	// Read returns ErrNotFound (no KV node URL set) — that's the expected
	// behavior for the dual-provider fallthrough pattern.
	_, err = p.GetKV(context.Background(), ns, key)
	require.ErrorIs(t, err, memory.ErrNotFound)
}
