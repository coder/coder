package workspacetools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// PROTOTYPE (CODAGT-1083). Workspace hooks run on the agent inside the
// tool request. These helpers turn their decisions into tool results
// for every surface that runs the workspace tools.

// HookDeniedError is returned by the workspace tools when a workspace
// hook denied the call. Error() is the text the model reads: it must
// read as a policy decision, not a tool failure, or the model retries.
// Hooks carries the decisions for callers that record them; the
// decisions' model_context is deliberately not part of the text.
type HookDeniedError struct {
	Message string
	Hooks   []workspacesdk.HookDecision
}

func (e *HookDeniedError) Error() string { return e.Message }

// hookDenial converts an agent hook denial into a HookDeniedError.
// subject names what did not run, such as "command". It returns nil
// when err is not a denial.
func hookDenial(err error, subject string) *HookDeniedError {
	var denied *workspacesdk.HookDeniedError
	if !errors.As(err, &denied) {
		return nil
	}
	d := denied.Denied()
	message := "This tool usage was blocked by a workspace hook"
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
	return &HookDeniedError{Message: message, Hooks: denied.Hooks}
}

// DeniedHooks returns the decisions carried by a HookDeniedError, or
// nil for any other error.
func DeniedHooks(err error) []workspacesdk.HookDecision {
	var denied *HookDeniedError
	if errors.As(err, &denied) {
		return denied.Hooks
	}
	return nil
}
