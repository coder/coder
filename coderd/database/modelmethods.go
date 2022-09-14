package database

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

var validAPIKeyScopes map[string]ApiKeyScope

func init() {
	validAPIKeyScopes = make(map[string]ApiKeyScope)
	for _, scope := range []ApiKeyScope{ApiKeyScopeAny, ApiKeyScopeDevurls} {
		validAPIKeyScopes[string(scope)] = scope
	}
}

func ToAPIKeyScope(v string) (ApiKeyScope, error) {
	scope, ok := validAPIKeyScopes[v]
	if !ok {
		return ApiKeyScope(""), xerrors.Errorf("invalid token scope: %s", v)
	}

	return scope, nil
}

func (s ApiKeyScope) ToRBAC() rbac.Scope {
	switch {
	case s == ApiKeyScopeAny:
		return rbac.ScopeAny
	case s == ApiKeyScopeDevurls:
		return rbac.ScopeDevURLs
	default:
		panic("developer error: unknown scope type " + string(s))
	}
}

func (t Template) RBACObject() rbac.Object {
	return rbac.ResourceTemplate.InOrg(t.OrganizationID)
}

func (t TemplateVersion) RBACObject() rbac.Object {
	// Just use the parent template resource for controlling versions
	return rbac.ResourceTemplate.InOrg(t.OrganizationID)
}

func (w Workspace) RBACObject() rbac.Object {
	return rbac.ResourceWorkspace.InOrg(w.OrganizationID).WithOwner(w.OwnerID.String())
}

func (w Workspace) ExecutionRBAC() rbac.Object {
	return rbac.ResourceWorkspaceExecution.InOrg(w.OrganizationID).WithOwner(w.OwnerID.String())
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
