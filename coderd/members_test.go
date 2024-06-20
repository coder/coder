package coderd_test

import (
	"net/http"
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

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)
		ctx := testutil.Context(t, testutil.WaitMedium)
		org, err := owner.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "other",
			DisplayName: "",
			Description: "",
			Icon:        "",
		})
		require.NoError(t, err)

		// Make a user not in the second organization
		_, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

		members, err := owner.OrganizationMembers(ctx, org.ID)
		require.NoError(t, err)
		require.Len(t, members, 1) // Verify just the 1 member

		// Add user to org
		_, err = owner.PostOrganizationMember(ctx, org.ID, user.Username)
		require.NoError(t, err)

		members, err = owner.OrganizationMembers(ctx, org.ID)
		require.NoError(t, err)
		// Owner + new member
		require.Len(t, members, 2)
		require.ElementsMatch(t,
			[]uuid.UUID{first.UserID, user.ID},
			db2sdk.List(members, onlyIDs))
	})

	t.Run("UserNotExists", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, owner)
		ctx := testutil.Context(t, testutil.WaitMedium)

		org, err := owner.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "other",
			DisplayName: "",
			Description: "",
			Icon:        "",
		})
		require.NoError(t, err)

		// Add user to org
		_, err = owner.PostOrganizationMember(ctx, org.ID, uuid.NewString())
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Contains(t, apiErr.Message, "must be an existing")
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

	// Calling it from a user without the org access.
	t.Run("NotInOrg", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)

		client, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitShort)
		org, err := owner.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "test",
			DisplayName: "",
			Description: "",
		})
		require.NoError(t, err, "create organization")

		// 404 error is expected instead of a 403/401 to not leak existence of
		// an organization.
		_, err = client.OrganizationMembers(ctx, org.ID)
		require.ErrorContains(t, err, "404")
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

	t.Run("MemberNotInOrg", func(t *testing.T) {
		t.Parallel()
		owner := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, owner)
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitMedium)
		// nolint:gocritic // requires owner to make a new org
		org, _ := owner.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "other",
			DisplayName: "",
			Description: "",
			Icon:        "",
		})

		_, user := coderdtest.CreateAnotherUser(t, owner, org.ID)

		// Delete a user that is not in the organization
		err := orgAdminClient.DeleteOrganizationMember(ctx, first.OrganizationID, user.Username)
		require.Error(t, err)
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusNotFound, apiError.StatusCode())
	})
}

func onlyIDs(u codersdk.OrganizationMemberWithName) uuid.UUID {
	return u.UserID
}
