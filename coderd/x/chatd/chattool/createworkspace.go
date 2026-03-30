package chattool

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// CreateWorkspaceFn creates a workspace for the given owner.
type CreateWorkspaceFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	req codersdk.CreateWorkspaceRequest,
) (codersdk.Workspace, error)

// AgentConnFunc provides access to workspace agent connections.
type AgentConnFunc func(
	ctx context.Context,
	agentID uuid.UUID,
) (workspacesdk.AgentConn, func(), error)

// CreateWorkspaceOptions configures the create_workspace tool.
type CreateWorkspaceOptions struct {
	DB                             database.Store
	OwnerID                        uuid.UUID
	ChatID                         uuid.UUID
	CreateFn                       CreateWorkspaceFn
	AgentConnFn                    AgentConnFunc
	AgentInactiveDisconnectTimeout time.Duration
	WorkspaceMu                    *sync.Mutex
	OnChatUpdated                  func(database.Chat)
	Logger                         slog.Logger
	AllowedTemplateIDs             func() map[uuid.UUID]bool
}

type createWorkspaceArgs struct {
	TemplateID string            `json:"template_id"`
	Name       string            `json:"name,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// CreateWorkspace returns a tool that creates a new workspace from a
// template. The tool is idempotent: if the chat already has a
// workspace that is building or running, it returns the existing
// workspace instead of creating a new one. A mutex prevents parallel
// calls from creating duplicate workspaces.
func CreateWorkspace(options CreateWorkspaceOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"create_workspace",
		"Create a new workspace from a template. Requires a "+
			"template_id (from list_templates). Optionally provide "+
			"a name and parameter values (from read_template). "+
			"If no name is given, one will be generated. "+
			"This tool is idempotent — if the chat already has a "+
			"workspace that is building or running, the existing "+
			"workspace is returned.",
		func(ctx context.Context, args createWorkspaceArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.CreateFn == nil {
				return fantasy.NewTextErrorResponse("workspace creator is not configured"), nil
			}

			templateIDStr := strings.TrimSpace(args.TemplateID)
			if templateIDStr == "" {
				return fantasy.NewTextErrorResponse("template_id is required; use list_templates to find one"), nil
			}
			templateID, err := uuid.Parse(templateIDStr)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("invalid template_id: %w", err).Error(),
				), nil
			}

			if !isTemplateAllowed(options.AllowedTemplateIDs, templateID) {
				return fantasy.NewTextErrorResponse("template not available for chat workspaces; use list_templates to find allowed templates"), nil
			}

			// Serialize workspace creation to prevent parallel
			// tool calls from creating duplicate workspaces.
			if options.WorkspaceMu != nil {
				options.WorkspaceMu.Lock()
				defer options.WorkspaceMu.Unlock()
			}

			// Check for an existing workspace on the chat.
			existing, done, existErr := options.checkExistingWorkspace(ctx)
			if existErr != nil {
				return fantasy.NewTextErrorResponse(existErr.Error()), nil
			}
			if done {
				return toolResponse(existing), nil
			}
			ownerID := options.OwnerID

			// Set up dbauthz context for DB lookups.
			if options.DB != nil {
				ownerCtx, ownerErr := asOwner(ctx, options.DB, ownerID)
				if ownerErr != nil {
					return fantasy.NewTextErrorResponse(ownerErr.Error()), nil
				}
				ctx = ownerCtx
			}

			var ttlMs *int64
			if options.DB != nil {
				raw, err := options.DB.GetChatWorkspaceTTL(ctx)
				if err != nil {
					options.Logger.Error(ctx, "failed to read chat workspace TTL setting, using template default",
						slog.Error(err),
					)
				} else {
					d, parseErr := codersdk.ParseChatWorkspaceTTL(raw)
					if parseErr != nil {
						options.Logger.Warn(ctx, "invalid chat workspace TTL setting, using template default",
							slog.F("raw", raw),
							slog.Error(parseErr),
						)
					} else if d > 0 {
						ms := d.Milliseconds()
						ttlMs = &ms
					}
				}
			}

			createReq := codersdk.CreateWorkspaceRequest{
				TemplateID: templateID,
				TTLMillis:  ttlMs,
			}

			// Resolve workspace name.
			name := strings.TrimSpace(args.Name)
			if name == "" {
				seed := "workspace"
				if options.DB != nil {
					if t, lookupErr := options.DB.GetTemplateByID(ctx, templateID); lookupErr == nil {
						seed = t.Name
					}
				}
				name = generatedWorkspaceName(seed)
			} else if err := codersdk.NameValid(name); err != nil {
				name = generatedWorkspaceName(name)
			}
			createReq.Name = name

			// Map parameters.
			for k, v := range args.Parameters {
				createReq.RichParameterValues = append(
					createReq.RichParameterValues,
					codersdk.WorkspaceBuildParameter{Name: k, Value: v},
				)
			}

			workspace, err := options.CreateFn(ctx, ownerID, createReq)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			// Persist the workspace binding immediately so that
			// workspace-dependent tools can discover the workspace
			// and wait for the build via getWorkspaceConn.
			if options.DB != nil && options.ChatID != uuid.Nil {
				updatedChat, err := options.DB.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
					ID: options.ChatID,
					WorkspaceID: uuid.NullUUID{
						UUID:  workspace.ID,
						Valid: true,
					},
					// BuildID and AgentID are intentionally left null
					// here. The chatd runtime (loadWorkspaceAgentLocked)
					// will bind them on the next turn. Authoritative
					// tool-path binding is deferred to a follow-up PR.
					BuildID: uuid.NullUUID{},
					AgentID: uuid.NullUUID{},
				})
				if err != nil {
					options.Logger.Error(ctx, "failed to persist chat workspace association",
						slog.F("chat_id", options.ChatID),
						slog.F("workspace_id", workspace.ID),
						slog.Error(err),
					)
				} else if options.OnChatUpdated != nil {
					options.OnChatUpdated(updatedChat)
				}
			}

			// Return immediately — workspace tools will
			// transparently wait for the build to complete via
			// getWorkspaceConn when they are actually invoked.
			return toolResponse(map[string]any{
				"created":        true,
				"workspace_name": workspace.FullName(),
				"status":         "building",
				"message":        "Workspace build started. Workspace tools will wait for it automatically.",
			}), nil
		})
}

// checkExistingWorkspace checks whether the configured chat already has
// a usable workspace. Returns the result map and true if the caller
// should return early (workspace exists and is alive or building).
// Returns false if the caller should proceed with creation (workspace
// is dead or missing).
func (o CreateWorkspaceOptions) checkExistingWorkspace(
	ctx context.Context,
) (map[string]any, bool, error) {
	if o.DB == nil || o.ChatID == uuid.Nil {
		return nil, false, nil
	}

	db := o.DB
	chatID := o.ChatID
	agentConnFn := o.AgentConnFn
	agentInactiveDisconnectTimeout := o.AgentInactiveDisconnectTimeout

	chat, err := db.GetChatByID(ctx, chatID)
	if err != nil {
		return nil, false, xerrors.Errorf("load chat: %w", err)
	}
	if !chat.WorkspaceID.Valid {
		return nil, false, nil
	}

	ws, err := db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		return nil, false, xerrors.Errorf("load workspace: %w", err)
	}
	// Workspace was soft-deleted — allow creation.
	if ws.Deleted {
		return nil, false, nil
	}

	// Check the latest build status.
	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
	if err != nil {
		// Can't determine status — allow creation.
		return nil, false, nil
	}

	job, err := db.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return nil, false, nil
	}

	switch job.JobStatus {
	case database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusRunning:
		// Build is in progress — return immediately so the
		// agent can continue working. Workspace tools will
		// wait for the build via getWorkspaceConn.
		return map[string]any{
			"created":        false,
			"workspace_name": ws.OwnerUsername + "/" + ws.Name,
			"status":         "building",
			"message":        "Workspace is currently building. Workspace tools will wait for it automatically.",
		}, true, nil

	case database.ProvisionerJobStatusSucceeded:
		// If the workspace was stopped, tell the model to use
		// start_workspace instead of creating a new one.
		if build.Transition == database.WorkspaceTransitionStop {
			return map[string]any{
				"created":        false,
				"workspace_name": ws.Name,
				"status":         "stopped",
				"message":        "workspace is stopped; use start_workspace to start it",
			}, true, nil
		}

		// Build succeeded — use the agent's recent DB-backed
		// connection status to decide whether the workspace is
		// still usable.
		agents, agentsErr := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
		if agentsErr == nil && len(agents) > 0 {
			status := agents[0].Status(agentInactiveDisconnectTimeout)
			result := map[string]any{
				"created":        false,
				"workspace_name": ws.Name,
				"status":         "already_exists",
			}

			switch status.Status {
			case database.WorkspaceAgentStatusConnected:
				result["message"] = "workspace is already running and recently connected"
				for k, v := range WaitForAgentReady(ctx, db, agents[0].ID, nil) {
					result[k] = v
				}
				return result, true, nil
			case database.WorkspaceAgentStatusConnecting:
				result["message"] = "workspace exists and the agent is still connecting"
				for k, v := range WaitForAgentReady(ctx, db, agents[0].ID, agentConnFn) {
					result[k] = v
				}
				return result, true, nil
			case database.WorkspaceAgentStatusDisconnected,
				database.WorkspaceAgentStatusTimeout:
				// Agent is offline or never became ready — allow
				// creation.
			}
		}
		// No agent ID or no agent status — allow creation.
		return nil, false, nil

	default:
		// Failed, canceled, etc — allow creation.
		return nil, false, nil
	}
}

func generatedWorkspaceName(seed string) string {
	base := codersdk.UsernameFrom(strings.TrimSpace(strings.ToLower(seed)))
	if strings.TrimSpace(base) == "" {
		base = "workspace"
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	if len(base) > 27 {
		base = strings.Trim(base[:27], "-")
	}
	if base == "" {
		base = "workspace"
	}

	name := fmt.Sprintf("%s-%s", base, suffix)
	if err := codersdk.NameValid(name); err == nil {
		return name
	}
	return namesgenerator.NameDigitWith("-")
}
