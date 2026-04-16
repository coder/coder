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
	DB            database.Store
	OwnerID       uuid.UUID
	ChatID        uuid.UUID
	StartFn       StartWorkspaceFn
	AgentConnFn   AgentConnFunc
	WorkspaceMu   *sync.Mutex
	OnChatUpdated func(database.Chat)
	Logger        slog.Logger
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
				// Publish the build ID to the frontend so it
				// can start streaming logs immediately.
				updatedChat, bindErr := options.DB.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
					ID:          options.ChatID,
					WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
					BuildID: uuid.NullUUID{
						UUID:  build.ID,
						Valid: build.ID != uuid.Nil,
					},
					AgentID: uuid.NullUUID{},
				})
				if bindErr != nil {
					options.Logger.Error(ctx, "failed to persist build ID on chat binding",
						slog.F("chat_id", options.ChatID),
						slog.F("build_id", build.ID),
						slog.Error(bindErr),
					)
				} else if options.OnChatUpdated != nil {
					options.OnChatUpdated(updatedChat)
				}
				if err := waitForBuild(ctx, options.DB, build.ID); err != nil {
					// newBuildError returns via toolResponse (IsError: false)
					// rather than NewTextErrorResponse (IsError: true) so the
					// JSON result preserves build_id for the frontend's log
					// viewer. The fantasy/chatprompt pipeline discards structured
					// fields from IsError content.
					// The frontend detects errors via the "error" key instead.
					return buildToolResponse(newBuildError(
						xerrors.Errorf("waiting for in-progress build: %w", err).Error(),
						build.ID,
					)), nil
				}
				result := waitForAgentAndRespond(ctx, options.DB, options.AgentConnFn, ws, build.ID)
				// Re-fire after the agent is fully ready so
				// callers can load instruction files (AGENTS.md).
				// This must happen after waitForAgentAndRespond —
				// firing earlier races with agent startup.
				if options.OnChatUpdated != nil {
					if latest, err := options.DB.GetChatByID(ctx, options.ChatID); err == nil {
						options.OnChatUpdated(latest)
					}
				}
				return toolResponse(result), nil
			case database.ProvisionerJobStatusSucceeded:
				// If the latest successful build is a start
				// transition, the workspace should be running.
				if build.Transition == database.WorkspaceTransitionStart {
					return toolResponse(waitForAgentAndRespond(ctx, options.DB, options.AgentConnFn, ws, uuid.Nil)), nil
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

			startBuild, err := options.StartFn(ownerCtx, options.OwnerID, ws.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			})
			if err != nil {
				if responseErr, ok := httperror.IsResponder(err); ok {
					_, resp := responseErr.Response()
					if resp.Message != "" {
						return fantasy.NewTextErrorResponse(resp.Message), nil
					}
				}
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("start workspace: %w", err).Error(),
				), nil
			}

			// Persist the build ID on the chat binding so the
			// frontend can stream logs without polling.
			updatedChat, bindErr := options.DB.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
				ID:          options.ChatID,
				WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
				BuildID: uuid.NullUUID{
					UUID:  startBuild.ID,
					Valid: startBuild.ID != uuid.Nil,
				},
				AgentID: uuid.NullUUID{},
			})
			if bindErr != nil {
				options.Logger.Error(ctx, "failed to persist build ID on chat binding",
					slog.F("chat_id", options.ChatID),
					slog.F("build_id", startBuild.ID),
					slog.Error(bindErr),
				)
			} else if options.OnChatUpdated != nil {
				options.OnChatUpdated(updatedChat)
			}
			if err := waitForBuild(ctx, options.DB, startBuild.ID); err != nil {
				return buildToolResponse(newBuildError(
					xerrors.Errorf("workspace start build failed: %w", err).Error(),
					startBuild.ID,
				)), nil
			}

			result := waitForAgentAndRespond(ctx, options.DB, options.AgentConnFn, ws, startBuild.ID)

			// If the template version changed, annotate the
			// response so the model knows an auto-update
			// occurred.
			if startBuild.TemplateVersionID != uuid.Nil &&
				build.TemplateVersionID != uuid.Nil &&
				startBuild.TemplateVersionID != build.TemplateVersionID {
				result["updated_to_active_version"] = true
				result["update_reason"] = "template requires active versions"
				result["message"] = "Workspace started and was updated to the active template version because the template requires active versions."
			}

			// Re-fire after the agent is fully ready so
			// callers can load instruction files (AGENTS.md).
			// This must happen after waitForAgentAndRespond —
			// firing earlier races with agent startup.
			if options.OnChatUpdated != nil {
				if latest, err := options.DB.GetChatByID(ctx, options.ChatID); err == nil {
					options.OnChatUpdated(latest)
				}
			}
			return toolResponse(result), nil
		})
}

// waitForAgentAndRespond selects the chat agent from the workspace's
// latest build, waits for it to become reachable, and returns a
// result map. When buildID is non-zero, it is included in the
// result so the frontend can fetch historical build logs. Pass
// uuid.Nil when no build was triggered (e.g. workspace already
// running); the result will include no_build: true so the
// frontend can suppress the build-log section.
//
// The caller is responsible for converting the returned map to a
// fantasy.ToolResponse via toolResponse(), and may add extra
// fields before doing so.
func waitForAgentAndRespond(
	ctx context.Context,
	db database.Store,
	agentConnFn AgentConnFunc,
	ws database.Workspace,
	buildID uuid.UUID,
) map[string]any {
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
	if err != nil || len(agents) == 0 {
		// Workspace started but no agent found - still report
		// success so the model knows the workspace is up.
		result := map[string]any{
			"started":        true,
			"workspace_name": ws.Name,
			"agent_status":   "no_agent",
		}
		setBuildID(result, buildID)
		setNoBuild(result, buildID)
		return result
	}

	selected, err := agentselect.FindChatAgent(agents)
	if err != nil {
		result := map[string]any{
			"started":        true,
			"workspace_name": ws.Name,
			"agent_status":   "selection_error",
			"agent_error":    err.Error(),
		}
		setBuildID(result, buildID)
		setNoBuild(result, buildID)
		return result
	}

	result := map[string]any{
		"started":        true,
		"workspace_name": ws.Name,
	}
	setBuildID(result, buildID)
	setNoBuild(result, buildID)
	for k, v := range waitForAgentReady(ctx, db, selected.ID, agentConnFn) {
		result[k] = v
	}
	return result
}
