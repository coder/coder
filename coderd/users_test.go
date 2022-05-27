package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func TestFirstUser(t *testing.T) {
	t.Parallel()
	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateFirstUser(context.Background(), codersdk.CreateFirstUserRequest{})
		require.Error(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, err := client.CreateFirstUser(context.Background(), codersdk.CreateFirstUserRequest{
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
		_, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
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
		req := codersdk.CreateFirstUserRequest{
			Email:            "testuser@coder.com",
			Username:         "testuser",
			Password:         "testpass",
			OrganizationName: "testorg",
		}
		_, err := client.CreateFirstUser(context.Background(), req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: "badpass",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		req := codersdk.CreateFirstUserRequest{
			Email:            "testuser@coder.com",
			Username:         "testuser",
			Password:         "testpass",
			OrganizationName: "testorg",
		}
		_, err := client.CreateFirstUser(context.Background(), req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		require.NoError(t, err)
	})
}

func TestPostLogout(t *testing.T) {
	t.Parallel()

	t.Run("ClearCookie", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		fullURL, err := client.URL.Parse("/api/v2/users/logout")
		require.NoError(t, err, "Server URL should parse successfully")

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, fullURL.String(), nil)
		require.NoError(t, err, "/logout request construction should succeed")

		httpClient := &http.Client{}

		response, err := httpClient.Do(req)
		require.NoError(t, err, "/logout request should succeed")
		response.Body.Close()

		cookies := response.Cookies()
		require.Len(t, cookies, 1, "Exactly one cookie should be returned")

		require.Equal(t, cookies[0].Name, httpmw.SessionTokenKey, "Cookie should be the auth cookie")
		require.Equal(t, cookies[0].MaxAge, -1, "Cookie should be set to delete")
	})
}

func TestPostUsers(t *testing.T) {
	t.Parallel()
	t.Run("NoAuth", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{})
		require.Error(t, err)
	})

	t.Run("Conflicting", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		me, err := client.User(context.Background(), codersdk.Me)
		require.NoError(t, err)
		_, err = client.CreateUser(context.Background(), codersdk.CreateUserRequest{
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
		_, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
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
		org, err := other.CreateOrganization(context.Background(), codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)

		_, err = notInOrg.CreateUser(context.Background(), codersdk.CreateUserRequest{
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
		_, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
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
		_, err := client.UpdateUserProfile(context.Background(), uuid.New().String(), codersdk.UpdateUserProfileRequest{
			Username: "newusername",
			Email:    "newemail@coder.com",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		// Right now, we are raising a BAD request error because we don't support a
		// user accessing other users info
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("ConflictingEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		existentUser, _ := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "bruno@coder.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		_, err := client.UpdateUserProfile(context.Background(), codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: "newusername",
			Email:    existentUser.Email,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("ConflictingUsername", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		existentUser, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "bruno@coder.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		_, err = client.UpdateUserProfile(context.Background(), codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: existentUser.Username,
			Email:    "newemail@coder.com",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("UpdateUsernameAndEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		userProfile, err := client.UpdateUserProfile(context.Background(), codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: "newusername",
			Email:    "newemail@coder.com",
		})
		require.NoError(t, err)
		require.Equal(t, userProfile.Username, "newusername")
		require.Equal(t, userProfile.Email, "newemail@coder.com")
	})

	t.Run("UpdateUsername", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		me, _ := client.User(context.Background(), codersdk.Me)
		userProfile, err := client.UpdateUserProfile(context.Background(), codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: me.Username,
			Email:    "newemail@coder.com",
		})
		require.NoError(t, err)
		require.Equal(t, userProfile.Username, me.Username)
		require.Equal(t, userProfile.Email, "newemail@coder.com")
	})
}

func TestUpdateUserPassword(t *testing.T) {
	t.Parallel()

	t.Run("MemberCantUpdateAdminPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		err := member.UpdateUserPassword(context.Background(), admin.UserID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.Error(t, err, "member should not be able to update admin password")
	})

	t.Run("AdminCanUpdateMemberPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		member, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "coder@coder.com",
			Username:       "coder",
			Password:       "password",
			OrganizationID: admin.OrganizationID,
		})
		require.NoError(t, err, "create member")
		err = client.UpdateUserPassword(context.Background(), member.ID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.NoError(t, err, "admin should be able to update member password")
		// Check if the member can login using the new password
		_, err = client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    "coder@coder.com",
			Password: "newpassword",
		})
		require.NoError(t, err, "member should login successfully with the new password")
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
		ctx := context.Background()
		admin := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, admin)
		member := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)
		memberUser, err := member.User(ctx, codersdk.Me)
		require.NoError(t, err, "member user")

		_, err = admin.UpdateUserRoles(ctx, codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{rbac.RoleOrgMember(first.OrganizationID)},
		})
		require.Error(t, err, "org role in site")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateUserRoles(ctx, uuid.New().String(), codersdk.UpdateRoles{
			Roles: []string{rbac.RoleOrgMember(first.OrganizationID)},
		})
		require.Error(t, err, "user does not exist")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{rbac.RoleMember()},
		})
		require.Error(t, err, "site role in org")
		requireStatusCode(t, err, http.StatusBadRequest)

		_, err = admin.UpdateOrganizationMemberRoles(ctx, uuid.New(), codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{rbac.RoleMember()},
		})
		require.Error(t, err, "role in org without membership")
		requireStatusCode(t, err, http.StatusNotFound)

		_, err = member.UpdateUserRoles(ctx, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{rbac.RoleMember()},
		})
		require.Error(t, err, "member cannot change other's roles")
		requireStatusCode(t, err, http.StatusForbidden)

		_, err = member.UpdateUserRoles(ctx, memberUser.ID.String(), codersdk.UpdateRoles{
			Roles: []string{rbac.RoleMember()},
		})
		require.Error(t, err, "member cannot change any roles")
		requireStatusCode(t, err, http.StatusForbidden)

		_, err = member.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, first.UserID.String(), codersdk.UpdateRoles{
			Roles: []string{rbac.RoleMember()},
		})
		require.Error(t, err, "member cannot change other's org roles")
		requireStatusCode(t, err, http.StatusForbidden)
	})

	t.Run("FirstUserRoles", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		roles, err := client.GetUserRoles(ctx, codersdk.Me)
		require.NoError(t, err)
		require.ElementsMatch(t, roles.Roles, []string{
			rbac.RoleAdmin(),
			rbac.RoleMember(),
		}, "should be a member and admin")

		require.ElementsMatch(t, roles.OrganizationRoles[first.OrganizationID], []string{
			rbac.RoleOrgMember(first.OrganizationID),
			rbac.RoleOrgAdmin(first.OrganizationID),
		}, "should be a member and admin")
	})

	t.Run("GrantAdmin", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		admin := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, admin)

		member := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)
		roles, err := member.GetUserRoles(ctx, codersdk.Me)
		require.NoError(t, err)
		require.ElementsMatch(t, roles.Roles, []string{
			rbac.RoleMember(),
		}, "should be a member and admin")
		require.ElementsMatch(t,
			roles.OrganizationRoles[first.OrganizationID],
			[]string{rbac.RoleOrgMember(first.OrganizationID)},
		)

		memberUser, err := member.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch member")

		// Grant
		_, err = admin.UpdateUserRoles(ctx, memberUser.ID.String(), codersdk.UpdateRoles{
			Roles: []string{
				// Promote to site admin
				rbac.RoleMember(),
				rbac.RoleAdmin(),
			},
		})
		require.NoError(t, err, "grant member admin role")

		// Promote to org admin
		_, err = member.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, codersdk.Me, codersdk.UpdateRoles{
			Roles: []string{
				// Promote to org admin
				rbac.RoleOrgMember(first.OrganizationID),
				rbac.RoleOrgAdmin(first.OrganizationID),
			},
		})
		require.NoError(t, err, "grant member org admin role")

		roles, err = member.GetUserRoles(ctx, codersdk.Me)
		require.NoError(t, err)
		require.ElementsMatch(t, roles.Roles, []string{
			rbac.RoleMember(),
			rbac.RoleAdmin(),
		}, "should be a member and admin")

		require.ElementsMatch(t, roles.OrganizationRoles[first.OrganizationID], []string{
			rbac.RoleOrgMember(first.OrganizationID),
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
		client.User(context.Background(), codersdk.Me)
		user, _ := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "bruno@coder.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: me.OrganizationID,
		})
		user, err := client.UpdateUserStatus(context.Background(), user.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)
		require.Equal(t, user.Status, codersdk.UserStatusSuspended)
	})

	t.Run("SuspendItSelf", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		client.User(context.Background(), codersdk.Me)
		_, err := client.UpdateUserStatus(context.Background(), codersdk.Me, codersdk.UserStatusSuspended)

		require.ErrorContains(t, err, "suspend yourself", "cannot suspend yourself")
	})
}

func TestGetUser(t *testing.T) {
	t.Parallel()

	t.Run("ByMe", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		user, err := client.User(context.Background(), codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID, user.ID)
		require.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("ByID", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		user, err := client.User(context.Background(), firstUser.UserID.String())
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID, user.ID)
		require.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("ByUsername", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		exp, err := client.User(context.Background(), firstUser.UserID.String())
		require.NoError(t, err)

		user, err := client.User(context.Background(), exp.Username)
		require.NoError(t, err)
		require.Equal(t, exp, user)
	})
}

func TestGetUsers(t *testing.T) {
	t.Parallel()
	t.Run("AllUsers", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "alice@email.com",
			Username:       "alice",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		// No params is all users
		users, err := client.Users(context.Background(), codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, users, 2)
		require.Len(t, users[0].OrganizationIDs, 1)
	})
	t.Run("ActiveUsers", func(t *testing.T) {
		t.Parallel()
		active := make([]codersdk.User, 0)
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		firstUser, err := client.User(context.Background(), first.UserID.String())
		require.NoError(t, err, "")
		active = append(active, firstUser)

		// Alice will be suspended
		alice, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "alice@email.com",
			Username:       "alice",
			Password:       "password",
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)

		bruno, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "bruno@email.com",
			Username:       "bruno",
			Password:       "password",
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)
		active = append(active, bruno)

		_, err = client.UpdateUserStatus(context.Background(), alice.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)

		users, err := client.Users(context.Background(), codersdk.UsersRequest{
			Status: string(codersdk.UserStatusActive),
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

		client.SessionToken = ""
		_, err := client.CreateAPIKey(context.Background(), codersdk.Me)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		apiKey, err := client.CreateAPIKey(context.Background(), codersdk.Me)
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
		workspaces, err := client.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Owner: codersdk.Me,
		})
		require.NoError(t, err)
		require.Len(t, workspaces, 0)
	})
	t.Run("Access", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		newUser, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          "test@coder.com",
			Username:       "someone",
			Password:       "password",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		auth, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
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

		workspaces, err := newUserClient.Workspaces(context.Background(), codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err)
		require.Len(t, workspaces, 0)

		workspaces, err = client.Workspaces(context.Background(), codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
	})
}

// TestPaginatedUsers creates a list of users, then tries to paginate through
// them using different page sizes.
func TestPaginatedUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := coderdtest.New(t, &coderdtest.Options{APIRateLimit: -1})
	coderdtest.CreateFirstUser(t, client)
	me, err := client.User(context.Background(), codersdk.Me)
	require.NoError(t, err)
	orgID := me.OrganizationIDs[0]

	allUsers := make([]codersdk.User, 0)
	allUsers = append(allUsers, me)
	specialUsers := make([]codersdk.User, 0)

	// When 100 users exist
	total := 100
	// Create users
	for i := 0; i < total; i++ {
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
		newUser, err := client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          email,
			Username:       username,
			Password:       "password",
			OrganizationID: orgID,
		})
		require.NoError(t, err)
		allUsers = append(allUsers, newUser)
		if i%2 == 0 {
			specialUsers = append(specialUsers, newUser)
		}
	}

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
	opt func(request codersdk.UsersRequest) codersdk.UsersRequest) {
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
