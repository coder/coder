package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

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

	t.Run("RejectsEmptyPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(t, mockConn, storeFile, nil, false)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":""}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "path is required (use the chat-specific absolute plan path)", resp.Content)
	})

	t.Run("RejectsNonMarkdownPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(t, mockConn, storeFile, nil, false)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/.coder/plans/PLAN-chat.txt"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "path must end with .md", resp.Content)
	})

	t.Run("PlanTurnDefaultsEmptyPathToResolvedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Plan")), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				return chatPlanPath, "/home/coder", nil
			},
			true,
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":""}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		result := decodeProposePlanResponse(t, resp)
		assert.True(t, result.OK)
		assert.Equal(t, chatPlanPath, result.Path)
		assert.Equal(t, "plan", result.Kind)
		assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", result.FileID)
		assert.Equal(t, "text/markdown", result.MediaType)
		assert.Equal(t, "# Plan", string(*stored))
	})

	t.Run("PlanTurnRejectsWrongPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			storeFile,
			func(context.Context) (string, string, error) {
				return chatPlanPath, "/home/coder", nil
			},
			true,
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"/home/coder/README.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "during plan turns, propose_plan path must be "+chatPlanPath, resp.Content)
	})
	t.Run("RejectsReadFileErrors", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(nil, "", xerrors.New("read failed"))

		storeFile, _ := fakeStoreFile(t)
		tool := newProposePlanToolWithPlanPath(t, mockConn, storeFile, nil, false)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chatPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "read failed", resp.Content)
	})

	t.Run("PlanTurnRejectsEmptyPlan", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("")), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		storeCalled := false
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error) {
				storeCalled = true
				return storeFile(ctx, name, mediaType, data)
			},
			func(context.Context) (string, string, error) {
				return chatPlanPath, "/home/coder", nil
			},
			true,
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chatPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "plan file is empty")
		assert.Contains(t, resp.Content, chatPlanPath)
		assert.False(t, storeCalled)
		assert.Nil(t, *stored)
	})

	t.Run("RejectsOversizedPlan", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader(strings.Repeat("x", 32*1024+1))), "text/markdown", nil)

		storeFile, stored := fakeStoreFile(t)
		storeCalled := false
		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error) {
				storeCalled = true
				return storeFile(ctx, name, mediaType, data)
			},
			nil,
			false,
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chatPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "plan file exceeds 32 KiB size limit", resp.Content)
		assert.False(t, storeCalled)
		assert.Nil(t, *stored)
	})

	t.Run("PropagatesStoreFileErrors", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Plan")), "text/markdown", nil)

		tool := newProposePlanToolWithPlanPath(
			t,
			mockConn,
			func(context.Context, string, string, []byte) (uuid.UUID, error) {
				return uuid.Nil, xerrors.New("store failed")
			},
			nil,
			false,
		)
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "propose_plan",
			Input: `{"path":"` + chatPlanPath + `"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "failed to store plan file: store failed", resp.Content)
	})
}

func newProposePlanToolWithPlanPath(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
	storeFile func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error),
	resolvePlanPath func(context.Context) (string, string, error),
	isPlanTurn bool,
) fantasy.AgentTool {
	t.Helper()
	return chattool.ProposePlan(chattool.ProposePlanOptions{
		GetWorkspaceConn: func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		ResolvePlanPath: resolvePlanPath,
		StoreFile:       storeFile,
		IsPlanTurn:      isPlanTurn,
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
