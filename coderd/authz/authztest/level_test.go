package authztest_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz"
	"github.com/coder/coder/coderd/authz/authztest"
	"github.com/stretchr/testify/require"
)

func Test_GroupedPermissions(t *testing.T) {
	t.Parallel()

	set := make(authztest.Set, 0)
	var total int
	for _, lvl := range authz.PermissionLevels {
		for _, s := range []bool{true, false} {
			for _, a := range []authz.Action{authz.ActionRead, authztest.OtherOption} {
				if lvl == authz.LevelOrg {
					set = append(set, &authz.Permission{
						Sign:         s,
						Level:        lvl,
						LevelID:      "mem",
						ResourceType: authz.ResourceWorkspace,
						Action:       a,
					})
					total++
				}
				set = append(set, &authz.Permission{
					Sign:         s,
					Level:        lvl,
					ResourceType: authz.ResourceWorkspace,
					Action:       a,
				})
				total++
			}
		}
	}

	require.Equal(t, total, len(set), "total set size")
	grp := authztest.GroupedPermissions(set)
	grp.Org()

	cases := []struct {
		Name   string
		Lvl    authztest.LevelGroup
		ExpPos int
		ExpNeg int
		ExpAbs int
	}{
		{
			Name:   "Wild",
			Lvl:    grp.Wildcard(),
			ExpPos: 1, ExpNeg: 1, ExpAbs: 2,
		},
		{
			Name:   "Site",
			Lvl:    grp.Site(),
			ExpPos: 1, ExpNeg: 1, ExpAbs: 2,
		},
		{
			Name:   "Org",
			Lvl:    grp.Org(),
			ExpPos: 1, ExpNeg: 1, ExpAbs: 2,
		},
		{
			Name:   "Org:mem",
			Lvl:    grp.OrgMem(),
			ExpPos: 1, ExpNeg: 1, ExpAbs: 2,
		},
		{
			Name:   "Org:*",
			Lvl:    grp.AllOrgs(),
			ExpPos: 2, ExpNeg: 2, ExpAbs: 4,
		},
		{
			Name:   "User",
			Lvl:    grp.User(),
			ExpPos: 1, ExpNeg: 1, ExpAbs: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, c.ExpPos+c.ExpNeg+c.ExpAbs, len(c.Lvl.All()), "set size")
			require.Equal(t, c.ExpPos, len(c.Lvl.Positive()), "correct num pos")
			require.Equal(t, c.ExpNeg, len(c.Lvl.Negative()), "correct num neg")
			require.Equal(t, c.ExpAbs, len(c.Lvl.Abstain()), "correct num abs")
		})
	}
}
