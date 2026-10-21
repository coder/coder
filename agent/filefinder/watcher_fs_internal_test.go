package filefinder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func requireCloses(t *testing.T, w *fsWatcher) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- w.Close() }()
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestFSWatcher_CloseUnstarted(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	w, err := newFSWatcher(t.TempDir(), logger)
	require.NoError(t, err)
	requireCloses(t, w)
	// Start after Close does nothing.
	w.Start(context.Background())
}

func TestFSWatcher_CloseAfterCanceledStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), nil, 0o600))
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	w, err := newFSWatcher(dir, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.Start(ctx)
	requireCloses(t, w)
}
