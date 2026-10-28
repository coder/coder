package database

import "github.com/coder/coder/v2/coderd/rbac"

// RBACObject identifies the independent organization-scoped Foundation.
func (f Foundation) RBACObject() rbac.Object {
	return rbac.ResourceFoundation.WithID(f.ID).InOrg(f.OrganizationID)
}
