package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestInterruptChatBroadcastsStatusAcrossInstances(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replicaA := newTestServer(t, db, ps, uuid.New())
	replicaB := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replicaA.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-me",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	runningWorker := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: runningWorker, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	_, events, cancel, ok := replicaB.Subscribe(ctx, chat.ID, nil)
	require.True(t, ok)
	t.Cleanup(cancel)

	updated := replicaA.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)
	require.False(t, updated.WorkerID.Valid)

	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type != codersdk.ChatStreamEventTypeStatus || event.Status == nil {
				return false
			}
			return event.Status.Status == codersdk.ChatStatusWaiting
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestInterruptChatClearsWorkerInDatabase(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "db-transition",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	updated := replica.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)
	require.False(t, updated.WorkerID.Valid)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)
}

func TestUpdateChatHeartbeatRequiresOwnership(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "heartbeat-ownership",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	workerID := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	rows, err := db.UpdateChatHeartbeat(ctx, database.UpdateChatHeartbeatParams{
		ID:       chat.ID,
		WorkerID: uuid.New(),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), rows)

	rows, err = db.UpdateChatHeartbeat(ctx, database.UpdateChatHeartbeatParams{
		ID:       chat.ID,
		WorkerID: workerID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
}

func TestSendMessageQueueBehaviorQueuesWhenBusy(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "queue-when-busy",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	workerID := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []fantasy.Content{fantasy.TextContent{Text: "queued"}},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	require.Equal(t, database.ChatStatusRunning, result.Chat.Status)
	require.Equal(t, workerID, result.Chat.WorkerID.UUID)
	require.True(t, result.Chat.WorkerID.Valid)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)

	messages, err := db.GetChatMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestSendMessageInterruptBehaviorSendsImmediatelyWhenBusy(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "interrupt-when-busy",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []fantasy.Content{fantasy.TextContent{Text: "interrupt"}},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	require.False(t, result.Queued)
	require.Equal(t, database.ChatStatusPending, result.Chat.Status)
	require.False(t, result.Chat.WorkerID.Valid)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 0)

	messages, err := db.GetChatMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, messages[len(messages)-1].ID, result.Message.ID)
}

func TestEditMessageUpdatesAndTruncatesAndClearsQueue(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "edit-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "original"}},
	})
	require.NoError(t, err)

	initialMessages, err := db.GetChatMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, initialMessages, 1)
	editedMessageID := initialMessages[0].ID

	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []fantasy.Content{fantasy.TextContent{Text: "follow-up"}},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []fantasy.Content{fantasy.TextContent{Text: "another"}},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)

	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  chat.ID,
		Content: json.RawMessage(`"queued"`),
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	editResult, err := replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMessageID,
		Content:         []fantasy.Content{fantasy.TextContent{Text: "edited"}},
	})
	require.NoError(t, err)
	require.Equal(t, editedMessageID, editResult.Message.ID)
	require.Equal(t, database.ChatStatusPending, editResult.Chat.Status)
	require.False(t, editResult.Chat.WorkerID.Valid)

	editedSDK := db2sdk.ChatMessage(editResult.Message)
	require.Len(t, editedSDK.Content, 1)
	require.Equal(t, "edited", editedSDK.Content[0].Text)

	messages, err := db.GetChatMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, editedMessageID, messages[0].ID)
	onlyMessage := db2sdk.ChatMessage(messages[0])
	require.Len(t, onlyMessage.Content, 1)
	require.Equal(t, "edited", onlyMessage.Content[0].Text)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 0)

	chatFromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, chatFromDB.Status)
	require.False(t, chatFromDB.WorkerID.Valid)
}

func TestEditMessageRejectsMissingMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "missing-edited-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: 999999,
		Content:         []fantasy.Content{fantasy.TextContent{Text: "edited"}},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, chatd.ErrEditedMessageNotFound))
}

func TestEditMessageRejectsNonUserMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "non-user-edited-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	assistantMessage, err := db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:          "assistant",
		Content: pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`"assistant"`),
			Valid:      true,
		},
		Visibility:          database.ChatMessageVisibilityBoth,
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
		Compressed:          sql.NullBool{},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: assistantMessage.ID,
		Content:         []fantasy.Content{fantasy.TextContent{Text: "edited"}},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, chatd.ErrEditedMessageNotUser))
}

func TestRecoverStaleChatsPeriodically(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Use a very short stale threshold so the periodic recovery
	// kicks in quickly during the test.
	staleAfter := 500 * time.Millisecond

	// Create a chat and simulate a dead worker by setting the chat
	// to running with a heartbeat in the past.
	deadWorkerID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "stale-recovery-periodic",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// Start a new replica. Its startup recovery will reset the
	// chat (since the heartbeat is old), but the key point is that
	// the periodic loop also recovers newly-stale chats.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitSuperLong,
		InFlightChatStaleAfter:     staleAfter,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// The startup recovery should have already reset our stale
	// chat.
	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Now simulate a second stale chat appearing AFTER startup.
	// This tests the periodic recovery, not just the startup one.
	deadWorkerID2 := uuid.New()
	chat2, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "stale-recovery-periodic-2",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat2.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadWorkerID2, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// The periodic stale recovery loop (running at staleAfter/5 =
	// 100ms intervals) should pick this up without a restart.
	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat2.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestNewReplicaRecoversStaleChatFromDeadReplica(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Simulate a chat left running by a dead replica with a stale
	// heartbeat (well beyond the stale threshold).
	deadReplicaID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "orphaned-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Set the heartbeat far in the past so it's definitely stale.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadReplicaID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// Start a new replica — it should recover the stale chat on
	// startup.
	newReplica := newTestServer(t, db, ps, uuid.New())
	_ = newReplica

	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending &&
			!fromDB.WorkerID.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestWaitingChatsAreNotRecoveredAsStale(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat in waiting status — this should NOT be touched
	// by stale recovery.
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		Title:             "waiting-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Start a replica with a short stale threshold.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitSuperLong,
		InFlightChatStaleAfter:     500 * time.Millisecond,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Wait long enough for multiple periodic recovery cycles to
	// run (staleAfter/5 = 100ms intervals).
	require.Never(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status != database.ChatStatusWaiting
	}, time.Second, testutil.IntervalFast,
		"waiting chat should not be modified by stale recovery")
}

func newTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	replicaID uuid.UUID,
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitSuperLong,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

func seedChatDependencies(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)
	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	return user, model
}
