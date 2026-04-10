package objstore_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/objstore"
)

func TestLocalFS(t *testing.T) {
	t.Parallel()

	newStore := func(t *testing.T) objstore.Store {
		t.Helper()
		store, err := objstore.NewLocal(objstore.LocalConfig{Dir: t.TempDir()})
		require.NoError(t, err)
		t.Cleanup(func() { store.Close() })
		return store
	}

	t.Run("WriteAndRead", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		data := []byte("hello, object store")

		err := store.Write(ctx, "ns", "key1", data)
		require.NoError(t, err)

		rc, info, err := store.Read(ctx, "ns", "key1")
		require.NoError(t, err)
		defer rc.Close()

		require.Equal(t, "key1", info.Key)
		require.Equal(t, int64(len(data)), info.Size)
		require.False(t, info.LastModified.IsZero())

		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Equal(t, data, got)
	})

	t.Run("ReadNotFound", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		_, _, err := store.Read(ctx, "ns", "nonexistent")
		require.Error(t, err)
		require.True(t, errors.Is(err, objstore.ErrNotFound), "expected ErrNotFound, got: %v", err)
	})

	t.Run("Overwrite", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		err := store.Write(ctx, "ns", "key1", []byte("v1"))
		require.NoError(t, err)

		err = store.Write(ctx, "ns", "key1", []byte("v2"))
		require.NoError(t, err)

		rc, _, err := store.Read(ctx, "ns", "key1")
		require.NoError(t, err)
		defer rc.Close()

		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Equal(t, []byte("v2"), got)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		err := store.Write(ctx, "ns", "key1", []byte("data"))
		require.NoError(t, err)

		err = store.Delete(ctx, "ns", "key1")
		require.NoError(t, err)

		_, _, err = store.Read(ctx, "ns", "key1")
		require.True(t, errors.Is(err, objstore.ErrNotFound))
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		err := store.Delete(ctx, "ns", "nonexistent")
		require.True(t, errors.Is(err, objstore.ErrNotFound), "expected ErrNotFound, got: %v", err)
	})

	t.Run("ListAll", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		err := store.Write(ctx, "ns", "a", []byte("1"))
		require.NoError(t, err)
		err = store.Write(ctx, "ns", "b", []byte("2"))
		require.NoError(t, err)
		err = store.Write(ctx, "ns", "c", []byte("3"))
		require.NoError(t, err)

		var keys []string
		for info, err := range store.List(ctx, "ns", "") {
			require.NoError(t, err)
			keys = append(keys, info.Key)
		}

		slices.Sort(keys)
		require.Equal(t, []string{"a", "b", "c"}, keys)
	})

	t.Run("ListWithPrefix", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		err := store.Write(ctx, "ns", "logs/a", []byte("1"))
		require.NoError(t, err)
		err = store.Write(ctx, "ns", "logs/b", []byte("2"))
		require.NoError(t, err)
		err = store.Write(ctx, "ns", "other/c", []byte("3"))
		require.NoError(t, err)

		var keys []string
		for info, err := range store.List(ctx, "ns", "logs/") {
			require.NoError(t, err)
			keys = append(keys, info.Key)
		}

		slices.Sort(keys)
		require.Equal(t, []string{"logs/a", "logs/b"}, keys)
	})

	t.Run("ListEmptyNamespace", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		var count int
		for _, err := range store.List(ctx, "empty", "") {
			require.NoError(t, err)
			count++
		}
		require.Zero(t, count)
	})

	t.Run("NamespaceIsolation", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)
		ctx := context.Background()

		err := store.Write(ctx, "ns1", "key", []byte("ns1-data"))
		require.NoError(t, err)
		err = store.Write(ctx, "ns2", "key", []byte("ns2-data"))
		require.NoError(t, err)

		rc1, _, err := store.Read(ctx, "ns1", "key")
		require.NoError(t, err)
		got1, _ := io.ReadAll(rc1)
		rc1.Close()

		rc2, _, err := store.Read(ctx, "ns2", "key")
		require.NoError(t, err)
		got2, _ := io.ReadAll(rc2)
		rc2.Close()

		require.Equal(t, []byte("ns1-data"), got1)
		require.Equal(t, []byte("ns2-data"), got2)
	})

	t.Run("CloseThenOps", func(t *testing.T) {
		t.Parallel()
		store, err := objstore.NewLocal(objstore.LocalConfig{Dir: t.TempDir()})
		require.NoError(t, err)

		err = store.Close()
		require.NoError(t, err)

		err = store.Write(context.Background(), "ns", "key", []byte("data"))
		require.True(t, errors.Is(err, objstore.ErrClosed), "expected ErrClosed, got: %v", err)

		_, _, err = store.Read(context.Background(), "ns", "key")
		require.True(t, errors.Is(err, objstore.ErrClosed), "expected ErrClosed, got: %v", err)
	})
}
