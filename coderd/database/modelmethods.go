package database

import (
	"github.com/coder/coder/coderd/rbac"
)

const AllUsersGroup = "Everyone"

func (s APIKeyScope) ToRBAC() rbac.Scope {
	switch s {
	case APIKeyScopeAll:
		return rbac.ScopeAll
	case APIKeyScopeApplicationConnect:
		return rbac.ScopeApplicationConnect
	default:
		panic("developer error: unknown scope type " + string(s))
	}
}

func (t Template) RBACObject() rbac.Object {
	obj := rbac.ResourceTemplate
	return obj.InOrg(t.OrganizationID).
		WithACLUserList(t.UserACL).
		WithGroupACL(t.GroupACL)
}

func (TemplateVersion) RBACObject(template Template) rbac.Object {
	// Just use the parent template resource for controlling versions
	return template.RBACObject()
}

func (g Group) RBACObject() rbac.Object {
	return rbac.ResourceGroup.InOrg(g.OrganizationID)
}

func (w Workspace) RBACObject() rbac.Object {
	return rbac.ResourceWorkspace.InOrg(w.OrganizationID).WithOwner(w.OwnerID.String())
}

func (w Workspace) ExecutionRBAC() rbac.Object {
	return rbac.ResourceWorkspaceExecution.InOrg(w.OrganizationID).WithOwner(w.OwnerID.String())
}

func (w Workspace) ApplicationConnectRBAC() rbac.Object {
	return rbac.ResourceWorkspaceApplicationConnect.InOrg(w.OrganizationID).WithOwner(w.OwnerID.String())
}

func (m OrganizationMember) RBACObject() rbac.Object {
	return rbac.ResourceOrganizationMember.InOrg(m.OrganizationID)
}

func (o Organization) RBACObject() rbac.Object {
	return rbac.ResourceOrganization.InOrg(o.ID)
}

func (ProvisionerDaemon) RBACObject() rbac.Object {
	return rbac.ResourceProvisionerDaemon
}

func (f File) RBACObject() rbac.Object {
	return rbac.ResourceFile.WithOwner(f.CreatedBy.String())
}

// RBACObject returns the RBAC object for the site wide user resource.
// If you are trying to get the RBAC object for the UserData, use
// rbac.ResourceUserData
func (User) RBACObject() rbac.Object {
	return rbac.ResourceUser
}

func (License) RBACObject() rbac.Object {
	return rbac.ResourceLicense
}
