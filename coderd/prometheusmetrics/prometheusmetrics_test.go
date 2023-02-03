package prometheusmetrics_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestActiveUsers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Database func(t *testing.T) database.Store
		Count    int
	}{{
		Name: "None",
		Database: func(t *testing.T) database.Store {
			return dbfake.New()
		},
		Count: 0,
	}, {
		Name: "One",
		Database: func(t *testing.T) database.Store {
			db := dbfake.New()
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})
			return db
		},
		Count: 1,
	}, {
		Name: "OneWithExpired",
		Database: func(t *testing.T) database.Store {
			db := dbfake.New()

			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})

			// Because this API key hasn't been used in the past hour, this shouldn't
			// add to the user count.
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now().Add(-2 * time.Hour),
			})
			return db
		},
		Count: 1,
	}, {
		Name: "Multiple",
		Database: func(t *testing.T) database.Store {
			db := dbfake.New()
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})
			dbgen.APIKey(t, db, database.APIKey{
				LastUsed: database.Now(),
			})
			return db
		},
		Count: 2,
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			cancel, err := prometheusmetrics.ActiveUsers(context.Background(), registry, tc.Database(t), time.Millisecond)
			require.NoError(t, err)
			t.Cleanup(cancel)

			require.Eventually(t, func() bool {
				metrics, err := registry.Gather()
				assert.NoError(t, err)
				result := int(*metrics[0].Metric[0].Gauge.Value)
				return result == tc.Count
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}

func TestWorkspaces(t *testing.T) {
	t.Parallel()

	insertRunning := func(db database.Store) database.ProvisionerJob {
		job, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			CreatedAt:     database.Now(),
			UpdatedAt:     database.Now(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		})
		require.NoError(t, err)
		_, err = db.InsertWorkspaceBuild(context.Background(), database.InsertWorkspaceBuildParams{
			ID:          uuid.New(),
			WorkspaceID: uuid.New(),
			JobID:       job.ID,
			BuildNumber: 1,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		})
		require.NoError(t, err)
		// This marks the job as started.
		_, err = db.AcquireProvisionerJob(context.Background(), database.AcquireProvisionerJobParams{
			StartedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
			Types: []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		require.NoError(t, err)
		return job
	}

	insertCanceled := func(db database.Store) {
		job := insertRunning(db)
		err := db.UpdateProvisionerJobWithCancelByID(context.Background(), database.UpdateProvisionerJobWithCancelByIDParams{
			ID: job.ID,
			CanceledAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
		err = db.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
	}

	insertFailed := func(db database.Store) {
		job := insertRunning(db)
		err := db.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
			Error: sql.NullString{
				String: "failed",
				Valid:  true,
			},
		})
		require.NoError(t, err)
	}

	insertSuccess := func(db database.Store) {
		job := insertRunning(db)
		err := db.UpdateProvisionerJobWithCompleteByID(context.Background(), database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			},
		})
		require.NoError(t, err)
	}

	for _, tc := range []struct {
		Name     string
		Database func() database.Store
		Total    int
		Status   map[codersdk.ProvisionerJobStatus]int
	}{{
		Name: "None",
		Database: func() database.Store {
			return dbfake.New()
		},
		Total: 0,
	}, {
		Name: "Multiple",
		Database: func() database.Store {
			db := dbfake.New()
			insertCanceled(db)
			insertFailed(db)
			insertFailed(db)
			insertSuccess(db)
			insertSuccess(db)
			insertSuccess(db)
			insertRunning(db)
			return db
		},
		Total: 7,
		Status: map[codersdk.ProvisionerJobStatus]int{
			codersdk.ProvisionerJobCanceled:  1,
			codersdk.ProvisionerJobFailed:    2,
			codersdk.ProvisionerJobSucceeded: 3,
			codersdk.ProvisionerJobRunning:   1,
		},
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			cancel, err := prometheusmetrics.Workspaces(context.Background(), registry, tc.Database(), time.Millisecond)
			require.NoError(t, err)
			t.Cleanup(cancel)

			require.Eventually(t, func() bool {
				metrics, err := registry.Gather()
				assert.NoError(t, err)
				if len(metrics) < 1 {
					return false
				}
				sum := 0
				for _, metric := range metrics[0].Metric {
					count, ok := tc.Status[codersdk.ProvisionerJobStatus(metric.Label[0].GetValue())]
					if metric.Gauge.GetValue() == 0 {
						continue
					}
					if !ok {
						t.Fail()
					}
					if metric.Gauge.GetValue() != float64(count) {
						return false
					}
					sum += int(metric.Gauge.GetValue())
				}
				t.Logf("sum %d == total %d", sum, tc.Total)
				return sum == tc.Total
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}
