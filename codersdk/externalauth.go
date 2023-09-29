package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type ExternalAuth struct {
	Authenticated bool   `json:"authenticated"`
	Device        bool   `json:"device"`
	Type          string `json:"type"`

	// User is the user that authenticated with the provider.
	User *ExternalAuthUser `json:"user"`
	// AppInstallable is true if the request for app installs was successful.
	AppInstallable bool `json:"app_installable"`
	// AppInstallations are the installations that the user has access to.
	AppInstallations []ExternalAuthAppInstallation `json:"installations"`
	// AppInstallURL is the URL to install the app.
	AppInstallURL string `json:"app_install_url"`
}

type ExternalAuthAppInstallation struct {
	ID           int              `json:"id"`
	Account      ExternalAuthUser `json:"account"`
	ConfigureURL string           `json:"configure_url"`
}

type ExternalAuthUser struct {
	Login      string `json:"login"`
	AvatarURL  string `json:"avatar_url"`
	ProfileURL string `json:"profile_url"`
	Name       string `json:"name"`
}

// ExternalAuthDevice is the response from the device authorization endpoint.
// See: https://tools.ietf.org/html/rfc8628#section-3.2
type ExternalAuthDevice struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type ExternalAuthDeviceExchange struct {
	DeviceCode string `json:"device_code"`
}

func (c *Client) ExternalAuthDeviceByID(ctx context.Context, provider string) (ExternalAuthDevice, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/externalauth/%s/device", provider), nil)
	if err != nil {
		return ExternalAuthDevice{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ExternalAuthDevice{}, ReadBodyAsError(res)
	}
	var extAuth ExternalAuthDevice
	return extAuth, json.NewDecoder(res.Body).Decode(&extAuth)
}

// ExchangeGitAuth exchanges a device code for an external auth token.
func (c *Client) ExternalAuthDeviceExchange(ctx context.Context, provider string, req ExternalAuthDeviceExchange) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/externalauth/%s/device", provider), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// ExternalAuthByID returns the external auth for the given provider by ID.
func (c *Client) ExternalAuthByID(ctx context.Context, provider string) (ExternalAuth, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/externalauth/%s", provider), nil)
	if err != nil {
		return ExternalAuth{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ExternalAuth{}, ReadBodyAsError(res)
	}
	var extAuth ExternalAuth
	return extAuth, json.NewDecoder(res.Body).Decode(&extAuth)
}
