package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/websocket"
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
	logger := api.Logger.Named("agentrpc")

	version := r.URL.Query().Get("version")
	if version == "" {
		// The initial version on this HTTP endpoint was 2.0, so assume this version if unspecified.
		// Coder v2.7.1 (not to be confused with the Agent API version) calls this endpoint without
		// a version parameter and wants Agent API version 2.0.
		version = "2.0"
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
	build := httpmw.LatestBuild(r)

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

	logger = logger.With(
		slog.F("owner", owner.Username),
		slog.F("workspace_name", workspace.Name),
		slog.F("agent_name", workspaceAgent.Name),
	)

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	ycfg := yamux.DefaultConfig()
	ycfg.LogOutput = nil
	ycfg.Logger = slog.Stdlib(ctx, logger.Named("yamux"), slog.LevelInfo)

	mux, err := yamux.Server(wsNetConn, ycfg)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to start yamux over websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer mux.Close()

	logger.Debug(ctx, "accepting agent RPC connection",
		slog.F("agent_id", workspaceAgent.ID),
		slog.F("agent_created_at", workspaceAgent.CreatedAt),
		slog.F("agent_updated_at", workspaceAgent.UpdatedAt),
		slog.F("agent_name", workspaceAgent.Name),
		slog.F("agent_first_connected_at", workspaceAgent.FirstConnectedAt.Time),
		slog.F("agent_last_connected_at", workspaceAgent.LastConnectedAt.Time),
		slog.F("agent_disconnected_at", workspaceAgent.DisconnectedAt.Time),
		slog.F("agent_version", workspaceAgent.Version),
		slog.F("agent_last_connected_replica_id", workspaceAgent.LastConnectedReplicaID),
		slog.F("agent_connection_timeout_seconds", workspaceAgent.ConnectionTimeoutSeconds),
		slog.F("agent_api_version", workspaceAgent.APIVersion),
		slog.F("agent_resource_id", workspaceAgent.ResourceID))

	closeCtx, closeCtxCancel := context.WithCancel(ctx)
	defer closeCtxCancel()
	monitor := api.startAgentYamuxMonitor(closeCtx, workspace, workspaceAgent, build, mux)
	defer monitor.close()

	agentAPI := agentapi.New(agentapi.Options{
		AgentID:     workspaceAgent.ID,
		OwnerID:     workspace.OwnerID,
		WorkspaceID: workspace.ID,

		Ctx:                               api.ctx,
		Log:                               logger,
		Clock:                             api.Clock,
		Database:                          api.Database,
		NotificationsEnqueuer:             api.NotificationsEnqueuer,
		Pubsub:                            api.Pubsub,
		Auditor:                           &api.Auditor,
		DerpMapFn:                         api.DERPMap,
		TailnetCoordinator:                &api.TailnetCoordinator,
		AppearanceFetcher:                 &api.AppearanceFetcher,
		StatsReporter:                     api.statsReporter,
		PublishWorkspaceUpdateFn:          api.publishWorkspaceUpdate,
		PublishWorkspaceAgentLogsUpdateFn: api.publishWorkspaceAgentLogsUpdate,
		NetworkTelemetryHandler:           api.NetworkTelemetryBatcher.Handler,

		AccessURL:                 api.AccessURL,
		AppHostname:               api.AppHostname,
		AgentStatsRefreshInterval: api.AgentStatsRefreshInterval,
		DisableDirectConnections:  api.DeploymentValues.DERP.Config.BlockDirect.Value(),
		DerpForceWebSockets:       api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		DerpMapUpdateFrequency:    api.Options.DERPMapUpdateFrequency,
		ExternalAuthConfigs:       api.ExternalAuthConfigs,
		Experiments:               api.Experiments,

		// Optional:
		UpdateAgentMetricsFn: api.UpdateAgentMetrics,
	})

	streamID := tailnet.StreamID{
		Name: fmt.Sprintf("%s-%s-%s", owner.Username, workspace.Name, workspaceAgent.Name),
		ID:   workspaceAgent.ID,
		Auth: tailnet.AgentCoordinateeAuth{ID: workspaceAgent.ID},
	}
	ctx = tailnet.WithStreamID(ctx, streamID)
	ctx = agentapi.WithAPIVersion(ctx, version)
	err = agentAPI.Serve(ctx, mux)
	if err != nil && !errors.Is(err, yamux.ErrSessionShutdown) && !errors.Is(err, io.EOF) {
		logger.Warn(ctx, "workspace agent RPC listen error", slog.Error(err))
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

func (api *API) handleNetworkTelemetry(batch []*tailnetproto.TelemetryEvent) {
	var (
		telemetryEvents = make([]telemetry.NetworkEvent, 0, len(batch))
		didLogErr       = false
	)
	for _, pEvent := range batch {
		tEvent, err := telemetry.NetworkEventFromProto(pEvent)
		if err != nil {
			if !didLogErr {
				api.Logger.Warn(api.ctx, "error converting network telemetry event", slog.Error(err))
				didLogErr = true
			}
			// Events that fail to be converted get discarded for now.
			continue
		}
		telemetryEvents = append(telemetryEvents, tEvent)
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		NetworkEvents: telemetryEvents,
	})
}

type yamuxPingerCloser struct {
	mux *yamux.Session
}

func (y *yamuxPingerCloser) Close(websocket.StatusCode, string) error {
	return y.mux.Close()
}

func (y *yamuxPingerCloser) Ping(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		_, err := y.mux.Ping()
		errCh <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (api *API) startAgentYamuxMonitor(ctx context.Context,
	workspace database.Workspace,
	workspaceAgent database.WorkspaceAgent,
	workspaceBuild database.WorkspaceBuild,
	mux *yamux.Session,
) *agentConnectionMonitor {
	monitor := &agentConnectionMonitor{
		apiCtx:            api.ctx,
		workspace:         workspace,
		workspaceAgent:    workspaceAgent,
		workspaceBuild:    workspaceBuild,
		conn:              &yamuxPingerCloser{mux: mux},
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
	publishWorkspaceUpdate(ctx context.Context, ownerID uuid.UUID, event wspubsub.WorkspaceEvent)
}

type pingerCloser interface {
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

type agentConnectionMonitor struct {
	apiCtx         context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	workspace      database.Workspace
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
func (m *agentConnectionMonitor) sendPings(ctx context.Context) {
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

func (m *agentConnectionMonitor) updateConnectionTimes(ctx context.Context) error {
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

func (m *agentConnectionMonitor) init() {
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

func (m *agentConnectionMonitor) start(ctx context.Context) {
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

func (m *agentConnectionMonitor) monitor(ctx context.Context) {
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
			if !errors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
				m.logger.Error(finalCtx, "failed to update agent disconnect time",
					slog.Error(err),
				)
			}
		}
		m.updater.publishWorkspaceUpdate(finalCtx, m.workspace.OwnerID, wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindAgentConnectionUpdate,
			WorkspaceID: m.workspaceBuild.WorkspaceID,
			AgentID:     &m.workspaceAgent.ID,
		})
	}()
	reason := "disconnect"
	defer func() {
		m.logger.Debug(ctx, "agent connection monitor is closing connection",
			slog.F("reason", reason))
		_ = m.conn.Close(websocket.StatusGoingAway, reason)
	}()

	err := m.updateConnectionTimes(ctx)
	if err != nil {
		reason = err.Error()
		return
	}
	m.updater.publishWorkspaceUpdate(ctx, m.workspace.OwnerID, wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindAgentConnectionUpdate,
		WorkspaceID: m.workspaceBuild.WorkspaceID,
		AgentID:     &m.workspaceAgent.ID,
	})

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
			m.logger.Warn(ctx, "connection to agent timed out")
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
			if !database.IsQueryCanceledError(err) {
				m.logger.Error(ctx, "failed to update agent connection times", slog.Error(err))
			}
			return
		}
		if connectionStatusChanged {
			m.updater.publishWorkspaceUpdate(ctx, m.workspace.OwnerID, wspubsub.WorkspaceEvent{
				Kind:        wspubsub.WorkspaceEventKindAgentConnectionUpdate,
				WorkspaceID: m.workspaceBuild.WorkspaceID,
				AgentID:     &m.workspaceAgent.ID,
			})
		}
		err = checkBuildIsLatest(ctx, m.db, m.workspaceBuild)
		if err != nil {
			reason = err.Error()
			m.logger.Info(ctx, "disconnected possibly outdated agent", slog.Error(err))
			return
		}
	}
}

func (m *agentConnectionMonitor) close() {
	m.cancel()
	m.wg.Wait()
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
