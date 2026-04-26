package fallback_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/fallback"
)

type fakeLLM struct {
	resp llm.Response
	err  error
}

func (f *fakeLLM) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	return f.resp, f.err
}

func TestFallback_PrimarySuccess_NoFallbackHook(t *testing.T) {
	primary := &fakeLLM{resp: llm.Response{Text: "primary", Model: "p", Sealed: true}}
	secondary := &fakeLLM{resp: llm.Response{Text: "secondary", Model: "s", Sealed: false}}
	hookCalls := 0
	p := fallback.New(primary, secondary, func(err error) { hookCalls++ })

	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.Equal(t, "primary", resp.Text)
	require.True(t, resp.Sealed)
	require.Equal(t, 0, hookCalls, "primary success → no hook")
}

func TestFallback_PrimaryFail_SecondarySuccess_HookCalled(t *testing.T) {
	primaryErr := errors.New("primary down")
	primary := &fakeLLM{err: primaryErr}
	secondary := &fakeLLM{resp: llm.Response{Text: "secondary", Model: "s", Sealed: false}}
	var hookErr error
	p := fallback.New(primary, secondary, func(err error) { hookErr = err })

	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.Equal(t, "secondary", resp.Text)
	require.False(t, resp.Sealed, "secondary's Sealed flag passes through (false for openrouter)")
	require.ErrorIs(t, hookErr, primaryErr)
}

func TestFallback_BothFail_ErrorWrapsBoth(t *testing.T) {
	primaryErr := errors.New("primary down")
	secondaryErr := errors.New("secondary down")
	primary := &fakeLLM{err: primaryErr}
	secondary := &fakeLLM{err: secondaryErr}
	hookCalls := 0
	p := fallback.New(primary, secondary, func(err error) { hookCalls++ })

	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "primary down")
	require.Contains(t, err.Error(), "secondary down")
	require.Equal(t, 1, hookCalls, "hook called once for primary failure even when secondary also fails")
}

func TestFallback_NilHookOK(t *testing.T) {
	primary := &fakeLLM{err: errors.New("primary down")}
	secondary := &fakeLLM{resp: llm.Response{Text: "ok"}}
	p := fallback.New(primary, secondary, nil) // nil hook = noop

	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
}
