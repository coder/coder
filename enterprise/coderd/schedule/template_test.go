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

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
	agplschedule "github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/enterprise/coderd/schedule"
	"github.com/coder/coder/testutil"
)

func TestTemplateUpdateBuildDeadlines(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	var (
		org  = dbgen.Organization(t, db, database.Organization{})
		user = dbgen.User(t, db, database.User{})
		file = dbgen.File(t, db, database.File{
			CreatedBy: user.ID,
		})
		templateJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			OrganizationID: org.ID,
			FileID:         file.ID,
			InitiatorID:    user.ID,
			Tags: database.StringMap{
				"foo": "bar",
			},
		})
		templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			JobID:          templateJob.ID,
		})
	)

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
	nextQuietHours := time.Date(nowY, nowM, nowD+1, 0, 0, 0, 0, time.UTC) // midnight tomorrow UTC

	// Workspace old max_deadline too soon
	cases := []struct {
		name           string
		now            time.Time
		deadline       time.Time
		maxDeadline    time.Time
		newDeadline    time.Time // 0 for no change
		newMaxDeadline time.Time
	}{
		{
			name:        "SkippedWorkspaceMaxDeadlineTooSoon",
			now:         buildTime,
			deadline:    buildTime,
			maxDeadline: buildTime.Add(1 * time.Hour),
			// Unchanged since the max deadline is too soon.
			newDeadline:    time.Time{},
			newMaxDeadline: buildTime.Add(1 * time.Hour),
		},
		{
			name: "NewWorkspaceMaxDeadlineBeforeNow",
			// After the new max deadline...
			now:      nextQuietHours.Add(6 * time.Hour),
			deadline: buildTime,
			// Far into the future...
			maxDeadline: nextQuietHours.Add(24 * time.Hour),
			newDeadline: time.Time{},
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
			newDeadline: time.Time{},
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
			newDeadline:    time.Time{},
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
			newDeadline:    nextQuietHours,
			newMaxDeadline: nextQuietHours,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			t.Log("buildTime", buildTime)
			t.Log("nextQuietHours", nextQuietHours)
			t.Log("now", c.now)
			t.Log("deadline", c.deadline)
			t.Log("maxDeadline", c.maxDeadline)
			t.Log("newDeadline", c.newDeadline)
			t.Log("newMaxDeadline", c.newMaxDeadline)

			var (
				template = dbgen.Template(t, db, database.Template{
					OrganizationID:  org.ID,
					ActiveVersionID: templateVersion.ID,
					CreatedBy:       user.ID,
				})
				ws = dbgen.Workspace(t, db, database.Workspace{
					OrganizationID: org.ID,
					OwnerID:        user.ID,
					TemplateID:     template.ID,
				})
				job = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
					OrganizationID: org.ID,
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
				})
			)

			acquiredJob, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
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

			err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
				ID:               wsBuild.ID,
				UpdatedAt:        buildTime,
				ProvisionerState: []byte{},
				Deadline:         c.deadline,
				MaxDeadline:      c.maxDeadline,
			})
			require.NoError(t, err)

			wsBuild, err = db.GetWorkspaceBuildByID(ctx, wsBuild.ID)
			require.NoError(t, err)

			userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule)
			require.NoError(t, err)
			userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
			userQuietHoursStorePtr.Store(&userQuietHoursStore)

			// Set the template policy.
			templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr)
			templateScheduleStore.UseRestartRequirement.Store(true)
			templateScheduleStore.TimeNowFn = func() time.Time {
				return c.now
			}
			_, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
				UserAutostartEnabled:  false,
				UserAutostopEnabled:   false,
				DefaultTTL:            0,
				MaxTTL:                0,
				UseRestartRequirement: true,
				RestartRequirement: agplschedule.TemplateRestartRequirement{
					// Every day
					DaysOfWeek: 0b01111111,
					Weeks:      0,
				},
				FailureTTL:    0,
				InactivityTTL: 0,
				LockedTTL:     0,
			})
			require.NoError(t, err)

			// Check that the workspace build has the expected deadlines.
			newBuild, err := db.GetWorkspaceBuildByID(ctx, wsBuild.ID)
			require.NoError(t, err)

			if c.newDeadline.IsZero() {
				c.newDeadline = wsBuild.Deadline
			}
			require.WithinDuration(t, c.newDeadline, newBuild.Deadline, time.Second)
			require.WithinDuration(t, c.newMaxDeadline, newBuild.MaxDeadline, time.Second)
		})
	}
}

func TestTemplateUpdateBuildDeadlinesSkip(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	var (
		org  = dbgen.Organization(t, db, database.Organization{})
		user = dbgen.User(t, db, database.User{})
		file = dbgen.File(t, db, database.File{
			CreatedBy: user.ID,
		})
		templateJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			OrganizationID: org.ID,
			FileID:         file.ID,
			InitiatorID:    user.ID,
			Tags: database.StringMap{
				"foo": "bar",
			},
		})
		templateVersion = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			JobID:          templateJob.ID,
		})
		template = dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			ActiveVersionID: templateVersion.ID,
			CreatedBy:       user.ID,
		})
		otherTemplate = dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			ActiveVersionID: templateVersion.ID,
			CreatedBy:       user.ID,
		})
	)

	// Create a workspace that will be shared by two builds.
	ws := dbgen.Workspace(t, db, database.Workspace{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
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
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				TemplateID:     b.templateID,
			})
			wsID = ws.ID
		}
		job := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			OrganizationID: org.ID,
			FileID:         file.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			Tags: database.StringMap{
				wsID.String(): "yeah",
			},
		})
		wsBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wsID,
			BuildNumber:       b.buildNumber,
			JobID:             job.ID,
			InitiatorID:       user.ID,
			TemplateVersionID: templateVersion.ID,
		})

		err := db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
			ID:               wsBuild.ID,
			UpdatedAt:        buildTime,
			ProvisionerState: []byte{},
			Deadline:         originalMaxDeadline,
			MaxDeadline:      originalMaxDeadline,
		})
		require.NoError(t, err)

		wsBuild, err = db.GetWorkspaceBuildByID(ctx, wsBuild.ID)
		require.NoError(t, err)

		builds[i].wsBuild = wsBuild

		if !b.buildStarted {
			continue
		}

		acquiredJob, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
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

	userQuietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(userQuietHoursSchedule)
	require.NoError(t, err)
	userQuietHoursStorePtr := &atomic.Pointer[agplschedule.UserQuietHoursScheduleStore]{}
	userQuietHoursStorePtr.Store(&userQuietHoursStore)

	// Set the template policy.
	templateScheduleStore := schedule.NewEnterpriseTemplateScheduleStore(userQuietHoursStorePtr)
	templateScheduleStore.UseRestartRequirement.Store(true)
	templateScheduleStore.TimeNowFn = func() time.Time {
		return now
	}
	_, err = templateScheduleStore.Set(ctx, db, template, agplschedule.TemplateScheduleOptions{
		UserAutostartEnabled:  false,
		UserAutostopEnabled:   false,
		DefaultTTL:            0,
		MaxTTL:                0,
		UseRestartRequirement: true,
		RestartRequirement: agplschedule.TemplateRestartRequirement{
			// Every day
			DaysOfWeek: 0b01111111,
			Weeks:      0,
		},
		FailureTTL:    0,
		InactivityTTL: 0,
		LockedTTL:     0,
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
	}
}
