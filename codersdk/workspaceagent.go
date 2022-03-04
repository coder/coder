package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

// AuthWorkspaceAgentWithGoogleInstanceIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceAgentWithGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (coderd.WorkspaceAgentAuthenticateResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.httpClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return coderd.WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.request(ctx, http.MethodPost, "/api/v2/workspaceagent/authenticate/google-instance-identity", coderd.GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return coderd.WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp coderd.WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DialWorkspaceAgent creates a connection to the specified resource.
func (c *Client) DialWorkspaceAgent(ctx context.Context, resource uuid.UUID) (proto.DRPCPeerBrokerClient, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaceresource/%s/dial", resource.String()))
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
	return proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(session)), nil
}

// ListenWorkspaceAgent connects as a workspace agent.
// It obtains the agent ID based off the session token.
func (c *Client) ListenWorkspaceAgent(ctx context.Context, opts *peer.ConnOptions) (*peerbroker.Listener, error) {
	serverURL, err := c.URL.Parse("/api/v2/workspaceagent/listen")
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
	return peerbroker.Listen(session, opts)
}
