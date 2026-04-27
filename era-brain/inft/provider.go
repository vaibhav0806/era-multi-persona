// Package inft defines the iNFT (ERC-7857) registry interface.
// Reference impl in inft/zg_7857 lands in M7-D.
package inft

import "context"

// Persona binds a persona to an iNFT. Returned by Lookup, used by callers to know
// which token to recordInvocation against after a run.
type Persona struct {
	Name            string
	TokenID         string
	ContractAddr    string
	OwnerAddr       string
	SystemPromptURI string // 0G Storage URI to the persona's system prompt blob (M7-B)
	MintTxHash      string // tx hash of the mint, populated by Registry.Mint (M7-F.1) — for DM-rendering chainscan link
}

// Registry exposes mint + lookup + invocation-recording. M7-A defines the
// interface; M7-D ships zg_7857 impl backed by a forked ERC-7857 contract.
type Registry interface {
	Mint(ctx context.Context, name, systemPromptURI string) (Persona, error)
	Lookup(ctx context.Context, ownerAddr, name string) (Persona, error)
	RecordInvocation(ctx context.Context, tokenID, receiptHash string) error
}
