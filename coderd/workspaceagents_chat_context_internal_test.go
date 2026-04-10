package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func TestUpdateAgentChatLastInjectedContextFromMessagesUsesMessageIDTieBreaker(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	createdAt := time.Date(2026, time.April, 9, 13, 0, 0, 0, time.UTC)
	oldAgentID := uuid.New()
	newAgentID := uuid.New()

	oldContent, err := json.Marshal([]codersdk.ChatMessagePart{{
		Type:               codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath:    "/old/AGENTS.md",
		ContextFileContent: "old instructions",
		ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
	}})
	require.NoError(t, err)
	newContent, err := json.Marshal([]codersdk.ChatMessagePart{{
		Type:               codersdk.ChatMessagePartTypeContextFile,
		ContextFilePath:    "/new/AGENTS.md",
		ContextFileContent: "new instructions",
		ContextFileAgentID: uuid.NullUUID{UUID: newAgentID, Valid: true},
	}})
	require.NoError(t, err)

	db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	}).Return([]database.ChatMessage{
		{
			ID:        2,
			CreatedAt: createdAt,
			Content: pqtype.NullRawMessage{
				RawMessage: newContent,
				Valid:      true,
			},
		},
		{
			ID:        1,
			CreatedAt: createdAt,
			Content: pqtype.NullRawMessage{
				RawMessage: oldContent,
				Valid:      true,
			},
		},
	}, nil)

	db.EXPECT().UpdateChatLastInjectedContext(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg database.UpdateChatLastInjectedContextParams) (database.Chat, error) {
			require.Equal(t, chatID, arg.ID)
			require.True(t, arg.LastInjectedContext.Valid)
			var cached []codersdk.ChatMessagePart
			require.NoError(t, json.Unmarshal(arg.LastInjectedContext.RawMessage, &cached))
			require.Len(t, cached, 1)
			require.Equal(t, "/new/AGENTS.md", cached[0].ContextFilePath)
			require.Equal(t, uuid.NullUUID{UUID: newAgentID, Valid: true}, cached[0].ContextFileAgentID)
			return database.Chat{}, nil
		},
	)

	err = updateAgentChatLastInjectedContextFromMessages(
		context.Background(),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		db,
		chatID,
	)
	require.NoError(t, err)
}

func insertAgentChatTestModelConfig(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	userID uuid.UUID,
) database.ChatModelConfig {
	t.Helper()

	sysCtx := dbauthz.AsSystemRestricted(ctx)
	createdBy := uuid.NullUUID{UUID: userID, Valid: true}

	_, err := db.InsertChatProvider(sysCtx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-api-key",
		ApiKeyKeyID:          sql.NullString{},
		CreatedBy:            createdBy,
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(sysCtx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            createdBy,
		UpdatedBy:            createdBy,
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	return model
}
