package schedule_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	agplschedule "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestTemplateUpdateBuildDeadlines(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	var (
		quietUser = dbgen.User(t, db, database.User{
			Username: "quiet",
		})
		noQuietUser = dbgen.User(t, db, database.User{
			Username: "no-quiet",
		})
		file = dbgen.File(t, db, database.File{
			CreatedBy: quietUser.ID,
		})
		templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			FileID:      file.ID,
			InitiatorID: quietUser.ID,
			Tags: database.StringMap{
				"foo": "bar",
			},
		})
		templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: templateJob.OrganizationID,
			CreatedBy:      quietUser.ID,
			JobID:          templateJob.ID,
		})
		organizationID = templateJob.OrganizationID
	)

	const userQuietHoursSchedule = "CRON_TZ=UTC 0 0 * * *" // midnight UTC
	ctx := testutil.Context(t, testutil.WaitLong)
	quietUser, err := db.UpdateUserQuietHoursSchedule(ctx, database.UpdateUserQuietHoursScheduleParams{
		ID:                 quietUser.ID,
		QuietHoursSchedule: userQuietHoursSchedule,
	})
	require.NoError(t, err)

	realNow := time.Now().UTC()
	nowY, nowM, nowD := realNow.Date()
	buildTime := time.Date(nowY, nowM, nowD, 12, 0, 0, 0, time.UTC)       // noon today UTC
	nextQuietHours := time.Date(nowY, nowM, nowD+1, 0, 0, 0, 0, time.UTC) // midnight tomorrow UTC

	// Workspace old max_deadline too soon
	cases := []struct {
		name        string
		now         time.Time
		deadline    time.Time
		maxDeadline time.Time
		// Set to nil for no change.
		newDeadline    *time.Time
		newMaxDeadline time.Time
		noQuietHours   bool
		autostopReq    *agplschedule.TemplateAutostopRequirement
	}{
		{
			name:        "SkippedWorkspaceMaxDeadlineTooSoon",
			now:         buildTime,
			deadline:    buildTime,
			maxDeadline: buildTime.Add(1 * time.Hour),
			// Unchanged since the max deadline is too soon.
			newDeadline:    nil,
			newMaxDeadline: buildTime.Add(1 * time.Hour),
		},
		{
			name: "NewWorkspaceMaxDeadlineBeforeNow",
			// After the new max deadline...
			now:      nextQuietHours.Add(6 * time.Hour),
			deadline: buildTime,
			// Far into the future...
			maxDeadline: nextQuietHours.Add(24 * time.Hour),
			newDeadline: nil,
			// We will use now() + 2 hours if the newly calculated max deadline
			// from the workspace build time is before now.
			newMaxDeadline: nextQuietHours.Add(8 * time.Hour),
		},
		{
			name: "NewWorkspaceMaxDeadlineSoon",
			// Right before the new max deadline...
			now:      nextQuietHours.Add(-1 * time.Hour),
			deadline: buildTime,
			// Far into the future...
			maxDeadline: nextQuietHours.Add(24 * time.Hour),
			newDeadline: nil,
			// We will use now() + 2 hours if the newly calculated max deadline
			// from the workspace build time is within the next 2 hours.
			newMaxDeadline: nextQuietHours.Add(1 * time.Hour),
		},
		{
			name: "NewWorkspaceMaxDeadlineFuture",
			// Well before the new max deadline...
			now:      nextQuietHours.Add(-6 * time.Hour),
			deadline: buildTime,
			// Far into the future...
			maxDeadline:    nextQuietHours.Add(24 * time.Hour),
			newDeadline:    nil,
			newMaxDeadline: nextQuietHours,
		},
		{
			name: "DeadlineAfterNewWorkspaceMaxDeadline",
			// Well before the new max deadline...
			now: nextQuietHours.Add(-6 * time.Hour),
			// Far into the future...
			deadline:    nextQuietHours.Add(24 * time.Hour),
			maxDeadline: nextQuietHours.Add(24 * time.Hour),
			// The deadline should match since it is after the new max deadline.
			newDeadline:    ptr.Ref(nextQuietHours),
			newMaxDeadline: nextQuietHours,
		},
		{
			// There was a bug if a user has no quiet hours set, and autostop
			// req is not turned on, then the max deadline is set to `time.Time{}`.
			// This zero value was "in the past", so the workspace deadline would
			// be set to "now" + 2 hours.
			// This is a mistake because the max deadline being zero means
			// there is no max deadline.
			name:        "MaxDeadlineShouldBeUnset",
			now:         buildTime,
			deadline:    buildTime.Add(time.Hour * 8),
			maxDeadline: time.Time{}, // No max set
			// Should be unchanged
			newDeadline:    ptr.Ref(buildTime.Add(time.Hour * 8)),
			newMaxDeadline: time.Time{},
			noQuietHours:   true,
			autostopReq: &agplschedule.TemplateAutostopRequirement{
				DaysOfWeek: 0,
				Weeks:      0,
			},
		},
		{
			// A bug existed where MaxDeadline could be set, but deadline was
			// `time.Time{}`. This is a logical inconsistency because the "max"
			// deadline was ignored.
			name:        "NoDeadline",
			now:         buildTime,
			deadline:    time.Time{},
			maxDeadline: time.Time{}, // No max set
			// Should be unchanged
			newDeadline:    ptr.Ref(time.Time{}),
			newMaxDeadline: time.Time{},
			noQuietHours:   true,
			autostopReq: &agplschedule.TemplateAutostopRequirement{
				DaysOfWeek: 0,
				Weeks:      0,
			},
		},

		{
			// Similar to 'NoDeadline' test. This has a MaxDeadline set, so
			// the deadline of the workspace should now be set.
			name: "WorkspaceDeadlineNowSet",
			now:  nextQuietHours.Add(-6 * time.Hour),
			// Start with unset times
			deadline:       time.Time{},
			maxDeadline:    time.Time{},
			newDeadline:    ptr.Ref(nextQuietHours),
			newMaxDeadline: nextQuietHours,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			user := quietUser
			if c.noQuietHours {
				user = noQuietUser
			}

			t.Log("buildTime", buildTime)
			t.Log("nextQuietHours", nextQuietHours)
			t.Log("now", c.now)
			t.Log("deadline", c.deadline)
			t.Log("maxDeadline", c.maxDeadline)
			t.Log("newDeadline", c.newDeadline)
			t.Log("newMaxDeadline", c.newMaxDeadline)

			var (
				template = dbgen.Template(t, db, database.Template{
					OrganizationID:  organizationID,
					ActiveVersionID: templateVersion.ID,
					CreatedBy:       user.ID,
				})
				ws = dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: organizationID,
					OwnerID:        user.ID,
					TemplateID:     template.ID,
				})
				job = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					OrganizationID: organizationID,
					FileID:         file.ID,
					InitiatorID:    user.ID,
					Provisioner:    database.ProvisionerTypeEcho,
					Tags: database.StringMap{
						c.name: "yeah",
					},
				})
				wsBuild = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					WorkspaceID:       ws.ID,
					BuildNumber:       1,
					JobID:             job.ID,
					InitiatorID:       user.ID,
					TemplateVersionID: templateVersion.ID,
					ProvisionerState:  []byte(must(cryptorand.String(64))),
				})
			)

			// Assert test invariant: workspace build state must not be empty
			require.NotEmpty(t, wsBuild.ProvisionerState, "provisioner state must not be empty")

			acquiredJob, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				OrganizationID: job.OrganizationID,
				StartedAt: sql.NullTime{
					Time:  buildTime,
					Valid: true,
				},
				WorkerID: uuid.NullUUID{
					UUID:  uuid.New(),
					Valid: true,
				},
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: json.RawMessage(fmt.Sprintf(`{%q: "yeah"}`, c.name)),
			})
			require.NoError(t, err)
			require.Equal(t, job.ID, acquiredJob.ID)
			err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID: job.ID,
				CompletedAt: sql.NullTime{
					Time:  buildTime,
					Valid: true,
				},
				UpdatedAt: buildTime,
			})
			require.NoError(t, err)

			err = db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
				ID:          wsBuild.ID,
				UpdatedAt:   buildTime,
				Deadline:    c.deadline,
				MaxDeadline: c.maxDeadline,
			})
			require.NoError(t, err)

			wsBuild, err = db.GetWorkspaceBuildByID(ctx, wsBuild.ID)
			require.NoError(t, err)

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

			userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
			require.NoError(t, err)
			userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
			userQuietHoursStorePtr.Store(&userQuietHoursStore)

			clock := quartz.NewMock(t)
			clock.Set(c.now)

			// Set the template policy.
			templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr, notifications.NewNoopEnqueuer(), logger, clock)

			autostopReq := agplschedule.TemplateAutostopRequirement{
				// Every day
				DaysOfWeek: 0b01111111,
				Weeks:      0,
			}
			if c.autostopReq != nil {
				autostopReq = *c.autostopReq
			}
			_, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
				UserAutostartEnabled:     false,
				UserAutostopEnabled:      false,
				DefaultTTL:               0,
				AutostopRequirement:      autostopReq,
				FailureTTL:               0,
				TimeTilDormant:           0,
				TimeTilDormantAutoDelete: 0,
			})
			require.NoError(t, err)

			// Check that the workspace build has the expected deadlines.
			newBuild, err := db.GetWorkspaceBuildByID(ctx, wsBuild.ID)
			require.NoError(t, err)

			if c.newDeadline == nil {
				c.newDeadline = &wsBuild.Deadline
			}
			require.WithinDuration(t, *c.newDeadline, newBuild.Deadline, time.Second)
			require.WithinDuration(t, c.newMaxDeadline, newBuild.MaxDeadline, time.Second)

			// Check that the new build has the same state as before.
			require.Equal(t, wsBuild.ProvisionerState, newBuild.ProvisionerState, "provisioner state mismatch")
		})
	}
}

func TestTemplateUpdateBuildDeadlinesSkip(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	var (
		user = dbgen.User(t, db, database.User{})
		file = dbgen.File(t, db, database.File{
			CreatedBy: user.ID,
		})
		templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			FileID:      file.ID,
			InitiatorID: user.ID,
			Tags: database.StringMap{
				"foo": "bar",
			},
		})
		templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			CreatedBy:      user.ID,
			JobID:          templateJob.ID,
			OrganizationID: templateJob.OrganizationID,
		})
		template = dbgen.Template(t, db, database.Template{
			ActiveVersionID: templateVersion.ID,
			CreatedBy:       user.ID,
			OrganizationID:  templateJob.OrganizationID,
		})
		otherTemplate = dbgen.Template(t, db, database.Template{
			ActiveVersionID: templateVersion.ID,
			CreatedBy:       user.ID,
			OrganizationID:  templateJob.OrganizationID,
		})
	)

	// Create a workspace that will be shared by two builds.
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		TemplateID:     template.ID,
		OrganizationID: templateJob.OrganizationID,
	})

	const userQuietHoursSchedule = "CRON_TZ=UTC 0 0 * * *" // midnight UTC
	ctx := testutil.Context(t, testutil.WaitLong)
	user, err := db.UpdateUserQuietHoursSchedule(ctx, database.UpdateUserQuietHoursScheduleParams{
		ID:                 user.ID,
		QuietHoursSchedule: userQuietHoursSchedule,
	})
	require.NoError(t, err)

	realNow := time.Now().UTC()
	nowY, nowM, nowD := realNow.Date()
	buildTime := time.Date(nowY, nowM, nowD, 12, 0, 0, 0, time.UTC)       // noon today UTC
	now := time.Date(nowY, nowM, nowD, 18, 0, 0, 0, time.UTC)             // 6pm today UTC
	nextQuietHours := time.Date(nowY, nowM, nowD+1, 0, 0, 0, 0, time.UTC) // midnight tomorrow UTC

	// A date very far in the future which would definitely be updated.
	originalMaxDeadline := time.Date(nowY+1, nowM, nowD, 0, 0, 0, 0, time.UTC)

	_ = otherTemplate

	builds := []struct {
		name       string
		templateID uuid.UUID
		// Nil workspaceID means create a new workspace.
		workspaceID    uuid.UUID
		buildNumber    int32
		buildStarted   bool
		buildCompleted bool
		buildError     bool

		shouldBeUpdated bool

		// Set below:
		wsBuild database.WorkspaceBuild
	}{
		{
			name:            "DifferentTemplate",
			templateID:      otherTemplate.ID,
			workspaceID:     uuid.Nil,
			buildNumber:     1,
			buildStarted:    true,
			buildCompleted:  true,
			buildError:      false,
			shouldBeUpdated: false,
		},
		{
			name:            "NonStartedBuild",
			templateID:      template.ID,
			workspaceID:     uuid.Nil,
			buildNumber:     1,
			buildStarted:    false,
			buildCompleted:  false,
			buildError:      false,
			shouldBeUpdated: false,
		},
		{
			name:            "InProgressBuild",
			templateID:      template.ID,
			workspaceID:     uuid.Nil,
			buildNumber:     1,
			buildStarted:    true,
			buildCompleted:  false,
			buildError:      false,
			shouldBeUpdated: false,
		},
		{
			name:            "FailedBuild",
			templateID:      template.ID,
			workspaceID:     uuid.Nil,
			buildNumber:     1,
			buildStarted:    true,
			buildCompleted:  true,
			buildError:      true,
			shouldBeUpdated: false,
		},
		{
			name:           "NonLatestBuild",
			templateID:     template.ID,
			workspaceID:    ws.ID,
			buildNumber:    1,
			buildStarted:   true,
			buildCompleted: true,
			buildError:     false,
			// This build was successful but is not the latest build for this
			// workspace, see the next build.
			shouldBeUpdated: false,
		},
		{
			name:            "LatestBuild",
			templateID:      template.ID,
			workspaceID:     ws.ID,
			buildNumber:     2,
			buildStarted:    true,
			buildCompleted:  true,
			buildError:      false,
			shouldBeUpdated: true,
		},
		{
			name:            "LatestBuildOtherWorkspace",
			templateID:      template.ID,
			workspaceID:     uuid.Nil,
			buildNumber:     1,
			buildStarted:    true,
			buildCompleted:  true,
			buildError:      false,
			shouldBeUpdated: true,
		},
	}

	for i, b := range builds {
		wsID := b.workspaceID
		if wsID == uuid.Nil {
			ws := dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     b.templateID,
				OrganizationID: templateJob.OrganizationID,
			})
			wsID = ws.ID
		}
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			FileID:      file.ID,
			InitiatorID: user.ID,
			Provisioner: database.ProvisionerTypeEcho,
			Tags: database.StringMap{
				wsID.String(): "yeah",
			},
			OrganizationID: templateJob.OrganizationID,
		})
		wsBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wsID,
			BuildNumber:       b.buildNumber,
			JobID:             job.ID,
			InitiatorID:       user.ID,
			TemplateVersionID: templateVersion.ID,
			ProvisionerState:  []byte(must(cryptorand.String(64))),
		})

		// Assert test invariant: workspace build state must not be empty
		require.NotEmpty(t, wsBuild.ProvisionerState, "provisioner state must not be empty")

		err := db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:          wsBuild.ID,
			UpdatedAt:   buildTime,
			Deadline:    originalMaxDeadline,
			MaxDeadline: originalMaxDeadline,
		})
		require.NoError(t, err)

		wsBuild, err = db.GetWorkspaceBuildByID(ctx, wsBuild.ID)
		require.NoError(t, err)

		// Assert test invariant: workspace build state must not be empty
		require.NotEmpty(t, wsBuild.ProvisionerState, "provisioner state must not be empty")

		builds[i].wsBuild = wsBuild

		if !b.buildStarted {
			continue
		}

		acquiredJob, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID: job.OrganizationID,
			StartedAt: sql.NullTime{
				Time:  buildTime,
				Valid: true,
			},
			WorkerID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
			Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
			ProvisionerTags: json.RawMessage(fmt.Sprintf(`{%q: "yeah"}`, wsID)),
		})
		require.NoError(t, err)
		require.Equal(t, job.ID, acquiredJob.ID)

		if !b.buildCompleted {
			continue
		}

		buildError := ""
		if b.buildError {
			buildError = "error"
		}
		err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  buildTime,
				Valid: true,
			},
			Error: sql.NullString{
				String: buildError,
				Valid:  b.buildError,
			},
			UpdatedAt: buildTime,
		})
		require.NoError(t, err)
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
	require.NoError(t, err)
	userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
	userQuietHoursStorePtr.Store(&userQuietHoursStore)

	clock := quartz.NewMock(t)
	clock.Set(now)

	// Set the template policy.
	templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr, notifications.NewNoopEnqueuer(), logger, clock)
	_, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
		UserAutostartEnabled: false,
		UserAutostopEnabled:  false,
		DefaultTTL:           0,
		AutostopRequirement: agplschedule.TemplateAutostopRequirement{
			// Every day
			DaysOfWeek: 0b01111111,
			Weeks:      0,
		},
		FailureTTL:               0,
		TimeTilDormant:           0,
		TimeTilDormantAutoDelete: 0,
	})
	require.NoError(t, err)

	// Check each build.
	for i, b := range builds {
		msg := fmt.Sprintf("build %d: %s", i, b.name)
		newBuild, err := db.GetWorkspaceBuildByID(ctx, b.wsBuild.ID)
		require.NoError(t, err)

		if b.shouldBeUpdated {
			assert.WithinDuration(t, nextQuietHours, newBuild.Deadline, time.Second, msg)
			assert.WithinDuration(t, nextQuietHours, newBuild.MaxDeadline, time.Second, msg)
		} else {
			assert.WithinDuration(t, originalMaxDeadline, newBuild.Deadline, time.Second, msg)
			assert.WithinDuration(t, originalMaxDeadline, newBuild.MaxDeadline, time.Second, msg)
		}

		assert.Equal(t, builds[i].wsBuild.ProvisionerState, newBuild.ProvisionerState, "provisioner state mismatch")
	}
}

func TestNotifications(t *testing.T) {
	t.Parallel()

	t.Run("Dormancy", func(t *testing.T) {
		t.Parallel()

		var (
			db, _ = dbtestutil.NewDB(t)
			ctx   = testutil.Context(t, testutil.WaitLong)
			user  = dbgen.User(t, db, database.User{})
			file  = dbgen.File(t, db, database.File{
				CreatedBy: user.ID,
			})
			templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				FileID:      file.ID,
				InitiatorID: user.ID,
				Tags: database.StringMap{
					"foo": "bar",
				},
			})
			timeTilDormant  = time.Minute * 2
			templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				JobID:          templateJob.ID,
				OrganizationID: templateJob.OrganizationID,
			})
			template = dbgen.Template(t, db, database.Template{
				ActiveVersionID:          templateVersion.ID,
				CreatedBy:                user.ID,
				OrganizationID:           templateJob.OrganizationID,
				TimeTilDormant:           int64(timeTilDormant),
				TimeTilDormantAutoDelete: int64(timeTilDormant),
			})
		)

		// Add two dormant workspaces and one active workspace.
		dormantWorkspaces := []database.WorkspaceTable{
			dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     template.ID,
				OrganizationID: templateJob.OrganizationID,
				LastUsedAt:     time.Now().Add(-time.Hour),
			}),
			dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     template.ID,
				OrganizationID: templateJob.OrganizationID,
				LastUsedAt:     time.Now().Add(-time.Hour),
			}),
		}
		dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			TemplateID:     template.ID,
			OrganizationID: templateJob.OrganizationID,
			LastUsedAt:     time.Now(),
		})
		for _, ws := range dormantWorkspaces {
			db.UpdateWorkspaceDormantDeletingAt(ctx, database.UpdateWorkspaceDormantDeletingAtParams{
				ID: ws.ID,
				DormantAt: sql.NullTime{
					Time:  ws.LastUsedAt.Add(timeTilDormant),
					Valid: true,
				},
			})
		}

		// Setup dependencies
		notifyEnq := notificationstest.FakeEnqueuer{Store: db}
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		const userQuietHoursSchedule = "CRON_TZ=UTC 0 0 * * *" // midnight UTC
		userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
		require.NoError(t, err)
		userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
		userQuietHoursStorePtr.Store(&userQuietHoursStore)
		templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr, &notifyEnq, logger, nil)

		// Lower the dormancy TTL to ensure the schedule recalculates deadlines and
		// triggers notifications.
		// nolint:gocritic // Need an actor in the context.
		_, err = templateScheduleStore.Set(dbauthz.AsNotifier(ctx), db, template, agplschedule.TemplateScheduleOptions{
			TimeTilDormant:           timeTilDormant / 2,
			TimeTilDormantAutoDelete: timeTilDormant / 2,
		})
		require.NoError(t, err)

		// We should expect a notification for each dormant workspace.
		sent := notifyEnq.Sent()
		require.Len(t, sent, len(dormantWorkspaces))
		for i, dormantWs := range dormantWorkspaces {
			require.Equal(t, sent[i].UserID, dormantWs.OwnerID)
			require.Equal(t, sent[i].TemplateID, notifications.TemplateWorkspaceMarkedForDeletion)
			require.Contains(t, sent[i].Targets, template.ID)
			require.Contains(t, sent[i].Targets, dormantWs.ID)
			require.Contains(t, sent[i].Targets, dormantWs.OrganizationID)
			require.Contains(t, sent[i].Targets, dormantWs.OwnerID)
		}
	})
}

func TestTemplateTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		allowUserAutostop bool
		fromTTL           time.Duration
		toTTL             time.Duration
		expected          sql.NullInt64
	}{
		{
			name:              "AllowUserAutostopFalse/ModifyTTLDurationDown",
			allowUserAutostop: false,
			fromTTL:           24 * time.Hour,
			toTTL:             1 * time.Hour,
			expected:          sql.NullInt64{Valid: true, Int64: int64(1 * time.Hour)},
		},
		{
			name:              "AllowUserAutostopFalse/ModifyTTLDurationUp",
			allowUserAutostop: false,
			fromTTL:           24 * time.Hour,
			toTTL:             36 * time.Hour,
			expected:          sql.NullInt64{Valid: true, Int64: int64(36 * time.Hour)},
		},
		{
			name:              "AllowUserAutostopFalse/ModifyTTLDurationSame",
			allowUserAutostop: false,
			fromTTL:           24 * time.Hour,
			toTTL:             24 * time.Hour,
			expected:          sql.NullInt64{Valid: true, Int64: int64(24 * time.Hour)},
		},
		{
			name:              "AllowUserAutostopFalse/DisableTTL",
			allowUserAutostop: false,
			fromTTL:           24 * time.Hour,
			toTTL:             0,
			expected:          sql.NullInt64{},
		},
		{
			name:              "AllowUserAutostopTrue/ModifyTTLDurationDown",
			allowUserAutostop: true,
			fromTTL:           24 * time.Hour,
			toTTL:             1 * time.Hour,
			expected:          sql.NullInt64{Valid: true, Int64: int64(24 * time.Hour)},
		},
		{
			name:              "AllowUserAutostopTrue/ModifyTTLDurationUp",
			allowUserAutostop: true,
			fromTTL:           24 * time.Hour,
			toTTL:             36 * time.Hour,
			expected:          sql.NullInt64{Valid: true, Int64: int64(24 * time.Hour)},
		},
		{
			name:              "AllowUserAutostopTrue/ModifyTTLDurationSame",
			allowUserAutostop: true,
			fromTTL:           24 * time.Hour,
			toTTL:             24 * time.Hour,
			expected:          sql.NullInt64{Valid: true, Int64: int64(24 * time.Hour)},
		},
		{
			name:              "AllowUserAutostopTrue/DisableTTL",
			allowUserAutostop: true,
			fromTTL:           24 * time.Hour,
			toTTL:             0,
			expected:          sql.NullInt64{Valid: true, Int64: int64(24 * time.Hour)},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				db, _  = dbtestutil.NewDB(t)
				ctx    = testutil.Context(t, testutil.WaitLong)
				user   = dbgen.User(t, db, database.User{})
				file   = dbgen.File(t, db, database.File{CreatedBy: user.ID})
				// Create first template
				templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					FileID:      file.ID,
					InitiatorID: user.ID,
					Tags:        database.StringMap{"foo": "bar"},
				})
				templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					JobID:          templateJob.ID,
					OrganizationID: templateJob.OrganizationID,
				})
				template = dbgen.Template(t, db, database.Template{
					ActiveVersionID:   templateVersion.ID,
					CreatedBy:         user.ID,
					OrganizationID:    templateJob.OrganizationID,
					AllowUserAutostop: false,
				})
				// Create second template
				otherTTL         = tt.fromTTL + 6*time.Hour
				otherTemplateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					FileID:      file.ID,
					InitiatorID: user.ID,
					Tags:        database.StringMap{"foo": "bar"},
				})
				otherTemplateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					CreatedBy:      user.ID,
					JobID:          otherTemplateJob.ID,
					OrganizationID: otherTemplateJob.OrganizationID,
				})
				otherTemplate = dbgen.Template(t, db, database.Template{
					ActiveVersionID:   otherTemplateVersion.ID,
					CreatedBy:         user.ID,
					OrganizationID:    otherTemplateJob.OrganizationID,
					AllowUserAutostop: false,
				})
			)

			// Setup the template schedule store
			notifyEnq := notifications.NewNoopEnqueuer()
			const userQuietHoursSchedule = "CRON_TZ=UTC 0 0 * * *" // midnight UTC
			userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
			require.NoError(t, err)
			userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
			userQuietHoursStorePtr.Store(&userQuietHoursStore)
			templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr, notifyEnq, logger, nil)

			// Set both template's default TTL
			template, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
				DefaultTTL: tt.fromTTL,
			})
			require.NoError(t, err)
			otherTemplate, err = templateScheduleStore.Set(ctx, db, otherTemplate, agplschedule.TemplateScheduleOptions{
				DefaultTTL: otherTTL,
			})
			require.NoError(t, err)

			// We create two workspaces here, one with the template we're modifying, the
			// other with a different template. We want to ensure we only modify one
			// of the workspaces.
			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     template.ID,
				OrganizationID: templateJob.OrganizationID,
				LastUsedAt:     dbtime.Now(),
				Ttl:            sql.NullInt64{Valid: true, Int64: int64(tt.fromTTL)},
			})
			otherWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				TemplateID:     otherTemplate.ID,
				OrganizationID: otherTemplateJob.OrganizationID,
				LastUsedAt:     dbtime.Now(),
				Ttl:            sql.NullInt64{Valid: true, Int64: int64(otherTTL)},
			})

			// Ensure the workspace's start with the correct TTLs
			require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(tt.fromTTL)}, workspace.Ttl)
			require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(otherTTL)}, otherWorkspace.Ttl)

			// Update _only_ the primary template's TTL
			_, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
				UserAutostopEnabled: tt.allowUserAutostop,
				DefaultTTL:          tt.toTTL,
			})
			require.NoError(t, err)

			// Verify the primary workspace's TTL is what we expect
			ws, err := db.GetWorkspaceByID(ctx, workspace.ID)
			require.NoError(t, err)
			require.Equal(t, tt.expected, ws.Ttl)

			// Verify we haven't changed the other workspace's TTL
			ws, err = db.GetWorkspaceByID(ctx, otherWorkspace.ID)
			require.NoError(t, err)
			require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(otherTTL)}, ws.Ttl)
		})
	}

	t.Run("WorkspaceTTLUpdatedWhenAllowUserAutostopGetsDisabled", func(t *testing.T) {
		t.Parallel()

		var (
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
			db, _  = dbtestutil.NewDB(t)
			ctx    = testutil.Context(t, testutil.WaitLong)
			user   = dbgen.User(t, db, database.User{})
			file   = dbgen.File(t, db, database.File{CreatedBy: user.ID})
			// Create first template
			templateJob = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				FileID:      file.ID,
				InitiatorID: user.ID,
				Tags:        database.StringMap{"foo": "bar"},
			})
			templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				JobID:          templateJob.ID,
				OrganizationID: templateJob.OrganizationID,
			})
			template = dbgen.Template(t, db, database.Template{
				ActiveVersionID: templateVersion.ID,
				CreatedBy:       user.ID,
				OrganizationID:  templateJob.OrganizationID,
			})
		)

		// Setup the template schedule store
		notifyEnq := notifications.NewNoopEnqueuer()
		const userQuietHoursSchedule = "CRON_TZ=UTC 0 0 * * *" // midnight UTC
		userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
		require.NoError(t, err)
		userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
		userQuietHoursStorePtr.Store(&userQuietHoursStore)
		templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr, notifyEnq, logger, nil)

		// Enable AllowUserAutostop
		template, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
			DefaultTTL:          24 * time.Hour,
			UserAutostopEnabled: true,
		})
		require.NoError(t, err)

		// Create a workspace with a TTL different than the template's default TTL
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			TemplateID:     template.ID,
			OrganizationID: templateJob.OrganizationID,
			LastUsedAt:     dbtime.Now(),
			Ttl:            sql.NullInt64{Valid: true, Int64: int64(48 * time.Hour)},
		})

		// Ensure the workspace start with the correct TTLs
		require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(48 * time.Hour)}, workspace.Ttl)

		// Disable AllowUserAutostop
		template, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
			DefaultTTL:          23 * time.Hour,
			UserAutostopEnabled: false,
		})
		require.NoError(t, err)

		// Ensure the workspace ends with the correct TTLs
		ws, err := db.GetWorkspaceByID(ctx, workspace.ID)
		require.NoError(t, err)
		require.Equal(t, sql.NullInt64{Valid: true, Int64: int64(23 * time.Hour)}, ws.Ttl)
	})
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
