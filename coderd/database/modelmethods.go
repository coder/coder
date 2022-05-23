package database

import "github.com/coder/coder/coderd/rbac"

func (t Template) RBACObject() rbac.Object {
	return rbac.ResourceTemplate.InOrg(t.OrganizationID).WithID(t.ID.String())
}
