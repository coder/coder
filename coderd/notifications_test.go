package coderd_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
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

func TestUpdateNotificationsSettings(t *testing.T) {
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

func TestNotificationPreferences(t *testing.T) {
	t.Parallel()

	t.Run("Initial state", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member in its initial state.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: calling the API.
		prefs, err := memberClient.GetUserNotificationPreferences(ctx, member.ID)
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
		_, err := member2Client.GetUserNotificationPreferences(ctx, member1.ID)

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
		prefs, err := api.GetUserNotificationPreferences(ctx, member.ID)

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
		prefs, err := api.UpdateUserNotificationPreferences(ctx, member.ID, codersdk.UpdateUserNotificationPreferences{
			TemplateDisabledMap: map[string]bool{
				notifications.TemplateWorkspaceMarkedForDeletion.String(): true,
			},
		})

		// Then: the request should succeed and the user should be able to query their own preferences to see the same result.
		require.NoError(t, err)
		require.Len(t, prefs, 1)

		memberPrefs, err := memberClient.GetUserNotificationPreferences(ctx, member.ID)
		require.NoError(t, err)
		require.Len(t, memberPrefs, 1)
		require.Equal(t, prefs[0].NotificationTemplateID, memberPrefs[0].NotificationTemplateID)
		require.Equal(t, prefs[0].Disabled, memberPrefs[0].Disabled)
	})

	t.Run("Add preferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member with no preferences.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		prefs, err := memberClient.GetUserNotificationPreferences(ctx, member.ID)
		require.NoError(t, err)
		require.Len(t, prefs, 0)

		// When: attempting to add new preferences.
		template := notifications.TemplateWorkspaceDeleted
		prefs, err = memberClient.UpdateUserNotificationPreferences(ctx, member.ID, codersdk.UpdateUserNotificationPreferences{
			TemplateDisabledMap: map[string]bool{
				template.String(): true,
			},
		})

		// Then: the returning preferences should be set as expected.
		require.NoError(t, err)
		require.Len(t, prefs, 1)
		require.Equal(t, prefs[0].NotificationTemplateID, template)
		require.True(t, prefs[0].Disabled)
	})

	t.Run("Modify preferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api := coderdtest.New(t, createOpts(t))
		firstUser := coderdtest.CreateFirstUser(t, api)

		// Given: a member with preferences.
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		prefs, err := memberClient.UpdateUserNotificationPreferences(ctx, member.ID, codersdk.UpdateUserNotificationPreferences{
			TemplateDisabledMap: map[string]bool{
				notifications.TemplateWorkspaceDeleted.String(): true,
				notifications.TemplateWorkspaceDormant.String(): true,
			},
		})
		require.NoError(t, err)
		require.Len(t, prefs, 2)

		// When: attempting to modify their preferences.
		prefs, err = memberClient.UpdateUserNotificationPreferences(ctx, member.ID, codersdk.UpdateUserNotificationPreferences{
			TemplateDisabledMap: map[string]bool{
				notifications.TemplateWorkspaceDeleted.String(): true,
				notifications.TemplateWorkspaceDormant.String(): false, // <--- this one was changed
			},
		})
		require.NoError(t, err)
		require.Len(t, prefs, 2)

		// Then: the modified preferences should be set as expected.
		var found bool
		for _, p := range prefs {
			switch p.NotificationTemplateID {
			case notifications.TemplateWorkspaceDormant:
				found = true
				require.False(t, p.Disabled)
			case notifications.TemplateWorkspaceDeleted:
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
	webhookOpts.DeploymentValues.Notifications.Method = serpent.String(database.NotificationMethodWebhook)

	tests := []struct {
		name            string
		opts            *coderdtest.Options
		expectedDefault string
	}{
		{
			name:            "default",
			opts:            defaultOpts,
			expectedDefault: string(database.NotificationMethodSmtp),
		},
		{
			name:            "non-default",
			opts:            webhookOpts,
			expectedDefault: string(database.NotificationMethodWebhook),
		},
	}

	var allMethods []string
	for _, nm := range database.AllNotificationMethodValues() {
		if nm == database.NotificationMethodInbox {
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

			resp, err := api.GetNotificationDispatchMethods(ctx)
			require.NoError(t, err)

			slices.Sort(resp.AvailableNotificationMethods)
			require.EqualValues(t, resp.AvailableNotificationMethods, allMethods)
			require.Equal(t, tc.expectedDefault, resp.DefaultNotificationMethod)
		})
	}
}

func TestNotificationTest(t *testing.T) {
	t.Parallel()

	t.Run("OwnerCanSendTestNotification", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &notificationstest.FakeEnqueuer{}
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
		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTestNotification))
		require.Len(t, sent, 1)
	})

	t.Run("MemberCannotSendTestNotification", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		notifyEnq := &notificationstest.FakeEnqueuer{}
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

		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTestNotification))
		require.Len(t, sent, 0)
	})
}

const (
	// These are valid keys for a web push subscription.
	// DO NOT REUSE THESE IN ANY REAL CODE.
	validEndpointAuthKey   = "zqbxT6JKstKSY9JKibZLSQ=="
	validEndpointP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk="
)

func TestWebpushSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWebPush)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: dv,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	handlerCalled := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		handlerCalled <- true
	}))
	defer server.Close()

	err := memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  server.URL,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.NoError(t, err, "create webpush subscription")
	require.True(t, <-handlerCalled, "handler should have been called")

	err = memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test web push notification")
	require.True(t, <-handlerCalled, "handler should have been called again")

	err = memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
		Endpoint: server.URL,
	})
	require.NoError(t, err, "delete webpush subscription")
}
