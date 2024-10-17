package agentapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/wspubsub"
)

type contextKeyAPIVersion struct{}

func WithAPIVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, contextKeyAPIVersion{}, version)
}

type LifecycleAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	WorkspaceID              uuid.UUID
	Database                 database.Store
	Log                      slog.Logger
	PublishWorkspaceUpdateFn func(context.Context, *database.WorkspaceAgent, wspubsub.WorkspaceEventKind) error

	TimeNowFn func() time.Time // defaults to dbtime.Now()
}

func (a *LifecycleAPI) now() time.Time {
	if a.TimeNowFn != nil {
		return a.TimeNowFn()
	}
	return dbtime.Now()
}

func (a *LifecycleAPI) UpdateLifecycle(ctx context.Context, req *agentproto.UpdateLifecycleRequest) (*agentproto.Lifecycle, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	logger := a.Log.With(
		slog.F("workspace_id", a.WorkspaceID),
		slog.F("payload", req),
	)
	logger.Debug(ctx, "workspace agent state report")

	var lifecycleState database.WorkspaceAgentLifecycleState
	switch req.Lifecycle.State {
	case agentproto.Lifecycle_CREATED:
		lifecycleState = database.WorkspaceAgentLifecycleStateCreated
	case agentproto.Lifecycle_STARTING:
		lifecycleState = database.WorkspaceAgentLifecycleStateStarting
	case agentproto.Lifecycle_START_TIMEOUT:
		lifecycleState = database.WorkspaceAgentLifecycleStateStartTimeout
	case agentproto.Lifecycle_START_ERROR:
		lifecycleState = database.WorkspaceAgentLifecycleStateStartError
	case agentproto.Lifecycle_READY:
		lifecycleState = database.WorkspaceAgentLifecycleStateReady
	case agentproto.Lifecycle_SHUTTING_DOWN:
		lifecycleState = database.WorkspaceAgentLifecycleStateShuttingDown
	case agentproto.Lifecycle_SHUTDOWN_TIMEOUT:
		lifecycleState = database.WorkspaceAgentLifecycleStateShutdownTimeout
	case agentproto.Lifecycle_SHUTDOWN_ERROR:
		lifecycleState = database.WorkspaceAgentLifecycleStateShutdownError
	case agentproto.Lifecycle_OFF:
		lifecycleState = database.WorkspaceAgentLifecycleStateOff
	default:
		return nil, xerrors.Errorf("unknown lifecycle state %q", req.Lifecycle.State)
	}
	if !lifecycleState.Valid() {
		return nil, xerrors.Errorf("unknown lifecycle state %q", req.Lifecycle.State)
	}

	changedAt := req.Lifecycle.ChangedAt.AsTime()
	if changedAt.IsZero() {
		changedAt = a.now()
		req.Lifecycle.ChangedAt = timestamppb.New(changedAt)
	}
	dbChangedAt := sql.NullTime{Time: changedAt, Valid: true}

	startedAt := workspaceAgent.StartedAt
	readyAt := workspaceAgent.ReadyAt
	switch lifecycleState {
	case database.WorkspaceAgentLifecycleStateStarting:
		startedAt = dbChangedAt
		// This agent is (re)starting, so it's not ready yet.
		readyAt.Time = time.Time{}
		readyAt.Valid = false
	case database.WorkspaceAgentLifecycleStateReady,
		database.WorkspaceAgentLifecycleStateStartTimeout,
		database.WorkspaceAgentLifecycleStateStartError:
		if !startedAt.Valid {
			startedAt = dbChangedAt
		}
		readyAt = dbChangedAt
	}

	err = a.Database.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             workspaceAgent.ID,
		LifecycleState: lifecycleState,
		StartedAt:      startedAt,
		ReadyAt:        readyAt,
	})
	if err != nil {
		if !database.IsQueryCanceledError(err) {
			// not an error if we are canceled
			logger.Error(ctx, "failed to update lifecycle state", slog.Error(err))
		}
		return nil, xerrors.Errorf("update workspace agent lifecycle state: %w", err)
	}

	if a.PublishWorkspaceUpdateFn != nil {
		err = a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent, wspubsub.WorkspaceEventKindAgentLifecycleUpdate)
		if err != nil {
			return nil, xerrors.Errorf("publish workspace update: %w", err)
		}
	}

	return req.Lifecycle, nil
}

func (a *LifecycleAPI) UpdateStartup(ctx context.Context, req *agentproto.UpdateStartupRequest) (*agentproto.Startup, error) {
	apiVersion, ok := ctx.Value(contextKeyAPIVersion{}).(string)
	if !ok {
		return nil, xerrors.Errorf("internal error; api version unspecified")
	}
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	a.Log.Debug(
		ctx,
		"post workspace agent version",
		slog.F("workspace_id", a.WorkspaceID),
		slog.F("agent_version", req.Startup.Version),
	)

	if !semver.IsValid(req.Startup.Version) {
		return nil, xerrors.Errorf("invalid agent semver version %q", req.Startup.Version)
	}

	// Validate subsystems.
	dbSubsystems := make([]database.WorkspaceAgentSubsystem, 0, len(req.Startup.Subsystems))
	seenSubsystems := make(map[database.WorkspaceAgentSubsystem]struct{}, len(req.Startup.Subsystems))
	for _, s := range req.Startup.Subsystems {
		var dbSubsystem database.WorkspaceAgentSubsystem
		switch s {
		case agentproto.Startup_ENVBOX:
			dbSubsystem = database.WorkspaceAgentSubsystemEnvbox
		case agentproto.Startup_ENVBUILDER:
			dbSubsystem = database.WorkspaceAgentSubsystemEnvbuilder
		case agentproto.Startup_EXECTRACE:
			dbSubsystem = database.WorkspaceAgentSubsystemExectrace
		default:
			return nil, xerrors.Errorf("invalid agent subsystem %q", s)
		}

		if _, ok := seenSubsystems[dbSubsystem]; !ok {
			seenSubsystems[dbSubsystem] = struct{}{}
			dbSubsystems = append(dbSubsystems, dbSubsystem)
		}
	}
	slices.Sort(dbSubsystems)

	err = a.Database.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                workspaceAgent.ID,
		Version:           req.Startup.Version,
		ExpandedDirectory: req.Startup.ExpandedDirectory,
		Subsystems:        dbSubsystems,
		APIVersion:        apiVersion,
	})
	if err != nil {
		return nil, xerrors.Errorf("update workspace agent startup in database: %w", err)
	}

	return req.Startup, nil
}
