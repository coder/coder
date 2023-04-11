package wsproxysdk

import (
	"context"
	"net/http"
	"net/url"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// Client is a HTTP client for a subset of Coder API routes that external
// proxies need.
type Client struct {
	CoderSDKClient *codersdk.Client
	// HACK: the issue-signed-app-token requests may issue redirect responses
	// (which need to be forwarded to the client), so the client we use to make
	// those requests must ignore redirects.
	CoderSDKClientIgnoreRedirects *codersdk.Client
}

// New creates a external proxy client for the provided primary coder server
// URL.
func New(serverURL *url.URL) *Client {
	coderSDKClient := codersdk.New(serverURL)
	coderSDKClient.TokenHeader = httpmw.ExternalProxyAuthTokenHeader

	coderSDKClientIgnoreRedirects := codersdk.New(serverURL)
	coderSDKClientIgnoreRedirects.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	coderSDKClientIgnoreRedirects.TokenHeader = httpmw.ExternalProxyAuthTokenHeader

	return &Client{
		CoderSDKClient:                coderSDKClient,
		CoderSDKClientIgnoreRedirects: coderSDKClientIgnoreRedirects,
	}
}

// SetSessionToken sets the session token for the client. An error is returned
// if the session token is not in the correct format for external proxies.
func (c *Client) SetSessionToken(token string) error {
	c.CoderSDKClient.SetSessionToken(token)
	c.CoderSDKClientIgnoreRedirects.SetSessionToken(token)
	return nil
}

// SessionToken returns the currently set token for the client.
func (c *Client) SessionToken() string {
	return c.CoderSDKClient.SessionToken()
}

// Request wraps the underlying codersdk.Client's Request method.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	return c.CoderSDKClient.Request(ctx, method, path, body, opts...)
}

// RequestIgnoreRedirects wraps the underlying codersdk.Client's Request method
// on the client that ignores redirects.
func (c *Client) RequestIgnoreRedirects(ctx context.Context, method, path string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	return c.CoderSDKClientIgnoreRedirects.Request(ctx, method, path, body, opts...)
}
