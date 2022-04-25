package rbac

import (
	"strings"

	"golang.org/x/xerrors"
)

const (
	Admin  = "admin"
	Member = "member"

	OrganizationMember = "organization-member"
	OrganizationAdmin  = "organization-admin"
)

// RoleByName returns the permissions associated with a given role name.
// This allows just the role names to be stored.
func RoleByName(name string) (Role, error) {
	arr := strings.Split(name, ":")
	if len(arr) > 2 {
		return Role{}, xerrors.Errorf("too many semicolons in role name")
	}

	roleName := arr[0]
	var scopeID string
	if len(arr) > 1 {
		scopeID = arr[1]
	}

	// If the role requires a scope, the scope will be checked at the end
	// of the switch statement.
	var scopedRole Role
	switch roleName {
	case Admin:
		return RoleAdmin, nil
	case Member:
		return RoleMember, nil
	case OrganizationMember:
		scopedRole = RoleOrgMember(scopeID)
	case OrganizationAdmin:
		scopedRole = RoleOrgAdmin(scopeID)
	default:
		// No role found
		return Role{}, xerrors.Errorf("role %q not found", roleName)
	}

	// Scoped roles should be checked their scope is set
	if scopeID == "" {
		return Role{}, xerrors.Errorf("%q requires a scope id", roleName)
	}

	return scopedRole, nil
}

func RoleName(name string, scopeID string) string {
	if scopeID == "" {
		return name
	}
	return name + ":" + scopeID
}
