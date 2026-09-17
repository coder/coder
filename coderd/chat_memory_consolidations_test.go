package coderd_test

import (
	"testing"

	"github.com/google/uuid"
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

	projectRecords, err := client.ListChatProjectMemoryConsolidations(ctx, project.ID)
	require.NoError(t, err)
	require.Empty(t, projectRecords)
	personalRecords, err := client.ListChatUserMemoryConsolidations(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, personalRecords)

	dbgen.ChatMemoryConsolidation(t, db, database.ChatMemoryConsolidation{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      uuidNull(project.ID),
		Model:          "test-model",
		MemoriesBefore: 2,
	})
	dbgen.ChatMemoryConsolidation(t, db, database.ChatMemoryConsolidation{
		OrganizationID: firstUser.OrganizationID,
		UserID:         uuidNull(firstUser.UserID),
		Model:          "test-model",
		MemoriesBefore: 2,
	})

	projectRecords, err = client.ListChatProjectMemoryConsolidations(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, projectRecords, 1)
	require.Equal(t, codersdk.ChatMemoryConsolidationStatusRunning, projectRecords[0].Status)
	require.Equal(t, project.ID, *projectRecords[0].ProjectID)
	// A run without mutations serializes an empty array, never null.
	require.NotNil(t, projectRecords[0].Mutations)
	require.Empty(t, projectRecords[0].Mutations)

	personalRecords, err = client.ListChatUserMemoryConsolidations(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, personalRecords, 1)
	require.Equal(t, firstUser.UserID, *personalRecords[0].UserID)

	memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberRaw)
	projectRecords, err = member.ListChatProjectMemoryConsolidations(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, projectRecords, 1)
	personalRecords, err = member.ListChatUserMemoryConsolidations(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, personalRecords)
}

func uuidNull(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: true}
}
