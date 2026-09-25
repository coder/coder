package chattool

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/quartz"
)

// ToolCallAge measures time since a tool call was committed without
// comparing coderd's clock with the database's. The age at the
// database read comes from database timestamps only, and time since
// that read comes from coderd's monotonic clock.
type ToolCallAge struct {
	atRead time.Duration
	readAt time.Time
	clock  quartz.Clock
}

// NewToolCallAge returns the age of a tool call committed at
// committedAt, where dbNow is the database clock read just before this
// call and committedAt is the assistant message's database timestamp.
func NewToolCallAge(clock quartz.Clock, dbNow, committedAt time.Time) ToolCallAge {
	return ToolCallAge{
		atRead: dbNow.Sub(committedAt),
		readAt: clock.Now(),
		clock:  clock,
	}
}

// Now returns the current tool call age. It is never negative. The
// zero value reports zero.
func (a ToolCallAge) Now() time.Duration {
	if a.clock == nil {
		return 0
	}
	return max(a.atRead+a.clock.Since(a.readAt), 0)
}

// ToolCallIdentity identifies the committed tool call a tool is running
// for: the chat, the assistant message that contains the call, and the
// provider tool call ID.
type ToolCallIdentity struct {
	ChatID     uuid.UUID
	MessageID  int64
	ToolCallID string
	Age        ToolCallAge
}

type toolCallIdentityKey struct{}

// WithToolCallIdentity returns a context carrying id for the tool call
// run with it.
func WithToolCallIdentity(ctx context.Context, id ToolCallIdentity) context.Context {
	return context.WithValue(ctx, toolCallIdentityKey{}, id)
}

// ToolCallIdentityFromContext returns the identity set by
// WithToolCallIdentity. ok is false when the tool call has none, for
// example outside a chat generation.
func ToolCallIdentityFromContext(ctx context.Context) (ToolCallIdentity, bool) {
	id, ok := ctx.Value(toolCallIdentityKey{}).(ToolCallIdentity)
	return id, ok
}
