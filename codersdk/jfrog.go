package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type JFrogXrayScan struct {
	WorkspaceID uuid.UUID `json:"workspace_id" format:"uuid"`
	AgentID     uuid.UUID `json:"agent_id" format:"uuid"`
	Critical    int       `json:"critical"`
	High        int       `json:"high"`
	Medium      int       `json:"medium"`
	ResultsURL  string    `json:"results_url"`
}

func (c *Client) PostJFrogXrayScan(ctx context.Context, req JFrogXrayScan) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/integrations/jfrog/xray-scan", req)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) JFrogXRayScan(ctx context.Context, workspaceID, agentID uuid.UUID) (JFrogXrayScan, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/integrations/jfrog/xray-scan", nil,
		WithQueryParam("workspace_id", workspaceID.String()),
		WithQueryParam("agent_id", agentID.String()),
	)
	if err != nil {
		return JFrogXrayScan{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return JFrogXrayScan{}, ReadBodyAsError(res)
	}

	var resp JFrogXrayScan
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
