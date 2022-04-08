package authz

import "fmt"

// Role is a set of permissions at multiple levels:
// - Site level permissions apply EVERYWHERE
// - Org level permissions apply to EVERYTHING in a given ORG
// - User level permissions are the lowest
// In most cases, you will just want to use the pre-defined roles
// below.
type Role struct {
	Name string
	Site []Permission
	Org  map[string][]Permission
	User []Permission
}

var (
	// RoleSiteAdmin is a role that allows everything everywhere.
	RoleSiteAdmin = Role{
		Name: "site-admin",
		Site: []Permission{
			{
				Negate:       false,
				ResourceType: "*",
				ResourceID:   "*",
				Action:       "*",
			},
		},
	}

	// RoleDenyAll is a role that denies everything everywhere.
	RoleDenyAll = Role{
		Name: "deny-all",
		Site: []Permission{
			{
				Negate:       true,
				ResourceType: "*",
				ResourceID:   "*",
				Action:       "*",
			},
		},
	}

	// RoleSiteMember is a role that allows access to user-level resources.
	RoleSiteMember = Role{
		Name: "site-member",
		User: []Permission{
			{
				Negate:       false,
				ResourceType: "*",
				ResourceID:   "*",
				Action:       "*",
			},
		},
	}
)

// RoleOrgAdmin returns a role with all actions allows in a given
// organization scope.
func RoleOrgAdmin(orgID string) Role {
	return Role{
		Name: "org-admin-" + orgID,
		Org: map[string][]Permission{
			orgID: {
				{
					Negate:       false,
					ResourceType: "*",
					ResourceID:   "*",
					Action:       "*",
				},
			},
		},
	}
}

// RoleOrgMember returns a role with default permissions in a given
// organization scope.
func RoleOrgMember(orgID string) Role {
	return Role{
		Name: "org-member-" + orgID,
		Org: map[string][]Permission{
			orgID: {},
		},
	}
}

// RoleWorkspaceAgent returns a role with permission to read a given
// workspace.
func RoleWorkspaceAgent(workspaceID string) Role {
	return Role{
		Name: fmt.Sprintf("agent-%s", workspaceID),
		Site: []Permission{
			{
				Negate:       false,
				ResourceType: ResourceWorkspace,
				ResourceID:   workspaceID,
				Action:       ActionRead,
			},
		},
	}
}
