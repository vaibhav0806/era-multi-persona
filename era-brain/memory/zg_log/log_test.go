package zg_log_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
)

type fakeKVOps struct {
	mu    sync.Mutex
	store map[string]map[string][]byte // streamID → key → val
}

func newFakeKVOps() *fakeKVOps { return &fakeKVOps{store: map[string]map[string][]byte{}} }

func (f *fakeKVOps) Set(_ context.Context, sid string, key, val []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store[sid] == nil {
		f.store[sid] = map[string][]byte{}
	}
	f.store[sid][string(key)] = val
	return nil
}

func (f *fakeKVOps) Get(_ context.Context, sid string, key []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store[sid] == nil {
		return nil, zg_log.ErrKeyNotFound
	}
	v, ok := f.store[sid][string(key)]
	if !ok {
		return nil, zg_log.ErrKeyNotFound
	}
	return v, nil
}

func (f *fakeKVOps) Iterate(_ context.Context, sid string) ([][2][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.store[sid]))
	for k := range f.store[sid] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2][]byte, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2][]byte{[]byte(k), f.store[sid][k]})
	}
	return out, nil
}

func TestZGLog_AppendAndRead(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	ctx := context.Background()

	for _, e := range [][]byte{[]byte("a"), []byte("b"), []byte("c")} {
		require.NoError(t, p.AppendLog(ctx, "audit/t1", e))
	}
	got, err := p.ReadLog(ctx, "audit/t1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("b"), []byte("c")}, got)
}

func TestZGLog_NamespaceIsolation(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	ctx := context.Background()
	require.NoError(t, p.AppendLog(ctx, "ns1", []byte("x")))
	require.NoError(t, p.AppendLog(ctx, "ns2", []byte("y")))

	got, err := p.ReadLog(ctx, "ns1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("x")}, got)
}

func TestZGLog_ReadEmptyNamespaceReturnsEmpty(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	got, err := p.ReadLog(context.Background(), "never-written")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestZGLog_PutKVReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	err := p.PutKV(context.Background(), "ns", "k", []byte("v"))
	require.True(t, errors.Is(err, zg_log.ErrKVUnsupported))
}

func TestZGLog_GetKVReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	_, err := p.GetKV(context.Background(), "ns", "k")
	require.True(t, errors.Is(err, zg_log.ErrKVUnsupported))
}

func TestZGLog_AppendIsAtomic_AllEntriesLand(t *testing.T) {
	// 5 goroutines append "G<i>-<n>" 4 times each. 20 total entries, each unique.
	// Read-back: contains all 20. (Order across goroutines is non-deterministic
	// but the test only asserts count.)
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	ctx := context.Background()

	var wg sync.WaitGroup
	for g := 0; g < 5; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			for n := 0; n < 4; n++ {
				_ = p.AppendLog(ctx, "ns", []byte(fmt.Sprintf("G%d-%d", g, n)))
			}
		}()
	}
	wg.Wait()

	got, err := p.ReadLog(ctx, "ns")
	require.NoError(t, err)
	require.Len(t, got, 20)
}
