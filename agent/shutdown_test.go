package agent_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestAgentCloseLogsWorkspaceShutdown(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("test command requires a POSIX shell")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	sink := testutil.NewFakeSink(t)
	shutdown := &proto.WorkspaceShutdown{BuildId: uuid.NewString(), Transition: "stop", BuildReason: "autostop"}
	//nolint:dogsled
	conn, _, _, _, agnt := setupAgent(t, agentsdk.Manifest{}, 0, func(c *agenttest.Client, o *agent.Options) {
		o.Client = &shutdownAgentClient{Client: c, shutdown: shutdown}
		o.Logger = sink.Logger()
	})
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	defer session.Close()
	stdin, err := session.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()
	stdout := expecter.NewAttachedToSSHSession(t, session)
	require.NoError(t, session.Start("echo ready; cat"))
	stdout.ExpectMatch(ctx, "ready")
	require.NoError(t, agnt.Close())
	for _, message := range []string{"ssh connection complete", "ssh session closed"} {
		entries := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == message })
		require.Len(t, entries, 1)
		require.Contains(t, entries[0].Fields, codersdk.DisconnectReasonWorkspaceStopped.SlogField())
		require.Contains(t, entries[0].Fields, codersdk.DisconnectInitiatorServer.SlogField())
		require.Contains(t, entries[0].Fields, slog.F("build_id", shutdown.BuildId))
		require.Contains(t, entries[0].Fields, slog.F("build_reason", shutdown.BuildReason))
	}
}

type shutdownAgentClient struct {
	*agenttest.Client
	shutdown *proto.WorkspaceShutdown
}

func (c *shutdownAgentClient) ConnectRPC213WithRole(ctx context.Context, role string) (proto.DRPCAgentClient213, tailnetproto.DRPCTailnetClient28, error) {
	a, tailnet, err := c.Client.ConnectRPC213WithRole(ctx, role)
	if err != nil {
		return nil, nil, err
	}
	return &shutdownAgentAPI{DRPCAgentClient213: a, shutdown: c.shutdown}, tailnet, nil
}

type shutdownAgentAPI struct {
	proto.DRPCAgentClient213
	shutdown *proto.WorkspaceShutdown
}

func (c *shutdownAgentAPI) GetWorkspaceShutdown(context.Context, *emptypb.Empty) (*proto.WorkspaceShutdown, error) {
	return c.shutdown, nil
}
