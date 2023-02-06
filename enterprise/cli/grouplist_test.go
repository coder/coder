package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/cli"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestGroupList(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		})

		ctx, _ := testutil.Context(t)
		_, user1 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		// We intentionally create the first group as beta so that we
		// can assert that things are being sorted by name intentionally
		// and not by chance (or some other parameter like created_at).
		group1, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "beta",
		})
		require.NoError(t, err)

		group2, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "alpha",
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String()},
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group2.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String()},
		})
		require.NoError(t, err)

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(), "groups", "list")

		pty := ptytest.New(t)

		cmd.SetOut(pty.Output())
		clitest.SetupConfig(t, client, root)

		err = cmd.Execute()
		require.NoError(t, err)

		matches := []string{"NAME", "ORGANIZATION ID", "MEMBERS", " AVATAR URL",
			group2.Name, group2.OrganizationID.String(), user2.Email, group2.AvatarURL,
			group1.Name, group1.OrganizationID.String(), user1.Email, group1.AvatarURL,
		}

		for _, match := range matches {
			pty.ExpectMatch(match)
		}
	})

	t.Run("NoGroups", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		})

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(), "groups", "list")

		pty := ptytest.New(t)

		cmd.SetErr(pty.Output())
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.NoError(t, err)

		pty.ExpectMatch("No groups found")
		pty.ExpectMatch("coder groups create <name>")
	})
}
