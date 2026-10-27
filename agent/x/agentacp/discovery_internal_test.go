package agentacp

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDiscoveryCatalog(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	m, dir, configDir := newTestManager(t, fakeHarnessModeBoth)
	catalog := m.Catalog()
	require.Equal(t, "fake", catalog[0].DisplayName)
	require.True(t, catalog[0].LoadSession)
	require.True(t, catalog[0].ResumeSession)
	require.Equal(t, "default", catalog[0].ConfigOptions[0].CurrentValue)
	require.Len(t, catalog[0].ConfigOptions, 2)
	require.Equal(t, workspacesdk.ACPConfigOption{
		ID: "mode", Name: "Mode", Description: "Choose a mode", CurrentValue: "code", Category: "mode",
		Values: []workspacesdk.ACPConfigValue{
			{ID: "code", Name: "Code", Description: "Edit files", Group: "write", GroupName: "Write"},
			{ID: "ask", Name: "Ask", Group: "read", GroupName: "Read"},
		},
	}, catalog[0].ConfigOptions[1])
	require.Equal(t, configDir, m.ContextResources()[0].SourcePath)
	hash := m.ContextResources()[0].ContentHash
	require.NoError(t, m.Reload(ctx))
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
	require.NoError(t, m.Reload(ctx))
	catalog = m.Catalog()
	require.Len(t, catalog, 4)
	for _, h := range catalog {
		if h.Slug != "fake" {
			require.NotEmpty(t, h.Error)
		}
	}
	info := createTestSession(t, m, dir)
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false", "display_name": "Changed"})
	require.NoError(t, m.Reload(ctx))
	var found bool
	for _, resource := range m.ContextResources() {
		if resource.Source == "fake" {
			found = true
			require.NotEqual(t, hash, resource.ContentHash)
			require.Equal(t, "Changed", resource.ACPHarness.DisplayName)
			require.NotEmpty(t, resource.Error)
		}
	}
	require.True(t, found, "the edited harness must remain in the context snapshot")
	session, err := m.Read(ctx, info.ID, nil)
	require.NoError(t, err)
	require.Equal(t, info.ID, session.Session.ID)
	require.Equal(t, "fake", session.Session.HarnessDisplayName)
	require.NoError(t, os.Remove(filepath.Join(configDir, "fake.json")))
	require.NoError(t, m.Reload(ctx))
	require.Len(t, m.Catalog(), 3)
	// Discovery never falls back when the manifest directory is invalid.
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth)})
	for _, tc := range []struct {
		directory string
		wantError string
	}{
		{directory: "", wantError: "workspace agent directory is unset"},
		{directory: "relative", wantError: "workspace agent directory must be an absolute path"},
		{directory: filepath.Join(configDir, "nonexistent"), wantError: "workspace agent directory is missing or is not a directory"},
		{directory: filepath.Join(configDir, "fake.json"), wantError: "workspace agent directory is missing or is not a directory"},
	} {
		m.workingDir = func() string { return tc.directory }
		require.NoError(t, m.Reload(ctx))
		found := false
		for _, h := range m.Catalog() {
			if h.Slug == "fake" {
				found = true
				require.Equal(t, tc.wantError, h.Error)
			}
		}
		require.True(t, found, "the invalid directory must be reported in the catalog")
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
	// Enabled configuration is still required for a new runtime.
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, fakeHarnessModeBoth), "enabled": false})
	advanceTestDiscovery(ctx, t, clock)
	testutil.RequireReceive(ctx, t, changed)
	_, err := m.Create(ctx, workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir})
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestProbeDeadline(t *testing.T) {
	t.Parallel()
	m, _, configDir := newTestManager(t, fakeHarnessModeBoth)
	ctx := testutil.Context(t, testutil.WaitLong)
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	require.NoError(t, listener.SetDeadline(deadline))
	command := "CODER_ACP_TEST_READY_ADDR=" + shellQuote(listener.Addr().String()) + " " + fakeCommand(t, fakeHarnessModeHang)
	writeConfig(t, filepath.Join(configDir, "hang.json"), map[string]any{"command": command})
	done := make(chan error, 1)
	go func() { done <- m.Reload(ctx) }()
	ready, err := listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ready.Close() })
	require.NoError(t, ready.SetDeadline(deadline))
	var signal [1]byte
	_, err = io.ReadFull(ready, signal[:])
	require.NoError(t, err, "the helper must enter Initialize before the deadline advances")
	require.Equal(t, byte(1), signal[0])
	select {
	case err := <-done:
		t.Fatalf("probe completed before its deadline: %v", err)
	default:
	}
	m.clock.(*quartz.Mock).Advance(setupTimeout).MustWait(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
	_, err = ready.Read(signal[:])
	// Windows may report a connection reset instead of EOF when the helper exits.
	require.Error(t, err, "the helper must exit after the probe deadline")
	require.NotErrorIs(t, err, os.ErrDeadlineExceeded, "the helper must close the connection before the read deadline")
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

func TestDiscoveryEnvironmentChanges(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	changed := make(chan struct{}, 8)
	m.SetOnChange(func() { changed <- struct{}{} })
	clock := m.clock.(*quartz.Mock)
	ctx := testutil.Context(t, testutil.WaitLong)
	var (
		extraEnv      []string
		envErr        error
		callbackCalls int
	)
	m.updateEnv = func(current []string) ([]string, error) {
		callbackCalls++
		if envErr != nil {
			return nil, envErr
		}
		return append(current, extraEnv...), nil
	}
	startTestDiscovery(ctx, t, m)
	require.Equal(t, 1, callbackCalls)

	extraEnv = []string{"CODER_ACP_TEST_MODEL=other", "CODER_ACP_TEST_UNUSED=value"}
	advanceTestDiscovery(ctx, t, clock)
	require.Equal(t, "other", m.Catalog()[0].ConfigOptions[0].CurrentValue)
	testutil.RequireReceive(ctx, t, changed)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	// Reordering the same environment must reuse the successful probe.
	extraEnv = []string{"CODER_ACP_TEST_UNUSED=value", "CODER_ACP_TEST_MODEL=other"}
	advanceTestDiscovery(ctx, t, clock)
	require.Equal(t, 3, callbackCalls)
	require.Empty(t, changed)
	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	envErr = xerrors.New("environment unavailable")
	require.ErrorIs(t, m.Reload(ctx), envErr)
	require.Equal(t, "other", m.Catalog()[0].ConfigOptions[0].CurrentValue)
	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	envErr = nil
	extraEnv = nil
	advanceTestDiscovery(ctx, t, clock)
	require.Equal(t, 5, callbackCalls)
	testutil.RequireReceive(ctx, t, changed)
	require.Equal(t, "default", m.Catalog()[0].ConfigOptions[0].CurrentValue)
	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 3)
}

func TestDiscoveryEnvironmentChangesDuringProbe(t *testing.T) {
	t.Parallel()
	m, dir, configDir := newTestManager(t, fakeHarnessModeBoth)
	ctx := testutil.Context(t, testutil.WaitLong)
	env := append(m.envInfo.Environ(), "CODER_ACP_TEST_MODEL=other")
	var callbackCalls int
	m.updateEnv = func([]string) ([]string, error) {
		callbackCalls++
		return env, nil
	}
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{
		"command": fakeCommand(t, fakeHarnessModeBoth), "display_name": "Edited",
	})
	trap := m.clock.(*quartz.Mock).Trap().AfterFunc("acp-timeout")
	defer func() {
		if trap != nil {
			trap.Close()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- m.Reload(ctx) }()
	call := trap.MustWait(ctx)
	// A callback may reuse its output buffer on a later invocation.
	env[len(env)-1] = "CODER_ACP_TEST_MODEL=default"
	call.MustRelease(ctx)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
	require.Equal(t, 1, callbackCalls)
	require.Equal(t, "other", m.Catalog()[0].ConfigOptions[0].CurrentValue)
	trap.Close()
	trap = nil

	// The in-flight probe must not cache its result for the newer environment.
	require.NoError(t, m.Reload(ctx))
	require.Equal(t, 2, callbackCalls)
	require.Equal(t, "default", m.Catalog()[0].ConfigOptions[0].CurrentValue)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 3)
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
