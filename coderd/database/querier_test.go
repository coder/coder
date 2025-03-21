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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/provisionersdk"
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

func TestGetEligibleProvisionerDaemonsByProvisionerJobIDs(t *testing.T) {
	t.Parallel()

	t.Run("NoJobsReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		daemons, err := db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(context.Background(), []uuid.UUID{})
		require.NoError(t, err)
		require.Empty(t, daemons)
	})

	t.Run("MatchesProvisionerType", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Provisioner:    database.ProvisionerTypeEcho,
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		matchingDaemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "matching-daemon",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "non-matching-daemon",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		daemons, err := db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(context.Background(), []uuid.UUID{job.ID})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		require.Equal(t, matchingDaemon.ID, daemons[0].ProvisionerDaemon.ID)
	})

	t.Run("MatchesOrganizationScope", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Provisioner:    database.ProvisionerTypeEcho,
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		})

		orgDaemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "org-daemon",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "user-daemon",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeUser,
			},
		})

		daemons, err := db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(context.Background(), []uuid.UUID{job.ID})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		require.Equal(t, orgDaemon.ID, daemons[0].ProvisionerDaemon.ID)
	})

	t.Run("MatchesMultipleProvisioners", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Provisioner:    database.ProvisionerTypeEcho,
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		daemon1 := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "daemon-1",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		daemon2 := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "daemon-2",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "daemon-3",
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})

		daemons, err := db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(context.Background(), []uuid.UUID{job.ID})
		require.NoError(t, err)
		require.Len(t, daemons, 2)

		daemonIDs := []uuid.UUID{daemons[0].ProvisionerDaemon.ID, daemons[1].ProvisionerDaemon.ID}
		require.ElementsMatch(t, []uuid.UUID{daemon1.ID, daemon2.ID}, daemonIDs)
	})
}

func TestGetProvisionerDaemonsWithStatusByOrganization(t *testing.T) {
	t.Parallel()

	t.Run("NoDaemonsInOrgReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "non-matching-daemon",
			OrganizationID: otherOrg.ID,
		})
		daemons, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID: org.ID,
		})
		require.NoError(t, err)
		require.Empty(t, daemons)
	})

	t.Run("MatchesProvisionerIDs", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		matchingDaemon0 := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "matching-daemon0",
			OrganizationID: org.ID,
		})
		matchingDaemon1 := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "matching-daemon1",
			OrganizationID: org.ID,
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "non-matching-daemon",
			OrganizationID: org.ID,
		})

		daemons, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID: org.ID,
			IDs:            []uuid.UUID{matchingDaemon0.ID, matchingDaemon1.ID},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 2)
		if daemons[0].ProvisionerDaemon.ID != matchingDaemon0.ID {
			daemons[0], daemons[1] = daemons[1], daemons[0]
		}
		require.Equal(t, matchingDaemon0.ID, daemons[0].ProvisionerDaemon.ID)
		require.Equal(t, matchingDaemon1.ID, daemons[1].ProvisionerDaemon.ID)
	})

	t.Run("MatchesTags", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		fooDaemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "foo-daemon",
			OrganizationID: org.ID,
			Tags: database.StringMap{
				"foo": "bar",
			},
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "baz-daemon",
			OrganizationID: org.ID,
			Tags: database.StringMap{
				"baz": "qux",
			},
		})

		daemons, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID: org.ID,
			Tags:           database.StringMap{"foo": "bar"},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 1)
		require.Equal(t, fooDaemon.ID, daemons[0].ProvisionerDaemon.ID)
	})

	t.Run("UsesStaleInterval", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		daemon1 := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "stale-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-time.Hour),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-time.Hour),
			},
		})
		daemon2 := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "idle-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-(30 * time.Minute)),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-(30 * time.Minute)),
			},
		})

		daemons, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID:  org.ID,
			StaleIntervalMS: 45 * time.Minute.Milliseconds(),
		})
		require.NoError(t, err)
		require.Len(t, daemons, 2)

		if daemons[0].ProvisionerDaemon.ID != daemon1.ID {
			daemons[0], daemons[1] = daemons[1], daemons[0]
		}
		require.Equal(t, daemon1.ID, daemons[0].ProvisionerDaemon.ID)
		require.Equal(t, daemon2.ID, daemons[1].ProvisionerDaemon.ID)
		require.Equal(t, database.ProvisionerDaemonStatusOffline, daemons[0].Status)
		require.Equal(t, database.ProvisionerDaemonStatusIdle, daemons[1].Status)
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
		workspace1 := dbgen.Workspace(t, db, database.WorkspaceTable{
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
		workspace2 := dbgen.Workspace(t, db, database.WorkspaceTable{
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
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
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

func TestGetAuthorizedWorkspacesAndAgentsByOwnerID(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{
		RBACRoles: []string{rbac.RoleOwner().String()},
	})
	user := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      owner.ID,
	})

	pendingID := uuid.New()
	createTemplateVersion(t, db, tpl, tvArgs{
		Status:          database.ProvisionerJobStatusPending,
		CreateWorkspace: true,
		WorkspaceID:     pendingID,
		CreateAgent:     true,
	})
	failedID := uuid.New()
	createTemplateVersion(t, db, tpl, tvArgs{
		Status:          database.ProvisionerJobStatusFailed,
		CreateWorkspace: true,
		CreateAgent:     true,
		WorkspaceID:     failedID,
	})
	succeededID := uuid.New()
	createTemplateVersion(t, db, tpl, tvArgs{
		Status:              database.ProvisionerJobStatusSucceeded,
		WorkspaceTransition: database.WorkspaceTransitionStart,
		CreateWorkspace:     true,
		WorkspaceID:         succeededID,
		CreateAgent:         true,
		ExtraAgents:         1,
		ExtraBuilds:         2,
	})
	deletedID := uuid.New()
	createTemplateVersion(t, db, tpl, tvArgs{
		Status:              database.ProvisionerJobStatusSucceeded,
		WorkspaceTransition: database.WorkspaceTransitionDelete,
		CreateWorkspace:     true,
		WorkspaceID:         deletedID,
		CreateAgent:         false,
	})

	ownerCheckFn := func(ownerRows []database.GetWorkspacesAndAgentsByOwnerIDRow) {
		require.Len(t, ownerRows, 4)
		for _, row := range ownerRows {
			switch row.ID {
			case pendingID:
				require.Len(t, row.Agents, 1)
				require.Equal(t, database.ProvisionerJobStatusPending, row.JobStatus)
			case failedID:
				require.Len(t, row.Agents, 1)
				require.Equal(t, database.ProvisionerJobStatusFailed, row.JobStatus)
			case succeededID:
				require.Len(t, row.Agents, 2)
				require.Equal(t, database.ProvisionerJobStatusSucceeded, row.JobStatus)
				require.Equal(t, database.WorkspaceTransitionStart, row.Transition)
			case deletedID:
				require.Len(t, row.Agents, 0)
				require.Equal(t, database.ProvisionerJobStatusSucceeded, row.JobStatus)
				require.Equal(t, database.WorkspaceTransitionDelete, row.Transition)
			default:
				t.Fatalf("unexpected workspace ID: %s", row.ID)
			}
		}
	}
	t.Run("sqlQuerier", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		userSubject, _, err := httpmw.UserRBACSubject(ctx, db, user.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedUser, err := authorizer.Prepare(ctx, userSubject, policy.ActionRead, rbac.ResourceWorkspace.Type)
		require.NoError(t, err)
		userCtx := dbauthz.As(ctx, userSubject)
		userRows, err := db.GetAuthorizedWorkspacesAndAgentsByOwnerID(userCtx, owner.ID, preparedUser)
		require.NoError(t, err)
		require.Len(t, userRows, 0)

		ownerSubject, _, err := httpmw.UserRBACSubject(ctx, db, owner.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedOwner, err := authorizer.Prepare(ctx, ownerSubject, policy.ActionRead, rbac.ResourceWorkspace.Type)
		require.NoError(t, err)
		ownerCtx := dbauthz.As(ctx, ownerSubject)
		ownerRows, err := db.GetAuthorizedWorkspacesAndAgentsByOwnerID(ownerCtx, owner.ID, preparedOwner)
		require.NoError(t, err)
		ownerCheckFn(ownerRows)
	})

	t.Run("dbauthz", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		authzdb := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())

		userSubject, _, err := httpmw.UserRBACSubject(ctx, authzdb, user.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		userCtx := dbauthz.As(ctx, userSubject)

		ownerSubject, _, err := httpmw.UserRBACSubject(ctx, authzdb, owner.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		ownerCtx := dbauthz.As(ctx, ownerSubject)

		userRows, err := authzdb.GetWorkspacesAndAgentsByOwnerID(userCtx, owner.ID)
		require.NoError(t, err)
		require.Len(t, userRows, 0)

		ownerRows, err := authzdb.GetWorkspacesAndAgentsByOwnerID(ownerCtx, owner.ID)
		require.NoError(t, err)
		ownerCheckFn(ownerRows)
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

	// Create default provisioner daemon:
	dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
		Name:         "default_provisioner",
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
		// Ensure the `tags` field is NOT NULL for the default provisioner;
		// otherwise, it won't be able to pick up any jobs.
		Tags: database.StringMap{},
	})

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
		ProvisionerTags: json.RawMessage("{}"),
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
	WorkspaceID         uuid.UUID
	CreateAgent         bool
	WorkspaceTransition database.WorkspaceTransition
	ExtraAgents         int
	ExtraBuilds         int
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

	latestJob := database.ProvisionerJob{
		ID:             version.JobID,
		Error:          sql.NullString{},
		OrganizationID: tpl.OrganizationID,
		InitiatorID:    tpl.CreatedBy,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	}
	setJobStatus(t, args.Status, &latestJob)
	dbgen.ProvisionerJob(t, db, nil, latestJob)
	if args.CreateWorkspace {
		wrk := dbgen.Workspace(t, db, database.WorkspaceTable{
			ID:             args.WorkspaceID,
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
		latestJob = database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    tpl.CreatedBy,
			OrganizationID: tpl.OrganizationID,
		}
		setJobStatus(t, args.Status, &latestJob)
		latestJob = dbgen.ProvisionerJob(t, db, nil, latestJob)
		latestResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: latestJob.ID,
		})
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wrk.ID,
			TemplateVersionID: version.ID,
			BuildNumber:       1,
			Transition:        trans,
			InitiatorID:       tpl.CreatedBy,
			JobID:             latestJob.ID,
		})
		for i := 0; i < args.ExtraBuilds; i++ {
			latestJob = database.ProvisionerJob{
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    tpl.CreatedBy,
				OrganizationID: tpl.OrganizationID,
			}
			setJobStatus(t, args.Status, &latestJob)
			latestJob = dbgen.ProvisionerJob(t, db, nil, latestJob)
			latestResource = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
				JobID: latestJob.ID,
			})
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID:       wrk.ID,
				TemplateVersionID: version.ID,
				BuildNumber:       int32(i) + 2,
				Transition:        trans,
				InitiatorID:       tpl.CreatedBy,
				JobID:             latestJob.ID,
			})
		}

		if args.CreateAgent {
			dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID: latestResource.ID,
			})
		}
		for i := 0; i < args.ExtraAgents; i++ {
			dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID: latestResource.ID,
			})
		}
	}
	return version
}

func setJobStatus(t testing.TB, status database.ProvisionerJobStatus, j *database.ProvisionerJob) {
	t.Helper()

	earlier := sql.NullTime{
		Time:  dbtime.Now().Add(time.Second * -30),
		Valid: true,
	}
	now := sql.NullTime{
		Time:  dbtime.Now(),
		Valid: true,
	}
	switch status {
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
		t.Fatalf("invalid status: %s", status)
	}
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

func TestGetProvisionerJobsByIDsWithQueuePosition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		jobTags        []database.StringMap
		daemonTags     []database.StringMap
		queueSizes     []int64
		queuePositions []int64
		// GetProvisionerJobsByIDsWithQueuePosition takes jobIDs as a parameter.
		// If skipJobIDs is empty, all jobs are passed to the function; otherwise, the specified jobs are skipped.
		// NOTE: Skipping job IDs means they will be excluded from the result,
		// but this should not affect the queue position or queue size of other jobs.
		skipJobIDs map[int]struct{}
	}{
		// Baseline test case
		{
			name: "test-case-1",
			jobTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
			},
			queueSizes:     []int64{2, 2, 0},
			queuePositions: []int64{1, 1, 0},
		},
		// Includes an additional provisioner
		{
			name: "test-case-2",
			jobTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{3, 3, 3},
			queuePositions: []int64{1, 1, 3},
		},
		// Skips job at index 0
		{
			name: "test-case-3",
			jobTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{3, 3},
			queuePositions: []int64{1, 3},
			skipJobIDs: map[int]struct{}{
				0: {},
			},
		},
		// Skips job at index 1
		{
			name: "test-case-4",
			jobTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{3, 3},
			queuePositions: []int64{1, 3},
			skipJobIDs: map[int]struct{}{
				1: {},
			},
		},
		// Skips job at index 2
		{
			name: "test-case-5",
			jobTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{3, 3},
			queuePositions: []int64{1, 1},
			skipJobIDs: map[int]struct{}{
				2: {},
			},
		},
		// Skips jobs at indexes 0 and 2
		{
			name: "test-case-6",
			jobTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{3},
			queuePositions: []int64{1},
			skipJobIDs: map[int]struct{}{
				0: {},
				2: {},
			},
		},
		// Includes two additional jobs that any provisioner can execute.
		{
			name: "test-case-7",
			jobTags: []database.StringMap{
				{},
				{},
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{5, 5, 5, 5, 5},
			queuePositions: []int64{1, 2, 3, 3, 5},
		},
		// Includes two additional jobs that any provisioner can execute, but they are intentionally skipped.
		{
			name: "test-case-8",
			jobTags: []database.StringMap{
				{},
				{},
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "c": "3"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1"},
				{"a": "1", "b": "2", "c": "3"},
			},
			queueSizes:     []int64{5, 5, 5},
			queuePositions: []int64{3, 3, 5},
			skipJobIDs: map[int]struct{}{
				0: {},
				1: {},
			},
		},
		// N jobs (1 job with 0 tags) & 0 provisioners exist
		{
			name: "test-case-9",
			jobTags: []database.StringMap{
				{},
				{"a": "1"},
				{"b": "2"},
			},
			daemonTags:     []database.StringMap{},
			queueSizes:     []int64{0, 0, 0},
			queuePositions: []int64{0, 0, 0},
		},
		// N jobs (1 job with 0 tags) & N provisioners
		{
			name: "test-case-10",
			jobTags: []database.StringMap{
				{},
				{"a": "1"},
				{"b": "2"},
			},
			daemonTags: []database.StringMap{
				{},
				{"a": "1"},
				{"b": "2"},
			},
			queueSizes:     []int64{2, 2, 2},
			queuePositions: []int64{1, 2, 2},
		},
		// (N + 1) jobs (1 job with 0 tags) & N provisioners
		// 1 job not matching any provisioner (first in the list)
		{
			name: "test-case-11",
			jobTags: []database.StringMap{
				{"c": "3"},
				{},
				{"a": "1"},
				{"b": "2"},
			},
			daemonTags: []database.StringMap{
				{},
				{"a": "1"},
				{"b": "2"},
			},
			queueSizes:     []int64{0, 2, 2, 2},
			queuePositions: []int64{0, 1, 2, 2},
		},
		// 0 jobs & 0 provisioners
		{
			name:           "test-case-12",
			jobTags:        []database.StringMap{},
			daemonTags:     []database.StringMap{},
			queueSizes:     nil, // TODO(yevhenii): should it be empty array instead?
			queuePositions: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture loop variable to avoid data races
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			now := dbtime.Now()
			ctx := testutil.Context(t, testutil.WaitShort)

			// Create provisioner jobs based on provided tags:
			allJobs := make([]database.ProvisionerJob, len(tc.jobTags))
			for idx, tags := range tc.jobTags {
				// Make sure jobs are stored in correct order, first job should have the earliest createdAt timestamp.
				// Example for 3 jobs:
				// job_1 createdAt: now - 3 minutes
				// job_2 createdAt: now - 2 minutes
				// job_3 createdAt: now - 1 minute
				timeOffsetInMinutes := len(tc.jobTags) - idx
				timeOffset := time.Duration(timeOffsetInMinutes) * time.Minute
				createdAt := now.Add(-timeOffset)

				allJobs[idx] = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					CreatedAt: createdAt,
					Tags:      tags,
				})
			}

			// Create provisioner daemons based on provided tags:
			for idx, tags := range tc.daemonTags {
				dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
					Name:         fmt.Sprintf("prov_%v", idx),
					Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
					Tags:         tags,
				})
			}

			// Assert invariant: the jobs are in pending status
			for idx, job := range allJobs {
				require.Equal(t, database.ProvisionerJobStatusPending, job.JobStatus, "expected job %d to have status %s", idx, database.ProvisionerJobStatusPending)
			}

			filteredJobs := make([]database.ProvisionerJob, 0)
			filteredJobIDs := make([]uuid.UUID, 0)
			for idx, job := range allJobs {
				if _, skip := tc.skipJobIDs[idx]; skip {
					continue
				}

				filteredJobs = append(filteredJobs, job)
				filteredJobIDs = append(filteredJobIDs, job.ID)
			}

			// When: we fetch the jobs by their IDs
			actualJobs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, filteredJobIDs)
			require.NoError(t, err)
			require.Len(t, actualJobs, len(filteredJobs), "should return all unskipped jobs")

			// Then: the jobs should be returned in the correct order (sorted by createdAt)
			sort.Slice(filteredJobs, func(i, j int) bool {
				return filteredJobs[i].CreatedAt.Before(filteredJobs[j].CreatedAt)
			})
			for idx, job := range actualJobs {
				assert.EqualValues(t, filteredJobs[idx], job.ProvisionerJob)
			}

			// Then: the queue size should be set correctly
			var queueSizes []int64
			for _, job := range actualJobs {
				queueSizes = append(queueSizes, job.QueueSize)
			}
			assert.EqualValues(t, tc.queueSizes, queueSizes, "expected queue positions to be set correctly")

			// Then: the queue position should be set correctly:
			var queuePositions []int64
			for _, job := range actualJobs {
				queuePositions = append(queuePositions, job.QueuePosition)
			}
			assert.EqualValues(t, tc.queuePositions, queuePositions, "expected queue positions to be set correctly")
		})
	}
}

func TestGetProvisionerJobsByIDsWithQueuePosition_MixedStatuses(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.SkipNow()
	}

	db, _ := dbtestutil.NewDB(t)
	now := dbtime.Now()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Create the following provisioner jobs:
	allJobs := []database.ProvisionerJob{
		// Pending. This will be the last in the queue because
		// it was created most recently.
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-time.Minute),
			StartedAt:   sql.NullTime{},
			CanceledAt:  sql.NullTime{},
			CompletedAt: sql.NullTime{},
			Error:       sql.NullString{},
			// Ensure the `tags` field is NOT NULL for both provisioner jobs and provisioner daemons;
			// otherwise, provisioner daemons won't be able to pick up any jobs.
			Tags: database.StringMap{},
		}),

		// Another pending. This will come first in the queue
		// because it was created before the previous job.
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-2 * time.Minute),
			StartedAt:   sql.NullTime{},
			CanceledAt:  sql.NullTime{},
			CompletedAt: sql.NullTime{},
			Error:       sql.NullString{},
			Tags:        database.StringMap{},
		}),

		// Running
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-3 * time.Minute),
			StartedAt:   sql.NullTime{Valid: true, Time: now},
			CanceledAt:  sql.NullTime{},
			CompletedAt: sql.NullTime{},
			Error:       sql.NullString{},
			Tags:        database.StringMap{},
		}),

		// Succeeded
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-4 * time.Minute),
			StartedAt:   sql.NullTime{Valid: true, Time: now},
			CanceledAt:  sql.NullTime{},
			CompletedAt: sql.NullTime{Valid: true, Time: now},
			Error:       sql.NullString{},
			Tags:        database.StringMap{},
		}),

		// Canceling
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-5 * time.Minute),
			StartedAt:   sql.NullTime{},
			CanceledAt:  sql.NullTime{Valid: true, Time: now},
			CompletedAt: sql.NullTime{},
			Error:       sql.NullString{},
			Tags:        database.StringMap{},
		}),

		// Canceled
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-6 * time.Minute),
			StartedAt:   sql.NullTime{},
			CanceledAt:  sql.NullTime{Valid: true, Time: now},
			CompletedAt: sql.NullTime{Valid: true, Time: now},
			Error:       sql.NullString{},
			Tags:        database.StringMap{},
		}),

		// Failed
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt:   now.Add(-7 * time.Minute),
			StartedAt:   sql.NullTime{},
			CanceledAt:  sql.NullTime{},
			CompletedAt: sql.NullTime{},
			Error:       sql.NullString{String: "failed", Valid: true},
			Tags:        database.StringMap{},
		}),
	}

	// Create default provisioner daemon:
	dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
		Name:         "default_provisioner",
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
		Tags:         database.StringMap{},
	})

	// Assert invariant: the jobs are in the expected order
	require.Len(t, allJobs, 7, "expected 7 jobs")
	for idx, status := range []database.ProvisionerJobStatus{
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusRunning,
		database.ProvisionerJobStatusSucceeded,
		database.ProvisionerJobStatusCanceling,
		database.ProvisionerJobStatusCanceled,
		database.ProvisionerJobStatusFailed,
	} {
		require.Equal(t, status, allJobs[idx].JobStatus, "expected job %d to have status %s", idx, status)
	}

	var jobIDs []uuid.UUID
	for _, job := range allJobs {
		jobIDs = append(jobIDs, job.ID)
	}

	// When: we fetch the jobs by their IDs
	actualJobs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
	require.NoError(t, err)
	require.Len(t, actualJobs, len(allJobs), "should return all jobs")

	// Then: the jobs should be returned in the correct order (sorted by createdAt)
	sort.Slice(allJobs, func(i, j int) bool {
		return allJobs[i].CreatedAt.Before(allJobs[j].CreatedAt)
	})
	for idx, job := range actualJobs {
		assert.EqualValues(t, allJobs[idx], job.ProvisionerJob)
	}

	// Then: the queue size should be set correctly
	var queueSizes []int64
	for _, job := range actualJobs {
		queueSizes = append(queueSizes, job.QueueSize)
	}
	assert.EqualValues(t, []int64{0, 0, 0, 0, 0, 2, 2}, queueSizes, "expected queue positions to be set correctly")

	// Then: the queue position should be set correctly:
	var queuePositions []int64
	for _, job := range actualJobs {
		queuePositions = append(queuePositions, job.QueuePosition)
	}
	assert.EqualValues(t, []int64{0, 0, 0, 0, 0, 1, 2}, queuePositions, "expected queue positions to be set correctly")
}

func TestGetProvisionerJobsByIDsWithQueuePosition_OrderValidation(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	now := dbtime.Now()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Create the following provisioner jobs:
	allJobs := []database.ProvisionerJob{
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-4 * time.Minute),
			// Ensure the `tags` field is NOT NULL for both provisioner jobs and provisioner daemons;
			// otherwise, provisioner daemons won't be able to pick up any jobs.
			Tags: database.StringMap{},
		}),

		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-5 * time.Minute),
			Tags:      database.StringMap{},
		}),

		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-6 * time.Minute),
			Tags:      database.StringMap{},
		}),

		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-3 * time.Minute),
			Tags:      database.StringMap{},
		}),

		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-2 * time.Minute),
			Tags:      database.StringMap{},
		}),

		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-1 * time.Minute),
			Tags:      database.StringMap{},
		}),
	}

	// Create default provisioner daemon:
	dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
		Name:         "default_provisioner",
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
		Tags:         database.StringMap{},
	})

	// Assert invariant: the jobs are in the expected order
	require.Len(t, allJobs, 6, "expected 7 jobs")
	for idx, status := range []database.ProvisionerJobStatus{
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusPending,
		database.ProvisionerJobStatusPending,
	} {
		require.Equal(t, status, allJobs[idx].JobStatus, "expected job %d to have status %s", idx, status)
	}

	var jobIDs []uuid.UUID
	for _, job := range allJobs {
		jobIDs = append(jobIDs, job.ID)
	}

	// When: we fetch the jobs by their IDs
	actualJobs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
	require.NoError(t, err)
	require.Len(t, actualJobs, len(allJobs), "should return all jobs")

	// Then: the jobs should be returned in the correct order (sorted by createdAt)
	sort.Slice(allJobs, func(i, j int) bool {
		return allJobs[i].CreatedAt.Before(allJobs[j].CreatedAt)
	})
	for idx, job := range actualJobs {
		assert.EqualValues(t, allJobs[idx], job.ProvisionerJob)
		assert.EqualValues(t, allJobs[idx].CreatedAt, job.ProvisionerJob.CreatedAt)
	}

	// Then: the queue size should be set correctly
	var queueSizes []int64
	for _, job := range actualJobs {
		queueSizes = append(queueSizes, job.QueueSize)
	}
	assert.EqualValues(t, []int64{6, 6, 6, 6, 6, 6}, queueSizes, "expected queue positions to be set correctly")

	// Then: the queue position should be set correctly:
	var queuePositions []int64
	for _, job := range actualJobs {
		queuePositions = append(queuePositions, job.QueuePosition)
	}
	assert.EqualValues(t, []int64{1, 2, 3, 4, 5, 6}, queuePositions, "expected queue positions to be set correctly")
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

func TestGetUserStatusCounts(t *testing.T) {
	t.Parallel()
	t.Skip("https://github.com/coder/internal/issues/464")

	if !dbtestutil.WillUsePostgres() {
		t.SkipNow()
	}

	timezones := []string{
		"Canada/Newfoundland",
		"Africa/Johannesburg",
		"America/New_York",
		"Europe/London",
		"Asia/Tokyo",
		"Australia/Sydney",
	}

	for _, tz := range timezones {
		tz := tz
		t.Run(tz, func(t *testing.T) {
			t.Parallel()

			location, err := time.LoadLocation(tz)
			if err != nil {
				t.Fatalf("failed to load location: %v", err)
			}
			today := dbtime.Now().In(location)
			createdAt := today.Add(-5 * 24 * time.Hour)
			firstTransitionTime := createdAt.Add(2 * 24 * time.Hour)
			secondTransitionTime := firstTransitionTime.Add(2 * 24 * time.Hour)

			t.Run("No Users", func(t *testing.T) {
				t.Parallel()
				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				counts, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					StartTime: createdAt,
					EndTime:   today,
				})
				require.NoError(t, err)
				require.Empty(t, counts, "should return no results when there are no users")
			})

			t.Run("One User/Creation Only", func(t *testing.T) {
				t.Parallel()

				testCases := []struct {
					name   string
					status database.UserStatus
				}{
					{
						name:   "Active Only",
						status: database.UserStatusActive,
					},
					{
						name:   "Dormant Only",
						status: database.UserStatusDormant,
					},
					{
						name:   "Suspended Only",
						status: database.UserStatusSuspended,
					},
				}

				for _, tc := range testCases {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						db, _ := dbtestutil.NewDB(t)
						ctx := testutil.Context(t, testutil.WaitShort)

						// Create a user that's been in the specified status for the past 30 days
						dbgen.User(t, db, database.User{
							Status:    tc.status,
							CreatedAt: createdAt,
							UpdatedAt: createdAt,
						})

						userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
							StartTime: dbtime.StartOfDay(createdAt),
							EndTime:   dbtime.StartOfDay(today),
						})
						require.NoError(t, err)

						numDays := int(dbtime.StartOfDay(today).Sub(dbtime.StartOfDay(createdAt)).Hours() / 24)
						require.Len(t, userStatusChanges, numDays+1, "should have 1 entry per day between the start and end time, including the end time")

						for i, row := range userStatusChanges {
							require.Equal(t, tc.status, row.Status, "should have the correct status")
							require.True(
								t,
								row.Date.In(location).Equal(dbtime.StartOfDay(createdAt).AddDate(0, 0, i)),
								"expected date %s, but got %s for row %n",
								dbtime.StartOfDay(createdAt).AddDate(0, 0, i),
								row.Date.In(location).String(),
								i,
							)
							if row.Date.Before(createdAt) {
								require.Equal(t, int64(0), row.Count, "should have 0 users before creation")
							} else {
								require.Equal(t, int64(1), row.Count, "should have 1 user after creation")
							}
						}
					})
				}
			})

			t.Run("One User/One Transition", func(t *testing.T) {
				t.Parallel()

				testCases := []struct {
					name           string
					initialStatus  database.UserStatus
					targetStatus   database.UserStatus
					expectedCounts map[time.Time]map[database.UserStatus]int64
				}{
					{
						name:          "Active to Dormant",
						initialStatus: database.UserStatusActive,
						targetStatus:  database.UserStatusDormant,
						expectedCounts: map[time.Time]map[database.UserStatus]int64{
							createdAt: {
								database.UserStatusActive:  1,
								database.UserStatusDormant: 0,
							},
							firstTransitionTime: {
								database.UserStatusDormant: 1,
								database.UserStatusActive:  0,
							},
							today: {
								database.UserStatusDormant: 1,
								database.UserStatusActive:  0,
							},
						},
					},
					{
						name:          "Active to Suspended",
						initialStatus: database.UserStatusActive,
						targetStatus:  database.UserStatusSuspended,
						expectedCounts: map[time.Time]map[database.UserStatus]int64{
							createdAt: {
								database.UserStatusActive:    1,
								database.UserStatusSuspended: 0,
							},
							firstTransitionTime: {
								database.UserStatusSuspended: 1,
								database.UserStatusActive:    0,
							},
							today: {
								database.UserStatusSuspended: 1,
								database.UserStatusActive:    0,
							},
						},
					},
					{
						name:          "Dormant to Active",
						initialStatus: database.UserStatusDormant,
						targetStatus:  database.UserStatusActive,
						expectedCounts: map[time.Time]map[database.UserStatus]int64{
							createdAt: {
								database.UserStatusDormant: 1,
								database.UserStatusActive:  0,
							},
							firstTransitionTime: {
								database.UserStatusActive:  1,
								database.UserStatusDormant: 0,
							},
							today: {
								database.UserStatusActive:  1,
								database.UserStatusDormant: 0,
							},
						},
					},
					{
						name:          "Dormant to Suspended",
						initialStatus: database.UserStatusDormant,
						targetStatus:  database.UserStatusSuspended,
						expectedCounts: map[time.Time]map[database.UserStatus]int64{
							createdAt: {
								database.UserStatusDormant:   1,
								database.UserStatusSuspended: 0,
							},
							firstTransitionTime: {
								database.UserStatusSuspended: 1,
								database.UserStatusDormant:   0,
							},
							today: {
								database.UserStatusSuspended: 1,
								database.UserStatusDormant:   0,
							},
						},
					},
					{
						name:          "Suspended to Active",
						initialStatus: database.UserStatusSuspended,
						targetStatus:  database.UserStatusActive,
						expectedCounts: map[time.Time]map[database.UserStatus]int64{
							createdAt: {
								database.UserStatusSuspended: 1,
								database.UserStatusActive:    0,
							},
							firstTransitionTime: {
								database.UserStatusActive:    1,
								database.UserStatusSuspended: 0,
							},
							today: {
								database.UserStatusActive:    1,
								database.UserStatusSuspended: 0,
							},
						},
					},
					{
						name:          "Suspended to Dormant",
						initialStatus: database.UserStatusSuspended,
						targetStatus:  database.UserStatusDormant,
						expectedCounts: map[time.Time]map[database.UserStatus]int64{
							createdAt: {
								database.UserStatusSuspended: 1,
								database.UserStatusDormant:   0,
							},
							firstTransitionTime: {
								database.UserStatusDormant:   1,
								database.UserStatusSuspended: 0,
							},
							today: {
								database.UserStatusDormant:   1,
								database.UserStatusSuspended: 0,
							},
						},
					},
				}

				for _, tc := range testCases {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						db, _ := dbtestutil.NewDB(t)
						ctx := testutil.Context(t, testutil.WaitShort)

						// Create a user that starts with initial status
						user := dbgen.User(t, db, database.User{
							Status:    tc.initialStatus,
							CreatedAt: createdAt,
							UpdatedAt: createdAt,
						})

						// After 2 days, change status to target status
						user, err := db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
							ID:        user.ID,
							Status:    tc.targetStatus,
							UpdatedAt: firstTransitionTime,
						})
						require.NoError(t, err)

						// Query for the last 5 days
						userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
							StartTime: dbtime.StartOfDay(createdAt),
							EndTime:   dbtime.StartOfDay(today),
						})
						require.NoError(t, err)

						for i, row := range userStatusChanges {
							require.True(
								t,
								row.Date.In(location).Equal(dbtime.StartOfDay(createdAt).AddDate(0, 0, i/2)),
								"expected date %s, but got %s for row %n",
								dbtime.StartOfDay(createdAt).AddDate(0, 0, i/2),
								row.Date.In(location).String(),
								i,
							)
							if row.Date.Before(createdAt) {
								require.Equal(t, int64(0), row.Count)
							} else if row.Date.Before(firstTransitionTime) {
								if row.Status == tc.initialStatus {
									require.Equal(t, int64(1), row.Count)
								} else if row.Status == tc.targetStatus {
									require.Equal(t, int64(0), row.Count)
								}
							} else if !row.Date.After(today) {
								if row.Status == tc.initialStatus {
									require.Equal(t, int64(0), row.Count)
								} else if row.Status == tc.targetStatus {
									require.Equal(t, int64(1), row.Count)
								}
							} else {
								t.Errorf("date %q beyond expected range end %q", row.Date, today)
							}
						}
					})
				}
			})

			t.Run("Two Users/One Transition", func(t *testing.T) {
				t.Parallel()

				type transition struct {
					from database.UserStatus
					to   database.UserStatus
				}

				type testCase struct {
					name            string
					user1Transition transition
					user2Transition transition
				}

				testCases := []testCase{
					{
						name: "Active->Dormant and Dormant->Suspended",
						user1Transition: transition{
							from: database.UserStatusActive,
							to:   database.UserStatusDormant,
						},
						user2Transition: transition{
							from: database.UserStatusDormant,
							to:   database.UserStatusSuspended,
						},
					},
					{
						name: "Suspended->Active and Active->Dormant",
						user1Transition: transition{
							from: database.UserStatusSuspended,
							to:   database.UserStatusActive,
						},
						user2Transition: transition{
							from: database.UserStatusActive,
							to:   database.UserStatusDormant,
						},
					},
					{
						name: "Dormant->Active and Suspended->Dormant",
						user1Transition: transition{
							from: database.UserStatusDormant,
							to:   database.UserStatusActive,
						},
						user2Transition: transition{
							from: database.UserStatusSuspended,
							to:   database.UserStatusDormant,
						},
					},
					{
						name: "Active->Suspended and Suspended->Active",
						user1Transition: transition{
							from: database.UserStatusActive,
							to:   database.UserStatusSuspended,
						},
						user2Transition: transition{
							from: database.UserStatusSuspended,
							to:   database.UserStatusActive,
						},
					},
					{
						name: "Dormant->Suspended and Dormant->Active",
						user1Transition: transition{
							from: database.UserStatusDormant,
							to:   database.UserStatusSuspended,
						},
						user2Transition: transition{
							from: database.UserStatusDormant,
							to:   database.UserStatusActive,
						},
					},
				}

				for _, tc := range testCases {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()

						db, _ := dbtestutil.NewDB(t)
						ctx := testutil.Context(t, testutil.WaitShort)

						user1 := dbgen.User(t, db, database.User{
							Status:    tc.user1Transition.from,
							CreatedAt: createdAt,
							UpdatedAt: createdAt,
						})
						user2 := dbgen.User(t, db, database.User{
							Status:    tc.user2Transition.from,
							CreatedAt: createdAt,
							UpdatedAt: createdAt,
						})

						// First transition at 2 days
						user1, err := db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
							ID:        user1.ID,
							Status:    tc.user1Transition.to,
							UpdatedAt: firstTransitionTime,
						})
						require.NoError(t, err)

						// Second transition at 4 days
						user2, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
							ID:        user2.ID,
							Status:    tc.user2Transition.to,
							UpdatedAt: secondTransitionTime,
						})
						require.NoError(t, err)

						userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
							StartTime: dbtime.StartOfDay(createdAt),
							EndTime:   dbtime.StartOfDay(today),
						})
						require.NoError(t, err)
						require.NotEmpty(t, userStatusChanges)
						gotCounts := map[time.Time]map[database.UserStatus]int64{}
						for _, row := range userStatusChanges {
							dateInLocation := row.Date.In(location)
							if gotCounts[dateInLocation] == nil {
								gotCounts[dateInLocation] = map[database.UserStatus]int64{}
							}
							gotCounts[dateInLocation][row.Status] = row.Count
						}

						expectedCounts := map[time.Time]map[database.UserStatus]int64{}
						for d := dbtime.StartOfDay(createdAt); !d.After(dbtime.StartOfDay(today)); d = d.AddDate(0, 0, 1) {
							expectedCounts[d] = map[database.UserStatus]int64{}

							// Default values
							expectedCounts[d][tc.user1Transition.from] = 0
							expectedCounts[d][tc.user1Transition.to] = 0
							expectedCounts[d][tc.user2Transition.from] = 0
							expectedCounts[d][tc.user2Transition.to] = 0

							// Counted Values
							if d.Before(createdAt) {
								continue
							} else if d.Before(firstTransitionTime) {
								expectedCounts[d][tc.user1Transition.from]++
								expectedCounts[d][tc.user2Transition.from]++
							} else if d.Before(secondTransitionTime) {
								expectedCounts[d][tc.user1Transition.to]++
								expectedCounts[d][tc.user2Transition.from]++
							} else if d.Before(today) {
								expectedCounts[d][tc.user1Transition.to]++
								expectedCounts[d][tc.user2Transition.to]++
							} else {
								t.Fatalf("date %q beyond expected range end %q", d, today)
							}
						}

						require.Equal(t, expectedCounts, gotCounts)
					})
				}
			})

			t.Run("User precedes and survives query range", func(t *testing.T) {
				t.Parallel()
				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				_ = dbgen.User(t, db, database.User{
					Status:    database.UserStatusActive,
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				})

				userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					StartTime: dbtime.StartOfDay(createdAt.Add(time.Hour * 24)),
					EndTime:   dbtime.StartOfDay(today),
				})
				require.NoError(t, err)

				for i, row := range userStatusChanges {
					require.True(
						t,
						row.Date.In(location).Equal(dbtime.StartOfDay(createdAt).AddDate(0, 0, 1+i)),
						"expected date %s, but got %s for row %n",
						dbtime.StartOfDay(createdAt).AddDate(0, 0, 1+i),
						row.Date.In(location).String(),
						i,
					)
					require.Equal(t, database.UserStatusActive, row.Status)
					require.Equal(t, int64(1), row.Count)
				}
			})

			t.Run("User deleted before query range", func(t *testing.T) {
				t.Parallel()
				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				user := dbgen.User(t, db, database.User{
					Status:    database.UserStatusActive,
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				})

				err = db.UpdateUserDeletedByID(ctx, user.ID)
				require.NoError(t, err)

				userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					StartTime: today.Add(time.Hour * 24),
					EndTime:   today.Add(time.Hour * 48),
				})
				require.NoError(t, err)
				require.Empty(t, userStatusChanges)
			})

			t.Run("User deleted during query range", func(t *testing.T) {
				t.Parallel()

				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				user := dbgen.User(t, db, database.User{
					Status:    database.UserStatusActive,
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				})

				err := db.UpdateUserDeletedByID(ctx, user.ID)
				require.NoError(t, err)

				userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					StartTime: dbtime.StartOfDay(createdAt),
					EndTime:   dbtime.StartOfDay(today.Add(time.Hour * 24)),
				})
				require.NoError(t, err)
				for i, row := range userStatusChanges {
					require.True(
						t,
						row.Date.In(location).Equal(dbtime.StartOfDay(createdAt).AddDate(0, 0, i)),
						"expected date %s, but got %s for row %n",
						dbtime.StartOfDay(createdAt).AddDate(0, 0, i),
						row.Date.In(location).String(),
						i,
					)
					require.Equal(t, database.UserStatusActive, row.Status)
					if row.Date.Before(createdAt) {
						require.Equal(t, int64(0), row.Count)
					} else if i == len(userStatusChanges)-1 {
						require.Equal(t, int64(0), row.Count)
					} else {
						require.Equal(t, int64(1), row.Count)
					}
				}
			})
		})
	}
}

func TestOrganizationDeleteTrigger(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.SkipNow()
	}

	t.Run("WorkspaceExists", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		orgA := dbfake.Organization(t, db).Do()

		user := dbgen.User(t, db, database.User{})

		dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: orgA.Org.ID,
			OwnerID:        user.ID,
		}).Do()

		ctx := testutil.Context(t, testutil.WaitShort)
		err := db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			UpdatedAt: dbtime.Now(),
			ID:        orgA.Org.ID,
		})
		require.Error(t, err)
		// cannot delete organization: organization has 1 workspaces and 1 templates that must be deleted first
		require.ErrorContains(t, err, "cannot delete organization")
		require.ErrorContains(t, err, "has 1 workspaces")
		require.ErrorContains(t, err, "1 templates")
	})

	t.Run("TemplateExists", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		orgA := dbfake.Organization(t, db).Do()

		user := dbgen.User(t, db, database.User{})

		dbgen.Template(t, db, database.Template{
			OrganizationID: orgA.Org.ID,
			CreatedBy:      user.ID,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		err := db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			UpdatedAt: dbtime.Now(),
			ID:        orgA.Org.ID,
		})
		require.Error(t, err)
		// cannot delete organization: organization has 0 workspaces and 1 templates that must be deleted first
		require.ErrorContains(t, err, "cannot delete organization")
		require.ErrorContains(t, err, "has 0 workspaces")
		require.ErrorContains(t, err, "1 templates")
	})

	t.Run("ProvisionerKeyExists", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		orgA := dbfake.Organization(t, db).Do()

		dbgen.ProvisionerKey(t, db, database.ProvisionerKey{
			OrganizationID: orgA.Org.ID,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		err := db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			UpdatedAt: dbtime.Now(),
			ID:        orgA.Org.ID,
		})
		require.Error(t, err)
		// cannot delete organization: organization has 1 provisioner keys that must be deleted first
		require.ErrorContains(t, err, "cannot delete organization")
		require.ErrorContains(t, err, "1 provisioner keys")
	})

	t.Run("GroupExists", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		orgA := dbfake.Organization(t, db).Do()

		dbgen.Group(t, db, database.Group{
			OrganizationID: orgA.Org.ID,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		err := db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			UpdatedAt: dbtime.Now(),
			ID:        orgA.Org.ID,
		})
		require.Error(t, err)
		// cannot delete organization: organization has 1 groups that must be deleted first
		require.ErrorContains(t, err, "cannot delete organization")
		require.ErrorContains(t, err, "has 1 groups")
	})

	t.Run("MemberExists", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		orgA := dbfake.Organization(t, db).Do()

		userA := dbgen.User(t, db, database.User{})
		userB := dbgen.User(t, db, database.User{})

		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: orgA.Org.ID,
			UserID:         userA.ID,
		})

		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: orgA.Org.ID,
			UserID:         userB.ID,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		err := db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			UpdatedAt: dbtime.Now(),
			ID:        orgA.Org.ID,
		})
		require.Error(t, err)
		// cannot delete organization: organization has 1 members that must be deleted first
		require.ErrorContains(t, err, "cannot delete organization")
		require.ErrorContains(t, err, "has 1 members")
	})
}

func TestGetPresetsBackoff(t *testing.T) {
	t.Parallel()
	type extTmplVersion struct {
		database.TemplateVersion
		preset database.TemplateVersionPreset
	}

	now := dbtime.Now()
	orgID := uuid.New()
	userID := uuid.New()

	createTemplate := func(db database.Store) database.Template {
		// create template
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID:  orgID,
			CreatedBy:       userID,
			ActiveVersionID: uuid.New(),
		})

		return tmpl
	}
	type tmplVersionOpts struct {
		DesiredInstances int
	}
	createTmplVersion := func(db database.Store, tmpl database.Template, versionId uuid.UUID, opts *tmplVersionOpts) extTmplVersion {
		// Create template version with corresponding preset and preset prebuild
		tmplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			ID: versionId,
			TemplateID: uuid.NullUUID{
				UUID:  tmpl.ID,
				Valid: true,
			},
			OrganizationID: tmpl.OrganizationID,
			CreatedAt:      now,
			UpdatedAt:      now,
			CreatedBy:      tmpl.CreatedBy,
		})
		preset := dbgen.Preset(t, db, database.InsertPresetParams{
			TemplateVersionID: tmplVersion.ID,
			Name:              "preset",
		})
		desiredInstances := 1
		if opts != nil {
			desiredInstances = opts.DesiredInstances
		}
		dbgen.PresetPrebuild(t, db, database.InsertPresetPrebuildParams{
			PresetID:         preset.ID,
			DesiredInstances: int32(desiredInstances),
		})

		return extTmplVersion{
			TemplateVersion: tmplVersion,
			preset:          preset,
		}
	}
	type workspaceBuildOpts struct {
		successfulJob bool
		createdAt     time.Time
	}
	createWorkspaceBuild := func(
		db database.Store,
		tmpl database.Template,
		extTmplVersion extTmplVersion,
		opts *workspaceBuildOpts,
	) {
		// Create job with corresponding resource and agent
		jobError := sql.NullString{String: "failed", Valid: true}
		if opts != nil && opts.successfulJob {
			jobError = sql.NullString{}
		}
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: orgID,

			CreatedAt: now.Add(-1 * time.Minute),
			Error:     jobError,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})

		// Create corresponding workspace and workspace build
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        tmpl.CreatedBy,
			OrganizationID: tmpl.OrganizationID,
			TemplateID:     tmpl.ID,
		})
		createdAt := now
		if opts != nil {
			createdAt = opts.createdAt
		}
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			CreatedAt:         createdAt,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: extTmplVersion.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       tmpl.CreatedBy,
			JobID:             job.ID,
			TemplateVersionPresetID: uuid.NullUUID{
				UUID:  extTmplVersion.preset.ID,
				Valid: true,
			},
		})
	}
	findBackoffByTmplVersionId := func(backoffs []database.GetPresetsBackoffRow, tmplVersionID uuid.UUID) *database.GetPresetsBackoffRow {
		for _, backoff := range backoffs {
			if backoff.TemplateVersionID == tmplVersionID {
				return &backoff
			}
		}

		return nil
	}

	t.Run("Single Workspace Build", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl := createTemplate(db)
		tmplV1 := createTmplVersion(db, tmpl, tmpl.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl, tmplV1, nil)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 1)
		backoff := backoffs[0]
		require.Equal(t, backoff.TemplateVersionID, tmpl.ActiveVersionID)
		require.Equal(t, backoff.PresetID, tmplV1.preset.ID)
		require.Equal(t, int32(1), backoff.NumFailed)
	})

	t.Run("Multiple Workspace Builds", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl := createTemplate(db)
		tmplV1 := createTmplVersion(db, tmpl, tmpl.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl, tmplV1, nil)
		createWorkspaceBuild(db, tmpl, tmplV1, nil)
		createWorkspaceBuild(db, tmpl, tmplV1, nil)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 1)
		backoff := backoffs[0]
		require.Equal(t, backoff.TemplateVersionID, tmpl.ActiveVersionID)
		require.Equal(t, backoff.PresetID, tmplV1.preset.ID)
		require.Equal(t, int32(3), backoff.NumFailed)
	})

	t.Run("Ignore Inactive Version", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl := createTemplate(db)
		tmplV1 := createTmplVersion(db, tmpl, uuid.New(), nil)
		createWorkspaceBuild(db, tmpl, tmplV1, nil)

		// Active Version
		tmplV2 := createTmplVersion(db, tmpl, tmpl.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl, tmplV2, nil)
		createWorkspaceBuild(db, tmpl, tmplV2, nil)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 1)
		backoff := backoffs[0]
		require.Equal(t, backoff.TemplateVersionID, tmpl.ActiveVersionID)
		require.Equal(t, backoff.PresetID, tmplV2.preset.ID)
		require.Equal(t, int32(2), backoff.NumFailed)
	})

	t.Run("Multiple Templates", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl1, tmpl1V1, nil)

		tmpl2 := createTemplate(db)
		tmpl2V1 := createTmplVersion(db, tmpl2, tmpl2.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl2, tmpl2V1, nil)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 2)
		{
			backoff := findBackoffByTmplVersionId(backoffs, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(1), backoff.NumFailed)
		}
		{
			backoff := findBackoffByTmplVersionId(backoffs, tmpl2.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl2.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl2V1.preset.ID)
			require.Equal(t, int32(1), backoff.NumFailed)
		}
	})

	t.Run("Multiple Templates, Versions and Workspace Builds", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl1, tmpl1V1, nil)

		tmpl2 := createTemplate(db)
		tmpl2V1 := createTmplVersion(db, tmpl2, tmpl2.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl2, tmpl2V1, nil)
		createWorkspaceBuild(db, tmpl2, tmpl2V1, nil)

		tmpl3 := createTemplate(db)
		tmpl3V1 := createTmplVersion(db, tmpl3, uuid.New(), nil)
		createWorkspaceBuild(db, tmpl3, tmpl3V1, nil)

		tmpl3V2 := createTmplVersion(db, tmpl3, tmpl3.ActiveVersionID, nil)
		createWorkspaceBuild(db, tmpl3, tmpl3V2, nil)
		createWorkspaceBuild(db, tmpl3, tmpl3V2, nil)
		createWorkspaceBuild(db, tmpl3, tmpl3V2, nil)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 3)
		{
			backoff := findBackoffByTmplVersionId(backoffs, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(1), backoff.NumFailed)
		}
		{
			backoff := findBackoffByTmplVersionId(backoffs, tmpl2.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl2.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl2V1.preset.ID)
			require.Equal(t, int32(2), backoff.NumFailed)
		}
		{
			backoff := findBackoffByTmplVersionId(backoffs, tmpl3.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl3.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl3V2.preset.ID)
			require.Equal(t, int32(3), backoff.NumFailed)
		}
	})

	t.Run("No Workspace Builds", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, nil)
		_ = tmpl1V1

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)
		require.Nil(t, backoffs)
	})

	t.Run("No Failed Workspace Builds", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, nil)
		successfulJobOpts := workspaceBuildOpts{
			successfulJob: true,
		}
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &successfulJobOpts)
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &successfulJobOpts)
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &successfulJobOpts)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)
		require.Nil(t, backoffs)
	})

	t.Run("Last job is successful - no backoff", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, &tmplVersionOpts{
			DesiredInstances: 1,
		})
		failedJobOpts := workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-2 * time.Minute),
		}
		successfulJobOpts := workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-1 * time.Minute),
		}
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &failedJobOpts)
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &successfulJobOpts)

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)
		require.Nil(t, backoffs)
	})

	t.Run("Last 3 jobs are successful - no backoff", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, &tmplVersionOpts{
			DesiredInstances: 3,
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-4 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-3 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-2 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-1 * time.Minute),
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)
		require.Nil(t, backoffs)
	})

	t.Run("1 job failed out of 3 - backoff", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, &tmplVersionOpts{
			DesiredInstances: 3,
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-3 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-2 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-1 * time.Minute),
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 1)
		{
			backoff := backoffs[0]
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(1), backoff.NumFailed)
		}
	})

	t.Run("3 job failed out of 5 - backoff", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})
		lookbackPeriod := time.Hour

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, &tmplVersionOpts{
			DesiredInstances: 3,
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-lookbackPeriod - time.Minute), // earlier than lookback period - skipped
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-4 * time.Minute), // within lookback period - counted as failed job
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-3 * time.Minute), // within lookback period - counted as failed job
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-2 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: true,
			createdAt:     now.Add(-1 * time.Minute),
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-lookbackPeriod))
		require.NoError(t, err)

		require.Len(t, backoffs, 1)
		{
			backoff := backoffs[0]
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(2), backoff.NumFailed)
		}
	})

	t.Run("check LastBuildAt timestamp", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
		dbgen.User(t, db, database.User{
			ID: userID,
		})
		lookbackPeriod := time.Hour

		tmpl1 := createTemplate(db)
		tmpl1V1 := createTmplVersion(db, tmpl1, tmpl1.ActiveVersionID, &tmplVersionOpts{
			DesiredInstances: 6,
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-lookbackPeriod - time.Minute), // earlier than lookback period - skipped
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-4 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-0 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-3 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-1 * time.Minute),
		})
		createWorkspaceBuild(db, tmpl1, tmpl1V1, &workspaceBuildOpts{
			successfulJob: false,
			createdAt:     now.Add(-2 * time.Minute),
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-lookbackPeriod))
		require.NoError(t, err)

		require.Len(t, backoffs, 1)
		{
			backoff := backoffs[0]
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(5), backoff.NumFailed)
			// make sure LastBuildAt is equal to latest failed build timestamp
			require.Equal(t, 0, now.Compare(backoff.LastBuildAt.(time.Time)))
		}
	})
}

func requireUsersMatch(t testing.TB, expected []database.User, found []database.GetUsersRow, msg string) {
	t.Helper()
	require.ElementsMatch(t, expected, database.ConvertUserRows(found), msg)
}
