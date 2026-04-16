package chattool_test

import (
	"context"
	"net/http"
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

	t.Run("PlanTurnRejectsNonPlanPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		getWorkspaceConnCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				getWorkspaceConnCalled = true
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/home/coder/README.md","edits":[{"search":"old","replace":"new"}]}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "during plan turns, edit_files is restricted to "+planPath, resp.Content)
		assert.False(t, getWorkspaceConnCalled)
	})

	t.Run("PlanTurnRejectsMixedPaths", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		getWorkspaceConnCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				getWorkspaceConnCalled = true
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:   "call-1",
			Name: "edit_files",
			Input: `{"files":[` +
				`{"path":"` + planPath + `","edits":[{"search":"old","replace":"new"}]},` +
				`{"path":"/home/coder/README.md","edits":[{"search":"old","replace":"new"}]}` +
				`]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "during plan turns, edit_files is restricted to "+planPath, resp.Content)
		assert.False(t, getWorkspaceConnCalled)
	})

	t.Run("PlanTurnAllowsResolvedPlanPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		resolvePlanPathCalls := 0
		mockConn.EXPECT().ResolvePath(gomock.Any(), planPath).Return(planPath, nil)
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     planPath,
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalls++
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"` + planPath + `","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, 1, resolvePlanPathCalls)
	})

	t.Run("PlanTurnAllowsLegacyAgentWithoutResolvePath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		mockConn.EXPECT().
			ResolvePath(gomock.Any(), planPath).
			Return("", statusError{statusCode: http.StatusNotFound, message: "missing resolve-path endpoint"})
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     planPath,
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"` + planPath + `","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})

	t.Run("PlanTurnRejectsSymlinkedPlanPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		mockConn.EXPECT().ResolvePath(gomock.Any(), planPath).Return("/home/coder/README.md", nil)
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"` + planPath + `","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "the chat-specific plan path /home/coder/.coder/plans/PLAN-test-uuid.md resolves to /home/coder/README.md; symlinked plan paths are not allowed during plan turns", resp.Content)
	})

	t.Run("RejectsPlanPathsWhenResolvePlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                 string
			input                string
			expectedRejectedPath string
		}{
			{
				name:                 "SingleHomeRootPlanPath",
				input:                `{"files":[{"path":"/Users/dev/plan.md","ed_script":"1,$s/old/new/g"}]}`,
				expectedRejectedPath: "/Users/dev/plan.md",
			},
			{
				name: "MultiFileBatchWithHomeRootPlanPath",
				input: `{"files":[` +
					`{"path":"/Users/dev/subdir/plan.md","ed_script":"1,$s/old/new/g"},` +
					`{"path":"/Users/dev/plan.md","ed_script":"1,$s/old/new/g"}` +
					`]}`,
				expectedRejectedPath: "/Users/dev/plan.md",
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)
				resolvePlanPathCalls := 0
				tool := chattool.EditFiles(chattool.EditFilesOptions{
					GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
						return mockConn, nil
					},
					ResolvePlanPath: func(context.Context) (string, string, error) {
						resolvePlanPathCalls++
						return "/Users/dev/.coder/plans/PLAN-chat.md", "/Users/dev", nil
					},
				})

				resp, err := tool.Run(context.Background(), fantasy.ToolCall{
					ID:    "call-1",
					Name:  "edit_files",
					Input: testCase.input,
				})
				require.NoError(t, err)
				assert.True(t, resp.IsError)
				assert.Equal(t, 1, resolvePlanPathCalls)
				assert.Equal(
					t,
					editFilesBatchRejectedMessage(sharedPlanPathResolvedMessage(
						testCase.expectedRejectedPath,
						"/Users/dev/.coder/plans/PLAN-chat.md",
					)),
					resp.Content,
				)
			})
		}
	})

	t.Run("RejectsSharedPlanPathWhenResolverFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return "", "", xerrors.New("workspace unavailable")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/home/coder/plan.md","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, editFilesBatchRejectedMessage(planPathVerificationMessage("/home/coder/plan.md")), resp.Content)
	})

	t.Run("RejectsRelativePlanPathsWhenResolvePlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		resolvePlanPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"plan.md","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
		assert.Equal(t, editFilesBatchRejectedMessage(relativePlanPathMessage()), resp.Content)
	})

	t.Run("PerChatPlanPathIsAllowed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-123e4567-e89b-12d3-a456-426614174000.md"
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     chatPlanPath,
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		resolvePlanPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return chatPlanPath, "/home/coder", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"` + chatPlanPath + `","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
	})

	t.Run("NestedPlanPathAllowedWhenResolverFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     "/home/coder/myproject/plan.md",
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return "", "", xerrors.New("workspace unavailable")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/home/coder/myproject/plan.md","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})

	t.Run("NestedPlanPathUnderHomeIsAllowed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     "/home/coder/myproject/plan.md",
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		planPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				planPathCalled = true
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/home/coder/myproject/plan.md","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.True(t, planPathCalled)
	})

	t.Run("AllowsNonSharedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     "/home/dev/my-plan.md",
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		resolvePlanPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return "", "", xerrors.New("should not be called")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/home/dev/my-plan.md","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
	})

	t.Run("AllowsSharedPlanPathWhenResolvePlanPathIsNil", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{Files: []workspacesdk.FileEdits{{
			Path:     chattool.LegacySharedPlanPath,
			EdScript: "1,$s/old/new/g",
		}}}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"` + chattool.LegacySharedPlanPath + `","ed_script":"1,$s/old/new/g"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})

	t.Run("ReturnsDiffInResult", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().EditFiles(gomock.Any(), gomock.Any()).Return(workspacesdk.FileEditResponse{
			Diffs: []workspacesdk.FileEditDiff{
				{Path: "/a.go", Diff: "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"},
				{Path: "/b.go", Diff: "--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n-x\n+y\n"},
			},
		}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"files":[{"path":"/a.go","ed_script":"1s/old/new/"},{"path":"/b.go","ed_script":"1s/x/y/"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Contains(t, resp.Content, "-old")
		assert.Contains(t, resp.Content, "+new")
		assert.Contains(t, resp.Content, "-x")
		assert.Contains(t, resp.Content, "+y")
	})
}
