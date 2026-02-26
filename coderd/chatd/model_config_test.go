package chatd //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestParseChatModelConfig_ModelConfigID(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	raw := json.RawMessage(`{"model":"gpt-4","model_config_id":"` + modelConfigID.String() + `"}`)

	config, err := parseChatModelConfig(raw)
	require.NoError(t, err)
	require.NotNil(t, config.ModelConfigID)
	require.Equal(t, modelConfigID, *config.ModelConfigID)
}

func TestParseChatModelConfig_InvalidModelConfigIDIgnored(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"model":"gpt-4","model_config_id":"not-a-uuid"}`)
	config, err := parseChatModelConfig(raw)
	require.NoError(t, err)
	require.Nil(t, config.ModelConfigID)
}

func TestApplyFallbackChatModelConfig_SetsModelConfigID(t *testing.T) {
	t.Parallel()

	fallbackModelConfigID := uuid.New()
	config := applyFallbackChatModelConfig(
		chatModelConfig{},
		database.ChatModelConfig{
			ID:       fallbackModelConfigID,
			Provider: "openai",
			Model:    "gpt-4",
		},
	)

	require.NotNil(t, config.ModelConfigID)
	require.Equal(t, fallbackModelConfigID, *config.ModelConfigID)
}

func TestCreateChat_PersistsModelConfigIDForInitialMessages(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	server := &Server{db: store}

	chatID := uuid.New()
	ownerID := uuid.New()
	modelConfigID := uuid.New()
	waitingChat := database.Chat{
		ID:      chatID,
		OwnerID: ownerID,
		Status:  database.ChatStatusWaiting,
	}
	pendingChat := waitingChat
	pendingChat.Status = database.ChatStatusPending

	gomock.InOrder(
		store.EXPECT().InsertChat(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ database.InsertChatParams) (database.Chat, error) {
				return waitingChat, nil
			},
		),
		store.EXPECT().InsertChatMessage(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error) {
				require.Equal(t, "system", arg.Role)
				require.True(t, arg.ModelConfigID.Valid)
				require.Equal(t, modelConfigID, arg.ModelConfigID.UUID)
				return database.ChatMessage{
					ID:            1,
					ChatID:        chatID,
					Role:          arg.Role,
					ModelConfigID: arg.ModelConfigID,
				}, nil
			},
		),
		store.EXPECT().InsertChatMessage(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error) {
				require.Equal(t, "user", arg.Role)
				require.True(t, arg.ModelConfigID.Valid)
				require.Equal(t, modelConfigID, arg.ModelConfigID.UUID)
				return database.ChatMessage{
					ID:            2,
					ChatID:        chatID,
					Role:          arg.Role,
					ModelConfigID: arg.ModelConfigID,
				}, nil
			},
		),
		store.EXPECT().GetChatByID(gomock.Any(), chatID).Return(waitingChat, nil),
		store.EXPECT().UpdateChatStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg database.UpdateChatStatusParams) (database.Chat, error) {
				require.Equal(t, chatID, arg.ID)
				require.Equal(t, database.ChatStatusPending, arg.Status)
				return pendingChat, nil
			},
		),
	)

	_, err := server.CreateChat(context.Background(), CreateOptions{
		OwnerID:            ownerID,
		Title:              "New chat",
		ModelConfigID:      modelConfigID,
		SystemPrompt:       "system prompt",
		InitialUserContent: json.RawMessage(`"hello"`),
	})
	require.NoError(t, err)
}

func TestPostMessages_PersistsModelConfigID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	server := &Server{db: store}

	chatID := uuid.New()
	modelConfigID := uuid.New()
	chat := database.Chat{
		ID:     chatID,
		Status: database.ChatStatusPending,
	}

	store.EXPECT().InTx(gomock.Any(), gomock.Nil()).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(store)
		},
	)
	store.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(chat, nil)
	store.EXPECT().InsertChatMessage(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error) {
			require.Equal(t, "user", arg.Role)
			require.True(t, arg.ModelConfigID.Valid)
			require.Equal(t, modelConfigID, arg.ModelConfigID.UUID)
			return database.ChatMessage{
				ID:            1,
				ChatID:        chatID,
				Role:          arg.Role,
				ModelConfigID: arg.ModelConfigID,
			}, nil
		},
	)

	_, err := server.PostMessages(context.Background(), PostMessagesOptions{
		ChatID:        chatID,
		Content:       json.RawMessage(`"hello"`),
		ModelConfigID: &modelConfigID,
	})
	require.NoError(t, err)
}

func TestPromoteQueued_PersistsModelConfigID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	server := &Server{db: store}

	chatID := uuid.New()
	modelConfigID := uuid.New()
	queuedMessageID := int64(42)
	chat := database.Chat{
		ID:     chatID,
		Status: database.ChatStatusWaiting,
	}

	store.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil)
	store.EXPECT().InTx(gomock.Any(), gomock.Nil()).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(store)
		},
	)
	store.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(chat, nil)
	queueCalls := 0
	store.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).DoAndReturn(
		func(_ context.Context, _ uuid.UUID) ([]database.ChatQueuedMessage, error) {
			queueCalls++
			if queueCalls == 1 {
				return []database.ChatQueuedMessage{{
					ID:      queuedMessageID,
					ChatID:  chatID,
					Content: json.RawMessage(`"queued"`),
				}}, nil
			}
			return []database.ChatQueuedMessage{}, nil
		},
	).Times(2)
	store.EXPECT().DeleteChatQueuedMessage(gomock.Any(), database.DeleteChatQueuedMessageParams{
		ID:     queuedMessageID,
		ChatID: chatID,
	}).Return(nil)
	store.EXPECT().InsertChatMessage(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error) {
			require.Equal(t, "user", arg.Role)
			require.True(t, arg.ModelConfigID.Valid)
			require.Equal(t, modelConfigID, arg.ModelConfigID.UUID)
			return database.ChatMessage{
				ID:            7,
				ChatID:        chatID,
				Role:          arg.Role,
				ModelConfigID: arg.ModelConfigID,
			}, nil
		},
	)
	store.EXPECT().UpdateChatStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg database.UpdateChatStatusParams) (database.Chat, error) {
			require.Equal(t, chatID, arg.ID)
			require.Equal(t, database.ChatStatusPending, arg.Status)
			return database.Chat{ID: chatID, Status: database.ChatStatusPending}, nil
		},
	)
	store.EXPECT().GetChatMessagesByChatID(gomock.Any(), chatID).Return(
		[]database.ChatMessage{{ID: 7, ChatID: chatID, Role: "user"}},
		nil,
	)

	_, err := server.PromoteQueued(context.Background(), PromoteQueuedOptions{
		ChatID:          chatID,
		QueuedMessageID: queuedMessageID,
		ModelConfigID:   &modelConfigID,
	})
	require.NoError(t, err)
}
