package rbac

import "fmt"

type Permission struct {
	// Negate makes this a negative permission
	Negate       bool   `json:"negate"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Action       Action `json:"action"`
}

// Role is a set of permissions at multiple levels:
// - Site level permissions apply EVERYWHERE
// - Org level permissions apply to EVERYTHING in a given ORG
// - User level permissions are the lowest
// In most cases, you will just want to use the pre-defined roles
// below.
type Role struct {
	Name string       `json:"name"`
	Site []Permission `json:"site"`
	// Org is a map of orgid to permissions. We represent orgid as a string.
	// TODO: Maybe switch to uuid, but tokens might need to support a "wildcard" org
	//		which could be a special uuid (like all 0s?)
	Org  map[string][]Permission `json:"org"`
	User []Permission            `json:"user"`
}

// Roles are stored as structs, so they can be serialized and stored. Until we store them elsewhere,
// const's will do just fine.
var (
	// RoleAdmin is a role that allows everything everywhere.
	RoleAdmin = Role{
		Name: "admin",
		Site: permissions(map[Object][]Action{
			ResourceWildcard: {WildcardSymbol},
		}),
	}

	// RoleMember is a role that allows access to user-level resources.
	RoleMember = Role{
		Name: "member",
		User: permissions(map[Object][]Action{
			ResourceWildcard: {WildcardSymbol},
		}),
	}

	// RoleAuditor is an example on how to give more precise permissions
	RoleAuditor = Role{
		Name: "auditor",
		Site: permissions(map[Object][]Action{
			// TODO: @emyrk when audit logs are added, add back a read perm
			//ResourceAuditLogs: {ActionRead},
			// Should be able to read user details to associate with logs.
			// Without this the user-id in logs is not very helpful
			ResourceWorkspace: {ActionRead},
		}),
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
		// This is at the site level to prevent the token from losing access if the user
		// is kicked from the org
		Site: []Permission{
			{
				Negate:       false,
				ResourceType: ResourceWorkspace.Type,
				ResourceID:   workspaceID,
				Action:       ActionRead,
			},
		},
	}
}

func permissions(perms map[Object][]Action) []Permission {
	list := make([]Permission, 0, len(perms))
	for k, actions := range perms {
		for _, act := range actions {
			act := act
			list = append(list, Permission{
				Negate:       false,
				ResourceType: k.Type,
				ResourceID:   WildcardSymbol,
				Action:       act,
			})
		}
	}
	return list
}
