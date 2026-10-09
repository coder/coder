package agentacp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDiscoveryCatalog(t *testing.T) {
	t.Parallel()
	m, _, configDir := newTestManager(t, fakeHarnessModeBoth)
	catalog := m.Catalog()
	require.Equal(t, "fake", catalog[0].DisplayName)
	require.True(t, catalog[0].Steering)
	require.True(t, catalog[0].LoadSession)
	require.True(t, catalog[0].ResumeSession)
	require.Equal(t, "default", catalog[0].ConfigOptions[0].CurrentValue)
	require.Equal(t, configDir, m.ContextResources()[0].SourcePath)
	hash := m.ContextResources()[0].ContentHash
	require.NoError(t, m.Reload(context.Background()))
	require.Equal(t, hash, m.ContextResources()[0].ContentHash)
	// Returned snapshots must not let callers mutate discovery state.
	catalog[0].ConfigOptions[0].Values[0].Name = "mutated"
	require.Equal(t, "Default", m.Catalog()[0].ConfigOptions[0].Values[0].Name)
	writeConfig(t, filepath.Join(configDir, "disabled.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth), "enabled": false})
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "broken.json"), []byte(`{`), 0o600))
	writeConfig(t, filepath.Join(configDir, "missing.json"), map[string]any{})
	writeConfig(t, filepath.Join(configDir, "failure.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeFail)})
	require.NoError(t, os.Mkdir(filepath.Join(configDir, "nested"), 0o700))
	writeConfig(t, filepath.Join(configDir, "nested", "ignored.json"), map[string]any{"command": "false"})
	require.NoError(t, m.Reload(context.Background()))
	catalog = m.Catalog()
	require.Len(t, catalog, 4)
	for _, h := range catalog {
		if h.Slug != "fake" {
			require.NotEmpty(t, h.Error)
		}
	}
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false", "display_name": "Changed"})
	require.NoError(t, m.Reload(context.Background()))
	require.NotEqual(t, hash, m.ContextResources()[1].ContentHash)
	require.NoError(t, os.Remove(filepath.Join(configDir, "fake.json")))
	require.NoError(t, m.Reload(context.Background()))
	require.Len(t, m.Catalog(), 3)
	// Discovery never falls back when the manifest directory is invalid.
	m.workingDir = func() string { return "" }
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	require.NoError(t, m.Reload(context.Background()))
	for _, h := range m.Catalog() {
		if h.Slug == "fake" {
			require.Contains(t, h.Error, "explicit absolute")
		}
	}
}

func TestDiscoveryDirectoryCreation(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	clock := quartz.NewMock(t)
	m := NewManager(context.Background(), Options{
		Logger:     testutil.Logger(t),
		Clock:      clock,
		Execer:     agentexec.DefaultExecer,
		Filesystem: afero.NewOsFs(),
		EnvInfo:    testEnv{home: home},
		WorkingDir: func() string { return dir },
	})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	changed := make(chan struct{}, 8)
	m.SetOnChange(func() { changed <- struct{}{} })
	ctx := testutil.Context(t, testutil.WaitLong)
	startTestDiscovery(ctx, t, m)
	testutil.RequireReceive(ctx, t, changed)
	require.Empty(t, m.Catalog())
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	require.Empty(t, m.Catalog())
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Len(t, m.Catalog(), 1)
	require.Empty(t, m.Catalog()[0].Error)
	// Disabled harnesses are removed from the catalog.
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth), "enabled": false})
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Empty(t, m.Catalog())
}

func TestProbeDeadline(t *testing.T) {
	t.Parallel()
	m, _, configDir := newTestManager(t, fakeHarnessModeBoth)
	writeConfig(t, filepath.Join(configDir, "hang.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeHang)})
	clock := m.clock.(*quartz.Mock)
	trap := clock.Trap().AfterFunc("acp-timeout")
	defer func() {
		if trap != nil {
			trap.Close()
		}
	}()
	ctx := testutil.Context(t, testutil.WaitLong)
	done := make(chan error, 1)
	go func() { done <- m.Reload(ctx) }()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	trap.Close()
	trap = nil
	clock.Advance(30 * time.Second).MustWait(ctx)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	catalog := m.Catalog()
	require.Len(t, catalog, 2)
	require.Empty(t, catalog[0].Error)
	require.NotEmpty(t, catalog[1].Error)
}

func TestDiscoveryEditsAndDirectoryRecreation(t *testing.T) {
	t.Parallel()
	m, _, configDir := newTestManager(t, fakeHarnessModeBoth)
	changed := make(chan struct{}, 16)
	m.SetOnChange(func() { changed <- struct{}{} })
	clock := m.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	startTestDiscovery(ctx, t, m)
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth), "display_name": "Edited"})
	require.Equal(t, "fake", m.Catalog()[0].DisplayName)
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Equal(t, "Edited", m.Catalog()[0].DisplayName)
	backup := configDir + "-backup"
	require.NoError(t, os.Rename(configDir, backup))
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Empty(t, m.Catalog())
	require.NoError(t, os.Mkdir(configDir, 0o700))
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Len(t, m.Catalog(), 1)
	require.NoError(t, os.Remove(filepath.Join(configDir, "fake.json")))
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Empty(t, m.Catalog())
}

func TestDiscoveryUnchangedCatalog(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	changed := make(chan struct{}, 8)
	m.SetOnChange(func() { changed <- struct{}{} })
	clock := m.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	hash := m.ContextResources()[0].ContentHash
	startTestDiscovery(ctx, t, m)
	m.RunDiscovery()
	for range 2 {
		advanceTestDiscovery(ctx, t, clock)
		require.Equal(t, hash, m.ContextResources()[0].ContentHash)
		require.Empty(t, changed)
		// Each probe creates a native session file. Unchanged polls must not
		// start another subprocess or create another native session.
		files, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, files, 1)
	}
	require.NoError(t, m.Close())
	_, pending := clock.Peek()
	require.False(t, pending, "shutdown must stop the discovery timer")
}

func TestDiscoveryRefresh(t *testing.T) {
	t.Parallel()
	m, _, configDir := newTestManager(t, fakeHarnessModeBoth)
	changed := make(chan struct{}, 8)
	m.SetOnChange(func() { changed <- struct{}{} })
	ctx := testutil.Context(t, testutil.WaitLong)
	startTestDiscovery(ctx, t, m)
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth), "display_name": "Refreshed"})
	m.RefreshDiscovery()
	testutil.RequireReceive(ctx, t, changed)
	require.Equal(t, "Refreshed", m.Catalog()[0].DisplayName)
}

func startTestDiscovery(ctx context.Context, t *testing.T, m *Manager) {
	t.Helper()
	trap := m.clock.(*quartz.Mock).Trap().NewTimer("acp-discovery")
	defer trap.Close()
	m.RunDiscovery()
	call := trap.MustWait(ctx)
	require.Equal(t, discoveryInterval, call.Duration)
	call.MustRelease(ctx)
}

func advanceTestDiscovery(ctx context.Context, t *testing.T, clock *quartz.Mock) {
	t.Helper()
	trap := clock.Trap().TimerReset("acp-discovery")
	defer trap.Close()
	clock.Advance(discoveryInterval).MustWait(ctx)
	call := trap.MustWait(ctx)
	require.Equal(t, discoveryInterval, call.Duration)
	call.MustRelease(ctx)
}
