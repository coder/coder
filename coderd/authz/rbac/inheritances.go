package rbac

import (
	"fmt"
	"strings"
)

// Inheritances map a role to roles from which it inherits permissions.
type Inheritances map[Role]Roles

// GetAncestors returns all of the roles from which a role inherits permissions.
func (i Inheritances) GetAncestors(role Role) Roles {
	return i[role]
}

// String returns a string representation of inheritances.
func (i Inheritances) String() string {
	var b strings.Builder
	for role, roles := range i {
		_, _ = fmt.Fprintf(&b, "%v:%v", role, roles)
	}
	return b.String()
}
