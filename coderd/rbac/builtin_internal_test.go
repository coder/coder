package rbac

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
)

func TestRoleByName(t *testing.T) {
	t.Parallel()

	t.Run("BuiltIns", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			Role Role
		}{
			{Role: builtInRoles[admin]("")},
			{Role: builtInRoles[member]("")},
			{Role: builtInRoles[auditor]("")},

			{Role: builtInRoles[orgAdmin](uuid.New().String())},
			{Role: builtInRoles[orgAdmin](uuid.New().String())},
			{Role: builtInRoles[orgAdmin](uuid.New().String())},

			{Role: builtInRoles[orgMember](uuid.New().String())},
			{Role: builtInRoles[orgMember](uuid.New().String())},
			{Role: builtInRoles[orgMember](uuid.New().String())},
		}

		for _, c := range testCases {
			c := c
			t.Run(c.Role.Name, func(t *testing.T) {
				role, err := RoleByName(c.Role.Name)
				require.NoError(t, err, "role exists")
				require.Equal(t, c.Role, role)
			})
		}
	})

	// nolint:paralleltest
	t.Run("Errors", func(t *testing.T) {
		var err error

		_, err = RoleByName("")
		require.Error(t, err, "empty role")

		_, err = RoleByName("too:many:colons")
		require.Error(t, err, "too many colons")

		_, err = RoleByName(orgMember)
		require.Error(t, err, "expect orgID")
	})
}
