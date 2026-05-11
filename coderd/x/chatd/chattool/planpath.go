package chattool

import (
	"context"
	"path"
	"strings"

	"charm.land/fantasy"
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

func resolvePlanTurnPath(
	ctx context.Context,
	resolvePlanPath ResolveChatPlanPath,
) (string, error) {
	if resolvePlanPath == nil {
		return "", xerrors.New("chat-specific plan path resolver is not configured")
	}

	planPath, _, err := resolvePlanPath(ctx)
	if err != nil {
		return "", xerrors.Errorf("resolve chat-specific plan path: %w", err)
	}
	planPath = strings.TrimSpace(planPath)
	if planPath == "" {
		return "", xerrors.New("chat-specific plan path is empty")
	}

	return planPath, nil
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

// ResolveChatPlanPath is the resolver signature used by every chattool
// handler that needs to validate plan-file paths. It returns the
// chat-specific plan path, the workspace home directory, and any
// resolution error.
type ResolveChatPlanPath func(ctx context.Context) (chatPath string, home string, err error)

// validatePlanPath enforces plan-file path rules for a single requested
// path. It returns (response, true) when the caller must short-circuit
// with the returned error response, and (zero, false) when the path is
// acceptable and the caller can proceed.
//
// The rules are:
//
//  1. A path whose basename matches plan.md (case-insensitive) must be
//     absolute. Relative plan paths are rejected because the agent has
//     no consistent working directory between turns.
//  2. When a resolver is configured, a path that targets the legacy
//     shared plan file or the same plan.md sitting directly in the
//     workspace home is rejected and the user is pointed at the
//     chat-specific plan path.
//
// Suitable for single-path tools (write_file, propose_plan). Multi-path
// tools (edit_files) should call this per file and may wrap the error
// content to add batch context. Wrap the resolver in
// memoizedPlanPathResolver to avoid resolving the workspace home once
// per file when iterating a batch.
func validatePlanPath(
	ctx context.Context,
	requestedPath string,
	resolvePlanPath ResolveChatPlanPath,
) (fantasy.ToolResponse, bool) {
	hasPlanFileName := looksLikePlanFileName(requestedPath)
	if hasPlanFileName && !isAbsolutePath(requestedPath) {
		return fantasy.NewTextErrorResponse(
			"plan files must use absolute paths; use the chat-specific absolute plan path",
		), true
	}

	if resolvePlanPath == nil || !hasPlanFileName {
		return fantasy.ToolResponse{}, false
	}

	chatPath, home, err := resolvePlanPath(ctx)
	return rejectSharedPlanPath(requestedPath, home, chatPath, err)
}

// memoizedPlanPathResolver wraps resolver so the underlying call runs at
// most once. Subsequent invocations return the cached chat path, home,
// and error. Returns nil when resolver is nil. The returned resolver is
// not safe for concurrent use; chattool handlers iterate file batches
// sequentially.
func memoizedPlanPathResolver(resolver ResolveChatPlanPath) ResolveChatPlanPath {
	if resolver == nil {
		return nil
	}
	var (
		loaded   bool
		chatPath string
		home     string
		err      error
	)
	return func(ctx context.Context) (string, string, error) {
		if !loaded {
			chatPath, home, err = resolver(ctx)
			loaded = true
		}
		return chatPath, home, err
	}
}
