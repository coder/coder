package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/database"
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
		InitialUserContent: mustJSON(t, []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: "hello"}}),
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
		InitialUserContent: mustJSON(t, []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: "hello"}}),
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
		InitialUserContent: mustJSON(t, []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: "hello"}}),
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

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
