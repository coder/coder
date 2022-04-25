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

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func (api *api) workspaceAgent(rw http.ResponseWriter, r *http.Request) {
	agent := httpmw.WorkspaceAgentParam(r)
	apiAgent, err := convertWorkspaceAgent(agent, api.AgentConnectionUpdateFrequency)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspace agent: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, apiAgent)
}

func (api *api) workspaceAgentDial(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	agent := httpmw.WorkspaceAgentParam(r)
	apiAgent, err := convertWorkspaceAgent(agent, api.AgentConnectionUpdateFrequency)
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
		ChannelID: agent.ID.String(),
		Logger:    api.Logger.Named("peerbroker-proxy-dial"),
		Pubsub:    api.Pubsub,
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("serve: %s", err))
		return
	}
}

func (api *api) workspaceAgentMe(rw http.ResponseWriter, r *http.Request) {
	agent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(agent, api.AgentConnectionUpdateFrequency)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspace agent: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, apiAgent)
}

func (api *api) workspaceAgentListen(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	agent := httpmw.WorkspaceAgent(r)
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), agent.ResourceID)
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
		ChannelID: agent.ID.String(),
		Pubsub:    api.Pubsub,
		Logger:    api.Logger.Named("peerbroker-proxy-listen"),
	})
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	defer closer.Close()
	firstConnectedAt := agent.FirstConnectedAt
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
	disconnectedAt := agent.DisconnectedAt
	updateConnectionTimes := func() error {
		err = api.Database.UpdateWorkspaceAgentConnectionByID(r.Context(), database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               agent.ID,
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
		latestBuild, err := api.Database.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), build.WorkspaceID)
		if err != nil {
			return err
		}
		if build.ID.String() != latestBuild.ID.String() {
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

	api.Logger.Info(r.Context(), "accepting agent", slog.F("resource", resource), slog.F("agent", agent))

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

func (api *api) workspaceAgentICEServers(rw http.ResponseWriter, _ *http.Request) {
	httpapi.Write(rw, http.StatusOK, api.ICEServers)
}

// workspaceAgentTurn proxies a WebSocket connection to the TURN server.
func (api *api) workspaceAgentTurn(rw http.ResponseWriter, r *http.Request) {
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

func convertWorkspaceAgent(dbAgent database.WorkspaceAgent, agentUpdateFrequency time.Duration) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal: %w", err)
		}
	}
	agent := codersdk.WorkspaceAgent{
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
	}
	if dbAgent.FirstConnectedAt.Valid {
		agent.FirstConnectedAt = &dbAgent.FirstConnectedAt.Time
	}
	if dbAgent.LastConnectedAt.Valid {
		agent.LastConnectedAt = &dbAgent.LastConnectedAt.Time
	}
	if dbAgent.DisconnectedAt.Valid {
		agent.DisconnectedAt = &dbAgent.DisconnectedAt.Time
	}
	switch {
	case !dbAgent.FirstConnectedAt.Valid:
		// If the agent never connected, it's waiting for the compute
		// to start up.
		agent.Status = codersdk.WorkspaceAgentConnecting
	case dbAgent.DisconnectedAt.Time.After(dbAgent.LastConnectedAt.Time):
		// If we've disconnected after our last connection, we know the
		// agent is no longer connected.
		agent.Status = codersdk.WorkspaceAgentDisconnected
	case agentUpdateFrequency*2 >= database.Now().Sub(dbAgent.LastConnectedAt.Time):
		// The connection updated it's timestamp within the update frequency.
		// We multiply by two to allow for some lag.
		agent.Status = codersdk.WorkspaceAgentConnected
	case database.Now().Sub(dbAgent.LastConnectedAt.Time) > agentUpdateFrequency*2:
		// The connection died without updating the last connected.
		agent.Status = codersdk.WorkspaceAgentDisconnected
	}

	return agent, nil
}
