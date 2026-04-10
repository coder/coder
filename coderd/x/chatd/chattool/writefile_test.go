package chattool_test

import (
	"context"
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

func TestWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("RejectsSharedPlanPathWhenPlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		tool := chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			PlanPath: func(context.Context) (string, error) {
				return "/home/coder/.coder/plans/PLAN-chat.md", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "write_file",
			Input: `{"path":"` + chattool.LegacySharedPlanPath + `","content":"# Plan"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(
			t,
			sharedPlanPathResolvedMessage("/home/coder/.coder/plans/PLAN-chat.md"),
			resp.Content,
		)
	})

	t.Run("AllowsNonSharedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			WriteFile(gomock.Any(), "/home/dev/my-plan.md", gomock.Any()).
			DoAndReturn(func(_ context.Context, path string, reader io.Reader) error {
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, "/home/dev/my-plan.md", path)
				require.Equal(t, "# Plan", string(data))
				return nil
			})

		planPathCalled := false
		tool := chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			PlanPath: func(context.Context) (string, error) {
				planPathCalled = true
				return "", xerrors.New("should not be called")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "write_file",
			Input: `{"path":"/home/dev/my-plan.md","content":"# Plan"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, planPathCalled)
		assert.Equal(t, `{"ok":true}`, strings.TrimSpace(resp.Content))
	})

	t.Run("AllowsSharedPlanPathWhenPlanPathIsNil", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			WriteFile(gomock.Any(), chattool.LegacySharedPlanPath, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, reader io.Reader) error {
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, "# Plan", string(data))
				return nil
			})

		tool := chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "write_file",
			Input: `{"path":"` + chattool.LegacySharedPlanPath + `","content":"# Plan"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})
}
