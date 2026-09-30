package workspacesdk

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// CoderToolCallIDHeader carries the chat tool call ID. The agent
// deduplicates requests by chat and tool call ID, and uses the ID as the
// process ID of a process it starts.
const CoderToolCallIDHeader = "Coder-Tool-Call-Id"

type toolCallIDContextKey struct{}

// WithToolCallID returns a context whose StartProcess, EditFiles, and
// WriteFile requests send id in CoderToolCallIDHeader. The ID goes on the
// context because parallel tool calls share one AgentConn.
func WithToolCallID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, toolCallIDContextKey{}, id)
}

// ToolCallIDFromContext returns the tool call ID set by WithToolCallID.
func ToolCallIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(toolCallIDContextKey{}).(uuid.UUID)
	return id, ok
}

// CancelToolCallResponse is the response to a tool call cancel.
type CancelToolCallResponse struct {
	// Received is false if the agent has no record of the tool call: it
	// never received it, restarted, or expired the record.
	Received bool `json:"received"`
	// Status, ContentType, and Body are the saved response, set if
	// Received.
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
