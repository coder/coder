package agentapi

import (
	"context"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

// API implements the DRPC agent API interface from agent/proto. This struct is
// instantiated once per agent connection and kept alive for the duration of the
// session.
type API struct {
	opts Options
	*ManifestAPI
	*ServiceBannerAPI
	*StatsAPI
	*LifecycleAPI
	*AppsAPI
	*MetadataAPI
	*LogsAPI
	*tailnet.DRPCService

	mu                sync.Mutex
	cachedWorkspaceID uuid.UUID
}

var _ agentproto.DRPCAgentServer = &API{}

type Options struct {
	AgentID uuid.UUID

	Ctx                               context.Context
	Log                               slog.Logger
	Database                          database.Store
	Pubsub                            pubsub.Pubsub
	DerpMapFn                         func() *tailcfg.DERPMap
	TailnetCoordinator                *atomic.Pointer[tailnet.Coordinator]
	TemplateScheduleStore             *atomic.Pointer[schedule.TemplateScheduleStore]
	StatsBatcher                      StatsBatcher
	AppearanceFetcher                 *atomic.Pointer[appearance.Fetcher]
	PublishWorkspaceUpdateFn          func(ctx context.Context, workspaceID uuid.UUID)
	PublishWorkspaceAgentLogsUpdateFn func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage)

	AccessURL                 *url.URL
	AppHostname               string
	AgentStatsRefreshInterval time.Duration
	DisableDirectConnections  bool
	DerpForceWebSockets       bool
	DerpMapUpdateFrequency    time.Duration
	ExternalAuthConfigs       []*externalauth.Config

	// Optional:
	// WorkspaceID avoids a future lookup to find the workspace ID by setting
	// the cache in advance.
	WorkspaceID          uuid.UUID
	UpdateAgentMetricsFn func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)
}

func New(opts Options) *API {
	api := &API{
		opts:              opts,
		mu:                sync.Mutex{},
		cachedWorkspaceID: opts.WorkspaceID,
	}

	api.ManifestAPI = &ManifestAPI{
		AccessURL:                opts.AccessURL,
		AppHostname:              opts.AppHostname,
		ExternalAuthConfigs:      opts.ExternalAuthConfigs,
		DisableDirectConnections: opts.DisableDirectConnections,
		DerpForceWebSockets:      opts.DerpForceWebSockets,
		AgentFn:                  api.agent,
		Database:                 opts.Database,
		DerpMapFn:                opts.DerpMapFn,
		WorkspaceIDFn: func(ctx context.Context, wa *database.WorkspaceAgent) (uuid.UUID, error) {
			if opts.WorkspaceID != uuid.Nil {
				return opts.WorkspaceID, nil
			}
			ws, err := opts.Database.GetWorkspaceByAgentID(ctx, wa.ID)
			if err != nil {
				return uuid.Nil, err
			}
			return ws.Workspace.ID, nil
		},
	}

	api.ServiceBannerAPI = &ServiceBannerAPI{
		appearanceFetcher: opts.AppearanceFetcher,
	}

	api.StatsAPI = &StatsAPI{
		AgentFn:                   api.agent,
		Database:                  opts.Database,
		Pubsub:                    opts.Pubsub,
		Log:                       opts.Log,
		StatsBatcher:              opts.StatsBatcher,
		TemplateScheduleStore:     opts.TemplateScheduleStore,
		AgentStatsRefreshInterval: opts.AgentStatsRefreshInterval,
		UpdateAgentMetricsFn:      opts.UpdateAgentMetricsFn,
	}

	api.LifecycleAPI = &LifecycleAPI{
		AgentFn:                  api.agent,
		WorkspaceIDFn:            api.workspaceID,
		Database:                 opts.Database,
		Log:                      opts.Log,
		PublishWorkspaceUpdateFn: api.publishWorkspaceUpdate,
	}

	api.AppsAPI = &AppsAPI{
		AgentFn:                  api.agent,
		Database:                 opts.Database,
		Log:                      opts.Log,
		PublishWorkspaceUpdateFn: api.publishWorkspaceUpdate,
	}

	api.MetadataAPI = &MetadataAPI{
		AgentFn:  api.agent,
		Database: opts.Database,
		Pubsub:   opts.Pubsub,
		Log:      opts.Log,
	}

	api.LogsAPI = &LogsAPI{
		AgentFn:                           api.agent,
		Database:                          opts.Database,
		Log:                               opts.Log,
		PublishWorkspaceUpdateFn:          api.publishWorkspaceUpdate,
		PublishWorkspaceAgentLogsUpdateFn: opts.PublishWorkspaceAgentLogsUpdateFn,
	}

	api.DRPCService = &tailnet.DRPCService{
		CoordPtr:               opts.TailnetCoordinator,
		Logger:                 opts.Log,
		DerpMapUpdateFrequency: opts.DerpMapUpdateFrequency,
		DerpMapFn:              opts.DerpMapFn,
	}

	return api
}

func (a *API) Server(ctx context.Context) (*drpcserver.Server, error) {
	mux := drpcmux.New()
	err := agentproto.DRPCRegisterAgent(mux, a)
	if err != nil {
		return nil, xerrors.Errorf("register agent API protocol in DRPC mux: %w", err)
	}

	err = tailnetproto.DRPCRegisterTailnet(mux, a)
	if err != nil {
		return nil, xerrors.Errorf("register tailnet API protocol in DRPC mux: %w", err)
	}

	return drpcserver.NewWithOptions(&tracing.DRPCHandler{Handler: mux},
		drpcserver.Options{
			Log: func(err error) {
				if xerrors.Is(err, io.EOF) {
					return
				}
				a.opts.Log.Debug(ctx, "drpc server error", slog.Error(err))
			},
		},
	), nil
}

func (a *API) Serve(ctx context.Context, l net.Listener) error {
	server, err := a.Server(ctx)
	if err != nil {
		return xerrors.Errorf("create agent API server: %w", err)
	}

	return server.Serve(ctx, l)
}

func (a *API) agent(ctx context.Context) (database.WorkspaceAgent, error) {
	agent, err := a.opts.Database.GetWorkspaceAgentByID(ctx, a.opts.AgentID)
	if err != nil {
		return database.WorkspaceAgent{}, xerrors.Errorf("get workspace agent by id %q: %w", a.opts.AgentID, err)
	}
	return agent, nil
}

func (a *API) workspaceID(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
	a.mu.Lock()
	if a.cachedWorkspaceID != uuid.Nil {
		id := a.cachedWorkspaceID
		a.mu.Unlock()
		return id, nil
	}

	if agent == nil {
		agnt, err := a.agent(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		agent = &agnt
	}

	getWorkspaceAgentByIDRow, err := a.opts.Database.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get workspace by agent id %q: %w", agent.ID, err)
	}

	a.mu.Lock()
	a.cachedWorkspaceID = getWorkspaceAgentByIDRow.Workspace.ID
	a.mu.Unlock()
	return getWorkspaceAgentByIDRow.Workspace.ID, nil
}

func (a *API) publishWorkspaceUpdate(ctx context.Context, agent *database.WorkspaceAgent) error {
	workspaceID, err := a.workspaceID(ctx, agent)
	if err != nil {
		return err
	}

	a.opts.PublishWorkspaceUpdateFn(ctx, workspaceID)
	return nil
}
