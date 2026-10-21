package chattool_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestReconnectMCPServers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	conn.EXPECT().ResyncContext(gomock.Any()).Return(workspacesdk.ContextSnapshotResponse{
		Resources: []workspacesdk.ContextSnapshotResource{
			{Kind: "instruction_file", Name: "AGENTS.md", Status: "ok"},
			{Kind: "mcp_server", Name: "storybook", Status: "unreadable", Error: "connection refused"},
		},
	}, nil)

	tool := chattool.ReconnectMCPServers(chattool.ProcessToolOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "reconnect_mcp_servers", Input: "{}"})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	require.JSONEq(t,
		`{"servers":[{"kind":"mcp_server","name":"storybook","status":"unreadable","error":"connection refused"}]}`,
		resp.Content)
}
