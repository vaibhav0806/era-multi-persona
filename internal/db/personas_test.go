package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestPersonas_InsertAndGet(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	p := queue.Persona{
		TokenID:         "1",
		Name:            "alice",
		OwnerAddr:       "0xabc",
		SystemPromptURI: "ipfs://prompt",
		ENSSubname:      "alice.era.eth",
		Description:     "test persona",
	}
	require.NoError(t, r.InsertPersona(ctx, p))

	got, err := r.GetPersonaByName(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "1", got.TokenID)
	require.Equal(t, "alice", got.Name)
	require.Equal(t, "0xabc", got.OwnerAddr)
	require.Equal(t, "ipfs://prompt", got.SystemPromptURI)
	require.Equal(t, "alice.era.eth", got.ENSSubname)
	require.Equal(t, "test persona", got.Description)
	require.False(t, got.CreatedAt.IsZero())
}

func TestPersonas_DuplicateName(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID:         "1",
		Name:            "alice",
		OwnerAddr:       "0xabc",
		SystemPromptURI: "ipfs://a",
	}))

	err := r.InsertPersona(ctx, queue.Persona{
		TokenID:         "2",
		Name:            "alice",
		OwnerAddr:       "0xdef",
		SystemPromptURI: "ipfs://b",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, queue.ErrPersonaNameTaken), "expected ErrPersonaNameTaken, got %v", err)
}

func TestPersonas_NotFound(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	_, err := r.GetPersonaByName(ctx, "nobody")
	require.Error(t, err)
	require.True(t, errors.Is(err, queue.ErrPersonaNotFound), "expected ErrPersonaNotFound, got %v", err)
}

func TestPersonas_List(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	// Insert out of order to verify ordering by token_id (numeric).
	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID: "10", Name: "ten", OwnerAddr: "0x10", SystemPromptURI: "ipfs://10",
	}))
	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID: "2", Name: "two", OwnerAddr: "0x2", SystemPromptURI: "ipfs://2",
	}))
	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID: "1", Name: "one", OwnerAddr: "0x1", SystemPromptURI: "ipfs://1",
	}))

	list, err := r.ListPersonas(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)
	// Numeric ascending: 1, 2, 10 (not lexicographic 1, 10, 2).
	require.Equal(t, "1", list[0].TokenID)
	require.Equal(t, "2", list[1].TokenID)
	require.Equal(t, "10", list[2].TokenID)
}

func TestPersonas_InsertWithPromptText(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID:         "1",
		Name:            "alice",
		OwnerAddr:       "0xabc",
		SystemPromptURI: "ipfs://prompt",
		Description:     "test persona",
		PromptText:      "You are alice. Be helpful.",
	}))

	prompt, err := r.GetPersonaPrompt(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "You are alice. Be helpful.", prompt)
}

func TestPersonas_GetPrompt_NotFound(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	_, err := r.GetPersonaPrompt(ctx, "nobody")
	require.Error(t, err)
	require.True(t, errors.Is(err, queue.ErrPersonaNotFound), "expected ErrPersonaNotFound, got %v", err)
}

func TestPersonas_GetPrompt_EmptyPromptIsValid(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID:         "1",
		Name:            "alice",
		OwnerAddr:       "0xabc",
		SystemPromptURI: "ipfs://prompt",
	}))

	prompt, err := r.GetPersonaPrompt(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "", prompt)
}

func TestPersonas_UpdatePromptText(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	require.NoError(t, r.InsertPersona(ctx, queue.Persona{
		TokenID:         "9",
		Name:            "needs-backfill",
		OwnerAddr:       "0xabc",
		SystemPromptURI: "zg://abc",
		// PromptText omitted — defaults to empty
	}))

	got, err := r.GetPersonaPrompt(ctx, "needs-backfill")
	require.NoError(t, err)
	require.Equal(t, "", got)

	require.NoError(t, r.UpdatePromptText(ctx, "needs-backfill", "RUSTACEAN-PROMPT"))

	got, err = r.GetPersonaPrompt(ctx, "needs-backfill")
	require.NoError(t, err)
	require.Equal(t, "RUSTACEAN-PROMPT", got)
}
