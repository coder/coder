package coderd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func (api *API) workspaceAgent(rw http.ResponseWriter, r *http.Request) {
	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, api.AgentConnectionUpdateFrequency)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspace agent: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, apiAgent)
}

func (api *API) workspaceAgentDial(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, api.AgentConnectionUpdateFrequency)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspace agent: %s", err),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("Agent isn't connected! Status: %s", apiAgent.Status),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), config)
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	err = peerbroker.ProxyListen(r.Context(), session, peerbroker.ProxyOptions{
		ChannelID: workspaceAgent.ID.String(),
		Logger:    api.Logger.Named("peerbroker-proxy-dial"),
		Pubsub:    api.Pubsub,
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("serve: %s", err))
		return
	}
}

func (api *API) workspaceAgentMetadata(rw http.ResponseWriter, r *http.Request) {
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, api.AgentConnectionUpdateFrequency)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspace agent: %s", err),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace resource: %s", err),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), build.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}
	owner, err := api.Database.GetUserByID(r.Context(), workspace.OwnerID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, agent.Metadata{
		OwnerEmail:           owner.Email,
		OwnerUsername:        owner.Username,
		EnvironmentVariables: apiAgent.EnvironmentVariables,
		StartupScript:        apiAgent.StartupScript,
		Directory:            apiAgent.Directory,
	})
}

func (api *API) workspaceAgentListen(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}

	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), config)
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	closer, err := peerbroker.ProxyDial(proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(session)), peerbroker.ProxyOptions{
		ChannelID: workspaceAgent.ID.String(),
		Pubsub:    api.Pubsub,
		Logger:    api.Logger.Named("peerbroker-proxy-listen"),
	})
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	defer closer.Close()
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
		err = api.Database.UpdateWorkspaceAgentConnectionByID(r.Context(), database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               workspaceAgent.ID,
			FirstConnectedAt: firstConnectedAt,
			LastConnectedAt:  lastConnectedAt,
			DisconnectedAt:   disconnectedAt,
		})
		if err != nil {
			return err
		}
		return nil
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
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

	defer func() {
		disconnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
		_ = updateConnectionTimes()
	}()

	err = ensureLatestBuild()
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, "")
		return
	}
	err = updateConnectionTimes()
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}

	api.Logger.Info(r.Context(), "accepting agent", slog.F("resource", resource), slog.F("agent", workspaceAgent))

	ticker := time.NewTicker(api.AgentConnectionUpdateFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-session.CloseChan():
			return
		case <-ticker.C:
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
}

func (api *API) workspaceAgentICEServers(rw http.ResponseWriter, _ *http.Request) {
	httpapi.Write(rw, http.StatusOK, api.ICEServers)
}

// workspaceAgentTurn proxies a WebSocket connection to the TURN server.
func (api *API) workspaceAgentTurn(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	localAddress, _ := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	remoteAddress := &net.TCPAddr{
		IP: net.ParseIP(r.RemoteAddr),
	}
	// By default requests have the remote address and port.
	host, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("get remote address: %s", err),
		})
		return
	}
	remoteAddress.IP = net.ParseIP(host)
	remoteAddress.Port, err = strconv.Atoi(port)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("remote address %q has no parsable port: %s", r.RemoteAddr, err),
		})
		return
	}

	wsConn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	defer func() {
		_ = wsConn.Close(websocket.StatusNormalClosure, "")
	}()
	netConn := websocket.NetConn(r.Context(), wsConn, websocket.MessageBinary)
	api.Logger.Debug(r.Context(), "accepting turn connection", slog.F("remote-address", r.RemoteAddr), slog.F("local-address", localAddress))
	select {
	case <-api.TURNServer.Accept(netConn, remoteAddress, localAddress).Closed():
	case <-r.Context().Done():
	}
	api.Logger.Debug(r.Context(), "completed turn connection", slog.F("remote-address", r.RemoteAddr), slog.F("local-address", localAddress))
}

// workspaceAgentPTY spawns a PTY and pipes it over a WebSocket.
// This is used for the web terminal.
func (api *API) workspaceAgentPTY(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, api.AgentConnectionUpdateFrequency)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspace agent: %s", err),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
			Message: fmt.Sprintf("agent must be in the connected state: %s", apiAgent.Status),
		})
		return
	}

	reconnect, err := uuid.Parse(r.URL.Query().Get("reconnect"))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("reconnection must be a uuid: %s", err),
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
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "ended")
	}()
	// Accept text connections, because it's more developer friendly.
	wsNetConn := websocket.NetConn(r.Context(), conn, websocket.MessageBinary)
	agentConn, err := api.dialWorkspaceAgent(r, workspaceAgent.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("dial workspace agent: %s", err))
		return
	}
	defer agentConn.Close()
	ptNetConn, err := agentConn.ReconnectingPTY(reconnect.String(), uint16(height), uint16(width))
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

// dialWorkspaceAgent connects to a workspace agent by ID.
func (api *API) dialWorkspaceAgent(r *http.Request, agentID uuid.UUID) (*agent.Conn, error) {
	client, server := provisionersdk.TransportPipe()
	go func() {
		_ = peerbroker.ProxyListen(r.Context(), server, peerbroker.ProxyOptions{
			ChannelID: agentID.String(),
			Logger:    api.Logger.Named("peerbroker-proxy-dial"),
			Pubsub:    api.Pubsub,
		})
		_ = client.Close()
		_ = server.Close()
	}()

	peerClient := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
	stream, err := peerClient.NegotiateConnection(r.Context())
	if err != nil {
		return nil, xerrors.Errorf("negotiate: %w", err)
	}
	options := &peer.ConnOptions{}
	options.SettingEngine.SetSrflxAcceptanceMinWait(0)
	options.SettingEngine.SetRelayAcceptanceMinWait(0)
	// Use the ProxyDialer for the TURN server.
	// This is required for connections where P2P is not enabled.
	options.SettingEngine.SetICEProxyDialer(turnconn.ProxyDialer(func() (c net.Conn, err error) {
		clientPipe, serverPipe := net.Pipe()
		go func() {
			<-r.Context().Done()
			_ = clientPipe.Close()
			_ = serverPipe.Close()
		}()
		localAddress, _ := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
		remoteAddress := &net.TCPAddr{
			IP: net.ParseIP(r.RemoteAddr),
		}
		// By default requests have the remote address and port.
		host, port, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return nil, xerrors.Errorf("split remote address: %w", err)
		}
		remoteAddress.IP = net.ParseIP(host)
		remoteAddress.Port, err = strconv.Atoi(port)
		if err != nil {
			return nil, xerrors.Errorf("convert remote port: %w", err)
		}
		api.TURNServer.Accept(clientPipe, remoteAddress, localAddress)
		return serverPipe, nil
	}))
	peerConn, err := peerbroker.Dial(stream, append(api.ICEServers, turnconn.Proxy), options)
	if err != nil {
		return nil, xerrors.Errorf("dial: %w", err)
	}
	return &agent.Conn{
		Negotiator: peerClient,
		Conn:       peerConn,
	}, nil
}

func convertWorkspaceAgent(dbAgent database.WorkspaceAgent, agentUpdateFrequency time.Duration) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal: %w", err)
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
		EnvironmentVariables: envs,
		Directory:            dbAgent.Directory,
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
	case agentUpdateFrequency*2 >= database.Now().Sub(dbAgent.LastConnectedAt.Time):
		// The connection updated it's timestamp within the update frequency.
		// We multiply by two to allow for some lag.
		workspaceAgent.Status = codersdk.WorkspaceAgentConnected
	case database.Now().Sub(dbAgent.LastConnectedAt.Time) > agentUpdateFrequency*2:
		// The connection died without updating the last connected.
		workspaceAgent.Status = codersdk.WorkspaceAgentDisconnected
	}

	return workspaceAgent, nil
}
