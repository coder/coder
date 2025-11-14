package tallymansdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

// TallymanValidColorOverrides contains the valid color names that can be overridden
// in Metronome dashboards. These correspond to the color palette available in the
// Metronome embedding API.
var TallymanValidColorOverrides = map[string]struct{}{
	"Gray_dark":               {},
	"Gray_medium":             {},
	"Gray_light":              {},
	"Gray_extralight":         {},
	"White":                   {},
	"Primary_medium":          {},
	"Primary_light":           {},
	"UsageLine_0":             {},
	"UsageLine_1":             {},
	"UsageLine_2":             {},
	"UsageLine_3":             {},
	"UsageLine_4":             {},
	"UsageLine_5":             {},
	"UsageLine_6":             {},
	"UsageLine_7":             {},
	"UsageLine_8":             {},
	"UsageLine_9":             {},
	"Primary_green":           {},
	"Primary_red":             {},
	"Progress_bar":            {},
	"Progress_bar_background": {},
}

// DashboardType represents the type of dashboard to embed.
// Corresponds to codersdk.UsageEmbeddableDashboardType.
type DashboardType string

const (
	// DashboardTypeUsage is the usage dashboard type.
	DashboardTypeUsage DashboardType = "usage"
)

// DashboardColorOverride represents a color override for a dashboard.
// Corresponds to codersdk.DashboardColorOverride.
type DashboardColorOverride struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RetrieveEmbeddableDashboardRequest is a request to get an embed URL for a dashboard.
// Corresponds to codersdk.GetUsageEmbeddableDashboardRequest.
type RetrieveEmbeddableDashboardRequest struct {
	Dashboard      DashboardType            `json:"dashboard"`
	ColorOverrides []DashboardColorOverride `json:"color_overrides,omitempty"`
}

// RetrieveEmbeddableDashboardResponse is a response containing a dashboard embed URL.
// Corresponds to codersdk.GetUsageEmbeddableDashboardResponse.
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
