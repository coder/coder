package chattool

import (
	"context"
	"database/sql"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// CreateWorkspaceBuildFn creates a workspace build (start/stop/delete)
// for the given workspace.
type CreateWorkspaceBuildFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	workspaceID uuid.UUID,
	req codersdk.CreateWorkspaceBuildRequest,
) (codersdk.WorkspaceBuild, error)

// StartWorkspaceOptions configures the start_workspace tool.
type StartWorkspaceOptions struct {
	DB                   database.Store
	OwnerID              uuid.UUID
	ChatID               uuid.UUID
	CreateWorkspaceBuild CreateWorkspaceBuildFn
	AgentConnFn          AgentConnFunc
	WorkspaceMu          *sync.Mutex
}

type startWorkspaceArgs struct{}

// StartWorkspace returns a tool that starts the chat's workspace if it
// is stopped. After the build completes it refreshes the chat's
// workspace_agent_id (agents get new IDs on each build) and waits for
// the agent to become reachable.
func StartWorkspace(options StartWorkspaceOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"start_workspace",
		"Start the workspace associated with this chat. "+
			"Use this when the workspace exists but is stopped. "+
			"The tool waits for the build to finish and the "+
			"workspace agent to become reachable before returning.",
		func(ctx context.Context, _ startWorkspaceArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.CreateWorkspaceBuild == nil {
				return fantasy.NewTextErrorResponse("workspace build creator is not configured"), nil
			}

			// Serialize workspace operations to prevent parallel
			// tool calls from racing.
			if options.WorkspaceMu != nil {
				options.WorkspaceMu.Lock()
				defer options.WorkspaceMu.Unlock()
			}

			if options.DB == nil || options.ChatID == uuid.Nil {
				return fantasy.NewTextErrorResponse("chat context is not available"), nil
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

			// Check if workspace still exists.
			ws, err := options.DB.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
			if err != nil {
				if xerrors.Is(err, sql.ErrNoRows) {
					return fantasy.NewTextErrorResponse(
						"workspace was deleted; use create_workspace to create a new one",
					), nil
				}
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("load workspace: %w", err).Error(),
				), nil
			}

			// Check the current build state to provide a
			// helpful response if already running or building.
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

			switch {
			case (job.JobStatus == database.ProvisionerJobStatusPending ||
				job.JobStatus == database.ProvisionerJobStatusRunning) &&
				build.Transition == database.WorkspaceTransitionStart:
				// Already starting — wait for it.
				if err := waitForBuild(ctx, options.DB, ws.ID); err != nil {
					return fantasy.NewTextErrorResponse(
						xerrors.Errorf("existing start build failed: %w", err).Error(),
					), nil
				}
				// Refresh agent ID after the build completes.
				if refreshErr := refreshChatAgentID(ctx, options.DB, options.ChatID, ws.ID); refreshErr != nil {
					return fantasy.NewTextErrorResponse(refreshErr.Error()), nil
				}
				return toolResponse(map[string]any{
					"started":        true,
					"workspace_name": ws.Name,
					"message":        "workspace was already starting and is now ready",
				}), nil

			case job.JobStatus == database.ProvisionerJobStatusSucceeded &&
				build.Transition == database.WorkspaceTransitionStart:
				// Already running — check agent connectivity.
				if chat.WorkspaceAgentID.Valid && options.AgentConnFn != nil {
					pingCtx, cancel := context.WithTimeout(ctx, agentPingTimeout)
					defer cancel()
					conn, release, connErr := options.AgentConnFn(pingCtx, chat.WorkspaceAgentID.UUID)
					if connErr == nil {
						release()
						_ = conn
						return toolResponse(map[string]any{
							"started":        false,
							"workspace_name": ws.Name,
							"status":         "already_running",
							"message":        "workspace is already running and reachable",
						}), nil
					}
				}
				// Agent unreachable despite succeeded start build —
				// the agent ID may be stale. Refresh it.
				if refreshErr := refreshChatAgentID(ctx, options.DB, options.ChatID, ws.ID); refreshErr != nil {
					return fantasy.NewTextErrorResponse(refreshErr.Error()), nil
				}
				return toolResponse(map[string]any{
					"started":        true,
					"workspace_name": ws.Name,
					"message":        "workspace is running; agent connection refreshed",
				}), nil
			}

			// Workspace is stopped, failed, or in a non-start
			// state — issue a start build.
			ownerCtx, ownerErr := asOwner(ctx, options.DB, options.OwnerID)
			if ownerErr != nil {
				return fantasy.NewTextErrorResponse(ownerErr.Error()), nil
			}

			_, err = options.CreateWorkspaceBuild(ownerCtx, options.OwnerID, ws.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("start workspace: %w", err).Error(),
				), nil
			}

			// Wait for the build to complete.
			if err := waitForBuild(ctx, options.DB, ws.ID); err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("workspace start build failed: %w", err).Error(),
				), nil
			}

			// Refresh the chat's workspace_agent_id since agents
			// get new IDs on each build.
			if refreshErr := refreshChatAgentID(ctx, options.DB, options.ChatID, ws.ID); refreshErr != nil {
				return fantasy.NewTextErrorResponse(refreshErr.Error()), nil
			}

			// Wait for the agent to come online.
			agents, agentErr := options.DB.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
			if agentErr == nil && len(agents) > 0 && options.AgentConnFn != nil {
				if err := waitForAgent(ctx, options.AgentConnFn, agents[0].ID); err != nil {
					return toolResponse(map[string]any{
						"started":        true,
						"workspace_name": ws.Name,
						"agent_status":   "not_ready",
						"agent_error":    err.Error(),
					}), nil
				}
			}

			return toolResponse(map[string]any{
				"started":        true,
				"workspace_name": ws.Name,
			}), nil
		},
	)
}

// refreshChatAgentID looks up the first agent in the workspace's
// latest build and updates the chat's workspace_agent_id. This is
// necessary because workspace agents get new UUIDs on each build, so
// after a start transition the old agent ID is stale.
func refreshChatAgentID(
	ctx context.Context,
	db database.Store,
	chatID uuid.UUID,
	workspaceID uuid.UUID,
) error {
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace agents: %w", err)
	}
	if len(agents) == 0 {
		return xerrors.New("workspace has no agents in the latest build")
	}

	_, err = db.UpdateChatWorkspace(ctx, database.UpdateChatWorkspaceParams{
		ID: chatID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  agents[0].ID,
			Valid: true,
		},
	})
	if err != nil {
		return xerrors.Errorf("update chat workspace agent: %w", err)
	}
	return nil
}
