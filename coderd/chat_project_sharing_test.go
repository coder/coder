package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectSharing(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	project := createChatProject(t, client, orgID, "Shared agent")
	require.Equal(t, codersdk.ChatProjectPermissions{Edit: true, Share: true, Delete: true, ManageTriggers: true}, project.Permissions)

	editorSDK, editorUser := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	editor := codersdk.NewExperimentalClient(editorSDK)
	userSDK, useUser := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	user := codersdk.NewExperimentalClient(userSDK)

	// Not shared yet: members cannot see it.
	_, err := user.GetChatProject(ctx, orgID, project.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())

	err = client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{
			editorUser.ID.String(): codersdk.ChatProjectRoleEdit,
			useUser.ID.String():    codersdk.ChatProjectRoleUse,
		},
	})
	require.NoError(t, err)

	projectACL, err := client.ChatProjectACL(ctx, orgID, project.ID)
	require.NoError(t, err)
	roles := map[string]codersdk.ChatProjectRole{}
	for _, entry := range projectACL.Users {
		roles[entry.ID.String()] = entry.Role
	}
	require.Equal(t, map[string]codersdk.ChatProjectRole{
		editorUser.ID.String(): codersdk.ChatProjectRoleEdit,
		useUser.ID.String():    codersdk.ChatProjectRoleUse,
	}, roles)

	// Shared agents are listed with the viewer's permissions.
	listed, err := user.ListChatProjects(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, project.ID, listed[0].ID)
	require.Equal(t, codersdk.ChatProjectPermissions{}, listed[0].Permissions)

	// Anyone the agent is shared with can start chats in it.
	_ = createChatModel(t, client)
	sharedChat := createChatInProject(t, user, orgID, &project.ID)
	require.Equal(t, &project.ID, sharedChat.ProjectID)

	editorView, err := editor.GetChatProject(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatProjectPermissions{Edit: true}, editorView.Permissions)

	// Users can read but not change the agent.
	name := "Renamed"
	_, err = user.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())

	// Editors can change settings but cannot share, delete, or manage
	// triggers.
	updated, err := editor.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, name, updated.Name)

	_, err = editor.ChatProjectACL(ctx, orgID, project.ID)
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	err = editor.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{useUser.ID.String(): codersdk.ChatProjectRoleEdit},
	})
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())

	_, err = editor.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:   "Deploys",
		Prompt: "Summarize the deploy.",
	})
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())

	ownerTrigger, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:   "Owner deploys",
		Prompt: "Summarize the deploy.",
	})
	require.NoError(t, err)
	prompt := "Do something else."
	_, err = editor.UpdateChatProjectTrigger(ctx, orgID, project.ID, ownerTrigger.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{Prompt: &prompt})
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	err = editor.DeleteChatProjectTrigger(ctx, orgID, project.ID, ownerTrigger.Trigger.ID)
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())

	err = editor.DeleteChatProject(ctx, orgID, project.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())

	// Owners cannot share with themselves, and roles are validated.
	err = client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleEdit},
	})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
	err = client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{useUser.ID.String(): "admin"},
	})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	// Removing a share revokes access.
	err = client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{useUser.ID.String(): codersdk.ChatProjectRoleDeleted},
	})
	require.NoError(t, err)
	_, err = user.GetChatProject(ctx, orgID, project.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	_, err = user.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		ProjectID:      &project.ID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "revoked"}},
	})
	requireChatProjectNotFound(t, err)

	available, err := client.ChatProjectACLAvailable(ctx, orgID, project.ID, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, available.Users)
}
