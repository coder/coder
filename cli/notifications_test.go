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
	"github.com/coder/coder/v2/coderd/coderdtest"
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
		tt := tt
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

		notifyEnq := &notificationstest.FakeEnqueuer{}

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

		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTestNotification))
		require.Len(t, sent, 1)
	})

	t.Run("MemberCannotSendTestNotification", func(t *testing.T) {
		t.Parallel()

		notifyEnq := &notificationstest.FakeEnqueuer{}

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

		sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTestNotification))
		require.Len(t, sent, 0)
	})
}
