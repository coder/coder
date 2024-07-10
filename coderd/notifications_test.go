package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateNotificationsSettings(t *testing.T) {
	t.Parallel()

	t.Run("Permissions denied", func(t *testing.T) {
		t.Parallel()

		api := coderdtest.New(t, nil)
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

		client := coderdtest.New(t, nil)
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
		client := coderdtest.New(t, nil)
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
