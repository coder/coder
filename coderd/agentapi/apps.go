package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/wspubsub"
)

type AppsAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	Agent                    *CachedAgentFields
	Database                 database.Store
	Log                      slog.Logger
	PublishWorkspaceUpdateFn func(context.Context, *CachedAgentFields, wspubsub.WorkspaceEventKind) error
}

func (a *AppsAPI) BatchUpdateAppHealths(ctx context.Context, req *agentproto.BatchUpdateAppHealthRequest) (*agentproto.BatchUpdateAppHealthResponse, error) {
	// Use cached agent ID if available to avoid database query.
	agentID, ok := a.Agent.AsAgentID()
	var agentCache *CachedAgentFields
	if !ok {
		// Fallback to querying the agent if cache is not populated.
		workspaceAgent, err := a.AgentFn(ctx)
		if err != nil {
			return nil, err
		}
		agentID = workspaceAgent.ID
		// Create a cache from the fallback agent for publishing.
		agentCache = &CachedAgentFields{}
		agentCache.UpdateValues(workspaceAgent.ID, workspaceAgent.Name)
	} else {
		// Use the existing cache.
		agentCache = a.Agent
	}

	a.Log.Debug(ctx, "got batch app health update",
		slog.F("agent_id", agentID.String()),
		slog.F("updates", req.Updates),
	)

	if len(req.Updates) == 0 {
		return &agentproto.BatchUpdateAppHealthResponse{}, nil
	}

	apps, err := a.Database.GetWorkspaceAppsByAgentID(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace apps by agent ID %q: %w", agentID, err)
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
		// Use the agent cache (either from the API struct or created from fallback).
		err = a.PublishWorkspaceUpdateFn(ctx, agentCache, wspubsub.WorkspaceEventKindAppHealthUpdate)
		if err != nil {
			return nil, xerrors.Errorf("publish workspace update: %w", err)
		}
	}
	return &agentproto.BatchUpdateAppHealthResponse{}, nil
}
