package coderd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatModelConfigListReadContracts is a differential read probe for the
// M2 split: the management list (GetChatModelConfigs) must be the authorized
// SQL filter, so org-scoped and custom-site chat_model_config readers see
// exactly their authorized rows (own disabled included, nothing cross-org),
// and a site role holding only chat_model_config:read must not error.
func TestChatModelConfigListReadContracts(t *testing.T) {
	t.Parallel()

	// Setup context, used only before the parallel subtests.
	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, pubsub := dbtestutil.NewDB(t)
	client := newChatClient(t, func(opts *coderdtest.Options) {
		opts.Database = rawDB
		opts.Pubsub = pubsub
	})
	_ = coderdtest.CreateFirstUser(t, client.Client)

	defaultOrg, err := rawDB.GetDefaultOrganization(ctx)
	require.NoError(t, err)
	otherOrg := dbgen.Organization(t, rawDB, database.Organization{IsDefault: false})
	dbgen.Group(t, rawDB, database.Group{ID: otherOrg.ID, Name: database.EveryoneGroup, OrganizationID: otherOrg.ID})

	// A disabled config in the default org, everyone-read ACL like M1 seeds.
	ownDisabled := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: defaultOrg.ID,
		Enabled:        false,
		GroupACL: database.ChatACL{
			defaultOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	})
	// An enabled config in the other org; org-scoped readers of the default
	// org must not see it.
	otherEnabled := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: otherOrg.ID,
		Enabled:        true,
		GroupACL: database.ChatACL{
			otherOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	})

	contains := func(configs []codersdk.ChatModel, id uuid.UUID) bool {
		for _, c := range configs {
			if c.ID == id {
				return true
			}
		}
		return false
	}

	t.Run("OrgAdminSeesOwnDisabledNotCrossOrg", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		orgAdminClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.ScopedRoleOrgAdmin(defaultOrg.ID))
		orgAdminClient := codersdk.NewExperimentalClient(orgAdminClientRaw)
		configs, err := orgAdminClient.ChatModels(ctx, defaultOrg.ID)
		require.NoError(t, err)
		require.True(t, contains(configs.Models, ownDisabled.ID), "org admin must see own disabled config")
		require.False(t, contains(configs.Models, otherEnabled.ID), "org admin must not see other org's config")
	})

	t.Run("OrgAuditorSeesOwnDisabledNotCrossOrg", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		orgAuditorClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.ScopedRoleOrgAuditor(defaultOrg.ID))
		orgAuditorClient := codersdk.NewExperimentalClient(orgAuditorClientRaw)
		configs, err := orgAuditorClient.ChatModels(ctx, defaultOrg.ID)
		require.NoError(t, err)
		require.True(t, contains(configs.Models, ownDisabled.ID), "org auditor must see own disabled config")
		require.False(t, contains(configs.Models, otherEnabled.ID), "org auditor must not see other org's config")
	})

	t.Run("CustomSiteReadRoleSeesAuthorizedRows", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		role, err := rawDB.InsertCustomRole(ctx, database.InsertCustomRoleParams{
			Name:        testutil.GetRandomName(t),
			DisplayName: "Chat Model Config Reader",
			OrganizationID: uuid.NullUUID{
				UUID:  defaultOrg.ID,
				Valid: true,
			},
			SitePermissions: database.CustomRolePermissions{
				{
					ResourceType: rbac.ResourceChatModelConfig.Type,
					Action:       policy.ActionRead,
				},
			},
		})
		require.NoError(t, err)

		readerClientRaw, readerUser := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID)
		_, err = client.Client.UpdateOrganizationMemberRoles(
			ctx,
			defaultOrg.ID,
			readerUser.ID.String(),
			codersdk.UpdateRoles{Roles: []string{role.Name}},
		)
		require.NoError(t, err)
		readerClient := codersdk.NewExperimentalClient(readerClientRaw)

		configs, err := readerClient.ChatModels(ctx, defaultOrg.ID)
		require.NoError(t, err, "chat_model_config:read site role must not 500")
		// The list is org-scoped; the probe's contract is that the request
		// uses the authorized list (no 500, own disabled visible).
		require.True(t, contains(configs.Models, ownDisabled.ID), "site reader must see own disabled config")
	})
}
