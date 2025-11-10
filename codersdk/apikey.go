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
	ID              string               `json:"id" validate:"required"`
	UserID          uuid.UUID            `json:"user_id" validate:"required" format:"uuid"`
	LastUsed        time.Time            `json:"last_used" validate:"required" format:"date-time"`
	ExpiresAt       time.Time            `json:"expires_at" validate:"required" format:"date-time"`
	CreatedAt       time.Time            `json:"created_at" validate:"required" format:"date-time"`
	UpdatedAt       time.Time            `json:"updated_at" validate:"required" format:"date-time"`
	LoginType       LoginType            `json:"login_type" validate:"required" enums:"password,github,oidc,token"`
	Scope           APIKeyScope          `json:"scope" enums:"all,application_connect"` // Deprecated: use Scopes instead.
	Scopes          []APIKeyScope        `json:"scopes"`
	TokenName       string               `json:"token_name" validate:"required"`
	LifetimeSeconds int64                `json:"lifetime_seconds" validate:"required"`
	AllowList       []APIAllowListTarget `json:"allow_list"`
}

// LoginType is the type of login used to create the API key.
type LoginType string

const (
	LoginTypeUnknown  LoginType = ""
	LoginTypePassword LoginType = "password"
	LoginTypeGithub   LoginType = "github"
	LoginTypeOIDC     LoginType = "oidc"
	LoginTypeToken    LoginType = "token"
	// LoginTypeNone is used if no login method is available for this user.
	// If this is set, the user has no method of logging in.
	// API keys can still be created by an owner and used by the user.
	// These keys would use the `LoginTypeToken` type.
	LoginTypeNone LoginType = "none"
)

type APIKeyScope string

type CreateTokenRequest struct {
	Lifetime  time.Duration        `json:"lifetime"`
	Scope     APIKeyScope          `json:"scope,omitempty"` // Deprecated: use Scopes instead.
	Scopes    []APIKeyScope        `json:"scopes,omitempty"`
	TokenName string               `json:"token_name"`
	AllowList []APIAllowListTarget `json:"allow_list,omitempty"`
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
// CreateToken should be used over CreateAPIKey. CreateToken allows better
// tracking of the token's usage and allows for custom expiration.
// Only use CreateAPIKey if you want to emulate the session created for
// a browser like login.
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

type TokenConfig struct {
	MaxTokenLifetime time.Duration `json:"max_token_lifetime"`
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

// APIKeyByID returns the api key by id.
func (c *Client) APIKeyByID(ctx context.Context, userID string, id string) (*APIKey, error) {
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

// APIKeyByName returns the api key by name.
func (c *Client) APIKeyByName(ctx context.Context, userID string, name string) (*APIKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/tokens/%s", userID, name), nil)
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

// GetTokenConfig returns deployment options related to token management
func (c *Client) GetTokenConfig(ctx context.Context, userID string) (TokenConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/tokens/tokenconfig", userID), nil)
	if err != nil {
		return TokenConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return TokenConfig{}, ReadBodyAsError(res)
	}
	tokenConfig := TokenConfig{}
	return tokenConfig, json.NewDecoder(res.Body).Decode(&tokenConfig)
}
