package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/peer"
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
	res, err := c.request(ctx, http.MethodPost, "/api/v2/workspaceagents/google-instance-identity", GoogleInstanceIdentityToken{
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

	res, err = c.request(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", AWSInstanceIdentityToken{
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

// ListenWorkspaceAgent connects as a workspace agent.
// It obtains the agent ID based off the session token.
func (c *Client) ListenWorkspaceAgent(ctx context.Context, opts *peer.ConnOptions) (*peerbroker.Listener, error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceagents/me")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  httpmw.AuthCookie,
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
	return peerbroker.Listen(session, func(ctx context.Context) ([]webrtc.ICEServer, error) {
		return []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}}, nil
	}, opts)
}

// DialWorkspaceAgent creates a connection to the specified resource.
func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, iceServers []webrtc.ICEServer, opts *peer.ConnOptions) (*agent.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/dial", agentID.String()))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  httpmw.AuthCookie,
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
	peerConn, err := peerbroker.Dial(stream, iceServers, opts)
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
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s", id), nil)
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
