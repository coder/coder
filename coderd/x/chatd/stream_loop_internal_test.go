package chatd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStreamLoopSyncHintDecision(t *testing.T) {
	t.Parallel()

	workerA := uuid.New()
	workerB := uuid.New()
	loop := &streamLoop{
		state: streamLocalState{
			snapshotVersion:   5,
			historyVersion:    2,
			queueVersion:      3,
			retryVersion:      4,
			status:            database.ChatStatusRunning,
			workerID:          uuid.NullUUID{UUID: workerA, Valid: true},
			generationAttempt: 1,
		},
	}

	for _, tt := range []struct {
		name string
		hint streamSyncHint
		want bool
	}{
		{
			name: "stale snapshot ignored even with higher history",
			hint: streamSyncHint{snapshotVersion: 5, historyVersion: 3},
		},
		{
			name: "duplicate snapshot ignored",
			hint: streamSyncHint{snapshotVersion: 5},
		},
		{
			name: "new snapshot with no changed fields is ignored",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 2, queueVersion: 3, retryVersion: 4, status: database.ChatStatusRunning, workerID: uuid.NullUUID{UUID: workerA, Valid: true}, generationAttempt: 1},
		},
		{
			name: "new history fetches",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 3},
			want: true,
		},
		{
			name: "new queue fetches",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 2, queueVersion: 4},
			want: true,
		},
		{
			name: "new retry fetches",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 2, queueVersion: 3, retryVersion: 5},
			want: true,
		},
		{
			name: "new status fetches",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 2, queueVersion: 3, retryVersion: 4, status: database.ChatStatusWaiting, workerID: uuid.NullUUID{UUID: workerA, Valid: true}, generationAttempt: 1},
			want: true,
		},
		{
			name: "new worker fetches",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 2, queueVersion: 3, retryVersion: 4, status: database.ChatStatusRunning, workerID: uuid.NullUUID{UUID: workerB, Valid: true}, generationAttempt: 1},
			want: true,
		},
		{
			name: "new generation attempt fetches",
			hint: streamSyncHint{snapshotVersion: 6, historyVersion: 2, queueVersion: 3, retryVersion: 4, status: database.ChatStatusRunning, workerID: uuid.NullUUID{UUID: workerA, Valid: true}, generationAttempt: 2},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, loop.shouldFetch(tt.hint))
		})
	}
}

func TestStreamLoopMessageSyncAfterIDAndEdits(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 1)
	initial := streamDBSnapshot{
		chat: database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusRunning,
			SnapshotVersion: 1,
			HistoryVersion:  1,
		},
		changedMessages: []database.ChatMessage{
			streamMessage(t, chatID, 1, 1, database.ChatMessageRoleUser, "already seen", false),
			streamMessage(t, chatID, 2, 1, database.ChatMessageRoleAssistant, "new", false),
		},
	}

	events := loop.applyDBSnapshot(initial)
	requireEventTypes(t, events,
		codersdk.ChatStreamEventTypeMessage,
		codersdk.ChatStreamEventTypeStatus,
		codersdk.ChatStreamEventTypePreviewReset,
	)
	require.Equal(t, int64(2), events[0].Message.ID)

	edited := streamDBSnapshot{
		chat: database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusRunning,
			SnapshotVersion: 2,
			HistoryVersion:  2,
		},
		changedMessages: []database.ChatMessage{
			streamMessage(t, chatID, 1, 2, database.ChatMessageRoleUser, "edited", false),
		},
	}
	events = loop.applyDBSnapshot(edited)
	requireEventTypes(t, events,
		codersdk.ChatStreamEventTypeMessage,
		codersdk.ChatStreamEventTypePreviewReset,
	)
	require.Equal(t, int64(1), events[0].Message.ID)
}

func TestStreamLoopHistoryReset(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)
	loop.state.snapshotVersion = 1
	loop.state.historyVersion = 1
	loop.state.status = database.ChatStatusRunning
	loop.state.initialMessageSyncDone = true
	loop.state.knownMessages[1] = 1
	loop.state.knownMessages[2] = 1

	events := loop.applyDBSnapshot(streamDBSnapshot{
		chat: database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusRunning,
			SnapshotVersion: 2,
			HistoryVersion:  2,
		},
		changedMessages: []database.ChatMessage{
			streamMessage(t, chatID, 1, 2, database.ChatMessageRoleUser, "deleted", true),
		},
		historyReset: true,
		fullHistory: []database.ChatMessage{
			streamMessage(t, chatID, 3, 2, database.ChatMessageRoleUser, "replacement", false),
		},
	})

	requireEventTypes(t, events,
		codersdk.ChatStreamEventTypeHistoryReset,
		codersdk.ChatStreamEventTypeMessage,
		codersdk.ChatStreamEventTypePreviewReset,
	)
	require.Equal(t, int64(3), events[1].Message.ID)
	require.Equal(t, map[int64]int64{3: 2}, loop.state.knownMessages)
}

func TestStreamLoopQueueStatusRetryErrorActionRequiredAndPreviewReset(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	retry := codersdk.ChatStreamRetry{Attempt: 2, DelayMs: 100, Error: "retrying", RetryingAt: time.Now()}
	retryRaw, err := json.Marshal(retry)
	require.NoError(t, err)
	chatError := codersdk.ChatError{Message: "provider failed", Kind: codersdk.ChatErrorKindConfig}
	errorRaw, err := json.Marshal(chatError)
	require.NoError(t, err)

	loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)
	loop.state.snapshotVersion = 1
	loop.state.historyVersion = 1
	loop.state.queueVersion = 1
	loop.state.retryVersion = 1
	loop.state.generationAttempt = 1
	loop.state.status = database.ChatStatusRunning

	events := loop.applyDBSnapshot(streamDBSnapshot{
		chat: database.Chat{
			ID:                chatID,
			Status:            database.ChatStatusError,
			SnapshotVersion:   2,
			HistoryVersion:    2,
			QueueVersion:      2,
			RetryStateVersion: 2,
			GenerationAttempt: 2,
			LastError:         pqtype.NullRawMessage{RawMessage: errorRaw, Valid: true},
			RetryState:        pqtype.NullRawMessage{RawMessage: retryRaw, Valid: true},
		},
		queue: []database.ChatQueuedMessage{},
	})

	requireEventTypes(t, events,
		codersdk.ChatStreamEventTypeQueueUpdate,
		codersdk.ChatStreamEventTypeStatus,
		codersdk.ChatStreamEventTypeError,
		codersdk.ChatStreamEventTypeRetry,
		codersdk.ChatStreamEventTypePreviewReset,
	)
	require.Equal(t, chatError.Message, events[2].Error.Message)
	require.Equal(t, retry.Attempt, events[3].Retry.Attempt)

	actionLoop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)
	actionEvents := actionLoop.applyDBSnapshot(streamDBSnapshot{
		chat: database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusRequiresAction,
			SnapshotVersion: 1,
			HistoryVersion:  1,
		},
		actionRequired: &codersdk.ChatStreamActionRequired{ToolCalls: []codersdk.ChatStreamToolCall{{ToolCallID: "call-1", ToolName: "browser"}}},
	})
	requireEventTypes(t, actionEvents,
		codersdk.ChatStreamEventTypeStatus,
		codersdk.ChatStreamEventTypeActionRequired,
		codersdk.ChatStreamEventTypePreviewReset,
	)
	require.Equal(t, "call-1", actionEvents[1].ActionRequired.ToolCalls[0].ToolCallID)
}

func TestStreamLoopActionRequiredFromHistory(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	toolDefs, err := json.Marshal([]codersdk.DynamicTool{{Name: "browser"}})
	require.NoError(t, err)
	assistant := streamMessageParts(t, chatID, 1, 1, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "call-1",
		ToolName:   "browser",
		Args:       json.RawMessage(`{"url":"https://example.com"}`),
	}}, false)
	loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)
	action, err := loop.actionRequiredFromHistory(database.Chat{
		ID:           chatID,
		DynamicTools: pqtype.NullRawMessage{RawMessage: toolDefs, Valid: true},
	}, []database.ChatMessage{assistant})
	require.NoError(t, err)
	require.Len(t, action.ToolCalls, 1)
	require.Equal(t, "call-1", action.ToolCalls[0].ToolCallID)
	require.Equal(t, "browser", action.ToolCalls[0].ToolName)
}

func TestStreamLoopPartValidation(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), 0)
	loop.state.historyVersion = 7
	loop.state.generationAttempt = 3

	event, accepted, err := loop.part(StreamPart{HistoryVersion: 7, GenerationAttempt: 3, Seq: 1, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("a")})
	require.NoError(t, err)
	require.True(t, accepted)
	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, event.Type)
	require.Equal(t, int64(7), event.MessagePart.HistoryVersion)
	require.Equal(t, int64(3), event.MessagePart.GenerationAttempt)
	require.Equal(t, int64(1), event.MessagePart.Seq)

	_, accepted, err = loop.part(StreamPart{HistoryVersion: 6, GenerationAttempt: 3, Seq: 2, Part: codersdk.ChatMessageText("old history")})
	require.NoError(t, err)
	require.False(t, accepted)
	_, accepted, err = loop.part(StreamPart{HistoryVersion: 7, GenerationAttempt: 2, Seq: 2, Part: codersdk.ChatMessageText("old attempt")})
	require.NoError(t, err)
	require.False(t, accepted)
	_, accepted, err = loop.part(StreamPart{HistoryVersion: 7, GenerationAttempt: 3, Seq: 1, Part: codersdk.ChatMessageText("dup")})
	require.NoError(t, err)
	require.False(t, accepted)
	_, accepted, err = loop.part(StreamPart{HistoryVersion: 7, GenerationAttempt: 3, Seq: 3, Part: codersdk.ChatMessageText("gap")})
	require.Error(t, err)
	require.False(t, accepted)
}

func TestStreamLoopInitialSyncRecoversWithoutHint(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	loop := newStreamLoop(database.Chat{ID: chatID}, db, slogtest.Make(t, nil), 0)
	loop.state.snapshotVersion = 1
	loop.state.status = database.ChatStatusRunning

	db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(tx) },
	)
	tx.EXPECT().GetChatByIDForShare(gomock.Any(), chatID).Return(database.Chat{
		ID:              chatID,
		Status:          database.ChatStatusWaiting,
		SnapshotVersion: 2,
	}, nil)
	tx.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:              chatID,
		Status:          database.ChatStatusWaiting,
		SnapshotVersion: 2,
	}, nil)

	events, _, changed, err := loop.syncDB(ctx)
	require.NoError(t, err)
	require.True(t, changed)
	requireEventTypes(t, events, codersdk.ChatStreamEventTypeStatus)
	require.Equal(t, codersdk.ChatStatusWaiting, events[0].Status.Status)
}

func requireEventTypes(t *testing.T, events []codersdk.ChatStreamEvent, types ...codersdk.ChatStreamEventType) {
	t.Helper()
	require.Len(t, events, len(types))
	for i, typ := range types {
		require.Equal(t, typ, events[i].Type, "event %d", i)
	}
}

func streamMessage(t *testing.T, chatID uuid.UUID, id int64, revision int64, role database.ChatMessageRole, text string, deleted bool) database.ChatMessage {
	t.Helper()
	return streamMessageParts(t, chatID, id, revision, role, []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}, deleted)
}

func streamMessageParts(t *testing.T, chatID uuid.UUID, id int64, revision int64, role database.ChatMessageRole, parts []codersdk.ChatMessagePart, deleted bool) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		ChatID:         chatID,
		CreatedAt:      time.Unix(id, 0),
		Role:           role,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		Deleted:        deleted,
		Revision:       revision,
	}
}
