package coderd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAddMember(t *testing.T) {
	t.Parallel()

	t.Run("AlreadyMember", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)
		_, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		// Add user to org, even though they already exist
		// nolint:gocritic // must be an owner to see the user
		_, err := owner.PostOrganizationMember(ctx, first.OrganizationID, user.Username)
		require.ErrorContains(t, err, "already exists")
	})
}

func TestListMembers(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)

		client, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Len(t, members, 2)
		require.ElementsMatch(t,
			[]uuid.UUID{first.UserID, user.ID},
			db2sdk.List(members, onlyIDs))
	})
}

func TestRemoveMember(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)
		orgAdminClient, orgAdmin := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		_, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		// Verify the org of 3 members
		members, err := orgAdminClient.OrganizationMembers(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Len(t, members, 3)
		require.ElementsMatch(t,
			[]uuid.UUID{first.UserID, user.ID, orgAdmin.ID},
			db2sdk.List(members, onlyIDs))

		// Delete a member
		err = orgAdminClient.DeleteOrganizationMember(ctx, first.OrganizationID, user.Username)
		require.NoError(t, err)

		members, err = orgAdminClient.OrganizationMembers(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Len(t, members, 2)
		require.ElementsMatch(t,
			[]uuid.UUID{first.UserID, orgAdmin.ID},
			db2sdk.List(members, onlyIDs))
	})
}

func onlyIDs(u codersdk.OrganizationMemberWithUserData) uuid.UUID {
	return u.UserID
}
