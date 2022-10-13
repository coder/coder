package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID string `json:"id" validate:"required"`
	// NOTE: do not ever return the HashedSecret
	UserID          uuid.UUID `json:"user_id" validate:"required"`
	LastUsed        time.Time `json:"last_used" validate:"required"`
	ExpiresAt       time.Time `json:"expires_at" validate:"required"`
	CreatedAt       time.Time `json:"created_at" validate:"required"`
	UpdatedAt       time.Time `json:"updated_at" validate:"required"`
	LoginType       LoginType `json:"login_type" validate:"required"`
	LifetimeSeconds int64     `json:"lifetime_seconds" validate:"required"`
}

type LoginType string

const (
	LoginTypePassword LoginType = "password"
	LoginTypeGithub   LoginType = "github"
	LoginTypeOIDC     LoginType = "oidc"
	LoginTypeToken    LoginType = "token"
)

// CreateToken generates an API key that doesn't expire.
func (c *Client) CreateToken(ctx context.Context, userID string) (*GenerateAPIKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys/tokens", userID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return nil, readBodyAsError(res)
	}
	apiKey := &GenerateAPIKeyResponse{}
	return apiKey, json.NewDecoder(res.Body).Decode(apiKey)
}

// CreateAPIKey generates an API key for the user ID provided.
func (c *Client) CreateAPIKey(ctx context.Context, user string) (*GenerateAPIKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys", user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return nil, readBodyAsError(res)
	}
	apiKey := &GenerateAPIKeyResponse{}
	return apiKey, json.NewDecoder(res.Body).Decode(apiKey)
}

// GetTokens list machine API keys.
func (c *Client) GetTokens(ctx context.Context, userID string) ([]APIKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/tokens", userID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var apiKey = []APIKey{}
	return apiKey, json.NewDecoder(res.Body).Decode(&apiKey)
}

// GetAPIKey returns the api key by id.
func (c *Client) GetAPIKey(ctx context.Context, userID string, id string) (*APIKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/%s", userID, id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return nil, readBodyAsError(res)
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
		return readBodyAsError(res)
	}
	return nil
}
