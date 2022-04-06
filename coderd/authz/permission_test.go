package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authz"
)

func TestPermissionString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name       string
		Permission authz.Permission
		Expected   string
	}{
		{
			Name: "BasicPositive",
			Permission: authz.Permission{
				Negate:         false,
				Level:          authz.LevelSite,
				OrganizationID: "",
				ResourceType:   authz.ResourceWorkspace,
				ResourceID:     "*",
				Action:         authz.ActionRead,
			},
			Expected: "+site.workspace.*.read",
		},
		{
			Name: "BasicNegative",
			Permission: authz.Permission{
				Negate:         true,
				Level:          authz.LevelUser,
				OrganizationID: "",
				ResourceType:   authz.ResourceDevURL,
				ResourceID:     "1234",
				Action:         authz.ActionCreate,
			},
			Expected: "-user.devurl.1234.create",
		},
		{
			Name: "OrgID",
			Permission: authz.Permission{
				Negate:         true,
				Level:          authz.LevelOrg,
				OrganizationID: "default",
				ResourceType:   authz.ResourceProject,
				ResourceID:     "456",
				Action:         authz.ActionUpdate,
			},
			Expected: "-org:default.project.456.update",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

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

func TestParsePermissions(t *testing.T) {
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
					Negate:         false,
					Level:          "org",
					OrganizationID: "1234",
					ResourceType:   authz.ResourceWorkspace,
					ResourceID:     "5678",
					Action:         authz.ActionRead,
				},
				{
					Negate:         true,
					Level:          "site",
					OrganizationID: "",
					ResourceType:   "*",
					ResourceID:     "*",
					Action:         authz.ActionCreate,
				},
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
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
