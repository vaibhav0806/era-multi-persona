// Package zg_kv is a memory.Provider impl backed by 0G Storage KV streams.
//
// It supports KV semantics only. Log methods return ErrLogUnsupported —
// for log semantics use memory/zg_log, which uses the same underlying
// 0G KV streams API but with sequence-numbered keys.
package zg_kv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// ErrKeyNotFound is what kvOps.Get returns when the (streamID, key) pair has
// no value — including when the underlying KV node is unreachable. We
// deliberately conflate "actually missing" and "node down" here so the dual
// provider's cache-fallthrough works cleanly when 0G's testnet KV gateway is
// flaky (which it is). This is a hackathon-scope decision; production should
// distinguish the two.
var ErrKeyNotFound = errors.New("zg_kv: key not found")

// ErrLogUnsupported is returned by AppendLog/ReadLog. zg_kv does not support
// Log semantics — use memory/zg_log instead.
var ErrLogUnsupported = errors.New("zg_kv: log semantics not supported; use memory/zg_log")

// ErrIterateUnsupported is returned by Iterate when called on the KV provider.
var ErrIterateUnsupported = errors.New("zg_kv: iterate not supported on KV provider")

// kvOps is the interface seam between zg_kv.Provider and the 0G SDK.
// Tests inject a fake; production wraps github.com/0gfoundation/0g-storage-client/kv.
type kvOps interface {
	Set(ctx context.Context, streamID string, key, val []byte) error
	Get(ctx context.Context, streamID string, key []byte) ([]byte, error)
	Iterate(ctx context.Context, streamID string) ([][2][]byte, error)
}

// Provider implements memory.Provider on top of 0G KV streams.
type Provider struct {
	ops kvOps
}

// NewWithOps constructs a Provider with a custom kvOps. Used by tests and by
// memory/zg_log (which wraps the same SDK ops in a different shape).
func NewWithOps(ops kvOps) *Provider {
	return &Provider{ops: ops}
}

// streamID derives a stream ID from a namespace string. Sha256 hex so the
// same namespace always maps to the same stream and so namespaces don't
// collide on similar prefixes.
func streamID(ns string) string {
	h := sha256.Sum256([]byte(ns))
	return hex.EncodeToString(h[:])
}

func (p *Provider) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	val, err := p.ops.Get(ctx, streamID(ns), []byte(key))
	if errors.Is(err, ErrKeyNotFound) {
		return nil, memory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("zg_kv getkv: %w", err)
	}
	return val, nil
}

func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if err := p.ops.Set(ctx, streamID(ns), []byte(key), val); err != nil {
		return fmt.Errorf("zg_kv putkv: %w", err)
	}
	return nil
}

func (p *Provider) AppendLog(_ context.Context, _ string, _ []byte) error {
	return ErrLogUnsupported
}

func (p *Provider) ReadLog(_ context.Context, _ string) ([][]byte, error) {
	return nil, ErrLogUnsupported
}

// LiveOpsType is exported so other packages (e.g. memory/zg_log) can wrap a
// LiveOps as their own kvOps without needing to redeclare the interface.
// Keep this in sync with kvOps.
type LiveOpsType = kvOps
