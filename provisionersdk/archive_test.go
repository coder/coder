package provisionersdk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisionersdk"
)

func TestTar(t *testing.T) {
	t.Parallel()
	t.Run("NoTF", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "")
		require.NoError(t, err)
		_ = file.Close()
		_, err = provisionersdk.Tar(dir, 1024)
		require.Error(t, err)
	})
	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_ = file.Close()
		_, err = provisionersdk.Tar(dir, 1024)
		require.NoError(t, err)
	})
}

func TestUntar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "*.tf")
	require.NoError(t, err)
	_ = file.Close()
	archive, err := provisionersdk.Tar(dir, 1024)
	require.NoError(t, err)
	dir = t.TempDir()
	err = provisionersdk.Untar(dir, archive)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, filepath.Base(file.Name())))
	require.NoError(t, err)
}
