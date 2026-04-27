package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubENS struct {
	parent  string
	values  map[string]string // key = "label:textKey"
	failKey string            // when set, ReadTextRecord returns error for this key
}

func (s *stubENS) ParentName() string { return s.parent }

func (s *stubENS) ReadTextRecord(_ context.Context, label, key string) (string, error) {
	if s.failKey != "" && key == s.failKey {
		return "", errStub
	}
	return s.values[label+":"+key], nil
}

var errStub = stubErr("stub: simulated read error")

type stubErr string

func (e stubErr) Error() string { return string(e) }

func TestEnsFooter_RendersAllThreePersonas(t *testing.T) {
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":      "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"planner:inft_token_id":  "0",
			"coder:inft_addr":        "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"coder:inft_token_id":    "1",
			"reviewer:inft_addr":     "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"reviewer:inft_token_id": "2",
		},
	}
	footer := ensFooter(context.Background(), stub, nil)
	require.Contains(t, footer, "personas:")
	require.Contains(t, footer, "planner.vaibhav-era.eth")
	require.Contains(t, footer, "coder.vaibhav-era.eth")
	require.Contains(t, footer, "reviewer.vaibhav-era.eth")
	require.Contains(t, footer, "token #0")
	require.Contains(t, footer, "token #1")
	require.Contains(t, footer, "token #2")
	require.Contains(t, footer, "0x33847c")
	require.True(t, strings.HasPrefix(footer, "\n\n"), "footer should start with double newline for separation")
}

func TestEnsFooter_NilResolverReturnsEmpty(t *testing.T) {
	footer := ensFooter(context.Background(), nil, nil)
	require.Equal(t, "", footer)
}

func TestEnsFooter_ReadFailureReturnsEmpty(t *testing.T) {
	stub := &stubENS{
		parent:  "vaibhav-era.eth",
		values:  map[string]string{},
		failKey: "inft_addr",
	}
	footer := ensFooter(context.Background(), stub, nil)
	require.Equal(t, "", footer, "any read failure should drop the entire footer to avoid partial DMs")
}

func TestEnsFooter_PartialDataReturnsEmpty(t *testing.T) {
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":     "0xabc",
			"planner:inft_token_id": "0",
			// coder + reviewer absent
		},
	}
	footer := ensFooter(context.Background(), stub, nil)
	require.Equal(t, "", footer)
}

func TestEnsFooter_CustomPersonaLabels(t *testing.T) {
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":       "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"planner:inft_token_id":   "0",
			"rustacean:inft_addr":     "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"rustacean:inft_token_id": "3",
			"reviewer:inft_addr":      "0x33847c5500C2443E2f3BBf547d9b069B334c3D16",
			"reviewer:inft_token_id":  "2",
		},
	}
	labels := []string{"planner", "rustacean", "reviewer"}
	footer := ensFooter(context.Background(), stub, labels)
	require.Contains(t, footer, "rustacean.vaibhav-era.eth")
	require.Contains(t, footer, "token #3")
	require.NotContains(t, footer, "coder.")
}

func TestEnsFooter_NilLabelsFallsBackToDefaults(t *testing.T) {
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":      "0xabc",
			"planner:inft_token_id":  "0",
			"coder:inft_addr":        "0xabc",
			"coder:inft_token_id":    "1",
			"reviewer:inft_addr":     "0xabc",
			"reviewer:inft_token_id": "2",
		},
	}
	footer := ensFooter(context.Background(), stub, nil)
	require.Contains(t, footer, "planner.vaibhav-era.eth")
	require.Contains(t, footer, "coder.vaibhav-era.eth")
	require.Contains(t, footer, "reviewer.vaibhav-era.eth")
}
