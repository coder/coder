package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/tailnet"
)

func (api *API) workspaceAgent(rw http.ResponseWriter, r *http.Request) {
	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	dbApps, err := api.Database.GetWorkspaceAppsByAgentID(r.Context(), workspaceAgent.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent applications.",
			Detail:  err.Error(),
		})
		return
	}
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, api.TailnetCoordinator, workspaceAgent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, apiAgent)
}

func (api *API) workspaceAgentMetadata(rw http.ResponseWriter, r *http.Request) {
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, api.TailnetCoordinator, workspaceAgent, nil, api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, agent.Metadata{
		DERPMap:              api.DERPMap,
		EnvironmentVariables: apiAgent.EnvironmentVariables,
		StartupScript:        apiAgent.StartupScript,
		Directory:            apiAgent.Directory,
	})
}

func (api *API) postWorkspaceAgentVersion(rw http.ResponseWriter, r *http.Request) {
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, api.TailnetCoordinator, workspaceAgent, nil, api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.PostWorkspaceAgentVersionRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	api.Logger.Info(r.Context(), "post workspace agent version", slog.F("agent_id", apiAgent.ID), slog.F("agent_version", req.Version))

	if !semver.IsValid(req.Version) {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid workspace agent version provided.",
			Detail:  fmt.Sprintf("invalid semver version: %q", req.Version),
		})
		return
	}

	if err := api.Database.UpdateWorkspaceAgentVersionByID(r.Context(), database.UpdateWorkspaceAgentVersionByIDParams{
		ID:      apiAgent.ID,
		Version: req.Version,
	}); err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error setting agent version",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, nil)
}

// workspaceAgentPTY spawns a PTY and pipes it over a WebSocket.
// This is used for the web terminal.
func (api *API) workspaceAgentPTY(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionCreate, workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, api.TailnetCoordinator, workspaceAgent, nil, api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(rw, http.StatusPreconditionRequired, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	reconnect, err := uuid.Parse(r.URL.Query().Get("reconnect"))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query param 'reconnect' must be a valid UUID.",
			Validations: []codersdk.ValidationError{
				{Field: "reconnect", Detail: "invalid UUID"},
			},
		})
		return
	}
	height, err := strconv.Atoi(r.URL.Query().Get("height"))
	if err != nil {
		height = 80
	}
	width, err := strconv.Atoi(r.URL.Query().Get("width"))
	if err != nil {
		width = 80
	}

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	_, wsNetConn := websocketNetConn(r.Context(), conn, websocket.MessageBinary)
	defer wsNetConn.Close() // Also closes conn.

	agentConn, release, err := api.workspaceAgentCache.Acquire(r, workspaceAgent.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("dial workspace agent: %s", err))
		return
	}
	defer release()
	ptNetConn, err := agentConn.ReconnectingPTY(reconnect.String(), uint16(height), uint16(width), r.URL.Query().Get("command"))
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("dial: %s", err))
		return
	}
	defer ptNetConn.Close()
	// Pipe the ends together!
	go func() {
		_, _ = io.Copy(wsNetConn, ptNetConn)
	}()
	_, _ = io.Copy(ptNetConn, wsNetConn)
}

func (api *API) dialWorkspaceAgentTailnet(r *http.Request, agentID uuid.UUID) (*agent.Conn, error) {
	clientConn, serverConn := net.Pipe()
	go func() {
		<-r.Context().Done()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}()

	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   api.DERPMap,
		Logger:    api.Logger.Named("tailnet").Leveled(slog.LevelDebug),
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}

	sendNodes, _ := tailnet.ServeCoordinator(clientConn, func(node []*tailnet.Node) error {
		return conn.UpdateNodes(node)
	})
	conn.SetNodeCallback(sendNodes)
	go func() {
		err := api.TailnetCoordinator.ServeClient(serverConn, uuid.New(), agentID)
		if err != nil {
			_ = conn.Close()
		}
	}()
	return &agent.Conn{
		Conn: conn,
	}, nil
}

func (api *API) workspaceAgentConnection(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(rw, http.StatusOK, codersdk.WorkspaceAgentConnectionInfo{
		DERPMap: api.DERPMap,
	})
}

func (api *API) workspaceAgentCoordinate(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace build job.",
			Detail:  err.Error(),
		})
		return
	}
	// Ensure the resource is still valid!
	// We only accept agents for resources on the latest build.
	ensureLatestBuild := func() error {
		latestBuild, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), build.WorkspaceID)
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
		api.Logger.Debug(r.Context(), "agent tried to connect from non-latest built",
			slog.F("resource", resource),
			slog.F("agent", workspaceAgent),
		)
		httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
			Message: "Agent trying to connect from non-latest build.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	ctx, wsNetConn := websocketNetConn(r.Context(), conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	firstConnectedAt := workspaceAgent.FirstConnectedAt
	if !firstConnectedAt.Valid {
		firstConnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
	}
	lastConnectedAt := sql.NullTime{
		Time:  database.Now(),
		Valid: true,
	}
	disconnectedAt := workspaceAgent.DisconnectedAt
	updateConnectionTimes := func() error {
		err = api.Database.UpdateWorkspaceAgentConnectionByID(ctx, database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               workspaceAgent.ID,
			FirstConnectedAt: firstConnectedAt,
			LastConnectedAt:  lastConnectedAt,
			DisconnectedAt:   disconnectedAt,
			UpdatedAt:        database.Now(),
		})
		if err != nil {
			return err
		}
		return nil
	}

	defer func() {
		disconnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
		_ = updateConnectionTimes()
	}()

	err = updateConnectionTimes()
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}

	// end span so we don't get long lived trace data
	tracing.EndHTTPSpan(r, 200)
	api.Logger.Info(ctx, "accepting agent", slog.F("resource", resource), slog.F("agent", workspaceAgent))

	defer conn.Close(websocket.StatusNormalClosure, "")

	closeChan := make(chan struct{})
	go func() {
		defer close(closeChan)
		err = api.TailnetCoordinator.ServeAgent(wsNetConn, workspaceAgent.ID)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	}()
	ticker := time.NewTicker(api.AgentConnectionUpdateFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-closeChan:
			return
		case <-ticker.C:
		}
		lastConnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
		err = updateConnectionTimes()
		if err != nil {
			_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
			return
		}
		err = ensureLatestBuild()
		if err != nil {
			// Disconnect agents that are no longer valid.
			_ = conn.Close(websocket.StatusGoingAway, "")
			return
		}
	}
}

// workspaceAgentClientCoordinate accepts a WebSocket that reads node network updates.
// After accept a PubSub starts listening for new connection node updates
// which are written to the WebSocket.
func (api *API) workspaceAgentClientCoordinate(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionCreate, workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	err = api.TailnetCoordinator.ServeClient(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), uuid.New(), workspaceAgent.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

func convertApps(dbApps []database.WorkspaceApp) []codersdk.WorkspaceApp {
	apps := make([]codersdk.WorkspaceApp, 0)
	for _, dbApp := range dbApps {
		apps = append(apps, codersdk.WorkspaceApp{
			ID:      dbApp.ID,
			Name:    dbApp.Name,
			Command: dbApp.Command.String,
			Icon:    dbApp.Icon,
		})
	}
	return apps
}

func convertWorkspaceAgent(derpMap *tailcfg.DERPMap, coordinator *tailnet.Coordinator, dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, agentInactiveDisconnectTimeout time.Duration) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal env vars: %w", err)
		}
	}
	workspaceAgent := codersdk.WorkspaceAgent{
		ID:                   dbAgent.ID,
		CreatedAt:            dbAgent.CreatedAt,
		UpdatedAt:            dbAgent.UpdatedAt,
		ResourceID:           dbAgent.ResourceID,
		InstanceID:           dbAgent.AuthInstanceID.String,
		Name:                 dbAgent.Name,
		Architecture:         dbAgent.Architecture,
		OperatingSystem:      dbAgent.OperatingSystem,
		StartupScript:        dbAgent.StartupScript.String,
		Version:              dbAgent.Version,
		EnvironmentVariables: envs,
		Directory:            dbAgent.Directory,
		Apps:                 apps,
	}
	node := coordinator.Node(dbAgent.ID)
	if node != nil {
		workspaceAgent.DERPLatency = map[string]codersdk.DERPRegion{}
		for rawRegion, latency := range node.DERPLatency {
			regionParts := strings.SplitN(rawRegion, "-", 2)
			regionID, err := strconv.Atoi(regionParts[0])
			if err != nil {
				return codersdk.WorkspaceAgent{}, xerrors.Errorf("convert derp region id %q: %w", rawRegion, err)
			}
			region, found := derpMap.Regions[regionID]
			if !found {
				// It's possible that a workspace agent is using an old DERPMap
				// and reports regions that do not exist. If that's the case,
				// report the region as unknown!
				region = &tailcfg.DERPRegion{
					RegionID:   regionID,
					RegionName: fmt.Sprintf("Unnamed %d", regionID),
				}
			}
			workspaceAgent.DERPLatency[region.RegionName] = codersdk.DERPRegion{
				Preferred:           node.PreferredDERP == regionID,
				LatencyMilliseconds: latency * 1000,
			}
		}
	}

	if dbAgent.FirstConnectedAt.Valid {
		workspaceAgent.FirstConnectedAt = &dbAgent.FirstConnectedAt.Time
	}
	if dbAgent.LastConnectedAt.Valid {
		workspaceAgent.LastConnectedAt = &dbAgent.LastConnectedAt.Time
	}
	if dbAgent.DisconnectedAt.Valid {
		workspaceAgent.DisconnectedAt = &dbAgent.DisconnectedAt.Time
	}
	switch {
	case !dbAgent.FirstConnectedAt.Valid:
		// If the agent never connected, it's waiting for the compute
		// to start up.
		workspaceAgent.Status = codersdk.WorkspaceAgentConnecting
	case dbAgent.DisconnectedAt.Time.After(dbAgent.LastConnectedAt.Time):
		// If we've disconnected after our last connection, we know the
		// agent is no longer connected.
		workspaceAgent.Status = codersdk.WorkspaceAgentDisconnected
	case database.Now().Sub(dbAgent.LastConnectedAt.Time) > agentInactiveDisconnectTimeout:
		// The connection died without updating the last connected.
		workspaceAgent.Status = codersdk.WorkspaceAgentDisconnected
	case dbAgent.LastConnectedAt.Valid:
		// The agent should be assumed connected if it's under inactivity timeouts
		// and last connected at has been properly set.
		workspaceAgent.Status = codersdk.WorkspaceAgentConnected
	}

	return workspaceAgent, nil
}
func (api *API) workspaceAgentReportStats(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace resource.",
			Detail:  err.Error(),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get build.",
			Detail:  err.Error(),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(r.Context(), build.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer conn.Close(websocket.StatusAbnormalClosure, "")

	// Allow overriding the stat interval for debugging and testing purposes.
	ctx := r.Context()
	timer := time.NewTicker(api.AgentStatsRefreshInterval)
	var lastReport codersdk.AgentStatsReportResponse
	for {
		err := wsjson.Write(ctx, conn, codersdk.AgentStatsReportRequest{})
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to write report request.",
				Detail:  err.Error(),
			})
			return
		}
		var rep codersdk.AgentStatsReportResponse

		err = wsjson.Read(ctx, conn, &rep)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to read report response.",
				Detail:  err.Error(),
			})
			return
		}

		repJSON, err := json.Marshal(rep)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to marshal stat json.",
				Detail:  err.Error(),
			})
			return
		}

		// Avoid inserting duplicate rows to preserve DB space.
		// We will see duplicate reports when on idle connections
		// (e.g. web terminal left open) or when there are no connections at
		// all.
		// We also don't want to update the workspace last used at on duplicate
		// reports.
		var updateDB = !reflect.DeepEqual(lastReport, rep)

		api.Logger.Debug(ctx, "read stats report",
			slog.F("interval", api.AgentStatsRefreshInterval),
			slog.F("agent", workspaceAgent.ID),
			slog.F("resource", resource.ID),
			slog.F("workspace", workspace.ID),
			slog.F("update_db", updateDB),
			slog.F("payload", rep),
		)

		if updateDB {
			lastReport = rep

			_, err = api.Database.InsertAgentStat(ctx, database.InsertAgentStatParams{
				ID:          uuid.New(),
				CreatedAt:   time.Now(),
				AgentID:     workspaceAgent.ID,
				WorkspaceID: build.WorkspaceID,
				UserID:      workspace.OwnerID,
				TemplateID:  workspace.TemplateID,
				Payload:     json.RawMessage(repJSON),
			})
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert agent stat.",
					Detail:  err.Error(),
				})
				return
			}

			err = api.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
				ID:         build.WorkspaceID,
				LastUsedAt: time.Now(),
			})
			if err != nil {
				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update workspace last used at.",
					Detail:  err.Error(),
				})
				return
			}
		}

		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
type wsNetConn struct {
	cancel context.CancelFunc
	net.Conn
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Close() error {
	defer c.cancel()
	return c.Conn.Close()
}

// websocketNetConn wraps websocket.NetConn and returns a context that
// is tied to the parent context and the lifetime of the conn. Any error
// during read or write will cancel the context, but not close the
// conn. Close should be called to release context resources.
func websocketNetConn(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) (context.Context, net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, conn, msgType)
	return ctx, &wsNetConn{
		cancel: cancel,
		Conn:   nc,
	}
}
