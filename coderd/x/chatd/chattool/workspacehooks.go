package chattool

import (
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// PROTOTYPE (CODAGT-1083). Workspace hooks run on the agent inside the
// tool request, and workspacetools turns their decisions into results.
// This file carries the decisions back through the chat tool response
// without putting model-only content in model-visible text.

// workspaceHookMetadata is the client metadata shape a workspace tool
// attaches to its response. It rides in ToolResultContent.ClientMetadata,
// which is persisted with the step and never sent to the model.
type workspaceHookMetadata struct {
	WorkspaceHooks []workspacesdk.HookDecision `json:"workspace_hooks,omitempty"`
}

// WorkspaceHooksFromMetadata decodes the hook decisions a workspace tool
// attached to its response. Metadata without the key yields nil.
func WorkspaceHooksFromMetadata(metadata string) ([]workspacesdk.HookDecision, error) {
	if strings.TrimSpace(metadata) == "" {
		return nil, nil
	}
	var decoded workspaceHookMetadata
	if err := json.Unmarshal([]byte(metadata), &decoded); err != nil {
		return nil, xerrors.Errorf("unmarshal workspace hook metadata: %w", err)
	}
	return decoded.WorkspaceHooks, nil
}

// withWorkspaceHooks attaches hook decisions to a response. Responses
// from these tools carry no other client metadata, so replacing it is
// safe.
func withWorkspaceHooks(resp fantasy.ToolResponse, hooks []workspacesdk.HookDecision) fantasy.ToolResponse {
	if len(hooks) == 0 {
		return resp
	}
	return fantasy.WithResponseMetadata(resp, workspaceHookMetadata{WorkspaceHooks: hooks})
}

// workspaceToolErrorResponse renders a workspace tool error with the
// hook decisions that ran: those of a denial, or else resultHooks.
func workspaceToolErrorResponse(message string, err error, resultHooks []workspacesdk.HookDecision) fantasy.ToolResponse {
	hooks := workspacetools.DeniedHooks(err)
	if hooks == nil {
		hooks = resultHooks
	}
	return withWorkspaceHooks(fantasy.NewTextErrorResponse(message), hooks)
}
