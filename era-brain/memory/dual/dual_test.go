package dual_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/dual"
)

type fakeProvider struct {
	mu      sync.Mutex
	kv      map[string][]byte
	logs    map[string][][]byte
	failPut bool
	failGet bool
}

func newFake() *fakeProvider { return &fakeProvider{kv: map[string][]byte{}, logs: map[string][][]byte{}} }

var errBoom = errors.New("provider failure")

func (f *fakeProvider) GetKV(_ context.Context, ns, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		return nil, errBoom
	}
	v, ok := f.kv[ns+"/"+key]
	if !ok {
		return nil, memory.ErrNotFound
	}
	return v, nil
}

func (f *fakeProvider) PutKV(_ context.Context, ns, key string, val []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPut {
		return errBoom
	}
	f.kv[ns+"/"+key] = val
	return nil
}

func (f *fakeProvider) AppendLog(_ context.Context, ns string, e []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPut {
		return errBoom
	}
	f.logs[ns] = append(f.logs[ns], e)
	return nil
}

func (f *fakeProvider) ReadLog(_ context.Context, ns string) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		return nil, errBoom
	}
	return f.logs[ns], nil
}

func TestDual_PutKV_WritesToBoth(t *testing.T) {
	cache := newFake()
	primary := newFake()
	d := dual.New(cache, primary, nil)
	require.NoError(t, d.PutKV(context.Background(), "ns", "k", []byte("v")))
	require.Equal(t, []byte("v"), cache.kv["ns/k"])
	require.Equal(t, []byte("v"), primary.kv["ns/k"])
}

func TestDual_PutKV_PrimaryFailureDoesNotBlock(t *testing.T) {
	cache := newFake()
	primary := newFake()
	primary.failPut = true
	var primaryErrs []error
	d := dual.New(cache, primary, func(op string, err error) {
		primaryErrs = append(primaryErrs, err)
	})

	err := d.PutKV(context.Background(), "ns", "k", []byte("v"))
	require.NoError(t, err)
	require.Equal(t, []byte("v"), cache.kv["ns/k"])
	require.Empty(t, primary.kv)
	require.Len(t, primaryErrs, 1)
}

func TestDual_PutKV_CacheFailureIsFatal(t *testing.T) {
	cache := newFake()
	cache.failPut = true
	primary := newFake()
	d := dual.New(cache, primary, nil)

	err := d.PutKV(context.Background(), "ns", "k", []byte("v"))
	require.Error(t, err)
}

func TestDual_GetKV_PrefersCache(t *testing.T) {
	cache := newFake()
	primary := newFake()
	require.NoError(t, cache.PutKV(context.Background(), "ns", "k", []byte("from-cache")))
	require.NoError(t, primary.PutKV(context.Background(), "ns", "k", []byte("from-primary")))

	d := dual.New(cache, primary, nil)
	got, err := d.GetKV(context.Background(), "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("from-cache"), got)
}

func TestDual_GetKV_FallsThroughToPrimary(t *testing.T) {
	cache := newFake()
	primary := newFake()
	require.NoError(t, primary.PutKV(context.Background(), "ns", "k", []byte("from-primary")))

	d := dual.New(cache, primary, nil)
	got, err := d.GetKV(context.Background(), "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("from-primary"), got)
}

func TestDual_GetKV_BothMissingReturnsErrNotFound(t *testing.T) {
	d := dual.New(newFake(), newFake(), nil)
	_, err := d.GetKV(context.Background(), "ns", "missing")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestDual_AppendLog_WritesToBoth(t *testing.T) {
	cache := newFake()
	primary := newFake()
	d := dual.New(cache, primary, nil)
	require.NoError(t, d.AppendLog(context.Background(), "ns", []byte("a")))
	require.Equal(t, [][]byte{[]byte("a")}, cache.logs["ns"])
	require.Equal(t, [][]byte{[]byte("a")}, primary.logs["ns"])
}

func TestDual_ReadLog_PrefersCache(t *testing.T) {
	cache := newFake()
	primary := newFake()
	cache.logs["ns"] = [][]byte{[]byte("c")}
	primary.logs["ns"] = [][]byte{[]byte("p")}

	d := dual.New(cache, primary, nil)
	got, err := d.ReadLog(context.Background(), "ns")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("c")}, got)
}

func TestDual_ReadLog_FallsThroughOnEmptyCache(t *testing.T) {
	cache := newFake()
	primary := newFake()
	primary.logs["ns"] = [][]byte{[]byte("p")}

	d := dual.New(cache, primary, nil)
	got, err := d.ReadLog(context.Background(), "ns")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("p")}, got)
}
