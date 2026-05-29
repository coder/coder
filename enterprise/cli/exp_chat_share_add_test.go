package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestEnterpriseExpChatShareAdd(t *testing.T) {
	t.Parallel()

	t.Run("ShareWithGroupExplicitReadRole", func(t *testing.T) {
		t.Parallel()

		client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		group := coderdtest.CreateGroup(t, client, firstUser.OrganizationID, "chat-share-group")
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add group",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := newCLI(t, "exp", "chat", "share", "add", chat.ID.String(), "--group", group.Name+":read")
		clitest.SetupConfig(t, client, root)

		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := codersdk.NewExperimentalClient(client).GetChatACL(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, acl.Groups, 1)
		assert.Equal(t, group.ID, acl.Groups[0].ID)
		assert.Equal(t, group.Name, acl.Groups[0].Name)
		assert.Equal(t, codersdk.ChatRoleRead, acl.Groups[0].Role)
		assert.Contains(t, out.String(), group.Name)
		assert.Contains(t, out.String(), string(codersdk.ChatRoleRead))
	})

	t.Run("RejectsMissingGroupWithOrganizationID", func(t *testing.T) {
		t.Parallel()

		client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add missing group",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := newCLI(t, "exp", "chat", "share", "add", chat.ID.String(), "--group", "missing-group:read")
		clitest.SetupConfig(t, client, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find group named missing-group belonging to the organization")
		require.Contains(t, err.Error(), firstUser.OrganizationID.String())
	})
}
