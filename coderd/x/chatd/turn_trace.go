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
// the running states, so the runner's lifetime bounds the turn.
//
// The span is created on the first generation task rather than at
// runner construction: runners are also spawned to abandon or time
// out chats, which run no turn.
type runnerTurnSpan struct {
	stages *chatloop.StageTracer

	mu      sync.Mutex
	span    *chatloop.StageSpan
	spanCtx trace.SpanContext
	started bool
	ended   bool
}

func newRunnerTurnSpan(stages *chatloop.StageTracer) *runnerTurnSpan {
	return &runnerTurnSpan{stages: stages}
}

// Ensure starts the chat_turn span on first call and returns a
// context parented to it. triggerAt is the trigger message's
// creation time and becomes the span's start timestamp, so the
// acquisition stage reconstructed from the same instant falls inside
// the turn. The window between triggerAt and now is emitted as the
// acquisition stage, which covers the delay between the message
// landing in history and a worker picking the chat up.
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
	if t.started {
		return t.contextLocked(ctx)
	}
	t.started = true

	attrs := []attribute.KeyValue{
		attribute.String(chatloop.AttrChatID, chat.ID.String()),
		attribute.String(chatloop.AttrChatKind, chatKindAttr(chat)),
	}
	turnCtx, span := t.stages.StartRootAt(ctx, chatloop.StageChatTurn, triggerAt, nil, attrs...)
	t.span = span
	t.spanCtx = span.SpanContext()
	// The turn's model is not resolved until preparation runs, so the
	// acquisition stage carries no model identity.
	t.stages.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{},
		triggerAt, time.Now(), nil, attrs...)
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
	if !t.started || t.ended || !t.spanCtx.IsValid() {
		return ctx
	}
	return trace.ContextWithSpanContext(ctx, t.spanCtx)
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
	t.span.End(err)
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
