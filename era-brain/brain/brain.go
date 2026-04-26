package brain

import (
	"context"
	"errors"
	"fmt"
)

// Brain orchestrates a sequential chain of Personas. Each Persona sees prior
// Personas' Outputs in the order they ran. Brain accumulates Receipts and stops
// at the first error (subsequent Personas don't run).
type Brain struct{}

// New returns a Brain. It's stateless; the type exists for forward-compatibility
// (M7-B may add Brain-level memory hooks).
func New() *Brain { return &Brain{} }

// Result is what Brain.Run returns: the per-persona outputs (in run order) and
// flattened receipts (in run order).
type Result struct {
	Outputs  []Output
	Receipts []Receipt
}

// Run executes the persona chain. Returns whatever outputs/receipts completed
// successfully even on partial failure — caller can inspect Result.Outputs to
// see how far the chain progressed before erroring.
func (b *Brain) Run(ctx context.Context, in Input, personas []Persona) (Result, error) {
	if len(personas) == 0 {
		return Result{}, errors.New("brain: empty persona list")
	}
	var res Result
	for _, p := range personas {
		stepIn := in
		stepIn.PriorOutputs = append([]Output(nil), res.Outputs...)
		out, err := p.Run(ctx, stepIn)
		if err != nil {
			return res, fmt.Errorf("persona %q failed: %w", p.Name(), err)
		}
		res.Outputs = append(res.Outputs, out)
		res.Receipts = append(res.Receipts, out.Receipt)
	}
	return res, nil
}
