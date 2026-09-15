package coderd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTar(t *testing.T) {
	t.Parallel()

	buildTar := func(t *testing.T) []byte {
		t.Helper()

		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "main.tf", Mode: 0o600, Size: 4}))
		_, err := tw.Write([]byte("main"))
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		return buf.Bytes()
	}

	gzipBytes := func(t *testing.T, data []byte) []byte {
		t.Helper()

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, err := gw.Write(data)
		require.NoError(t, err)
		require.NoError(t, gw.Close())

		return buf.Bytes()
	}

	t.Run("Tar", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateTar(buildTar(t)))
	})

	t.Run("EmptyArchive", func(t *testing.T) {
		t.Parallel()
		// Two zeroed blocks are how a tar marks its end, and uploads of that
		// shape are already accepted today.
		require.NoError(t, validateTar(make([]byte, 1024)))
	})

	t.Run("Gzipped", func(t *testing.T) {
		t.Parallel()
		err := validateTar(gzipBytes(t, buildTar(t)))
		require.Error(t, err)
		require.Contains(t, err.Error(), "gzip")
	})

	t.Run("NotAnArchive", func(t *testing.T) {
		t.Parallel()
		require.Error(t, validateTar([]byte("this is not a tar archive")))
	})
}
