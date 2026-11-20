package chattool

import (
	"context"
	"errors"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/codersdk"
)

const activeVersionRetryInstructions = "The workspace needs the template's active version before it can start. Use read_template with this workspace's template_id to inspect the active version's required parameters, then retry start_workspace with a parameters object that supplies any missing or changed values. If the correct value for a parameter is not obvious from its description or defaults, ask the user rather than guessing."

// StartWorkspaceOptions configures the start_workspace tool.
type StartWorkspaceOptions struct {
	OwnerID       uuid.UUID
	BuildFn       BuildWorkspaceFn
	AgentConnFn   AgentConnFunc
	WorkspaceMu   *sync.Mutex
	OnChatUpdated func(database.Chat)
	Logger        slog.Logger
}

type startWorkspaceArgs struct {
	Parameters map[string]string `json:"parameters,omitempty"`
}

// StartWorkspace returns a tool that starts a stopped workspace
// associated with the current chat. The tool is idempotent: if the
// workspace is already running or building, it returns immediately.
// db must not be nil and chatID must not be uuid.Nil.
func StartWorkspace(db database.Store, chatID uuid.UUID, options StartWorkspaceOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"start_workspace",
		"Start the chat's workspace if it is currently stopped. "+
			"This tool is idempotent: if the workspace is already "+
			"running, it returns immediately. Use create_workspace "+
			"first if no workspace exists yet. Provide parameter "+
			"values (from read_template) only if necessary or "+
			"explicitly requested by the user.",
		func(ctx context.Context, args startWorkspaceArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.BuildFn == nil {
				return fantasy.NewTextErrorResponse("workspace starter is not configured"), nil
			}

			// Serialize with create_workspace and stop_workspace to prevent races.
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
				return workspaceLoadErrorResponse(err), nil
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

			// If a build is already in progress, wait for it.
			switch job.JobStatus {
			case database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning:
				// Publish the build ID to the frontend so it
				// can start streaming logs immediately.
				if err := publishBuildBinding(ctx, db, chatID, ws.ID, build.ID, options.OnChatUpdated); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				if err := waitForBuild(ctx, db, build.ID); err != nil {
					// newBuildError returns via toolResponse (IsError: false)
					// rather than NewTextErrorResponse (IsError: true) so the
					// JSON result preserves build_id for the frontend's log
					// viewer. The fantasy/chatprompt pipeline discards structured
					// fields from IsError content.
					// The frontend detects errors via the "error" key instead.
					return buildFailureToolResponse(
						ctx,
						options.Logger,
						db,
						options.OwnerID,
						ws.OrganizationID,
						buildFailureActionStart,
						build.ID,
						xerrors.Errorf("waiting for in-progress build: %w", err),
					), nil
				}
				result, err := waitForAgentAndRespond(ctx, db, options.AgentConnFn, ws, build.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				// Re-fire after the agent is fully ready so
				// callers can load instruction files (AGENTS.md).
				// This must happen after waitForAgentAndRespond —
				// firing earlier races with agent startup.
				if options.OnChatUpdated != nil {
					latest, err := db.GetChatByID(ctx, chatID)
					if err != nil {
						return fantasy.NewTextErrorResponse(
							xerrors.Errorf("load chat: %w", err).Error(),
						), nil
					}
					options.OnChatUpdated(latest)
				}
				return toolResponse(result), nil
			case database.ProvisionerJobStatusSucceeded:
				// If the latest successful build is a start
				// transition, the workspace should be running.
				if build.Transition == database.WorkspaceTransitionStart {
					if chat.BuildID.UUID != build.ID {
						if err := publishBuildBinding(ctx, db, chatID, ws.ID, build.ID, options.OnChatUpdated); err != nil {
							return fantasy.NewTextErrorResponse(err.Error()), nil
						}
					}
					result, err := waitForAgentAndRespond(ctx, db, options.AgentConnFn, ws, uuid.Nil)
					if err != nil {
						return fantasy.NewTextErrorResponse(err.Error()), nil
					}
					return toolResponse(result), nil
				}
				// Otherwise it is stopped (or deleted) — proceed
				// to start it below.

			default:
				// Failed, canceled, etc — try starting anyway.
			}

			// Set up dbauthz context for the start call.
			ownerCtx, ownerErr := asOwner(ctx, db, options.OwnerID)
			if ownerErr != nil {
				return fantasy.NewTextErrorResponse(ownerErr.Error()), nil
			}

			activeVersionID, err := requiredActiveVersion(ownerCtx, db, ws.TemplateID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("start workspace: %w", err).Error(),
				), nil
			}
			updating := activeVersionID != uuid.Nil && activeVersionID != build.TemplateVersionID

			startReq := codersdk.CreateWorkspaceBuildRequest{
				Transition:        codersdk.WorkspaceTransitionStart,
				TemplateVersionID: activeVersionID,
			}
			for k, v := range args.Parameters {
				startReq.RichParameterValues = append(
					startReq.RichParameterValues,
					codersdk.WorkspaceBuildParameter{Name: k, Value: v},
				)
			}

			startBuild, err := options.BuildFn(ownerCtx, options.OwnerID, ws.ID, startReq)
			if err != nil {
				if responseErr, ok := httperror.IsResponder(err); ok {
					_, resp := responseErr.Response()
					needsParameters := updating && isParameterError(err)
					if needsParameters {
						resp = withActiveVersionRetryInstructions(resp, err.Error())
					}
					result := responseErrorResult(resp)
					if needsParameters || len(resp.Validations) > 0 {
						result["template_id"] = ws.TemplateID.String()
					}
					return toolResponse(result), nil
				}
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("start workspace: %w", err).Error(),
				), nil
			}

			// Persist the build ID on the chat binding so the
			// frontend can stream logs without polling.
			if err := publishBuildBinding(ctx, db, chatID, ws.ID, startBuild.ID, options.OnChatUpdated); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if err := waitForBuild(ctx, db, startBuild.ID); err != nil {
				return buildFailureToolResponse(
					ctx,
					options.Logger,
					db,
					options.OwnerID,
					ws.OrganizationID,
					buildFailureActionStart,
					startBuild.ID,
					xerrors.Errorf("workspace start build failed: %w", err),
				), nil
			}

			result, err := waitForAgentAndRespond(ctx, db, options.AgentConnFn, ws, startBuild.ID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			if updating {
				result["updated_to_active_version"] = true
				result["update_reason"] = "template requires active versions"
				result["message"] = "Workspace started and was updated to the active template version because the template requires active versions."
			}

			// Re-fire after the agent is fully ready so
			// callers can load instruction files (AGENTS.md).
			// This must happen after waitForAgentAndRespond —
			// firing earlier races with agent startup.
			if options.OnChatUpdated != nil {
				latest, err := db.GetChatByID(ctx, chatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(
						xerrors.Errorf("load chat: %w", err).Error(),
					), nil
				}
				options.OnChatUpdated(latest)
			}
			return toolResponse(result), nil
		})
}

// requiredActiveVersion returns the template's active version when the
// template requires workspaces to use it, and uuid.Nil otherwise.
func requiredActiveVersion(ctx context.Context, db database.Store, templateID uuid.UUID) (uuid.UUID, error) {
	template, err := db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get template: %w", err)
	}
	if !template.RequireActiveVersion {
		return uuid.Nil, nil
	}
	return template.ActiveVersionID, nil
}

func isParameterError(err error) bool {
	var diagnosticErr *dynamicparameters.DiagnosticError
	return errors.As(err, &diagnosticErr) ||
		errors.Is(err, dynamicparameters.ErrParameterValidation)
}

func withActiveVersionRetryInstructions(resp codersdk.Response, fallbackDetail string) codersdk.Response {
	originalMessage := resp.Message
	resp.Message = activeVersionRetryInstructions
	if len(resp.Validations) == 0 && originalMessage != "" {
		if resp.Detail == "" {
			resp.Detail = originalMessage
		} else {
			resp.Detail = originalMessage + ": " + resp.Detail
		}
	} else if resp.Detail == "" {
		resp.Detail = fallbackDetail
	}
	return resp
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
) (map[string]any, error) {
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}
	if len(agents) == 0 {
		// Workspace started but no agent found - still report
		// success so the model knows the workspace is up.
		result := map[string]any{
			"started":        true,
			"workspace_name": ws.Name,
			"agent_status":   "no_agent",
		}
		setBuildID(result, buildID)
		setNoBuild(result, buildID)
		return result, nil
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
		return result, nil
	}

	result := map[string]any{
		"started":        true,
		"workspace_name": ws.Name,
	}
	setBuildID(result, buildID)
	setNoBuild(result, buildID)
	agentStatus, err := waitForAgentReady(ctx, db, selected, agentConnFn)
	if err != nil {
		return nil, err
	}
	for k, v := range agentStatus {
		result[k] = v
	}
	return result, nil
}
