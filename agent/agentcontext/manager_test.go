package agentcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

// TestMain points the test binary's HOME (and USERPROFILE on
// Windows) at a fresh empty directory before any test runs.
// The package's built-in scan roots (~/.coder,
// ~/.coder/skills, ~/.claude/plugins/cache) canonicalize
// against this directory, so they resolve to non-existent
// paths and the resolver silently skips them. Without this,
// running the tests on a developer host pulls real Coder and
// Claude config files into snapshots and breaks every
// Len(Resources, N) assertion.
func TestMain(m *testing.M) {
	// The MCP runner re-execs this test binary as a fake stdio MCP
	// server (TEST_MCP_FAKE_SERVER=1). Serve and exit before any test
	// setup runs.
	if maybeServeFakeMCPServer() {
		os.Exit(0)
	}

	home, err := os.MkdirTemp("", "agentcontext-test-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Setenv("USERPROFILE", home); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func newTestManager(t *testing.T, opts agentcontext.ManagerOptions) *agentcontext.Manager {
	t.Helper()
	opts.Logger = testutil.Logger(t).Named("agentcontext-test")
	m := agentcontext.NewManager(opts)
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
	require.Len(t, snap.Resources, 1)
}

func TestManager_AddSourceTriggersResolve(t *testing.T) {
	t.Parallel()
	wd := testutil.TempDirResolved(t)
	src := testutil.TempDirResolved(t)
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

// TestManager_AddSourceAcceptsLateWorkingDir mirrors the agent's
// real boot order: AllowedRoots is configured before the
// manifest provides the workspace working directory. The Manager
// must consult WorkingDir on every check so paths under the
// resolved working dir validate once the manifest lands.
func TestManager_AddSourceAcceptsLateWorkingDir(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	var resolved atomic.Pointer[string]
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string {
			if p := resolved.Load(); p != nil {
				return *p
			}
			return ""
		},
		AllowedRoots: []string{"/never-used-home"},
	})

	// Before the manifest "loads", workingDir is empty; sources
	// under wd must be rejected.
	_, err := m.AddSource(agentcontext.Source{Path: wd})
	require.Error(t, err)

	// After the manifest "loads", workingDir resolves and the
	// same path validates without restarting the Manager.
	resolved.Store(&wd)
	_, err = m.AddSource(agentcontext.Source{Path: wd})
	require.NoError(t, err)
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
	src := testutil.TempDirResolved(t)

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

// TestManager_ResyncCanceledKeepsLiveSnapshot guards CRF-44:
// a context cancellation mid-walk must not replace the live
// Snapshot with an empty one. Resync returns the existing
// Snapshot and ctx.Err() instead of publishing a stub.
func TestManager_ResyncCanceledKeepsLiveSnapshot(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, "AGENTS.md"), "live content")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return wd },
	})

	// Capture the live snapshot the Manager populated at
	// construction time.
	live := m.Snapshot()
	require.Len(t, live.Resources, 1)
	require.Equal(t, "live content", string(live.Resources[0].Payload))

	// Cancel the context before calling Resync so
	// ResolveContext observes the cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	snap, err := m.Resync(ctx)
	require.ErrorIs(t, err, context.Canceled)
	// The returned snapshot must still expose the live
	// resources, not an empty result from the canceled walk.
	require.Len(t, snap.Resources, 1)
	require.Equal(t, "live content", string(snap.Resources[0].Payload))

	// The next Snapshot call must also return live content;
	// no stub was published.
	after := m.Snapshot()
	require.Equal(t, live.Version, after.Version)
	require.Len(t, after.Resources, 1)
}

func TestManager_InitialSourcesSeeded(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := testutil.TempDirResolved(t)
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

// TestManager_SeedSourcesLateBindsAfterManifest models the
// agent's behavior when CODER_AGENT_EXP_*_DIRS contains a
// relative path that cannot resolve until the manifest's
// working directory lands. SeedSources must adopt the
// previously-unresolvable path, bypass AllowedRoots
// validation, and trigger a re-resolve.
func TestManager_SeedSourcesLateBindsAfterManifest(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	late := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(late, "AGENTS.md"), "late binding")

	// AllowedRoots intentionally omits `late` so AddSource
	// would reject it. SeedSources must accept it anyway,
	// since the path comes from the trusted template config.
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd},
	})

	require.Empty(t, m.Sources())

	m.SeedSources([]agentcontext.Source{{Path: late}})

	sources := m.Sources()
	require.Len(t, sources, 1)
	require.Equal(t, late, sources[0].Path)

	snap, err := m.Resync(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	require.Len(t, snap.Resources, 1)
	require.Equal(t, late, snap.Resources[0].SourcePath)
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

	// Wait for Run to claim the running flag, then verify the
	// second call rejects with a deterministic error rather than
	// racing the scheduler.
	select {
	case <-agentcontext.ManagerStarted(m):
	case <-ctx.Done():
		t.Fatalf("manager never started: %v", ctx.Err())
	}

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

// TestManager_MCPResourcesAppliesToSnapshot verifies that MCP resources
// supplied via the resolver contribute KindMCPServer resources (with
// their tools) to the resolved snapshot.
func TestManager_MCPResourcesAppliesToSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		Resolver: &agentcontext.Resolver{
			MCPResources: func() []agentcontext.Resource {
				return []agentcontext.Resource{{
					ID:     "mcp_server:fs",
					Kind:   agentcontext.KindMCPServer,
					Source: "fs",
					Name:   "fs",
					Status: agentcontext.StatusOK,
					Tools:  []agentcontext.MCPTool{{Name: "read", Description: "Read"}},
				}}
			},
		},
	})

	snap := m.Snapshot()
	var found bool
	for _, r := range snap.Resources {
		if r.Kind == agentcontext.KindMCPServer && r.Source == "fs" {
			found = true
			require.Len(t, r.Tools, 1)
			require.Equal(t, "read", r.Tools[0].Name)
		}
	}
	require.True(t, found, "expected MCP server resource in snapshot")
}
