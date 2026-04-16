package chattool

import (
	"context"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func ensurePlanPathResolvesToItself(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	planPath string,
) error {
	if conn == nil {
		return xerrors.New("workspace connection is required")
	}

	normalizedPlanPath := normalizeWorkspacePath(planPath)
	resolvedPath, err := conn.ResolvePath(ctx, planPath)
	if err != nil {
		if resolvePathUnsupported(err) {
			// Older workspace agents do not expose /resolve-path yet. Keep
			// plan turns working during rolling upgrades, even though they
			// cannot enforce the symlink guard until the agent is upgraded.
			return nil
		}
		return xerrors.Errorf("resolve plan path: %w", err)
	}
	resolvedPath = normalizeWorkspacePath(resolvedPath)
	if resolvedPath != normalizedPlanPath {
		return xerrors.New(symlinkedPlanPathMessage(normalizedPlanPath, resolvedPath))
	}

	return nil
}

func resolvePathUnsupported(err error) bool {
	var statusErr interface{ StatusCode() int }
	return xerrors.As(err, &statusErr) && statusErr.StatusCode() == http.StatusNotFound
}

func normalizeWorkspacePath(pathString string) string {
	pathString = strings.TrimSpace(pathString)
	if pathString == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(pathString))
}
