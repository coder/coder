package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatMemoryConsolidationLists(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Consolidation Project")

	records, err := client.ListChatProjectMemoryConsolidations(ctx, project.ID)
	require.NoError(t, err)
	require.Empty(t, records)

	dbgen.ChatMemoryConsolidation(t, db, database.ChatMemoryConsolidation{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      project.ID,
		Model:          "test-model",
		MemoriesBefore: 2,
	})

	records, err = client.ListChatProjectMemoryConsolidations(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, codersdk.ChatMemoryConsolidationStatusRunning, records[0].Status)
	require.Equal(t, project.ID, records[0].ProjectID)
	// A run without mutations serializes an empty array, never null.
	require.NotNil(t, records[0].Mutations)
	require.Empty(t, records[0].Mutations)

	// The journal follows the project ACL like the memories it describes.
	memberRaw, memberUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberRaw)
	_, err = member.ListChatProjectMemoryConsolidations(ctx, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
	require.NoError(t, client.UpdateChatProjectACL(ctx, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{memberUser.ID.String(): codersdk.ChatProjectRoleRead},
	}))
	records, err = member.ListChatProjectMemoryConsolidations(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, records, 1)
}
