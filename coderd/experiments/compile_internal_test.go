package experiments

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgramCache(t *testing.T) {
	t.Parallel()

	t.Run("ReusesIdenticalSource", func(t *testing.T) {
		t.Parallel()
		c := newProgramCache()
		first, err := c.get(`"owner" in user.roles`)
		require.NoError(t, err)
		second, err := c.get(`"owner" in user.roles`)
		require.NoError(t, err)
		require.Equal(t, 1, c.compiles)
		require.Same(t, first, second)
	})

	t.Run("RecompilesChangedSource", func(t *testing.T) {
		t.Parallel()
		c := newProgramCache()
		_, err := c.get(`"owner" in user.roles`)
		require.NoError(t, err)
		// Keys are exact source text, so whitespace changes recompile.
		_, err = c.get(`"owner"  in user.roles`)
		require.NoError(t, err)
		require.Equal(t, 2, c.compiles)
	})

	t.Run("CachesFailures", func(t *testing.T) {
		t.Parallel()
		c := newProgramCache()
		_, err := c.get(`user.nickname == "x"`)
		require.Error(t, err)
		_, err = c.get(`user.nickname == "x"`)
		require.Error(t, err)
		require.Equal(t, 1, c.compiles)
	})

	t.Run("Bounded", func(t *testing.T) {
		t.Parallel()
		c := newProgramCache()
		for i := range maxCachedPrograms + 10 {
			_, err := c.get(fmt.Sprintf(`user.username == "u%d"`, i))
			require.NoError(t, err)
			require.LessOrEqual(t, len(c.programs), maxCachedPrograms)
		}
		require.Len(t, c.programs, maxCachedPrograms)
	})

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		c := newProgramCache()
		var wg sync.WaitGroup
		for g := range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range 20 {
					_, err := c.get(fmt.Sprintf(`user.username == "u%d"`, (g+i)%5))
					if err != nil {
						t.Error(err)
						return
					}
				}
			}()
		}
		wg.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()
		require.Len(t, c.programs, 5)
	})
}
