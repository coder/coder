package workspacesdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// PROTOTYPE (CODAGT-1083). Transport for workspace-declared hooks that
// run on the agent inside a tool request. Shapes are not final.

const (
	// CoderToolCallIDHeader carries the model's tool call ID so the agent
	// can report it to hooks as tool_use_id.
	CoderToolCallIDHeader = "Coder-Tool-Call-Id"
	// CoderWorkspaceHookHeader is set to "deny" on a response whose body
	// is a HookDeniedResponse, so clients can tell a policy denial from
	// any other 403.
	CoderWorkspaceHookHeader = "Coder-Workspace-Hook"
	// HookDecisionDeny is the decision value on a denying HookDecision.
	HookDecisionDeny = "deny"
)

// HookDecision records one workspace hook run for a tool call.
type HookDecision struct {
	Hook          string          `json:"hook"`
	Event         string          `json:"event"`
	Decision      string          `json:"decision"`
	Reason        string          `json:"reason,omitempty"`
	ModelContext  string          `json:"model_context,omitempty"`
	UserMessage   string          `json:"user_message,omitempty"`
	InputOverride json.RawMessage `json:"input_override,omitempty"`
	DurationMs    int64           `json:"duration_ms"`
	// Error carries the failure detail when a hook failed closed.
	Error string `json:"error,omitempty"`
}

// HookDeniedResponse is the body of a 403 the agent returns when a
// workspace hook denies a tool call. The tool did not run.
type HookDeniedResponse struct {
	Message string         `json:"message"`
	Hooks   []HookDecision `json:"hooks"`
}

// HookDeniedError is returned by agent tool methods when a workspace
// hook denied the call. Callers must present it as a policy decision,
// not a tool failure.
type HookDeniedError struct {
	Hooks []HookDecision
}

func (e *HookDeniedError) Error() string {
	d := e.Denied()
	msg := "workspace hook denied the tool call"
	if d.Hook != "" {
		msg += fmt.Sprintf(" (%q)", d.Hook)
	}
	if reason := strings.TrimSpace(d.Reason); reason != "" {
		msg += ": " + reason
	}
	return msg
}

// Denied returns the decision that denied the call.
func (e *HookDeniedError) Denied() HookDecision {
	for _, h := range e.Hooks {
		if h.Decision == HookDecisionDeny {
			return h
		}
	}
	return HookDecision{}
}

// readAgentError converts a non-200 agent response into an error. A
// hook denial becomes a *HookDeniedError; anything else keeps the
// existing codersdk.Error shape.
func readAgentError(res *http.Response) error {
	if res.StatusCode == http.StatusForbidden && res.Header.Get(CoderWorkspaceHookHeader) == HookDecisionDeny {
		var body HookDeniedResponse
		if err := decodeAgentJSON(res, &body); err == nil && len(body.Hooks) > 0 {
			return &HookDeniedError{Hooks: body.Hooks}
		}
	}
	return codersdk.ReadBodyAsError(res)
}

type toolCallIDKey struct{}

// WithToolCallID tags ctx with the model's tool call ID. Agent API
// requests made with this ctx send it as CoderToolCallIDHeader.
func WithToolCallID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, toolCallIDKey{}, id)
}

func toolCallIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(toolCallIDKey{}).(string)
	return id
}
