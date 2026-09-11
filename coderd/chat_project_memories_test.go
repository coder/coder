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

func TestChatProjectMemoriesCRUD(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Memory Project")

	created, err := client.CreateChatProjectMemory(ctx, project.ID, codersdk.CreateChatProjectMemoryRequest{
		Type:        codersdk.ChatProjectMemoryTypeProject,
		Name:        "release-process",
		Description: "Release process notes",
		Body:        "Use the release checklist before tagging.",
	})
	require.NoError(t, err)
	require.Equal(t, project.ID, created.ProjectID)
	require.Equal(t, firstUser.UserID, created.CreatedBy)

	memories, err := client.ListChatProjectMemories(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, created.ID, memories[0].ID)

	got, err := client.GetChatProjectMemory(ctx, project.ID, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Body, got.Body)

	updatedDescription := "Updated release notes"
	updated, err := client.UpdateChatProjectMemory(ctx, project.ID, created.ID, codersdk.UpdateChatProjectMemoryRequest{
		Description: &updatedDescription,
	})
	require.NoError(t, err)
	require.Equal(t, updatedDescription, updated.Description)

	_, err = client.CreateChatProjectMemory(ctx, project.ID, codersdk.CreateChatProjectMemoryRequest{
		Type:        codersdk.ChatProjectMemoryTypeProject,
		Name:        "release-process",
		Description: "Duplicate",
		Body:        "Duplicate body.",
	})
	require.Equal(t, 409, coderdtest.SDKError(t, err).StatusCode())

	otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
	otherRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrganization.ID)
	other := codersdk.NewExperimentalClient(otherRaw)
	_, err = other.GetChatProjectMemory(ctx, project.ID, created.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberRaw)
	memberBody := "Members can collaboratively edit project memory."
	memberUpdated, err := member.UpdateChatProjectMemory(ctx, project.ID, created.ID, codersdk.UpdateChatProjectMemoryRequest{Body: &memberBody})
	require.NoError(t, err)
	require.Equal(t, memberBody, memberUpdated.Body)
	require.NoError(t, member.DeleteChatProjectMemory(ctx, project.ID, created.ID))
}

func TestChatProjectMemoryCap(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Capped Memory Project")

	for i := range 200 {
		dbgen.ChatProjectMemory(t, db, database.ChatProjectMemory{
			ProjectID:      project.ID,
			OrganizationID: firstUser.OrganizationID,
			CreatedBy:      firstUser.UserID,
			Name:           "memory-" + uuid.NewString() + string(rune('a'+i%26)),
			Type:           database.ChatProjectMemoryTypeProject,
			Description:    "Seeded memory",
			Body:           "Seeded durable memory.",
		})
	}

	_, err := client.CreateChatProjectMemory(ctx, project.ID, codersdk.CreateChatProjectMemoryRequest{
		Type:        codersdk.ChatProjectMemoryTypeProject,
		Name:        "one-too-many",
		Description: "Too many memories",
		Body:        "This should be rejected.",
	})
	require.Equal(t, 409, coderdtest.SDKError(t, err).StatusCode())
}
