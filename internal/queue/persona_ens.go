package queue

import (
	"context"
	"fmt"
)

// SyncPersonaENSRecords runs EnsureSubname + the four canonical text records
// (inft_addr, inft_token_id, zg_storage_uri, description) for a single persona.
// Used by both Queue.MintPersona (Phase 3) and the boot-time ENS reconcile
// pass (Phase 5). Errors are returned to the caller; logging is the caller's
// responsibility so each call site can decide whether to fail-open or fail-closed.
//
// Exported (capital S) so packages outside internal/queue (e.g., the
// orchestrator's reconcile helper in cmd/orchestrator) can reuse the body
// without copy-paste.
//
// If ens is nil, this is a no-op returning nil — production wiring without
// PI_ZG_ENS_* env vars is supported.
func SyncPersonaENSRecords(ctx context.Context, ens ENSWriter, p Persona, inftAddr string) error {
	if ens == nil {
		return nil
	}
	if err := ens.EnsureSubname(ctx, p.Name); err != nil {
		return fmt.Errorf("ensureSubname: %w", err)
	}
	desc := p.Description
	if len(desc) > 60 {
		desc = desc[:60]
	}
	records := map[string]string{
		"inft_addr":      inftAddr,
		"inft_token_id":  p.TokenID,
		"zg_storage_uri": p.SystemPromptURI,
		"description":    desc,
	}
	for k, v := range records {
		if err := ens.SetTextRecord(ctx, p.Name, k, v); err != nil {
			return fmt.Errorf("setText %s: %w", k, err)
		}
	}
	return nil
}
