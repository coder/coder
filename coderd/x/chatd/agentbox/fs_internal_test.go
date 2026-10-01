package agentbox

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sysfs"
)

func (l *openLimit) held() int64 { return l.open.Load() }

func TestDefaultMaxConcurrent(t *testing.T) {
	t.Parallel()
	for procs, want := range map[int]int{1: 2, 4: 2, 6: 3, 8: 4, 64: 4} {
		assert.Equal(t, want, defaultMaxConcurrent(procs), procs)
	}
}

func TestBoxFS(t *testing.T) {
	t.Parallel()

	newFS := func(t *testing.T, limit int64) (*boxFS, *quota) {
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

	t.Run("ChargesApparentSize", func(t *testing.T) {
		t.Parallel()
		f, q := newFS(t, entryCost+100)
		file, errno := f.OpenFile("s", experimentalsys.O_CREAT|experimentalsys.O_RDWR, 0o600)
		require.Zero(t, errno)

		// Overwriting bytes inside the file charges nothing.
		_, errno = file.Write(make([]byte, 100))
		require.Zero(t, errno)
		_, errno = file.Pwrite(make([]byte, 50), 10)
		require.Zero(t, errno)
		assert.Zero(t, q.remaining.Load())

		// A write past a seek hole is charged for the hole.
		_, errno = file.Seek(1<<40, io.SeekStart)
		require.Zero(t, errno)
		_, errno = file.Write([]byte{1})
		assert.Equal(t, errQuotaExceeded, errno)
		_, errno = file.Pwrite([]byte{1}, 1<<40)
		assert.Equal(t, errQuotaExceeded, errno)
		require.Zero(t, file.Close())

		require.Zero(t, f.Unlink("s"))
		assert.Equal(t, int64(entryCost+100), q.remaining.Load(), "removal refunds exactly what was charged")
	})

	t.Run("OpenUnlinkedFileKeepsCharge", func(t *testing.T) {
		t.Parallel()
		f, q := newFS(t, 1<<20)
		file, errno := f.OpenFile("o", experimentalsys.O_CREAT|experimentalsys.O_RDWR, 0o600)
		require.Zero(t, errno)
		second, errno := f.OpenFile("o", experimentalsys.O_RDONLY, 0)
		require.Zero(t, errno)
		_, errno = file.Write(make([]byte, 1000))
		require.Zero(t, errno)

		require.Zero(t, f.Unlink("o"))
		assert.Equal(t, int64(1<<20-1000), q.remaining.Load(), "only the entry is refunded while open")
		_, errno = file.Write(make([]byte, 24))
		require.Zero(t, errno)
		require.Zero(t, file.Close())
		assert.Equal(t, int64(1<<20-1024), q.remaining.Load())
		require.Zero(t, second.Close())
		require.Zero(t, second.Close(), "close is idempotent")
		assert.Equal(t, int64(1<<20), q.remaining.Load(), "last close refunds the bytes")
	})

	t.Run("MkdirExistingAtQuota", func(t *testing.T) {
		t.Parallel()
		f, q := newFS(t, entryCost)
		require.Zero(t, f.Mkdir("d", 0o700))
		require.Zero(t, q.remaining.Load())
		assert.Equal(t, experimentalsys.EEXIST, f.Mkdir("d", 0o700))
		assert.False(t, f.quotaHit.Load(), "an existing directory is not a quota failure")
		assert.Equal(t, errQuotaExceeded, f.Mkdir("e", 0o700))
		assert.True(t, f.quotaHit.Load())
	})

	t.Run("ProtectsMountRoot", func(t *testing.T) {
		t.Parallel()
		f, _ := newFS(t, 1<<20)
		require.Zero(t, f.Mkdir("d", 0o700))
		for _, root := range []string{"", "."} {
			assert.Equal(t, experimentalsys.EPERM, f.Rmdir(root))
			assert.Equal(t, experimentalsys.EPERM, f.Unlink(root))
			assert.Equal(t, experimentalsys.EPERM, f.Rename(root, "x"))
			assert.Equal(t, experimentalsys.EPERM, f.Rename("d", root))
		}
		_, errno := f.Stat("d")
		assert.Zero(t, errno)
	})
}

func TestLimitedFS(t *testing.T) {
	t.Parallel()
	limit := &openLimit{max: 2}
	f := limitedFS{FS: sysfs.DirFS(t.TempDir()), limit: limit}
	a, errno := f.OpenFile(".", experimentalsys.O_RDONLY, 0)
	require.Zero(t, errno)
	b, errno := f.OpenFile(".", experimentalsys.O_RDONLY, 0)
	require.Zero(t, errno)
	_, errno = f.OpenFile(".", experimentalsys.O_RDONLY, 0)
	assert.Equal(t, errTooManyOpenFiles, errno)
	_, errno = f.OpenFile("missing", experimentalsys.O_RDONLY, 0)
	assert.Equal(t, errTooManyOpenFiles, errno)

	require.Zero(t, a.Close())
	require.Zero(t, a.Close())
	assert.Equal(t, int64(1), limit.held(), "a repeated close releases once")
	_, errno = f.OpenFile("missing", experimentalsys.O_RDONLY, 0)
	assert.Equal(t, experimentalsys.ENOENT, errno)
	assert.Equal(t, int64(1), limit.held(), "a failed open releases its slot")
	require.Zero(t, b.Close())
	assert.Zero(t, limit.held())
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
