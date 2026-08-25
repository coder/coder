package codersdk

import (
	"context"
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
	// Enabled controls whether the secret is injected into workspaces.
	// Disabled secrets remain visible and editable, but are not added
	// to the agent manifest, so they are not exposed as environment
	// variables or written to secret files.
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time"`
}

// CreateUserSecretRequest is the payload for creating a new user
// secret. Name and Value are required. An enabled secret must have at
// least one of EnvName or FilePath non-empty so it has an injection
// target; to keep a secret without injecting it, set Enabled to false.
// A deployment may disable file path delivery, which rejects a
// non-empty FilePath. All other fields are optional and default to
// empty string. Enabled defaults to true when omitted.
type CreateUserSecretRequest struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	EnvName     string `json:"env_name,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// UpdateUserSecretRequest is the payload for partially updating a
// user secret. At least one field must be non-nil. Pointer fields
// distinguish "not sent" (nil) from "set to empty string" (pointer
// to empty string). If the post-update row is enabled it must still
// have at least one of EnvName or FilePath non-empty; clearing both
// targets is only allowed when the secret is (or becomes) disabled.
// When a deployment disables file path delivery, an enabled row also
// requires EnvName.
type UpdateUserSecretRequest struct {
	Value       *string `json:"value,omitempty"`
	Description *string `json:"description,omitempty"`
	EnvName     *string `json:"env_name,omitempty"`
	FilePath    *string `json:"file_path,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
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
	return secret, ReadBodyAsJSON(res, &secret)
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
	return secrets, ReadBodyAsJSON(res, &secrets)
}

// ImportUserSecretsRequest is the payload for the bulk secret import
// endpoint. Content is the raw file bytes and Format selects the parser.
type ImportUserSecretsRequest struct {
	Format  SecretsFileFormat `json:"format" validate:"required"`
	Content string            `json:"content" validate:"required"`
}

// ImportUserSecrets parses the supplied file content and creates the
// resulting secrets atomically: either all secrets are created or, if
// any entry fails validation, uniqueness, or a per-user limit, none
// are. It returns the created secrets' metadata (never their values).
func (c *Client) ImportUserSecrets(ctx context.Context, user string, req ImportUserSecretsRequest) ([]UserSecret, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/secrets/batch", user), req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return nil, ReadBodyAsError(res)
	}
	var secrets []UserSecret
	return secrets, ReadBodyAsJSON(res, &secrets)
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
	return secret, ReadBodyAsJSON(res, &secret)
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
	return secret, ReadBodyAsJSON(res, &secret)
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
