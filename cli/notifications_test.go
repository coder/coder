package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
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

func TestNotifications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		command      string
		expectPaused bool
	}{
		{
			name:         "PauseNotifications",
			command:      "pause",
			expectPaused: true,
		},
		{
			name:         "ResumeNotifications",
			command:      "resume",
			expectPaused: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// given
			ownerClient, db := coderdtest.NewWithDatabase(t, createOpts(t))
			_ = coderdtest.CreateFirstUser(t, ownerClient)

			// when
			inv, root := clitest.New(t, "notifications", tt.command)
			clitest.SetupConfig(t, ownerClient, root)

			var buf bytes.Buffer
			inv.Stdout = &buf
			err := inv.Run()
			require.NoError(t, err)

			// then
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			t.Cleanup(cancel)
			settingsJSON, err := db.GetNotificationsSettings(ctx)
			require.NoError(t, err)

			var settings codersdk.NotificationsSettings
			err = json.Unmarshal([]byte(settingsJSON), &settings)
			require.NoError(t, err)
			require.Equal(t, tt.expectPaused, settings.NotifierPaused)
		})
	}
}

func TestPauseNotifications_RegularUser(t *testing.T) {
	t.Parallel()

	// given
	ownerClient, db := coderdtest.NewWithDatabase(t, createOpts(t))
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	anotherClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	// when
	inv, root := clitest.New(t, "notifications", "pause")
	clitest.SetupConfig(t, anotherClient, root)

	var buf bytes.Buffer
	inv.Stdout = &buf
	err := inv.Run()
	var sdkError *codersdk.Error
	require.Error(t, err)
	require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
	assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	assert.Contains(t, sdkError.Message, "Forbidden.")

	// then
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	t.Cleanup(cancel)
	settingsJSON, err := db.GetNotificationsSettings(ctx)
	require.NoError(t, err)

	var settings codersdk.NotificationsSettings
	err = json.Unmarshal([]byte(settingsJSON), &settings)
	require.NoError(t, err)
	require.False(t, settings.NotifierPaused) // still running
}

func TestNotificationsTest(t *testing.T) {
	t.Parallel()

	t.Run("OwnerCanSendTestNotification", func(t *testing.T) {
		t.Parallel()

		notifyEnq := &alertstest.FakeEnqueuer{}

		// Given: An owner user.
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})
		_ = coderdtest.CreateFirstUser(t, ownerClient)

		// When: The owner user attempts to send the test notification.
		inv, root := clitest.New(t, "notifications", "test")
		clitest.SetupConfig(t, ownerClient, root)

		// Then: we expect a notification to be sent.
		err := inv.Run()
		require.NoError(t, err)

		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateTestNotification))
		require.Len(t, sent, 1)
	})

	t.Run("MemberCannotSendTestNotification", func(t *testing.T) {
		t.Parallel()

		notifyEnq := &alertstest.FakeEnqueuer{}

		// Given: A member user.
		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})
		ownerUser := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, ownerUser.OrganizationID)

		// When: The member user attempts to send the test notification.
		inv, root := clitest.New(t, "notifications", "test")
		clitest.SetupConfig(t, memberClient, root)

		// Then: we expect an error and no notifications to be sent.
		err := inv.Run()
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())

		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateTestNotification))
		require.Len(t, sent, 0)
	})
}

func TestCustomNotifications(t *testing.T) {
	t.Parallel()

	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()

		notifyEnq := &alertstest.FakeEnqueuer{}

		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A member user
		ownerUser := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, ownerUser.OrganizationID)

		// When: The member user attempts to send a custom notification with empty title and message
		inv, root := clitest.New(t, "notifications", "custom", "", "")
		clitest.SetupConfig(t, memberClient, root)

		// Then: an error is expected with no notifications sent
		err := inv.Run()
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
		inv, root := clitest.New(t, "notifications", "custom", "Custom Title", "Custom Message")
		clitest.SetupConfig(t, systemUserClient, root)

		// Then: an error is expected with no notifications sent
		err := inv.Run()
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

		notifyEnq := &alertstest.FakeEnqueuer{}

		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues:      coderdtest.DeploymentValues(t),
			NotificationsEnqueuer: notifyEnq,
		})

		// Given: A member user
		ownerUser := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, memberUser := coderdtest.CreateAnotherUser(t, ownerClient, ownerUser.OrganizationID)

		// When: The member user attempts to send a custom notification
		inv, root := clitest.New(t, "notifications", "custom", "Custom Title", "Custom Message")
		clitest.SetupConfig(t, memberClient, root)

		// Then: we expect a custom notification to be sent to the member user
		err := inv.Run()
		require.NoError(t, err)

		sent := notifyEnq.Sent(alertstest.WithTemplateID(alerts.TemplateCustomNotification))
		require.Len(t, sent, 1)
		require.Equal(t, memberUser.ID, sent[0].UserID)
		require.Len(t, sent[0].Labels, 2)
		require.Equal(t, "Custom Title", sent[0].Labels["custom_title"])
		require.Equal(t, "Custom Message", sent[0].Labels["custom_message"])
		require.Equal(t, memberUser.ID.String(), sent[0].CreatedBy)
	})
}
