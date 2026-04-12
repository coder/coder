package chattool

import (
	"fmt"
	"strings"

	"charm.land/fantasy"
)

func rejectSharedPlanPath(
	requestedPath string,
	home string,
	planPath string,
	planPathErr error,
) (fantasy.ToolResponse, bool) {
	if !looksLikeLegacyHomePlanPath(requestedPath, home) {
		return fantasy.ToolResponse{}, false
	}

	return fantasy.NewTextErrorResponse(
		sharedPlanPathMessage(requestedPath, planPath, planPathErr),
	), true
}

func looksLikeLegacyHomePlanPath(requestedPath, home string) bool {
	if strings.TrimSpace(home) == "" {
		return strings.EqualFold(requestedPath, LegacySharedPlanPath)
	}

	return LooksLikeHomePlanFile(requestedPath, home)
}

func sharedPlanPathMessage(requestedPath, planPath string, err error) string {
	if err == nil && strings.TrimSpace(planPath) != "" {
		return fmt.Sprintf(
			"the plan path %s is no longer supported at the home root; use the chat-specific plan path: %s",
			requestedPath,
			planPath,
		)
	}

	return fmt.Sprintf(
		"the plan path %s is no longer supported at the home root; the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly",
		requestedPath,
	)
}
