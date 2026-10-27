package filefinder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
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
	w, err := newFSWatcher(t.TempDir(), logger, NewIgnorer(afero.NewOsFs(), ""))
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
	w, err := newFSWatcher(dir, logger, NewIgnorer(afero.NewOsFs(), ""))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.Start(ctx)
	requireCloses(t, w)
}

func TestEventBatch_KeepsFinalStatePerPath(t *testing.T) {
	t.Parallel()

	// The sequence an atomic write produces: create and modify a temp
	// file, rename it away, then the real file is created by the
	// rename.
	var b eventBatch
	b.add(FSEvent{Op: OpCreate, Path: "/r/.a.txt.tmp.1"})
	b.add(FSEvent{Op: OpModify, Path: "/r/.a.txt.tmp.1"})
	b.add(FSEvent{Op: OpRename, Path: "/r/.a.txt.tmp.1"})
	b.add(FSEvent{Op: OpCreate, Path: "/r/a.txt"})
	b.add(FSEvent{Op: OpModify, Path: "/r/a.txt"})
	// A removed then recreated path ends as a create, after the dir
	// removal that preceded the recreate.
	b.add(FSEvent{Op: OpCreate, Path: "/r/d/x"})
	b.add(FSEvent{Op: OpRemove, Path: "/r/d", IsDir: true})
	b.add(FSEvent{Op: OpCreate, Path: "/r/d/x"})

	require.Equal(t, []FSEvent{
		{Op: OpRename, Path: "/r/.a.txt.tmp.1"},
		{Op: OpCreate, Path: "/r/a.txt"},
		{Op: OpRemove, Path: "/r/d", IsDir: true},
		{Op: OpCreate, Path: "/r/d/x"},
	}, b.take())
	require.Empty(t, b.take())
}

func TestEngine_AtomicWriteBatchLeavesNoTempEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	e := NewEngine(logger)
	t.Cleanup(func() { _ = e.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, e.AddRoot(ctx, dir))

	var b eventBatch
	for i := range 3 {
		tmp := filepath.Join(dir, fmt.Sprintf(".api.txt.tmp.%d", i))
		b.add(FSEvent{Op: OpCreate, Path: tmp})
		b.add(FSEvent{Op: OpModify, Path: tmp})
		b.add(FSEvent{Op: OpRename, Path: tmp})
		b.add(FSEvent{Op: OpCreate, Path: filepath.Join(dir, "api.txt")})
	}
	e.mu.Lock()
	gen := e.roots[dir].gen
	e.mu.Unlock()
	e.applyEvents(rootEvent{root: dir, gen: gen, events: b.take()})

	results, err := e.Search(ctx, "api.txt", DefaultSearchOptions())
	require.NoError(t, err)
	paths := make([]string, 0, len(results))
	for _, r := range results {
		paths = append(paths, r.Path)
	}
	require.Equal(t, []string{"api.txt"}, paths)
}

func fallbackIgnorer() *Ignorer {
	return NewIgnorer(afero.NewOsFs(), "", WithGit(nil, ""))
}

func TestFSWatcher_DeliversChangesAfterStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), nil, 0o600))
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	w, err := newFSWatcher(dir, logger, fallbackIgnorer())
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)

	initial := w.Start(ctx)
	require.Equal(t, []FSEvent{{Op: OpCreate, Path: filepath.Join(dir, "a.go")}}, initial)

	// Changes after the walk arrive as events, so an index built from
	// the walk converges once they are applied.
	require.NoError(t, os.Remove(filepath.Join(dir, "a.go")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), nil, 0o600))
	var removed, created bool
	for !removed || !created {
		for _, ev := range testutil.RequireReceive(ctx, t, w.Events()) {
			removed = removed || ev.Op == OpRemove && ev.Path == filepath.Join(dir, "a.go")
			created = created || ev.Op == OpCreate && ev.Path == filepath.Join(dir, "b.go")
		}
	}
}

func TestEngine_DropsEventsFromOldWatcher(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	e := NewEngine(logger, WithIgnorer(fallbackIgnorer()))
	t.Cleanup(func() { _ = e.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, e.AddRoot(ctx, dir))

	e.mu.Lock()
	gen := e.roots[dir].gen
	e.mu.Unlock()
	e.applyEvents(rootEvent{root: dir, gen: gen - 1, events: []FSEvent{{Op: OpCreate, Path: filepath.Join(dir, "stale.go")}}})
	e.applyEvents(rootEvent{root: dir, gen: gen, events: []FSEvent{{Op: OpCreate, Path: filepath.Join(dir, "current.go")}}})

	e.mu.Lock()
	defer e.mu.Unlock()
	require.False(t, e.roots[dir].index.Has("stale.go"))
	require.True(t, e.roots[dir].index.Has("current.go"))
}

func TestEngine_ReloadWaitsForWalkSlotAndStopsOnRemove(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slot_file.txt"), nil, 0o600))
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	e := NewEngine(logger, WithIgnorer(fallbackIgnorer()))
	t.Cleanup(func() { _ = e.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, e.AddRoot(ctx, dir))

	// Hold every walk slot so the reload must wait.
	for range maxConcurrentWalks {
		e.walks <- struct{}{}
	}
	t.Cleanup(func() {
		for range maxConcurrentWalks {
			<-e.walks
		}
	})
	e.mu.Lock()
	rs := e.roots[dir]
	e.mu.Unlock()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.txt\n"), 0o600))
	require.Eventually(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		return rs.reloading
	}, testutil.WaitShort, testutil.IntervalFast)
	// Without a slot the reload cannot finish. An unadmitted reload
	// of this tiny root would finish within milliseconds.
	require.Never(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		return !rs.reloading || !rs.index.Has("slot_file.txt")
	}, testutil.IntervalSlow, testutil.IntervalFast, "reload ran without a walk slot")

	// Removing the root cancels the waiting reload.
	require.NoError(t, e.RemoveRoot(dir))
	require.Eventually(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		return !rs.reloading
	}, testutil.WaitShort, testutil.IntervalFast)
}
