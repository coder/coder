package chatd

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestWorker_AutoArchiveDisabled(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, codersdk.DefaultChatAutoArchiveDays))

	pubsub := newRecordingPubsub(f.pubsub)
	worker := f.newArchiveWorker(t, pubsub, nil, nil)
	worker.archiveOnce(ctx, now)

	refreshed, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, refreshed.Archived)
	require.Empty(t, pubsub.watchEvents(t))
	require.Empty(t, pubsub.stateUpdateMessages(t, chat.ID))
}

func TestWorker_AutoArchivesInactiveRoot(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	insertArchiveMessage(t, f, chat.ID, now.Add(-100*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))
	require.NoError(t, f.db.UpsertChatRetentionDays(ctx, 30))

	pubsub := newRecordingPubsub(f.pubsub)
	auditor := audit.NewMock()
	enqueuer := notificationstest.NewFakeEnqueuer()
	worker := f.newArchiveWorker(t, pubsub, mockAuditorPtr(auditor), enqueuer)
	worker.archiveOnce(ctx, now)

	refreshed, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, refreshed.Archived)
	require.Greater(t, refreshed.SnapshotVersion, chat.SnapshotVersion)

	updates := pubsub.stateUpdateMessages(t, chat.ID)
	require.NotEmpty(t, updates)
	require.True(t, updates[len(updates)-1].Archived)
	requireWatchEvent(t, pubsub, chat.ID, codersdk.ChatWatchEventKindDeleted)

	logs := auditor.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, chat.ID, logs[0].ResourceID)
	require.Equal(t, database.ResourceTypeChat, logs[0].ResourceType)
	require.Equal(t, database.AuditActionWrite, logs[0].Action)
	require.Contains(t, string(logs[0].AdditionalFields), string(audit.BackgroundSubsystemChatAutoArchive))

	sent := enqueuer.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, notifications.TemplateChatAutoArchiveDigest, sent[0].TemplateID)
	require.Equal(t, f.user.ID, sent[0].UserID)
	require.Equal(t, "90", sent[0].Data["auto_archive_days"])
	require.Equal(t, "30", sent[0].Data["retention_days"])
}

func TestWorker_AutoArchiveRejectsActiveChild(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	root := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	child := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	f.linkChild(t, root.ID, child.ID)
	forceExecutionState(t, f, child.ID, database.ChatStatusRunning, false)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	pubsub := newRecordingPubsub(f.pubsub)
	worker := f.newArchiveWorker(t, pubsub, nil, nil)
	worker.archiveOnce(ctx, now)

	refreshedRoot, err := f.db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.False(t, refreshedRoot.Archived)
	refreshedChild, err := f.db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.False(t, refreshedChild.Archived)
	require.Empty(t, pubsub.watchEvents(t))
}

func TestWorker_AutoArchivePublishesStateUpdatesForFamily(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	root := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	child := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	f.linkChild(t, root.ID, child.ID)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	pubsub := newRecordingPubsub(f.pubsub)
	worker := f.newArchiveWorker(t, pubsub, nil, nil)
	worker.archiveOnce(ctx, now)

	refreshedRoot, err := f.db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.True(t, refreshedRoot.Archived)
	refreshedChild, err := f.db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, refreshedChild.Archived)
	require.NotEmpty(t, pubsub.stateUpdateMessages(t, root.ID))
	require.NotEmpty(t, pubsub.stateUpdateMessages(t, child.ID))
	requireWatchEvent(t, pubsub, root.ID, codersdk.ChatWatchEventKindDeleted)
	requireWatchEvent(t, pubsub, child.ID, codersdk.ChatWatchEventKindDeleted)
}

func TestWorker_AutoArchiveExpectedTransitionFailureDoesNotAbortTick(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	blockedRoot := f.createArchiveCandidate(t, now.Add(-130*24*time.Hour))
	blockedChild := f.createArchiveCandidate(t, now.Add(-130*24*time.Hour))
	f.linkChild(t, blockedRoot.ID, blockedChild.ID)
	forceExecutionState(t, f, blockedChild.ID, database.ChatStatusRunning, false)
	valid := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	pubsub := newRecordingPubsub(f.pubsub)
	worker := f.newArchiveWorker(t, pubsub, nil, nil)
	worker.archiveOnce(ctx, now)

	blockedAfter, err := f.db.GetChatByID(ctx, blockedRoot.ID)
	require.NoError(t, err)
	require.False(t, blockedAfter.Archived)
	validAfter, err := f.db.GetChatByID(ctx, valid.ID)
	require.NoError(t, err)
	require.True(t, validAfter.Archived)
	requireWatchEvent(t, pubsub, valid.ID, codersdk.ChatWatchEventKindDeleted)
}

func TestWorker_AutoArchiveDateBoundary(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	onCutoff := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	insertArchiveMessage(t, f, onCutoff.ID, time.Date(2026, 2, 28, 23, 59, 59, 0, time.UTC))
	beforeCutoff := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	insertArchiveMessage(t, f, beforeCutoff.ID, time.Date(2026, 2, 27, 23, 59, 59, 0, time.UTC))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), nil, nil)
	worker.archiveOnce(ctx, now)

	refreshedOn, err := f.db.GetChatByID(ctx, onCutoff.ID)
	require.NoError(t, err)
	require.False(t, refreshedOn.Archived)
	refreshedBefore, err := f.db.GetChatByID(ctx, beforeCutoff.ID)
	require.NoError(t, err)
	require.True(t, refreshedBefore.Archived)
}

func (f *workerTestFixture) createArchiveCandidate(t *testing.T, createdAt time.Time) database.Chat {
	t.Helper()
	return f.createArchiveCandidateForOwner(t, f.user.ID, createdAt)
}

func (f *workerTestFixture) createArchiveCandidateForOwner(t *testing.T, ownerID uuid.UUID, createdAt time.Time) database.Chat {
	t.Helper()
	chat := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           ownerID,
		LastModelConfigID: f.model.ID,
		Title:             testutil.GetRandomName(t),
		Status:            database.ChatStatusWaiting,
	})
	_, err := f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), "UPDATE chats SET created_at = $1, updated_at = $1 WHERE id = $2", createdAt, chat.ID)
	require.NoError(t, err)
	chat.CreatedAt = createdAt
	chat.UpdatedAt = createdAt
	return chat
}

func (f *workerTestFixture) setPinOrder(t *testing.T, chatID uuid.UUID, order int32) {
	t.Helper()
	_, err := f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), "UPDATE chats SET pin_order = $1 WHERE id = $2", order, chatID)
	require.NoError(t, err)
}

func (f *workerTestFixture) softDeleteMessages(t *testing.T, chatID uuid.UUID) {
	t.Helper()
	_, err := f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), "UPDATE chat_messages SET deleted = true WHERE chat_id = $1", chatID)
	require.NoError(t, err)
}

func (f *workerTestFixture) archived(t *testing.T, chatID uuid.UUID) bool {
	t.Helper()
	chat, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chatID)
	require.NoError(t, err)
	return chat.Archived
}

func (f *workerTestFixture) linkChild(t *testing.T, rootID uuid.UUID, childID uuid.UUID) {
	t.Helper()
	_, err := f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), "UPDATE chats SET parent_chat_id = $1, root_chat_id = $1 WHERE id = $2", rootID, childID)
	require.NoError(t, err)
}

func insertArchiveMessage(t *testing.T, f *workerTestFixture, chatID uuid.UUID, createdAt time.Time) {
	t.Helper()
	msg := dbgen.ChatMessage(t, f.db, database.ChatMessage{
		ChatID:        chatID,
		CreatedBy:     uuid.NullUUID{UUID: f.user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: f.model.ID, Valid: true},
		Role:          database.ChatMessageRoleUser,
	})
	_, err := f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), "UPDATE chat_messages SET created_at = $1 WHERE id = $2", createdAt, msg.ID)
	require.NoError(t, err)
}

func (f *workerTestFixture) newArchiveWorker(
	t *testing.T,
	pubsub *recordingPubsub,
	auditor *atomic.Pointer[audit.Auditor],
	enqueuer *notificationstest.FakeEnqueuer,
) *chatWorker {
	t.Helper()
	if pubsub == nil {
		pubsub = newRecordingPubsub(f.pubsub)
	}
	if enqueuer == nil {
		enqueuer = notificationstest.NewFakeEnqueuer()
	}
	opts := f.archiveWorkerOptions()
	opts.Pubsub = pubsub
	opts.NotificationsEnqueuer = enqueuer
	opts.Auditor = auditor
	return f.newArchiveWorkerWithOptions(t, opts)
}

// archiveWorkerOptions returns a baseline chatWorkerOptions with the long
// intervals and channel sizes the archive tests rely on. Callers override
// Pubsub, Store, Clock, and the dispatch dependencies as needed.
func (f *workerTestFixture) archiveWorkerOptions() chatWorkerOptions {
	return chatWorkerOptions{
		WorkerID:                   uuid.New(),
		Store:                      f.db,
		Logger:                     slog.Make(),
		TaskStarter:                newRecordingTaskStarter(),
		AcquisitionInterval:        time.Hour,
		AcquisitionBatchSize:       10,
		ArchiveInterval:            time.Hour,
		ArchiveBatchSize:           10,
		RunnerSyncInterval:         time.Hour,
		HeartbeatInterval:          time.Hour,
		HeartbeatCleanupInterval:   time.Hour,
		HeartbeatStaleSeconds:      30,
		StateChannelSize:           16,
		RunnerManagerChannelSize:   16,
		AcquisitionWakeChannelSize: 1,
	}
}

func (f *workerTestFixture) newArchiveWorkerWithOptions(t *testing.T, opts chatWorkerOptions) *chatWorker {
	t.Helper()
	if opts.Pubsub == nil {
		opts.Pubsub = newRecordingPubsub(f.pubsub)
	}
	if opts.NotificationsEnqueuer == nil {
		opts.NotificationsEnqueuer = notificationstest.NewFakeEnqueuer()
	}
	// The archive tick dereferences the worker's server for debug cleanup
	// and pubsub events, so give it a real (unstarted) Server. Route the
	// server through the recording pubsub when one is in use so tests can
	// observe server-published watch events.
	serverPS := f.pubsub
	if recording, ok := opts.Pubsub.(*recordingPubsub); ok {
		serverPS = recording
	}
	worker, err := newChatWorker(newUnstartedServer(t, serverPS, f.db), opts)
	require.NoError(t, err)
	return worker
}

func mockAuditorPtr(auditor *audit.MockAuditor) *atomic.Pointer[audit.Auditor] {
	var ptr atomic.Pointer[audit.Auditor]
	var asInterface audit.Auditor = auditor
	ptr.Store(&asInterface)
	return &ptr
}

func requireWatchEvent(t *testing.T, pubsub *recordingPubsub, chatID uuid.UUID, kind codersdk.ChatWatchEventKind) {
	t.Helper()
	for _, event := range pubsub.watchEvents(t) {
		if event.Kind == kind && event.Chat.ID == chatID {
			return
		}
	}
	t.Fatalf("missing watch event kind=%s chat_id=%s", kind, chatID)
}

// Candidate selection (query) semantics.

func TestWorker_AutoArchiveSkipsPinnedRoot(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	f.setPinOrder(t, chat.ID, 1)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), nil, nil)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, chat.ID), "pinned root must not be auto-archived")
}

func TestWorker_AutoArchiveSkipsActiveStatusRoot(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	forceExecutionState(t, f, chat.ID, database.ChatStatusRunning, false)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), nil, nil)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, chat.ID), "running root must not be auto-archived")
}

func TestWorker_AutoArchiveIgnoresDeletedMessages(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	insertArchiveMessage(t, f, chat.ID, now.Add(-10*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), nil, nil)
	worker.archiveOnce(ctx, now)
	require.False(t, f.archived(t, chat.ID), "recent message must keep the chat active")

	// Once the only recent message is soft-deleted, activity falls back to
	// created_at and the chat becomes eligible.
	f.softDeleteMessages(t, chat.ID)
	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, chat.ID), "chat with only deleted messages must archive on created_at")
}

func TestWorker_AutoArchiveChildActivityKeepsRootAlive(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	root := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	child := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	f.linkChild(t, root.ID, child.ID)
	insertArchiveMessage(t, f, child.ID, now.Add(-5*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), nil, nil)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, root.ID), "recent child activity must keep the root alive")
	require.False(t, f.archived(t, child.ID))
}

func TestWorker_AutoArchiveBatchSizeLimitsAndPaginates(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	oldest := f.createArchiveCandidate(t, now.Add(-122*24*time.Hour))
	middle := f.createArchiveCandidate(t, now.Add(-121*24*time.Hour))
	newest := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	opts := f.archiveWorkerOptions()
	opts.Pubsub = newRecordingPubsub(f.pubsub)
	opts.ArchiveBatchSize = 2
	worker := f.newArchiveWorkerWithOptions(t, opts)

	// First tick archives the two oldest roots (created_at ASC, limited).
	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, oldest.ID), "oldest root should archive in the first batch")
	require.True(t, f.archived(t, middle.ID), "middle root should archive in the first batch")
	require.False(t, f.archived(t, newest.ID), "newest root should wait for the next tick")

	// Second tick drains the remaining backlog.
	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, newest.ID), "newest root should archive on the second tick")
}

func TestWorker_AutoArchiveNoEligibleChats(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	// A recent chat is well within the inactivity window.
	chat := f.createArchiveCandidate(t, now.Add(-24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	auditor := audit.NewMock()
	enqueuer := notificationstest.NewFakeEnqueuer()
	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), mockAuditorPtr(auditor), enqueuer)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, chat.ID))
	require.Empty(t, auditor.AuditLogs())
	require.Empty(t, enqueuer.Sent())
}

// Dispatch (audit + digest) semantics.

func TestWorker_AutoArchiveMultipleOwnersGetSeparateDigests(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	user2 := dbgen.User(t, f.db, database.User{})
	chat1 := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	chat2 := f.createArchiveCandidateForOwner(t, user2.ID, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	auditor := audit.NewMock()
	enqueuer := notificationstest.NewFakeEnqueuer()
	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), mockAuditorPtr(auditor), enqueuer)
	worker.archiveOnce(ctx, now)

	require.True(t, f.archived(t, chat1.ID))
	require.True(t, f.archived(t, chat2.ID))

	sent := enqueuer.Sent()
	require.Len(t, sent, 2, "each owner should receive its own digest")
	require.ElementsMatch(t, []uuid.UUID{f.user.ID, user2.ID}, []uuid.UUID{sent[0].UserID, sent[1].UserID})
	require.Len(t, auditor.AuditLogs(), 2, "each archived root should be audited")
}

func TestWorker_AutoArchiveAuditsAndDigestsRootOnlyForFamily(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	root := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	child := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	f.linkChild(t, root.ID, child.ID)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	auditor := audit.NewMock()
	enqueuer := notificationstest.NewFakeEnqueuer()
	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), mockAuditorPtr(auditor), enqueuer)
	worker.archiveOnce(ctx, now)

	require.True(t, f.archived(t, root.ID))
	require.True(t, f.archived(t, child.ID))

	logs := auditor.AuditLogs()
	require.Len(t, logs, 1, "only the root should be audited; children inherit the decision")
	require.Equal(t, root.ID, logs[0].ResourceID)
	require.Len(t, enqueuer.Sent(), 1, "a single-owner family produces one digest")
}

func TestWorker_AutoArchiveIncrementsRecordsCounter(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_chat_auto_archive_records_total"})
	opts := f.archiveWorkerOptions()
	opts.Pubsub = newRecordingPubsub(f.pubsub)
	opts.AutoArchiveRecords = counter
	worker := f.newArchiveWorkerWithOptions(t, opts)

	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, chat.ID))
	require.InDelta(t, 1.0, promtestutil.ToFloat64(counter), 0.0001, "counter should reflect one archived root")
}

func TestWorker_AutoArchiveSecondTickIdempotent(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	auditor := audit.NewMock()
	enqueuer := notificationstest.NewFakeEnqueuer()
	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), mockAuditorPtr(auditor), enqueuer)

	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, chat.ID))
	require.Len(t, auditor.AuditLogs(), 1)
	require.Len(t, enqueuer.Sent(), 1)

	// An already-archived chat is no longer a candidate, so a second tick is a
	// no-op for both audit and digest dispatch.
	worker.archiveOnce(ctx, now)
	require.Len(t, auditor.AuditLogs(), 1, "second tick must not re-audit")
	require.Len(t, enqueuer.Sent(), 1, "second tick must not re-notify")
}

func TestWorker_AutoArchiveCutoffStableAcrossSameDayTicks(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	// created_at is far in the past so the boundary decision is driven purely
	// by message activity sitting exactly on the cutoff date.
	chat := f.createArchiveCandidate(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	// StartOfDay(2026-05-29) - 90d = 2026-02-28; activity on that date is not
	// strictly before the cutoff.
	insertArchiveMessage(t, f, chat.ID, time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	worker := f.newArchiveWorker(t, newRecordingPubsub(f.pubsub), nil, nil)

	// Tick early in the UTC day.
	worker.archiveOnce(ctx, time.Date(2026, 5, 29, 23, 49, 0, 0, time.UTC))
	require.False(t, f.archived(t, chat.ID), "activity on the cutoff date must survive")

	// Tick later the same UTC day: advancing wall-clock time within a day must
	// not change the cutoff ("no trickle").
	worker.archiveOnce(ctx, time.Date(2026, 5, 29, 23, 59, 0, 0, time.UTC))
	require.False(t, f.archived(t, chat.ID), "same-day tick must not change the decision")

	// Tick on the next UTC day: the cutoff advances to 2026-03-01 and the chat
	// becomes eligible.
	worker.archiveOnce(ctx, time.Date(2026, 5, 30, 0, 9, 0, 0, time.UTC))
	require.True(t, f.archived(t, chat.ID), "cutoff advances on the next UTC day")
}

func TestWorker_AutoArchiveDigestDispatchContinuesAfterEnqueueError(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	owner1 := uuid.New()
	owner2 := uuid.New()
	enq := &recordingEnqueuer{failOwner: owner1}
	opts := f.archiveWorkerOptions()
	opts.NotificationsEnqueuer = enq
	worker := f.newArchiveWorkerWithOptions(t, opts)

	roots := []autoArchivedChat{
		{Chat: database.Chat{OwnerID: owner1, OrganizationID: f.org.ID, Title: "a"}, LastActivityAt: time.Now()},
		{Chat: database.Chat{OwnerID: owner2, OrganizationID: f.org.ID, Title: "b"}, LastActivityAt: time.Now()},
	}
	worker.enqueueAutoArchiveDigests(context.Background(), time.Now(), 90, 30, roots)

	require.ElementsMatch(t, []uuid.UUID{owner1, owner2}, enq.enqueuedOwners(),
		"a transient enqueue failure must not abort the dispatch loop")
}

func TestWorker_AutoArchiveDigestDispatchStopsWhenCanceled(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	enq := &recordingEnqueuer{}
	opts := f.archiveWorkerOptions()
	opts.NotificationsEnqueuer = enq
	worker := f.newArchiveWorkerWithOptions(t, opts)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	roots := []autoArchivedChat{
		{Chat: database.Chat{OwnerID: uuid.New(), OrganizationID: f.org.ID}, LastActivityAt: time.Now()},
		{Chat: database.Chat{OwnerID: uuid.New(), OrganizationID: f.org.ID}, LastActivityAt: time.Now()},
	}
	worker.enqueueAutoArchiveDigests(ctx, time.Now(), 90, 30, roots)

	require.Empty(t, enq.enqueuedOwners(), "canceled dispatch must enqueue nothing")
}

// Config / query error handling.

func TestWorker_AutoArchiveDaysConfigReadFailureSkipsTick(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	enqueuer := notificationstest.NewFakeEnqueuer()
	opts := f.archiveWorkerOptions()
	opts.Store = &archiveErrStore{Store: f.db, autoArchiveDaysErr: xerrors.New("boom")}
	opts.NotificationsEnqueuer = enqueuer
	worker := f.newArchiveWorkerWithOptions(t, opts)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, chat.ID), "auto-archive config read failure must skip the tick")
	require.Empty(t, enqueuer.Sent())
}

func TestWorker_AutoArchiveRetentionConfigReadFailureSkipsTick(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	enqueuer := notificationstest.NewFakeEnqueuer()
	opts := f.archiveWorkerOptions()
	opts.Store = &archiveErrStore{Store: f.db, retentionDaysErr: xerrors.New("boom")}
	opts.NotificationsEnqueuer = enqueuer
	worker := f.newArchiveWorkerWithOptions(t, opts)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, chat.ID), "retention config read failure must skip the tick")
	require.Empty(t, enqueuer.Sent())
}

func TestWorker_AutoArchiveCandidateQueryFailureSkipsTick(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	enqueuer := notificationstest.NewFakeEnqueuer()
	opts := f.archiveWorkerOptions()
	opts.Store = &archiveErrStore{Store: f.db, candidatesErr: xerrors.New("boom")}
	opts.NotificationsEnqueuer = enqueuer
	worker := f.newArchiveWorkerWithOptions(t, opts)
	worker.archiveOnce(ctx, now)

	require.False(t, f.archived(t, chat.ID), "candidate query failure must skip the tick")
	require.Empty(t, enqueuer.Sent())
}

// Loop wiring.

func TestWorker_AutoArchiveLoopRunsImmediatelyAndOnTick(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))

	mClock := quartz.NewMock(t)
	now := mClock.Now().UTC()
	first := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))

	opts := f.archiveWorkerOptions()
	opts.Pubsub = newRecordingPubsub(f.pubsub)
	opts.Clock = mClock
	opts.ArchiveInterval = time.Minute
	worker := f.newArchiveWorkerWithOptions(t, opts)

	nowTrap := mClock.Trap().Now("chatworker", "auto-archive")
	defer nowTrap.Close()
	tickerTrap := mClock.Trap().NewTicker("chatworker", "auto-archive")
	defer tickerTrap.Close()
	tickerStopTrap := mClock.Trap().TickerStop("chatworker", "auto-archive")
	defer tickerStopTrap.Close()
	tickerResetTrap := mClock.Trap().TickerReset("chatworker", "auto-archive")
	defer tickerResetTrap.Close()

	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.archiveLoop(loopCtx)
	}()

	nowTrap.MustWait(ctx).MustRelease(ctx)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return f.archived(t, first.ID)
	}, testutil.IntervalFast, "immediate startup tick should archive the first candidate")
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	// A second candidate is only archived once the interval ticker fires.
	second := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	advanced := mClock.Advance(time.Minute)
	tickerStopTrap.MustWait(ctx).MustRelease(ctx)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return f.archived(t, second.ID)
	}, testutil.IntervalFast, "interval tick should archive the second candidate")
	resetCall := tickerResetTrap.MustWait(ctx)
	require.Equal(t, time.Minute, resetCall.Duration)
	resetCall.MustRelease(ctx)
	advanced.MustWait(ctx)

	cancel()
	tickerStopTrap.MustWait(ctx).MustRelease(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("archiveLoop did not exit after context cancellation")
	}
}

func TestBuildAutoArchiveDigestData(t *testing.T) {
	t.Parallel()
	tickStart := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	t.Run("UnderCap", func(t *testing.T) {
		t.Parallel()
		rows := make([]autoArchivedChat, 0, 3)
		for i := range 3 {
			rows = append(rows, autoArchivedChat{
				Chat:           database.Chat{Title: fmt.Sprintf("chat-%d", i)},
				LastActivityAt: tickStart.Add(-time.Duration(i+1) * 24 * time.Hour),
			})
		}
		data := buildAutoArchiveDigestData(rows, 90, 30, tickStart)
		require.Equal(t, "90", data["auto_archive_days"])
		require.Equal(t, "30", data["retention_days"])
		chats, ok := data["archived_chats"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, chats, 3)
		require.Equal(t, "chat-0", chats[0]["title"])
		require.Contains(t, chats[0]["last_activity_humanized"].(string), "ago")
		require.NotContains(t, data, "additional_archived_count")
	})

	t.Run("OverflowCap", func(t *testing.T) {
		t.Parallel()
		total := chatAutoArchiveDigestMaxChats + 5
		rows := make([]autoArchivedChat, 0, total)
		for i := range total {
			rows = append(rows, autoArchivedChat{
				Chat:           database.Chat{Title: fmt.Sprintf("chat-%d", i)},
				LastActivityAt: tickStart.Add(-24 * time.Hour),
			})
		}
		data := buildAutoArchiveDigestData(rows, 90, 0, tickStart)
		chats, ok := data["archived_chats"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, chats, chatAutoArchiveDigestMaxChats, "titles are capped")
		require.Equal(t, "5", data["additional_archived_count"])
		require.Equal(t, "0", data["retention_days"])
	})
}

func TestIsExpectedAutoArchiveError(t *testing.T) {
	t.Parallel()
	expected := []error{
		sql.ErrNoRows,
		chatstate.ErrChatNotFound,
		chatstate.ErrChatNotRoot,
		chatstate.ErrInvalidState,
		chatstate.ErrTransitionNotAllowed,
	}
	for _, err := range expected {
		require.True(t, isExpectedAutoArchiveError(err), "%v should be classified as expected", err)
		require.True(t, isExpectedAutoArchiveError(xerrors.Errorf("wrapped: %w", err)),
			"wrapped %v should still be classified as expected", err)
	}
	require.False(t, isExpectedAutoArchiveError(xerrors.New("unexpected")))
}

// recordingEnqueuer records the owner of every enqueue and can be configured to
// fail for a specific owner (or all owners) to exercise dispatch resilience.
type recordingEnqueuer struct {
	mu        sync.Mutex
	owners    []uuid.UUID
	failOwner uuid.UUID
	failAll   bool
}

func (e *recordingEnqueuer) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	return e.EnqueueWithData(ctx, userID, templateID, labels, nil, createdBy, targets...)
}

func (e *recordingEnqueuer) EnqueueWithData(_ context.Context, userID, _ uuid.UUID, _ map[string]string, _ map[string]any, _ string, _ ...uuid.UUID) ([]uuid.UUID, error) {
	e.mu.Lock()
	e.owners = append(e.owners, userID)
	e.mu.Unlock()
	if e.failAll || userID == e.failOwner {
		return nil, xerrors.New("enqueue failed")
	}
	return []uuid.UUID{uuid.New()}, nil
}

func (e *recordingEnqueuer) enqueuedOwners() []uuid.UUID {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]uuid.UUID(nil), e.owners...)
}

// archiveErrStore wraps a real store and injects errors on the reads performed
// at the start of an auto-archive tick.
type archiveErrStore struct {
	database.Store
	autoArchiveDaysErr error
	retentionDaysErr   error
	candidatesErr      error
}

func (s *archiveErrStore) GetChatAutoArchiveDays(ctx context.Context, defaultAutoArchiveDays int32) (int32, error) {
	if s.autoArchiveDaysErr != nil {
		return 0, s.autoArchiveDaysErr
	}
	return s.Store.GetChatAutoArchiveDays(ctx, defaultAutoArchiveDays)
}

func (s *archiveErrStore) GetChatRetentionDays(ctx context.Context) (int32, error) {
	if s.retentionDaysErr != nil {
		return 0, s.retentionDaysErr
	}
	return s.Store.GetChatRetentionDays(ctx)
}

func (s *archiveErrStore) GetAutoArchiveInactiveChatCandidates(ctx context.Context, arg database.GetAutoArchiveInactiveChatCandidatesParams) ([]database.GetAutoArchiveInactiveChatCandidatesRow, error) {
	if s.candidatesErr != nil {
		return nil, s.candidatesErr
	}
	return s.Store.GetAutoArchiveInactiveChatCandidates(ctx, arg)
}
