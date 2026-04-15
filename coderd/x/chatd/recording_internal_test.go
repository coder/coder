package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/textproto"
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

// partSpec describes a single part for buildMultipartResponse.
type partSpec struct {
	contentType string
	data        []byte
}

// buildMultipartResponse constructs a StopDesktopRecordingResponse
// with the given content type/data pairs encoded as multipart/mixed.
func buildMultipartResponse(parts ...partSpec) workspacesdk.StopDesktopRecordingResponse {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		partWriter, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type": {p.contentType},
		})
		_, _ = partWriter.Write(p.data)
	}
	_ = mw.Close()
	return workspacesdk.StopDesktopRecordingResponse{
		Body:        io.NopCloser(bytes.NewReader(buf.Bytes())),
		ContentType: "multipart/mixed; boundary=" + mw.Boundary(),
	}
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
	org database.Organization,
	model database.ChatModelConfig,
	workspace database.WorkspaceTable,
	agent database.WorkspaceAgent,
	parentTitle, childTitle string,
) (parent, child database.Chat) {
	t.Helper()

	// Insert the parent chat directly via DB to avoid triggering
	// the server's background processing.
	parent, err := server.db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
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
		OrganizationID:    org.ID,
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

	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create the server WITHOUT agentConnFn so the background
	// processing of the parent chat doesn't use the mock.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createComputerUseParentChild(
		ctx, t, server, user, org, model, workspace, agent,
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
		Return(buildMultipartResponse(partSpec{"video/mp4", fakeMp4}), nil).Times(1)

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

// TestWaitAgentComputerUseRecordingWithThumbnail verifies the
// recording flow when the agent produces both video and thumbnail:
// both file IDs appear in the wait_agent tool response.
func TestWaitAgentComputerUseRecordingWithThumbnail(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createComputerUseParentChild(
		ctx, t, server, user, org, model, workspace, agent,
		"parent-recording-thumb", "computer-use-child-thumb",
	)

	server.drainInflight()

	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		require.Equal(t, agent.ID, agentID)
		return mockConn, func() {}, nil
	}

	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "I opened Firefox and took a screenshot.")

	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	fakeMp4 := []byte("fake-mp4-data-with-thumbnail-test")
	fakeThumb := []byte("fake-jpeg-thumbnail-data")

	mockConn.EXPECT().
		StartDesktopRecording(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req workspacesdk.StartDesktopRecordingRequest) error {
			require.NotEmpty(t, req.RecordingID)
			return nil
		}).
		Times(1)

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(buildMultipartResponse(
			partSpec{"video/mp4", fakeMp4},
			partSpec{"image/jpeg", fakeThumb},
		), nil).Times(1)

	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))

	// Verify recording_file_id is present and valid.
	storedFileID, ok := result["recording_file_id"].(string)
	require.True(t, ok, "recording_file_id must be present in response")
	require.NotEmpty(t, storedFileID)
	fileUUID, err := uuid.Parse(storedFileID)
	require.NoError(t, err)
	chatFile, err := db.GetChatFileByID(ctx, fileUUID)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", chatFile.Mimetype)
	assert.Equal(t, fakeMp4, chatFile.Data)

	// Verify thumbnail_file_id is present and valid.
	thumbFileID, ok := result["thumbnail_file_id"].(string)
	require.True(t, ok, "thumbnail_file_id must be present in response")
	require.NotEmpty(t, thumbFileID)
	thumbUUID, err := uuid.Parse(thumbFileID)
	require.NoError(t, err)
	thumbFile, err := db.GetChatFileByID(ctx, thumbUUID)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", thumbFile.Mimetype)
	assert.Equal(t, fakeThumb, thumbFile.Data)
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

	user, org, model := seedInternalChatDeps(ctx, t, db)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create parent and regular (non-computer_use) child.
	parent, child := createParentChildChats(ctx, t, server, user, org, model)

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

	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create the server WITHOUT agentConnFn so the background
	// processing of the parent chat doesn't use the mock.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create parent + computer_use child.
	parent, child := createComputerUseParentChild(
		ctx, t, server, user, org, model, workspace, agent,
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

	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create the server WITHOUT agentConnFn so the background
	// processing of the parent chat doesn't use the mock.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Create parent + computer_use child.
	parent, child := createComputerUseParentChild(
		ctx, t, server, user, org, model, workspace, agent,
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
		Return(workspacesdk.StopDesktopRecordingResponse{}, xerrors.New("disk full")).
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

	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)

	// Create parent + computer_use child.
	_, child := createComputerUseParentChild(
		ctx, t, server, user, org, model, workspace, agent,
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

// TestStopAndStoreRecording_Oversized verifies that when the
// recording data exceeds MaxRecordingSize, stopAndStoreRecording
// returns an empty string and does NOT call InsertChatFile.
func TestStopAndStoreRecording_Oversized(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Build a streaming multipart response with a video/mp4 part
	// that exceeds MaxRecordingSize without allocating the full
	// buffer in memory.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		partWriter, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type": {"video/mp4"},
		})
		// Stream MaxRecordingSize+1 zero bytes.
		_, _ = io.Copy(partWriter, io.LimitReader(&zeroReader{}, int64(workspacesdk.MaxRecordingSize+1)))
		_ = mw.Close()
		_ = pw.Close()
	}()

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StopDesktopRecordingResponse{
			Body:        pr,
			ContentType: "multipart/mixed; boundary=" + mw.Boundary(),
		}, nil).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)
	assert.Empty(t, result.recordingFileID, "oversized recording should not be stored")
}

// TestStopAndStoreRecording_OversizedThumbnail verifies that when the
// thumbnail part exceeds MaxThumbnailSize it is skipped while the
// normal-sized video part is still stored.
func TestStopAndStoreRecording_OversizedThumbnail(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	videoData := bytes.Repeat([]byte{0xAA}, 1024)

	// Build a streaming multipart response with a normal video part
	// and an oversized thumbnail part.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		vw, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type": {"video/mp4"},
		})
		_, _ = vw.Write(videoData)
		tw, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type": {"image/jpeg"},
		})
		// Stream MaxThumbnailSize+1 zero bytes for the thumbnail.
		_, _ = io.Copy(tw, io.LimitReader(&zeroReader{}, int64(workspacesdk.MaxThumbnailSize+1)))
		_ = mw.Close()
		_ = pw.Close()
	}()

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StopDesktopRecordingResponse{
			Body:        pr,
			ContentType: "multipart/mixed; boundary=" + mw.Boundary(),
		}, nil).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	// Video should be stored.
	recUUID, err := uuid.Parse(result.recordingFileID)
	require.NoError(t, err, "RecordingFileID should be a valid UUID")
	recFile, err := db.GetChatFileByID(ctx, recUUID)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", recFile.Mimetype)
	assert.Equal(t, videoData, recFile.Data)

	// Thumbnail should be skipped (oversized).
	assert.Empty(t, result.thumbnailFileID, "oversized thumbnail should not be stored")
}

// TestStopAndStoreRecording_DuplicatePartsIgnored verifies that when
// a multipart response contains two video/mp4 parts, only the first
// is stored and the duplicate is skipped.
func TestStopAndStoreRecording_DuplicatePartsIgnored(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	firstVideo := bytes.Repeat([]byte{0x01}, 512)
	secondVideo := bytes.Repeat([]byte{0x02}, 512)

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(buildMultipartResponse(
			partSpec{"video/mp4", firstVideo},
			partSpec{"video/mp4", secondVideo},
		), nil).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	// Only the first video part should be stored.
	recUUID, err := uuid.Parse(result.recordingFileID)
	require.NoError(t, err)
	recFile, err := db.GetChatFileByID(ctx, recUUID)
	require.NoError(t, err)
	assert.Equal(t, firstVideo, recFile.Data, "first video part should be stored, not the duplicate")
}

// TestStopAndStoreRecording_Empty verifies that when the recording
// data is empty, stopAndStoreRecording returns an empty string and
// does NOT call InsertChatFile.
func TestStopAndStoreRecording_Empty(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// Build a multipart response with an empty video/mp4 part.
	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(buildMultipartResponse(partSpec{"video/mp4", nil}), nil).Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)
	assert.Empty(t, result.recordingFileID, "empty recording should not be stored")
}

// TestStopAndStoreRecording_WithThumbnail verifies that a multipart
// response containing both a video/mp4 part and an image/jpeg part
// results in both files being stored with correct mimetypes.
func TestStopAndStoreRecording_WithThumbnail(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	videoData := bytes.Repeat([]byte{0xDE, 0xAD}, 512) // 1024 bytes
	thumbData := bytes.Repeat([]byte{0xFF, 0xD8}, 256) // 512 bytes

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(buildMultipartResponse(
			partSpec{"video/mp4", videoData},
			partSpec{"image/jpeg", thumbData},
		), nil).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	// Both file IDs should be valid UUIDs.
	recUUID, err := uuid.Parse(result.recordingFileID)
	require.NoError(t, err, "RecordingFileID should be a valid UUID")

	thumbUUID, err := uuid.Parse(result.thumbnailFileID)
	require.NoError(t, err, "ThumbnailFileID should be a valid UUID")
	// Verify the recording file in the database.
	recFile, err := db.GetChatFileByID(ctx, recUUID)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", recFile.Mimetype)
	assert.Equal(t, videoData, recFile.Data)

	// Verify the thumbnail file in the database.
	thumbFile, err := db.GetChatFileByID(ctx, thumbUUID)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", thumbFile.Mimetype)
	assert.Equal(t, thumbData, thumbFile.Data)
}

// TestStopAndStoreRecording_VideoOnly verifies that a multipart
// response with only a video/mp4 part stores the recording but
// leaves thumbnailFileID empty.
func TestStopAndStoreRecording_VideoOnly(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	videoData := make([]byte, 1024)

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(buildMultipartResponse(partSpec{"video/mp4", videoData}), nil).Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	// Recording should be stored.
	recUUID, err := uuid.Parse(result.recordingFileID)
	require.NoError(t, err, "RecordingFileID should be a valid UUID")

	recFile, err := db.GetChatFileByID(ctx, recUUID)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", recFile.Mimetype)
	assert.Equal(t, videoData, recFile.Data)

	// No thumbnail.
	assert.Empty(t, result.thumbnailFileID, "ThumbnailFileID should be empty when no thumbnail part is present")
}

// TestStopAndStoreRecording_DownloadFailure verifies that when
// StopDesktopRecording returns an error, stopAndStoreRecording
// returns an empty recordingResult without panicking.
func TestStopAndStoreRecording_DownloadFailure(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StopDesktopRecordingResponse{}, xerrors.New("network error")).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	assert.Empty(t, result.recordingFileID, "RecordingFileID should be empty on download failure")
	assert.Empty(t, result.thumbnailFileID, "ThumbnailFileID should be empty on download failure")
}

// TestStopAndStoreRecording_UnknownPartIgnored verifies that parts
// with unrecognized content types are silently skipped while known
// parts (video/mp4 and image/jpeg) are still stored.
func TestStopAndStoreRecording_UnknownPartIgnored(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	videoData := make([]byte, 1024)
	thumbData := make([]byte, 512)
	unknownData := make([]byte, 256)

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(buildMultipartResponse(
			partSpec{"video/mp4", videoData},
			partSpec{"image/jpeg", thumbData},
			partSpec{"application/octet-stream", unknownData},
		), nil).Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	// Both known parts should be stored.
	recUUID, err := uuid.Parse(result.recordingFileID)
	require.NoError(t, err, "RecordingFileID should be a valid UUID")

	thumbUUID, err := uuid.Parse(result.thumbnailFileID)
	require.NoError(t, err, "ThumbnailFileID should be a valid UUID")

	// Verify only 2 files exist (unknown part was skipped).
	recFile, err := db.GetChatFileByID(ctx, recUUID)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", recFile.Mimetype)
	assert.Equal(t, videoData, recFile.Data)

	thumbFile, err := db.GetChatFileByID(ctx, thumbUUID)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", thumbFile.Mimetype)
	assert.Equal(t, thumbData, thumbFile.Data)
}

// TestStopAndStoreRecording_MalformedContentType verifies that a
// response with an unparseable Content-Type returns an empty result.
func TestStopAndStoreRecording_MalformedContentType(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StopDesktopRecordingResponse{
			Body:        io.NopCloser(bytes.NewReader(nil)),
			ContentType: "",
		}, nil).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	assert.Empty(t, result.recordingFileID, "RecordingFileID should be empty for malformed content type")
	assert.Empty(t, result.thumbnailFileID, "ThumbnailFileID should be empty for malformed content type")
}

// TestStopAndStoreRecording_MissingBoundary verifies that a
// multipart response without a boundary parameter returns an empty
// result.
func TestStopAndStoreRecording_MissingBoundary(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	user, _, _ := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	mockConn.EXPECT().
		StopDesktopRecording(gomock.Any(), gomock.Any()).
		Return(workspacesdk.StopDesktopRecordingResponse{
			Body:        io.NopCloser(bytes.NewReader(nil)),
			ContentType: "multipart/mixed",
		}, nil).
		Times(1)

	recordingID := uuid.New().String()
	result := server.stopAndStoreRecording(
		ctx, mockConn, recordingID, user.ID,
		uuid.NullUUID{UUID: workspace.ID, Valid: true},
	)

	assert.Empty(t, result.recordingFileID, "RecordingFileID should be empty when boundary is missing")
	assert.Empty(t, result.thumbnailFileID, "ThumbnailFileID should be empty when boundary is missing")
}
