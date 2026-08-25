package coderd_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
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

// TestMembersWithRetiredRole verifies that stale grants of retired built-in
// roles, which linger in the database until a cleanup migration lands, do
// not break membership management.
func TestMembersWithRetiredRole(t *testing.T) {
	t.Parallel()

	// The raw, unauthorized store is required to seed stale data without
	// tripping the assignment validation under test.
	db, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	ctx := testutil.Context(t, testutil.WaitMedium)

	// Seed stale data in the store: the retired agents-access role remains
	// in a member's role array and in the org's default member roles.
	_, err := db.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
		GrantedRoles: []string{"agents-access"},
		UserID:       member.ID,
		OrgID:        owner.OrganizationID,
	})
	require.NoError(t, err)

	org, err := db.GetOrganizationByID(ctx, owner.OrganizationID)
	require.NoError(t, err)
	updateOrg := database.UpdateOrganizationParams{
		ID:                    org.ID,
		UpdatedAt:             dbtime.Now(),
		Name:                  org.Name,
		DisplayName:           org.DisplayName,
		Description:           org.Description,
		Icon:                  org.Icon,
		DefaultOrgMemberRoles: append(org.DefaultOrgMemberRoles, "agents-access"),
	}
	_, err = db.UpdateOrganization(ctx, updateOrg)
	require.NoError(t, err)

	// Requests expand the member's stored roles, including the stale grant.
	_, err = memberClient.User(ctx, codersdk.Me)
	require.NoError(t, err)

	// User creation inserts an organization membership, which validates the
	// org's default member roles. The stale default must not fail it.
	coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Membership responses hide the stale grant so role editors do not
	// display or resubmit it.
	members, err := client.OrganizationMembers(ctx, owner.OrganizationID)
	require.NoError(t, err)
	for _, m := range members {
		if m.UserID != member.ID {
			continue
		}
		for _, role := range m.Roles {
			require.NotEqual(t, "agents-access", role.Name)
		}
	}

	// Organization responses hide the stale default role the same way.
	orgResp, err := client.Organization(ctx, owner.OrganizationID)
	require.NoError(t, err)
	require.NotContains(t, orgResp.DefaultOrgMemberRoles, "agents-access")

	// The user-roles endpoint hides the stale grant the same way.
	userRoles, err := memberClient.UserRoles(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotContains(t, userRoles.Roles, "agents-access")
	require.NotContains(t, userRoles.OrganizationRoles[owner.OrganizationID], "agents-access")

	// Restore the defaults so the stale grant is no longer implied and the
	// next update must validate its removal.
	updateOrg.DefaultOrgMemberRoles = org.DefaultOrgMemberRoles
	_, err = db.UpdateOrganization(ctx, updateOrg)
	require.NoError(t, err)

	// Role updates validate the removed set, which includes the stale
	// grant. The update must succeed and strip the retired role.
	updated, err := client.UpdateOrganizationMemberRoles(ctx, owner.OrganizationID, member.ID.String(), codersdk.UpdateRoles{
		Roles: []string{codersdk.RoleOrganizationAuditor},
	})
	require.NoError(t, err)
	names := make([]string, 0, len(updated.Roles))
	for _, role := range updated.Roles {
		names = append(names, role.Name)
	}
	require.Contains(t, names, codersdk.RoleOrganizationAuditor)
	require.NotContains(t, names, "agents-access")

	// Explicitly granting the retired role again is rejected for both org
	// and site scope, so tolerance of stale data cannot be used to store
	// fresh grants that a binary rollback would resolve again.
	_, err = client.UpdateOrganizationMemberRoles(ctx, owner.OrganizationID, member.ID.String(), codersdk.UpdateRoles{
		Roles: []string{codersdk.RoleOrganizationAuditor, "agents-access"},
	})
	require.ErrorContains(t, err, "retired")

	_, err = client.UpdateUserRoles(ctx, member.ID.String(), codersdk.UpdateRoles{
		Roles: []string{"agents-access"},
	})
	require.ErrorContains(t, err, "retired")

	// Adding the retired role to the org defaults is rejected at the
	// authorization boundary, mirroring the explicit grant paths. The
	// authorized store is used on purpose, unlike the raw seeding above.
	authzdb := dbauthz.New(db, rbac.NewAuthorizer(prometheus.NewRegistry()), testutil.Logger(t), coderdtest.AccessControlStorePointer())
	ownerUser, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	ownerSubject := coderdtest.AuthzUserSubjectWithDB(ctx, t, db, ownerUser)
	_, err = authzdb.UpdateOrganization(dbauthz.As(ctx, ownerSubject), database.UpdateOrganizationParams{
		ID:                    org.ID,
		UpdatedAt:             dbtime.Now(),
		Name:                  org.Name,
		DisplayName:           org.DisplayName,
		Description:           org.Description,
		Icon:                  org.Icon,
		DefaultOrgMemberRoles: append(org.DefaultOrgMemberRoles, "agents-access"),
	})
	require.ErrorContains(t, err, "retired")
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
			slice.List(members, onlyIDs))
	})

	t.Run("UserID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, owner.OrganizationID, codersdk.OrganizationMembersQueryOptionUserID(orgMember.ID))
		require.NoError(t, err)
		require.Len(t, members, 1)
		require.ElementsMatch(t,
			[]uuid.UUID{orgMember.ID},
			slice.List(members, onlyIDs))
	})

	t.Run("IncludeSystem", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, owner.OrganizationID, codersdk.OrganizationMembersQueryOptionIncludeSystem())
		require.NoError(t, err)
		require.Len(t, members, 4)
		require.ElementsMatch(t,
			[]uuid.UUID{owner.UserID, orgMember.ID, orgAdmin.ID, database.PrebuildsSystemUserID},
			slice.List(members, onlyIDs))
	})

	t.Run("GithubUserID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		members, err := client.OrganizationMembers(ctx, anotherOrg.ID, codersdk.OrganizationMembersQueryOptionGithubUserID(anotherUser.GithubComUserID.Int64))
		require.NoError(t, err)
		require.Len(t, members, 1)
		require.ElementsMatch(t,
			[]uuid.UUID{anotherUser.ID},
			slice.List(members, onlyIDs))
	})
}

func TestGetOrgMembersFilter(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		OIDCConfig: &coderd.OIDCConfig{
			AllowSignups: true,
		},
	})
	first := coderdtest.CreateFirstUser(t, client)

	setupCtx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	coderdtest.UsersFilter(setupCtx, t, client, api.Database, nil, nil, func(testCtx context.Context, req codersdk.UsersRequest) []codersdk.ReducedUser {
		res, err := client.OrganizationMembersPaginated(testCtx, first.OrganizationID, req)
		require.NoError(t, err)
		reduced := make([]codersdk.ReducedUser, len(res.Members))
		for i, user := range res.Members {
			reduced[i] = orgMemberToReducedUser(user)
		}
		return reduced
	})
}

func TestGetOrgMembersPagination(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	coderdtest.UsersPagination(ctx, t, client, nil, func(req codersdk.UsersRequest) ([]codersdk.ReducedUser, int) {
		res, err := client.OrganizationMembersPaginated(ctx, first.OrganizationID, req)
		require.NoError(t, err)
		reduced := make([]codersdk.ReducedUser, len(res.Members))
		for i, user := range res.Members {
			reduced[i] = orgMemberToReducedUser(user)
		}
		return reduced, res.Count
	})
}

func onlyIDs(u codersdk.OrganizationMemberWithUserData) uuid.UUID {
	return u.UserID
}

func orgMemberToReducedUser(user codersdk.OrganizationMemberWithUserData) codersdk.ReducedUser {
	return codersdk.ReducedUser{
		MinimalUser: codersdk.MinimalUser{
			ID:        user.UserID,
			Username:  user.Username,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
		},
		Email:            user.Email,
		CreatedAt:        user.UserCreatedAt,
		UpdatedAt:        user.UserUpdatedAt,
		LastSeenAt:       user.LastSeenAt,
		Status:           user.Status,
		IsServiceAccount: user.IsServiceAccount,
		LoginType:        user.LoginType,
	}
}
