package intercept

import (
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Interceptor describes a (potentially) stateful interaction with an AI provider.
type Interceptor interface {
	// ID returns the unique identifier for this interception.
	ID() uuid.UUID
	// Setup injects some required dependencies. This MUST be called before using the interceptor
	// to process requests.
	Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier)
	// Model returns the model in use for this [Interceptor].
	Model() string
	// ProcessRequest handles the HTTP request.
	ProcessRequest(w http.ResponseWriter, r *http.Request) error
	// Specifies whether an interceptor handles streaming or not.
	Streaming() bool
	// TraceAttributes returns tracing attributes for this [Interceptor]
	TraceAttributes(*http.Request) []attribute.KeyValue
	// Credential returns the credential metadata for this interception.
	Credential() CredentialInfo
	// CorrelatingToolCallID returns the ID of a tool call result submitted
	// in the request, if present. This is used to correlate the current
	// interception back to the previous interception that issued those tool
	// calls. If multiple tool use results are present, we use the last one
	// (most recent). Both Anthropic's /v1/messages and OpenAI's /v1/responses
	// require that ALL tool results are submitted for tool choices returned
	// by the model, so any single tool call ID is sufficient to identify the
	// parent interception.
	CorrelatingToolCallID() *string
}
