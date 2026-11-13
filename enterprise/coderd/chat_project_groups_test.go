package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// A chat owner who could only use a project through a group takes their
// chats out of the project when they leave the group or it is deleted.
func TestChatProjectGroupRemovalChatsLeaveProject(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		remove func(ctx context.Context, client *codersdk.Client, group codersdk.Group, user codersdk.User) error
	}{
		{"RemoveMember", func(ctx context.Context, client *codersdk.Client, group codersdk.Group, user codersdk.User) error {
			_, err := client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{RemoveUsers: []string{user.ID.String()}})
			return err
		}},
		{"DeleteGroup", func(ctx context.Context, client *codersdk.Client, group codersdk.Group, _ codersdk.User) error {
			return client.DeleteGroup(ctx, group.ID)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = []string{string(codersdk.ExperimentChatProjectSharing)}
			ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{DeploymentValues: dv, Database: db, Pubsub: ps},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
				},
			})
			owner := codersdk.NewExperimentalClient(ownerClient)
			orgID := firstUser.OrganizationID
			_, member := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
			//nolint:gocritic // The owner sets up the group, project, and sharing.
			group, err := ownerClient.CreateGroup(ctx, orgID, codersdk.CreateGroupRequest{Name: "project-group"})
			require.NoError(t, err)
			group, err = ownerClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{AddUsers: []string{member.ID.String()}})
			require.NoError(t, err)

			project, err := owner.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{Name: "Group Project"})
			require.NoError(t, err)
			require.NoError(t, owner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
				GroupRoles: map[string]codersdk.ChatProjectRole{group.ID.String(): codersdk.ChatProjectRoleUse},
			}))

			_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
			modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "test-model", IsDefault: true, CompressionThreshold: 80})
			chatOf := func(ownerID uuid.UUID) database.Chat {
				return dbgen.Chat(t, db, database.Chat{
					OrganizationID:    orgID,
					OwnerID:           ownerID,
					LastModelConfigID: modelConfig.ID,
					ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
				})
			}
			ownerChat, memberChat := chatOf(firstUser.UserID), chatOf(member.ID)

			require.NoError(t, tc.remove(ctx, ownerClient, group, member))

			got, err := owner.GetChat(ctx, memberChat.ID)
			require.NoError(t, err)
			require.Nil(t, got.ProjectID)
			got, err = owner.GetChat(ctx, ownerChat.ID)
			require.NoError(t, err)
			require.Equal(t, &project.ID, got.ProjectID)
		})
	}
}
