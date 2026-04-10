package chattool_test

import (
	"context"
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

func TestEditFiles(t *testing.T) {
	t.Parallel()

	t.Run("RejectsHomeRootPlanPathsWhenPlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			PlanPath: func(context.Context) (string, string, error) {
				return "/Users/dev/.coder/plans/PLAN-chat.md", "/Users/dev", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/Users/dev/plan.md","edits":[{"search":"old","replace":"new"}]}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(
			t,
			sharedPlanPathResolvedMessage("/Users/dev/.coder/plans/PLAN-chat.md"),
			resp.Content,
		)
	})

	t.Run("AllowsNonSharedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path: "/home/dev/my-plan.md",
			Edits: []workspacesdk.FileEdit{{
				Search:  "old",
				Replace: "new",
			}},
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(nil)

		planPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			PlanPath: func(context.Context) (string, string, error) {
				planPathCalled = true
				return "", "", xerrors.New("should not be called")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/home/dev/my-plan.md","edits":[{"search":"old","replace":"new"}]}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, planPathCalled)
	})
}
