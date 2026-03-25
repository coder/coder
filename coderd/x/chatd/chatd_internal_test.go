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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

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
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{
		ID:                uuid.New(),
		OperatingSystem:   "linux",
		Directory:         "/home/coder/project",
		ExpandedDirectory: "/home/coder/project",
	}

	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		gomock.Any(),
		workspaceID,
	).Return([]database.WorkspaceAgent{workspaceAgent}, nil).Times(1)
	db.EXPECT().InsertChatMessages(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
		workspacesdk.LSResponse{},
		codersdk.NewTestError(404, "POST", "/api/v0/list-directory"),
	).Times(1)
	conn.EXPECT().ReadFile(
		gomock.Any(),
		"/home/coder/project/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
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

	instruction, err := server.persistInstructionFiles(
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

func TestTurnWorkspaceContextGetWorkspaceConnRefreshesWorkspaceAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	initialAgent := database.WorkspaceAgent{ID: uuid.New()}
	refreshedAgent := database.WorkspaceAgent{ID: uuid.New()}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			gomock.Any(),
			workspaceID,
		).Return([]database.WorkspaceAgent{initialAgent}, nil),
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			gomock.Any(),
			workspaceID,
		).Return([]database.WorkspaceAgent{refreshedAgent}, nil),
	)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	var dialed []uuid.UUID
	server := &Server{db: db}
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialed = append(dialed, agentID)
		if agentID == initialAgent.ID {
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
	require.Equal(t, []uuid.UUID{initialAgent.ID, refreshedAgent.ID}, dialed)
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
		Error:      "OpenAI is rate limiting requests (HTTP 429). Please try again later.",
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
		Message:    "OpenAI is rate limiting requests (HTTP 429). Please try again later.",
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
