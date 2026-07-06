package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateLastTurnSummaryRejectsStaleWrites(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})

	provider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		Enabled:     true,
	})

	modelCfg, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		AIProviderID:         uuid.NullUUID{UUID: provider.ID, Valid: true},
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

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello"),
	})
	require.NoError(t, err)
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: owner.ID})
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "summary-chat",
		ClientType:        database.ChatClientTypeUi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: owner.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
				APIKeyID:       sql.NullString{String: apiKey.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	chat := created.Chat

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{db: db, pubsub: ps}
	server.updateLastTurnSummary(ctx, chat, chat.HistoryVersion, "fresh summary", logger)

	fetched, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "fresh summary", Valid: true}, fetched.LastTurnSummary)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant response"),
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(db, ps, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleAssistant,
					Content:        assistantContent,
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
				},
			},
		})
		return err
	}))

	server.updateLastTurnSummary(context.WithoutCancel(ctx), chat, chat.HistoryVersion, "stale summary", logger)

	fetched, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "fresh summary", Valid: true}, fetched.LastTurnSummary)
}

func TestPendingChatPersistsSummaryButSkipsWebPush(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})

	provider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		Enabled:     true,
	})

	modelCfg, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		AIProviderID:         uuid.NullUUID{UUID: provider.ID, Valid: true},
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

	const summary = "Still working on request"
	var generateCalls atomic.Int32
	model := &chattest.FakeModel{
		ProviderName: "openai",
		ModelName:    "test-model",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			generateCalls.Add(1)
			return &fantasy.Response{
				Content: fantasy.ResponseContent{
					fantasy.TextContent{Text: "Unexpected label"},
				},
			}, nil
		},
	}

	dispatcher := &recordingWebpushDispatcher{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{ctx: t.Context(), db: db, pubsub: ps, webpushDispatcher: dispatcher}
	server.maybeFinalizeTurnStatusLabelAndPush(
		context.WithoutCancel(ctx),
		chat,
		database.ChatStatusPending,
		"",
		runChatResult{
			FinalAssistantText: "I finished the queued turn.",
			StatusLabelModel:   model,
			FallbackProvider:   model.Provider(),
			FallbackModel:      model.Model(),
		},
		logger,
	)
	server.drainInflight()

	fetched, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: summary, Valid: true}, fetched.LastTurnSummary)
	require.Equal(t, int32(0), generateCalls.Load())
	require.Equal(t, int32(0), dispatcher.dispatchCount.Load())
}

func TestSuccessfulChildChatOutcomeSkipsSummaryAndWebPush(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})

	provider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		Enabled:     true,
	})

	modelCfg, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		AIProviderID:         uuid.NullUUID{UUID: provider.ID, Valid: true},
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

	parent, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "summary-parent-chat",
		MCPServerIDs:      []uuid.UUID{},
	})
	require.NoError(t, err)
	child, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		LastModelConfigID: modelCfg.ID,
		Title:             "summary-child-chat",
		MCPServerIDs:      []uuid.UUID{},
	})
	require.NoError(t, err)

	dispatcher := &recordingWebpushDispatcher{}
	server := &Server{
		ctx:               t.Context(),
		db:                db,
		pubsub:            ps,
		logger:            slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		webpushDispatcher: dispatcher,
	}
	require.NoError(t, server.afterGenerationOutcome(ctx, generationOutcome{
		Chat: child,
		Kind: runnerActionKindFinishTurn,
	}))
	server.drainInflight()

	fetched, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.False(t, fetched.LastTurnSummary.Valid)
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
