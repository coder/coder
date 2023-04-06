package wsproxysdk

import (
	"context"
	"net/http"
	"net/url"

	"github.com/coder/coder/codersdk"
)

const (
	// AuthTokenHeader is the auth header used for requests from
	// external workspace proxies.
	//
	// The format of an external proxy token is:
	//     <proxy id>:<proxy secret>
	//
	//nolint:gosec
	AuthTokenHeader = "Coder-External-Proxy-Token"
)

// Client is a HTTP client for a subset of Coder API routes that external
// proxies need.
type Client struct {
	CoderSDKClient *codersdk.Client
}

// New creates a external proxy client for the provided primary coder server
// URL.
func New(serverURL *url.URL) *Client {
	coderSDKClient := codersdk.New(serverURL)
	coderSDKClient.TokenHeader = AuthTokenHeader

	return &Client{
		CoderSDKClient: coderSDKClient,
	}
}

// SetSessionToken sets the session token for the client. An error is returned
// if the session token is not in the correct format for external proxies.
func (c *Client) SetSessionToken(token string) error {
	c.CoderSDKClient.SetSessionToken(token)
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
