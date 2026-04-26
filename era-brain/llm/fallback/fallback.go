// Package fallback wraps two llm.Provider impls — a Primary and a Secondary —
// implementing try-primary-then-secondary semantics with an optional error hook.
//
// Use it to combine llm/zg_compute (sealed inference) with llm/openrouter
// (unsealed) so era-brain personas keep working when 0G Compute is flaky.
// Receipts inherit whichever provider succeeded — the caller can inspect
// llm.Response.Sealed to learn which path ran.
package fallback

import (
	"context"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

// FallbackErrorHandler is called when the primary provider returns an error
// (before the secondary is tried). Use to log + monitor; do not panic.
type FallbackErrorHandler func(primaryErr error)

// Provider implements llm.Provider with primary-first / secondary-fallback
// semantics.
type Provider struct {
	primary    llm.Provider
	secondary  llm.Provider
	onFallback FallbackErrorHandler
}

var _ llm.Provider = (*Provider)(nil)

// New constructs a fallback.Provider. onFallback may be nil.
func New(primary, secondary llm.Provider, onFallback FallbackErrorHandler) *Provider {
	return &Provider{primary: primary, secondary: secondary, onFallback: onFallback}
}

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	resp, err := p.primary.Complete(ctx, req)
	if err == nil {
		return resp, nil
	}
	if p.onFallback != nil {
		p.onFallback(err)
	}
	resp2, err2 := p.secondary.Complete(ctx, req)
	if err2 != nil {
		return llm.Response{}, fmt.Errorf("primary: %w; secondary: %w", err, err2)
	}
	return resp2, nil
}
