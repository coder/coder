package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type OAuth2ProviderApp struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	CallbackURL string    `json:"callback_url"`
	Icon        string    `json:"icon"`

	// Endpoints are included in the app response for easier discovery. The OAuth2
	// spec does not have a defined place to find these (for comparison, OIDC has
	// a '/.well-known/openid-configuration' endpoint).
	Endpoints OAuth2AppEndpoints `json:"endpoints"`
}

type OAuth2AppEndpoints struct {
	Authorization string `json:"authorization"`
	Token         string `json:"token"`
	// DeviceAuth is optional.
	DeviceAuth string `json:"device_authorization"`
}

type OAuth2ProviderAppFilter struct {
	UserID uuid.UUID `json:"user_id,omitempty" format:"uuid"`
}

// OAuth2ProviderApps returns the applications configured to authenticate using
// Coder as an OAuth2 provider.
func (c *Client) OAuth2ProviderApps(ctx context.Context, filter OAuth2ProviderAppFilter) ([]OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/oauth2-provider/apps", nil,
		func(r *http.Request) {
			if filter.UserID != uuid.Nil {
				q := r.URL.Query()
				q.Set("user_id", filter.UserID.String())
				r.URL.RawQuery = q.Encode()
			}
		})
	if err != nil {
		return []OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var apps []OAuth2ProviderApp
	return apps, json.NewDecoder(res.Body).Decode(&apps)
}

// OAuth2ProviderApp returns an application configured to authenticate using
// Coder as an OAuth2 provider.
func (c *Client) OAuth2ProviderApp(ctx context.Context, id uuid.UUID) (OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s", id), nil)
	if err != nil {
		return OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var apps OAuth2ProviderApp
	return apps, json.NewDecoder(res.Body).Decode(&apps)
}

type PostOAuth2ProviderAppRequest struct {
	Name        string `json:"name" validate:"required,oauth2_app_name"`
	CallbackURL string `json:"callback_url" validate:"required,http_url"`
	Icon        string `json:"icon" validate:"omitempty"`
}

// PostOAuth2ProviderApp adds an application that can authenticate using Coder
// as an OAuth2 provider.
func (c *Client) PostOAuth2ProviderApp(ctx context.Context, app PostOAuth2ProviderAppRequest) (OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/oauth2-provider/apps", app)
	if err != nil {
		return OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var resp OAuth2ProviderApp
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type PutOAuth2ProviderAppRequest struct {
	Name        string `json:"name" validate:"required,oauth2_app_name"`
	CallbackURL string `json:"callback_url" validate:"required,http_url"`
	Icon        string `json:"icon" validate:"omitempty"`
}

// PutOAuth2ProviderApp updates an application that can authenticate using Coder
// as an OAuth2 provider.
func (c *Client) PutOAuth2ProviderApp(ctx context.Context, id uuid.UUID, app PutOAuth2ProviderAppRequest) (OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s", id), app)
	if err != nil {
		return OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var resp OAuth2ProviderApp
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2ProviderApp deletes an application, also invalidating any tokens
// that were generated from it.
func (c *Client) DeleteOAuth2ProviderApp(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type OAuth2ProviderAppSecretFull struct {
	ID               uuid.UUID `json:"id" format:"uuid"`
	ClientSecretFull string    `json:"client_secret_full"`
}

type OAuth2ProviderAppSecret struct {
	ID                    uuid.UUID `json:"id" format:"uuid"`
	LastUsedAt            NullTime  `json:"last_used_at"`
	ClientSecretTruncated string    `json:"client_secret_truncated"`
}

// OAuth2ProviderAppSecrets returns the truncated secrets for an OAuth2
// application.
func (c *Client) OAuth2ProviderAppSecrets(ctx context.Context, appID uuid.UUID) ([]OAuth2ProviderAppSecret, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s/secrets", appID), nil)
	if err != nil {
		return []OAuth2ProviderAppSecret{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []OAuth2ProviderAppSecret{}, ReadBodyAsError(res)
	}
	var resp []OAuth2ProviderAppSecret
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PostOAuth2ProviderAppSecret creates a new secret for an OAuth2 application.
// This is the only time the full secret will be revealed.
func (c *Client) PostOAuth2ProviderAppSecret(ctx context.Context, appID uuid.UUID) (OAuth2ProviderAppSecretFull, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s/secrets", appID), nil)
	if err != nil {
		return OAuth2ProviderAppSecretFull{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ProviderAppSecretFull{}, ReadBodyAsError(res)
	}
	var resp OAuth2ProviderAppSecretFull
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2ProviderAppSecret deletes a secret from an OAuth2 application,
// also invalidating any tokens that generated from it.
func (c *Client) DeleteOAuth2ProviderAppSecret(ctx context.Context, appID uuid.UUID, secretID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s/secrets/%s", appID, secretID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type OAuth2ProviderGrantType string

const (
	OAuth2ProviderGrantTypeAuthorizationCode OAuth2ProviderGrantType = "authorization_code"
	OAuth2ProviderGrantTypeRefreshToken      OAuth2ProviderGrantType = "refresh_token"
)

func (e OAuth2ProviderGrantType) Valid() bool {
	switch e {
	case OAuth2ProviderGrantTypeAuthorizationCode, OAuth2ProviderGrantTypeRefreshToken:
		return true
	}
	return false
}

type OAuth2ProviderResponseType string

const (
	OAuth2ProviderResponseTypeCode OAuth2ProviderResponseType = "code"
)

func (e OAuth2ProviderResponseType) Valid() bool {
	//nolint:gocritic,revive // More cases might be added later.
	switch e {
	case OAuth2ProviderResponseTypeCode:
		return true
	}
	return false
}

// RevokeOAuth2ProviderApp completely revokes an app's access for the
// authenticated user.
func (c *Client) RevokeOAuth2ProviderApp(ctx context.Context, appID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, "/login/oauth2/tokens", nil, func(r *http.Request) {
		q := r.URL.Query()
		q.Set("client_id", appID.String())
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
