package wsproxysdk

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpmw"
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
