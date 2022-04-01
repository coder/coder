package authztest_test

import (
	"testing"
	
	"github.com/coder/coder/coderd/authz"
	"github.com/coder/coder/coderd/authz/authztest"
	crand "github.com/coder/coder/cryptorand"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnion(t *testing.T) {
	for i := 0; i < 10; i++ {
		allPerms := make(authztest.Set, 0)
		// 2 - 4 sets
		sets := make([]authztest.Set, 2+must(crand.Intn(2)))
		for j := range sets {
			sets[j] = make(authztest.Set, 1+must(crand.Intn(4)))
			allPerms = append(allPerms, sets[j]...)
		}

		u := authztest.Union(sets...)
		require.Equal(t, len(allPerms), u.Size(), "union set total")
		require.Equal(t, 1, u.ReturnSize(), "union ret size is 1")
		for c := 0; ; c++ {
			require.Equal(t, allPerms[c], u.Permission(), "permission order")
			require.Equal(t, 1, len(u.Permissions()), "permissions size")
			require.Equal(t, allPerms[c], u.Permissions()[0], "permission order")
			if !u.Next() {
				break
			}
		}

		u.Reset()
		require.True(t, u.Next(), "reset should make next true again")
	}
}

func RandomPermission() authz.Permission {
	actions := []authz.Action{
		authz.ReadAction,
		authz.DeleteAction,
		authz.WriteAction,
		authz.ModifyAction,
	}
	return authz.Permission{
		Sign:         must(crand.Intn(2))%2 == 0,
		Level:        authz.PermissionLevels[must(crand.Intn(len(authz.PermissionLevels)))],
		LevelID:      uuid.New().String(),
		ResourceType: authz.ResourceWorkspace,
		ResourceID:   uuid.New().String(),
		Action:       actions[must(crand.Intn(len(actions)))],
	}
}

func must[r any](v r, err error) r {
	if err != nil {
		panic(err)
	}
	return v
}
