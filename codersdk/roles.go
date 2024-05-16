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

// Permission is the format passed into the rego.
type Permission struct {
	// Negate makes this a negative permission
	Negate       bool         `json:"negate"`
	ResourceType RBACResource `json:"resource_type"`
	Action       RBACAction   `json:"action"`
}

// RolePermissions is a longer form of Role used to edit custom roles.
type RolePermissions struct {
	Name            string       `json:"name"`
	DisplayName     string       `json:"display_name"`
	SitePermissions []Permission `json:"site_permissions"`
	// map[<org_id>] -> Permissions
	OrganizationPermissions map[string][]Permission `json:"organization_permissions"`
	UserPermissions         []Permission            `json:"user_permissions"`
}

// UpsertCustomSiteRole will upsert a custom site wide role
func (c *Client) UpsertCustomSiteRole(ctx context.Context, req RolePermissions) (RolePermissions, error) {
	res, err := c.Request(ctx, http.MethodPatch, "/api/v2/users/roles", req)
	if err != nil {
		return RolePermissions{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return RolePermissions{}, ReadBodyAsError(res)
	}
	var role RolePermissions
	return role, json.NewDecoder(res.Body).Decode(&role)
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

// CreatePermissions is a helper function to quickly build permissions.
func CreatePermissions(mapping map[RBACResource][]RBACAction) []Permission {
	perms := make([]Permission, 0)
	for t, actions := range mapping {
		for _, action := range actions {
			perms = append(perms, Permission{
				ResourceType: t,
				Action:       action,
			})
		}
	}
	return perms
}
