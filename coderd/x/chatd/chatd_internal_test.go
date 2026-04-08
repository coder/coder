package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

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
		payload coderdpubsub.ChatEvent
		err     error
	}, 1)
	cancelSub, err := pubsub.SubscribeWithErr(
		coderdpubsub.ChatEventChannel(ownerID),
		coderdpubsub.HandleChatEvent(func(_ context.Context, payload coderdpubsub.ChatEvent, err error) {
			messageEvents <- struct {
				payload coderdpubsub.ChatEvent
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
		Provider: "openai",
		APIKey:   "test-key",
		BaseUrl:  serverURL,
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
		require.Equal(t, coderdpubsub.ChatEventKindTitleChange, event.payload.Kind)
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
		payload coderdpubsub.ChatEvent
		err     error
	}, 1)
	cancelSub, err := pubsub.SubscribeWithErr(
		coderdpubsub.ChatEventChannel(ownerID),
		coderdpubsub.HandleChatEvent(func(_ context.Context, payload coderdpubsub.ChatEvent, err error) {
			messageEvents <- struct {
				payload coderdpubsub.ChatEvent
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
		Provider: "openai",
		APIKey:   "test-key",
		BaseUrl:  serverURL,
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
		require.Equal(t, coderdpubsub.ChatEventKindTitleChange, event.payload.Kind)
		require.Equal(t, chatID, event.payload.Chat.ID)
		require.Equal(t, wantTitle, event.payload.Chat.Title)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for title change pubsub event")
	}
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
		InstructionsDirs: []string{"/home/coder/.coder"},
		InstructionsFile: "AGENTS.md",
		SkillsDirs:       []string{"/home/coder/project/.agents/skills"},
		SkillMetaFile:    "SKILL.md",
	}, nil).AnyTimes()
	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
		workspacesdk.LSResponse{},
		codersdk.NewTestError(404, "POST", "/api/v0/list-directory"),
	).AnyTimes()
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/coder/project/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1)).Return(
		io.NopCloser(strings.NewReader("# Project instructions")),
		"",
		nil,
	).Times(1)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:     db,
		logger: logger,
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

	instruction, _, _, err := server.persistInstructionFiles(
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

func TestPersistInstructionFilesFallbackOnOlderAgent(t *testing.T) {
	t.Parallel()

	// When the agent doesn't support the context-config endpoint
	// (returns an error), the fallback path should:
	// 1. Read instruction files from ~/.coder using LSRelativityHome
	// 2. Discover skills from the working directory
	// 3. Return the default skill meta file name

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
	db.EXPECT().UpdateChatLastInjectedContext(gomock.Any(), gomock.Any()).Return(database.Chat{}, nil).AnyTimes()

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	// ContextConfig returns error — simulating an older agent.
	conn.EXPECT().ContextConfig(gomock.Any()).Return(
		workspacesdk.ContextConfigResponse{},
		codersdk.NewTestError(404, "GET", "/api/v0/context-config"),
	).Times(1)

	// Fallback: readHomeInstructionFile uses LSRelativityHome
	// to read from ~/.coder directory.
	conn.EXPECT().LS(gomock.Any(), "",
		gomock.Cond(func(x any) bool {
			req, ok := x.(workspacesdk.LSRequest)
			return ok && req.Relativity == workspacesdk.LSRelativityHome &&
				len(req.Path) == 1 && req.Path[0] == ".coder"
		}),
	).Return(workspacesdk.LSResponse{
		Contents: []workspacesdk.LSFile{{
			Name:               "AGENTS.md",
			AbsolutePathString: "/home/user/.coder/AGENTS.md",
			IsDir:              false,
		}},
	}, nil).Times(1)

	// ReadFile for the home instruction file.
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/user/.coder/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		io.NopCloser(strings.NewReader("# Home instructions")),
		"",
		nil,
	).Times(1)

	// Working directory instruction file: 404.
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/coder/project/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		nil, "",
		codersdk.NewTestError(404, "GET", "/api/v0/read-file"),
	).Times(1)

	// Skills directory: fallback constructs path from working dir.
	conn.EXPECT().LS(gomock.Any(), "",
		gomock.Cond(func(x any) bool {
			req, ok := x.(workspacesdk.LSRequest)
			return ok && req.Relativity == workspacesdk.LSRelativityRoot &&
				len(req.Path) == 1 && req.Path[0] == "/home/coder/project/.agents/skills"
		}),
	).Return(workspacesdk.LSResponse{
		Contents: []workspacesdk.LSFile{{
			Name:               "fallback-skill",
			AbsolutePathString: "/home/coder/project/.agents/skills/fallback-skill",
			IsDir:              true,
		}},
	}, nil).Times(1)

	skillContent := "---\nname: fallback-skill\ndescription: Discovered via fallback\n---\nBody"
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/coder/project/.agents/skills/fallback-skill/SKILL.md",
		int64(0),
		int64(64*1024+1),
	).Return(
		io.NopCloser(strings.NewReader(skillContent)),
		"",
		nil,
	).Times(1)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:     db,
		logger: logger,
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

	instruction, skills, skillMeta, err := server.persistInstructionFiles(
		ctx,
		chat,
		uuid.New(),
		workspaceCtx.getWorkspaceAgent,
		workspaceCtx.getWorkspaceConn,
	)
	require.NoError(t, err)
	// Instruction should contain the home instruction file content.
	require.Contains(t, instruction, "Home instructions")
	// OS and directory metadata should be present.
	require.Contains(t, instruction, "Operating System: linux")
	require.Contains(t, instruction, "Working Directory: /home/coder/project")
	// Skills should be discovered from the working directory.
	require.Len(t, skills, 1)
	require.Equal(t, "fallback-skill", skills[0].Name)
	// Skill meta file should be the default.
	require.Equal(t, workspacesdk.DefaultSkillMetaFile, skillMeta)
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

	instruction, _, _, err := server.persistInstructionFiles(
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
		InstructionsDirs: []string{"/home/coder/.coder"},
		InstructionsFile: "AGENTS.md",
		SkillsDirs:       []string{"/home/coder/project/.agents/skills"},
		SkillMetaFile:    "SKILL.md",
	}, nil).AnyTimes()

	// Instruction dir LS (.coder directory): return 404 so no
	// instruction file is found from the configured dir.
	conn.EXPECT().LS(gomock.Any(), "",
		gomock.Cond(func(x any) bool {
			req, ok := x.(workspacesdk.LSRequest)
			return ok && req.Relativity == workspacesdk.LSRelativityRoot &&
				len(req.Path) == 1 && req.Path[0] == "/home/coder/.coder"
		}),
	).Return(
		workspacesdk.LSResponse{},
		codersdk.NewTestError(404, "POST", "/api/v0/list-directory"),
	).Times(1)

	// Pwd AGENTS.md: return 404 so no working-directory
	// instruction file is found either.
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/coder/project/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		nil, "",
		codersdk.NewTestError(404, "GET", "/api/v0/read-file"),
	).Times(1)

	// Skills LS (.agents/skills directory): return one skill
	// directory so DiscoverSkills finds it.
	conn.EXPECT().LS(gomock.Any(), "",
		gomock.Cond(func(x any) bool {
			req, ok := x.(workspacesdk.LSRequest)
			return ok && req.Relativity == workspacesdk.LSRelativityRoot &&
				len(req.Path) == 1 && req.Path[0] == "/home/coder/project/.agents/skills"
		}),
	).Return(workspacesdk.LSResponse{
		Contents: []workspacesdk.LSFile{{
			Name:               "my-skill",
			AbsolutePathString: "/home/coder/project/.agents/skills/my-skill",
			IsDir:              true,
		}},
	}, nil).Times(1)

	// Skills SKILL.md ReadFile: return valid frontmatter.
	skillContent := "---\nname: my-skill\ndescription: A test skill\n---\nSkill body"
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/coder/project/.agents/skills/my-skill/SKILL.md",
		int64(0),
		int64(64*1024+1),
	).Return(
		io.NopCloser(strings.NewReader(skillContent)),
		"",
		nil,
	).Times(1)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:     db,
		logger: logger,
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

	instruction, skills, _, err := server.persistInstructionFiles(
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
		InstructionsDirs: []string{"/home/coder/.coder"},
		InstructionsFile: "AGENTS.md",
		SkillsDirs:       []string{"/home/coder/project/.agents/skills"},
		SkillMetaFile:    "SKILL.md",
	}, nil).AnyTimes()

	// All LS calls return 404: no .coder directory and no
	// .agents/skills directory.
	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
		workspacesdk.LSResponse{},
		codersdk.NewTestError(404, "POST", "/api/v0/list-directory"),
	).AnyTimes()

	// Pwd AGENTS.md: return 404.
	conn.EXPECT().ReadFile(gomock.Any(),
		"/home/coder/project/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		nil, "",
		codersdk.NewTestError(404, "GET", "/api/v0/read-file"),
	).Times(1)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:     db,
		logger: logger,
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

	instruction, skills, _, err := server.persistInstructionFiles(
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

func chatMessageWithParts(parts []codersdk.ChatMessagePart) database.ChatMessage {
	raw, _ := json.Marshal(parts)
	return database.ChatMessage{
		Content: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
	}
}

// TestProcessChat_IgnoresStaleControlNotification verifies that
// processChat is not interrupted by a "pending" notification
// published before processing begins. This is the race that caused
// TestOpenAIReasoningWithWebSearchRoundTripStoreFalse to flake:
// SendMessage publishes "pending" via PostgreSQL NOTIFY, and due
// to async delivery the notification can arrive at the control
// subscriber after it registers but before the processor publishes
// "running".
func TestProcessChat_IgnoresStaleControlNotification(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ps := dbpubsub.NewInMemory()
	clock := quartz.NewMock(t)

	chatID := uuid.New()
	workerID := uuid.New()

	server := &Server{
		db:                    db,
		logger:                logger,
		pubsub:                ps,
		clock:                 clock,
		workerID:              workerID,
		chatHeartbeatInterval: time.Minute,
		configCache:           newChatConfigCache(ctx, db, clock),
	}

	// Publish a stale "pending" notification on the control channel
	// BEFORE processChat subscribes. In production this is the
	// notification from SendMessage that triggered the processing.
	staleNotify, err := json.Marshal(coderdpubsub.ChatStreamNotifyMessage{
		Status: string(database.ChatStatusPending),
	})
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chatID), staleNotify)
	require.NoError(t, err)

	// Track which status processChat writes during cleanup.
	var finalStatus database.ChatStatus
	cleanupDone := make(chan struct{})

	// The deferred cleanup in processChat runs a transaction.
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(db)
		},
	)
	db.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(
		database.Chat{ID: chatID, Status: database.ChatStatusRunning, WorkerID: uuid.NullUUID{UUID: workerID, Valid: true}}, nil,
	)
	db.EXPECT().UpdateChatStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.UpdateChatStatusParams) (database.Chat, error) {
			finalStatus = params.Status
			close(cleanupDone)
			return database.Chat{ID: chatID, Status: params.Status}, nil
		},
	)

	// resolveChatModel fails immediately — that's fine, we only
	// need processChat to get past initialization without being
	// interrupted by the stale notification.
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), gomock.Any()).Return(
		database.ChatModelConfig{}, xerrors.New("no model configured"),
	).AnyTimes()
	db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(
		database.ChatUsageLimitConfig{}, sql.ErrNoRows,
	).AnyTimes()
	db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chatID).Return(nil, nil).AnyTimes()

	chat := database.Chat{ID: chatID, LastModelConfigID: uuid.New()}
	go server.processChat(ctx, chat)

	select {
	case <-cleanupDone:
	case <-ctx.Done():
		t.Fatal("processChat did not complete")
	}

	// If the stale notification interrupted us, status would be
	// "waiting" (the ErrInterrupted path). Since the gate blocked
	// it, processChat reached runChat, which failed on model
	// resolution → status is "error".
	require.Equal(t, database.ChatStatusError, finalStatus,
		"processChat should have reached runChat (error), not been interrupted (waiting)")
}

// drainStreamStatusEvents reads status events from the merged events
// channel until no more arrive. Uses the same timeout pattern as
// requireNoStreamEvent. The subscriber goroutine processes events
// at CPU speed with NewInMemory pubsub, so a short real-time window
// is sufficient.
func drainStreamStatusEvents(
	t *testing.T,
	events <-chan codersdk.ChatStreamEvent,
) []codersdk.ChatStatus {
	t.Helper()

	var statuses []codersdk.ChatStatus
	for {
		select {
		case event, ok := <-events:
			require.True(t, ok, "events channel closed unexpectedly")
			if event.Type == codersdk.ChatStreamEventTypeStatus {
				statuses = append(statuses, event.Status.Status)
			}
		case <-time.After(200 * time.Millisecond):
			return statuses
		}
	}
}

// TestSubscribeSuppressesDuplicatePubsubStatus verifies that when
// publishStatus delivers a status via both the local stream and
// pubsub, the subscriber goroutine forwards it exactly once.
func TestSubscribeSuppressesDuplicatePubsubStatus(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusWaiting}
	initialMessage := database.ChatMessage{
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
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	// Production code path: publishStatus publishes to both
	// local stream and pubsub in one call.
	server.publishStatus(chatID, database.ChatStatusPending, uuid.NullUUID{})

	// Drain all status events. Should be exactly 1 (from local).
	// Before the counter-map dedup, this returned 2
	// (the pubsub duplicate leaked through).
	statuses := drainStreamStatusEvents(t, events)
	require.Equal(t, []codersdk.ChatStatus{codersdk.ChatStatusPending}, statuses,
		"expected exactly 1 pending status, got %d: %v", len(statuses), statuses)
}

// TestSubscribeSuppressesDuplicateAutoPromote verifies that the
// counter-map dedup handles repeated status values correctly. The
// auto-promote path publishes pending twice (SendMessage + cleanup)
// without an intervening waiting. A plain boolean set would treat
// the second pending as a no-op and leak one pubsub duplicate.
// The counter map tracks both and suppresses both.
func TestSubscribeSuppressesDuplicateAutoPromote(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusWaiting}
	initialMessage := database.ChatMessage{
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
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	// Simulate: SendMessage -> pending, processChat -> running,
	// processChat cleanup auto-promote -> pending.
	server.publishStatus(chatID, database.ChatStatusPending, uuid.NullUUID{})
	server.publishStatus(chatID, database.ChatStatusRunning, uuid.NullUUID{})
	server.publishStatus(chatID, database.ChatStatusPending, uuid.NullUUID{})

	// Expect exactly 3 status events (one per publishStatus call),
	// not 6 (which would mean all pubsub duplicates leaked).
	statuses := drainStreamStatusEvents(t, events)
	require.Equal(t,
		[]codersdk.ChatStatus{
			codersdk.ChatStatusPending,
			codersdk.ChatStatusRunning,
			codersdk.ChatStatusPending,
		},
		statuses,
		"expected exactly 3 statuses (pending, running, pending), got %d: %v",
		len(statuses), statuses,
	)
}

// TestSubscribeForwardsRemoteReplicaStatus verifies that a status
// published only via pubsub (simulating a remote replica) is
// forwarded to the subscriber. The counter-map should not suppress
// it because no local counterpart was delivered.
func TestSubscribeForwardsRemoteReplicaStatus(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusWaiting}
	initialMessage := database.ChatMessage{
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
	)

	server := newSubscribeTestServer(t, db)
	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	// Publish directly to pubsub, bypassing the local stream.
	// This simulates a status change from a remote replica where
	// processChat runs on a different server.
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		Status: string(database.ChatStatusRunning),
	})

	statuses := drainStreamStatusEvents(t, events)
	require.Equal(t,
		[]codersdk.ChatStatus{codersdk.ChatStatusRunning},
		statuses,
		"remote-replica status should be forwarded (counter == 0), got %d: %v",
		len(statuses), statuses,
	)
}

// TestSubscribeDedupPreventsStalePendingWipe reproduces the
// full Race 1 production scenario: SendMessage publishes pending,
// processChat publishes running and streams message_parts, and a
// stale pubsub pending arrives after parts have been delivered.
//
// The harmful interleaving depends on Go's select scheduling and
// may not trigger on every run. When it does trigger, it proves
// that a consumer tracking status+stream state (like the frontend)
// would have its in-progress stream wiped by the stale pending.
//
// After the fix (counter-map dedup), the stale pending is never
// delivered, so the harmful pattern is impossible.
func TestSubscribeDedupPreventsStalePendingWipe(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusWaiting}
	initialMessage := database.ChatMessage{
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
	)

	server := newSubscribeTestServer(t, db)

	// Start buffering so publishMessagePart events are delivered
	// to subscribers (processChat does this at chatd.go:3548-3553).
	streamState := server.getOrCreateStreamState(chatID)
	streamState.mu.Lock()
	streamState.buffering = true
	streamState.mu.Unlock()
	defer func() {
		streamState.mu.Lock()
		streamState.buffering = false
		server.cleanupStreamIfIdle(chatID, streamState)
		streamState.mu.Unlock()
	}()

	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	// Full production scenario:
	// 1. SendMessage publishes pending status.
	server.publishStatus(chatID, database.ChatStatusPending, uuid.NullUUID{})
	// 2. processChat publishes running status.
	server.publishStatus(chatID, database.ChatStatusRunning, uuid.NullUUID{
		UUID:  uuid.New(),
		Valid: true,
	})
	// 3. LLM streams message parts.
	for i := range 5 {
		server.publishMessagePart(chatID, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{
			Type: "text",
			Text: strings.Repeat("word ", i+1),
		})
	}

	// Consume events as the frontend would: track status and
	// whether stream content has been received.
	var (
		currentStatus        codersdk.ChatStatus
		hasStreamContent     bool
		streamWipedByPending bool
	)

	allEvents := drainAllEvents(t, events)
	for _, event := range allEvents {
		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			nextStatus := event.Status.Status
			// Reproduce the frontend's destructive cleanup:
			// when pending/waiting arrives, stream is wiped.
			if nextStatus == codersdk.ChatStatusPending || nextStatus == codersdk.ChatStatusWaiting {
				if currentStatus == codersdk.ChatStatusRunning && hasStreamContent {
					// The harmful pattern: pending arrived while
					// stream was active. This would wipe the
					// frontend's in-progress stream.
					streamWipedByPending = true
				}
				hasStreamContent = false
			}
			currentStatus = nextStatus
		case codersdk.ChatStreamEventTypeMessagePart:
			hasStreamContent = true
		}
	}

	require.False(t, streamWipedByPending,
		"stale pubsub pending arrived after running + message parts, wiping the active stream")
}

// drainAllEvents reads all events from the channel until no more
// arrive within a short window. Returns them as a slice for
// iteration.
func drainAllEvents(
	t *testing.T,
	events <-chan codersdk.ChatStreamEvent,
) []codersdk.ChatStreamEvent {
	t.Helper()

	var result []codersdk.ChatStreamEvent
	for {
		select {
		case event, ok := <-events:
			require.True(t, ok, "events channel closed unexpectedly")
			result = append(result, event)
		case <-time.After(200 * time.Millisecond):
			return result
		}
	}
}
