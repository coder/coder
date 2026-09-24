package chatd

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// turnToken identifies one turn. Methods that take a token act only
// while that turn is the open one, so a holder of a token for a turn
// that has since been replaced cannot finish or invalidate the
// replacement.
type turnToken uint64

// errChatInterrupted is the error an interrupted chat_turn span ends
// with.
var errChatInterrupted = xerrors.New("chat interrupted")

// runnerTurnSpan owns the chat_turn span of one runner. The span opens
// on the first Ensure call, not at construction, and one instance runs
// several turns in sequence: each Ensure that finds no open turn, or an
// open turn for an older trigger, starts a new span.
//
// A turn closes in two steps: Complete marks it finished and Settle
// closes the span, so stages still open at Complete end inside the
// turn's span and count in its accounting if they end before Settle.
type runnerTurnSpan struct {
	stages *chatloop.StageTracer
	// organizationName resolves a chat's organization ID to the name
	// carried on the turn's stage spans. A nil resolver leaves it empty.
	organizationName func(context.Context, uuid.UUID) string

	mu           sync.Mutex
	span         *chatloop.StageSpan
	spanCtx      trace.SpanContext
	acc          *chatloop.TurnAccumulator
	chatID       string
	chatKind     chatloop.ChatKind
	organization string
	// token is the identity of the open turn. It advances every time a
	// turn span is started.
	token turnToken
	open  bool
	ended bool
	// finished marks an open turn that reached a terminal transition
	// and is waiting for Settle to close it.
	finished bool
	// outcome is the classification Invalidate recorded for the open
	// turn, empty until then. Only the first Invalidate is kept.
	outcome chatloop.TurnOutcome
	// invalidErr is the error recorded alongside outcome. The span ends
	// with it.
	invalidErr error
	// triggerAt is the trigger time passed to the Ensure call that
	// opened the open turn, before any adjustment of the anchor.
	triggerAt time.Time
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

func newRunnerTurnSpan(stages *chatloop.StageTracer, organizationName func(context.Context, uuid.UUID) string, takenOver bool) *runnerTurnSpan {
	return &runnerTurnSpan{stages: stages, organizationName: organizationName, takenOver: takenOver}
}

// Ensure returns a context parented to the open chat_turn span and the
// token of that turn, starting the span when none is open. triggerAt is
// the time of the event that triggered the turn and becomes the span's
// start timestamp, and an acquisition stage is recorded from it to now
// inside the turn. The acquisition stage carries no model identity;
// none is known when the turn opens.
//
// A turn that already reached a terminal transition, or that was
// invalidated, is closed first. An open turn is closed as abandoned
// when triggerAt is after the trigger it opened for, since the call
// runs a newer prompt.
//
// A new turn starts at now and records no acquisition in three cases:
// it is the first turn on a runner that took the chat over from a
// previous owner; triggerAt is at or before the previous turn's
// anchor, which is also counted as a stale_anchor anomaly; or
// triggerAt is zero because the chat has neither a user prompt nor a
// compaction request.
//
// When ctx is done, Ensure returns ctx and the zero token, which no
// turn matches, and leaves the open turn untouched: a canceled task
// cannot join, finish, or invalidate whichever turn is open.
//
// The span is a standalone trace root. The request that triggered the
// turn is handled by a different goroutine, and often a different
// replica, than the worker that runs it, so no inbound span context
// is available here to link.
func (t *runnerTurnSpan) Ensure(ctx context.Context, chat database.Chat, triggerAt time.Time) (context.Context, turnToken) {
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
	if t.ended || ctx.Err() != nil {
		return ctx, 0
	}
	if t.open && (t.finished || t.outcome != "") {
		t.closeLocked(nil)
	}
	if t.open {
		if !triggerAt.After(t.triggerAt) {
			return t.contextLocked(ctx), t.token
		}
		t.closeLocked(nil)
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
	t.chatID = chat.ID.String()
	t.chatKind = chatKind(chat)
	t.organization = organization
	turnCtx := t.startLocked(ctx, anchorAt)
	t.triggerAt = triggerAt
	// A zero anchor has no start to measure acquisition from.
	if !anchorAt.IsZero() {
		t.stages.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{},
			anchorAt, t.stages.Now(), nil,
			attribute.String(chatloop.AttrChatID, t.chatID))
	}
	return t.contextLocked(ctx), t.token
}

// startLocked opens a chat_turn span anchored at startAt, or at now
// when startAt is zero, and returns the context parented to it.
func (t *runnerTurnSpan) startLocked(ctx context.Context, startAt time.Time) context.Context {
	if startAt.IsZero() {
		startAt = t.stages.Now()
	}
	t.token++
	t.open = true
	t.finished = false
	t.outcome = ""
	t.invalidErr = nil
	t.lastAnchorAt = startAt
	t.acc = chatloop.NewTurnAccumulator()

	// The turn is open but has no span yet, so contextLocked adds only
	// the stage identity and accumulator the chat_turn span reads.
	turnCtx, span := t.stages.StartRootAt(t.contextLocked(ctx), chatloop.StageChatTurn, startAt,
		attribute.String(chatloop.AttrChatID, t.chatID))
	t.span = span
	t.spanCtx = span.SpanContext()
	return turnCtx
}

func (t *runnerTurnSpan) contextLocked(ctx context.Context) context.Context {
	if !t.open || t.ended {
		return ctx
	}
	// The stage identity and accumulator are set independently of the
	// span context so stages run on this context keep them when tracing
	// is not recording.
	ctx = withStageIdentity(ctx, chatloop.ScopeTurn, t.chatKind, t.organization)
	ctx = chatloop.ContextWithTurnAccumulator(ctx, t.acc)
	if !t.spanCtx.IsValid() {
		return ctx
	}
	return trace.ContextWithSpanContext(ctx, t.spanCtx)
}

// Complete marks the turn identified by token as finished normally.
// The span stays open until Settle.
func (t *runnerTurnSpan) Complete(token turnToken) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ownsLocked(token) {
		return
	}
	t.finished = true
}

// Invalidate records outcome and err against the turn identified by
// token. outcome is one of chatloop.TurnOutcomeInterrupted,
// TurnOutcomeError, or TurnOutcomeAbandoned. The first call is kept and
// later ones are ignored, as is a call after Complete: the finishing
// transition has committed by then, so a later failure does not undo
// the turn. The span stays open; when it closes it ends with err and
// carries outcome.
func (t *runnerTurnSpan) Invalidate(token turnToken, outcome chatloop.TurnOutcome, err error) {
	if t == nil || outcome == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ownsLocked(token) || t.finished || t.outcome != "" {
		return
	}
	t.outcome = outcome
	t.invalidErr = err
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
	if !t.ownsLocked(token) || (!t.finished && t.outcome == "") {
		return
	}
	t.closeLocked(nil)
}

// ownsLocked reports whether token identifies the open turn.
func (t *runnerTurnSpan) ownsLocked(token turnToken) bool {
	return t.open && !t.ended && token == t.token
}

// OpenToken returns the token of the open turn, or the zero token when
// no turn is open.
func (t *runnerTurnSpan) OpenToken() turnToken {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.open || t.ended {
		return 0
	}
	return t.token
}

// End closes the chat_turn span. Later calls are ignored.
func (t *runnerTurnSpan) End(err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended || !t.open {
		t.ended = true
		return
	}
	t.ended = true
	t.closeLocked(err)
}

// closeLocked ends the open turn span with exactly one turn_outcome,
// which also labels the turn's outcome count and time partition.
// An outcome recorded by Invalidate wins, and its error replaces err
// so the root span reports the failure that stopped the turn. Without
// one, a turn Complete marked finished is completed and any other open
// turn is abandoned.
func (t *runnerTurnSpan) closeLocked(err error) {
	outcome := t.outcome
	switch {
	case outcome != "":
		err = t.invalidErr
	case t.finished:
		outcome = chatloop.TurnOutcomeCompleted
	default:
		outcome = chatloop.TurnOutcomeAbandoned
	}
	t.span.EndTurn(outcome, err)
	t.span = nil
	t.spanCtx = trace.SpanContext{}
	t.acc = nil
	t.open = false
	t.finished = false
	t.outcome = ""
	t.invalidErr = nil
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
