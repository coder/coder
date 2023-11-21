package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/batchstats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

type AgentAPI struct {
	agentID     uuid.UUID
	workspaceID uuid.UUID

	accessURL                       *url.URL
	appHostname                     string
	agentInactiveDisconnectTimeout  time.Duration
	agentFallbackTroubleshootingURL string
	agentStatsRefreshInterval       time.Duration
	disableDirectConnections        bool
	derpForceWebSockets             bool
	derpMapUpdateFrequency          time.Duration
	externalAuthConfigs             []*externalauth.Config

	ctx                             context.Context
	log                             slog.Logger
	database                        database.Store
	pubsub                          pubsub.Pubsub
	derpMapFn                       func() *tailcfg.DERPMap
	tailnetCoordinator              *atomic.Pointer[tailnet.Coordinator]
	templateScheduleStore           *atomic.Pointer[schedule.TemplateScheduleStore]
	statsBatcher                    *batchstats.Batcher
	publishWorkspaceUpdate          func(ctx context.Context, workspaceID uuid.UUID)
	publishWorkspaceAgentLogsUpdate func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage)

	// Optional:
	updateAgentMetrics func(ctx context.Context, username, workspaceName, agentName string, metrics []*agentproto.Stats_Metric)
}

var _ agentproto.DRPCAgentServer = &AgentAPI{}

func (a *AgentAPI) Server(ctx context.Context) (*drpcserver.Server, error) {
	mux := drpcmux.New()
	err := agentproto.DRPCRegisterAgent(mux, a)
	if err != nil {
		return nil, xerrors.Errorf("register agent API protocol in DRPC mux: %w", err)
	}

	return drpcserver.NewWithOptions(&tracing.DRPCHandler{Handler: mux},
		drpcserver.Options{
			Log: func(err error) {
				if xerrors.Is(err, io.EOF) {
					return
				}
				a.log.Debug(ctx, "drpc server error", slog.Error(err))
			},
		},
	), nil
}

func (a *AgentAPI) Serve(ctx context.Context, l net.Listener) error {
	server, err := a.Server(ctx)
	if err != nil {
		return xerrors.Errorf("create agent API server: %w", err)
	}

	return server.Serve(ctx, l)
}

func (a *AgentAPI) agent(ctx context.Context) (database.WorkspaceAgent, error) {
	agent, err := a.database.GetWorkspaceAgentByID(ctx, a.agentID)
	if err != nil {
		return database.WorkspaceAgent{}, xerrors.Errorf("get workspace agent by id %q: %w", a.agentID, err)
	}
	return agent, nil
}

func (a *AgentAPI) GetManifest(ctx context.Context, _ *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}

	apiAgent, err := convertWorkspaceAgent(
		a.derpMapFn(), *a.tailnetCoordinator.Load(), workspaceAgent, nil, nil, nil, a.agentInactiveDisconnectTimeout,
		a.agentFallbackTroubleshootingURL,
	)
	if err != nil {
		return nil, xerrors.Errorf("converting workspace agent: %w", err)
	}

	var (
		dbApps    []database.WorkspaceApp
		scripts   []database.WorkspaceAgentScript
		metadata  []database.WorkspaceAgentMetadatum
		resource  database.WorkspaceResource
		build     database.WorkspaceBuild
		workspace database.Workspace
		owner     database.User
	)

	var eg errgroup.Group
	eg.Go(func() (err error) {
		dbApps, err = a.database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	eg.Go(func() (err error) {
		// nolint:gocritic // This is necessary to fetch agent scripts!
		scripts, err = a.database.GetWorkspaceAgentScriptsByAgentIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{workspaceAgent.ID})
		return err
	})
	eg.Go(func() (err error) {
		metadata, err = a.database.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
			WorkspaceAgentID: workspaceAgent.ID,
			Keys:             nil,
		})
		return err
	})
	eg.Go(func() (err error) {
		resource, err = a.database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
		if err != nil {
			return xerrors.Errorf("getting resource by id: %w", err)
		}
		build, err = a.database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
		if err != nil {
			return xerrors.Errorf("getting workspace build by job id: %w", err)
		}
		workspace, err = a.database.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("getting workspace by id: %w", err)
		}
		owner, err = a.database.GetUserByID(ctx, workspace.OwnerID)
		if err != nil {
			return xerrors.Errorf("getting workspace owner by id: %w", err)
		}
		return err
	})
	err = eg.Wait()
	if err != nil {
		return nil, xerrors.Errorf("fetching workspace agent data: %w", err)
	}

	appHost := httpapi.ApplicationURL{
		AppSlugOrPort: "{{port}}",
		AgentName:     workspaceAgent.Name,
		WorkspaceName: workspace.Name,
		Username:      owner.Username,
	}
	vscodeProxyURI := a.accessURL.Scheme + "://" + strings.ReplaceAll(a.appHostname, "*", appHost.String())
	if a.appHostname == "" {
		vscodeProxyURI += a.accessURL.Hostname()
	}
	if a.accessURL.Port() != "" {
		vscodeProxyURI += fmt.Sprintf(":%s", a.accessURL.Port())
	}

	var gitAuthConfigs uint32
	for _, cfg := range a.externalAuthConfigs {
		if codersdk.EnhancedExternalAuthProvider(cfg.Type).Git() {
			gitAuthConfigs++
		}
	}

	return &agentproto.Manifest{
		AgentId:                  workspaceAgent.ID[:],
		GitAuthConfigs:           gitAuthConfigs,
		EnvironmentVariables:     apiAgent.EnvironmentVariables,
		Directory:                apiAgent.Directory,
		VsCodePortProxyUri:       vscodeProxyURI,
		MotdPath:                 workspaceAgent.MOTDFile,
		DisableDirectConnections: a.disableDirectConnections,
		DerpForceWebsockets:      a.derpForceWebSockets,

		DerpMap:  tailnetproto.DERPMapToProto(a.derpMapFn()),
		Scripts:  agentproto.DBAgentScriptsToProto(scripts),
		Apps:     agentproto.DBAppsToProto(dbApps, workspaceAgent, owner.Username, workspace),
		Metadata: agentproto.DBAgentMetadataToProtoDescription(metadata),
	}, nil
}

func (a *AgentAPI) GetServiceBanner(ctx context.Context, req *agentproto.GetServiceBannerRequest) (*agentproto.ServiceBanner, error) {
	serviceBannerJSON, err := a.database.GetServiceBanner(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get service banner: %w", err)
	}

	var cfg codersdk.ServiceBannerConfig
	if serviceBannerJSON != "" {
		err = json.Unmarshal([]byte(serviceBannerJSON), &cfg)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal json: %w, raw: %s", err, serviceBannerJSON)
		}
	}

	return &agentproto.ServiceBanner{
		Enabled:         cfg.Enabled,
		Message:         cfg.Message,
		BackgroundColor: cfg.BackgroundColor,
	}, nil
}

func (a *AgentAPI) UpdateStats(ctx context.Context, req *agentproto.UpdateStatsRequest) (*agentproto.UpdateStatsResponse, error) {
	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}
	workspace, err := a.database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent ID %q: %w", workspaceAgent.ID, err)
	}

	res := &agentproto.UpdateStatsResponse{
		ReportInterval: durationpb.New(a.agentStatsRefreshInterval),
	}

	// An empty stat means it's just looking for the report interval.
	if len(req.Stats.ConnectionsByProto) == 0 {
		return res, nil
	}

	a.log.Debug(ctx, "read stats report",
		slog.F("interval", a.agentStatsRefreshInterval),
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)

	if req.Stats.ConnectionCount > 0 {
		var nextAutostart time.Time
		if workspace.AutostartSchedule.String != "" {
			templateSchedule, err := (*(a.templateScheduleStore.Load())).Get(ctx, a.database, workspace.TemplateID)
			// If the template schedule fails to load, just default to bumping without the next trasition and log it.
			if err != nil {
				a.log.Warn(ctx, "failed to load template schedule bumping activity, defaulting to bumping by 60min",
					slog.F("workspace_id", workspace.ID),
					slog.F("template_id", workspace.TemplateID),
					slog.Error(err),
				)
			} else {
				next, allowed := autobuild.NextAutostartSchedule(time.Now(), workspace.AutostartSchedule.String, templateSchedule)
				if allowed {
					nextAutostart = next
				}
			}
		}
		activityBumpWorkspace(ctx, a.log.Named("activity_bump"), a.database, workspace.ID, nextAutostart)
	}

	now := dbtime.Now()

	var errGroup errgroup.Group
	errGroup.Go(func() error {
		if err := a.statsBatcher.Add(time.Now(), workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, req.Stats); err != nil {
			a.log.Error(ctx, "failed to add stats to batcher", slog.Error(err))
			return xerrors.Errorf("can't insert workspace agent stat: %w", err)
		}
		return nil
	})
	errGroup.Go(func() error {
		err := a.database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		})
		if err != nil {
			return xerrors.Errorf("can't update workspace LastUsedAt: %w", err)
		}
		return nil
	})
	if a.updateAgentMetrics != nil {
		errGroup.Go(func() error {
			user, err := a.database.GetUserByID(ctx, workspace.OwnerID)
			if err != nil {
				return xerrors.Errorf("can't get user: %w", err)
			}

			a.updateAgentMetrics(ctx, user.Username, workspace.Name, workspaceAgent.Name, req.Stats.Metrics)
			return nil
		})
	}
	err = errGroup.Wait()
	if err != nil {
		return nil, xerrors.Errorf("update stats in database: %w", err)
	}

	return res, nil
}

func (a *AgentAPI) UpdateLifecycle(ctx context.Context, req *agentproto.UpdateLifecycleRequest) (*agentproto.Lifecycle, error) {
	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}

	logger := a.log.With(
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", a.workspaceID),
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
		changedAt = dbtime.Now()
		req.Lifecycle.ChangedAt = timestamppb.New(changedAt)
	}
	dbChangedAt := sql.NullTime{Time: changedAt, Valid: true}

	startedAt := workspaceAgent.StartedAt
	readyAt := workspaceAgent.ReadyAt
	switch lifecycleState {
	case database.WorkspaceAgentLifecycleStateStarting:
		startedAt = dbChangedAt
		readyAt.Valid = false // This agent is re-starting, so it's not ready yet.
	case database.WorkspaceAgentLifecycleStateReady, database.WorkspaceAgentLifecycleStateStartError:
		readyAt = dbChangedAt
	}

	err = a.database.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             workspaceAgent.ID,
		LifecycleState: lifecycleState,
		StartedAt:      startedAt,
		ReadyAt:        readyAt,
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			// not an error if we are canceled
			logger.Error(ctx, "failed to update lifecycle state", slog.Error(err))
		}
		return nil, xerrors.Errorf("update workspace agent lifecycle state: %w", err)
	}

	a.publishWorkspaceUpdate(ctx, a.workspaceID)

	return req.Lifecycle, nil
}

func (a *AgentAPI) BatchUpdateAppHealths(ctx context.Context, req *agentproto.BatchUpdateAppHealthRequest) (*agentproto.BatchUpdateAppHealthResponse, error) {
	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.Updates) == 0 {
		return &agentproto.BatchUpdateAppHealthResponse{}, nil
	}

	apps, err := a.database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
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

		// Don't save if the value hasn't changed
		if old.Health == newHealth {
			continue
		}
		old.Health = database.WorkspaceAppHealth(newHealth)

		newApps = append(newApps, *old)
	}

	for _, app := range newApps {
		err = a.database.UpdateWorkspaceAppHealthByID(ctx, database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: app.Health,
		})
		if err != nil {
			return nil, xerrors.Errorf("update workspace app health for app %q (%q): %w", err, app.ID, app.Slug)
		}
	}

	a.publishWorkspaceUpdate(ctx, a.workspaceID)
	return &agentproto.BatchUpdateAppHealthResponse{}, nil
}

func (a *AgentAPI) UpdateStartup(ctx context.Context, req *agentproto.UpdateStartupRequest) (*agentproto.Startup, error) {
	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}

	a.log.Debug(
		ctx,
		"post workspace agent version",
		slog.F("agent_id", workspaceAgent.ID),
		slog.F("workspace_id", a.workspaceID),
		slog.F("agent_version", req.Startup.Version),
	)

	if !semver.IsValid(req.Startup.Version) {
		return nil, xerrors.Errorf("invalid agent semver version %q", req.Startup.Version)
	}

	// Validate subsystems.
	dbSubsystems := make([]database.WorkspaceAgentSubsystem, len(req.Startup.Subsystems))
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

	err = a.database.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                workspaceAgent.ID,
		Version:           req.Startup.Version,
		ExpandedDirectory: req.Startup.ExpandedDirectory,
		Subsystems:        dbSubsystems,
		APIVersion:        AgentAPIVersionREST,
	})
	if err != nil {
		return nil, xerrors.Errorf("update workspace agent startup in database: %w", err)
	}

	return req.Startup, nil
}

func (a *AgentAPI) BatchUpdateMetadata(ctx context.Context, req *agentproto.BatchUpdateMetadataRequest) (*agentproto.BatchUpdateMetadataResponse, error) {
	const (
		// maxValueLen is set to 2048 to stay under the 8000 byte Postgres
		// NOTIFY limit. Since both value and error can be set, the real
		// payload limit is 2 * 2048 * 4/3 <base64 expansion> = 5461 bytes + a few hundred bytes for JSON
		// syntax, key names, and metadata.
		maxValueLen = 2048
		maxErrorLen = maxValueLen
	)

	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}

	collectedAt := time.Now()
	datum := database.UpdateWorkspaceAgentMetadataParams{
		WorkspaceAgentID: workspaceAgent.ID,
		Key:              make([]string, 0, len(req.Metadata)),
		Value:            make([]string, 0, len(req.Metadata)),
		Error:            make([]string, 0, len(req.Metadata)),
		CollectedAt:      make([]time.Time, 0, len(req.Metadata)),
	}

	for _, md := range req.Metadata {
		metadataError := md.Result.Error

		// We overwrite the error if the provided payload is too long.
		if len(md.Result.Value) > maxValueLen {
			metadataError = fmt.Sprintf("value of %d bytes exceeded %d bytes", len(md.Result.Value), maxValueLen)
			md.Result.Value = md.Result.Value[:maxValueLen]
		}

		if len(md.Result.Error) > maxErrorLen {
			metadataError = fmt.Sprintf("error of %d bytes exceeded %d bytes", len(md.Result.Error), maxErrorLen)
			md.Result.Error = ""
		}

		// We don't want a misconfigured agent to fill the database.
		datum.Key = append(datum.Key, md.Key)
		datum.Value = append(datum.Value, md.Result.Value)
		datum.Error = append(datum.Error, metadataError)
		// We ignore the CollectedAt from the agent to avoid bugs caused by
		// clock skew.
		datum.CollectedAt = append(datum.CollectedAt, collectedAt)

		a.log.Debug(
			ctx, "accepted metadata report",
			slog.F("workspace_agent_id", workspaceAgent.ID),
			slog.F("collected_at", collectedAt),
			slog.F("original_collected_at", collectedAt),
			slog.F("key", md.Key),
			slog.F("value", ellipse(md.Result.Value, 16)),
		)
	}

	payload, err := json.Marshal(workspaceAgentMetadataChannelPayload{
		CollectedAt: collectedAt,
		Keys:        datum.Key,
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal workspace agent metadata channel payload: %w", err)
	}

	err = a.database.UpdateWorkspaceAgentMetadata(ctx, datum)
	if err != nil {
		return nil, xerrors.Errorf("update workspace agent metadata in database: %w", err)
	}

	err = a.pubsub.Publish(watchWorkspaceAgentMetadataChannel(workspaceAgent.ID), payload)
	if err != nil {
		return nil, xerrors.Errorf("publish workspace agent metadata: %w", err)
	}

	return &agentproto.BatchUpdateMetadataResponse{}, nil
}

func (a *AgentAPI) BatchCreateLogs(ctx context.Context, req *agentproto.BatchCreateLogsRequest) (*agentproto.BatchCreateLogsResponse, error) {
	workspaceAgent, err := a.agent(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.Logs) == 0 {
		return &agentproto.BatchCreateLogsResponse{}, nil
	}
	logSourceID, err := uuid.FromBytes(req.LogSourceId)
	if err != nil {
		return nil, xerrors.Errorf("parse log source ID %q: %w", req.LogSourceId, err)
	}

	// This is to support the legacy API where the log source ID was
	// not provided in the request body. We default to the external
	// log source in this case.
	if logSourceID == uuid.Nil {
		// Use the external log source
		externalSources, err := a.database.InsertWorkspaceAgentLogSources(ctx, database.InsertWorkspaceAgentLogSourcesParams{
			WorkspaceAgentID: workspaceAgent.ID,
			CreatedAt:        dbtime.Now(),
			ID:               []uuid.UUID{agentsdk.ExternalLogSourceID},
			DisplayName:      []string{"External"},
			Icon:             []string{"/emojis/1f310.png"},
		})
		if database.IsUniqueViolation(err, database.UniqueWorkspaceAgentLogSourcesPkey) {
			err = nil
			logSourceID = agentsdk.ExternalLogSourceID
		}
		if err != nil {
			return nil, xerrors.Errorf("insert external workspace agent log source: %w", err)
		}
		if len(externalSources) == 1 {
			logSourceID = externalSources[0].ID
		}
	}

	output := make([]string, 0)
	level := make([]database.LogLevel, 0)
	outputLength := 0
	for _, logEntry := range req.Logs {
		output = append(output, logEntry.Output)
		outputLength += len(logEntry.Output)

		var dbLevel database.LogLevel
		switch logEntry.Level {
		case agentproto.Log_TRACE:
			dbLevel = database.LogLevelTrace
		case agentproto.Log_DEBUG:
			dbLevel = database.LogLevelDebug
		case agentproto.Log_INFO:
			dbLevel = database.LogLevelInfo
		case agentproto.Log_WARN:
			dbLevel = database.LogLevelWarn
		case agentproto.Log_ERROR:
			dbLevel = database.LogLevelError
		default:
			// Default to "info" to support older clients that didn't have the
			// level field.
			dbLevel = database.LogLevelInfo
		}
		level = append(level, dbLevel)
	}

	logs, err := a.database.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:      workspaceAgent.ID,
		CreatedAt:    dbtime.Now(),
		Output:       output,
		Level:        level,
		LogSourceID:  logSourceID,
		OutputLength: int32(outputLength),
	})
	if err != nil {
		if !database.IsWorkspaceAgentLogsLimitError(err) {
			return nil, xerrors.Errorf("insert workspace agent logs: %w", err)
		}
		if workspaceAgent.LogsOverflowed {
			return nil, xerrors.New("workspace agent logs overflowed")
		}
		err := a.database.UpdateWorkspaceAgentLogOverflowByID(ctx, database.UpdateWorkspaceAgentLogOverflowByIDParams{
			ID:             workspaceAgent.ID,
			LogsOverflowed: true,
		})
		if err != nil {
			// We don't want to return here, because the agent will retry on
			// failure and this isn't a huge deal. The overflow state is just a
			// hint to the user that the logs are incomplete.
			a.log.Warn(ctx, "failed to update workspace agent log overflow", slog.Error(err))
		}

		a.publishWorkspaceUpdate(ctx, a.workspaceID)

		return nil, xerrors.New("workspace agent log limit exceeded")
	}

	// Publish by the lowest log ID inserted so the log stream will fetch
	// everything from that point.
	lowestLogID := logs[0].ID
	a.publishWorkspaceAgentLogsUpdate(ctx, workspaceAgent.ID, agentsdk.LogsNotifyMessage{
		CreatedAfter: lowestLogID - 1,
	})

	if workspaceAgent.LogsLength == 0 {
		// If these are the first logs being appended, we publish a UI update
		// to notify the UI that logs are now available.
		a.publishWorkspaceUpdate(ctx, a.workspaceID)
	}

	return &agentproto.BatchCreateLogsResponse{}, nil
}

func (a *AgentAPI) StreamDERPMaps(_ *tailnetproto.StreamDERPMapsRequest, stream agentproto.DRPCAgent_StreamDERPMapsStream) error {
	defer stream.Close()

	ticker := time.NewTicker(a.derpMapUpdateFrequency)
	defer ticker.Stop()

	var lastDERPMap *tailcfg.DERPMap
	for {
		derpMap := a.derpMapFn()
		if lastDERPMap == nil || !tailnet.CompareDERPMaps(lastDERPMap, derpMap) {
			protoDERPMap := tailnetproto.DERPMapToProto(derpMap)
			err := stream.Send(protoDERPMap)
			if err != nil {
				return xerrors.Errorf("send derp map: %w", err)
			}
			lastDERPMap = derpMap
		}

		ticker.Reset(a.derpMapUpdateFrequency)
		select {
		case <-stream.Context().Done():
			return nil
		case <-a.ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (*AgentAPI) CoordinateTailnet(_ agentproto.DRPCAgent_CoordinateTailnetStream) error {
	// TODO: implement this
	return xerrors.New("CoordinateTailnet is unimplemented")
}
