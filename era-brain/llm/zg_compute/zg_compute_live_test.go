//go:build zg_live

package zg_compute_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
)

func TestZGCompute_LiveTestnet_SealedRoundtrip(t *testing.T) {
	endpoint := os.Getenv("PI_ZG_COMPUTE_ENDPOINT")
	bearer := os.Getenv("PI_ZG_COMPUTE_BEARER")
	model := os.Getenv("PI_ZG_COMPUTE_MODEL")
	if endpoint == "" || bearer == "" || model == "" {
		t.Skip("PI_ZG_COMPUTE_ENDPOINT/BEARER/MODEL required")
	}

	p := zg_compute.New(zg_compute.Config{
		BearerToken:      bearer,
		ProviderEndpoint: endpoint,
		DefaultModel:     model,
	})

	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "You answer with exactly one digit.",
		UserPrompt:   "What is 2+2? Answer with just the digit.",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Text)
	require.True(t, resp.Sealed, "TEE-signature header should be present on real testnet response")
}
