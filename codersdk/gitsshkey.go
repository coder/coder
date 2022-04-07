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
	UserID     uuid.UUID `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	PrivateKey string    `json:"private_key"`
	PublicKey  string    `json:"public_key"`
}

// GitSSHKey returns the user's git SSH key pair.
func (c *Client) GitSSHKey(ctx context.Context) (GitSSHKey, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/me/gitsshkey"), nil)
	if err != nil {
		return GitSSHKey{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GitSSHKey{}, readBodyAsError(res)
	}

	var gitsshkey GitSSHKey
	return gitsshkey, json.NewDecoder(res.Body).Decode(&gitsshkey)
}

// RegenerateGitSSHKey will create a new SSH key pair for the user and return it.
func (c *Client) RegenerateGitSSHKey(ctx context.Context) (GitSSHKey, error) {
	res, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/me/gitsshkey"), nil)
	if err != nil {
		return GitSSHKey{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GitSSHKey{}, readBodyAsError(res)
	}

	var gitsshkey GitSSHKey
	return gitsshkey, json.NewDecoder(res.Body).Decode(&gitsshkey)
}
