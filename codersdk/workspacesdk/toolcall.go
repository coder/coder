package workspacesdk

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const (
	// CoderToolCallMessageIDHeader carries ToolCall.MessageID as a
	// decimal integer greater than zero.
	CoderToolCallMessageIDHeader = "Coder-Tool-Call-Message-Id"
	// CoderToolCallIDHeader carries ToolCall.ID escaped with
	// url.PathEscape, so any provider tool call ID is a valid header
	// value.
	CoderToolCallIDHeader = "Coder-Tool-Call-Id"
	// CoderToolCallAgeMsHeader carries ToolCall.Age as a non-negative
	// decimal number of milliseconds.
	CoderToolCallAgeMsHeader = "Coder-Tool-Call-Age-Ms"
)

// ToolCall identifies the chat tool call a workspace agent request acts
// for.
type ToolCall struct {
	// MessageID is the chat message ID of the assistant message that
	// contains the tool call.
	MessageID int64
	// ID is the provider tool call ID, raw (unescaped).
	ID string
	// Age is the time since the tool call was committed, measured when
	// the request is sent.
	Age time.Duration
}

// SetHeaders sets the tool call headers on h. Age is truncated to whole
// milliseconds, and a negative Age is sent as zero.
func (tc ToolCall) SetHeaders(h http.Header) {
	h.Set(CoderToolCallMessageIDHeader, strconv.FormatInt(tc.MessageID, 10))
	h.Set(CoderToolCallIDHeader, url.PathEscape(tc.ID))
	h.Set(CoderToolCallAgeMsHeader, strconv.FormatInt(max(tc.Age, 0).Milliseconds(), 10))
}

// ToolCallFromHeaders parses the tool call headers from h. ok is false
// when none of the three headers is present. An error is returned when
// only some are present, a header has more than one value, or a value
// is malformed.
func ToolCallFromHeaders(h http.Header) (tc ToolCall, ok bool, err error) {
	names := []string{CoderToolCallMessageIDHeader, CoderToolCallIDHeader, CoderToolCallAgeMsHeader}
	var missing []string
	for _, name := range names {
		switch len(h.Values(name)) {
		case 0:
			missing = append(missing, name)
		case 1:
		default:
			return ToolCall{}, false, xerrors.Errorf("invalid %s header: multiple values", name)
		}
	}
	switch len(missing) {
	case len(names):
		return ToolCall{}, false, nil
	case 0:
	default:
		return ToolCall{}, false, xerrors.Errorf("incomplete tool call headers: missing %s", strings.Join(missing, ", "))
	}

	messageID, err := strconv.ParseInt(h.Get(CoderToolCallMessageIDHeader), 10, 64)
	if err != nil || messageID <= 0 {
		return ToolCall{}, false, xerrors.Errorf("invalid %s header %q: must be an integer greater than 0", CoderToolCallMessageIDHeader, h.Get(CoderToolCallMessageIDHeader))
	}
	id, err := url.PathUnescape(h.Get(CoderToolCallIDHeader))
	if err != nil {
		return ToolCall{}, false, xerrors.Errorf("invalid %s header: %w", CoderToolCallIDHeader, err)
	}
	if id == "" {
		return ToolCall{}, false, xerrors.Errorf("invalid %s header: must not be empty", CoderToolCallIDHeader)
	}
	ageMs, err := strconv.ParseInt(h.Get(CoderToolCallAgeMsHeader), 10, 64)
	// The upper bound keeps the conversion to time.Duration from
	// overflowing.
	if err != nil || ageMs < 0 || ageMs > math.MaxInt64/int64(time.Millisecond) {
		return ToolCall{}, false, xerrors.Errorf("invalid %s header %q: must be a non-negative integer number of milliseconds", CoderToolCallAgeMsHeader, h.Get(CoderToolCallAgeMsHeader))
	}

	return ToolCall{
		MessageID: messageID,
		ID:        id,
		Age:       time.Duration(ageMs) * time.Millisecond,
	}, true, nil
}

// ToolCallUUIDNamespace is the UUIDv5 namespace of ToolCallUUID. It is
// uuid5(NAMESPACE_URL, "https://coder.com/workspace-agent/tool-call").
var ToolCallUUIDNamespace = uuid.MustParse("c5773f4c-0b3f-57bc-9147-36daf7921a1b")

// ToolCallUUID returns the process ID, and the ID in cancel routes, for
// a tool call. chatd and the workspace agent both derive it, so the
// encoding must not change: UUIDv5 in ToolCallUUIDNamespace of the 16
// chat ID bytes, the big-endian uint64 message ID, and the raw provider
// tool call ID bytes.
func ToolCallUUID(chatID uuid.UUID, messageID int64, toolCallID string) uuid.UUID {
	name := make([]byte, 0, len(chatID)+8+len(toolCallID))
	name = append(name, chatID[:]...)
	name = binary.BigEndian.AppendUint64(name, uint64(messageID)) //nolint:gosec // Two's complement bytes are the defined encoding.
	name = append(name, toolCallID...)
	return uuid.NewSHA1(ToolCallUUIDNamespace, name)
}

type toolCallContextKey struct{}

// WithToolCall returns a context whose agent requests carry tc's
// headers. agentConn.apiRequest applies them per request because
// parallel tool calls share one AgentConn, so connection-wide
// SetExtraHeaders cannot be used.
func WithToolCall(ctx context.Context, tc ToolCall) context.Context {
	return context.WithValue(ctx, toolCallContextKey{}, tc)
}

// ToolCallFromContext returns the ToolCall set by WithToolCall.
func ToolCallFromContext(ctx context.Context) (ToolCall, bool) {
	tc, ok := ctx.Value(toolCallContextKey{}).(ToolCall)
	return tc, ok
}

// ToolCallErrorCode identifies why the workspace agent refused a
// request with tool call headers.
type ToolCallErrorCode string

const (
	// ToolCallErrorStale means the tool call's message is older than
	// the chat's latest message ID, so chatd has already resolved it.
	ToolCallErrorStale ToolCallErrorCode = "stale_tool_call"
	// ToolCallErrorAgentStartedAfterToolCall means the agent has no
	// record of the tool call and started after it was committed, so
	// an earlier agent may have run it.
	ToolCallErrorAgentStartedAfterToolCall ToolCallErrorCode = "agent_started_after_tool_call"
	// ToolCallErrorInputMismatch means the agent has a record of the
	// tool call with a different input.
	ToolCallErrorInputMismatch ToolCallErrorCode = "input_mismatch"
	// ToolCallErrorCanceled means a request with tool call headers
	// arrived after a cancel recorded the tool call as canceled.
	ToolCallErrorCanceled ToolCallErrorCode = "tool_call_canceled"
)

func (c ToolCallErrorCode) known() bool {
	switch c {
	case ToolCallErrorStale, ToolCallErrorAgentStartedAfterToolCall, ToolCallErrorInputMismatch, ToolCallErrorCanceled:
		return true
	default:
		return false
	}
}

// ToolCallError is the body of an HTTP 409 from a request with tool
// call headers. Only a 409 whose body has a known Code is a
// ToolCallError; any other error response stays a *codersdk.Error.
type ToolCallError struct {
	codersdk.Response
	Code ToolCallErrorCode `json:"code"`
}

func (e *ToolCallError) Error() string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "tool call error %s: %s", e.Code, e.Message)
	if e.Detail != "" {
		_, _ = fmt.Fprintf(&builder, "\n\tError: %s", e.Detail)
	}
	for _, err := range e.Validations {
		_, _ = fmt.Fprintf(&builder, "\n\t%s: %s", err.Field, err.Detail)
	}
	return builder.String()
}
