package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// GitProvider is a constant that represents the
// type of providers that are supported within Coder.
// @typescript-ignore GitProvider
type GitProvider string

const (
	GitProviderAzureDevops = "azure_devops"
	GitProviderGitHub      = "github"
	GitProviderGitLab      = "gitlab"
	GitProviderBitBucket   = "bitbucket"
)

type WorkspaceAgentGitAuthResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
	URL      string `json:"url"`
}

// WorkspaceAgentGitAuth submits a URL to fetch a GIT_ASKPASS username
// and password for. If the URL doesn't match
func (c *Client) WorkspaceAgentGitAuth(ctx context.Context, gitURL string, listen bool) (WorkspaceAgentGitAuthResponse, error) {
	url := "/api/v2/workspaceagents/me/gitauth?url=" + url.QueryEscape(gitURL)
	if listen {
		url += "&listen"
	}
	res, err := c.Request(ctx, http.MethodGet, url, nil)
	if err != nil {
		return WorkspaceAgentGitAuthResponse{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentGitAuthResponse{}, readBodyAsError(res)
	}

	var authResp WorkspaceAgentGitAuthResponse
	return authResp, json.NewDecoder(res.Body).Decode(&authResp)
}
