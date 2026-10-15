package mcp

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// ToolCallResult classifies the outcome of an MCP tool call.
type ToolCallResult string

const (
	// ToolCallResultSuccess means the tool returned a result.
	ToolCallResultSuccess ToolCallResult = "success"
	// ToolCallResultToolError means the tool failed for a reason other
	// than invalid arguments or missing authorization.
	ToolCallResultToolError ToolCallResult = "tool_error"
	// ToolCallResultValidationError means the tool rejected its arguments.
	ToolCallResultValidationError ToolCallResult = "validation_error"
	// ToolCallResultAuthzError means the caller was not authenticated or
	// not authorized to perform the operation.
	ToolCallResultAuthzError ToolCallResult = "authz_error"
)

// Normalized client names. Clients are reduced to this fixed set so that
// metric and telemetry cardinality stays bounded regardless of what a
// client reports about itself.
const (
	ClientClaudeCode = "claude-code"
	ClientCursor     = "cursor"
	ClientVSCode     = "vscode"
	ClientChatGPT    = "chatgpt"
	ClientOther      = "other"
)

// ToolOther is the tool label used for any tool name that is not part of
// toolsdk.All.
const ToolOther = "other"

// ToolCall describes a completed MCP tool call. It intentionally carries
// no arguments, results, or user identifiers.
type ToolCall struct {
	// Tool is the normalized tool name, see NormalizeToolName.
	Tool string
	// Client is the normalized client name, see NormalizeClientName.
	Client   string
	Result   ToolCallResult
	Duration time.Duration
}

// ToolCallRecorder observes completed MCP tool calls. Implementations
// must be safe for concurrent use.
type ToolCallRecorder interface {
	RecordToolCall(call ToolCall)
}

var knownToolNames = func() map[string]struct{} {
	names := make(map[string]struct{}, len(toolsdk.All))
	for _, tool := range toolsdk.All {
		names[tool.Name] = struct{}{}
	}
	return names
}()

// NormalizeToolName returns name if it is a tool in toolsdk.All, and
// ToolOther otherwise.
func NormalizeToolName(name string) string {
	if _, ok := knownToolNames[name]; ok {
		return name
	}
	return ToolOther
}

// NormalizeClientName maps a client identifier, such as the MCP
// clientInfo.name or an HTTP User-Agent, to one of the Client* constants.
func NormalizeClientName(name string) string {
	name = strings.ToLower(name)
	switch {
	case name == "":
		return ClientOther
	case strings.Contains(name, "claude-code"), strings.Contains(name, "claude code"):
		return ClientClaudeCode
	// Cursor identifies itself as "cursor-vscode", so it must be matched
	// before VS Code.
	case strings.Contains(name, "cursor"):
		return ClientCursor
	case strings.Contains(name, "visual studio code"),
		strings.Contains(name, "visual-studio-code"),
		strings.Contains(name, "vscode"):
		return ClientVSCode
	case strings.Contains(name, "chatgpt"), strings.Contains(name, "openai"):
		return ClientChatGPT
	default:
		return ClientOther
	}
}

// ClassifyToolError returns the result class for an error returned by a
// toolsdk tool handler.
func ClassifyToolError(err error) ToolCallResult {
	if err == nil {
		return ToolCallResultSuccess
	}
	if validationErr := new(toolsdk.ArgumentValidationError); errors.As(err, &validationErr) {
		return ToolCallResultValidationError
	}
	if authzErr := new(authzError); errors.As(err, &authzErr) {
		return ToolCallResultAuthzError
	}
	if sdkErr, ok := errors.AsType[*codersdk.Error](err); ok {
		switch sdkErr.StatusCode() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return ToolCallResultAuthzError
		}
	}
	return ToolCallResultToolError
}

// authzError marks an authorization failure that does not come from a
// coderd API response.
type authzError struct {
	message string
}

func (e *authzError) Error() string {
	return e.message
}

// NewAuthzError returns an error with the given message that
// ClassifyToolError reports as ToolCallResultAuthzError.
func NewAuthzError(message string) error {
	return &authzError{message: message}
}

// toolCallClient returns the normalized client of a tool call request. The
// MCP clientInfo is preferred. Stateless HTTP requests on older protocol
// versions carry no clientInfo, so the User-Agent header is the fallback.
func toolCallClient(req *mcp.CallToolRequest) string {
	if info := req.ClientInfo(); info != nil && info.Name != "" {
		return NormalizeClientName(info.Name)
	}
	if req.Extra != nil && req.Extra.Header != nil {
		return NormalizeClientName(req.Extra.Header.Get("User-Agent"))
	}
	return ClientOther
}

// recordToolCall reports a completed call to recorder, if any.
func recordToolCall(recorder ToolCallRecorder, req *mcp.CallToolRequest, tool string, start time.Time, err error) {
	if recorder == nil {
		return
	}
	recorder.RecordToolCall(ToolCall{
		Tool:     NormalizeToolName(tool),
		Client:   toolCallClient(req),
		Result:   ClassifyToolError(err),
		Duration: time.Since(start),
	})
}

// ToolCallRecorders fans each tool call out to every recorder.
type ToolCallRecorders []ToolCallRecorder

// RecordToolCall implements ToolCallRecorder.
func (rs ToolCallRecorders) RecordToolCall(call ToolCall) {
	for _, r := range rs {
		r.RecordToolCall(call)
	}
}

// TelemetryRecorder aggregates tool calls into deployment telemetry.
type TelemetryRecorder struct {
	Usage *telemetry.MCPToolUsage
	// Transport is telemetry.MCPTransportRemote or
	// telemetry.MCPTransportStdio.
	Transport string
}

// RecordToolCall implements ToolCallRecorder.
func (r TelemetryRecorder) RecordToolCall(call ToolCall) {
	r.Usage.Add(telemetry.MCPToolUsageKey{
		Transport: r.Transport,
		Tool:      call.Tool,
		Client:    call.Client,
	}, ToolCallCounts(call.Result))
}

// ToolCallCounts returns the telemetry counts for a single call with the
// given result.
func ToolCallCounts(result ToolCallResult) telemetry.MCPToolUsageCounts {
	counts := telemetry.MCPToolUsageCounts{Calls: 1}
	switch result {
	case ToolCallResultToolError:
		counts.ToolErrors = 1
	case ToolCallResultValidationError:
		counts.ValidationErrors = 1
	case ToolCallResultAuthzError:
		counts.AuthzErrors = 1
	}
	return counts
}
