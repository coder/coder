package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
)

// UpdateCheckResponse contains information on the latest release of Coder.
type UpdateCheckResponse struct {
	// Current indicates whether the server version is the same as the latest.
	Current bool `json:"current"`
	// Version is the semantic version for the latest release of Coder.
	Version string `json:"version"`
	// URL to download the latest release of Coder.
	URL string `json:"url"`
}

// UpdateCheck returns information about the latest release version of
// Coder and whether or not the server is running the latest release.
func (c *Client) UpdateCheck(ctx context.Context) (UpdateCheckResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/updatecheck", nil)
	if err != nil {
		return UpdateCheckResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UpdateCheckResponse{}, ReadBodyAsError(res)
	}

	var buildInfo UpdateCheckResponse
	return buildInfo, json.NewDecoder(res.Body).Decode(&buildInfo)
}
