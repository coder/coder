package wsproxysdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
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
	// DerpEnabled indicates whether the proxy should be included in the DERP
	// map or not.
	DerpEnabled bool `json:"derp_enabled"`

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
	AppSecurityKey string `json:"app_security_key"`
	DERPMeshKey    string `json:"derp_mesh_key"`
	DERPRegionID   int32  `json:"derp_region_id"`
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
		return RegisterWorkspaceProxyResponse{}, xerrors.Errorf("make request: %w", err)
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
		return xerrors.Errorf("make request: %w", err)
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
	CallbackFn func(ctx context.Context, res RegisterWorkspaceProxyResponse) error
	// FailureFn is called with the last error returned from the server if the
	// context is canceled, registration fails for more than MaxFailureCount,
	// or if any permanent values in the response change.
	FailureFn func(err error)
}

// RegisterWorkspaceProxyLoop will register the workspace proxy and then start a
// goroutine to keep registering periodically in the background.
//
// The first response is returned immediately, and subsequent responses will be
// notified to the given CallbackFn. When the context is canceled the loop will
// stop immediately and the context error will be returned to the FailureFn.
//
// The returned channel will be closed when the loop stops and can be used to
// ensure the loop is dead before continuing. When a fatal error is encountered,
// the proxy will be deregistered (with the same ReplicaID and AttemptTimeout)
// before calling the FailureFn.
func (c *Client) RegisterWorkspaceProxyLoop(ctx context.Context, opts RegisterWorkspaceProxyLoopOpts) (RegisterWorkspaceProxyResponse, <-chan struct{}, error) {
	if opts.Interval == 0 {
		opts.Interval = 30 * time.Second
	}
	if opts.MaxFailureCount == 0 {
		opts.MaxFailureCount = 10
	}
	if opts.AttemptTimeout == 0 {
		opts.AttemptTimeout = 10 * time.Second
	}
	if opts.MutateFn == nil {
		opts.MutateFn = func(_ *RegisterWorkspaceProxyRequest) {}
	}
	if opts.CallbackFn == nil {
		opts.CallbackFn = func(_ context.Context, _ RegisterWorkspaceProxyResponse) error {
			return nil
		}
	}

	failureFn := func(err error) {
		// We have to use background context here because the original context
		// may be canceled.
		deregisterCtx, cancel := context.WithTimeout(context.Background(), opts.AttemptTimeout)
		defer cancel()
		deregisterErr := c.DeregisterWorkspaceProxy(deregisterCtx, DeregisterWorkspaceProxyRequest{
			ReplicaID: opts.Request.ReplicaID,
		})
		if deregisterErr != nil {
			opts.Logger.Error(ctx,
				"failed to deregister workspace proxy with Coder primary (it will be automatically deregistered shortly)",
				slog.F("err", deregisterErr),
			)
		}

		if opts.FailureFn != nil {
			opts.FailureFn(err)
		}
	}

	originalRes, err := c.RegisterWorkspaceProxy(ctx, opts.Request)
	if err != nil {
		return RegisterWorkspaceProxyResponse{}, nil, xerrors.Errorf("register workspace proxy: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		var (
			failedAttempts = 0
			ticker         = time.NewTicker(opts.Interval)
		)
		for {
			select {
			case <-ctx.Done():
				failureFn(ctx.Err())
				return
			case <-ticker.C:
			}

			opts.Logger.Debug(ctx,
				"re-registering workspace proxy with Coder primary",
				slog.F("req", opts.Request),
				slog.F("timeout", opts.AttemptTimeout),
				slog.F("failed_attempts", failedAttempts),
			)
			opts.MutateFn(&opts.Request)
			registerCtx, cancel := context.WithTimeout(ctx, opts.AttemptTimeout)
			res, err := c.RegisterWorkspaceProxy(registerCtx, opts.Request)
			cancel()
			if err != nil {
				failedAttempts++
				opts.Logger.Warn(ctx,
					"failed to re-register workspace proxy with Coder primary",
					slog.F("req", opts.Request),
					slog.F("timeout", opts.AttemptTimeout),
					slog.F("failed_attempts", failedAttempts),
					slog.F("err", err),
				)

				if failedAttempts > opts.MaxFailureCount {
					failureFn(xerrors.Errorf("exceeded re-registration failure count of %d: last error: %w", opts.MaxFailureCount, err))
					return
				}
				continue
			}
			failedAttempts = 0

			if res.AppSecurityKey != originalRes.AppSecurityKey {
				failureFn(xerrors.New("app security key has changed, proxy must be restarted"))
				return
			}
			if res.DERPMeshKey != originalRes.DERPMeshKey {
				failureFn(xerrors.New("DERP mesh key has changed, proxy must be restarted"))
				return
			}
			if res.DERPRegionID != originalRes.DERPRegionID {
				failureFn(xerrors.New("DERP region ID has changed, proxy must be restarted"))
			}

			err = opts.CallbackFn(ctx, res)
			if err != nil {
				failureFn(xerrors.Errorf("callback fn returned error: %w", err))
				return
			}

			ticker.Reset(opts.Interval)
		}
	}()

	return originalRes, done, nil
}
