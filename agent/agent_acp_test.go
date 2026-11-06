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
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

type acpEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e acpEnvInfo) HomeDir() (string, error) { return e.home, nil }

func TestAgent_ACPInterface(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	home, dir := t.TempDir(), t.TempDir()
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "broken.json"), []byte(`{"command":"false"}`), 0o600))
	binary, err := os.Executable()
	require.NoError(t, err)
	command := fmt.Sprintf("CODER_AGENT_ACP_TEST_HELPER=1 '%s' -test.run=^TestAgentACPHarness$", strings.ReplaceAll(binary, "'", "'\"'\"'"))
	raw, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	manifest := agentsdk.Manifest{
		Directory: dir,
		Scripts: []codersdk.WorkspaceAgentScript{{
			ID: uuid.New(), LogSourceID: uuid.New(), RunOnStart: true,
			Script: "printf '%s' " + quote(string(raw)) + " > " + quote(filepath.Join(configDir, "fake.json")),
		}},
	}
	//nolint:dogsled // The integration fixture also returns unrelated transports.
	conn, client, _, _, _ := setupAgent(t, manifest, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.EnvInfo = acpEnvInfo{home: home}
		opts.Filesystem = afero.NewOsFs()
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	var pushes []*agentproto.PushContextStateRequest
	require.Eventually(t, func() bool {
		pushes = client.ContextStatePushes()
		return len(pushes) > 0
	}, testutil.WaitLong, testutil.IntervalFast)
	first := pushes[0]
	require.True(t, first.GetInitial())
	harnesses := make(map[string]*agentproto.ContextResource)
	for _, resource := range first.GetResources() {
		if harness := resource.GetAcpHarness(); harness != nil {
			harnesses[harness.Slug] = resource
		}
	}
	require.Contains(t, harnesses, "fake", "the first context push must include successful probes")
	require.Equal(t, agentproto.ContextResource_OK, harnesses["fake"].GetStatus())
	require.Contains(t, harnesses, "broken", "the first context push must include failed probes")
	require.Equal(t, agentproto.ContextResource_INVALID, harnesses["broken"].GetStatus())
	require.NotEmpty(t, harnesses["broken"].GetError())
	_, err = conn.ListACPHarnesses(ctx)
	require.NoError(t, err)
	tool := uuid.New()
	conn.SetExtraHeaders(http.Header{
		workspacesdk.CoderChatIDHeader: {uuid.NewString()},
	})
	ctx = workspacesdk.WithToolCallID(ctx, tool)
	var catalog []workspacesdk.ACPHarness
	require.Eventually(t, func() bool { catalog, err = conn.ListACPHarnesses(ctx); return err == nil && len(catalog) == 2 }, testutil.WaitLong, testutil.IntervalFast)
	require.Equal(t, "broken", catalog[0].Slug)
	info, err := conn.CreateACPSession(ctx, workspacesdk.ACPCreateSessionRequest{HarnessSlug: "fake", WorkingDirectory: dir})
	require.NoError(t, err)
	events, closer, err := conn.WatchACPSession(ctx, testutil.Logger(t), info.ID, workspacesdk.ACPReadOptions{})
	require.NoError(t, err)
	defer closer.Close()
	for {
		event := testutil.RequireReceive(ctx, t, events)
		if event.Kind == workspacesdk.ACPEventKindSnapshot {
			require.Equal(t, info.ID, event.Session.ID)
			break
		}
	}
}

func TestAgent_ACPReadiness(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	for _, failed := range []bool{false, true} {
		name := "Ready"
		if failed {
			name = "StartError"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			require.NoError(t, err)
			t.Cleanup(func() { _ = listener.Close() })
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.NoError(t, listener.SetDeadline(deadline))
			home, dir := t.TempDir(), t.TempDir()
			configDir := filepath.Join(home, ".coder", "acp")
			require.NoError(t, os.MkdirAll(configDir, 0o700))
			binary, err := os.Executable()
			require.NoError(t, err)
			command := fmt.Sprintf("CODER_AGENT_ACP_TEST_READY_ADDR=%s CODER_AGENT_ACP_TEST_HELPER=1 '%s' -test.run=^TestAgentACPHarness$", listener.Addr(), strings.ReplaceAll(binary, "'", "'\"'\"'"))
			raw, err := json.Marshal(map[string]string{"command": command})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(configDir, "fake.json"), raw, 0o600))
			script := "true"
			if failed {
				script = "false"
			}
			//nolint:dogsled // Only lifecycle reports and context pushes are needed.
			_, client, _, _, _ := setupAgent(t, agentsdk.Manifest{
				Directory: dir,
				Scripts: []codersdk.WorkspaceAgentScript{{
					ID: uuid.New(), LogSourceID: uuid.New(), RunOnStart: true, Script: script,
				}},
			}, 0, func(_ *agenttest.Client, opts *agent.Options) {
				opts.EnvInfo = acpEnvInfo{home: home}
				opts.Filesystem = afero.NewOsFs()
			})
			ready, err := listener.Accept()
			require.NoError(t, err)
			t.Cleanup(func() { _ = ready.Close() })
			require.NoError(t, ready.SetDeadline(deadline))
			var signal [1]byte
			_, err = io.ReadFull(ready, signal[:])
			require.NoError(t, err, "the probe must enter initialization before lifecycle assertions")
			require.Eventually(t, func() bool {
				return slices.Contains(client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleStarting)
			}, testutil.WaitLong, testutil.IntervalFast)
			require.NotContains(t, client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleReady)
			require.NotContains(t, client.GetLifecycleStates(), codersdk.WorkspaceAgentLifecycleStartError)
			require.Empty(t, client.ContextStatePushes(), "the initial context waits for probes on both startup outcomes")
			_, err = ready.Write([]byte{1})
			require.NoError(t, err)
			require.Eventually(t, func() bool { return len(client.ContextStatePushes()) > 0 }, testutil.WaitLong, testutil.IntervalFast)
			first := client.ContextStatePushes()[0]
			require.True(t, first.GetInitial())
			var found bool
			for _, resource := range first.GetResources() {
				if h := resource.GetAcpHarness(); h != nil && h.Slug == "fake" {
					found = true
					require.Equal(t, agentproto.ContextResource_OK, resource.Status)
				}
			}
			require.True(t, found)
			want := codersdk.WorkspaceAgentLifecycleReady
			if failed {
				want = codersdk.WorkspaceAgentLifecycleStartError
			}
			require.Eventually(t, func() bool {
				return slices.Contains(client.GetLifecycleStates(), want)
			}, testutil.WaitLong, testutil.IntervalFast)
		})
	}
}

// TestAgentACPHarness provides a real ACP transport for the full agent router.
//
//nolint:paralleltest // The helper branch owns stdout and exits directly.
func TestAgentACPHarness(t *testing.T) {
	if os.Getenv("CODER_AGENT_ACP_TEST_HELPER") != "1" {
		t.Parallel()
		return
	}
	conn := acp.NewAgentSideConnection(agentACPHarness{}, os.Stdout, os.Stdin)
	<-conn.Done()
	os.Exit(0) //nolint:revive // Keep Go test output out of the ACP stream.
}

type agentACPHarness struct{ acp.Agent }

func (agentACPHarness) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	if address := os.Getenv("CODER_AGENT_ACP_TEST_READY_ADDR"); address != "" {
		ready, err := net.Dial("tcp", address)
		if err != nil {
			return acp.InitializeResponse{}, err
		}
		defer ready.Close()
		if _, err := ready.Write([]byte{1}); err != nil {
			return acp.InitializeResponse{}, err
		}
		var signal [1]byte
		if _, err := io.ReadFull(ready, signal[:]); err != nil {
			return acp.InitializeResponse{}, err
		}
	}
	return acp.InitializeResponse{ProtocolVersion: acp.ProtocolVersionNumber}, nil
}

func (agentACPHarness) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: acp.SessionId(uuid.NewString())}, nil
}
