package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type OAuth2App struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	CallbackURL string    `json:"callback_url"`
	Icon        string    `json:"icon"`
}

// OAuth2Apps returns the applications configured to authenticate using Coder as
// an OAuth2 provider.
func (c *Client) OAuth2Apps(ctx context.Context) ([]OAuth2App, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/oauth2/apps", nil)
	if err != nil {
		return []OAuth2App{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []OAuth2App{}, ReadBodyAsError(res)
	}
	var apps []OAuth2App
	return apps, json.NewDecoder(res.Body).Decode(&apps)
}

// OAuth2App returns an application configured to authenticate using Coder as an
// OAuth2 provider.
func (c *Client) OAuth2App(ctx context.Context, id uuid.UUID) (OAuth2App, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/oauth2/apps/%s", id), nil)
	if err != nil {
		return OAuth2App{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2App{}, ReadBodyAsError(res)
	}
	var apps OAuth2App
	return apps, json.NewDecoder(res.Body).Decode(&apps)
}

type PostOAuth2AppRequest struct {
	Name        string `json:"name" validate:"required,oauth2_app_name"`
	CallbackURL string `json:"callback_url" validate:"required,http_url"`
	Icon        string `json:"icon" validate:"omitempty"`
}

// PostOAuth2App adds an application that can authenticate using Coder as an
// OAuth2 provider.
func (c *Client) PostOAuth2App(ctx context.Context, app PostOAuth2AppRequest) (OAuth2App, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/oauth2/apps", app)
	if err != nil {
		return OAuth2App{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return OAuth2App{}, ReadBodyAsError(res)
	}
	var resp OAuth2App
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type PutOAuth2AppRequest struct {
	Name        string `json:"name" validate:"required,oauth2_app_name"`
	CallbackURL string `json:"callback_url" validate:"required,http_url"`
	Icon        string `json:"icon" validate:"omitempty"`
}

// PutOAuth2App updates an application that can authenticate using Coder as an
// OAuth2 provider.
func (c *Client) PutOAuth2App(ctx context.Context, id uuid.UUID, app PutOAuth2AppRequest) (OAuth2App, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/oauth2/apps/%s", id), app)
	if err != nil {
		return OAuth2App{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2App{}, ReadBodyAsError(res)
	}
	var resp OAuth2App
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2App deletes an application, also invalidating any tokens that
// were generated from it.
func (c *Client) DeleteOAuth2App(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/oauth2/apps/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type OAuth2AppSecretFull struct {
	ID               uuid.UUID `json:"id" format:"uuid"`
	ClientSecretFull string    `json:"client_secret_full"`
}

type OAuth2AppSecret struct {
	ID                    uuid.UUID `json:"id" format:"uuid"`
	LastUsedAt            NullTime  `json:"last_used_at"`
	ClientSecretTruncated string    `json:"client_secret_truncated"`
}

// OAuth2AppSecrets returns the truncated secrets for an OAuth2 application.
func (c *Client) OAuth2AppSecrets(ctx context.Context, appID uuid.UUID) ([]OAuth2AppSecret, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/oauth2/apps/%s/secrets", appID), nil)
	if err != nil {
		return []OAuth2AppSecret{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []OAuth2AppSecret{}, ReadBodyAsError(res)
	}
	var resp []OAuth2AppSecret
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PostOAuth2AppSecret creates a new secret for an OAuth2 application.  This is
// the only time the full secret will be revealed.
func (c *Client) PostOAuth2AppSecret(ctx context.Context, appID uuid.UUID) (OAuth2AppSecretFull, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/oauth2/apps/%s/secrets", appID), nil)
	if err != nil {
		return OAuth2AppSecretFull{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2AppSecretFull{}, ReadBodyAsError(res)
	}
	var resp OAuth2AppSecretFull
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2AppSecret deletes a secret from an OAuth2 application, also
// invalidating any tokens that generated from it.
func (c *Client) DeleteOAuth2AppSecret(ctx context.Context, appID uuid.UUID, secretID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/oauth2/apps/%s/secrets/%s", appID, secretID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
