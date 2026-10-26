package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/agent/x/agentnative"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentNativeAgentsStartupAndClose(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("native agents require Linux")
	}
	home := t.TempDir()
	ctx := testutil.Context(t, testutil.WaitLong)
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	stopResult := make(chan error, 1)
	execer := &nativeStartupExecer{
		Execer: agentexec.DefaultExecer, started: started, release: release,
		onStop: func() {
			// Stop scripts need the local API to request explicit shutdown.
			descriptor, err := agentnative.ReadDescriptor(agentnative.DefaultStateDir(home))
			if err != nil {
				stopResult <- err
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, descriptor.URL+"/v1/shutdown", strings.NewReader("{}"))
			if err != nil {
				stopResult <- err
				return
			}
			req.Header.Set("Authorization", "Bearer "+descriptor.Token)
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = res.Body.Close()
				if res.StatusCode != http.StatusOK {
					err = xerrors.Errorf("local API unavailable to stop script: %d", res.StatusCode)
				}
			}
			stopResult <- err
		},
	}
	//nolint:dogsled // Only the connection and agent lifetime are under test.
	conn, _, _, _, agnt := setupAgent(t, agentsdk.Manifest{
		Directory: home,
		Scripts: []codersdk.WorkspaceAgentScript{
			{ID: uuid.New(), LogSourceID: uuid.New(), Script: "true", RunOnStart: true},
			{ID: uuid.New(), LogSourceID: uuid.New(), Script: "true # native-stop", RunOnStop: true},
		},
	}, 0, func(_ *agenttest.Client, options *agent.Options) {
		options.NativeAgents = true
		options.EnvInfo = nativeAgentHome{home: home}
		options.Execer = execer
		options.Filesystem = afero.NewOsFs()
		options.TempDir = t.TempDir()
		options.LogDir = t.TempDir()
		options.ScriptDataDir = t.TempDir()
	})
	testutil.TryReceive(ctx, t, started)
	stateDir := agentnative.DefaultStateDir(home)
	_, err := os.Stat(filepath.Join(stateDir, "manager.json"))
	require.ErrorIs(t, err, os.ErrNotExist, "manager must not start before scripts finish")
	_, err = conn.ConnectNativeAgentACP(ctx)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode())
	unblock()

	var stream net.Conn
	require.Eventually(t, func() bool {
		stream, err = conn.ConnectNativeAgentACP(ctx)
		return err == nil
	}, testutil.WaitLong, testutil.IntervalFast)
	defer stream.Close()
	_, err = io.WriteString(stream, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":1}}\n")
	require.NoError(t, err)
	var response struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.NewDecoder(stream).Decode(&response))
	require.Equal(t, 1, response.ID)
	require.NotEmpty(t, response.Result)
	require.NoError(t, stream.Close())

	// Even the local bearer credential does not expose local REST on the
	// remote API. Only ACP is mounted, never attach, shutdown or runtime.
	descriptor, err := agentnative.ReadDescriptor(stateDir)
	require.NoError(t, err)
	for _, path := range []string{
		"/api/v0/native-agents/v1/sessions", "/api/v0/native-agents/v1/sessions/id/attach",
		"/api/v0/native-agents/v1/shutdown", "/api/v0/native-agents/v1/runtime", "/v1/sessions",
	} {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://localhost:%d%s", workspacesdk.AgentHTTPAPIServerPort, path), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+descriptor.Token)
		res, err := conn.AppHTTPClient().Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, res.Body.Close())
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, res.StatusCode, path)
		require.NotContains(t, string(body), descriptor.Token)
	}

	require.NoError(t, agnt.Close())
	require.NoError(t, testutil.TryReceive(ctx, t, stopResult))
	// The exclusive lock is released on agent Close, permitting an agent-only
	// restart to reopen the same histories instead of shutting them down.
	manager, err := agentnative.New(agentnative.Options{StateDir: stateDir, Environment: []string{"HOME=" + home}})
	require.NoError(t, err)
	require.NoError(t, manager.Close())
}

type nativeAgentHome struct {
	usershell.SystemEnvInfo
	home string
}

func (e nativeAgentHome) HomeDir() (string, error) { return e.home, nil }
func (e nativeAgentHome) Environ() []string        { return []string{"HOME=" + e.home, "PATH=/usr/bin:/bin"} }

type nativeStartupExecer struct {
	agentexec.Execer
	once    sync.Once
	started chan struct{}
	release <-chan struct{}
	onStop  func()
}

func (e *nativeStartupExecer) PTYCommandContext(ctx context.Context, name string, args ...string) *pty.Cmd {
	e.once.Do(func() {
		close(e.started)
		select {
		case <-ctx.Done():
		case <-e.release:
		}
	})
	if slices.Contains(args, "true # native-stop") {
		e.onStop()
	}
	return e.Execer.PTYCommandContext(ctx, name, args...)
}
