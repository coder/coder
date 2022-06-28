package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/tabbed/pqtype"
	"go4.org/mem"
	"golang.org/x/xerrors"
	"inet.af/netaddr"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace agent applications.",
			Detail:  err.Error(),
		})
		return
	}
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
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
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, nil, api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("Agent isn't connected! Status: %s.", apiAgent.Status),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := websocketNetConn(r.Context(), conn, websocket.MessageBinary)
	defer wsNetConn.Close() // Also closes conn.

	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(wsNetConn, config)
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	err = peerbroker.ProxyListen(ctx, session, peerbroker.ProxyOptions{
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
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, nil, api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), build.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}
	owner, err := api.Database.GetUserByID(r.Context(), workspace.OwnerID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace owner.",
			Detail:  err.Error(),
		})
		return
	}

	ipp, ok := netaddr.FromStdIPNet(&workspaceAgent.IPAddresses.IPNet)
	if !ok {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Workspace agent has an invalid ipv6 address.",
			Detail:  workspaceAgent.WireguardNodeIPv6.IPNet.String(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, agent.Metadata{
		TailscaleAddresses: []netaddr.IPPrefix{ipp},

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
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
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
		httpapi.Write(rw, http.StatusForbidden, httpapi.Response{
			Message: "Agent trying to connect from non-latest build.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := websocketNetConn(r.Context(), conn, websocket.MessageBinary)
	defer wsNetConn.Close() // Also closes conn.

	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(wsNetConn, config)
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
		err = api.Database.UpdateWorkspaceAgentConnectionByID(ctx, database.UpdateWorkspaceAgentConnectionByIDParams{
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

	api.Logger.Info(ctx, "accepting agent", slog.F("resource", resource), slog.F("agent", workspaceAgent))

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
			Message: "Invalid remote address.",
			Detail:  err.Error(),
		})
		return
	}
	remoteAddress.IP = net.ParseIP(host)
	remoteAddress.Port, err = strconv.Atoi(port)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("Port for remote address %q must be an integer.", r.RemoteAddr),
			Detail:  err.Error(),
		})
		return
	}

	wsConn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := websocketNetConn(r.Context(), wsConn, websocket.MessageBinary)
	defer wsNetConn.Close() // Also closes conn.

	api.Logger.Debug(ctx, "accepting turn connection", slog.F("remote-address", r.RemoteAddr), slog.F("local-address", localAddress))
	select {
	case <-api.TURNServer.Accept(wsNetConn, remoteAddress, localAddress).Closed():
	case <-ctx.Done():
	}
	api.Logger.Debug(ctx, "completed turn connection", slog.F("remote-address", r.RemoteAddr), slog.F("local-address", localAddress))
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
	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	apiAgent, err := convertWorkspaceAgent(workspaceAgent, nil, api.AgentInactiveDisconnectTimeout)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	reconnect, err := uuid.Parse(r.URL.Query().Get("reconnect"))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Query param 'reconnect' must be a valid UUID.",
			Validations: []httpapi.Error{
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
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
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

func (a *API) derpMap(rw http.ResponseWriter, _ *http.Request) {
	var derpPort int
	rawPort := a.AccessURL.Port()
	if rawPort == "" {
		if a.AccessURL.Scheme == "https" {
			derpPort = 443
		} else {
			derpPort = 80
		}
	} else {
		var err error
		derpPort, err = strconv.Atoi(a.AccessURL.Port())
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: "Get ",
			})
			return
		}
	}

	httpapi.Write(rw, http.StatusOK, &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: &tailcfg.DERPRegion{
				RegionID:   1,
				RegionCode: "coder",
				RegionName: "Coder",
				Nodes: []*tailcfg.DERPNode{{
					Name:     "1a",
					RegionID: 1,
					HostName: a.AccessURL.Hostname(),
					DERPPort: derpPort,
					STUNPort: -1,
				}},
			},
		},
	})
}

// workspaceAgentSelfNetmap accepts a WebSocket that reads
// node updates and sends new node connection info.
func (api *API) workspaceAgentSelfNetmap(rw http.ResponseWriter, r *http.Request) {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgent(r)

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, nc := websocketNetConn(r.Context(), conn, websocket.MessageBinary)
	agentIDBytes, _ := workspaceAgent.ID.MarshalText()
	subCancel, err := api.Pubsub.Subscribe("tailnet_dial", func(ctx context.Context, message []byte) {
		// Since we subscribe to all peer broadcasts, we do a light check to
		// make sure we're the intended recipient without fully decoding the
		// message.
		if len(message) < len(agentIDBytes) {
			api.Logger.Error(ctx, "wireguard peer message too short", slog.F("got", len(message)))
			return
		}
		// We aren't the intended recipient.
		if !bytes.Equal(message[:len(agentIDBytes)-1], agentIDBytes) {
			return
		}
		_, _ = nc.Write(message)
	})
	if err != nil {
		api.Logger.Error(ctx, "pubsub listen", slog.Error(err))
		return
	}
	defer subCancel()

	decoder := json.NewDecoder(nc)
	for {
		var node tailnet.Node
		err = decoder.Decode(&node)
		if err != nil {
			return
		}
		err := api.Database.UpdateWorkspaceAgentNetworkByID(ctx, database.UpdateWorkspaceAgentNetworkByIDParams{
			ID:                    workspaceAgent.ID,
			NodePublicKey:         node.Key.String(),
			DERPLatency: ,
			TailnetNodePublicKey:  node.Key.String(),
			TailnetDiscoPublicKey: node.DiscoKey.String(),
			TailnetNodeDERP:       node.DERP,
			UpdatedAt:             database.Now(),
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: "Internal error setting agent keys.",
				Detail:  err.Error(),
			})
			return
		}
		nodeData, err := json.Marshal(node)
		if err != nil {
			return
		}
		id, _ := workspaceAgent.ID.MarshalText()
		msg := base64.StdEncoding.EncodeToString(nodeData)
		err = api.Pubsub.Publish("tailnet_listen", append(id, msg...))
		if err != nil {
			conn.Close(websocket.StatusAbnormalClosure, err.Error())
			return
		}
	}
}

func (api *API) workspaceAgentNetmap(r *http.Request, rw http.ResponseWriter) {

}

// dialWorkspaceAgent connects to a workspace agent by ID. Only rely on
// r.Context() for cancellation if it's use is safe or r.Hijack() has
// not been performed.
func (api *API) dialWorkspaceAgent(r *http.Request, agentID uuid.UUID) (*agent.Conn, error) {
	client, server := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		_ = peerbroker.ProxyListen(ctx, server, peerbroker.ProxyOptions{
			ChannelID: agentID.String(),
			Logger:    api.Logger.Named("peerbroker-proxy-dial"),
			Pubsub:    api.Pubsub,
		})
		_ = client.Close()
		_ = server.Close()
	}()

	peerClient := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
	stream, err := peerClient.NegotiateConnection(ctx)
	if err != nil {
		cancelFunc()
		return nil, xerrors.Errorf("negotiate: %w", err)
	}
	options := &peer.ConnOptions{
		Logger: api.Logger.Named("agent-dialer"),
	}
	options.SettingEngine.SetSrflxAcceptanceMinWait(0)
	options.SettingEngine.SetRelayAcceptanceMinWait(0)
	// Use the ProxyDialer for the TURN server.
	// This is required for connections where P2P is not enabled.
	options.SettingEngine.SetICEProxyDialer(turnconn.ProxyDialer(func() (c net.Conn, err error) {
		clientPipe, serverPipe := net.Pipe()
		go func() {
			<-ctx.Done()
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
		cancelFunc()
		return nil, xerrors.Errorf("dial: %w", err)
	}
	go func() {
		<-peerConn.Closed()
		cancelFunc()
	}()
	return &agent.Conn{
		Negotiator: peerClient,
		Conn:       peerConn,
	}, nil
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

func inetToNetaddr(inet pqtype.Inet) netaddr.IPPrefix {
	if !inet.Valid {
		return netaddr.IPPrefixFrom(netaddr.IPv6Unspecified(), 128)
	}

	ipp, ok := netaddr.FromStdIPNet(&inet.IPNet)
	if !ok {
		return netaddr.IPPrefixFrom(netaddr.IPv6Unspecified(), 128)
	}

	return ipp
}

func convertWorkspaceAgent(dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, agentInactiveDisconnectTimeout time.Duration) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal: %w", err)
		}
	}
	nodePublicKey, err := key.ParseNodePublicUntyped(mem.S(dbAgent.NodePublicKey))
	if err != nil {
		return codersdk.WorkspaceAgent{}, xerrors.Errorf("parse node public key: %w", err)
	}
	var discoPublicKey key.DiscoPublic
	err = discoPublicKey.UnmarshalText([]byte(dbAgent.DiscoPublicKey))
	if err != nil {
		return codersdk.WorkspaceAgent{}, xerrors.Errorf("parse disco public key: %w", err)
	}
	ips := make([]netaddr.IP, 0)
	for _, ip := range dbAgent.IPAddresses {
		var ipData [16]byte
		copy(ipData[:], []byte(ip.IPNet.IP))
		ips = append(ips, netaddr.IPFrom16(ipData))
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
		Apps:                 apps,
		NodePublicKey:        nodePublicKey,
		DiscoPublicKey:       discoPublicKey,
		DERP:                 dbAgent.DERP,
		DERPLatency:          dbAgent.DERPLatency,
		IPAddresses:          ips,
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
