package wsproxysdk
import (
	"fmt"
	"errors"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
	"github.com/google/uuid"
	"tailscale.com/tailcfg"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/websocket"
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
func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *workspacesdk.DialAgentOptions) (agentConn *workspacesdk.AgentConn, err error) {
	return workspacesdk.New(c.SDKClient).DialAgent(ctx, agentID, options)
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
		return IssueSignedAppTokenResponse{}, fmt.Errorf("make request: %w", err)
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
		writeError(rw, fmt.Errorf("perform issue signed app token request: %w", err))
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
			writeError(rw, fmt.Errorf("copy response body: %w", err))
		}
		return IssueSignedAppTokenResponse{}, false
	}
	var res IssueSignedAppTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		writeError(rw, fmt.Errorf("decode response body: %w", err))
		return IssueSignedAppTokenResponse{}, false
	}
	return res, true
}
type ReportAppStatsRequest struct {
	Stats []workspaceapps.StatsReport `json:"stats"`
}
// ReportAppStats reports the given app stats to the primary coder server.
func (c *Client) ReportAppStats(ctx context.Context, req ReportAppStatsRequest) error {
	resp, err := c.Request(ctx, http.MethodPost, "/api/v2/workspaceproxies/me/app-stats", req)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(resp)
	}
	return nil
}
type RegisterWorkspaceProxyRequest struct {
	// AccessURL that hits the workspace proxy api.
	AccessURL string `json:"access_url"`
	// WildcardHostname that the workspace proxy api is serving for subdomain apps.
	WildcardHostname string `json:"wildcard_hostname"`
	// DerpEnabled indicates whether the proxy should be included in the DERP
	// map or not.
	DerpEnabled bool `json:"derp_enabled"`
	// DerpOnly indicates whether the proxy should only be included in the DERP
	// map and should not be used for serving apps.
	DerpOnly bool `json:"derp_only"`
	// ReplicaID is a unique identifier for the replica of the proxy that is
	// registering. It should be generated by the client on startup and
	// persisted (in memory only) until the process is restarted.
	ReplicaID uuid.UUID `json:"replica_id"`
	// ReplicaHostname is the OS hostname of the machine that the proxy is running
	// on.  This is only used for tracking purposes in the replicas table.
	ReplicaHostname string `json:"hostname"`
	// ReplicaError is the error that the replica encountered when trying to
	// dial it's peers. This is stored in the replicas table for debugging
	// purposes but does not affect the proxy's ability to register.
	//
	// This value is only stored on subsequent requests to the register
	// endpoint, not the first request.
	ReplicaError string `json:"replica_error"`
	// ReplicaRelayAddress is the DERP address of the replica that other
	// replicas may use to connect internally for DERP meshing.
	ReplicaRelayAddress string `json:"replica_relay_address"`
	// Version is the Coder version of the proxy.
	Version string `json:"version"`
}
type RegisterWorkspaceProxyResponse struct {
	DERPMeshKey         string           `json:"derp_mesh_key"`
	DERPRegionID        int32            `json:"derp_region_id"`
	DERPMap             *tailcfg.DERPMap `json:"derp_map"`
	DERPForceWebSockets bool             `json:"derp_force_websockets"`
	// SiblingReplicas is a list of all other replicas of the proxy that have
	// not timed out.
	SiblingReplicas []codersdk.Replica `json:"sibling_replicas"`
}
func (c *Client) RegisterWorkspaceProxy(ctx context.Context, req RegisterWorkspaceProxyRequest) (RegisterWorkspaceProxyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies/me/register",
		req,
	)
	if err != nil {
		return RegisterWorkspaceProxyResponse{}, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return RegisterWorkspaceProxyResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp RegisterWorkspaceProxyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
type DeregisterWorkspaceProxyRequest struct {
	// ReplicaID is a unique identifier for the replica of the proxy that is
	// deregistering. It should be generated by the client on startup and
	// should've already been passed to the register endpoint.
	ReplicaID uuid.UUID `json:"replica_id"`
}
func (c *Client) DeregisterWorkspaceProxy(ctx context.Context, req DeregisterWorkspaceProxyRequest) error {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies/me/deregister",
		req,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}
type RegisterWorkspaceProxyLoopOpts struct {
	Logger  slog.Logger
	Request RegisterWorkspaceProxyRequest
	// Interval between registration attempts. Defaults to 30 seconds. Note that
	// the initial registration is not delayed by this interval.
	Interval time.Duration
	// MaxFailureCount is the maximum amount of attempts that the loop will
	// retry registration before giving up. Defaults to 10 (for ~5 minutes).
	MaxFailureCount int
	// AttemptTimeout is the maximum amount of time that the loop will wait for
	// a response from the server before considering the attempt a failure.
	// Defaults to 10 seconds.
	AttemptTimeout time.Duration
	// MutateFn is called before each request to mutate the request struct. This
	// can be used to update fields like ReplicaError.
	MutateFn func(req *RegisterWorkspaceProxyRequest)
	// CallbackFn is called with the response from the server after each
	// successful registration, except the first. The callback function is
	// called in a blocking manner, so it should avoid blocking for too long. If
	// the callback returns an error, the loop will stop immediately and the
	// error will be returned to the FailureFn.
	CallbackFn func(res RegisterWorkspaceProxyResponse) error
	// FailureFn is called with the last error returned from the server if the
	// context is canceled, registration fails for more than MaxFailureCount,
	// or if any permanent values in the response change.
	FailureFn func(err error)
}
type RegisterWorkspaceProxyLoop struct {
	opts RegisterWorkspaceProxyLoopOpts
	c    *Client
	// runLoopNow takes a response channel to send the response to and triggers
	// the loop to run immediately if it's waiting.
	runLoopNow chan chan RegisterWorkspaceProxyResponse
	closedCtx  context.Context
	close      context.CancelFunc
	done       chan struct{}
}
func (l *RegisterWorkspaceProxyLoop) register(ctx context.Context) (RegisterWorkspaceProxyResponse, error) {
	registerCtx, registerCancel := context.WithTimeout(ctx, l.opts.AttemptTimeout)
	res, err := l.c.RegisterWorkspaceProxy(registerCtx, l.opts.Request)
	registerCancel()
	if err != nil {
		return RegisterWorkspaceProxyResponse{}, fmt.Errorf("register workspace proxy: %w", err)
	}
	return res, nil
}
// Start starts the proxy registration loop. The provided context is only used
// for the initial registration. Use Close() to stop.
func (l *RegisterWorkspaceProxyLoop) Start(ctx context.Context) (RegisterWorkspaceProxyResponse, error) {
	if l.opts.Interval == 0 {
		l.opts.Interval = 15 * time.Second
	}
	if l.opts.MaxFailureCount == 0 {
		l.opts.MaxFailureCount = 10
	}
	if l.opts.AttemptTimeout == 0 {
		l.opts.AttemptTimeout = 10 * time.Second
	}
	var err error
	originalRes, err := l.register(ctx)
	if err != nil {
		return RegisterWorkspaceProxyResponse{}, fmt.Errorf("initial registration: %w", err)
	}
	go func() {
		defer close(l.done)
		var (
			failedAttempts = 0
			ticker         = time.NewTicker(l.opts.Interval)
		)
		for {
			var respCh chan RegisterWorkspaceProxyResponse
			select {
			case <-l.closedCtx.Done():
				l.failureFn(fmt.Errorf("proxy registration loop closed"))
				return
			case respCh = <-l.runLoopNow:
			case <-ticker.C:
			}
			l.opts.Logger.Debug(context.Background(),
				"re-registering workspace proxy with Coder primary",
				slog.F("req", l.opts.Request),
				slog.F("timeout", l.opts.AttemptTimeout),
				slog.F("failed_attempts", failedAttempts),
			)
			l.mutateFn(&l.opts.Request)
			resp, err := l.register(l.closedCtx)
			if err != nil {
				failedAttempts++
				l.opts.Logger.Warn(context.Background(),
					"failed to re-register workspace proxy with Coder primary",
					slog.F("req", l.opts.Request),
					slog.F("timeout", l.opts.AttemptTimeout),
					slog.F("failed_attempts", failedAttempts),
					slog.Error(err),
				)
				if failedAttempts > l.opts.MaxFailureCount {
					l.failureFn(fmt.Errorf("exceeded re-registration failure count of %d: last error: %w", l.opts.MaxFailureCount, err))
					return
				}
				continue
			}
			failedAttempts = 0
			if originalRes.DERPMeshKey != resp.DERPMeshKey {
				l.failureFn(errors.New("DERP mesh key has changed, proxy must be restarted"))
				return
			}
			if originalRes.DERPRegionID != resp.DERPRegionID {
				l.failureFn(errors.New("DERP region ID has changed, proxy must be restarted"))
				return
			}
			err = l.callbackFn(resp)
			if err != nil {
				l.failureFn(fmt.Errorf("callback function returned an error: %w", err))
				return
			}
			// If we were triggered by RegisterNow(), send the response back.
			if respCh != nil {
				respCh <- resp
				close(respCh)
			}
			ticker.Reset(l.opts.Interval)
		}
	}()
	return originalRes, nil
}
// RegisterNow asks the registration loop to register immediately. A timeout of
// 2x the attempt timeout is used to wait for the response.
func (l *RegisterWorkspaceProxyLoop) RegisterNow() (RegisterWorkspaceProxyResponse, error) {
	// The channel is closed by the loop after sending the response.
	respCh := make(chan RegisterWorkspaceProxyResponse, 1)
	select {
	case <-l.done:
		return RegisterWorkspaceProxyResponse{}, errors.New("proxy registration loop closed")
	case l.runLoopNow <- respCh:
	}
	select {
	case <-l.done:
		return RegisterWorkspaceProxyResponse{}, errors.New("proxy registration loop closed")
	case resp := <-respCh:
		return resp, nil
	}
}
func (l *RegisterWorkspaceProxyLoop) Close() {
	l.close()
	<-l.done
}
func (l *RegisterWorkspaceProxyLoop) mutateFn(req *RegisterWorkspaceProxyRequest) {
	if l.opts.MutateFn != nil {
		l.opts.MutateFn(req)
	}
}
func (l *RegisterWorkspaceProxyLoop) callbackFn(res RegisterWorkspaceProxyResponse) error {
	if l.opts.CallbackFn != nil {
		return l.opts.CallbackFn(res)
	}
	return nil
}
func (l *RegisterWorkspaceProxyLoop) failureFn(err error) {
	// We have to use background context here because the original context may
	// be canceled.
	deregisterCtx, cancel := context.WithTimeout(context.Background(), l.opts.AttemptTimeout)
	defer cancel()
	deregisterErr := l.c.DeregisterWorkspaceProxy(deregisterCtx, DeregisterWorkspaceProxyRequest{
		ReplicaID: l.opts.Request.ReplicaID,
	})
	if deregisterErr != nil {
		l.opts.Logger.Error(context.Background(),
			"failed to deregister workspace proxy with Coder primary (it will be automatically deregistered shortly)",
			slog.Error(deregisterErr),
		)
	}
	if l.opts.FailureFn != nil {
		l.opts.FailureFn(err)
	}
}
// RegisterWorkspaceProxyLoop will register the workspace proxy and then start a
// goroutine to keep registering periodically in the background.
//
// The first response is returned immediately, and subsequent responses will be
// notified to the given CallbackFn. When the loop is Close()d it will stop
// immediately and an error will be returned to the FailureFn.
//
// When a fatal error is encountered (or the proxy is closed), the proxy will be
// deregistered (with the same ReplicaID and AttemptTimeout) before calling the
// FailureFn.
func (c *Client) RegisterWorkspaceProxyLoop(ctx context.Context, opts RegisterWorkspaceProxyLoopOpts) (*RegisterWorkspaceProxyLoop, RegisterWorkspaceProxyResponse, error) {
	closedCtx, closeFn := context.WithCancel(context.Background())
	loop := &RegisterWorkspaceProxyLoop{
		opts:       opts,
		c:          c,
		runLoopNow: make(chan chan RegisterWorkspaceProxyResponse),
		closedCtx:  closedCtx,
		close:      closeFn,
		done:       make(chan struct{}),
	}
	regResp, err := loop.Start(ctx)
	if err != nil {
		return nil, RegisterWorkspaceProxyResponse{}, fmt.Errorf("start loop: %w", err)
	}
	return loop, regResp, nil
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
func (c *Client) TailnetDialer() (*workspacesdk.WebsocketDialer, error) {
	logger := c.SDKClient.Logger().Named("tailnet_dialer")
	coordinateURL, err := c.SDKClient.URL.Parse("/api/v2/workspaceproxies/me/coordinate")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	coordinateHeaders := make(http.Header)
	tokenHeader := codersdk.SessionTokenHeader
	if c.SDKClient.SessionTokenHeader != "" {
		tokenHeader = c.SDKClient.SessionTokenHeader
	}
	coordinateHeaders.Set(tokenHeader, c.SessionToken())
	return workspacesdk.NewWebsocketDialer(logger, coordinateURL, &websocket.DialOptions{
		HTTPClient: c.SDKClient.HTTPClient,
		HTTPHeader: coordinateHeaders,
	}), nil
}
type CryptoKeysResponse struct {
	CryptoKeys []codersdk.CryptoKey `json:"crypto_keys"`
}
func (c *Client) CryptoKeys(ctx context.Context, feature codersdk.CryptoKeyFeature) (CryptoKeysResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/workspaceproxies/me/crypto-keys", nil,
		codersdk.WithQueryParam("feature", string(feature)),
	)
	if err != nil {
		return CryptoKeysResponse{}, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return CryptoKeysResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp CryptoKeysResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
