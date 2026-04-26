package brain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReceiptHash_DeterministicForSameInputs(t *testing.T) {
	r1 := Receipt{Persona: "planner", Model: "qwen3.6-plus", InputHash: "abc", OutputHash: "def", Sealed: true, TimestampUnix: 1700000000}
	r2 := Receipt{Persona: "planner", Model: "qwen3.6-plus", InputHash: "abc", OutputHash: "def", Sealed: true, TimestampUnix: 1700000000}
	require.Equal(t, ReceiptHash(r1), ReceiptHash(r2))
}

func TestReceiptHash_DiffersWhenSealedFlagFlips(t *testing.T) {
	r1 := Receipt{Persona: "coder", Model: "x", InputHash: "i", OutputHash: "o", Sealed: true, TimestampUnix: 1}
	r2 := r1
	r2.Sealed = false
	require.NotEqual(t, ReceiptHash(r1), ReceiptHash(r2))
}

func TestReceiptHash_HexString64Chars(t *testing.T) {
	h := ReceiptHash(Receipt{Persona: "p", Model: "m", InputHash: "i", OutputHash: "o", Sealed: false, TimestampUnix: 1})
	require.Len(t, h, 64)
	require.NotContains(t, h, " ")
	require.True(t, strings.ContainsAny(h, "0123456789abcdef"))
}
