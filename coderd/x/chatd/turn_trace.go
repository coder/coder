package chatd

import (
	"context"
	"sync"
	"time"

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

// runnerTurnSpan owns the chat_turn span of one runner. A runner is
// created when a chat is acquired and torn down when the chat leaves
// the running states, so the runner's lifetime bounds the turns it
// runs.
//
// The span is created on the first generation task rather than at
// runner construction: runners are also spawned to abandon or time
// out chats, which run no turn. One runner can run several turns in
// sequence: a finished turn is replaced by a new span, either when a
// queued message is promoted or when the next prompt starts a task.
//
// A turn closes in two steps: Complete marks it finished and Settle
// closes the span, so stages still open at Complete end inside the
// turn's span if they end before Settle.
type runnerTurnSpan struct {
	stages *chatloop.StageTracer

	mu       sync.Mutex
	span     *chatloop.StageSpan
	spanCtx  trace.SpanContext
	chatID   string
	chatKind string
	// token is the identity of the open turn. It advances every time a
	// turn span is started.
	token turnToken
	// open is true while a turn span exists that has not been closed.
	open  bool
	ended bool
	// finished marks an open turn that reached a terminal transition
	// and is waiting for Settle to close it.
	finished bool
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

func newRunnerTurnSpan(stages *chatloop.StageTracer) *runnerTurnSpan {
	return &runnerTurnSpan{stages: stages}
}

// Ensure returns a context parented to the open chat_turn span and the
// token of that turn, starting the span when none is open. triggerAt is
// the trigger message's creation time and becomes the span's start
// timestamp, so the acquisition stage reconstructed from the same
// instant falls inside the turn. The acquisition stage carries no model
// identity: the turn's model is not resolved until preparation runs.
//
// A turn that already reached a terminal transition is settled first:
// the prompt this call runs is a new turn, and folding it into the old
// span would report the two as one.
//
// The span is a standalone trace root. The request that triggered the
// turn is handled by a different goroutine, and often a different
// replica, than the worker that runs it, so no inbound span context
// is available here to link.
func (t *runnerTurnSpan) Ensure(ctx context.Context, chat database.Chat, triggerAt time.Time) (context.Context, turnToken) {
	if t == nil {
		return ctx, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return ctx, 0
	}
	if t.open && t.finished {
		t.settleLocked(ctx)
	}
	if t.open {
		return t.contextLocked(ctx), t.token
	}
	t.chatID = chat.ID.String()
	t.chatKind = chatKindAttr(chat)
	turnCtx := t.startLocked(ctx, triggerAt)
	// The window between the trigger message landing in history and a
	// worker picking the chat up is the acquisition. A turn opened by a
	// promotion has no acquisition: its head is the queue wait of the
	// message that opened it, and recording both would count that
	// window twice.
	t.stages.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{},
		triggerAt, t.stages.Now(), nil,
		attribute.String(chatloop.AttrChatID, t.chatID))
	return t.contextLocked(ctx), t.token
}

// startLocked opens a chat_turn span and returns the context parented
// to it.
func (t *runnerTurnSpan) startLocked(ctx context.Context, startAt time.Time) context.Context {
	t.token++
	t.open = true
	t.finished = false
	t.pendingPromotion = nil

	// The chat kind rides on the context so every stage of the turn
	// carries the kind.
	ctx = chatloop.ContextWithChatKind(ctx, t.chatKind)
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
	// The scope and chat kind are set independently of the span context
	// so stages run on this context keep them when tracing is not
	// recording.
	ctx = chatloop.ContextWithScope(ctx, chatloop.ScopeTurn)
	ctx = chatloop.ContextWithChatKind(ctx, t.chatKind)
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

// settleLocked closes the open, finished turn. When the finishing
// transition promoted a queued message, it opens the next turn anchored
// at the moment the message was queued and records that message's
// queue wait against the new turn.
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

// closeLocked ends the open turn span.
func (t *runnerTurnSpan) closeLocked(err error) {
	t.span.End(err)
	t.span = nil
	t.spanCtx = trace.SpanContext{}
	t.open = false
	t.finished = false
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
