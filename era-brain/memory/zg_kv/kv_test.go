package zg_kv_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
)

// fakeKVOps records writes in-memory; reads return what was last written.
type fakeKVOps struct {
	store map[string][]byte // key = streamID + "/" + key
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

func TestZGKV_PutAndGet(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	ctx := context.Background()

	require.NoError(t, p.PutKV(ctx, "planner-mem", "userX", []byte("hello")))
	got, err := p.GetKV(ctx, "planner-mem", "userX")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), got)
}

func TestZGKV_GetMissingReturnsErrNotFound(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	_, err := p.GetKV(context.Background(), "ns", "missing")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestZGKV_AppendLogReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	err := p.AppendLog(context.Background(), "ns", []byte("e"))
	require.ErrorIs(t, err, zg_kv.ErrLogUnsupported)
}

func TestZGKV_ReadLogReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	_, err := p.ReadLog(context.Background(), "ns")
	require.ErrorIs(t, err, zg_kv.ErrLogUnsupported)
}

func TestZGKV_NamespaceIsolation(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "planner-mem", "u1", []byte("p")))
	require.NoError(t, p.PutKV(ctx, "coder-mem", "u1", []byte("c")))

	got1, err := p.GetKV(ctx, "planner-mem", "u1")
	require.NoError(t, err)
	require.Equal(t, []byte("p"), got1)

	got2, err := p.GetKV(ctx, "coder-mem", "u1")
	require.NoError(t, err)
	require.Equal(t, []byte("c"), got2)
}
