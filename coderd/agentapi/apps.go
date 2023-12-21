package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type AppsAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	Database                 database.Store
	Log                      slog.Logger
	PublishWorkspaceUpdateFn func(context.Context, *database.WorkspaceAgent) error
}

func (a *AppsAPI) BatchUpdateAppHealths(ctx context.Context, req *agentproto.BatchUpdateAppHealthRequest) (*agentproto.BatchUpdateAppHealthResponse, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

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
			return nil, xerrors.Errorf("update workspace app health for app %q (%q): %w", err, app.ID, app.Slug)
		}
	}

	err = a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent)
	if err != nil {
		return nil, xerrors.Errorf("publish workspace update: %w", err)
	}
	return &agentproto.BatchUpdateAppHealthResponse{}, nil
}
