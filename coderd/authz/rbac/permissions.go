package rbac

import (
	"fmt"
	"strings"
)

// ResourcePermissions map a resource to a list of permitted operations on that resource.
type ResourcePermissions map[Resource]Operations

// RolePermissions map a role to a set of resource permissions.
type RolePermissions map[Role]ResourcePermissions

// String returns a string representation of resource permissions.
func (rp ResourcePermissions) String() string {
	var b strings.Builder
	for resource, operations := range rp {
		_, _ = fmt.Fprintf(&b, "\n%s.%s", resource, operations)
	}
	return b.String()
}

// String returns a string representation of role permissions.
func (rp RolePermissions) String() string {
	var b strings.Builder
	for role, permissions := range rp {
		_, _ = fmt.Fprintf(&b, "%s:%s", role, permissions)
	}
	return b.String()
}
