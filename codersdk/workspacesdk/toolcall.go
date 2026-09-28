package workspacesdk

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	// CoderToolCallNameHeader carries ToolCall.Name escaped with
	// url.PathEscape.
	CoderToolCallNameHeader = "Coder-Tool-Call-Name"
)

// ToolCall identifies the chat tool call a workspace agent request acts
// for. Together with the chat ID from CoderChatIDHeader it names one
// tool call.
type ToolCall struct {
	// MessageID is the chat message ID of the assistant message that
	// contains the tool call.
	MessageID int64
	// ID is the provider tool call ID, raw (unescaped).
	ID string
	// Name is the tool name, raw (unescaped).
	Name string
}

// SetHeaders sets the tool call headers on h, replacing any earlier
// values.
func (tc ToolCall) SetHeaders(h http.Header) {
	h.Set(CoderToolCallMessageIDHeader, strconv.FormatInt(tc.MessageID, 10))
	h.Set(CoderToolCallIDHeader, url.PathEscape(tc.ID))
	h.Set(CoderToolCallNameHeader, url.PathEscape(tc.Name))
}

// ToolCallFromHeaders parses the tool call headers from h. ok is false
// when none of the headers is present. An error is returned when only
// some are present, a header has more than one value, or a value is
// malformed.
func ToolCallFromHeaders(h http.Header) (tc ToolCall, ok bool, err error) {
	names := []string{CoderToolCallMessageIDHeader, CoderToolCallIDHeader, CoderToolCallNameHeader}
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
	id, err := unescapeNonEmpty(CoderToolCallIDHeader, h.Get(CoderToolCallIDHeader))
	if err != nil {
		return ToolCall{}, false, err
	}
	name, err := unescapeNonEmpty(CoderToolCallNameHeader, h.Get(CoderToolCallNameHeader))
	if err != nil {
		return ToolCall{}, false, err
	}

	return ToolCall{
		MessageID: messageID,
		ID:        id,
		Name:      name,
	}, true, nil
}

func unescapeNonEmpty(header, value string) (string, error) {
	s, err := url.PathUnescape(value)
	if err != nil {
		return "", xerrors.Errorf("invalid %s header: %w", header, err)
	}
	if s == "" {
		return "", xerrors.Errorf("invalid %s header: must not be empty", header)
	}
	return s, nil
}

// ToolCallUUIDNamespace is the UUIDv5 namespace of ToolCallUUID. It is
// uuid5(NAMESPACE_URL, "https://coder.com/workspace-agent/tool-call").
var ToolCallUUIDNamespace = uuid.MustParse("c5773f4c-0b3f-57bc-9147-36daf7921a1b")

// ToolCallUUID returns the tool call UUID: the process ID of a process
// the tool call starts, and the ID in the cancel route. chatd and the
// workspace agent both derive it, so the encoding must not change:
// UUIDv5 in ToolCallUUIDNamespace of the 16 chat ID bytes, the
// big-endian uint64 message ID, the big-endian uint64 byte length of
// the tool name, the tool name bytes, and the raw provider tool call ID
// bytes. The length prefix keeps the tool name and tool call ID from
// running into each other.
func ToolCallUUID(chatID uuid.UUID, messageID int64, toolName, toolCallID string) uuid.UUID {
	name := make([]byte, 0, len(chatID)+8+8+len(toolName)+len(toolCallID))
	name = append(name, chatID[:]...)
	name = binary.BigEndian.AppendUint64(name, uint64(messageID)) //nolint:gosec // Two's complement bytes are the defined encoding.
	name = binary.BigEndian.AppendUint64(name, uint64(len(toolName)))
	name = append(name, toolName...)
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
	// ToolCallErrorCanceled means a cancel recorded the tool call as
	// canceled before the agent received its request, so the agent
	// never ran it and never will.
	ToolCallErrorCanceled ToolCallErrorCode = "tool_call_canceled"
	// ToolCallErrorUnknown means the agent has no record of the tool
	// call and its message is at or below the agent's cutoff message
	// ID, so the agent cannot tell whether the tool call ran.
	ToolCallErrorUnknown ToolCallErrorCode = "tool_call_unknown"
)

func (c ToolCallErrorCode) known() bool {
	switch c {
	case ToolCallErrorCanceled, ToolCallErrorUnknown:
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
