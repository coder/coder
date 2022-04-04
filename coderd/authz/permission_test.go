package authz_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz"
	"github.com/stretchr/testify/require"
)

func Test_PermissionString(t *testing.T) {
	t.Parallel()

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
