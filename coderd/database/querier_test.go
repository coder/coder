//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
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
		stats, err := db.GetDeploymentWorkspaceAgentStats(ctx, dbtime.Now().Add(-time.Hour))
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
		insertTime := dbtime.Now()
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
		stats, err := db.GetDeploymentWorkspaceAgentStats(ctx, dbtime.Now().Add(-time.Hour))
		require.NoError(t, err)

		require.Equal(t, int64(2), stats.WorkspaceTxBytes)
		require.Equal(t, int64(2), stats.WorkspaceRxBytes)
		require.Equal(t, 1.5, stats.WorkspaceConnectionLatency50)
		require.Equal(t, 1.95, stats.WorkspaceConnectionLatency95)
		require.Equal(t, int64(1), stats.SessionCountVSCode)
	})
}

func TestGetDeploymentWorkspaceAgentUsageStats(t *testing.T) {
	t.Parallel()

	t.Run("Aggregates", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		authz := rbac.NewAuthorizer(prometheus.NewRegistry())
		db = dbauthz.New(db, authz, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())
		ctx := context.Background()
		agentID := uuid.New()
		// Since the queries exclude the current minute
		insertTime := dbtime.Now().Add(-time.Minute)

		// Old stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime.Add(-time.Minute),
			AgentID:                   agentID,
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountSSH:    4,
			SessionCountVSCode: 3,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:          insertTime.Add(-time.Minute),
			AgentID:            agentID,
			SessionCountVSCode: 1,
			Usage:              true,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                   insertTime.Add(-time.Minute),
			AgentID:                     agentID,
			SessionCountReconnectingPTY: 1,
			Usage:                       true,
		})

		// Latest stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agentID,
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 2,
			// Should be ignored
			SessionCountSSH:    3,
			SessionCountVSCode: 1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:          insertTime,
			AgentID:            agentID,
			SessionCountVSCode: 1,
			Usage:              true,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:       insertTime,
			AgentID:         agentID,
			SessionCountSSH: 1,
			Usage:           true,
		})

		stats, err := db.GetDeploymentWorkspaceAgentUsageStats(ctx, dbtime.Now().Add(-time.Hour))
		require.NoError(t, err)

		require.Equal(t, int64(2), stats.WorkspaceTxBytes)
		require.Equal(t, int64(2), stats.WorkspaceRxBytes)
		require.Equal(t, 1.5, stats.WorkspaceConnectionLatency50)
		require.Equal(t, 1.95, stats.WorkspaceConnectionLatency95)
		require.Equal(t, int64(1), stats.SessionCountVSCode)
		require.Equal(t, int64(1), stats.SessionCountSSH)
		require.Equal(t, int64(0), stats.SessionCountReconnectingPTY)
		require.Equal(t, int64(0), stats.SessionCountJetBrains)
	})

	t.Run("NoUsage", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		authz := rbac.NewAuthorizer(prometheus.NewRegistry())
		db = dbauthz.New(db, authz, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())
		ctx := context.Background()
		agentID := uuid.New()
		// Since the queries exclude the current minute
		insertTime := dbtime.Now().Add(-time.Minute)

		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agentID,
			TxBytes:                   3,
			RxBytes:                   4,
			ConnectionMedianLatencyMS: 2,
			// Should be ignored
			SessionCountSSH:    3,
			SessionCountVSCode: 1,
		})

		stats, err := db.GetDeploymentWorkspaceAgentUsageStats(ctx, dbtime.Now().Add(-time.Hour))
		require.NoError(t, err)

		require.Equal(t, int64(3), stats.WorkspaceTxBytes)
		require.Equal(t, int64(4), stats.WorkspaceRxBytes)
		require.Equal(t, int64(0), stats.SessionCountVSCode)
		require.Equal(t, int64(0), stats.SessionCountSSH)
		require.Equal(t, int64(0), stats.SessionCountReconnectingPTY)
		require.Equal(t, int64(0), stats.SessionCountJetBrains)
	})
}

func TestGetWorkspaceAgentUsageStats(t *testing.T) {
	t.Parallel()

	t.Run("Aggregates", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		authz := rbac.NewAuthorizer(prometheus.NewRegistry())
		db = dbauthz.New(db, authz, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())
		ctx := context.Background()
		// Since the queries exclude the current minute
		insertTime := dbtime.Now().Add(-time.Minute)

		agentID1 := uuid.New()
		agentID2 := uuid.New()
		workspaceID1 := uuid.New()
		workspaceID2 := uuid.New()
		templateID1 := uuid.New()
		templateID2 := uuid.New()
		userID1 := uuid.New()
		userID2 := uuid.New()

		// Old workspace 1 stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime.Add(-time.Minute),
			AgentID:                   agentID1,
			WorkspaceID:               workspaceID1,
			TemplateID:                templateID1,
			UserID:                    userID1,
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountVSCode: 3,
			SessionCountSSH:    1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:          insertTime.Add(-time.Minute),
			AgentID:            agentID1,
			WorkspaceID:        workspaceID1,
			TemplateID:         templateID1,
			UserID:             userID1,
			SessionCountVSCode: 1,
			Usage:              true,
		})

		// Latest workspace 1 stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agentID1,
			WorkspaceID:               workspaceID1,
			TemplateID:                templateID1,
			UserID:                    userID1,
			TxBytes:                   2,
			RxBytes:                   2,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountVSCode: 3,
			SessionCountSSH:    4,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:          insertTime,
			AgentID:            agentID1,
			WorkspaceID:        workspaceID1,
			TemplateID:         templateID1,
			UserID:             userID1,
			SessionCountVSCode: 1,
			Usage:              true,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:             insertTime,
			AgentID:               agentID1,
			WorkspaceID:           workspaceID1,
			TemplateID:            templateID1,
			UserID:                userID1,
			SessionCountJetBrains: 1,
			Usage:                 true,
		})

		// Latest workspace 2 stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agentID2,
			WorkspaceID:               workspaceID2,
			TemplateID:                templateID2,
			UserID:                    userID2,
			TxBytes:                   4,
			RxBytes:                   8,
			ConnectionMedianLatencyMS: 1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agentID2,
			WorkspaceID:               workspaceID2,
			TemplateID:                templateID2,
			UserID:                    userID2,
			TxBytes:                   2,
			RxBytes:                   3,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountVSCode: 3,
			SessionCountSSH:    4,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:       insertTime,
			AgentID:         agentID2,
			WorkspaceID:     workspaceID2,
			TemplateID:      templateID2,
			UserID:          userID2,
			SessionCountSSH: 1,
			Usage:           true,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:             insertTime,
			AgentID:               agentID2,
			WorkspaceID:           workspaceID2,
			TemplateID:            templateID2,
			UserID:                userID2,
			SessionCountJetBrains: 1,
			Usage:                 true,
		})

		reqTime := dbtime.Now().Add(-time.Hour)
		stats, err := db.GetWorkspaceAgentUsageStats(ctx, reqTime)
		require.NoError(t, err)

		ws1Stats, ws2Stats := stats[0], stats[1]
		if ws1Stats.WorkspaceID != workspaceID1 {
			ws1Stats, ws2Stats = ws2Stats, ws1Stats
		}
		require.Equal(t, int64(3), ws1Stats.WorkspaceTxBytes)
		require.Equal(t, int64(3), ws1Stats.WorkspaceRxBytes)
		require.Equal(t, int64(1), ws1Stats.SessionCountVSCode)
		require.Equal(t, int64(1), ws1Stats.SessionCountJetBrains)
		require.Equal(t, int64(0), ws1Stats.SessionCountSSH)
		require.Equal(t, int64(0), ws1Stats.SessionCountReconnectingPTY)

		require.Equal(t, int64(6), ws2Stats.WorkspaceTxBytes)
		require.Equal(t, int64(11), ws2Stats.WorkspaceRxBytes)
		require.Equal(t, int64(1), ws2Stats.SessionCountSSH)
		require.Equal(t, int64(1), ws2Stats.SessionCountJetBrains)
		require.Equal(t, int64(0), ws2Stats.SessionCountVSCode)
		require.Equal(t, int64(0), ws2Stats.SessionCountReconnectingPTY)
	})

	t.Run("NoUsage", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		authz := rbac.NewAuthorizer(prometheus.NewRegistry())
		db = dbauthz.New(db, authz, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())
		ctx := context.Background()
		// Since the queries exclude the current minute
		insertTime := dbtime.Now().Add(-time.Minute)

		agentID := uuid.New()

		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agentID,
			TxBytes:                   3,
			RxBytes:                   4,
			ConnectionMedianLatencyMS: 2,
			// Should be ignored
			SessionCountSSH:    3,
			SessionCountVSCode: 1,
		})

		stats, err := db.GetWorkspaceAgentUsageStats(ctx, dbtime.Now().Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, stats, 1)
		require.Equal(t, int64(3), stats[0].WorkspaceTxBytes)
		require.Equal(t, int64(4), stats[0].WorkspaceRxBytes)
		require.Equal(t, int64(0), stats[0].SessionCountVSCode)
		require.Equal(t, int64(0), stats[0].SessionCountSSH)
		require.Equal(t, int64(0), stats[0].SessionCountReconnectingPTY)
		require.Equal(t, int64(0), stats[0].SessionCountJetBrains)
	})
}

func TestGetWorkspaceAgentUsageStatsAndLabels(t *testing.T) {
	t.Parallel()

	t.Run("Aggregates", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		insertTime := dbtime.Now()

		// Insert user, agent, template, workspace
		user1 := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		job1 := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
		})
		resource1 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job1.ID,
		})
		agent1 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource1.ID,
		})
		template1 := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user1.ID,
		})
		workspace1 := dbgen.Workspace(t, db, database.Workspace{
			OwnerID:        user1.ID,
			OrganizationID: org.ID,
			TemplateID:     template1.ID,
		})
		user2 := dbgen.User(t, db, database.User{})
		job2 := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
		})
		resource2 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job2.ID,
		})
		agent2 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource2.ID,
		})
		template2 := dbgen.Template(t, db, database.Template{
			CreatedBy:      user1.ID,
			OrganizationID: org.ID,
		})
		workspace2 := dbgen.Workspace(t, db, database.Workspace{
			OwnerID:        user2.ID,
			OrganizationID: org.ID,
			TemplateID:     template2.ID,
		})

		// Old workspace 1 stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime.Add(-time.Minute),
			AgentID:                   agent1.ID,
			WorkspaceID:               workspace1.ID,
			TemplateID:                template1.ID,
			UserID:                    user1.ID,
			TxBytes:                   1,
			RxBytes:                   1,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountVSCode: 3,
			SessionCountSSH:    1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:          insertTime.Add(-time.Minute),
			AgentID:            agent1.ID,
			WorkspaceID:        workspace1.ID,
			TemplateID:         template1.ID,
			UserID:             user1.ID,
			SessionCountVSCode: 1,
			Usage:              true,
		})

		// Latest workspace 1 stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agent1.ID,
			WorkspaceID:               workspace1.ID,
			TemplateID:                template1.ID,
			UserID:                    user1.ID,
			TxBytes:                   2,
			RxBytes:                   2,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountVSCode: 4,
			SessionCountSSH:    3,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:             insertTime,
			AgentID:               agent1.ID,
			WorkspaceID:           workspace1.ID,
			TemplateID:            template1.ID,
			UserID:                user1.ID,
			SessionCountJetBrains: 1,
			Usage:                 true,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                   insertTime,
			AgentID:                     agent1.ID,
			WorkspaceID:                 workspace1.ID,
			TemplateID:                  template1.ID,
			UserID:                      user1.ID,
			SessionCountReconnectingPTY: 1,
			Usage:                       true,
		})

		// Latest workspace 2 stats
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime,
			AgentID:                   agent2.ID,
			WorkspaceID:               workspace2.ID,
			TemplateID:                template2.ID,
			UserID:                    user2.ID,
			TxBytes:                   4,
			RxBytes:                   8,
			ConnectionMedianLatencyMS: 1,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:          insertTime,
			AgentID:            agent2.ID,
			WorkspaceID:        workspace2.ID,
			TemplateID:         template2.ID,
			UserID:             user2.ID,
			SessionCountVSCode: 1,
			Usage:              true,
		})
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:       insertTime,
			AgentID:         agent2.ID,
			WorkspaceID:     workspace2.ID,
			TemplateID:      template2.ID,
			UserID:          user2.ID,
			SessionCountSSH: 1,
			Usage:           true,
		})

		stats, err := db.GetWorkspaceAgentUsageStatsAndLabels(ctx, insertTime.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, stats, 2)
		require.Contains(t, stats, database.GetWorkspaceAgentUsageStatsAndLabelsRow{
			Username:                    user1.Username,
			AgentName:                   agent1.Name,
			WorkspaceName:               workspace1.Name,
			TxBytes:                     3,
			RxBytes:                     3,
			SessionCountJetBrains:       1,
			SessionCountReconnectingPTY: 1,
			ConnectionMedianLatencyMS:   1,
		})

		require.Contains(t, stats, database.GetWorkspaceAgentUsageStatsAndLabelsRow{
			Username:                  user2.Username,
			AgentName:                 agent2.Name,
			WorkspaceName:             workspace2.Name,
			RxBytes:                   8,
			TxBytes:                   4,
			SessionCountVSCode:        1,
			SessionCountSSH:           1,
			ConnectionMedianLatencyMS: 1,
		})
	})

	t.Run("NoUsage", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		insertTime := dbtime.Now()
		// Insert user, agent, template, workspace
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		workspace := dbgen.Workspace(t, db, database.Workspace{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     template.ID,
		})

		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt:                 insertTime.Add(-time.Minute),
			AgentID:                   agent.ID,
			WorkspaceID:               workspace.ID,
			TemplateID:                template.ID,
			UserID:                    user.ID,
			RxBytes:                   4,
			TxBytes:                   5,
			ConnectionMedianLatencyMS: 1,
			// Should be ignored
			SessionCountVSCode: 3,
			SessionCountSSH:    1,
		})

		stats, err := db.GetWorkspaceAgentUsageStatsAndLabels(ctx, insertTime.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, stats, 1)
		require.Contains(t, stats, database.GetWorkspaceAgentUsageStatsAndLabelsRow{
			Username:                  user.Username,
			AgentName:                 agent.Name,
			WorkspaceName:             workspace.Name,
			RxBytes:                   4,
			TxBytes:                   5,
			ConnectionMedianLatencyMS: 1,
		})
	})
}

func TestInsertWorkspaceAgentLogs(t *testing.T) {
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
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})
	source := dbgen.WorkspaceAgentLogSource(t, db, database.WorkspaceAgentLogSource{
		WorkspaceAgentID: agent.ID,
	})
	logs, err := db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:     agent.ID,
		CreatedAt:   dbtime.Now(),
		Output:      []string{"first"},
		Level:       []database.LogLevel{database.LogLevelInfo},
		LogSourceID: source.ID,
		// 1 MB is the max
		OutputLength: 1 << 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), logs[0].ID)

	_, err = db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:      agent.ID,
		CreatedAt:    dbtime.Now(),
		Output:       []string{"second"},
		Level:        []database.LogLevel{database.LogLevelInfo},
		LogSourceID:  source.ID,
		OutputLength: 1,
	})
	require.True(t, database.IsWorkspaceAgentLogsLimitError(err))
}

func TestProxyByHostname(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	// Insert a bunch of different proxies.
	proxies := []struct {
		name             string
		accessURL        string
		wildcardHostname string
	}{
		{
			name:             "one",
			accessURL:        "https://one.coder.com",
			wildcardHostname: "*.wildcard.one.coder.com",
		},
		{
			name:             "two",
			accessURL:        "https://two.coder.com",
			wildcardHostname: "*--suffix.two.coder.com",
		},
	}
	for _, p := range proxies {
		dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{
			Name:             p.name,
			Url:              p.accessURL,
			WildcardHostname: p.wildcardHostname,
		})
	}

	cases := []struct {
		name              string
		testHostname      string
		allowAccessURL    bool
		allowWildcardHost bool
		matchProxyName    string
	}{
		{
			name:              "NoMatch",
			testHostname:      "test.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "MatchAccessURL",
			testHostname:      "one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "one",
		},
		{
			name:              "MatchWildcard",
			testHostname:      "something.wildcard.one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "one",
		},
		{
			name:              "MatchSuffix",
			testHostname:      "something--suffix.two.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "two",
		},
		{
			name:              "ValidateHostname/1",
			testHostname:      ".*ne.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "ValidateHostname/2",
			testHostname:      "https://one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "ValidateHostname/3",
			testHostname:      "one.coder.com:8080/hello",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "IgnoreAccessURLMatch",
			testHostname:      "one.coder.com",
			allowAccessURL:    false,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "IgnoreWildcardMatch",
			testHostname:      "hi.wildcard.one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: false,
			matchProxyName:    "",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			proxy, err := db.GetWorkspaceProxyByHostname(context.Background(), database.GetWorkspaceProxyByHostnameParams{
				Hostname:              c.testHostname,
				AllowAccessUrl:        c.allowAccessURL,
				AllowWildcardHostname: c.allowWildcardHost,
			})
			if c.matchProxyName == "" {
				require.ErrorIs(t, err, sql.ErrNoRows)
				require.Empty(t, proxy)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, proxy)
				require.Equal(t, c.matchProxyName, proxy.Name)
			}
		})
	}
}

func TestDefaultProxy(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	ctx := testutil.Context(t, testutil.WaitLong)
	depID := uuid.NewString()
	err = db.InsertDeploymentID(ctx, depID)
	require.NoError(t, err, "insert deployment id")

	// Fetch empty proxy values
	defProxy, err := db.GetDefaultProxyConfig(ctx)
	require.NoError(t, err, "get def proxy")

	require.Equal(t, defProxy.DisplayName, "Default")
	require.Equal(t, defProxy.IconUrl, "/emojis/1f3e1.png")

	// Set the proxy values
	args := database.UpsertDefaultProxyParams{
		DisplayName: "displayname",
		IconUrl:     "/icon.png",
	}
	err = db.UpsertDefaultProxy(ctx, args)
	require.NoError(t, err, "insert def proxy")

	defProxy, err = db.GetDefaultProxyConfig(ctx)
	require.NoError(t, err, "get def proxy")
	require.Equal(t, defProxy.DisplayName, args.DisplayName)
	require.Equal(t, defProxy.IconUrl, args.IconUrl)

	// Upsert values
	args = database.UpsertDefaultProxyParams{
		DisplayName: "newdisplayname",
		IconUrl:     "/newicon.png",
	}
	err = db.UpsertDefaultProxy(ctx, args)
	require.NoError(t, err, "upsert def proxy")

	defProxy, err = db.GetDefaultProxyConfig(ctx)
	require.NoError(t, err, "get def proxy")
	require.Equal(t, defProxy.DisplayName, args.DisplayName)
	require.Equal(t, defProxy.IconUrl, args.IconUrl)

	// Ensure other site configs are the same
	found, err := db.GetDeploymentID(ctx)
	require.NoError(t, err, "get deployment id")
	require.Equal(t, depID, found)
}

func TestQueuePosition(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.SkipNow()
	}
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	ctx := testutil.Context(t, testutil.WaitLong)

	org := dbgen.Organization(t, db, database.Organization{})
	jobCount := 10
	jobs := []database.ProvisionerJob{}
	jobIDs := []uuid.UUID{}
	for i := 0; i < jobCount; i++ {
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Tags:           database.StringMap{},
		})
		jobs = append(jobs, job)
		jobIDs = append(jobIDs, job.ID)

		// We need a slight amount of time between each insertion to ensure that
		// the queue position is correct... it's sorted by `created_at`.
		time.Sleep(time.Millisecond)
	}

	queued, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
	require.NoError(t, err)
	require.Len(t, queued, jobCount)
	sort.Slice(queued, func(i, j int) bool {
		return queued[i].QueuePosition < queued[j].QueuePosition
	})
	// Ensure that the queue positions are correct based on insertion ID!
	for index, job := range queued {
		require.Equal(t, job.QueuePosition, int64(index+1))
		require.Equal(t, job.ProvisionerJob.ID, jobs[index].ID)
	}

	job, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		OrganizationID: org.ID,
		StartedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
		Types: database.AllProvisionerTypeValues(),
		WorkerID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		},
		Tags: json.RawMessage("{}"),
	})
	require.NoError(t, err)
	require.Equal(t, jobs[0].ID, job.ID)

	queued, err = db.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
	require.NoError(t, err)
	require.Len(t, queued, jobCount)
	sort.Slice(queued, func(i, j int) bool {
		return queued[i].QueuePosition < queued[j].QueuePosition
	})
	// Ensure that queue positions are updated now that the first job has been acquired!
	for index, job := range queued {
		if index == 0 {
			require.Equal(t, job.QueuePosition, int64(0))
			continue
		}
		require.Equal(t, job.QueuePosition, int64(index))
		require.Equal(t, job.ProvisionerJob.ID, jobs[index].ID)
	}
}

func TestUserLastSeenFilter(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	t.Run("Before", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		err := migrations.Up(sqlDB)
		require.NoError(t, err)
		db := database.New(sqlDB)
		ctx := context.Background()
		now := dbtime.Now()

		yesterday := dbgen.User(t, db, database.User{
			LastSeenAt: now.Add(time.Hour * -25),
		})
		today := dbgen.User(t, db, database.User{
			LastSeenAt: now,
		})
		lastWeek := dbgen.User(t, db, database.User{
			LastSeenAt: now.Add((time.Hour * -24 * 7) + (-1 * time.Hour)),
		})

		beforeToday, err := db.GetUsers(ctx, database.GetUsersParams{
			LastSeenBefore: now.Add(time.Hour * -24),
		})
		require.NoError(t, err)
		database.ConvertUserRows(beforeToday)

		requireUsersMatch(t, []database.User{yesterday, lastWeek}, beforeToday, "before today")

		justYesterday, err := db.GetUsers(ctx, database.GetUsersParams{
			LastSeenBefore: now.Add(time.Hour * -24),
			LastSeenAfter:  now.Add(time.Hour * -24 * 2),
		})
		require.NoError(t, err)
		requireUsersMatch(t, []database.User{yesterday}, justYesterday, "just yesterday")

		all, err := db.GetUsers(ctx, database.GetUsersParams{
			LastSeenBefore: now.Add(time.Hour),
		})
		require.NoError(t, err)
		requireUsersMatch(t, []database.User{today, yesterday, lastWeek}, all, "all")

		allAfterLastWeek, err := db.GetUsers(ctx, database.GetUsersParams{
			LastSeenAfter: now.Add(time.Hour * -24 * 7),
		})
		require.NoError(t, err)
		requireUsersMatch(t, []database.User{today, yesterday}, allAfterLastWeek, "after last week")
	})
}

func TestUserChangeLoginType(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	ctx := context.Background()

	alice := dbgen.User(t, db, database.User{
		LoginType: database.LoginTypePassword,
	})
	bob := dbgen.User(t, db, database.User{
		LoginType: database.LoginTypePassword,
	})
	bobExpPass := bob.HashedPassword
	require.NotEmpty(t, alice.HashedPassword, "hashed password should not start empty")
	require.NotEmpty(t, bob.HashedPassword, "hashed password should not start empty")

	alice, err = db.UpdateUserLoginType(ctx, database.UpdateUserLoginTypeParams{
		NewLoginType: database.LoginTypeOIDC,
		UserID:       alice.ID,
	})
	require.NoError(t, err)

	require.Empty(t, alice.HashedPassword, "hashed password should be empty")

	// First check other users are not affected
	bob, err = db.GetUserByID(ctx, bob.ID)
	require.NoError(t, err)
	require.Equal(t, bobExpPass, bob.HashedPassword, "hashed password should not change")

	// Then check password -> password is a noop
	bob, err = db.UpdateUserLoginType(ctx, database.UpdateUserLoginTypeParams{
		NewLoginType: database.LoginTypePassword,
		UserID:       bob.ID,
	})
	require.NoError(t, err)

	bob, err = db.GetUserByID(ctx, bob.ID)
	require.NoError(t, err)
	require.Equal(t, bobExpPass, bob.HashedPassword, "hashed password should not change")
}

func TestDefaultOrg(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	ctx := context.Background()

	// Should start with the default org
	all, err := db.GetOrganizations(ctx, database.GetOrganizationsParams{})
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.True(t, all[0].IsDefault, "first org should always be default")
}

func TestAuditLogDefaultLimit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	for i := 0; i < 110; i++ {
		dbgen.AuditLog(t, db, database.AuditLog{})
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	rows, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{})
	require.NoError(t, err)
	// The length should match the default limit of the SQL query.
	// Updating the sql query requires changing the number below to match.
	require.Len(t, rows, 100)
}

func TestWorkspaceQuotas(t *testing.T) {
	t.Parallel()
	orgMemberIDs := func(o database.OrganizationMember) uuid.UUID {
		return o.UserID
	}
	groupMemberIDs := func(m database.GroupMember) uuid.UUID {
		return m.UserID
	}

	t.Run("CorruptedEveryone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		db, _ := dbtestutil.NewDB(t)
		// Create an extra org as a distraction
		distract := dbgen.Organization(t, db, database.Organization{})
		_, err := db.InsertAllUsersGroup(ctx, distract.ID)
		require.NoError(t, err)

		_, err = db.UpdateGroupByID(ctx, database.UpdateGroupByIDParams{
			QuotaAllowance: 15,
			ID:             distract.ID,
		})
		require.NoError(t, err)

		// Create an org with 2 users
		org := dbgen.Organization(t, db, database.Organization{})

		everyoneGroup, err := db.InsertAllUsersGroup(ctx, org.ID)
		require.NoError(t, err)

		// Add a quota to the everyone group
		_, err = db.UpdateGroupByID(ctx, database.UpdateGroupByIDParams{
			QuotaAllowance: 50,
			ID:             everyoneGroup.ID,
		})
		require.NoError(t, err)

		// Add people to the org
		one := dbgen.User(t, db, database.User{})
		two := dbgen.User(t, db, database.User{})
		memOne := dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         one.ID,
		})
		memTwo := dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         two.ID,
		})

		// Fetch the 'Everyone' group members
		everyoneMembers, err := db.GetGroupMembersByGroupID(ctx, org.ID)
		require.NoError(t, err)

		require.ElementsMatch(t, db2sdk.List(everyoneMembers, groupMemberIDs),
			db2sdk.List([]database.OrganizationMember{memOne, memTwo}, orgMemberIDs))

		// Check the quota is correct.
		allowance, err := db.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
			UserID:         one.ID,
			OrganizationID: org.ID,
		})
		require.NoError(t, err)
		require.Equal(t, int64(50), allowance)

		// Now try to corrupt the DB
		// Insert rows into the everyone group
		err = db.InsertGroupMember(ctx, database.InsertGroupMemberParams{
			UserID:  memOne.UserID,
			GroupID: org.ID,
		})
		require.NoError(t, err)

		// Ensure allowance remains the same
		allowance, err = db.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
			UserID:         one.ID,
			OrganizationID: org.ID,
		})
		require.NoError(t, err)
		require.Equal(t, int64(50), allowance)
	})
}

// TestReadCustomRoles tests the input params returns the correct set of roles.
func TestReadCustomRoles(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)

	db := database.New(sqlDB)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Make a few site roles, and a few org roles
	orgIDs := make([]uuid.UUID, 3)
	for i := range orgIDs {
		orgIDs[i] = uuid.New()
	}

	allRoles := make([]database.CustomRole, 0)
	siteRoles := make([]database.CustomRole, 0)
	orgRoles := make([]database.CustomRole, 0)
	for i := 0; i < 15; i++ {
		orgID := uuid.NullUUID{
			UUID:  orgIDs[i%len(orgIDs)],
			Valid: true,
		}
		if i%4 == 0 {
			// Some should be site wide
			orgID = uuid.NullUUID{}
		}

		role, err := db.InsertCustomRole(ctx, database.InsertCustomRoleParams{
			Name:           fmt.Sprintf("role-%d", i),
			OrganizationID: orgID,
		})
		require.NoError(t, err)
		allRoles = append(allRoles, role)
		if orgID.Valid {
			orgRoles = append(orgRoles, role)
		} else {
			siteRoles = append(siteRoles, role)
		}
	}

	// normalizedRoleName allows for the simple ElementsMatch to work properly.
	normalizedRoleName := func(role database.CustomRole) string {
		return role.Name + ":" + role.OrganizationID.UUID.String()
	}

	roleToLookup := func(role database.CustomRole) database.NameOrganizationPair {
		return database.NameOrganizationPair{
			Name:           role.Name,
			OrganizationID: role.OrganizationID.UUID,
		}
	}

	testCases := []struct {
		Name   string
		Params database.CustomRolesParams
		Match  func(role database.CustomRole) bool
	}{
		{
			Name: "NilRoles",
			Params: database.CustomRolesParams{
				LookupRoles:     nil,
				ExcludeOrgRoles: false,
				OrganizationID:  uuid.UUID{},
			},
			Match: func(role database.CustomRole) bool {
				return true
			},
		},
		{
			// Empty params should return all roles
			Name: "Empty",
			Params: database.CustomRolesParams{
				LookupRoles:     []database.NameOrganizationPair{},
				ExcludeOrgRoles: false,
				OrganizationID:  uuid.UUID{},
			},
			Match: func(role database.CustomRole) bool {
				return true
			},
		},
		{
			Name: "Organization",
			Params: database.CustomRolesParams{
				LookupRoles:     []database.NameOrganizationPair{},
				ExcludeOrgRoles: false,
				OrganizationID:  orgIDs[1],
			},
			Match: func(role database.CustomRole) bool {
				return role.OrganizationID.UUID == orgIDs[1]
			},
		},
		{
			Name: "SpecificOrgRole",
			Params: database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           orgRoles[0].Name,
						OrganizationID: orgRoles[0].OrganizationID.UUID,
					},
				},
			},
			Match: func(role database.CustomRole) bool {
				return role.Name == orgRoles[0].Name && role.OrganizationID.UUID == orgRoles[0].OrganizationID.UUID
			},
		},
		{
			Name: "SpecificSiteRole",
			Params: database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           siteRoles[0].Name,
						OrganizationID: siteRoles[0].OrganizationID.UUID,
					},
				},
			},
			Match: func(role database.CustomRole) bool {
				return role.Name == siteRoles[0].Name && role.OrganizationID.UUID == siteRoles[0].OrganizationID.UUID
			},
		},
		{
			Name: "FewSpecificRoles",
			Params: database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           orgRoles[0].Name,
						OrganizationID: orgRoles[0].OrganizationID.UUID,
					},
					{
						Name:           orgRoles[1].Name,
						OrganizationID: orgRoles[1].OrganizationID.UUID,
					},
					{
						Name:           siteRoles[0].Name,
						OrganizationID: siteRoles[0].OrganizationID.UUID,
					},
				},
			},
			Match: func(role database.CustomRole) bool {
				return (role.Name == orgRoles[0].Name && role.OrganizationID.UUID == orgRoles[0].OrganizationID.UUID) ||
					(role.Name == orgRoles[1].Name && role.OrganizationID.UUID == orgRoles[1].OrganizationID.UUID) ||
					(role.Name == siteRoles[0].Name && role.OrganizationID.UUID == siteRoles[0].OrganizationID.UUID)
			},
		},
		{
			Name: "AllRolesByLookup",
			Params: database.CustomRolesParams{
				LookupRoles: db2sdk.List(allRoles, roleToLookup),
			},
			Match: func(role database.CustomRole) bool {
				return true
			},
		},
		{
			Name: "NotExists",
			Params: database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           "not-exists",
						OrganizationID: uuid.New(),
					},
					{
						Name:           "not-exists",
						OrganizationID: uuid.Nil,
					},
				},
			},
			Match: func(role database.CustomRole) bool {
				return false
			},
		},
		{
			Name: "Mixed",
			Params: database.CustomRolesParams{
				LookupRoles: []database.NameOrganizationPair{
					{
						Name:           "not-exists",
						OrganizationID: uuid.New(),
					},
					{
						Name:           "not-exists",
						OrganizationID: uuid.Nil,
					},
					{
						Name:           orgRoles[0].Name,
						OrganizationID: orgRoles[0].OrganizationID.UUID,
					},
					{
						Name: siteRoles[0].Name,
					},
				},
			},
			Match: func(role database.CustomRole) bool {
				return (role.Name == orgRoles[0].Name && role.OrganizationID.UUID == orgRoles[0].OrganizationID.UUID) ||
					(role.Name == siteRoles[0].Name && role.OrganizationID.UUID == siteRoles[0].OrganizationID.UUID)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			found, err := db.CustomRoles(ctx, tc.Params)
			require.NoError(t, err)
			filtered := make([]database.CustomRole, 0)
			for _, role := range allRoles {
				if tc.Match(role) {
					filtered = append(filtered, role)
				}
			}

			a := db2sdk.List(filtered, normalizedRoleName)
			b := db2sdk.List(found, normalizedRoleName)
			require.Equal(t, a, b)
		})
	}
}

func TestAuthorizedAuditLogs(t *testing.T) {
	t.Parallel()

	var allLogs []database.AuditLog
	db, _ := dbtestutil.NewDB(t)
	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	db = dbauthz.New(db, authz, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())

	siteWideIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range siteWideIDs {
		allLogs = append(allLogs, dbgen.AuditLog(t, db, database.AuditLog{
			ID:             id,
			OrganizationID: uuid.Nil,
		}))
	}

	// This map is a simple way to insert a given number of organizations
	// and audit logs for each organization.
	// map[orgID][]AuditLogID
	orgAuditLogs := map[uuid.UUID][]uuid.UUID{
		uuid.New(): {uuid.New(), uuid.New()},
		uuid.New(): {uuid.New(), uuid.New()},
	}
	orgIDs := make([]uuid.UUID, 0, len(orgAuditLogs))
	for orgID := range orgAuditLogs {
		orgIDs = append(orgIDs, orgID)
	}
	for orgID, ids := range orgAuditLogs {
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		for _, id := range ids {
			allLogs = append(allLogs, dbgen.AuditLog(t, db, database.AuditLog{
				ID:             id,
				OrganizationID: orgID,
			}))
		}
	}

	// Now fetch all the logs
	ctx := testutil.Context(t, testutil.WaitLong)
	auditorRole, err := rbac.RoleByName(rbac.RoleAuditor())
	require.NoError(t, err)

	memberRole, err := rbac.RoleByName(rbac.RoleMember())
	require.NoError(t, err)

	orgAuditorRoles := func(t *testing.T, orgID uuid.UUID) rbac.Role {
		t.Helper()

		role, err := rbac.RoleByName(rbac.ScopedRoleOrgAuditor(orgID))
		require.NoError(t, err)
		return role
	}

	t.Run("NoAccess", func(t *testing.T) {
		t.Parallel()

		// Given: A user who is a member of 0 organizations
		memberCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "member",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{memberRole},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for audit logs
		logs, err := db.GetAuditLogsOffset(memberCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)
		// Then: No logs returned
		require.Len(t, logs, 0, "no logs should be returned")
	})

	t.Run("SiteWideAuditor", func(t *testing.T) {
		t.Parallel()

		// Given: A site wide auditor
		siteAuditorCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "owner",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{auditorRole},
			Scope:        rbac.ScopeAll,
		})

		// When: the auditor queries for audit logs
		logs, err := db.GetAuditLogsOffset(siteAuditorCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)
		// Then: All logs are returned
		require.ElementsMatch(t, auditOnlyIDs(allLogs), auditOnlyIDs(logs))
	})

	t.Run("SingleOrgAuditor", func(t *testing.T) {
		t.Parallel()

		orgID := orgIDs[0]
		// Given: An organization scoped auditor
		orgAuditCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, orgID)},
			Scope:        rbac.ScopeAll,
		})

		// When: The auditor queries for audit logs
		logs, err := db.GetAuditLogsOffset(orgAuditCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)
		// Then: Only the logs for the organization are returned
		require.ElementsMatch(t, orgAuditLogs[orgID], auditOnlyIDs(logs))
	})

	t.Run("TwoOrgAuditors", func(t *testing.T) {
		t.Parallel()

		first := orgIDs[0]
		second := orgIDs[1]
		// Given: A user who is an auditor for two organizations
		multiOrgAuditCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, first), orgAuditorRoles(t, second)},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for audit logs
		logs, err := db.GetAuditLogsOffset(multiOrgAuditCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)
		// Then: All logs for both organizations are returned
		require.ElementsMatch(t, append(orgAuditLogs[first], orgAuditLogs[second]...), auditOnlyIDs(logs))
	})

	t.Run("ErroneousOrg", func(t *testing.T) {
		t.Parallel()

		// Given: A user who is an auditor for an organization that has 0 logs
		userCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, uuid.New())},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for audit logs
		logs, err := db.GetAuditLogsOffset(userCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)
		// Then: No logs are returned
		require.Len(t, logs, 0, "no logs should be returned")
	})
}

func auditOnlyIDs[T database.AuditLog | database.GetAuditLogsOffsetRow](logs []T) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(logs))
	for _, log := range logs {
		switch log := any(log).(type) {
		case database.AuditLog:
			ids = append(ids, log.ID)
		case database.GetAuditLogsOffsetRow:
			ids = append(ids, log.AuditLog.ID)
		default:
			panic("unreachable")
		}
	}
	return ids
}

type tvArgs struct {
	Status database.ProvisionerJobStatus
	// CreateWorkspace is true if we should create a workspace for the template version
	CreateWorkspace     bool
	WorkspaceTransition database.WorkspaceTransition
}

// createTemplateVersion is a helper function to create a version with its dependencies.
func createTemplateVersion(t testing.TB, db database.Store, tpl database.Template, args tvArgs) database.TemplateVersion {
	t.Helper()
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID: uuid.NullUUID{
			UUID:  tpl.ID,
			Valid: true,
		},
		OrganizationID: tpl.OrganizationID,
		CreatedAt:      dbtime.Now(),
		UpdatedAt:      dbtime.Now(),
		CreatedBy:      tpl.CreatedBy,
	})

	earlier := sql.NullTime{
		Time:  dbtime.Now().Add(time.Second * -30),
		Valid: true,
	}
	now := sql.NullTime{
		Time:  dbtime.Now(),
		Valid: true,
	}
	j := database.ProvisionerJob{
		ID:             version.JobID,
		CreatedAt:      earlier.Time,
		UpdatedAt:      earlier.Time,
		Error:          sql.NullString{},
		OrganizationID: tpl.OrganizationID,
		InitiatorID:    tpl.CreatedBy,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	}

	switch args.Status {
	case database.ProvisionerJobStatusRunning:
		j.StartedAt = earlier
	case database.ProvisionerJobStatusPending:
	case database.ProvisionerJobStatusFailed:
		j.StartedAt = earlier
		j.CompletedAt = now
		j.Error = sql.NullString{
			String: "failed",
			Valid:  true,
		}
		j.ErrorCode = sql.NullString{
			String: "failed",
			Valid:  true,
		}
	case database.ProvisionerJobStatusSucceeded:
		j.StartedAt = earlier
		j.CompletedAt = now
	default:
		t.Fatalf("invalid status: %s", args.Status)
	}

	dbgen.ProvisionerJob(t, db, nil, j)
	if args.CreateWorkspace {
		wrk := dbgen.Workspace(t, db, database.Workspace{
			CreatedAt:      time.Time{},
			UpdatedAt:      time.Time{},
			OwnerID:        tpl.CreatedBy,
			OrganizationID: tpl.OrganizationID,
			TemplateID:     tpl.ID,
		})
		trans := database.WorkspaceTransitionStart
		if args.WorkspaceTransition != "" {
			trans = args.WorkspaceTransition
		}
		buildJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			CompletedAt:    now,
			InitiatorID:    tpl.CreatedBy,
			OrganizationID: tpl.OrganizationID,
		})
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wrk.ID,
			TemplateVersionID: version.ID,
			BuildNumber:       1,
			Transition:        trans,
			InitiatorID:       tpl.CreatedBy,
			JobID:             buildJob.ID,
		})
	}
	return version
}

func TestArchiveVersions(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	t.Run("ArchiveFailedVersions", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		err := migrations.Up(sqlDB)
		require.NoError(t, err)
		db := database.New(sqlDB)
		ctx := context.Background()

		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		// Create some versions
		failed := createTemplateVersion(t, db, tpl, tvArgs{
			Status:          database.ProvisionerJobStatusFailed,
			CreateWorkspace: false,
		})
		unused := createTemplateVersion(t, db, tpl, tvArgs{
			Status:          database.ProvisionerJobStatusSucceeded,
			CreateWorkspace: false,
		})
		createTemplateVersion(t, db, tpl, tvArgs{
			Status:          database.ProvisionerJobStatusSucceeded,
			CreateWorkspace: true,
		})
		deleted := createTemplateVersion(t, db, tpl, tvArgs{
			Status:              database.ProvisionerJobStatusSucceeded,
			CreateWorkspace:     true,
			WorkspaceTransition: database.WorkspaceTransitionDelete,
		})

		// Now archive failed versions
		archived, err := db.ArchiveUnusedTemplateVersions(ctx, database.ArchiveUnusedTemplateVersionsParams{
			UpdatedAt:  dbtime.Now(),
			TemplateID: tpl.ID,
			// All versions
			TemplateVersionID: uuid.Nil,
			JobStatus: database.NullProvisionerJobStatus{
				ProvisionerJobStatus: database.ProvisionerJobStatusFailed,
				Valid:                true,
			},
		})
		require.NoError(t, err, "archive failed versions")
		require.Len(t, archived, 1, "should only archive one version")
		require.Equal(t, failed.ID, archived[0], "should archive failed version")

		// Archive all unused versions
		archived, err = db.ArchiveUnusedTemplateVersions(ctx, database.ArchiveUnusedTemplateVersionsParams{
			UpdatedAt:  dbtime.Now(),
			TemplateID: tpl.ID,
			// All versions
			TemplateVersionID: uuid.Nil,
		})
		require.NoError(t, err, "archive failed versions")
		require.Len(t, archived, 2)
		require.ElementsMatch(t, []uuid.UUID{deleted.ID, unused.ID}, archived, "should archive unused versions")
	})
}

func TestExpectOne(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	t.Run("ErrNoRows", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		err := migrations.Up(sqlDB)
		require.NoError(t, err)
		db := database.New(sqlDB)
		ctx := context.Background()

		_, err = database.ExpectOne(db.GetUsers(ctx, database.GetUsersParams{}))
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("TooMany", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		err := migrations.Up(sqlDB)
		require.NoError(t, err)
		db := database.New(sqlDB)
		ctx := context.Background()

		// Create 2 organizations so the query returns >1
		dbgen.Organization(t, db, database.Organization{})
		dbgen.Organization(t, db, database.Organization{})

		// Organizations is an easy table without foreign key dependencies
		_, err = database.ExpectOne(db.GetOrganizations(ctx, database.GetOrganizationsParams{}))
		require.ErrorContains(t, err, "too many rows returned")
	})
}

func TestGroupRemovalTrigger(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	orgA := dbgen.Organization(t, db, database.Organization{})
	_, err := db.InsertAllUsersGroup(context.Background(), orgA.ID)
	require.NoError(t, err)

	orgB := dbgen.Organization(t, db, database.Organization{})
	_, err = db.InsertAllUsersGroup(context.Background(), orgB.ID)
	require.NoError(t, err)

	orgs := []database.Organization{orgA, orgB}

	user := dbgen.User(t, db, database.User{})
	extra := dbgen.User(t, db, database.User{})
	users := []database.User{user, extra}

	groupA1 := dbgen.Group(t, db, database.Group{
		OrganizationID: orgA.ID,
	})
	groupA2 := dbgen.Group(t, db, database.Group{
		OrganizationID: orgA.ID,
	})

	groupB1 := dbgen.Group(t, db, database.Group{
		OrganizationID: orgB.ID,
	})
	groupB2 := dbgen.Group(t, db, database.Group{
		OrganizationID: orgB.ID,
	})

	groups := []database.Group{groupA1, groupA2, groupB1, groupB2}

	// Add users to all organizations
	for _, u := range users {
		for _, o := range orgs {
			dbgen.OrganizationMember(t, db, database.OrganizationMember{
				OrganizationID: o.ID,
				UserID:         u.ID,
			})
		}
	}

	// Add users to all groups
	for _, u := range users {
		for _, g := range groups {
			dbgen.GroupMember(t, db, database.GroupMemberTable{
				GroupID: g.ID,
				UserID:  u.ID,
			})
		}
	}

	// Verify user is in all groups
	ctx := testutil.Context(t, testutil.WaitLong)
	onlyGroupIDs := func(row database.GetGroupsRow) uuid.UUID {
		return row.Group.ID
	}
	userGroups, err := db.GetGroups(ctx, database.GetGroupsParams{
		HasMemberID: user.ID,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{
		orgA.ID, orgB.ID, // Everyone groups
		groupA1.ID, groupA2.ID, groupB1.ID, groupB2.ID, // Org groups
	}, db2sdk.List(userGroups, onlyGroupIDs))

	// Remove the user from org A
	err = db.DeleteOrganizationMember(ctx, database.DeleteOrganizationMemberParams{
		OrganizationID: orgA.ID,
		UserID:         user.ID,
	})
	require.NoError(t, err)

	// Verify user is no longer in org A groups
	userGroups, err = db.GetGroups(ctx, database.GetGroupsParams{
		HasMemberID: user.ID,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{
		orgB.ID,                // Everyone group
		groupB1.ID, groupB2.ID, // Org groups
	}, db2sdk.List(userGroups, onlyGroupIDs))

	// Verify extra user is unchanged
	extraUserGroups, err := db.GetGroups(ctx, database.GetGroupsParams{
		HasMemberID: extra.ID,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{
		orgA.ID, orgB.ID, // Everyone groups
		groupA1.ID, groupA2.ID, groupB1.ID, groupB2.ID, // Org groups
	}, db2sdk.List(extraUserGroups, onlyGroupIDs))
}

func requireUsersMatch(t testing.TB, expected []database.User, found []database.GetUsersRow, msg string) {
	t.Helper()
	require.ElementsMatch(t, expected, database.ConvertUserRows(found), msg)
}
