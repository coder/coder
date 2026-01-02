package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRoleFromJSON(t *testing.T) {
	t.Parallel()

	t.Run("SingleObject", func(t *testing.T) {
		t.Parallel()

		input := `{
			"name": "test-role",
			"organization_id": "550e8400-e29b-41d4-a716-446655440000",
			"display_name": "Test Role",
			"organization_permissions": [
				{"resource_type": "workspace", "action": "read"}
			]
		}`

		role, err := parseRoleFromJSON([]byte(input))
		require.NoError(t, err)
		require.Equal(t, "test-role", role.Name)
		require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", role.OrganizationID)
		require.Equal(t, "Test Role", role.DisplayName)
		require.Len(t, role.OrganizationPermissions, 1)
	})

	t.Run("ArrayWithSingleElement", func(t *testing.T) {
		t.Parallel()

		// This is the format exported by `coder organization roles show --output json`
		input := `[{
			"name": "test-role",
			"organization_id": "550e8400-e29b-41d4-a716-446655440000",
			"display_name": "Test Role",
			"site_permissions": [],
			"organization_permissions": [
				{"resource_type": "workspace", "action": "read"}
			],
			"user_permissions": [],
			"assignable": true,
			"built_in": false
		}]`

		role, err := parseRoleFromJSON([]byte(input))
		require.NoError(t, err)
		require.Equal(t, "test-role", role.Name)
		require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", role.OrganizationID)
		require.Equal(t, "Test Role", role.DisplayName)
		require.Len(t, role.OrganizationPermissions, 1)
	})

	t.Run("ArrayWithMultipleElements", func(t *testing.T) {
		t.Parallel()

		input := `[
			{"name": "role1", "organization_id": "550e8400-e29b-41d4-a716-446655440000"},
			{"name": "role2", "organization_id": "550e8400-e29b-41d4-a716-446655440000"}
		]`

		_, err := parseRoleFromJSON([]byte(input))
		require.Error(t, err)
		require.Contains(t, err.Error(), "only 1 role can be sent at a time")
	})

	t.Run("EmptyArray", func(t *testing.T) {
		t.Parallel()

		input := `[]`

		_, err := parseRoleFromJSON([]byte(input))
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not appear to be a valid role")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()

		input := `{invalid json`

		_, err := parseRoleFromJSON([]byte(input))
		require.Error(t, err)
	})

	t.Run("EmptyObject", func(t *testing.T) {
		t.Parallel()

		input := `{}`

		_, err := parseRoleFromJSON([]byte(input))
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not appear to be a valid role")
	})

	t.Run("ArrayWithEmptyObject", func(t *testing.T) {
		t.Parallel()

		input := `[{}]`

		_, err := parseRoleFromJSON([]byte(input))
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not appear to be a valid role")
	})
}
