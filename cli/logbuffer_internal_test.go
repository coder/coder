package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
)

func TestClampLogBufferSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   int64
		want int64
	}{
		{in: -1, want: 0},
		{in: 0, want: 0},
		{in: 1000, want: 1000},
		{in: maxCLILogBufferSize, want: maxCLILogBufferSize},
		{in: maxCLILogBufferSize + 1, want: maxCLILogBufferSize},
	}
	for _, c := range cases {
		require.Equal(t, c.want, clampLogBufferSize(c.in))
	}
}

func TestPruneSessionLogs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := time.Now()
	const total = 55
	for i := 0; i < total; i++ {
		path := filepath.Join(dir, fmt.Sprintf("coder-ssh-%03d.log", i))
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
		// Older index => older modtime, so the highest indexes are newest.
		modTime := base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, os.Chtimes(path, modTime, modTime))
	}
	// A non-matching file should never be pruned.
	other := filepath.Join(dir, "unrelated.txt")
	require.NoError(t, os.WriteFile(other, []byte("x"), 0o600))

	pruneSessionLogs(dir, keepSessionLogFiles)

	remaining, err := filepath.Glob(filepath.Join(dir, "coder-*.log"))
	require.NoError(t, err)
	require.Len(t, remaining, keepSessionLogFiles)
	// The oldest (lowest index) files were removed; the newest were kept.
	require.NoFileExists(t, filepath.Join(dir, "coder-ssh-000.log"))
	require.FileExists(t, filepath.Join(dir, fmt.Sprintf("coder-ssh-%03d.log", total-1)))
	require.FileExists(t, other)
}

func TestDefaultSessionLogDir(t *testing.T) {
	t.Parallel()

	require.True(t, strings.HasSuffix(
		filepath.ToSlash(defaultSessionLogDir()), "coder/logs"),
		"got %q", defaultSessionLogDir())
}

func TestBufferedLogger_BuffersUntilError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	r := &RootCmd{}
	logger := r.bufferedLogger(slog.Make(), sloghuman.Sink(&buf), 10)

	ctx := context.Background()
	logger.Debug(ctx, "buffered-debug")
	logger.Sync()
	require.NotContains(t, buf.String(), "buffered-debug",
		"debug entry should be buffered, not written")

	// Logging an error flushes the buffered history before the error entry.
	logger.Error(ctx, "command failed for test")
	logger.Sync()
	got := buf.String()
	require.Contains(t, got, "buffered-debug")
	require.Contains(t, got, "command failed for test")
}

func TestBufferedLogger_FlushEmitsBuffer(t *testing.T) {
	t.Parallel()

	// Flush is the path bufferedLoggerMiddleware uses to emit the buffered
	// history on a returned error without logging a duplicate error line.
	var buf bytes.Buffer
	r := &RootCmd{}
	logger := r.bufferedLogger(slog.Make(), sloghuman.Sink(&buf), 10)

	ctx := context.Background()
	logger.Debug(ctx, "buffered-debug")
	logger.Sync()
	require.NotContains(t, buf.String(), "buffered-debug")

	logger.Flush(ctx)
	logger.Sync()
	require.Contains(t, buf.String(), "buffered-debug")
}

func TestBufferedLogger_VerboseWritesDebug(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	r := &RootCmd{verbose: true}
	logger := r.bufferedLogger(slog.Make(), sloghuman.Sink(&buf), 10)

	logger.Debug(context.Background(), "verbose-debug")
	logger.Sync()
	require.Contains(t, buf.String(), "verbose-debug",
		"verbose should write debug entries directly")
}
