// Package memory defines the MemoryProvider interface and ships reference impls.
//
// A Provider exposes both KV (mutable) and Log (append-only) semantics so a single
// dependency injection point covers persona memory (KV) and audit history (Log).
// Real impls live in subpackages: memory/sqlite (M7-A), memory/zg_kv + memory/zg_log (M7-B).
package memory

import (
	"context"
	"errors"
)

// ErrNotFound is returned by GetKV when the (namespace, key) pair has no value.
var ErrNotFound = errors.New("memory: not found")

// Provider is the unified KV + Log interface. Impls must be safe for concurrent use.
//
// KV semantics: last-write-wins on (ns, key). Get of missing key returns ErrNotFound.
// Log semantics: append-only, ordered. ReadLog returns entries in insertion order.
type Provider interface {
	GetKV(ctx context.Context, ns, key string) ([]byte, error)
	PutKV(ctx context.Context, ns, key string, val []byte) error
	AppendLog(ctx context.Context, ns string, entry []byte) error
	ReadLog(ctx context.Context, ns string) ([][]byte, error)
}
