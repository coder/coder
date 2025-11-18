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
	"github.com/coder/coder/v2/coderd/agentapi/resourcesmonitor"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

const workspaceCacheRefreshInterval = 5 * time.Minute

// CachedWorkspaceFields contains workspace data that is safe to cache for the
// duration of an agent connection. These fields are used to reduce database calls
// in high-frequency operations like stats reporting and metadata updates.
//
// IMPORTANT: ACL fields (GroupACL, UserACL) are NOT cached because they can be
// modified in the database and we must use fresh data for authorization checks.
//
// Prebuild Safety: When a prebuild is claimed, the owner_id changes in the database
// but the agent connection persists. Currently we handle this by periodically refreshing
// the cached fields (every 5 minutes) to pick up changes like prebuild claims.
type CachedWorkspaceFields struct {
	// Identity fields
	ID             uuid.UUID
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	TemplateID     uuid.UUID

	// Display fields for logging/metrics
	Name          string
	OwnerUsername string
	TemplateName  string
}

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
	*ConnLogAPI
	*SubAgentAPI
	*tailnet.DRPCService

	cachedWorkspaceFields CachedWorkspaceFields

	mu sync.Mutex
}

var _ agentproto.DRPCAgentServer = &API{}

type Options struct {
	AgentID        uuid.UUID
	OwnerID        uuid.UUID
	WorkspaceID    uuid.UUID
	OrganizationID uuid.UUID

	Ctx                               context.Context
	Log                               slog.Logger
	Clock                             quartz.Clock
	Database                          database.Store
	NotificationsEnqueuer             notifications.Enqueuer
	Pubsub                            pubsub.Pubsub
	ConnectionLogger                  *atomic.Pointer[connectionlog.ConnectionLogger]
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

func New(opts Options, workspace database.Workspace) *API {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}

	api := &API{
		opts: opts,
		mu: sync.Mutex{},
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

	// Don't cache details for prebuilds, though the cached fields will eventually be updated
	// by the refresh routine once the prebuild workspace is claimed.
	if !workspace.IsPrebuild() {
		api.cachedWorkspaceFields = CachedWorkspaceFields{
			ID:             workspace.ID,
			OwnerID:        workspace.OwnerID,
			OrganizationID: workspace.OrganizationID,
			TemplateID:     workspace.TemplateID,
			Name:           workspace.Name,
			OwnerUsername:  workspace.OwnerUsername,
			TemplateName:   workspace.TemplateName,
		}
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
		Debounce:              30 * time.Minute,

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
		WorkspaceFn:               api.workspace,
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
		AgentFn:       api.agent,
		RBACContextFn: api.rbacContext,
		Database:      opts.Database,
		Pubsub:        opts.Pubsub,
		Log:           opts.Log,
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

	api.ConnLogAPI = &ConnLogAPI{
		AgentFn:          api.agent,
		ConnectionLogger: opts.ConnectionLogger,
		Database:         opts.Database,
		Log:              opts.Log,
	}

	api.DRPCService = &tailnet.DRPCService{
		CoordPtr:                opts.TailnetCoordinator,
		Logger:                  opts.Log,
		DerpMapUpdateFrequency:  opts.DerpMapUpdateFrequency,
		DerpMapFn:               opts.DerpMapFn,
		NetworkTelemetryHandler: opts.NetworkTelemetryHandler,
	}

	api.SubAgentAPI = &SubAgentAPI{
		OwnerID:        opts.OwnerID,
		OrganizationID: opts.OrganizationID,
		AgentID:        opts.AgentID,
		AgentFn:        api.agent,
		Log:            opts.Log,
		Clock:          opts.Clock,
		Database:       opts.Database,
	}

	// Start background cache refresh loop to handle workspace changes
	// like prebuild claims where owner_id and other fields may be modified in the DB.
	go api.startCacheRefreshLoop(opts.Ctx)

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
			Manager: drpcsdk.DefaultDRPCOptions(nil),
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

	if err := a.ResourcesMonitoringAPI.InitMonitors(ctx); err != nil {
		return xerrors.Errorf("initialize resource monitoring: %w", err)
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

func (a *API) workspace() database.Workspace {
	a.mu.Lock()
	defer a.mu.Unlock()

	return database.Workspace{
		ID:                a.cachedWorkspaceFields.ID,
		OwnerID:           a.cachedWorkspaceFields.OwnerID,
		OrganizationID:    a.cachedWorkspaceFields.OrganizationID,
		TemplateID:        a.cachedWorkspaceFields.TemplateID,
		Name:              a.cachedWorkspaceFields.Name,
		OwnerUsername:     a.cachedWorkspaceFields.OwnerUsername,
		TemplateName:      a.cachedWorkspaceFields.TemplateName,
	}
}

func (a *API) rbacContext(ctx context.Context) (context.Context, error) {
	workspace := a.workspace()
	return dbauthz.WithWorkspaceRBAC(ctx, workspace.RBACObject())
}

// refreshCachedWorkspace periodically updates the cached workspace fields.
// This ensures that changes like prebuild claims (which modify owner_id, name, etc.)
// are eventually reflected in the cache without requiring agent reconnection.
func (a *API) refreshCachedWorkspace(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ws, err := a.opts.Database.GetWorkspaceByID(ctx, a.opts.WorkspaceID)
	if err != nil {
		a.opts.Log.Warn(ctx, "failed to refresh cached workspace fields", slog.Error(err))
		return
	}

	if ws.IsPrebuild() {
		a.opts.Log.Debug(ctx, "workspace is a prebuild, not caching in AgentAPI")
		return
	}

	// Update fields that can change during workspace lifecycle (e.g., prebuild claim)
	a.cachedWorkspaceFields = CachedWorkspaceFields{}


	a.opts.Log.Debug(ctx, "refreshed cached workspace fields",
		slog.F("workspace_id", ws.ID),
		slog.F("owner_id", ws.OwnerID),
		slog.F("name", ws.Name))
}

// startCacheRefreshLoop runs a background goroutine that periodically refreshes
// the cached workspace fields. This is primarily needed to handle prebuild claims
// where the owner_id and other fields change while the agent connection persists.
func (a *API) startCacheRefreshLoop(ctx context.Context) {
	// Refresh every 5 minutes. This provides a reasonable balance between:
	// - Keeping cache fresh for prebuild claims and other workspace updates
	// - Minimizing unnecessary database queries
	ticker := a.opts.Clock.TickerFunc(ctx, workspaceCacheRefreshInterval, func() error {
		a.refreshCachedWorkspace(ctx)
		return nil
	}, "cache_refresh")

	// We need to wait on the ticker exiting.
	_ = ticker.Wait()

	a.opts.Log.Debug(ctx, "cache refresh loop exited, invalidating the workspace cache on agent API",
		slog.F("workspace_id", a.cachedWorkspaceFields.ID),
		slog.F("owner_id", a.cachedWorkspaceFields.OwnerUsername),
		slog.F("name", a.cachedWorkspaceFields.Name))
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cachedWorkspaceFields = CachedWorkspaceFields{}
}

func (a *API) publishWorkspaceUpdate(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
	a.opts.PublishWorkspaceUpdateFn(ctx, a.opts.OwnerID, wspubsub.WorkspaceEvent{
		Kind:        kind,
		WorkspaceID: a.opts.WorkspaceID,
		AgentID:     &agent.ID,
	})
	return nil
}
