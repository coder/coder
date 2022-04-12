package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

type LoginType struct {
	Type string `json:"type"`
}

// GitSSHKey returns the user's git SSH public key.
func (c *Client) LoginTypes(ctx context.Context) ([]LoginType, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/auth/login-types", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var loginTypes []LoginType
	return nil, json.NewDecoder(res.Body).Decode(&loginTypes)
}
