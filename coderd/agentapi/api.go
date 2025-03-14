package agentapi

import (
	"fmt"
	"errors"
	"context"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"

	"time"
	"github.com/google/uuid"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"
	"cdr.dev/slog"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi/resourcesmonitor"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)
// API implements the DRPC agent API interface from agent/proto. This struct is
// instantiated once per agent connection and kept alive for the duration of the

// session.
type API struct {
	opts Options
	*ManifestAPI
	*AnnouncementBannerAPI
	*StatsAPI
	*LifecycleAPI
	*AppsAPI
	*MetadataAPI
	*ResourcesMonitoringAPI
	*LogsAPI
	*ScriptsAPI
	*AuditAPI
	*tailnet.DRPCService
	mu sync.Mutex
}
var _ agentproto.DRPCAgentServer = &API{}

type Options struct {
	AgentID     uuid.UUID
	OwnerID     uuid.UUID

	WorkspaceID uuid.UUID
	Ctx                               context.Context

	Log                               slog.Logger
	Clock                             quartz.Clock
	Database                          database.Store
	NotificationsEnqueuer             notifications.Enqueuer
	Pubsub                            pubsub.Pubsub

	Auditor                           *atomic.Pointer[audit.Auditor]
	DerpMapFn                         func() *tailcfg.DERPMap
	TailnetCoordinator                *atomic.Pointer[tailnet.Coordinator]
	StatsReporter                     *workspacestats.Reporter
	AppearanceFetcher                 *atomic.Pointer[appearance.Fetcher]
	PublishWorkspaceUpdateFn          func(ctx context.Context, userID uuid.UUID, event wspubsub.WorkspaceEvent)
	PublishWorkspaceAgentLogsUpdateFn func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage)
	NetworkTelemetryHandler           func(batch []*tailnetproto.TelemetryEvent)
	AccessURL                 *url.URL
	AppHostname               string
	AgentStatsRefreshInterval time.Duration
	DisableDirectConnections  bool
	DerpForceWebSockets       bool
	DerpMapUpdateFrequency    time.Duration
	ExternalAuthConfigs       []*externalauth.Config

	Experiments               codersdk.Experiments
	UpdateAgentMetricsFn func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)
}
func New(opts Options) *API {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	api := &API{
		opts: opts,

		mu:   sync.Mutex{},
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
		WorkspaceID:              opts.WorkspaceID,
	}

	api.AnnouncementBannerAPI = &AnnouncementBannerAPI{
		appearanceFetcher: opts.AppearanceFetcher,
	}
	api.ResourcesMonitoringAPI = &ResourcesMonitoringAPI{
		AgentID:               opts.AgentID,
		WorkspaceID:           opts.WorkspaceID,
		Clock:                 opts.Clock,
		Database:              opts.Database,
		NotificationsEnqueuer: opts.NotificationsEnqueuer,
		Debounce:              5 * time.Minute,
		Config: resourcesmonitor.Config{
			NumDatapoints:      20,

			CollectionInterval: 10 * time.Second,
			Alert: resourcesmonitor.AlertConfig{
				MinimumNOKsPercent:     20,
				ConsecutiveNOKsPercent: 50,

			},
		},
	}
	api.StatsAPI = &StatsAPI{
		AgentFn:                   api.agent,
		Database:                  opts.Database,
		Log:                       opts.Log,
		StatsReporter:             opts.StatsReporter,

		AgentStatsRefreshInterval: opts.AgentStatsRefreshInterval,
		Experiments:               opts.Experiments,
	}
	api.LifecycleAPI = &LifecycleAPI{

		AgentFn:                  api.agent,
		WorkspaceID:              opts.WorkspaceID,
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
	api.ScriptsAPI = &ScriptsAPI{
		Database: opts.Database,
	}
	api.AuditAPI = &AuditAPI{
		AgentFn:  api.agent,
		Auditor:  opts.Auditor,

		Database: opts.Database,
		Log:      opts.Log,
	}
	api.DRPCService = &tailnet.DRPCService{
		CoordPtr:                opts.TailnetCoordinator,
		Logger:                  opts.Log,
		DerpMapUpdateFrequency:  opts.DerpMapUpdateFrequency,

		DerpMapFn:               opts.DerpMapFn,
		NetworkTelemetryHandler: opts.NetworkTelemetryHandler,
	}
	return api
}
func (a *API) Server(ctx context.Context) (*drpcserver.Server, error) {
	mux := drpcmux.New()
	err := agentproto.DRPCRegisterAgent(mux, a)

	if err != nil {
		return nil, fmt.Errorf("register agent API protocol in DRPC mux: %w", err)
	}
	err = tailnetproto.DRPCRegisterTailnet(mux, a)

	if err != nil {
		return nil, fmt.Errorf("register tailnet API protocol in DRPC mux: %w", err)
	}
	return drpcserver.NewWithOptions(&tracing.DRPCHandler{Handler: mux},
		drpcserver.Options{
			Log: func(err error) {
				if errors.Is(err, io.EOF) {

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
		return fmt.Errorf("create agent API server: %w", err)

	}
	return server.Serve(ctx, l)
}
func (a *API) agent(ctx context.Context) (database.WorkspaceAgent, error) {
	agent, err := a.opts.Database.GetWorkspaceAgentByID(ctx, a.opts.AgentID)
	if err != nil {
		return database.WorkspaceAgent{}, fmt.Errorf("get workspace agent by id %q: %w", a.opts.AgentID, err)

	}
	return agent, nil
}
func (a *API) publishWorkspaceUpdate(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
	a.opts.PublishWorkspaceUpdateFn(ctx, a.opts.OwnerID, wspubsub.WorkspaceEvent{

		Kind:        kind,
		WorkspaceID: a.opts.WorkspaceID,
		AgentID:     &agent.ID,
	})
	return nil
}
