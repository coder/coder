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
type runnerTurnSpan struct {
	stages *chatloop.StageTracer

	mu       sync.Mutex
	span     *chatloop.StageSpan
	spanCtx  trace.SpanContext
	acc      *chatloop.TurnAccumulator
	chatID   string
	chatKind string
	started  bool
	ended    bool
	// finished marks a turn that reached a terminal transition, so a
	// further prompt opens a new span instead of extending this one.
	finished bool
}

func newRunnerTurnSpan(stages *chatloop.StageTracer) *runnerTurnSpan {
	return &runnerTurnSpan{stages: stages}
}

// Ensure starts the chat_turn span on first call and returns a
// context parented to it. triggerAt is the trigger message's
// creation time and becomes the span's start timestamp, so the
// acquisition stage reconstructed from the same instant falls inside
// the turn. The acquisition stage carries no model identity: the
// turn's model is not resolved until preparation runs.
//
// A turn that already reached a terminal transition is replaced: the
// prompt this call runs is a new turn, and folding it into the old
// span would report the two as one.
//
// The span is a standalone trace root. The request that triggered the
// turn is handled by a different goroutine, and often a different
// replica, than the worker that runs it, so no inbound span context
// is available here to link.
func (t *runnerTurnSpan) Ensure(ctx context.Context, chat database.Chat, triggerAt time.Time) context.Context {
	if t == nil {
		return ctx
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return ctx
	}
	if t.started && !t.finished {
		return t.contextLocked(ctx)
	}
	if t.started {
		t.closeLocked(nil)
	}
	t.chatID = chat.ID.String()
	t.chatKind = chatKindAttr(chat)
	turnCtx := t.startLocked(ctx, triggerAt)
	// The window between the trigger message landing in history and a
	// worker picking the chat up is the acquisition. A rotated turn has
	// no acquisition: its head is the queue wait of the message that
	// opened it, and recording both would count that window twice.
	t.stages.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{},
		triggerAt, time.Now(), nil,
		attribute.String(chatloop.AttrChatID, t.chatID))
	return t.contextLocked(ctx)
}

// startLocked opens a chat_turn span with a fresh accumulator and
// returns the context parented to it.
func (t *runnerTurnSpan) startLocked(ctx context.Context, startAt time.Time) context.Context {
	t.started = true
	t.finished = false
	t.acc = chatloop.NewTurnAccumulator()

	// The chat kind and the accumulator ride on the context so every
	// stage of the turn carries the kind and reports its time to the
	// turn that contains it.
	ctx = chatloop.ContextWithChatKind(ctx, t.chatKind)
	ctx = chatloop.ContextWithTurnAccumulator(ctx, t.acc)
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
	if !t.started || t.ended {
		return ctx
	}
	// The scope, chat kind, and accumulator are set independently of
	// the span context so stages run on this context keep them when
	// tracing is not recording.
	ctx = chatloop.ContextWithScope(ctx, chatloop.ScopeTurn)
	ctx = chatloop.ContextWithChatKind(ctx, t.chatKind)
	ctx = chatloop.ContextWithTurnAccumulator(ctx, t.acc)
	if !t.spanCtx.IsValid() {
		return ctx
	}
	return trace.ContextWithSpanContext(ctx, t.spanCtx)
}

// Complete marks the turn as finished normally, which is what makes
// its accounting emittable when the span closes.
func (t *runnerTurnSpan) Complete() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	t.acc.MarkCompleted()
}

// Invalidate drops the turn's accounting. A turn that errored or was
// interrupted stops partway through its stages, so its totals do not
// describe a full turn.
func (t *runnerTurnSpan) Invalidate() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	t.acc.Invalidate()
}

// Rotate closes the current turn and opens the next one, anchored at
// startAt. It is for a queued message promoted by the transition that
// finished the previous turn: the wait that message served and the
// work it causes belong to the turn it opens, not to the one that
// released it.
func (t *runnerTurnSpan) Rotate(ctx context.Context, startAt time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started || t.ended {
		return
	}
	t.closeLocked(nil)
	t.startLocked(ctx, startAt)
}

// End closes the chat_turn span. Later calls are ignored.
func (t *runnerTurnSpan) End(err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended || !t.started {
		t.ended = true
		return
	}
	t.ended = true
	t.closeLocked(err)
}

// closeLocked ends the open turn span. A non-nil error also drops the
// turn's accounting, because the stages it collected stop where the
// error happened.
func (t *runnerTurnSpan) closeLocked(err error) {
	if err != nil {
		t.acc.Invalidate()
	}
	t.span.End(err)
	t.span = nil
	t.spanCtx = trace.SpanContext{}
	t.acc = nil
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
