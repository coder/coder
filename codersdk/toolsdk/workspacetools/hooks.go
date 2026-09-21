package workspacetools

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// PROTOTYPE (CODAGT-1083). Workspace hooks run on the agent inside the
// tool request. These helpers turn their decisions into tool results
// for every surface that runs the workspace tools.

// hookDenied reports whether a workspace hook denied the call.
func hookDenied(hooks []workspacesdk.HookDecision) bool {
	for _, h := range hooks {
		if h.Decision == "deny" {
			return true
		}
	}
	return false
}

// hookDeniedResult renders a workspace hook denial the way the
// deployment lifecycle hook renders one: the model must read it as a
// policy decision, not a tool failure, or it retries.
func hookDeniedResult(hooks []workspacesdk.HookDecision) ExecuteResult {
	var denied workspacesdk.HookDecision
	for _, h := range hooks {
		if h.Decision == "deny" {
			denied = h
			break
		}
	}
	message := "This tool usage was blocked by a workspace hook"
	if denied.Hook != "" {
		message += fmt.Sprintf(" (%q)", denied.Hook)
	}
	message += "; the command was not executed."
	if reason := strings.TrimSpace(denied.Reason); reason != "" {
		message += " Reason: " + reason + "."
	}
	message += " This is a policy decision declared in the workspace, not a" +
		" tool or workspace failure; retrying the same call will be denied" +
		" again. Explain the block to the user and adjust your approach."
	if ctx := strings.TrimSpace(denied.ModelContext); ctx != "" {
		message += "\n\n" + ctx
	}
	return ExecuteResult{
		Success: false,
		Error:   message,
		Hooks:   hooks,
	}
}
