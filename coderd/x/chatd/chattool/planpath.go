package chattool

import (
	"context"
	pathpkg "path"
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
	return pathpkg.Join(
		home,
		".coder",
		"plans",
		planFileNamePrefix+chatID.String()+".md",
	)
}

// isAbsolutePath reports whether p is an absolute path on either
// POSIX or Windows. Since chatd runs on Linux, the POSIX
// absolute-path check only recognizes forward-slash roots. This
// also detects Windows drive letter prefixes (for example, C:\ or
// C:/).
func isAbsolutePath(p string) bool {
	if pathpkg.IsAbs(p) {
		return true
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') {
		c := p[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}

// looksLikePlanFileName reports whether the base name of requestedPath
// is "plan.md" (case-insensitive), ignoring the directory component.
func looksLikePlanFileName(requestedPath string) bool {
	cleaned := pathpkg.Clean(strings.ReplaceAll(requestedPath, "\\", "/"))
	return strings.EqualFold(pathpkg.Base(cleaned), "plan.md")
}

// LooksLikeHomePlanFile reports whether requestedPath is a plan.md
// variant (case-insensitive) sitting directly in the workspace home
// directory.
// The filename is compared case-insensitively because LLM output varies.
// The directory is compared exactly because it comes from the system.
func LooksLikeHomePlanFile(requestedPath, home string) bool {
	// Normalize backslashes so Windows workspace paths (for example,
	// C:\\Users\\coder\\PLAN.md) are handled correctly. The chatd server
	// runs on Linux, so the POSIX cleaner alone only parses forward slashes.
	normalized := pathpkg.Clean(strings.ReplaceAll(requestedPath, "\\", "/"))
	normalizedHome := pathpkg.Clean(strings.ReplaceAll(home, "\\", "/"))

	return looksLikePlanFileName(normalized) &&
		pathpkg.Dir(normalized) == normalizedHome
}

func isLegacySharedPlanPath(requested string) bool {
	return requested == LegacySharedPlanPath
}
