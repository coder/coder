package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
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
		ctx := testutil.Context(t, testutil.WaitShort)
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		u, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Equal(t, coderdtest.FirstUserParams.Name, u.Name)
		assert.Equal(t, coderdtest.FirstUserParams.Email, u.Email)
		assert.Equal(t, coderdtest.FirstUserParams.Username, u.Username)
	})

	t.Run("Trial", func(t *testing.T) {
		t.Parallel()
		trialGenerated := make(chan struct{})
		entitlementsRefreshed := make(chan struct{})

		client := coderdtest.New(t, &coderdtest.Options{
			TrialGenerator: func(context.Context, codersdk.LicensorTrialRequest) error {
				close(trialGenerated)
				return nil
			},
			RefreshEntitlements: func(context.Context) error {
				close(entitlementsRefreshed)
				return nil
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.CreateFirstUserRequest{
			Email:    "testuser@coder.com",
			Username: "testuser",
			Name:     "Test User",
			Password: "SomeSecurePassword!",
			Trial:    true,
		}
		_, err := client.CreateFirstUser(ctx, req)
		require.NoError(t, err)

		_ = testutil.TryReceive(ctx, t, trialGenerated)
		_ = testutil.TryReceive(ctx, t, entitlementsRefreshed)
	})
}

func TestFirstUser_OnboardingTelemetry(t *testing.T) {
	t.Parallel()

	t.Run("OnboardingInfoFlowsToSnapshot", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
		client := coderdtest.New(t, &coderdtest.Options{
			TelemetryReporter: fTelemetry,
		})

		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:    "admin@coder.com",
			Username: "admin",
			Password: "SomeSecurePassword!",
			OnboardingInfo: &codersdk.CreateFirstUserOnboardingInfo{
				NewsletterMarketing: false,
				NewsletterReleases:  true,
			},
		})
		require.NoError(t, err)

		snapshot := testutil.TryReceive(ctx, t, fTelemetry.snapshots)
		require.NotNil(t, snapshot.FirstUserOnboarding)
		require.False(t, snapshot.FirstUserOnboarding.NewsletterMarketing)
		require.True(t, snapshot.FirstUserOnboarding.NewsletterReleases)
	})

	t.Run("NilWhenOnboardingInfoOmitted", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
		client := coderdtest.New(t, &coderdtest.Options{
			TelemetryReporter: fTelemetry,
		})

		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:    "admin@coder.com",
			Username: "admin",
			Password: "SomeSecurePassword!",
			// No OnboardingInfo — simulates old CLI or OIDC flow.
		})
		require.NoError(t, err)

		snapshot := testutil.TryReceive(ctx, t, fTelemetry.snapshots)
		require.Nil(t, snapshot.FirstUserOnboarding)
	})

	t.Run("EmptyOnboardingInfoIsNonNilWithZeroFields", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
		client := coderdtest.New(t, &coderdtest.Options{
			TelemetryReporter: fTelemetry,
		})
		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email: "admin@coder.com", Username: "admin",
			Password:       "SomeSecurePassword!",
			OnboardingInfo: &codersdk.CreateFirstUserOnboardingInfo{},
		})
		require.NoError(t, err)
		snapshot := testutil.TryReceive(ctx, t, fTelemetry.snapshots)
		require.NotNil(t, snapshot.FirstUserOnboarding,
			"non-nil OnboardingInfo must produce non-nil telemetry")
		require.False(t, snapshot.FirstUserOnboarding.NewsletterMarketing)
		require.False(t, snapshot.FirstUserOnboarding.NewsletterReleases)
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
		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "test+user-@coder.com",
			Username:        "user",
			Password:        password,
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
		})
		require.NoError(t, err)

		dc.DisablePasswordAuth = serpent.Bool(true)

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

		require.True(t, apiKey.ExpiresAt.After(dbtime.Now().Add(time.Hour*24*6)), "default tokens lasts more than 6 days")
		require.True(t, apiKey.ExpiresAt.Before(dbtime.Now().Add(time.Hour*24*8)), "default tokens lasts less than 8 days")
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
		another, err = client.CreateUserWithOrgs(context.Background(), codersdk.CreateUserRequestWithOrgs{
			Email:           another.Email,
			Username:        another.Username,
			Password:        "SomeSecurePassword!",
			OrganizationIDs: []uuid.UUID{user.OrganizationID},
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
		authz.AssertChecked(t, policy.ActionCreate, rbac.ResourceUser)
		authz.AssertChecked(t, policy.ActionDelete, another)
	})
	t.Run("NoPermission", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, api)
		client, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		err := client.DeleteUser(context.Background(), firstUser.UserID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
	t.Run("HasWorkspaces", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithProvisionerCloser(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, another := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateWorkspace(t, anotherClient, template.ID)
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
	t.Run("CountCheckIncludesAllWorkspaces", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithProvisionerCloser(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Create a target user who will own a workspace
		targetUserClient, targetUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		// Create a User Admin who should not have permission to see the target user's workspace
		userAdminClient, userAdmin := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		// Grant User Admin role to the userAdmin
		userAdmin, err := client.UpdateUserRoles(context.Background(), userAdmin.ID.String(), codersdk.UpdateRoles{
			Roles: []string{rbac.RoleUserAdmin().String()},
		})
		require.NoError(t, err)

		// Create a template and workspace owned by the target user
		version := coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, firstUser.OrganizationID, version.ID)
		_ = coderdtest.CreateWorkspace(t, targetUserClient, template.ID)

		workspaces, err := userAdminClient.Workspaces(context.Background(), codersdk.WorkspaceFilter{
			Owner: targetUser.Username,
		})
		require.NoError(t, err)
		require.Len(t, workspaces.Workspaces, 0)

		// Attempt to delete the target user - this should fail because the
		// user has a workspace not visible to the deleting user.
		err = userAdminClient.DeleteUser(context.Background(), targetUser.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusExpectationFailed, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "has workspaces")
	})
}

func TestNotifyUserStatusChanged(t *testing.T) {
	t.Parallel()

	type expectedNotification struct {
		TemplateID uuid.UUID
		UserID     uuid.UUID
	}

	verifyNotificationDispatched := func(notifyEnq *notificationstest.FakeEnqueuer, expectedNotifications []expectedNotification, member codersdk.User, label string) {
		require.Equal(t, len(expectedNotifications), len(notifyEnq.Sent()))

		// Validate that each expected notification is present in notifyEnq.Sent()
		for _, expected := range expectedNotifications {
			found := false
			for _, sent := range notifyEnq.Sent(notificationstest.WithTemplateID(expected.TemplateID)) {
				if sent.TemplateID == expected.TemplateID &&
					sent.UserID == expected.UserID &&
					slices.Contains(sent.Targets, member.ID) &&
					sent.Labels[label] == member.Username {
					found = true

					require.IsType(t, map[string]any{}, sent.Data["user"])
					userData := sent.Data["user"].(map[string]any)
					require.Equal(t, member.ID, userData["id"])
					require.Equal(t, member.Name, userData["name"])
					require.Equal(t, member.Email, userData["email"])

					break
				}
			}
			require.True(t, found, "Expected notification not found: %+v", expected)
		}
	}

	t.Run("Account suspended", func(t *testing.T) {
		t.Parallel()

		notifyEnq := &notificationstest.FakeEnqueuer{}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		firstUser := coderdtest.CreateFirstUser(t, adminClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, userAdmin := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID, rbac.RoleUserAdmin())

		member, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		notifyEnq.Clear()

		// when
		_, err = adminClient.UpdateUserStatus(context.Background(), member.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)

		// then
		verifyNotificationDispatched(notifyEnq, []expectedNotification{
			{TemplateID: notifications.TemplateUserAccountSuspended, UserID: firstUser.UserID},
			{TemplateID: notifications.TemplateUserAccountSuspended, UserID: userAdmin.ID},
			{TemplateID: notifications.TemplateYourAccountSuspended, UserID: member.ID},
		}, member, "suspended_account_name")
	})

	t.Run("Account reactivated", func(t *testing.T) {
		t.Parallel()

		// given
		notifyEnq := &notificationstest.FakeEnqueuer{}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		firstUser := coderdtest.CreateFirstUser(t, adminClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, userAdmin := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID, rbac.RoleUserAdmin())

		member, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		_, err = adminClient.UpdateUserStatus(context.Background(), member.Username, codersdk.UserStatusSuspended)
		require.NoError(t, err)

		notifyEnq.Clear()

		// when
		_, err = adminClient.UpdateUserStatus(context.Background(), member.Username, codersdk.UserStatusActive)
		require.NoError(t, err)

		// then
		verifyNotificationDispatched(notifyEnq, []expectedNotification{
			{TemplateID: notifications.TemplateUserAccountActivated, UserID: firstUser.UserID},
			{TemplateID: notifications.TemplateUserAccountActivated, UserID: userAdmin.ID},
			{TemplateID: notifications.TemplateYourAccountActivated, UserID: member.ID},
		}, member, "activated_account_name")
	})
}

func TestNotifyDeletedUser(t *testing.T) {
	t.Parallel()

	t.Run("OwnerNotified", func(t *testing.T) {
		t.Parallel()

		// given
		notifyEnq := &notificationstest.FakeEnqueuer{}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		firstUserResponse := coderdtest.CreateFirstUser(t, adminClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		firstUser, err := adminClient.User(ctx, firstUserResponse.UserID.String())
		require.NoError(t, err)

		user, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUserResponse.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		// when
		err = adminClient.DeleteUser(context.Background(), user.ID)
		require.NoError(t, err)

		// then
		require.Len(t, notifyEnq.Sent(), 2)
		// notifyEnq.Sent()[0] is create account event
		require.Equal(t, notifications.TemplateUserAccountDeleted, notifyEnq.Sent()[1].TemplateID)
		require.Equal(t, firstUser.ID, notifyEnq.Sent()[1].UserID)
		require.Contains(t, notifyEnq.Sent()[1].Targets, user.ID)
		require.Equal(t, user.Username, notifyEnq.Sent()[1].Labels["deleted_account_name"])
		require.Equal(t, user.Name, notifyEnq.Sent()[1].Labels["deleted_account_user_name"])
		require.Equal(t, firstUser.Name, notifyEnq.Sent()[1].Labels["initiator"])
	})

	t.Run("UserAdminNotified", func(t *testing.T) {
		t.Parallel()

		// given
		notifyEnq := &notificationstest.FakeEnqueuer{}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		firstUser := coderdtest.CreateFirstUser(t, adminClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, userAdmin := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID, rbac.RoleUserAdmin())

		member, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		// when
		err = adminClient.DeleteUser(context.Background(), member.ID)
		require.NoError(t, err)

		// then
		sent := notifyEnq.Sent()
		require.Len(t, sent, 5)
		// Other notifications:
		// "User admin" account created, "owner" notified
		// "Member" account created, "owner" notified
		// "Member" account created, "user admin" notified

		// "Member" account deleted, "owner" notified
		ownerNotifications := notifyEnq.Sent(func(n *notificationstest.FakeNotification) bool {
			return n.TemplateID == notifications.TemplateUserAccountDeleted &&
				n.UserID == firstUser.UserID &&
				slices.Contains(n.Targets, member.ID) &&
				n.Labels["deleted_account_name"] == member.Username
		})
		require.Len(t, ownerNotifications, 1)

		// "Member" account deleted, "user admin" notified
		adminNotifications := notifyEnq.Sent(func(n *notificationstest.FakeNotification) bool {
			return n.TemplateID == notifications.TemplateUserAccountDeleted &&
				n.UserID == userAdmin.ID &&
				slices.Contains(n.Targets, member.ID) &&
				n.Labels["deleted_account_name"] == member.Username
		})
		require.Len(t, adminNotifications, 1)
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

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{})
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
		_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           me.Email,
			Username:        me.Username,
			Password:        "MySecurePassword!",
			OrganizationIDs: []uuid.UUID{uuid.New()},
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

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{uuid.New()},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
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

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		// User should default to dormant.
		require.Equal(t, codersdk.UserStatusDormant, user.Status)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionCreate, auditor.AuditLogs()[numLogs-1].Action)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs()[numLogs-2].Action)

		require.Len(t, user.OrganizationIDs, 1)
		assert.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	// CreateWithAgentsExperiment verifies that new users
	// are auto-assigned the agents-access role when the
	// experiment is enabled. The experiment-disabled case
	// is implicitly covered by TestInitialRoles, which
	// asserts exactly [owner] with no experiment — it
	// would fail if agents-access leaked through.
	t.Run("CreateWithAgentsExperiment", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentAgents)}
		client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: dv})
		firstUser := coderdtest.CreateFirstUser(t, client)

		ctx := testutil.Context(t, testutil.WaitLong)

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		roles, err := client.UserRoles(ctx, user.Username)
		require.NoError(t, err)
		require.Contains(t, roles.Roles, codersdk.RoleAgentsAccess,
			"new user should have agents-access role when agents experiment is enabled")
	})

	t.Run("CreateWithStatus", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		firstUser := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
			UserStatus:      ptr.Ref(codersdk.UserStatusActive),
		})
		require.NoError(t, err)

		require.Equal(t, codersdk.UserStatusActive, user.Status)

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

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "",
			UserLoginType:   codersdk.LoginTypeNone,
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

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Email:           email,
			Username:        "someone-else",
			Password:        "",
			UserLoginType:   codersdk.LoginTypeOIDC,
		})
		require.NoError(t, err)

		// Try to log in with OIDC.
		userClient, _ := fake.Login(t, client, jwt.MapClaims{
			"email": email,
			"sub":   uuid.NewString(),
		})

		found, err := userClient.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, found.LoginType, codersdk.LoginTypeOIDC)
	})

	t.Run("ServiceAccount/OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-ok",
			UserLoginType:   codersdk.LoginTypeNone,
			ServiceAccount:  true,
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.LoginTypeNone, user.LoginType)
		require.Empty(t, user.Email)
		require.Equal(t, "service-acct-ok", user.Username)
		require.Equal(t, codersdk.UserStatusDormant, user.Status)
	})

	t.Run("ServiceAccount/WithEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-email",
			Email:           "should-not-have@email.com",
			ServiceAccount:  true,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Email cannot be set for service accounts")
	})

	t.Run("ServiceAccount/WithPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-password",
			Password:        "ShouldNotHavePassword123!",
			ServiceAccount:  true,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Password cannot be set for service accounts")
	})

	t.Run("ServiceAccount/WithInvalidLoginType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-login-type",
			UserLoginType:   codersdk.LoginTypePassword,
			ServiceAccount:  true,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Service accounts must use login type 'none'")
	})

	t.Run("ServiceAccount/DefaultLoginType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-default-login",
			ServiceAccount:  true,
		})
		require.NoError(t, err)

		found, err := client.User(ctx, user.ID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.LoginTypeNone, found.LoginType)
		require.Empty(t, found.Email)
	})

	t.Run("NonServiceAccount/WithoutEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "regular-no-email",
			UserLoginType:   codersdk.LoginTypePassword,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("ServiceAccount/MultipleWithoutEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user1, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-multi-1",
			ServiceAccount:  true,
		})
		require.NoError(t, err)
		require.Empty(t, user1.Email)

		user2, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{first.OrganizationID},
			Username:        "service-acct-multi-2",
			ServiceAccount:  true,
		})
		require.NoError(t, err)
		require.Empty(t, user2.Email)
		require.NotEqual(t, user1.ID, user2.ID)
	})
}

func TestNotifyCreatedUser(t *testing.T) {
	t.Parallel()

	t.Run("OwnerNotified", func(t *testing.T) {
		t.Parallel()

		// given
		notifyEnq := &notificationstest.FakeEnqueuer{}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		firstUser := coderdtest.CreateFirstUser(t, adminClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// when
		user, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		// then
		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateUserAccountCreated))
		require.Len(t, sent, 1)
		require.Equal(t, notifications.TemplateUserAccountCreated, sent[0].TemplateID)
		require.Equal(t, firstUser.UserID, sent[0].UserID)
		require.Contains(t, sent[0].Targets, user.ID)
		require.Equal(t, user.Username, sent[0].Labels["created_account_name"])

		require.IsType(t, map[string]any{}, sent[0].Data["user"])
		userData := sent[0].Data["user"].(map[string]any)
		require.Equal(t, user.ID, userData["id"])
		require.Equal(t, user.Name, userData["name"])
		require.Equal(t, user.Email, userData["email"])
	})

	t.Run("UserAdminNotified", func(t *testing.T) {
		t.Parallel()

		// given
		notifyEnq := &notificationstest.FakeEnqueuer{}
		adminClient := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		firstUser := coderdtest.CreateFirstUser(t, adminClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		userAdmin, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "user-admin@user.org",
			Username:        "mr-user-admin",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		_, err = adminClient.UpdateUserRoles(ctx, userAdmin.Username, codersdk.UpdateRoles{
			Roles: []string{
				rbac.RoleUserAdmin().String(),
			},
		})
		require.NoError(t, err)

		// when
		member, err := adminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "another@user.org",
			Username:        "someone-else",
			Password:        "SomeSecurePassword!",
		})
		require.NoError(t, err)

		// then
		sent := notifyEnq.Sent()
		require.Len(t, sent, 3)

		// "User admin" account created, "owner" notified
		ownerNotifiedAboutUserAdmin := notifyEnq.Sent(func(n *notificationstest.FakeNotification) bool {
			return n.TemplateID == notifications.TemplateUserAccountCreated &&
				n.UserID == firstUser.UserID &&
				slices.Contains(n.Targets, userAdmin.ID) &&
				n.Labels["created_account_name"] == userAdmin.Username
		})
		require.Len(t, ownerNotifiedAboutUserAdmin, 1)

		// "Member" account created, "owner" notified
		ownerNotifiedAboutMember := notifyEnq.Sent(func(n *notificationstest.FakeNotification) bool {
			return n.TemplateID == notifications.TemplateUserAccountCreated &&
				n.UserID == firstUser.UserID &&
				slices.Contains(n.Targets, member.ID) &&
				n.Labels["created_account_name"] == member.Username
		})
		require.Len(t, ownerNotifiedAboutMember, 1)

		// "Member" account created, "user admin" notified
		userAdminNotifiedAboutMember := notifyEnq.Sent(func(n *notificationstest.FakeNotification) bool {
			return n.TemplateID == notifications.TemplateUserAccountCreated &&
				n.UserID == userAdmin.ID &&
				slices.Contains(n.Targets, member.ID) &&
				n.Labels["created_account_name"] == member.Username
		})
		require.Len(t, userAdminNotifiedAboutMember, 1)
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
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("ConflictingUsername", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		existentUser, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "bruno@coder.com",
			Username:        "bruno",
			Password:        "SomeSecurePassword!",
			OrganizationIDs: []uuid.UUID{user.OrganizationID},
		})
		require.NoError(t, err)
		_, err = client.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: existentUser.Username,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("UpdateSelf", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for login

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)

		userProfile, err := client.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Username: me.Username + "1",
			Name:     me.Name + "1",
		})
		numLogs++ // add an audit log for user update

		require.NoError(t, err)
		require.Equal(t, me.Username+"1", userProfile.Username)
		require.Equal(t, me.Name+"1", userProfile.Name)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("UpdateSelfAsMember_Name", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		firstUser := coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for login

		memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		numLogs++ // add an audit log for user creation

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		newName := coderdtest.RandomName(t)
		userProfile, err := memberClient.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Name:     newName,
			Username: memberUser.Username,
		})
		numLogs++ // add an audit log for user update
		numLogs++ // add an audit log for API key creation

		require.NoError(t, err)
		require.Equal(t, memberUser.Username, userProfile.Username)
		require.Equal(t, newName, userProfile.Name)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("UpdateSelfAsMember_Username", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})

		firstUser := coderdtest.CreateFirstUser(t, client)
		memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		newUsername := coderdtest.RandomUsername(t)
		_, err := memberClient.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Name:     memberUser.Name,
			Username: newUsername,
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("UpdateMemberAsAdmin_Username", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		adminClient := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		numLogs := len(auditor.AuditLogs())

		adminUser := coderdtest.CreateFirstUser(t, adminClient)
		numLogs++ // add an audit log for login

		_, memberUser := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		numLogs++ // add an audit log for user creation

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		newUsername := coderdtest.RandomUsername(t)
		userProfile, err := adminClient.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Name:     memberUser.Name,
			Username: newUsername,
		})

		numLogs++ // add an audit log for user update
		numLogs++ // add an audit log for API key creation

		require.NoError(t, err)
		require.Equal(t, newUsername, userProfile.Username)
		require.Equal(t, memberUser.Name, userProfile.Name)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("InvalidRealUserName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "john@coder.com",
			Username:        "john",
			Password:        "SomeSecurePassword!",
			OrganizationIDs: []uuid.UUID{user.OrganizationID},
		})
		require.NoError(t, err)
		_, err = client.UpdateUserProfile(ctx, codersdk.Me, codersdk.UpdateUserProfileRequest{
			Name: " Mr Bean", // must not have leading space
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
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

		member, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "coder@coder.com",
			Username:        "coder",
			Password:        "SomeStrongPassword!",
			OrganizationIDs: []uuid.UUID{owner.OrganizationID},
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

	t.Run("AuditorCantUpdateOtherUserPassword", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		auditor, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		member, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "coder@coder.com",
			Username:        "coder",
			Password:        "SomeStrongPassword!",
			OrganizationIDs: []uuid.UUID{owner.OrganizationID},
		})
		require.NoError(t, err, "create member")

		err = auditor.UpdateUserPassword(ctx, member.ID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "SomeNewStrongPassword!",
		})
		require.Error(t, err, "auditor should not be able to update member password")
		require.ErrorContains(t, err, "unexpected status code 404: Resource not found or you do not have access to this resource")
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
		require.ErrorContains(t, err, "Old password is required.")
	})

	t.Run("AuditorCantTellIfPasswordIncorrect", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		adminClient := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})

		adminUser := coderdtest.CreateFirstUser(t, adminClient)

		auditorClient, _ := coderdtest.CreateAnotherUser(t, adminClient,
			adminUser.OrganizationID,
			rbac.RoleAuditor(),
		)

		_, memberUser := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
		numLogs := len(auditor.AuditLogs())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := auditorClient.UpdateUserPassword(ctx, memberUser.ID.String(), codersdk.UpdateUserPasswordRequest{
			Password: "MySecurePassword!",
		})
		numLogs++ // add an audit log for user update

		require.Error(t, err, "auditors shouldn't be able to update passwords")
		var httpErr *codersdk.Error
		require.True(t, xerrors.As(err, &httpErr))
		// ensure that the error we get is "not found" and not "bad request"
		require.Equal(t, http.StatusNotFound, httpErr.StatusCode())

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
		require.Equal(t, int32(http.StatusNotFound), auditor.AuditLogs()[numLogs-1].StatusCode)
	})

	t.Run("AdminCantUpdateOwnPasswordWithoutOldPassword", func(t *testing.T) {
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

		require.Error(t, err, "admin should not be able to update own password without providing old password")
		require.ErrorContains(t, err, "Old password is required.")

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("ValidateUserPassword", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})

		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.ValidateUserPassword(ctx, codersdk.ValidateUserPasswordRequest{
			Password: "MySecurePassword!",
		})

		require.NoError(t, err, "users shoud be able to validate complexity of a potential new password")
		require.True(t, resp.Valid)
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
			OldPassword: "SomeSecurePassword!",
			Password:    "MyNewSecurePassword!",
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

// TestInitialRoles ensures the starting roles for the first user are correct.
func TestInitialRoles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)

	roles, err := client.UserRoles(ctx, codersdk.Me)
	require.NoError(t, err)
	require.ElementsMatch(t, roles.Roles, []string{
		codersdk.RoleOwner,
	}, "should be a member and admin")

	require.ElementsMatch(t, roles.OrganizationRoles[first.OrganizationID], []string{}, "should be a member")
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
	anotherUser, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		Email:           "coder@coder.com",
		Username:        "coder",
		Password:        "SomeStrongPassword!",
		OrganizationIDs: []uuid.UUID{me.OrganizationID},
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

	// Single instance shared across all sub-tests. All lookups
	// are read-only against the first user.
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)

	t.Run("ByMe", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID, user.ID)
		require.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("ByID", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, firstUser.UserID.String())
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID, user.ID)
		require.Equal(t, firstUser.OrganizationID, user.OrganizationIDs[0])
	})

	t.Run("ByUsername", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		exp, err := client.User(ctx, firstUser.UserID.String())
		require.NoError(t, err)

		user, err := client.User(ctx, exp.Username)
		require.NoError(t, err)
		require.Equal(t, exp.ID, user.ID)
	})
}

func TestGetUsersFilter(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		OIDCConfig: &coderd.OIDCConfig{
			AllowSignups: true,
		},
	})
	_ = coderdtest.CreateFirstUser(t, client)

	setupCtx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	coderdtest.UsersFilter(setupCtx, t, client, api.Database, nil, func(testCtx context.Context, req codersdk.UsersRequest) []codersdk.ReducedUser {
		res, err := client.Users(testCtx, req)
		require.NoError(t, err)
		reduced := make([]codersdk.ReducedUser, len(res.Users))
		for i, user := range res.Users {
			reduced[i] = user.ReducedUser
		}
		return reduced
	})
}

func TestGetUsersPagination(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	coderdtest.UsersPagination(ctx, t, client, nil, func(req codersdk.UsersRequest) ([]codersdk.ReducedUser, int) {
		res, err := client.Users(ctx, req)
		require.NoError(t, err)
		reduced := make([]codersdk.ReducedUser, len(res.Users))
		for i, user := range res.Users {
			reduced[i] = user.ReducedUser
		}
		return reduced, res.Count
	})
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

func TestUserTerminalFont(t *testing.T) {
	t.Parallel()

	// Single instance shared across all sub-tests. Each sub-test
	// creates its own non-admin user for isolation.
	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)

	t.Run("valid font", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		initial, err := client.GetUserAppearanceSettings(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, codersdk.TerminalFontName(""), initial.TerminalFont)

		// when
		updated, err := client.UpdateUserAppearanceSettings(ctx, codersdk.Me, codersdk.UpdateUserAppearanceSettingsRequest{
			ThemePreference: "light",
			TerminalFont:    "fira-code",
		})
		require.NoError(t, err)

		// then
		require.Equal(t, codersdk.TerminalFontFiraCode, updated.TerminalFont)
	})

	t.Run("unsupported font", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		initial, err := client.GetUserAppearanceSettings(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, codersdk.TerminalFontName(""), initial.TerminalFont)

		// when
		_, err = client.UpdateUserAppearanceSettings(ctx, codersdk.Me, codersdk.UpdateUserAppearanceSettingsRequest{
			ThemePreference: "light",
			TerminalFont:    "foobar",
		})

		// then
		require.Error(t, err)
	})

	t.Run("undefined font is not ok", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		initial, err := client.GetUserAppearanceSettings(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, codersdk.TerminalFontName(""), initial.TerminalFont)

		// when
		_, err = client.UpdateUserAppearanceSettings(ctx, codersdk.Me, codersdk.UpdateUserAppearanceSettingsRequest{
			ThemePreference: "light",
			TerminalFont:    "",
		})

		// then
		require.Error(t, err)
	})
}

func TestUserTaskNotificationAlertDismissed(t *testing.T) {
	t.Parallel()

	// Single instance shared across all sub-tests. Each sub-test
	// creates its own non-admin user for isolation.
	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)

	t.Run("defaults to false", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// When: getting user preference settings for a user
		settings, err := client.GetUserPreferenceSettings(ctx, codersdk.Me)
		require.NoError(t, err)

		// Then: the task notification alert dismissed should default to false
		require.False(t, settings.TaskNotificationAlertDismissed)
	})

	t.Run("update to true", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// When: user dismisses the task notification alert
		updated, err := client.UpdateUserPreferenceSettings(ctx, codersdk.Me, codersdk.UpdateUserPreferenceSettingsRequest{
			TaskNotificationAlertDismissed: true,
		})
		require.NoError(t, err)

		// Then: the setting is updated to true
		require.True(t, updated.TaskNotificationAlertDismissed)
	})

	t.Run("update to false", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// Given: user has dismissed the task notification alert
		_, err := client.UpdateUserPreferenceSettings(ctx, codersdk.Me, codersdk.UpdateUserPreferenceSettingsRequest{
			TaskNotificationAlertDismissed: true,
		})
		require.NoError(t, err)

		// When: the task notification alert dismissal is cleared
		// (e.g., when user enables a task notification in the UI settings)
		updated, err := client.UpdateUserPreferenceSettings(ctx, codersdk.Me, codersdk.UpdateUserPreferenceSettingsRequest{
			TaskNotificationAlertDismissed: false,
		})
		require.NoError(t, err)

		// Then: the setting is updated to false
		require.False(t, updated.TaskNotificationAlertDismissed)
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

		newUser, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "test@coder.com",
			Username:        "someone",
			Password:        "MySecurePassword!",
			OrganizationIDs: []uuid.UUID{user.OrganizationID},
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
		coderdtest.CreateWorkspace(t, client, template.ID)

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
	newUser, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		Email:           "test@coder.com",
		Username:        "someone",
		Password:        "MySecurePassword!",
		OrganizationIDs: []uuid.UUID{user.OrganizationID},
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
		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           email,
			Username:        username,
			Password:        "MySecurePassword!",
			OrganizationIDs: []uuid.UUID{orgID},
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

func TestUserAutofillParameters(t *testing.T) {
	t.Parallel()
	t.Run("NotSelf", func(t *testing.T) {
		t.Parallel()
		client1, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})

		u1 := coderdtest.CreateFirstUser(t, client1)

		client2, u2 := coderdtest.CreateAnotherUser(t, client1, u1.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		db := api.Database

		version := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
			CreatedBy:      u1.UserID,
			OrganizationID: u1.OrganizationID,
		}).Params(database.TemplateVersionParameter{
			Name:     "param",
			Required: true,
		}).Do()

		_, err := client2.UserAutofillParameters(
			ctx,
			u1.UserID.String(),
			version.Template.ID,
		)

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())

		// u1 should be able to read u2's parameters as u1 is site admin.
		_, err = client1.UserAutofillParameters(
			ctx,
			u2.ID.String(),
			version.Template.ID,
		)
		require.NoError(t, err)
	})

	t.Run("FindsParameters", func(t *testing.T) {
		t.Parallel()
		client1, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})

		u1 := coderdtest.CreateFirstUser(t, client1)

		client2, u2 := coderdtest.CreateAnotherUser(t, client1, u1.OrganizationID)

		db := api.Database

		version := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
			CreatedBy:      u1.UserID,
			OrganizationID: u1.OrganizationID,
		}).Params(database.TemplateVersionParameter{
			Name:     "param",
			Required: true,
		},
			database.TemplateVersionParameter{
				Name:      "param2",
				Ephemeral: true,
			},
		).Do()

		dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        u2.ID,
			TemplateID:     version.Template.ID,
			OrganizationID: u1.OrganizationID,
		}).Params(
			database.WorkspaceBuildParameter{
				Name:  "param",
				Value: "foo",
			},
			database.WorkspaceBuildParameter{
				Name:  "param2",
				Value: "bar",
			},
		).Do()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Use client2 since client1 is site admin, so
		// we don't get good coverage on RBAC working.
		params, err := client2.UserAutofillParameters(
			ctx,
			u2.ID.String(),
			version.Template.ID,
		)
		require.NoError(t, err)

		require.Equal(t, 1, len(params))

		require.Equal(t, "param", params[0].Name)
		require.Equal(t, "foo", params[0].Value)

		// Verify that latest parameter value is returned.
		dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: u1.OrganizationID,
			OwnerID:        u2.ID,
			TemplateID:     version.Template.ID,
		}).Params(
			database.WorkspaceBuildParameter{
				Name:  "param",
				Value: "foo_new",
			},
		).Do()

		params, err = client2.UserAutofillParameters(
			ctx,
			u2.ID.String(),
			version.Template.ID,
		)
		require.NoError(t, err)

		require.Equal(t, 1, len(params))

		require.Equal(t, "param", params[0].Name)
		require.Equal(t, "foo_new", params[0].Value)
	})
}

// TestPaginatedUsers creates a list of users, then tries to paginate through
// them using different page sizes.
func TestPaginatedUsers(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, nil)
	coderdtest.CreateFirstUser(t, client)

	// This test takes longer than a long time.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*4)
	t.Cleanup(cancel)

	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	// When 50 users exist
	total := 50
	allUsers := make([]database.User, total+1)
	allUsers[0] = database.User{
		Email:    me.Email,
		Username: me.Username,
	}
	specialUsers := make([]database.User, total/2)

	eg, _ := errgroup.WithContext(ctx)
	// Create users
	for i := 0; i < total; i++ {
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

			// We used to use the API to ceate users, but that is slow.
			// Instead, we create them directly in the database now
			// to prevent timeout flakes.
			newUser := dbgen.User(t, db, database.User{
				Email:    email,
				Username: username,
			})
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
	sortDatabaseUsers(allUsers)
	sortDatabaseUsers(specialUsers)

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
		allUsers []database.User
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
func assertPagination(ctx context.Context, t *testing.T, client *codersdk.Client, limit int, allUsers []database.User,
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
	require.Equalf(t, onlyUsernames(page.Users), onlyUsernames(allUsers[:limit]), "first page, limit=%d", limit)
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

		var expected []database.User
		if count+limit > len(allUsers) {
			expected = allUsers[count:]
		} else {
			expected = allUsers[count : count+limit]
		}
		require.Equalf(t, onlyUsernames(page.Users), onlyUsernames(expected), "next users, after=%s, limit=%d", afterCursor, limit)
		require.Equalf(t, onlyUsernames(offsetPage.Users), onlyUsernames(expected), "offset users, offset=%d, limit=%d", count, limit)

		// Also check the before
		prevPage, err := client.Users(ctx, opt(codersdk.UsersRequest{
			Pagination: codersdk.Pagination{
				Offset: count - limit,
				Limit:  limit,
			},
		}))
		require.NoError(t, err, "prev page")
		require.Equal(t, onlyUsernames(allUsers[count-limit:count]), onlyUsernames(prevPage.Users), "prev users")
		count += len(page.Users)
	}
}

// sortUsers sorts by (created_at, id)
func sortUsers(users []codersdk.User) {
	slices.SortFunc(users, func(a, b codersdk.User) int {
		return slice.Ascending(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
}

func sortDatabaseUsers(users []database.User) {
	slices.SortFunc(users, func(a, b database.User) int {
		return slice.Ascending(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
}

func onlyUsernames[U codersdk.User | database.User](users []U) []string {
	var out []string
	for _, u := range users {
		switch u := (any(u)).(type) {
		case codersdk.User:
			out = append(out, u.Username)
		case database.User:
			out = append(out, u.Username)
		}
	}
	return out
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
