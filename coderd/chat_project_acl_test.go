package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestChatProjectSharing(t *testing.T) {
	t.Parallel()

	t.Run("User", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMock()
		client, _ := newChatProjectClientWithOptions(t, func(opts *coderdtest.Options) {
			opts.Auditor = mAudit
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Shared Project")
		memory, err := client.CreateChatProjectMemory(ctx, project.OrganizationID, project.ID, codersdk.CreateChatProjectMemoryRequest{
			Name:        "shared-notes",
			Description: "Notes for sharees",
			Body:        "Sharees read and write project memories.",
		})
		require.NoError(t, err)

		sharee, shareeUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		_, err = sharee.GetChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)

		mAudit.ResetLogs()
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{shareeUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		require.True(t, mAudit.Contains(t, database.AuditLog{
			Action:       database.AuditActionWrite,
			ResourceType: database.ResourceTypeChatProject,
			ResourceID:   project.ID,
			UserID:       firstUser.UserID,
		}))

		acl, err := client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Groups)
		require.Len(t, acl.Users, 1)
		require.Equal(t, shareeUser.ID, acl.Users[0].ID)
		require.Equal(t, codersdk.ChatProjectRoleUse, acl.Users[0].Role)

		// A use sharee sees the project and its memories, starts chats in
		// it, and can see who else it is shared with.
		requireChatProjectListed(t, sharee, project.ID)
		_, err = sharee.GetChatProject(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		memories, err := sharee.ListChatProjectMemories(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Len(t, memories, 1)
		require.Equal(t, memory.ID, memories[0].ID)
		_, err = sharee.CreateChatProjectMemory(ctx, project.OrganizationID, project.ID, codersdk.CreateChatProjectMemoryRequest{
			Name:        "sharee-notes",
			Description: "Written by a sharee",
			Body:        "Sharee memory.",
		})
		require.NoError(t, err)
		shareeChat := createChatInProject(t, sharee, project.OrganizationID, &project.ID)
		require.Equal(t, &project.ID, shareeChat.ProjectID)
		_, err = sharee.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)

		// Sharing the project does not share the chats in it.
		ownerChat := createChatInProject(t, client, project.OrganizationID, &project.ID)
		_, err = sharee.GetChat(ctx, ownerChat.ID)
		requireSDKError(t, err, http.StatusNotFound)

		// Use does not grant editing, sharing, or deletion.
		name := "Renamed by sharee"
		_, err = sharee.UpdateChatProject(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
		requireSDKError(t, err, http.StatusNotFound)
		err = sharee.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusForbidden)
		err = sharee.DeleteChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)

		// Removing the entry revokes access.
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{shareeUser.ID.String(): codersdk.ChatProjectRoleDeleted},
		})
		require.NoError(t, err)
		_, err = sharee.GetChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)
		projects, err := sharee.ListChatProjects(ctx)
		require.NoError(t, err)
		require.Empty(t, projects)
		_, err = sharee.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: project.OrganizationID,
			ProjectID:      &project.ID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "revoked"}},
		})
		requireChatProjectNotFound(t, err)
	})

	t.Run("Admin", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Admin Project")
		admin, adminUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		_, otherUser := newChatProjectMember(t, client, firstUser.OrganizationID)

		err := client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{adminUser.ID.String(): codersdk.ChatProjectRoleAdmin},
		})
		require.NoError(t, err)

		name := "Renamed by admin"
		updated, err := admin.UpdateChatProject(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
		require.NoError(t, err)
		require.Equal(t, name, updated.Name)
		// Renaming keeps the ACL.
		acl, err := client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, codersdk.ChatProjectRoleAdmin, acl.Users[0].Role)

		err = admin.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{otherUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		err = admin.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{adminUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot change your own project sharing role.", sdkErr.Message)

		// Only the owner deletes a project.
		err = admin.DeleteChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("GroupAndEveryone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		groupProject := createChatProject(t, client, firstUser.OrganizationID, "Group Project")
		orgProject := createChatProject(t, client, firstUser.OrganizationID, "Organization Project")

		groupMember, groupMemberUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		nonMember, _ := newChatProjectMember(t, client, firstUser.OrganizationID)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: groupMemberUser.ID})

		err := client.UpdateChatProjectACL(ctx, groupProject.OrganizationID, groupProject.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{group.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		// The Everyone group's ID is the organization ID.
		err = client.UpdateChatProjectACL(ctx, orgProject.OrganizationID, orgProject.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{firstUser.OrganizationID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)

		acl, err := client.ChatProjectACL(ctx, orgProject.OrganizationID, orgProject.ID)
		require.NoError(t, err)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, firstUser.OrganizationID, acl.Groups[0].ID)
		require.Equal(t, database.EveryoneGroup, acl.Groups[0].Name)
		require.Equal(t, codersdk.ChatProjectRoleUse, acl.Groups[0].Role)

		projects, err := groupMember.ListChatProjects(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{groupProject.ID, orgProject.ID}, chatProjectIDs(projects))
		projects, err = nonMember.ListChatProjects(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{orgProject.ID}, chatProjectIDs(projects))
		_, err = nonMember.GetChatProject(ctx, groupProject.OrganizationID, groupProject.ID)
		requireSDKError(t, err, http.StatusNotFound)

		// Members of another organization do not reach the Everyone grant.
		otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
		outsiderRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrganization.ID)
		outsider := codersdk.NewExperimentalClient(outsiderRaw)
		projects, err = outsider.ListChatProjects(ctx)
		require.NoError(t, err)
		require.Empty(t, projects)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Validated Project")
		_, member := newChatProjectMember(t, client, firstUser.OrganizationID)

		err := client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{member.ID.String(): "owner"},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{uuid.NewString(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
		foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrganization.ID})
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{foreignGroup.ID.String(): codersdk.ChatProjectRoleUse},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "group_roles", sdkErr.Validations[0].Field)

		acl, err := client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
		require.Empty(t, acl.Groups)
	})
}

func newChatProjectClientWithOptions(t testing.TB, override func(*coderdtest.Options)) (*codersdk.ExperimentalClient, database.Store) {
	t.Helper()
	return newChatClientWithDatabase(t,
		func(options *coderdtest.Options) {
			options.DeploymentValues.Experiments = serpent.StringArray{
				string(codersdk.ExperimentChatProjects),
			}
		},
		withChatWorkerDisabled,
		override,
	)
}

func newChatProjectMember(t testing.TB, client *codersdk.ExperimentalClient, organizationID uuid.UUID) (*codersdk.ExperimentalClient, codersdk.User) {
	t.Helper()
	raw, user := coderdtest.CreateAnotherUser(t, client.Client, organizationID)
	return codersdk.NewExperimentalClient(raw), user
}

func requireChatProjectListed(t testing.TB, client *codersdk.ExperimentalClient, projectID uuid.UUID) {
	t.Helper()
	projects, err := client.ListChatProjects(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err)
	require.Contains(t, chatProjectIDs(projects), projectID)
}

func chatProjectIDs(projects []codersdk.ChatProject) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	return ids
}
