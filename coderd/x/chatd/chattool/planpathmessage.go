package chattool

import (
	"fmt"

	"charm.land/fantasy"
)

// rejectSharedPlanPath reports whether requestedPath targets the shared
// home-root plan file and, if so, returns a rejection response that
// points callers at the chat-specific plan path.
func rejectSharedPlanPath(
	requestedPath string,
	home string,
	chatPath string,
	planPathErr error,
) (fantasy.ToolResponse, bool) {
	if planPathErr != nil {
		// When the resolver fails, we cannot determine the actual
		// home directory. Fall back to rejecting only the exact
		// legacy shared path (case-insensitive) rather than every
		// file named plan.md.
		if !looksLikeLegacySharedPlanPath(requestedPath) {
			return fantasy.ToolResponse{}, false
		}

		return fantasy.NewTextErrorResponse(
			planPathVerificationMessage(requestedPath),
		), true
	}

	if !LooksLikeHomePlanFile(requestedPath, home) && !looksLikeLegacySharedPlanPath(requestedPath) {
		return fantasy.ToolResponse{}, false
	}

	return fantasy.NewTextErrorResponse(
		sharedPlanPathMessage(requestedPath, chatPath),
	), true
}

func sharedPlanPathMessage(requestedPath, chatPath string) string {
	return fmt.Sprintf(
		"the plan path %s is no longer supported at the home root; use the chat-specific plan path: %s",
		requestedPath,
		chatPath,
	)
}

func symlinkedPlanPathMessage(planPath, resolvedPath string) string {
	return fmt.Sprintf(
		"the chat-specific plan path %s resolves to %s; symlinked plan paths are not allowed during plan turns",
		planPath,
		resolvedPath,
	)
}

func planPathVerificationMessage(requestedPath string) string {
	return fmt.Sprintf(
		"the plan path %s could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly",
		requestedPath,
	)
}
