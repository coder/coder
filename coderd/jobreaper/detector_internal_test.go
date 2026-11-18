package jobreaper

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
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
			name:              "EligibleProvisionerMakingProgress",
			provisionerOnline: true,
			wantError:         "an eligible provisioner is online and making progress",
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
				CreatedAt: now.Add(-PendingJobDuration - 5*time.Minute),
				UpdatedAt: now.Add(-PendingJobDuration - 5*time.Minute),
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
				online := database.ProvisionerDaemon{
					ID:         uuid.New(),
					LastSeenAt: sql.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
				}
				stale := database.ProvisionerDaemon{
					ID:         uuid.New(),
					LastSeenAt: sql.NullTime{Time: now.Add(-ProvisionerStaleInterval - time.Minute), Valid: true},
				}
				tx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), []uuid.UUID{job.ID}).Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{
					{JobID: job.ID, ProvisionerDaemon: online},
					{JobID: job.ID, ProvisionerDaemon: stale},
				}, nil)
				tx.EXPECT().HasProvisionerDaemonMakingProgress(gomock.Any(), gomock.Cond(func(arg database.HasProvisionerDaemonMakingProgressParams) bool {
					return slices.Equal(arg.DaemonIds, []uuid.UUID{online.ID})
				})).Return(true, nil)
			}
			if tc.lookupError {
				tx.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), []uuid.UUID{job.ID}).Return(nil, xerrors.New("lookup failed"))
			}
			err := reapJob(t.Context(), testutil.Logger(t), db, nil, &jobToReap{
				ID: job.ID, Type: Pending, Threshold: PendingJobDuration,
			})
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestReapPendingJobProvisionerProgress(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	now := dbtime.Now()
	for _, tc := range []struct {
		name        string
		completedAt time.Time
		updatedAt   time.Time
	}{
		{name: "CompletedBeforeHungCutoff", completedAt: now.Add(-HungJobDuration - time.Minute)},
		{name: "RunningAndUpdated", updatedAt: now.Add(-time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			org := dbgen.Organization(t, db, database.Organization{})
			tags := database.StringMap{"pool": "test"}
			queuedAt := now.Add(-PendingJobDuration - 5*time.Minute)
			job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				OrganizationID: org.ID, CreatedAt: queuedAt, UpdatedAt: queuedAt, Tags: tags,
			})
			daemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{OrganizationID: org.ID, Tags: tags})
			daemonJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				OrganizationID: org.ID,
				WorkerID:       uuid.NullUUID{UUID: daemon.ID, Valid: true},
				StartedAt:      sql.NullTime{Time: now.Add(-ProvisionerProgressWindow - time.Minute), Valid: true},
				CompletedAt:    sql.NullTime{Time: tc.completedAt, Valid: !tc.completedAt.IsZero()},
			})
			if !tc.updatedAt.IsZero() {
				err := db.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{ID: daemonJob.ID, UpdatedAt: tc.updatedAt})
				require.NoError(t, err)
			}

			err := reapJob(ctx, testutil.Logger(t), db, ps, &jobToReap{ID: job.ID, Type: Pending, Threshold: PendingJobDuration})
			require.ErrorContains(t, err, "an eligible provisioner is online and making progress")
		})
	}
}
