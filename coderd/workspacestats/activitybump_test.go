package workspacestats_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/testutil"
)

func Test_ActivityBumpWorkspace(t *testing.T) {
	t.Parallel()

	// We test the below in multiple timezones specifically
	// chosen to trigger timezone-related bugs.
	timezones := []string{
		"Asia/Kolkata",     // No DST, positive fractional offset
		"America/St_Johns", // DST, negative fractional offset
		"Europe/Paris",     // DST, positive offset
		"US/Arizona",       // No DST, negative offset
		"UTC",              // Baseline
	}

	for _, tt := range []struct {
		name                          string
		transition                    database.WorkspaceTransition
		jobCompletedAt                sql.NullTime
		buildDeadlineOffset           *time.Duration
		maxDeadlineOffset             *time.Duration
		workspaceTTL                  time.Duration
		templateTTL                   time.Duration
		templateActivityBump          time.Duration
		templateDisallowsUserAutostop bool
		expectedBump                  time.Duration
		// If the tests get queued, we need to be able to set the next autostart
		// based on the actual time the unit test is running.
		nextAutostart func(now time.Time) time.Time
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
			// Expected bump is 0 because the original deadline is more than 1 hour
			// out, so a bump would decrease the deadline.
			name:                "BumpLessThanDeadline",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-30 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(8*time.Hour - 30*time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        0,
		},
		{
			name:                "TimeToBump",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-30 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(-30 * time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        time.Hour,
		},
		{
			name:                "TimeToBumpNextAutostart",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-30 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(-30 * time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        8*time.Hour + 30*time.Minute,
			nextAutostart:       func(now time.Time) time.Time { return now.Add(time.Minute * 30) },
		},
		{
			name:                "MaxDeadline",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(time.Minute), // last chance to bump!
			maxDeadlineOffset:   ptr.Ref(time.Minute * 30),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        time.Minute * 30,
		},
		{
			// A workspace that is still running, has passed its deadline, but has not
			// yet been auto-stopped should still bump the deadline.
			name:                "PastDeadlineStillBumps",
			transition:          database.WorkspaceTransitionStart,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-24 * time.Minute)},
			buildDeadlineOffset: ptr.Ref(-time.Minute),
			workspaceTTL:        8 * time.Hour,
			expectedBump:        time.Hour,
		},
		{
			// A stopped workspace should never bump.
			name:                "StoppedWorkspace",
			transition:          database.WorkspaceTransitionStop,
			jobCompletedAt:      sql.NullTime{Valid: true, Time: dbtime.Now().Add(-time.Minute)},
			buildDeadlineOffset: ptr.Ref(-time.Minute),
			workspaceTTL:        8 * time.Hour,
		},
		{
			// A workspace built from a template that disallows user autostop should bump
			// by the template TTL instead.
			name:                          "TemplateDisallowsUserAutostop",
			transition:                    database.WorkspaceTransitionStart,
			jobCompletedAt:                sql.NullTime{Valid: true, Time: dbtime.Now().Add(-3 * time.Hour)},
			buildDeadlineOffset:           ptr.Ref(-30 * time.Minute),
			workspaceTTL:                  2 * time.Hour,
			templateTTL:                   10 * time.Hour,
			templateDisallowsUserAutostop: true,
			expectedBump:                  10*time.Hour + (time.Minute * 30),
			nextAutostart:                 func(now time.Time) time.Time { return now.Add(time.Minute * 30) },
		},
		{
			// Custom activity bump duration specified on the template.
			name:                 "TemplateCustomActivityBump",
			transition:           database.WorkspaceTransitionStart,
			jobCompletedAt:       sql.NullTime{Valid: true, Time: dbtime.Now().Add(-30 * time.Minute)},
			buildDeadlineOffset:  ptr.Ref(-30 * time.Minute),
			workspaceTTL:         8 * time.Hour,
			templateActivityBump: 5 * time.Hour, // instead of default 1h
			expectedBump:         5 * time.Hour,
		},
		{
			// Activity bump duration is 0.
			name:                 "TemplateCustomActivityBumpZero",
			transition:           database.WorkspaceTransitionStart,
			jobCompletedAt:       sql.NullTime{Valid: true, Time: dbtime.Now().Add(-30 * time.Minute)},
			buildDeadlineOffset:  ptr.Ref(-30 * time.Minute),
			workspaceTTL:         8 * time.Hour,
			templateActivityBump: -1, // negative values get changed to 0 in the test
			expectedBump:         0,
		},
	} {
		for _, tz := range timezones {
			t.Run(tt.name+"/"+tz, func(t *testing.T) {
				t.Parallel()
				nextAutostart := tt.nextAutostart
				if tt.nextAutostart == nil {
					nextAutostart = func(now time.Time) time.Time { return time.Time{} }
				}

				var (
					now   = dbtime.Now()
					ctx   = testutil.Context(t, testutil.WaitLong)
					log   = testutil.Logger(t)
					db, _ = dbtestutil.NewDB(t, dbtestutil.WithTimezone(tz))
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
					ws = dbgen.Workspace(t, db, database.WorkspaceTable{
						OwnerID:        user.ID,
						OrganizationID: org.ID,
						TemplateID:     template.ID,
						Ttl:            sql.NullInt64{Valid: true, Int64: int64(tt.workspaceTTL)},
					})
					job = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
						OrganizationID: org.ID,
						CompletedAt:    tt.jobCompletedAt,
					})
					_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
						JobID: job.ID,
					})
					buildID = uuid.New()
				)

				activityBump := 1 * time.Hour
				if tt.templateActivityBump < 0 {
					// less than 0 => 0
					activityBump = 0
				} else if tt.templateActivityBump != 0 {
					activityBump = tt.templateActivityBump
				}
				require.NoError(t, db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
					ID:                template.ID,
					UpdatedAt:         dbtime.Now(),
					AllowUserAutostop: !tt.templateDisallowsUserAutostop,
					DefaultTTL:        int64(tt.templateTTL),
					ActivityBump:      int64(activityBump),
				}), "unexpected error updating template schedule")

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

				// Wait a bit before bumping as dbtime is rounded to the nearest millisecond.
				// This should also hopefully be enough for Windows time resolution to register
				// a tick (win32 max timer resolution is apparently between 0.5 and 15.6ms)
				<-time.After(testutil.IntervalFast)

				// Bump duration is measured from the time of the bump, so we measure from here.
				start := dbtime.Now()
				workspacestats.ActivityBumpWorkspace(ctx, log, db, bld.WorkspaceID, nextAutostart(start), workspacestats.ActivityBumpReasonSSH)
				end := dbtime.Now()

				// Validate our state after bump
				updatedBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, bld.WorkspaceID)
				require.NoError(t, err, "unexpected error getting latest workspace build")
				require.Equal(t, bld.MaxDeadline.UTC(), updatedBuild.MaxDeadline.UTC(), "max_deadline should not have changed")

				updatedWorkspace, err := db.GetWorkspaceByID(ctx, bld.WorkspaceID)
				require.NoError(t, err, "unexpected error getting workspace")

				if tt.expectedBump == 0 {
					assert.Equal(t, bld.UpdatedAt.UTC(), updatedBuild.UpdatedAt.UTC(), "should not have bumped updated_at")
					assert.Equal(t, bld.Deadline.UTC(), updatedBuild.Deadline.UTC(), "should not have bumped deadline")
					assert.False(t, updatedWorkspace.LastActivitySource.Valid, "last_activity_source should not be set when the deadline was not bumped")
					assert.False(t, updatedWorkspace.LastActivityAt.Valid, "last_activity_at should not be set when the deadline was not bumped")
					return
				}
				assert.NotEqual(t, bld.UpdatedAt.UTC(), updatedBuild.UpdatedAt.UTC(), "should have bumped updated_at")
				assert.Equal(t, sql.NullString{String: string(workspacestats.ActivityBumpReasonSSH), Valid: true}, updatedWorkspace.LastActivitySource, "last_activity_source should be recorded when the deadline was bumped")
				require.True(t, updatedWorkspace.LastActivityAt.Valid, "last_activity_at should be recorded when the deadline was bumped")
				// 1min buffer on either side to tolerate clock skew between
				// the test process and the database server, matching the
				// deadline assertions below.
				assert.GreaterOrEqual(t, updatedWorkspace.LastActivityAt.Time, start.Add(-time.Minute), "last_activity_at should be at or after the start of the bump")
				assert.LessOrEqual(t, updatedWorkspace.LastActivityAt.Time, end.Add(time.Minute), "last_activity_at should be at or before the end of the bump")
				if tt.maxDeadlineOffset != nil {
					assert.Equal(t, bld.MaxDeadline.UTC(), updatedBuild.MaxDeadline.UTC(), "new deadline must equal original max deadline")
					return
				}

				// Assert that the bump occurred between start and end. 1min buffer on either side.
				expectedDeadlineStart := start.Add(tt.expectedBump).Add(time.Minute * -1)
				expectedDeadlineEnd := end.Add(tt.expectedBump).Add(time.Minute)
				require.GreaterOrEqual(t, updatedBuild.Deadline, expectedDeadlineStart, "new deadline should be greater than or equal to start")
				require.LessOrEqual(t, updatedBuild.Deadline, expectedDeadlineEnd, "new deadline should be less than or equal to end")
			})
		}
	}
}

// Test_ActivityBumpWorkspace_SourceOverwrites verifies that
// last_activity_source/last_activity_at are overwritten in place on each
// bump rather than accumulating any history, per the feature's design
// (see coder/coder#17320).
func Test_ActivityBumpWorkspace_SourceOverwrites(t *testing.T) {
	t.Parallel()

	var (
		ctx   = testutil.Context(t, testutil.WaitLong)
		log   = testutil.Logger(t)
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
		ws = dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     template.ID,
			Ttl:            sql.NullInt64{Valid: true, Int64: int64(8 * time.Hour)},
		})
		job = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now().Add(-30 * time.Minute)},
		})
	)

	require.NoError(t, db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:                template.ID,
		UpdatedAt:         dbtime.Now(),
		AllowUserAutostop: true,
		DefaultTTL:        int64(8 * time.Hour),
		ActivityBump:      int64(1 * time.Hour),
	}))

	buildID := uuid.New()
	require.NoError(t, db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
		ID:                buildID,
		CreatedAt:         dbtime.Now(),
		UpdatedAt:         dbtime.Now(),
		BuildNumber:       1,
		InitiatorID:       user.ID,
		Reason:            database.BuildReasonInitiator,
		WorkspaceID:       ws.ID,
		JobID:             job.ID,
		TemplateVersionID: templateVersion.ID,
		Transition:        database.WorkspaceTransitionStart,
		// A deadline already in the past satisfies the "5% of the
		// deadline has elapsed" guard, so the bump fires immediately.
		Deadline: dbtime.Now().Add(-30 * time.Minute),
	}))

	workspacestats.ActivityBumpWorkspace(ctx, log, db, ws.ID, time.Time{}, workspacestats.ActivityBumpReasonSSH)
	afterSSH, err := db.GetWorkspaceByID(ctx, ws.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "ssh", Valid: true}, afterSSH.LastActivitySource)

	// Move the deadline back into bump range again and bump with a
	// different source. It should overwrite, not append.
	require.NoError(t, db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
		ID:          buildID,
		UpdatedAt:   dbtime.Now(),
		Deadline:    dbtime.Now().Add(-30 * time.Minute),
		MaxDeadline: time.Time{},
	}))
	workspacestats.ActivityBumpWorkspace(ctx, log, db, ws.ID, time.Time{}, workspacestats.ActivityBumpReasonVSCode)
	afterVSCode, err := db.GetWorkspaceByID(ctx, ws.ID)
	require.NoError(t, err)
	assert.Equal(t, sql.NullString{String: "vscode", Valid: true}, afterVSCode.LastActivitySource, "source should overwrite to the latest value, not accumulate history")
}

func TestActivityBumpReasonFromStats(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		stats    *agentproto.Stats
		expected workspacestats.ActivityBumpReason
	}{
		{
			name:     "SSH",
			stats:    &agentproto.Stats{SessionCountSsh: 1},
			expected: workspacestats.ActivityBumpReasonSSH,
		},
		{
			name:     "VSCode",
			stats:    &agentproto.Stats{SessionCountVscode: 1},
			expected: workspacestats.ActivityBumpReasonVSCode,
		},
		{
			name:     "JetBrains",
			stats:    &agentproto.Stats{SessionCountJetbrains: 1},
			expected: workspacestats.ActivityBumpReasonJetBrains,
		},
		{
			name:     "ReconnectingPTY",
			stats:    &agentproto.Stats{SessionCountReconnectingPty: 1},
			expected: workspacestats.ActivityBumpReasonReconnectingPTY,
		},
		{
			name: "SSHTakesPriorityOverAll",
			stats: &agentproto.Stats{
				SessionCountSsh:             1,
				SessionCountVscode:          1,
				SessionCountJetbrains:       1,
				SessionCountReconnectingPty: 1,
			},
			expected: workspacestats.ActivityBumpReasonSSH,
		},
		{
			name: "VSCodeTakesPriorityOverJetBrainsAndPTY",
			stats: &agentproto.Stats{
				SessionCountVscode:          1,
				SessionCountJetbrains:       1,
				SessionCountReconnectingPty: 1,
			},
			expected: workspacestats.ActivityBumpReasonVSCode,
		},
		{
			name: "JetBrainsTakesPriorityOverPTY",
			stats: &agentproto.Stats{
				SessionCountJetbrains:       1,
				SessionCountReconnectingPty: 1,
			},
			expected: workspacestats.ActivityBumpReasonJetBrains,
		},
		{
			name:     "NoSessionCountsFallsBackToSSH",
			stats:    &agentproto.Stats{ConnectionCount: 1},
			expected: workspacestats.ActivityBumpReasonSSH,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, workspacestats.ActivityBumpReasonFromStats(tt.stats))
		})
	}
}

func insertPrevWorkspaceBuild(t *testing.T, db database.Store, orgID, tvID, workspaceID uuid.UUID, transition database.WorkspaceTransition, buildNumber int32) {
	t.Helper()

	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
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
