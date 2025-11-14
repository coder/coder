package tallymansdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

// DashboardType represents the type of dashboard to embed.
type DashboardType string

const (
	// DashboardTypeUsage is the usage dashboard type.
	DashboardTypeUsage DashboardType = "usage"
)

// DashboardColorOverride represents a color override for a dashboard.
type DashboardColorOverride struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RetrieveEmbeddableDashboardRequest is a request to get an embed URL for a dashboard.
type RetrieveEmbeddableDashboardRequest struct {
	Dashboard      DashboardType            `json:"dashboard"`
	ColorOverrides []DashboardColorOverride `json:"color_overrides,omitempty"`
}

// RetrieveEmbeddableDashboardResponse is a response containing a dashboard embed URL.
type RetrieveEmbeddableDashboardResponse struct {
	DashboardURL string `json:"dashboard_url"`
}

// RetrieveEmbeddableDashboard retrieves an embed URL for a dashboard from the Tallyman API.
func (c *Client) RetrieveEmbeddableDashboard(ctx context.Context, req RetrieveEmbeddableDashboardRequest) (RetrieveEmbeddableDashboardResponse, error) {
	resp, err := c.Request(ctx, http.MethodPost, "/api/v1/dashboards/embed", req)
	if err != nil {
		return RetrieveEmbeddableDashboardResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RetrieveEmbeddableDashboardResponse{}, readErrorResponse(resp)
	}

	var respBody RetrieveEmbeddableDashboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return RetrieveEmbeddableDashboardResponse{}, xerrors.Errorf("decode response body: %w", err)
	}

	return respBody, nil
}
