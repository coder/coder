package coderd

import (
	"database/sql"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"
)

func Test_ActivityBumpWorkspace(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                string
		transition          database.WorkspaceTransition
		jobCompletedAt      sql.NullTime
		buildDeadlineOffset *time.Duration
		maxDeadlineOffset   *time.Duration
		workspaceTTL        time.Duration
		expectedBump        time.Duration
	}{
		{
			name:                "NotFinishedYet",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{},
			buildDeadlineOffset: ptr.Ref(8 * time.Hour),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        0,
		},
		{
			name:                "ManualShutdown",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
			buildDeadlineOffset: nil,
			expectedBump:        0,
		},
		{
			name:                "NotTimeToBumpYet",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
			buildDeadlineOffset: ptr.Ref(8 * time.Hour),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        0,
		},
		{
			name:                "TimeToBump",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(8*time.Hour - 24*time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        8 * time.Hour,
		},
		{
			name:                "MaxDeadline",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(time.Minute), // last chance to bump!
			maxDeadlineOffset:   ptr.Ref(time.Hour),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        1 * time.Hour,
		},
		{
			// A workspace that is still running, has passed its deadline, but has not
			// yet been auto-stopped should still bump the deadline.
			name:                "PastDeadlineStillBumps",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(-time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        8 * time.Hour,
		},
		{
			// A stopped workspace should never bump.
			name:                "StoppedWorkspace",
			transition:          database.WorkspaceTransitionStop,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-time.Minute)},
			buildDeadlineOffset: ptr.Ref(-time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        0,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				now   = dbtime.Now()
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

			var buildNumber int32 = 1
			// Insert a number of previous workspace builds.
			for i := 0; i < 5; i++ {
				insertPrevWorkspaceBuild(t, db, org.ID, templateVersion.ID, ws.ID, database.WorkspaceTransitionStart, buildNumber)
				buildNumber++
				insertPrevWorkspaceBuild(t, db, org.ID, templateVersion.ID, ws.ID, database.WorkspaceTransitionStop, buildNumber)
				buildNumber++
			}

			// dbgen.WorkspaceBuild automatically sets deadline to now+1 hour if not set
			var buildDeadline time.Time
			if tt.buildDeadlineOffset != nil {
				buildDeadline = now.Add(*tt.buildDeadlineOffset)
			}
			var maxDeadline time.Time
			if tt.maxDeadlineOffset != nil {
				maxDeadline = now.Add(*tt.maxDeadlineOffset)
			}
			err := db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
				ID:                buildID,
				CreatedAt:         dbtime.Now(),
				UpdatedAt:         dbtime.Now(),
				BuildNumber:       buildNumber,
				InitiatorID:       user.ID,
				Reason:            database.BuildReasonInitiator,
				WorkspaceID:       ws.ID,
				JobID:             job.ID,
				TemplateVersionID: templateVersion.ID,
				Transition:        tt.transition,
				Deadline:          buildDeadline,
				MaxDeadline:       maxDeadline,
			})
			require.NoError(t, err, "unexpected error inserting workspace build")
			bld, err := db.GetWorkspaceBuildByID(ctx, buildID)
			require.NoError(t, err, "unexpected error fetching inserted workspace build")

			// Validate our initial state before bump
			require.Equal(t, tt.transition, bld.Transition, "unexpected transition before bump")
			require.Equal(t, tt.jobCompletedAt.Time.UTC(), job.CompletedAt.Time.UTC(), "unexpected job completed at before bump")
			require.Equal(t, buildDeadline.UTC(), bld.Deadline.UTC(), "unexpected build deadline before bump")
			require.Equal(t, maxDeadline.UTC(), bld.MaxDeadline.UTC(), "unexpected max deadline before bump")
			require.Equal(t, tt.workspaceTTL, time.Duration(ws.Ttl.Int64), "unexpected workspace TTL before bump")

			workaroundWindowsTimeResolution(t)

			// Bump duration is measured from the time of the bump, so we measure from here.
			start := dbtime.Now()
			activityBumpWorkspace(ctx, log, db, bld.WorkspaceID)
			elapsed := time.Since(start)
			if elapsed > 15*time.Second {
				t.Logf("warning: activityBumpWorkspace took longer than 15 seconds: %s", elapsed)
			}
			// The actual bump could have happened anywhere in the elapsed time, so we
			// guess at the approximate time of the bump.
			approxBumpTime := start.Add(elapsed / 2)

			// Validate our state after bump
			updatedBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, bld.WorkspaceID)
			require.NoError(t, err, "unexpected error getting latest workspace build")
			if tt.expectedBump == 0 {
				require.Equal(t, bld.UpdatedAt.UTC(), updatedBuild.UpdatedAt.UTC(), "should not have bumped updated_at")
				require.Equal(t, bld.Deadline.UTC(), updatedBuild.Deadline.UTC(), "should not have bumped deadline")
			} else {
				require.NotEqual(t, bld.UpdatedAt.UTC(), updatedBuild.UpdatedAt.UTC(), "should have bumped updated_at")
				expectedDeadline := approxBumpTime.Add(tt.expectedBump).UTC()
				// Note: if CI is especially slow, this test may fail. There is an internal 15-second
				// deadline in activityBumpWorkspace, so we allow the same window here.
				require.WithinDuration(t, expectedDeadline, updatedBuild.Deadline.UTC(), 15*time.Second, "unexpected deadline after bump")
			}
		})
	}
}

func insertPrevWorkspaceBuild(t *testing.T, db database.Store, orgID, tvID, workspaceID uuid.UUID, transition database.WorkspaceTransition, buildNumber int32) {
	t.Helper()

	job := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
		OrganizationID: orgID,
	})
	_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		BuildNumber:       buildNumber,
		WorkspaceID:       workspaceID,
		JobID:             job.ID,
		TemplateVersionID: tvID,
		Transition:        transition,
	})
}

func workaroundWindowsTimeResolution(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Logf("workaround: sleeping for a short time to avoid time resolution issues on Windows")
		<-time.After(testutil.IntervalSlow)
	}
}
