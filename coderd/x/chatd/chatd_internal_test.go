package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRegenerateChatTitle_PersistsAndBroadcasts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	lockTx := dbmock.NewMockStore(ctrl)
	usageTx := dbmock.NewMockStore(ctrl)
	unlockTx := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	pubsub := dbpubsub.NewInMemory()
	clock := quartz.NewReal()

	ownerID := uuid.New()
	chatID := uuid.New()
	modelConfigID := uuid.New()
	workerID := uuid.New()
	userPrompt := "review pull request 23633 and fix review threads"
	wantTitle := "Review PR 23633"

	chat := database.Chat{
		ID:                chatID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Status:            database.ChatStatusRunning,
		WorkerID:          uuid.NullUUID{UUID: workerID, Valid: true},
		Title:             fallbackChatTitle(userPrompt),
	}
	modelConfig := database.ChatModelConfig{
		ID:           modelConfigID,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: 8192,
	}
	updatedChat := chat
	updatedChat.Title = wantTitle

	messageEvents := make(chan struct {
		payload codersdk.ChatWatchEvent
		err     error
	}, 1)
	cancelSub, err := pubsub.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(ownerID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
			messageEvents <- struct {
				payload codersdk.ChatWatchEvent
				err     error
			}{payload: payload, err: err}
		}),
	)
	require.NoError(t, err)
	defer cancelSub()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		require.Equal(t, "gpt-4o-mini", req.Model)
		return chattest.OpenAINonStreamingResponse("{\"title\":\"" + wantTitle + "\"}")
	})

	server := &Server{
		db:          db,
		logger:      logger,
		pubsub:      pubsub,
		configCache: newChatConfigCache(context.Background(), db, clock),
	}

	db.EXPECT().GetChatModelConfigByID(gomock.Any(), modelConfigID).Return(modelConfig, nil)
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{
		Provider:             "openai",
		CentralApiKeyEnabled: true,
		APIKey:               "test-key",
		BaseUrl:              serverURL,
	}}, nil)
	db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(database.ChatUsageLimitConfig{}, sql.ErrNoRows)
	db.EXPECT().GetChatMessagesByChatIDAscPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDAscPaginatedParams{
			ChatID:   chatID,
			AfterID:  0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return([]database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText(userPrompt),
		),
		mustChatMessage(
			t,
			database.ChatMessageRoleAssistant,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("checking the diff now"),
		),
	}, nil)
	db.EXPECT().GetChatMessagesByChatIDDescPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDDescPaginatedParams{
			ChatID:   chatID,
			BeforeID: 0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return(nil, nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(nil, nil)

	gomock.InOrder(
		db.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("chat_title_regenerate_lock")).DoAndReturn(
			func(fn func(database.Store) error, opts *database.TxOptions) error {
				require.Equal(t, "chat_title_regenerate_lock", opts.TxIdentifier)
				return fn(lockTx)
			},
		),
		db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
			func(fn func(database.Store) error, opts *database.TxOptions) error {
				require.Nil(t, opts)
				return fn(usageTx)
			},
		),
		db.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("chat_title_regenerate_unlock")).DoAndReturn(
			func(fn func(database.Store) error, opts *database.TxOptions) error {
				require.Equal(t, "chat_title_regenerate_unlock", opts.TxIdentifier)
				return fn(unlockTx)
			},
		),
	)

	lockTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(chat, nil)

	usageTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(chat, nil)
	usageTx.EXPECT().InsertChatMessages(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatMessagesParams{})).DoAndReturn(
		func(_ context.Context, arg database.InsertChatMessagesParams) ([]database.ChatMessage, error) {
			require.Equal(t, []uuid.UUID{ownerID}, arg.CreatedBy)
			require.Equal(t, []uuid.UUID{modelConfigID}, arg.ModelConfigID)
			require.Equal(t, []string{"[]"}, arg.Content)
			return []database.ChatMessage{{ID: 91}}, nil
		},
	)
	usageTx.EXPECT().SoftDeleteChatMessageByID(gomock.Any(), int64(91)).Return(nil)
	usageTx.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
		ID:    chatID,
		Title: wantTitle,
	}).Return(updatedChat, nil)

	unlockTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(updatedChat, nil)

	gotChat, err := server.RegenerateChatTitle(ctx, chat)
	require.NoError(t, err)
	require.Equal(t, updatedChat, gotChat)

	select {
	case event := <-messageEvents:
		require.NoError(t, event.err)
		require.Equal(t, codersdk.ChatWatchEventKindTitleChange, event.payload.Kind)
		require.Equal(t, chatID, event.payload.Chat.ID)
		require.Equal(t, wantTitle, event.payload.Chat.Title)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for title change pubsub event")
	}
}

func TestRegenerateChatTitle_PersistsAndBroadcasts_IdleChatReleasesManualLock(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	lockTx := dbmock.NewMockStore(ctrl)
	usageTx := dbmock.NewMockStore(ctrl)
	unlockTx := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	pubsub := dbpubsub.NewInMemory()
	clock := quartz.NewReal()

	ownerID := uuid.New()
	chatID := uuid.New()
	modelConfigID := uuid.New()
	userPrompt := "review pull request 23633 and fix review threads"
	wantTitle := "Review PR 23633"

	chat := database.Chat{
		ID:                chatID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Status:            database.ChatStatusCompleted,
		Title:             fallbackChatTitle(userPrompt),
	}
	lockedChat := chat
	lockedChat.WorkerID = uuid.NullUUID{UUID: manualTitleLockWorkerID, Valid: true}
	lockedChat.StartedAt = sql.NullTime{Time: time.Now(), Valid: true}
	modelConfig := database.ChatModelConfig{
		ID:           modelConfigID,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: 8192,
	}
	updatedChat := lockedChat
	updatedChat.Title = wantTitle
	unlockedChat := updatedChat
	unlockedChat.WorkerID = uuid.NullUUID{}
	unlockedChat.StartedAt = sql.NullTime{}

	messageEvents := make(chan struct {
		payload codersdk.ChatWatchEvent
		err     error
	}, 1)
	cancelSub, err := pubsub.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(ownerID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
			messageEvents <- struct {
				payload codersdk.ChatWatchEvent
				err     error
			}{payload: payload, err: err}
		}),
	)
	require.NoError(t, err)
	defer cancelSub()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		require.Equal(t, "gpt-4o-mini", req.Model)
		return chattest.OpenAINonStreamingResponse("{\"title\":\"" + wantTitle + "\"}")
	})

	server := &Server{
		db:          db,
		logger:      logger,
		pubsub:      pubsub,
		configCache: newChatConfigCache(context.Background(), db, clock),
	}

	db.EXPECT().GetChatModelConfigByID(gomock.Any(), modelConfigID).Return(modelConfig, nil)
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{
		Provider:             "openai",
		CentralApiKeyEnabled: true,
		APIKey:               "test-key",
		BaseUrl:              serverURL,
	}}, nil)
	db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(database.ChatUsageLimitConfig{}, sql.ErrNoRows)
	db.EXPECT().GetChatMessagesByChatIDAscPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDAscPaginatedParams{
			ChatID:   chatID,
			AfterID:  0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return([]database.ChatMessage{
		mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText(userPrompt),
		),
		mustChatMessage(
			t,
			database.ChatMessageRoleAssistant,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("checking the diff now"),
		),
	}, nil)
	db.EXPECT().GetChatMessagesByChatIDDescPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDDescPaginatedParams{
			ChatID:   chatID,
			BeforeID: 0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return(nil, nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(nil, nil)

	gomock.InOrder(
		db.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("chat_title_regenerate_lock")).DoAndReturn(
			func(fn func(database.Store) error, opts *database.TxOptions) error {
				require.Equal(t, "chat_title_regenerate_lock", opts.TxIdentifier)
				return fn(lockTx)
			},
		),
		db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
			func(fn func(database.Store) error, opts *database.TxOptions) error {
				require.Nil(t, opts)
				return fn(usageTx)
			},
		),
		db.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("chat_title_regenerate_unlock")).DoAndReturn(
			func(fn func(database.Store) error, opts *database.TxOptions) error {
				require.Equal(t, "chat_title_regenerate_unlock", opts.TxIdentifier)
				return fn(unlockTx)
			},
		),
	)

	lockTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(chat, nil)
	lockTx.EXPECT().UpdateChatStatusPreserveUpdatedAt(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.UpdateChatStatusPreserveUpdatedAtParams{}),
	).DoAndReturn(func(_ context.Context, arg database.UpdateChatStatusPreserveUpdatedAtParams) (database.Chat, error) {
		require.Equal(t, chat.ID, arg.ID)
		require.Equal(t, chat.Status, arg.Status)
		require.Equal(t, uuid.NullUUID{UUID: manualTitleLockWorkerID, Valid: true}, arg.WorkerID)
		require.True(t, arg.StartedAt.Valid)
		require.WithinDuration(t, time.Now(), arg.StartedAt.Time, time.Second)
		require.False(t, arg.HeartbeatAt.Valid)
		require.Equal(t, chat.LastError, arg.LastError)
		require.Equal(t, chat.UpdatedAt, arg.UpdatedAt)
		return lockedChat, nil
	})

	usageTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(lockedChat, nil)
	usageTx.EXPECT().InsertChatMessages(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatMessagesParams{})).DoAndReturn(
		func(_ context.Context, arg database.InsertChatMessagesParams) ([]database.ChatMessage, error) {
			require.Equal(t, []uuid.UUID{ownerID}, arg.CreatedBy)
			require.Equal(t, []uuid.UUID{modelConfigID}, arg.ModelConfigID)
			require.Equal(t, []string{"[]"}, arg.Content)
			return []database.ChatMessage{{ID: 91}}, nil
		},
	)
	usageTx.EXPECT().SoftDeleteChatMessageByID(gomock.Any(), int64(91)).Return(nil)
	usageTx.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
		ID:    chatID,
		Title: wantTitle,
	}).Return(updatedChat, nil)

	unlockTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(updatedChat, nil)
	unlockTx.EXPECT().UpdateChatStatusPreserveUpdatedAt(
		gomock.Any(),
		database.UpdateChatStatusPreserveUpdatedAtParams{
			ID:          updatedChat.ID,
			Status:      updatedChat.Status,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   updatedChat.LastError,
			UpdatedAt:   updatedChat.UpdatedAt,
		},
	).Return(unlockedChat, nil)

	gotChat, err := server.RegenerateChatTitle(ctx, chat)
	require.NoError(t, err)
	require.Equal(t, updatedChat, gotChat)

	select {
	case event := <-messageEvents:
		require.NoError(t, event.err)
		require.Equal(t, codersdk.ChatWatchEventKindTitleChange, event.payload.Kind)
		require.Equal(t, chatID, event.payload.Chat.ID)
		require.Equal(t, wantTitle, event.payload.Chat.Title)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for title change pubsub event")
	}
}

func TestResolveUserProviderAPIKeys_StripsDisabledFallbackKeys(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()

	server := &Server{
		db: db,
		configCache: newChatConfigCache(
			context.Background(),
			db,
			quartz.NewReal(),
		),
		providerAPIKeys: chatprovider.ProviderAPIKeys{
			OpenAI:    "openai-deployment-key",
			Anthropic: "anthropic-deployment-key",
			ByProvider: map[string]string{
				"openai":    "openai-deployment-key",
				"anthropic": "anthropic-deployment-key",
			},
			BaseURLByProvider: map[string]string{
				"openai":    "https://openai.example.com",
				"anthropic": "https://anthropic.example.com",
			},
		},
	}

	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{
		Provider:                   "anthropic",
		CentralApiKeyEnabled:       true,
		AllowCentralApiKeyFallback: true,
	}}, nil)

	keys, err := server.resolveUserProviderAPIKeys(ctx, ownerID)
	require.NoError(t, err)
	require.Empty(t, keys.OpenAI)
	require.Empty(t, keys.APIKey("openai"))
	require.Empty(t, keys.BaseURL("openai"))
	require.Equal(t, "anthropic-deployment-key", keys.Anthropic)
	require.Equal(t, "anthropic-deployment-key", keys.APIKey("anthropic"))
	require.Equal(t, "https://anthropic.example.com", keys.BaseURL("anthropic"))
	require.Equal(t, map[string]string{"anthropic": "anthropic-deployment-key"}, keys.ByProvider)
	require.Equal(t, map[string]string{"anthropic": "https://anthropic.example.com"}, keys.BaseURLByProvider)
}

func TestResolveUserProviderAPIKeys_SkipsUserKeyLookupWhenNoProviderAllowsUserKeys(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()

	server := &Server{
		db: db,
		configCache: newChatConfigCache(
			context.Background(),
			db,
			quartz.NewReal(),
		),
		providerAPIKeys: chatprovider.ProviderAPIKeys{
			OpenAI: "openai-deployment-key",
			ByProvider: map[string]string{
				"openai": "openai-deployment-key",
			},
		},
	}

	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return([]database.ChatProvider{{
		Provider:             "openai",
		CentralApiKeyEnabled: true,
	}}, nil)

	keys, err := server.resolveUserProviderAPIKeys(ctx, ownerID)
	require.NoError(t, err)
	require.Equal(t, "openai-deployment-key", keys.OpenAI)
	require.Equal(t, "openai-deployment-key", keys.APIKey("openai"))
}

func TestRefreshChatWorkspaceSnapshot_NoReloadWhenWorkspacePresent(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			calls++
			return database.Chat{}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, chat, refreshed)
	require.Equal(t, 0, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReloadsWhenWorkspaceMissing(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workspaceID := uuid.New()
	chat := database.Chat{ID: chatID}
	reloaded := database.Chat{
		ID: chatID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(_ context.Context, id uuid.UUID) (database.Chat, error) {
			calls++
			require.Equal(t, chatID, id)
			return reloaded, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, reloaded, refreshed)
	require.Equal(t, 1, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReturnsReloadError(t *testing.T) {
	t.Parallel()

	chat := database.Chat{ID: uuid.New()}
	loadErr := xerrors.New("boom")

	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, loadErr
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "reload chat workspace state")
	require.ErrorContains(t, err, loadErr.Error())
	require.Equal(t, chat, refreshed)
}

func TestPersistInstructionFilesIncludesAgentMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{
		ID:                agentID,
		OperatingSystem:   "linux",
		Directory:         "/home/coder/project",
		ExpandedDirectory: "/home/coder/project",
	}

	db.EXPECT().GetWorkspaceAgentByID(
		gomock.Any(),
		agentID,
	).Return(workspaceAgent, nil).Times(1)
	db.EXPECT().InsertChatMessages(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().UpdateChatLastInjectedContext(gomock.Any(),
		gomock.Cond(func(x any) bool {
			arg, ok := x.(database.UpdateChatLastInjectedContextParams)
			if !ok || arg.ID != chat.ID {
				return false
			}
			if !arg.LastInjectedContext.Valid {
				return false
			}
			var parts []codersdk.ChatMessagePart
			if err := json.Unmarshal(arg.LastInjectedContext.RawMessage, &parts); err != nil {
				return false
			}
			// Expect at least one context-file part for the
			// working-directory AGENTS.md, with internal fields
			// stripped (no content, OS, or directory).
			for _, p := range parts {
				if p.Type == codersdk.ChatMessagePartTypeContextFile && p.ContextFilePath != "" {
					return p.ContextFileContent == "" &&
						p.ContextFileOS == "" &&
						p.ContextFileDirectory == ""
				}
			}
			return false
		}),
	).Return(database.Chat{}, nil).Times(1)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	conn.EXPECT().ContextConfig(gomock.Any()).Return(workspacesdk.ContextConfigResponse{
		Parts: []codersdk.ChatMessagePart{{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/home/coder/project/AGENTS.md",
			ContextFileContent: "# Project instructions",
		}},
	}, nil).AnyTimes()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:                       db,
		logger:                   logger,
		instructionLookupTimeout: 5 * time.Second,
		agentConnFn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return conn, func() {}, nil
		},
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	instruction, _, err := server.persistInstructionFiles(
		ctx,
		chat,
		uuid.New(),
		workspaceCtx.getWorkspaceAgent,
		workspaceCtx.getWorkspaceConn,
	)
	require.NoError(t, err)
	require.Contains(t, instruction, "Operating System: linux")
	require.Contains(t, instruction, "Working Directory: /home/coder/project")
}

func TestPersistInstructionFilesSkipsSentinelWhenWorkspaceUnavailable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		},
	}
	server := &Server{
		db:     db,
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}

	instruction, _, err := server.persistInstructionFiles(
		ctx,
		chat,
		uuid.New(),
		func(context.Context) (database.WorkspaceAgent, error) {
			return database.WorkspaceAgent{
				ID:        uuid.New(),
				Directory: "/home/coder/project",
			}, nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			return nil, errChatHasNoWorkspaceAgent
		},
	)
	require.NoError(t, err)
	require.Empty(t, instruction)
}

func TestPersistInstructionFilesSentinelWithSkills(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{
		ID:                agentID,
		OperatingSystem:   "linux",
		Directory:         "/home/coder/project",
		ExpandedDirectory: "/home/coder/project",
	}

	db.EXPECT().GetWorkspaceAgentByID(
		gomock.Any(),
		agentID,
	).Return(workspaceAgent, nil).Times(1)
	db.EXPECT().InsertChatMessages(gomock.Any(),
		gomock.Cond(func(x any) bool {
			arg, ok := x.(database.InsertChatMessagesParams)
			if !ok || arg.ChatID != chat.ID || len(arg.Content) != 1 {
				return false
			}
			var parts []codersdk.ChatMessagePart
			if err := json.Unmarshal([]byte(arg.Content[0]), &parts); err != nil {
				return false
			}
			foundMarker := false
			foundSkill := false
			for _, p := range parts {
				switch p.Type {
				case codersdk.ChatMessagePartTypeContextFile:
					if p.ContextFileAgentID == (uuid.NullUUID{UUID: agentID, Valid: true}) && p.ContextFileContent == "" {
						foundMarker = true
					}
				case codersdk.ChatMessagePartTypeSkill:
					if p.SkillName == "my-skill" && p.ContextFileAgentID == (uuid.NullUUID{UUID: agentID, Valid: true}) {
						foundSkill = true
					}
				}
			}
			return foundMarker && foundSkill
		}),
	).Return(nil, nil).Times(1)
	db.EXPECT().UpdateChatLastInjectedContext(gomock.Any(),
		gomock.Cond(func(x any) bool {
			arg, ok := x.(database.UpdateChatLastInjectedContextParams)
			if !ok || arg.ID != chat.ID {
				return false
			}
			if !arg.LastInjectedContext.Valid {
				return false
			}
			var parts []codersdk.ChatMessagePart
			if err := json.Unmarshal(arg.LastInjectedContext.RawMessage, &parts); err != nil {
				return false
			}
			// The sentinel path should persist only skill parts
			// with ContextFileAgentID set.
			for _, p := range parts {
				if p.Type == codersdk.ChatMessagePartTypeSkill &&
					p.SkillName == "my-skill" &&
					p.ContextFileAgentID == (uuid.NullUUID{UUID: agentID, Valid: true}) {
					return true
				}
			}
			return false
		}),
	).Return(database.Chat{}, nil).Times(1)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	conn.EXPECT().ContextConfig(gomock.Any()).Return(workspacesdk.ContextConfigResponse{
		// Agent returns pre-read content: no instruction files
		// found but one skill discovered.
		Parts: []codersdk.ChatMessagePart{{
			Type:             codersdk.ChatMessagePartTypeSkill,
			SkillName:        "my-skill",
			SkillDescription: "A test skill",
			SkillDir:         "/home/coder/project/.agents/skills/my-skill",
		}},
	}, nil).AnyTimes()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:                       db,
		logger:                   logger,
		instructionLookupTimeout: 5 * time.Second,
		agentConnFn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return conn, func() {}, nil
		},
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	instruction, skills, err := server.persistInstructionFiles(
		ctx,
		chat,
		uuid.New(),
		workspaceCtx.getWorkspaceAgent,
		workspaceCtx.getWorkspaceConn,
	)
	require.NoError(t, err)
	// Sentinel path returns empty instruction string.
	require.Empty(t, instruction)
	// Skills are still discovered and returned.
	require.Len(t, skills, 1)
	require.Equal(t, "my-skill", skills[0].Name)
}

func TestPersistInstructionFilesSentinelNoSkillsClearsColumn(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{
		ID:                agentID,
		OperatingSystem:   "linux",
		Directory:         "/home/coder/project",
		ExpandedDirectory: "/home/coder/project",
	}

	db.EXPECT().GetWorkspaceAgentByID(
		gomock.Any(),
		agentID,
	).Return(workspaceAgent, nil).Times(1)
	db.EXPECT().InsertChatMessages(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().UpdateChatLastInjectedContext(gomock.Any(),
		gomock.Cond(func(x any) bool {
			arg, ok := x.(database.UpdateChatLastInjectedContextParams)
			if !ok || arg.ID != chat.ID {
				return false
			}
			// No skills discovered, so the column should be
			// cleared to NULL.
			return !arg.LastInjectedContext.Valid
		}),
	).Return(database.Chat{}, nil).Times(1)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	conn.EXPECT().ContextConfig(gomock.Any()).Return(workspacesdk.ContextConfigResponse{
		// Agent returns pre-read content: no files, no skills.
		Parts: []codersdk.ChatMessagePart{},
	}, nil).AnyTimes()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:                       db,
		logger:                   logger,
		instructionLookupTimeout: 5 * time.Second,
		agentConnFn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return conn, func() {}, nil
		},
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	instruction, skills, err := server.persistInstructionFiles(
		ctx,
		chat,
		uuid.New(),
		workspaceCtx.getWorkspaceAgent,
		workspaceCtx.getWorkspaceConn,
	)
	require.NoError(t, err)
	// Sentinel path: empty instruction, no skills.
	require.Empty(t, instruction)
	require.Empty(t, skills)
}

func TestTurnWorkspaceContext_BindingFirstPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{ID: agentID}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(workspaceAgent, nil).Times(1)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, chat, chatSnapshot)
	require.Equal(t, workspaceAgent, agent)

	gotAgent, err := workspaceCtx.getWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, workspaceAgent, gotAgent)
	require.Equal(t, chat, currentChat)
}

func TestTurnWorkspaceContext_NullBindingLazyBind(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	buildID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{ID: agentID}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: agentID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return([]database.WorkspaceAgent{workspaceAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
			ID:      chat.ID,
		}).Return(updatedChat, nil),
	)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, updatedChat, chatSnapshot)
	require.Equal(t, workspaceAgent, agent)
	require.Equal(t, updatedChat, currentChat)

	gotAgent, err := workspaceCtx.getWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, workspaceAgent, gotAgent)
}

func TestTurnWorkspaceContext_StaleBindingRepair(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	staleAgentID := uuid.New()
	buildID := uuid.New()
	currentAgentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  staleAgentID,
			Valid: true,
		},
	}
	currentAgent := database.WorkspaceAgent{ID: currentAgentID}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: currentAgentID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), staleAgentID).Return(database.WorkspaceAgent{}, xerrors.New("missing agent")),
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return([]database.WorkspaceAgent{currentAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: currentAgentID, Valid: true},
			ID:      chat.ID,
		}).Return(updatedChat, nil),
	)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, updatedChat, chatSnapshot)
	require.Equal(t, currentAgent, agent)
	require.Equal(t, updatedChat, currentChat)
}

func TestTurnWorkspaceContextGetWorkspaceConnLazyValidationSwitchesWorkspaceAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	staleAgentID := uuid.New()
	currentAgentID := uuid.New()
	buildID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  staleAgentID,
			Valid: true,
		},
	}
	staleAgent := database.WorkspaceAgent{ID: staleAgentID}
	currentAgent := database.WorkspaceAgent{ID: currentAgentID}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: currentAgentID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), staleAgentID).Return(staleAgent, nil),
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return([]database.WorkspaceAgent{currentAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), currentAgentID).Return(currentAgent, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: currentAgentID, Valid: true},
			ID:      chat.ID,
		}).Return(updatedChat, nil),
	)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	var dialed []uuid.UUID
	server := &Server{db: db}
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialed = append(dialed, agentID)
		if agentID == staleAgentID {
			return nil, nil, xerrors.New("dial failed")
		}
		return conn, func() {}, nil
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, conn, gotConn)
	require.Equal(t, []uuid.UUID{staleAgentID, currentAgentID}, dialed)
	require.Equal(t, updatedChat, currentChat)

	gotAgent, err := workspaceCtx.getWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, currentAgent, gotAgent)
}

func TestTurnWorkspaceContextGetWorkspaceConnFastFailsWithoutCurrentAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	staleAgentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  staleAgentID,
			Valid: true,
		},
	}

	staleAgent := database.WorkspaceAgent{ID: staleAgentID}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), staleAgentID).
		Return(staleAgent, nil).
		Times(1)
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{}, nil).
		Times(1)

	server := &Server{db: db}
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return nil, nil, xerrors.New("dial failed")
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	require.ErrorIs(t, err, errChatHasNoWorkspaceAgent)

	workspaceCtx.mu.Lock()
	defer workspaceCtx.mu.Unlock()
	require.Equal(t, database.WorkspaceAgent{}, workspaceCtx.agent)
	require.False(t, workspaceCtx.agentLoaded)
	require.Nil(t, workspaceCtx.conn)
	require.Nil(t, workspaceCtx.releaseConn)
	require.Equal(t, uuid.NullUUID{}, workspaceCtx.cachedWorkspaceID)
}

func TestTurnWorkspaceContext_SelectWorkspaceClearsCachedState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	currentChat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		},
	}
	updatedChat := database.Chat{
		ID: currentChat.ID,
		WorkspaceID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		},
	}
	cachedConn := agentconnmock.NewMockAgentConn(ctrl)
	releaseCalls := 0

	workspaceCtx := turnWorkspaceContext{
		chatStateMu: &sync.Mutex{},
		currentChat: &currentChat,
	}
	workspaceCtx.agent = database.WorkspaceAgent{ID: uuid.New()}
	workspaceCtx.agentLoaded = true
	workspaceCtx.conn = cachedConn
	workspaceCtx.cachedWorkspaceID = currentChat.WorkspaceID
	workspaceCtx.releaseConn = func() {
		releaseCalls++
	}

	workspaceCtx.selectWorkspace(updatedChat)

	require.Equal(t, updatedChat, currentChat)
	require.Equal(t, 1, releaseCalls)

	workspaceCtx.mu.Lock()
	defer workspaceCtx.mu.Unlock()
	require.Equal(t, database.WorkspaceAgent{}, workspaceCtx.agent)
	require.False(t, workspaceCtx.agentLoaded)
	require.Nil(t, workspaceCtx.conn)
	require.Nil(t, workspaceCtx.releaseConn)
	require.Equal(t, uuid.NullUUID{}, workspaceCtx.cachedWorkspaceID)
}

func TestTurnWorkspaceContext_EnsureWorkspaceAgentIgnoresCachedAgentForDifferentWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceOneID := uuid.New()
	workspaceTwoID := uuid.New()
	buildID := uuid.New()
	cachedAgent := database.WorkspaceAgent{ID: uuid.New()}
	resolvedAgent := database.WorkspaceAgent{ID: uuid.New()}
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceTwoID,
			Valid: true,
		},
	}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: resolvedAgent.ID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceTwoID).Return([]database.WorkspaceAgent{resolvedAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceTwoID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			ID:      chat.ID,
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: resolvedAgent.ID, Valid: true},
		}).Return(updatedChat, nil),
	)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	workspaceCtx.agent = cachedAgent
	workspaceCtx.agentLoaded = true
	workspaceCtx.cachedWorkspaceID = uuid.NullUUID{UUID: workspaceOneID, Valid: true}
	defer workspaceCtx.close()

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, updatedChat, chatSnapshot)
	require.Equal(t, resolvedAgent, agent)
	require.Equal(t, updatedChat, currentChat)
}

func TestSubscribeSkipsDatabaseCatchupForLocallyDeliveredMessage(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}
	initialMessage := database.ChatMessage{
		ID:     1,
		ChatID: chatID,
		Role:   database.ChatMessageRoleUser,
	}
	localMessage := database.ChatMessage{
		ID:     2,
		ChatID: chatID,
		Role:   database.ChatMessageRoleAssistant,
	}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{initialMessage}, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	server.publishMessage(chatID, localMessage)

	event := requireStreamMessageEvent(t, events)
	require.Equal(t, int64(2), event.Message.ID)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

func TestSubscribeDeliversLocalDurableMessageBeforeDelayedPubsubCatchup(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}
	initialMessage := database.ChatMessage{
		ID:     1,
		ChatID: chatID,
		Role:   database.ChatMessageRoleUser,
	}
	localMessage := database.ChatMessage{
		ID:     2,
		ChatID: chatID,
		Role:   database.ChatMessageRoleAssistant,
	}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{initialMessage}, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
	)

	ps := newDelayedAfterMessagePubsub(dbpubsub.NewInMemory())
	ps.DelayAfterMessage(coderdpubsub.ChatStreamNotifyChannel(chatID), localMessage.ID-1)
	server := &Server{
		db:     db,
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		pubsub: ps,
	}

	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	server.publishMessage(chatID, localMessage)
	server.publishStatus(chatID, database.ChatStatusWaiting, uuid.NullUUID{})

	messageEvent := requireStreamMessageEvent(t, events)
	require.Equal(t, localMessage.ID, messageEvent.Message.ID)

	select {
	case event, ok := <-events:
		require.True(t, ok, "chat stream closed before delivering status")
		require.Equal(t, codersdk.ChatStreamEventTypeStatus, event.Type)
		require.NotNil(t, event.Status)
		require.Equal(t, codersdk.ChatStatusWaiting, event.Status.Status)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat stream status event")
	}

	require.NoError(t, ps.ReleaseAfterMessage(coderdpubsub.ChatStreamNotifyChannel(chatID), localMessage.ID-1))

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case event, ok := <-events:
			require.True(t, ok, "chat stream closed unexpectedly")
			require.NotEqual(t, codersdk.ChatStreamEventTypeMessage, event.Type,
				"delayed pubsub catch-up should not redeliver the durable message")
		case <-deadline:
			return
		}
	}
}

func TestSubscribeUsesDurableCacheWhenLocalMessageWasNotDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}
	initialMessage := database.ChatMessage{
		ID:     1,
		ChatID: chatID,
		Role:   database.ChatMessageRoleUser,
	}
	cachedMessage := codersdk.ChatMessage{
		ID:     2,
		ChatID: chatID,
		Role:   codersdk.ChatMessageRoleAssistant,
	}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{initialMessage}, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
	)

	server := newSubscribeTestServer(t, db)
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		ChatID:  chatID,
		Message: &cachedMessage,
	})

	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		AfterMessageID: 1,
	})

	event := requireStreamMessageEvent(t, events)
	require.Equal(t, int64(2), event.Message.ID)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

func TestSubscribeQueriesDatabaseWhenDurableCacheMisses(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}
	initialMessage := database.ChatMessage{
		ID:     1,
		ChatID: chatID,
		Role:   database.ChatMessageRoleUser,
	}
	catchupMessage := database.ChatMessage{
		ID:     2,
		ChatID: chatID,
		Role:   database.ChatMessageRoleAssistant,
	}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{initialMessage}, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 1,
		}).Return([]database.ChatMessage{catchupMessage}, nil),
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		AfterMessageID: 1,
	})

	event := requireStreamMessageEvent(t, events)
	require.Equal(t, int64(2), event.Message.ID)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

func TestSubscribeFullRefreshStillUsesDatabaseCatchup(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}
	initialMessage := database.ChatMessage{
		ID:     1,
		ChatID: chatID,
		Role:   database.ChatMessageRoleUser,
	}
	editedMessage := database.ChatMessage{
		ID:     1,
		ChatID: chatID,
		Role:   database.ChatMessageRoleUser,
	}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{initialMessage}, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{editedMessage}, nil),
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	server.publishEditedMessage(chatID, editedMessage)

	event := requireStreamMessageEvent(t, events)
	require.Equal(t, int64(1), event.Message.ID)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

func TestSubscribeDeliversRetryEventViaPubsubOnce(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return(nil, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	retryingAt := time.Unix(1_700_000_000, 0).UTC()
	expected := &codersdk.ChatStreamRetry{
		Attempt:    1,
		DelayMs:    (1500 * time.Millisecond).Milliseconds(),
		Error:      "OpenAI is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
		Provider:   "openai",
		StatusCode: 429,
		RetryingAt: retryingAt,
	}

	server.publishRetry(chatID, expected)

	event := requireStreamRetryEvent(t, events)
	require.Equal(t, expected, event.Retry)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

func TestSubscribePrefersStructuredErrorPayloadViaPubsub(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return(nil, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	classified := chaterror.ClassifiedError{
		Message:    "OpenAI is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 429,
	}
	server.publishError(chatID, classified)

	event := requireStreamErrorEvent(t, events)
	require.Equal(t, chaterror.StreamErrorPayload(classified), event.Error)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

func TestSubscribeFallsBackToLegacyErrorStringViaPubsub(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusPending}

	gomock.InOrder(
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return(nil, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		Error: "legacy error only",
	})

	event := requireStreamErrorEvent(t, events)
	require.Equal(t, &codersdk.ChatStreamError{Message: "legacy error only"}, event.Error)
	requireNoStreamEvent(t, events, 200*time.Millisecond)
}

type delayedAfterMessageKey struct {
	event          string
	afterMessageID int64
}

// delayedAfterMessagePubsub buffers selected AfterMessageID notifications until
// a test releases them. This models batched durable-message pubsub delivery
// while leaving local stream delivery immediate.
type delayedAfterMessagePubsub struct {
	inner dbpubsub.Pubsub

	mu              sync.Mutex
	delayEnabled    map[delayedAfterMessageKey]bool
	delayedMessages map[delayedAfterMessageKey][][]byte
}

func newDelayedAfterMessagePubsub(inner dbpubsub.Pubsub) *delayedAfterMessagePubsub {
	return &delayedAfterMessagePubsub{
		inner:           inner,
		delayEnabled:    make(map[delayedAfterMessageKey]bool),
		delayedMessages: make(map[delayedAfterMessageKey][][]byte),
	}
}

func (p *delayedAfterMessagePubsub) DelayAfterMessage(event string, afterMessageID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.delayEnabled[delayedAfterMessageKey{event: event, afterMessageID: afterMessageID}] = true
}

func (p *delayedAfterMessagePubsub) ReleaseAfterMessage(event string, afterMessageID int64) error {
	key := delayedAfterMessageKey{event: event, afterMessageID: afterMessageID}

	p.mu.Lock()
	delete(p.delayEnabled, key)
	messages := p.delayedMessages[key]
	delete(p.delayedMessages, key)
	p.mu.Unlock()

	for _, message := range messages {
		if err := p.inner.Publish(event, message); err != nil {
			return err
		}
	}
	return nil
}

func (p *delayedAfterMessagePubsub) Subscribe(event string, listener dbpubsub.Listener) (func(), error) {
	return p.inner.Subscribe(event, listener)
}

func (p *delayedAfterMessagePubsub) SubscribeWithErr(event string, listener dbpubsub.ListenerWithErr) (func(), error) {
	return p.inner.SubscribeWithErr(event, listener)
}

func (p *delayedAfterMessagePubsub) Publish(event string, message []byte) error {
	var notify coderdpubsub.ChatStreamNotifyMessage
	if err := json.Unmarshal(message, &notify); err == nil {
		key := delayedAfterMessageKey{event: event, afterMessageID: notify.AfterMessageID}
		p.mu.Lock()
		delay := p.delayEnabled[key]
		if delay {
			p.delayedMessages[key] = append(p.delayedMessages[key], append([]byte(nil), message...))
		}
		p.mu.Unlock()
		if delay {
			return nil
		}
	}
	return p.inner.Publish(event, message)
}

func (p *delayedAfterMessagePubsub) Close() error {
	return p.inner.Close()
}

func newSubscribeTestServer(t *testing.T, db database.Store) *Server {
	t.Helper()

	return &Server{
		db:     db,
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		pubsub: dbpubsub.NewInMemory(),
	}
}

func requireStreamMessageEvent(t *testing.T, events <-chan codersdk.ChatStreamEvent) codersdk.ChatStreamEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "chat stream closed before delivering an event")
		require.Equal(t, codersdk.ChatStreamEventTypeMessage, event.Type)
		require.NotNil(t, event.Message)
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat stream message event")
		return codersdk.ChatStreamEvent{}
	}
}

func requireStreamRetryEvent(t *testing.T, events <-chan codersdk.ChatStreamEvent) codersdk.ChatStreamEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "chat stream closed before delivering an event")
		require.Equal(t, codersdk.ChatStreamEventTypeRetry, event.Type)
		require.NotNil(t, event.Retry)
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat stream retry event")
		return codersdk.ChatStreamEvent{}
	}
}

func requireStreamErrorEvent(t *testing.T, events <-chan codersdk.ChatStreamEvent) codersdk.ChatStreamEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "chat stream closed before delivering an event")
		require.Equal(t, codersdk.ChatStreamEventTypeError, event.Type)
		require.NotNil(t, event.Error)
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat stream error event")
		return codersdk.ChatStreamEvent{}
	}
}

func requireNoStreamEvent(t *testing.T, events <-chan codersdk.ChatStreamEvent, wait time.Duration) {
	t.Helper()

	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("chat stream closed unexpectedly")
		}
		t.Fatalf("unexpected chat stream event: %+v", event)
	case <-time.After(wait):
	}
}

// TestPublishToStream_DropWarnRateLimiting walks through a
// realistic lifecycle: buffer fills up, subscriber channel fills
// up, counters get reset between steps. It verifies that WARN
// logs are rate-limited to at most once per streamDropWarnInterval
// and that counter resets re-enable an immediate WARN.
func TestPublishToStream_DropWarnRateLimiting(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	mClock := quartz.NewMock(t)

	server := &Server{
		logger: sink.Logger(),
		clock:  mClock,
	}

	chatID := uuid.New()
	subCh := make(chan codersdk.ChatStreamEvent, 1)
	subCh <- codersdk.ChatStreamEvent{} // pre-fill so sends always drop

	// Set up state that mirrors a running chat: buffer at capacity,
	// buffering enabled, one saturated subscriber.
	state := &chatStreamState{
		buffering: true,
		buffer:    make([]codersdk.ChatStreamEvent, maxStreamBufferSize),
		subscribers: map[uuid.UUID]chan codersdk.ChatStreamEvent{
			uuid.New(): subCh,
		},
	}
	server.chatStreams.Store(chatID, state)

	bufferMsg := "chat stream buffer full, dropping oldest event"
	subMsg := "dropping chat stream event"

	filter := func(level slog.Level, msg string) func(slog.SinkEntry) bool {
		return func(e slog.SinkEntry) bool {
			return e.Level == level && e.Message == msg
		}
	}

	// --- Phase 1: buffer-full rate limiting ---
	// message_part events hit both the buffer-full and subscriber-full
	// paths. The first publish triggers a WARN for each; the rest
	// within the window are DEBUG.
	partEvent := codersdk.ChatStreamEvent{
		Type:        codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{},
	}
	for i := 0; i < 50; i++ {
		server.publishToStream(chatID, partEvent)
	}

	require.Len(t, sink.Entries(filter(slog.LevelWarn, bufferMsg)), 1)
	require.Empty(t, sink.Entries(filter(slog.LevelDebug, bufferMsg)))
	requireFieldValue(t, sink.Entries(filter(slog.LevelWarn, bufferMsg))[0], "dropped_count", int64(1))

	// Subscriber also saw 50 drops (one per publish).
	require.Len(t, sink.Entries(filter(slog.LevelWarn, subMsg)), 1)
	require.Empty(t, sink.Entries(filter(slog.LevelDebug, subMsg)))
	requireFieldValue(t, sink.Entries(filter(slog.LevelWarn, subMsg))[0], "dropped_count", int64(1))

	// --- Phase 2: clock advance triggers second WARN with count ---
	mClock.Advance(streamDropWarnInterval + time.Second)
	server.publishToStream(chatID, partEvent)

	bufWarn := sink.Entries(filter(slog.LevelWarn, bufferMsg))
	require.Len(t, bufWarn, 2)
	requireFieldValue(t, bufWarn[1], "dropped_count", int64(50))

	subWarn := sink.Entries(filter(slog.LevelWarn, subMsg))
	require.Len(t, subWarn, 2)
	requireFieldValue(t, subWarn[1], "dropped_count", int64(50))

	// --- Phase 3: counter reset (simulates step persist) ---
	state.mu.Lock()
	state.buffer = make([]codersdk.ChatStreamEvent, maxStreamBufferSize)
	state.resetDropCounters()
	state.mu.Unlock()

	// The very next drop should WARN immediately — the reset zeroed
	// lastWarnAt so the interval check passes.
	server.publishToStream(chatID, partEvent)

	bufWarn = sink.Entries(filter(slog.LevelWarn, bufferMsg))
	require.Len(t, bufWarn, 3, "expected WARN immediately after counter reset")
	requireFieldValue(t, bufWarn[2], "dropped_count", int64(1))

	subWarn = sink.Entries(filter(slog.LevelWarn, subMsg))
	require.Len(t, subWarn, 3, "expected subscriber WARN immediately after counter reset")
	requireFieldValue(t, subWarn[2], "dropped_count", int64(1))
}

func TestResolveUserCompactionThreshold(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	modelConfigID := uuid.New()
	expectedKey := codersdk.CompactionThresholdKey(modelConfigID)

	tests := []struct {
		name        string
		dbReturn    string
		dbErr       error
		wantVal     int32
		wantOK      bool
		wantWarnLog bool
	}{
		{
			name:   "NoRowsReturnsDefault",
			dbErr:  sql.ErrNoRows,
			wantOK: false,
		},
		{
			name:     "ValidOverride",
			dbReturn: "75",
			wantVal:  75,
			wantOK:   true,
		},
		{
			name:     "OutOfRangeValue",
			dbReturn: "101",
			wantOK:   false,
		},
		{
			name:     "NonIntegerValue",
			dbReturn: "abc",
			wantOK:   false,
		},
		{
			name:        "UnexpectedDBError",
			dbErr:       xerrors.New("connection refused"),
			wantOK:      false,
			wantWarnLog: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockDB := dbmock.NewMockStore(ctrl)
			sink := testutil.NewFakeSink(t)

			srv := &Server{
				db:     mockDB,
				logger: sink.Logger(),
			}

			mockDB.EXPECT().GetUserChatCompactionThreshold(gomock.Any(), database.GetUserChatCompactionThresholdParams{
				UserID: userID,
				Key:    expectedKey,
			}).Return(tc.dbReturn, tc.dbErr)

			val, ok := srv.resolveUserCompactionThreshold(context.Background(), userID, modelConfigID)
			require.Equal(t, tc.wantVal, val)
			require.Equal(t, tc.wantOK, ok)

			warns := sink.Entries(func(e slog.SinkEntry) bool {
				return e.Level == slog.LevelWarn
			})
			if tc.wantWarnLog {
				require.NotEmpty(t, warns, "expected a warning log entry")
				return
			}
			require.Empty(t, warns, "unexpected warning log entry")
		})
	}
}

// requireFieldValue asserts that a SinkEntry contains a field with
// the given name and value.
func requireFieldValue(t *testing.T, entry slog.SinkEntry, name string, expected interface{}) {
	t.Helper()
	for _, f := range entry.Fields {
		if f.Name == name {
			require.Equal(t, expected, f.Value, "field %q value mismatch", name)
			return
		}
	}
	t.Fatalf("field %q not found in log entry", name)
}

func TestSkillsFromParts(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := skillsFromParts(nil)
		require.Empty(t, got)
	})

	t.Run("NoSkillParts", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeText, Text: "hello"},
			}),
		}
		got := skillsFromParts(msgs)
		require.Empty(t, got)
	})

	t.Run("SingleSkill", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:             codersdk.ChatMessagePartTypeSkill,
					SkillName:        "deep-review",
					SkillDescription: "Multi-reviewer code review",
					SkillDir:         "/home/coder/.agents/skills/deep-review",
				},
			}),
		}
		got := skillsFromParts(msgs)
		require.Len(t, got, 1)
		require.Equal(t, "deep-review", got[0].Name)
		require.Equal(t, "Multi-reviewer code review", got[0].Description)
		require.Equal(t, "/home/coder/.agents/skills/deep-review", got[0].Dir)
	})

	t.Run("MultipleSkillsAcrossMessages", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:      codersdk.ChatMessagePartTypeSkill,
					SkillName: "pull-requests",
					SkillDir:  "/home/coder/.agents/skills/pull-requests",
				},
			}),
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:      codersdk.ChatMessagePartTypeSkill,
					SkillName: "deep-review",
					SkillDir:  "/home/coder/.agents/skills/deep-review",
				},
			}),
		}
		got := skillsFromParts(msgs)
		require.Len(t, got, 2)
		require.Equal(t, "pull-requests", got[0].Name)
		require.Equal(t, "deep-review", got[1].Name)
	})

	t.Run("MixedPartTypes", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:            codersdk.ChatMessagePartTypeContextFile,
					ContextFilePath: "/home/coder/.coder/AGENTS.md",
				},
				{
					Type:      codersdk.ChatMessagePartTypeSkill,
					SkillName: "refine-plan",
					SkillDir:  "/home/coder/.agents/skills/refine-plan",
				},
			}),
			// A text-only message should be skipped entirely.
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeText, Text: "user turn"},
			}),
		}
		got := skillsFromParts(msgs)
		require.Len(t, got, 1)
		require.Equal(t, "refine-plan", got[0].Name)
		require.Equal(t, "/home/coder/.agents/skills/refine-plan", got[0].Dir)
	})

	t.Run("OptionalDescriptionOmitted", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:      codersdk.ChatMessagePartTypeSkill,
					SkillName: "refine-plan",
					SkillDir:  "/home/coder/.agents/skills/refine-plan",
				},
			}),
		}
		got := skillsFromParts(msgs)
		require.Len(t, got, 1)
		require.Equal(t, "refine-plan", got[0].Name)
		require.Empty(t, got[0].Description)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			{
				Content: pqtype.NullRawMessage{
					RawMessage: []byte(`not valid json with "skill" in it`),
					Valid:      true,
				},
			},
		}
		got := skillsFromParts(msgs)
		require.Empty(t, got)
	})

	t.Run("RoundTrip", func(t *testing.T) {
		// Simulate persist -> reconstruct cycle: marshal skill
		// parts the same way persistInstructionFiles does, then
		// verify skillsFromParts recovers the metadata.
		t.Parallel()
		want := []chattool.SkillMeta{
			{Name: "deep-review", Description: "Multi-reviewer review", Dir: "/skills/deep-review"},
			{Name: "pull-requests", Description: "", Dir: "/skills/pull-requests"},
		}
		agentID := uuid.New()
		var parts []codersdk.ChatMessagePart
		for _, s := range want {
			parts = append(parts, codersdk.ChatMessagePart{
				Type:               codersdk.ChatMessagePartTypeSkill,
				SkillName:          s.Name,
				SkillDescription:   s.Description,
				SkillDir:           s.Dir,
				ContextFileAgentID: uuid.NullUUID{UUID: agentID, Valid: true},
			})
		}
		msgs := []database.ChatMessage{chatMessageWithParts(parts)}
		got := skillsFromParts(msgs)
		require.Len(t, got, len(want))
		for i, w := range want {
			require.Equal(t, w.Name, got[i].Name)
			require.Equal(t, w.Description, got[i].Description)
			require.Equal(t, w.Dir, got[i].Dir)
		}
	})
}

func TestContextFileAgentID(t *testing.T) {
	t.Parallel()

	t.Run("EmptyMessages", func(t *testing.T) {
		t.Parallel()
		id, ok := contextFileAgentID(nil)
		require.Equal(t, uuid.Nil, id)
		require.False(t, ok)
	})

	t.Run("NoContextFileParts", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeText, Text: "hello"},
			}),
		}
		id, ok := contextFileAgentID(msgs)
		require.Equal(t, uuid.Nil, id)
		require.False(t, ok)
	})

	t.Run("SingleContextFile", func(t *testing.T) {
		t.Parallel()
		agentID := uuid.New()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:               codersdk.ChatMessagePartTypeContextFile,
					ContextFilePath:    "/some/path",
					ContextFileAgentID: uuid.NullUUID{UUID: agentID, Valid: true},
				},
			}),
		}
		id, ok := contextFileAgentID(msgs)
		require.Equal(t, agentID, id)
		require.True(t, ok)
	})

	t.Run("MultipleContextFiles", func(t *testing.T) {
		t.Parallel()
		agentID1 := uuid.New()
		agentID2 := uuid.New()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:               codersdk.ChatMessagePartTypeContextFile,
					ContextFilePath:    "/first/path",
					ContextFileAgentID: uuid.NullUUID{UUID: agentID1, Valid: true},
				},
			}),
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:               codersdk.ChatMessagePartTypeContextFile,
					ContextFilePath:    "/second/path",
					ContextFileAgentID: uuid.NullUUID{UUID: agentID2, Valid: true},
				},
			}),
		}
		id, ok := contextFileAgentID(msgs)
		require.Equal(t, agentID2, id)
		require.True(t, ok)
	})

	t.Run("IgnoresSkillOnlySentinel", func(t *testing.T) {
		t.Parallel()
		instructionAgentID := uuid.New()
		sentinelAgentID := uuid.New()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/AGENTS.md",
				ContextFileAgentID: uuid.NullUUID{UUID: instructionAgentID, Valid: true},
			}}),
			chatMessageWithParts([]codersdk.ChatMessagePart{{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: AgentChatContextSentinelPath,
				ContextFileAgentID: uuid.NullUUID{
					UUID:  sentinelAgentID,
					Valid: true,
				},
			}}),
		}
		id, ok := contextFileAgentID(msgs)
		require.Equal(t, instructionAgentID, id)
		require.True(t, ok)
	})

	t.Run("SentinelWithoutAgentID", func(t *testing.T) {
		t.Parallel()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{
				{
					Type:               codersdk.ChatMessagePartTypeContextFile,
					ContextFileAgentID: uuid.NullUUID{Valid: false},
				},
			}),
		}
		id, ok := contextFileAgentID(msgs)
		require.Equal(t, uuid.Nil, id)
		require.False(t, ok)
	})
}

func TestHasPersistedInstructionFiles(t *testing.T) {
	t.Parallel()

	t.Run("IgnoresAgentChatContextSentinel", func(t *testing.T) {
		t.Parallel()
		agentID := uuid.New()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: AgentChatContextSentinelPath,
				ContextFileAgentID: uuid.NullUUID{
					UUID:  agentID,
					Valid: true,
				},
			}}),
		}
		require.False(t, hasPersistedInstructionFiles(msgs))
	})

	t.Run("AcceptsPersistedInstructionFile", func(t *testing.T) {
		t.Parallel()
		agentID := uuid.New()
		msgs := []database.ChatMessage{
			chatMessageWithParts([]codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/workspace/AGENTS.md",
				ContextFileContent: "repo instructions",
				ContextFileAgentID: uuid.NullUUID{UUID: agentID, Valid: true},
			}}),
		}
		require.True(t, hasPersistedInstructionFiles(msgs))
	})
}

func TestInstructionFromContextFilesUsesLatestContextAgent(t *testing.T) {
	t.Parallel()

	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	msgs := []database.ChatMessage{
		chatMessageWithParts([]codersdk.ChatMessagePart{{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/old/AGENTS.md",
			ContextFileContent:   "old instructions",
			ContextFileOS:        "darwin",
			ContextFileDirectory: "/old",
			ContextFileAgentID:   uuid.NullUUID{UUID: oldAgentID, Valid: true},
		}}),
		chatMessageWithParts([]codersdk.ChatMessagePart{{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/new/AGENTS.md",
			ContextFileContent:   "new instructions",
			ContextFileOS:        "linux",
			ContextFileDirectory: "/new",
			ContextFileAgentID:   uuid.NullUUID{UUID: newAgentID, Valid: true},
		}}),
	}

	got := instructionFromContextFiles(msgs)
	require.Contains(t, got, "new instructions")
	require.Contains(t, got, "Operating System: linux")
	require.Contains(t, got, "Working Directory: /new")
	require.NotContains(t, got, "old instructions")
	require.NotContains(t, got, "Operating System: darwin")
}

func TestInstructionFromContextFilesKeepsLegacyUnstampedParts(t *testing.T) {
	t.Parallel()

	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	msgs := []database.ChatMessage{
		chatMessageWithParts([]codersdk.ChatMessagePart{{
			Type:               codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:    "/legacy/AGENTS.md",
			ContextFileContent: "legacy instructions",
		}}),
		chatMessageWithParts([]codersdk.ChatMessagePart{{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/old/AGENTS.md",
			ContextFileContent:   "old instructions",
			ContextFileOS:        "darwin",
			ContextFileDirectory: "/old",
			ContextFileAgentID:   uuid.NullUUID{UUID: oldAgentID, Valid: true},
		}}),
		chatMessageWithParts([]codersdk.ChatMessagePart{{
			Type:                 codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath:      "/new/AGENTS.md",
			ContextFileContent:   "new instructions",
			ContextFileOS:        "linux",
			ContextFileDirectory: "/new",
			ContextFileAgentID:   uuid.NullUUID{UUID: newAgentID, Valid: true},
		}}),
	}

	got := instructionFromContextFiles(msgs)
	require.Contains(t, got, "legacy instructions")
	require.Contains(t, got, "new instructions")
	require.Contains(t, got, "Operating System: linux")
	require.Contains(t, got, "Working Directory: /new")
	require.NotContains(t, got, "old instructions")
	require.NotContains(t, got, "Operating System: darwin")
}

func TestSkillsFromPartsKeepsLegacyUnstampedParts(t *testing.T) {
	t.Parallel()

	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	msgs := []database.ChatMessage{
		chatMessageWithParts([]codersdk.ChatMessagePart{{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "repo-helper-legacy",
			SkillDir:  "/skills/repo-helper-legacy",
		}}),
		chatMessageWithParts([]codersdk.ChatMessagePart{
			{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/old/AGENTS.md",
				ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
			},
			{
				Type:               codersdk.ChatMessagePartTypeSkill,
				SkillName:          "repo-helper-old",
				SkillDir:           "/skills/repo-helper-old",
				ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
			},
		}),
		chatMessageWithParts([]codersdk.ChatMessagePart{
			{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: AgentChatContextSentinelPath,
				ContextFileAgentID: uuid.NullUUID{
					UUID:  newAgentID,
					Valid: true,
				},
			},
			{
				Type:               codersdk.ChatMessagePartTypeSkill,
				SkillName:          "repo-helper-new",
				SkillDir:           "/skills/repo-helper-new",
				ContextFileAgentID: uuid.NullUUID{UUID: newAgentID, Valid: true},
			},
		}),
	}

	got := skillsFromParts(msgs)
	require.Equal(t, []chattool.SkillMeta{
		{Name: "repo-helper-legacy", Dir: "/skills/repo-helper-legacy"},
		{Name: "repo-helper-new", Dir: "/skills/repo-helper-new"},
	}, got)
}

func TestSkillsFromPartsUsesLatestContextAgent(t *testing.T) {
	t.Parallel()

	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	msgs := []database.ChatMessage{
		chatMessageWithParts([]codersdk.ChatMessagePart{
			{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    "/old/AGENTS.md",
				ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
			},
			{
				Type:               codersdk.ChatMessagePartTypeSkill,
				SkillName:          "repo-helper-old",
				SkillDir:           "/skills/repo-helper-old",
				ContextFileAgentID: uuid.NullUUID{UUID: oldAgentID, Valid: true},
			},
		}),
		chatMessageWithParts([]codersdk.ChatMessagePart{
			{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: AgentChatContextSentinelPath,
				ContextFileAgentID: uuid.NullUUID{
					UUID:  newAgentID,
					Valid: true,
				},
			},
			{
				Type:               codersdk.ChatMessagePartTypeSkill,
				SkillName:          "repo-helper-new",
				SkillDir:           "/skills/repo-helper-new",
				ContextFileAgentID: uuid.NullUUID{UUID: newAgentID, Valid: true},
			},
		}),
	}

	got := skillsFromParts(msgs)
	require.Equal(t, []chattool.SkillMeta{{
		Name: "repo-helper-new",
		Dir:  "/skills/repo-helper-new",
	}}, got)
}

func TestMergeSkillMetas(t *testing.T) {
	t.Parallel()

	persisted := []chattool.SkillMeta{{
		Name:        "repo-helper",
		Description: "Persisted skill",
		Dir:         "/skills/repo-helper-old",
	}}
	discovered := []chattool.SkillMeta{
		{
			Name:        "repo-helper",
			Description: "Discovered replacement",
			Dir:         "/skills/repo-helper-new",
			MetaFile:    "SKILL.md",
		},
		{
			Name:        "deep-review",
			Description: "Discovered skill",
			Dir:         "/skills/deep-review",
		},
	}

	got := mergeSkillMetas(persisted, discovered)
	require.Equal(t, []chattool.SkillMeta{
		discovered[0],
		discovered[1],
	}, got)
}

func TestSelectSkillMetasForInstructionRefresh(t *testing.T) {
	t.Parallel()

	persisted := []chattool.SkillMeta{{Name: "persisted", Dir: "/skills/persisted"}}
	discovered := []chattool.SkillMeta{{Name: "discovered", Dir: "/skills/discovered"}}
	currentAgentID := uuid.New()
	otherAgentID := uuid.New()

	t.Run("MergesCurrentAgentSkills", func(t *testing.T) {
		t.Parallel()
		got := selectSkillMetasForInstructionRefresh(
			persisted,
			discovered,
			uuid.NullUUID{UUID: currentAgentID, Valid: true},
			uuid.NullUUID{UUID: currentAgentID, Valid: true},
		)
		require.Equal(t, []chattool.SkillMeta{discovered[0], persisted[0]}, got)
	})

	t.Run("DropsStalePersistedSkillsWhenAgentChanged", func(t *testing.T) {
		t.Parallel()
		got := selectSkillMetasForInstructionRefresh(
			persisted,
			discovered,
			uuid.NullUUID{UUID: currentAgentID, Valid: true},
			uuid.NullUUID{UUID: otherAgentID, Valid: true},
		)
		require.Equal(t, discovered, got)
	})

	t.Run("PreservesPersistedSkillsWhenAgentLookupFails", func(t *testing.T) {
		t.Parallel()
		got := selectSkillMetasForInstructionRefresh(
			persisted,
			nil,
			uuid.NullUUID{},
			uuid.NullUUID{UUID: otherAgentID, Valid: true},
		)
		require.Equal(t, persisted, got)
	})
}

func TestResolveChainModeIgnoresSkillOnlySentinelMessages(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	assistant := database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
	skillOnly := chatMessageWithParts([]codersdk.ChatMessagePart{
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: AgentChatContextSentinelPath,
			ContextFileAgentID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		},
		{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "repo-helper",
			SkillDir:  "/skills/repo-helper",
		},
	})
	skillOnly.Role = database.ChatMessageRoleUser
	user := chatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "latest user message",
	}})
	user.Role = database.ChatMessageRoleUser

	got := resolveChainMode([]database.ChatMessage{assistant, skillOnly, user})
	require.Equal(t, "resp-123", got.previousResponseID)
	require.Equal(t, modelConfigID, got.modelConfigID)
	require.Equal(t, 2, got.trailingUserCount)
	require.Equal(t, 1, got.contributingTrailingUserCount)
}

func TestFilterPromptForChainModeKeepsContributingUsersAcrossSkippedSentinelTurns(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	priorUser := chatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "prior user message",
	}})
	priorUser.Role = database.ChatMessageRoleUser
	assistant := database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
	firstTrailingUser := chatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "first trailing user",
	}})
	firstTrailingUser.Role = database.ChatMessageRoleUser
	skillOnly := chatMessageWithParts([]codersdk.ChatMessagePart{
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: AgentChatContextSentinelPath,
			ContextFileAgentID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		},
		{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "repo-helper",
			SkillDir:  "/skills/repo-helper",
		},
	})
	skillOnly.Role = database.ChatMessageRoleUser
	lastTrailingUser := chatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "last trailing user",
	}})
	lastTrailingUser.Role = database.ChatMessageRoleUser

	chainInfo := resolveChainMode([]database.ChatMessage{
		priorUser,
		assistant,
		firstTrailingUser,
		skillOnly,
		lastTrailingUser,
	})
	require.Equal(t, 3, chainInfo.trailingUserCount)
	require.Equal(t, 2, chainInfo.contributingTrailingUserCount)

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "system instruction"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "prior user message"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "assistant reply"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "first trailing user"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "last trailing user"},
			},
		},
	}

	got := filterPromptForChainMode(prompt, chainInfo)
	require.Len(t, got, 3)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[2].Role)

	firstPart, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "first trailing user", firstPart.Text)
	lastPart, ok := fantasy.AsMessagePart[fantasy.TextPart](got[2].Content[0])
	require.True(t, ok)
	require.Equal(t, "last trailing user", lastPart.Text)
}

func TestFilterPromptForChainModeUsesContributingTrailingUsers(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	priorUser := chatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "prior user message",
	}})
	priorUser.Role = database.ChatMessageRoleUser
	assistant := database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
	skillOnly := chatMessageWithParts([]codersdk.ChatMessagePart{
		{
			Type:            codersdk.ChatMessagePartTypeContextFile,
			ContextFilePath: AgentChatContextSentinelPath,
			ContextFileAgentID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		},
		{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "repo-helper",
			SkillDir:  "/skills/repo-helper",
		},
	})
	skillOnly.Role = database.ChatMessageRoleUser
	latestUser := chatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "latest user message",
	}})
	latestUser.Role = database.ChatMessageRoleUser

	chainInfo := resolveChainMode([]database.ChatMessage{
		priorUser,
		assistant,
		skillOnly,
		latestUser,
	})
	require.Equal(t, 2, chainInfo.trailingUserCount)
	require.Equal(t, 1, chainInfo.contributingTrailingUserCount)

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "system instruction"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "prior user message"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "assistant reply"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "latest user message"},
			},
		},
	}

	got := filterPromptForChainMode(prompt, chainInfo)
	require.Len(t, got, 2)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[1].Role)

	part, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "latest user message", part.Text)
}

func chatMessageWithParts(parts []codersdk.ChatMessagePart) database.ChatMessage {
	raw, _ := json.Marshal(parts)
	return database.ChatMessage{
		Content: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
	}
}

func TestShouldInterruptActiveRunFromControlMessage(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	entry := &heartbeatEntry{chatID: chatID, runGeneration: 7}

	tests := []struct {
		name  string
		entry *heartbeatEntry
		msg   coderdpubsub.ChatControlMessage
		want  bool
	}{
		{
			name:  "newer generation restarts",
			entry: entry,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        chatID,
				RunGeneration: 8,
				Reason:        coderdpubsub.ChatControlReasonRestart,
			},
			want: true,
		},
		{
			name:  "equal generation interrupt cancels",
			entry: entry,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        chatID,
				RunGeneration: 7,
				Reason:        coderdpubsub.ChatControlReasonInterrupt,
			},
			want: true,
		},
		{
			name:  "equal generation archive cancels",
			entry: entry,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        chatID,
				RunGeneration: 7,
				Reason:        coderdpubsub.ChatControlReasonArchive,
			},
			want: true,
		},
		{
			name:  "equal generation restart is ignored",
			entry: entry,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        chatID,
				RunGeneration: 7,
				Reason:        coderdpubsub.ChatControlReasonRestart,
			},
			want: false,
		},
		{
			name:  "older generation interrupt is ignored",
			entry: entry,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        chatID,
				RunGeneration: 6,
				Reason:        coderdpubsub.ChatControlReasonInterrupt,
			},
			want: false,
		},
		{
			name:  "different chat is ignored",
			entry: entry,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        uuid.New(),
				RunGeneration: 8,
				Reason:        coderdpubsub.ChatControlReasonRestart,
			},
			want: false,
		},
		{
			name:  "missing entry is ignored",
			entry: nil,
			msg: coderdpubsub.ChatControlMessage{
				ChatID:        chatID,
				RunGeneration: 8,
				Reason:        coderdpubsub.ChatControlReasonRestart,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, shouldInterruptActiveRunFromControlMessage(tt.entry, tt.msg))
		})
	}
}

func TestSubscribeWorkerControl_CancelsRegisteredRun(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ps := dbpubsub.NewInMemory()

	chatID := uuid.New()
	workerID := uuid.New()
	runGeneration := int64(7)

	server := &Server{
		logger:            logger,
		pubsub:            ps,
		workerID:          workerID,
		heartbeatRegistry: make(map[uuid.UUID]*heartbeatEntry),
	}

	chatCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	entry := &heartbeatEntry{
		cancelWithCause: cancel,
		chatID:          chatID,
		runGeneration:   runGeneration,
		logger:          logger,
	}
	server.registerHeartbeat(entry)
	defer server.unregisterHeartbeat(entry)

	controlCancel := server.subscribeWorkerControl(ctx)
	require.NotNil(t, controlCancel)
	defer controlCancel()

	payload, err := json.Marshal(coderdpubsub.ChatControlMessage{
		ChatID:        chatID,
		RunGeneration: runGeneration + 1,
		Reason:        coderdpubsub.ChatControlReasonRestart,
	})
	require.NoError(t, err)
	require.NoError(t, ps.Publish(coderdpubsub.ChatControlChannel(workerID), payload))

	require.Eventually(t, func() bool {
		return errors.Is(context.Cause(chatCtx), chatloop.ErrInterrupted)
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestRegisterHeartbeat_ReplacesOlderGenerationAndIgnoresStaleUnregister(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chatID := uuid.New()

	server := &Server{
		logger:            logger,
		heartbeatRegistry: make(map[uuid.UUID]*heartbeatEntry),
	}

	oldCtx, oldCancel := context.WithCancelCause(ctx)
	defer oldCancel(nil)
	oldEntry := &heartbeatEntry{
		cancelWithCause: oldCancel,
		chatID:          chatID,
		runGeneration:   1,
		logger:          logger,
	}
	server.registerHeartbeat(oldEntry)

	newCtx, newCancel := context.WithCancelCause(ctx)
	defer newCancel(nil)
	newEntry := &heartbeatEntry{
		cancelWithCause: newCancel,
		chatID:          chatID,
		runGeneration:   2,
		logger:          logger,
	}
	server.registerHeartbeat(newEntry)

	require.ErrorIs(t, context.Cause(oldCtx), chatloop.ErrInterrupted)
	require.NoError(t, context.Cause(newCtx))

	server.heartbeatMu.Lock()
	require.Same(t, newEntry, server.heartbeatRegistry[chatID])
	server.heartbeatMu.Unlock()

	server.unregisterHeartbeat(oldEntry)

	server.heartbeatMu.Lock()
	require.Same(t, newEntry, server.heartbeatRegistry[chatID])
	server.heartbeatMu.Unlock()

	server.unregisterHeartbeat(newEntry)

	server.heartbeatMu.Lock()
	_, exists := server.heartbeatRegistry[chatID]
	server.heartbeatMu.Unlock()
	require.False(t, exists)
}

// TestProcessChat_IgnoresStaleStatusNotification verifies that
// processChat is not interrupted by a stale "pending" status
// fanout delivered after the worker has already published
// "running". Worker control now lives on a separate per-worker
// channel, so delayed status fanout is observational only.
func TestProcessChat_IgnoresStaleStatusNotification(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ps := chattest.NewDelayedStatusPubsub(dbpubsub.NewInMemory())
	clock := quartz.NewMock(t)

	chatID := uuid.New()
	workerID := uuid.New()
	runGeneration := int64(7)
	chatChannel := coderdpubsub.ChatStreamNotifyChannel(chatID)
	ps.DelayStatus(chatChannel, string(database.ChatStatusPending))

	server := &Server{
		db:                    db,
		logger:                logger,
		pubsub:                ps,
		clock:                 clock,
		workerID:              workerID,
		chatHeartbeatInterval: time.Minute,
		configCache:           newChatConfigCache(ctx, db, clock),
		heartbeatRegistry:     make(map[uuid.UUID]*heartbeatEntry),
	}

	staleNotify, err := json.Marshal(coderdpubsub.ChatStreamNotifyMessage{
		Status: string(database.ChatStatusPending),
	})
	require.NoError(t, err)
	require.NoError(t, ps.Publish(chatChannel, staleNotify))

	var finalStatus database.ChatStatus
	cleanupDone := make(chan struct{})
	allowModelResolution := make(chan struct{})

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(
		database.Chat{
			ID:            chatID,
			Status:        database.ChatStatusRunning,
			WorkerID:      uuid.NullUUID{UUID: workerID, Valid: true},
			RunGeneration: runGeneration,
		}, nil,
	)
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(db)
		},
	)
	db.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(
		database.Chat{
			ID:            chatID,
			Status:        database.ChatStatusRunning,
			WorkerID:      uuid.NullUUID{UUID: workerID, Valid: true},
			RunGeneration: runGeneration,
		}, nil,
	)
	db.EXPECT().UpdateChatStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.UpdateChatStatusParams) (database.Chat, error) {
			finalStatus = params.Status
			close(cleanupDone)
			return database.Chat{ID: chatID, Status: params.Status, RunGeneration: runGeneration}, nil
		},
	)
	// resolveChatModel fails after the stale notification has been
	// released. That ensures the test exercises delayed status fanout
	// while the chat is already running.
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			<-allowModelResolution
			return database.ChatModelConfig{}, xerrors.New("no model configured")
		},
	).AnyTimes()
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(
		database.ChatUsageLimitConfig{}, sql.ErrNoRows,
	).AnyTimes()
	db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chatID).Return(nil, nil).AnyTimes()

	chat := database.Chat{ID: chatID, LastModelConfigID: uuid.New(), RunGeneration: runGeneration}
	go server.processChat(ctx, chat)

	require.NoError(t, ps.WaitForStatusPublish(ctx, chatChannel, string(database.ChatStatusRunning)))
	require.NoError(t, ps.ReleaseStatus(chatChannel, string(database.ChatStatusPending)))
	close(allowModelResolution)

	select {
	case <-cleanupDone:
	case <-ctx.Done():
		t.Fatal("processChat did not complete")
	}

	require.Equal(t, database.ChatStatusError, finalStatus,
		"processChat should have reached runChat (error), not been interrupted (waiting)")
}

// TestHeartbeatTick_StolenChatIsInterrupted verifies that when the
// batch heartbeat UPDATE does not return a registered chat's ID
// (because another replica stole it or it was completed), the
// heartbeat tick cancels that chat's context with ErrInterrupted
// while leaving surviving chats untouched.
func TestHeartbeatTick_StolenChatIsInterrupted(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)

	workerID := uuid.New()

	server := &Server{
		db:                    db,
		logger:                logger,
		clock:                 clock,
		workerID:              workerID,
		chatHeartbeatInterval: time.Minute,
		heartbeatRegistry:     make(map[uuid.UUID]*heartbeatEntry),
	}

	// Create three chats with independent cancel functions.
	chat1 := uuid.New()
	chat2 := uuid.New()
	chat3 := uuid.New()

	_, cancel1 := context.WithCancelCause(ctx)
	_, cancel2 := context.WithCancelCause(ctx)
	ctx3, cancel3 := context.WithCancelCause(ctx)

	server.registerHeartbeat(&heartbeatEntry{
		cancelWithCause: cancel1,
		chatID:          chat1,
		runGeneration:   1,
		logger:          logger,
	})
	server.registerHeartbeat(&heartbeatEntry{
		cancelWithCause: cancel2,
		chatID:          chat2,
		runGeneration:   2,
		logger:          logger,
	})
	server.registerHeartbeat(&heartbeatEntry{
		cancelWithCause: cancel3,
		chatID:          chat3,
		runGeneration:   3,
		logger:          logger,
	})

	// The batch UPDATE returns only chat1 and chat2 —
	// chat3 was "stolen" by another replica.
	db.EXPECT().UpdateChatHeartbeats(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.UpdateChatHeartbeatsParams) ([]uuid.UUID, error) {
			require.Equal(t, workerID, params.WorkerID)
			require.Len(t, params.IDs, 3)
			require.Len(t, params.RunGenerations, 3)
			got := make(map[uuid.UUID]int64, len(params.IDs))
			for i, id := range params.IDs {
				got[id] = params.RunGenerations[i]
			}
			require.Equal(t, map[uuid.UUID]int64{chat1: 1, chat2: 2, chat3: 3}, got)
			// Return only chat1 and chat2 as surviving.
			return []uuid.UUID{chat1, chat2}, nil
		},
	)

	server.heartbeatTick(ctx)

	// chat3's context should be canceled with ErrInterrupted.
	require.ErrorIs(t, context.Cause(ctx3), chatloop.ErrInterrupted,
		"stolen chat should be interrupted")

	// chat3 should have been removed from the registry by
	// unregister (in production this happens via defer in
	// processChat). The heartbeat tick itself does not
	// unregister — it only cancels. Verify the entry is
	// still present (processChat's defer would clean it up).
	server.heartbeatMu.Lock()
	_, chat1Exists := server.heartbeatRegistry[chat1]
	_, chat2Exists := server.heartbeatRegistry[chat2]
	_, chat3Exists := server.heartbeatRegistry[chat3]
	server.heartbeatMu.Unlock()

	require.True(t, chat1Exists, "surviving chat1 should remain registered")
	require.True(t, chat2Exists, "surviving chat2 should remain registered")
	require.True(t, chat3Exists,
		"stolen chat3 should still be in registry (processChat defer removes it)")
}

// TestHeartbeatTick_DBErrorDoesNotInterruptChats verifies that a
// transient database failure causes the tick to log and return
// without canceling any registered chats.
func TestHeartbeatTick_DBErrorDoesNotInterruptChats(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)

	server := &Server{
		db:                    db,
		logger:                logger,
		clock:                 clock,
		workerID:              uuid.New(),
		chatHeartbeatInterval: time.Minute,
		heartbeatRegistry:     make(map[uuid.UUID]*heartbeatEntry),
	}

	chatID := uuid.New()
	chatCtx, cancel := context.WithCancelCause(ctx)

	server.registerHeartbeat(&heartbeatEntry{
		cancelWithCause: cancel,
		chatID:          chatID,
		runGeneration:   11,
		logger:          logger,
	})

	// Simulate a transient DB error.
	db.EXPECT().UpdateChatHeartbeats(gomock.Any(), gomock.Any()).Return(
		nil, xerrors.New("connection reset"),
	)

	server.heartbeatTick(ctx)

	// Chat should NOT be interrupted — the tick logged and
	// returned early.
	require.NoError(t, chatCtx.Err(),
		"chat context should not be canceled on transient DB error")
}
