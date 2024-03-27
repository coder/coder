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

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	agplschedule "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/testutil"
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
				ws = dbgen.Workspace(t, db, database.Workspace{
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
				Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
				Tags:  json.RawMessage(fmt.Sprintf(`{%q: "yeah"}`, c.name)),
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

			userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
			require.NoError(t, err)
			userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
			userQuietHoursStorePtr.Store(&userQuietHoursStore)

			// Set the template policy.
			templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr)
			templateScheduleStore.TimeNowFn = func() time.Time {
				return c.now
			}

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
	ws := dbgen.Workspace(t, db, database.Workspace{
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
			ws := dbgen.Workspace(t, db, database.Workspace{
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
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags:  json.RawMessage(fmt.Sprintf(`{%q: "yeah"}`, wsID)),
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

	userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule, true)
	require.NoError(t, err)
	userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
	userQuietHoursStorePtr.Store(&userQuietHoursStore)

	// Set the template policy.
	templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr)
	templateScheduleStore.TimeNowFn = func() time.Time {
		return now
	}
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

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
