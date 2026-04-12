package chattool

import (
	"fmt"

	"charm.land/fantasy"
)

func rejectSharedPlanPath(
	requestedPath string,
	home string,
	planPath string,
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

	if !LooksLikeHomePlanFile(requestedPath, home) {
		return fantasy.ToolResponse{}, false
	}

	return fantasy.NewTextErrorResponse(
		sharedPlanPathMessage(requestedPath, planPath),
	), true
}

func sharedPlanPathMessage(requestedPath, planPath string) string {
	return fmt.Sprintf(
		"the plan path %s is no longer supported at the home root; use the chat-specific plan path: %s",
		requestedPath,
		planPath,
	)
}

func planPathVerificationMessage(requestedPath string) string {
	return fmt.Sprintf(
		"the plan path %s could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly",
		requestedPath,
	)
}
