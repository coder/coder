package chatd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	streamSyncRetryInitialBackoff = 100 * time.Millisecond
	streamSyncRetryMaxBackoff     = time.Second
	streamSyncRetryMaxAttempts    = 5
)

// SessionConfig configures a live chat stream session.
type SessionConfig struct {
	ctx             context.Context
	chat            database.Chat
	requestHeader   http.Header
	afterMessageID  int64
	db              database.Store
	pubsub          dbpubsub.Pubsub
	poller          *streamSyncPoller
	streamPartsDial StreamPartsDialer
	clock           quartz.Clock
	logger          slog.Logger
}

// Session owns a live chat stream subscription.
type Session struct {
	initial []codersdk.ChatStreamEvent
	events  <-chan codersdk.ChatStreamEvent
	cancel  context.CancelFunc
	unsub   func()
	done    <-chan struct{}
	close   sync.Once
}

// StreamSessionConfig returns the production configuration for a chat stream.
func (p *Server) StreamSessionConfig(
	ctx context.Context,
	chat database.Chat,
	requestHeader http.Header,
	afterMessageID int64,
) SessionConfig {
	if p == nil {
		return SessionConfig{}
	}
	return SessionConfig{
		ctx:             ctx,
		chat:            chat,
		requestHeader:   requestHeader,
		afterMessageID:  afterMessageID,
		db:              p.db,
		pubsub:          p.pubsub,
		poller:          p.streamSyncPoller,
		streamPartsDial: p.streamPartsDialer,
		clock:           p.clock,
		logger:          p.logger.With(slog.F("chat_id", chat.ID)),
	}
}

// NewSession starts a live chat stream session.
func NewSession(cfg SessionConfig) *Session {
	if cfg.ctx == nil || cfg.db == nil || cfg.pubsub == nil || cfg.poller == nil {
		return nil
	}
	if cfg.clock == nil {
		cfg.clock = quartz.NewReal()
	}

	ctx, cancel := context.WithCancel(cfg.ctx)
	events := make(chan codersdk.ChatStreamEvent, 128)
	updates := make(chan streamSyncHint, 32)
	unsub, err := cfg.pubsub.SubscribeWithErr(
		coderdpubsub.ChatStateUpdateChannel(cfg.chat.ID),
		coderdpubsub.HandleChatStateUpdate(func(_ context.Context, payload coderdpubsub.ChatStateUpdateMessage, err error) {
			if err != nil {
				cfg.logger.Warn(ctx, "chat stream pubsub error", slog.Error(err))
				return
			}
			select {
			case updates <- streamSyncHintFromUpdate(payload):
			case <-ctx.Done():
			}
		}),
	)
	if err != nil {
		cfg.logger.Warn(cfg.ctx, "failed to subscribe to chat state updates", slog.Error(err))
		cancel()
		return newSessionError(cfg.chat.ID, "failed to subscribe to chat updates")
	}

	pollerEvents, unregister := cfg.poller.Register(cfg.chat.ID)
	state := newStreamLoop(cfg.chat, cfg.db, cfg.logger, cfg.afterMessageID)
	//nolint:gocritic // The HTTP route authorizes the chat before constructing the session.
	initial, target, _, err := state.syncDB(dbauthz.AsChatd(cfg.ctx))
	if err != nil {
		cfg.logger.Error(cfg.ctx, "failed to load initial chat stream snapshot", slog.Error(err))
		unregister()
		unsub()
		cancel()
		return newSessionError(cfg.chat.ID, "failed to load initial snapshot")
	}

	relay := newStreamRelayForwarder(
		cfg.chat.ID,
		cfg.requestHeader,
		cfg.streamPartsDial,
		cfg.clock,
		cfg.logger,
	)
	relay.Configure(ctx, target)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(events)
		defer relay.Close()
		defer unregister()
		runSync := func(hint streamSyncHint) bool {
			syncEvents, target, changed, err := syncSessionWithRetry(ctx, cfg, state, hint)
			if err != nil {
				cfg.logger.Error(ctx, "failed to sync chat stream after retries", slog.Error(err))
				return false
			}
			for _, event := range syncEvents {
				if !sendStreamEvent(ctx, events, event) {
					return false
				}
			}
			if changed {
				relay.Configure(ctx, target)
			}
			return true
		}
		for {
			select {
			case <-ctx.Done():
				return
			case hint := <-updates:
				if !runSync(hint) {
					return
				}
			case hint, ok := <-pollerEvents:
				if !ok || !runSync(hint) {
					return
				}
			case part, ok := <-relay.Parts():
				if !ok {
					return
				}
				event, accepted, err := state.part(part)
				if err != nil {
					cfg.logger.Error(ctx, "chat stream invariant violation", slog.Error(err))
					return
				}
				if accepted && !sendStreamEvent(ctx, events, event) {
					return
				}
			}
		}
	}()

	return &Session{initial: initial, events: events, cancel: cancel, unsub: unsub, done: done}
}

// InitialSnapshot returns the events captured before the live stream started.
func (s *Session) InitialSnapshot() []codersdk.ChatStreamEvent {
	if s == nil {
		return nil
	}
	return s.initial
}

// Events returns live events until the session closes.
func (s *Session) Events() <-chan codersdk.ChatStreamEvent {
	if s == nil {
		return nil
	}
	return s.events
}

// Close stops the session and waits for its goroutines to exit.
func (s *Session) Close() {
	if s == nil {
		return
	}
	s.close.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.unsub != nil {
			s.unsub()
		}
		if s.done != nil {
			<-s.done
		}
	})
}

func newSessionError(chatID uuid.UUID, message string) *Session {
	events := make(chan codersdk.ChatStreamEvent)
	close(events)
	return &Session{
		initial: []codersdk.ChatStreamEvent{{
			Type:   codersdk.ChatStreamEventTypeError,
			ChatID: chatID,
			Error:  &codersdk.ChatError{Message: message},
		}},
		events: events,
	}
}

func syncSessionWithRetry(
	ctx context.Context,
	cfg SessionConfig,
	state *streamLoop,
	hint streamSyncHint,
) ([]codersdk.ChatStreamEvent, streamRelayTarget, bool, error) {
	var (
		events  []codersdk.ChatStreamEvent
		target  streamRelayTarget
		changed bool
		err     error
	)
	for attempt := 1; attempt <= streamSyncRetryMaxAttempts; attempt++ {
		//nolint:gocritic // The session was authorized before construction.
		events, target, changed, err = state.sync(dbauthz.AsChatd(ctx), hint)
		if err == nil || ctx.Err() != nil {
			return events, target, changed, err
		}
		cfg.logger.Warn(ctx, "failed to sync chat stream", slog.F("attempt", attempt), slog.Error(err))
		if attempt < streamSyncRetryMaxAttempts && !waitBeforeSessionSyncRetry(ctx, cfg.clock, attempt) {
			return nil, state.currentRelayTarget(), false, ctx.Err()
		}
	}
	return nil, state.currentRelayTarget(), false, err
}

func waitBeforeSessionSyncRetry(ctx context.Context, clock quartz.Clock, attempt int) bool {
	delay := streamSyncRetryInitialBackoff
	for range attempt - 1 {
		delay *= 2
		if delay >= streamSyncRetryMaxBackoff {
			delay = streamSyncRetryMaxBackoff
			break
		}
	}
	timer := clock.NewTimer(delay, "chatd", "stream-sync-retry")
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func sendStreamEvent(ctx context.Context, ch chan<- codersdk.ChatStreamEvent, event codersdk.ChatStreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

type streamLoop struct {
	chatID uuid.UUID
	db     database.Store
	logger slog.Logger
	state  streamLocalState
}

type streamLocalState struct {
	snapshotVersion int64
	historyVersion  int64
	queueVersion    int64
	retryVersion    int64

	knownMessages map[int64]int64

	status database.ChatStatus

	errorHistoryVersion          int64
	actionRequiredHistoryVersion int64

	workerID          uuid.NullUUID
	generationAttempt int64
	lastPartSeq       int64

	afterMessageID         int64
	initialMessageSyncDone bool
}

type streamSyncHint struct {
	snapshotVersion   int64
	historyVersion    int64
	queueVersion      int64
	retryVersion      int64
	status            database.ChatStatus
	workerID          uuid.NullUUID
	generationAttempt int64
}

type streamDBSnapshot struct {
	chat database.Chat

	historyChanged  bool
	changedMessages []database.ChatMessage
	historyReset    bool
	fullHistory     []database.ChatMessage

	queueChanged bool
	queue        []database.ChatQueuedMessage

	actionRequired *codersdk.ChatStreamActionRequired
}

func newStreamLoop(chat database.Chat, db database.Store, logger slog.Logger, afterMessageID int64) *streamLoop {
	return &streamLoop{
		chatID: chat.ID,
		db:     db,
		logger: logger,
		state: streamLocalState{
			knownMessages:  make(map[int64]int64),
			afterMessageID: afterMessageID,
		},
	}
}

func streamSyncHintFromUpdate(update coderdpubsub.ChatStateUpdateMessage) streamSyncHint {
	hint := streamSyncHint{
		snapshotVersion:   update.SnapshotVersion,
		historyVersion:    update.HistoryVersion,
		queueVersion:      update.QueueVersion,
		retryVersion:      update.RetryStateVersion,
		status:            database.ChatStatus(update.Status),
		generationAttempt: update.GenerationAttempt,
	}
	if update.WorkerID != nil {
		hint.workerID = uuid.NullUUID{UUID: *update.WorkerID, Valid: true}
	}
	return hint
}

func (l *streamLoop) sync(ctx context.Context, hint streamSyncHint) ([]codersdk.ChatStreamEvent, streamRelayTarget, bool, error) {
	if !l.shouldFetch(hint) {
		return nil, l.currentRelayTarget(), false, nil
	}
	return l.syncDB(ctx)
}

func (l *streamLoop) syncDB(ctx context.Context) ([]codersdk.ChatStreamEvent, streamRelayTarget, bool, error) {
	snapshot, err := l.loadDBSnapshot(ctx)
	if err != nil {
		return nil, l.currentRelayTarget(), false, err
	}
	if snapshot.chat.SnapshotVersion <= l.state.snapshotVersion {
		return nil, l.currentRelayTarget(), false, nil
	}
	return l.applyDBSnapshot(snapshot), l.currentRelayTarget(), true, nil
}

func (l *streamLoop) shouldFetch(hint streamSyncHint) bool {
	if hint.snapshotVersion <= l.state.snapshotVersion {
		return false
	}
	if hint.historyVersion > l.state.historyVersion {
		return true
	}
	if hint.queueVersion > l.state.queueVersion {
		return true
	}
	if hint.retryVersion > l.state.retryVersion {
		return true
	}
	if hint.status != l.state.status {
		return true
	}
	if !nullUUIDEqual(hint.workerID, l.state.workerID) {
		return true
	}
	if hint.generationAttempt != l.state.generationAttempt {
		return true
	}
	return false
}

func (l *streamLoop) loadDBSnapshot(ctx context.Context) (streamDBSnapshot, error) {
	var snapshot streamDBSnapshot
	machine := chatstate.NewChatMachine(l.db, nil, l.chatID)
	err := machine.ReadLock(ctx, func(tx database.Store) error {
		chat, err := tx.GetChatByID(ctx, l.chatID)
		if err != nil {
			return xerrors.Errorf("get chat for stream: %w", err)
		}
		snapshot.chat = chat

		if chat.HistoryVersion > l.state.historyVersion {
			snapshot.historyChanged = true
			snapshot.changedMessages, err = tx.GetChatMessagesByRevisionForStream(ctx, database.GetChatMessagesByRevisionForStreamParams{
				ChatID:        l.chatID,
				AfterRevision: l.state.historyVersion,
			})
			if err != nil {
				return xerrors.Errorf("get changed chat messages: %w", err)
			}
			for _, msg := range snapshot.changedMessages {
				if msg.Deleted {
					snapshot.historyReset = true
					break
				}
			}
			if snapshot.historyReset {
				snapshot.fullHistory, err = tx.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
					ChatID:  l.chatID,
					AfterID: 0,
				})
				if err != nil {
					return xerrors.Errorf("get full chat history: %w", err)
				}
			}
		}

		if chat.QueueVersion > l.state.queueVersion {
			snapshot.queueChanged = true
			snapshot.queue, err = tx.GetChatQueuedMessages(ctx, l.chatID)
			if err != nil {
				return xerrors.Errorf("get chat queue: %w", err)
			}
		}

		if chat.Status == database.ChatStatusRequiresAction {
			history := snapshot.fullHistory
			if len(history) == 0 {
				history, err = tx.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
					ChatID:  l.chatID,
					AfterID: 0,
				})
				if err != nil {
					return xerrors.Errorf("get requires_action history: %w", err)
				}
			}
			actionRequired, err := l.actionRequiredFromHistory(chat, history)
			if err != nil {
				return err
			}
			snapshot.actionRequired = actionRequired
		}
		return nil
	})
	if err != nil {
		return streamDBSnapshot{}, err
	}
	return snapshot, nil
}

func (*streamLoop) actionRequiredFromHistory(chat database.Chat, messages []database.ChatMessage) (*codersdk.ChatStreamActionRequired, error) {
	dynamicToolNames, err := parseDynamicToolNames(chat.DynamicTools)
	if err != nil {
		return nil, xerrors.Errorf("parse dynamic tools for stream: %w", err)
	}
	_, pending, err := unresolvedToolCallsFromHistory(messages, dynamicToolNames)
	if err != nil {
		return nil, xerrors.Errorf("derive pending dynamic tool calls: %w", err)
	}
	toolCalls := make([]codersdk.ChatStreamToolCall, 0, len(pending))
	for _, call := range pending {
		toolCalls = append(toolCalls, codersdk.ChatStreamToolCall{
			ToolCallID: call.ToolCallID,
			ToolName:   call.ToolName,
			Args:       call.Args,
		})
	}
	return &codersdk.ChatStreamActionRequired{ToolCalls: toolCalls}, nil
}

func (l *streamLoop) applyDBSnapshot(snapshot streamDBSnapshot) []codersdk.ChatStreamEvent {
	chat := snapshot.chat
	events := make([]codersdk.ChatStreamEvent, 0)
	historyChanged := chat.HistoryVersion > l.state.historyVersion
	generationChanged := chat.GenerationAttempt != l.state.generationAttempt

	if historyChanged {
		events = append(events, l.messageEvents(snapshot)...)
	}
	if !l.state.initialMessageSyncDone {
		l.state.initialMessageSyncDone = true
	}

	if chat.QueueVersion > l.state.queueVersion {
		events = append(events, codersdk.ChatStreamEvent{
			Type:           codersdk.ChatStreamEventTypeQueueUpdate,
			ChatID:         l.chatID,
			QueuedMessages: db2sdk.ChatQueuedMessages(snapshot.queue),
		})
	}

	if chat.Status != l.state.status {
		events = append(events, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeStatus,
			ChatID: l.chatID,
			Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(chat.Status)},
		})
	}

	if chat.Status == database.ChatStatusError && chat.HistoryVersion > l.state.errorHistoryVersion {
		events = append(events, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeError,
			ChatID: l.chatID,
			Error:  l.chatError(chat),
		})
		l.state.errorHistoryVersion = chat.HistoryVersion
	}

	if chat.Status == database.ChatStatusRequiresAction && chat.HistoryVersion > l.state.actionRequiredHistoryVersion {
		actionRequired := snapshot.actionRequired
		if actionRequired == nil {
			actionRequired = &codersdk.ChatStreamActionRequired{}
		}
		events = append(events, codersdk.ChatStreamEvent{
			Type:           codersdk.ChatStreamEventTypeActionRequired,
			ChatID:         l.chatID,
			ActionRequired: actionRequired,
		})
		l.state.actionRequiredHistoryVersion = chat.HistoryVersion
	}

	if chat.RetryStateVersion > l.state.retryVersion {
		if retry := l.retryEvent(chat); retry != nil {
			events = append(events, *retry)
		}
	}

	if historyChanged || (generationChanged && chat.GenerationAttempt != 0) {
		l.state.lastPartSeq = 0
		events = append(events, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypePreviewReset,
			ChatID: l.chatID,
		})
	}

	l.state.snapshotVersion = chat.SnapshotVersion
	l.state.historyVersion = chat.HistoryVersion
	l.state.queueVersion = chat.QueueVersion
	l.state.retryVersion = chat.RetryStateVersion
	l.state.status = chat.Status
	l.state.workerID = chat.WorkerID
	l.state.generationAttempt = chat.GenerationAttempt
	return events
}

func (l *streamLoop) messageEvents(snapshot streamDBSnapshot) []codersdk.ChatStreamEvent {
	if snapshot.historyReset {
		events := []codersdk.ChatStreamEvent{{
			Type:   codersdk.ChatStreamEventTypeHistoryReset,
			ChatID: l.chatID,
		}}
		clear(l.state.knownMessages)
		for _, msg := range snapshot.fullHistory {
			l.state.knownMessages[msg.ID] = msg.Revision
			sdkMsg := db2sdk.ChatMessage(msg)
			events = append(events, codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				ChatID:  l.chatID,
				Message: &sdkMsg,
			})
		}
		return events
	}

	events := make([]codersdk.ChatStreamEvent, 0, len(snapshot.changedMessages))
	for _, msg := range snapshot.changedMessages {
		knownRevision := l.state.knownMessages[msg.ID]
		if knownRevision >= msg.Revision {
			continue
		}
		l.state.knownMessages[msg.ID] = msg.Revision
		if !l.state.initialMessageSyncDone && msg.ID <= l.state.afterMessageID {
			continue
		}
		sdkMsg := db2sdk.ChatMessage(msg)
		events = append(events, codersdk.ChatStreamEvent{
			Type:    codersdk.ChatStreamEventTypeMessage,
			ChatID:  l.chatID,
			Message: &sdkMsg,
		})
	}
	return events
}

func (l *streamLoop) chatError(chat database.Chat) *codersdk.ChatError {
	if !chat.LastError.Valid || len(chat.LastError.RawMessage) == 0 {
		return &codersdk.ChatError{
			Message: "The chat request failed unexpectedly.",
			Kind:    codersdk.ChatErrorKindGeneric,
		}
	}
	var payload codersdk.ChatError
	if err := json.Unmarshal(chat.LastError.RawMessage, &payload); err != nil {
		l.logger.Warn(context.Background(), "failed to parse chat stream last_error",
			slog.F("chat_id", l.chatID),
			slog.Error(err),
		)
		return &codersdk.ChatError{
			Message: "The chat request failed unexpectedly.",
			Kind:    codersdk.ChatErrorKindGeneric,
		}
	}
	if payload.Message == "" {
		payload.Message = "The chat request failed unexpectedly."
	}
	if payload.Kind == "" {
		payload.Kind = codersdk.ChatErrorKindGeneric
	}
	return &payload
}

func (l *streamLoop) retryEvent(chat database.Chat) *codersdk.ChatStreamEvent {
	if !chat.RetryState.Valid || len(chat.RetryState.RawMessage) == 0 {
		return nil
	}
	var retry codersdk.ChatStreamRetry
	if err := json.Unmarshal(chat.RetryState.RawMessage, &retry); err != nil {
		l.logger.Warn(context.Background(), "failed to parse chat stream retry_state",
			slog.F("chat_id", l.chatID),
			slog.Error(err),
		)
		return nil
	}
	return &codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeRetry,
		ChatID: l.chatID,
		Retry:  &retry,
	}
}

func (l *streamLoop) part(part streamPart) (event codersdk.ChatStreamEvent, accepted bool, err error) {
	if part.HistoryVersion != l.state.historyVersion || part.GenerationAttempt != l.state.generationAttempt {
		return codersdk.ChatStreamEvent{}, false, nil
	}
	if part.Seq <= l.state.lastPartSeq {
		return codersdk.ChatStreamEvent{}, false, nil
	}
	if part.Seq != l.state.lastPartSeq+1 {
		err := xerrors.Errorf(
			"chat stream message part sequence gap: got %d after %d",
			part.Seq,
			l.state.lastPartSeq,
		)
		l.logger.Error(context.Background(), "chat stream message part sequence gap",
			slog.F("chat_id", l.chatID),
			slog.F("history_version", part.HistoryVersion),
			slog.F("generation_attempt", part.GenerationAttempt),
			slog.F("last_seq", l.state.lastPartSeq),
			slog.F("seq", part.Seq),
			slog.Error(err),
		)
		return codersdk.ChatStreamEvent{}, false, err
	}
	l.state.lastPartSeq = part.Seq
	return codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeMessagePart,
		ChatID: l.chatID,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role:              part.Role,
			Part:              part.Part,
			HistoryVersion:    part.HistoryVersion,
			GenerationAttempt: part.GenerationAttempt,
			Seq:               part.Seq,
		},
	}, true, nil
}

func (l *streamLoop) currentRelayTarget() streamRelayTarget {
	return streamRelayTarget{
		workerID:          l.state.workerID,
		historyVersion:    l.state.historyVersion,
		generationAttempt: l.state.generationAttempt,
	}
}

const (
	streamRelayRetryInitialBackoff = 100 * time.Millisecond
	streamRelayRetryMaxBackoff     = 5 * time.Second
)

type streamRelayForwarder struct {
	chatID        uuid.UUID
	requestHeader http.Header
	dialer        StreamPartsDialer
	clock         quartz.Clock
	logger        slog.Logger

	parts chan StreamPart

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	configure chan streamRelayTarget
	closeOnce sync.Once
}

func newStreamRelayForwarder(
	chatID uuid.UUID,
	requestHeader http.Header,
	dialer StreamPartsDialer,
	clock quartz.Clock,
	logger slog.Logger,
) *streamRelayForwarder {
	if clock == nil {
		clock = quartz.NewReal()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &streamRelayForwarder{
		chatID:        chatID,
		requestHeader: requestHeader.Clone(),
		dialer:        dialer,
		clock:         clock,
		logger:        logger,
		parts:         make(chan StreamPart, 128),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
		configure:     make(chan streamRelayTarget, 1),
	}
	go f.loop()
	return f
}

func (f *streamRelayForwarder) Parts() <-chan StreamPart {
	return f.parts
}

func (f *streamRelayForwarder) Configure(ctx context.Context, target streamRelayTarget) {
	if f == nil {
		return
	}
	// Drop any pending target so the buffered channel always holds the most
	// recent configuration.
	select {
	case <-f.configure:
	default:
	}
	select {
	case f.configure <- target:
	case <-f.ctx.Done():
	case <-ctx.Done():
	}
}

func (f *streamRelayForwarder) Close() {
	if f == nil {
		return
	}
	f.closeOnce.Do(func() {
		f.cancel()
		<-f.done
	})
}

func (f *streamRelayForwarder) loop() {
	defer close(f.done)
	defer close(f.parts)
	var (
		target       streamRelayTarget
		connected    streamRelayTarget
		session      StreamPartsSession
		sessionParts <-chan StreamPart
		retryTimer   *quartz.Timer
		retryC       <-chan time.Time
		retryBackoff = streamRelayRetryInitialBackoff
	)
	stopRetry := func() {
		if retryTimer != nil {
			retryTimer.Stop()
			retryTimer = nil
			retryC = nil
		}
	}
	defer stopRetry()
	closeSession := func() {
		if session != nil {
			_ = session.Close()
		}
		session = nil
		sessionParts = nil
		connected = streamRelayTarget{}
	}
	defer closeSession()
	scheduleRetry := func() {
		if !target.needsRelay() || f.dialer == nil || retryTimer != nil {
			return
		}
		retryTimer = f.clock.NewTimer(retryBackoff, "chatd", "stream-relay-retry")
		retryC = retryTimer.C
		if retryBackoff < streamRelayRetryMaxBackoff {
			retryBackoff *= 2
			if retryBackoff > streamRelayRetryMaxBackoff {
				retryBackoff = streamRelayRetryMaxBackoff
			}
		}
	}
	connect := func(ctx context.Context) {
		stopRetry()
		if !target.needsRelay() {
			closeSession()
			return
		}
		if f.dialer == nil {
			return
		}
		if session != nil && connected.workerID.Valid && nullUUIDEqual(connected.workerID, target.workerID) {
			if err := session.SelectEpisode(ctx, target.historyVersion, target.generationAttempt); err != nil {
				f.logger.Warn(ctx, "failed to select stream parts episode",
					slog.F("chat_id", f.chatID),
					slog.F("history_version", target.historyVersion),
					slog.F("generation_attempt", target.generationAttempt),
					slog.Error(err),
				)
				closeSession()
				scheduleRetry()
				return
			}
			connected = target
			retryBackoff = streamRelayRetryInitialBackoff
			return
		}
		closeSession()
		newSession, err := f.dialer(ctx, StreamPartsDialInput{
			ChatID:        f.chatID,
			WorkerID:      target.workerID.UUID,
			RequestHeader: f.requestHeader.Clone(),
		})
		if err != nil {
			f.logger.Warn(ctx, "failed to dial stream parts relay",
				slog.F("chat_id", f.chatID),
				slog.F("worker_id", target.workerID.UUID),
				slog.Error(err),
			)
			// Unrecoverable dial errors (e.g. auth failures) will not
			// succeed on retry with the same inputs, so wait for the next
			// configuration instead of scheduling a retry.
			if !streamPartsDialUnrecoverable(err) {
				scheduleRetry()
			}
			return
		}
		session = newSession
		sessionParts = newSession.Parts()
		connected = streamRelayTarget{workerID: target.workerID}
		if err := session.SelectEpisode(ctx, target.historyVersion, target.generationAttempt); err != nil {
			f.logger.Warn(ctx, "failed to select stream parts episode",
				slog.F("chat_id", f.chatID),
				slog.F("history_version", target.historyVersion),
				slog.F("generation_attempt", target.generationAttempt),
				slog.Error(err),
			)
			closeSession()
			scheduleRetry()
			return
		}
		connected = target
		retryBackoff = streamRelayRetryInitialBackoff
	}

	for {
		select {
		case <-f.ctx.Done():
			return
		case nextTarget := <-f.configure:
			target = nextTarget
			connect(f.ctx)
		case <-retryC:
			retryTimer = nil
			retryC = nil
			connect(f.ctx)
		case part, ok := <-sessionParts:
			if !ok {
				closeSession()
				scheduleRetry()
				continue
			}
			if !connected.sameEpisode(target) ||
				part.HistoryVersion != target.historyVersion ||
				part.GenerationAttempt != target.generationAttempt {
				continue
			}
			select {
			case f.parts <- part:
			case <-f.ctx.Done():
				return
			}
		}
	}
}

func (t streamRelayTarget) needsRelay() bool {
	return t.workerID.Valid && t.generationAttempt > 0
}

// streamPartsDialUnrecoverable reports whether a dial error signals that
// retrying with the same inputs is futile, such as an auth failure. Dialers
// opt in by returning errors that implement IsUnrecoverable.
func streamPartsDialUnrecoverable(err error) bool {
	var unrecoverable interface{ IsUnrecoverable() bool }
	return errors.As(err, &unrecoverable) && unrecoverable.IsUnrecoverable()
}

func (t streamRelayTarget) sameEpisode(other streamRelayTarget) bool {
	return nullUUIDEqual(t.workerID, other.workerID) &&
		t.historyVersion == other.historyVersion &&
		t.generationAttempt == other.generationAttempt
}

const streamSyncInterval = 10 * time.Second

type streamSyncPoller struct {
	ctx    context.Context
	cancel context.CancelFunc
	db     database.Store
	clock  quartz.Clock
	logger slog.Logger

	mu          sync.Mutex
	subscribers map[uuid.UUID]map[*streamSyncPollerSubscriber]struct{}
}

type streamSyncPollerSubscriber struct {
	chatID uuid.UUID
	hints  chan streamSyncHint
}

func newStreamSyncPoller(
	ctx context.Context,
	db database.Store,
	clock quartz.Clock,
	logger slog.Logger,
) *streamSyncPoller {
	if clock == nil {
		clock = quartz.NewReal()
	}
	//nolint:gocritic // The poller is internal chatd infrastructure. Each
	// registered stream was already authorized before subscription, and this
	// batch query only fetches synchronization metadata for subscribed chats.
	pollerCtx, cancel := context.WithCancel(dbauthz.AsChatd(ctx))
	return &streamSyncPoller{
		ctx:         pollerCtx,
		cancel:      cancel,
		db:          db,
		clock:       clock,
		logger:      logger,
		subscribers: make(map[uuid.UUID]map[*streamSyncPollerSubscriber]struct{}),
	}
}

func (p *streamSyncPoller) Start() {
	if p == nil {
		return
	}
	go p.loop()
}

func (p *streamSyncPoller) Close() {
	if p == nil {
		return
	}
	p.cancel()
}

func (p *streamSyncPoller) Register(chatID uuid.UUID) (<-chan streamSyncHint, func()) {
	if p == nil {
		ch := make(chan streamSyncHint)
		close(ch)
		return ch, func() {}
	}
	subscriber := &streamSyncPollerSubscriber{
		chatID: chatID,
		hints:  make(chan streamSyncHint, 1),
	}
	p.mu.Lock()
	if p.subscribers[chatID] == nil {
		p.subscribers[chatID] = make(map[*streamSyncPollerSubscriber]struct{})
	}
	p.subscribers[chatID][subscriber] = struct{}{}
	p.mu.Unlock()

	return subscriber.hints, func() {
		p.unregister(subscriber)
	}
}

func (p *streamSyncPoller) unregister(subscriber *streamSyncPollerSubscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	chatSubscribers := p.subscribers[subscriber.chatID]
	if chatSubscribers == nil {
		return
	}
	delete(chatSubscribers, subscriber)
	if len(chatSubscribers) == 0 {
		delete(p.subscribers, subscriber.chatID)
	}
}

func (p *streamSyncPoller) loop() {
	ticker := p.clock.NewTicker(streamSyncInterval, "chatd", "stream-sync-poller")
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce()
		}
	}
}

func (p *streamSyncPoller) pollOnce() {
	chatIDs, subscribers := p.snapshotSubscribers()
	if len(chatIDs) == 0 {
		return
	}
	rows, err := p.db.GetChatStreamSyncRows(p.ctx, chatIDs)
	if err != nil {
		if p.ctx.Err() == nil {
			p.logger.Warn(p.ctx, "failed to poll chat streams", slog.Error(err))
		}
		return
	}
	for _, row := range rows {
		hint := streamSyncHintFromPollRow(row)
		for _, subscriber := range subscribers[row.ID] {
			select {
			case subscriber.hints <- hint:
			default:
			}
		}
	}
}

func (p *streamSyncPoller) snapshotSubscribers() ([]uuid.UUID, map[uuid.UUID][]*streamSyncPollerSubscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	chatIDs := make([]uuid.UUID, 0, len(p.subscribers))
	subscribers := make(map[uuid.UUID][]*streamSyncPollerSubscriber, len(p.subscribers))
	for chatID, chatSubscribers := range p.subscribers {
		chatIDs = append(chatIDs, chatID)
		for subscriber := range chatSubscribers {
			subscribers[chatID] = append(subscribers[chatID], subscriber)
		}
	}
	return chatIDs, subscribers
}

func streamSyncHintFromPollRow(row database.GetChatStreamSyncRowsRow) streamSyncHint {
	return streamSyncHint{
		snapshotVersion:   row.SnapshotVersion,
		historyVersion:    row.HistoryVersion,
		queueVersion:      row.QueueVersion,
		retryVersion:      row.RetryStateVersion,
		status:            row.Status,
		workerID:          row.WorkerID,
		generationAttempt: row.GenerationAttempt,
	}
}
