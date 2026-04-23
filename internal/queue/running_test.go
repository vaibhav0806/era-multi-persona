package queue_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestRunningSet_RegisterGetDeregister(t *testing.T) {
	rs := queue.NewRunningSet()
	_, ok := rs.Get(1)
	require.False(t, ok)
	rs.Register(1, "era-runner-1-abc")
	name, ok := rs.Get(1)
	require.True(t, ok)
	require.Equal(t, "era-runner-1-abc", name)
	rs.Deregister(1)
	_, ok = rs.Get(1)
	require.False(t, ok)
}

func TestRunningSet_KilledFlag(t *testing.T) {
	rs := queue.NewRunningSet()
	require.False(t, rs.WasKilled(5))
	rs.MarkKilled(5)
	require.True(t, rs.WasKilled(5))
	// Deregister removes the container-name only; killed flag must survive
	// long enough for RunNext to read it after the adapter's defer fires.
	rs.Deregister(5)
	require.True(t, rs.WasKilled(5))
	// ClearKilled is the explicit way to drop the flag.
	rs.ClearKilled(5)
	require.False(t, rs.WasKilled(5))
}

func TestRunningSet_ConcurrentRegisterAndGet(t *testing.T) {
	rs := queue.NewRunningSet()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rs.Register(int64(i), "name")
			rs.Get(int64(i))
			rs.MarkKilled(int64(i))
			rs.WasKilled(int64(i))
			rs.Deregister(int64(i))
		}(i)
	}
	wg.Wait()
}
