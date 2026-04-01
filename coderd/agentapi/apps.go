package agentapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	strutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

type AppsAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	Database                 database.Store
	Log                      slog.Logger
	PublishWorkspaceUpdateFn func(context.Context, *database.WorkspaceAgent, wspubsub.WorkspaceEventKind) error
	NotificationsEnqueuer    notifications.Enqueuer
	Clock                    quartz.Clock
}

func (a *AppsAPI) BatchUpdateAppHealths(ctx context.Context, req *agentproto.BatchUpdateAppHealthRequest) (*agentproto.BatchUpdateAppHealthResponse, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	a.Log.Debug(ctx, "got batch app health update",
		slog.F("agent_id", workspaceAgent.ID.String()),
		slog.F("updates", req.Updates),
	)

	if len(req.Updates) == 0 {
		return &agentproto.BatchUpdateAppHealthResponse{}, nil
	}

	apps, err := a.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace apps by agent ID %q: %w", workspaceAgent.ID, err)
	}

	var newApps []database.WorkspaceApp
	for _, update := range req.Updates {
		updateID, err := uuid.FromBytes(update.Id)
		if err != nil {
			return nil, xerrors.Errorf("parse workspace app ID %q: %w", update.Id, err)
		}

		old := func() *database.WorkspaceApp {
			for _, app := range apps {
				if app.ID == updateID {
					return &app
				}
			}

			return nil
		}()
		if old == nil {
			return nil, xerrors.Errorf("workspace app ID %q not found", updateID)
		}

		if old.HealthcheckUrl == "" {
			return nil, xerrors.Errorf("workspace app %q (%q) does not have healthchecks enabled", updateID, old.Slug)
		}

		var newHealth database.WorkspaceAppHealth
		switch update.Health {
		case agentproto.AppHealth_DISABLED:
			newHealth = database.WorkspaceAppHealthDisabled
		case agentproto.AppHealth_INITIALIZING:
			newHealth = database.WorkspaceAppHealthInitializing
		case agentproto.AppHealth_HEALTHY:
			newHealth = database.WorkspaceAppHealthHealthy
		case agentproto.AppHealth_UNHEALTHY:
			newHealth = database.WorkspaceAppHealthUnhealthy
		default:
			return nil, xerrors.Errorf("unknown health status %q for app %q (%q)", update.Health, updateID, old.Slug)
		}

		// Don't bother updating if the value hasn't changed.
		if old.Health == newHealth {
			continue
		}
		old.Health = newHealth

		newApps = append(newApps, *old)
	}

	for _, app := range newApps {
		err = a.Database.UpdateWorkspaceAppHealthByID(ctx, database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: app.Health,
		})
		if err != nil {
			return nil, xerrors.Errorf("update workspace app health for app %q (%q): %w", app.ID, app.Slug, err)
		}
	}

	if a.PublishWorkspaceUpdateFn != nil && len(newApps) > 0 {
		err = a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent, wspubsub.WorkspaceEventKindAppHealthUpdate)
		if err != nil {
			return nil, xerrors.Errorf("publish workspace update: %w", err)
		}
	}
	return &agentproto.BatchUpdateAppHealthResponse{}, nil
}

func (a *AppsAPI) UpdateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
	if len(req.Message) > 160 {
		return nil, codersdk.NewError(http.StatusBadRequest, codersdk.Response{
			Message: "Message is too long.",
			Detail:  "Message must be less than 160 characters.",
			Validations: []codersdk.ValidationError{
				{Field: "message", Detail: "Message must be less than 160 characters."},
			},
		})
	}

	var dbState database.WorkspaceAppStatusState
	switch req.State {
	case agentproto.UpdateAppStatusRequest_COMPLETE:
		dbState = database.WorkspaceAppStatusStateComplete
	case agentproto.UpdateAppStatusRequest_FAILURE:
		dbState = database.WorkspaceAppStatusStateFailure
	case agentproto.UpdateAppStatusRequest_WORKING:
		dbState = database.WorkspaceAppStatusStateWorking
	case agentproto.UpdateAppStatusRequest_IDLE:
		dbState = database.WorkspaceAppStatusStateIdle
	default:
		return nil, codersdk.NewError(http.StatusBadRequest, codersdk.Response{
			Message: "Invalid state provided.",
			Detail:  fmt.Sprintf("invalid state: %q", req.State),
			Validations: []codersdk.ValidationError{
				{Field: "state", Detail: "State must be one of: complete, failure, working, idle."},
			},
		})
	}

	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}
	app, err := a.Database.GetWorkspaceAppByAgentIDAndSlug(ctx, database.GetWorkspaceAppByAgentIDAndSlugParams{
		AgentID: workspaceAgent.ID,
		Slug:    req.Slug,
	})
	if err != nil {
		return nil, codersdk.NewError(http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace app.",
			Detail:  fmt.Sprintf("No app found with slug %q", req.Slug),
		})
	}

	workspace, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, codersdk.NewError(http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
	}

	// Treat the message as untrusted input.
	cleaned := strutil.UISanitize(req.Message)

	// Get the latest status for the workspace app to detect no-op updates
	// nolint:gocritic // This is a system restricted operation.
	latestAppStatus, err := a.Database.GetLatestWorkspaceAppStatusByAppID(dbauthz.AsSystemRestricted(ctx), app.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, codersdk.NewError(http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get latest workspace app status.",
			Detail:  err.Error(),
		})
	}
	// If no rows found, latestAppStatus will be a zero-value struct (ID == uuid.Nil)

	// nolint:gocritic // This is a system restricted operation.
	_, err = a.Database.InsertWorkspaceAppStatus(dbauthz.AsSystemRestricted(ctx), database.InsertWorkspaceAppStatusParams{
		ID:          uuid.New(),
		CreatedAt:   dbtime.Now(),
		WorkspaceID: workspace.ID,
		AgentID:     workspaceAgent.ID,
		AppID:       app.ID,
		State:       dbState,
		Message:     cleaned,
		Uri: sql.NullString{
			String: req.Uri,
			Valid:  req.Uri != "",
		},
	})
	if err != nil {
		return nil, codersdk.NewError(http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to insert workspace app status.",
			Detail:  err.Error(),
		})
	}

	if a.PublishWorkspaceUpdateFn != nil {
		err = a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent, wspubsub.WorkspaceEventKindAgentAppStatusUpdate)
		if err != nil {
			return nil, codersdk.NewError(http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to publish workspace update.",
				Detail:  err.Error(),
			})
		}
	}

	// Notify on state change to Working/Idle for AI tasks
	a.enqueueAITaskStateNotification(ctx, app.ID, latestAppStatus, dbState, workspace, workspaceAgent)

	if shouldBump(dbState, latestAppStatus) {
		// We pass time.Time{} for nextAutostart since we don't have access to
		// TemplateScheduleStore here. The activity bump logic handles this by
		// defaulting to the template's activity_bump duration (typically 1 hour).
		workspacestats.ActivityBumpWorkspace(ctx, a.Log, a.Database, workspace.ID, time.Time{})
	}
	// just return a blank response because it doesn't contain any settable fields at present.
	return new(agentproto.UpdateAppStatusResponse), nil
}

func shouldBump(dbState database.WorkspaceAppStatusState, latestAppStatus database.WorkspaceAppStatus) bool {
	// Bump deadline when agent reports working or transitions away from working.
	// This prevents auto-pause during active work and gives users time to interact
	// after work completes.

	// Bump if reporting working state.
	if dbState == database.WorkspaceAppStatusStateWorking {
		return true
	}

	// Bump if transitioning away from working state.
	if latestAppStatus.ID != uuid.Nil {
		prevState := latestAppStatus.State
		if prevState == database.WorkspaceAppStatusStateWorking {
			return true
		}
	}
	return false
}

// enqueueAITaskStateNotification enqueues a notification when an AI task's app
// transitions to Working or Idle.
// No-op if:
//   - the workspace agent app isn't configured as an AI task,
//   - the new state equals the latest persisted state,
//   - the workspace agent is not ready (still starting up).
func (a *AppsAPI) enqueueAITaskStateNotification(
	ctx context.Context,
	appID uuid.UUID,
	latestAppStatus database.WorkspaceAppStatus,
	newAppStatus database.WorkspaceAppStatusState,
	workspace database.Workspace,
	agent database.WorkspaceAgent,
) {
	var notificationTemplate uuid.UUID
	switch newAppStatus {
	case database.WorkspaceAppStatusStateWorking:
		notificationTemplate = notifications.TemplateTaskWorking
	case database.WorkspaceAppStatusStateIdle:
		notificationTemplate = notifications.TemplateTaskIdle
	case database.WorkspaceAppStatusStateComplete:
		notificationTemplate = notifications.TemplateTaskCompleted
	case database.WorkspaceAppStatusStateFailure:
		notificationTemplate = notifications.TemplateTaskFailed
	default:
		// Not a notifiable state, do nothing
		return
	}

	if !workspace.TaskID.Valid {
		// Workspace has no task ID, do nothing.
		return
	}

	// Only send notifications when the agent is ready. We want to skip
	// any state transitions that occur whilst the workspace is starting
	// up as it doesn't make sense to receive them.
	if agent.LifecycleState != database.WorkspaceAgentLifecycleStateReady {
		a.Log.Debug(ctx, "skipping AI task notification because agent is not ready",
			slog.F("agent_id", agent.ID),
			slog.F("lifecycle_state", agent.LifecycleState),
			slog.F("new_app_status", newAppStatus),
		)
		return
	}

	task, err := a.Database.GetTaskByID(ctx, workspace.TaskID.UUID)
	if err != nil {
		a.Log.Warn(ctx, "failed to get task", slog.Error(err))
		return
	}

	if !task.WorkspaceAppID.Valid || task.WorkspaceAppID.UUID != appID {
		// Non-task app, do nothing.
		return
	}

	// Skip if the latest persisted state equals the new state (no new transition)
	// Note: uuid.Nil check is valid here. If no previous status exists,
	// GetLatestWorkspaceAppStatusByAppID returns sql.ErrNoRows and we get a zero-value struct.
	if latestAppStatus.ID != uuid.Nil && latestAppStatus.State == newAppStatus {
		return
	}

	// Skip the initial "Working" notification when the task first starts.
	// This is obvious to the user since they just created the task.
	// We still notify on the first "Idle" status and all subsequent transitions.
	if latestAppStatus.ID == uuid.Nil && newAppStatus == database.WorkspaceAppStatusStateWorking {
		return
	}

	if _, err := a.NotificationsEnqueuer.EnqueueWithData(
		// nolint:gocritic // Need notifier actor to enqueue notifications
		dbauthz.AsNotifier(ctx),
		workspace.OwnerID,
		notificationTemplate,
		map[string]string{
			"task":      task.Name,
			"workspace": workspace.Name,
		},
		map[string]any{
			// Use a 1-minute bucketed timestamp to bypass per-day dedupe,
			// allowing identical content to resend within the same day
			// (but not more than once every 10s).
			"dedupe_bypass_ts": a.Clock.Now().UTC().Truncate(time.Minute),
		},
		"api-workspace-agent-app-status",
		// Associate this notification with related entities
		workspace.ID, workspace.OwnerID, workspace.OrganizationID, appID,
	); err != nil {
		a.Log.Warn(ctx, "failed to notify of task state", slog.Error(err))
		return
	}
}
