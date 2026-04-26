package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

type fakeLLM struct {
	respText string
	model    string
	sealed   bool
}

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	return llm.Response{
		Text:   f.respText + " (echo: " + strings.TrimSpace(req.UserPrompt) + ")",
		Model:  f.model,
		Sealed: f.sealed,
	}, nil
}

func TestLLMProviderContract_BasicComplete(t *testing.T) {
	var p llm.Provider = &fakeLLM{respText: "ok", model: "test-model", sealed: false}
	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "you are a helper",
		UserPrompt:   "hello",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Text, "echo: hello")
	require.Equal(t, "test-model", resp.Model)
	require.False(t, resp.Sealed)
}

func TestLLMProviderContract_SealedFlagPropagates(t *testing.T) {
	var p llm.Provider = &fakeLLM{respText: "ok", model: "qwen3.6-plus", sealed: true}
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.True(t, resp.Sealed)
}
