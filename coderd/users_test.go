package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestFirstUser(t *testing.T) {
	t.Parallel()
	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{})
		require.Error(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:            "some@email.com",
			Username:         "exampleuser",
			Password:         "password",
			OrganizationName: "someorg",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
	})
}

func TestPostLogin(t *testing.T) {
	t.Parallel()
	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    "my@email.org",
			Password: "password",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("BadPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.CreateFirstUserRequest{
			Email:            "testuser@coder.com",
			Username:         "testuser",
			Password:         "testpass",
			OrganizationName: "testorg",
		}
		_, err := client.CreateFirstUser(ctx, req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: "badpass",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Suspended", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		member := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		memberUser, err := member.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch member user")

		_, err = client.UpdateUserStatus(ctx, memberUser.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err, "suspend member")

		// Test an existing session
		_, err = member.User(ctx, codersdk.Me)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Contact an admin")

		// Test a new session
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    memberUser.Email,
			Password: "testpass",
		})
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "suspended")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.CreateFirstUserRequest{
			Email:            "testuser@coder.com",
			Username:         "testuser",
			Password:         "testpass",
			OrganizationName: "testorg",
		}
		_, err := client.CreateFirstUser(ctx, req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		require.NoError(t, err)
	})

	t.Run("Lifetime&Expire", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		split := strings.Split(client.SessionToken, "-")
		key, err := client.GetAPIKey(ctx, admin.UserID.String(), split[0])
		require.NoError(t, err, "fetch login key")
		require.Equal(t, int64(86400), key.LifetimeSeconds, "default should be 86400")

		// Generated tokens have a longer life
		token, err := client.CreateAPIKey(ctx, admin.UserID.String())
		require.NoError(t, err, "make new api key")
		split = strings.Split(token.Key, "-")
		apiKey, err := client.GetAPIKey(ctx, admin.UserID.String(), split[0])
		require.NoError(t, err, "fetch api key")

		require.True(t, apiKey.ExpiresAt.After(time.Now().Add(time.Hour*24*6)), "api key lasts more than 6 days")
		require.True(t, apiKey.ExpiresAt.After(key.ExpiresAt.Add(time.Hour)), "api key should be longer expires")
		require.Greater(t, apiKey.LifetimeSeconds, key.LifetimeSeconds, "api key should have longer lifetime")
	})
}

func TestPostLogout(t *testing.T) {
	t.Parallel()

	// Checks that the cookie is cleared and the API Key is deleted from the database.
	t.Run("Logout", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		keyID := strings.Split(client.SessionToken, "-")[0]
		apiKey, err := client.GetAPIKey(ctx, admin.UserID.String(), keyID)
		require.NoError(t, err)
		require.Equal(t, keyID, apiKey.ID, "API key should exist in the database")

		fullURL, err := client.URL.Parse("/api/v2/users/logout")
		require.NoError(t, err, "Server URL should parse successfully")

		res, err := client.Request(ctx, http.MethodPost, fullURL.String(), nil)
		require.NoError(t, err, "/logout request should succeed")
		res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		cookies := res.Cookies()
		require.Len(t, cookies, 1, "Exactly one cookie should be returned")

		require.Equal(t, codersdk.SessionTokenKey, cookies[0].Name, "Cookie should be the auth cookie")
		require.Equal(t, -1, cookies[0].MaxAge, "Cookie should be set to delete")

		_, err = client.GetAPIKey(ctx, admin.UserID.String(), keyID)
		sdkErr := &codersdk.Error{}
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode(), "Expecting 401")
	})
}

func TestPostUsers(t *testing.T) {
	t.Parallel()
	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUser(ctx, codersdk.CreateUserRequest{})
		require.Error(t, err)
	})

	t.Run("Conflicting", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		_, err = client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          me.Email,
			Username:       me.Username,
			Password:       "password",
			OrganizationID: uuid.New(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("OrganizationNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			OrganizationID: uuid.New(),
			Email:          "another@user.org",
			Username:       "someone-else",
			Password:       "testing",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("OrganizationNoAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		notInOrg := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleAdmin(), rbac.RoleMember())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		org, err := other.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)

		_, err = notInOrg.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "some@domain.com",
			Username:       "anotheruser",
			Password:       "testing",
			OrganizationID: org.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			OrganizationID: user.OrganizationID,
			Email:          "another@user.org",
			Username:       "someone-else",
			Password:       "testing",
		})
		require.NoError(t, err)
	})
}

func TestUpdateUserProfile(t *testing.T) {
	t.Parallel()
	t.Run("UserNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateUserProfile(ctx, uuid.New().String(), codersdk.UpdateUserProfileRequest{
			Username: "newusername",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		// Right now, we are raising a BAD request error because we don't support a
		// user accessing other users info
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("ConflictingUsername", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		existentUser, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "bruno@coder.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		_, err = client.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: existentUser.Username,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("UpdateUsername", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, _ = client.User(ctx, codersdk.Me)
		userProfile, err := client.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: "newusername",
		})
		require.NoError(t, err)
		require.Equal(t, userProfile.Username, "newusername")
	})
}

func TestUpdateUserPassword(t *testing.T) {
	t.Parallel()

	t.Run("MemberCantUpdateAdminPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := member.UpdateUserPassword(ctx, admin.UserID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.Error(t, err, "member should not be able to update admin password")
	})

	t.Run("AdminCanUpdateMemberPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		member, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "coder@coder.com",
			Username:       "coder",
			Password:       "password",
			OrganizationID: admin.OrganizationID,
		})
		require.NoError(t, err, "create member")
		err = client.UpdateUserPassword(ctx, member.ID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.NoError(t, err, "admin should be able to update member password")
		// Check if the member can login using the new password
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    "coder@coder.com",
			Password: "newpassword",
		})
		require.NoError(t, err, "member should login successfully with the new password")
	})
	t.Run("MemberCanUpdateOwnPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := member.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			OldPassword: "testpass",
			Password:    "newpassword",
		})
		require.NoError(t, err, "member should be able to update own password")
	})
	t.Run("MemberCantUpdateOwnPasswordWithoutOldPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := member.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.Error(t, err, "member should not be able to update own password without providing old password")
	})
	t.Run("AdminCanUpdateOwnPasswordWithoutOldPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.NoError(t, err, "admin should be able to update own password without providing old password")
	})
}

func TestGrantRoles(t *testing.T) {
	t.Parallel()

	requireStatusCode := func(t *testing.T, err error, statusCode int) {
		t.Helper()
		var e *codersdk.Error
		require.ErrorAs(t, err, &e, "error is codersdk error")
		require.Equal(t, statusCode, e.StatusCode(), "correct status code")
	}

	t.Run("UpdateIncorrectRoles", func(t *testing.T) {
		t.Parallel()
		var err error

		admin := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, admin)
		member := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err = admin.UpdateUserRoles(ctx, codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{rbac.RoleOrgAdmin(first.OrganizationID)},
		})
		require.Error(t, err, "org role in site")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateUserRoles(ctx, uuid.New().String(), codersdk.UpdateRoles{
			Roles: []string{rbac.RoleOrgAdmin(first.OrganizationID)},
		})
		require.Error(t, err, "user does not exist")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{rbac.RoleAdmin()},
		})
		require.Error(t, err, "site role in org")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateOrganizationMemberRoles(ctx, uuid.New(), codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{},
		})
		require.Error(t, err, "role in org without membership")
		requireStatusCode(t, err, http.StatusNotFound)

		_, err = member.UpdateUserRoles(ctx, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{},
		})
		require.Error(t, err, "member cannot change other's roles")
		requireStatusCode(t, err, http.StatusForbidden)

		_, err = member.UpdateUserRoles(ctx, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{},
		})
		require.Error(t, err, "member cannot change any roles")
		requireStatusCode(t, err, http.StatusForbidden)

		_, err = member.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{},
		})
		require.Error(t, err, "member cannot change other's org roles")
		requireStatusCode(t, err, http.StatusForbidden)

		_, err = admin.UpdateUserRoles(ctx, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{},
		})
		require.Error(t, err, "admin cannot change self roles")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{},
		})
		require.Error(t, err, "admin cannot change self org roles")
		requireStatusCode(t, err, http.StatusBadRequest)
	})

	t.Run("FirstUserRoles", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		roles, err := client.GetUserRoles(ctx, codersdk.Me)
		require.NoError(t, err)
		require.ElementsMatch(t, roles.Roles, []string{
			rbac.RoleAdmin(),
		}, "should be a member and admin")

		require.ElementsMatch(t, roles.OrganizationRoles[first.OrganizationID], []string{
			rbac.RoleOrgAdmin(first.OrganizationID),
		}, "should be a member and admin")
	})

	t.Run("GrantAdmin", func(t *testing.T) {
		t.Parallel()
		admin := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, admin)

		member := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		roles, err := member.GetUserRoles(ctx, codersdk.Me)
		require.NoError(t, err)
		require.ElementsMatch(t, roles.Roles, []string{}, "should be a member")
		require.ElementsMatch(t,
			roles.OrganizationRoles[first.OrganizationID],
			[]string{},
		)

		memberUser, err := member.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch member")

		// Grant
		_, err = admin.UpdateUserRoles(ctx, memberUser.ID.String(), codersdk.UpdateRoles{
			Roles: []string{
				// Promote to site admin
				rbac.RoleAdmin(),
			},
		})
		require.NoError(t, err, "grant member admin role")

		// Promote to org admin
		_, err = admin.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, memberUser.ID.String(), codersdk.UpdateRoles{
			Roles: []string{
				// Promote to org admin
				rbac.RoleOrgAdmin(first.OrganizationID),
			},
		})
		require.NoError(t, err, "grant member org admin role")

		roles, err = member.GetUserRoles(ctx, codersdk.Me)
		require.NoError(t, err)
		require.ElementsMatch(t, roles.Roles, []string{
			rbac.RoleAdmin(),
		}, "should be a member and admin")

		require.ElementsMatch(t, roles.OrganizationRoles[first.OrganizationID], []string{
			rbac.RoleOrgAdmin(first.OrganizationID),
		}, "should be a member and admin")
	})
}

func TestPutUserSuspend(t *testing.T) {
	t.Parallel()

	t.Run("SuspendAnotherUser", func(t *testing.T) {
		t.Skip()
		t.Parallel()
		client := coderdtest.New(t, nil)
		me := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client.User(ctx, codersdk.Me)
		user, _ := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "bruno@coder.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: me.OrganizationID,
		})
		user, err := client.UpdateUserStatus(ctx, user.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)
		require.Equal(t, user.Status, codersdk.UserStatusSuspended)
	})

	t.Run("SuspendItSelf", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client.User(ctx, codersdk.Me)
		_, err := client.UpdateUserStatus(ctx, codersdk.Me, codersdk.UserStatusSuspended)

		require.ErrorContains(t, err, "suspend yourself", "cannot suspend yourself")
	})
}

func TestGetUser(t *testing.T) {
	t.Parallel()

	t.Run("ByMe", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID, user.ID)
		require.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("ByID", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, firstUser.UserID.String())
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID, user.ID)
		require.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("ByUsername", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		exp, err := client.User(ctx, firstUser.UserID.String())
		require.NoError(t, err)

		user, err := client.User(ctx, exp.Username)
		require.NoError(t, err)
		require.Equal(t, exp, user)
	})
}

// TestUsersFilter creates a set of users to run various filters against for testing.
func TestUsersFilter(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	first := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	firstUser, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err, "fetch me")

	users := make([]codersdk.User, 0)
	users = append(users, firstUser)
	for i := 0; i < 15; i++ {
		roles := []string{}
		if i%2 == 0 {
			roles = append(roles, rbac.RoleAdmin())
		}
		if i%3 == 0 {
			roles = append(roles, "auditor")
		}
		userClient := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, roles...)
		user, err := userClient.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch me")

		if i%4 == 0 {
			user, err = client.UpdateUserStatus(ctx, user.ID.String(), codersdk.UserStatusSuspended)
			require.NoError(t, err, "suspend user")
		}

		if i%5 == 0 {
			user, err = client.UpdateUserProfile(ctx, user.ID.String(), codersdk.UpdateUserProfileRequest{
				Username: strings.ToUpper(user.Username),
			})
			require.NoError(t, err, "update username to uppercase")
		}

		users = append(users, user)
	}

	// --- Setup done ---
	testCases := []struct {
		Name   string
		Filter codersdk.UsersRequest
		// If FilterF is true, we include it in the expected results
		FilterF func(f codersdk.UsersRequest, user codersdk.User) bool
	}{
		{
			Name: "All",
			Filter: codersdk.UsersRequest{
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return true
			},
		},
		{
			Name: "Active",
			Filter: codersdk.UsersRequest{
				Status: codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusActive
			},
		},
		{
			Name: "ActiveUppercase",
			Filter: codersdk.UsersRequest{
				Status: "ACTIVE",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusActive
			},
		},
		{
			Name: "Suspended",
			Filter: codersdk.UsersRequest{
				Status: codersdk.UserStatusSuspended,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusSuspended
			},
		},
		{
			Name: "NameContains",
			Filter: codersdk.UsersRequest{
				Search: "a",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return (strings.ContainsAny(u.Username, "aA") || strings.ContainsAny(u.Email, "aA"))
			},
		},
		{
			Name: "Admins",
			Filter: codersdk.UsersRequest{
				Role:   rbac.RoleAdmin(),
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleAdmin() {
						return true
					}
				}
				return false
			},
		},
		{
			Name: "AdminsUppercase",
			Filter: codersdk.UsersRequest{
				Role:   "ADMIN",
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleAdmin() {
						return true
					}
				}
				return false
			},
		},
		{
			Name: "Members",
			Filter: codersdk.UsersRequest{
				Role:   rbac.RoleMember(),
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return true
			},
		},
		{
			Name: "SearchQuery",
			Filter: codersdk.UsersRequest{
				SearchQuery: "i role:admin status:active",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleAdmin() {
						return (strings.ContainsAny(u.Username, "iI") || strings.ContainsAny(u.Email, "iI")) &&
							u.Status == codersdk.UserStatusActive
					}
				}
				return false
			},
		},
		{
			Name: "SearchQueryInsensitive",
			Filter: codersdk.UsersRequest{
				SearchQuery: "i Role:Admin STATUS:Active",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleAdmin() {
						return (strings.ContainsAny(u.Username, "iI") || strings.ContainsAny(u.Email, "iI")) &&
							u.Status == codersdk.UserStatusActive
					}
				}
				return false
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			matched, err := client.Users(ctx, c.Filter)
			require.NoError(t, err, "fetch workspaces")

			exp := make([]codersdk.User, 0)
			for _, made := range users {
				match := c.FilterF(c.Filter, made)
				if match {
					exp = append(exp, made)
				}
			}
			require.ElementsMatch(t, exp, matched, "expected workspaces returned")
		})
	}
}

func TestGetUsers(t *testing.T) {
	t.Parallel()
	t.Run("AllUsers", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "alice@email.com",
			Username:       "alice",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		// No params is all users
		users, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users, 2)
		require.Len(t, users[0].OrganizationIDs, 1)
	})
	t.Run("ActiveUsers", func(t *testing.T) {
		t.Parallel()
		active := make([]codersdk.User, 0)
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		firstUser, err := client.User(ctx, first.UserID.String())
		require.NoError(t, err, "")
		active = append(active, firstUser)

		// Alice will be suspended
		alice, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "alice@email.com",
			Username:       "alice",
			Password:       "password",
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)

		bruno, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "bruno@email.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)
		active = append(active, bruno)

		_, err = client.UpdateUserStatus(ctx, alice.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)

		users, err := client.Users(ctx, codersdk.UsersRequest{
			Status: codersdk.UserStatusActive,
		})
		require.NoError(t, err)
		require.ElementsMatch(t, active, users)
	})
}

func TestPostAPIKey(t *testing.T) {
	t.Parallel()
	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client.SessionToken = ""
		_, err := client.CreateAPIKey(ctx, codersdk.Me)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		apiKey, err := client.CreateAPIKey(ctx, codersdk.Me)
		require.NotNil(t, apiKey)
		require.GreaterOrEqual(t, len(apiKey.Key), 2)
		require.NoError(t, err)
	})
}

func TestWorkspacesByUser(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: codersdk.Me,
		})
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})
	t.Run("Access", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		newUser, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "test@coder.com",
			Username:       "someone",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		auth, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    newUser.Email,
			Password: "password",
		})
		require.NoError(t, err)

		newUserClient := codersdk.New(client.URL)
		newUserClient.SessionToken = auth.SessionToken
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		workspaces, err := newUserClient.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err)
		require.Len(t, workspaces, 0)

		workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})
}

// TestSuspendedPagination is when the after_id is a suspended record.
// The database query should still return the correct page, as the after_id
// is in a subquery that finds the record regardless of its status.
// This is mainly to confirm the db fake has the same behavior.
func TestSuspendedPagination(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{APIRateLimit: -1})
	coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgID := me.OrganizationIDs[0]

	total := 10
	users := make([]codersdk.User, 0, total)
	// Create users
	for i := 0; i < total; i++ {
		email := fmt.Sprintf("%d@coder.com", i)
		username := fmt.Sprintf("user%d", i)
		user, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          email,
			Username:       username,
			Password:       "password",
			OrganizationID: orgID,
		})
		require.NoError(t, err)
		users = append(users, user)
	}
	sortUsers(users)
	deletedUser := users[2]
	expected := users[3:8]
	_, err = client.UpdateUserStatus(ctx, deletedUser.ID.String(), codersdk.UserStatusSuspended)
	require.NoError(t, err, "suspend user")

	page, err := client.Users(ctx, codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Limit:   len(expected),
			AfterID: deletedUser.ID,
		},
	})
	require.NoError(t, err)
	require.Equal(t, expected, page, "expected page")
}

// TestPaginatedUsers creates a list of users, then tries to paginate through
// them using different page sizes.
func TestPaginatedUsers(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{APIRateLimit: -1})
	coderdtest.CreateFirstUser(t, client)

	// This test can take longer than a long time.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*4)
	defer cancel()

	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgID := me.OrganizationIDs[0]

	// When 100 users exist
	total := 100
	allUsers := make([]codersdk.User, total+1) // +1 forme
	allUsers[0] = me
	specialUsers := make([]codersdk.User, total/2)

	eg, egCtx := errgroup.WithContext(ctx)
	// Create users
	for i := 0; i < total; i++ {
		i := i
		eg.Go(func() error {
			email := fmt.Sprintf("%d@coder.com", i)
			username := fmt.Sprintf("user%d", i)
			if i%2 == 0 {
				email = fmt.Sprintf("%d@gmail.com", i)
				username = fmt.Sprintf("specialuser%d", i)
			}
			// One side effect of having to use the api vs the db calls directly, is you cannot
			// mock time. Ideally I could pass in mocked times and space these users out.
			//
			// But this also serves as a good test. Postgres has microsecond precision on its timestamps.
			// If 2 users share the same created_at, that could cause an issue if you are strictly paginating via
			// timestamps. The pagination goes by timestamps and uuids.
			newUser, err := client.CreateUser(egCtx, codersdk.CreateUserRequest{
				Email:          email,
				Username:       username,
				Password:       "password",
				OrganizationID: orgID,
			})
			if err != nil {
				return err
			}
			allUsers[i+1] = newUser
			if i%2 == 0 {
				specialUsers[i/2] = newUser
			}

			return nil
		})
	}
	err = eg.Wait()
	require.NoError(t, err, "create users failed")

	// Sorting the users will sort by (created_at, uuid). This is to handle
	// the off case that created_at is identical for 2 users.
	// This is a really rare case in production, but does happen in unit tests
	// due to the fake database being in memory and exceptionally quick.
	sortUsers(allUsers)
	sortUsers(specialUsers)

	assertPagination(ctx, t, client, 10, allUsers, nil)
	assertPagination(ctx, t, client, 5, allUsers, nil)
	assertPagination(ctx, t, client, 3, allUsers, nil)
	assertPagination(ctx, t, client, 1, allUsers, nil)

	// Try a search
	gmailSearch := func(request codersdk.UsersRequest) codersdk.UsersRequest {
		request.Search = "gmail"
		return request
	}
	assertPagination(ctx, t, client, 3, specialUsers, gmailSearch)
	assertPagination(ctx, t, client, 7, specialUsers, gmailSearch)

	usernameSearch := func(request codersdk.UsersRequest) codersdk.UsersRequest {
		request.Search = "specialuser"
		return request
	}
	assertPagination(ctx, t, client, 3, specialUsers, usernameSearch)
	assertPagination(ctx, t, client, 1, specialUsers, usernameSearch)
}

// Assert pagination will page through the list of all users using the given
// limit for each page. The 'allUsers' is the expected full list to compare
// against.
func assertPagination(ctx context.Context, t *testing.T, client *codersdk.Client, limit int, allUsers []codersdk.User,
	opt func(request codersdk.UsersRequest) codersdk.UsersRequest,
) {
	// Ensure any single assertion doesn't take too long.s
	ctx, cancel := context.WithTimeout(ctx, testutil.WaitLong)
	defer cancel()

	var count int
	if opt == nil {
		opt = func(request codersdk.UsersRequest) codersdk.UsersRequest {
			return request
		}
	}

	// Check the first page
	page, err := client.Users(ctx, opt(codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Limit: limit,
		},
	}))
	require.NoError(t, err, "first page")
	require.Equalf(t, page, allUsers[:limit], "first page, limit=%d", limit)
	count += len(page)

	for {
		if len(page) == 0 {
			break
		}

		afterCursor := page[len(page)-1].ID
		// Assert each page is the next expected page
		// This is using a cursor, and only works if all users created_at
		// is unique.
		page, err = client.Users(ctx, opt(codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Limit:   limit,
				AfterID: afterCursor,
			},
		}))
		require.NoError(t, err, "next cursor page")

		// Also check page by offset
		offsetPage, err := client.Users(ctx, opt(codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Limit:  limit,
				Offset: count,
			},
		}))
		require.NoError(t, err, "next offset page")

		var expected []codersdk.User
		if count+limit > len(allUsers) {
			expected = allUsers[count:]
		} else {
			expected = allUsers[count : count+limit]
		}
		require.Equalf(t, page, expected, "next users, after=%s, limit=%d", afterCursor, limit)
		require.Equalf(t, offsetPage, expected, "offset users, offset=%d, limit=%d", count, limit)

		// Also check the before
		prevPage, err := client.Users(ctx, opt(codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Offset: count - limit,
				Limit:  limit,
			},
		}))
		require.NoError(t, err, "prev page")
		require.Equal(t, allUsers[count-limit:count], prevPage, "prev users")
		count += len(page)
	}
}

// sortUsers sorts by (created_at, id)
func sortUsers(users []codersdk.User) {
	sort.Slice(users, func(i, j int) bool {
		if users[i].CreatedAt.Equal(users[j].CreatedAt) {
			return users[i].ID.String() < users[j].ID.String()
		}
		return users[i].CreatedAt.Before(users[j].CreatedAt)
	})
}
