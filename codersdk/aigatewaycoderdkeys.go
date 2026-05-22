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

// AIGatewayCoderdKey is a shared secret used by a standalone AI Gateway
// to authenticate into coderd.
type AIGatewayCoderdKey struct {
	ID         uuid.UUID  `json:"id" format:"uuid"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at" format:"date-time"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" format:"date-time"`
}

type CreateAIGatewayCoderdKeyRequest struct {
	Name string `json:"name" validate:"required"`
}

// CreateAIGatewayCoderdKeyResponse returns all key information.
// Key value is only returned here and cannot be recovered afterwards.
type CreateAIGatewayCoderdKeyResponse struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	KeyPrefix string    `json:"key_prefix"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
}

// CreateAIGatewayCoderdKey creates a new AI Gateway coderd key.
func (c *Client) CreateAIGatewayCoderdKey(ctx context.Context, req CreateAIGatewayCoderdKeyRequest) (CreateAIGatewayCoderdKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/aibridge/coderd-keys", req)
	if err != nil {
		return CreateAIGatewayCoderdKeyResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return CreateAIGatewayCoderdKeyResponse{}, ReadBodyAsError(res)
	}
	var resp CreateAIGatewayCoderdKeyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ListAIGatewayCoderdKeys lists all AI Gateway coderd keys.
func (c *Client) ListAIGatewayCoderdKeys(ctx context.Context) ([]AIGatewayCoderdKey, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/coderd-keys", nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []AIGatewayCoderdKey
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteAIGatewayCoderdKey deletes an AI Gateway coderd key by ID.
func (c *Client) DeleteAIGatewayCoderdKey(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/aibridge/coderd-keys/%s", id.String()), nil)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
