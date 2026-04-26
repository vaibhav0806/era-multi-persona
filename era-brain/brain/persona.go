package brain

import "context"

// Persona is one stage in a Brain run. It receives the threaded conversation
// state, produces output, and writes a Receipt. Impls choose how to use the
// underlying LLMProvider and MemoryProvider; brain only orchestrates the chain.
type Persona interface {
	Name() string
	Run(ctx context.Context, in Input) (Output, error)
}

// Input threads task context through the persona chain. Each successive persona
// sees prior personas' outputs in PriorOutputs (in order).
type Input struct {
	TaskID          string
	UserID          string
	TaskDescription string
	PriorOutputs    []Output // populated by Brain; planner sees [], coder sees [planner.Output], reviewer sees [planner.Output, coder.Output]
}

// Output is what a persona emits. Brain accumulates Outputs and threads them.
type Output struct {
	PersonaName string
	Text        string
	Receipt     Receipt
}
