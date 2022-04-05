package authztest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
	"github.com/coder/coder/coderd/authz/authztest"
	crand "github.com/coder/coder/cryptorand"
)

func Test_Set(t *testing.T) {
	t.Parallel()

	t.Run("Simple", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 10; i++ {
			set := RandomSet(i)
			require.Equal(t, i, len(set), "set size")
			require.Equal(t, i, len(set.Permissions()), "set size")
			perms := set.Permissions()
			for i, p := range set {
				require.Equal(t, *p, perms[i])
			}
		}
	})

	t.Run("NilPerms", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 100; i++ {
			set := RandomSet(i)
			// Set some nils
			nilCount := 0
			for i := 0; i < len(set); i++ {
				if must(crand.Bool()) {
					set[i] = nil
					nilCount++
				}
			}
			require.Equal(t, i-nilCount, len(set.Permissions()))
		}
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()

		set := authztest.Set{
			&authz.Permission{
				Negate:       false,
				Level:        authz.LevelOrg,
				LevelID:      "1234",
				ResourceType: authz.ResourceWorkspace,
				ResourceID:   "1234",
				Action:       authz.ActionRead,
			},
			nil,
			&authz.Permission{
				Negate:       true,
				Level:        authz.LevelSite,
				LevelID:      "",
				ResourceType: authz.ResourceWorkspace,
				ResourceID:   "*",
				Action:       authz.ActionRead,
			},
		}

		require.Equal(t,
			"+org:1234.workspace.1234.read, -site.workspace.*.read",
			set.String(), "exp string")
	})
}
