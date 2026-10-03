package experiments

import (
	"fmt"
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
		require.Same(t, first, second)
	})

	t.Run("CachesFailures", func(t *testing.T) {
		t.Parallel()
		c := newProgramCache()
		_, first := c.get(`user.nickname == "x"`)
		require.Error(t, first)
		_, second := c.get(`user.nickname == "x"`)
		require.Same(t, first, second)
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
}
