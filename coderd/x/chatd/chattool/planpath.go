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

// IsLegacySharedPlanPath reports whether requested is the exact legacy
// shared plan file path.
func IsLegacySharedPlanPath(requested string) bool {
	return requested == LegacySharedPlanPath
}
