package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestGroupList(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleUserAdmin())

		_, user1 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		// We intentionally create the first group as beta so that we
		// can assert that things are being sorted by name intentionally
		// and not by chance (or some other parameter like created_at).
		group1 := coderdtest.CreateGroup(t, client, admin.OrganizationID, "beta", user1)
		group2 := coderdtest.CreateGroup(t, client, admin.OrganizationID, "alpha", user2)

		inv, conf := newCLI(t, "groups", "list")

		pty := ptytest.New(t)

		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, anotherClient, conf)

		err := inv.Run()
		require.NoError(t, err)

		matches := []string{
			"NAME", "ORGANIZATION ID", "MEMBERS", " AVATAR URL",
			group2.Name, group2.OrganizationID.String(), user2.Email, group2.AvatarURL,
			group1.Name, group1.OrganizationID.String(), user1.Email, group1.AvatarURL,
		}

		for _, match := range matches {
			pty.ExpectMatch(match)
		}
	})

	t.Run("Everyone", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleUserAdmin())

		inv, conf := newCLI(t, "groups", "list")

		pty := ptytest.New(t)

		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, anotherClient, conf)

		err := inv.Run()
		require.NoError(t, err)

		matches := []string{
			"NAME", "ORGANIZATION ID", "MEMBERS", " AVATAR URL",
			"Everyone", admin.OrganizationID.String(), coderdtest.FirstUserParams.Email, "",
		}

		for _, match := range matches {
			pty.ExpectMatch(match)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleUserAdmin())

		_, user1 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		group := coderdtest.CreateGroup(t, client, admin.OrganizationID, "alpha", user1)

		inv, conf := newCLI(t, "groups", "list", "-o", "json")
		clitest.SetupConfig(t, anotherClient, conf)

		buf := new(bytes.Buffer)
		inv.Stdout = buf

		err := inv.Run()
		require.NoError(t, err)

		var rows []codersdk.Group
		err = json.Unmarshal(buf.Bytes(), &rows)
		require.NoError(t, err, "unmarshal JSON output")

		require.Len(t, rows, 2, "expected Everyone group and alpha group")

		groupsByName := make(map[string]codersdk.Group)
		for _, g := range rows {
			groupsByName[g.Name] = g
		}

		// Verify the "Everyone" group.
		everyone, ok := groupsByName["Everyone"]
		require.True(t, ok, "expected Everyone group in JSON output")
		assert.Equal(t, admin.OrganizationID, everyone.ID, "Everyone group ID matches org ID")
		assert.Equal(t, admin.OrganizationID, everyone.OrganizationID)

		// Verify the created group.
		alpha, ok := groupsByName["alpha"]
		require.True(t, ok, "expected alpha group in JSON output")
		assert.Equal(t, group.ID, alpha.ID)
		assert.Equal(t, group.Name, alpha.Name)
		assert.Equal(t, group.DisplayName, alpha.DisplayName)
		assert.Equal(t, group.OrganizationID, alpha.OrganizationID)
		assert.Equal(t, group.AvatarURL, alpha.AvatarURL)
		assert.Equal(t, group.QuotaAllowance, alpha.QuotaAllowance)
		assert.Equal(t, group.Source, alpha.Source)
		require.Len(t, alpha.Members, 1)
		assert.Equal(t, user1.ID, alpha.Members[0].ID)
		assert.Equal(t, user1.Email, alpha.Members[0].Email)
	})
}
