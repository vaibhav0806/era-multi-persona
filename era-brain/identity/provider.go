// Package identity defines the IdentityResolver interface for persona name → metadata lookup.
// Reference impl in identity/ens lands in M7-E.
package identity

import "context"

// Resolution carries the result of a name → identity lookup.
type Resolution struct {
	Name             string // e.g. "coder.vaibhav-era.eth"
	INFTContractAddr string // text record from the resolver
	INFTTokenID      string
	MemoryURI        string // 0G Storage URI for the persona's memory blob (M7-B)
}

// Resolver does name → metadata lookups and (for owners) subname registration.
type Resolver interface {
	Resolve(ctx context.Context, name string) (Resolution, error)
	RegisterSubname(ctx context.Context, parent, label string, res Resolution) error
}
