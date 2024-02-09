package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

const (
	WorkspaceAgentPortShareLevelOwner         WorkspaceAgentPortShareLevel = "owner"
	WorkspaceAgentPortShareLevelAuthenticated WorkspaceAgentPortShareLevel = "authenticated"
	WorkspaceAgentPortShareLevelPublic        WorkspaceAgentPortShareLevel = "public"
)

type (
	WorkspaceAgentPortShareLevel         string
	UpdateWorkspaceAgentPortShareRequest struct {
		AgentName  string                       `json:"agent_name"`
		Port       int32                        `json:"port"`
		ShareLevel WorkspaceAgentPortShareLevel `json:"share_level"`
	}
	WorkspaceAgentPortShares struct {
		Shares []WorkspaceAgentPortShare `json:"shares"`
	}
	WorkspaceAgentPortShare struct {
		WorkspaceID uuid.UUID                    `json:"workspace_id"`
		AgentName   string                       `json:"agent_name"`
		Port        int32                        `json:"port"`
		ShareLevel  WorkspaceAgentPortShareLevel `json:"share_level"`
	}
	DeleteWorkspaceAgentPortShareRequest struct {
		AgentName string `json:"agent_name"`
		Port      int32  `json:"port"`
	}
)

func (l WorkspaceAgentPortShareLevel) ValidMaxLevel() bool {
	return l == WorkspaceAgentPortShareLevelOwner ||
		l == WorkspaceAgentPortShareLevelAuthenticated ||
		l == WorkspaceAgentPortShareLevelPublic
}

func (l WorkspaceAgentPortShareLevel) ValidPortShareLevel() bool {
	return l == WorkspaceAgentPortShareLevelAuthenticated ||
		l == WorkspaceAgentPortShareLevelPublic
}

func (c *Client) GetWorkspaceAgentPortShares(ctx context.Context, workspaceID uuid.UUID) (WorkspaceAgentPortShares, error) {
	var shares WorkspaceAgentPortShares
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/port-share", workspaceID), nil)
	if err != nil {
		return shares, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return shares, ReadBodyAsError(res)
	}

	var resp WorkspaceAgentPortShares
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) CreateWorkspaceAgentPortShare(ctx context.Context, workspaceID uuid.UUID, req UpdateWorkspaceAgentPortShareRequest) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/port-share", workspaceID), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) DeleteWorkspaceAgentPortShare(ctx context.Context, workspaceID uuid.UUID, req DeleteWorkspaceAgentPortShareRequest) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/workspaces/%s/port-share", workspaceID), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
