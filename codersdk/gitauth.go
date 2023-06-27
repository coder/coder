package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type GitAuth struct {
	Authenticated bool `json:"authenticated"`
	Device        bool `json:"device"`

	// User is the user that authenticated with the provider.
	User *GitAuthUser `json:"user"`
	// AppInstallations are the installations that the user has access to.
	AppInstallations []GitAuthAppInstallation `json:"installations"`
}

type GitAuthAppInstallation struct {
	ID           int          `json:"id"`
	Account      *GitAuthUser `json:"account"`
	ConfigureURL string       `json:"configure_url"`
}

type GitAuthUser struct {
	Login      string
	AvatarURL  string
	ProfileURL string
	Name       string
}

// GitAuthDevice is the response from the device authorization endpoint.
// See: https://tools.ietf.org/html/rfc8628#section-3.2
type GitAuthDevice struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type GitAuthDeviceExchange struct {
	DeviceCode string `json:"device_code"`
}

func (c *Client) GitAuthDeviceByID(ctx context.Context, provider string) (GitAuthDevice, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/gitauth/%s/device", provider), nil)
	if err != nil {
		return GitAuthDevice{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return GitAuthDevice{}, ReadBodyAsError(res)
	}
	var gitauth GitAuthDevice
	return gitauth, json.NewDecoder(res.Body).Decode(&gitauth)
}

// ExchangeGitAuth exchanges a device code for a git auth token.
func (c *Client) GitAuthDeviceExchange(ctx context.Context, provider string, req GitAuthDeviceExchange) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/gitauth/%s/device", provider), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// GitAuthByID returns the git auth for the given provider by ID.
func (c *Client) GitAuthByID(ctx context.Context, provider string) (GitAuth, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/gitauth/%s", provider), nil)
	if err != nil {
		return GitAuth{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return GitAuth{}, ReadBodyAsError(res)
	}
	var gitauth GitAuth
	return gitauth, json.NewDecoder(res.Body).Decode(&gitauth)
}
