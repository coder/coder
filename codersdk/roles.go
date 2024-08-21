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
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	OrganizationID string `json:"organization_id,omitempty"`
}

func (s SlimRole) String() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Name
}

// UniqueName concatenates the organization ID to create a globally unique
// string name for the role.
func (s SlimRole) UniqueName() string {
	if s.OrganizationID != "" {
		return s.Name + ":" + s.OrganizationID
	}
	return s.Name
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

// Role is a longer form of SlimRole that includes permissions details.
type Role struct {
	Name            string       `json:"name" table:"name,default_sort" validate:"username"`
	OrganizationID  string       `json:"organization_id,omitempty" table:"organization id" format:"uuid"`
	DisplayName     string       `json:"display_name" table:"display name"`
	SitePermissions []Permission `json:"site_permissions" table:"site permissions"`
	// OrganizationPermissions are specific for the organization in the field 'OrganizationID' above.
	OrganizationPermissions []Permission `json:"organization_permissions" table:"organization permissions"`
	UserPermissions         []Permission `json:"user_permissions" table:"user permissions"`
}

// CustomRoleRequest is used to edit custom roles.
type CustomRoleRequest struct {
	Name            string       `json:"name" table:"name,default_sort" validate:"username"`
	DisplayName     string       `json:"display_name" table:"display name"`
	SitePermissions []Permission `json:"site_permissions" table:"site permissions"`
	// OrganizationPermissions are specific to the organization the role belongs to.
	OrganizationPermissions []Permission `json:"organization_permissions" table:"organization permissions"`
	UserPermissions         []Permission `json:"user_permissions" table:"user permissions"`
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

// CreateOrganizationRole will create a custom organization role
func (c *Client) CreateOrganizationRole(ctx context.Context, role Role) (Role, error) {
	req := CustomRoleRequest{
		Name:                    role.Name,
		DisplayName:             role.DisplayName,
		SitePermissions:         role.SitePermissions,
		OrganizationPermissions: role.OrganizationPermissions,
		UserPermissions:         role.UserPermissions,
	}

	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/members/roles", role.OrganizationID), req)
	if err != nil {
		return Role{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Role{}, ReadBodyAsError(res)
	}
	var r Role
	return r, json.NewDecoder(res.Body).Decode(&r)
}

// UpdateOrganizationRole will update an existing custom organization role
func (c *Client) UpdateOrganizationRole(ctx context.Context, role Role) (Role, error) {
	req := CustomRoleRequest{
		Name:                    role.Name,
		DisplayName:             role.DisplayName,
		SitePermissions:         role.SitePermissions,
		OrganizationPermissions: role.OrganizationPermissions,
		UserPermissions:         role.UserPermissions,
	}

	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/organizations/%s/members/roles", role.OrganizationID), req)
	if err != nil {
		return Role{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Role{}, ReadBodyAsError(res)
	}
	var r Role
	return r, json.NewDecoder(res.Body).Decode(&r)
}

// DeleteOrganizationRole will delete a custom organization role
func (c *Client) DeleteOrganizationRole(ctx context.Context, organizationID uuid.UUID, roleName string) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/organizations/%s/members/roles/%s", organizationID.String(), roleName), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
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
