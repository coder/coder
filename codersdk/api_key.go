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

// CreateMachineKey generates an API key that doesn't expire.
func (c *Client) CreateMachineKey(ctx context.Context) (*GenerateAPIKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys/machine", Me), nil)
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

func (c *Client) GetAPIKey(ctx context.Context, user string, id string) (*APIKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/keys/%s", user, id), nil)
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
