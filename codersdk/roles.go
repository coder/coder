package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// SlimRole omits permission information from a role.
// At present, this is because our apis do not return permission information,
// and it would require extra db calls to fetch this information. The UI does
// not need it, so most api calls will use this structure that omits information.
type SlimRole struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type AssignableRoles struct {
	Role       `table:"r,recursive_inline"`
	Assignable bool `json:"assignable" table:"assignable"`
	// BuiltIn roles are immutable
	BuiltIn bool `json:"built_in" table:"built_in"`
}

// Permission is the format passed into the rego.
type Permission struct {
	// Negate makes this a negative permission
	Negate       bool         `json:"negate"`
	ResourceType RBACResource `json:"resource_type"`
	Action       RBACAction   `json:"action"`
}

// Role is a longer form of SlimRole used to edit custom roles.
type Role struct {
	Name            string       `json:"name" table:"name,default_sort" validate:"username"`
	OrganizationID  string       `json:"organization_id" table:"organization_id" format:"uuid"`
	DisplayName     string       `json:"display_name" table:"display_name"`
	SitePermissions []Permission `json:"site_permissions" table:"site_permissions"`
	// OrganizationPermissions are specific for the organization in the field 'OrganizationID' above.
	OrganizationPermissions []Permission `json:"organization_permissions" table:"org_permissions"`
	UserPermissions         []Permission `json:"user_permissions" table:"user_permissions"`
}

// FullName returns the role name scoped to the organization ID. This is useful if
// printing a set of roles from different scopes, as duplicated names across multiple
// scopes will become unique.
// In practice, this is primarily used in testing.
func (r Role) FullName() string {
	if r.OrganizationID == "" {
		return r.Name
	}
	return r.Name + ":" + r.OrganizationID
}

// PatchOrganizationRole will upsert a custom organization role
func (c *Client) PatchOrganizationRole(ctx context.Context, organizationID uuid.UUID, req Role) (Role, error) {
	res, err := c.Request(ctx, http.MethodPatch,
		fmt.Sprintf("/api/v2/organizations/%s/members/roles", organizationID.String()), req)
	if err != nil {
		return Role{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Role{}, ReadBodyAsError(res)
	}
	var role Role
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
