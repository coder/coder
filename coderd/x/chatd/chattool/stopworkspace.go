package chattool

import (
	"context"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
)

// StopWorkspaceFn stops a workspace by creating a new build with
// the "stop" transition.
type StopWorkspaceFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	workspaceID uuid.UUID,
	req codersdk.CreateWorkspaceBuildRequest,
) (codersdk.WorkspaceBuild, error)

// StopWorkspaceOptions configures the stop_workspace tool.
type StopWorkspaceOptions struct {
	OwnerID       uuid.UUID
	StopFn        StopWorkspaceFn
	WorkspaceMu   *sync.Mutex
	OnChatUpdated func(database.Chat)
	Logger        slog.Logger
}

type stopWorkspaceArgs struct{}

// StopWorkspace returns a tool that stops the workspace associated
// with the current chat. The tool is idempotent when the workspace is
// already stopped. db must not be nil and chatID must not be uuid.Nil.
func StopWorkspace(db database.Store, chatID uuid.UUID, options StopWorkspaceOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"stop_workspace",
		"Stop the chat's workspace and wait for the stop build to complete. "+
			"If another workspace build is already in progress, this waits "+
			"for that build first, then stops the workspace if needed. "+
			"After waiting, this tool is idempotent if the workspace is "+
			"already stopped or the in-progress build stopped it. Use "+
			"this when the "+
			"user explicitly asks to stop the workspace, or when a "+
			"workspace-agent error tells you to stop and then start the "+
			"workspace. Stopping a workspace terminates running processes "+
			"and may discard unsaved in-memory state. This tool does not "+
			"delete the workspace.",
		func(ctx context.Context, _ stopWorkspaceArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.StopFn == nil {
				return fantasy.NewTextErrorResponse("workspace stopper is not configured"), nil
			}

			// Serialize with create_workspace and start_workspace to
			// prevent lifecycle races.
			if options.WorkspaceMu != nil {
				options.WorkspaceMu.Lock()
				defer options.WorkspaceMu.Unlock()
			}

			chat, err := db.GetChatByID(ctx, chatID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("load chat: %w", err).Error(),
				), nil
			}
			if !chat.WorkspaceID.Valid {
				return fantasy.NewTextErrorResponse(
					"chat has no workspace; use create_workspace first",
				), nil
			}

			ws, err := db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("load workspace: %w", err).Error(),
				), nil
			}
			if ws.Deleted {
				return fantasy.NewTextErrorResponse(
					"workspace was deleted; use create_workspace to make a new one",
				), nil
			}

			build, job, err := latestWorkspaceBuildAndJob(ctx, db, ws.ID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			// If a build is already in progress, wait for it before
			// deciding whether a stop build is still needed.
			switch job.JobStatus {
			case database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
				database.ProvisionerJobStatusCanceling:
				publishBuildBinding(ctx, db, options.Logger, chatID, ws.ID, build.ID, options.OnChatUpdated)

				waitErr := waitForBuild(ctx, db, build.ID)
				// Re-read after waiting because another transition may
				// have completed while this tool was blocked.
				ws, err = db.GetWorkspaceByID(ctx, ws.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(
						xerrors.Errorf("load workspace: %w", err).Error(),
					), nil
				}
				if ws.Deleted {
					return fantasy.NewTextErrorResponse(
						"workspace was deleted; use create_workspace to make a new one",
					), nil
				}
				build, job, err = latestWorkspaceBuildAndJob(ctx, db, ws.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				// The fresh job row is authoritative. A wait error can
				// be stale if the build reached a terminal state while the
				// wait context was ending.
				if waitErr != nil && !provisionerJobTerminal(job.JobStatus) {
					return buildToolResponse(newBuildError(
						xerrors.Errorf("waiting for in-progress build: %w", waitErr).Error(),
						build.ID,
					)), nil
				}
			}

			if job.JobStatus == database.ProvisionerJobStatusSucceeded &&
				build.Transition == database.WorkspaceTransitionStop {
				result := map[string]any{
					"stopped":        true,
					"workspace_name": ws.Name,
				}
				setNoBuild(result, uuid.Nil)
				return toolResponse(result), nil
			}

			ownerCtx, ownerErr := asOwner(ctx, db, options.OwnerID)
			if ownerErr != nil {
				return fantasy.NewTextErrorResponse(ownerErr.Error()), nil
			}

			stopBuild, err := options.StopFn(ownerCtx, options.OwnerID, ws.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
			})
			if err != nil {
				if responseErr, ok := httperror.IsResponder(err); ok {
					_, resp := responseErr.Response()
					return toolResponse(responseErrorResult(resp)), nil
				}
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("stop workspace: %w", err).Error(),
				), nil
			}

			publishBuildBinding(ctx, db, options.Logger, chatID, ws.ID, stopBuild.ID, options.OnChatUpdated)
			if err := waitForBuild(ctx, db, stopBuild.ID); err != nil {
				return buildToolResponse(newBuildError(
					xerrors.Errorf("workspace stop build failed: %w", err).Error(),
					stopBuild.ID,
				)), nil
			}

			if options.OnChatUpdated != nil {
				if latest, err := db.GetChatByID(ctx, chatID); err == nil {
					options.OnChatUpdated(latest)
				}
			}

			result := map[string]any{
				"stopped":        true,
				"workspace_name": ws.Name,
			}
			setBuildID(result, stopBuild.ID)
			return toolResponse(result), nil
		})
}
