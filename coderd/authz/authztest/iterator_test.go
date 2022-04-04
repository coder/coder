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
	t.Parallel()

	for i := 0; i < 100; i++ {
		allPerms := make(authztest.Set, 0)
		// 2 - 4 sets
		sets := make([]authztest.Set, 1+must(crand.Intn(2)))
		for j := range sets {
			sets[j] = RandomSet(1 + must(crand.Intn(4)))
			allPerms = append(allPerms, sets[j]...)
		}

		ui := authztest.Union(sets...).Iterator()
		require.Equal(t, len(allPerms), ui.Size(), "union set total")
		require.Equal(t, 1, ui.ReturnSize(), "union ret size is 1")
		for c := 0; ; c++ {
			require.Equal(t, 1, len(ui.Permissions()), "permissions size")
			require.Equal(t, allPerms[c], ui.Permissions()[0], "permission order")
			if !ui.Next() {
				break
			}
		}

		ui.Reset()
		// If the size is 1, next will always return false
		if ui.Size() > 1 {
			require.True(t, ui.Next(), "reset should make next true again")
		}
	}
}

func RandomSet(size int) authztest.Set {
	set := make(authztest.Set, 0, size)
	for i := 0; i < size; i++ {
		p := RandomPermission()
		set = append(set, &p)
	}
	return set
}

func RandomPermission() authz.Permission {
	actions := []authz.Action{
		authz.ActionRead,
		authz.ActionCreate,
		authz.ActionModify,
		authz.ActionDelete,
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
