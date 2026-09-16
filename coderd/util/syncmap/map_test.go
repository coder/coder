package syncmap_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/syncmap"
)

// The tests below pin Map to the sync.Map contract it wraps. Where the
// stdlib returns a value, Map must return that same value typed as V,
// and where the stdlib returns nil, Map must return the zero V.

func TestStoreLoad(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	v, ok := m.Load("missing")
	require.False(t, ok)
	require.Zero(t, v)

	m.Store("key", 1)
	v, ok = m.Load("key")
	require.True(t, ok)
	require.Equal(t, 1, v)

	m.Store("key", 2)
	v, ok = m.Load("key")
	require.True(t, ok)
	require.Equal(t, 2, v)
}

func TestLoadOrStore(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	actual, loaded := m.LoadOrStore("key", 1)
	require.False(t, loaded)
	require.Equal(t, 1, actual, "stored value must be returned, not the zero value")

	actual, loaded = m.LoadOrStore("key", 2)
	require.True(t, loaded)
	require.Equal(t, 1, actual, "existing value must win")

	v, ok := m.Load("key")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

// TestLoadOrStorePointer covers the load-or-create pattern, where a
// zero-value return is a nil pointer the caller then dereferences.
func TestLoadOrStorePointer(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, *atomic.Int32]()

	for range 3 {
		counter, _ := m.LoadOrStore("key", &atomic.Int32{})
		require.NotNil(t, counter)
		counter.Add(1)
	}

	counter, ok := m.Load("key")
	require.True(t, ok)
	require.Equal(t, int32(3), counter.Load(), "all callers must share one counter")
}

func TestLoadOrStoreConcurrent(t *testing.T) {
	t.Parallel()

	const goroutines = 16

	m := syncmap.New[string, *atomic.Int32]()

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(goroutines)
	winners := make([]*atomic.Int32, goroutines)
	loadedFlags := make([]bool, goroutines)
	for i := range goroutines {
		go func() {
			defer done.Done()
			start.Wait()
			winners[i], loadedFlags[i] = m.LoadOrStore("key", &atomic.Int32{})
		}()
	}
	start.Done()
	done.Wait()

	stored, ok := m.Load("key")
	require.True(t, ok)
	stores := 0
	for i, winner := range winners {
		require.Same(t, stored, winner, "goroutine %d observed a different value than the map holds", i)
		if !loadedFlags[i] {
			stores++
		}
	}
	require.Equal(t, 1, stores, "exactly one goroutine should store")
}

func TestLoadAndDelete(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	actual, loaded := m.LoadAndDelete("missing")
	require.False(t, loaded)
	require.Zero(t, actual)

	m.Store("key", 1)
	actual, loaded = m.LoadAndDelete("key")
	require.True(t, loaded)
	require.Equal(t, 1, actual)

	_, ok := m.Load("key")
	require.False(t, ok)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	m.Delete("missing") // No-op.

	m.Store("key", 1)
	m.Delete("key")
	_, ok := m.Load("key")
	require.False(t, ok)
}

func TestSwap(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	previous, loaded := m.Swap("key", 1)
	require.False(t, loaded)
	require.Zero(t, previous)

	previous, loaded = m.Swap("key", 2)
	require.True(t, loaded)
	require.Equal(t, 1, previous)

	v, ok := m.Load("key")
	require.True(t, ok)
	require.Equal(t, 2, v)
}

// TestSwapTyped pins previous to V rather than any: dereferencing it
// only compiles if the wrapper returns the value type.
func TestSwapTyped(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, *int]()
	first, second := 1, 2

	previous, loaded := m.Swap("key", &first)
	require.False(t, loaded)
	require.Nil(t, previous)

	previous, loaded = m.Swap("key", &second)
	require.True(t, loaded)
	require.Equal(t, 1, *previous)
}

func TestCompareAndSwap(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	require.False(t, m.CompareAndSwap("missing", 1, 2))

	m.Store("key", 1)
	require.False(t, m.CompareAndSwap("key", 2, 3), "swap must not happen on mismatch")
	require.True(t, m.CompareAndSwap("key", 1, 3))

	v, ok := m.Load("key")
	require.True(t, ok)
	require.Equal(t, 3, v)
}

func TestCompareAndDelete(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()

	require.False(t, m.CompareAndDelete("missing", 1))

	m.Store("key", 1)
	require.False(t, m.CompareAndDelete("key", 2), "delete must not happen on mismatch")
	require.True(t, m.CompareAndDelete("key", 1))

	_, ok := m.Load("key")
	require.False(t, ok)
}

func TestRange(t *testing.T) {
	t.Parallel()

	m := syncmap.New[string, int]()
	want := map[string]int{"a": 1, "b": 2, "c": 3}
	for k, v := range want {
		m.Store(k, v)
	}

	got := make(map[string]int)
	m.Range(func(key string, value int) bool {
		got[key] = value
		return true
	})
	require.Equal(t, want, got)

	visited := 0
	m.Range(func(string, int) bool {
		visited++
		return false
	})
	require.Equal(t, 1, visited, "returning false must stop iteration")
}

// TestNilInterfaceValue covers an interface value type holding nil.
// sync.Map stores it as a nil `any`, which cannot be type-asserted, so
// every read path has to return the zero V instead of panicking.
func TestNilInterfaceValue(t *testing.T) {
	t.Parallel()

	var nilErr error

	t.Run("LoadOrStore", func(t *testing.T) {
		t.Parallel()

		m := syncmap.New[string, error]()
		actual, loaded := m.LoadOrStore("key", nilErr)
		require.False(t, loaded)
		require.NoError(t, actual)
	})

	t.Run("Load", func(t *testing.T) {
		t.Parallel()

		m := syncmap.New[string, error]()
		m.Store("key", nilErr)
		v, ok := m.Load("key")
		require.True(t, ok, "a stored nil is still a present key")
		require.NoError(t, v)
	})

	t.Run("LoadAndDelete", func(t *testing.T) {
		t.Parallel()

		m := syncmap.New[string, error]()
		m.Store("key", nilErr)
		v, loaded := m.LoadAndDelete("key")
		require.True(t, loaded)
		require.NoError(t, v)
	})

	t.Run("Swap", func(t *testing.T) {
		t.Parallel()

		m := syncmap.New[string, error]()
		m.Store("key", nilErr)
		previous, loaded := m.Swap("key", nilErr)
		require.True(t, loaded)
		require.NoError(t, previous)
	})

	t.Run("Range", func(t *testing.T) {
		t.Parallel()

		m := syncmap.New[string, error]()
		m.Store("key", nilErr)
		visited := 0
		m.Range(func(key string, value error) bool {
			visited++
			require.Equal(t, "key", key)
			require.NoError(t, value)
			return true
		})
		require.Equal(t, 1, visited)
	})
}
