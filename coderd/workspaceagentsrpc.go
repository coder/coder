package coderd

import (
	"context"
	"database/sql"
	"net/http"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
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

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgent(r)

	ensureLatestBuildFn, build, ok := ensureLatestBuild(ctx, api.Database, api.Logger, rw, workspaceAgent)
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

	pingFn, ok := api.agentConnectionUpdate(ctx, workspaceAgent, build.WorkspaceID, conn)
	if !ok {
		return
	}

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

	closeCtx, closeCtxCancel := context.WithCancel(ctx)
	go func() {
		defer closeCtxCancel()
		err := agentAPI.Serve(ctx, mux)
		if err != nil {
			api.Logger.Warn(ctx, "workspace agent RPC listen error", slog.Error(err))
			_ = conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	}()

	pingFn(closeCtx, ensureLatestBuildFn)
}

func ensureLatestBuild(ctx context.Context, db database.Store, logger slog.Logger, rw http.ResponseWriter, workspaceAgent database.WorkspaceAgent) (func() error, database.WorkspaceBuild, bool) {
	resource, err := db.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace agent resource.",
			Detail:  err.Error(),
		})
		return nil, database.WorkspaceBuild{}, false
	}

	build, err := db.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace build job.",
			Detail:  err.Error(),
		})
		return nil, database.WorkspaceBuild{}, false
	}

	// Ensure the resource is still valid!
	// We only accept agents for resources on the latest build.
	ensureLatestBuild := func() error {
		latestBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, build.WorkspaceID)
		if err != nil {
			return err
		}
		if build.ID != latestBuild.ID {
			return xerrors.New("build is outdated")
		}
		return nil
	}

	err = ensureLatestBuild()
	if err != nil {
		logger.Debug(ctx, "agent tried to connect from non-latest build",
			slog.F("resource", resource),
			slog.F("agent", workspaceAgent),
		)
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Agent trying to connect from non-latest build.",
			Detail:  err.Error(),
		})
		return nil, database.WorkspaceBuild{}, false
	}

	return ensureLatestBuild, build, true
}

func (api *API) agentConnectionUpdate(ctx context.Context, workspaceAgent database.WorkspaceAgent, workspaceID uuid.UUID, conn *websocket.Conn) (func(closeCtx context.Context, ensureLatestBuildFn func() error), bool) {
	// We use a custom heartbeat routine here instead of `httpapi.Heartbeat`
	// because we want to log the agent's last ping time.
	var lastPing atomic.Pointer[time.Time]
	lastPing.Store(ptr.Ref(time.Now())) // Since the agent initiated the request, assume it's alive.

	go pprof.Do(ctx, pprof.Labels("agent", workspaceAgent.ID.String()), func(ctx context.Context) {
		// TODO(mafredri): Is this too frequent? Use separate ping disconnect timeout?
		t := time.NewTicker(api.AgentConnectionUpdateFrequency)
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
			err := conn.Ping(ctx)
			if err != nil {
				return
			}
			lastPing.Store(ptr.Ref(time.Now()))
		}
	})

	firstConnectedAt := workspaceAgent.FirstConnectedAt
	if !firstConnectedAt.Valid {
		firstConnectedAt = sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		}
	}
	lastConnectedAt := sql.NullTime{
		Time:  dbtime.Now(),
		Valid: true,
	}
	disconnectedAt := workspaceAgent.DisconnectedAt
	updateConnectionTimes := func(ctx context.Context) error {
		//nolint:gocritic // We only update ourself.
		err := api.Database.UpdateWorkspaceAgentConnectionByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               workspaceAgent.ID,
			FirstConnectedAt: firstConnectedAt,
			LastConnectedAt:  lastConnectedAt,
			DisconnectedAt:   disconnectedAt,
			UpdatedAt:        dbtime.Now(),
			LastConnectedReplicaID: uuid.NullUUID{
				UUID:  api.ID,
				Valid: true,
			},
		})
		if err != nil {
			return err
		}
		return nil
	}

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
		ctx, cancel := context.WithTimeout(dbauthz.AsSystemRestricted(api.ctx), api.AgentInactiveDisconnectTimeout)
		defer cancel()

		// Only update timestamp if the disconnect is new.
		if !disconnectedAt.Valid {
			disconnectedAt = sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			}
		}
		err := updateConnectionTimes(ctx)
		if err != nil {
			// This is a bug with unit tests that cancel the app context and
			// cause this error log to be generated. We should fix the unit tests
			// as this is a valid log.
			//
			// The pq error occurs when the server is shutting down.
			if !xerrors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
				api.Logger.Error(ctx, "failed to update agent disconnect time",
					slog.Error(err),
					slog.F("workspace_id", workspaceID),
				)
			}
		}
		api.publishWorkspaceUpdate(ctx, workspaceID)
	}()

	err := updateConnectionTimes(ctx)
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, err.Error())
		return nil, false
	}
	api.publishWorkspaceUpdate(ctx, workspaceID)

	return func(closeCtx context.Context, ensureLatestBuildFn func() error) {
		ticker := time.NewTicker(api.AgentConnectionUpdateFrequency)
		defer ticker.Stop()
		for {
			select {
			case <-closeCtx.Done():
				return
			case <-ticker.C:
			}

			lastPing := *lastPing.Load()

			var connectionStatusChanged bool
			if time.Since(lastPing) > api.AgentInactiveDisconnectTimeout {
				if !disconnectedAt.Valid {
					connectionStatusChanged = true
					disconnectedAt = sql.NullTime{
						Time:  dbtime.Now(),
						Valid: true,
					}
				}
			} else {
				connectionStatusChanged = disconnectedAt.Valid
				// TODO(mafredri): Should we update it here or allow lastConnectedAt to shadow it?
				disconnectedAt = sql.NullTime{}
				lastConnectedAt = sql.NullTime{
					Time:  dbtime.Now(),
					Valid: true,
				}
			}
			err = updateConnectionTimes(ctx)
			if err != nil {
				_ = conn.Close(websocket.StatusGoingAway, err.Error())
				return
			}
			if connectionStatusChanged {
				api.publishWorkspaceUpdate(ctx, workspaceID)
			}
			err := ensureLatestBuildFn()
			if err != nil {
				// Disconnect agents that are no longer valid.
				_ = conn.Close(websocket.StatusGoingAway, "")
				return
			}
		}
	}, true
}
