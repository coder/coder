package codersdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v3"
	"golang.org/x/net/proxy"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/tailnet"
	"github.com/coder/retry"
)

type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

type AWSInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Document  string `json:"document" validate:"required"`
}

type AzureInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Encoding  string `json:"encoding" validate:"required"`
}

// WorkspaceAgentAuthenticateResponse is returned when an instance ID
// has been exchanged for a session token.
type WorkspaceAgentAuthenticateResponse struct {
	SessionToken string `json:"session_token"`
}

// WorkspaceAgentConnectionInfo returns required information for establishing
// a connection with a workspace.
type WorkspaceAgentConnectionInfo struct {
	DERPMap     *tailcfg.DERPMap `json:"derp_map"`
	IPAddresses []netip.Addr     `json:"ip_address"`
}

// AuthWorkspaceGoogleInstanceIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (WorkspaceAgentAuthenticateResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.HTTPClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/google-instance-identity", GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAWSInstanceIdentity uses the Amazon Metadata API to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceAWSInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	token, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	signature, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	document, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	res, err = c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", AWSInstanceIdentityToken{
		Signature: string(signature),
		Document:  string(document),
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAzureInstanceIdentity uses the Azure Instance Metadata Service to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
func (c *Client) AuthWorkspaceAzureInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/metadata/attested/document?api-version=2020-09-01", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("Metadata", "true")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()

	var token AzureInstanceIdentityToken
	err = json.NewDecoder(res.Body).Decode(&token)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}

	res, err = c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/azure-instance-identity", token)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// WorkspaceAgentMetadata fetches metadata for the currently authenticated workspace agent.
func (c *Client) WorkspaceAgentMetadata(ctx context.Context) (agent.Metadata, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/metadata", nil)
	if err != nil {
		return agent.Metadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return agent.Metadata{}, readBodyAsError(res)
	}
	var agentMetadata agent.Metadata
	return agentMetadata, json.NewDecoder(res.Body).Decode(&agentMetadata)
}

// ListenWorkspaceAgent connects as a workspace agent identifying with the session token.
// On each inbound connection request, connection info is fetched.
func (c *Client) ListenWorkspaceAgent(ctx context.Context, logger slog.Logger) (*peerbroker.Listener, error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceagents/me/listen")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return peerbroker.Listen(session, func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error) {
		// This can be cached if it adds to latency too much.
		res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/iceservers", nil)
		if err != nil {
			return nil, nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, nil, readBodyAsError(res)
		}
		var iceServers []webrtc.ICEServer
		err = json.NewDecoder(res.Body).Decode(&iceServers)
		if err != nil {
			return nil, nil, err
		}

		options := webrtc.SettingEngine{}
		options.SetSrflxAcceptanceMinWait(0)
		options.SetRelayAcceptanceMinWait(0)
		options.SetICEProxyDialer(c.turnProxyDialer(ctx, httpClient, "/api/v2/workspaceagents/me/turn"))
		iceServers = append(iceServers, turnconn.Proxy)
		return iceServers, &peer.ConnOptions{
			SettingEngine: options,
			Logger:        logger,
		}, nil
	})
}

// UpdateWorkspaceAgentNode publishes a node update for the provided agent.
// This should be used to negotiate a connection.
func (c *Client) UpdateWorkspaceAgentNode(ctx context.Context, agentID uuid.UUID, node *tailnet.Node) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaceagents/%s/node",
		agentID,
	), node)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

func (c *Client) ListenWorkspaceAgentTailnet(ctx context.Context) (*websocket.Conn, error) {
	coordinateURL, err := c.URL.Parse("/api/v2/workspaceagents/me/coordinate")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, _, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	return conn, err
}

func (c *Client) DialWorkspaceAgentTailnet(ctx context.Context, logger slog.Logger, agentID uuid.UUID) (agent.Conn, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/connection", agentID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var connInfo WorkspaceAgentConnectionInfo
	err = json.NewDecoder(res.Body).Decode(&connInfo)
	if err != nil {
		return nil, xerrors.Errorf("decode conn info: %w", err)
	}

	ip := tailnet.IP()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:   connInfo.DERPMap,
		Logger:    logger,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}

	coordinateURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", agentID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(coordinateURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	go func() {
		for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
			logger.Debug(ctx, "connecting")
			ws, _, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
				HTTPClient: httpClient,
				// Need to disable compression to avoid a data-race.
				CompressionMode: websocket.CompressionDisabled,
			})
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				logger.Debug(ctx, "failed to dial", slog.Error(err))
				continue
			}
			sendNode, errChan := tailnet.ServeCoordinator(ctx, ws, func(node []*tailnet.Node) error {
				return conn.UpdateNodes(node)
			})
			conn.SetNodeCallback(sendNode)
			logger.Debug(ctx, "serving coordinator")
			err = <-errChan
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				logger.Debug(ctx, "error serving coordinator", slog.Error(err))
				continue
			}
		}
	}()
	return &agent.TailnetConn{
		Target: connInfo.IPAddresses[0],
		Conn:   conn,
	}, nil
}

// DialWorkspaceAgent creates a connection to the specified resource.
func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *peer.ConnOptions) (agent.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/dial", agentID.String()))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	client := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(session))
	stream, err := client.NegotiateConnection(ctx)
	if err != nil {
		return nil, xerrors.Errorf("negotiate connection: %w", err)
	}

	res, err = c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/iceservers", agentID.String()), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var iceServers []webrtc.ICEServer
	err = json.NewDecoder(res.Body).Decode(&iceServers)
	if err != nil {
		return nil, err
	}

	if options == nil {
		options = &peer.ConnOptions{}
	}
	options.SettingEngine.SetSrflxAcceptanceMinWait(0)
	options.SettingEngine.SetRelayAcceptanceMinWait(0)
	options.SettingEngine.SetICEProxyDialer(c.turnProxyDialer(ctx, httpClient, fmt.Sprintf("/api/v2/workspaceagents/%s/turn", agentID.String())))
	iceServers = append(iceServers, turnconn.Proxy)

	peerConn, err := peerbroker.Dial(stream, iceServers, options)
	if err != nil {
		return nil, xerrors.Errorf("dial peer: %w", err)
	}
	return &agent.WebRTCConn{
		Negotiator: client,
		Conn:       peerConn,
	}, nil
}

// WorkspaceAgent returns an agent by ID.
func (c *Client) WorkspaceAgent(ctx context.Context, id uuid.UUID) (WorkspaceAgent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s", id), nil)
	if err != nil {
		return WorkspaceAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgent{}, readBodyAsError(res)
	}
	var workspaceAgent WorkspaceAgent
	return workspaceAgent, json.NewDecoder(res.Body).Decode(&workspaceAgent)
}

// WorkspaceAgentReconnectingPTY spawns a PTY that reconnects using the token provided.
// It communicates using `agent.ReconnectingPTYRequest` marshaled as JSON.
// Responses are PTY output that can be rendered.
func (c *Client) WorkspaceAgentReconnectingPTY(ctx context.Context, agentID, reconnect uuid.UUID, height, width int, command string) (net.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/pty?reconnect=%s&height=%d&width=%d&command=%s", agentID, reconnect, height, width, command))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
}

func (c *Client) turnProxyDialer(ctx context.Context, httpClient *http.Client, path string) proxy.Dialer {
	return turnconn.ProxyDialer(func() (net.Conn, error) {
		turnURL, err := c.URL.Parse(path)
		if err != nil {
			return nil, xerrors.Errorf("parse url: %w", err)
		}
		conn, res, err := websocket.Dial(ctx, turnURL.String(), &websocket.DialOptions{
			HTTPClient: httpClient,
			// Need to disable compression to avoid a data-race.
			CompressionMode: websocket.CompressionDisabled,
		})
		if err != nil {
			if res == nil {
				return nil, err
			}
			return nil, readBodyAsError(res)
		}
		return websocket.NetConn(ctx, conn, websocket.MessageBinary), nil
	})
}

// workspaceAgentNodeBroker is used to listen for node updates
// and write them.
type workspaceAgentNodeBroker struct {
	conn *websocket.Conn
}

func (w *workspaceAgentNodeBroker) Read(ctx context.Context) (*tailnet.Node, error) {
	var node tailnet.Node
	err := wsjson.Read(ctx, w.conn, &node)
	return &node, err
}

func (w *workspaceAgentNodeBroker) Write(ctx context.Context, node *tailnet.Node) error {
	return wsjson.Write(ctx, w.conn, node)
}

func (w *workspaceAgentNodeBroker) Close() error {
	return w.conn.Close(websocket.StatusGoingAway, "")
}
