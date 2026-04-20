package chattool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	"github.com/coder/coder/v2/coderd/x/chatd/internal/agentselect"
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
	// startupScriptTimeout is the maximum time to wait for the
	// workspace agent's startup scripts to finish after the agent
	// is reachable.
	startupScriptTimeout = 10 * time.Minute
	// startupScriptPollInterval is how often we check the agent's
	// lifecycle state while waiting for startup scripts.
	startupScriptPollInterval = 2 * time.Second
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
func CreateWorkspace(organizationID uuid.UUID, db database.Store, options CreateWorkspaceOptions) fantasy.AgentTool {
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
			if db == nil {
				return fantasy.NewTextErrorResponse("database is not configured"), nil
			}
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
			check := options.checkExistingWorkspace(ctx, db)
			if check.Err != nil {
				if check.FailedBuildID != uuid.Nil {
					return buildToolResponse(newBuildError(check.Err.Error(), check.FailedBuildID)), nil
				}
				return fantasy.NewTextErrorResponse(check.Err.Error()), nil
			}
			if check.Done {
				return toolResponse(check.Result), nil
			}
			ownerID := options.OwnerID

			// Set up dbauthz context for DB lookups.
			ownerCtx, ownerErr := asOwner(ctx, db, ownerID)
			if ownerErr != nil {
				return fantasy.NewTextErrorResponse(ownerErr.Error()), nil
			}
			ctx = ownerCtx

			// Verify the template belongs to the same org as the
			// chat. Without this check the tool could silently
			// bind a cross-org workspace to the chat.
			tmpl, tmplErr := db.GetTemplateByID(ctx, templateID)
			if tmplErr != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("look up template: %w", tmplErr).Error(),
				), nil
			}
			if tmpl.OrganizationID != organizationID {
				return fantasy.NewTextErrorResponse(
					"template belongs to a different organization than this chat; " +
						"use list_templates to find templates in the correct organization",
				), nil
			}

			var ttlMs *int64
			raw, err := db.GetChatWorkspaceTTL(ctx)
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

			createReq := codersdk.CreateWorkspaceRequest{
				TemplateID: templateID,
				TTLMillis:  ttlMs,
			}

			name := strings.TrimSpace(args.Name)
			if name == "" {
				name = generatedWorkspaceName(tmpl.Name)
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

			// Persist the workspace binding on the chat
			// immediately so the frontend can start streaming
			// build logs while the build is still running.
			// Note: this binding is intentional even if the build
			// later fails. The checkExistingWorkspace recovery
			// path handles failed workspaces by allowing
			// re-creation.
			if options.ChatID != uuid.Nil {
				updatedChat, err := db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
					ID: options.ChatID,
					WorkspaceID: uuid.NullUUID{
						UUID:  workspace.ID,
						Valid: true,
					},
					BuildID: uuid.NullUUID{
						UUID:  workspace.LatestBuild.ID,
						Valid: workspace.LatestBuild.ID != uuid.Nil,
					},
					// AgentID is left null because the build hasn't
					// completed yet. The chatd runtime binds it once
					// the agent comes online.
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

			// Wait for the build to complete and the agent to
			// come online so subsequent tools can use the
			// workspace immediately.
			buildID := workspace.LatestBuild.ID
			if buildID != uuid.Nil {
				if err := waitForBuild(ctx, db, buildID); err != nil {
					return buildToolResponse(newBuildError(
						xerrors.Errorf("workspace build failed: %w", err).Error(),
						buildID,
					)), nil
				}
			}

			result := map[string]any{
				"created":        true,
				"workspace_name": workspace.FullName(),
			}
			setBuildID(result, buildID)

			// Select the chat agent so follow-up tools wait on the
			// intended workspace agent.
			workspaceAgentID := uuid.Nil
			agents, agentErr := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
			if agentErr == nil {
				if len(agents) == 0 {
					result["agent_status"] = "no_agent"
				} else {
					selected, selectErr := agentselect.FindChatAgent(agents)
					if selectErr != nil {
						result["agent_status"] = "selection_error"
						result["agent_error"] = selectErr.Error()
					} else {
						workspaceAgentID = selected.ID
					}
				}
			}

			// Wait for the agent to come online and startup scripts to finish.
			if workspaceAgentID != uuid.Nil {
				agentStatus := waitForAgentReady(ctx, db, workspaceAgentID, options.AgentConnFn)
				for k, v := range agentStatus {
					result[k] = v
				}
			}

			// Re-fire after the agent is fully ready so callers
			// can load instruction files (AGENTS.md) from the
			// running agent. This must happen after
			// waitForAgentReady — firing earlier (e.g. right
			// after waitForBuild) races with the agent startup
			// and the connection usually times out before the
			// agent is reachable.
			if options.OnChatUpdated != nil {
				if latest, err := db.GetChatByID(ctx, options.ChatID); err == nil {
					options.OnChatUpdated(latest)
				}
			}

			return toolResponse(result), nil
		})
}

// existingWorkspaceResult holds the outcome of checking for an
// existing workspace on the chat.
type existingWorkspaceResult struct {
	// Result is the tool response map when Done is true.
	Result map[string]any
	// Done indicates the caller should return early.
	Done bool
	// FailedBuildID is set when waitForBuild failed, so the
	// caller can include it in a structured error response.
	FailedBuildID uuid.UUID
	// Err is non-nil when the check itself failed.
	Err error
}

// checkExistingWorkspace checks whether the configured chat
// already has a usable workspace. Returns an
// existingWorkspaceResult with Done set when the caller should
// return early (workspace exists and is alive or building).
// Returns Done unset if the caller should proceed with creation
// (workspace is dead or missing).
func (o CreateWorkspaceOptions) checkExistingWorkspace(
	ctx context.Context,
	db database.Store,
) existingWorkspaceResult {
	if o.ChatID == uuid.Nil {
		return existingWorkspaceResult{}
	}

	chatID := o.ChatID
	agentConnFn := o.AgentConnFn
	agentInactiveDisconnectTimeout := o.AgentInactiveDisconnectTimeout

	chat, err := db.GetChatByID(ctx, chatID)
	if err != nil {
		return existingWorkspaceResult{Err: xerrors.Errorf("load chat: %w", err)}
	}
	if !chat.WorkspaceID.Valid {
		return existingWorkspaceResult{}
	}

	ws, err := db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		return existingWorkspaceResult{Err: xerrors.Errorf("load workspace: %w", err)}
	}
	// Workspace was soft-deleted — allow creation.
	if ws.Deleted {
		return existingWorkspaceResult{}
	}

	// Check the latest build status.
	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
	if err != nil {
		// Can't determine status — allow creation.
		return existingWorkspaceResult{}
	}

	job, err := db.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return existingWorkspaceResult{}
	}

	switch job.JobStatus {
	case database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusRunning:
		// Build is in progress. Publish the build ID so the
		// frontend can start streaming logs, then wait.
		updatedChat, bindErr := db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
			ID:          o.ChatID,
			WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
			BuildID: uuid.NullUUID{
				UUID:  build.ID,
				Valid: build.ID != uuid.Nil,
			},
			AgentID: uuid.NullUUID{},
		})
		if bindErr != nil {
			o.Logger.Error(ctx, "failed to persist build ID on chat binding",
				slog.F("chat_id", o.ChatID),
				slog.F("build_id", build.ID),
				slog.Error(bindErr),
			)
		} else if o.OnChatUpdated != nil {
			o.OnChatUpdated(updatedChat)
		}
		if err := waitForBuild(ctx, db, build.ID); err != nil {
			return existingWorkspaceResult{
				FailedBuildID: build.ID,
				Err:           xerrors.Errorf("existing workspace build failed: %w", err),
			}
		}
		result := map[string]any{
			"created":        false,
			"workspace_name": ws.Name,
			"status":         "already_exists",
			"message":        "workspace build completed",
		}
		setBuildID(result, build.ID)
		agents, agentsErr := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
		if agentsErr == nil && len(agents) > 0 {
			selected, selectErr := agentselect.FindChatAgent(agents)
			if selectErr != nil {
				o.Logger.Debug(ctx, "agent selection failed, falling back to first agent for readiness check",
					slog.F("workspace_id", ws.ID),
					slog.Error(selectErr),
				)
				selected = agents[0]
			}
			for k, v := range waitForAgentReady(ctx, db, selected.ID, agentConnFn) {
				result[k] = v
			}
		}
		return existingWorkspaceResult{Result: result, Done: true}

	case database.ProvisionerJobStatusSucceeded:
		// If the workspace was stopped, tell the model to use
		// start_workspace instead of creating a new one.
		if build.Transition == database.WorkspaceTransitionStop {
			return existingWorkspaceResult{Result: map[string]any{
				"created":        false,
				"workspace_name": ws.Name,
				"status":         "stopped",
				"message":        "workspace is stopped; use start_workspace to start it",
			}, Done: true}
		}

		// Build succeeded — use the agent's recent DB-backed
		// connection status to decide whether the workspace is
		// still usable.
		agents, agentsErr := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, ws.ID)
		if agentsErr == nil && len(agents) > 0 {
			selected, selectErr := agentselect.FindChatAgent(agents)
			if selectErr != nil {
				o.Logger.Debug(ctx, "agent selection failed, falling back to first agent for status check",
					slog.F("workspace_id", ws.ID),
					slog.Error(selectErr),
				)
				selected = agents[0]
			}
			status := selected.Status(dbtime.Now(), agentInactiveDisconnectTimeout)
			result := map[string]any{
				"created":        false,
				"workspace_name": ws.Name,
				"status":         "already_exists",
			}

			switch status.Status {
			case database.WorkspaceAgentStatusConnected:
				result["message"] = "workspace is already running and recently connected"
				for k, v := range waitForAgentReady(ctx, db, selected.ID, nil) {
					result[k] = v
				}
				return existingWorkspaceResult{Result: result, Done: true}
			case database.WorkspaceAgentStatusConnecting:
				result["message"] = "workspace exists and the agent is still connecting"
				for k, v := range waitForAgentReady(ctx, db, selected.ID, agentConnFn) {
					result[k] = v
				}
				return existingWorkspaceResult{Result: result, Done: true}
			case database.WorkspaceAgentStatusDisconnected,
				database.WorkspaceAgentStatusTimeout:
				// Agent is offline or never became ready - allow
				// creation.
			}
		}
		// No agent ID or no agent status — allow creation.
		return existingWorkspaceResult{}

	default:
		// Failed, canceled, etc — allow creation.
		return existingWorkspaceResult{}
	}
}

// waitForBuild polls the specified build until its provisioner job
// completes or the context expires.
func waitForBuild(
	ctx context.Context,
	db database.Store,
	buildID uuid.UUID,
) error {
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	ticker := time.NewTicker(buildPollInterval)
	defer ticker.Stop()

	for {
		build, err := db.GetWorkspaceBuildByID(buildCtx, buildID)
		if err != nil {
			return xerrors.Errorf("get build: %w", err)
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

// waitForAgentReady waits for the workspace agent to become
// reachable and for its startup scripts to finish. It returns
// status fields suitable for merging into a tool response.
func waitForAgentReady(
	ctx context.Context,
	db database.Store,
	agentID uuid.UUID,
	agentConnFn AgentConnFunc,
) map[string]any {
	result := map[string]any{}

	// Phase 1: retry connecting to the agent.
	if agentConnFn != nil {
		agentCtx, agentCancel := context.WithTimeout(ctx, agentConnectTimeout)
		defer agentCancel()

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
				break
			}
			lastErr = err

			select {
			case <-agentCtx.Done():
				result["agent_status"] = "not_ready"
				result["agent_error"] = lastErr.Error()
				return result
			case <-ticker.C:
			}
		}
	}

	// Phase 2: poll lifecycle until startup scripts finish.
	if db != nil {
		scriptCtx, scriptCancel := context.WithTimeout(ctx, startupScriptTimeout)
		defer scriptCancel()

		ticker := time.NewTicker(startupScriptPollInterval)
		defer ticker.Stop()

		var lastState database.WorkspaceAgentLifecycleState
		for {
			row, err := db.GetWorkspaceAgentLifecycleStateByID(scriptCtx, agentID)
			if err == nil {
				lastState = row.LifecycleState
				switch lastState {
				case database.WorkspaceAgentLifecycleStateCreated,
					database.WorkspaceAgentLifecycleStateStarting:
					// Still in progress, keep polling.
				case database.WorkspaceAgentLifecycleStateReady:
					return result
				default:
					// Terminal non-ready state.
					result["startup_scripts"] = "startup_scripts_failed"
					result["lifecycle_state"] = string(lastState)
					return result
				}
			}

			select {
			case <-scriptCtx.Done():
				if errors.Is(scriptCtx.Err(), context.DeadlineExceeded) {
					result["startup_scripts"] = "startup_scripts_timeout"
				} else {
					result["startup_scripts"] = "startup_scripts_unknown"
				}
				return result
			case <-ticker.C:
			}
		}
	}

	return result
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
