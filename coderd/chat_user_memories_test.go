package coderd_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatUserMemoriesCRUD(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	created, err := client.CreateChatUserMemory(ctx, codersdk.CreateChatUserMemoryRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           "release-process",
		Description:    "Release process notes",
		Body:           "Use the release checklist before tagging.",
	})
	require.NoError(t, err)
	require.Equal(t, firstUser.OrganizationID, created.OrganizationID)
	require.Equal(t, firstUser.UserID, created.UserID)
	require.NotEmpty(t, created.CreatedByUsername)

	memories, err := client.ListChatUserMemories(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, created.ID, memories[0].ID)
	require.Equal(t, created.CreatedByUsername, memories[0].CreatedByUsername)

	got, err := client.GetChatUserMemory(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Body, got.Body)
	require.Equal(t, created.CreatedByUsername, got.CreatedByUsername)

	updatedDescription := "Updated release notes"
	updated, err := client.UpdateChatUserMemory(ctx, created.ID, codersdk.UpdateChatUserMemoryRequest{
		Description: &updatedDescription,
	})
	require.NoError(t, err)
	require.Equal(t, updatedDescription, updated.Description)
	require.Equal(t, created.CreatedByUsername, updated.CreatedByUsername)

	_, err = client.CreateChatUserMemory(ctx, codersdk.CreateChatUserMemoryRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           "release-process",
		Description:    "Duplicate",
		Body:           "Duplicate body.",
	})
	require.Equal(t, 409, coderdtest.SDKError(t, err).StatusCode())

	require.NoError(t, client.DeleteChatUserMemory(ctx, created.ID))
	_, err = client.GetChatUserMemory(ctx, created.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
}

func TestChatUserMemoriesValidationAndScope(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	_, err := client.CreateChatUserMemory(ctx, codersdk.CreateChatUserMemoryRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           "not valid",
		Description:    "Description",
		Body:           "Body",
	})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())

	_, err = client.CreateChatUserMemory(ctx, codersdk.CreateChatUserMemoryRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           "valid-name",
		Description:    strings.Repeat("a", chattool.MaxMemoryDescriptionChars+1),
		Body:           "Body",
	})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())

	mine, err := client.CreateChatUserMemory(ctx, codersdk.CreateChatUserMemoryRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           "mine",
		Description:    "My memory",
		Body:           "My durable memory.",
	})
	require.NoError(t, err)

	memberRaw, memberUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberRaw)
	dbgen.ChatUserMemory(t, db, database.ChatUserMemory{
		OrganizationID: firstUser.OrganizationID,
		UserID:         memberUser.ID,
		Name:           "member-memory",
		Description:    "Member memory",
		Body:           "A different user's memory.",
	})

	memories, err := client.ListChatUserMemories(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, mine.ID, memories[0].ID)

	otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
	dbgen.ChatUserMemory(t, db, database.ChatUserMemory{
		OrganizationID: otherOrganization.ID,
		UserID:         firstUser.UserID,
		Name:           "other-organization",
		Description:    "Other organization memory",
		Body:           "This must not be listed.",
	})
	memories, err = client.ListChatUserMemories(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, mine.ID, memories[0].ID)

	_, err = member.GetChatUserMemory(ctx, mine.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
	name := "not-allowed"
	_, err = member.UpdateChatUserMemory(ctx, mine.ID, codersdk.UpdateChatUserMemoryRequest{Name: &name})
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
	err = member.DeleteChatUserMemory(ctx, mine.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	adminRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
	admin := codersdk.NewExperimentalClient(adminRaw)
	_, err = admin.GetChatUserMemory(ctx, mine.ID)
	require.NoError(t, err)
	adminDescription := "Updated by admin"
	_, err = admin.UpdateChatUserMemory(ctx, mine.ID, codersdk.UpdateChatUserMemoryRequest{Description: &adminDescription})
	require.NoError(t, err)
	require.NoError(t, admin.DeleteChatUserMemory(ctx, mine.ID))
}

func TestChatUserMemorySettings(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	_ = coderdtest.CreateFirstUser(t, client.Client)

	settings, err := client.ChatPersonalMemorySettings(ctx)
	require.NoError(t, err)
	require.True(t, settings.Enabled)

	settings, err = client.UpdateChatPersonalMemorySettings(ctx, codersdk.UpdateChatPersonalMemorySettingsRequest{Enabled: false})
	require.NoError(t, err)
	require.False(t, settings.Enabled)

	settings, err = client.ChatPersonalMemorySettings(ctx)
	require.NoError(t, err)
	require.False(t, settings.Enabled)
}

func TestChatUserMemoryCap(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	for i := 0; i < chattool.MaxMemories; i++ {
		dbgen.ChatUserMemory(t, db, database.ChatUserMemory{
			OrganizationID: firstUser.OrganizationID,
			UserID:         firstUser.UserID,
			Name:           "memory-" + uuid.NewString(),
			Description:    "Seeded memory",
			Body:           "Seeded durable memory.",
		})
	}

	_, err := client.CreateChatUserMemory(ctx, codersdk.CreateChatUserMemoryRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           "one-too-many",
		Description:    "Too many memories",
		Body:           "This should be rejected.",
	})
	require.Equal(t, 409, coderdtest.SDKError(t, err).StatusCode())
}
