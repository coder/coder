//go:build !slim

package cli

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
)

func TestSelectExistingUsers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)

	const poolSize = 5
	for range poolSize {
		coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	}

	const userCount = 4
	selected, err := selectExistingUsers(ctx, client, userCount)
	require.NoError(t, err)
	require.Len(t, selected, userCount)

	// Selection excludes the caller and any owner/template-admin, and does not
	// mutate the users it returns.
	for _, u := range selected {
		require.NotEqual(t, firstUser.UserID, u.ID)
		require.False(t, userHasRole(u, codersdk.RoleOwner))
		require.False(t, userHasRole(u, codersdk.RoleTemplateAdmin))
	}
}

func TestSelectExistingUsersFailsFast(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)

	for range 2 {
		coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	}

	_, err := selectExistingUsers(ctx, client, 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 5 eligible existing users")
}

func TestPrepareAndRestoreUsers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	const poolSize = 4
	for range poolSize {
		coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	}

	selected, err := selectExistingUsers(ctx, client, poolSize)
	require.NoError(t, err)

	const adminCount = 2
	candidates := make([]preparedUser, len(selected))
	for i, u := range selected {
		candidates[i] = preparedUser{
			User:          u,
			IsAdmin:       i < adminCount,
			originalRoles: userRoleNames(u),
		}
	}

	err = prepareUsers(ctx, metrics, client, candidates)
	require.NoError(t, err)

	for i, pu := range candidates {
		require.NotEmpty(t, pu.SessionToken)

		// The minted token must authenticate as the prepared user.
		userClient := codersdk.New(client.URL, codersdk.WithSessionToken(pu.SessionToken))
		me, err := userClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, pu.User.ID, me.ID)

		// Admins are promoted to template admin; regular users are not.
		got, err := client.User(ctx, pu.User.ID.String())
		require.NoError(t, err)
		require.Equal(t, i < adminCount, userHasRole(got, codersdk.RoleTemplateAdmin))
	}

	err = restoreUsers(ctx, metrics, client, candidates)
	require.NoError(t, err)

	for _, pu := range candidates {
		// Promoted users are demoted back to their original roles.
		got, err := client.User(ctx, pu.User.ID.String())
		require.NoError(t, err)
		require.False(t, userHasRole(got, codersdk.RoleTemplateAdmin))

		// The minted token is revoked.
		userClient := codersdk.New(client.URL, codersdk.WithSessionToken(pu.SessionToken))
		_, err = userClient.User(ctx, codersdk.Me)
		require.Error(t, err)
	}

	// No users were deleted: first user + pool remain.
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, poolSize+1)
}

func TestPrepareUsersFatalOnFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	// A user that does not exist makes token minting fail, which must be fatal.
	candidates := []preparedUser{{
		User: codersdk.User{ReducedUser: codersdk.ReducedUser{MinimalUser: codersdk.MinimalUser{ID: uuid.New()}}},
	}}

	err := prepareUsers(ctx, metrics, client, candidates)
	require.Error(t, err)
	require.Empty(t, candidates[0].SessionToken)
}

func TestCreateAndDeleteUsers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	const userCount = 4
	const adminCount = 2
	candidates := make([]preparedUser, userCount)
	for i := range candidates {
		username, email, err := loadtestutil.GenerateUserIdentifier(strconv.Itoa(i))
		require.NoError(t, err)
		candidates[i] = preparedUser{
			IsAdmin:  i < adminCount,
			username: username,
			email:    email,
		}
	}

	err := createAndLoginUsers(ctx, metrics, client, firstUser.OrganizationID, candidates)
	require.NoError(t, err)

	// The pool users plus the original first user now exist.
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, userCount+1)

	for i, pu := range candidates {
		require.NotEqual(t, uuid.Nil, pu.User.ID)
		require.NotEmpty(t, pu.SessionToken)

		// The session token authenticates as the created user.
		userClient := codersdk.New(client.URL, codersdk.WithSessionToken(pu.SessionToken))
		me, err := userClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, pu.User.ID, me.ID)

		// Admins are created with the template-admin role; regular users are not.
		got, err := client.User(ctx, pu.User.ID.String())
		require.NoError(t, err)
		require.Equal(t, i < adminCount, userHasRole(got, codersdk.RoleTemplateAdmin))
	}

	err = deleteUsers(ctx, metrics, client, candidates)
	require.NoError(t, err)

	// Only the original first user remains.
	users, err = client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	require.Equal(t, firstUser.UserID, users.Users[0].ID)
}
