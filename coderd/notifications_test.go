package coderd_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/coderd/alerts/alertstest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func createOpts(t *testing.T) *coderdtest.Options {
	t.Helper()

	dt := coderdtest.DeploymentValues(t)
	return &coderdtest.Options{
		DeploymentValues: dt,
	}
}

func TestUpdateAlertsSettings(t *testing.T) {
	t.Parallel()

	t.Run("Permissions denied", func(t *testing.T) {
		t.Parallel()

		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)
		anotherClient, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// given
		expected := codersdk.NotificationsSettings{
			NotifierPaused: true,
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		// when
		err := anotherClient.PutNotificationsSettings(ctx, expected)

		// then
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("Settings modified", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, createOpts(t))
		_ = coderdtest.CreateFirstUser(t, client)

		// given
		expected := codersdk.NotificationsSettings{
			NotifierPaused: true,
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		// when
		err := client.PutNotificationsSettings(ctx, expected)
		require.NoError(t, err)

		// then
		actual, err := client.GetNotificationsSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("Settings not modified", func(t *testing.T) {
		t.Parallel()

		// Empty state: notifications Settings are undefined now (default).
		client := coderdtest.New(t, createOpts(t))
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Change the state: pause notifications
		err := client.PutNotificationsSettings(ctx, codersdk.NotificationsSettings{
			NotifierPaused: true,
		})
		require.NoError(t, err)

		// Verify the state: notifications are paused.
		actual, err := client.GetNotificationsSettings(ctx)
		require.NoError(t, err)
		require.True(t, actual.NotifierPaused)

		// Change the stage again: notifications are paused.
		expected := actual
		err = client.PutNotificationsSettings(ctx, codersdk.NotificationsSettings{
			NotifierPaused: true,
		})
		require.NoError(t, err)

		// Verify the state: notifications are still paused, and there is no error returned.
		actual, err = client.GetNotificationsSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected.NotifierPaused, actual.NotifierPaused)
	})
}

func TestAlertPreferences(t *testing.T) {
	t.Parallel()

	t.Run("Initial state", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member in its initial state.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: calling the API.
		prefs, err := memberClient.GetUserAlertPreferences(ctx, member.ID)
		require.NoError(t, err)

		// Then: no preferences will be returned.
		require.Len(t, prefs, 0)
	})

	t.Run("Insufficient permissions", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: 2 members.
		_, member1 := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		member2Client, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: attempting to retrieve the preferences of another member.
		_, err := member2Client.GetUserAlertPreferences(ctx, member1.ID)

		// Then: the API should reject the request.
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		// NOTE: ExtractUserParam gets in the way here, and returns a 400 Bad Request instead of a 403 Forbidden.
		// This is not ideal, and we should probably change this behavior.
		require.Equal(t, http.StatusBadRequest, sdkError.StatusCode())
	})

	t.Run("Admin may read any users' preferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member.
		_, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: attempting to retrieve the preferences of another member as an admin.
		prefs, err := api.GetUserAlertPreferences(ctx, member.ID)

		// Then: the API should not reject the request.
		require.NoError(t, err)
		require.Len(t, prefs, 0)
	})

	t.Run("Admin may update any users' preferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: attempting to modify and subsequently retrieve the preferences of another member as an admin.
		prefs, err := api.UpdateUserAlertPreferences(ctx, member.ID, codersdk.UpdateUserAlertPreferences{
			TemplateDisabledMap: map[string]bool{
				alerts.TemplateWorkspaceMarkedForDeletion.String(): true,
			},
		})

		// Then: the request should succeed and the user should be able to query their own preferences to see the same result.
		require.NoError(t, err)
		require.Len(t, prefs, 1)

		memberPrefs, err := memberClient.GetUserAlertPreferences(ctx, member.ID)
		require.NoError(t, err)
		require.Len(t, memberPrefs, 1)
		require.Equal(t, prefs[0].AlertTemplateID, memberPrefs[0].AlertTemplateID)
		require.Equal(t, prefs[0].Disabled, memberPrefs[0].Disabled)
	})

	t.Run("Add preferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member with no preferences.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		prefs, err := memberClient.GetUserAlertPreferences(ctx, member.ID)
		require.NoError(t, err)
		require.Len(t, prefs, 0)

		// When: attempting to add new preferences.
		template := alerts.TemplateWorkspaceDeleted
		prefs, err = memberClient.UpdateUserAlertPreferences(ctx, member.ID, codersdk.UpdateUserAlertPreferences{
			TemplateDisabledMap: map[string]bool{
				template.String(): true,
			},
		})

		// Then: the returning preferences should be set as expected.
		require.NoError(t, err)
		require.Len(t, prefs, 1)
		require.Equal(t, prefs[0].AlertTemplateID, template)
		require.True(t, prefs[0].Disabled)
	})

	t.Run("Modify preferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member with preferences.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		prefs, err := memberClient.UpdateUserAlertPreferences(ctx, member.ID, codersdk.UpdateUserAlertPreferences{
			TemplateDisabledMap: map[string]bool{
				alerts.TemplateWorkspaceDeleted.String(): true,
				alerts.TemplateWorkspaceDormant.String(): true,
			},
		})
		require.NoError(t, err)
		require.Len(t, prefs, 2)

		// When: attempting to modify their preferences.
		prefs, err = memberClient.UpdateUserAlertPreferences(ctx, member.ID, codersdk.UpdateUserAlertPreferences{
			TemplateDisabledMap: map[string]bool{
				alerts.TemplateWorkspaceDeleted.String(): true,
				alerts.TemplateWorkspaceDormant.String(): false, // <--- this one was changed
			},
		})
		require.NoError(t, err)
		require.Len(t, prefs, 2)

		// Then: the modified preferences should be set as expected.
		var found bool
		for _, p := range prefs {
			switch p.AlertTemplateID {
			case alerts.TemplateWorkspaceDormant:
				found = true
				require.False(t, p.Disabled)
			case alerts.TemplateWorkspaceDeleted:
				require.True(t, p.Disabled)
			}
		}
		require.True(t, found, "dormant notification preference was not found")
	})
}

func TestNotificationDispatchMethods(t *testing.T) {
	t.Parallel()

	defaultOpts := createOpts(t)
	webhookOpts := createOpts(t)
	webhookOpts.DeploymentValues.Notifications.Method = serpent.String(database.AlertMethodWebhook)

	tests := []struct {
		name            string
		opts            *coderdtest.Options
		expectedDefault string
	}{
		{
			name:            "default",
			opts:            defaultOpts,
			expectedDefault: string(database.AlertMethodSmtp),
		},
		{
			name:            "non-default",
			opts:            webhookOpts,
			expectedDefault: string(database.AlertMethodWebhook),
		},
	}

	var allMethods []string
	for _, nm := range database.AllAlertMethodValues() {
		if nm == database.AlertMethodInbox {
			continue
		}
		allMethods = append(allMethods, string(nm))
	}
	slices.Sort(allMethods)

	// nolint:paralleltest // Not since Go v1.22.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitSuperLong)
			api := coderdtest.New(t, tc.opts)
			_ = coderdtest.CreateFirstUser(t, api)

			resp, err := api.GetAlertDispatchMethods(ctx)
			require.NoError(t, err)

			slices.Sort(resp.AvailableAlertMethods)
			require.EqualValues(t, resp.AvailableAlertMethods, allMethods)
			require.Equal(t, tc.expectedDefault, resp.DefaultAlertMethod)
		})
	}
}

func TestNotificationTest(t *testing.T) {
	t.Parallel()

	t.Run("OwnerCanSendTestNotification", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &alertstest.FakeEnqueuer{}
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A user with owner permissions.
		_ = coderdtest.CreateFirstUser(t, ownerClient)

		// When: They attempt to send a test notification.
		err := ownerClient.PostTestNotification(ctx)
		require.NoError(t, err)

		// Then: We expect a notification to have been sent.
		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateTestNotification))
		require.Len(t, sent, 1)
	})

	t.Run("MemberCannotSendTestNotification", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &alertstest.FakeEnqueuer{}
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A user without owner permissions.
		ownerUser := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, ownerUser.OrganizationID)

		// When: They attempt to send a test notification.
		err := memberClient.PostTestNotification(ctx)

		// Then: We expect a forbidden error with no notifications sent
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateTestNotification))
		require.Len(t, sent, 0)
	})
}

func TestCustomNotification(t *testing.T) {
	t.Parallel()

	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &alertstest.FakeEnqueuer{}
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A member user
		ownerUser := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, ownerUser.OrganizationID)

		// When: The member user attempts to send a custom notification with empty title and message
		err := memberClient.PostCustomNotification(ctx, codersdk.CustomNotificationRequest{
			Content: &codersdk.CustomNotificationContent{
				Title:   "",
				Message: "",
			},
		})

		// Then: a bad request error is expected with no notifications sent
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusBadRequest, sdkError.StatusCode())
		require.Equal(t, "Invalid request body", sdkError.Message)

		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateTestNotification))
		require.Len(t, sent, 0)
	})

	t.Run("SystemUserNotAllowed", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &alertstest.FakeEnqueuer{}
		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A system user (prebuilds system user)
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID:    database.PrebuildsSystemUserID,
			LoginType: database.LoginTypeNone,
		})
		systemUserClient := codersdk.New(ownerClient.URL)
		systemUserClient.SetSessionToken(token)

		// When: The system user attempts to send a custom notification
		err := systemUserClient.PostCustomNotification(ctx, codersdk.CustomNotificationRequest{
			Content: &codersdk.CustomNotificationContent{
				Title:   "Custom Title",
				Message: "Custom Message",
			},
		})

		// Then: a forbidden error is expected with no notifications sent
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusForbidden, sdkError.StatusCode())
		require.Equal(t, "Forbidden", sdkError.Message)

		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateTestNotification))
		require.Len(t, sent, 0)
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &alertstest.FakeEnqueuer{}
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A member user
		ownerUser := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, memberUser := coderdtest.CreateAnotherUser(t, ownerClient, ownerUser.OrganizationID)

		// When: The member user attempts to send a custom notification
		err := memberClient.PostCustomNotification(ctx, codersdk.CustomNotificationRequest{
			Content: &codersdk.CustomNotificationContent{
				Title:   "Custom Title",
				Message: "Custom Message",
			},
		})
		require.NoError(t, err)

		// Then: we expect a custom notification to be sent to the member user
		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateCustomNotification))
		require.Len(t, sent, 1)
		require.Equal(t, memberUser.ID, sent[0].UserID)
		require.Len(t, sent[0].Labels, 2)
		require.Equal(t, "Custom Title", sent[0].Labels["custom_title"])
		require.Equal(t, "Custom Message", sent[0].Labels["custom_message"])
		require.Equal(t, memberUser.ID.String(), sent[0].CreatedBy)
	})
}
