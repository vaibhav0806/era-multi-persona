package zg_kv

import (
	"context"
	"fmt"
	"time"

	"github.com/0gfoundation/0g-storage-client/common/blockchain"
	"github.com/0gfoundation/0g-storage-client/indexer"
	"github.com/0gfoundation/0g-storage-client/kv"
	"github.com/0gfoundation/0g-storage-client/node"
	"github.com/0gfoundation/0g-storage-client/transfer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/openweb3/web3go"
)

// LiveOpsConfig configures a kvOps backed by the real 0G SDK.
type LiveOpsConfig struct {
	PrivateKey   string        // hex (with or without 0x prefix)
	EVMRPCURL    string        // e.g. https://evmrpc-testnet.0g.ai
	IndexerURL   string        // e.g. https://indexer-storage-testnet-turbo.0g.ai
	KVNodeURL    string        // optional; if empty Get returns ErrKeyNotFound for everything
	WriteTimeout time.Duration // default 30s
}

// LiveOps is a kvOps backed by the real 0G SDK. Construct with NewLiveOps.
type LiveOps struct {
	cfg    LiveOpsConfig
	w3     *web3go.Client
	idx    *indexer.Client
	read   *kv.Client  // nil if KVNodeURL empty
	kvNode *node.KvClient // nil if KVNodeURL empty
	nodes  *transfer.SelectedNodes
}

// NewLiveOps constructs a LiveOps. Caller must call Close when done.
func NewLiveOps(cfg LiveOpsConfig) (*LiveOps, error) {
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	w3, err := blockchain.NewWeb3(cfg.EVMRPCURL, cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("web3: %w", err)
	}
	idx, err := indexer.NewClient(cfg.IndexerURL, indexer.IndexerClientOption{})
	if err != nil {
		w3.Close()
		return nil, fmt.Errorf("indexer client: %w", err)
	}
	// SelectNodes(ctx, expectedReplica, dropped, method, fullTrusted)
	// method="min" + fullTrusted=true mirrors SDK CLI defaults; empty method fails.
	nodes, err := idx.SelectNodes(context.Background(), 1, []string{}, "min", true)
	if err != nil {
		w3.Close()
		return nil, fmt.Errorf("select nodes: %w", err)
	}
	if len(nodes.Trusted) == 0 && len(nodes.Discovered) == 0 {
		w3.Close()
		return nil, fmt.Errorf("no storage nodes available")
	}

	live := &LiveOps{cfg: cfg, w3: w3, idx: idx, nodes: nodes}

	if cfg.KVNodeURL != "" {
		kvNode := node.MustNewKvClient(cfg.KVNodeURL)
		live.kvNode = kvNode
		live.read = kv.NewClient(kvNode)
	}
	return live, nil
}

// Close releases SDK resources. Safe to call multiple times.
func (l *LiveOps) Close() {
	if l.kvNode != nil {
		l.kvNode.Close()
	}
	if l.w3 != nil {
		l.w3.Close()
	}
}

func (l *LiveOps) Set(ctx context.Context, streamID string, key, val []byte) error {
	ctx, cancel := context.WithTimeout(ctx, l.cfg.WriteTimeout)
	defer cancel()

	streamHash := common.HexToHash("0x" + streamID)
	batcher := kv.NewBatcher(0, l.nodes, l.w3)
	batcher.Set(streamHash, key, val) // discard return — Set mutates embedded state; Exec calls Build internally
	if _, err := batcher.Exec(ctx); err != nil {
		return fmt.Errorf("batcher exec: %w", err)
	}
	return nil
}

func (l *LiveOps) Get(ctx context.Context, streamID string, key []byte) ([]byte, error) {
	if l.read == nil {
		// No KV node URL configured — treat as "not found" so the dual
		// provider falls through to the cache cleanly.
		return nil, ErrKeyNotFound
	}
	streamHash := common.HexToHash("0x" + streamID)
	val, err := l.read.GetValue(ctx, streamHash, key)
	if err != nil {
		// Map ALL errors (network, not-found, RPC) to ErrKeyNotFound.
		// Hackathon-scope decision; refactor when 0G testnet KV nodes stabilize.
		return nil, ErrKeyNotFound
	}
	if val == nil || len(val.Data) == 0 {
		return nil, ErrKeyNotFound
	}
	return val.Data, nil
}

func (l *LiveOps) Iterate(ctx context.Context, streamID string) ([][2][]byte, error) {
	if l.read == nil {
		return nil, nil // empty — caller's "no entries" branch
	}
	streamHash := common.HexToHash("0x" + streamID)
	iter := l.read.NewIterator(streamHash)
	if err := iter.SeekToFirst(ctx); err != nil {
		// Treat iterate failure as empty — same fallthrough rationale as Get.
		return nil, nil
	}
	var out [][2][]byte
	for iter.Valid() {
		// iter.KeyValue() returns *node.KeyValue with .Key and .Data fields.
		entry := iter.KeyValue()
		out = append(out, [2][]byte{entry.Key, entry.Data})
		if err := iter.Next(ctx); err != nil {
			break
		}
	}
	return out, nil
}
