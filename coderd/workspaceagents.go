package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/tailnet"
)

func (api *API) workspaceAgent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	dbApps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent applications.",
			Detail:  err.Error(),
		})
		return
	}
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout, api.DeploymentConfig.AgentFallbackTroubleshootingURL.Value)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiAgent)
}

func (api *API) workspaceAgentMetadata(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout, api.DeploymentConfig.AgentFallbackTroubleshootingURL.Value)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	dbApps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent applications.",
			Detail:  err.Error(),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}
	owner, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace owner.",
			Detail:  err.Error(),
		})
		return
	}

	vscodeProxyURI := strings.ReplaceAll(api.AppHostname, "*",
		fmt.Sprintf("%s://{{port}}--%s--%s--%s",
			api.AccessURL.Scheme,
			workspaceAgent.Name,
			workspace.Name,
			owner.Username,
		))

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentMetadata{
		Apps:                 convertApps(dbApps),
		DERPMap:              api.DERPMap,
		GitAuthConfigs:       len(api.GitAuthConfigs),
		EnvironmentVariables: apiAgent.EnvironmentVariables,
		StartupScript:        apiAgent.StartupScript,
		Directory:            apiAgent.Directory,
		VSCodePortProxyURI:   vscodeProxyURI,
		MOTDFile:             workspaceAgent.MOTDFile,
	})
}

func (api *API) postWorkspaceAgentVersion(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout, api.DeploymentConfig.AgentFallbackTroubleshootingURL.Value)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.PostWorkspaceAgentVersionRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	api.Logger.Info(ctx, "post workspace agent version", slog.F("agent_id", apiAgent.ID), slog.F("agent_version", req.Version))

	if !semver.IsValid(req.Version) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid workspace agent version provided.",
			Detail:  fmt.Sprintf("invalid semver version: %q", req.Version),
		})
		return
	}

	if err := api.Database.UpdateWorkspaceAgentVersionByID(ctx, database.UpdateWorkspaceAgentVersionByIDParams{
		ID:      apiAgent.ID,
		Version: req.Version,
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error setting agent version",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

// workspaceAgentPTY spawns a PTY and pipes it over a WebSocket.
// This is used for the web terminal.
func (api *API) workspaceAgentPTY(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionCreate, workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	apiAgent, err := convertWorkspaceAgent(api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout, api.DeploymentConfig.AgentFallbackTroubleshootingURL.Value)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(ctx, rw, http.StatusPreconditionRequired, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	reconnect, err := uuid.Parse(r.URL.Query().Get("reconnect"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
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
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	_, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close() // Also closes conn.

	agentConn, release, err := api.workspaceAgentCache.Acquire(r, workspaceAgent.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("dial workspace agent: %s", err))
		return
	}
	defer release()
	ptNetConn, err := agentConn.ReconnectingPTY(ctx, reconnect, uint16(height), uint16(width), r.URL.Query().Get("command"))
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("dial: %s", err))
		return
	}
	defer ptNetConn.Close()
	agent.Bicopy(ctx, wsNetConn, ptNetConn)
}

func (api *API) workspaceAgentListeningPorts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	apiAgent, err := convertWorkspaceAgent(api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout, api.DeploymentConfig.AgentFallbackTroubleshootingURL.Value)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(ctx, rw, http.StatusPreconditionRequired, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	agentConn, release, err := api.workspaceAgentCache.Acquire(r, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error dialing workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	portsResponse, err := agentConn.ListeningPorts(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching listening ports.",
			Detail:  err.Error(),
		})
		return
	}

	// Get a list of ports that are in-use by applications.
	apps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if xerrors.Is(err, sql.ErrNoRows) {
		apps = []database.WorkspaceApp{}
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace apps.",
			Detail:  err.Error(),
		})
		return
	}
	appPorts := make(map[uint16]struct{}, len(apps))
	for _, app := range apps {
		if !app.Url.Valid || app.Url.String == "" {
			continue
		}
		u, err := url.Parse(app.Url.String)
		if err != nil {
			continue
		}
		port := u.Port()
		if port == "" {
			continue
		}
		portNum, err := strconv.Atoi(port)
		if err != nil {
			continue
		}
		if portNum < 1 || portNum > 65535 {
			continue
		}
		appPorts[uint16(portNum)] = struct{}{}
	}

	// Filter out ports that are globally blocked, in-use by applications, or
	// common non-HTTP ports such as databases, FTP, SSH, etc.
	filteredPorts := make([]codersdk.ListeningPort, 0, len(portsResponse.Ports))
	for _, port := range portsResponse.Ports {
		if port.Port < uint16(codersdk.MinimumListeningPort) {
			continue
		}
		if _, ok := appPorts[port.Port]; ok {
			continue
		}
		if _, ok := codersdk.IgnoredListeningPorts[port.Port]; ok {
			continue
		}
		filteredPorts = append(filteredPorts, port)
	}

	portsResponse.Ports = filteredPorts
	httpapi.Write(ctx, rw, http.StatusOK, portsResponse)
}

func (api *API) dialWorkspaceAgentTailnet(r *http.Request, agentID uuid.UUID) (*codersdk.AgentConn, error) {
	clientConn, serverConn := net.Pipe()
	go func() {
		<-r.Context().Done()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}()

	derpMap := api.DERPMap.Clone()
	for _, region := range derpMap.Regions {
		if !region.EmbeddedRelay {
			continue
		}
		var node *tailcfg.DERPNode
		for _, n := range region.Nodes {
			if n.STUNOnly {
				continue
			}
			node = n
			break
		}
		if node == nil {
			continue
		}
		// TODO: This should dial directly to execute the
		// DERP server instead of contacting localhost.
		//
		// This requires modification of Tailscale internals
		// to pipe through a proxy function per-region, so
		// this is an easy and mostly reliable hack for now.
		cloned := node.Clone()
		// Add p for proxy.
		// This first node supports TLS.
		cloned.Name += "p"
		cloned.IPv4 = "127.0.0.1"
		cloned.InsecureForTests = true
		region.Nodes = append(region.Nodes, cloned.Clone())
		// This second node forces HTTP.
		cloned.Name += "-http"
		cloned.ForceHTTP = true
		region.Nodes = append(region.Nodes, cloned)
	}

	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   derpMap,
		Logger:    api.Logger.Named("tailnet"),
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}

	sendNodes, _ := tailnet.ServeCoordinator(clientConn, func(node []*tailnet.Node) error {
		return conn.UpdateNodes(node)
	})
	conn.SetNodeCallback(sendNodes)
	go func() {
		err := (*api.TailnetCoordinator.Load()).ServeClient(serverConn, uuid.New(), agentID)
		if err != nil {
			api.Logger.Warn(r.Context(), "tailnet coordinator client error", slog.Error(err))
			_ = conn.Close()
		}
	}()
	return &codersdk.AgentConn{
		Conn: conn,
	}, nil
}

func (api *API) workspaceAgentConnection(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentConnectionInfo{
		DERPMap: api.DERPMap,
	})
}

func (api *API) workspaceAgentCoordinate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace build job.",
			Detail:  err.Error(),
		})
		return
	}
	// Ensure the resource is still valid!
	// We only accept agents for resources on the latest build.
	ensureLatestBuild := func() error {
		latestBuild, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, build.WorkspaceID)
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
		api.Logger.Debug(ctx, "agent tried to connect from non-latest built",
			slog.F("resource", resource),
			slog.F("agent", workspaceAgent),
		)
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Agent trying to connect from non-latest build.",
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
	go httpapi.Heartbeat(ctx, conn)

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
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
		disconnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
		_ = updateConnectionTimes()
		_ = api.Pubsub.Publish(watchWorkspaceChannel(build.WorkspaceID), []byte{})
	}()

	err = updateConnectionTimes()
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, err.Error())
		return
	}
	api.publishWorkspaceUpdate(ctx, build.WorkspaceID)

	// End span so we don't get long lived trace data.
	tracing.EndHTTPSpan(r, http.StatusOK, trace.SpanFromContext(ctx))
	// Ignore all trace spans after this.
	ctx = trace.ContextWithSpan(ctx, tracing.NoopSpan)

	api.Logger.Info(ctx, "accepting agent", slog.F("agent", workspaceAgent))

	defer conn.Close(websocket.StatusNormalClosure, "")

	closeChan := make(chan struct{})
	go func() {
		defer close(closeChan)
		err := (*api.TailnetCoordinator.Load()).ServeAgent(wsNetConn, workspaceAgent.ID)
		if err != nil {
			api.Logger.Warn(ctx, "tailnet coordinator agent error", slog.Error(err))
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
			_ = conn.Close(websocket.StatusGoingAway, err.Error())
			return
		}
		err := ensureLatestBuild()
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
	ctx := r.Context()

	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionCreate, workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// This is used by Enterprise code to control the functionality of this route.
	override := api.WorkspaceClientCoordinateOverride.Load()
	if override != nil {
		overrideFunc := *override
		if overrideFunc != nil && overrideFunc(rw) {
			return
		}
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	defer conn.Close(websocket.StatusNormalClosure, "")
	err = (*api.TailnetCoordinator.Load()).ServeClient(websocket.NetConn(ctx, conn, websocket.MessageBinary), uuid.New(), workspaceAgent.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

func convertApps(dbApps []database.WorkspaceApp) []codersdk.WorkspaceApp {
	apps := make([]codersdk.WorkspaceApp, 0)
	for _, dbApp := range dbApps {
		apps = append(apps, codersdk.WorkspaceApp{
			ID:           dbApp.ID,
			Slug:         dbApp.Slug,
			DisplayName:  dbApp.DisplayName,
			Command:      dbApp.Command.String,
			Icon:         dbApp.Icon,
			Subdomain:    dbApp.Subdomain,
			SharingLevel: codersdk.WorkspaceAppSharingLevel(dbApp.SharingLevel),
			Healthcheck: codersdk.Healthcheck{
				URL:       dbApp.HealthcheckUrl,
				Interval:  dbApp.HealthcheckInterval,
				Threshold: dbApp.HealthcheckThreshold,
			},
			Health: codersdk.WorkspaceAppHealth(dbApp.Health),
		})
	}
	return apps
}

func convertWorkspaceAgent(derpMap *tailcfg.DERPMap, coordinator tailnet.Coordinator, dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, agentInactiveDisconnectTimeout time.Duration, agentFallbackTroubleshootingURL string) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal env vars: %w", err)
		}
	}
	troubleshootingURL := agentFallbackTroubleshootingURL
	if dbAgent.TroubleshootingURL != "" {
		troubleshootingURL = dbAgent.TroubleshootingURL
	}
	workspaceAgent := codersdk.WorkspaceAgent{
		ID:                       dbAgent.ID,
		CreatedAt:                dbAgent.CreatedAt,
		UpdatedAt:                dbAgent.UpdatedAt,
		ResourceID:               dbAgent.ResourceID,
		InstanceID:               dbAgent.AuthInstanceID.String,
		Name:                     dbAgent.Name,
		Architecture:             dbAgent.Architecture,
		OperatingSystem:          dbAgent.OperatingSystem,
		StartupScript:            dbAgent.StartupScript.String,
		Version:                  dbAgent.Version,
		EnvironmentVariables:     envs,
		Directory:                dbAgent.Directory,
		Apps:                     apps,
		ConnectionTimeoutSeconds: dbAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       troubleshootingURL,
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

	connectionTimeout := time.Duration(dbAgent.ConnectionTimeoutSeconds) * time.Second
	switch {
	case !dbAgent.FirstConnectedAt.Valid:
		switch {
		case connectionTimeout > 0 && database.Now().Sub(dbAgent.CreatedAt) > connectionTimeout:
			// If the agent took too long to connect the first time,
			// mark it as timed out.
			workspaceAgent.Status = codersdk.WorkspaceAgentTimeout
		default:
			// If the agent never connected, it's waiting for the compute
			// to start up.
			workspaceAgent.Status = codersdk.WorkspaceAgentConnecting
		}
	case dbAgent.DisconnectedAt.Time.After(dbAgent.LastConnectedAt.Time):
		// If we've disconnected after our last connection, we know the
		// agent is no longer connected.
		workspaceAgent.Status = codersdk.WorkspaceAgentDisconnected
	case database.Now().Sub(dbAgent.LastConnectedAt.Time) > agentInactiveDisconnectTimeout:
		// The connection died without updating the last connected.
		workspaceAgent.Status = codersdk.WorkspaceAgentDisconnected
		// Client code needs an accurate disconnected at if the agent has been inactive.
		workspaceAgent.DisconnectedAt = &dbAgent.LastConnectedAt.Time
	case dbAgent.LastConnectedAt.Valid:
		// The agent should be assumed connected if it's under inactivity timeouts
		// and last connected at has been properly set.
		workspaceAgent.Status = codersdk.WorkspaceAgentConnected
	}

	return workspaceAgent, nil
}

func (api *API) workspaceAgentReportStats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.AgentStats
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.RxBytes == 0 && req.TxBytes == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentStatsResponse{
			ReportInterval: api.AgentStatsRefreshInterval,
		})
		return
	}

	activityBumpWorkspace(api.Logger.Named("activity_bump"), api.Database, workspace.ID)

	now := database.Now()
	_, err = api.Database.InsertAgentStat(ctx, database.InsertAgentStatParams{
		ID:          uuid.New(),
		CreatedAt:   now,
		AgentID:     workspaceAgent.ID,
		WorkspaceID: workspace.ID,
		UserID:      workspace.OwnerID,
		TemplateID:  workspace.TemplateID,
		Payload:     json.RawMessage("{}"),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	err = api.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
		ID:         workspace.ID,
		LastUsedAt: now,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentStatsResponse{
		ReportInterval: api.AgentStatsRefreshInterval,
	})
}

func (api *API) workspaceAgentReportStatsWebsocket(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	defer conn.Close(websocket.StatusGoingAway, "")

	var lastReport codersdk.AgentStatsReportResponse
	latestStat, err := api.Database.GetLatestAgentStat(ctx, workspaceAgent.ID)
	if err == nil {
		err = json.Unmarshal(latestStat.Payload, &lastReport)
		if err != nil {
			api.Logger.Debug(ctx, "unmarshal stat payload", slog.Error(err))
			conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("unmarshal stat payload: %s", err))
			return
		}
	}

	// Allow overriding the stat interval for debugging and testing purposes.
	timer := time.NewTicker(api.AgentStatsRefreshInterval)
	defer timer.Stop()

	go func() {
		for {
			err := wsjson.Write(ctx, conn, codersdk.AgentStatsReportRequest{})
			if err != nil {
				conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("write report request: %s", err))
				return
			}

			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}()

	for {
		var rep codersdk.AgentStatsReportResponse
		err = wsjson.Read(ctx, conn, &rep)
		if err != nil {
			conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("read report response: %s", err))
			return
		}

		repJSON, err := json.Marshal(rep)
		if err != nil {
			api.Logger.Debug(ctx, "marshal stat json", slog.Error(err))
			conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("marshal stat json: %s", err))
			return
		}

		// Avoid inserting duplicate rows to preserve DB space.
		// We will see duplicate reports when on idle connections
		// (e.g. web terminal left open) or when there are no connections at
		// all.
		// We also don't want to update the workspace last used at on duplicate
		// reports.
		updateDB := !reflect.DeepEqual(lastReport, rep)

		api.Logger.Debug(ctx, "read stats report",
			slog.F("interval", api.AgentStatsRefreshInterval),
			slog.F("agent", workspaceAgent.ID),
			slog.F("workspace", workspace.ID),
			slog.F("update_db", updateDB),
			slog.F("payload", rep),
		)

		if updateDB {
			go activityBumpWorkspace(api.Logger.Named("activity_bump"), api.Database, workspace.ID)

			lastReport = rep

			_, err = api.Database.InsertAgentStat(ctx, database.InsertAgentStatParams{
				ID:          uuid.New(),
				CreatedAt:   database.Now(),
				AgentID:     workspaceAgent.ID,
				WorkspaceID: workspace.ID,
				UserID:      workspace.OwnerID,
				TemplateID:  workspace.TemplateID,
				Payload:     json.RawMessage(repJSON),
			})
			if err != nil {
				api.Logger.Debug(ctx, "insert agent stat", slog.Error(err))
				conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("insert agent stat: %s", err))
				return
			}

			err = api.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
				ID:         workspace.ID,
				LastUsedAt: database.Now(),
			})
			if err != nil {
				api.Logger.Debug(ctx, "update workspace last used at", slog.Error(err))
				conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("update workspace last used at: %s", err))
				return
			}
		}
	}
}

func (api *API) postWorkspaceAppHealth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	var req codersdk.PostWorkspaceAppHealthsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Healths == nil || len(req.Healths) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Health field is empty",
		})
		return
	}

	apps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error getting agent apps",
			Detail:  err.Error(),
		})
		return
	}

	var newApps []database.WorkspaceApp
	for id, newHealth := range req.Healths {
		old := func() *database.WorkspaceApp {
			for _, app := range apps {
				if app.ID == id {
					return &app
				}
			}

			return nil
		}()
		if old == nil {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  xerrors.Errorf("workspace app name %s not found", id).Error(),
			})
			return
		}

		if old.HealthcheckUrl == "" {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  xerrors.Errorf("health checking is disabled for workspace app %s", id).Error(),
			})
			return
		}

		switch newHealth {
		case codersdk.WorkspaceAppHealthInitializing:
		case codersdk.WorkspaceAppHealthHealthy:
		case codersdk.WorkspaceAppHealthUnhealthy:
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  xerrors.Errorf("workspace app health %s is not a valid value", newHealth).Error(),
			})
			return
		}

		// don't save if the value hasn't changed
		if old.Health == database.WorkspaceAppHealth(newHealth) {
			continue
		}
		old.Health = database.WorkspaceAppHealth(newHealth)

		newApps = append(newApps, *old)
	}

	for _, app := range newApps {
		err = api.Database.UpdateWorkspaceAppHealthByID(ctx, database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: app.Health,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  err.Error(),
			})
			return
		}
	}

	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, job.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}
	api.publishWorkspaceUpdate(ctx, workspace.ID)

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

// postWorkspaceAgentsGitAuth returns a username and password for use
// with GIT_ASKPASS.
func (api *API) workspaceAgentsGitAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gitURL := r.URL.Query().Get("url")
	if gitURL == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing 'url' query parameter!",
		})
		return
	}
	// listen determines if the request will wait for a
	// new token to be issued!
	listen := r.URL.Query().Has("listen")

	var gitAuthConfig *gitauth.Config
	for _, gitAuth := range api.GitAuthConfigs {
		matches := gitAuth.Regex.MatchString(gitURL)
		if !matches {
			continue
		}
		gitAuthConfig = gitAuth
	}
	if gitAuthConfig == nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No git provider found for URL %q", gitURL),
		})
		return
	}
	workspaceAgent := httpmw.WorkspaceAgent(r)
	// We must get the workspace to get the owner ID!
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	if listen {
		// If listening we await a new token...
		authChan := make(chan struct{}, 1)
		cancelFunc, err := api.Pubsub.Subscribe("gitauth", func(ctx context.Context, message []byte) {
			ids := strings.Split(string(message), "|")
			if len(ids) != 2 {
				return
			}
			if ids[0] != gitAuthConfig.ID {
				return
			}
			if ids[1] != workspace.OwnerID.String() {
				return
			}
			select {
			case authChan <- struct{}{}:
			default:
			}
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to listen for git auth token.",
				Detail:  err.Error(),
			})
			return
		}
		defer cancelFunc()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-authChan:
			}
			gitAuthLink, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
				ProviderID: gitAuthConfig.ID,
				UserID:     workspace.OwnerID,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to get git auth link.",
					Detail:  err.Error(),
				})
				return
			}
			if gitAuthLink.OAuthExpiry.Before(database.Now()) {
				continue
			}
			httpapi.Write(ctx, rw, http.StatusOK, formatGitAuthAccessToken(gitAuthConfig.Type, gitAuthLink.OAuthAccessToken))
			return
		}
	}

	// This is the URL that will redirect the user with a state token.
	redirectURL, err := api.AccessURL.Parse(fmt.Sprintf("/gitauth/%s", gitAuthConfig.ID))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to parse access URL.",
			Detail:  err.Error(),
		})
		return
	}

	gitAuthLink, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
		ProviderID: gitAuthConfig.ID,
		UserID:     workspace.OwnerID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get git auth link.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentGitAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}

	// If the token is expired and refresh is disabled, we prompt
	// the user to authenticate again.
	if gitAuthConfig.NoRefresh && gitAuthLink.OAuthExpiry.Before(database.Now()) {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentGitAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}

	token, err := gitAuthConfig.TokenSource(ctx, &oauth2.Token{
		AccessToken:  gitAuthLink.OAuthAccessToken,
		RefreshToken: gitAuthLink.OAuthRefreshToken,
		Expiry:       gitAuthLink.OAuthExpiry,
	}).Token()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentGitAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}

	if token.AccessToken != gitAuthLink.OAuthAccessToken {
		// Update it
		err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
			ProviderID:        gitAuthConfig.ID,
			UserID:            workspace.OwnerID,
			UpdatedAt:         database.Now(),
			OAuthAccessToken:  token.AccessToken,
			OAuthRefreshToken: token.RefreshToken,
			OAuthExpiry:       token.Expiry,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update git auth link.",
				Detail:  err.Error(),
			})
			return
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, formatGitAuthAccessToken(gitAuthConfig.Type, token.AccessToken))
}

// Provider types have different username/password formats.
func formatGitAuthAccessToken(typ codersdk.GitProvider, token string) codersdk.WorkspaceAgentGitAuthResponse {
	var resp codersdk.WorkspaceAgentGitAuthResponse
	switch typ {
	case codersdk.GitProviderGitLab:
		// https://stackoverflow.com/questions/25409700/using-gitlab-token-to-clone-without-authentication
		resp = codersdk.WorkspaceAgentGitAuthResponse{
			Username: "oauth2",
			Password: token,
		}
	case codersdk.GitProviderBitBucket:
		// https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/#Cloning-a-repository-with-an-access-token
		resp = codersdk.WorkspaceAgentGitAuthResponse{
			Username: "x-token-auth",
			Password: token,
		}
	default:
		resp = codersdk.WorkspaceAgentGitAuthResponse{
			Username: token,
		}
	}
	return resp
}

func (api *API) gitAuthCallback(gitAuthConfig *gitauth.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			state  = httpmw.OAuth2(r)
			apiKey = httpmw.APIKey(r)
		)

		_, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			ProviderID: gitAuthConfig.ID,
			UserID:     apiKey.UserID,
		})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to get git auth link.",
					Detail:  err.Error(),
				})
				return
			}

			_, err = api.Database.InsertGitAuthLink(ctx, database.InsertGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		} else {
			err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				UpdatedAt:         database.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		}

		err = api.Pubsub.Publish("gitauth", []byte(fmt.Sprintf("%s|%s", gitAuthConfig.ID, apiKey.UserID)))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to publish auth update.",
				Detail:  err.Error(),
			})
			return
		}

		// This is a nicely rendered screen on the frontend
		http.Redirect(rw, r, "/gitauth", http.StatusTemporaryRedirect)
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
