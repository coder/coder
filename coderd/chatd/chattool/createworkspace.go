package chattool

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// buildPollInterval is how often we check if the workspace
	// build has completed.
	buildPollInterval = 2 * time.Second
	// buildTimeout is the maximum time to wait for a workspace
	// build to complete before giving up.
	buildTimeout = 10 * time.Minute
	// agentConnectTimeout is the maximum time to wait for the
	// workspace agent to become reachable after a successful build.
	agentConnectTimeout = 2 * time.Minute
	// agentRetryInterval is how often we retry connecting to the
	// workspace agent.
	agentRetryInterval = 2 * time.Second
	// agentAttemptTimeout is the timeout for a single connection
	// attempt to the workspace agent during the retry loop.
	agentAttemptTimeout = 5 * time.Second
	// agentPingTimeout is the timeout for a single agent ping
	// when checking whether an existing workspace is alive.
	agentPingTimeout = 5 * time.Second
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
	DB          database.Store
	OwnerID     uuid.UUID
	ChatID      uuid.UUID
	CreateFn    CreateWorkspaceFn
	AgentConnFn AgentConnFunc
	WorkspaceMu *sync.Mutex
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

			// Serialize workspace creation to prevent parallel
			// tool calls from creating duplicate workspaces.
			if options.WorkspaceMu != nil {
				options.WorkspaceMu.Lock()
				defer options.WorkspaceMu.Unlock()
			}

			// Check for an existing workspace on the chat.
			if options.DB != nil && options.ChatID != uuid.Nil {
				existing, done, existErr := checkExistingWorkspace(
					ctx, options.DB, options.ChatID,
					options.AgentConnFn,
				)
				if existErr != nil {
					return fantasy.NewTextErrorResponse(existErr.Error()), nil
				}
				if done {
					return toolResponse(existing), nil
				}
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

			createReq := codersdk.CreateWorkspaceRequest{
				TemplateID: templateID,
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

			// Wait for the build to complete and the agent to
			// come online so subsequent tools can use the
			// workspace immediately.
			if options.DB != nil {
				if err := waitForBuild(ctx, options.DB, workspace.ID); err != nil {
					return fantasy.NewTextErrorResponse(
						xerrors.Errorf("workspace build failed: %w", err).Error(),
					), nil
				}
			}

			// Look up the first agent so we can link it to the chat.
			workspaceAgentID := uuid.Nil
			if options.DB != nil {
				agents, agentErr := options.DB.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
				if agentErr == nil && len(agents) > 0 {
					workspaceAgentID = agents[0].ID
				}
			}

			// Persist workspace + agent association on the chat.
			if options.DB != nil && options.ChatID != uuid.Nil {
				_, _ = options.DB.UpdateChatWorkspace(ctx, database.UpdateChatWorkspaceParams{
					ID: options.ChatID,
					WorkspaceID: uuid.NullUUID{
						UUID:  workspace.ID,
						Valid: true,
					},
				})
			}

			// Wait for the agent to come online.
			if workspaceAgentID != uuid.Nil && options.AgentConnFn != nil {
				if err := waitForAgent(ctx, options.AgentConnFn, workspaceAgentID); err != nil {
					// Non-fatal: the workspace was created
					// successfully, the agent just isn't ready
					// yet. The model can retry.
					return toolResponse(map[string]any{
						"created":        true,
						"workspace_name": workspace.FullName(),
						"agent_status":   "not_ready",
						"agent_error":    err.Error(),
					}), nil
				}
			}

			return toolResponse(map[string]any{
				"created":        true,
				"workspace_name": workspace.FullName(),
			}), nil
		},
	)
}

// checkExistingWorkspace checks whether the chat already has a usable
// workspace. Returns the result map and true if the caller should
// return early (workspace exists and is alive or building). Returns
// false if the caller should proceed with creation (workspace is dead
// or missing).
func checkExistingWorkspace(
	ctx context.Context,
	db database.Store,
	chatID uuid.UUID,
	agentConnFn AgentConnFunc,
) (map[string]any, bool, error) {
	chat, err := db.GetChatByID(ctx, chatID)
	if err != nil {
		return nil, false, xerrors.Errorf("load chat: %w", err)
	}
	if !chat.WorkspaceID.Valid {
		return nil, false, nil
	}

	// Check if workspace still exists.
	ws, err := db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// Workspace was deleted — allow creation.
			return nil, false, nil
		}
		return nil, false, xerrors.Errorf("load workspace: %w", err)
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
		// Build is in progress — wait for it instead of
		// creating a new workspace.
		if err := waitForBuild(ctx, db, ws.ID); err != nil {
			return nil, false, xerrors.Errorf(
				"existing workspace build failed: %w", err,
			)
		}
		return map[string]any{
			"created":        false,
			"workspace_name": ws.Name,
			"status":         "already_exists",
			"message":        "workspace was already being built and is now ready",
		}, true, nil

	case database.ProvisionerJobStatusSucceeded:
		// Build succeeded — check if agent is reachable.
		agents, agentsErr := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
		if agentsErr == nil && len(agents) > 0 && agentConnFn != nil {
			pingCtx, cancel := context.WithTimeout(
				ctx, agentPingTimeout,
			)
			defer cancel()

			conn, release, connErr := agentConnFn(
				pingCtx, agents[0].ID,
			)
			if connErr == nil {
				release()
				_ = conn
				return map[string]any{
					"created":        false,
					"workspace_name": ws.Name,
					"status":         "already_exists",
					"message":        "workspace is already running and reachable",
				}, true, nil
			}
			// Agent unreachable — workspace is dead, allow
			// creation.
		}
		// No agent ID or no conn func — allow creation.
		return nil, false, nil

	default:
		// Failed, canceled, etc — allow creation.
		return nil, false, nil
	}
}

// waitForBuild polls the workspace's latest build until it
// completes or the context expires.
func waitForBuild(
	ctx context.Context,
	db database.Store,
	workspaceID uuid.UUID,
) error {
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	ticker := time.NewTicker(buildPollInterval)
	defer ticker.Stop()

	for {
		build, err := db.GetLatestWorkspaceBuildByWorkspaceID(
			buildCtx, workspaceID,
		)
		if err != nil {
			return xerrors.Errorf("get latest build: %w", err)
		}

		job, err := db.GetProvisionerJobByID(buildCtx, build.JobID)
		if err != nil {
			return xerrors.Errorf("get provisioner job: %w", err)
		}

		switch job.JobStatus {
		case database.ProvisionerJobStatusSucceeded:
			return nil
		case database.ProvisionerJobStatusFailed:
			errMsg := "build failed"
			if job.Error.Valid {
				errMsg = job.Error.String
			}
			return xerrors.New(errMsg)
		case database.ProvisionerJobStatusCanceled:
			return xerrors.New("build was canceled")
		case database.ProvisionerJobStatusPending,
			database.ProvisionerJobStatusRunning,
			database.ProvisionerJobStatusCanceling:
			// Still in progress — keep waiting.
		default:
			return xerrors.Errorf("unexpected job status: %s", job.JobStatus)
		}

		select {
		case <-buildCtx.Done():
			return xerrors.Errorf(
				"timed out waiting for workspace build: %w",
				buildCtx.Err(),
			)
		case <-ticker.C:
		}
	}
}

// waitForAgent retries connecting to the workspace agent until it
// succeeds or the timeout expires.
func waitForAgent(
	ctx context.Context,
	agentConnFn AgentConnFunc,
	agentID uuid.UUID,
) error {
	agentCtx, cancel := context.WithTimeout(ctx, agentConnectTimeout)
	defer cancel()

	ticker := time.NewTicker(agentRetryInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(agentCtx, agentAttemptTimeout)
		conn, release, err := agentConnFn(attemptCtx, agentID)
		attemptCancel()
		if err == nil {
			release()
			_ = conn
			return nil
		}
		lastErr = err

		select {
		case <-agentCtx.Done():
			return xerrors.Errorf(
				"timed out waiting for workspace agent: %w",
				lastErr,
			)
		case <-ticker.C:
		}
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
