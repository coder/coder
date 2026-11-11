package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatProjectArchive archives a project with chats of its owner and of
// a sharee, and unarchives it. Archiving needs the delete permission, so project ACL grants do not
// allow it.
func TestChatProjectArchive(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	siteOwner, api := newChatProjectClientWithAPI(t)
	firstUser := coderdtest.CreateFirstUser(t, siteOwner.Client)
	_ = createChatModel(t, siteOwner)
	orgID := firstUser.OrganizationID

	newMember := func(roles ...rbac.RoleIdentifier) (*codersdk.ExperimentalClient, codersdk.User) {
		raw, user := coderdtest.CreateAnotherUser(t, siteOwner.Client, orgID, roles...)
		return codersdk.NewExperimentalClient(raw), user
	}
	owner, _ := newMember()
	orgAdmin, _ := newMember(rbac.ScopedRoleOrgAdmin(orgID))
	sharedAdmin, sharedAdminUser := newMember()
	sharedUse, sharedUseUser := newMember()

	project := createChatProject(t, owner, orgID, "Archive me")
	require.NoError(t, owner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			sharedAdminUser.ID.String(): codersdk.ChatProjectRoleAdmin,
			sharedUseUser.ID.String():   codersdk.ChatProjectRoleUse,
		},
	}))

	// The owner's chat is running, with an idle subagent chat. The sharee's
	// chat is idle. An archived chat stays archived after the unarchive.
	running := createChatInProject(t, owner, orgID, &project.ID)
	subagent := createSubagentChat(t, api, running, database.ChatStatusWaiting)
	shareeChat := createChatInProject(t, sharedUse, orgID, &project.ID)
	driveChatToWaiting(ctx, t, api, shareeChat.ID)
	archivedEarlier := createChatInProject(t, owner, orgID, &project.ID)
	driveChatToWaiting(ctx, t, api, archivedEarlier.ID)
	require.NoError(t, owner.UpdateChat(ctx, archivedEarlier.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)}))

	for _, sharee := range []*codersdk.ExperimentalClient{sharedAdmin, sharedUse} {
		got, err := sharee.GetChatProject(ctx, orgID, project.ID)
		require.NoError(t, err)
		require.False(t, got.Permissions.Archive)
		_, err = sharee.ChatProjectArchivePreview(ctx, orgID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)
		_, err = sharee.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
		requireSDKError(t, err, http.StatusForbidden)
	}
	require.True(t, project.Permissions.Archive)

	preview, err := owner.ChatProjectArchivePreview(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatProjectArchivePreview{ChatCount: 2, ActiveChatCount: 1}, preview)

	_, err = owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{})
	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Equal(t, "Chat project has 1 active chat. Set stop_active_chats to stop and archive it.", sdkErr.Message)
	// Requests without a body, or with an empty chunked body, are
	// refused like {}.
	archivePath := fmt.Sprintf("/api/experimental/organizations/%s/chats/projects/%s/archive", orgID, project.ID)
	for name, body := range map[string]any{
		"none": nil,
		// A reader of unknown length makes the client send a chunked body.
		"empty chunked": io.MultiReader(),
	} {
		res, err := owner.Request(ctx, http.MethodPost, archivePath, body)
		require.NoError(t, err, name)
		requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusConflict)
		_ = res.Body.Close()
	}
	requireChatArchived(ctx, t, owner, running.ID, false)

	archived, err := owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt)
	requireChatArchived(ctx, t, owner, running.ID, true)
	requireChatArchived(ctx, t, owner, subagent.ID, true)
	requireChatArchived(ctx, t, sharedUse, shareeChat.ID, true)
	_, err = owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
	requireSDKError(t, err, http.StatusConflict)

	// Unarchiving needs the delete permission too, so project ACL grants
	// do not allow it. An org admin can unarchive; only chats archived
	// with the project come back.
	for _, sharee := range []*codersdk.ExperimentalClient{sharedAdmin, sharedUse} {
		_, err = sharee.UnarchiveChatProject(ctx, orgID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)
	}
	unarchived, err := orgAdmin.UnarchiveChatProject(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Nil(t, unarchived.ArchivedAt)
	requireChatArchived(ctx, t, owner, running.ID, false)
	requireChatArchived(ctx, t, owner, subagent.ID, false)
	requireChatArchived(ctx, t, sharedUse, shareeChat.ID, false)
	requireChatArchived(ctx, t, owner, archivedEarlier.ID, true)
	_, err = orgAdmin.UnarchiveChatProject(ctx, orgID, project.ID)
	requireSDKError(t, err, http.StatusConflict)

	// The site owner can archive too, and an archived project can still be
	// deleted, which detaches its chats and leaves them archived.
	_, err = siteOwner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
	require.NoError(t, err)
	require.NoError(t, owner.DeleteChatProject(ctx, orgID, project.ID))
	requireChatArchived(ctx, t, owner, running.ID, true)
	requireChatProjectID(ctx, t, owner, running.ID, nil)
}

func requireChatArchived(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, want bool) {
	t.Helper()
	chat, err := client.GetChat(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, want, chat.Archived)
}
