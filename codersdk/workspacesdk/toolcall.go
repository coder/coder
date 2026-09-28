package workspacesdk

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// CoderToolCallIDHeader carries the chat tool call ID. The agent runs a
// request with this header at most once per chat and tool call ID, and
// a process started with it uses the tool call ID as its process ID.
const CoderToolCallIDHeader = "Coder-Tool-Call-Id"

type toolCallIDContextKey struct{}

// WithToolCallID returns a context whose StartProcess, EditFiles, and
// WriteFile requests carry id in CoderToolCallIDHeader. It is per
// request because parallel tool calls share one AgentConn.
func WithToolCallID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, toolCallIDContextKey{}, id)
}

// ToolCallIDFromContext returns the tool call ID set by WithToolCallID.
func ToolCallIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(toolCallIDContextKey{}).(uuid.UUID)
	return id, ok
}

// CancelToolCallResponse is the answer to a tool call cancel.
type CancelToolCallResponse struct {
	// Received is false when the agent never received a request for the
	// tool call; the agent now refuses it.
	Received bool `json:"received"`
	// Status, ContentType, and Body are the saved response of the tool
	// call's request when Received is true.
	Status      int    `json:"status,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

// StartProcessResult decodes a saved start response as StartProcess
// does.
func (r CancelToolCallResponse) StartProcessResult() (StartProcessResponse, error) {
	return readStartProcessResponse(r.httpResponse()) //nolint:bodyclose // In-memory body.
}

// EditFilesResult decodes a saved edit response as EditFiles does.
func (r CancelToolCallResponse) EditFilesResult() (FileEditResponse, error) {
	return readEditFilesResponse(r.httpResponse()) //nolint:bodyclose // In-memory body.
}

// WriteFileResult decodes a saved write response as WriteFile does.
func (r CancelToolCallResponse) WriteFileResult() error {
	return readWriteFileResponse(r.httpResponse()) //nolint:bodyclose // In-memory body.
}

func (r CancelToolCallResponse) httpResponse() *http.Response {
	return &http.Response{
		StatusCode: r.Status,
		Header:     http.Header{"Content-Type": {r.ContentType}},
		Body:       io.NopCloser(bytes.NewReader(r.Body)),
	}
}
