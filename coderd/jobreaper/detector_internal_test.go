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

// TestReapPendingJobRecheck verifies that the locked recheck skips pending jobs
// whose state changed after candidate selection.
func TestReapPendingJobRecheck(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name              string
		acquired          bool
		provisionerOnline bool
		lookupError       bool
		wantError         string
	}{
		{
			name:      "NoLongerPending",
			acquired:  true,
			wantError: "job is no longer pending",
		},
		{
			name:              "EligibleProvisionerOnline",
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
			if tc.acquired {
				job.JobStatus = database.ProvisionerJobStatusRunning
				job.StartedAt = sql.NullTime{Time: now, Valid: true}
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
					{
						JobID: job.ID,
						ProvisionerDaemon: database.ProvisionerDaemon{
							LastSeenAt: sql.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
						},
					},
				}, nil)
			}
			if tc.lookupError {
				tx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), []uuid.UUID{job.ID}).Return(nil, xerrors.New("lookup failed"))
			}
			reapType, err := reapJob(t.Context(), testutil.Logger(t), db, nil, &jobToReap{
				ID: job.ID, Type: Pending, Threshold: PendingJobDuration,
			})
			require.ErrorContains(t, err, tc.wantError)
			require.Equal(t, Pending, reapType)
		})
	}
}

// TestReapHungJobRecheck verifies that a hung job updated after candidate
// selection is skipped.
func TestReapHungJobRecheck(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	job := database.ProvisionerJob{
		ID:        uuid.New(),
		JobStatus: database.ProvisionerJobStatusRunning,
		StartedAt: sql.NullTime{Time: now.Add(-10 * time.Minute), Valid: true},
		UpdatedAt: now,
	}
	db := dbmock.NewMockStore(gomock.NewController(t))
	tx := dbmock.NewMockStore(gomock.NewController(t))
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
		return fn(tx)
	})
	tx.EXPECT().GetProvisionerJobByIDForUpdate(gomock.Any(), job.ID).Return(job, nil)

	_, err := reapJob(t.Context(), testutil.Logger(t), db, nil, &jobToReap{
		ID: job.ID, Type: Hung, Threshold: HungJobDuration,
	})
	require.ErrorAs(t, err, &jobIneligibleError{})
}
