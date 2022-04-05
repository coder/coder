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
	UserID    uuid.UUID `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	PublicKey string    `json:"public_key"`
}

type AgentGitSSHKey struct {
	UserID     uuid.UUID `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	PrivateKey string    `json:"private_key"`
	PublicKey  string    `json:"public_key"`
}

// GitSSHKey returns the user
func (c *Client) GitSSHKey(ctx context.Context, userID uuid.UUID) (GitSSHKey, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/gitsshkey", userID.String()), nil)
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

func (c *Client) RegenerateGitSSHKey(ctx context.Context, userID uuid.UUID) error {
	res, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/gitsshkey", userID.String()), nil)
	if err != nil {
		return xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}

	return nil
}

func (c *Client) AgentGitSSHKey(ctx context.Context) (AgentGitSSHKey, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/workspaceresources/agent/gitsshkey", nil)
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
