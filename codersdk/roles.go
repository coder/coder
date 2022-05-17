package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type Role struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// ListSiteRoles lists all available site wide roles.
// This is not user specific.
func (c *Client) ListSiteRoles(ctx context.Context) ([]Role, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/users/roles", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var roles []Role
	return roles, json.NewDecoder(res.Body).Decode(&roles)
}

// ListOrganizationRoles lists all available roles for a given organization.
// This is not user specific.
func (c *Client) ListOrganizationRoles(ctx context.Context, org uuid.UUID) ([]Role, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/members/roles", org.String()), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var roles []Role
	return roles, json.NewDecoder(res.Body).Decode(&roles)
}

func (c *Client) CheckPermissions(ctx context.Context, checks UserAuthorizationRequest) (UserAuthorizationResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/authorization", Me), checks)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var roles UserAuthorizationResponse
	return roles, json.NewDecoder(res.Body).Decode(&roles)
}
