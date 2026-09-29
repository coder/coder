package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuildDebugEvent(t *testing.T) {
	t.Parallel()

	t.Run("AdminClicksMembersFailedBuild", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			TelemetryReporter: fTelemetry,
			DeploymentValues: coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
				values.Experiments = []string{string(codersdk.ExperimentEnableAIWorkspaceDebug)}
			}),
		})
		user := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        member.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStop,
			Reason:     database.BuildReasonAutostop,
		}).Failed().Do()

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
		require.Equal(t, string(database.WorkspaceTransitionStop), event.Transition)
		require.Equal(t, string(database.BuildReasonAutostop), event.Reason)
		require.False(t, event.CreatedAt.IsZero())
	})

	for _, tt := range []struct {
		name               string
		experimentDisabled bool
		chatDenied         bool
		otherUsersBuild    bool
		missingID          bool
		jobStatus          database.ProvisionerJobStatus
		wantStatus         int
	}{
		{name: "MissingID", missingID: true, jobStatus: database.ProvisionerJobStatusFailed, wantStatus: http.StatusBadRequest},
		{name: "ExperimentDisabled", experimentDisabled: true, jobStatus: database.ProvisionerJobStatusFailed, wantStatus: http.StatusNotFound},
		{name: "ChatCreateDenied", chatDenied: true, jobStatus: database.ProvisionerJobStatusFailed, wantStatus: http.StatusForbidden},
		{name: "OtherUsersBuild", otherUsersBuild: true, jobStatus: database.ProvisionerJobStatusFailed, wantStatus: http.StatusNotFound},
		{name: "SucceededBuild", jobStatus: database.ProvisionerJobStatusSucceeded, wantStatus: http.StatusBadRequest},
		{name: "RunningBuild", jobStatus: database.ProvisionerJobStatusRunning, wantStatus: http.StatusBadRequest},
		{name: "PendingBuild", jobStatus: database.ProvisionerJobStatusPending, wantStatus: http.StatusBadRequest},
		{name: "CanceledBuild", jobStatus: database.ProvisionerJobStatusCanceled, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
			options := &coderdtest.Options{
				TelemetryReporter: fTelemetry,
				DeploymentValues: coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
					if !tt.experimentDisabled {
						values.Experiments = []string{string(codersdk.ExperimentEnableAIWorkspaceDebug)}
					}
				}),
			}
			if tt.chatDenied {
				options.Authorizer = &coderdtest.FakeAuthorizer{
					ConditionalReturn: func(_ context.Context, _ rbac.Subject, action policy.Action, object rbac.Object) error {
						if action == policy.ActionCreate && object.Type == rbac.ResourceChat.Type {
							return xerrors.New("chat creation denied")
						}
						return nil
					},
				}
			}
			client, db := coderdtest.NewWithDatabase(t, options)
			user := coderdtest.CreateFirstUser(t, client)
			build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: user.OrganizationID,
				OwnerID:        user.UserID,
			})
			switch tt.jobStatus {
			case database.ProvisionerJobStatusFailed:
				build = build.Failed()
			case database.ProvisionerJobStatusSucceeded:
				build = build.Succeeded()
			case database.ProvisionerJobStatusRunning:
				build = build.Starting()
			case database.ProvisionerJobStatusPending, database.ProvisionerJobStatusCanceled:
				build = build.Pending()
			default:
				t.Fatalf("unexpected job status %q", tt.jobStatus)
			}
			r := build.Do()
			if tt.jobStatus == database.ProvisionerJobStatusCanceled {
				require.NoError(t, client.CancelWorkspaceBuild(ctx, r.Build.ID, codersdk.CancelWorkspaceBuildParams{}))
			}
			if tt.otherUsersBuild {
				client, _ = coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
			}
			req := codersdk.WorkspaceBuildDebugEventRequest{ID: uuid.New()}
			if tt.missingID {
				req.ID = uuid.Nil
			}

			err := client.ReportWorkspaceBuildDebugClick(ctx, r.Build.ID, req)
			require.Error(t, err)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, tt.wantStatus, sdkErr.StatusCode())

			// Reports are synchronous, so any event emitted by this request
			// is already buffered when the response arrives.
			for {
				select {
				case snapshot := <-fTelemetry.snapshots:
					require.Empty(t, snapshot.WorkspaceBuildDebugEvents)
				default:
					return
				}
			}
		})
	}
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
