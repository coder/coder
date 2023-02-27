package provisionersdk_test

import (
	"bytes"
	"io"
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
		err = provisionersdk.Tar(io.Discard, dir, 1024)
		require.Error(t, err)
	})
	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_ = file.Close()
		err = provisionersdk.Tar(io.Discard, dir, 1024)
		require.NoError(t, err)
	})
	t.Run("HiddenFiles", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		type file struct {
			Name     string
			Archives bool
		}
		files := []*file{{
			Name:     "*.tf",
			Archives: true,
		}, {
			Name:     ".*",
			Archives: false,
		}, {
			Name:     "./testing/.test/*.tf",
			Archives: false,
		}, {
			Name:     "./testing/asd.*",
			Archives: true,
		}, {
			Name:     ".terraform/.*",
			Archives: false,
		}, {
			Name:     "example/.terraform/*",
			Archives: false,
		}}
		for _, file := range files {
			newDir := dir
			file.Name = filepath.FromSlash(file.Name)
			if filepath.Base(file.Name) != file.Name {
				newDir = filepath.Join(newDir, filepath.Dir(file.Name))
				err := os.MkdirAll(newDir, 0o755)
				require.NoError(t, err)
				file.Name = filepath.Base(file.Name)
			}
			tmpFile, err := os.CreateTemp(newDir, file.Name)
			require.NoError(t, err)
			_ = tmpFile.Close()
			file.Name, err = filepath.Rel(dir, tmpFile.Name())
			require.NoError(t, err)
		}
		archive := new(bytes.Buffer)
		err := provisionersdk.Tar(archive, dir, 1024)
		require.NoError(t, err)
		dir = t.TempDir()
		err = provisionersdk.Untar(dir, archive)
		require.NoError(t, err)
		for _, file := range files {
			_, err = os.Stat(filepath.Join(dir, file.Name))
			t.Logf("stat %q %+v", file.Name, err)
			if file.Archives {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		}
	})
}

func TestUntar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file, err := os.CreateTemp(dir, "*.tf")
	require.NoError(t, err)
	_ = file.Close()
	archive := new(bytes.Buffer)
	err = provisionersdk.Tar(archive, dir, 1024)
	require.NoError(t, err)
	dir = t.TempDir()
	err = provisionersdk.Untar(dir, archive)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, filepath.Base(file.Name())))
	require.NoError(t, err)
}
