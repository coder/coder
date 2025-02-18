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

type GitSSHKey struct {
	UserID    uuid.UUID `json:"user_id" format:"uuid"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time"`
	// PublicKey is the SSH public key in OpenSSH format.
	// Example: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID3OmYJvT7q1cF1azbybYy0OZ9yrXfA+M6Lr4vzX5zlp\n"
	// Note: The key includes a trailing newline (\n).
	PublicKey string `json:"public_key"`
}

// GitSSHKey returns the user's git SSH public key.
func (c *Client) GitSSHKey(ctx context.Context, user string) (GitSSHKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/gitsshkey", user), nil)
	if err != nil {
		return GitSSHKey{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GitSSHKey{}, ReadBodyAsError(res)
	}

	var gitsshkey GitSSHKey
	return gitsshkey, json.NewDecoder(res.Body).Decode(&gitsshkey)
}

// RegenerateGitSSHKey will create a new SSH key pair for the user and return it.
func (c *Client) RegenerateGitSSHKey(ctx context.Context, user string) (GitSSHKey, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/gitsshkey", user), nil)
	if err != nil {
		return GitSSHKey{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GitSSHKey{}, ReadBodyAsError(res)
	}

	var gitsshkey GitSSHKey
	return gitsshkey, json.NewDecoder(res.Body).Decode(&gitsshkey)
}
