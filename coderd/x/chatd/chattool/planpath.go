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

// chatd consumes agent-normalized POSIX paths. Workspace agents are
// expected to convert separators to forward slashes before these
// helpers run.

// isAbsolutePath reports whether p is an absolute POSIX path.
func isAbsolutePath(p string) bool {
	return path.IsAbs(p)
}

// looksLikePlanFileName reports whether the base name of requestedPath
// is "plan.md" (case-insensitive), ignoring the directory component.
func looksLikePlanFileName(requestedPath string) bool {
	cleaned := path.Clean(requestedPath)
	return strings.EqualFold(path.Base(cleaned), "plan.md")
}

// LooksLikeHomePlanFile reports whether requestedPath is a plan.md
// variant (case-insensitive) sitting directly in the workspace home
// directory.
// The filename is compared case-insensitively because LLM output varies.
func LooksLikeHomePlanFile(requestedPath, home string) bool {
	normalized := path.Clean(requestedPath)
	normalizedHome := path.Clean(home)

	return looksLikePlanFileName(normalized) &&
		strings.EqualFold(path.Dir(normalized), normalizedHome)
}

// looksLikeLegacySharedPlanPath reports whether requestedPath
// matches the legacy shared plan path (case-insensitive). Used as a
// narrow fallback when the workspace home cannot be resolved.
func looksLikeLegacySharedPlanPath(requestedPath string) bool {
	normalized := path.Clean(requestedPath)
	return strings.EqualFold(normalized, LegacySharedPlanPath)
}
