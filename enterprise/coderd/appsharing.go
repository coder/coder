package coderd

import (
	"net/http"

	"golang.org/x/xerrors"

	agplcoderd "github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
)

// EnterpriseAppAuthorizer provides an enterprise implementation of
// agplcoderd.AppAuthorizer that allows apps to be shared at certain levels.
type EnterpriseAppAuthorizer struct {
	RBAC                      rbac.Authorizer
	LevelOwnerAllowed         bool
	LevelTemplateAllowed      bool
	LevelAuthenticatedAllowed bool
	LevelPublicAllowed        bool
}

var _ agplcoderd.AppAuthorizer = &EnterpriseAppAuthorizer{}

// Authorize implements agplcoderd.AppAuthorizer.
func (a *EnterpriseAppAuthorizer) Authorize(r *http.Request, db database.Store, SharingLevel database.AppSharingLevel, workspace database.Workspace) (bool, error) {
	ctx := r.Context()

	// Short circuit if not authenticated.
	roles, ok := httpmw.UserAuthorizationOptional(r)
	if !ok {
		// The user is not authenticated, so they can only access the app if it
		// is public and the public level is allowed.
		return SharingLevel == database.AppSharingLevelPublic && a.LevelPublicAllowed, nil
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications.
	err := a.RBAC.ByRoleName(ctx, roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), rbac.ActionCreate, workspace.ApplicationConnectRBAC())
	if err == nil {
		return true, nil
	}

	// Ensure the app's share level is allowed.
	switch SharingLevel {
	case database.AppSharingLevelOwner:
		if !a.LevelOwnerAllowed {
			return false, nil
		}
	case database.AppSharingLevelTemplate:
		if !a.LevelTemplateAllowed {
			return false, nil
		}
	case database.AppSharingLevelAuthenticated:
		if !a.LevelAuthenticatedAllowed {
			return false, nil
		}
	case database.AppSharingLevelPublic:
		if !a.LevelPublicAllowed {
			return false, nil
		}
	default:
		return false, xerrors.Errorf("unknown workspace app sharing level %q", SharingLevel)
	}

	switch SharingLevel {
	case database.AppSharingLevelOwner:
		// We essentially already did this above.
	case database.AppSharingLevelTemplate:
		// Check if the user has access to the same template as the workspace.
		template, err := db.GetTemplateByID(ctx, workspace.TemplateID)
		if err != nil {
			return false, xerrors.Errorf("get template %q: %w", workspace.TemplateID, err)
		}

		err = a.RBAC.ByRoleName(ctx, roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), rbac.ActionRead, template.RBACObject())
		if err == nil {
			return true, nil
		}
	case database.AppSharingLevelAuthenticated:
		// The user is authenticated at this point, but we need to make sure
		// that they have ApplicationConnect permissions to their own
		// workspaces. This ensures that the key's scope has permission to
		// connect to workspace apps.
		object := rbac.ResourceWorkspaceApplicationConnect.WithOwner(roles.ID.String())
		err := a.RBAC.ByRoleName(ctx, roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), rbac.ActionCreate, object)
		if err == nil {
			return true, nil
		}
	case database.AppSharingLevelPublic:
		// We don't really care about scopes and stuff if it's public anyways.
		// Someone with a restricted-scope API key could just not submit the
		// API key cookie in the request and access the page.
		return true, nil
	}

	// No checks were successful.
	return false, nil
}
