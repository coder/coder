package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EnhancedExternalAuthProvider is a constant that represents enhanced
// support for a type of external authentication. All of the Git providers
// are examples of enhanced, because they support intercepting "git clone".
type EnhancedExternalAuthProvider string

func (e EnhancedExternalAuthProvider) String() string {
	return string(e)
}

// Git returns whether the provider is a Git provider.
func (e EnhancedExternalAuthProvider) Git() bool {
	switch e {
	case EnhancedExternalAuthProviderGitHub,
		EnhancedExternalAuthProviderGitLab,
		EnhancedExternalAuthProviderBitBucketCloud,
		EnhancedExternalAuthProviderBitBucketServer,
		EnhancedExternalAuthProviderAzureDevops,
		EnhancedExternalAuthProviderAzureDevopsEntra,
		EnhancedExternalAuthProviderGitea:
		return true
	default:
		return false
	}
}

const (
	EnhancedExternalAuthProviderAzureDevops EnhancedExternalAuthProvider = "azure-devops"
	// Authenticate to ADO using an app registration in Entra ID
	EnhancedExternalAuthProviderAzureDevopsEntra EnhancedExternalAuthProvider = "azure-devops-entra"
	EnhancedExternalAuthProviderGitHub           EnhancedExternalAuthProvider = "github"
	EnhancedExternalAuthProviderGitLab           EnhancedExternalAuthProvider = "gitlab"
	// EnhancedExternalAuthProviderBitBucketCloud is the Bitbucket Cloud provider.
	// Not to be confused with the self-hosted 'EnhancedExternalAuthProviderBitBucketServer'
	EnhancedExternalAuthProviderBitBucketCloud  EnhancedExternalAuthProvider = "bitbucket-cloud"
	EnhancedExternalAuthProviderBitBucketServer EnhancedExternalAuthProvider = "bitbucket-server"
	EnhancedExternalAuthProviderSlack           EnhancedExternalAuthProvider = "slack"
	EnhancedExternalAuthProviderJFrog           EnhancedExternalAuthProvider = "jfrog"
	EnhancedExternalAuthProviderGitea           EnhancedExternalAuthProvider = "gitea"
)

type ExternalAuth struct {
	Authenticated bool   `json:"authenticated"`
	Device        bool   `json:"device"`
	DisplayName   string `json:"display_name"`

	// User is the user that authenticated with the provider.
	User *ExternalAuthUser `json:"user"`
	// AppInstallable is true if the request for app installs was successful.
	AppInstallable bool `json:"app_installable"`
	// AppInstallations are the installations that the user has access to.
	AppInstallations []ExternalAuthAppInstallation `json:"installations"`
	// AppInstallURL is the URL to install the app.
	AppInstallURL string `json:"app_install_url"`
}

type ListUserExternalAuthResponse struct {
	Providers []ExternalAuthLinkProvider `json:"providers"`
	// Links are all the authenticated links for the user.
	// If a link has a provider ID that does not exist, then that provider
	// is no longer configured, rendering it unusable. It is still valuable
	// to include these links so that the user can unlink them.
	Links []ExternalAuthLink `json:"links"`
}

// ExternalAuthLink is a link between a user and an external auth provider.
// It excludes information that requires a token to access, so can be statically
// built from the database and configs.
type ExternalAuthLink struct {
	ProviderID      string    `json:"provider_id"`
	CreatedAt       time.Time `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time `json:"updated_at" format:"date-time"`
	HasRefreshToken bool      `json:"has_refresh_token"`
	Expires         time.Time `json:"expires" format:"date-time"`
	Authenticated   bool      `json:"authenticated"`
	ValidateError   string    `json:"validate_error"`
}

// ExternalAuthLinkProvider are the static details of a provider.
type ExternalAuthLinkProvider struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Device        bool   `json:"device"`
	DisplayName   string `json:"display_name"`
	DisplayIcon   string `json:"display_icon"`
	AllowRefresh  bool   `json:"allow_refresh"`
	AllowValidate bool   `json:"allow_validate"`
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/external-auth/%s/device", provider), nil)
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
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/external-auth/%s/device", provider), req)
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/external-auth/%s", provider), nil)
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

// UnlinkExternalAuthByID deletes the external auth for the given provider by ID
// for the user. This does not revoke the token from the IDP.
func (c *Client) UnlinkExternalAuthByID(ctx context.Context, provider string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/external-auth/%s", provider), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// ListExternalAuths returns the available external auth providers and the user's
// authenticated links if they exist.
func (c *Client) ListExternalAuths(ctx context.Context) (ListUserExternalAuthResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/external-auth", nil)
	if err != nil {
		return ListUserExternalAuthResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ListUserExternalAuthResponse{}, ReadBodyAsError(res)
	}
	var extAuth ListUserExternalAuthResponse
	return extAuth, json.NewDecoder(res.Body).Decode(&extAuth)
}
