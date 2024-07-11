package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPauseNotifications(t *testing.T) {
	t.Parallel()

	// given
	ownerClient, db := coderdtest.NewWithDatabase(t, nil)
	_ = coderdtest.CreateFirstUser(t, ownerClient)

	// when
	inv, root := clitest.New(t, "notifications", "pause")
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
	require.True(t, settings.NotifierPaused)
}

func TestPauseNotifications_RegularUser(t *testing.T) {
	t.Parallel()

	// given
	ownerClient, db := coderdtest.NewWithDatabase(t, nil)
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
	require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

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

func TestResumeNotifications(t *testing.T) {
	t.Parallel()

	// given
	ownerClient, db := coderdtest.NewWithDatabase(t, nil)
	_ = coderdtest.CreateFirstUser(t, ownerClient)

	// when
	inv, root := clitest.New(t, "notifications", "resume")
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
	require.False(t, settings.NotifierPaused)
}
