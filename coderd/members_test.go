package coderd_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAddMember(t *testing.T) {
	t.Parallel()

	owner := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, owner)
	_, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

	t.Run("AlreadyMember", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		// Add user to org, even though they already exist
		// nolint:gocritic // must be an owner to see the user
		_, err := owner.PostOrganizationMember(ctx, first.OrganizationID, user.Username)
		require.ErrorContains(t, err, "already an organization member")

		org, err := owner.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)

		member, err := owner.OrganizationMember(ctx, org.Name, user.Username)
		require.NoError(t, err)
		require.Equal(t, member.UserID, user.ID)
	})

	t.Run("Me", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		member, err := owner.OrganizationMember(ctx, first.OrganizationID.String(), codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, member.UserID, first.UserID)
	})
}

func TestDeleteMember(t *testing.T) {
	t.Parallel()

	t.Run("Allowed", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)
		_, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		// Deleting members from the default org is not allowed.
		// If this behavior changes, and we allow deleting members from the default org,
		// this test should be updated to check there is no error.
		// nolint:gocritic // must be an owner to see the user
		err := owner.DeleteOrganizationMember(ctx, first.OrganizationID, user.Username)
		require.NoError(t, err)
	})
}

func TestListMembers(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	_, orgMember := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	_, orgAdmin := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	anotherOrg := dbgen.Organization(t, db, database.Organization{})
	anotherUser := dbgen.User(t, db, database.User{
		GithubComUserID: sql.NullInt64{Valid: true, Int64: 12345},
	})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: anotherOrg.ID,
		UserID:         anotherUser.ID,
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, owner.OrganizationID)
		require.NoError(t, err)
		require.Len(t, members, 3)
		require.ElementsMatch(t,
			[]uuid.UUID{owner.UserID, orgMember.ID, orgAdmin.ID},
			db2sdk.List(members, onlyIDs))
	})

	t.Run("UserID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, owner.OrganizationID, codersdk.OrganizationMembersQueryOptionUserID(orgMember.ID))
		require.NoError(t, err)
		require.Len(t, members, 1)
		require.ElementsMatch(t,
			[]uuid.UUID{orgMember.ID},
			db2sdk.List(members, onlyIDs))
	})

	t.Run("IncludeSystem", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, owner.OrganizationID, codersdk.OrganizationMembersQueryOptionIncludeSystem())
		require.NoError(t, err)
		require.Len(t, members, 4)
		require.ElementsMatch(t,
			[]uuid.UUID{owner.UserID, orgMember.ID, orgAdmin.ID, database.PrebuildsSystemUserID},
			db2sdk.List(members, onlyIDs))
	})

	t.Run("GithubUserID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, anotherOrg.ID, codersdk.OrganizationMembersQueryOptionGithubUserID(anotherUser.GithubComUserID.Int64))
		require.NoError(t, err)
		require.Len(t, members, 1)
		require.ElementsMatch(t,
			[]uuid.UUID{anotherUser.ID},
			db2sdk.List(members, onlyIDs))
	})
}

func onlyIDs(u codersdk.OrganizationMemberWithUserData) uuid.UUID {
	return u.UserID
}
