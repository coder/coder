package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuildDebugEvent(t *testing.T) {
	t.Parallel()

	t.Run("Click", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			TelemetryReporter: fTelemetry,
		})
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
			Reason:     database.BuildReasonInitiator,
		}).Do()

		eventID := uuid.New()
		err := client.ReportWorkspaceBuildDebugClick(ctx, r.Build.ID, codersdk.WorkspaceBuildDebugEventRequest{
			ID: eventID,
		})
		require.NoError(t, err)

		event := receiveWorkspaceBuildDebugEvent(ctx, t, fTelemetry)
		require.Equal(t, eventID, event.ID)
		require.Equal(t, telemetry.WorkspaceBuildDebugEventClick, event.EventType)
		require.Equal(t, user.UserID, event.UserID)
		require.Equal(t, r.Workspace.ID, event.WorkspaceID)
		require.Equal(t, r.Build.ID, event.WorkspaceBuildID)
		require.Equal(t, string(database.WorkspaceTransitionStart), event.Transition)
		require.Equal(t, string(database.BuildReasonInitiator), event.Reason)
		require.False(t, event.CreatedAt.IsZero())
	})

	t.Run("MissingID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Do()

		err := client.ReportWorkspaceBuildDebugClick(ctx, r.Build.ID, codersdk.WorkspaceBuildDebugEventRequest{})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("OtherUsersBuild", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client, db := coderdtest.NewWithDatabase(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: admin.OrganizationID,
			OwnerID:        admin.UserID,
		}).Do()

		err := memberClient.ReportWorkspaceBuildDebugClick(ctx, r.Build.ID, codersdk.WorkspaceBuildDebugEventRequest{
			ID: uuid.New(),
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

// receiveWorkspaceBuildDebugEvent drains snapshots until one carries a debug
// event. Creating the first user reports its own snapshots on the same channel.
func receiveWorkspaceBuildDebugEvent(ctx context.Context, t *testing.T, reporter *fakeTelemetryReporter) telemetry.WorkspaceBuildDebugEvent {
	t.Helper()

	for {
		snapshot := testutil.TryReceive(ctx, t, reporter.snapshots)
		if len(snapshot.WorkspaceBuildDebugEvents) > 0 {
			require.Len(t, snapshot.WorkspaceBuildDebugEvents, 1)
			return snapshot.WorkspaceBuildDebugEvents[0]
		}
	}
}
