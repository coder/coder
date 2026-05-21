package testutil

import (
	"archive/tar"
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/archive"
)

// Creates an in-memory tar of the files provided.
// Files in the archive are written with nobody
// owner/group, and -rw-rw-rw- permissions.
func CreateTar(t testing.TB, files map[string]string) []byte {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	// Keep track of directories previously added.
	addedDirs := make(map[string]bool)
	for path, content := range files {
		// Add parent directories if they don't exist
		dir := filepath.Dir(path)
		if dir != "." && !addedDirs[dir] {
			err := writer.WriteHeader(&tar.Header{
				Name:     dir + "/", // Directory names must end with /
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			})
			require.NoError(t, err)
			addedDirs[dir] = true
		}

		err := writer.WriteHeader(&tar.Header{
			Name: path,
			Size: int64(len(content)),
			Uid:  65534, // nobody
			Gid:  65534, // nogroup
			Mode: 0o666, // -rw-rw-rw-
		})
		require.NoError(t, err)

		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
	}

	err := writer.Flush()
	require.NoError(t, err)
	return buffer.Bytes()
}

// Creates an in-memory zip of the files provided.
// Uses archive.CreateZipFromTar under the hood.
func CreateZip(t testing.TB, files map[string]string) []byte {
	ta := CreateTar(t, files)
	tr := tar.NewReader(bytes.NewReader(ta))
	za, err := archive.CreateZipFromTar(tr, int64(len(ta)))
	require.NoError(t, err)
	return za
}
