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
				Action:       authz.ActionCreate,
			},
			Expected: "-user.devurl.1234.create",
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
			perm, err := authz.ParsePermission(c.Expected)
			require.NoError(t, err, "parse perm string")
			require.Equal(t, c.Permission, perm, "expected perm")

			perms, err := authz.ParsePermissions(c.Expected)
			require.NoError(t, err, "parse perms string")
			require.Equal(t, c.Permission, perms[0], "expected perm")
			require.Len(t, perms, 1, "expect 1 perm")
		})
	}
}

func Test_ParsePermissions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name        string
		Str         string
		Permissions []authz.Permission
		ErrStr      string
	}{
		{
			Name:   "NoSign",
			Str:    "site.*.*.*",
			ErrStr: "sign must be +/-",
		},
		{
			Name:   "BadLevel",
			Str:    "+unknown.*.*.*",
			ErrStr: "unsupported level",
		},
		{
			Name:   "NotEnoughParts",
			Str:    "+*.*.*",
			ErrStr: "permission expects 4 parts",
		},
		{
			Name:   "ShortLevel",
			Str:    "*.*.*.*",
			ErrStr: "permission level is too short",
		},
		{
			Name:   "BadLevelID",
			Str:    "org:1234:extra.*.*.*",
			ErrStr: "unsupported level format",
		},
		{
			Name: "GoodSet",
			Str:  "+org:1234.workspace.5678.read, -site.*.*.create",
			Permissions: []authz.Permission{
				{
					Sign:         true,
					Level:        "org",
					LevelID:      "1234",
					ResourceType: authz.ResourceWorkspace,
					ResourceID:   "5678",
					Action:       authz.ActionRead,
				},
				{
					Sign:         false,
					Level:        "site",
					LevelID:      "",
					ResourceType: "*",
					ResourceID:   "*",
					Action:       authz.ActionCreate,
				},
			},
		},
	}
	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			perms, err := authz.ParsePermissions(c.Str)
			if c.ErrStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.ErrStr, "exp error")
			} else {
				require.NoError(t, err, "parse error")
				require.Equal(t, c.Permissions, perms, "exp perms")
			}
		})
	}
}
