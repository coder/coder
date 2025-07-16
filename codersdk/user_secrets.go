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

// TODO: add and register custom validator functions. check codersdk/name.go for examples.
// TODO: reuse NameValid func for Name?
type CreateUserSecretRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty" validate:"lt=1000"`
	Value       string `json:"value" validate:"required"`
}

type UpdateUserSecretRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty" validate:"lt=1000"`
	Value       string `json:"value" validate:"required"`
}

// Response types
type UserSecret struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	UserID      uuid.UUID `json:"user_id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

type UserSecretValue struct {
	Value string `json:"value"`
}

type ListUserSecretsResponse struct {
	Secrets []UserSecret `json:"secrets"`
}

func (c *Client) CreateUserSecret(ctx context.Context, req CreateUserSecretRequest) (UserSecret, error) {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/users/secrets",
		req,
	)
	if err != nil {
		return UserSecret{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return UserSecret{}, ReadBodyAsError(res)
	}

	var userSecret UserSecret
	return userSecret, json.NewDecoder(res.Body).Decode(&userSecret)
}

func (c *Client) ListUserSecrets(ctx context.Context) (ListUserSecretsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/users/secrets",
		nil,
	)
	if err != nil {
		return ListUserSecretsResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ListUserSecretsResponse{}, ReadBodyAsError(res)
	}

	var userSecrets ListUserSecretsResponse
	return userSecrets, json.NewDecoder(res.Body).Decode(&userSecrets)
}

func (c *Client) GetUserSecret(ctx context.Context, secretName string) (UserSecret, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/users/secrets/%v", secretName),
		nil,
	)
	if err != nil {
		return UserSecret{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UserSecret{}, ReadBodyAsError(res)
	}

	var userSecret UserSecret
	return userSecret, json.NewDecoder(res.Body).Decode(&userSecret)
}
