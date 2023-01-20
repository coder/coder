package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

func (c *Client) DeploymentDAUs(ctx context.Context) (*TemplateDAUsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/insights/daus", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var resp TemplateDAUsResponse
	return &resp, json.NewDecoder(res.Body).Decode(&resp)
}
