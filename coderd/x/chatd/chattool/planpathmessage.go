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

	return fantasy.NewTextErrorResponse(sharedPlanPathMessage(planPath, planPathErr)), true
}

func looksLikeLegacyHomePlanPath(requestedPath, home string) bool {
	if strings.TrimSpace(home) == "" {
		return IsLegacySharedPlanPath(requestedPath)
	}

	return LooksLikeHomePlanFile(requestedPath, home)
}

func sharedPlanPathMessage(planPath string, err error) string {
	if err == nil && strings.TrimSpace(planPath) != "" {
		return fmt.Sprintf(
			"the shared plan path %s is no longer supported; use the chat-specific plan path: %s",
			LegacySharedPlanPath,
			planPath,
		)
	}

	return fmt.Sprintf(
		"the shared plan path %s is no longer supported; use the chat-specific plan path provided in your instructions",
		LegacySharedPlanPath,
	)
}
