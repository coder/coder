package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
)

type PrebuildsSettings struct {
	ReconciliationPaused bool `json:"reconciliation_paused"`
}

// GetPrebuildsSettings retrieves the prebuilds settings, which currently just describes whether all
// prebuild reconciliation is paused.
func (c *Client) GetPrebuildsSettings(ctx context.Context) (PrebuildsSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/prebuilds/settings", nil)
	if err != nil {
		return PrebuildsSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return PrebuildsSettings{}, ReadBodyAsError(res)
	}
	var settings PrebuildsSettings
	return settings, json.NewDecoder(res.Body).Decode(&settings)
}

// PutPrebuildsSettings modifies the prebuilds settings, which currently just controls whether all
// prebuild reconciliation is paused.
func (c *Client) PutPrebuildsSettings(ctx context.Context, settings PrebuildsSettings) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/prebuilds/settings", settings)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		return nil
	}
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
