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

	t.Run("PlanTurnDefaultsEmptyPathToResolvedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-chat.md"

		mockConn.EXPECT().
			ReadFile(gomock.Any(), chatPlanPath, int64(0), int64(32*1024+1)).
			Return(io.NopCloser(strings.NewReader("# Plan")), "text/markdown", nil)

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
			Input: `{"path":""}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		result := decodeProposePlanResponse(t, resp)
		assert.Equal(t, chatPlanPath, result.Path)
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
		assert.Contains(t, resp.Content, "empty")
		assert.Contains(t, resp.Content, chatPlanPath)
		assert.False(t, storeCalled)
		assert.Nil(t, *stored)
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
