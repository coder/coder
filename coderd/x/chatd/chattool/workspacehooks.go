package chattool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// PROTOTYPE (CODAGT-1083). Workspace hooks run on the agent inside the
// tool request. This file carries their decisions back through the
// tool result without putting model-only content in model-visible
// text.

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

// workspaceHookDenial extracts a hook denial from an agent error. It
// returns false when err is not a denial. The message mirrors the
// deployment hook denial so the model reads it as a policy decision,
// not a tool failure, and does not retry. The decision's model_context
// is deliberately absent from the text; it reaches the model as a
// model-only transcript row via the metadata.
func workspaceHookDenial(err error, subject string) (message string, hooks []workspacesdk.HookDecision, ok bool) {
	var denied *workspacesdk.HookDeniedError
	if !errors.As(err, &denied) {
		return "", nil, false
	}
	d := denied.Denied()
	message = "This tool usage was blocked by a workspace hook"
	if d.Hook != "" {
		message += fmt.Sprintf(" (%q)", d.Hook)
	}
	message += "; the " + subject + " was not executed."
	if reason := strings.TrimSpace(d.Reason); reason != "" {
		message += " Reason: " + reason + "."
	}
	message += " This is a policy decision declared in the workspace, not a" +
		" tool or workspace failure; retrying the same call will be denied" +
		" again. Explain the block to the user and adjust your approach."
	return message, denied.Hooks, true
}
