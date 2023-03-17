package codersdk

import "github.com/coder/coder/coderd/rbac"

// This mirrors modelmethods.go in /database.
// For unit testing, it helps to have these convenience functions for asserting
// rbac authorization calls.

func (t Template) RBACObject(userACL, groupACL map[string][]rbac.Action) rbac.Object {
	return rbac.ResourceTemplate.WithID(t.ID).
		InOrg(t.OrganizationID).
		WithACLUserList(userACL).
		WithGroupACL(groupACL)
}

// RBACObjectNoTemplate is for orphaned template versions.
func (v TemplateVersion) RBACObjectNoTemplate() rbac.Object {
	return rbac.ResourceTemplate.InOrg(v.OrganizationID)
}
