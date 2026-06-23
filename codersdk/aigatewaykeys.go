package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// AIGatewayKey is a shared secret used by a standalone AI Gateway
// to authenticate into coderd.
type AIGatewayKey struct {
	ID         uuid.UUID  `json:"id" table:"id" format:"uuid"`
	Name       string     `json:"name" table:"name,default_sort"`
	KeyPrefix  string     `json:"key_prefix" table:"key prefix"`
	CreatedAt  time.Time  `json:"created_at" table:"created at" format:"date-time"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" table:"last used at" format:"date-time"`
}

// CreateAIGatewayKeyRequest requests a new AI Gateway key.
type CreateAIGatewayKeyRequest struct {
	Name string `json:"name" validate:"required"`
}

// CreateAIGatewayKeyResponse returns all key information.
// Key value is only returned here and cannot be recovered afterwards.
type CreateAIGatewayKeyResponse struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	KeyPrefix string    `json:"key_prefix"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
}

// CreateAIGatewayKey creates a new AI Gateway key.
func (c *Client) CreateAIGatewayKey(ctx context.Context, req CreateAIGatewayKeyRequest) (CreateAIGatewayKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/ai-gateway/keys", req)
	if err != nil {
		return CreateAIGatewayKeyResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return CreateAIGatewayKeyResponse{}, ReadBodyAsError(res)
	}
	var resp CreateAIGatewayKeyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ListAIGatewayKeys lists all AI Gateway keys.
func (c *Client) ListAIGatewayKeys(ctx context.Context) ([]AIGatewayKey, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai-gateway/keys", nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []AIGatewayKey
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteAIGatewayKey deletes an AI Gateway key by ID.
func (c *Client) DeleteAIGatewayKey(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/ai-gateway/keys/%s", id.String()), nil)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
