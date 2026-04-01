package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatStreamNotifyPublishedMetrics(t *testing.T) {
	t.Parallel()

	_, ps := dbtestutil.NewDB(t)
	reg := prometheus.NewRegistry()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		logger:  logger,
		pubsub:  ps,
		metrics: newChatStreamMetrics(reg),
	}

	chatID := uuid.New()
	message := database.ChatMessage{ID: 42, ChatID: chatID, Role: database.ChatMessageRoleAssistant}

	server.publishMessage(chatID, message)
	server.publishEditedMessage(chatID, message)
	server.publishStatus(chatID, database.ChatStatusRunning, uuid.NullUUID{})
	server.publishError(chatID, chaterror.ClassifiedError{Message: "boom"})
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{QueueUpdate: true}, chatStreamNotifyReasonQueueUpdate)

	require.Equal(t, float64(1), promtest.ToFloat64(server.metrics.notifyPublished.WithLabelValues(chatStreamNotifyReasonMessagePersisted)))
	require.Equal(t, float64(1), promtest.ToFloat64(server.metrics.notifyPublished.WithLabelValues(chatStreamNotifyReasonFullRefresh)))
	require.Equal(t, float64(1), promtest.ToFloat64(server.metrics.notifyPublished.WithLabelValues(chatStreamNotifyReasonStatusChange)))
	require.Equal(t, float64(1), promtest.ToFloat64(server.metrics.notifyPublished.WithLabelValues(chatStreamNotifyReasonError)))
	require.Equal(t, float64(1), promtest.ToFloat64(server.metrics.notifyPublished.WithLabelValues(chatStreamNotifyReasonQueueUpdate)))
}

func TestChatStreamCatchupMetrics(t *testing.T) {
	t.Parallel()

	t.Run("AfterMessageID", func(t *testing.T) {
		t.Parallel()

		server, db, ps := newMetricsTestServer(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, firstMessage := createMetricsTestChat(ctx, t, server, db)
		_, _, cancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
		require.True(t, ok)
		t.Cleanup(cancel)

		insertMetricsChatMessage(ctx, t, db, chat.ID, chat.LastModelConfigID, "assistant", "second reply")
		publishMetricsNotify(t, ps, chat.ID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: firstMessage.ID})

		require.Eventually(t, func() bool {
			return promtest.ToFloat64(server.metrics.dbCatchupQueries.WithLabelValues(chatStreamNotifyReasonMessagePersisted)) == 1 &&
				promtest.ToFloat64(server.metrics.dbCatchupMessages.WithLabelValues(chatStreamNotifyReasonMessagePersisted)) == 1
		}, testutil.WaitMedium, testutil.IntervalFast)
	})

	t.Run("FullRefresh", func(t *testing.T) {
		t.Parallel()

		server, db, ps := newMetricsTestServer(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := createMetricsTestChat(ctx, t, server, db)
		insertMetricsChatMessage(ctx, t, db, chat.ID, chat.LastModelConfigID, "assistant", "second reply")
		_, _, cancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
		require.True(t, ok)
		t.Cleanup(cancel)

		publishMetricsNotify(t, ps, chat.ID, coderdpubsub.ChatStreamNotifyMessage{FullRefresh: true})

		require.Eventually(t, func() bool {
			return promtest.ToFloat64(server.metrics.dbCatchupQueries.WithLabelValues(chatStreamNotifyReasonFullRefresh)) == 1 &&
				promtest.ToFloat64(server.metrics.dbCatchupMessages.WithLabelValues(chatStreamNotifyReasonFullRefresh)) == 2
		}, testutil.WaitMedium, testutil.IntervalFast)
	})

	t.Run("QueueUpdate", func(t *testing.T) {
		t.Parallel()

		server, db, ps := newMetricsTestServer(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := createMetricsTestChat(ctx, t, server, db)
		_, _, cancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
		require.True(t, ok)
		t.Cleanup(cancel)

		publishMetricsNotify(t, ps, chat.ID, coderdpubsub.ChatStreamNotifyMessage{QueueUpdate: true})

		require.Eventually(t, func() bool {
			return promtest.ToFloat64(server.metrics.queueRefreshQueries) == 1
		}, testutil.WaitMedium, testutil.IntervalFast)
	})
}

func newMetricsTestServer(t *testing.T) (*Server, database.Store, dbpubsub.Pubsub) {
	t.Helper()

	db, ps := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := New(Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		PrometheusRegisterer:       prometheus.NewRegistry(),
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server, db, ps
}

func createMetricsTestChat(ctx context.Context, t *testing.T, server *Server, db database.Store) (database.Chat, database.ChatMessage) {
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

	chat, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "metrics-chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return chat, messages[0]
}

func insertMetricsChatMessage(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID, modelConfigID uuid.UUID, role database.ChatMessageRole, content string) database.ChatMessage {
	t.Helper()

	parts, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(content)})
	require.NoError(t, err)

	messages, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelConfigID},
		Role:                []database.ChatMessageRole{role},
		Content:             []string{string(parts.RawMessage)},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return messages[0]
}

func publishMetricsNotify(t *testing.T, ps dbpubsub.Pubsub, chatID uuid.UUID, notify coderdpubsub.ChatStreamNotifyMessage) {
	t.Helper()

	payload, err := json.Marshal(notify)
	require.NoError(t, err)
	require.NoError(t, ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chatID), payload))
}
