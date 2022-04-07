package authz

type Role []Permission

var (
	RoleAllowAll = Role{
		{
			Negate:       false,
			ResourceType: "*",
			ResourceID:   "*",
			Action:       "*",
		},
	}

	RoleReadOnly = Role{
		{
			Negate:       false,
			ResourceType: "*",
			ResourceID:   "*",
			Action:       ActionRead,
		},
	}

	RoleBlockAll = Role{
		{
			Negate:       true,
			ResourceType: "*",
			ResourceID:   "*",
			Action:       "*",
		},
	}

	RoleNoPerm = Role{}
)

func WorkspaceAgentRole(workspaceID string) Role {
	return Role{
		{
			Negate:       false,
			ResourceType: ResourceWorkspace,
			ResourceID:   workspaceID,
			Action:       ActionRead,
		},
	}
}
