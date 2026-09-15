package agentmcp

import (
	"encoding/json"
	"io"
	"os"
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

// newTestPluginScope creates a plugin root under a fresh temp dir and
// returns a scope whose DataDir does not exist yet.
func newTestPluginScope(t *testing.T, name string) PluginScope {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, name)
	require.NoError(t, os.Mkdir(root, 0o755))
	canonRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		canonRoot = resolved
	}
	return PluginScope{
		Name:    name,
		Root:    canonRoot,
		DataDir: filepath.Join(base, "data", name),
	}
}

// fakeServerFileName is the file name the test binary is installed
// under inside a plugin root. Windows resolves an extension-less command
// by appending .exe, so the copy carries the suffix there.
var fakeServerFileName = "server" + func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// installFakeServer places the test binary inside the plugin root as
// "server" so a ./-relative command resolves within the root. A hard
// link is used when the filesystems allow it; otherwise the binary is
// copied. Windows always copies: a hard link to the running test
// executable cannot be deleted while the process is alive, which
// breaks TempDir cleanup.
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

// fakePluginServerEntry returns a plugin mcp.json entry that launches
// the fake MCP server installed by installFakeServer.
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

// writePluginConfig writes mcp.json at the plugin root and returns the
// matching ConfigSource.
func writePluginConfig(t *testing.T, scope PluginScope, servers map[string]any) ConfigSource {
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

// findStatus returns the catalog entry with the given name and plugin.
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

func TestReloadSources_LegacyPerEntryIsolation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	_, entry := fakeMCPServerConfig(t, "good")
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	content := `{"mcpServers":{"good":` + string(raw) + `,"bad__name":{"command":"run"},"empty":{}}}`
	path := filepath.Join(dir, ".mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Reload(ctx, []string{path}))

	tools := m.connectedTools()
	require.Len(t, tools, 1, "the valid sibling must connect")
	assert.Equal(t, "good", tools[0].server)

	catalog := m.Catalog()
	require.Len(t, catalog, 3)
	bad := findStatus(t, catalog, "bad__name", "")
	assert.False(t, bad.Connected)
	assert.Contains(t, bad.Err, "reserved separator")
	empty := findStatus(t, catalog, "empty", "")
	assert.False(t, empty.Connected)
	assert.Contains(t, empty.Err, "no command or url")
}

func TestReloadSources_PluginPerEntryIsolation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	scope := newTestPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	src := writePluginConfig(t, scope, map[string]any{
		"good":   fakePluginServerEntry(nil),
		"legacy": map[string]any{"type": "http", "url": "https://example.com/mcp"},
		"escape": map[string]any{"type": "stdio", "command": "/bin/sh"},
		"events": map[string]any{"type": "sse", "url": "https://example.com/sse"},
	})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.ReloadSources(ctx, []ConfigSource{src}))

	tools := m.connectedTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "good", tools[0].server)

	catalog := m.Catalog()
	require.Len(t, catalog, 4)
	good := findStatus(t, catalog, "good", "tools")
	assert.True(t, good.Connected)
	legacy := findStatus(t, catalog, "legacy", "tools")
	assert.False(t, legacy.Connected)
	assert.Contains(t, legacy.Err, `unsupported type "http"`)
	escape := findStatus(t, catalog, "escape", "tools")
	assert.False(t, escape.Connected)
	assert.Contains(t, escape.Err, "bare name or a ./-relative path")
	events := findStatus(t, catalog, "events", "tools")
	assert.False(t, events.Connected)
	assert.Equal(t, "tools", events.PluginName)
	assert.Contains(t, events.Err, "transport sse is deprecated in MCP and not supported for plugins; use streamable-http")
}

func TestReloadSources_InvalidPluginFileDisablesOnlyThatPlugin(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	okScope := newTestPluginScope(t, "ok")
	installFakeServer(t, okScope.Root)
	okSrc := writePluginConfig(t, okScope, map[string]any{"srv": fakePluginServerEntry(nil)})

	badScope := newTestPluginScope(t, "bad")
	badPath := filepath.Join(badScope.Root, "mcp.json")
	require.NoError(t, os.WriteFile(badPath, []byte(`{"mcpServers":{}}`), 0o600))
	badSrc := ConfigSource{Path: badPath, Plugin: &badScope}

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.ReloadSources(ctx, []ConfigSource{badSrc, okSrc}))

	tools := m.connectedTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "srv", tools[0].server)

	// The unusable file is reported once, attributed to its plugin and
	// named after the file, so the plugin's missing servers have a cause.
	catalog := m.Catalog()
	require.Len(t, catalog, 2)
	badFile := findStatus(t, catalog, "mcp.json", "bad")
	assert.False(t, badFile.Connected)
	assert.Contains(t, badFile.Err, "mcp.json not loaded: ")
	assert.Contains(t, badFile.Err, "top level must contain exactly $schema and mcpServers")
}

func TestReloadSources_PluginNameCollision(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	legacyDir := t.TempDir()
	_, entry := fakeMCPServerConfig(t, "srv")
	legacyPath := writeMCPConfig(t, legacyDir, map[string]mcpServerEntry{"srv": entry})

	first := newTestPluginScope(t, "first")
	installFakeServer(t, first.Root)
	firstSrc := writePluginConfig(t, first, map[string]any{
		"srv":   fakePluginServerEntry(nil),
		"other": fakePluginServerEntry(nil),
	})

	second := newTestPluginScope(t, "second")
	installFakeServer(t, second.Root)
	secondSrc := writePluginConfig(t, second, map[string]any{
		"other": fakePluginServerEntry(nil),
	})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.ReloadSources(ctx, []ConfigSource{
		{Path: legacyPath}, firstSrc, secondSrc,
	}))

	tools := m.connectedTools()
	require.Len(t, tools, 2)

	catalog := m.Catalog()
	require.Len(t, catalog, 4)

	legacySrv := findStatus(t, catalog, "srv", "")
	assert.True(t, legacySrv.Connected, "legacy file claims the name first")
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

func TestSnapshotChanged_PluginScope(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newTestPluginScope(t, "tools")
	src := writePluginConfig(t, scope, map[string]any{})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.ReloadSources(ctx, []ConfigSource{src}))
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

	require.NoError(t, m.ReloadSources(ctx, []ConfigSource{{Path: src.Path, Plugin: &renamed}}))
	assert.False(t, m.SnapshotChanged([]ConfigSource{{Path: src.Path, Plugin: &renamed}}))
	assert.True(t, m.SnapshotChanged([]ConfigSource{src}))
}

// TestReloadSources_PluginStdioLaunch launches the fake server through a
// plugin scope and asserts, via the server's env tool, that the process
// sees only the allow-listed environment plus the plugin variables and
// runs in the configured cwd.
func TestReloadSources_PluginStdioLaunch(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	scope := newTestPluginScope(t, "tools")
	installFakeServer(t, scope.Root)
	require.NoError(t, os.Mkdir(filepath.Join(scope.Root, "work"), 0o755))

	entry := fakePluginServerEntry(map[string]string{
		"EXTRA":     "${PLUGIN_ROOT}/x",
		"NOT_A_VAR": "$HOME",
	})
	entry["cwd"] = "./work"
	src := writePluginConfig(t, scope, map[string]any{"srv": entry})

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

	require.NoError(t, m.ReloadSources(ctx, []ConfigSource{src}))
	require.Len(t, m.connectedTools(), 1)
	assert.False(t, updateEnvCalled, "plugin servers must not receive the enriched agent environment")

	info, err := os.Stat(scope.DataDir)
	require.NoError(t, err, "PLUGIN_DATA must be created before launch")
	assert.True(t, info.IsDir())
	if runtime.GOOS != "windows" {
		// Windows does not map directory ACLs onto Unix permission bits.
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
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
	assert.Equal(t, home, env["HOME"], "HOME falls back to the user's home dir")
	assert.Equal(t, os.Getenv("PATH"), env["PATH"])
	assert.Equal(t, "C.UTF-8", env["LC_ALL"])
	assert.Equal(t, scope.Root+"/x", env["EXTRA"])
	assert.Equal(t, "$HOME", env["NOT_A_VAR"], "only plugin placeholders are expanded")
	assert.Equal(t, "1", env["TEST_MCP_FAKE_SERVER"])
	assert.NotContains(t, env, "CODER_AGENT_TOKEN")
	assert.NotContains(t, env, "CODER_AGENT_URL")
	assert.NotContains(t, env, "UNRELATED")
	assert.NotContains(t, env, "FROM_UPDATE_ENV")

	assert.Equal(t, filepath.Join(scope.Root, "work"), report.Cwd)
}
