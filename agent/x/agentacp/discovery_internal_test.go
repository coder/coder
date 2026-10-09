package agentacp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDiscoveryCatalog(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	m, configDir := newTestManager(t)
	catalog := m.Catalog()
	require.Equal(t, "fake", catalog[0].DisplayName)
	require.NoError(t, m.Reload(ctx))
	require.Equal(t, catalog, m.Catalog())
	// Returned snapshots must not let callers mutate discovery state.
	catalog[0].DisplayName = "mutated"
	require.Equal(t, "fake", m.Catalog()[0].DisplayName)
	writeConfig(t, filepath.Join(configDir, "disabled.json"), map[string]any{"command": "false", "enabled": false})
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "broken.json"), []byte(`{`), 0o600))
	writeConfig(t, filepath.Join(configDir, "missing.json"), map[string]any{})
	require.NoError(t, os.Mkdir(filepath.Join(configDir, "nested"), 0o700))
	writeConfig(t, filepath.Join(configDir, "nested", "ignored.json"), map[string]any{"command": "false"})
	require.NoError(t, m.Reload(ctx))
	catalog = m.Catalog()
	require.Len(t, catalog, 3)
	for _, h := range catalog {
		if h.Slug != "fake" {
			require.NotEmpty(t, h.Error)
		}
	}
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false", "display_name": "Changed"})
	require.NoError(t, m.Reload(ctx))
	var found bool
	for _, harness := range m.Catalog() {
		if harness.Slug == "fake" {
			found = true
			require.Equal(t, "Changed", harness.DisplayName)
			require.Empty(t, harness.Error)
		}
	}
	require.True(t, found, "the edited harness must remain in the catalog")
	require.NoError(t, os.Remove(filepath.Join(configDir, "fake.json")))
	require.NoError(t, m.Reload(ctx))
	require.Len(t, m.Catalog(), 2)
}

func TestDiscoveryDirectoryCreation(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	clock := quartz.NewMock(t)
	m := NewManager(context.Background(), Options{
		Logger:     testutil.Logger(t),
		Clock:      clock,
		Filesystem: afero.NewOsFs(),
		EnvInfo:    testEnv{home: home},
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
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false"})
	require.Empty(t, m.Catalog())
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	require.Len(t, m.Catalog(), 1)
	require.Empty(t, m.Catalog()[0].Error)
}

func TestDiscoveryEditsAndDirectoryRecreation(t *testing.T) {
	t.Parallel()
	m, configDir := newTestManager(t)
	changed := make(chan struct{}, 16)
	m.SetOnChange(func() { changed <- struct{}{} })
	clock := m.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	startTestDiscovery(ctx, t, m)
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false", "display_name": "Edited"})
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
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false"})
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
	m, _ := newTestManager(t)
	changed := make(chan struct{}, 8)
	m.SetOnChange(func() { changed <- struct{}{} })
	clock := m.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	catalog := m.Catalog()
	startTestDiscovery(ctx, t, m)
	m.RunDiscovery()
	for range 2 {
		advanceTestDiscovery(ctx, t, clock)
		require.Equal(t, catalog, m.Catalog())
		require.Empty(t, changed)
	}
	require.NoError(t, m.Close())
	_, pending := clock.Peek()
	require.False(t, pending, "shutdown must stop the discovery timer")
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
