package chattool

import (
	"context"
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
		return xerrors.Errorf("resolve plan path: %w", err)
	}
	resolvedPath = normalizeWorkspacePath(resolvedPath)
	if resolvedPath != normalizedPlanPath {
		return xerrors.New(symlinkedPlanPathMessage(normalizedPlanPath, resolvedPath))
	}

	return nil
}

func normalizeWorkspacePath(pathString string) string {
	pathString = strings.TrimSpace(pathString)
	if pathString == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(pathString))
}
