//go:build unix

package agentfiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPublishUploadAt_FallsBackWithoutHardLinks(t *testing.T) {
	t.Parallel()

	for _, linkErr := range []error{unix.EPERM, unix.EOPNOTSUPP, unix.EXDEV} {
		t.Run(linkErr.Error(), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
			require.NoError(t, err)
			t.Cleanup(func() { _ = unix.Close(dirFD) })
			const tmp = ".archive.zip.upload-0123456789abcdef"
			require.NoError(t, os.WriteFile(filepath.Join(dir, tmp), []byte("payload"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "archive.zip"), []byte("existing"), 0o600))

			linkCalls := 0
			name, err := publishUploadAt(dirFD, tmp, "archive.zip", func(int, string, int, string, int) error {
				linkCalls++
				return linkErr
			})
			require.NoError(t, err)

			require.Equal(t, "archive_2.zip", name)
			require.Equal(t, 1, linkCalls)
			contents, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			require.Equal(t, "payload", string(contents))
			contents, err = os.ReadFile(filepath.Join(dir, "archive.zip"))
			require.NoError(t, err)
			require.Equal(t, "existing", string(contents))
			_, err = os.Lstat(filepath.Join(dir, tmp))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestPublishUploadAt_ReturnsOtherLinkErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(dirFD) })

	_, err = publishUploadAt(dirFD, ".archive.zip.upload-0123456789abcdef", "archive.zip", func(int, string, int, string, int) error {
		return unix.EIO
	})
	require.ErrorIs(t, err, unix.EIO)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
