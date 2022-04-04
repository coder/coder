package authz_test

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/coder/coder/coderd/authz"
	crand "github.com/coder/coder/cryptorand"
)

func Test_PermissionString(t *testing.T) {
	testCases := []struct {
		Name       string
		Permission authz.Permission
		Expected   string
	}{
		{
			Name: "BasicPositive",
			Permission: authz.Permission{
				Sign:         true,
				Level:        authz.LevelSite,
				LevelID:      "",
				ResourceType: authz.ResourceWorkspace,
				ResourceID:   "*",
				Action:       authz.ActionRead,
			},
			Expected: "+site.workspace.*.read",
		},
		{
			Name: "BasicNegative",
			Permission: authz.Permission{
				Sign:         false,
				Level:        authz.LevelUser,
				LevelID:      "",
				ResourceType: authz.ResourceDevURL,
				ResourceID:   "1234",
				Action:       authz.ActionWrite,
			},
			Expected: "-user.devurl.1234.write",
		},
		{
			Name: "OrgID",
			Permission: authz.Permission{
				Sign:         false,
				Level:        authz.LevelOrg,
				LevelID:      "default",
				ResourceType: authz.ResourceProject,
				ResourceID:   "456",
				Action:       authz.ActionModify,
			},
			Expected: "-org:default.project.456.modify",
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, c.Expected, c.Permission.String())
		})
	}

}

func BenchmarkPermissionString(b *testing.B) {
	total := 10000
	if b.N < total {
		total = b.N
	}
	perms := make([]authz.Permission, b.N)
	for n := 0; n < total; n++ {
		perms[n] = RandomPermission()
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var _ = perms[n%total].String()
	}
}

var resourceTypes = []authz.ResourceType{
	"project", "config", "user", "user_role",
	"workspace", "dev-url", "metric", "*",
}

var actions = []authz.Action{
	"read", "create", "delete", "modify", "*",
}

func RandomPermission() authz.Permission {
	n, _ := crand.Intn(len(authz.PermissionLevels))
	m, _ := crand.Intn(len(resourceTypes))
	a, _ := crand.Intn(len(actions))
	return authz.Permission{
		Sign:         n%2 == 0,
		Level:        authz.PermissionLevels[n],
		ResourceType: resourceTypes[m],
		ResourceID:   "*",
		Action:       actions[a],
	}
}
