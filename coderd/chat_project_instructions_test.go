package coderd_test

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectInstructions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	owner, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	project := createChatProject(t, client, firstUser.OrganizationID, "Instructions Project")

	// A project without instructions reports the unset state.
	instructions, err := client.ChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatProjectInstructions{ProjectID: project.ID}, instructions)

	created, err := client.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		// Invisible characters are stripped before storage.
		Instructions: "Always\u200b reply in French.",
	})
	require.NoError(t, err)
	require.Equal(t, "Always reply in French.", created.Instructions)
	require.NotNil(t, created.UpdatedBy)
	require.Equal(t, owner.ID, created.UpdatedBy.ID)
	require.Equal(t, owner.Username, created.UpdatedBy.Username)
	require.NotNil(t, created.UpdatedAt)

	instructions, err = client.ChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Equal(t, created, instructions)

	// Another editor replaces the text and becomes the recorded editor.
	adminRaw, admin := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID,
		rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
	adminClient := codersdk.NewExperimentalClient(adminRaw)
	updated, err := adminClient.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		Instructions: "Always reply in German.",
	})
	require.NoError(t, err)
	require.Equal(t, "Always reply in German.", updated.Instructions)
	require.NotNil(t, updated.UpdatedBy)
	require.Equal(t, admin.ID, updated.UpdatedBy.ID)
	require.False(t, updated.UpdatedAt.Before(*created.UpdatedAt))

	// Clearing instructions is a delete, not a blank update.
	for _, blank := range []string{"", " \n\u200b "} {
		_, err = client.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
			Instructions: blank,
		})
		sdkErr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "Instructions must not be blank.", sdkErr.Message)
	}

	_, err = client.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		Instructions: strings.Repeat("x", codersdk.DefaultChatMaxPromptBytes+1),
	})
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, "Instructions exceed maximum length.", sdkErr.Message)

	// Rejected updates leave the stored instructions alone.
	instructions, err = client.ChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Equal(t, updated, instructions)

	// Users are soft deleted, so a deleted editor must be dropped explicitly
	// rather than reported by name.
	require.NoError(t, client.DeleteUser(ctx, admin.ID))
	instructions, err = client.ChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Nil(t, instructions.UpdatedBy)
	require.Equal(t, updated.Instructions, instructions.Instructions)
	require.Equal(t, updated.UpdatedAt, instructions.UpdatedAt)

	// A member who cannot read the project can neither see nor change its
	// instructions, and the response does not reveal that the project exists.
	memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberRaw)
	_, err = member.ChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	_, err = member.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		Instructions: "Hijacked.",
	})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	err = member.DeleteChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())

	require.NoError(t, client.DeleteChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID))
	instructions, err = client.ChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatProjectInstructions{ProjectID: project.ID}, instructions)
	// Deleting unset instructions succeeds.
	require.NoError(t, client.DeleteChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID))

	// Deleting a project with instructions succeeds and removes them.
	_, err = client.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		Instructions: "Always reply in Spanish.",
	})
	require.NoError(t, err)
	require.NoError(t, client.DeleteChatProject(ctx, firstUser.OrganizationID, project.ID))
	_, err = db.GetChatProjectInstructionsByProjectID(dbauthz.AsSystemRestricted(ctx), project.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
