package autobuild

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	t.Parallel()

	t.Run("CallsLoaderOnce", func(t *testing.T) {
		callCount := 0
		cache := newCacheOf[uuid.UUID, int]()
		key := uuid.New()

		// Call LoadOrStore for key `key` for the first time.
		// We expect this to call our loader function.
		value, err := cache.LoadOrStore(key, func() (int, error) {
			callCount += 1
			return 1, nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, value)
		require.Equal(t, 1, callCount)

		// Call LoadOrStore for key `key` for the second time.
		// We expect this to return data from the previous load.
		value, err = cache.LoadOrStore(key, func() (int, error) {
			callCount += 1

			// We return a different value to further check
			// that this function isn't called.
			return 2, nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, value)
		require.Equal(t, 1, callCount)
	})

	t.Run("ReturnsErrOnLoaderErr", func(t *testing.T) {
		exampleErr := errors.New("example error")
		cache := newCacheOf[uuid.UUID, int]()
		key := uuid.New()

		_, err := cache.LoadOrStore(key, func() (int, error) {
			return 0, exampleErr
		})
		require.Error(t, err)
		require.Equal(t, exampleErr, err)
	})

	t.Run("CanCacheWithMultipleKeys", func(t *testing.T) {
		cache := newCacheOf[uuid.UUID, int]()
		keyA := uuid.New()
		keyB := uuid.New()

		// We first insert data with our first key
		value, err := cache.LoadOrStore(keyA, func() (int, error) {
			return 10, nil
		})
		require.NoError(t, err)
		require.Equal(t, 10, value)

		// Next we insert data with a different key
		value, err = cache.LoadOrStore(keyB, func() (int, error) {
			return 20, nil
		})
		require.NoError(t, err)
		require.Equal(t, 20, value)

		// Now we check the data is still available for the first key
		value, err = cache.LoadOrStore(keyA, nil)
		require.NoError(t, err)
		require.Equal(t, 10, value)

		// And that the data is also still available for the second key
		value, err = cache.LoadOrStore(keyB, nil)
		require.NoError(t, err)
		require.Equal(t, 20, value)
	})
}
