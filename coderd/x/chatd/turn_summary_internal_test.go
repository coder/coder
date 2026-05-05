package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateLastTurnSummaryRejectsStaleWrites(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})

	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	modelCfg, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "summary-chat",
	})
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{db: db}
	server.updateLastTurnSummary(ctx, chat, chat.UpdatedAt, "fresh summary", logger)

	fetched, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "fresh summary", Valid: true}, fetched.LastTurnSummary)

	advancedUpdatedAt := chat.UpdatedAt.Add(time.Second)
	_, err = db.UpdateChatStatusPreserveUpdatedAt(ctx, database.UpdateChatStatusPreserveUpdatedAtParams{
		ID:        chat.ID,
		Status:    database.ChatStatusRunning,
		UpdatedAt: advancedUpdatedAt,
	})
	require.NoError(t, err)

	server.updateLastTurnSummary(context.WithoutCancel(ctx), chat, chat.UpdatedAt, "stale summary", logger)

	fetched, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "fresh summary", Valid: true}, fetched.LastTurnSummary)
	require.Equal(t, advancedUpdatedAt, fetched.UpdatedAt)
}

func TestPendingChatPersistsSummaryButSkipsWebPush(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})

	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	modelCfg, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusPending,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "summary-pending-chat",
	})
	require.NoError(t, err)

	const summary = "Finished the queued turn."
	model := &chattest.FakeModel{
		ProviderName: "openai",
		ModelName:    "test-model",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			return &fantasy.Response{
				Content: fantasy.ResponseContent{
					fantasy.TextContent{Text: summary},
				},
			}, nil
		},
	}

	dispatcher := &recordingWebpushDispatcher{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{db: db, webpushDispatcher: dispatcher}
	server.maybeFinalizeTurnSummaryAndPush(
		context.WithoutCancel(ctx),
		chat,
		database.ChatStatusPending,
		"",
		runChatResult{
			FinalAssistantText: "I finished the queued turn.",
			PushSummaryModel:   model,
			FallbackProvider:   model.Provider(),
			FallbackModel:      model.Model(),
		},
		logger,
	)
	server.drainInflight()

	fetched, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: summary, Valid: true}, fetched.LastTurnSummary)
	require.Equal(t, int32(0), dispatcher.dispatchCount.Load())
}

type recordingWebpushDispatcher struct {
	dispatchCount atomic.Int32
}

func (d *recordingWebpushDispatcher) Dispatch(
	_ context.Context,
	_ uuid.UUID,
	_ codersdk.WebpushMessage,
) error {
	d.dispatchCount.Add(1)
	return nil
}

func (*recordingWebpushDispatcher) Test(_ context.Context, _ codersdk.WebpushSubscription) error {
	return nil
}

func (*recordingWebpushDispatcher) PublicKey() string {
	return "test-vapid-public-key"
}
