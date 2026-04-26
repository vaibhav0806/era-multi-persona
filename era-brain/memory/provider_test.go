package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// fakeProvider is the in-memory reference impl used by the contract test.
// Real impls (sqlite, zg_kv, zg_log) must satisfy the same contract.
type fakeProvider struct {
	kv  map[string][]byte
	log map[string][][]byte
}

func newFake() *fakeProvider { return &fakeProvider{kv: map[string][]byte{}, log: map[string][][]byte{}} }

func (f *fakeProvider) GetKV(_ context.Context, ns, key string) ([]byte, error) {
	v, ok := f.kv[ns+"/"+key]
	if !ok {
		return nil, memory.ErrNotFound
	}
	return v, nil
}
func (f *fakeProvider) PutKV(_ context.Context, ns, key string, val []byte) error {
	f.kv[ns+"/"+key] = val
	return nil
}
func (f *fakeProvider) AppendLog(_ context.Context, ns string, entry []byte) error {
	f.log[ns] = append(f.log[ns], entry)
	return nil
}
func (f *fakeProvider) ReadLog(_ context.Context, ns string) ([][]byte, error) {
	return f.log[ns], nil
}

func TestMemoryProviderContract_KV_PutThenGetRoundtrips(t *testing.T) {
	var p memory.Provider = newFake()
	require.NoError(t, p.PutKV(context.Background(), "planner-mem", "userX", []byte(`{"prior_plans":[]}`)))
	got, err := p.GetKV(context.Background(), "planner-mem", "userX")
	require.NoError(t, err)
	require.Equal(t, []byte(`{"prior_plans":[]}`), got)
}

func TestMemoryProviderContract_KV_GetMissingReturnsErrNotFound(t *testing.T) {
	var p memory.Provider = newFake()
	_, err := p.GetKV(context.Background(), "planner-mem", "missing")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestMemoryProviderContract_Log_AppendThenReadInOrder(t *testing.T) {
	var p memory.Provider = newFake()
	ctx := context.Background()
	require.NoError(t, p.AppendLog(ctx, "audit/task42", []byte("a")))
	require.NoError(t, p.AppendLog(ctx, "audit/task42", []byte("b")))
	require.NoError(t, p.AppendLog(ctx, "audit/task42", []byte("c")))
	entries, err := p.ReadLog(ctx, "audit/task42")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("b"), []byte("c")}, entries)
}
