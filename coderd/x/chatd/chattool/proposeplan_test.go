package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

type proposePlanResponse struct {
	OK        bool   `json:"ok"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	FileID    string `json:"file_id"`
	MediaType string `json:"media_type"`
}

func TestProposePlan(t *testing.T) {
	t.Parallel()

	t.Run("EmptyPathReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
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

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
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

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/plan.txt"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "path must end with .md")
	})

	t.Run("RelativePlanPathReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		storeFile, _ := fakeStoreFile(t)
		resolvePlanPathCalled := false
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"plan.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
		assert.Equal(t, relativePlanPathMessage(), resp.Content)
	})

	t.Run("OversizedFileRejected", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		largeContent := strings.Repeat("x", 32*1024+1)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/PLAN.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader(largeContent)), "text/markdown", nil)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "plan file exceeds 32 KiB size limit")
	})

	t.Run("ExactBoundaryFileSucceeds", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		content := strings.Repeat("x", 32*1024)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/PLAN.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader(content)), "text/markdown", nil)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})

	t.Run("ValidPlanReadsFile", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/docs/PLAN.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Plan\n\nContent")), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		planPathCalled := false
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				planPathCalled = true
				return "/home/coder/.coder/plans/PLAN-xxx.md", "/home/coder", nil
			},
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/docs/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.True(t, planPathCalled)

		result := decodeProposePlanResponse(t, resp)
		assert.True(t, result.OK)
		assert.Equal(t, "/home/coder/docs/PLAN.md", result.Path)
		assert.Equal(t, "plan", result.Kind)
		assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", result.FileID)
		assert.Equal(t, "text/markdown", result.MediaType)
		assert.Equal(t, []byte("# Plan\n\nContent"), *stored)
		assert.NotContains(t, resp.Content, "content")
	})

	t.Run("NestedPlanPathUnderHomeIsAllowed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/myproject/plan.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Nested Plan")), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		planPathCalled := false
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				planPathCalled = true
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/myproject/plan.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.True(t, planPathCalled)

		result := decodeProposePlanResponse(t, resp)
		assert.True(t, result.OK)
		assert.Equal(t, "/home/coder/myproject/plan.md", result.Path)
		assert.Equal(t, []byte("# Nested Plan"), *stored)
	})

	t.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/PLAN.md", int64(0), int64(32*1024+1)).
			Return(nil, "", xerrors.New("file not found"))

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "file not found")
	})

	t.Run("ReadAllError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/PLAN.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(iotest.ErrReader(xerrors.New("connection reset"))), "text/markdown", nil)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanTool(t, mockConn, storeFile)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "connection reset")
	})

	t.Run("StoreFileError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/PLAN.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Plan")), "text/markdown", nil)

		tool := newProposePlanTool(t, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (uuid.UUID, error) {
			return uuid.Nil, xerrors.New("storage unavailable")
		})
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "storage unavailable")
	})

	t.Run("RejectsSharedPlanPathWithResolvedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chattool.LegacySharedPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(
			t,
			sharedPlanPathResolvedMessage(chattool.LegacySharedPlanPath, "/home/coder/.coder/plans/PLAN-chat.md"),
			resp.Content,
		)
	})

	t.Run("RejectsSharedPlanPathWhenResolverFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				return "", "", xerrors.New("workspace unavailable")
			},
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chattool.LegacySharedPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, planPathVerificationMessage(chattool.LegacySharedPlanPath), resp.Content)
	})

	t.Run("PerChatPlanPathIsAllowed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-123e4567-e89b-12d3-a456-426614174000.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Per-Chat Plan")), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		resolvePlanPathCalled := false
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return chatPlanPath, "/home/coder", nil
			},
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chatPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)

		result := decodeProposePlanResponse(t, resp)
		assert.True(t, result.OK)
		assert.Equal(t, chatPlanPath, result.Path)
		assert.Equal(t, []byte("# Per-Chat Plan"), *stored)
	})

	t.Run("NestedPlanPathAllowedWhenResolverFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		mockConn.EXPECT().
			ReadFile(gomock.Any(), "/home/coder/myproject/plan.md", int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Nested Plan")), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				return "", "", xerrors.New("workspace unavailable")
			},
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/myproject/plan.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		result := decodeProposePlanResponse(t, resp)
		assert.True(t, result.OK)
		assert.Equal(t, "/home/coder/myproject/plan.md", result.Path)
		assert.Equal(t, []byte("# Nested Plan"), *stored)
	})

	t.Run("WorkspaceConnectionError", func(t *testing.T) {
		t.Parallel()
		storeFile, _ := fakeStoreFile(t)
		tool := chattool.ProposePlan(chattool.ProposePlanOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return nil, xerrors.New("connection failed")
			},
			StoreFile: storeFile,
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
		assert.Contains(t, resp.Content, "workspace connection resolver is not configured")
	})

	t.Run("NilStoreFile", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		tool := chattool.ProposePlan(chattool.ProposePlanOptions{
			GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/PLAN.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "file storage is not configured")
	})
}

func newProposePlanTool(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
	storeFile func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error),
) fantasy.AgentTool {
	t.Helper()
	return newProposePlanToolWithPlanPath(t, mockConn, storeFile, nil)
}

func newProposePlanToolWithPlanPath(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
	storeFile func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error),
	resolvePlanPath func(context.Context) (string, string, error),
) fantasy.AgentTool {
	t.Helper()
	return chattool.ProposePlan(chattool.ProposePlanOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		ResolvePlanPath: resolvePlanPath,
		StoreFile:       storeFile,
	})
}

func sharedPlanPathResolvedMessage(requestedPath, planPath string) string {
	return "the plan path " + requestedPath +
		" is no longer supported at the home root; use the chat-specific plan path: " + planPath
}

func planPathVerificationMessage(requestedPath string) string {
	return "the plan path " + requestedPath +
		" could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly"
}

func editFilesBatchRejectedMessage(message string) string {
	return message + "; no files in this batch were applied"
}

func relativePlanPathMessage() string {
	return "plan files must use absolute paths; use the chat-specific absolute plan path"
}

func fakeStoreFile(t *testing.T) (func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error), *[]byte) {
	t.Helper()

	var stored []byte
	return func(_ context.Context, name string, mediaType string, data []byte) (uuid.UUID, error) {
		assert.NotEmpty(t, name)
		assert.Equal(t, "text/markdown", mediaType)
		stored = append([]byte(nil), data...)
		return uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"), nil
	}, &stored
}

func decodeProposePlanResponse(t *testing.T, resp fantasy.ToolResponse) proposePlanResponse {
	t.Helper()

	var result proposePlanResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}
