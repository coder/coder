package chattool

import (
	"context"
	"path"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const planFileNamePrefix = "PLAN-"

// LegacySharedPlanPath is the original shared plan file path used by
// every chat in a workspace.
const LegacySharedPlanPath = "/home/coder/PLAN.md"

// ResolveWorkspaceHome returns the workspace user's home directory.
func ResolveWorkspaceHome(
	ctx context.Context,
	conn workspacesdk.AgentConn,
) (string, error) {
	if conn == nil {
		return "", xerrors.New("workspace connection is required")
	}

	resp, err := conn.LS(ctx, "", workspacesdk.LSRequest{
		Path:       []string{},
		Relativity: workspacesdk.LSRelativityHome,
	})
	if err != nil {
		return "", xerrors.Errorf("resolve workspace home: %w", err)
	}

	home := strings.TrimSpace(resp.AbsolutePathString)
	if home == "" {
		return "", xerrors.New("workspace home path is empty")
	}

	return home, nil
}

// PlanPathForChat returns the per-chat plan file path rooted in the
// workspace home directory.
func PlanPathForChat(home string, chatID uuid.UUID) string {
	return path.Join(
		home,
		".coder",
		"plans",
		planFileNamePrefix+chatID.String()+".md",
	)
}

func looksLikePlanFileName(requestedPath string) bool {
	cleaned := path.Clean(strings.ReplaceAll(requestedPath, "\\", "/"))
	return strings.EqualFold(path.Base(cleaned), "plan.md")
}

// LooksLikeHomePlanFile reports whether requestedPath is a plan.md
// variant (case-insensitive) sitting directly in the workspace home
// directory.
func LooksLikeHomePlanFile(requestedPath, home string) bool {
	// Normalize backslashes so Windows workspace paths (for example,
	// C:\Users\coder\PLAN.md) are handled correctly. The chatd server
	// runs on Linux, so path.Clean alone only parses forward slashes.
	normalized := path.Clean(strings.ReplaceAll(requestedPath, "\\", "/"))
	normalizedHome := path.Clean(strings.ReplaceAll(home, "\\", "/"))

	return looksLikePlanFileName(normalized) &&
		path.Dir(normalized) == normalizedHome
}

// IsLegacySharedPlanPath reports whether requested is the exact legacy
// shared plan file path.
func IsLegacySharedPlanPath(requested string) bool {
	return requested == LegacySharedPlanPath
}
