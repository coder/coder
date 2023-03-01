package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// APIKey: do not ever return the HashedSecret
type APIKey struct {
	ID              string      `json:"id" validate:"required"`
	UserID          uuid.UUID   `json:"user_id" validate:"required" format:"uuid"`
	LastUsed        time.Time   `json:"last_used" validate:"required" format:"date-time"`
	ExpiresAt       time.Time   `json:"expires_at" validate:"required" format:"date-time"`
	CreatedAt       time.Time   `json:"created_at" validate:"required" format:"date-time"`
	UpdatedAt       time.Time   `json:"updated_at" validate:"required" format:"date-time"`
	LoginType       LoginType   `json:"login_type" validate:"required" enums:"password,github,oidc,token"`
	Scope           APIKeyScope `json:"scope" validate:"required" enums:"all,application_connect"`
	LifetimeSeconds int64       `json:"lifetime_seconds" validate:"required"`
}

// LoginType is the type of login used to create the API key.
type LoginType string

const (
	LoginTypePassword LoginType = "password"
	LoginTypeGithub   LoginType = "github"
	LoginTypeOIDC     LoginType = "oidc"
	LoginTypeToken    LoginType = "token"
)

type APIKeyScope string

const (
	// APIKeyScopeAll is a scope that allows the user to do everything.
	APIKeyScopeAll APIKeyScope = "all"
	// APIKeyScopeApplicationConnect is a scope that allows the user
	// to connect to applications in a workspace.
	APIKeyScopeApplicationConnect APIKeyScope = "application_connect"
)

type CreateTokenRequest struct {
	Lifetime time.Duration `json:"lifetime"`
	Scope    APIKeyScope   `json:"scope" enums:"all,application_connect"`
}

// GenerateAPIKeyResponse contains an API key for a user.
type GenerateAPIKeyResponse struct {
	Key string `json:"key"`
}

// CreateToken generates an API key for the user ID provided with
// custom expiration. These tokens can be used for long-lived access,
// like for use with CI.
func (c *Client) CreateToken(ctx context.Context, userID string, req CreateTokenRequest) (GenerateAPIKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys/tokens", userID), req)
	if err != nil {
		return GenerateAPIKeyResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return GenerateAPIKeyResponse{}, ReadBodyAsError(res)
	}

	var apiKey GenerateAPIKeyResponse
	return apiKey, json.NewDecoder(res.Body).Decode(&apiKey)
}

// CreateAPIKey generates an API key for the user ID provided.
// DEPRECATED: use CreateToken instead.
func (c *Client) CreateAPIKey(ctx context.Context, user string) (GenerateAPIKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys", user), nil)
	if err != nil {
		return GenerateAPIKeyResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return GenerateAPIKeyResponse{}, ReadBodyAsError(res)
	}

	var apiKey GenerateAPIKeyResponse
	return apiKey, json.NewDecoder(res.Body).Decode(&apiKey)
}

type TokensFilter struct {
	IncludeAll bool `json:"include_all"`
}

type APIKeyWithOwner struct {
	APIKey
	Username string `json:"username"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (f TokensFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		q.Set("include_all", fmt.Sprintf("%t", f.IncludeAll))
		r.URL.RawQuery = q.Encode()
	}
}

// Tokens list machine API keys.
func (c *Client) Tokens(ctx context.Context, userID string, filter TokensFilter) ([]APIKeyWithOwner, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/tokens", userID), nil, filter.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	apiKey := []APIKeyWithOwner{}
	return apiKey, json.NewDecoder(res.Body).Decode(&apiKey)
}

// APIKey returns the api key by id.
func (c *Client) APIKey(ctx context.Context, userID string, id string) (*APIKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/%s", userID, id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return nil, ReadBodyAsError(res)
	}
	apiKey := &APIKey{}
	return apiKey, json.NewDecoder(res.Body).Decode(apiKey)
}

// DeleteAPIKey deletes API key by id.
func (c *Client) DeleteAPIKey(ctx context.Context, userID string, id string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/users/%s/keys/%s", userID, id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
