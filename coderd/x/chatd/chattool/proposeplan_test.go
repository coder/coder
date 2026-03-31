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
		tool := newProposePlanTool(t, mockConn, storeFile)
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
		assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", result.FileID)
		assert.Equal(t, "text/markdown", result.MediaType)
		assert.Equal(t, []byte("# Plan\n\nContent"), *stored)
		assert.NotContains(t, resp.Content, "content")
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
	return chattool.ProposePlan(chattool.ProposePlanOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		StoreFile: storeFile,
	})
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
