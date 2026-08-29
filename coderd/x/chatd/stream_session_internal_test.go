package chatd

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

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

func TestSessionSyncSourcesAndClose(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	state, db := newSessionTestStore(t, database.Chat{
		ID:              chatID,
		Status:          database.ChatStatusRunning,
		SnapshotVersion: 1,
		HistoryVersion:  1,
	})
	state.setMessages([]database.ChatMessage{
		streamMessage(t, chatID, 1, 1, database.ChatMessageRoleUser, "initial", false),
	})
	ps := dbpubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })
	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker("chatd", "stream-sync-poller")
	defer tickerTrap.Close()
	poller := newStreamSyncPoller(ctx, db, clock, slogtest.Make(t, nil))
	poller.Start()
	t.Cleanup(poller.Close)
	tick := tickerTrap.MustWait(ctx)
	require.Equal(t, streamSyncInterval, tick.Duration)
	tick.MustRelease(ctx)

	cfg := SessionConfig{
		ctx:    ctx,
		chat:   state.chat(),
		db:     db,
		pubsub: ps,
		poller: poller,
		clock:  clock,
		logger: slogtest.Make(t, nil),
	}
	session := NewSession(cfg)
	require.NotNil(t, session)
	requireEventTypes(t, session.InitialSnapshot(),
		codersdk.ChatStreamEventTypeMessage,
		codersdk.ChatStreamEventTypeStatus,
		codersdk.ChatStreamEventTypePreviewReset,
	)

	state.set(database.Chat{
		ID:              chatID,
		Status:          database.ChatStatusWaiting,
		SnapshotVersion: 2,
		HistoryVersion:  2,
	})
	state.setMessages([]database.ChatMessage{
		streamMessage(t, chatID, 1, 2, database.ChatMessageRoleUser, "edited", false),
	})
	publishSessionUpdate(t, ps, state.chat())
	require.Equal(t, "edited", receiveSessionEvent(ctx, t, session.Events(), codersdk.ChatStreamEventTypeMessage).Message.Content[0].Text)
	require.Equal(t, codersdk.ChatStatusWaiting, receiveSessionEvent(ctx, t, session.Events(), codersdk.ChatStreamEventTypeStatus).Status.Status)

	state.set(database.Chat{
		ID:              chatID,
		Status:          database.ChatStatusRunning,
		SnapshotVersion: 3,
		HistoryVersion:  2,
	})
	clock.Advance(streamSyncInterval).MustWait(ctx)
	require.Equal(t, codersdk.ChatStatusRunning, receiveSessionEvent(ctx, t, session.Events(), codersdk.ChatStreamEventTypeStatus).Status.Status)

	session.Close()
	session.Close()
	_, open := <-session.Events()
	require.False(t, open)
	ids, _ := poller.snapshotSubscribers()
	require.Empty(t, ids)

	state.set(database.Chat{
		ID:              chatID,
		Status:          database.ChatStatusWaiting,
		SnapshotVersion: 4,
		HistoryVersion:  2,
	})
	reconnected := NewSession(cfg)
	require.NotNil(t, reconnected)
	require.Equal(t, codersdk.ChatStatusWaiting, receiveSnapshotEvent(t, reconnected.InitialSnapshot(), codersdk.ChatStreamEventTypeStatus).Status.Status)
	reconnected.Close()
}

func TestSessionSyncRetryBackoff(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	state, db := newSessionTestStore(t, database.Chat{ID: chatID, Status: database.ChatStatusRunning, SnapshotVersion: 1})
	ps := dbpubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })
	clock := quartz.NewMock(t)
	poller := newStreamSyncPoller(ctx, db, clock, slogtest.Make(t, nil))
	t.Cleanup(poller.Close)
	session := NewSession(SessionConfig{ctx: ctx, chat: state.chat(), db: db, pubsub: ps, poller: poller, clock: clock, logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})})
	require.NotNil(t, session)
	t.Cleanup(session.Close)

	state.set(database.Chat{ID: chatID, Status: database.ChatStatusWaiting, SnapshotVersion: 2})
	state.failLocks(4)
	trap := clock.Trap().NewTimer("chatd", "stream-sync-retry")
	defer trap.Close()
	publishSessionUpdate(t, ps, state.chat())
	for _, delay := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond} {
		call := trap.MustWait(ctx)
		require.Equal(t, delay, call.Duration)
		call.MustRelease(ctx)
		clock.Advance(delay).MustWait(ctx)
	}
	require.Equal(t, codersdk.ChatStatusWaiting, receiveSessionEvent(ctx, t, session.Events(), codersdk.ChatStreamEventTypeStatus).Status.Status)
}

func TestSessionRelayReconnectAndReselect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	workerID := uuid.New()
	state, db := newSessionTestStore(t, database.Chat{
		ID:                chatID,
		Status:            database.ChatStatusRunning,
		SnapshotVersion:   1,
		HistoryVersion:    1,
		GenerationAttempt: 1,
		WorkerID:          uuid.NullUUID{UUID: workerID, Valid: true},
	})
	ps := dbpubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })
	clock := quartz.NewMock(t)
	poller := newStreamSyncPoller(ctx, db, clock, slogtest.Make(t, nil))
	t.Cleanup(poller.Close)
	first := newTestStreamPartsSession()
	second := newTestStreamPartsSession()
	available := make(chan *testStreamPartsSession, 2)
	available <- first
	available <- second
	var dials atomic.Int32
	dialer := func(_ context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
		require.Equal(t, chatID, input.ChatID)
		require.Equal(t, workerID, input.WorkerID)
		dials.Add(1)
		return <-available, nil
	}

	session := NewSession(SessionConfig{
		ctx:             ctx,
		chat:            state.chat(),
		requestHeader:   http.Header{"X-Test": {"relay"}},
		db:              db,
		pubsub:          ps,
		poller:          poller,
		streamPartsDial: dialer,
		clock:           clock,
		logger:          slogtest.Make(t, nil),
	})
	require.NotNil(t, session)
	t.Cleanup(session.Close)
	require.Equal(t, streamEpisode{history: 1, attempt: 1}, testutil.RequireReceive(ctx, t, first.selected))

	first.parts <- StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 1, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("first")}
	require.Equal(t, "first", receiveSessionEvent(ctx, t, session.Events(), codersdk.ChatStreamEventTypeMessagePart).MessagePart.Part.Text)

	retryTrap := clock.Trap().NewTimer("chatd", "stream-relay-retry")
	defer retryTrap.Close()
	close(first.parts)
	retry := retryTrap.MustWait(ctx)
	require.Equal(t, streamRelayRetryInitialBackoff, retry.Duration)
	retry.MustRelease(ctx)
	clock.Advance(streamRelayRetryInitialBackoff).MustWait(ctx)
	require.Equal(t, streamEpisode{history: 1, attempt: 1}, testutil.RequireReceive(ctx, t, second.selected))

	state.set(database.Chat{
		ID:                chatID,
		Status:            database.ChatStatusRunning,
		SnapshotVersion:   2,
		HistoryVersion:    2,
		GenerationAttempt: 2,
		WorkerID:          uuid.NullUUID{UUID: workerID, Valid: true},
	})
	publishSessionUpdate(t, ps, state.chat())
	require.Equal(t, streamEpisode{history: 2, attempt: 2}, testutil.RequireReceive(ctx, t, second.selected))
	second.parts <- StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("stale")}
	second.parts <- StreamPart{HistoryVersion: 2, GenerationAttempt: 2, Seq: 1, Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessageText("second")}
	require.Equal(t, "second", receiveSessionEvent(ctx, t, session.Events(), codersdk.ChatStreamEventTypeMessagePart).MessagePart.Part.Text)
	require.Equal(t, int32(2), dials.Load())

	session.Close()
	select {
	case <-second.closed:
	case <-ctx.Done():
		require.FailNow(t, "stream parts session did not close")
	}
}

func TestStreamSyncPollerUnregisterDuringPoll(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	db.EXPECT().GetChatStreamSyncRows(gomock.Any(), []uuid.UUID{chatID}).DoAndReturn(func(context.Context, []uuid.UUID) ([]database.GetChatStreamSyncRowsRow, error) {
		close(started)
		<-release
		return []database.GetChatStreamSyncRowsRow{{ID: chatID, SnapshotVersion: 1}}, nil
	})
	poller := newStreamSyncPoller(ctx, db, nil, slogtest.Make(t, nil))
	t.Cleanup(poller.Close)
	_, unregister := poller.Register(chatID)
	go func() {
		defer close(done)
		poller.pollOnce()
	}()

	<-started
	unregister()
	close(release)
	<-done
	chatIDs, _ := poller.snapshotSubscribers()
	require.Empty(t, chatIDs)
}

type sessionTestStore struct {
	mu       sync.Mutex
	current  database.Chat
	messages []database.ChatMessage
	failures int
}

func newSessionTestStore(t *testing.T, chat database.Chat) (*sessionTestStore, *dbmock.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	state := &sessionTestStore{current: chat}
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) })
	db.EXPECT().GetChatByIDForShare(gomock.Any(), chat.ID).AnyTimes().DoAndReturn(func(context.Context, uuid.UUID) (database.Chat, error) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.failures > 0 {
			state.failures--
			return database.Chat{}, xerrors.New("retry")
		}
		return state.current, nil
	})
	db.EXPECT().GetChatByID(gomock.Any(), chat.ID).AnyTimes().DoAndReturn(func(context.Context, uuid.UUID) (database.Chat, error) { return state.chat(), nil })
	db.EXPECT().GetChatMessagesByRevisionForStream(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, arg database.GetChatMessagesByRevisionForStreamParams) ([]database.ChatMessage, error) {
		state.mu.Lock()
		defer state.mu.Unlock()
		var messages []database.ChatMessage
		for _, message := range state.messages {
			if message.Revision > arg.AfterRevision {
				messages = append(messages, message)
			}
		}
		return messages, nil
	})
	db.EXPECT().GetChatMessagesByChatID(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, database.GetChatMessagesByChatIDParams) ([]database.ChatMessage, error) {
		state.mu.Lock()
		defer state.mu.Unlock()
		return append([]database.ChatMessage(nil), state.messages...), nil
	})
	db.EXPECT().GetChatQueuedMessages(gomock.Any(), chat.ID).AnyTimes().Return(nil, nil)
	db.EXPECT().GetChatStreamSyncRows(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(context.Context, []uuid.UUID) ([]database.GetChatStreamSyncRowsRow, error) {
		current := state.chat()
		return []database.GetChatStreamSyncRowsRow{{
			ID: current.ID, SnapshotVersion: current.SnapshotVersion, HistoryVersion: current.HistoryVersion,
			QueueVersion: current.QueueVersion, RetryStateVersion: current.RetryStateVersion,
			GenerationAttempt: current.GenerationAttempt, Status: current.Status, WorkerID: current.WorkerID,
		}}, nil
	})
	return state, db
}

func (s *sessionTestStore) set(chat database.Chat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = chat
}

func (s *sessionTestStore) setMessages(messages []database.ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = messages
}

func (s *sessionTestStore) failLocks(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = count
}

func (s *sessionTestStore) chat() database.Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

type streamEpisode struct {
	history int64
	attempt int64
}

type testStreamPartsSession struct {
	parts    chan StreamPart
	selected chan streamEpisode
	closed   chan struct{}
	close    sync.Once
}

func newTestStreamPartsSession() *testStreamPartsSession {
	return &testStreamPartsSession{parts: make(chan StreamPart, 4), selected: make(chan streamEpisode, 4), closed: make(chan struct{})}
}

func (s *testStreamPartsSession) SelectEpisode(_ context.Context, historyVersion, generationAttempt int64) error {
	s.selected <- streamEpisode{history: historyVersion, attempt: generationAttempt}
	return nil
}

func (s *testStreamPartsSession) Parts() <-chan StreamPart { return s.parts }

func (s *testStreamPartsSession) Close() error {
	s.close.Do(func() { close(s.closed) })
	return nil
}

func publishSessionUpdate(t *testing.T, ps dbpubsub.Pubsub, chat database.Chat) {
	t.Helper()
	workerID := (*uuid.UUID)(nil)
	if chat.WorkerID.Valid {
		workerID = &chat.WorkerID.UUID
	}
	payload, err := json.Marshal(coderdpubsub.ChatStateUpdateMessage{
		SnapshotVersion: chat.SnapshotVersion, WorkerID: workerID, HistoryVersion: chat.HistoryVersion,
		QueueVersion: chat.QueueVersion, RetryStateVersion: chat.RetryStateVersion,
		GenerationAttempt: chat.GenerationAttempt, Status: string(chat.Status),
	})
	require.NoError(t, err)
	require.NoError(t, ps.Publish(coderdpubsub.ChatStateUpdateChannel(chat.ID), payload))
}

func receiveSessionEvent(ctx context.Context, t *testing.T, events <-chan codersdk.ChatStreamEvent, eventType codersdk.ChatStreamEventType) codersdk.ChatStreamEvent {
	t.Helper()
	for {
		event := testutil.RequireReceive(ctx, t, events)
		if event.Type == eventType {
			return event
		}
	}
}

func receiveSnapshotEvent(t *testing.T, events []codersdk.ChatStreamEvent, eventType codersdk.ChatStreamEventType) codersdk.ChatStreamEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	require.FailNow(t, "event not found", "type: %s", eventType)
	return codersdk.ChatStreamEvent{}
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
