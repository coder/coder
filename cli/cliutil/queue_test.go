package cliutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliutil"
)

func TestQueue(t *testing.T) {
	t.Parallel()

	t.Run("DropsFirst", func(t *testing.T) {
		t.Parallel()

		q := cliutil.NewQueue[int](10)
		require.Equal(t, 0, q.Len())

		for i := 0; i < 20; i++ {
			err := q.Push(i)
			require.NoError(t, err)
			if i < 10 {
				require.Equal(t, i+1, q.Len())
			} else {
				require.Equal(t, 10, q.Len())
			}
		}

		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, 10, val)
		require.Equal(t, 9, q.Len())
	})

	t.Run("Pop", func(t *testing.T) {
		t.Parallel()

		q := cliutil.NewQueue[int](10)
		for i := 0; i < 5; i++ {
			err := q.Push(i)
			require.NoError(t, err)
		}

		// No blocking, should pop immediately.
		for i := 0; i < 5; i++ {
			val, ok := q.Pop()
			require.True(t, ok)
			require.Equal(t, i, val)
		}

		// Pop should block until the next push.
		go func() {
			err := q.Push(55)
			assert.NoError(t, err)
		}()

		item, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, 55, item)
	})

	t.Run("Close", func(t *testing.T) {
		t.Parallel()

		q := cliutil.NewQueue[int](10)

		done := make(chan bool)
		go func() {
			_, ok := q.Pop()
			done <- ok
		}()

		q.Close()

		require.False(t, <-done)

		_, ok := q.Pop()
		require.False(t, ok)

		err := q.Push(10)
		require.Error(t, err)
	})

	t.Run("WithPredicate", func(t *testing.T) {
		t.Parallel()

		q := cliutil.NewQueue[int](10)
		q.WithPredicate(func(n int) (int, bool) {
			if n == 2 {
				return n, false
			}
			return n + 1, true
		})

		for i := 0; i < 5; i++ {
			err := q.Push(i)
			require.NoError(t, err)
		}

		got := []int{}
		for i := 0; i < 4; i++ {
			val, ok := q.Pop()
			require.True(t, ok)
			got = append(got, val)
		}
		require.Equal(t, []int{1, 2, 4, 5}, got)
	})
}
