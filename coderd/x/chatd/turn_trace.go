package chatd

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

// turnToken identifies one turn. Methods that take a token act only
// while that turn is the open one, so a holder of a token for a turn
// that has since been replaced cannot finish or invalidate the
// replacement.
type turnToken uint64

// runnerTurnSpan owns the chat_turn span of one runner. The span opens
// on the first Ensure call, not at construction, and one instance runs
// several turns in sequence: a finished turn is replaced by a new span
// when a queued message is promoted or when the next Ensure arrives.
//
// A turn closes in two steps: Complete marks it finished and Settle
// closes the span, so stages still open at Complete end inside the
// turn's span if they end before Settle.
type runnerTurnSpan struct {
	stages *chatloop.StageTracer
	// organizationName resolves a chat's organization ID to the name
	// carried on the turn's stage spans. A nil resolver leaves it empty.
	organizationName func(context.Context, uuid.UUID) string

	mu           sync.Mutex
	span         *chatloop.StageSpan
	spanCtx      trace.SpanContext
	chatID       string
	chatKind     string
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
	outcome string
	// invalidErr is the error recorded alongside outcome. The span ends
	// with it.
	invalidErr error
	// lastTriggerAt is the anchor the previous turn on this runner
	// opened with, including a promoted turn anchored at the moment its
	// message was queued. No later turn is anchored before it.
	lastTriggerAt time.Time
	// pendingPromotion is the queued message the finishing transition
	// promoted, when it promoted one. Settle opens the next turn from
	// it.
	pendingPromotion *turnPromotion
}

// turnPromotion records a queued message promoted by the transition
// that finished a turn: when the message was queued and when the
// promotion happened.
type turnPromotion struct {
	queuedAt   time.Time
	promotedAt time.Time
}

func newRunnerTurnSpan(stages *chatloop.StageTracer, organizationName func(context.Context, uuid.UUID) string) *runnerTurnSpan {
	return &runnerTurnSpan{stages: stages, organizationName: organizationName}
}

// Ensure returns a context parented to the open chat_turn span and the
// token of that turn, starting the span when none is open. triggerAt is
// the time of the event that triggered the turn and becomes the span's
// start timestamp, so the acquisition stage reconstructed from the same
// instant falls inside the turn. The acquisition stage carries no model
// identity; none is known when the turn opens.
//
// The anchor never precedes the anchor of the previous turn on this
// runner: a triggerAt older than that anchor is clamped to it and
// counted as a stale_anchor anomaly, so a turn started by an event
// older than the previous turn's trigger does not claim that turn's
// time as its acquisition. A trigger that lands while the previous
// turn is still running is a valid anchor and is kept.
//
// A turn that already reached a terminal transition, or that was
// invalidated, is closed first: the prompt this call runs is a new
// turn, and folding it into the old span would report the two as one.
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
	if t.ended {
		return ctx, 0
	}
	if t.open && (t.finished || t.outcome != "") {
		t.settleLocked(ctx)
	}
	if t.open {
		return t.contextLocked(ctx), t.token
	}
	if !triggerAt.IsZero() && triggerAt.Before(t.lastTriggerAt) {
		triggerAt = t.lastTriggerAt
		t.stages.RecordAnomaly(chatloop.StageAnomalyStaleAnchor)
	}
	t.chatID = chat.ID.String()
	t.chatKind = chatKindAttr(chat)
	t.organization = organization
	turnCtx := t.startLocked(ctx, triggerAt)
	// The window between the trigger message landing in history and a
	// worker picking the chat up is the acquisition. A turn opened by a
	// promotion has no acquisition: its head is the queue wait of the
	// message that opened it, and recording both would count that
	// window twice. A zero triggerAt gives no start to measure from.
	if !triggerAt.IsZero() {
		t.stages.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{},
			triggerAt, t.stages.Now(), nil,
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
	t.pendingPromotion = nil
	t.lastTriggerAt = startAt

	// The chat kind and organization ride on the context so every stage
	// of the turn carries them.
	ctx = chatloop.ContextWithChatKind(ctx, t.chatKind)
	ctx = chatloop.ContextWithOrganization(ctx, t.organization)
	turnCtx, span := t.stages.StartRootAt(ctx, chatloop.StageChatTurn, startAt, nil,
		attribute.String(chatloop.AttrChatID, t.chatID))
	t.span = span
	t.spanCtx = span.SpanContext()
	return turnCtx
}

// Context returns ctx parented to the chat_turn span, or ctx
// unchanged while no turn span is open.
func (t *runnerTurnSpan) Context(ctx context.Context) context.Context {
	if t == nil {
		return ctx
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.contextLocked(ctx)
}

func (t *runnerTurnSpan) contextLocked(ctx context.Context) context.Context {
	if !t.open || t.ended {
		return ctx
	}
	// The scope, chat kind, and organization are set independently of
	// the span context so stages run on this context keep them when
	// tracing is not recording.
	ctx = chatloop.ContextWithScope(ctx, chatloop.ScopeTurn)
	ctx = chatloop.ContextWithChatKind(ctx, t.chatKind)
	ctx = chatloop.ContextWithOrganization(ctx, t.organization)
	if !t.spanCtx.IsValid() {
		return ctx
	}
	return trace.ContextWithSpanContext(ctx, t.spanCtx)
}

// Complete marks the turn identified by token as finished normally.
// The span stays open until Settle.
//
// A non-zero queuedAt is the creation time of a queued message the
// finishing transition promoted. Settle opens the next turn anchored
// at it, because the wait that message served and the work it causes
// belong to the turn it starts rather than the one that released it.
func (t *runnerTurnSpan) Complete(token turnToken, queuedAt time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ownsLocked(token) {
		return
	}
	t.finished = true
	if !queuedAt.IsZero() {
		t.pendingPromotion = &turnPromotion{queuedAt: queuedAt, promotedAt: t.stages.Now()}
	}
}

// Invalidate records outcome and err against the turn identified by
// token. outcome is one of chatloop.TurnOutcomeInterrupted,
// TurnOutcomeError, or TurnOutcomeAbandoned. The first call is kept and
// later ones are ignored. The span stays open; when it closes it ends
// with err and carries outcome.
func (t *runnerTurnSpan) Invalidate(token turnToken, outcome string, err error) {
	if t == nil || outcome == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ownsLocked(token) || t.outcome != "" {
		return
	}
	t.outcome = outcome
	t.invalidErr = err
}

// Settle closes the turn identified by token if Complete marked it
// finished, and opens the next turn when the finishing transition
// promoted a queued message. A turn that is not finished is left open.
func (t *runnerTurnSpan) Settle(ctx context.Context, token turnToken) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ownsLocked(token) || !t.finished {
		return
	}
	t.settleLocked(ctx)
}

// ownsLocked reports whether token identifies the open turn.
func (t *runnerTurnSpan) ownsLocked(token turnToken) bool {
	return t.open && !t.ended && token == t.token
}

// settleLocked closes the open turn. When the finishing transition
// promoted a queued message, it opens the next turn anchored at the
// moment the message was queued and records that message's queue wait
// against the new turn.
func (t *runnerTurnSpan) settleLocked(ctx context.Context) {
	promotion := t.pendingPromotion
	t.closeLocked(nil)
	if promotion == nil {
		return
	}
	turnCtx := t.startLocked(ctx, promotion.queuedAt)
	t.stages.Record(turnCtx, chatloop.StageQueueWait, chatloop.StageModel{},
		promotion.queuedAt, promotion.promotedAt, nil,
		attribute.String(chatloop.AttrChatID, t.chatID))
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

// closeLocked ends the open turn span with exactly one turn_outcome.
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
	t.span.SetAttributes(attribute.String(chatloop.AttrTurnOutcome, outcome))
	t.span.End(err)
	t.span = nil
	t.spanCtx = trace.SpanContext{}
	t.open = false
	t.finished = false
	t.outcome = ""
	t.invalidErr = nil
	t.pendingPromotion = nil
}

// chatKindAttr labels a chat as a subagent or a top-level chat.
func chatKindAttr(chat database.Chat) string {
	if chat.ParentChatID.Valid {
		return chatloop.ChatKindSubagent
	}
	return chatloop.ChatKindRoot
}

// triggerMessageTime returns the creation time of the message that
// triggered the turn, which is the last user prompt in history. It
// returns the zero time when the history holds no user prompt.
func triggerMessageTime(messages []database.ChatMessage) time.Time {
	index := lastUserPromptIndex(messages)
	if index == -1 {
		return time.Time{}
	}
	return messages[index].CreatedAt
}

// turnTriggerTime returns the time of the event that triggered the
// turn: the later of the last user prompt's creation and a pending
// compaction request. It returns the zero time when neither exists.
func turnTriggerTime(chat database.Chat, messages []database.ChatMessage) time.Time {
	triggerAt := triggerMessageTime(messages)
	if chat.CompactionRequestedAt.Valid && chat.CompactionRequestedAt.Time.After(triggerAt) {
		triggerAt = chat.CompactionRequestedAt.Time
	}
	return triggerAt
}
