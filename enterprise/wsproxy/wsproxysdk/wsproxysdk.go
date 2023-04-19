package wsproxysdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
