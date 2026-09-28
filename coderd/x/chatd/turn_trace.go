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

// turnToken identifies one turn. Methods that take a token act only on
// that turn, and only until it is emitted, so a holder of a token for a
// turn that has since been replaced cannot finish or invalidate the
// replacement.
type turnToken uint64

// runnerTurnSpan owns the chat_turn spans of one runner. A span opens
// on the first Ensure call, not at construction, and one instance runs
// several turns in sequence: each Ensure that finds no current turn, or
// a current turn for an older trigger, starts a new span. At most one
// turn is current, the one Ensure joins.
//
// A turn closes in two steps: Complete marks it finished and Settle
// closes it, so stages still open at Complete end inside the turn's
// span if they end before Settle. Closing fixes the turn's end time.
// The span is emitted once the turn is closed and every task that
// joined it has called Release, so an outcome recorded by a task that
// is still unwinding reaches the turn it ran.
type runnerTurnSpan struct {
	stages *chatloop.StageTracer
	// organizationName resolves a chat's organization ID to the name
	// carried on the turn's stage spans. A nil resolver leaves it empty.
	organizationName func(context.Context, uuid.UUID) string

	mu sync.Mutex
	// turns holds every turn that has opened and is not yet emitted.
	turns map[turnToken]*turnState
	// current is the token of the turn Ensure joins, zero when none.
	current turnToken
	// lastToken advances every time a turn span is started.
	lastToken turnToken
	// shutDown is set by End. No turn opens after it.
	shutDown bool
	// lastAnchorAt is the anchor requested for the most recent turn:
	// its trigger time, or this replica's clock for a turn that opened
	// at now. A trigger ahead of this replica's clock is kept here even
	// though the span itself starts at now.
	lastAnchorAt time.Time
	// takenOver is set until the first turn opens on a runner that
	// acquired its chat from a previous owner. That turn starts at now
	// and records no acquisition.
	takenOver bool
}

// turnState is one turn of a runnerTurnSpan, guarded by its mutex.
type turnState struct {
	token        turnToken
	span         *chatloop.StageSpan
	spanCtx      trace.SpanContext
	chatKind     chatloop.ChatKind
	organization string
	// triggerAt is the trigger time passed to the Ensure call that
	// opened the turn, before any adjustment of the anchor.
	triggerAt time.Time
	// finished marks a turn that reached a terminal transition.
	finished bool
	// outcome is the classification Invalidate recorded, empty until
	// then. Only the first Invalidate is kept.
	outcome chatloop.TurnOutcome
	// invalidErr is the error recorded alongside outcome. The span ends
	// with it.
	invalidErr error
	closed     bool
	endAt      time.Time
	// closeErr is the error the span ends with when no outcome was
	// recorded.
	closeErr error
	// holders are the tasks that joined the turn and have not called
	// Release.
	holders map[uuid.UUID]struct{}
}

func newRunnerTurnSpan(stages *chatloop.StageTracer, organizationName func(context.Context, uuid.UUID) string, takenOver bool) *runnerTurnSpan {
	return &runnerTurnSpan{
		stages:           stages,
		organizationName: organizationName,
		turns:            map[turnToken]*turnState{},
		takenOver:        takenOver,
	}
}

// Ensure returns a context parented to the current chat_turn span and
// the token of that turn, starting the span when none is current, and
// records taskID as holding the turn until Release. A zero taskID
// holds nothing. triggerAt is the time of the event that triggered the
// turn and becomes the span's start timestamp, and an acquisition stage
// is recorded from it to now inside the turn. The acquisition stage
// carries no model identity; none is known when the turn opens.
//
// A current turn that already reached a terminal transition, or that
// was invalidated, is closed first. A current turn is also closed when
// triggerAt is after the trigger it opened for, since the call runs a
// newer prompt; it is counted as abandoned unless its task records an
// outcome before releasing it.
//
// A new turn starts at now and records no acquisition in three cases:
// it is the first turn on a runner that took the chat over from a
// previous owner; triggerAt is at or before the previous turn's
// anchor, which is also counted as a stale_anchor anomaly; or
// triggerAt is zero because the chat has neither a user prompt nor a
// compaction request.
//
// When ctx is done, Ensure returns ctx and the zero token, which no
// turn matches, and leaves every turn untouched: a canceled task
// cannot join, finish, or invalidate a turn it did not already hold.
//
// The span is a standalone trace root. The request that triggered the
// turn is handled by a different goroutine, and often a different
// replica, than the worker that runs it, so no inbound span context
// is available here to link.
func (t *runnerTurnSpan) Ensure(ctx context.Context, taskID uuid.UUID, chat database.Chat, triggerAt time.Time) (context.Context, turnToken) {
	if t == nil {
		return ctx, 0
	}
	// The resolver may hit the database, so it runs before the lock is
	// taken.
	organization := ""
	if t.organizationName != nil {
		organization = t.organizationName(ctx, chat.OrganizationID)
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
	turn := t.startLocked(ctx, chat, organization, triggerAt, anchorAt)
	turn.hold(taskID)
	return turn.context(ctx), turn.token
}

// startLocked opens a chat_turn span anchored at startAt, or at now
// when startAt is zero, makes it the current turn, and records the
// acquisition stage from a nonzero startAt to now.
func (t *runnerTurnSpan) startLocked(ctx context.Context, chat database.Chat, organization string, triggerAt, startAt time.Time) *turnState {
	anchored := !startAt.IsZero()
	if !anchored {
		startAt = t.stages.Now()
	}
	t.lastToken++
	turn := &turnState{
		token:        t.lastToken,
		chatKind:     chatKind(chat),
		organization: organization,
		triggerAt:    triggerAt,
		holders:      map[uuid.UUID]struct{}{},
	}
	t.turns[turn.token] = turn
	t.current = turn.token
	t.lastAnchorAt = startAt

	chatID := attribute.String(chatloop.AttrChatID, chat.ID.String())
	// The turn has no span yet, so context adds only the stage identity
	// the chat_turn span reads.
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

// context returns ctx carrying the turn's stage identity and, once the
// span has started, its span context.
func (turn *turnState) context(ctx context.Context) context.Context {
	// The stage identity is set independently of the span context so
	// stages run on this context keep it when tracing is not recording.
	ctx = withStageIdentity(ctx, chatloop.ScopeTurn, turn.chatKind, turn.organization)
	if !turn.spanCtx.IsValid() {
		return ctx
	}
	return trace.ContextWithSpanContext(ctx, turn.spanCtx)
}

// Complete marks the turn identified by token as finished normally.
// The turn stays open until Settle, or until a newer turn replaces it.
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

// Invalidate records outcome and err against the turn identified by
// token. outcome is chatloop.TurnOutcomeInterrupted or
// TurnOutcomeError. The first call is kept and later ones are ignored,
// as is a call after Complete: the finishing transition has committed
// by then, so a later failure does not undo the turn. The span ends
// with err and carries outcome.
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

// Settle closes the turn identified by token if Complete marked it
// finished or Invalidate recorded an outcome for it. Any other turn is
// left open.
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

// Release records that taskID no longer runs any turn, and emits every
// closed turn it was the last holder of. Call it after the task has
// returned, so every stage it started has ended.
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

// OpenToken returns the token of the current turn, or the zero token
// when none is current. Like Ensure, it returns the zero token when ctx
// is done, so a canceled task cannot act on the current turn.
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

// End closes the current turn with err and emits every turn not yet
// emitted, whether or not its tasks released it. No turn opens after
// End. Later calls are ignored.
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

// pendingLocked returns the turns not yet emitted, oldest first.
func (t *runnerTurnSpan) pendingLocked() []*turnState {
	tokens := slices.Sorted(maps.Keys(t.turns))
	turns := make([]*turnState, 0, len(tokens))
	for _, token := range tokens {
		turns = append(turns, t.turns[token])
	}
	return turns
}

// closeLocked fixes the turn's end time at now and emits it if no task
// holds it. A closed turn is no longer current. err is the error the
// span ends with when no outcome is recorded. Closing a closed turn
// does nothing.
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

// emitIfReleasedLocked ends the span of a closed turn with no holders
// at its end time, with exactly one turn_outcome. An outcome recorded
// by Invalidate wins, and its error replaces the close error so the
// root span reports the failure that stopped the turn. Without one, a
// turn Complete marked finished is completed and any other turn is
// abandoned.
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

// triggerMessageTime returns the creation time of the message that
// triggered the turn: the last user prompt, or a later tool result for
// one of dynamicTools, which the client submits to resume a chat that
// entered requires_action. It returns the zero time when history holds
// neither.
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

// turnTriggerTime returns the time of the event that triggered the
// turn: the later of the trigger message's creation and a pending
// compaction request. It returns the zero time when neither exists.
func turnTriggerTime(chat database.Chat, messages []database.ChatMessage) time.Time {
	triggerAt := triggerMessageTime(messages, dynamicToolNamesFromChat(chat))
	if chat.CompactionRequestedAt.Valid && chat.CompactionRequestedAt.Time.After(triggerAt) {
		triggerAt = chat.CompactionRequestedAt.Time
	}
	return triggerAt
}
