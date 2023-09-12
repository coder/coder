package coderd

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"
)

func Test_ActivityBumpWorkspace(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name           string
		transition     database.WorkspaceTransition
		jobCompletedAt sql.NullTime
		buildDeadline  time.Time
		maxDeadline    time.Time
		workspaceTTL   time.Duration
		expectedBump   time.Duration
	}{
		{
			name:           "NotFinishedYet",
			transition:     database.WorkspaceTransitionStart,
			jobCompletedAt: sql.NullTime{},
			buildDeadline:  dbtime.Now().Add(8 * time.Hour),
			workspaceTTL:   8 * time.Hour,
			expectedBump:   0,
		},
		{
			name:           "ManualShutdown",
			transition:     database.WorkspaceTransitionStart,
			jobCompletedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
			buildDeadline:  time.Time{},
			expectedBump:   0,
		},
		{
			name:           "NotTimeToBumpYet",
			transition:     database.WorkspaceTransitionStart,
			jobCompletedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
			buildDeadline:  dbtime.Now().Add(8 * time.Hour),
			workspaceTTL:   8 * time.Hour,
			expectedBump:   0,
		},
		{
			name:           "TimeToBump",
			transition:     database.WorkspaceTransitionStart,
			jobCompletedAt: sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadline:  dbtime.Now().Add(8*time.Hour - 24*time.Minute),
			workspaceTTL:   8 * time.Hour,
			expectedBump:   8 * time.Hour,
		},
		{
			name:           "MaxDeadline",
			transition:     database.WorkspaceTransitionStart,
			jobCompletedAt: sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadline:  dbtime.Now().Add(time.Minute), // last chance to bump!
			maxDeadline:    dbtime.Now().Add(time.Hour),
			workspaceTTL:   8 * time.Hour,
			expectedBump:   1 * time.Hour,
		},
		{
			// A workspace that is still running, has passed its deadline, but has not
			// yet been auto-stopped should still bump the deadline.
			name:           "PastDeadlineStillBumps",
			transition:     database.WorkspaceTransitionStart,
			jobCompletedAt: sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadline:  dbtime.Now().Add(-time.Minute),
			workspaceTTL:   8 * time.Hour,
			expectedBump:   8 * time.Hour,
		},
		{
			// A stopped workspace should never bump.
			name:           "StoppedWorkspace",
			transition:     database.WorkspaceTransitionStop,
			jobCompletedAt: sql.NullTime{Valid: true, Time: dbtime.Now().Add(-time.Minute)},
			buildDeadline:  dbtime.Now().Add(-time.Minute),
			workspaceTTL:   8 * time.Hour,
			expectedBump:   0,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				ctx   = testutil.Context(t, testutil.WaitShort)
				log   = slogtest.Make(t, nil)
				db, _ = dbtestutil.NewDB(t)
				org   = dbgen.Organization(t, db, database.Organization{})
				user  = dbgen.User(t, db, database.User{
					Status: database.UserStatusActive,
				})
				_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
					UserID:         user.ID,
					OrganizationID: org.ID,
				})
				templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					OrganizationID: org.ID,
					CreatedBy:      user.ID,
				})
				template = dbgen.Template(t, db, database.Template{
					OrganizationID:  org.ID,
					ActiveVersionID: templateVersion.ID,
					CreatedBy:       user.ID,
				})
				ws = dbgen.Workspace(t, db, database.Workspace{
					OwnerID:        user.ID,
					OrganizationID: org.ID,
					TemplateID:     template.ID,
					Ttl:            sql.NullInt64{Valid: true, Int64: int64(tt.workspaceTTL)},
				})
				job = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
					OrganizationID: org.ID,
					CompletedAt:    tt.jobCompletedAt,
				})
				_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
					JobID: job.ID,
				})
				buildID = uuid.New()
			)
			// dbgen.WorkspaceBuild automatically sets deadline to now+1 hour if not set
			err := db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
				ID:                buildID,
				CreatedAt:         dbtime.Now(),
				UpdatedAt:         dbtime.Now(),
				BuildNumber:       1,
				InitiatorID:       user.ID,
				Reason:            database.BuildReasonInitiator,
				WorkspaceID:       ws.ID,
				JobID:             job.ID,
				TemplateVersionID: templateVersion.ID,
				Transition:        tt.transition,
				Deadline:          tt.buildDeadline,
				MaxDeadline:       tt.maxDeadline,
			})
			require.NoError(t, err, "unexpected error inserting workspace build")
			bld, err := db.GetWorkspaceBuildByID(ctx, buildID)
			require.NoError(t, err, "unexpected error fetching inserted workspace build")

			// Validate our initial state before bump
			require.Equal(t, tt.transition, bld.Transition, "unexpected transition before bump")
			require.Equal(t, tt.jobCompletedAt.Time.UTC(), job.CompletedAt.Time.UTC(), "unexpected job completed at before bump")
			require.Equal(t, tt.buildDeadline.UTC(), bld.Deadline.UTC(), "unexpected build deadline before bump")
			require.Equal(t, tt.maxDeadline.UTC(), bld.MaxDeadline.UTC(), "unexpected max deadline before bump")
			require.Equal(t, tt.workspaceTTL, time.Duration(ws.Ttl.Int64), "unexpected workspace TTL before bump")

			// new deadline is calculated from the time of the bump
			approxBumpTime := dbtime.Now()
			activityBumpWorkspace(ctx, log, db, bld.WorkspaceID)

			// Validate our state after bump
			updatedBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, bld.WorkspaceID)
			require.NoError(t, err, "unexpected error getting latest workspace build")
			if tt.expectedBump == 0 {
				require.Equal(t, bld.UpdatedAt.UTC(), updatedBuild.UpdatedAt.UTC(), "should not have bumped updated_at")
				require.Equal(t, bld.Deadline.UTC(), updatedBuild.Deadline.UTC(), "should not have bumped deadline")
			} else {
				require.NotEqual(t, bld.UpdatedAt.UTC(), updatedBuild.UpdatedAt.UTC(), "should have bumped updated_at")
				expectedDeadline := approxBumpTime.Add(tt.expectedBump).UTC()
				require.WithinDuration(t, expectedDeadline, updatedBuild.Deadline.UTC(), 15*time.Second, "unexpected deadline after bump")
			}
		})
	}
}
