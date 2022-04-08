package authz

import "fmt"

type Role struct {
	Name string
	Site []Permission
	Org  map[string][]Permission
	User []Permission
}

var (
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

	RoleNoPerm = Role{}
)

func OrgAdmin(orgID string) Role {
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

func OrgMember(orgID string) Role {
	return Role{
		Name: "org-member-" + orgID,
		Org: map[string][]Permission{
			orgID: {},
		},
	}
}

func WorkspaceAgentRole(workspaceID string) Role {
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
