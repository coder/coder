//go:build linux

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/migrations"
)

func TestGetDeploymentWorkspaceAgentStats(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	t.Run("Aggregates", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		err := migrations.Up(sqlDB)
		require.NoError(t, err)
		db := database.New(sqlDB)
		ctx := context.Background()
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 1,
			SessionCountVSCode:        1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 2,
			SessionCountVSCode:        1,
		})
		stats, err := db.GetDeploymentWorkspaceAgentStats(ctx, database.Now().Add(-time.Hour))
		require.NoError(t, err)

		require.Equal(t, int64(2), stats.WorkspaceTxBytes)
		require.Equal(t, int64(2), stats.WorkspaceRxBytes)
		require.Equal(t, 1.5, stats.WorkspaceConnectionLatency50)
		require.Equal(t, 1.95, stats.WorkspaceConnectionLatency95)
		require.Equal(t, int64(2), stats.SessionCountVSCode)
	})

	t.Run("GroupsByAgentID", func(t *testing.T) {
		t.Parallel()

		sqlDB := testSQLDB(t)
		err := migrations.Up(sqlDB)
		require.NoError(t, err)
		db := database.New(sqlDB)
		ctx := context.Background()
		agentID := uuid.New()
		insertTime := database.Now()
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime.Add(-time.Second),
			AgentID:                   agentID,
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 1,
			SessionCountVSCode:        1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			// Ensure this stat is newer!
			CreatedAt:                 insertTime,
			AgentID:                   agentID,
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 2,
			SessionCountVSCode:        1,
		})
		stats, err := db.GetDeploymentWorkspaceAgentStats(ctx, database.Now().Add(-time.Hour))
		require.NoError(t, err)

		require.Equal(t, int64(2), stats.WorkspaceTxBytes)
		require.Equal(t, int64(2), stats.WorkspaceRxBytes)
		require.Equal(t, 1.5, stats.WorkspaceConnectionLatency50)
		require.Equal(t, 1.95, stats.WorkspaceConnectionLatency95)
		require.Equal(t, int64(1), stats.SessionCountVSCode)
	})
}

func TestInsertWorkspaceAgentStartupLogs(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	sqlDB := testSQLDB(t)
	ctx := context.Background()
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	org := dbgen.Organization(t, db, database.Organization{})
	job := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
		OrganizationID: org.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})
	logs, err := db.InsertWorkspaceAgentStartupLogs(ctx, database.InsertWorkspaceAgentStartupLogsParams{
		AgentID:   agent.ID,
		CreatedAt: []time.Time{database.Now()},
		Output:    []string{"first"},
		// 1 MB is the max
		OutputLength: 1 << 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), logs[0].ID)

	_, err = db.InsertWorkspaceAgentStartupLogs(ctx, database.InsertWorkspaceAgentStartupLogsParams{
		AgentID:      agent.ID,
		CreatedAt:    []time.Time{database.Now()},
		Output:       []string{"second"},
		OutputLength: 1,
	})
	require.True(t, database.IsStartupLogsLimitError(err))
}
