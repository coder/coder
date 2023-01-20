package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

type DeploymentDAUsResponse struct {
	Entries []DAUEntry `json:"entries"`
}

func (c *Client) DeploymentDAUs(ctx context.Context) (*DeploymentDAUsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/insights/daus", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var resp DeploymentDAUsResponse
	return &resp, json.NewDecoder(res.Body).Decode(&resp)
}
