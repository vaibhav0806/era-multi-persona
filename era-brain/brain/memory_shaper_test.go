package brain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

func TestBareHistoryShaper_TruncatesAtMaxChars(t *testing.T) {
	shaper := brain.BareHistoryShaper(50)
	in := brain.Input{TaskID: "t1"}
	out := brain.Output{Text: strings.Repeat("x", 100)}
	got := shaper(in, out)
	require.LessOrEqual(t, len(got), 50, "should truncate")
	require.NotEmpty(t, got)
}

func TestBareHistoryShaper_ReturnsEmptyForEmptyText(t *testing.T) {
	shaper := brain.BareHistoryShaper(100)
	got := shaper(brain.Input{}, brain.Output{Text: ""})
	require.Equal(t, "", got, "empty text → no observation")
}

func TestBareHistoryShaper_PassesShortTextThrough(t *testing.T) {
	shaper := brain.BareHistoryShaper(100)
	got := shaper(brain.Input{}, brain.Output{Text: "hello world"})
	require.Equal(t, "hello world", got)
}
