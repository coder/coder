package rbac

import (
	"fmt"
	"strings"
)

// Enforcer is a lookup table of permissions.
type Enforcer struct {
	Inheritances
	RolePermissions
}

// GetResourcePermissions returns all of the permissions of a given role.
func (e Enforcer) GetResourcePermissions(role Role) ResourcePermissions {
	return e.RolePermissions[role]
}

// GetOperations return all operations a role can do on a resource.
func (e Enforcer) GetOperations(role Role, resource Resource) Operations {
	return e.RolePermissions[role][resource]
}

// RoleHasDirectPermission returns whether a role has been given a permission directly.
func (e Enforcer) RoleHasDirectPermission(role Role, resource Resource, operation Operation) bool {
	for _, op := range e.GetOperations(role, resource) {
		if op == operation {
			return true
		}
	}
	return false
}

// RoleHasInheritedPermission returns whether a Role has been given a permission indirectly via inheritance.
func (e Enforcer) RoleHasInheritedPermission(role Role, resource Resource, operation Operation) bool {
	for _, relative := range e.Inheritances.GetAncestors(role) {
		if e.RoleHasDirectPermission(relative, resource, operation) {
			return true
		}
	}
	return false
}

// RoleHasPermission returns whether a role has a permission, directly or indirectly.
func (e Enforcer) RoleHasPermission(role Role, resource Resource, operation Operation) bool {
	return e.RoleHasDirectPermission(role, resource, operation) || e.RoleHasInheritedPermission(role, resource, operation)
}

// RoleHasAllPermissions returns whether a role has *all* of the permissions.
func (e Enforcer) RoleHasAllPermissions(role Role, resource Resource, operations Operations) bool {
	for _, operation := range operations {
		if !e.RoleHasPermission(role, resource, operation) {
			return false
		}
	}
	return true
}

// RolesHavePermission returns whether *any* of the roles have a permission.
func (e Enforcer) RolesHavePermission(roles Roles, resource Resource, operation Operation) bool {
	for _, role := range roles {
		if e.RoleHasPermission(role, resource, operation) {
			return true
		}
	}
	return false
}

// RolesHaveAllPermissions returns whether *any* of the roles have *all* of the permissions.
func (e Enforcer) RolesHaveAllPermissions(roles Roles, resource Resource, operations Operations) bool {
	for _, role := range roles {
		if e.RoleHasAllPermissions(role, resource, operations) {
			return true
		}
	}
	return false
}

// String returns a string representation of a permissions table.
func (e Enforcer) String() string {
	var b strings.Builder
	_, _ = fmt.Fprintln(&b, e.Inheritances.String())
	for role, permissions := range e.RolePermissions {
		_, _ = fmt.Fprintf(&b, "%v:\n%v", role, permissions)
	}
	return b.String()
}

// Resolve returns the enforcer structure as JSON
func (e Enforcer) Resolve() map[Role]map[Resource]map[Operation]bool {
	output := make(map[Role]map[Resource]map[Operation]bool)
	for role, resources := range e.RolePermissions {
		output[role] = make(map[Resource]map[Operation]bool)
		for resource, ops := range resources {
			output[role][resource] = make(map[Operation]bool)
			for _, op := range ops {
				output[role][resource][op] = true
			}
		}
	}

	// inflate inheritances
	for currentRole, inheritedRoles := range e.Inheritances {
		if output[currentRole] == nil {
			output[currentRole] = make(map[Resource]map[Operation]bool)
		}

		for _, inheritedRole := range inheritedRoles {
			for role, resourcesPermissions := range e.RolePermissions {
				if role == inheritedRole {
					for resource, ops := range resourcesPermissions {
						if output[currentRole][resource] == nil {
							output[currentRole][resource] = make(map[Operation]bool)
						}

						for _, op := range ops {
							output[currentRole][resource][op] = true
						}
					}
				}
			}
		}
	}

	return output
}
