package chatd

import (
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
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
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

// TestStreamLoopQueuedMessageLink verifies that every message event
// carries the queued message link, so clients can tell apart promoted
// messages with identical content, whether they arrive in one live
// sync, an after_id replay, or a history_reset replay.
func TestStreamLoopQueuedMessageLink(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	promoted := func(id, queuedID int64) database.ChatMessage {
		msg := streamMessage(t, chatID, id, 1, database.ChatMessageRoleUser, "twin", false)
		msg.QueuedMessageID = sql.NullInt64{Int64: queuedID, Valid: true}
		return msg
	}
	seen := streamMessage(t, chatID, 1, 1, database.ChatMessageRoleUser, "direct", false)
	twins := []database.ChatMessage{promoted(2, 11), promoted(3, 12)}
	requireTwinEvents := func(t *testing.T, events []codersdk.ChatStreamEvent) {
		t.Helper()
		var messages []codersdk.ChatMessage
		for _, event := range events {
			if event.Type == codersdk.ChatStreamEventTypeMessage {
				require.NotNil(t, event.Message)
				messages = append(messages, *event.Message)
			}
		}
		require.Len(t, messages, 2)
		require.Equal(t, messages[0].Content, messages[1].Content)
		require.Equal(t, int64(2), messages[0].ID)
		require.Equal(t, ptr.Ref(int64(11)), messages[0].QueuedMessageID)
		require.Equal(t, int64(3), messages[1].ID)
		require.Equal(t, ptr.Ref(int64(12)), messages[1].QueuedMessageID)
	}

	t.Run("live sync with two promotions", func(t *testing.T) {
		t.Parallel()
		loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)
		events := loop.applyDBSnapshot(streamDBSnapshot{
			chat: database.Chat{
				ID:              chatID,
				Status:          database.ChatStatusRunning,
				SnapshotVersion: 1,
				HistoryVersion:  1,
			},
			changedMessages: []database.ChatMessage{seen},
		})
		require.Nil(t, events[0].Message.QueuedMessageID,
			"direct sends carry no queued message link")

		events = loop.applyDBSnapshot(streamDBSnapshot{
			chat: database.Chat{
				ID:              chatID,
				Status:          database.ChatStatusRunning,
				SnapshotVersion: 2,
				HistoryVersion:  2,
			},
			changedMessages: twins,
		})
		requireTwinEvents(t, events)
	})

	t.Run("after_id replay", func(t *testing.T) {
		t.Parallel()
		loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), seen.ID)
		events := loop.applyDBSnapshot(streamDBSnapshot{
			chat: database.Chat{
				ID:              chatID,
				Status:          database.ChatStatusRunning,
				SnapshotVersion: 2,
				HistoryVersion:  2,
			},
			changedMessages: append([]database.ChatMessage{seen}, twins...),
		})
		requireTwinEvents(t, events)
	})

	t.Run("history_reset replay", func(t *testing.T) {
		t.Parallel()
		loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)
		loop.state.snapshotVersion = 1
		loop.state.historyVersion = 1
		loop.state.status = database.ChatStatusRunning
		loop.state.initialMessageSyncDone = true
		loop.state.knownMessages[1] = 1
		events := loop.applyDBSnapshot(streamDBSnapshot{
			chat: database.Chat{
				ID:              chatID,
				Status:          database.ChatStatusRunning,
				SnapshotVersion: 2,
				HistoryVersion:  2,
			},
			changedMessages: []database.ChatMessage{
				streamMessage(t, chatID, 1, 2, database.ChatMessageRoleUser, "direct", true),
			},
			historyReset: true,
			fullHistory:  twins,
		})
		require.Equal(t, codersdk.ChatStreamEventTypeHistoryReset, events[0].Type)
		requireTwinEvents(t, events)
	})
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

func TestStreamLoopInitialSyncBoundsFetchToCursorRevision(t *testing.T) {
	t.Parallel()

	newLockedTx := func(t *testing.T, chatID uuid.UUID, chat database.Chat) (*dbmock.MockStore, *dbmock.MockStore) {
		t.Helper()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		tx := dbmock.NewMockStore(ctrl)
		db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(tx) },
		)
		tx.EXPECT().GetChatByIDForShare(gomock.Any(), chatID).Return(chat, nil)
		tx.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil)
		return db, tx
	}

	t.Run("CursorPresent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := uuid.New()
		chat := database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusWaiting,
			SnapshotVersion: 9,
			HistoryVersion:  9,
		}
		db, tx := newLockedTx(t, chatID, chat)
		loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 7)

		cursor := streamMessage(t, chatID, 7, 5, database.ChatMessageRoleUser, "already seen", false)
		tx.EXPECT().GetChatMessageByIDForStream(gomock.Any(), int64(7)).Return(cursor, nil)
		tx.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), database.GetChatMessagesByRevisionForStreamParams{
			ChatID:        chatID,
			AfterRevision: 4,
		}).Return([]database.ChatMessage{
			cursor,
			streamMessage(t, chatID, 8, 9, database.ChatMessageRoleAssistant, "new", false),
		}, nil)

		events, _, changed, err := loop.syncDB(ctx)
		require.NoError(t, err)
		require.True(t, changed)
		requireEventTypes(t, events,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		)
		require.Equal(t, int64(8), events[0].Message.ID)
	})

	t.Run("TruncationSharingCursorRevision", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := uuid.New()
		chat := database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusWaiting,
			SnapshotVersion: 6,
			HistoryVersion:  6,
		}
		db, tx := newLockedTx(t, chatID, chat)
		loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 9)

		cursor := streamMessage(t, chatID, 9, 5, database.ChatMessageRoleUser, "edited", false)
		tx.EXPECT().GetChatMessageByIDForStream(gomock.Any(), int64(9)).Return(cursor, nil)
		tx.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), database.GetChatMessagesByRevisionForStreamParams{
			ChatID:        chatID,
			AfterRevision: 4,
		}).Return([]database.ChatMessage{
			streamMessage(t, chatID, 7, 5, database.ChatMessageRoleUser, "original", true),
			streamMessage(t, chatID, 8, 5, database.ChatMessageRoleAssistant, "discarded", true),
			cursor,
			streamMessage(t, chatID, 10, 6, database.ChatMessageRoleAssistant, "new", false),
		}, nil)

		events, _, changed, err := loop.syncDB(ctx)
		require.NoError(t, err)
		require.True(t, changed)
		requireEventTypes(t, events,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		)
		require.Equal(t, int64(10), events[0].Message.ID)
	})

	t.Run("TombstoneAfterCursor", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := uuid.New()
		chat := database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusWaiting,
			SnapshotVersion: 7,
			HistoryVersion:  7,
		}
		db, tx := newLockedTx(t, chatID, chat)
		loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 7)

		cursor := streamMessage(t, chatID, 7, 5, database.ChatMessageRoleAssistant, "already seen", false)
		tx.EXPECT().GetChatMessageByIDForStream(gomock.Any(), int64(7)).Return(cursor, nil)
		tx.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), database.GetChatMessagesByRevisionForStreamParams{
			ChatID:        chatID,
			AfterRevision: 4,
		}).Return([]database.ChatMessage{
			cursor,
			streamMessage(t, chatID, 8, 6, database.ChatMessageRoleUser, "discarded", true),
			streamMessage(t, chatID, 9, 7, database.ChatMessageRoleUser, "edited", false),
		}, nil)

		events, _, changed, err := loop.syncDB(ctx)
		require.NoError(t, err)
		require.True(t, changed)
		requireEventTypes(t, events,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		)
		require.Equal(t, int64(9), events[0].Message.ID)
	})

	t.Run("HeldMessageDeletedAfterCursor", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := uuid.New()
		chat := database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusWaiting,
			SnapshotVersion: 8,
			HistoryVersion:  8,
		}
		db, tx := newLockedTx(t, chatID, chat)
		loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 7)

		cursor := streamMessage(t, chatID, 7, 5, database.ChatMessageRoleAssistant, "already seen", false)
		tx.EXPECT().GetChatMessageByIDForStream(gomock.Any(), int64(7)).Return(cursor, nil)
		tx.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), database.GetChatMessagesByRevisionForStreamParams{
			ChatID:        chatID,
			AfterRevision: 4,
		}).Return([]database.ChatMessage{
			streamMessage(t, chatID, 3, 8, database.ChatMessageRoleUser, "removed", true),
			cursor,
		}, nil)
		tx.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{
			streamMessage(t, chatID, 2, 1, database.ChatMessageRoleUser, "kept", false),
			cursor,
		}, nil)

		events, _, changed, err := loop.syncDB(ctx)
		require.NoError(t, err)
		require.True(t, changed)
		requireEventTypes(t, events,
			codersdk.ChatStreamEventTypeHistoryReset,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		)
		require.Equal(t, int64(2), events[1].Message.ID)
		require.Equal(t, int64(7), events[2].Message.ID)
	})

	t.Run("CursorDeleted", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := uuid.New()
		chat := database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusWaiting,
			SnapshotVersion: 9,
			HistoryVersion:  9,
		}
		db, tx := newLockedTx(t, chatID, chat)
		loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 7)

		tx.EXPECT().GetChatMessageByIDForStream(gomock.Any(), int64(7)).Return(
			streamMessage(t, chatID, 7, 9, database.ChatMessageRoleUser, "deleted", true), nil)
		tx.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), database.GetChatMessagesByRevisionForStreamParams{
			ChatID:        chatID,
			AfterRevision: 0,
		}).Return([]database.ChatMessage{
			streamMessage(t, chatID, 6, 5, database.ChatMessageRoleUser, "kept", false),
			streamMessage(t, chatID, 7, 9, database.ChatMessageRoleUser, "deleted", true),
		}, nil)
		tx.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{
			streamMessage(t, chatID, 6, 5, database.ChatMessageRoleUser, "kept", false),
		}, nil)

		events, _, changed, err := loop.syncDB(ctx)
		require.NoError(t, err)
		require.True(t, changed)
		requireEventTypes(t, events,
			codersdk.ChatStreamEventTypeHistoryReset,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		)
		require.Equal(t, int64(6), events[1].Message.ID)
	})

	t.Run("CursorUnknown", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		chatID := uuid.New()
		chat := database.Chat{
			ID:              chatID,
			Status:          database.ChatStatusWaiting,
			SnapshotVersion: 9,
			HistoryVersion:  9,
		}
		db, tx := newLockedTx(t, chatID, chat)
		loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 7)

		tx.EXPECT().GetChatMessageByIDForStream(gomock.Any(), int64(7)).Return(database.ChatMessage{}, sql.ErrNoRows)
		tx.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), database.GetChatMessagesByRevisionForStreamParams{
			ChatID:        chatID,
			AfterRevision: 0,
		}).Return([]database.ChatMessage{
			streamMessage(t, chatID, 6, 5, database.ChatMessageRoleUser, "already seen", false),
			streamMessage(t, chatID, 8, 9, database.ChatMessageRoleAssistant, "new", false),
		}, nil)

		events, _, changed, err := loop.syncDB(ctx)
		require.NoError(t, err)
		require.True(t, changed)
		requireEventTypes(t, events,
			codersdk.ChatStreamEventTypeMessage,
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		)
		require.Equal(t, int64(8), events[0].Message.ID)
	})
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
