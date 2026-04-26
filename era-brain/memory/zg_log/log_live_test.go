//go:build zg_live

package zg_log_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
)

func TestZGLog_LiveTestnet_AppendOnly(t *testing.T) {
	priv := os.Getenv("PI_ZG_PRIVATE_KEY")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	indexerURL := os.Getenv("PI_ZG_INDEXER_RPC")
	if priv == "" || rpc == "" || indexerURL == "" {
		t.Skip("PI_ZG_* env vars required")
	}

	live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
		PrivateKey: priv, EVMRPCURL: rpc, IndexerURL: indexerURL,
		// KVNodeURL omitted — write-only test. ReadLog will return empty
		// (Iterate returns nil when read client is nil — see live.go).
	})
	require.NoError(t, err)
	t.Cleanup(live.Close)

	var p memory.Provider = zg_log.NewWithOps(live)
	ns := fmt.Sprintf("era-brain-live-log-%d", time.Now().UnixNano())
	ctx := context.Background()

	// Single append — proves the write path works through zg_log's
	// counter-then-Set logic. Multiple appends would burn unnecessary gas.
	require.NoError(t, p.AppendLog(ctx, ns, []byte("entry-0")))
}
