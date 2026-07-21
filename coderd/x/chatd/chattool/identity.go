package chattool

import (
	"context"

	"github.com/google/uuid"
)

// ToolCallIdentity names one persisted tool call, so a tool that must
// run at most once per call can derive an idempotency key from it. The
// assistant message ID is what separates a regenerated call from the
// original, since a history edit re-inserts the message under a new
// row ID while the provider tool call ID may repeat.
type ToolCallIdentity struct {
	ChatID             uuid.UUID
	AssistantMessageID int64
	ToolCallID         string
}

type toolCallIdentityContextKey struct{}

// WithDispatchIdentity returns a context carrying the chat and
// assistant message a batch of tool calls belongs to. The dispatcher
// completes it per call with WithToolCallID.
func WithDispatchIdentity(ctx context.Context, chatID uuid.UUID, assistantMessageID int64) context.Context {
	return context.WithValue(ctx, toolCallIdentityContextKey{}, ToolCallIdentity{
		ChatID:             chatID,
		AssistantMessageID: assistantMessageID,
	})
}

// WithToolCallID completes the dispatch identity with the call being
// run. It is a no-op without a dispatch identity, since a tool call ID
// alone does not identify a persisted call.
func WithToolCallID(ctx context.Context, toolCallID string) context.Context {
	identity, ok := ctx.Value(toolCallIdentityContextKey{}).(ToolCallIdentity)
	if !ok {
		return ctx
	}
	identity.ToolCallID = toolCallID
	return context.WithValue(ctx, toolCallIdentityContextKey{}, identity)
}

// ToolCallIdentityFromContext returns the identity of the call being
// run. It reports false unless every component is present, because a
// partial identity would alias distinct calls onto one key.
func ToolCallIdentityFromContext(ctx context.Context) (ToolCallIdentity, bool) {
	identity, ok := ctx.Value(toolCallIdentityContextKey{}).(ToolCallIdentity)
	if !ok || identity.ChatID == uuid.Nil || identity.AssistantMessageID == 0 || identity.ToolCallID == "" {
		return ToolCallIdentity{}, false
	}
	return identity, true
}
