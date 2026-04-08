package chattool

import (
	"context"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/internal/agentselect"
	"github.com/coder/coder/v2/codersdk"
)

// StartWorkspaceFn starts a workspace by creating a new build with
// the "start" transition.
type StartWorkspaceFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	workspaceID uuid.UUID,
	req codersdk.CreateWorkspaceBuildRequest,
) (codersdk.WorkspaceBuild, error)

// StartWorkspaceOptions configures the start_workspace tool.
type StartWorkspaceOptions struct {
	DB          database.Store
	OwnerID     uuid.UUID
	ChatID      uuid.UUID
	StartFn     StartWorkspaceFn
	AgentConnFn AgentConnFunc
	WorkspaceMu *sync.Mutex
}

// StartWorkspace returns a tool that starts a stopped workspace
// associated with the current chat. The tool is idempotent: if the
// workspace is already running or building, it returns immediately.
func StartWorkspace(options StartWorkspaceOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"start_workspace",
		"Start the chat's workspace if it is currently stopped. "+
			"This tool is idempotent — if the workspace is already "+
			"running, it returns immediately. Use create_workspace "+
			"first if no workspace exists yet.",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.StartFn == nil {
				return fantasy.NewTextErrorResponse("workspace starter is not configured"), nil
			}

			// Serialize with create_workspace to prevent races.
			if options.WorkspaceMu != nil {
				options.WorkspaceMu.Lock()
				defer options.WorkspaceMu.Unlock()
			}

			if options.DB == nil || options.ChatID == uuid.Nil {
				return fantasy.NewTextErrorResponse("start_workspace is not properly configured"), nil
			}

			chat, err := options.DB.GetChatByID(ctx, options.ChatID)
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

			ws, err := options.DB.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
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

			build, err := options.DB.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("get latest build: %w", err).Error(),
				), nil
			}

			job, err := options.DB.GetProvisionerJobByID(ctx, build.JobID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("get provisioner job: %w", err).Error(),
				), nil
			}

			// If a build is already in progress, wait for it.
			switch job.JobStatus {
			case database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning:
				if err := waitForBuild(ctx, options.DB, ws.ID); err != nil {
					return fantasy.NewTextErrorResponse(
						xerrors.Errorf("waiting for in-progress build: %w", err).Error(),
					), nil
				}
				return waitForAgentAndRespond(ctx, options.DB, options.AgentConnFn, ws)

			case database.ProvisionerJobStatusSucceeded:
				// If the latest successful build is a start
				// transition, the workspace should be running.
				if build.Transition == database.WorkspaceTransitionStart {
					return waitForAgentAndRespond(ctx, options.DB, options.AgentConnFn, ws)
				}
				// Otherwise it is stopped (or deleted) — proceed
				// to start it below.

			default:
				// Failed, canceled, etc — try starting anyway.
			}

			// Set up dbauthz context for the start call.
			ownerCtx, ownerErr := asOwner(ctx, options.DB, options.OwnerID)
			if ownerErr != nil {
				return fantasy.NewTextErrorResponse(ownerErr.Error()), nil
			}

			_, err = options.StartFn(ownerCtx, options.OwnerID, ws.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("start workspace: %w", err).Error(),
				), nil
			}

			if err := waitForBuild(ctx, options.DB, ws.ID); err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("workspace start build failed: %w", err).Error(),
				), nil
			}

			return waitForAgentAndRespond(ctx, options.DB, options.AgentConnFn, ws)
		},
	)
}

// waitForAgentAndRespond selects the chat agent from the workspace's
// latest build, waits for it to become reachable, and returns a
// success response.
func waitForAgentAndRespond(
	ctx context.Context,
	db database.Store,
	agentConnFn AgentConnFunc,
	ws database.Workspace,
) (fantasy.ToolResponse, error) {
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
	if err != nil || len(agents) == 0 {
		// Workspace started but no agent found - still report
		// success so the model knows the workspace is up.
		return toolResponse(map[string]any{
			"started":        true,
			"workspace_name": ws.Name,
			"agent_status":   "no_agent",
		}), nil
	}

	selected, err := agentselect.FindChatAgent(agents)
	if err != nil {
		return toolResponse(map[string]any{
			"started":        true,
			"workspace_name": ws.Name,
			"agent_status":   "selection_error",
			"agent_error":    err.Error(),
		}), nil
	}

	result := map[string]any{
		"started":        true,
		"workspace_name": ws.Name,
	}
	for k, v := range waitForAgentReady(ctx, db, selected.ID, agentConnFn) {
		result[k] = v
	}
	return toolResponse(result), nil
}
