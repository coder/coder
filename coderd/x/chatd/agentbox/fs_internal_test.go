package agentbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sysfs"
)

func TestBoxFS(t *testing.T) {
	t.Parallel()

	newFS := func(t *testing.T, limit int64) (boxFS, *quota) {
		q := newQuota(limit)
		return newBoxFS(sysfs.DirFS(t.TempDir()), q), q
	}

	t.Run("DeniesLinks", func(t *testing.T) {
		t.Parallel()
		f, _ := newFS(t, 1<<20)
		file, errno := f.OpenFile("a", experimentalsys.O_CREAT|experimentalsys.O_WRONLY, 0o600)
		require.Zero(t, errno)
		require.Zero(t, file.Close())
		assert.Equal(t, experimentalsys.EPERM, f.Symlink("a", "b"))
		assert.Equal(t, experimentalsys.EPERM, f.Link("a", "b"))
		_, errno = f.Lstat("b")
		assert.Equal(t, experimentalsys.ENOENT, errno)
	})

	t.Run("ChargesEntriesAndBytes", func(t *testing.T) {
		t.Parallel()
		f, q := newFS(t, 3*entryCost)
		require.Zero(t, f.Mkdir("d", 0o700))
		assert.Equal(t, int64(2*entryCost), q.remaining.Load())

		file, errno := f.OpenFile("d/a", experimentalsys.O_CREAT|experimentalsys.O_RDWR, 0o600)
		require.Zero(t, errno)
		assert.Equal(t, int64(entryCost), q.remaining.Load())

		n, errno := file.Write(make([]byte, entryCost/2))
		require.Zero(t, errno)
		assert.Equal(t, entryCost/2, n)
		_, errno = file.Write(make([]byte, entryCost))
		assert.Equal(t, errQuotaExceeded, errno)

		// Growing past the budget fails; shrinking refunds.
		assert.Equal(t, errQuotaExceeded, file.Truncate(2*entryCost))
		require.Zero(t, file.Truncate(0))
		assert.Equal(t, int64(entryCost), q.remaining.Load())
		require.Zero(t, file.Truncate(entryCost/4))
		assert.Equal(t, int64(entryCost-entryCost/4), q.remaining.Load())
		require.Zero(t, file.Close())

		// Reopening an existing file charges no entry; O_TRUNC refunds
		// its bytes.
		file, errno = f.OpenFile("d/a", experimentalsys.O_TRUNC|experimentalsys.O_WRONLY, 0o600)
		require.Zero(t, errno)
		require.Zero(t, file.Close())
		assert.Equal(t, int64(entryCost), q.remaining.Load())

		require.Zero(t, f.Unlink("d/a"))
		assert.Equal(t, int64(2*entryCost), q.remaining.Load())
		require.Zero(t, f.Rmdir("d"))
		assert.Equal(t, int64(3*entryCost), q.remaining.Load())

		// A creation the budget cannot cover fails before touching disk.
		f2, _ := newFS(t, entryCost-1)
		assert.Equal(t, errQuotaExceeded, f2.Mkdir("x", 0o700))
		_, errno = f2.OpenFile("y", experimentalsys.O_CREAT|experimentalsys.O_WRONLY, 0o600)
		assert.Equal(t, errQuotaExceeded, errno)
		_, errno = f2.Lstat("y")
		assert.Equal(t, experimentalsys.ENOENT, errno)
	})
}

func TestBoundedWriter(t *testing.T) {
	t.Parallel()
	w := newBoundedWriter(5)
	n, err := w.Write([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = w.Write([]byte("defg"))
	require.NoError(t, err)
	assert.Equal(t, 4, n, "writes past the cap still report full length")
	assert.Equal(t, "abcde", w.String())
	assert.True(t, w.truncated())
}

func TestBoxPath(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"a.txt":         "a.txt",
		"/box/a.txt":    "a.txt",
		"/box/d//e.txt": "d/e.txt",
		"d/./e":         "d/e",
	} {
		got, err := boxPath(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	got, err := boxPath(" /box/x ")
	require.NoError(t, err)
	assert.Equal(t, "x", got)
	for _, in := range []string{"", "/", "/box", "/box/", "/boxy", "/etc/passwd", "..", "../a", "/box/../a", "a/../../b", "a\x00b"} {
		_, err := boxPath(in)
		require.Error(t, err, in)
	}
}
