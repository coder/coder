package database

import (
	"encoding/json"
	"fmt"

	"github.com/coder/coder/coderd/rbac"
)

// UserACL is a map of user_ids to permissions.
type UserACL map[string]TemplateRole

func (u UserACL) Actions() map[string][]rbac.Action {
	aclRBAC := make(map[string][]rbac.Action, len(u))
	for k, v := range u {
		aclRBAC[k] = templateRoleToActions(v)
	}

	return aclRBAC
}

func (t Template) UserACL() UserACL {
	var acl UserACL
	if len(t.userACL) == 0 {
		return acl
	}

	err := json.Unmarshal(t.userACL, &acl)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal template.userACL: %v", err.Error()))
	}

	return acl
}

func (t Template) SetUserACL(acl UserACL) Template {
	raw, err := json.Marshal(acl)
	if err != nil {
		panic(fmt.Sprintf("marshal user acl: %v", err))
	}

	t.userACL = raw
	return t
}

func templateRoleToActions(t TemplateRole) []rbac.Action {
	switch t {
	case TemplateRoleRead:
		return []rbac.Action{rbac.ActionRead}
	case TemplateRoleWrite:
		return []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}
	case TemplateRoleAdmin:
		// TODO: Why does rbac.Wildcard not work here?
		return []rbac.Action{rbac.ActionRead, rbac.ActionUpdate, rbac.ActionCreate, rbac.ActionDelete}
	}
	return nil
}

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
	if t.IsPrivate {
		obj = rbac.ResourceTemplatePrivate
	}
	return obj.InOrg(t.OrganizationID).WithACLUserList(t.UserACL().Actions())
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
