package singleflight_test

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/singleflight"
)

func TestSingleflightGroup(t *testing.T) {
	t.Parallel()

	t.Run("Sequential", func(t *testing.T) {
		t.Parallel()

		group := singleflight.NewGroup(nil)

		var refreshCalls atomic.Int64
		fn := func() (any, error) {
			return refreshCalls.Add(1), nil
		}

		calls := 5
		for i := range calls {
			result, err := group.Do("sequential", fn)
			require.NoError(t, err)
			require.Equal(t, int64(i+1), result)
		}

		// Should have been called each time.
		require.Equal(t, int64(calls), refreshCalls.Load())
	})

	t.Run("Parallel", func(t *testing.T) {
		t.Parallel()

		calls := 5

		ch := make(chan string)
		group := singleflight.NewGroup(ch)

		var refreshCalls atomic.Int64
		fn := func() (any, error) {
			// Wait for calls to have joined the group before returning, otherwise it
			// might return before all have joined and the test will flake.
			if refreshCalls.Add(1) == 1 {
				subscribed := 1
				for {
					<-ch
					subscribed++
					if subscribed >= calls {
						return 1, nil
					}
				}
			}
			return 0, xerrors.New("should not be called")
		}

		var eg errgroup.Group
		results := make([]int, calls)
		for i := range calls {
			eg.Go(func() error {
				result, err := group.Do("parallel", fn)
				results[i] = result.(int)
				return err
			})
		}

		// No call should error.
		err := eg.Wait()
		require.NoError(t, err)

		// First group of calls should have a one.
		for i := range calls {
			require.Equal(t, 1, results[i])
		}

		// Should only have called once.
		require.Equal(t, int64(1), refreshCalls.Load())
	})
}
