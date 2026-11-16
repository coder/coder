package agentmcp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// newPluginScope creates a plugin root and a not-yet-existing data dir
// under a symlink-resolved temp dir.
func newPluginScope(t *testing.T, name string) PluginScope {
	t.Helper()
	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	root := filepath.Join(base, name)
	require.NoError(t, os.Mkdir(root, 0o755))
	return PluginScope{
		Name:    name,
		Root:    root,
		DataDir: filepath.Join(base, "data", name),
	}
}

// fakeServerFileName carries .exe on Windows, which resolves an
// extension-less command by appending it.
var fakeServerFileName = "server" + func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// installFakeServer hard-links or copies the test binary into root.
// Windows always copies: a hard link to the running test executable
// cannot be deleted while it runs, which breaks TempDir cleanup.
func installFakeServer(t *testing.T, root string) {
	t.Helper()
	testBin, err := os.Executable()
	require.NoError(t, err)
	dst := filepath.Join(root, fakeServerFileName)
	if runtime.GOOS != "windows" {
		if err := os.Link(testBin, dst); err == nil {
			return
		}
	}
	src, err := os.Open(testBin)
	require.NoError(t, err)
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	require.NoError(t, err)
	_, err = io.Copy(out, src)
	require.NoError(t, err)
	require.NoError(t, out.Close())
}

func fakePluginServerEntry(extraEnv map[string]string) map[string]any {
	env := map[string]string{"TEST_MCP_FAKE_SERVER": "1"}
	for k, v := range extraEnv {
		env[k] = v
	}
	return map[string]any{
		"type":    "stdio",
		"command": "./" + fakeServerFileName,
		"args":    []string{"-test.run=^TestConnectServer_StdioProcessSurvivesConnect$"},
		"env":     env,
	}
}

func writePluginMCP(t *testing.T, scope PluginScope, servers map[string]any) ConfigSource {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"$schema":    pluginMCPSchema,
		"mcpServers": servers,
	})
	require.NoError(t, err)
	path := filepath.Join(scope.Root, "mcp.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return ConfigSource{Path: path, Plugin: &scope}
}

// umaskKeepsGroupOrOther reports whether a directory created 0755 keeps
// a group or other bit under the process umask, so a 0700 result shows
// the requested mode rather than the umask.
func umaskKeepsGroupOrOther(t *testing.T) bool {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe")
	require.NoError(t, os.Mkdir(probe, 0o755))
	info, err := os.Stat(probe)
	require.NoError(t, err)
	return info.Mode().Perm()&0o077 != 0
}

func findStatus(t *testing.T, catalog []ServerStatus, name, plugin string) ServerStatus {
	t.Helper()
	for _, st := range catalog {
		if st.Name == name && st.PluginName == plugin {
			return st
		}
	}
	require.Failf(t, "catalog entry missing", "no entry for %q (plugin %q) in %+v", name, plugin, catalog)
	return ServerStatus{}
}

func TestReloadCurrent_LegacyInvalidEntrySkipsFile(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	_, entry := fakeMCPServerConfig(t, "good")
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	content := `{"mcpServers":{"good":` + string(raw) + `,"empty":{}}}`
	path := filepath.Join(dir, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, LegacySources([]string{path})))
	assert.Empty(t, m.connectedTools())
	assert.Empty(t, m.Catalog())
}

// TestReloadCurrent_LegacyDuplicateNames: a legacy file with an invalid entry
// is skipped whole, so it claims no name.
func TestReloadCurrent_LegacyDuplicateNames(t *testing.T) {
	t.Parallel()

	marked := func(file string) mcpServerEntry {
		_, entry := fakeMCPServerConfig(t, "dup")
		entry.Env["FILE"] = file
		return entry
	}
	invalid := mcpServerEntry{}

	for _, tc := range []struct {
		name          string
		first, second mcpServerEntry
		wantFile      string
	}{
		{name: "BothValid", first: marked("first"), second: marked("second"), wantFile: "first"},
		{name: "LaterInvalid", first: marked("first"), second: invalid, wantFile: "first"},
		{name: "EarlierInvalid", first: invalid, second: marked("second"), wantFile: "second"},
		{name: "BothInvalid", first: invalid, second: invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

			first := writeMCPConfig(t, t.TempDir(), map[string]mcpServerEntry{"dup": tc.first})
			second := writeMCPConfig(t, t.TempDir(), map[string]mcpServerEntry{"dup": tc.second})

			m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
			t.Cleanup(func() { _ = m.Close() })
			require.NoError(t, reloadWith(ctx, m, LegacySources([]string{first, second})))

			catalog := m.Catalog()
			if tc.wantFile == "" {
				assert.Empty(t, catalog)
				return
			}
			require.Len(t, catalog, 1, "catalog: %+v", catalog)
			assert.Equal(t, "dup", catalog[0].Name)
			assert.True(t, catalog[0].Connected, "err: %s", catalog[0].Err)
			m.mu.RLock()
			got := m.servers["dup"].config.Env["FILE"]
			m.mu.RUnlock()
			assert.Equal(t, tc.wantFile, got)
		})
	}
}

func TestReloadCurrent_PluginPerEntryIsolation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	src := writePluginMCP(t, scope, map[string]any{
		"good":     fakePluginServerEntry(nil),
		"httpType": map[string]any{"type": "http", "url": "https://example.com/mcp"},
		"escape":   map[string]any{"type": "stdio", "command": "/bin/sh"},
	})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))

	tools := m.connectedTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "good", tools[0].server)

	catalog := m.Catalog()
	require.Len(t, catalog, 3)
	good := findStatus(t, catalog, "good", "tools")
	assert.True(t, good.Connected)
	httpType := findStatus(t, catalog, "httpType", "tools")
	assert.False(t, httpType.Connected)
	assert.Contains(t, httpType.Err, `unsupported type "http"`)
	escape := findStatus(t, catalog, "escape", "tools")
	assert.False(t, escape.Connected)
	assert.Contains(t, escape.Err, "bare name or a ./-relative path")
}

func TestReloadCurrent_InvalidPluginFileDisablesOnlyThatPlugin(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	okScope := newPluginScope(t, "ok")
	installFakeServer(t, okScope.Root)
	okSrc := writePluginMCP(t, okScope, map[string]any{"srv": fakePluginServerEntry(nil)})

	badScope := newPluginScope(t, "bad")
	badPath := filepath.Join(badScope.Root, "mcp.json")
	require.NoError(t, os.WriteFile(badPath, []byte(`{"mcpServers":{}}`), 0o600))
	badSrc := ConfigSource{Path: badPath, Plugin: &badScope}

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{badSrc, okSrc}))

	tools := m.connectedTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "srv", tools[0].server)

	catalog := m.Catalog()
	require.Len(t, catalog, 2)
	badFile := findStatus(t, catalog, "mcp.json", "bad")
	assert.False(t, badFile.Connected)
	assert.Contains(t, badFile.Err, "mcp.json not loaded: ")
	assert.Contains(t, badFile.Err, "top level must contain exactly $schema and mcpServers")
}

func TestReloadCurrent_PluginLoadErrorReportedForExistingMCPConfig(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	withConfig := newPluginScope(t, "with-config")
	withConfigSrc := writePluginMCP(t, withConfig, map[string]any{"srv": fakePluginServerEntry(nil)})
	withConfigSrc.Plugin.DataDir = ""
	withConfigSrc.LoadError = "home directory unavailable"

	without := newPluginScope(t, "without-config")
	withoutSrc := ConfigSource{
		Path:      filepath.Join(without.Root, "mcp.json"),
		Plugin:    &PluginScope{Name: without.Name, Root: without.Root},
		LoadError: "home directory unavailable",
	}

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{withConfigSrc, withoutSrc}))

	require.Empty(t, m.connectedTools())
	catalog := m.Catalog()
	require.Len(t, catalog, 1, "a plugin without mcp.json reports nothing")
	st := findStatus(t, catalog, "mcp.json", "with-config")
	assert.False(t, st.Connected)
	assert.Equal(t, "mcp.json not loaded: home directory unavailable", st.Err)
	assert.False(t, m.SnapshotChanged([]ConfigSource{withConfigSrc, withoutSrc}))

	otherErr := withConfigSrc
	otherErr.LoadError = "home directory moved"
	assert.True(t, m.SnapshotChanged([]ConfigSource{otherErr, withoutSrc}), "a LoadError change must reload")
}

func TestReloadCurrent_PluginNameCollision(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	legacyDir := t.TempDir()
	_, entry := fakeMCPServerConfig(t, "srv")
	legacyPath := writeMCPConfig(t, legacyDir, map[string]mcpServerEntry{"srv": entry})

	first := newPluginScope(t, "first")
	installFakeServer(t, first.Root)
	firstSrc := writePluginMCP(t, first, map[string]any{
		"srv":   fakePluginServerEntry(nil),
		"other": fakePluginServerEntry(nil),
	})

	second := newPluginScope(t, "second")
	installFakeServer(t, second.Root)
	secondSrc := writePluginMCP(t, second, map[string]any{
		"other": fakePluginServerEntry(nil),
	})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	// The legacy source is listed last and still claims its name first.
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{
		firstSrc, secondSrc, {Path: legacyPath},
	}))

	tools := m.connectedTools()
	require.Len(t, tools, 2)

	catalog := m.Catalog()
	require.Len(t, catalog, 4)

	legacySrv := findStatus(t, catalog, "srv", "")
	assert.True(t, legacySrv.Connected, "legacy file claims the name")
	pluginSrv := findStatus(t, catalog, "srv", "first")
	assert.False(t, pluginSrv.Connected)
	assert.Equal(t, "server name already in use by "+legacyPath, pluginSrv.Err)

	firstOther := findStatus(t, catalog, "other", "first")
	assert.True(t, firstOther.Connected, "earlier plugin claims the name")
	secondOther := findStatus(t, catalog, "other", "second")
	assert.False(t, secondOther.Connected)
	assert.Equal(t, "server name already in use by plugin first", secondOther.Err)

	m.mu.RLock()
	srvCfg := m.servers["srv"].config
	otherCfg := m.servers["other"].config
	m.mu.RUnlock()
	assert.Nil(t, srvCfg.Plugin, "the connected srv is the legacy one")
	require.NotNil(t, otherCfg.Plugin)
	assert.Equal(t, "first", otherCfg.Plugin.Name)
}

// TestReloadCurrent_FailedConnectClosesClientFromOtherScope covers a
// server whose plugin scope changes while the connect under the new
// scope fails.
func TestReloadCurrent_FailedConnectClosesClientFromOtherScope(t *testing.T) {
	t.Parallel()

	brokenEntry := map[string]any{"type": "stdio", "command": "coder-test-nonexistent-mcp-server"}
	workingPlugin := func(t *testing.T) ConfigSource {
		scope := newPluginScope(t, "a")
		installFakeServer(t, scope.Root)
		return writePluginMCP(t, scope, map[string]any{"x": fakePluginServerEntry(nil)})
	}
	workingLegacy := func(t *testing.T) ConfigSource {
		_, entry := fakeMCPServerConfig(t, "x")
		return ConfigSource{Path: writeMCPConfig(t, t.TempDir(), map[string]mcpServerEntry{"x": entry})}
	}
	brokenPlugin := func(t *testing.T, _ ConfigSource) ConfigSource {
		return writePluginMCP(t, newPluginScope(t, "b"), map[string]any{"x": brokenEntry})
	}

	for _, tc := range []struct {
		name     string
		previous func(t *testing.T) ConfigSource
		// next returns the source that takes over x and fails to
		// connect.
		next       func(t *testing.T, prev ConfigSource) ConfigSource
		nextPlugin string
	}{
		{name: "FromOtherPlugin", previous: workingPlugin, next: brokenPlugin, nextPlugin: "b"},
		{name: "FromLegacy", previous: workingLegacy, next: brokenPlugin, nextPlugin: "b"},
		{
			name: "ToLegacy", previous: workingPlugin,
			next: func(t *testing.T, _ ConfigSource) ConfigSource {
				dir := t.TempDir()
				return ConfigSource{Path: writeMCPConfig(t, dir, map[string]mcpServerEntry{
					"x": {Command: "coder-test-nonexistent-mcp-server"},
				})}
			},
		},
		{
			// A DataDir under a regular file cannot be created, so the
			// launch fails.
			name: "SamePluginNewDataDir", previous: workingPlugin, nextPlugin: "a",
			next: func(t *testing.T, prev ConfigSource) ConfigSource {
				file := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(file, nil, 0o600))
				scope := *prev.Plugin
				scope.DataDir = filepath.Join(file, "data")
				return ConfigSource{Path: prev.Path, Plugin: &scope}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

			prevSrc := tc.previous(t)
			m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
			t.Cleanup(func() { _ = m.Close() })

			require.NoError(t, reloadWith(ctx, m, []ConfigSource{prevSrc}))
			require.Len(t, m.connectedTools(), 1)
			m.mu.RLock()
			oldClient := m.servers["x"].client
			m.mu.RUnlock()

			require.NoError(t, reloadWith(ctx, m, []ConfigSource{tc.next(t, prevSrc)}))

			m.mu.RLock()
			_, ok := m.servers["x"]
			m.mu.RUnlock()
			assert.False(t, ok, "the client from the previous scope must not stay installed")
			assert.Empty(t, m.connectedTools())

			st := findStatus(t, m.Catalog(), "x", tc.nextPlugin)
			assert.False(t, st.Connected)
			assert.Equal(t, "failed to connect", st.Err)

			listCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
			defer cancel()
			_, err := oldClient.ListTools(listCtx, nil)
			assert.Error(t, err, "the previous session must be closed")
		})
	}
}

func TestReloadCurrent_PluginReorderChangesCollisionWinner(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	first := newPluginScope(t, "first")
	installFakeServer(t, first.Root)
	firstSrc := writePluginMCP(t, first, map[string]any{"other": fakePluginServerEntry(nil)})
	second := newPluginScope(t, "second")
	installFakeServer(t, second.Root)
	secondSrc := writePluginMCP(t, second, map[string]any{"other": fakePluginServerEntry(nil)})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{firstSrc, secondSrc}))
	reordered := []ConfigSource{secondSrc, firstSrc}
	require.True(t, m.SnapshotChanged(reordered))
	require.NoError(t, reloadWith(ctx, m, reordered))

	assert.True(t, findStatus(t, m.Catalog(), "other", "second").Connected)
	m.mu.RLock()
	otherCfg := m.servers["other"].config
	m.mu.RUnlock()
	require.NotNil(t, otherCfg.Plugin)
	assert.Equal(t, "second", otherCfg.Plugin.Name)
}

func TestReloadCurrent_LegacyNameMatchesPluginServerID(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	_, entry := fakeMCPServerConfig(t, "tools/srv")
	legacyPath := writeMCPConfig(t, t.TempDir(), map[string]mcpServerEntry{"tools/srv": entry})

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	src := writePluginMCP(t, scope, map[string]any{
		"srv":  fakePluginServerEntry(nil),
		"keep": fakePluginServerEntry(nil),
	})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src, {Path: legacyPath}}))

	catalog := m.Catalog()
	require.Len(t, catalog, 3)
	assert.True(t, findStatus(t, catalog, "tools/srv", "").Connected)
	assert.True(t, findStatus(t, catalog, "keep", "tools").Connected)
	pluginSrv := findStatus(t, catalog, "srv", "tools")
	assert.False(t, pluginSrv.Connected)
	assert.Equal(t, `server ID "tools/srv" is already used by a legacy server in `+legacyPath, pluginSrv.Err)

	m.mu.RLock()
	_, started := m.servers["srv"]
	m.mu.RUnlock()
	assert.False(t, started, "the plugin server must not run")
}

func TestSnapshotChanged_PluginScope(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	src := writePluginMCP(t, scope, map[string]any{})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	assert.False(t, m.SnapshotChanged([]ConfigSource{src}), "same path and scope")

	sameValues := scope
	assert.False(t, m.SnapshotChanged([]ConfigSource{{Path: src.Path, Plugin: &sameValues}}),
		"a distinct pointer to equal scope values is unchanged")

	renamed := scope
	renamed.Name = "renamed"
	assert.True(t, m.SnapshotChanged([]ConfigSource{{Path: src.Path, Plugin: &renamed}}),
		"plugin rename with an untouched file must reload")

	movedData := scope
	movedData.DataDir = filepath.Join(t.TempDir(), "elsewhere")
	assert.True(t, m.SnapshotChanged([]ConfigSource{{Path: src.Path, Plugin: &movedData}}),
		"data dir change must reload")

	assert.True(t, m.SnapshotChanged([]ConfigSource{{Path: src.Path}}),
		"the same file read as a legacy source must reload")

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{{Path: src.Path, Plugin: &renamed}}))
	assert.False(t, m.SnapshotChanged([]ConfigSource{{Path: src.Path, Plugin: &renamed}}))
	assert.True(t, m.SnapshotChanged([]ConfigSource{src}))

	// One path under two scopes: the first source wins everywhere.
	twice := []ConfigSource{src, {Path: src.Path, Plugin: &renamed}}
	require.NoError(t, reloadWith(ctx, m, twice))
	assert.False(t, m.SnapshotChanged(twice))

	inPlace := scope
	inPlace.Name = "in-place"
	inPlaceSrc := ConfigSource{Path: src.Path, Plugin: &inPlace}
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{inPlaceSrc}))
	inPlace.Name = "edited"
	assert.True(t, m.SnapshotChanged([]ConfigSource{inPlaceSrc}),
		"a scope edited in place after the reload must reload")
}

func TestReloadCurrent_CatalogOrdersSharedNameByPlugin(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	zeta := newPluginScope(t, "zeta")
	installFakeServer(t, zeta.Root)
	zetaSrc := writePluginMCP(t, zeta, map[string]any{"other": fakePluginServerEntry(nil)})
	alpha := newPluginScope(t, "alpha")
	installFakeServer(t, alpha.Root)
	alphaSrc := writePluginMCP(t, alpha, map[string]any{"other": fakePluginServerEntry(nil)})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{zetaSrc, alphaSrc}))

	var plugins []string
	for _, st := range m.Catalog() {
		plugins = append(plugins, st.PluginName)
	}
	assert.Equal(t, []string{"alpha", "zeta"}, plugins)
	assert.True(t, findStatus(t, m.Catalog(), "other", "zeta").Connected, "the plugin listed first claims the name")
}

// callEnvTool calls the fake server's env tool on the named server.
func callEnvTool(ctx context.Context, t *testing.T, m *Manager, server string) (env map[string]string, cwd string) {
	t.Helper()
	resp, err := m.CallTool(ctx, workspacesdk.CallMCPToolRequest{ToolName: server + ToolNameSep + "env"})
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	var report fakeEnvReport
	require.NoError(t, json.Unmarshal([]byte(resp.Content[0].Text), &report))
	env = make(map[string]string, len(report.Env))
	for _, kv := range report.Env {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	return env, report.Cwd
}

// TestReloadCurrent_PluginChangesReconnect changes the scope with
// mcp.json untouched, then only the entry's cwd, and checks each change
// reaches a new process.
func TestReloadCurrent_PluginChangesReconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	require.NoError(t, os.Mkdir(filepath.Join(scope.Root, "a"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(scope.Root, "bb"), 0o755))
	entry := fakePluginServerEntry(nil)
	entry["cwd"] = "./a"
	src := writePluginMCP(t, scope, map[string]any{"srv": entry})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	env, cwd := callEnvTool(ctx, t, m, "srv")
	assert.Equal(t, scope.DataDir, env["PLUGIN_DATA"])
	assert.Equal(t, filepath.Join(scope.Root, "a"), cwd)

	moved := scope
	moved.DataDir = filepath.Join(filepath.Dir(scope.DataDir), "moved")
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{{Path: src.Path, Plugin: &moved}}))
	env, _ = callEnvTool(ctx, t, m, "srv")
	assert.Equal(t, moved.DataDir, env["PLUGIN_DATA"], "a scope change must reconnect")

	entry["cwd"] = "./bb"
	movedSrc := writePluginMCP(t, moved, map[string]any{"srv": entry})
	require.NoError(t, reloadWith(ctx, m, []ConfigSource{movedSrc}))
	_, cwd = callEnvTool(ctx, t, m, "srv")
	assert.Equal(t, filepath.Join(scope.Root, "bb"), cwd, "a cwd change must reconnect")
}

func TestWatcher_PluginRewriteKeepsScope(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	src := writePluginMCP(t, scope, map[string]any{"srv": fakePluginServerEntry(nil)})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	useFastDebounce(t, m)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	writePluginMCP(t, scope, map[string]any{"srv2": fakePluginServerEntry(nil)})

	awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 1 && tools[0].server == "srv2"
	})
	m.mu.RLock()
	plugin := m.servers["srv2"].config.Plugin
	m.mu.RUnlock()
	require.NotNil(t, plugin, "a watched reload must keep the plugin scope")
	assert.Equal(t, scope, *plugin)
}

// TestReloadCurrent_PluginStdioLaunch inspects the launched process's
// env and cwd through the fake server's env tool.
func TestReloadCurrent_PluginStdioLaunch(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	require.NoError(t, os.Mkdir(filepath.Join(scope.Root, "work"), 0o755))

	entry := fakePluginServerEntry(map[string]string{
		"EXTRA":     "${PLUGIN_ROOT}/x",
		"NOT_A_VAR": "$HOME",
	})
	entry["cwd"] = "./work"
	src := writePluginMCP(t, scope, map[string]any{"srv": entry})

	home := t.TempDir()
	envInfo := &fakeEnvInfo{
		environ: []string{
			"PATH=" + os.Getenv("PATH"),
			"CODER_AGENT_TOKEN=secret",
			"CODER_AGENT_URL=https://coder.example.com",
			"LC_ALL=C.UTF-8",
			"UNRELATED=1",
		},
		home: home,
	}
	updateEnvCalled := false
	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, envInfo,
		func(env []string) ([]string, error) {
			updateEnvCalled = true
			return append(env, "FROM_UPDATE_ENV=1"), nil
		}, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	require.Len(t, m.connectedTools(), 1)
	assert.True(t, updateEnvCalled, "plugin servers receive the enriched agent environment")

	info, err := os.Stat(scope.DataDir)
	require.NoError(t, err, "PLUGIN_DATA must be created before launch")
	assert.True(t, info.IsDir())
	// Windows does not map directory ACLs onto Unix permission bits.
	if runtime.GOOS != "windows" {
		if umaskKeepsGroupOrOther(t) {
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		} else {
			t.Log("umask clears group and other bits, so the 0700 mode is not checked")
		}
	}

	resp, err := m.CallTool(ctx, workspacesdk.CallMCPToolRequest{ToolName: "srv" + ToolNameSep + "env"})
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	var report fakeEnvReport
	require.NoError(t, json.Unmarshal([]byte(resp.Content[0].Text), &report))

	env := make(map[string]string, len(report.Env))
	for _, kv := range report.Env {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	assert.Equal(t, scope.Root, env["PLUGIN_ROOT"])
	assert.Equal(t, scope.DataDir, env["PLUGIN_DATA"])
	assert.Equal(t, os.Getenv("PATH"), env["PATH"])
	assert.Equal(t, "C.UTF-8", env["LC_ALL"])
	assert.Equal(t, scope.Root+"/x", env["EXTRA"])
	assert.Equal(t, "$HOME", env["NOT_A_VAR"], "only plugin placeholders are expanded")
	assert.Equal(t, "1", env["TEST_MCP_FAKE_SERVER"])
	assert.Equal(t, "secret", env["CODER_AGENT_TOKEN"])
	assert.Equal(t, "1", env["UNRELATED"])
	assert.Equal(t, "1", env["FROM_UPDATE_ENV"])

	assert.Equal(t, filepath.Join(scope.Root, "work"), report.Cwd)
}

func TestReloadCurrent_PluginWithoutMCPConfig(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{
		{Path: filepath.Join(scope.Root, "mcp.json"), Plugin: &scope},
	}))
	assert.Empty(t, m.Catalog())
}

func TestReloadCurrent_RejectedEntryOmitsURL(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	src := writePluginMCP(t, scope, map[string]any{
		"leak": map[string]any{"type": "streamable-http", "url": "https://alice:s3cret@example.com/mcp?token=abc"},
	})
	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	leak := findStatus(t, m.Catalog(), "leak", "tools")
	assert.Equal(t, `server "leak": url must not contain userinfo`, leak.Err)
}

func TestReloadCurrent_PluginDataSubdirCwd(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	entry := fakePluginServerEntry(nil)
	entry["cwd"] = "${PLUGIN_DATA}/cache"
	src := writePluginMCP(t, scope, map[string]any{"srv": entry})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	srv := findStatus(t, m.Catalog(), "srv", "tools")
	require.True(t, srv.Connected, "err: %s", srv.Err)
	info, err := os.Stat(filepath.Join(scope.DataDir, "cache"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestReloadCurrent_PluginDataDirCreateFails(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	scope := newPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	scope.DataDir = filepath.Join(blocker, "data")
	src := writePluginMCP(t, scope, map[string]any{"srv": fakePluginServerEntry(nil)})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, reloadWith(ctx, m, []ConfigSource{src}))
	srv := findStatus(t, m.Catalog(), "srv", "tools")
	assert.False(t, srv.Connected)
	assert.Equal(t, "failed to connect", srv.Err)
}

func TestHasLinkInside(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "link")))
	canonReal, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)

	canonDir := filepath.Dir(canonReal)

	assert.False(t, hasLinkInside(canonDir, filepath.Join(canonReal, "sub")))
	assert.True(t, hasLinkInside(canonDir, filepath.Join(canonDir, "link", "sub")), "symlinked ancestor below base")
	assert.True(t, hasLinkInside(canonDir, filepath.Join(canonReal, "missing")), "unreadable component")
	assert.True(t, hasLinkInside(filepath.Join(canonDir, "link"), filepath.Join(canonDir, "link")), "base that is itself a link")
	assert.True(t, hasLinkInside("/", filepath.Join(canonDir, "link", "sub")), "symlinked ancestor below a root base")
	linkedSub := filepath.Join(canonDir, "link", "sub")
	assert.False(t, hasLinkInside(linkedSub, linkedSub), "links above base are trusted")

	assert.NoError(t, requireInside("/", filepath.Join(canonReal, "sub")), "a root base contains every path")
	assert.True(t, pathHasPrefix(canonReal, "/"))
	assert.False(t, pathHasPrefix(canonReal+"-sibling", canonReal))
}

func TestHasLinkInside_Junction(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("junctions exist only on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
	link := filepath.Join(dir, "link")
	// Creating a junction needs no elevation, unlike a symlink.
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	require.NoError(t, err, string(out))

	assert.False(t, hasLinkInside(dir, filepath.Join(target, "sub")))
	assert.True(t, hasLinkInside(dir, filepath.Join(link, "sub")), "junction below base")
	assert.True(t, hasLinkInside(filepath.VolumeName(dir)+`\`, filepath.Join(link, "sub")), "junction below a drive root base")
}
