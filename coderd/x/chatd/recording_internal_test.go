package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// zeroReader is an io.Reader that produces zero-valued bytes
// without allocating large buffers.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// createComputerUseParentChild creates a parent chat and a
// computer_use child chat bound to the given workspace/agent.
// Both chats are inserted directly via DB to avoid triggering
// background processing (which would try to call the LLM and
// use the agent connection mock).
func createComputerUseParentChild(
	ctx context.Context,
	t *testing.T,
	server *Server,
	user database.User,
	model database.ChatModelConfig,
	workspace database.WorkspaceTable,
	agent database.WorkspaceAgent,
	parentTitle, childTitle string,
) (parent, child database.Chat) {
	t.Helper()

	// Insert the parent chat directly via DB to avoid triggering
	// the server's background processing.
	parent, err := server.db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agent.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             parentTitle,
		Status:            database.ChatStatusPending,
	})
	require.NoError(t, err)

	// Insert the child chat directly via DB to avoid triggering
	// the server's background processing (which would try to run
	// the chat without an LLM and get stuck).
	child, err = server.db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agent.ID, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             childTitle,
		Mode:              database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		Status:            database.ChatStatusPending,
	})
	require.NoError(t, err)

	return parent, child
}

// invokeWaitAgentTool builds the wait_agent tool from the server and
// invokes it with the given child chat ID and timeout.
func invokeWaitAgentTool(
	ctx context.Context,
	t *testing.T,
	server *Server,
	db database.Store,
	parentID uuid.UUID,
	childID uuid.UUID,
	timeoutSeconds int,
) (fantasy.ToolResponse, error) {
	t.Helper()

	// Re-fetch the parent so LastModelConfigID is populated.
	parentChat, err := db.GetChatByID(ctx, parentID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "wait_agent")
	require.NotNil(t, tool, "wait_agent tool must be present")

	argsJSON, err := json.Marshal(map[string]any{
		"chat_id":         childID.String(),
		"timeout_seconds": timeoutSeconds,
	})
	require.NoError(t, err)

	return tool.Run(ctx, fantasy.ToolCall{
		ID:    "test-call",
		Name:  "wait_agent",
		Input: string(argsJSON),
	})
}

// TestWaitAgentComputerUseRecording verifies the happy-path recording
// flow: for a computer_use child chat that completes successfully,
// the recording is stopped, the MP4 is stored in chat_files, and the
// file ID is returned.
func TestWaitAgentComputerUseRecording(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create the server WITHOUT agentConnFn so the background
	// processing of the parent chat doesn't use the mock.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createComputerUseParentChild(
		ctx, t, server, user, model, workspace, agent,
		"parent-recording", "computer-use-child",
	)

	// Wait for background processing triggered by CreateChat to
	// settle before setting up the mock agent connection.
	server.drainInflight()

	// Now wire up the mock agent connection.
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		require.Equal(t, agent.ID, agentID)
		return mockConn, func() {}, nil
	}

	// Add an assistant message so the report is extracted.
	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "I opened Firefox.")

	// Set child to waiting (terminal success state).
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	// Set up mock expectations for start and stop.
	fakeMp4 := []byte("fake-mp4-data-for-recording-test")

	mockConn.EXPECT().
		StartDesktopRecording(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req workspacesdk.StartDesktopRecordingRequest) error {
			require.NotEmpty(t, req.RecordingID, "recording ID should be non-empty")
			return nil
		}).
		Times(1)

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(fakeMp4)), nil).
		Times(1)

	// Invoke wait_agent via the tool closure.
	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	// Parse the response JSON and check for recording_file_id.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	storedFileID, ok := result["recording_file_id"].(string)
	require.True(t, ok, "recording_file_id must be present in response")
	require.NotEmpty(t, storedFileID)

	// Verify the file was inserted into the database.
	fileUUID, err := uuid.Parse(storedFileID)
	require.NoError(t, err)

	chatFile, err := db.GetChatFileByID(ctx, fileUUID)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", chatFile.Mimetype)
	assert.True(t, strings.HasPrefix(chatFile.Name, "recording-"),
		"expected name to start with 'recording-', got: %s", chatFile.Name)
	assert.Equal(t, user.ID, chatFile.OwnerID)
	assert.Equal(t, fakeMp4, chatFile.Data)
}

// TestWaitAgentNonComputerUseNoRecording verifies that when the
// child chat is NOT a computer_use chat, no recording is attempted.
// StartDesktopRecording must never be called.
func TestWaitAgentNonComputerUseNoRecording(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, model := seedInternalChatDeps(ctx, t, db)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create parent and regular (non-computer_use) child.
	parent, child := createParentChildChats(ctx, t, server, user, model)

	// Add an assistant message so the report is extracted.
	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "Done.")

	// Wait for background processing triggered by CreateChat to
	// settle before setting up the mock agent connection.
	server.drainInflight()

	// Wire up the mock agent connection. The mock has zero
	// expectations — gomock will fail if StartDesktopRecording
	// or any other method is called.
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return mockConn, func() {}, nil
	}

	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	// Invoke wait_agent via the tool closure — the isComputerUseChat
	// guard should be false, so no recording calls fire.
	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	// Parse the response JSON and verify no recording_file_id.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	_, hasRecording := result["recording_file_id"]
	assert.False(t, hasRecording, "non-computer_use chat should not produce recording_file_id")
}

// TestWaitAgentRecordingStartFails verifies that when
// StartDesktopRecording returns an error, the wait_agent flow still
// succeeds and no recording_id is produced. StopDesktopRecording
// must NOT be called since the recordingID is cleared on start
// failure.
func TestWaitAgentRecordingStartFails(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create the server WITHOUT agentConnFn so the background
	// processing of the parent chat doesn't use the mock.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create parent + computer_use child.
	parent, child := createComputerUseParentChild(
		ctx, t, server, user, model, workspace, agent,
		"parent-start-fail", "computer-use-start-fail",
	)

	// Now wire up the mock agent connection.
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return mockConn, func() {}, nil
	}

	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "Opened the browser.")
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	// StartDesktopRecording fails. StopDesktopRecording must NOT
	// be called — gomock enforces this: any unexpected call fails
	// the test.
	mockConn.EXPECT().
		StartDesktopRecording(gomock.Any(), gomock.Any()).
		Return(xerrors.New("ffmpeg not found")).
		Times(1)

	// Invoke wait_agent via the tool closure.
	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "recording failure is best-effort, tool should succeed")

	// Parse response JSON and assert no recording_file_id.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	_, hasRecording := result["recording_file_id"]
	assert.False(t, hasRecording, "no recording_file_id when start fails")
}

// TestWaitAgentRecordingStopFails verifies that when
// StopDesktopRecording returns an error, the wait_agent flow still
// succeeds but no recording_id is produced.
func TestWaitAgentRecordingStopFails(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create the server WITHOUT agentConnFn so the background
	// processing of the parent chat doesn't use the mock.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create parent + computer_use child.
	parent, child := createComputerUseParentChild(
		ctx, t, server, user, model, workspace, agent,
		"parent-stop-fail", "computer-use-stop-fail",
	)

	// Now wire up the mock agent connection.
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return mockConn, func() {}, nil
	}

	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "Checked settings.")
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	// Start succeeds, stop fails.
	mockConn.EXPECT().
		StartDesktopRecording(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(nil, xerrors.New("disk full")).
		Times(1)

	// Invoke wait_agent via the tool closure.
	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "recording failure is best-effort, tool should succeed")

	// Parse response JSON and assert no recording_file_id.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	_, hasRecording := result["recording_file_id"]
	assert.False(t, hasRecording, "no recording_file_id when stop fails")
}

// TestWaitAgentTimeoutLeavesRecordingRunning verifies that when the
// subagent times out, StopDesktopRecording is NOT called. The
// recording is left running on the agent so the next wait_agent
// call continues it seamlessly.
func TestWaitAgentTimeoutLeavesRecordingRunning(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	mClock := quartz.NewMock(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	// Use the mock clock server; don't set agentConnFn yet.
	server := newInternalTestServerWithClock(t, db, ps, chatprovider.ProviderAPIKeys{}, mClock)

	user, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create parent + computer_use child.
	_, child := createComputerUseParentChild(
		ctx, t, server, user, model, workspace, agent,
		"parent-timeout", "computer-use-timeout",
	)

	// Set child to running so it never completes.
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

	// Now wire up the mock agent connection.
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return mockConn, func() {}, nil
	}

	// Start recording succeeds.
	mockConn.EXPECT().
		StartDesktopRecording(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// StopDesktopRecording must NOT be called on timeout.
	// gomock enforces this: any unexpected call fails the test.

	// Trap the timeout timer to know when the function has entered
	// its poll loop.
	timerTrap := mClock.Trap().NewTimer("chatd", "subagent_await")

	type toolResult struct {
		resp fantasy.ToolResponse
		err  error
	}
	resultCh := make(chan toolResult, 1)

	// Re-fetch the parent so LastModelConfigID is populated.
	parentChat, err := db.GetChatByID(ctx, child.ParentChatID.UUID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "wait_agent")
	require.NotNil(t, tool, "wait_agent tool must be present")

	argsJSON, err := json.Marshal(map[string]any{
		"chat_id":         child.ID.String(),
		"timeout_seconds": 1,
	})
	require.NoError(t, err)

	go func() {
		resp, runErr := tool.Run(ctx, fantasy.ToolCall{
			ID:    "test-timeout-call",
			Name:  "wait_agent",
			Input: string(argsJSON),
		})
		resultCh <- toolResult{resp: resp, err: runErr}
	}()

	// Wait for the timer to be created, then release it.
	timerTrap.MustWait(ctx).MustRelease(ctx)
	timerTrap.Close()

	// Advance past the 1s timeout.
	mClock.Advance(time.Second).MustWait(ctx)

	result := testutil.RequireReceive(ctx, t, resultCh)
	require.NoError(t, result.err)
	assert.True(t, result.resp.IsError, "expected error response on timeout")
	assert.Contains(t, result.resp.Content, "timed out")
}

// TestStopAndStoreRecordingOversized verifies that when the recording
// data exceeds MaxRecordingSize, stopAndStoreRecording returns an
// empty string and does NOT call InsertChatFile.
func TestStopAndStoreRecordingOversized(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create a reader that produces MaxRecordingSize+1 bytes without
	// allocating the full buffer in memory.
	oversizedReader := io.LimitReader(
		&zeroReader{},
		int64(workspacesdk.MaxRecordingSize+1),
	)
	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(io.NopCloser(oversizedReader), nil).
		Times(1)

	recordingID := uuid.New().String()
	storedFileID := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)
	assert.Empty(t, storedFileID, "oversized recording should not be stored")
}

// TestStopAndStoreRecordingEmpty verifies that when the recording
// data is empty, stopAndStoreRecording returns an empty string and
// does NOT call InsertChatFile.
func TestStopAndStoreRecordingEmpty(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Return empty data.
	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(nil)), nil).
		Times(1)

	recordingID := uuid.New().String()
	storedFileID := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)
	assert.Empty(t, storedFileID, "empty recording should not be stored")
}
