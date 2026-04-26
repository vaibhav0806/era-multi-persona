package brain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

// stubPersona returns a fixed Output. Used to test Brain orchestration logic
// without spinning up real LLMs/memory.
type stubPersona struct {
	name string
	text string
}

func (s *stubPersona) Name() string { return s.name }
func (s *stubPersona) Run(_ context.Context, in brain.Input) (brain.Output, error) {
	return brain.Output{
		PersonaName: s.name,
		Text:        s.text + "(saw " + boolToStr(len(in.PriorOutputs) > 0) + " prior)",
		Receipt: brain.Receipt{
			Persona:       s.name,
			Model:         "stub",
			InputHash:     "i",
			OutputHash:    "o",
			Sealed:        false,
			TimestampUnix: 1,
		},
	}, nil
}

func boolToStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func TestBrain_Run_ChainsPersonasInOrderAndThreadsPriorOutputs(t *testing.T) {
	b := brain.New()
	personas := []brain.Persona{
		&stubPersona{name: "planner", text: "plan-out"},
		&stubPersona{name: "coder", text: "code-out"},
		&stubPersona{name: "reviewer", text: "review-out"},
	}

	res, err := b.Run(context.Background(), brain.Input{
		TaskID:          "t1",
		UserID:          "u1",
		TaskDescription: "do the thing",
	}, personas)
	require.NoError(t, err)

	require.Len(t, res.Outputs, 3)
	require.Equal(t, "planner", res.Outputs[0].PersonaName)
	require.Contains(t, res.Outputs[0].Text, "saw no prior")
	require.Equal(t, "coder", res.Outputs[1].PersonaName)
	require.Contains(t, res.Outputs[1].Text, "saw yes prior")
	require.Equal(t, "reviewer", res.Outputs[2].PersonaName)
	require.Contains(t, res.Outputs[2].Text, "saw yes prior")

	require.Len(t, res.Receipts, 3)
	require.Equal(t, "planner", res.Receipts[0].Persona)
}

type errorPersona struct{}

func (errorPersona) Name() string { return "boom" }
func (errorPersona) Run(_ context.Context, _ brain.Input) (brain.Output, error) {
	return brain.Output{}, context.DeadlineExceeded
}

func TestBrain_Run_StopsOnFirstPersonaError(t *testing.T) {
	b := brain.New()
	personas := []brain.Persona{
		&stubPersona{name: "planner", text: "ok"},
		errorPersona{},
		&stubPersona{name: "should-not-run", text: "never"},
	}
	res, err := b.Run(context.Background(), brain.Input{TaskID: "t1"}, personas)
	require.Error(t, err)
	require.Len(t, res.Outputs, 1) // planner ran successfully; reviewer never started
}

func TestBrain_Run_EmptyPersonaListError(t *testing.T) {
	b := brain.New()
	_, err := b.Run(context.Background(), brain.Input{}, nil)
	require.Error(t, err)
}
