package chatd

import (
	"context"
	"net/http"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// statusServer returns a server on the fixture's clock whose own worker
// delivers through the fake agent, so the helper's pokes reach it.
func (f workspaceFileDeliveryFixture) statusServer(t *testing.T) *Server {
	t.Helper()
	server := newUnstartedServer(t, f.pubsub, f.db, withInternalTestServerClock(f.clock))
	server.agentInactiveDisconnectTimeout = time.Hour
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return f.agent, func() {}, nil
	}
	return server
}

func (f workspaceFileDeliveryFixture) insertMessage(t *testing.T, chatID uuid.UUID, msg database.ChatMessage, parts ...codersdk.ChatMessagePart) {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	msg.ChatID = chatID
	msg.Content = content
	msg.ModelConfigID = uuid.NullUUID{UUID: f.model.ID, Valid: true}
	dbgen.ChatMessage(t, f.db, msg)
}

func (f workspaceFileDeliveryFixture) attach(t *testing.T, chatID uuid.UUID, fileIDs ...uuid.UUID) {
	t.Helper()
	parts := make([]codersdk.ChatMessagePart, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		parts = append(parts, codersdk.ChatMessageWorkspaceAttachment(fileID, "application/zip", "report.zip", int64(len("zip bytes"))))
	}
	f.insertMessage(t, chatID, database.ChatMessage{
		Role:      database.ChatMessageRoleUser,
		CreatedBy: uuid.NullUUID{UUID: f.user.ID, Valid: true},
	}, parts...)
}

func (f workspaceFileDeliveryFixture) setAgentFailure(fail func(workspacesdk.UploadChatFileRequest) error) {
	f.agent.mu.Lock()
	defer f.agent.mu.Unlock()
	f.agent.fail = fail
}

func deliveredHomePath(chatID, fileID uuid.UUID) string {
	return path.Join("~/.coder/chats", chatID.String(), "files", fileID.String(), "report.zip")
}

// awaitInBackground runs the helper and advances the clock one poll at a
// time until it returns.
func awaitInBackground(ctx context.Context, t *testing.T, f workspaceFileDeliveryFixture, server *Server, chatID uuid.UUID, mode workspaceFilesAwaitMode) chattool.WorkspaceAttachmentsReport {
	t.Helper()
	done := make(chan chattool.WorkspaceAttachmentsReport, 1)
	go func() {
		done <- server.awaitChatWorkspaceFiles(ctx, chatID, mode)
	}()
	return pollUntilReported(ctx, t, f, done)
}

func pollUntilReported(ctx context.Context, t *testing.T, f workspaceFileDeliveryFixture, done <-chan chattool.WorkspaceAttachmentsReport) chattool.WorkspaceAttachmentsReport {
	t.Helper()
	for {
		select {
		case report := <-done:
			return report
		case <-ctx.Done():
			t.Fatal("timed out waiting for the workspace attachment report")
		case <-time.After(testutil.IntervalFast):
			f.clock.Advance(workspaceFileStatusPollInterval).MustWait(ctx)
		}
	}
}

func TestAwaitChatWorkspaceFiles(t *testing.T) {
	t.Parallel()

	t.Run("RetryCopiesParkedFileAndReplayReportsIt", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.statusServer(t)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		f.attach(t, chat.ID, file.ID)

		f.setAgentFailure(func(workspacesdk.UploadChatFileRequest) error {
			return agentStatusError(http.StatusForbidden)
		})
		server.chatWorker.deliverWorkspaceFiles(ctx)

		// Reporting alone leaves the parked file alone.
		report := server.awaitChatWorkspaceFiles(ctx, chat.ID, workspaceFilesReport)
		require.NoError(t, report.Err)
		require.Len(t, report.Attachments, 1)
		require.Equal(t, chattool.WorkspaceAttachmentFailed, report.Attachments[0].Status)
		require.Contains(t, report.Attachments[0].Error, "agent says no")

		f.setAgentFailure(nil)
		trap := f.clock.Trap().NewTicker("chatworker", "workspace-file-delivery")
		loopCtx, cancel := context.WithCancel(ctx)
		loopDone := make(chan struct{})
		go func() {
			defer close(loopDone)
			server.chatWorker.workspaceFileDeliveryLoop(loopCtx)
		}()
		t.Cleanup(func() {
			cancel()
			<-loopDone
		})
		trap.MustWait(ctx).MustRelease(ctx)
		trap.Close()

		report = awaitInBackground(ctx, t, f, server, chat.ID, workspaceFilesRetry)
		require.NoError(t, report.Err)
		want := chattool.WorkspaceAttachment{
			FileID:      file.ID.String(),
			Name:        "report.zip",
			MediaType:   "application/zip",
			SizeBytes:   int64(len("zip bytes")),
			Status:      chattool.WorkspaceAttachmentCopied,
			Path:        deliveredHomePath(chat.ID, file.ID),
			WorkspaceID: chat.WorkspaceID.UUID.String(),
		}
		require.Equal(t, []chattool.WorkspaceAttachment{want}, report.Attachments)
		require.Len(t, f.agent.uploadRequests(), 2)

		// A replayed tool call reports the copy without uploading again.
		report = server.awaitChatWorkspaceFiles(ctx, chat.ID, workspaceFilesRetry)
		require.NoError(t, report.Err)
		require.Equal(t, []chattool.WorkspaceAttachment{want}, report.Attachments)
		require.Len(t, f.agent.uploadRequests(), 2)
	})

	t.Run("RetryWakesDeliveryWithoutASweep", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.statusServer(t)
		chat, agent := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateStarting})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		f.attach(t, chat.ID, file.ID)

		trap := f.clock.Trap().NewTicker("chatworker", "workspace-file-delivery")
		loopCtx, cancel := context.WithCancel(ctx)
		loopDone := make(chan struct{})
		go func() {
			defer close(loopDone)
			server.chatWorker.workspaceFileDeliveryLoop(loopCtx)
		}()
		t.Cleanup(func() {
			cancel()
			<-loopDone
		})
		trap.MustWait(ctx).MustRelease(ctx)
		trap.Close()

		// Startup finished without an event reaching the loop, and the
		// clock never reaches the sweep, so only the poke delivers.
		f.setLifecycle(ctx, t, agent.ID, database.WorkspaceAgentLifecycleStateReady)
		done := make(chan chattool.WorkspaceAttachmentsReport, 1)
		go func() {
			done <- server.awaitChatWorkspaceFiles(ctx, chat.ID, workspaceFilesRetry)
		}()
		f.requireDelivered(ctx, t, chat, file.ID)
		report := pollUntilReported(ctx, t, f, done)
		require.NoError(t, report.Err)
		require.Len(t, report.Attachments, 1)
		require.Equal(t, chattool.WorkspaceAttachmentCopied, report.Attachments[0].Status)
	})

	t.Run("ReportsEachStatus", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.statusServer(t)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})

		copied := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		server.chatWorker.deliverWorkspaceFiles(ctx)

		f.setAgentFailure(func(workspacesdk.UploadChatFileRequest) error {
			return agentStatusError(http.StatusForbidden)
		})
		failed := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		server.chatWorker.deliverWorkspaceFiles(ctx)

		// Another replica holds this file's lease on its last attempt
		// when the wait ends.
		copying := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		now := f.clock.Now()
		for attempt := int32(1); attempt <= database.ChatWorkspaceFileMaxDeliveryAttempts; attempt++ {
			claimed, err := f.db.AcquireChatWorkspaceFileDeliveries(ctx, database.AcquireChatWorkspaceFileDeliveriesParams{
				Now:             now,
				LeaseExpiresAt:  now.Add(workspaceFileDeliveryLease),
				ChatAgentSuffix: agentselect.Suffix,
				MaxAttempts:     database.ChatWorkspaceFileMaxDeliveryAttempts,
				ChatID:          uuid.NullUUID{UUID: chat.ID, Valid: true},
				LimitVal:        1,
			})
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.Equal(t, copying.ID, claimed[0].ID)
			require.Equal(t, attempt, claimed[0].WorkspaceDeliveryAttempts)
			if attempt == database.ChatWorkspaceFileMaxDeliveryAttempts {
				break
			}
			_, err = f.db.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
				ID:            copying.ID,
				Error:         "agent unavailable",
				NextAttemptAt: now,
				Mode:          database.ChatWorkspaceFileDeliveryFailureRetry,
				MaxAttempts:   database.ChatWorkspaceFileMaxDeliveryAttempts,
			})
			require.NoError(t, err)
		}

		evicted := uuid.New()
		f.attach(t, chat.ID, copied.ID, failed.ID, copying.ID, evicted)
		// Compaction appends a summary and keeps the earlier rows.
		f.insertMessage(t, chat.ID, database.ChatMessage{
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityModel,
			Compressed: true,
		}, codersdk.ChatMessageText("summary of the conversation"))
		f.insertMessage(t, chat.ID, database.ChatMessage{
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityUser,
			Compressed: true,
		}, codersdk.ChatMessageText("compacted"))
		f.insertMessage(t, chat.ID, database.ChatMessage{
			Role:      database.ChatMessageRoleUser,
			CreatedBy: uuid.NullUUID{UUID: f.user.ID, Valid: true},
		}, codersdk.ChatMessageText("unpack the report"))

		report := awaitInBackground(ctx, t, f, server, chat.ID, workspaceFilesReport)
		require.NoError(t, report.Err)
		statuses := make(map[string]chattool.WorkspaceAttachment, len(report.Attachments))
		for _, attachment := range report.Attachments {
			statuses[attachment.FileID] = attachment
		}
		require.Len(t, statuses, 4)
		require.Equal(t, chattool.WorkspaceAttachmentCopied, statuses[copied.ID.String()].Status)
		require.Equal(t, deliveredHomePath(chat.ID, copied.ID), statuses[copied.ID.String()].Path)
		require.Equal(t, chattool.WorkspaceAttachmentFailed, statuses[failed.ID.String()].Status)
		require.Contains(t, statuses[failed.ID.String()].Error, "agent says no")
		require.Equal(t, chattool.WorkspaceAttachmentCopying, statuses[copying.ID.String()].Status)
		require.Equal(t, chattool.WorkspaceAttachmentUnavailable, statuses[evicted.String()].Status)
		require.WithinDuration(t, now.Add(workspaceFileStatusMaxWait), f.clock.Now(), workspaceFileStatusPollInterval)
	})

	t.Run("PendingUntilStartupFinishes", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.statusServer(t)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateStarting})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		f.attach(t, chat.ID, file.ID)

		// Nothing is copied before startup finishes, so the helper
		// reports without waiting on the clock.
		report := server.awaitChatWorkspaceFiles(ctx, chat.ID, workspaceFilesRetry)
		require.NoError(t, report.Err)
		require.Len(t, report.Attachments, 1)
		require.Equal(t, chattool.WorkspaceAttachmentPending, report.Attachments[0].Status)
		require.Empty(t, f.agent.uploadRequests())
	})
}
