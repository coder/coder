package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// UserSecret represents a user secret's metadata. The secret value
// is never included in API responses.
type UserSecret struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	EnvName     string    `json:"env_name"`
	FilePath    string    `json:"file_path"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// CreateUserSecretRequest is the payload for creating a new user
// secret. Name and Value are required. All other fields are optional
// and default to empty string.
type CreateUserSecretRequest struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	EnvName     string `json:"env_name,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
}

// UpdateUserSecretRequest is the payload for partially updating a
// user secret. At least one field must be non-nil. Pointer fields
// distinguish "not sent" (nil) from "set to empty string" (pointer
// to empty string).
type UpdateUserSecretRequest struct {
	Value       *string `json:"value,omitempty"`
	Description *string `json:"description,omitempty"`
	EnvName     *string `json:"env_name,omitempty"`
	FilePath    *string `json:"file_path,omitempty"`
}

func (c *Client) CreateUserSecret(ctx context.Context, user string, req CreateUserSecretRequest) (UserSecret, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/secrets", user), req)
	if err != nil {
		return UserSecret{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return UserSecret{}, ReadBodyAsError(res)
	}
	var secret UserSecret
	return secret, json.NewDecoder(res.Body).Decode(&secret)
}

func (c *Client) UserSecrets(ctx context.Context, user string) ([]UserSecret, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/secrets", user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var secrets []UserSecret
	return secrets, json.NewDecoder(res.Body).Decode(&secrets)
}

func (c *Client) UserSecretByName(ctx context.Context, user string, name string) (UserSecret, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/secrets/%s", user, name), nil)
	if err != nil {
		return UserSecret{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserSecret{}, ReadBodyAsError(res)
	}
	var secret UserSecret
	return secret, json.NewDecoder(res.Body).Decode(&secret)
}

func (c *Client) UpdateUserSecret(ctx context.Context, user string, name string, req UpdateUserSecretRequest) (UserSecret, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/users/%s/secrets/%s", user, name), req)
	if err != nil {
		return UserSecret{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserSecret{}, ReadBodyAsError(res)
	}
	var secret UserSecret
	return secret, json.NewDecoder(res.Body).Decode(&secret)
}

func (c *Client) DeleteUserSecret(ctx context.Context, user string, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/users/%s/secrets/%s", user, name), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
