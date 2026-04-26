// Package llm defines the LLMProvider interface and ships reference impls.
// Real impls live in subpackages: llm/openrouter (M7-A), llm/zg_compute (M7-C).
package llm

import "context"

// Request is the minimal completion shape era-brain depends on.
// Tool-use, streaming, and function-calling are out of scope for M7-A and live as
// extensions on the impl side; brain orchestration only needs prompt → text.
type Request struct {
	SystemPrompt string
	UserPrompt   string
	Model        string  // optional override; empty = use Provider's default
	MaxTokens    int     // 0 = provider default
	Temperature  float32 // 0 = provider default
}

// Response is what a completion returns. Sealed=true only when the impl produced
// an attested receipt (M7-C 0G Compute path); openrouter impl always returns false.
type Response struct {
	Text       string
	Model      string // model the provider actually used (may differ from Request.Model)
	Sealed     bool
	InputHash  string // sha256 of (SystemPrompt+UserPrompt+Model); set by impls for receipt building
	OutputHash string // sha256 of Text; set by impls
}

// Provider is the LLM completion interface. Impls must be safe for concurrent use.
type Provider interface {
	Complete(ctx context.Context, req Request) (Response, error)
}
