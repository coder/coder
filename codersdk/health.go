package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

type HealthSettings struct {
	DismissedHealthchecks []string `json:"dismissed_healthchecks"`
}

type UpdateHealthSettings struct {
	DismissedHealthchecks []string `json:"dismissed_healthchecks"`
}

func (c *Client) HealthSettings(ctx context.Context) (HealthSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/debug/health/settings", nil)
	if err != nil {
		return HealthSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return HealthSettings{}, ReadBodyAsError(res)
	}
	var settings HealthSettings
	return settings, json.NewDecoder(res.Body).Decode(&settings)
}

func (c *Client) PutHealthSettings(ctx context.Context, settings HealthSettings) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/debug/health/settings", settings)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNoContent {
		return xerrors.New("health settings not modified")
	}
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
