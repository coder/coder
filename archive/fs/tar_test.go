package archivefs_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/archive"
	archivefs "github.com/coder/coder/v2/archive/fs"
)

func TestFromTarReader(t *testing.T) {
	t.Parallel()

	var tarBytes bytes.Buffer
	tw := tar.NewWriter(&tarBytes)
	contents := []byte("terraform {}")
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "main.tf", Mode: 0o644, Size: int64(len(contents))}))
	_, err := tw.Write(contents)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	validTar := tarBytes.Bytes()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		files, err := archivefs.FromTarReader(bytes.NewReader(validTar))
		require.NoError(t, err)
		got, err := fs.ReadFile(files, "main.tf")
		require.NoError(t, err)
		require.Equal(t, contents, got)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		files, err := archivefs.FromTarReader(bytes.NewReader(nil))
		require.NoError(t, err)
		entries, err := fs.ReadDir(files, ".")
		require.NoError(t, err)
		require.Empty(t, entries)
	})

	t.Run("Gzipped", func(t *testing.T) {
		t.Parallel()

		var gzipped bytes.Buffer
		gw := gzip.NewWriter(&gzipped)
		_, err := gw.Write(validTar)
		require.NoError(t, err)
		require.NoError(t, gw.Close())

		_, err = archivefs.FromTarReader(&gzipped)
		require.ErrorIs(t, err, archive.ErrGzipCompressed)
	})

	t.Run("InvalidHeader", func(t *testing.T) {
		t.Parallel()

		_, err := archivefs.FromTarReader(bytes.NewReader([]byte("this is not a tar archive")))
		require.ErrorContains(t, err, "invalid tar archive")
	})

	t.Run("TruncatedContents", func(t *testing.T) {
		t.Parallel()

		// The header block is intact but the entry contents are cut short.
		_, err := archivefs.FromTarReader(bytes.NewReader(validTar[:520]))
		require.ErrorContains(t, err, "invalid tar archive")
	})
}
