package agentcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

func newTestManager(t *testing.T, opts agentcontext.ManagerOptions) *agentcontext.Manager {
	t.Helper()
	opts.Logger = testutil.Logger(t).Named("agentcontext-test")
	m, err := agentcontext.NewManager(opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestManager_InitialSnapshotIsPopulated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "boot snapshot")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})

	snap := m.Snapshot()
	require.Equal(t, uint64(1), snap.Version)
	require.Equal(t, agentcontext.CurrentSchemaVersion, snap.SchemaVersion)
	require.Len(t, snap.Resources, 1)
}

func TestManager_AddSourceTriggersResolve(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "AGENTS.md"), "from source")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()

	t.Cleanup(func() { _ = m.Close() })

	// Subscribe before mutating so we observe the broadcast.
	ch, unsub := m.SubscribeChanges()
	defer unsub()

	added, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)
	require.Equal(t, src, added.Path)

	select {
	case <-ch:
	case <-time.After(testutil.WaitShort):
		t.Fatalf("expected a change broadcast after AddSource")
	}

	snap := m.Snapshot()
	require.Greater(t, snap.Version, uint64(1))

	found := false
	for _, r := range snap.Resources {
		if r.Kind == agentcontext.KindInstructionFile && r.SourcePath == src {
			found = true
		}
	}
	require.True(t, found, "expected AGENTS.md attributed to the user source")
}

func TestManager_AddSourceRejectsOutsideAllowedRoots(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	outside := t.TempDir()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd},
	})

	_, err := m.AddSource(agentcontext.Source{Path: outside})
	require.Error(t, err)
}

func TestManager_AddSourceIsIdempotent(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := t.TempDir()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	added1, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)
	added2, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)
	require.Equal(t, added1.Path, added2.Path)

	sources := m.Sources()
	require.Len(t, sources, 1)
}

func TestManager_RemoveSource(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := t.TempDir()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	_, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)
	require.NoError(t, m.RemoveSource(src))
	require.Empty(t, m.Sources())

	err = m.RemoveSource(src)
	require.ErrorIs(t, err, agentcontext.ErrSourceNotFound)
}

func TestManager_HasSource(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := t.TempDir()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	canonical, ok := m.HasSource(src)
	require.False(t, ok)
	require.Equal(t, src, canonical)

	_, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)

	canonical, ok = m.HasSource(src)
	require.True(t, ok)
	require.Equal(t, src, canonical)
}

func TestManager_ResyncReturnsLatestSnapshot(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, "AGENTS.md"), "first")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return wd },
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = m.Run(ctx)
	}()
	t.Cleanup(func() {
		_ = m.Close()
		<-runDone
	})

	// Mutate AGENTS.md and call Resync. The returned
	// snapshot must reflect the new content.
	require.NoError(t, os.WriteFile(filepath.Join(wd, "AGENTS.md"), []byte("second content edit"), 0o600))

	snap, err := m.Resync(ctx)
	require.NoError(t, err)

	require.Len(t, snap.Resources, 1)
	require.Equal(t, "second content edit", string(snap.Resources[0].Payload))
}

func TestManager_InitialSourcesSeeded(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := t.TempDir()
	mustWriteFile(t, filepath.Join(src, "AGENTS.md"), "from initial")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:     func() string { return wd },
		AllowedRoots:   []string{wd, src},
		InitialSources: []agentcontext.Source{{Path: src}},
	})

	sources := m.Sources()
	require.Len(t, sources, 1)
	require.Equal(t, src, sources[0].Path)

	snap := m.Snapshot()
	require.Len(t, snap.Resources, 1)
	require.Equal(t, src, snap.Resources[0].SourcePath)
}

func TestManager_CloseIsIdempotent(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})
	require.NoError(t, m.Close())
	require.NoError(t, m.Close())
}

func TestManager_RunOnce(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()
	go func() { _ = m.Run(ctx) }()
	// Brief wait so Run has a chance to set running=true.
	time.Sleep(50 * time.Millisecond)

	err := m.Run(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than once")
	cancel()
	_ = m.Close()
}

func TestManager_SubscribeBroadcastOnChange(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := t.TempDir()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()

	ch, unsub := m.SubscribeChanges()
	defer unsub()

	_, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)

	select {
	case <-ch:
	case <-time.After(testutil.WaitShort):
		t.Fatal("expected subscriber to be notified")
	}
}
