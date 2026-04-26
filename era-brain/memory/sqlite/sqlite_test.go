package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
)

func newProvider(t *testing.T) memory.Provider {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mem.db")
	p, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestSQLite_KV_PutAndGet(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "planner-mem", "u1", []byte("hello")))
	got, err := p.GetKV(ctx, "planner-mem", "u1")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), got)
}

func TestSQLite_KV_Overwrite(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "ns", "k", []byte("v1")))
	require.NoError(t, p.PutKV(ctx, "ns", "k", []byte("v2")))
	got, err := p.GetKV(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("v2"), got)
}

func TestSQLite_KV_GetMissingErrNotFound(t *testing.T) {
	p := newProvider(t)
	_, err := p.GetKV(context.Background(), "ns", "nope")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestSQLite_Log_AppendAndRead(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	for _, e := range [][]byte{[]byte("a"), []byte("b"), []byte("c")} {
		require.NoError(t, p.AppendLog(ctx, "audit/t1", e))
	}
	entries, err := p.ReadLog(ctx, "audit/t1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("b"), []byte("c")}, entries)
}

func TestSQLite_Log_NamespaceIsolation(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.AppendLog(ctx, "ns1", []byte("a")))
	require.NoError(t, p.AppendLog(ctx, "ns2", []byte("b")))
	got, err := p.ReadLog(ctx, "ns1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a")}, got)
}

func TestSQLite_KVAndLog_DontInterfere(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "ns", "k", []byte("kv-val")))
	require.NoError(t, p.AppendLog(ctx, "ns", []byte("log-val")))

	v, err := p.GetKV(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("kv-val"), v)

	entries, err := p.ReadLog(ctx, "ns")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("log-val")}, entries)
}
