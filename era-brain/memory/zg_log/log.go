// Package zg_log is a memory.Provider impl backed by 0G Storage KV streams,
// where keys are monotonic 6-digit sequence numbers so iteration returns
// entries in append order. Mirror image of memory/zg_kv: this package
// supports Log semantics; KV methods return ErrKVUnsupported.
package zg_log

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// kvOps mirrors zg_kv's interface. Same shape so callers can pass a
// *zg_kv.LiveOps directly to zg_log.NewWithOps.
type kvOps interface {
	Set(ctx context.Context, streamID string, key, val []byte) error
	Get(ctx context.Context, streamID string, key []byte) ([]byte, error)
	Iterate(ctx context.Context, streamID string) ([][2][]byte, error)
}

// ErrKeyNotFound — returned by ops.Get when a key is missing. Kept locally so
// the test fake can use zg_log.ErrKeyNotFound without importing zg_kv.
var ErrKeyNotFound = errors.New("zg_log: key not found")

// ErrKVUnsupported — KV methods on a Log provider always return this.
var ErrKVUnsupported = errors.New("zg_log: KV semantics not supported; use memory/zg_kv")

// Provider implements memory.Provider on top of 0G KV streams using
// sequence-numbered keys.
type Provider struct {
	ops kvOps

	// per-namespace counters guarded by mu so concurrent AppendLog calls
	// against the same namespace serialize on sequence-number assignment.
	// Single-process scope (M7-B.1); multi-process writers to the same
	// namespace would still race — out of scope.
	mu       sync.Mutex
	counters map[string]int
}

// NewWithOps constructs a Provider with the given kvOps. Pass a *zg_kv.LiveOps
// to share SDK init with the KV provider.
func NewWithOps(ops kvOps) *Provider {
	return &Provider{ops: ops, counters: map[string]int{}}
}

const seqWidth = 6 // "000001" — supports 999_999 entries per namespace

func streamID(ns string) string {
	h := sha256.Sum256([]byte(ns))
	return hex.EncodeToString(h[:])
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	sid := streamID(ns)

	p.mu.Lock()
	if _, ok := p.counters[ns]; !ok {
		entries, err := p.ops.Iterate(ctx, sid)
		if err != nil {
			p.mu.Unlock()
			return fmt.Errorf("zg_log appendlog (init counter): %w", err)
		}
		p.counters[ns] = len(entries)
	}
	p.counters[ns]++
	seq := p.counters[ns]
	p.mu.Unlock()

	// Counter+Set non-atomicity: if Set fails after counter advances, the
	// sequence number is "lost". Iterate-based reads tolerate the gap.
	// Survivable for M7-B.1 single-process scope.
	key := []byte(fmt.Sprintf("%0*d", seqWidth, seq))
	if err := p.ops.Set(ctx, sid, key, entry); err != nil {
		return fmt.Errorf("zg_log appendlog: %w", err)
	}
	return nil
}

func (p *Provider) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	entries, err := p.ops.Iterate(ctx, streamID(ns))
	if err != nil {
		return nil, fmt.Errorf("zg_log readlog: %w", err)
	}
	out := make([][]byte, 0, len(entries))
	for _, kv := range entries {
		out = append(out, kv[1])
	}
	return out, nil
}

func (p *Provider) GetKV(_ context.Context, _, _ string) ([]byte, error) {
	return nil, ErrKVUnsupported
}

func (p *Provider) PutKV(_ context.Context, _, _ string, _ []byte) error {
	return ErrKVUnsupported
}
