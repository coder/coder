package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

// @Summary Workspace agent RPC API
// @ID workspace-agent-rpc-api
// @Security CoderSessionToken
// @Tags Agents
// @Success 101
// @Router /workspaceagents/me/rpc [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentRPC(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	version := r.URL.Query().Get("version")
	if version == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing required query parameter: version",
		})
		return
	}
	if err := proto.CurrentVersion.Validate(version); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown or unsupported API version",
			Validations: []codersdk.ValidationError{
				{Field: "version", Detail: err.Error()},
			},
		})
		return
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgent(r)

	build, ok := ensureLatestBuild(ctx, api.Database, api.Logger, rw, workspaceAgent)
	if !ok {
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}

	owner, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	mux, err := yamux.Server(wsNetConn, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to start yamux over websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer mux.Close()

	api.Logger.Debug(ctx, "accepting agent RPC connection",
		slog.F("owner", owner.Username),
		slog.F("workspace", workspace.Name),
		slog.F("name", workspaceAgent.Name),
	)
	api.Logger.Debug(ctx, "accepting agent details", slog.F("agent", workspaceAgent))

	defer conn.Close(websocket.StatusNormalClosure, "")

	closeCtx, closeCtxCancel := context.WithCancel(ctx)
	defer closeCtxCancel()
	monitor := api.startAgentWebsocketMonitor(closeCtx, workspaceAgent, build, conn)
	defer monitor.close()

	agentAPI := agentapi.New(agentapi.Options{
		AgentID: workspaceAgent.ID,

		Ctx:                               api.ctx,
		Log:                               api.Logger,
		Database:                          api.Database,
		Pubsub:                            api.Pubsub,
		DerpMapFn:                         api.DERPMap,
		TailnetCoordinator:                &api.TailnetCoordinator,
		TemplateScheduleStore:             api.TemplateScheduleStore,
		StatsBatcher:                      api.statsBatcher,
		PublishWorkspaceUpdateFn:          api.publishWorkspaceUpdate,
		PublishWorkspaceAgentLogsUpdateFn: api.publishWorkspaceAgentLogsUpdate,

		AccessURL:                       api.AccessURL,
		AppHostname:                     api.AppHostname,
		AgentInactiveDisconnectTimeout:  api.AgentInactiveDisconnectTimeout,
		AgentFallbackTroubleshootingURL: api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
		AgentStatsRefreshInterval:       api.AgentStatsRefreshInterval,
		DisableDirectConnections:        api.DeploymentValues.DERP.Config.BlockDirect.Value(),
		DerpForceWebSockets:             api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		DerpMapUpdateFrequency:          api.Options.DERPMapUpdateFrequency,
		ExternalAuthConfigs:             api.ExternalAuthConfigs,

		// Optional:
		WorkspaceID:          build.WorkspaceID, // saves the extra lookup later
		UpdateAgentMetricsFn: api.UpdateAgentMetrics,
	})

	streamID := tailnet.StreamID{
		Name: fmt.Sprintf("%s-%s-%s", owner.Username, workspace.Name, workspaceAgent.Name),
		ID:   workspaceAgent.ID,
		Auth: tailnet.AgentTunnelAuth{},
	}
	ctx = tailnet.WithStreamID(ctx, streamID)
	err = agentAPI.Serve(ctx, mux)
	if err != nil {
		api.Logger.Warn(ctx, "workspace agent RPC listen error", slog.Error(err))
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

func ensureLatestBuild(ctx context.Context, db database.Store, logger slog.Logger, rw http.ResponseWriter, workspaceAgent database.WorkspaceAgent) (database.WorkspaceBuild, bool) {
	resource, err := db.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace agent resource.",
			Detail:  err.Error(),
		})
		return database.WorkspaceBuild{}, false
	}

	build, err := db.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace build job.",
			Detail:  err.Error(),
		})
		return database.WorkspaceBuild{}, false
	}

	// Ensure the resource is still valid!
	// We only accept agents for resources on the latest build.
	err = checkBuildIsLatest(ctx, db, build)
	if err != nil {
		logger.Debug(ctx, "agent tried to connect from non-latest build",
			slog.F("resource", resource),
			slog.F("agent", workspaceAgent),
		)
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Agent trying to connect from non-latest build.",
			Detail:  err.Error(),
		})
		return database.WorkspaceBuild{}, false
	}

	return build, true
}

func checkBuildIsLatest(ctx context.Context, db database.Store, build database.WorkspaceBuild) error {
	latestBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, build.WorkspaceID)
	if err != nil {
		return err
	}
	if build.ID != latestBuild.ID {
		return xerrors.New("build is outdated")
	}
	return nil
}

func (api *API) startAgentWebsocketMonitor(ctx context.Context,
	workspaceAgent database.WorkspaceAgent, workspaceBuild database.WorkspaceBuild,
	conn *websocket.Conn,
) *agentWebsocketMonitor {
	monitor := &agentWebsocketMonitor{
		apiCtx:            api.ctx,
		workspaceAgent:    workspaceAgent,
		workspaceBuild:    workspaceBuild,
		conn:              conn,
		pingPeriod:        api.AgentConnectionUpdateFrequency,
		db:                api.Database,
		replicaID:         api.ID,
		updater:           api,
		disconnectTimeout: api.AgentInactiveDisconnectTimeout,
		logger: api.Logger.With(
			slog.F("workspace_id", workspaceBuild.WorkspaceID),
			slog.F("agent_id", workspaceAgent.ID),
		),
	}
	monitor.init()
	monitor.start(ctx)

	return monitor
}

type workspaceUpdater interface {
	publishWorkspaceUpdate(ctx context.Context, workspaceID uuid.UUID)
}

type pingerCloser interface {
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

type agentWebsocketMonitor struct {
	apiCtx         context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	workspaceAgent database.WorkspaceAgent
	workspaceBuild database.WorkspaceBuild
	conn           pingerCloser
	db             database.Store
	replicaID      uuid.UUID
	updater        workspaceUpdater
	logger         slog.Logger
	pingPeriod     time.Duration

	// state manipulated by both sendPings() and monitor() goroutines: needs to be threadsafe
	lastPing atomic.Pointer[time.Time]

	// state manipulated only by monitor() goroutine: does not need to be threadsafe
	firstConnectedAt  sql.NullTime
	lastConnectedAt   sql.NullTime
	disconnectedAt    sql.NullTime
	disconnectTimeout time.Duration
}

// sendPings sends websocket pings.
//
// We use a custom heartbeat routine here instead of `httpapi.Heartbeat`
// because we want to log the agent's last ping time.
func (m *agentWebsocketMonitor) sendPings(ctx context.Context) {
	t := time.NewTicker(m.pingPeriod)
	defer t.Stop()

	for {
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}

		// We don't need a context that times out here because the ping will
		// eventually go through. If the context times out, then other
		// websocket read operations will receive an error, obfuscating the
		// actual problem.
		err := m.conn.Ping(ctx)
		if err != nil {
			return
		}
		m.lastPing.Store(ptr.Ref(time.Now()))
	}
}

func (m *agentWebsocketMonitor) updateConnectionTimes(ctx context.Context) error {
	//nolint:gocritic // We only update the agent we are minding.
	err := m.db.UpdateWorkspaceAgentConnectionByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentConnectionByIDParams{
		ID:               m.workspaceAgent.ID,
		FirstConnectedAt: m.firstConnectedAt,
		LastConnectedAt:  m.lastConnectedAt,
		DisconnectedAt:   m.disconnectedAt,
		UpdatedAt:        dbtime.Now(),
		LastConnectedReplicaID: uuid.NullUUID{
			UUID:  m.replicaID,
			Valid: true,
		},
	})
	if err != nil {
		return xerrors.Errorf("failed to update workspace agent connection times: %w", err)
	}
	return nil
}

func (m *agentWebsocketMonitor) init() {
	now := dbtime.Now()
	m.firstConnectedAt = m.workspaceAgent.FirstConnectedAt
	if !m.firstConnectedAt.Valid {
		m.firstConnectedAt = sql.NullTime{
			Time:  now,
			Valid: true,
		}
	}
	m.lastConnectedAt = sql.NullTime{
		Time:  now,
		Valid: true,
	}
	m.disconnectedAt = m.workspaceAgent.DisconnectedAt
	m.lastPing.Store(ptr.Ref(time.Now())) // Since the agent initiated the request, assume it's alive.
}

func (m *agentWebsocketMonitor) start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(2)
	go pprof.Do(ctx, pprof.Labels("agent", m.workspaceAgent.ID.String()),
		func(ctx context.Context) {
			defer m.wg.Done()
			m.sendPings(ctx)
		})
	go pprof.Do(ctx, pprof.Labels("agent", m.workspaceAgent.ID.String()),
		func(ctx context.Context) {
			defer m.wg.Done()
			m.monitor(ctx)
		})
}

func (m *agentWebsocketMonitor) monitor(ctx context.Context) {
	defer func() {
		// If connection closed then context will be canceled, try to
		// ensure our final update is sent. By waiting at most the agent
		// inactive disconnect timeout we ensure that we don't block but
		// also guarantee that the agent will be considered disconnected
		// by normal status check.
		//
		// Use a system context as the agent has disconnected and that token
		// may no longer be valid.
		//nolint:gocritic
		finalCtx, cancel := context.WithTimeout(dbauthz.AsSystemRestricted(m.apiCtx), m.disconnectTimeout)
		defer cancel()

		// Only update timestamp if the disconnect is new.
		if !m.disconnectedAt.Valid {
			m.disconnectedAt = sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			}
		}
		err := m.updateConnectionTimes(finalCtx)
		if err != nil {
			// This is a bug with unit tests that cancel the app context and
			// cause this error log to be generated. We should fix the unit tests
			// as this is a valid log.
			//
			// The pq error occurs when the server is shutting down.
			if !xerrors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
				m.logger.Error(finalCtx, "failed to update agent disconnect time",
					slog.Error(err),
				)
			}
		}
		m.updater.publishWorkspaceUpdate(finalCtx, m.workspaceBuild.WorkspaceID)
	}()
	reason := "disconnect"
	defer func() {
		m.logger.Debug(ctx, "agent websocket monitor is closing connection",
			slog.F("reason", reason))
		_ = m.conn.Close(websocket.StatusGoingAway, reason)
	}()

	err := m.updateConnectionTimes(ctx)
	if err != nil {
		reason = err.Error()
		return
	}
	m.updater.publishWorkspaceUpdate(ctx, m.workspaceBuild.WorkspaceID)

	ticker := time.NewTicker(m.pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			reason = "canceled"
			return
		case <-ticker.C:
		}

		lastPing := *m.lastPing.Load()
		if time.Since(lastPing) > m.disconnectTimeout {
			reason = "ping timeout"
			return
		}
		connectionStatusChanged := m.disconnectedAt.Valid
		m.disconnectedAt = sql.NullTime{}
		m.lastConnectedAt = sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		}

		err = m.updateConnectionTimes(ctx)
		if err != nil {
			reason = err.Error()
			return
		}
		if connectionStatusChanged {
			m.updater.publishWorkspaceUpdate(ctx, m.workspaceBuild.WorkspaceID)
		}
		err = checkBuildIsLatest(ctx, m.db, m.workspaceBuild)
		if err != nil {
			reason = err.Error()
			return
		}
	}
}

func (m *agentWebsocketMonitor) close() {
	m.cancel()
	m.wg.Wait()
}
