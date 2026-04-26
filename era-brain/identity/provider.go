// Package identity defines the Resolver interface for persona name → metadata
// lookup + subname management. Reference impl in identity/ens lands in M7-E.
package identity

import "context"

// Resolver registers ENS subnames + reads/writes their text records.
// Implementations: identity/ens.Provider (M7-E.1).
type Resolver interface {
	// EnsureSubname registers <label>.<parent> if not already owned by signer.
	// Idempotent.
	EnsureSubname(ctx context.Context, label string) error

	// SetTextRecord overwrites a text record. Idempotent: skips tx if value matches.
	SetTextRecord(ctx context.Context, label, key, value string) error

	// ReadTextRecord returns "" with nil error when key is unset.
	ReadTextRecord(ctx context.Context, label, key string) (string, error)

	// ParentName returns the configured parent ENS name, e.g. "vaibhav-era.eth".
	ParentName() string
}
