package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
)

type ServiceBanner struct {
	Enabled         bool   `json:"enabled"`
	Message         string `json:"message,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
}

func (c *Client) ServiceBanner(ctx context.Context) (*ServiceBanner, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/service-banner", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var b ServiceBanner
	return &b, json.NewDecoder(res.Body).Decode(&b)
}

func (c *Client) SetServiceBanner(ctx context.Context, s *ServiceBanner) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/service-banner", s)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}
