package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v3"
	"golang.org/x/net/proxy"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peer/peerwg"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
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

// ListenWorkspaceAgent connects as a workspace agent identifying with the session token.
// On each inbound connection request, connection info is fetched.
func (c *Client) ListenWorkspaceAgent(ctx context.Context, logger slog.Logger) (agent.Metadata, *peerbroker.Listener, error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceagents/me/listen")
	if err != nil {
		return agent.Metadata{}, nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return agent.Metadata{}, nil, xerrors.Errorf("create cookie jar: %w", err)
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
			return agent.Metadata{}, nil, err
		}
		return agent.Metadata{}, nil, readBodyAsError(res)
	}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return agent.Metadata{}, nil, xerrors.Errorf("multiplex client: %w", err)
	}
	listener, err := peerbroker.Listen(session, func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error) {
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
	if err != nil {
		return agent.Metadata{}, nil, xerrors.Errorf("listen peerbroker: %w", err)
	}
	res, err = c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/metadata", nil)
	if err != nil {
		return agent.Metadata{}, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return agent.Metadata{}, nil, readBodyAsError(res)
	}
	var agentMetadata agent.Metadata
	return agentMetadata, listener, json.NewDecoder(res.Body).Decode(&agentMetadata)
}

// PostWireguardPeer announces your public keys and IPv6 address to the
// specified recipient.
func (c *Client) PostWireguardPeer(ctx context.Context, workspaceID uuid.UUID, peerMsg peerwg.Handshake) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaceagents/%s/peer?workspace=%s",
		peerMsg.Recipient,
		workspaceID.String(),
	), peerMsg)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return readBodyAsError(res)
	}

	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

// WireguardPeerListener listens for wireguard peer messages. Peer messages are
// sent when a new client wants to connect. Once receiving a peer message, the
// peer should be added to the NetworkMap of the wireguard interface.
func (c *Client) WireguardPeerListener(ctx context.Context, logger slog.Logger) (<-chan peerwg.Handshake, func(), error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceagents/me/wireguardlisten")
	if err != nil {
		return nil, nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("create cookie jar: %w", err)
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
			return nil, nil, xerrors.Errorf("websocket dial: %w", err)
		}
		return nil, nil, readBodyAsError(res)
	}

	ch := make(chan peerwg.Handshake, 1)
	go func() {
		defer conn.Close(websocket.StatusGoingAway, "")
		defer close(ch)

		for {
			_, message, err := conn.Read(ctx)
			if err != nil {
				break
			}

			var msg peerwg.Handshake
			err = msg.UnmarshalText(message)
			if err != nil {
				logger.Error(ctx, "unmarshal wireguard peer message", slog.Error(err))
				continue
			}

			ch <- msg
		}
	}()

	return ch, func() { _ = conn.Close(websocket.StatusGoingAway, "") }, nil
}

// UploadWorkspaceAgentKeys uploads the public keys of the workspace agent that
// were generated on startup. These keys are used by clients to communicate with
// the workspace agent over the wireguard interface.
func (c *Client) UploadWorkspaceAgentKeys(ctx context.Context, keys agent.WireguardPublicKeys) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/keys", keys)
	if err != nil {
		return xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return readBodyAsError(res)
	}
	return nil
}

// DialWorkspaceAgent creates a connection to the specified resource.
func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *peer.ConnOptions) (*agent.Conn, error) {
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
	return &agent.Conn{
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
