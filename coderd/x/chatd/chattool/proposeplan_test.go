package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

type proposePlanResponse struct {
	OK      bool   `json:"ok"`
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

func TestProposePlan(t *testing.T) {
	t.Parallel()

	t.Run("EmptyPathReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		tool := newProposePlanTool(t, mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":""}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "path is required")
	})

	t.Run("WhitespaceOnlyPathReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		tool := newProposePlanTool(t, mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"  "}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "path is required")
	})

	t.Run("NonMdPathReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		tool := newProposePlanTool(t, mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/plan.txt"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "path must end with .md")
	})

	t.Run("ValidPlanReadsFile", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/docs/PLAN.md", int64(0), int64(0)).
			Return(io.NopCloser(strings.NewReader("# Plan\n\nContent")), "text/markdown", nil)

		tool := newProposePlanTool(t, mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/docs/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		result := decodeProposePlanResponse(t, resp)
		assert.True(t, result.OK)
		assert.Equal(t, "/home/coder/docs/PLAN.md", result.Path)
		assert.Equal(t, "plan", result.Kind)
		assert.Equal(t, "# Plan\n\nContent", result.Content)
	})

	t.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/PLAN.md", int64(0), int64(0)).
			Return(nil, "", xerrors.New("file not found"))

		tool := newProposePlanTool(t, mockConn)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "file not found")
	})

	t.Run("WorkspaceConnectionError", func(t *testing.T) {
		t.Parallel()
		tool := chattool.ProposePlan(chattool.ProposePlanOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return nil, xerrors.New("connection failed")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "connection failed")
	})

	t.Run("NilWorkspaceResolver", func(t *testing.T) {
		t.Parallel()
		tool := chattool.ProposePlan(chattool.ProposePlanOptions{})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "not configured")
	})
}

func newProposePlanTool(t *testing.T, mockConn *agentconnmock.MockAgentConn) fantasy.AgentTool {
	t.Helper()
	return chattool.ProposePlan(chattool.ProposePlanOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})
}

func decodeProposePlanResponse(t *testing.T, resp fantasy.ToolResponse) proposePlanResponse {
	t.Helper()

	var result proposePlanResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}
