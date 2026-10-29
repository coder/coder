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

	const maxPendingAge = time.Hour

	for _, tc := range []struct {
		name                   string
		status                 database.ProvisionerJobStatus
		completed              bool
		recentUpdate           bool
		provisionerOnline      bool
		provisionerLastSeenAgo time.Duration
		lookupError            bool
		maxAgeExceeded         bool
		wantReapType           ReapType
		wantError              string
	}{
		{
			name:         "AcquiredAfterSelection",
			status:       database.ProvisionerJobStatusRunning,
			wantReapType: Pending,
			wantError:    "job is no longer pending",
		},
		{
			name:         "CanceledAfterSelection",
			status:       database.ProvisionerJobStatusCanceling,
			wantReapType: Pending,
			wantError:    "job is no longer pending",
		},
		{
			name:         "CompletedAfterSelection",
			completed:    true,
			wantReapType: Pending,
			wantError:    "job is completed",
		},
		{
			name:         "UpdatedAfterSelection",
			recentUpdate: true,
			wantReapType: Pending,
			wantError:    "job has been updated recently",
		},
		{
			name:              "ProvisionerReturnedAfterSelection",
			provisionerOnline: true,
			wantReapType:      Pending,
			wantError:         "an eligible provisioner is online",
		},
		{
			name:                   "ProvisionerOnlineWithLongerStaleInterval",
			provisionerOnline:      true,
			provisionerLastSeenAgo: 2 * time.Minute,
			wantReapType:           Pending,
			wantError:              "an eligible provisioner is online",
		},
		{
			name:         "ProvisionerLookupFailed",
			lookupError:  true,
			wantReapType: Pending,
			wantError:    "get eligible provisioner daemons: lookup failed",
		},
		{
			name:           "MaximumAgeLogLookupFailed",
			maxAgeExceeded: true,
			recentUpdate:   true,
			wantReapType:   PendingMaxAge,
			wantError:      "get logs for pending_max_age job: lookup failed",
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
				job.CreatedAt = now.Add(-maxPendingAge - time.Hour)
			}
			if tc.maxAgeExceeded {
				job.CreatedAt = now.Add(-maxPendingAge - time.Hour)
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
					{
						JobID: job.ID,
						ProvisionerDaemon: database.ProvisionerDaemon{
							LastSeenAt: sql.NullTime{
								Time:  now.Add(-tc.provisionerLastSeenAgo),
								Valid: true,
							},
						},
					},
				}, nil)
			}
			if tc.lookupError {
				tx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), []uuid.UUID{job.ID}).Return(nil, xerrors.New("lookup failed"))
			}
			if tc.maxAgeExceeded {
				tx.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), database.GetProvisionerLogsAfterIDParams{
					JobID:        job.ID,
					CreatedAfter: 0,
				}).Return(nil, xerrors.New("lookup failed"))
			}
			reapType, err := reapJob(t.Context(), testutil.Logger(t), db, nil, &jobToReap{
				ID: job.ID, Type: Pending, Threshold: PendingJobDuration,
			}, maxPendingAge, 5*time.Minute)
			require.ErrorContains(t, err, tc.wantError)
			require.Equal(t, tc.wantReapType, reapType)
		})
	}
}

func TestReapHungJobRecheck(t *testing.T) {
	t.Parallel()

	// The selected hung job received an update before the row was locked.
	now := dbtime.Now()
	job := database.ProvisionerJob{
		ID:        uuid.New(),
		JobStatus: database.ProvisionerJobStatusRunning,
		CreatedAt: now.Add(-10 * time.Minute),
		StartedAt: sql.NullTime{Time: now.Add(-10 * time.Minute), Valid: true},
		UpdatedAt: now,
	}
	db := dbmock.NewMockStore(gomock.NewController(t))
	tx := dbmock.NewMockStore(gomock.NewController(t))
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
		return fn(tx)
	})
	tx.EXPECT().GetProvisionerJobByIDForUpdate(gomock.Any(), job.ID).Return(job, nil)

	// No log or job mutations are expected after the locked recheck.
	reapType, err := reapJob(t.Context(), testutil.Logger(t), db, nil, &jobToReap{
		ID: job.ID, Type: Hung, Threshold: HungJobDuration,
	}, 2*time.Hour, 5*time.Minute)
	var ineligibleErr jobIneligibleError
	require.ErrorAs(t, err, &ineligibleErr)
	require.ErrorContains(t, err, "job has been updated recently")
	require.Equal(t, Hung, reapType)
}
