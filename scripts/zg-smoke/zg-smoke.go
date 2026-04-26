// zg-smoke is a standalone SDK verification script. Run with:
//
//	set -a; source ../../.env; set +a
//	go run ./scripts/zg-smoke
//
// It writes a single KV pair to a 0G stream, reads it back, and prints
// the result. If this prints "OK", the era-brain zg_kv provider has a
// working foundation to build on.
//
// # Deviations from the M7-B.1 plan template
//
// The plan was written against pkg.go.dev docs; actual SDK shapes differ:
//
// 1. MODULE PATH: The Go module path is github.com/0gfoundation/0g-storage-client
//    (not github.com/0glabs/0g-storage-client as the plan shows). The 0g-storage-client
//    module declares itself as 0gfoundation, but pkg.go.dev redirects from the 0glabs
//    import path. Phase 1 must use github.com/0gfoundation/0g-storage-client everywhere.
//
// 2. WEB3 CLIENT: Use blockchain.NewWeb3(rpc, privKey) from
//    github.com/0gfoundation/0g-storage-client/common/blockchain — much cleaner than
//    raw web3go.NewClientWithOption. The plan's web3go.NewClientWithOption approach
//    compiles but the blockchain package is the SDK-idiomatic path.
//
// 3. BATCHER.SET BUILDER PATTERN: The plan says "capture the returned *streamDataBuilder
//    and call .Build() on it". This is WRONG. Set() returns *streamDataBuilder for
//    chaining, but Batcher embeds *streamDataBuilder — so Set() mutates batcher's embedded
//    state in place. The real CLI (cmd/kv_write.go) discards the Set() return entirely
//    and calls batcher.Exec(ctx) directly; Exec() calls Build() internally. Phase 1
//    must mirror this pattern — do NOT call Build() separately before Exec().
//
// 4. NODES.CLOSE: SelectedNodes (*transfer.SelectedNodes) has no Close() method.
//    The plan template's "defer nodes.Close()" does not compile. Removed.
//
// 5. SELECTED NODES TRUSTED FIELD: SelectedNodes.Trusted is []*node.ZgsClient (storage
//    nodes), not []*node.NodeInfo with a .URL field. ZgsClient has no exported URL.
//    Therefore the plan's "node.NewKvClient(nodes.Trusted[0].URL)" is wrong — you
//    cannot derive a KV node URL from SelectedNodes. A separate KV node URL is required
//    (PI_ZG_KV_NODE env var, added to .env.example alongside the original 3 vars).
//    The indexer selects ZGS file-storage nodes; KV operations need a dedicated KV node.
//
// TODO(phase-1): Phase 1's zg_kv provider must accept a KV node URL separately from the
// indexer URL. Suggested constructor: NewProvider(evmRPC, indexerURL, kvNodeURL, privKey).
// The kvOps interface seam should wrap both the write path (kv.Batcher + indexer nodes)
// and the read path (kv.Client from kv node) so unit tests can fake both independently.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/0gfoundation/0g-storage-client/common/blockchain"
	"github.com/0gfoundation/0g-storage-client/indexer"
	"github.com/0gfoundation/0g-storage-client/kv"
	"github.com/0gfoundation/0g-storage-client/node"
	"github.com/ethereum/go-ethereum/common"
)

func main() {
	priv := os.Getenv("PI_ZG_PRIVATE_KEY")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	indexerURL := os.Getenv("PI_ZG_INDEXER_RPC")
	kvNodeURL := os.Getenv("PI_ZG_KV_NODE") // optional for write-only smoke
	if priv == "" || rpc == "" || indexerURL == "" {
		log.Fatal("PI_ZG_PRIVATE_KEY, PI_ZG_EVM_RPC, PI_ZG_INDEXER_RPC required (PI_ZG_KV_NODE optional — write-only smoke if missing)")
	}

	// Web3 client for signing transactions (blockchain.NewWeb3 is the SDK-idiomatic way).
	w3, err := blockchain.NewWeb3(rpc, priv)
	if err != nil {
		log.Fatalf("web3 client: %v", err)
	}
	defer w3.Close()

	// Indexer client selects ZGS (file-storage) nodes for the write path.
	// NOTE: SelectNodes returns *transfer.SelectedNodes whose Trusted field is
	// []*node.ZgsClient — these are storage nodes, NOT KV nodes. Do not try
	// to derive the KV node URL from this; use PI_ZG_KV_NODE for that.
	// NOTE: NewClient takes (url string, option IndexerClientOption) — NOT variadic.
	// Pass a zero-value IndexerClientOption when no special options are needed.
	idx, err := indexer.NewClient(indexerURL, indexer.IndexerClientOption{})
	if err != nil {
		log.Fatalf("indexer client: %v", err)
	}
	// SelectNodes(ctx, expectedReplica uint, dropped []string, method string, fullTrusted bool)
	// method must be "min" / "max" / "random" / numeric — empty string fails with
	// "cannot select a subset that meets the replication requirement". The SDK
	// CLI's upload command defaults to "min" + fullTrusted=true; mirror that.
	nodes, err := idx.SelectNodes(context.Background(), 1, []string{}, "min", true)
	if err != nil {
		log.Fatalf("select nodes: %v", err)
	}
	// SelectedNodes has no Close() method — do not defer nodes.Close().

	if len(nodes.Trusted) == 0 && len(nodes.Discovered) == 0 {
		log.Fatal("no nodes returned from indexer")
	}

	// KV read client (optional — write-only smoke runs if PI_ZG_KV_NODE not set).
	// The indexer returns ZGS file-storage nodes; KV reads need a dedicated KV node.
	// If you don't have a KV node URL, the smoke still verifies the write path —
	// success = tx submitted to testnet without error.
	var kvClient *kv.Client
	if kvNodeURL != "" {
		kvNode := node.MustNewKvClient(kvNodeURL)
		defer kvNode.Close()
		kvClient = kv.NewClient(kvNode)
	}

	// streamId derived from a namespace string by sha256.
	streamId := sha256Hash("zg-smoke-ns")
	key := []byte("hello")
	val := []byte(fmt.Sprintf("world-%d", time.Now().Unix()))

	// Write via Batcher.
	// IMPORTANT: batcher.Set(...) mutates the embedded *streamDataBuilder in place
	// AND returns *streamDataBuilder for chaining — but the return can be discarded.
	// Do NOT call Build() separately; batcher.Exec() calls it internally.
	// This matches the SDK CLI's own cmd/kv_write.go pattern.
	batcher := kv.NewBatcher(0, nodes, w3)
	batcher.Set(streamId, key, val)

	txHash, err := batcher.Exec(context.Background())
	if err != nil {
		log.Fatalf("exec: %v", err)
	}
	fmt.Printf("[wrote] tx=%s stream=%s key=%s val=%s\n", txHash.Hex(), streamId.Hex(), key, val)

	if kvClient == nil {
		fmt.Println("[skip read — PI_ZG_KV_NODE not set; write path verified by tx hash above]")
		fmt.Println("OK (write-only)")
		return
	}

	// Wait for testnet confirmation.
	fmt.Println("[waiting 5s for confirmation...]")
	time.Sleep(5 * time.Second)

	// Read back via KV client.
	got, err := kvClient.GetValue(context.Background(), streamId, key)
	if err != nil {
		log.Fatalf("getvalue: %v", err)
	}
	fmt.Printf("[read]  val=%s\n", got.Data)

	if string(got.Data) != string(val) {
		log.Fatalf("MISMATCH: wrote %q, read %q", val, got.Data)
	}
	fmt.Println("OK")
}

func sha256Hash(s string) common.Hash {
	h := sha256.Sum256([]byte(s))
	return common.BytesToHash(h[:])
}
