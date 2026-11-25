package chatd

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// turnToken identifies one turn, so a stale holder cannot act on a
// replacement turn. The zero token matches no turn.
type turnToken uint64

// runnerTurnSpan owns the sequential chat_turn spans of one runner.
//
// Closing a turn fixes its end time, but the span is emitted only once
// every holder has called Release, so an unwinding task can still
// record the outcome.
type runnerTurnSpan struct {
	stages *chatloop.StageTracer

	mu sync.Mutex
	// turns holds every turn not yet emitted.
	turns     map[turnToken]*turnState
	current   turnToken
	lastToken turnToken
	shutDown  bool
	// lastAnchorAt keeps a future trigger even though its span starts
	// at now.
	lastAnchorAt time.Time
	// takenOver is cleared once the first turn opens.
	takenOver bool
}

// turnState is guarded by runnerTurnSpan.mu.
type turnState struct {
	token    turnToken
	span     *chatloop.StageSpan
	spanCtx  trace.SpanContext
	chatKind chatloop.ChatKind
	// triggerAt is unadjusted, unlike the span's anchor.
	triggerAt  time.Time
	finished   bool
	outcome    chatloop.TurnOutcome
	invalidErr error
	closed     bool
	endAt      time.Time
	closeErr   error
	holders    map[uuid.UUID]struct{}
}

func newRunnerTurnSpan(stages *chatloop.StageTracer, takenOver bool) *runnerTurnSpan {
	return &runnerTurnSpan{
		stages:    stages,
		turns:     map[turnToken]*turnState{},
		takenOver: takenOver,
	}
}

// Ensure joins taskID to the current turn, or starts one anchored at
// triggerAt. A newer triggerAt replaces the current turn. A canceled
// ctx gets the zero token so it cannot act on a replacement turn.
func (t *runnerTurnSpan) Ensure(ctx context.Context, taskID uuid.UUID, chat database.Chat, triggerAt time.Time) (context.Context, turnToken) {
	if t == nil {
		return ctx, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shutDown || ctx.Err() != nil {
		return ctx, 0
	}
	if turn := t.turns[t.current]; turn != nil {
		switch {
		case turn.finished || turn.outcome != "":
			t.closeLocked(turn, nil)
		case !triggerAt.After(turn.triggerAt):
			turn.hold(taskID)
			return turn.context(ctx), turn.token
		default:
			t.closeLocked(turn, nil)
		}
	}

	anchorAt := triggerAt
	switch {
	case t.takenOver:
		anchorAt = time.Time{}
	case !triggerAt.IsZero() && !t.lastAnchorAt.IsZero() && !triggerAt.After(t.lastAnchorAt):
		anchorAt = time.Time{}
		t.stages.RecordAnomaly(chatloop.StageAnomalyStaleAnchor)
	}
	t.takenOver = false
	turn := t.startLocked(ctx, chat, triggerAt, anchorAt)
	turn.hold(taskID)
	return turn.context(ctx), turn.token
}

// startLocked anchors at now, with no acquisition, when startAt is zero.
func (t *runnerTurnSpan) startLocked(ctx context.Context, chat database.Chat, triggerAt, startAt time.Time) *turnState {
	anchored := !startAt.IsZero()
	if !anchored {
		startAt = t.stages.Now()
	}
	t.lastToken++
	turn := &turnState{
		token:     t.lastToken,
		chatKind:  chatKind(chat),
		triggerAt: triggerAt,
		holders:   map[uuid.UUID]struct{}{},
	}
	t.turns[turn.token] = turn
	t.current = turn.token
	t.lastAnchorAt = startAt

	chatID := attribute.String(chatloop.AttrChatID, chat.ID.String())
	turnCtx, span := t.stages.StartRootAt(turn.context(ctx), chatloop.StageChatTurn, startAt, chatID)
	turn.span = span
	turn.spanCtx = span.SpanContext()
	if anchored {
		t.stages.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{},
			startAt, t.stages.Now(), nil, chatID)
	}
	return turn
}

func (turn *turnState) hold(taskID uuid.UUID) {
	if taskID != uuid.Nil {
		turn.holders[taskID] = struct{}{}
	}
}

func (turn *turnState) context(ctx context.Context) context.Context {
	// Stage identity is needed even when tracing is not recording.
	ctx = withStageIdentity(ctx, chatloop.ScopeTurn, turn.chatKind)
	if !turn.spanCtx.IsValid() {
		return ctx
	}
	return trace.ContextWithSpanContext(ctx, turn.spanCtx)
}

// Complete marks the turn finished; it stays open until Settle so
// in-flight stages end inside it.
func (t *runnerTurnSpan) Complete(token turnToken) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if turn := t.turns[token]; turn != nil {
		turn.finished = true
	}
}

// Invalidate is ignored after Complete, since the finishing transition
// has already committed. Only the first call is kept.
func (t *runnerTurnSpan) Invalidate(token turnToken, outcome chatloop.TurnOutcome, err error) {
	if t == nil || outcome == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	turn := t.turns[token]
	if turn == nil || turn.finished || turn.outcome != "" {
		return
	}
	turn.outcome = outcome
	turn.invalidErr = err
}

// Settle closes the turn only if it was completed or invalidated.
func (t *runnerTurnSpan) Settle(token turnToken) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	turn := t.turns[token]
	if turn == nil || (!turn.finished && turn.outcome == "") {
		return
	}
	t.closeLocked(turn, nil)
}

// Release must be called after the task returns so its stages have
// ended before the turn is emitted.
func (t *runnerTurnSpan) Release(taskID uuid.UUID) {
	if t == nil || taskID == uuid.Nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, turn := range t.pendingLocked() {
		delete(turn.holders, taskID)
		t.emitIfReleasedLocked(turn)
	}
}

// OpenToken returns the zero token when ctx is done.
func (t *runnerTurnSpan) OpenToken(ctx context.Context) turnToken {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if ctx.Err() != nil {
		return 0
	}
	return t.current
}

// End emits every pending turn regardless of holders. No turn opens
// afterwards.
func (t *runnerTurnSpan) End(err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shutDown {
		return
	}
	t.shutDown = true
	if turn := t.turns[t.current]; turn != nil {
		t.closeLocked(turn, err)
	}
	for _, turn := range t.pendingLocked() {
		t.closeLocked(turn, err)
		clear(turn.holders)
		t.emitIfReleasedLocked(turn)
	}
}

func (t *runnerTurnSpan) pendingLocked() []*turnState {
	tokens := slices.Sorted(maps.Keys(t.turns))
	turns := make([]*turnState, 0, len(tokens))
	for _, token := range tokens {
		turns = append(turns, t.turns[token])
	}
	return turns
}

func (t *runnerTurnSpan) closeLocked(turn *turnState, err error) {
	if turn.closed {
		return
	}
	turn.closed = true
	turn.endAt = t.stages.Now()
	turn.closeErr = err
	if t.current == turn.token {
		t.current = 0
	}
	t.emitIfReleasedLocked(turn)
}

func (t *runnerTurnSpan) emitIfReleasedLocked(turn *turnState) {
	if !turn.closed || len(turn.holders) > 0 {
		return
	}
	outcome, err := turn.outcome, turn.closeErr
	switch {
	case outcome != "":
		err = turn.invalidErr
	case turn.finished:
		outcome = chatloop.TurnOutcomeCompleted
	default:
		outcome = chatloop.TurnOutcomeAbandoned
	}
	turn.span.EndTurn(outcome, err, turn.endAt)
	delete(t.turns, turn.token)
}

// triggerMessageTime also counts dynamic tool results, which the client
// submits to resume a chat from requires_action.
func triggerMessageTime(messages []database.ChatMessage, dynamicTools map[string]bool) time.Time {
	var triggerAt time.Time
	start := 0
	if index := lastUserPromptIndex(messages); index != -1 {
		triggerAt = messages[index].CreatedAt
		start = index + 1
	}
	if len(dynamicTools) == 0 {
		return triggerAt
	}
	for _, msg := range messages[start:] {
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleTool || !msg.CreatedAt.After(triggerAt) {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeToolResult && dynamicTools[part.ToolName] {
				triggerAt = msg.CreatedAt
				break
			}
		}
	}
	return triggerAt
}

func turnTriggerTime(chat database.Chat, messages []database.ChatMessage) time.Time {
	triggerAt := triggerMessageTime(messages, dynamicToolNamesFromChat(chat))
	if chat.CompactionRequestedAt.Valid && chat.CompactionRequestedAt.Time.After(triggerAt) {
		triggerAt = chat.CompactionRequestedAt.Time
	}
	return triggerAt
}
