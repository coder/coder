package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

type NotificationsSettings struct {
	NotifierPaused bool `json:"notifier_paused"`
}

func (c *Client) GetNotificationsSettings(ctx context.Context) (NotificationsSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/notifications/settings", nil)
	if err != nil {
		return NotificationsSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return NotificationsSettings{}, ReadBodyAsError(res)
	}
	var settings NotificationsSettings
	return settings, json.NewDecoder(res.Body).Decode(&settings)
}

func (c *Client) PutNotificationsSettings(ctx context.Context, settings NotificationsSettings) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/notifications/settings", settings)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		return xerrors.New("notifications settings not modified")
	}
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
