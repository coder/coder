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
	PublicKey string    `json:"public_key"`
}

type AgentGitSSHKey struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// GitSSHKey returns the user's git SSH public key.
func (c *Client) GitSSHKey(ctx context.Context, user string) (GitSSHKey, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/gitsshkey", user), nil)
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
func (c *Client) RegenerateGitSSHKey(ctx context.Context, user string) (GitSSHKey, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/gitsshkey", user), nil)
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

// AgentGitSSHKey will return the user's SSH key pair for the workspace.
func (c *Client) AgentGitSSHKey(ctx context.Context) (AgentGitSSHKey, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/gitsshkey", nil)
	if err != nil {
		return AgentGitSSHKey{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AgentGitSSHKey{}, readBodyAsError(res)
	}

	var agentgitsshkey AgentGitSSHKey
	return agentgitsshkey, json.NewDecoder(res.Body).Decode(&agentgitsshkey)
}
