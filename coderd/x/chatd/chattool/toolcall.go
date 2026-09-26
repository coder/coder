package chattool

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
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

// AgentErrorKind classifies an error from a workspace agent request made
// for a tool call.
type AgentErrorKind int

const (
	// AgentErrorRefused is a *workspacesdk.ToolCallError: the agent
	// refused the tool call and did nothing.
	AgentErrorRefused AgentErrorKind = iota + 1
	// AgentErrorResponse is any other HTTP error answer from the agent
	// (*codersdk.Error).
	AgentErrorResponse
	// AgentErrorUnreachable means no answer arrived. The request may
	// still have reached the agent.
	AgentErrorUnreachable
	// AgentErrorUnreadable means the agent answered, but the answer
	// could not be read, so it may have acted.
	AgentErrorUnreadable
)

// ClassifyAgentError classifies err, returned by a workspace agent
// request made for a tool call. code is set for AgentErrorRefused.
func ClassifyAgentError(err error) (kind AgentErrorKind, code workspacesdk.ToolCallErrorCode) {
	var tcErr *workspacesdk.ToolCallError
	if errors.As(err, &tcErr) {
		return AgentErrorRefused, tcErr.Code
	}
	var sdkErr *codersdk.Error
	if errors.As(err, &sdkErr) {
		return AgentErrorResponse, ""
	}
	// http.Client.Do returns every transport failure as a *url.Error.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return AgentErrorUnreachable, ""
	}
	return AgentErrorUnreadable, ""
}

// AgentRestartedReason is why the outcome of a tool call refused with
// workspacesdk.ToolCallErrorAgentStartedAfterToolCall is unknown.
const AgentRestartedReason = "the workspace agent restarted after this tool call"

// AgentUnreachableReason is why the outcome of a tool call is unknown
// when the workspace agent could not be reached.
func AgentUnreachableReason(err error) string {
	return fmt.Sprintf("the workspace agent could not be reached (%v)", err)
}

// AgentUnreadableReason is why the outcome of a tool call is unknown
// when the workspace agent's answer could not be read.
func AgentUnreadableReason(err error) string {
	return fmt.Sprintf("the workspace agent's answer could not be read (%v)", err)
}

// UnknownOutcome words a tool call result whose side effect may or may
// not have happened: reason is one of the reasons above, effect says
// what may have happened, and check how the model can find out.
func UnknownOutcome(reason, effect, check string) string {
	return fmt.Sprintf("outcome unknown: %s, so %s. %s", reason, effect, check)
}
