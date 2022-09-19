package pointer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/pointer"
)

func TestHandle(t *testing.T) {
	t.Parallel()
	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		ptr := pointer.New("hello")
		ctx := context.Background()
		ctx, value := ptr.Load(ctx)
		require.Equal(t, "hello", value)
		ptr.Store("world")
		ctx, value = ptr.Load(ctx)
		require.Equal(t, "hello", value)
		_, value = ptr.Load(ctx)
		require.Equal(t, "hello", value)
	})
	t.Run("Multiple", func(t *testing.T) {
		t.Parallel()
		ptr1 := pointer.New("1")
		ptr2 := pointer.New("2")
		ctx := context.Background()
		ctx, v1 := ptr1.Load(ctx)
		require.Equal(t, "1", v1)
		_, v2 := ptr2.Load(ctx)
		require.Equal(t, "2", v2)
	})
}
