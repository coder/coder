package agentmcp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// newReportTestManager builds a manager whose onChange callback counts
// report notifications, so tests can assert both the published report
// and that consumers were told about it.
func newReportTestManager(t *testing.T) (*Manager, func() int) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })
	var (
		mu    sync.Mutex
		fires int
	)
	m.SetOnReload(func() {
		mu.Lock()
		fires++
		mu.Unlock()
	})
	return m, func() int {
		mu.Lock()
		defer mu.Unlock()
		return fires
	}
}

// newIncrementalReportTestManager builds a manager on a mock clock whose
// onChange callback captures each published report, so tests can observe
// the interim report a hung sibling would otherwise hold back and expire
// the hung handshake without waiting out connectTimeout.
func newIncrementalReportTestManager(t *testing.T) (*Manager, chan Report, *quartz.Mock) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() { _ = m.Close() })
	clock := quartz.NewMock(t)
	m.clock = clock
	reports := make(chan Report, 16)
	m.SetOnReload(func() { reports <- m.Report() })
	return m, reports, clock
}

func serverByName(t *testing.T, r Report, name string) ServerStatus {
	t.Helper()
	for _, s := range r.Servers {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("server %q missing from report %+v", name, r)
	return ServerStatus{}
}

func TestReport(t *testing.T) {
	t.Parallel()

	t.Run("ZeroServers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		configPath := writeMCPConfig(t, dir, nil)
		m, fires := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		assert.Empty(t, got.Servers)
		assert.Empty(t, got.ConfigErrors)
		assert.Equal(t, 0, fires(), "an unchanged empty report does not notify")
	})

	t.Run("MissingFileIsNotAnError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		m, _ := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{filepath.Join(t.TempDir(), ".mcp.json")}))
		assert.Empty(t, m.Report().ConfigErrors)
	})

	t.Run("InvalidJSONAttributedToFile", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		configPath := filepath.Join(t.TempDir(), ".mcp.json")
		require.NoError(t, os.WriteFile(configPath, []byte("{not json"), 0o600))
		m, fires := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		assert.Empty(t, got.Servers)
		require.Len(t, got.ConfigErrors, 1)
		assert.Equal(t, configPath, got.ConfigErrors[0].Path)
		assert.Contains(t, got.ConfigErrors[0].Err, "parse mcp config")
		assert.Equal(t, 1, fires(), "a config error with an unchanged catalog still notifies")
	})

	t.Run("SemanticErrorAttributedToFile", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		configPath := filepath.Join(t.TempDir(), ".mcp.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{"mcpServers":{"nothing":{"args":["x"]}}}`), 0o600))
		m, _ := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		assert.Empty(t, got.Servers, "a rejected file declares no servers")
		require.Len(t, got.ConfigErrors, 1)
		assert.Equal(t, configPath, got.ConfigErrors[0].Path)
		assert.Contains(t, got.ConfigErrors[0].Err, "has no command or url")
	})

	t.Run("ConfigErrorIsBounded", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		configPath := filepath.Join(t.TempDir(), ".mcp.json")
		// The engine echoes the offending server name in its error.
		longName := strings.Repeat("n", 2*maxDiagnosticBytes)
		require.NoError(t, os.WriteFile(configPath, []byte(`{"mcpServers":{"`+longName+`":{"args":["x"]}}}`), 0o600))
		m, _ := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		require.Len(t, got.ConfigErrors, 1)
		assert.LessOrEqual(t, len(got.ConfigErrors[0].Err), maxDiagnosticBytes)
	})

	t.Run("ZeroToolServerStaysConnected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, entry := fakeMCPServerConfig(t, "empty")
		entry.Env["TEST_MCP_FAKE_SERVER_TOOLS"] = "none"
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"empty": entry})
		m, fires := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := serverByName(t, m.Report(), "empty")
		assert.True(t, got.Connected)
		assert.Empty(t, got.Tools)
		assert.Empty(t, got.Err)
		assert.Equal(t, 1, fires())
	})

	t.Run("PartialFailureCarriesConnectError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, good := fakeMCPServerConfig(t, "good")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{
			"good": good,
			"bad":  {Command: filepath.Join(dir, "missing-binary")},
		})
		m, _ := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		require.Len(t, got.Servers, 2)
		assert.True(t, serverByName(t, got, "good").Connected)
		bad := serverByName(t, got, "bad")
		assert.False(t, bad.Connected)
		assert.NotEqual(t, "failed to connect", bad.Err, "the placeholder must not replace the real error")
		assert.Contains(t, bad.Err, "missing-binary")
	})

	t.Run("TotalFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{
			"a": {Command: filepath.Join(dir, "missing-a")},
			"b": {Command: filepath.Join(dir, "missing-b")},
		})
		m, fires := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		require.Len(t, got.Servers, 2)
		for _, s := range got.Servers {
			assert.False(t, s.Connected)
			assert.NotEmpty(t, s.Err)
		}
		assert.Equal(t, 1, fires())
	})

	t.Run("RetainedSessionErrorRedactedWithItsOwnConfig", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, entry := fakeMCPServerConfig(t, "srv")
		entry.Env["FIXTURE_SECRET"] = "old-config-sentinel"
		entry.Env["TEST_MCP_FAKE_SERVER_ECHO_ENV"] = "FIXTURE_SECRET"
		failMarker := filepath.Join(dir, "fail-list")
		entry.Env["TEST_MCP_FAKE_SERVER_LIST_FAILS_IF_FILE"] = failMarker
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
		m, _ := newReportTestManager(t)
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		require.Len(t, m.connectedTools(), 1)

		// The replacement config fails to connect, so the previous
		// session is retained and listed again; its error echoes the
		// previous config's secret, which only that config can redact.
		require.NoError(t, os.WriteFile(failMarker, nil, 0o600))
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": {
			Command: filepath.Join(dir, "missing-binary"),
			Env:     map[string]string{"FIXTURE_SECRET": "new-config-sentinel"},
		}})
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := serverByName(t, m.Report(), "srv")
		assert.False(t, got.Connected)
		assert.Contains(t, got.Err, "tools/list rejected")
		assert.NotContains(t, got.Err, "old-config-sentinel")
	})

	t.Run("InheritedSecretRedactedFromConnectError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, entry := fakeMCPServerConfig(t, "srv")
		entry.Env["TEST_MCP_FAKE_SERVER_INIT_FAILS"] = "1"
		entry.Env["TEST_MCP_FAKE_SERVER_ECHO_ENV"] = "CODER_AGENT_TOKEN"
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		// The token reaches the server through the agent's env
		// enrichment, not the config, exactly as in production.
		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil, nil, func(env []string) ([]string, error) {
			return append(env, "CODER_AGENT_TOKEN=agent-token-sentinel"), nil
		}, nil)
		t.Cleanup(func() { _ = m.Close() })
		m.SetInheritedSecrets(func() []string { return []string{"agent-token-sentinel"} })

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := serverByName(t, m.Report(), "srv")
		assert.False(t, got.Connected)
		assert.Contains(t, got.Err, "initialize rejected")
		assert.NotContains(t, got.Err, "agent-token-sentinel")
		assert.Contains(t, got.Err, "[redacted]")
	})

	t.Run("ReconnectFailureRetainsClientWithWarning", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
		m, fires := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		require.Len(t, m.connectedTools(), 1)
		require.Equal(t, 1, fires())

		// Point the same server at a broken command: the reconnect
		// fails and the previous connection keeps serving.
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": {Command: filepath.Join(dir, "missing-binary")}})
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := serverByName(t, m.Report(), "srv")
		assert.True(t, got.Connected)
		require.Len(t, got.Tools, 1, "tools still come from the retained connection")
		assert.Contains(t, got.Warning, "previous connection")
		assert.Contains(t, got.Warning, "missing-binary")
		assert.Equal(t, 2, fires(), "the warning alone changes the report")

		// Restoring a working configuration clears the warning.
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got = serverByName(t, m.Report(), "srv")
		assert.True(t, got.Connected)
		assert.Empty(t, got.Warning)
		assert.Equal(t, 3, fires())
	})

	t.Run("RemovedServerLeavesReport", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, a := fakeMCPServerConfig(t, "a")
		_, b := fakeMCPServerConfig(t, "b")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"a": a, "b": b})
		m, _ := newReportTestManager(t)

		require.NoError(t, m.Reload(ctx, []string{configPath}))
		require.Len(t, m.Report().Servers, 2)

		writeMCPConfig(t, dir, map[string]mcpServerEntry{"a": a})
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		got := m.Report()
		require.Len(t, got.Servers, 1)
		assert.Equal(t, "a", got.Servers[0].Name)
	})

	t.Run("HungSiblingPublishedIncrementally", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, fast := fakeMCPServerConfig(t, "fast")
		_, hung := fakeMCPServerConfig(t, "hung")
		hung.Env["TEST_MCP_FAKE_SERVER_HANG"] = "1"
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"fast": fast, "hung": hung})
		m, reports, clock := newIncrementalReportTestManager(t)
		connectTrap := clock.Trap().AfterFunc("agentmcp", "connect")
		defer connectTrap.Close()

		done := make(chan error, 1)
		go func() { done <- m.Reload(ctx, []string{configPath}) }()
		for range 2 {
			connectTrap.MustWait(ctx).MustRelease(ctx)
		}

		// The fast server is published while the hung handshake is
		// still outstanding.
		interim := testutil.RequireReceive(ctx, t, reports)
		require.Len(t, interim.Servers, 1, "a server still connecting is not reported yet")
		require.True(t, interim.Servers[0].Connected)
		require.Equal(t, "fast", interim.Servers[0].Name)
		require.Len(t, interim.Servers[0].Tools, 1)
		m.mu.RLock()
		earlyClient := m.servers["fast"].client
		m.mu.RUnlock()

		// Expire the hung handshake so the reload settles.
		clock.Advance(connectTimeout).MustWait(ctx)
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		final := m.Report()
		require.Len(t, final.Servers, 2)
		require.True(t, serverByName(t, final, "fast").Connected)
		hungStatus := serverByName(t, final, "hung")
		require.False(t, hungStatus.Connected)
		require.Contains(t, hungStatus.Err, errConnectTimeout.Error())

		// The final install keeps the early-installed client instead of
		// replacing and closing it.
		m.mu.RLock()
		finalClient := m.servers["fast"].client
		m.mu.RUnlock()
		require.Same(t, earlyClient, finalClient)
		listCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer cancel()
		_, err := finalClient.ListTools(listCtx, nil)
		require.NoError(t, err, "the early-installed client must still be open")

		// Drain the final-report and settlement notifications; a reload
		// of the same configuration then publishes nothing new.
		for len(reports) > 0 {
			<-reports
		}
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		require.Empty(t, reports, "an unchanged reload must not republish")
	})

	t.Run("CloseDuringIncrementalReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		dir := t.TempDir()
		_, fast := fakeMCPServerConfig(t, "fast")
		_, hung := fakeMCPServerConfig(t, "hung")
		hung.Env["TEST_MCP_FAKE_SERVER_HANG"] = "1"
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"fast": fast, "hung": hung})
		m, reports, _ := newIncrementalReportTestManager(t)

		done := make(chan error, 1)
		go func() { done <- m.Reload(ctx, []string{configPath}) }()
		interim := testutil.RequireReceive(ctx, t, reports)
		require.Len(t, interim.Servers, 1)

		// Closing with the hung connect outstanding must not stall and
		// must leave nothing published.
		require.NoError(t, m.Close())
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), ErrManagerClosed)
		require.Empty(t, m.Report().Servers)
	})

	t.Run("ReportIsACopy", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		configPath := filepath.Join(t.TempDir(), ".mcp.json")
		require.NoError(t, os.WriteFile(configPath, []byte("{"), 0o600))
		m, _ := newReportTestManager(t)
		require.NoError(t, m.Reload(ctx, []string{configPath}))

		got := m.Report()
		got.ConfigErrors[0].Err = "mutated"
		assert.NotEqual(t, "mutated", m.Report().ConfigErrors[0].Err)
	})
}

// TestReport_RedactsConfiguredSecrets forces stdio and HTTP failures on
// servers whose configuration carries secrets in every supported
// position and asserts none of them reach the report.
func TestReport_RedactsConfiguredSecrets(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	dir := t.TempDir()

	const (
		envSecret    = "ENVSECRET-sentinel-0f3a"
		querySecret  = "QUERYSECRET-sentinel-7b21"
		headerSecret = "HEADERSECRET-sentinel-91cc"
		userSecret   = "USERSECRET-sentinel-42de"
	)

	// A listener that is closed immediately yields a fast connection
	// refused error that echoes the URL.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{
		"stdio": {
			Command: filepath.Join(dir, "missing-binary"),
			Env:     map[string]string{"TOKEN": envSecret},
		},
		"http": {
			Type:    "http",
			URL:     "http://user:" + userSecret + "@" + addr + "/mcp?token=" + querySecret,
			Headers: map[string]string{"Authorization": "Bearer " + headerSecret},
			Env:     map[string]string{"OTHER": envSecret},
		},
	})
	m, _ := newReportTestManager(t)
	require.NoError(t, m.Reload(ctx, []string{configPath}))

	got := m.Report()
	require.Len(t, got.Servers, 2)
	for _, s := range got.Servers {
		require.False(t, s.Connected, "server %q must fail", s.Name)
		require.NotEmpty(t, s.Err, "server %q must carry an error", s.Name)
		for _, secret := range []string{envSecret, querySecret, headerSecret, userSecret} {
			assert.NotContains(t, s.Err, secret, "server %q leaks a secret", s.Name)
		}
	}
	assert.Contains(t, serverByName(t, got, "http").Err, "[redacted]")
}

func TestSanitizeMCPError(t *testing.T) {
	t.Parallel()

	cfg := ServerConfig{
		URL:     "https://alice:pw-sentinel@example.com/mcp?token=query-sentinel&x=1",
		Env:     map[string]string{"A": "env-sentinel", "DEBUG": "1"},
		Headers: map[string]string{"Authorization": "Bearer header-sentinel"},
	}
	err := xerrors.New(`dial https://alice:pw-sentinel@example.com/mcp?token=query-sentinel&x=1 with Bearer header-sentinel env-sentinel exit 1 at 127.0.0.1`)
	got := sanitizeMCPError(cfg, nil, err)
	for _, s := range []string{"pw-sentinel", "query-sentinel", "header-sentinel", "env-sentinel", "alice"} {
		assert.NotContains(t, got, s)
	}
	assert.Contains(t, got, "127.0.0.1", "short values are not redacted, so addresses survive")
	assert.True(t, strings.HasPrefix(got, "dial https://[redacted]@example.com/mcp?[redacted]"), got)
	assert.Empty(t, sanitizeMCPError(cfg, nil, nil))

	t.Run("PathCredential", func(t *testing.T) {
		t.Parallel()
		cfg := ServerConfig{URL: "https://mcp.example.com/api/s/path-sentinel/mcp"}
		got := sanitizeMCPError(cfg, nil, xerrors.New(`Post "https://mcp.example.com/api/s/path-sentinel/mcp": 404 for /api/s/path-sentinel/mcp`))
		assert.NotContains(t, got, "path-sentinel")
		assert.Contains(t, got, "https://mcp.example.com/[redacted]", "scheme and host stay readable")
	})

	t.Run("BoundedToReceiverCap", func(t *testing.T) {
		t.Parallel()
		got := sanitizeMCPError(ServerConfig{}, nil, xerrors.New(strings.Repeat("x", 2*maxDiagnosticBytes)))
		assert.LessOrEqual(t, len(got), maxDiagnosticBytes)
		assert.True(t, strings.HasSuffix(got, truncatedSuffix), got)
	})

	t.Run("InheritedSecrets", func(t *testing.T) {
		t.Parallel()
		// Values the agent injects into the server environment (agent
		// token, user secrets) are not in the config but can be echoed
		// back by the server, so they are redacted like configured ones.
		got := sanitizeMCPError(ServerConfig{}, []string{"token-sentinel", "1"}, xerrors.New("initialize rejected: token-sentinel at 127.0.0.1"))
		assert.NotContains(t, got, "token-sentinel")
		assert.Contains(t, got, "127.0.0.1", "the short-value floor applies to inherited values too")
	})

	t.Run("StdioArguments", func(t *testing.T) {
		t.Parallel()
		// Credentials are commonly passed as args; flags themselves stay
		// readable while their values and every positional arg go.
		cfg := ServerConfig{Command: "npx", Args: []string{"-y", "@scope/server-pkg", "--token", "arg-sentinel", "--key=eq-sentinel"}}
		got := sanitizeMCPError(cfg, nil, xerrors.New("exec npx -y @scope/server-pkg --token arg-sentinel --key=eq-sentinel: exit 1"))
		assert.NotContains(t, got, "arg-sentinel")
		assert.NotContains(t, got, "eq-sentinel")
		assert.NotContains(t, got, "server-pkg")
		assert.Contains(t, got, "--token", "flags are not secrets")
		assert.Contains(t, got, "--key=", "the flag part of a --flag=value pair stays")
		assert.Contains(t, got, "exec npx -y", "the command and short flags stay")
	})
}
