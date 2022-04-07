package authz

type Role []Permission

var (
	RoleAllowAll = Role{
		{
			Negate:         false,
			OrganizationID: "*",
			ResourceType:   "*",
			ResourceID:     "*",
			Action:         "*",
		},
	}

	RoleReadOnly = Role{
		{
			Negate:         false,
			OrganizationID: "*",
			ResourceType:   "*",
			ResourceID:     "*",
			Action:         ActionRead,
		},
	}

	RoleNoPerm = Role{}
)
