package agentapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

func TestGetWorkspaceShutdown(t *testing.T) {
	t.Parallel()
	for _, transition := range []database.WorkspaceTransition{database.WorkspaceTransitionStart, database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete} {
		t.Run(string(transition), func(t *testing.T) {
			t.Parallel()
			for _, status := range []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending, database.ProvisionerJobStatusRunning, database.ProvisionerJobStatusSucceeded,
				database.ProvisionerJobStatusCanceling, database.ProvisionerJobStatusCanceled, database.ProvisionerJobStatusFailed,
			} {
				t.Run(string(status), func(t *testing.T) {
					t.Parallel()
					db := dbmock.NewMockStore(gomock.NewController(t))
					api := &agentapi.WorkspaceShutdownAPI{WorkspaceID: uuid.New(), Database: db}
					build := database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), Transition: transition, Reason: database.BuildReasonAutostop}
					db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), api.WorkspaceID).Return(build, nil)
					if transition != database.WorkspaceTransitionStart {
						db.EXPECT().GetProvisionerJobByID(gomock.Any(), build.JobID).Return(database.ProvisionerJob{JobStatus: status}, nil)
					}
					shutdown, err := api.GetWorkspaceShutdown(testutil.Context(t, testutil.WaitLong), &emptypb.Empty{})
					require.NoError(t, err)
					if transition != database.WorkspaceTransitionStart && (status == database.ProvisionerJobStatusRunning || status == database.ProvisionerJobStatusSucceeded) {
						require.Equal(t, build.ID.String(), shutdown.BuildId)
						require.Equal(t, string(transition), shutdown.Transition)
						require.Equal(t, string(build.Reason), shutdown.BuildReason)
					} else {
						require.Empty(t, shutdown.BuildId)
					}
				})
			}
		})
	}
}

func TestGetWorkspaceShutdownDatabaseError(t *testing.T) {
	t.Parallel()
	for _, query := range []string{"build", "job"} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			api := &agentapi.WorkspaceShutdownAPI{WorkspaceID: uuid.New(), Database: db}
			queryErr := xerrors.New("database unavailable")
			build := database.WorkspaceBuild{JobID: uuid.New(), Transition: database.WorkspaceTransitionStop}
			if query == "build" {
				db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), api.WorkspaceID).Return(database.WorkspaceBuild{}, queryErr)
			} else {
				db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), api.WorkspaceID).Return(build, nil)
				db.EXPECT().GetProvisionerJobByID(gomock.Any(), build.JobID).Return(database.ProvisionerJob{}, queryErr)
			}
			_, err := api.GetWorkspaceShutdown(testutil.Context(t, testutil.WaitLong), &emptypb.Empty{})
			require.ErrorIs(t, err, queryErr)
		})
	}
}
