// Package dual wraps two memory.Provider impls — a fast local Cache and a
// canonical Primary — implementing write-both/read-cache-first semantics.
//
// Use it to combine memory/sqlite (cache) with memory/zg_kv + memory/zg_log
// (primary) so era-brain has both 0G's tamper-proof record AND fast local
// reads, with primary failures degrading gracefully (logged but non-fatal).
package dual

import (
	"context"
	"errors"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// PrimaryErrorHandler is called when a primary write fails. Operation is
// "put_kv" or "append_log". Use to log + monitor; do not panic.
type PrimaryErrorHandler func(op string, err error)

// Provider implements memory.Provider as a write-both/read-cache-first wrapper.
type Provider struct {
	cache    memory.Provider
	primary  memory.Provider
	onErrPri PrimaryErrorHandler
}

// New constructs a dual.Provider. onPrimaryError is optional (nil swallows
// failures silently); pass a function to log them.
func New(cache, primary memory.Provider, onPrimaryError PrimaryErrorHandler) *Provider {
	return &Provider{cache: cache, primary: primary, onErrPri: onPrimaryError}
}

func (p *Provider) reportPrimary(op string, err error) {
	if err == nil || p.onErrPri == nil {
		return
	}
	p.onErrPri(op, err)
}

func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if err := p.cache.PutKV(ctx, ns, key, val); err != nil {
		return fmt.Errorf("dual cache putkv: %w", err)
	}
	if err := p.primary.PutKV(ctx, ns, key, val); err != nil {
		p.reportPrimary("put_kv", err)
	}
	return nil
}

func (p *Provider) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	val, err := p.cache.GetKV(ctx, ns, key)
	if err == nil {
		return val, nil
	}
	if !errors.Is(err, memory.ErrNotFound) {
		// Cache errored for a non-not-found reason — fall through to primary
		// rather than failing, since the primary is the canonical source.
		// Trades visibility for resilience.
	}
	return p.primary.GetKV(ctx, ns, key)
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	if err := p.cache.AppendLog(ctx, ns, entry); err != nil {
		return fmt.Errorf("dual cache appendlog: %w", err)
	}
	if err := p.primary.AppendLog(ctx, ns, entry); err != nil {
		p.reportPrimary("append_log", err)
	}
	return nil
}

func (p *Provider) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	entries, err := p.cache.ReadLog(ctx, ns)
	if err == nil && len(entries) > 0 {
		return entries, nil
	}
	// Empty cache OR cache error — fall through to primary.
	return p.primary.ReadLog(ctx, ns)
}
