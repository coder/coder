package wsproxysdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
	agpl "github.com/coder/coder/tailnet"
)

// Client is a HTTP client for a subset of Coder API routes that external
// proxies need.
type Client struct {
	SDKClient *codersdk.Client
	// HACK: the issue-signed-app-token requests may issue redirect responses
	// (which need to be forwarded to the client), so the client we use to make
	// those requests must ignore redirects.
	sdkClientIgnoreRedirects *codersdk.Client
}

// New creates a external proxy client for the provided primary coder server
// URL.
func New(serverURL *url.URL) *Client {
	sdkClient := codersdk.New(serverURL)
	sdkClient.SessionTokenHeader = httpmw.WorkspaceProxyAuthTokenHeader

	sdkClientIgnoreRedirects := codersdk.New(serverURL)
	sdkClientIgnoreRedirects.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	sdkClientIgnoreRedirects.SessionTokenHeader = httpmw.WorkspaceProxyAuthTokenHeader

	return &Client{
		SDKClient:                sdkClient,
		sdkClientIgnoreRedirects: sdkClientIgnoreRedirects,
	}
}

// SetSessionToken sets the session token for the client. An error is returned
// if the session token is not in the correct format for external proxies.
func (c *Client) SetSessionToken(token string) error {
	c.SDKClient.SetSessionToken(token)
	c.sdkClientIgnoreRedirects.SetSessionToken(token)
	return nil
}

// SessionToken returns the currently set token for the client.
func (c *Client) SessionToken() string {
	return c.SDKClient.SessionToken()
}

// Request wraps the underlying codersdk.Client's Request method.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	return c.SDKClient.Request(ctx, method, path, body, opts...)
}

// RequestIgnoreRedirects wraps the underlying codersdk.Client's Request method
// on the client that ignores redirects.
func (c *Client) RequestIgnoreRedirects(ctx context.Context, method, path string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	return c.sdkClientIgnoreRedirects.Request(ctx, method, path, body, opts...)
}

// DialWorkspaceAgent calls the underlying codersdk.Client's DialWorkspaceAgent
// method.
func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *codersdk.DialWorkspaceAgentOptions) (agentConn *codersdk.WorkspaceAgentConn, err error) {
	return c.SDKClient.DialWorkspaceAgent(ctx, agentID, options)
}

type IssueSignedAppTokenResponse struct {
	// SignedTokenStr should be set as a cookie on the response.
	SignedTokenStr string `json:"signed_token_str"`
}

// IssueSignedAppToken issues a new signed app token for the provided app
// request. The error page will be returned as JSON. For use in external
// proxies, use IssueSignedAppTokenHTML instead.
func (c *Client) IssueSignedAppToken(ctx context.Context, req workspaceapps.IssueTokenRequest) (IssueSignedAppTokenResponse, error) {
	resp, err := c.RequestIgnoreRedirects(ctx, http.MethodPost, "/api/v2/workspaceproxies/me/issue-signed-app-token", req, func(r *http.Request) {
		// This forces any HTML error pages to be returned as JSON instead.
		r.Header.Set("Accept", "application/json")
	})
	if err != nil {
		return IssueSignedAppTokenResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return IssueSignedAppTokenResponse{}, codersdk.ReadBodyAsError(resp)
	}

	var res IssueSignedAppTokenResponse
	return res, json.NewDecoder(resp.Body).Decode(&res)
}

// IssueSignedAppTokenHTML issues a new signed app token for the provided app
// request. The error page will be returned as HTML in most cases, and will be
// written directly to the provided http.ResponseWriter.
func (c *Client) IssueSignedAppTokenHTML(ctx context.Context, rw http.ResponseWriter, req workspaceapps.IssueTokenRequest) (IssueSignedAppTokenResponse, bool) {
	writeError := func(rw http.ResponseWriter, err error) {
		res := codersdk.Response{
			Message: "Internal server error",
			Detail:  err.Error(),
		}
		rw.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(rw).Encode(res)
	}

	resp, err := c.RequestIgnoreRedirects(ctx, http.MethodPost, "/api/v2/workspaceproxies/me/issue-signed-app-token", req, func(r *http.Request) {
		r.Header.Set("Accept", "text/html")
	})
	if err != nil {
		writeError(rw, xerrors.Errorf("perform issue signed app token request: %w", err))
		return IssueSignedAppTokenResponse{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		// Copy the response to the ResponseWriter.
		for k, v := range resp.Header {
			rw.Header()[k] = v
		}
		rw.WriteHeader(resp.StatusCode)
		_, err = io.Copy(rw, resp.Body)
		if err != nil {
			writeError(rw, xerrors.Errorf("copy response body: %w", err))
		}
		return IssueSignedAppTokenResponse{}, false
	}

	var res IssueSignedAppTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		writeError(rw, xerrors.Errorf("decode response body: %w", err))
		return IssueSignedAppTokenResponse{}, false
	}
	return res, true
}

type RegisterWorkspaceProxyRequest struct {
	// AccessURL that hits the workspace proxy api.
	AccessURL string `json:"access_url"`
	// WildcardHostname that the workspace proxy api is serving for subdomain apps.
	WildcardHostname string `json:"wildcard_hostname"`
}

type RegisterWorkspaceProxyResponse struct {
	AppSecurityKey string `json:"app_security_key"`
}

func (c *Client) RegisterWorkspaceProxy(ctx context.Context, req RegisterWorkspaceProxyRequest) (RegisterWorkspaceProxyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies/me/register",
		req,
	)
	if err != nil {
		return RegisterWorkspaceProxyResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return RegisterWorkspaceProxyResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp RegisterWorkspaceProxyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) WorkspaceProxyGoingAway(ctx context.Context) error {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies/me/goingaway",
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

type CoordinateMessageType int

const (
	CoordinateMessageTypeSubscribe CoordinateMessageType = 1 + iota
	CoordinateMessageTypeUnsubscribe
	CoordinateMessageTypeNodeUpdate
)

type CoordinateMessage struct {
	Type    CoordinateMessageType `json:"type"`
	AgentID uuid.UUID             `json:"agent_id"`
	Node    *agpl.Node            `json:"node"`
}

type CoordinateNodes struct {
	Nodes []*agpl.Node
}

func (c *Client) DialCoordinator(ctx context.Context) (agpl.MultiAgentConn, error) {
	ctx, cancel := context.WithCancel(ctx)

	coordinateURL, err := c.SDKClient.URL.Parse("/api/v2/workspaceproxies/me/coordinate")
	if err != nil {
		cancel()
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	coordinateHeaders := make(http.Header)
	tokenHeader := codersdk.SessionTokenHeader
	if c.SDKClient.SessionTokenHeader != "" {
		tokenHeader = c.SDKClient.SessionTokenHeader
	}
	coordinateHeaders.Set(tokenHeader, c.SessionToken())

	//nolint:bodyclose
	conn, _, err := websocket.Dial(ctx, coordinateURL.String(), &websocket.DialOptions{
		HTTPClient: c.SDKClient.HTTPClient,
		HTTPHeader: coordinateHeaders,
	})
	if err != nil {
		cancel()
		return nil, xerrors.Errorf("dial coordinate websocket: %w", err)
	}

	go httpapi.HeartbeatClose(ctx, cancel, conn)

	nc := websocket.NetConn(ctx, conn, websocket.MessageText)
	rma := remoteMultiAgentHandler{
		sdk:              c,
		nc:               nc,
		legacyAgentCache: map[uuid.UUID]bool{},
	}

	ma := (&agpl.MultiAgent{
		ID:                uuid.New(),
		AgentIsLegacyFunc: rma.AgentIsLegacy,
		OnSubscribe:       rma.OnSubscribe,
		OnUnsubscribe:     rma.OnUnsubscribe,
		OnNodeUpdate:      rma.OnNodeUpdate,
		OnRemove:          func(uuid.UUID) { conn.Close(websocket.StatusGoingAway, "closed") },
	}).Init()

	go func() {
		defer cancel()
		dec := json.NewDecoder(nc)
		for {
			var msg CoordinateNodes
			err := dec.Decode(&msg)
			if err != nil {
				if xerrors.Is(err, io.EOF) {
					return
				}

				c.SDKClient.Logger().Error(ctx, "failed to decode coordinator nodes", slog.Error(err))
				return
			}

			err = ma.Enqueue(msg.Nodes)
			if err != nil {
				c.SDKClient.Logger().Error(ctx, "enqueue nodes from coordinator", slog.Error(err))
				continue
			}
		}
	}()

	return ma, nil
}

type remoteMultiAgentHandler struct {
	sdk *Client
	nc  net.Conn

	legacyMu           sync.RWMutex
	legacyAgentCache   map[uuid.UUID]bool
	legacySingleflight singleflight.Group[uuid.UUID, AgentIsLegacyResponse]
}

func (a *remoteMultiAgentHandler) writeJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return xerrors.Errorf("json marshal message: %w", err)
	}

	// Set a deadline so that hung connections don't put back pressure on the system.
	// Node updates are tiny, so even the dinkiest connection can handle them if it's not hung.
	err = a.nc.SetWriteDeadline(time.Now().Add(agpl.WriteTimeout))
	if err != nil {
		return xerrors.Errorf("set write deadline: %w", err)
	}
	_, err = a.nc.Write(data)
	if err != nil {
		return xerrors.Errorf("write message: %w", err)
	}

	// nhooyr.io/websocket has a bugged implementation of deadlines on a websocket net.Conn.  What they are
	// *supposed* to do is set a deadline for any subsequent writes to complete, otherwise the call to Write()
	// fails.  What nhooyr.io/websocket does is set a timer, after which it expires the websocket write context.
	// If this timer fires, then the next write will fail *even if we set a new write deadline*.  So, after
	// our successful write, it is important that we reset the deadline before it fires.
	err = a.nc.SetWriteDeadline(time.Time{})
	if err != nil {
		return xerrors.Errorf("clear write deadline: %w", err)
	}

	return nil
}

func (a *remoteMultiAgentHandler) OnNodeUpdate(_ uuid.UUID, node *agpl.Node) error {
	return a.writeJSON(CoordinateMessage{
		Type: CoordinateMessageTypeNodeUpdate,
		Node: node,
	})
}

func (a *remoteMultiAgentHandler) OnSubscribe(_ agpl.Queue, agentID uuid.UUID) (*agpl.Node, error) {
	return nil, a.writeJSON(CoordinateMessage{
		Type:    CoordinateMessageTypeSubscribe,
		AgentID: agentID,
	})
}

func (a *remoteMultiAgentHandler) OnUnsubscribe(_ agpl.Queue, agentID uuid.UUID) error {
	return a.writeJSON(CoordinateMessage{
		Type:    CoordinateMessageTypeUnsubscribe,
		AgentID: agentID,
	})
}

func (a *remoteMultiAgentHandler) AgentIsLegacy(agentID uuid.UUID) bool {
	a.legacyMu.RLock()
	if isLegacy, ok := a.legacyAgentCache[agentID]; ok {
		a.legacyMu.RUnlock()
		return isLegacy
	}
	a.legacyMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err, _ := a.legacySingleflight.Do(agentID, func() (AgentIsLegacyResponse, error) {
		return a.sdk.AgentIsLegacy(ctx, agentID)
	})
	if err != nil {
		a.sdk.SDKClient.Logger().Error(ctx, "failed to check agent legacy status", slog.Error(err))

		// Assume that the agent is legacy since this failed, while less
		// efficient it will always work.
		return true
	}
	// Assume legacy since the agent didn't exist.
	if !resp.Found {
		return true
	}

	a.legacyMu.Lock()
	a.legacyAgentCache[agentID] = resp.Legacy
	a.legacyMu.Unlock()

	return resp.Legacy
}

type AgentIsLegacyResponse struct {
	Found  bool `json:"found"`
	Legacy bool `json:"legacy"`
}

func (c *Client) AgentIsLegacy(ctx context.Context, agentID uuid.UUID) (AgentIsLegacyResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/workspaceagents/%s/legacy", agentID.String()),
		nil,
	)
	if err != nil {
		return AgentIsLegacyResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AgentIsLegacyResponse{}, codersdk.ReadBodyAsError(res)
	}

	var resp AgentIsLegacyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
