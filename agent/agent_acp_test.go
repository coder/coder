package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/usershell"
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
	home, dir := t.TempDir(), t.TempDir()
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "broken.json"), []byte(`{"command":"false"}`), 0o600))
	binary, err := os.Executable()
	require.NoError(t, err)
	command := fmt.Sprintf("CODER_AGENT_ACP_TEST_HELPER=1 '%s' -test.run=^TestAgentACPHarness$", strings.ReplaceAll(binary, "'", "'\"'\"'"))
	raw, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "fake.json"), raw, 0o600))
	//nolint:dogsled // The integration fixture also returns unrelated transports.
	conn, client, _, _, _ := setupAgent(t, agentsdk.Manifest{Directory: dir}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.EnvInfo = acpEnvInfo{home: home}
		opts.Filesystem = afero.NewOsFs()
	})
	ctx := testutil.Context(t, testutil.WaitLong)
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
	require.Eventually(t, func() bool {
		for _, push := range client.ContextStatePushes() {
			for _, resource := range push.Resources {
				if harness := resource.GetAcpHarness(); harness != nil && harness.Slug == "broken" {
					return true
				}
			}
		}
		return false
	}, testutil.WaitLong, testutil.IntervalFast, "context pushes must include the harness catalog")
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
	return acp.InitializeResponse{ProtocolVersion: acp.ProtocolVersionNumber}, nil
}

func (agentACPHarness) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: acp.SessionId(uuid.NewString())}, nil
}
