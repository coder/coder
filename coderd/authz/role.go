package authz

import "fmt"

const Wildcard = "*"

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

// Roles are stored as structs, so they can be serialized and stored. Until we store them elsewhere,
// const's will do just fine.
var (
	// RoleSiteAdmin is a role that allows everything everywhere.
	RoleSiteAdmin = Role{
		Name: "site-admin",
		Site: permissions(map[ResourceType][]Action{
			Wildcard: {Wildcard},
		}),
	}

	// RoleSiteMember is a role that allows access to user-level resources.
	RoleSiteMember = Role{
		Name: "site-member",
		User: permissions(map[ResourceType][]Action{
			Wildcard: {Wildcard},
		}),
	}

	// RoleSiteAuditor is an example on how to give more precise permissions
	RoleSiteAuditor = Role{
		Name: "site-auditor",
		Site: permissions(map[ResourceType][]Action{
			ResourceAuditLogs: {ActionRead},
			// Should be able to read user details to associate with logs.
			// Without this the user-id in logs is not very helpful
			ResourceUser: {ActionRead},
		}),
	}

	// RoleDenyAll is a role that denies everything everywhere.
	RoleDenyAll = Role{
		Name: "deny-all",
		// List out deny permissions explicitly
		Site: []Permission{
			{
				Negate:       true,
				ResourceType: Wildcard,
				ResourceID:   Wildcard,
				Action:       Wildcard,
			},
		},
	}
)

func RoleOrgDenyAll(orgID string) Role {
	return Role{
		Name: "org-deny-" + orgID,
		Org: map[string][]Permission{
			orgID: {
				{
					Negate:       true,
					ResourceType: "*",
					ResourceID:   "*",
					Action:       "*",
				},
			},
		},
	}
}

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

func permissions(perms map[ResourceType][]Action) []Permission {
	list := make([]Permission, 0, len(perms))
	for k, actions := range perms {
		for _, act := range actions {
			act := act
			list = append(list, Permission{
				Negate:       false,
				ResourceType: k,
				ResourceID:   Wildcard,
				Action:       act,
			})
		}
	}
	return list
}
