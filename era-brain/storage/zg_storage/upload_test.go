package zg_storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
	"github.com/vaibhav0806/era-multi-persona/era-brain/storage/zg_storage"
)

// fakeKVOps is the same shape as zg_kv's test fake. Inlined here to keep the
// dependency direction clean (zg_storage tests don't import zg_kv internals).
type fakeKVOps struct {
	store map[string][]byte
}

func newFakeKVOps() *fakeKVOps { return &fakeKVOps{store: map[string][]byte{}} }

func (f *fakeKVOps) Set(_ context.Context, streamID string, key, val []byte) error {
	f.store[streamID+"/"+string(key)] = val
	return nil
}

func (f *fakeKVOps) Get(_ context.Context, streamID string, key []byte) ([]byte, error) {
	v, ok := f.store[streamID+"/"+string(key)]
	if !ok {
		return nil, zg_kv.ErrKeyNotFound
	}
	return v, nil
}

func (f *fakeKVOps) Iterate(_ context.Context, _ string) ([][2][]byte, error) {
	return nil, zg_kv.ErrIterateUnsupported
}

func newClientWithFake(t *testing.T) *zg_storage.Client {
	t.Helper()
	kv := zg_kv.NewWithOps(newFakeKVOps())
	c := zg_storage.NewWithKV(kv)
	t.Cleanup(c.Close)
	return c
}

func TestUploadPrompt_ReturnsURI(t *testing.T) {
	c := newClientWithFake(t)

	uri, err := c.UploadPrompt(context.Background(), "You only write idiomatic Rust.")
	require.NoError(t, err)
	require.NotEmpty(t, uri)
	require.Contains(t, uri, "zg://", "URI should be prefixed with the zg:// scheme")
}

func TestUploadPrompt_Idempotent(t *testing.T) {
	c := newClientWithFake(t)
	ctx := context.Background()

	uri1, err := c.UploadPrompt(ctx, "deterministic content")
	require.NoError(t, err)
	uri2, err := c.UploadPrompt(ctx, "deterministic content")
	require.NoError(t, err)
	require.Equal(t, uri1, uri2, "same content should produce the same URI")

	// And FetchPrompt should round-trip.
	got, err := c.FetchPrompt(ctx, uri1)
	require.NoError(t, err)
	require.Equal(t, "deterministic content", got)
}
