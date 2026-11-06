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
	m := newPendingTestManager(t, opts)
	// Most tests want the ready behavior; release the gate so the first
	// snapshot is the resolved inventory at version 1. Gate tests use
	// newPendingTestManager directly.
	m.SetReady()
	return m
}

// newPendingTestManager builds a gated Manager (first snapshot is the
// empty version-0 placeholder) for tests that drive SetReady
// themselves.
func newPendingTestManager(t *testing.T, opts agentcontext.ManagerOptions) *agentcontext.Manager {
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
	wd := testutil.TempDirResolved(t)
	src := filepath.Join(wd, "source")
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
	_, err := m.AddSource(agentcontext.Source{Path: src})
	require.Error(t, err)

	// After the manifest "loads", workingDir resolves and the
	// same path validates without restarting the Manager.
	resolved.Store(&wd)
	_, err = m.AddSource(agentcontext.Source{Path: src})
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

// TestManager_SourceIdentityIsLexicalAndStable verifies the same configured
// source added before and after its symlink target exists collapses to one
// source keyed by the lexical (configured) path, not the resolved target.
func TestManager_SourceIdentityIsLexicalAndStable(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	root := testutil.TempDirResolved(t)
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link))

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return root },
		AllowedRoots: []string{root},
	})

	// Target missing: identity is the lexical link path.
	added1, err := m.AddSource(agentcontext.Source{Path: link})
	require.NoError(t, err)
	require.Equal(t, link, added1.Path)

	// Once the target exists the link resolves, but the same configured
	// path must still dedupe to one source.
	require.NoError(t, os.MkdirAll(target, 0o755))
	added2, err := m.AddSource(agentcontext.Source{Path: link})
	require.NoError(t, err)
	require.Equal(t, link, added2.Path,
		"expected the source identity to stay the lexical link path")

	require.Len(t, m.Sources(), 1)
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

func TestManager_AutomaticRootExclusions(t *testing.T) {
	t.Parallel()
	wd := testutil.TempDirResolved(t)
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "WorkingDirectory", path: wd},
		{name: "BuiltinCoder", path: "~/.coder"},
		{name: "BuiltinSkills", path: "~/.coder/skills"},
		{name: "BuiltinPlugins", path: "~/.claude/plugins/cache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newTestManager(t, agentcontext.ManagerOptions{
				WorkingDir:   func() string { return wd },
				AllowedRoots: []string{wd},
			})
			resolved, err := agentcontext.CanonicalizePath(tc.path)
			require.NoError(t, err)
			_, err = m.AddSource(agentcontext.Source{Path: tc.path})
			require.NoError(t, err)
			require.NoError(t, m.RemoveSource(tc.path))
			require.Equal(t, []agentcontext.Source{{Path: resolved, Excluded: true}}, m.Sources())
			require.NoError(t, m.RemoveSource(tc.path), "removing an exclusion is idempotent")
			_, err = m.AddSource(agentcontext.Source{Path: tc.path})
			require.NoError(t, err)
			require.Equal(t, []agentcontext.Source{{Path: resolved}}, m.Sources())
		})
	}
}

//nolint:paralleltest,tparallel // Uses t.Setenv.
func TestManager_AutomaticRootExclusionAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	home := testutil.TempDirResolved(t)
	switchHomeEnv(t, home)
	wd := testutil.TempDirResolved(t)
	for _, tc := range []struct {
		name   string
		path   string
		seeded bool
	}{
		{name: "WorkingDirectory", path: wd},
		{name: "BuiltinCoder", path: filepath.Join(home, ".coder")},
		{name: "BuiltinSkills", path: filepath.Join(home, ".coder", "skills")},
		{name: "BuiltinPlugins", path: filepath.Join(home, ".claude", "plugins", "cache")},
		{name: "SeededSource", path: filepath.Join(home, "seeded"), seeded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.MkdirAll(tc.path, 0o700))
			alias := filepath.Join(testutil.TempDirResolved(t), "alias")
			require.NoError(t, os.Symlink(tc.path, alias))
			m := newTestManager(t, agentcontext.ManagerOptions{
				WorkingDir:   func() string { return wd },
				AllowedRoots: []string{wd, home},
			})
			if tc.seeded {
				m.SeedSources([]agentcontext.Source{{Path: tc.path}})
			}
			require.NoError(t, m.RemoveSource(alias))
			require.Equal(t, []agentcontext.Source{{Path: tc.path, Excluded: true}}, m.Sources())
			if tc.seeded {
				m.SeedSources([]agentcontext.Source{{Path: alias}})
				require.Contains(t, m.Sources(), agentcontext.Source{Path: alias, Excluded: true})
			}
			_, err := m.AddSource(agentcontext.Source{Path: alias})
			require.NoError(t, err)
			require.Contains(t, m.Sources(), agentcontext.Source{Path: alias})
			require.NoError(t, m.RemoveSource(tc.path))
			for _, source := range m.Sources() {
				require.True(t, source.Excluded)
			}
		})
	}
}

func TestManager_SeededSourceExclusions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*agentcontext.Manager, string)
	}{
		{
			name: "InitialSource",
			setup: func(t *testing.T) (*agentcontext.Manager, string) {
				src := testutil.TempDirResolved(t)
				m := newTestManager(t, agentcontext.ManagerOptions{
					InitialSources: []agentcontext.Source{{Path: src}},
				})
				return m, src
			},
		},
		{
			name: "LateSeed",
			setup: func(t *testing.T) (*agentcontext.Manager, string) {
				src := testutil.TempDirResolved(t)
				m := newTestManager(t, agentcontext.ManagerOptions{})
				m.SeedSources([]agentcontext.Source{{Path: src}})
				return m, src
			},
		},
		{
			name: "ExistingSourceSeeded",
			setup: func(t *testing.T) (*agentcontext.Manager, string) {
				src := testutil.TempDirResolved(t)
				m := newTestManager(t, agentcontext.ManagerOptions{AllowedRoots: []string{src}})
				_, err := m.AddSource(agentcontext.Source{Path: src})
				require.NoError(t, err)
				m.SeedSources([]agentcontext.Source{{Path: src}})
				return m, src
			},
		},
		{
			name: "SeededBuiltin",
			setup: func(t *testing.T) (*agentcontext.Manager, string) {
				m := newTestManager(t, agentcontext.ManagerOptions{
					InitialSources: []agentcontext.Source{{Path: "~/.coder"}},
				})
				return m, "~/.coder"
			},
		},
		{
			name: "SeededWorkingDirectory",
			setup: func(t *testing.T) (*agentcontext.Manager, string) {
				wd := testutil.TempDirResolved(t)
				m := newTestManager(t, agentcontext.ManagerOptions{
					WorkingDir:     func() string { return wd },
					InitialSources: []agentcontext.Source{{Path: wd}},
				})
				return m, wd
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, path := tc.setup(t)
			resolved, err := agentcontext.CanonicalizePath(path)
			require.NoError(t, err)
			require.NoError(t, m.RemoveSource(path))
			m.SeedSources([]agentcontext.Source{{Path: path}})
			require.Equal(t, []agentcontext.Source{{Path: resolved, Excluded: true}}, m.Sources())
			_, err = m.AddSource(agentcontext.Source{Path: path})
			require.NoError(t, err)
			require.Equal(t, []agentcontext.Source{{Path: resolved}}, m.Sources())
		})
	}
}

//nolint:paralleltest,tparallel // Uses t.Setenv.
func TestManager_ExcludedNestedDiscovery(t *testing.T) {
	home := testutil.TempDirResolved(t)
	switchHomeEnv(t, home)
	skills := filepath.Join(home, ".coder", "skills")
	instructions := filepath.Join(home, ".coder", "AGENTS.md")
	mcpConfig := filepath.Join(home, ".coder", ".mcp.json")
	mustWriteSkill(t, skills, "builtin", "builtin skill")
	mustWriteFile(t, instructions, "builtin instructions")
	mustWriteFile(t, mcpConfig, `{"mcpServers":{}}`)
	opts := agentcontext.ManagerOptions{
		InitialSources: []agentcontext.Source{{Path: instructions}, {Path: mcpConfig}},
	}
	m := newTestManager(t, opts)
	require.Len(t, m.Snapshot().Resources, 3)
	for i, path := range []string{skills, instructions, mcpConfig} {
		require.NoError(t, m.RemoveSource(path))
		snapshot, err := m.Resync(testutil.Context(t, testutil.WaitShort))
		require.NoError(t, err)
		require.Len(t, snapshot.Resources, 2-i, "the parent ~/.coder scan must not rediscover an excluded root")
	}
	restarted := newTestManager(t, opts)
	require.Len(t, restarted.Snapshot().Resources, 3, "exclusions do not outlive the Manager")
	_, err := m.AddSource(agentcontext.Source{Path: filepath.Join(home, ".coder")})
	require.NoError(t, err)
	snapshot, err := m.Resync(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	require.Empty(t, snapshot.Resources, "adding a parent does not clear separate child exclusions")
	for _, path := range []string{skills, instructions, mcpConfig} {
		_, err = m.AddSource(agentcontext.Source{Path: path})
		require.NoError(t, err)
	}
	snapshot, err = m.Resync(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	require.Len(t, snapshot.Resources, 3)
}

func TestManager_ExclusionDescendants(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	wd := filepath.Join(root, "project")
	skills := filepath.Join(wd, ".agents", "skills")
	claudeSkills := filepath.Join(wd, ".claude", "skills")
	explicit := filepath.Join(wd, "explicit")
	sibling := filepath.Join(root, "project-other")
	late := filepath.Join(wd, "late")
	mustWriteFile(t, filepath.Join(wd, "AGENTS.md"), "project instructions")
	mustWriteSkill(t, skills, "auto", "automatic skill")
	mustWriteSkill(t, claudeSkills, "claude", "claude skill")
	mustWriteFile(t, filepath.Join(explicit, "AGENTS.md"), "explicit instructions")
	mustWriteFile(t, filepath.Join(sibling, "AGENTS.md"), "sibling instructions")
	seeded := []agentcontext.Source{{Path: skills}, {Path: sibling}}
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:     func() string { return wd },
		AllowedRoots:   []string{root},
		InitialSources: seeded,
	})
	_, err := m.AddSource(agentcontext.Source{Path: explicit})
	require.NoError(t, err)
	resourcePaths := func() []string {
		t.Helper()
		snap, err := m.Resync(testutil.Context(t, testutil.WaitShort))
		require.NoError(t, err)
		paths := make([]string, 0, len(snap.Resources))
		for _, resource := range snap.Resources {
			paths = append(paths, resource.Source)
		}
		return paths
	}
	initial := []string{
		filepath.Join(wd, "AGENTS.md"), filepath.Join(skills, "auto"),
		filepath.Join(claudeSkills, "claude"), filepath.Join(explicit, "AGENTS.md"),
		filepath.Join(sibling, "AGENTS.md"),
	}
	require.ElementsMatch(t, initial, resourcePaths())

	require.NoError(t, m.RemoveSource(wd))
	included := []string{filepath.Join(explicit, "AGENTS.md"), filepath.Join(sibling, "AGENTS.md")}
	require.ElementsMatch(t, included, resourcePaths())
	mustWriteFile(t, filepath.Join(late, "AGENTS.md"), "late seed")
	m.SeedSources(append(seeded, agentcontext.Source{Path: late}))
	require.ElementsMatch(t, included, resourcePaths(), "reconnect and late seeds cannot override an exclusion")
	require.Contains(t, m.Sources(), agentcontext.Source{Path: wd, Excluded: true})
	require.Contains(t, m.Sources(), agentcontext.Source{Path: skills, Excluded: true})
	require.Contains(t, m.Sources(), agentcontext.Source{Path: late, Excluded: true})

	_, err = m.AddSource(agentcontext.Source{Path: late})
	require.NoError(t, err)
	require.ElementsMatch(t, append(included, filepath.Join(late, "AGENTS.md")), resourcePaths())
	require.NoError(t, m.RemoveSource(late))
	_, err = m.AddSource(agentcontext.Source{Path: wd})
	require.NoError(t, err)
	require.ElementsMatch(t, initial, resourcePaths(), "adding the parent restores automatic descendants except separate exclusions")
}

func TestManager_GetSource(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := testutil.TempDirResolved(t)

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	source, ok := m.GetSource(src)
	require.False(t, ok)
	require.Equal(t, src, source.Path)

	_, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)

	source, ok = m.GetSource(src)
	require.True(t, ok)
	require.Equal(t, agentcontext.Source{Path: src}, source)
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

func TestManager_InitialSkillSourcesPreservePrecedence(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	firstRoot := filepath.Join(t.TempDir(), "b", "skills")
	secondRoot := filepath.Join(t.TempDir(), "a", "skills")
	mustWriteSkill(t, firstRoot, "make-coffee", "first root")
	mustWriteSkill(t, secondRoot, "make-coffee", "second root")
	mustWriteSkill(t, filepath.Join(wd, ".agents", "skills"), "make-coffee", "working directory")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, firstRoot, secondRoot},
		InitialSources: []agentcontext.Source{
			{Path: firstRoot},
			{Path: secondRoot},
		},
	})

	snap := m.Snapshot()
	require.Len(t, snap.Resources, 1)
	require.Equal(t, filepath.Join(firstRoot, "make-coffee"), snap.Resources[0].Source)
	require.Equal(t, "first root", snap.Resources[0].Description)
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

// TestManager_WithholdsCollectionUntilReady reproduces the boot-time
// race: collecting before startup finishes sees instruction-file
// symlinks (CLAUDE.md / .cursorrules -> AGENTS.md) with no target yet.
// The gated snapshot is the version-0 placeholder with no resources or
// errors; after SetReady the symlinks resolve cleanly.
func TestManager_WithholdsCollectionUntilReady(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	dir := testutil.TempDirResolved(t)
	// CLAUDE.md and .cursorrules symlink to an AGENTS.md that does not
	// exist yet, so an eager resolve would report them unreadable.
	require.NoError(t, os.Symlink(filepath.Join(dir, "AGENTS.md"), filepath.Join(dir, "CLAUDE.md")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "AGENTS.md"), filepath.Join(dir, ".cursorrules")))

	m := newPendingTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})

	// Before SetReady the snapshot is the version-0 placeholder: no
	// resources and no errors. The broken symlinks are NOT reported.
	snap := m.Snapshot()
	require.Zero(t, snap.Version, "gated snapshot must be the version-0 placeholder")
	require.Empty(t, snap.Resources, "gated snapshot must not collect a partial inventory")
	require.Empty(t, snap.SnapshotError, "gated snapshot must not surface transient errors")

	// Resync stays gated too, so callers see the version-0 placeholder
	// instead of a partial result.
	rs, err := m.Resync(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	require.Zero(t, rs.Version)
	require.Empty(t, rs.Resources)

	// Startup finishes: AGENTS.md now exists, so the symlinks resolve.
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "# rules\n")

	// Release the gate. SetReady performs the first real resolve.
	m.SetReady()

	snap = m.Snapshot()
	require.NotZero(t, snap.Version, "post-ready snapshot must be a real resolve")
	require.NotEmpty(t, snap.Resources, "post-ready snapshot must include resolved files")
	require.Empty(t, snap.SnapshotError)
	var instr int
	for _, r := range snap.Resources {
		require.NotEqualf(t, agentcontext.StatusUnreadable, r.Status,
			"resolved snapshot must not contain spurious unreadable issues: %s", r.Source)
		if r.Kind == agentcontext.KindInstructionFile {
			instr++
		}
	}
	// AGENTS.md and the two symlinks that resolve to it collapse to a
	// single instruction-file resource.
	require.Equal(t, 1, instr)
}

// TestManager_SetReadyIsIdempotent verifies the first SetReady resolves
// once (version 1) and repeated calls neither panic nor re-resolve.
func TestManager_SetReadyIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "rules")

	// Gated: first snapshot is the version-0 placeholder.
	m := newPendingTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})
	snap := m.Snapshot()
	require.Zero(t, snap.Version)
	require.Empty(t, snap.Resources)

	// First SetReady resolves once: version 1 with the inventory.
	m.SetReady()
	snap = m.Snapshot()
	require.Equal(t, uint64(1), snap.Version)
	require.Len(t, snap.Resources, 1)

	// Further calls are no-ops: version and inventory unchanged.
	m.SetReady()
	m.SetReady()
	snap = m.Snapshot()
	require.Equal(t, uint64(1), snap.Version)
	require.Len(t, snap.Resources, 1)
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

// TestManager_WorkingDirScannedShallow confirms the working
// directory is a single scan root: its top-level instruction files
// are read, but the resolver neither climbs to an ancestor (no
// walk-up to a .git project root) nor descends into subdirectories.
func TestManager_WorkingDirScannedShallow(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "root rules")
	cwd := filepath.Join(root, "service")
	require.NoError(t, os.MkdirAll(cwd, 0o755))
	mustWriteFile(t, filepath.Join(cwd, "AGENTS.md"), "service rules")
	// A subdirectory below the working dir must not be descended.
	mustWriteFile(t, filepath.Join(cwd, "nested", "AGENTS.md"), "nested rules")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return cwd },
	})

	snap := m.Snapshot()
	var sources []string
	for _, r := range snap.Resources {
		if r.Kind == agentcontext.KindInstructionFile {
			sources = append(sources, r.Source)
		}
	}
	// Only the working directory's own AGENTS.md is present: the
	// ancestor root and the nested subdirectory are both excluded.
	require.Equal(t, []string{filepath.Join(cwd, "AGENTS.md")}, sources)
}
