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
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

type AgentAPI struct {
	agentID uuid.UUID

	accessURL                       *url.URL
	appHostname                     string
	agentInactiveDisconnectTimeout  time.Duration
	agentFallbackTroubleshootingURL string
	agentStatsRefreshInterval       time.Duration
	disableDirectConnections        bool
	derpForceWebSockets             bool
	externalAuthConfigs             []*externalauth.Config

	log                    slog.Logger
	database               database.Store
	derpMapFn              func() *tailcfg.DERPMap
	tailnetCoordinator     *atomic.Pointer[tailnet.Coordinator]
	templateScheduleStore  *atomic.Pointer[schedule.TemplateScheduleStore]
	statsBatcher           *batchstats.Batcher
	publishWorkspaceUpdate func(ctx context.Context, workspaceID uuid.UUID)

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
	workspace, err := a.database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent ID %q: %w", workspaceAgent.ID, err)
	}

	logger := a.log.With(
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
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

	a.publishWorkspaceUpdate(ctx, workspace.ID)

	return req.Lifecycle, nil
}

func (a *AgentAPI) BatchUpdateAppHealths(ctx context.Context, req *agentproto.BatchUpdateAppHealthRequest) (*agentproto.BatchUpdateAppHealthResponse, error) {
	// TODO: implement this
	panic("unimplemented")
}

func (a *AgentAPI) UpdateStartup(ctx context.Context, req *agentproto.UpdateStartupRequest) (*agentproto.Startup, error) {
	// TODO: implement this
	panic("unimplemented")
}

func (a *AgentAPI) BatchUpdateMetadata(ctx context.Context, req *agentproto.BatchUpdateMetadataRequest) (*agentproto.BatchUpdateMetadataResponse, error) {
	// TODO: implement this
	panic("unimplemented")
}

func (a *AgentAPI) BatchCreateLogs(ctx context.Context, req *agentproto.BatchCreateLogsRequest) (*agentproto.BatchCreateLogsResponse, error) {
	// TODO: implement this
	panic("unimplemented")
}

func (a *AgentAPI) StreamDERPMaps(req *tailnetproto.StreamDERPMapsRequest, stream agentproto.DRPCAgent_StreamDERPMapsStream) error {
	// TODO: implement this
	panic("unimplemented")
}

func (a *AgentAPI) CoordinateTailnet(stream agentproto.DRPCAgent_CoordinateTailnetStream) error {
	// TODO: implement this
	panic("unimplemented")
}
