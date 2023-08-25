package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
)

func TestFile(t *testing.T) {
	t.Parallel()

	t.Run("Write", func(t *testing.T) {
		t.Parallel()
		err := config.Root(t.TempDir()).Session().Write("test")
		require.NoError(t, err)
	})

	t.Run("Read", func(t *testing.T) {
		t.Parallel()
		root := config.Root(t.TempDir())
		err := root.Session().Write("test")
		require.NoError(t, err)
		data, err := root.Session().Read()
		require.NoError(t, err)
		require.Equal(t, "test", data)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		root := config.Root(t.TempDir())
		err := root.Session().Write("test")
		require.NoError(t, err)
		err = root.Session().Delete()
		require.NoError(t, err)
	})
}
