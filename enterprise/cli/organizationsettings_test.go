package cli_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateGroupSync(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root := clitest.New(t, "organization", "settings", "set", "groupsync")
		//nolint:gocritic // Using the owner, testing the cli not perms
		clitest.SetupConfig(t, owner, root)

		expectedSettings := codersdk.GroupSyncSettings{
			Field: "groups",
			Mapping: map[string][]uuid.UUID{
				"test": {first.OrganizationID},
			},
			RegexFilter:       regexp.MustCompile("^foo"),
			AutoCreateMissing: true,
			LegacyNameMapping: nil,
		}
		expectedData, err := json.Marshal(expectedSettings)
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewBuffer(expectedData)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())

		// Now read it back
		inv, root = clitest.New(t, "organization", "settings", "show", "groupsync")
		//nolint:gocritic // Using the owner, testing the cli not perms
		clitest.SetupConfig(t, owner, root)

		buf = new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())
	})
}

func TestUpdateRoleSync(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root := clitest.New(t, "organization", "settings", "set", "rolesync")
		//nolint:gocritic // Using the owner, testing the cli not perms
		clitest.SetupConfig(t, owner, root)

		expectedSettings := codersdk.RoleSyncSettings{
			Field: "roles",
			Mapping: map[string][]string{
				"test": {rbac.RoleOrgAdmin()},
			},
		}
		expectedData, err := json.Marshal(expectedSettings)
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewBuffer(expectedData)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())

		// Now read it back
		inv, root = clitest.New(t, "organization", "settings", "show", "rolesync")
		//nolint:gocritic // Using the owner, testing the cli not perms
		clitest.SetupConfig(t, owner, root)

		buf = new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())
	})
}

func TestUpdateOrganizationSync(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root := clitest.New(t, "organization", "settings", "set", "organization-sync")
		//nolint:gocritic // Using the owner, testing the cli not perms
		clitest.SetupConfig(t, owner, root)

		expectedSettings := codersdk.OrganizationSyncSettings{
			Field: "organizations",
			Mapping: map[string][]uuid.UUID{
				"test": {uuid.New()},
			},
		}
		expectedData, err := json.Marshal(expectedSettings)
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewBuffer(expectedData)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())

		// Now read it back
		inv, root = clitest.New(t, "organization", "settings", "show", "organization-sync")
		//nolint:gocritic // Using the owner, testing the cli not perms
		clitest.SetupConfig(t, owner, root)

		buf = new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())
	})
}
