package chattool

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

// AgentToolCall returns the tool call to attach to a workspace agent
// request with workspacesdk.WithToolCall. Its age is measured now, so
// build it immediately before the request.
func (id ToolCallIdentity) AgentToolCall() workspacesdk.ToolCall {
	return workspacesdk.ToolCall{
		MessageID: id.MessageID,
		ID:        id.ToolCallID,
		Age:       id.Age.Now(),
	}
}

// UUID returns the tool call UUID. The workspace agent uses it as the ID
// of a process started for this tool call; an agent without tool call
// support picks another.
func (id ToolCallIdentity) UUID() string {
	return workspacesdk.ToolCallUUID(id.ChatID, id.MessageID, id.ToolCallID).String()
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
