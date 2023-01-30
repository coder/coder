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

type AssignableRoles struct {
	Role
	Assignable bool `json:"assignable"`
}

// ListSiteRoles lists all assignable site wide roles.
func (c *Client) ListSiteRoles(ctx context.Context) ([]AssignableRoles, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/users/roles", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var roles []AssignableRoles
	return roles, json.NewDecoder(res.Body).Decode(&roles)
}

// ListOrganizationRoles lists all assignable roles for a given organization.
func (c *Client) ListOrganizationRoles(ctx context.Context, org uuid.UUID) ([]AssignableRoles, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/members/roles", org.String()), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var roles []AssignableRoles
	return roles, json.NewDecoder(res.Body).Decode(&roles)
}
