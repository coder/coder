package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestFirstUser(t *testing.T) {
	t.Parallel()
	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		has, err := client.HasFirstUser(context.Background())
		require.NoError(t, err)
		require.False(t, has)

		_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{})
		require.Error(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:    "some@email.com",
			Username: "exampleuser",
			Password: "SomeSecurePassword!",
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

	t.Run("Trial", func(t *testing.T) {
		t.Parallel()
		called := make(chan struct{})
		client := coderdtest.New(t, &coderdtest.Options{
			TrialGenerator: func(ctx context.Context, s string) error {
				close(called)
				return nil
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.CreateFirstUserRequest{
			Email:    "testuser@coder.com",
			Username: "testuser",
			Password: "SomeSecurePassword!",
			Trial:    true,
		}
		_, err := client.CreateFirstUser(ctx, req)
		require.NoError(t, err)
		<-called
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
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.CreateFirstUserRequest{
			Email:    "testuser@coder.com",
			Username: "testuser",
			Password: "SomeSecurePassword!",
		}
		_, err := client.CreateFirstUser(ctx, req)
		require.NoError(t, err)
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: "badpass",
		})
		numLogs++ // add an audit log for login
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("Suspended", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())
		first := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for create user
		numLogs++ // add an audit log for login

		member, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		numLogs++ // add an audit log for create user

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		memberUser, err := member.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch member user")

		_, err = client.UpdateUserStatus(ctx, memberUser.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err, "suspend member")
		numLogs++ // add an audit log for update user

		// Test an existing session
		_, err = member.User(ctx, codersdk.Me)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Contact an admin")

		// Test a new session
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    memberUser.Email,
			Password: "SomeSecurePassword!",
		})
		numLogs++ // add an audit log for login
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "suspended")

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("DisabledPasswordAuth", func(t *testing.T) {
		t.Parallel()

		dc := coderdtest.DeploymentValues(t)
		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dc,
		})

		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// With a user account.
		const password = "SomeSecurePassword!"
		user, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "test+user-@coder.com",
			Username:       "user",
			Password:       password,
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)

		dc.DisablePasswordAuth = clibase.Bool(true)

		userClient := codersdk.New(client.URL)
		_, err = userClient.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: password,
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Password authentication is disabled")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.CreateFirstUserRequest{
			Email:    "testuser@coder.com",
			Username: "testuser",
			Password: "SomeSecurePassword!",
		}
		_, err := client.CreateFirstUser(ctx, req)
		require.NoError(t, err)
		numLogs++ // add an audit log for create user
		numLogs++ // add an audit log for login

		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		require.NoError(t, err)

		// Login should be case insensitive
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    strings.ToUpper(req.Email),
			Password: req.Password,
		})
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("Lifetime&Expire", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		split := strings.Split(client.SessionToken(), "-")
		key, err := client.APIKeyByID(ctx, owner.UserID.String(), split[0])
		require.NoError(t, err, "fetch login key")
		require.Equal(t, int64(86400), key.LifetimeSeconds, "default should be 86400")

		// tokens have a longer life
		token, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{})
		require.NoError(t, err, "make new token api key")
		split = strings.Split(token.Key, "-")
		apiKey, err := client.APIKeyByID(ctx, owner.UserID.String(), split[0])
		require.NoError(t, err, "fetch api key")

		require.True(t, apiKey.ExpiresAt.After(time.Now().Add(time.Hour*24*29)), "default tokens lasts more than 29 days")
		require.True(t, apiKey.ExpiresAt.Before(time.Now().Add(time.Hour*24*31)), "default tokens lasts less than 31 days")
		require.Greater(t, apiKey.LifetimeSeconds, key.LifetimeSeconds, "token should have longer lifetime")
	})
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()
	t.Run("Works", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		authz := coderdtest.AssertRBAC(t, api, client)

		anotherClient, another := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		err := client.DeleteUser(context.Background(), another.ID)
		require.NoError(t, err)
		// Attempt to create a user with the same email and username, and delete them again.
		another, err = client.CreateUser(context.Background(), codersdk.CreateUserRequest{
			Email:          another.Email,
			Username:       another.Username,
			Password:       "SomeSecurePassword!",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		err = client.DeleteUser(context.Background(), another.ID)
		require.NoError(t, err)

		// IMPORTANT: assert that the deleted user's session is no longer valid.
		_, err = anotherClient.User(context.Background(), codersdk.Me)
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())

		// RBAC checks
		authz.AssertChecked(t, rbac.ActionCreate, rbac.ResourceUser)
		authz.AssertChecked(t, rbac.ActionDelete, another)
	})
	t.Run("NoPermission", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, api)
		client, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		err := client.DeleteUser(context.Background(), firstUser.UserID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})
	t.Run("HasWorkspaces", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithProvisionerCloser(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, another := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateWorkspace(t, anotherClient, user.OrganizationID, template.ID)
		err := client.DeleteUser(context.Background(), another.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusExpectationFailed, apiErr.StatusCode())
	})
	t.Run("Self", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		err := client.DeleteUser(context.Background(), user.UserID)
		var apiErr *codersdk.Error
		require.Error(t, err, "should not be able to delete self")
		require.ErrorAs(t, err, &apiErr, "should be a coderd error")
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode(), "should be forbidden")
	})
}

func TestPostLogout(t *testing.T) {
	t.Parallel()

	// Checks that the cookie is cleared and the API Key is deleted from the database.
	t.Run("Logout", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		owner := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		keyID := strings.Split(client.SessionToken(), "-")[0]
		apiKey, err := client.APIKeyByID(ctx, owner.UserID.String(), keyID)
		require.NoError(t, err)
		require.Equal(t, keyID, apiKey.ID, "API key should exist in the database")

		fullURL, err := client.URL.Parse("/api/v2/users/logout")
		require.NoError(t, err, "Server URL should parse successfully")

		res, err := client.Request(ctx, http.MethodPost, fullURL.String(), nil)
		numLogs++ // add an audit log for logout

		require.NoError(t, err, "/logout request should succeed")
		res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionLogout, auditor.AuditLogs()[numLogs-1].Action)

		cookies := res.Cookies()

		var found bool
		for _, cookie := range cookies {
			if cookie.Name == codersdk.SessionTokenCookie {
				require.Equal(t, codersdk.SessionTokenCookie, cookie.Name, "Cookie should be the auth cookie")
				require.Equal(t, -1, cookie.MaxAge, "Cookie should be set to delete")
				found = true
			}
		}
		require.True(t, found, "auth cookie should be returned")

		_, err = client.APIKeyByID(ctx, owner.UserID.String(), keyID)
		sdkErr := &codersdk.Error{}
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode(), "Expecting 401")
	})
}

// nolint:bodyclose
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
			Password:       "MySecurePassword!",
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
			Password:       "SomeSecurePassword!",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("OrganizationNoAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		notInOrg, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleOwner(), rbac.RoleMember())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		org, err := other.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)

		_, err = notInOrg.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "some@domain.com",
			Username:       "anotheruser",
			Password:       "SomeSecurePassword!",
			OrganizationID: org.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("CreateWithoutOrg", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		firstUser := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:    "another@user.org",
			Username: "someone-else",
			Password: "SomeSecurePassword!",
		})
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionCreate, auditor.AuditLogs()[numLogs-1].Action)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs()[numLogs-2].Action)

		require.Len(t, user.OrganizationIDs, 1)
		assert.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		firstUser := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			OrganizationID: firstUser.OrganizationID,
			Email:          "another@user.org",
			Username:       "someone-else",
			Password:       "SomeSecurePassword!",
		})
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionCreate, auditor.AuditLogs()[numLogs-1].Action)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs()[numLogs-2].Action)

		require.Len(t, user.OrganizationIDs, 1)
		assert.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("LastSeenAt", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client := coderdtest.New(t, nil)
		firstUserResp := coderdtest.CreateFirstUser(t, client)

		firstUser, err := client.User(ctx, firstUserResp.UserID.String())
		require.NoError(t, err)

		_, _ = coderdtest.CreateAnotherUser(t, client, firstUserResp.OrganizationID)

		allUsersRes, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)

		require.Len(t, allUsersRes.Users, 2)

		// We sent the "GET Users" request with the first user, but the second user
		// should be Never since they haven't performed a request.
		for _, user := range allUsersRes.Users {
			if user.ID == firstUser.ID {
				require.WithinDuration(t, firstUser.LastSeenAt, dbtime.Now(), testutil.WaitShort)
			} else {
				require.Zero(t, user.LastSeenAt)
			}
		}
	})

	t.Run("CreateNoneLoginType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			OrganizationID: first.OrganizationID,
			Email:          "another@user.org",
			Username:       "someone-else",
			Password:       "",
			UserLoginType:  codersdk.LoginTypeNone,
		})
		require.NoError(t, err)

		found, err := client.User(ctx, user.ID.String())
		require.NoError(t, err)
		require.Equal(t, found.LoginType, codersdk.LoginTypeNone)
	})

	t.Run("CreateOIDCLoginType", func(t *testing.T) {
		t.Parallel()
		email := "another@user.org"
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithServing(),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		})

		client := coderdtest.New(t, &coderdtest.Options{
			OIDCConfig: cfg,
		})
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			OrganizationID: first.OrganizationID,
			Email:          email,
			Username:       "someone-else",
			Password:       "",
			UserLoginType:  codersdk.LoginTypeOIDC,
		})
		require.NoError(t, err)

		// Try to log in with OIDC.
		userClient, _ := fake.Login(t, client, jwt.MapClaims{
			"email": email,
		})

		found, err := userClient.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, found.LoginType, codersdk.LoginTypeOIDC)
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
			Password:       "SomeSecurePassword!",
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
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, _ = client.User(ctx, codersdk.Me)
		userProfile, err := client.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: "newusername",
		})
		require.NoError(t, err)
		require.Equal(t, userProfile.Username, "newusername")
		numLogs++ // add an audit log for user update

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})
}

func TestUpdateUserPassword(t *testing.T) {
	t.Parallel()

	t.Run("MemberCantUpdateAdminPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := member.UpdateUserPassword(ctx, owner.UserID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.Error(t, err, "member should not be able to update admin password")
	})

	t.Run("AdminCanUpdateMemberPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		member, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "coder@coder.com",
			Username:       "coder",
			Password:       "SomeStrongPassword!",
			OrganizationID: owner.OrganizationID,
		})
		require.NoError(t, err, "create member")
		err = client.UpdateUserPassword(ctx, member.ID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "SomeNewStrongPassword!",
		})
		require.NoError(t, err, "admin should be able to update member password")
		// Check if the member can login using the new password
		_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    "coder@coder.com",
			Password: "SomeNewStrongPassword!",
		})
		require.NoError(t, err, "member should login successfully with the new password")
	})
	t.Run("MemberCanUpdateOwnPassword", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		owner := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create
		numLogs++ // add an audit log for login

		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		numLogs++ // add an audit log for user create

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := member.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			OldPassword: "SomeSecurePassword!",
			Password:    "MyNewSecurePassword!",
		})
		numLogs++ // add an audit log for user update

		require.NoError(t, err, "member should be able to update own password")

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})
	t.Run("MemberCantUpdateOwnPasswordWithoutOldPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := member.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			Password: "newpassword",
		})
		require.Error(t, err, "member should not be able to update own password without providing old password")
	})
	t.Run("AdminCanUpdateOwnPasswordWithoutOldPassword", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		_ = coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			Password: "MySecurePassword!",
		})
		numLogs++ // add an audit log for user update

		require.NoError(t, err, "admin should be able to update own password without providing old password")

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("ChangingPasswordDeletesKeys", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		apikey1, err := client.CreateToken(ctx, user.UserID.String(), codersdk.CreateTokenRequest{})
		require.NoError(t, err)

		apikey2, err := client.CreateToken(ctx, user.UserID.String(), codersdk.CreateTokenRequest{})
		require.NoError(t, err)

		err = client.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			Password: "MyNewSecurePassword!",
		})
		require.NoError(t, err)

		// Trying to get an API key should fail since our client's token
		// has been deleted.
		_, err = client.APIKeyByID(ctx, user.UserID.String(), apikey1.Key)
		require.Error(t, err)
		cerr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusUnauthorized, cerr.StatusCode())

		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    coderdtest.FirstUserParams.Email,
			Password: "MyNewSecurePassword!",
		})
		require.NoError(t, err)

		client.SetSessionToken(resp.SessionToken)

		// Trying to get an API key should fail since all keys are deleted
		// on password change.
		_, err = client.APIKeyByID(ctx, user.UserID.String(), apikey1.Key)
		require.Error(t, err)
		cerr = coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		_, err = client.APIKeyByID(ctx, user.UserID.String(), apikey2.Key)
		require.Error(t, err)
		cerr = coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})

	t.Run("PasswordsMustDiffer", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		err := client.UpdateUserPassword(ctx, "me", codersdk.UpdateUserPasswordRequest{
			Password: coderdtest.FirstUserParams.Password,
		})
		require.Error(t, err)
		cerr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}

func TestGrantSiteRoles(t *testing.T) {
	t.Parallel()

	requireStatusCode := func(t *testing.T, err error, statusCode int) {
		t.Helper()
		var e *codersdk.Error
		require.ErrorAs(t, err, &e, "error is codersdk error")
		require.Equal(t, statusCode, e.StatusCode(), "correct status code")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)
	var err error

	admin := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, admin)
	member, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID, rbac.RoleOrgAdmin(first.OrganizationID))
	randOrg, err := admin.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "random",
	})
	require.NoError(t, err)
	_, randOrgUser := coderdtest.CreateAnotherUser(t, admin, randOrg.ID, rbac.RoleOrgAdmin(randOrg.ID))
	userAdmin, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID, rbac.RoleUserAdmin())

	const newUser = "newUser"

	testCases := []struct {
		Name          string
		Client        *codersdk.Client
		OrgID         uuid.UUID
		AssignToUser  string
		Roles         []string
		ExpectedRoles []string
		Error         bool
		StatusCode    int
	}{
		{
			Name:         "OrgRoleInSite",
			Client:       admin,
			AssignToUser: codersdk.Me,
			Roles:        []string{rbac.RoleOrgAdmin(first.OrganizationID)},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "UserNotExists",
			Client:       admin,
			AssignToUser: uuid.NewString(),
			Roles:        []string{rbac.RoleOwner()},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "MemberCannotUpdateRoles",
			Client:       member,
			AssignToUser: first.UserID.String(),
			Roles:        []string{},
			Error:        true,
			StatusCode:   http.StatusForbidden,
		},
		{
			// Cannot update your own roles
			Name:         "AdminOnSelf",
			Client:       admin,
			AssignToUser: first.UserID.String(),
			Roles:        []string{},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "SiteRoleInOrg",
			Client:       admin,
			OrgID:        first.OrganizationID,
			AssignToUser: codersdk.Me,
			Roles:        []string{rbac.RoleOwner()},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "RoleInNotMemberOrg",
			Client:       orgAdmin,
			OrgID:        randOrg.ID,
			AssignToUser: randOrgUser.ID.String(),
			Roles:        []string{rbac.RoleOrgMember(randOrg.ID)},
			Error:        true,
			StatusCode:   http.StatusNotFound,
		},
		{
			Name:         "AdminUpdateOrgSelf",
			Client:       admin,
			OrgID:        first.OrganizationID,
			AssignToUser: first.UserID.String(),
			Roles:        []string{},
			Error:        true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name:         "OrgAdminPromote",
			Client:       orgAdmin,
			OrgID:        first.OrganizationID,
			AssignToUser: newUser,
			Roles:        []string{rbac.RoleOrgAdmin(first.OrganizationID)},
			ExpectedRoles: []string{
				rbac.RoleOrgAdmin(first.OrganizationID),
			},
			Error: false,
		},
		{
			Name:         "UserAdminMakeMember",
			Client:       userAdmin,
			AssignToUser: newUser,
			Roles:        []string{rbac.RoleMember()},
			ExpectedRoles: []string{
				rbac.RoleMember(),
			},
			Error: false,
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			var err error
			if c.AssignToUser == newUser {
				orgID := first.OrganizationID
				if c.OrgID != uuid.Nil {
					orgID = c.OrgID
				}
				_, newUser := coderdtest.CreateAnotherUser(t, admin, orgID)
				c.AssignToUser = newUser.ID.String()
			}

			var newRoles []codersdk.Role
			if c.OrgID != uuid.Nil {
				// Org assign
				var mem codersdk.OrganizationMember
				mem, err = c.Client.UpdateOrganizationMemberRoles(ctx, c.OrgID, c.AssignToUser, codersdk.UpdateRoles{
					Roles: c.Roles,
				})
				newRoles = mem.Roles
			} else {
				// Site assign
				var user codersdk.User
				user, err = c.Client.UpdateUserRoles(ctx, c.AssignToUser, codersdk.UpdateRoles{
					Roles: c.Roles,
				})
				newRoles = user.Roles
			}

			if c.Error {
				require.Error(t, err)
				requireStatusCode(t, err, c.StatusCode)
			} else {
				require.NoError(t, err)
				roles := make([]string, 0, len(newRoles))
				for _, r := range newRoles {
					roles = append(roles, r.Name)
				}
				require.ElementsMatch(t, roles, c.ExpectedRoles)
			}
		})
	}
}

// TestInitialRoles ensures the starting roles for the first user are correct.
func TestInitialRoles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)

	roles, err := client.UserRoles(ctx, codersdk.Me)
	require.NoError(t, err)
	require.ElementsMatch(t, roles.Roles, []string{
		rbac.RoleOwner(),
	}, "should be a member and admin")

	require.ElementsMatch(t, roles.OrganizationRoles[first.OrganizationID], []string{
		rbac.RoleOrgMember(first.OrganizationID),
	}, "should be a member and admin")
}

func TestPutUserSuspend(t *testing.T) {
	t.Parallel()

	t.Run("SuspendAnOwner", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		me := coderdtest.CreateFirstUser(t, client)
		_, user := coderdtest.CreateAnotherUser(t, client, me.OrganizationID, rbac.RoleOwner())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateUserStatus(ctx, user.Username, codersdk.UserStatusSuspended)
		require.Error(t, err, "cannot suspend owners")
	})

	t.Run("SuspendAnotherUser", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		me := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create
		numLogs++ // add an audit log for login

		_, user := coderdtest.CreateAnotherUser(t, client, me.OrganizationID)
		numLogs++ // add an audit log for user create

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.UpdateUserStatus(ctx, user.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)
		require.Equal(t, user.Status, codersdk.UserStatusSuspended)
		numLogs++ // add an audit log for user update

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
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

func TestActivateDormantUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	// Create users
	me := coderdtest.CreateFirstUser(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	anotherUser, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          "coder@coder.com",
		Username:       "coder",
		Password:       "SomeStrongPassword!",
		OrganizationID: me.OrganizationID,
	})
	require.NoError(t, err)

	// Ensure that new user has dormant account
	require.Equal(t, codersdk.UserStatusDormant, anotherUser.Status)

	// Activate user account
	_, err = client.UpdateUserStatus(ctx, anotherUser.Username, codersdk.UserStatusActive)
	require.NoError(t, err)

	// Verify if the account is active now
	anotherUser, err = client.User(ctx, anotherUser.Username)
	require.NoError(t, err)
	require.Equal(t, codersdk.UserStatusActive, anotherUser.Status)
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

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	first := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	firstUser, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err, "fetch me")

	// Noon on Jan 18 is the "now" for this test for last_seen timestamps.
	// All these values are equal
	// 2023-01-18T12:00:00Z (UTC)
	// 2023-01-18T07:00:00-05:00 (America/New_York)
	// 2023-01-18T13:00:00+01:00 (Europe/Madrid)
	// 2023-01-16T00:00:00+12:00 (Asia/Anadyr)
	lastSeenNow := time.Date(2023, 1, 18, 12, 0, 0, 0, time.UTC)
	users := make([]codersdk.User, 0)
	users = append(users, firstUser)
	for i := 0; i < 15; i++ {
		roles := []string{}
		if i%2 == 0 {
			roles = append(roles, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())
		}
		if i%3 == 0 {
			roles = append(roles, "auditor")
		}
		userClient, userData := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, roles...)
		// Set the last seen for each user to a unique day
		// nolint:gocritic // Unit test
		_, err := api.Database.UpdateUserLastSeenAt(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLastSeenAtParams{
			ID:         userData.ID,
			LastSeenAt: lastSeenNow.Add(-1 * time.Hour * 24 * time.Duration(i)),
			UpdatedAt:  time.Now(),
		})
		require.NoError(t, err, "set a last seen")

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
				Role:   rbac.RoleOwner(),
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleOwner() {
						return true
					}
				}
				return false
			},
		},
		{
			Name: "AdminsUppercase",
			Filter: codersdk.UsersRequest{
				Role:   "OWNER",
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleOwner() {
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
				SearchQuery: "i role:owner status:active",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleOwner() {
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
				SearchQuery: "i Role:Owner STATUS:Active",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == rbac.RoleOwner() {
						return (strings.ContainsAny(u.Username, "iI") || strings.ContainsAny(u.Email, "iI")) &&
							u.Status == codersdk.UserStatusActive
					}
				}
				return false
			},
		},
		{
			Name: "LastSeenBeforeNow",
			Filter: codersdk.UsersRequest{
				SearchQuery: `last_seen_before:"2023-01-16T00:00:00+12:00"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.LastSeenAt.Before(lastSeenNow)
			},
		},
		{
			Name: "LastSeenLastWeek",
			Filter: codersdk.UsersRequest{
				SearchQuery: `last_seen_before:"2023-01-14T23:59:59Z" last_seen_after:"2023-01-08T00:00:00Z"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				start := time.Date(2023, 1, 8, 0, 0, 0, 0, time.UTC)
				end := time.Date(2023, 1, 14, 23, 59, 59, 0, time.UTC)
				return u.LastSeenAt.Before(end) && u.LastSeenAt.After(start)
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
			require.ElementsMatch(t, exp, matched.Users, "expected workspaces returned")
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
			Password:       "MySecurePassword!",
			OrganizationID: user.OrganizationID,
		})
		// No params is all users
		res, err := client.Users(ctx, codersdk.UsersRequest{})
		require.NoError(t, err)
		require.Len(t, res.Users, 2)
		require.Len(t, res.Users[0].OrganizationIDs, 1)
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
			Password:       "MySecurePassword!",
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)

		_, err = client.UpdateUserStatus(ctx, alice.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)

		// Tom will be active
		tom, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "tom@email.com",
			Username:       "tom",
			Password:       "MySecurePassword!",
			OrganizationID: first.OrganizationID,
		})
		require.NoError(t, err)

		tom, err = client.UpdateUserStatus(ctx, tom.Username, codersdk.UserStatusActive)
		require.NoError(t, err)
		active = append(active, tom)

		res, err := client.Users(ctx, codersdk.UsersRequest{
			Status: codersdk.UserStatusActive,
		})
		require.NoError(t, err)
		require.ElementsMatch(t, active, res.Users)
	})
}

func TestGetUsersPagination(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	_, err := client.User(ctx, first.UserID.String())
	require.NoError(t, err, "")

	_, err = client.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          "alice@email.com",
		Username:       "alice",
		Password:       "MySecurePassword!",
		OrganizationID: first.OrganizationID,
	})
	require.NoError(t, err)

	res, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, res.Users, 2)
	require.Equal(t, res.Count, 2)

	res, err = client.Users(ctx, codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Limit: 1,
		},
	})
	require.NoError(t, err)
	require.Len(t, res.Users, 1)
	require.Equal(t, res.Count, 2)

	res, err = client.Users(ctx, codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Offset: 1,
		},
	})
	require.NoError(t, err)
	require.Len(t, res.Users, 1)
	require.Equal(t, res.Count, 2)

	// if offset is higher than the count postgres returns an empty array
	// and not an ErrNoRows error.
	res, err = client.Users(ctx, codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Offset: 3,
		},
	})
	require.NoError(t, err)
	require.Len(t, res.Users, 0)
	require.Equal(t, res.Count, 0)
}

func TestPostTokens(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	apiKey, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{})
	require.NotNil(t, apiKey)
	require.GreaterOrEqual(t, len(apiKey.Key), 2)
	require.NoError(t, err)
}

func TestWorkspacesByUser(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: codersdk.Me,
		})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)
	})
	t.Run("Access", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		newUser, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "test@coder.com",
			Username:       "someone",
			Password:       "MySecurePassword!",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		auth, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    newUser.Email,
			Password: "MySecurePassword!",
		})
		require.NoError(t, err)

		newUserClient := codersdk.New(client.URL)
		newUserClient.SetSessionToken(auth.SessionToken)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		res, err := newUserClient.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 0)

		res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err)
		require.Len(t, res.Workspaces, 1)
	})
}

func TestDormantUser(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Create a new user
	newUser, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          "test@coder.com",
		Username:       "someone",
		Password:       "MySecurePassword!",
		OrganizationID: user.OrganizationID,
	})
	require.NoError(t, err)

	// User should be dormant as they haven't logged in yet
	users, err := client.Users(ctx, codersdk.UsersRequest{Search: newUser.Username})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	require.Equal(t, codersdk.UserStatusDormant, users.Users[0].Status)

	// User logs in now
	_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    newUser.Email,
		Password: "MySecurePassword!",
	})
	require.NoError(t, err)

	// User status should be active now
	users, err = client.Users(ctx, codersdk.UsersRequest{Search: newUser.Username})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	require.Equal(t, codersdk.UserStatusActive, users.Users[0].Status)
}

// TestSuspendedPagination is when the after_id is a suspended record.
// The database query should still return the correct page, as the after_id
// is in a subquery that finds the record regardless of its status.
// This is mainly to confirm the db fake has the same behavior.
func TestSuspendedPagination(t *testing.T) {
	t.Parallel()
	t.Skip("This fails when two users are created at the exact same time. The reason is unknown... See: https://github.com/coder/coder/actions/runs/3057047622/jobs/4931863163")
	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

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
			Password:       "MySecurePassword!",
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
	require.Equal(t, expected, page.Users, "expected page")
}

// TestPaginatedUsers creates a list of users, then tries to paginate through
// them using different page sizes.
func TestPaginatedUsers(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	// This test takes longer than a long time.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*4)
	t.Cleanup(cancel)

	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgID := me.OrganizationIDs[0]

	// When 50 users exist
	total := 50
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
			if i%3 == 0 {
				username = strings.ToUpper(username)
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
				Password:       "MySecurePassword!",
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

	// Sorting the users will sort by username.
	sortUsers(allUsers)
	sortUsers(specialUsers)

	gmailSearch := func(request codersdk.UsersRequest) codersdk.UsersRequest {
		request.Search = "gmail"
		return request
	}
	usernameSearch := func(request codersdk.UsersRequest) codersdk.UsersRequest {
		request.Search = "specialuser"
		return request
	}

	tests := []struct {
		name     string
		limit    int
		allUsers []codersdk.User
		opt      func(request codersdk.UsersRequest) codersdk.UsersRequest
	}{
		{name: "all users", limit: 10, allUsers: allUsers},
		{name: "all users", limit: 5, allUsers: allUsers},
		{name: "all users", limit: 3, allUsers: allUsers},
		{name: "gmail search", limit: 3, allUsers: specialUsers, opt: gmailSearch},
		{name: "gmail search", limit: 7, allUsers: specialUsers, opt: gmailSearch},
		{name: "username search", limit: 3, allUsers: specialUsers, opt: usernameSearch},
		{name: "username search", limit: 3, allUsers: specialUsers, opt: usernameSearch},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("%s %d", tt.name, tt.limit), func(t *testing.T) {
			t.Parallel()

			// This test takes longer than a long time.
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*2)
			defer cancel()

			assertPagination(ctx, t, client, tt.limit, tt.allUsers, tt.opt)
		})
	}
}

// Assert pagination will page through the list of all users using the given
// limit for each page. The 'allUsers' is the expected full list to compare
// against.
func assertPagination(ctx context.Context, t *testing.T, client *codersdk.Client, limit int, allUsers []codersdk.User,
	opt func(request codersdk.UsersRequest) codersdk.UsersRequest,
) {
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
	require.Equalf(t, page.Users, allUsers[:limit], "first page, limit=%d", limit)
	count += len(page.Users)

	for {
		if len(page.Users) == 0 {
			break
		}

		afterCursor := page.Users[len(page.Users)-1].ID
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
		require.Equalf(t, page.Users, expected, "next users, after=%s, limit=%d", afterCursor, limit)
		require.Equalf(t, offsetPage.Users, expected, "offset users, offset=%d, limit=%d", count, limit)

		// Also check the before
		prevPage, err := client.Users(ctx, opt(codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Offset: count - limit,
				Limit:  limit,
			},
		}))
		require.NoError(t, err, "prev page")
		require.Equal(t, allUsers[count-limit:count], prevPage.Users, "prev users")
		count += len(page.Users)
	}
}

// sortUsers sorts by (created_at, id)
func sortUsers(users []codersdk.User) {
	slices.SortFunc(users, func(a, b codersdk.User) int {
		return slice.Ascending(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
}

func BenchmarkUsersMe(b *testing.B) {
	client := coderdtest.New(b, nil)
	_ = coderdtest.CreateFirstUser(b, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.User(ctx, codersdk.Me)
		require.NoError(b, err)
	}
}
