package unhanger_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/unhanger"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDetectorNoJobs(t *testing.T) {
	t.Parallel()

	var (
		ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
		db, pubsub  = dbtestutil.NewDB(t)
		log         = slogtest.Make(t, nil)
		tickCh      = make(chan time.Time)
		statsCh     = make(chan unhanger.Stats)
	)

	detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()
	tickCh <- time.Now()

	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Empty(t, stats.HungJobIDs)

	cancel()
	detector.Wait()
}

func TestDetectorNoHungJobs(t *testing.T) {
	t.Parallel()

	var (
		ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
		db, pubsub  = dbtestutil.NewDB(t)
		log         = slogtest.Make(t, nil)
		tickCh      = make(chan time.Time)
		statsCh     = make(chan unhanger.Stats)
	)

	// Insert some jobs that are running and haven't been updated in a while,
	// but not enough to be considered hung.
	now := time.Now()
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	file := dbgen.File(t, db, database.File{})
	for i := 0; i < 5; i++ {
		dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: now.Add(-time.Minute * 5),
			UpdatedAt: now.Add(-time.Minute * time.Duration(i)),
			StartedAt: sql.NullTime{
				Time:  now.Add(-time.Minute * 5),
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
	}

	detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()
	tickCh <- now

	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Empty(t, stats.HungJobIDs)

	cancel()
	detector.Wait()
}

func TestDetectorHungWorkspaceBuild(t *testing.T) {
	t.Parallel()

	var (
		ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
		db, pubsub  = dbtestutil.NewDB(t)
		log         = slogtest.Make(t, nil)
		tickCh      = make(chan time.Time)
		statsCh     = make(chan unhanger.Stats)
	)

	var (
		now          = time.Now()
		twentyMinAgo = now.Add(-time.Minute * 20)
		tenMinAgo    = now.Add(-time.Minute * 10)
		sixMinAgo    = now.Add(-time.Minute * 6)
		org          = dbgen.Organization(t, db, database.Organization{})
		user         = dbgen.User(t, db, database.User{})
		file         = dbgen.File(t, db, database.File{})
		workspace    = dbgen.Workspace(t, db, database.Workspace{})

		// Previous build.
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
		previousWorkspaceBuildJob   = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: twentyMinAgo,
			UpdatedAt: twentyMinAgo,
			StartedAt: sql.NullTime{
				Time:  twentyMinAgo,
				Valid: true,
			},
			CompletedAt: sql.NullTime{
				Time:  twentyMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:      workspace.ID,
			BuildNumber:      1,
			ProvisionerState: expectedWorkspaceBuildState,
			JobID:            previousWorkspaceBuildJob.ID,
		})

		// Current build.
		currentWorkspaceBuildJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
		currentWorkspaceBuild = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID: workspace.ID,
			BuildNumber: 2,
			JobID:       currentWorkspaceBuildJob.ID,
			// No provisioner state.
		})
	)

	t.Log("previous job ID: ", previousWorkspaceBuildJob.ID)
	t.Log("current job ID: ", currentWorkspaceBuildJob.ID)

	detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()
	tickCh <- now

	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Len(t, stats.HungJobIDs, 1)
	require.Equal(t, currentWorkspaceBuildJob.ID, stats.HungJobIDs[0])

	// Check that the current provisioner job was updated.
	job, err := db.GetProvisionerJobByID(ctx, currentWorkspaceBuildJob.ID)
	require.NoError(t, err)
	require.WithinDuration(t, now, job.UpdatedAt, 30*time.Second)
	require.True(t, job.CompletedAt.Valid)
	require.WithinDuration(t, now, job.CompletedAt.Time, 30*time.Second)
	require.True(t, job.Error.Valid)
	require.Contains(t, job.Error.String, "Build has been detected as hung")
	require.False(t, job.ErrorCode.Valid)

	// Check that the provisioner state was copied.
	build, err := db.GetWorkspaceBuildByID(ctx, currentWorkspaceBuild.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, build.ProvisionerState)

	cancel()
	detector.Wait()
}

func TestDetectorHungWorkspaceBuildNoOverrideState(t *testing.T) {
	t.Parallel()

	var (
		ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
		db, pubsub  = dbtestutil.NewDB(t)
		log         = slogtest.Make(t, nil)
		tickCh      = make(chan time.Time)
		statsCh     = make(chan unhanger.Stats)
	)

	var (
		now          = time.Now()
		twentyMinAgo = now.Add(-time.Minute * 20)
		tenMinAgo    = now.Add(-time.Minute * 10)
		sixMinAgo    = now.Add(-time.Minute * 6)
		org          = dbgen.Organization(t, db, database.Organization{})
		user         = dbgen.User(t, db, database.User{})
		file         = dbgen.File(t, db, database.File{})
		workspace    = dbgen.Workspace(t, db, database.Workspace{})

		// Previous build.
		previousWorkspaceBuildJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: twentyMinAgo,
			UpdatedAt: twentyMinAgo,
			StartedAt: sql.NullTime{
				Time:  twentyMinAgo,
				Valid: true,
			},
			CompletedAt: sql.NullTime{
				Time:  twentyMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:      workspace.ID,
			BuildNumber:      1,
			ProvisionerState: []byte(`{"dean":"NOT cool","colin":"also NOT cool"}`),
			JobID:            previousWorkspaceBuildJob.ID,
		})

		// Current build.
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
		currentWorkspaceBuildJob    = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
		currentWorkspaceBuild = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID: workspace.ID,
			BuildNumber: 2,
			JobID:       currentWorkspaceBuildJob.ID,
			// Should not be overridden.
			ProvisionerState: expectedWorkspaceBuildState,
		})
	)

	t.Log("previous job ID: ", previousWorkspaceBuildJob.ID)
	t.Log("current job ID: ", currentWorkspaceBuildJob.ID)

	detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()
	tickCh <- now

	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Len(t, stats.HungJobIDs, 1)
	require.Equal(t, currentWorkspaceBuildJob.ID, stats.HungJobIDs[0])

	// Check that the current provisioner job was updated.
	job, err := db.GetProvisionerJobByID(ctx, currentWorkspaceBuildJob.ID)
	require.NoError(t, err)
	require.WithinDuration(t, now, job.UpdatedAt, 30*time.Second)
	require.True(t, job.CompletedAt.Valid)
	require.WithinDuration(t, now, job.CompletedAt.Time, 30*time.Second)
	require.True(t, job.Error.Valid)
	require.Contains(t, job.Error.String, "Build has been detected as hung")
	require.False(t, job.ErrorCode.Valid)

	// Check that the provisioner state was NOT copied.
	build, err := db.GetWorkspaceBuildByID(ctx, currentWorkspaceBuild.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, build.ProvisionerState)

	cancel()
	detector.Wait()
}

func TestDetectorHungWorkspaceBuildNoOverrideStateIfNoExistingBuild(t *testing.T) {
	t.Parallel()

	var (
		ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
		db, pubsub  = dbtestutil.NewDB(t)
		log         = slogtest.Make(t, nil)
		tickCh      = make(chan time.Time)
		statsCh     = make(chan unhanger.Stats)
	)

	var (
		now       = time.Now()
		tenMinAgo = now.Add(-time.Minute * 10)
		sixMinAgo = now.Add(-time.Minute * 6)
		org       = dbgen.Organization(t, db, database.Organization{})
		user      = dbgen.User(t, db, database.User{})
		file      = dbgen.File(t, db, database.File{})
		workspace = dbgen.Workspace(t, db, database.Workspace{})

		// First build.
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
		currentWorkspaceBuildJob    = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
		currentWorkspaceBuild = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID: workspace.ID,
			BuildNumber: 1,
			JobID:       currentWorkspaceBuildJob.ID,
			// Should not be overridden.
			ProvisionerState: expectedWorkspaceBuildState,
		})
	)

	t.Log("current job ID: ", currentWorkspaceBuildJob.ID)

	detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()
	tickCh <- now

	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Len(t, stats.HungJobIDs, 1)
	require.Equal(t, currentWorkspaceBuildJob.ID, stats.HungJobIDs[0])

	// Check that the current provisioner job was updated.
	job, err := db.GetProvisionerJobByID(ctx, currentWorkspaceBuildJob.ID)
	require.NoError(t, err)
	require.WithinDuration(t, now, job.UpdatedAt, 30*time.Second)
	require.True(t, job.CompletedAt.Valid)
	require.WithinDuration(t, now, job.CompletedAt.Time, 30*time.Second)
	require.True(t, job.Error.Valid)
	require.Contains(t, job.Error.String, "Build has been detected as hung")
	require.False(t, job.ErrorCode.Valid)

	// Check that the provisioner state was NOT updated.
	build, err := db.GetWorkspaceBuildByID(ctx, currentWorkspaceBuild.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, build.ProvisionerState)

	cancel()
	detector.Wait()
}

func TestDetectorHungOtherJobTypes(t *testing.T) {
	t.Parallel()

	var (
		ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
		db, pubsub  = dbtestutil.NewDB(t)
		log         = slogtest.Make(t, nil)
		tickCh      = make(chan time.Time)
		statsCh     = make(chan unhanger.Stats)
	)

	var (
		now       = time.Now()
		tenMinAgo = now.Add(-time.Minute * 10)
		sixMinAgo = now.Add(-time.Minute * 6)
		org       = dbgen.Organization(t, db, database.Organization{})
		user      = dbgen.User(t, db, database.User{})
		file      = dbgen.File(t, db, database.File{})

		// Template import job.
		templateImportJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte("{}"),
		})

		// Template dry-run job.
		templateDryRunJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
			Input:          []byte("{}"),
		})
	)

	t.Log("template import job ID: ", templateImportJob.ID)
	t.Log("template dry-run job ID: ", templateDryRunJob.ID)

	detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()
	tickCh <- now

	stats := <-statsCh
	require.NoError(t, stats.Error)
	require.Len(t, stats.HungJobIDs, 2)
	require.Contains(t, stats.HungJobIDs, templateImportJob.ID)
	require.Contains(t, stats.HungJobIDs, templateDryRunJob.ID)

	// Check that the template import job was updated.
	job, err := db.GetProvisionerJobByID(ctx, templateImportJob.ID)
	require.NoError(t, err)
	require.WithinDuration(t, now, job.UpdatedAt, 30*time.Second)
	require.True(t, job.CompletedAt.Valid)
	require.WithinDuration(t, now, job.CompletedAt.Time, 30*time.Second)
	require.True(t, job.Error.Valid)
	require.Contains(t, job.Error.String, "Build has been detected as hung")
	require.False(t, job.ErrorCode.Valid)

	// Check that the template dry-run job was updated.
	job, err = db.GetProvisionerJobByID(ctx, templateDryRunJob.ID)
	require.NoError(t, err)
	require.WithinDuration(t, now, job.UpdatedAt, 30*time.Second)
	require.True(t, job.CompletedAt.Valid)
	require.WithinDuration(t, now, job.CompletedAt.Time, 30*time.Second)
	require.True(t, job.Error.Valid)
	require.Contains(t, job.Error.String, "Build has been detected as hung")
	require.False(t, job.ErrorCode.Valid)

	cancel()
	detector.Wait()
}

func TestDetectorPushesLogs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		preLogCount int
		preLogStage string
		expectStage string
	}{
		{
			name:        "WithExistingLogs",
			preLogCount: 10,
			preLogStage: "Stage Name",
			expectStage: "Stage Name",
		},
		{
			name:        "WithExistingLogsNoStage",
			preLogCount: 10,
			preLogStage: "",
			expectStage: "Unknown",
		},
		{
			name:        "WithoutExistingLogs",
			preLogCount: 0,
			expectStage: "Unknown",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			var (
				ctx, cancel = context.WithCancel(testutil.Context(t, testutil.WaitLong))
				db, pubsub  = dbtestutil.NewDB(t)
				log         = slogtest.Make(t, nil)
				tickCh      = make(chan time.Time)
				statsCh     = make(chan unhanger.Stats)
			)

			var (
				now       = time.Now()
				tenMinAgo = now.Add(-time.Minute * 10)
				sixMinAgo = now.Add(-time.Minute * 6)
				org       = dbgen.Organization(t, db, database.Organization{})
				user      = dbgen.User(t, db, database.User{})
				file      = dbgen.File(t, db, database.File{})

				// Template import job.
				templateImportJob = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
					CreatedAt: tenMinAgo,
					UpdatedAt: sixMinAgo,
					StartedAt: sql.NullTime{
						Time:  tenMinAgo,
						Valid: true,
					},
					OrganizationID: org.ID,
					InitiatorID:    user.ID,
					Provisioner:    database.ProvisionerTypeEcho,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					FileID:         file.ID,
					Type:           database.ProvisionerJobTypeTemplateVersionImport,
					Input:          []byte("{}"),
				})
			)

			t.Log("template import job ID: ", templateImportJob.ID)

			// Insert some logs at the start of the job.
			if c.preLogCount > 0 {
				insertParams := database.InsertProvisionerJobLogsParams{
					JobID: templateImportJob.ID,
				}
				for i := 0; i < c.preLogCount; i++ {
					insertParams.CreatedAt = append(insertParams.CreatedAt, tenMinAgo.Add(time.Millisecond*time.Duration(i)))
					insertParams.Level = append(insertParams.Level, database.LogLevelInfo)
					insertParams.Stage = append(insertParams.Stage, c.preLogStage)
					insertParams.Source = append(insertParams.Source, database.LogSourceProvisioner)
					insertParams.Output = append(insertParams.Output, fmt.Sprintf("Output %d", i))
				}
				logs, err := db.InsertProvisionerJobLogs(ctx, insertParams)
				require.NoError(t, err)
				require.Len(t, logs, 10)
			}

			detector := unhanger.New(ctx, db, pubsub, log, tickCh).WithStatsChannel(statsCh)
			detector.Start()

			// Create pubsub subscription to listen for new log events.
			pubsubCalled := make(chan int64, 1)
			pubsubCancel, err := pubsub.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(templateImportJob.ID), func(ctx context.Context, message []byte) {
				defer close(pubsubCalled)
				var event provisionersdk.ProvisionerJobLogsNotifyMessage
				err := json.Unmarshal(message, &event)
				if !assert.NoError(t, err) {
					return
				}

				pubsubCalled <- event.CreatedAfter
			})
			require.NoError(t, err)
			defer pubsubCancel()

			tickCh <- now

			stats := <-statsCh
			require.NoError(t, stats.Error)
			require.Len(t, stats.HungJobIDs, 1)
			require.Contains(t, stats.HungJobIDs, templateImportJob.ID)

			after := <-pubsubCalled

			// Get the jobs after the given time and check that they are what we
			// expect.
			logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
				JobID:        templateImportJob.ID,
				CreatedAfter: after + 1,
			})
			require.NoError(t, err)
			require.Len(t, logs, len(unhanger.HungJobLogMessages))
			for i, log := range logs {
				assert.Equal(t, database.LogLevelError, log.Level)
				assert.Equal(t, c.expectStage, log.Stage)
				assert.Equal(t, database.LogSourceProvisionerDaemon, log.Source)
				assert.Equal(t, unhanger.HungJobLogMessages[i], log.Output)
			}

			// Double check the full log count.
			logs, err = db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
				JobID:        templateImportJob.ID,
				CreatedAfter: 0,
			})
			require.NoError(t, err)
			require.Len(t, logs, c.preLogCount+len(unhanger.HungJobLogMessages))

			cancel()
			detector.Wait()
		})
	}
}
