// Package brain defines the core interfaces and orchestration of era-brain.
package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Receipt records one persona invocation. M7-A produces unsealed receipts only;
// M7-C extends this with sealed-inference attestations from 0G Compute.
type Receipt struct {
	Persona       string // "planner", "coder", "reviewer", or custom
	Model         string // e.g. "qwen3.6-plus" or OpenRouter model id
	InputHash     string // sha256 of input prompt
	OutputHash    string // sha256 of output text
	Sealed        bool   // true only when the LLMProvider produced an attested receipt
	TimestampUnix int64  // unix seconds at completion
}

// ReceiptHash returns a deterministic 64-char hex digest over the receipt's fields.
// Used as the on-chain attestation key in M7-D's recordInvocation.
func ReceiptHash(r Receipt) string {
	h := sha256.New()
	// Pipe-delimited canonical form. DO NOT change separator or field order
	// without migrating any on-chain records that reference these hashes
	// (M7-D's recordInvocation logs ReceiptHash outputs).
	fmt.Fprintf(h, "%s|%s|%s|%s|%t|%d", r.Persona, r.Model, r.InputHash, r.OutputHash, r.Sealed, r.TimestampUnix)
	return hex.EncodeToString(h.Sum(nil))
}
