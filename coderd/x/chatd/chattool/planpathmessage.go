package chattool

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

func rejectSharedPlanPath(
	ctx context.Context,
	requestedPath string,
	resolvePlanPath func(context.Context) (string, error),
) (fantasy.ToolResponse, bool) {
	if resolvePlanPath == nil || !IsLegacySharedPlanPath(requestedPath) {
		return fantasy.ToolResponse{}, false
	}

	return fantasy.NewTextErrorResponse(sharedPlanPathMessage(ctx, resolvePlanPath)), true
}

func sharedPlanPathMessage(
	ctx context.Context,
	resolvePlanPath func(context.Context) (string, error),
) string {
	planPath, err := resolvePlanPath(ctx)
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
