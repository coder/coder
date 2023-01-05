package codersdk

import (
	"context"
	"net/http"
)

type UpdateBrandingRequest struct {
	LogoURL string `json:"logo_url"`
}

// UpdateBranding applies customization settings available to Enterprise customers.
func (c *Client) UpdateBranding(ctx context.Context, req UpdateBrandingRequest) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/branding", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}
