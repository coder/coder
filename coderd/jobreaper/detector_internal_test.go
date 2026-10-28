package jobreaper

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestReapPendingJobRecheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		status            database.ProvisionerJobStatus
		completed         bool
		recentUpdate      bool
		provisionerOnline bool
		lookupError       bool
		wantError         string
	}{
		{
			name:      "AcquiredAfterSelection",
			status:    database.ProvisionerJobStatusRunning,
			wantError: "job is no longer pending",
		},
		{
			name:      "CanceledAfterSelection",
			status:    database.ProvisionerJobStatusCanceling,
			wantError: "job is no longer pending",
		},
		{
			name:      "CompletedAfterSelection",
			completed: true,
			wantError: "job is completed",
		},
		{
			name:         "UpdatedAfterSelection",
			recentUpdate: true,
			wantError:    "job has been updated recently",
		},
		{
			name:              "ProvisionerReturnedAfterSelection",
			provisionerOnline: true,
			wantError:         "an eligible provisioner is online",
		},
		{
			name:        "ProvisionerLookupFailed",
			lookupError: true,
			wantError:   "get eligible provisioner daemons: lookup failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := dbtime.Now()
			job := database.ProvisionerJob{
				ID:        uuid.New(),
				JobStatus: database.ProvisionerJobStatusPending,
				CreatedAt: now.Add(-35 * time.Minute),
				UpdatedAt: now.Add(-35 * time.Minute),
			}
			if tc.status != "" {
				job.JobStatus = tc.status
				// An expired absolute age must not bypass the state check.
				job.CreatedAt = now.Add(-2 * time.Hour)
			}
			if tc.status == database.ProvisionerJobStatusRunning {
				job.StartedAt = sql.NullTime{Time: now, Valid: true}
				job.UpdatedAt = now
			}
			if tc.completed {
				job.CompletedAt = sql.NullTime{Time: now, Valid: true}
			}
			if tc.recentUpdate {
				job.UpdatedAt = now
			}
			db := dbmock.NewMockStore(gomock.NewController(t))
			tx := dbmock.NewMockStore(gomock.NewController(t))
			db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(tx)
			})
			tx.EXPECT().GetProvisionerJobByIDForUpdate(gomock.Any(), job.ID).Return(job, nil)
			if tc.provisionerOnline {
				tx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), []uuid.UUID{job.ID}).Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{
					{JobID: job.ID, ProvisionerDaemon: database.ProvisionerDaemon{LastSeenAt: sql.NullTime{Time: now, Valid: true}}},
				}, nil)
			}
			if tc.lookupError {
				tx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), []uuid.UUID{job.ID}).Return(nil, xerrors.New("lookup failed"))
			}
			// No mutation or outer-store lookup is expected for any of these cases.
			err := reapJob(t.Context(), testutil.Logger(t), db, nil, &jobToReap{
				ID: job.ID, Type: Pending, Threshold: PendingJobDuration,
			}, time.Hour)
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}
