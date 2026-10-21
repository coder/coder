//go:build unix

package agentfiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
			name, size, err := publishUploadAt(dirFD, tmp, "archive.zip", int64(len("payload")), uploadPublishUniqueName, func(int, string, int, string, int) error {
				linkCalls++
				return linkErr
			})
			require.NoError(t, err)

			require.Equal(t, "archive_2.zip", name)
			require.Equal(t, int64(len("payload")), size)
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

	_, _, err = publishUploadAt(dirFD, ".archive.zip.upload-0123456789abcdef", "archive.zip", 0, uploadPublishUniqueName, func(int, string, int, string, int) error {
		return unix.EIO
	})
	require.ErrorIs(t, err, unix.EIO)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestPublishUploadAt_WriteOnceWithoutHardLinks(t *testing.T) {
	t.Parallel()

	const tmp = ".archive.zip.upload-0123456789abcdef"
	tests := []struct {
		name     string
		existing func(t *testing.T, target string)
		wantSize int64
		wantBody string
		wantErr  error
	}{
		{
			name:     "Absent",
			existing: func(*testing.T, string) {},
			wantSize: int64(len("payload")),
			wantBody: "payload",
		},
		{
			name: "RegularFile",
			existing: func(t *testing.T, target string) {
				require.NoError(t, os.WriteFile(target, []byte("existing"), 0o600))
			},
			wantSize: int64(len("existing")),
			wantBody: "existing",
		},
		{
			name: "Directory",
			existing: func(t *testing.T, target string) {
				require.NoError(t, os.Mkdir(target, 0o755))
			},
			wantErr: errUploadTargetNotRegular,
		},
		{
			name: "PublishingPlaceholder",
			existing: func(t *testing.T, target string) {
				require.NoError(t, os.WriteFile(target, nil, 0o600))
			},
			wantErr: errUploadPublishInProgress,
		},
		{
			name: "AbandonedPlaceholder",
			existing: func(t *testing.T, target string) {
				require.NoError(t, os.WriteFile(target, nil, 0o600))
				old := time.Now().Add(-2 * time.Hour)
				require.NoError(t, os.Chtimes(target, old, old))
			},
			wantSize: int64(len("payload")),
			wantBody: "payload",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
			require.NoError(t, err)
			t.Cleanup(func() { _ = unix.Close(dirFD) })
			require.NoError(t, os.WriteFile(filepath.Join(dir, tmp), []byte("payload"), 0o600))
			target := filepath.Join(dir, "archive.zip")
			tc.existing(t, target)

			name, size, err := publishUploadAt(dirFD, tmp, "archive.zip", int64(len("payload")), uploadPublishWriteOnce, func(int, string, int, string, int) error {
				return unix.EPERM
			})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)

			require.Equal(t, "archive.zip", name)
			require.Equal(t, tc.wantSize, size)
			contents, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, tc.wantBody, string(contents))
		})
	}
}
