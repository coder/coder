package agentbox

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

func TestHostCallFile(t *testing.T) {
	t.Parallel()

	open := func(t *testing.T, fsys *hostCallFS) *hostCallFile {
		t.Helper()
		f, errno := fsys.OpenFile(hostCallFileName, experimentalsys.O_RDWR, 0)
		require.Equal(t, experimentalsys.Errno(0), errno)
		file, ok := f.(*hostCallFile)
		require.True(t, ok)
		return file
	}

	t.Run("Root", func(t *testing.T) {
		t.Parallel()
		fsys := newHostCallFS(context.Background(), nil)
		root, errno := fsys.OpenFile(".", experimentalsys.O_RDONLY, 0)
		require.Equal(t, experimentalsys.Errno(0), errno)
		isDir, errno := root.IsDir()
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.True(t, isDir)
		_, errno = fsys.OpenFile("other", experimentalsys.O_RDONLY, 0)
		assert.Equal(t, experimentalsys.ENOENT, errno)
		st, errno := fsys.Stat(hostCallFileName)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.True(t, st.Mode.IsRegular())
	})

	t.Run("SeekAndPread", func(t *testing.T) {
		t.Parallel()
		fsys := newHostCallFS(context.Background(), func(_ context.Context, req []byte) ([]byte, error) {
			return append([]byte("resp:"), req...), nil
		})
		f := open(t, fsys)
		n, errno := f.Write([]byte("abc"))
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.Equal(t, 3, n)

		buf := make([]byte, 4)
		n, errno = f.Read(buf)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.Equal(t, "resp", string(buf[:n]))

		// A write after the response is fixed fails.
		_, errno = f.Write([]byte("x"))
		assert.Equal(t, experimentalsys.EIO, errno)

		off, errno := f.Seek(-2, io.SeekCurrent)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.EqualValues(t, 2, off)
		n, errno = f.Read(buf)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.Equal(t, "sp:a", string(buf[:n]))

		n, errno = f.Pread(buf, 5)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.Equal(t, "abc", string(buf[:n]))

		off, errno = f.Seek(0, io.SeekEnd)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.EqualValues(t, 8, off)
		n, errno = f.Read(buf)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.Equal(t, 0, n)

		_, errno = f.Seek(-1, io.SeekStart)
		assert.Equal(t, experimentalsys.EINVAL, errno)
	})

	t.Run("CanceledBeforeCall", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		called := false
		fsys := newHostCallFS(ctx, func(context.Context, []byte) ([]byte, error) {
			called = true
			return nil, nil
		})
		f := open(t, fsys)
		buf := make([]byte, 256)
		n, errno := f.Read(buf)
		require.Equal(t, experimentalsys.Errno(0), errno)
		assert.JSONEq(t, `{"ok":false,"code":"canceled","error":"run canceled"}`, string(buf[:n]))
		assert.False(t, called)
	})
}
