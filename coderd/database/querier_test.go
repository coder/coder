package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
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
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
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
			Offline:        sql.NullBool{Bool: true, Valid: true},
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
			Offline:        sql.NullBool{Bool: true, Valid: true},
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
			Offline:         sql.NullBool{Bool: true, Valid: true},
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

	t.Run("ExcludeOffline", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "offline-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-time.Hour),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-time.Hour),
			},
		})
		fooDaemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "foo-daemon",
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
		require.Len(t, daemons, 1)

		require.Equal(t, fooDaemon.ID, daemons[0].ProvisionerDaemon.ID)
		require.Equal(t, database.ProvisionerDaemonStatusIdle, daemons[0].Status)
	})

	t.Run("IncludeOffline", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "offline-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-time.Hour),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-time.Hour),
			},
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "foo-daemon",
			OrganizationID: org.ID,
			Tags: database.StringMap{
				"foo": "bar",
			},
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "bar-daemon",
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
			Offline:         sql.NullBool{Bool: true, Valid: true},
		})
		require.NoError(t, err)
		require.Len(t, daemons, 3)

		statusCounts := make(map[database.ProvisionerDaemonStatus]int)
		for _, daemon := range daemons {
			statusCounts[daemon.Status]++
		}

		require.Equal(t, 2, statusCounts[database.ProvisionerDaemonStatusIdle])
		require.Equal(t, 1, statusCounts[database.ProvisionerDaemonStatusOffline])
	})

	t.Run("MatchesStatuses", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "offline-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-time.Hour),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-time.Hour),
			},
		})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "foo-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-(30 * time.Minute)),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-(30 * time.Minute)),
			},
		})

		type testCase struct {
			name        string
			statuses    []database.ProvisionerDaemonStatus
			expectedNum int
		}

		tests := []testCase{
			{
				name: "Get idle and offline",
				statuses: []database.ProvisionerDaemonStatus{
					database.ProvisionerDaemonStatusOffline,
					database.ProvisionerDaemonStatusIdle,
				},
				expectedNum: 2,
			},
			{
				name: "Get offline",
				statuses: []database.ProvisionerDaemonStatus{
					database.ProvisionerDaemonStatusOffline,
				},
				expectedNum: 1,
			},
			// Offline daemons should not be included without Offline param
			{
				name:        "Get idle - empty statuses",
				statuses:    []database.ProvisionerDaemonStatus{},
				expectedNum: 1,
			},
			{
				name:        "Get idle - nil statuses",
				statuses:    nil,
				expectedNum: 1,
			},
		}

		for _, tc := range tests {
			//nolint:tparallel,paralleltest
			t.Run(tc.name, func(t *testing.T) {
				daemons, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
					OrganizationID:  org.ID,
					StaleIntervalMS: 45 * time.Minute.Milliseconds(),
					Statuses:        tc.statuses,
				})
				require.NoError(t, err)
				require.Len(t, daemons, tc.expectedNum)
			})
		}
	})

	t.Run("FilterByMaxAge", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "foo-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-(45 * time.Minute)),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-(45 * time.Minute)),
			},
		})

		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:           "bar-daemon",
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now().Add(-(25 * time.Minute)),
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  dbtime.Now().Add(-(25 * time.Minute)),
			},
		})

		type testCase struct {
			name        string
			maxAge      sql.NullInt64
			expectedNum int
		}

		tests := []testCase{
			{
				name:        "Max age 1 hour",
				maxAge:      sql.NullInt64{Int64: time.Hour.Milliseconds(), Valid: true},
				expectedNum: 2,
			},
			{
				name:        "Max age 30 minutes",
				maxAge:      sql.NullInt64{Int64: (30 * time.Minute).Milliseconds(), Valid: true},
				expectedNum: 1,
			},
			{
				name:        "Max age 15 minutes",
				maxAge:      sql.NullInt64{Int64: (15 * time.Minute).Milliseconds(), Valid: true},
				expectedNum: 0,
			},
			{
				name:        "No max age",
				maxAge:      sql.NullInt64{Valid: false},
				expectedNum: 2,
			},
		}
		for _, tc := range tests {
			//nolint:tparallel,paralleltest
			t.Run(tc.name, func(t *testing.T) {
				daemons, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
					OrganizationID:  org.ID,
					StaleIntervalMS: 60 * time.Minute.Milliseconds(),
					MaxAgeMs:        tc.maxAge,
				})
				require.NoError(t, err)
				require.Len(t, daemons, tc.expectedNum)
			})
		}
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

	queued, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{
		IDs:             jobIDs,
		StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
	})
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

	queued, err = db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{
		IDs:             jobIDs,
		StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
	})
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

func TestAcquireProvisionerJob(t *testing.T) {
	t.Parallel()

	t.Run("HumanInitiatedJobsFirst", func(t *testing.T) {
		t.Parallel()
		var (
			db, _       = dbtestutil.NewDB(t)
			ctx         = testutil.Context(t, testutil.WaitMedium)
			org         = dbgen.Organization(t, db, database.Organization{})
			_           = dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{}) // Required for queue position
			now         = dbtime.Now()
			numJobs     = 10
			humanIDs    = make([]uuid.UUID, 0, numJobs/2)
			prebuildIDs = make([]uuid.UUID, 0, numJobs/2)
		)

		// Given: a number of jobs in the queue, with prebuilds and non-prebuilds interleaved
		for idx := range numJobs {
			var initiator uuid.UUID
			if idx%2 == 0 {
				initiator = database.PrebuildsSystemUserID
			} else {
				initiator = uuid.MustParse("c0dec0de-c0de-c0de-c0de-c0dec0dec0de")
			}
			pj, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
				ID:             uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-00000000000%x", idx+1)),
				CreatedAt:      time.Now().Add(-time.Second * time.Duration(idx)),
				UpdatedAt:      time.Now().Add(-time.Second * time.Duration(idx)),
				InitiatorID:    initiator,
				OrganizationID: org.ID,
				Provisioner:    database.ProvisionerTypeEcho,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				StorageMethod:  database.ProvisionerStorageMethodFile,
				FileID:         uuid.New(),
				Input:          json.RawMessage(`{}`),
				Tags:           database.StringMap{},
				TraceMetadata:  pqtype.NullRawMessage{},
			})
			require.NoError(t, err)
			// We expected prebuilds to be acquired after human-initiated jobs.
			if initiator == database.PrebuildsSystemUserID {
				prebuildIDs = append([]uuid.UUID{pj.ID}, prebuildIDs...)
			} else {
				humanIDs = append([]uuid.UUID{pj.ID}, humanIDs...)
			}
			t.Logf("created job id=%q initiator=%q created_at=%q", pj.ID.String(), pj.InitiatorID.String(), pj.CreatedAt.String())
		}

		expectedIDs := append(humanIDs, prebuildIDs...) //nolint:gocritic // not the same slice

		// When: we query the queue positions for the jobs
		qjs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{
			IDs:             expectedIDs,
			StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
		})
		require.NoError(t, err)
		require.Len(t, qjs, numJobs)
		// Ensure the jobs are sorted by queue position.
		sort.Slice(qjs, func(i, j int) bool {
			return qjs[i].QueuePosition < qjs[j].QueuePosition
		})

		// Then: the queue positions for the jobs should indicate the order in which
		// they will be acquired, with human-initiated jobs first.
		for idx, qj := range qjs {
			t.Logf("queued job %d/%d id=%q initiator=%q created_at=%q queue_position=%d", idx+1, numJobs, qj.ProvisionerJob.ID.String(), qj.ProvisionerJob.InitiatorID.String(), qj.ProvisionerJob.CreatedAt.String(), qj.QueuePosition)
			require.Equal(t, expectedIDs[idx].String(), qj.ProvisionerJob.ID.String(), "job %d/%d should match expected id", idx+1, numJobs)
			require.Equal(t, int64(idx+1), qj.QueuePosition, "job %d/%d should have queue position %d", idx+1, numJobs, idx+1)
		}

		// When: the jobs are acquired
		// Then: human-initiated jobs are prioritized first.
		for idx := range numJobs {
			acquired, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
				OrganizationID:  org.ID,
				StartedAt:       sql.NullTime{Time: time.Now(), Valid: true},
				WorkerID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
				Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
				ProvisionerTags: json.RawMessage(`{}`),
			})
			require.NoError(t, err)
			require.Equal(t, expectedIDs[idx].String(), acquired.ID.String(), "acquired job %d/%d with initiator %q", idx+1, numJobs, acquired.InitiatorID.String())
			t.Logf("acquired job id=%q initiator=%q created_at=%q", acquired.ID.String(), acquired.InitiatorID.String(), acquired.CreatedAt.String())
			err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID:          acquired.ID,
				UpdatedAt:   now,
				CompletedAt: sql.NullTime{Time: now, Valid: true},
				Error:       sql.NullString{},
				ErrorCode:   sql.NullString{},
			})
			require.NoError(t, err, "mark job %d/%d as complete", idx+1, numJobs)
		}
	})
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

func TestGetUsers_IncludeSystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		includeSystem  bool
		wantSystemUser bool
	}{
		{
			name:           "include system users",
			includeSystem:  true,
			wantSystemUser: true,
		},
		{
			name:           "exclude system users",
			includeSystem:  false,
			wantSystemUser: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)

			// Given: a system user
			// postgres: introduced by migration coderd/database/migrations/00030*_system_user.up.sql
			db, _ := dbtestutil.NewDB(t)
			other := dbgen.User(t, db, database.User{})
			users, err := db.GetUsers(ctx, database.GetUsersParams{
				IncludeSystem: tt.includeSystem,
			})
			require.NoError(t, err)

			// Should always find the regular user
			foundRegularUser := false
			foundSystemUser := false

			for _, u := range users {
				if u.IsSystem {
					foundSystemUser = true
					require.Equal(t, database.PrebuildsSystemUserID, u.ID)
				} else {
					foundRegularUser = true
					require.Equalf(t, other.ID.String(), u.ID.String(), "found unexpected regular user")
				}
			}

			require.True(t, foundRegularUser, "regular user should always be found")
			require.Equal(t, tt.wantSystemUser, foundSystemUser, "system user presence should match includeSystem setting")
			require.Equal(t, tt.wantSystemUser, len(users) == 2, "should have 2 users when including system user, 1 otherwise")
		})
	}
}

func TestUpdateSystemUser(t *testing.T) {
	t.Parallel()

	// TODO (sasswart): We've disabled the protection that prevents updates to system users
	// while we reassess the mechanism to do so. Rather than skip the test, we've just inverted
	// the assertions to ensure that the behavior is as desired.
	// Once we've re-enabeld the system user protection, we'll revert the assertions.

	ctx := testutil.Context(t, testutil.WaitLong)

	// Given: a system user introduced by migration coderd/database/migrations/00030*_system_user.up.sql
	db, _ := dbtestutil.NewDB(t)
	users, err := db.GetUsers(ctx, database.GetUsersParams{
		IncludeSystem: true,
	})
	require.NoError(t, err)
	var systemUser database.GetUsersRow
	for _, u := range users {
		if u.IsSystem {
			systemUser = u
		}
	}
	require.NotNil(t, systemUser)

	// When: attempting to update a system user's name.
	_, err = db.UpdateUserProfile(ctx, database.UpdateUserProfileParams{
		ID:        systemUser.ID,
		Email:     systemUser.Email,
		Username:  systemUser.Username,
		AvatarURL: systemUser.AvatarURL,
		Name:      "not prebuilds",
	})
	// Then: the attempt is rejected by a postgres trigger.
	// require.ErrorContains(t, err, "Cannot modify or delete system users")
	require.NoError(t, err)

	// When: attempting to delete a system user.
	err = db.UpdateUserDeletedByID(ctx, systemUser.ID)
	// Then: the attempt is rejected by a postgres trigger.
	// require.ErrorContains(t, err, "Cannot modify or delete system users")
	require.NoError(t, err)

	// When: attempting to update a user's roles.
	_, err = db.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
		ID:           systemUser.ID,
		GrantedRoles: []string{rbac.RoleAuditor().String()},
	})
	// Then: the attempt is rejected by a postgres trigger.
	// require.ErrorContains(t, err, "Cannot modify or delete system users")
	require.NoError(t, err)
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

func TestAuditLogCount(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	ctx := testutil.Context(t, testutil.WaitLong)

	dbgen.AuditLog(t, db, database.AuditLog{})

	count, err := db.CountAuditLogs(ctx, database.CountAuditLogsParams{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
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
		everyoneMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
			GroupID:       everyoneGroup.ID,
			IncludeSystem: false,
		})
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

func TestDeleteCustomRoleDoesNotDeleteSystemRole(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	ctx := testutil.Context(t, testutil.WaitShort)

	systemRole, err := db.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:        "test-system-role",
		DisplayName: "",
		OrganizationID: uuid.NullUUID{
			UUID:  org.ID,
			Valid: true,
		},
		SitePermissions:   database.CustomRolePermissions{},
		OrgPermissions:    database.CustomRolePermissions{},
		UserPermissions:   database.CustomRolePermissions{},
		MemberPermissions: database.CustomRolePermissions{},
		IsSystem:          true,
	})
	require.NoError(t, err)

	nonSystemRole, err := db.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:        "test-custom-role",
		DisplayName: "",
		OrganizationID: uuid.NullUUID{
			UUID:  org.ID,
			Valid: true,
		},
		SitePermissions:   database.CustomRolePermissions{},
		OrgPermissions:    database.CustomRolePermissions{},
		UserPermissions:   database.CustomRolePermissions{},
		MemberPermissions: database.CustomRolePermissions{},
		IsSystem:          false,
	})
	require.NoError(t, err)

	err = db.DeleteCustomRole(ctx, database.DeleteCustomRoleParams{
		Name: systemRole.Name,
		OrganizationID: uuid.NullUUID{
			UUID:  org.ID,
			Valid: true,
		},
	})
	require.NoError(t, err)

	err = db.DeleteCustomRole(ctx, database.DeleteCustomRoleParams{
		Name: nonSystemRole.Name,
		OrganizationID: uuid.NullUUID{
			UUID:  org.ID,
			Valid: true,
		},
	})
	require.NoError(t, err)

	roles, err := db.CustomRoles(ctx, database.CustomRolesParams{
		LookupRoles: []database.NameOrganizationPair{
			{
				Name:           systemRole.Name,
				OrganizationID: org.ID,
			},
			{
				Name:           nonSystemRole.Name,
				OrganizationID: org.ID,
			},
		},
		IncludeSystemRoles: true,
	})
	require.NoError(t, err)

	require.Len(t, roles, 1)
	require.Equal(t, systemRole.Name, roles[0].Name)
	require.True(t, roles[0].IsSystem)
}

func TestUpdateOrganizationWorkspaceSharingSettings(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	ctx := testutil.Context(t, testutil.WaitShort)

	updated, err := db.UpdateOrganizationWorkspaceSharingSettings(ctx, database.UpdateOrganizationWorkspaceSharingSettingsParams{
		ID:                       org.ID,
		WorkspaceSharingDisabled: true,
		UpdatedAt:                dbtime.Now(),
	})
	require.NoError(t, err)
	require.True(t, updated.WorkspaceSharingDisabled)

	got, err := db.GetOrganizationByID(ctx, org.ID)
	require.NoError(t, err)
	require.True(t, got.WorkspaceSharingDisabled)
}

func TestDeleteWorkspaceACLsByOrganization(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org1 := dbgen.Organization(t, db, database.Organization{})
	org2 := dbgen.Organization(t, db, database.Organization{})

	owner1 := dbgen.User(t, db, database.User{})
	owner2 := dbgen.User(t, db, database.User{})
	sharedUser := dbgen.User(t, db, database.User{})
	sharedGroup := dbgen.Group(t, db, database.Group{
		OrganizationID: org1.ID,
	})

	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org1.ID,
		UserID:         owner1.ID,
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org2.ID,
		UserID:         owner2.ID,
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org1.ID,
		UserID:         sharedUser.ID,
	})

	ws1 := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        owner1.ID,
		OrganizationID: org1.ID,
		UserACL: database.WorkspaceACL{
			sharedUser.ID.String(): {
				Permissions: []policy.Action{policy.ActionRead},
			},
		},
		GroupACL: database.WorkspaceACL{
			sharedGroup.ID.String(): {
				Permissions: []policy.Action{policy.ActionRead},
			},
		},
	}).Do().Workspace

	ws2 := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        owner2.ID,
		OrganizationID: org2.ID,
		UserACL: database.WorkspaceACL{
			uuid.NewString(): {
				Permissions: []policy.Action{policy.ActionRead},
			},
		},
	}).Do().Workspace

	ctx := testutil.Context(t, testutil.WaitShort)

	err := db.DeleteWorkspaceACLsByOrganization(ctx, org1.ID)
	require.NoError(t, err)

	got1, err := db.GetWorkspaceByID(ctx, ws1.ID)
	require.NoError(t, err)
	require.Empty(t, got1.UserACL)
	require.Empty(t, got1.GroupACL)

	got2, err := db.GetWorkspaceByID(ctx, ws2.ID)
	require.NoError(t, err)
	require.NotEmpty(t, got2.UserACL)
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
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A user who is a member of 0 organizations
		memberCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "member",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{memberRole},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for audit logs
		count, err := db.CountAuditLogs(memberCtx, database.CountAuditLogsParams{})
		require.NoError(t, err)
		logs, err := db.GetAuditLogsOffset(memberCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)

		// Then: No logs returned and count is 0
		require.Equal(t, int64(0), count, "count should be 0")
		require.Len(t, logs, 0, "no logs should be returned")
	})

	t.Run("SiteWideAuditor", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A site wide auditor
		siteAuditorCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "owner",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{auditorRole},
			Scope:        rbac.ScopeAll,
		})

		// When: the auditor queries for audit logs
		count, err := db.CountAuditLogs(siteAuditorCtx, database.CountAuditLogsParams{})
		require.NoError(t, err)
		logs, err := db.GetAuditLogsOffset(siteAuditorCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)

		// Then: All logs are returned and count matches
		require.Equal(t, int64(len(allLogs)), count, "count should match total number of logs")
		require.ElementsMatch(t, auditOnlyIDs(allLogs), auditOnlyIDs(logs), "all logs should be returned")
	})

	t.Run("SingleOrgAuditor", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		orgID := orgIDs[0]
		// Given: An organization scoped auditor
		orgAuditCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, orgID)},
			Scope:        rbac.ScopeAll,
		})

		// When: The auditor queries for audit logs
		count, err := db.CountAuditLogs(orgAuditCtx, database.CountAuditLogsParams{})
		require.NoError(t, err)
		logs, err := db.GetAuditLogsOffset(orgAuditCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)

		// Then: Only the logs for the organization are returned and count matches
		require.Equal(t, int64(len(orgAuditLogs[orgID])), count, "count should match organization logs")
		require.ElementsMatch(t, orgAuditLogs[orgID], auditOnlyIDs(logs), "only organization logs should be returned")
	})

	t.Run("TwoOrgAuditors", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

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
		count, err := db.CountAuditLogs(multiOrgAuditCtx, database.CountAuditLogsParams{})
		require.NoError(t, err)
		logs, err := db.GetAuditLogsOffset(multiOrgAuditCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)

		// Then: All logs for both organizations are returned and count matches
		expectedLogs := append([]uuid.UUID{}, orgAuditLogs[first]...)
		expectedLogs = append(expectedLogs, orgAuditLogs[second]...)
		require.Equal(t, int64(len(expectedLogs)), count, "count should match sum of both organizations")
		require.ElementsMatch(t, expectedLogs, auditOnlyIDs(logs), "logs from both organizations should be returned")
	})

	t.Run("ErroneousOrg", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A user who is an auditor for an organization that has 0 logs
		userCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, uuid.New())},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for audit logs
		count, err := db.CountAuditLogs(userCtx, database.CountAuditLogsParams{})
		require.NoError(t, err)
		logs, err := db.GetAuditLogsOffset(userCtx, database.GetAuditLogsOffsetParams{})
		require.NoError(t, err)

		// Then: No logs are returned and count is 0
		require.Equal(t, int64(0), count, "count should be 0")
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

func TestGetAuthorizedConnectionLogsOffset(t *testing.T) {
	t.Parallel()

	var allLogs []database.ConnectionLog
	db, _ := dbtestutil.NewDB(t)
	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	authDb := dbauthz.New(db, authz, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())

	orgA := dbfake.Organization(t, db).Do()
	orgB := dbfake.Organization(t, db).Do()

	user := dbgen.User(t, db, database.User{})

	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: orgA.Org.ID,
		CreatedBy:      user.ID,
	})

	wsID := uuid.New()
	createTemplateVersion(t, db, tpl, tvArgs{
		WorkspaceTransition: database.WorkspaceTransitionStart,
		Status:              database.ProvisionerJobStatusSucceeded,
		CreateWorkspace:     true,
		WorkspaceID:         wsID,
	})

	// This map is a simple way to insert a given number of organizations
	// and audit logs for each organization.
	// map[orgID][]ConnectionLogID
	orgConnectionLogs := map[uuid.UUID][]uuid.UUID{
		orgA.Org.ID: {uuid.New(), uuid.New()},
		orgB.Org.ID: {uuid.New(), uuid.New()},
	}
	orgIDs := make([]uuid.UUID, 0, len(orgConnectionLogs))
	for orgID := range orgConnectionLogs {
		orgIDs = append(orgIDs, orgID)
	}
	for orgID, ids := range orgConnectionLogs {
		for _, id := range ids {
			allLogs = append(allLogs, dbgen.ConnectionLog(t, authDb, database.UpsertConnectionLogParams{
				WorkspaceID:      wsID,
				WorkspaceOwnerID: user.ID,
				ID:               id,
				OrganizationID:   orgID,
			}))
		}
	}

	// Now fetch all the logs
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
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A user who is a member of 0 organizations
		memberCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "member",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{memberRole},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for connection logs
		logs, err := authDb.GetConnectionLogsOffset(memberCtx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		// Then: No logs returned
		require.Len(t, logs, 0, "no logs should be returned")
		// And: The count matches the number of logs returned
		count, err := authDb.CountConnectionLogs(memberCtx, database.CountConnectionLogsParams{})
		require.NoError(t, err)
		require.EqualValues(t, len(logs), count)
	})

	t.Run("SiteWideAuditor", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A site wide auditor
		siteAuditorCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "owner",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{auditorRole},
			Scope:        rbac.ScopeAll,
		})

		// When: the auditor queries for connection logs
		logs, err := authDb.GetConnectionLogsOffset(siteAuditorCtx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		// Then: All logs are returned
		require.ElementsMatch(t, connectionOnlyIDs(allLogs), connectionOnlyIDs(logs))
		// And: The count matches the number of logs returned
		count, err := authDb.CountConnectionLogs(siteAuditorCtx, database.CountConnectionLogsParams{})
		require.NoError(t, err)
		require.EqualValues(t, len(logs), count)
	})

	t.Run("SingleOrgAuditor", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		orgID := orgIDs[0]
		// Given: An organization scoped auditor
		orgAuditCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, orgID)},
			Scope:        rbac.ScopeAll,
		})

		// When: The auditor queries for connection logs
		logs, err := authDb.GetConnectionLogsOffset(orgAuditCtx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		// Then: Only the logs for the organization are returned
		require.ElementsMatch(t, orgConnectionLogs[orgID], connectionOnlyIDs(logs))
		// And: The count matches the number of logs returned
		count, err := authDb.CountConnectionLogs(orgAuditCtx, database.CountConnectionLogsParams{})
		require.NoError(t, err)
		require.EqualValues(t, len(logs), count)
	})

	t.Run("TwoOrgAuditors", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		first := orgIDs[0]
		second := orgIDs[1]
		// Given: A user who is an auditor for two organizations
		multiOrgAuditCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, first), orgAuditorRoles(t, second)},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for connection logs
		logs, err := authDb.GetConnectionLogsOffset(multiOrgAuditCtx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		// Then: All logs for both organizations are returned
		require.ElementsMatch(t, append(orgConnectionLogs[first], orgConnectionLogs[second]...), connectionOnlyIDs(logs))
		// And: The count matches the number of logs returned
		count, err := authDb.CountConnectionLogs(multiOrgAuditCtx, database.CountConnectionLogsParams{})
		require.NoError(t, err)
		require.EqualValues(t, len(logs), count)
	})

	t.Run("ErroneousOrg", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A user who is an auditor for an organization that has 0 logs
		userCtx := dbauthz.As(ctx, rbac.Subject{
			FriendlyName: "org-auditor",
			ID:           uuid.NewString(),
			Roles:        rbac.Roles{orgAuditorRoles(t, uuid.New())},
			Scope:        rbac.ScopeAll,
		})

		// When: The user queries for audit logs
		logs, err := authDb.GetConnectionLogsOffset(userCtx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		// Then: No logs are returned
		require.Len(t, logs, 0, "no logs should be returned")
		// And: The count matches the number of logs returned
		count, err := authDb.CountConnectionLogs(userCtx, database.CountConnectionLogsParams{})
		require.NoError(t, err)
		require.EqualValues(t, len(logs), count)
	})
}

func TestCountConnectionLogs(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	db, _ := dbtestutil.NewDB(t)

	orgA := dbfake.Organization(t, db).Do()
	userA := dbgen.User(t, db, database.User{})
	tplA := dbgen.Template(t, db, database.Template{OrganizationID: orgA.Org.ID, CreatedBy: userA.ID})
	wsA := dbgen.Workspace(t, db, database.WorkspaceTable{OwnerID: userA.ID, OrganizationID: orgA.Org.ID, TemplateID: tplA.ID})

	orgB := dbfake.Organization(t, db).Do()
	userB := dbgen.User(t, db, database.User{})
	tplB := dbgen.Template(t, db, database.Template{OrganizationID: orgB.Org.ID, CreatedBy: userB.ID})
	wsB := dbgen.Workspace(t, db, database.WorkspaceTable{OwnerID: userB.ID, OrganizationID: orgB.Org.ID, TemplateID: tplB.ID})

	// Create logs for two different orgs.
	for i := 0; i < 20; i++ {
		dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			OrganizationID:   wsA.OrganizationID,
			WorkspaceOwnerID: wsA.OwnerID,
			WorkspaceID:      wsA.ID,
			Type:             database.ConnectionTypeSsh,
		})
	}
	for i := 0; i < 10; i++ {
		dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
			OrganizationID:   wsB.OrganizationID,
			WorkspaceOwnerID: wsB.OwnerID,
			WorkspaceID:      wsB.ID,
			Type:             database.ConnectionTypeSsh,
		})
	}

	// Count with a filter for orgA.
	countParams := database.CountConnectionLogsParams{
		OrganizationID: orgA.Org.ID,
	}
	totalCount, err := db.CountConnectionLogs(ctx, countParams)
	require.NoError(t, err)
	require.Equal(t, int64(20), totalCount)

	// Get a paginated result for the same filter.
	getParams := database.GetConnectionLogsOffsetParams{
		OrganizationID: orgA.Org.ID,
		LimitOpt:       5,
		OffsetOpt:      10,
	}
	logs, err := db.GetConnectionLogsOffset(ctx, getParams)
	require.NoError(t, err)
	require.Len(t, logs, 5)

	// The count with the filter should remain the same, independent of pagination.
	countAfterGet, err := db.CountConnectionLogs(ctx, countParams)
	require.NoError(t, err)
	require.Equal(t, int64(20), countAfterGet)
}

func TestConnectionLogsOffsetFilters(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	orgA := dbfake.Organization(t, db).Do()
	orgB := dbfake.Organization(t, db).Do()

	user1 := dbgen.User(t, db, database.User{
		Username: "user1",
		Email:    "user1@test.com",
	})
	user2 := dbgen.User(t, db, database.User{
		Username: "user2",
		Email:    "user2@test.com",
	})
	user3 := dbgen.User(t, db, database.User{
		Username: "user3",
		Email:    "user3@test.com",
	})

	ws1Tpl := dbgen.Template(t, db, database.Template{OrganizationID: orgA.Org.ID, CreatedBy: user1.ID})
	ws1 := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user1.ID,
		OrganizationID: orgA.Org.ID,
		TemplateID:     ws1Tpl.ID,
	})
	ws2Tpl := dbgen.Template(t, db, database.Template{OrganizationID: orgB.Org.ID, CreatedBy: user2.ID})
	ws2 := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user2.ID,
		OrganizationID: orgB.Org.ID,
		TemplateID:     ws2Tpl.ID,
	})

	now := dbtime.Now()
	log1ConnID := uuid.New()
	log1 := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-4 * time.Hour),
		OrganizationID:   ws1.OrganizationID,
		WorkspaceOwnerID: ws1.OwnerID,
		WorkspaceID:      ws1.ID,
		WorkspaceName:    ws1.Name,
		Type:             database.ConnectionTypeWorkspaceApp,
		ConnectionStatus: database.ConnectionStatusConnected,
		UserID:           uuid.NullUUID{UUID: user1.ID, Valid: true},
		UserAgent:        sql.NullString{String: "Mozilla/5.0", Valid: true},
		SlugOrPort:       sql.NullString{String: "code-server", Valid: true},
		ConnectionID:     uuid.NullUUID{UUID: log1ConnID, Valid: true},
	})

	log2ConnID := uuid.New()
	log2 := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-3 * time.Hour),
		OrganizationID:   ws1.OrganizationID,
		WorkspaceOwnerID: ws1.OwnerID,
		WorkspaceID:      ws1.ID,
		WorkspaceName:    ws1.Name,
		Type:             database.ConnectionTypeVscode,
		ConnectionStatus: database.ConnectionStatusConnected,
		ConnectionID:     uuid.NullUUID{UUID: log2ConnID, Valid: true},
	})

	// Mark log2 as disconnected
	log2 = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-2 * time.Hour),
		ConnectionID:     log2.ConnectionID,
		WorkspaceID:      ws1.ID,
		WorkspaceOwnerID: ws1.OwnerID,
		AgentName:        log2.AgentName,
		ConnectionStatus: database.ConnectionStatusDisconnected,

		OrganizationID: log2.OrganizationID,
	})

	log3ConnID := uuid.New()
	log3 := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-2 * time.Hour),
		OrganizationID:   ws2.OrganizationID,
		WorkspaceOwnerID: ws2.OwnerID,
		WorkspaceID:      ws2.ID,
		WorkspaceName:    ws2.Name,
		Type:             database.ConnectionTypeSsh,
		ConnectionStatus: database.ConnectionStatusConnected,
		UserID:           uuid.NullUUID{UUID: user2.ID, Valid: true},
		ConnectionID:     uuid.NullUUID{UUID: log3ConnID, Valid: true},
	})

	// Mark log3 as disconnected
	log3 = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-1 * time.Hour),
		ConnectionID:     log3.ConnectionID,
		WorkspaceOwnerID: log3.WorkspaceOwnerID,
		WorkspaceID:      ws2.ID,
		AgentName:        log3.AgentName,
		ConnectionStatus: database.ConnectionStatusDisconnected,

		OrganizationID: log3.OrganizationID,
	})

	log4 := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
		Time:             now.Add(-1 * time.Hour),
		OrganizationID:   ws2.OrganizationID,
		WorkspaceOwnerID: ws2.OwnerID,
		WorkspaceID:      ws2.ID,
		WorkspaceName:    ws2.Name,
		Type:             database.ConnectionTypeVscode,
		ConnectionStatus: database.ConnectionStatusConnected,
		UserID:           uuid.NullUUID{UUID: user3.ID, Valid: true},
	})

	testCases := []struct {
		name           string
		params         database.GetConnectionLogsOffsetParams
		expectedLogIDs []uuid.UUID
	}{
		{
			name:   "NoFilter",
			params: database.GetConnectionLogsOffsetParams{},
			expectedLogIDs: []uuid.UUID{
				log1.ID, log2.ID, log3.ID, log4.ID,
			},
		},
		{
			name: "OrganizationID",
			params: database.GetConnectionLogsOffsetParams{
				OrganizationID: orgB.Org.ID,
			},
			expectedLogIDs: []uuid.UUID{log3.ID, log4.ID},
		},
		{
			name: "WorkspaceOwner",
			params: database.GetConnectionLogsOffsetParams{
				WorkspaceOwner: user1.Username,
			},
			expectedLogIDs: []uuid.UUID{log1.ID, log2.ID},
		},
		{
			name: "WorkspaceOwnerID",
			params: database.GetConnectionLogsOffsetParams{
				WorkspaceOwnerID: user1.ID,
			},
			expectedLogIDs: []uuid.UUID{log1.ID, log2.ID},
		},
		{
			name: "WorkspaceOwnerEmail",
			params: database.GetConnectionLogsOffsetParams{
				WorkspaceOwnerEmail: user2.Email,
			},
			expectedLogIDs: []uuid.UUID{log3.ID, log4.ID},
		},
		{
			name: "Type",
			params: database.GetConnectionLogsOffsetParams{
				Type: string(database.ConnectionTypeVscode),
			},
			expectedLogIDs: []uuid.UUID{log2.ID, log4.ID},
		},
		{
			name: "UserID",
			params: database.GetConnectionLogsOffsetParams{
				UserID: user1.ID,
			},
			expectedLogIDs: []uuid.UUID{log1.ID},
		},
		{
			name: "Username",
			params: database.GetConnectionLogsOffsetParams{
				Username: user1.Username,
			},
			expectedLogIDs: []uuid.UUID{log1.ID},
		},
		{
			name: "UserEmail",
			params: database.GetConnectionLogsOffsetParams{
				UserEmail: user3.Email,
			},
			expectedLogIDs: []uuid.UUID{log4.ID},
		},
		{
			name: "ConnectedAfter",
			params: database.GetConnectionLogsOffsetParams{
				ConnectedAfter: now.Add(-90 * time.Minute), // 1.5 hours ago
			},
			expectedLogIDs: []uuid.UUID{log4.ID},
		},
		{
			name: "ConnectedBefore",
			params: database.GetConnectionLogsOffsetParams{
				ConnectedBefore: now.Add(-150 * time.Minute),
			},
			expectedLogIDs: []uuid.UUID{log1.ID, log2.ID},
		},
		{
			name: "WorkspaceID",
			params: database.GetConnectionLogsOffsetParams{
				WorkspaceID: ws2.ID,
			},
			expectedLogIDs: []uuid.UUID{log3.ID, log4.ID},
		},
		{
			name: "ConnectionID",
			params: database.GetConnectionLogsOffsetParams{
				ConnectionID: log1.ConnectionID.UUID,
			},
			expectedLogIDs: []uuid.UUID{log1.ID},
		},
		{
			name: "StatusOngoing",
			params: database.GetConnectionLogsOffsetParams{
				Status: string(codersdk.ConnectionLogStatusOngoing),
			},
			expectedLogIDs: []uuid.UUID{log4.ID},
		},
		{
			name: "StatusCompleted",
			params: database.GetConnectionLogsOffsetParams{
				Status: string(codersdk.ConnectionLogStatusCompleted),
			},
			expectedLogIDs: []uuid.UUID{log2.ID, log3.ID},
		},
		{
			name: "OrganizationAndTypeAndStatus",
			params: database.GetConnectionLogsOffsetParams{
				OrganizationID: orgA.Org.ID,
				Type:           string(database.ConnectionTypeVscode),
				Status:         string(codersdk.ConnectionLogStatusCompleted),
			},
			expectedLogIDs: []uuid.UUID{log2.ID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logs, err := db.GetConnectionLogsOffset(ctx, tc.params)
			require.NoError(t, err)
			count, err := db.CountConnectionLogs(ctx, database.CountConnectionLogsParams{
				OrganizationID:      tc.params.OrganizationID,
				WorkspaceOwner:      tc.params.WorkspaceOwner,
				Type:                tc.params.Type,
				UserID:              tc.params.UserID,
				Username:            tc.params.Username,
				UserEmail:           tc.params.UserEmail,
				ConnectedAfter:      tc.params.ConnectedAfter,
				ConnectedBefore:     tc.params.ConnectedBefore,
				WorkspaceID:         tc.params.WorkspaceID,
				ConnectionID:        tc.params.ConnectionID,
				Status:              tc.params.Status,
				WorkspaceOwnerID:    tc.params.WorkspaceOwnerID,
				WorkspaceOwnerEmail: tc.params.WorkspaceOwnerEmail,
			})
			require.NoError(t, err)
			require.ElementsMatch(t, tc.expectedLogIDs, connectionOnlyIDs(logs))
			require.Equal(t, len(tc.expectedLogIDs), int(count), "CountConnectionLogs should match the number of returned logs (no offset or limit)")
		})
	}
}

func connectionOnlyIDs[T database.ConnectionLog | database.GetConnectionLogsOffsetRow](logs []T) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(logs))
	for _, log := range logs {
		switch log := any(log).(type) {
		case database.ConnectionLog:
			ids = append(ids, log.ID)
		case database.GetConnectionLogsOffsetRow:
			ids = append(ids, log.ConnectionLog.ID)
		default:
			panic("unreachable")
		}
	}
	return ids
}

func TestUpsertConnectionLog(t *testing.T) {
	t.Parallel()
	createWorkspace := func(t *testing.T, db database.Store) database.WorkspaceTable {
		u := dbgen.User(t, db, database.User{})
		o := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		return dbgen.Workspace(t, db, database.WorkspaceTable{
			ID:               uuid.New(),
			OwnerID:          u.ID,
			OrganizationID:   o.ID,
			AutomaticUpdates: database.AutomaticUpdatesNever,
			TemplateID:       tpl.ID,
		})
	}

	t.Run("ConnectThenDisconnect", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		ws := createWorkspace(t, db)

		connectionID := uuid.New()
		agentName := "test-agent"

		// 1. Insert a 'connect' event.
		connectTime := dbtime.Now()
		connectParams := database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             connectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        agentName,
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			Ip: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
				Valid: true,
			},
		}

		log1, err := db.UpsertConnectionLog(ctx, connectParams)
		require.NoError(t, err)
		require.Equal(t, connectParams.ID, log1.ID)
		require.False(t, log1.DisconnectTime.Valid, "DisconnectTime should not be set on connect")

		// Check that one row exists.
		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)

		// 2. Insert a 'disconnected' event for the same connection.
		disconnectTime := connectTime.Add(time.Second)
		disconnectParams := database.UpsertConnectionLogParams{
			ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
			WorkspaceID:      ws.ID,
			AgentName:        agentName,
			ConnectionStatus: database.ConnectionStatusDisconnected,

			// Updated to:
			Time:             disconnectTime,
			DisconnectReason: sql.NullString{String: "test disconnect", Valid: true},
			Code:             sql.NullInt32{Int32: 1, Valid: true},

			// Ignored
			ID:               uuid.New(),
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceName:    ws.Name,
			Type:             database.ConnectionTypeSsh,
			Ip: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 254),
				},
				Valid: true,
			},
		}

		log2, err := db.UpsertConnectionLog(ctx, disconnectParams)
		require.NoError(t, err)

		// Updated
		require.Equal(t, log1.ID, log2.ID)
		require.True(t, log2.DisconnectTime.Valid)
		require.True(t, disconnectTime.Equal(log2.DisconnectTime.Time))
		require.Equal(t, disconnectParams.DisconnectReason.String, log2.DisconnectReason.String)

		rows, err = db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
	})

	t.Run("ConnectDoesNotUpdate", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		ws := createWorkspace(t, db)

		connectionID := uuid.New()
		agentName := "test-agent"

		// 1. Insert a 'connect' event.
		connectTime := dbtime.Now()
		connectParams := database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             connectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        agentName,
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			Ip: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
				Valid: true,
			},
		}

		log, err := db.UpsertConnectionLog(ctx, connectParams)
		require.NoError(t, err)

		// 2. Insert another 'connect' event for the same connection.
		connectTime2 := connectTime.Add(time.Second)
		connectParams2 := database.UpsertConnectionLogParams{
			ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
			WorkspaceID:      ws.ID,
			AgentName:        agentName,
			ConnectionStatus: database.ConnectionStatusConnected,

			// Ignored
			ID:               uuid.New(),
			Time:             connectTime2,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceName:    ws.Name,
			Type:             database.ConnectionTypeSsh,
			Code:             sql.NullInt32{Int32: 0, Valid: false},
			Ip: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 254),
				},
				Valid: true,
			},
		}

		origLog, err := db.UpsertConnectionLog(ctx, connectParams2)
		require.NoError(t, err)
		require.Equal(t, log, origLog, "connect update should be a no-op")

		// Check that still only one row exists.
		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, log, rows[0].ConnectionLog)
	})

	t.Run("DisconnectThenConnect", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		ws := createWorkspace(t, db)

		connectionID := uuid.New()
		agentName := "test-agent"

		// Insert just a 'disconect' event
		disconnectTime := dbtime.Now()
		disconnectParams := database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             disconnectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        agentName,
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
			ConnectionStatus: database.ConnectionStatusDisconnected,
			DisconnectReason: sql.NullString{String: "server shutting down", Valid: true},
			Ip: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
				Valid: true,
			},
		}

		_, err := db.UpsertConnectionLog(ctx, disconnectParams)
		require.NoError(t, err)

		firstRows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		require.Len(t, firstRows, 1)

		// We expect the connection event to be marked as closed with the start
		// and close time being the same.
		require.True(t, firstRows[0].ConnectionLog.DisconnectTime.Valid)
		require.Equal(t, disconnectTime, firstRows[0].ConnectionLog.DisconnectTime.Time.UTC())
		require.Equal(t, firstRows[0].ConnectionLog.ConnectTime.UTC(), firstRows[0].ConnectionLog.DisconnectTime.Time.UTC())

		// Now insert a 'connect' event for the same connection.
		// This should be a no op
		connectTime := disconnectTime.Add(time.Second)
		connectParams := database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             connectTime,
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        agentName,
			Type:             database.ConnectionTypeSsh,
			ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
			ConnectionStatus: database.ConnectionStatusConnected,
			DisconnectReason: sql.NullString{String: "reconnected", Valid: true},
			Code:             sql.NullInt32{Int32: 0, Valid: false},
			Ip: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
				Valid: true,
			},
		}

		_, err = db.UpsertConnectionLog(ctx, connectParams)
		require.NoError(t, err)

		secondRows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		require.Len(t, secondRows, 1)
		require.Equal(t, firstRows, secondRows)

		// Upsert a disconnection, which should also be a no op
		disconnectParams.DisconnectReason = sql.NullString{
			String: "updated close reason",
			Valid:  true,
		}
		_, err = db.UpsertConnectionLog(ctx, disconnectParams)
		require.NoError(t, err)
		thirdRows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{})
		require.NoError(t, err)
		require.Len(t, secondRows, 1)
		// The close reason shouldn't be updated
		require.Equal(t, secondRows, thirdRows)
	})
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
				// #nosec G115 - Safe conversion as build number is expected to be within int32 range
				BuildNumber: int32(i) + 2,
				Transition:  trans,
				InitiatorID: tpl.CreatedBy,
				JobID:       latestJob.ID,
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
			actualJobs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{
				IDs:             filteredJobIDs,
				StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
			})
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
	actualJobs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{
		IDs:             jobIDs,
		StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
	})
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
	actualJobs, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx, database.GetProvisionerJobsByIDsWithQueuePositionParams{
		IDs:             jobIDs,
		StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
	})
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

	timezones := []string{
		"America/St_Johns",
		"Africa/Johannesburg",
		"America/New_York",
		"Europe/London",
		"Asia/Tokyo",
		"Australia/Sydney",
	}

	for _, tz := range timezones {
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
							switch {
							case row.Date.Before(createdAt):
								require.Equal(t, int64(0), row.Count)
							case row.Date.Before(firstTransitionTime):
								if row.Status == tc.initialStatus {
									require.Equal(t, int64(1), row.Count)
								} else if row.Status == tc.targetStatus {
									require.Equal(t, int64(0), row.Count)
								}
							case !row.Date.After(today):
								if row.Status == tc.initialStatus {
									require.Equal(t, int64(0), row.Count)
								} else if row.Status == tc.targetStatus {
									require.Equal(t, int64(1), row.Count)
								}
							default:
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
							switch {
							case d.Before(createdAt):
								continue
							case d.Before(firstTransitionTime):
								expectedCounts[d][tc.user1Transition.from]++
								expectedCounts[d][tc.user2Transition.from]++
							case d.Before(secondTransitionTime):
								expectedCounts[d][tc.user1Transition.to]++
								expectedCounts[d][tc.user2Transition.from]++
							case d.Before(today):
								expectedCounts[d][tc.user1Transition.to]++
								expectedCounts[d][tc.user2Transition.to]++
							default:
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
					switch {
					case row.Date.Before(createdAt):
						require.Equal(t, int64(0), row.Count)
					case i == len(userStatusChanges)-1:
						require.Equal(t, int64(0), row.Count)
					default:
						require.Equal(t, int64(1), row.Count)
					}
				}
			})
		})
	}
}

func TestOrganizationDeleteTrigger(t *testing.T) {
	t.Parallel()

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

	t.Run("UserDeletedButNotRemovedFromOrg", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		orgA := dbfake.Organization(t, db).Do()

		userA := dbgen.User(t, db, database.User{})
		userB := dbgen.User(t, db, database.User{})
		userC := dbgen.User(t, db, database.User{})

		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: orgA.Org.ID,
			UserID:         userA.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: orgA.Org.ID,
			UserID:         userB.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: orgA.Org.ID,
			UserID:         userC.ID,
		})

		// Delete one of the users but don't remove them from the org
		ctx := testutil.Context(t, testutil.WaitShort)
		db.UpdateUserDeletedByID(ctx, userB.ID)

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

type templateVersionWithPreset struct {
	database.TemplateVersion
	preset database.TemplateVersionPreset
}

func createTemplate(t *testing.T, db database.Store, orgID uuid.UUID, userID uuid.UUID) database.Template {
	// create template
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  orgID,
		CreatedBy:       userID,
		ActiveVersionID: uuid.New(),
	})

	return tmpl
}

type tmplVersionOpts struct {
	DesiredInstances int32
}

func createTmplVersionAndPreset(
	t *testing.T,
	db database.Store,
	tmpl database.Template,
	versionID uuid.UUID,
	now time.Time,
	opts *tmplVersionOpts,
) templateVersionWithPreset {
	// Create template version with corresponding preset and preset prebuild
	tmplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		ID: versionID,
		TemplateID: uuid.NullUUID{
			UUID:  tmpl.ID,
			Valid: true,
		},
		OrganizationID: tmpl.OrganizationID,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      tmpl.CreatedBy,
	})
	desiredInstances := int32(1)
	if opts != nil {
		desiredInstances = opts.DesiredInstances
	}
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: tmplVersion.ID,
		Name:              "preset",
		DesiredInstances: sql.NullInt32{
			Int32: desiredInstances,
			Valid: true,
		},
	})

	return templateVersionWithPreset{
		TemplateVersion: tmplVersion,
		preset:          preset,
	}
}

type createPrebuiltWorkspaceOpts struct {
	failedJob      bool
	createdAt      time.Time
	readyAgents    int
	notReadyAgents int
}

func createPrebuiltWorkspace(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	tmpl database.Template,
	extTmplVersion templateVersionWithPreset,
	orgID uuid.UUID,
	now time.Time,
	opts *createPrebuiltWorkspaceOpts,
) {
	// Create job with corresponding resource and agent
	jobError := sql.NullString{}
	if opts != nil && opts.failedJob {
		jobError = sql.NullString{String: "failed", Valid: true}
	}
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		OrganizationID: orgID,

		CreatedAt: now.Add(-1 * time.Minute),
		Error:     jobError,
	})

	// create ready agents
	readyAgents := 0
	if opts != nil {
		readyAgents = opts.readyAgents
	}
	for i := 0; i < readyAgents; i++ {
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		err := db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agent.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		})
		require.NoError(t, err)
	}

	// create not ready agents
	notReadyAgents := 1
	if opts != nil {
		notReadyAgents = opts.notReadyAgents
	}
	for i := 0; i < notReadyAgents; i++ {
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		err := db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agent.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
		})
		require.NoError(t, err)
	}

	// Create corresponding workspace and workspace build
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        uuid.MustParse("c42fdf75-3097-471c-8c33-fb52454d81c0"),
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

func TestWorkspacePrebuildsView(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	orgID := uuid.New()
	userID := uuid.New()

	type workspacePrebuild struct {
		ID              uuid.UUID
		Name            string
		CreatedAt       time.Time
		Ready           bool
		CurrentPresetID uuid.UUID
	}
	getWorkspacePrebuilds := func(sqlDB *sql.DB) []*workspacePrebuild {
		rows, err := sqlDB.Query("SELECT id, name, created_at, ready, current_preset_id FROM workspace_prebuilds")
		require.NoError(t, err)
		defer rows.Close()

		workspacePrebuilds := make([]*workspacePrebuild, 0)
		for rows.Next() {
			var wp workspacePrebuild
			err := rows.Scan(&wp.ID, &wp.Name, &wp.CreatedAt, &wp.Ready, &wp.CurrentPresetID)
			require.NoError(t, err)

			workspacePrebuilds = append(workspacePrebuilds, &wp)
		}

		return workspacePrebuilds
	}

	testCases := []struct {
		name           string
		readyAgents    int
		notReadyAgents int
		expectReady    bool
	}{
		{
			name:           "one ready agent",
			readyAgents:    1,
			notReadyAgents: 0,
			expectReady:    true,
		},
		{
			name:           "one not ready agent",
			readyAgents:    0,
			notReadyAgents: 1,
			expectReady:    false,
		},
		{
			name:           "one ready, one not ready",
			readyAgents:    1,
			notReadyAgents: 1,
			expectReady:    false,
		},
		{
			name:           "both ready",
			readyAgents:    2,
			notReadyAgents: 0,
			expectReady:    true,
		},
		{
			name:           "five ready, one not ready",
			readyAgents:    5,
			notReadyAgents: 1,
			expectReady:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sqlDB := testSQLDB(t)
			err := migrations.Up(sqlDB)
			require.NoError(t, err)
			db := database.New(sqlDB)

			ctx := testutil.Context(t, testutil.WaitShort)

			dbgen.Organization(t, db, database.Organization{
				ID: orgID,
			})
			dbgen.User(t, db, database.User{
				ID: userID,
			})

			tmpl := createTemplate(t, db, orgID, userID)
			tmplV1 := createTmplVersionAndPreset(t, db, tmpl, tmpl.ActiveVersionID, now, nil)
			createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
				readyAgents:    tc.readyAgents,
				notReadyAgents: tc.notReadyAgents,
			})

			workspacePrebuilds := getWorkspacePrebuilds(sqlDB)
			require.Len(t, workspacePrebuilds, 1)
			require.Equal(t, tc.expectReady, workspacePrebuilds[0].Ready)
		})
	}
}

func TestGetPresetsBackoff(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	orgID := uuid.New()
	userID := uuid.New()

	findBackoffByTmplVersionID := func(backoffs []database.GetPresetsBackoffRow, tmplVersionID uuid.UUID) *database.GetPresetsBackoffRow {
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

		tmpl := createTemplate(t, db, orgID, userID)
		tmplV1 := createTmplVersionAndPreset(t, db, tmpl, tmpl.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

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

		tmpl := createTemplate(t, db, orgID, userID)
		tmplV1 := createTmplVersionAndPreset(t, db, tmpl, tmpl.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

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

		tmpl := createTemplate(t, db, orgID, userID)
		tmplV1 := createTmplVersionAndPreset(t, db, tmpl, uuid.New(), now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		// Active Version
		tmplV2 := createTmplVersionAndPreset(t, db, tmpl, tmpl.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl2 := createTemplate(t, db, orgID, userID)
		tmpl2V1 := createTmplVersionAndPreset(t, db, tmpl2, tmpl2.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl2, tmpl2V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 2)
		{
			backoff := findBackoffByTmplVersionID(backoffs, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(1), backoff.NumFailed)
		}
		{
			backoff := findBackoffByTmplVersionID(backoffs, tmpl2.ActiveVersionID)
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl2 := createTemplate(t, db, orgID, userID)
		tmpl2V1 := createTmplVersionAndPreset(t, db, tmpl2, tmpl2.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl2, tmpl2V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl2, tmpl2V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl3 := createTemplate(t, db, orgID, userID)
		tmpl3V1 := createTmplVersionAndPreset(t, db, tmpl3, uuid.New(), now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl3V2 := createTmplVersionAndPreset(t, db, tmpl3, tmpl3.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-time.Hour))
		require.NoError(t, err)

		require.Len(t, backoffs, 3)
		{
			backoff := findBackoffByTmplVersionID(backoffs, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl1V1.preset.ID)
			require.Equal(t, int32(1), backoff.NumFailed)
		}
		{
			backoff := findBackoffByTmplVersionID(backoffs, tmpl2.ActiveVersionID)
			require.Equal(t, backoff.TemplateVersionID, tmpl2.ActiveVersionID)
			require.Equal(t, backoff.PresetID, tmpl2V1.preset.ID)
			require.Equal(t, int32(2), backoff.NumFailed)
		}
		{
			backoff := findBackoffByTmplVersionID(backoffs, tmpl3.ActiveVersionID)
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)

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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)
		successfulJobOpts := createPrebuiltWorkspaceOpts{}
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)

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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, &tmplVersionOpts{
			DesiredInstances: 1,
		})
		failedJobOpts := createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-2 * time.Minute),
		}
		successfulJobOpts := createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-1 * time.Minute),
		}
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &failedJobOpts)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)

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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, &tmplVersionOpts{
			DesiredInstances: 3,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-4 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-3 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-2 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-1 * time.Minute),
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, &tmplVersionOpts{
			DesiredInstances: 3,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-3 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-2 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-1 * time.Minute),
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, &tmplVersionOpts{
			DesiredInstances: 3,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-lookbackPeriod - time.Minute), // earlier than lookback period - skipped
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-4 * time.Minute), // within lookback period - counted as failed job
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-3 * time.Minute), // within lookback period - counted as failed job
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-2 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: false,
			createdAt: now.Add(-1 * time.Minute),
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, &tmplVersionOpts{
			DesiredInstances: 6,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-lookbackPeriod - time.Minute), // earlier than lookback period - skipped
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-4 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-0 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-3 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-1 * time.Minute),
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-2 * time.Minute),
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
			require.Equal(t, 0, now.Compare(backoff.LastBuildAt))
		}
	})

	t.Run("failed job outside lookback period", func(t *testing.T) {
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, &tmplVersionOpts{
			DesiredInstances: 1,
		})

		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
			createdAt: now.Add(-lookbackPeriod - time.Minute), // earlier than lookback period - skipped
		})

		backoffs, err := db.GetPresetsBackoff(ctx, now.Add(-lookbackPeriod))
		require.NoError(t, err)
		require.Len(t, backoffs, 0)
	})
}

func TestGetPresetsAtFailureLimit(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	hourBefore := now.Add(-time.Hour)
	orgID := uuid.New()
	userID := uuid.New()

	findPresetByTmplVersionID := func(hardLimitedPresets []database.GetPresetsAtFailureLimitRow, tmplVersionID uuid.UUID) *database.GetPresetsAtFailureLimitRow {
		for _, preset := range hardLimitedPresets {
			if preset.TemplateVersionID == tmplVersionID {
				return &preset
			}
		}

		return nil
	}

	testCases := []struct {
		name string
		// true - build is successful
		// false - build is unsuccessful
		buildSuccesses  []bool
		hardLimit       int64
		expHitHardLimit bool
	}{
		{
			name:            "failed build",
			buildSuccesses:  []bool{false},
			hardLimit:       1,
			expHitHardLimit: true,
		},
		{
			name:            "2 failed builds",
			buildSuccesses:  []bool{false, false},
			hardLimit:       1,
			expHitHardLimit: true,
		},
		{
			name:            "successful build",
			buildSuccesses:  []bool{true},
			hardLimit:       1,
			expHitHardLimit: false,
		},
		{
			name:            "last build is failed",
			buildSuccesses:  []bool{true, true, false},
			hardLimit:       1,
			expHitHardLimit: true,
		},
		{
			name:            "last build is successful",
			buildSuccesses:  []bool{false, false, true},
			hardLimit:       1,
			expHitHardLimit: false,
		},
		{
			name:            "last 3 builds are failed - hard limit is reached",
			buildSuccesses:  []bool{true, true, false, false, false},
			hardLimit:       3,
			expHitHardLimit: true,
		},
		{
			name:            "1 out of 3 last build is successful - hard limit is NOT reached",
			buildSuccesses:  []bool{false, false, true, false, false},
			hardLimit:       3,
			expHitHardLimit: false,
		},
		// hardLimit set to zero, implicitly disables the hard limit.
		{
			name:            "despite 5 failed builds, the hard limit is not reached because it's disabled.",
			buildSuccesses:  []bool{false, false, false, false, false},
			hardLimit:       0,
			expHitHardLimit: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			dbgen.Organization(t, db, database.Organization{
				ID: orgID,
			})
			dbgen.User(t, db, database.User{
				ID: userID,
			})

			tmpl := createTemplate(t, db, orgID, userID)
			tmplV1 := createTmplVersionAndPreset(t, db, tmpl, tmpl.ActiveVersionID, now, nil)
			for idx, buildSuccess := range tc.buildSuccesses {
				createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
					failedJob: !buildSuccess,
					createdAt: hourBefore.Add(time.Duration(idx) * time.Second),
				})
			}

			hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, tc.hardLimit)
			require.NoError(t, err)

			if !tc.expHitHardLimit {
				require.Len(t, hardLimitedPresets, 0)
				return
			}

			require.Len(t, hardLimitedPresets, 1)
			hardLimitedPreset := hardLimitedPresets[0]
			require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.PresetID, tmplV1.preset.ID)
		})
	}

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

		tmpl := createTemplate(t, db, orgID, userID)
		tmplV1 := createTmplVersionAndPreset(t, db, tmpl, uuid.New(), now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		// Active Version
		tmplV2 := createTmplVersionAndPreset(t, db, tmpl, tmpl.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl, tmplV2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, 1)
		require.NoError(t, err)

		require.Len(t, hardLimitedPresets, 1)
		hardLimitedPreset := hardLimitedPresets[0]
		require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl.ActiveVersionID)
		require.Equal(t, hardLimitedPreset.PresetID, tmplV2.preset.ID)
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl2 := createTemplate(t, db, orgID, userID)
		tmpl2V1 := createTmplVersionAndPreset(t, db, tmpl2, tmpl2.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl2, tmpl2V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, 1)

		require.NoError(t, err)

		require.Len(t, hardLimitedPresets, 2)
		{
			hardLimitedPreset := findPresetByTmplVersionID(hardLimitedPresets, tmpl1.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.PresetID, tmpl1V1.preset.ID)
		}
		{
			hardLimitedPreset := findPresetByTmplVersionID(hardLimitedPresets, tmpl2.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl2.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.PresetID, tmpl2V1.preset.ID)
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl2 := createTemplate(t, db, orgID, userID)
		tmpl2V1 := createTmplVersionAndPreset(t, db, tmpl2, tmpl2.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl2, tmpl2V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl2, tmpl2V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl3 := createTemplate(t, db, orgID, userID)
		tmpl3V1 := createTmplVersionAndPreset(t, db, tmpl3, uuid.New(), now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V1, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		tmpl3V2 := createTmplVersionAndPreset(t, db, tmpl3, tmpl3.ActiveVersionID, now, nil)
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})
		createPrebuiltWorkspace(ctx, t, db, tmpl3, tmpl3V2, orgID, now, &createPrebuiltWorkspaceOpts{
			failedJob: true,
		})

		hardLimit := int64(2)
		hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, hardLimit)
		require.NoError(t, err)

		require.Len(t, hardLimitedPresets, 3)
		{
			hardLimitedPreset := findPresetByTmplVersionID(hardLimitedPresets, tmpl1.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl1.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.PresetID, tmpl1V1.preset.ID)
		}
		{
			hardLimitedPreset := findPresetByTmplVersionID(hardLimitedPresets, tmpl2.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl2.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.PresetID, tmpl2V1.preset.ID)
		}
		{
			hardLimitedPreset := findPresetByTmplVersionID(hardLimitedPresets, tmpl3.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.TemplateVersionID, tmpl3.ActiveVersionID)
			require.Equal(t, hardLimitedPreset.PresetID, tmpl3V2.preset.ID)
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)

		hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, 1)
		require.NoError(t, err)
		require.Nil(t, hardLimitedPresets)
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

		tmpl1 := createTemplate(t, db, orgID, userID)
		tmpl1V1 := createTmplVersionAndPreset(t, db, tmpl1, tmpl1.ActiveVersionID, now, nil)
		successfulJobOpts := createPrebuiltWorkspaceOpts{}
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)
		createPrebuiltWorkspace(ctx, t, db, tmpl1, tmpl1V1, orgID, now, &successfulJobOpts)

		hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, 1)
		require.NoError(t, err)
		require.Nil(t, hardLimitedPresets)
	})
}

func TestWorkspaceAgentNameUniqueTrigger(t *testing.T) {
	t.Parallel()

	createWorkspaceWithAgent := func(t *testing.T, db database.Store, org database.Organization, agentName string) (database.WorkspaceBuild, database.WorkspaceResource, database.WorkspaceAgent) {
		t.Helper()

		user := dbgen.User(t, db, database.User{})
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			TemplateID:     template.ID,
			OwnerID:        user.ID,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			BuildNumber:       1,
			JobID:             job.ID,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
			Name:       agentName,
		})

		return build, resource, agent
	}

	t.Run("DuplicateNamesInSameWorkspaceResource", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A workspace with an agent
		_, resource, _ := createWorkspaceWithAgent(t, db, org, "duplicate-agent")

		// When: Another agent is created for that workspace with the same name.
		_, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:              uuid.New(),
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			Name:            "duplicate-agent", // Same name as agent1
			ResourceID:      resource.ID,
			AuthToken:       uuid.New(),
			Architecture:    "amd64",
			OperatingSystem: "linux",
			APIKeyScope:     database.AgentKeyScopeEnumAll,
		})

		// Then: We expect it to fail.
		require.Error(t, err)
		var pqErr *pq.Error
		require.True(t, errors.As(err, &pqErr))
		require.Equal(t, pq.ErrorCode("23505"), pqErr.Code) // unique_violation
		require.Contains(t, pqErr.Message, `workspace agent name "duplicate-agent" already exists in this workspace build`)
	})

	t.Run("DuplicateNamesInSameProvisionerJob", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A workspace with an agent
		_, resource, agent := createWorkspaceWithAgent(t, db, org, "duplicate-agent")

		// When: A child agent is created for that workspace with the same name.
		_, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:              uuid.New(),
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			Name:            agent.Name,
			ResourceID:      resource.ID,
			AuthToken:       uuid.New(),
			Architecture:    "amd64",
			OperatingSystem: "linux",
			APIKeyScope:     database.AgentKeyScopeEnumAll,
		})

		// Then: We expect it to fail.
		require.Error(t, err)
		var pqErr *pq.Error
		require.True(t, errors.As(err, &pqErr))
		require.Equal(t, pq.ErrorCode("23505"), pqErr.Code) // unique_violation
		require.Contains(t, pqErr.Message, `workspace agent name "duplicate-agent" already exists in this workspace build`)
	})

	t.Run("DuplicateChildNamesOverMultipleResources", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A workspace with two agents
		_, resource1, agent1 := createWorkspaceWithAgent(t, db, org, "parent-agent-1")

		resource2 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: resource1.JobID})
		agent2 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource2.ID,
			Name:       "parent-agent-2",
		})

		// Given: One agent has a child agent
		agent1Child := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ParentID:   uuid.NullUUID{Valid: true, UUID: agent1.ID},
			Name:       "child-agent",
			ResourceID: resource1.ID,
		})

		// When: A child agent is inserted for the other parent.
		_, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:              uuid.New(),
			ParentID:        uuid.NullUUID{Valid: true, UUID: agent2.ID},
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			Name:            agent1Child.Name,
			ResourceID:      resource2.ID,
			AuthToken:       uuid.New(),
			Architecture:    "amd64",
			OperatingSystem: "linux",
			APIKeyScope:     database.AgentKeyScopeEnumAll,
		})

		// Then: We expect it to fail.
		require.Error(t, err)
		var pqErr *pq.Error
		require.True(t, errors.As(err, &pqErr))
		require.Equal(t, pq.ErrorCode("23505"), pqErr.Code) // unique_violation
		require.Contains(t, pqErr.Message, `workspace agent name "child-agent" already exists in this workspace build`)
	})

	t.Run("SameNamesInDifferentWorkspaces", func(t *testing.T) {
		t.Parallel()

		agentName := "same-name-different-workspace"

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		// Given: A workspace with an agent
		_, _, agent1 := createWorkspaceWithAgent(t, db, org, agentName)
		require.Equal(t, agentName, agent1.Name)

		// When: A second workspace is created with an agent having the same name
		_, _, agent2 := createWorkspaceWithAgent(t, db, org, agentName)
		require.Equal(t, agentName, agent2.Name)

		// Then: We expect there to be different agents with the same name.
		require.NotEqual(t, agent1.ID, agent2.ID)
		require.Equal(t, agent1.Name, agent2.Name)
	})

	t.Run("NullWorkspaceID", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: A resource that does not belong to a workspace build (simulating template import)
		orphanJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: org.ID,
		})
		orphanResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: orphanJob.ID,
		})

		// And this resource has a workspace agent.
		agent1, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:              uuid.New(),
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			Name:            "orphan-agent",
			ResourceID:      orphanResource.ID,
			AuthToken:       uuid.New(),
			Architecture:    "amd64",
			OperatingSystem: "linux",
			APIKeyScope:     database.AgentKeyScopeEnumAll,
		})
		require.NoError(t, err)
		require.Equal(t, "orphan-agent", agent1.Name)

		// When: We created another resource that does not belong to a workspace build.
		orphanJob2 := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: org.ID,
		})
		orphanResource2 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: orphanJob2.ID,
		})

		// Then: We expect to be able to create an agent in this new resource that has the same name.
		agent2, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:              uuid.New(),
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			Name:            "orphan-agent", // Same name as agent1
			ResourceID:      orphanResource2.ID,
			AuthToken:       uuid.New(),
			Architecture:    "amd64",
			OperatingSystem: "linux",
			APIKeyScope:     database.AgentKeyScopeEnumAll,
		})
		require.NoError(t, err)
		require.Equal(t, "orphan-agent", agent2.Name)
		require.NotEqual(t, agent1.ID, agent2.ID)
	})
}

func TestGetWorkspaceAgentsByParentID(t *testing.T) {
	t.Parallel()

	t.Run("NilParentDoesNotReturnAllParentAgents", func(t *testing.T) {
		t.Parallel()

		// Given: A workspace agent
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			OrganizationID: org.ID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})

		ctx := testutil.Context(t, testutil.WaitShort)

		// When: We attempt to select agents with a null parent id
		agents, err := db.GetWorkspaceAgentsByParentID(ctx, uuid.Nil)
		require.NoError(t, err)

		// Then: We expect to see no agents.
		require.Len(t, agents, 0)
	})
}

func requireUsersMatch(t testing.TB, expected []database.User, found []database.GetUsersRow, msg string) {
	t.Helper()
	require.ElementsMatch(t, expected, database.ConvertUserRows(found), msg)
}

// TestGetRunningPrebuiltWorkspaces ensures the correct behavior of the
// GetRunningPrebuiltWorkspaces query.
func TestGetRunningPrebuiltWorkspaces(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	now := dbtime.Now()

	// Given: a prebuilt workspace with a successful start build and a stop build.
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: templateVersion.ID,
		DesiredInstances:  sql.NullInt32{Int32: 1, Valid: true},
	})

	setupFixture := func(t *testing.T, db database.Store, name string, deleted bool, transition database.WorkspaceTransition, jobStatus database.ProvisionerJobStatus) database.WorkspaceTable {
		t.Helper()
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:    database.PrebuildsSystemUserID,
			TemplateID: template.ID,
			Name:       name,
			Deleted:    deleted,
		})
		var canceledAt sql.NullTime
		var jobError sql.NullString
		switch jobStatus {
		case database.ProvisionerJobStatusFailed:
			jobError = sql.NullString{String: assert.AnError.Error(), Valid: true}
		case database.ProvisionerJobStatusCanceled:
			canceledAt = sql.NullTime{Time: now, Valid: true}
		}
		pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			InitiatorID:    database.PrebuildsSystemUserID,
			Provisioner:    database.ProvisionerTypeEcho,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StartedAt:      sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
			CanceledAt:     canceledAt,
			CompletedAt:    sql.NullTime{Time: now, Valid: true},
			Error:          jobError,
		})
		wb := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:             ws.ID,
			TemplateVersionID:       templateVersion.ID,
			TemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
			JobID:                   pj.ID,
			BuildNumber:             1,
			Transition:              transition,
			InitiatorID:             database.PrebuildsSystemUserID,
			Reason:                  database.BuildReasonInitiator,
		})
		// Ensure things are set up as expectd
		require.Equal(t, transition, wb.Transition)
		require.Equal(t, int32(1), wb.BuildNumber)
		require.Equal(t, jobStatus, pj.JobStatus)
		require.Equal(t, deleted, ws.Deleted)

		return ws
	}

	// Given: a number of prebuild workspaces with different states exist.
	runningPrebuild := setupFixture(t, db, "running-prebuild", false, database.WorkspaceTransitionStart, database.ProvisionerJobStatusSucceeded)
	_ = setupFixture(t, db, "stopped-prebuild", false, database.WorkspaceTransitionStop, database.ProvisionerJobStatusSucceeded)
	_ = setupFixture(t, db, "failed-prebuild", false, database.WorkspaceTransitionStart, database.ProvisionerJobStatusFailed)
	_ = setupFixture(t, db, "canceled-prebuild", false, database.WorkspaceTransitionStart, database.ProvisionerJobStatusCanceled)
	_ = setupFixture(t, db, "deleted-prebuild", true, database.WorkspaceTransitionStart, database.ProvisionerJobStatusSucceeded)

	// Given: a regular workspace also exists.
	_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:    user.ID,
		TemplateID: template.ID,
		Name:       "test-running-regular-workspace",
		Deleted:    false,
	})

	// When: we query for running prebuild workspaces
	runningPrebuilds, err := db.GetRunningPrebuiltWorkspaces(ctx)
	require.NoError(t, err)

	// Then: only the running prebuild workspace should be returned.
	require.Len(t, runningPrebuilds, 1, "expected only one running prebuilt workspace")
	require.Equal(t, runningPrebuild.ID, runningPrebuilds[0].ID, "expected the running prebuilt workspace to be returned")
}

func TestUserSecretsCRUDOperations(t *testing.T) {
	t.Parallel()

	// Use raw database without dbauthz wrapper for this test
	db, _ := dbtestutil.NewDB(t)

	t.Run("FullCRUDWorkflow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Create a new user for this test
		testUser := dbgen.User(t, db, database.User{})

		// 1. CREATE
		secretID := uuid.New()
		createParams := database.CreateUserSecretParams{
			ID:          secretID,
			UserID:      testUser.ID,
			Name:        "workflow-secret",
			Description: "Secret for full CRUD workflow",
			Value:       "workflow-value",
			EnvName:     "WORKFLOW_ENV",
			FilePath:    "/workflow/path",
		}

		createdSecret, err := db.CreateUserSecret(ctx, createParams)
		require.NoError(t, err)
		assert.Equal(t, secretID, createdSecret.ID)

		// 2. READ by ID
		readSecret, err := db.GetUserSecret(ctx, createdSecret.ID)
		require.NoError(t, err)
		assert.Equal(t, createdSecret.ID, readSecret.ID)
		assert.Equal(t, "workflow-secret", readSecret.Name)

		// 3. READ by UserID and Name
		readByNameParams := database.GetUserSecretByUserIDAndNameParams{
			UserID: testUser.ID,
			Name:   "workflow-secret",
		}
		readByNameSecret, err := db.GetUserSecretByUserIDAndName(ctx, readByNameParams)
		require.NoError(t, err)
		assert.Equal(t, createdSecret.ID, readByNameSecret.ID)

		// 4. LIST
		secrets, err := db.ListUserSecrets(ctx, testUser.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 1)
		assert.Equal(t, createdSecret.ID, secrets[0].ID)

		// 5. UPDATE
		updateParams := database.UpdateUserSecretParams{
			ID:          createdSecret.ID,
			Description: "Updated workflow description",
			Value:       "updated-workflow-value",
			EnvName:     "UPDATED_WORKFLOW_ENV",
			FilePath:    "/updated/workflow/path",
		}

		updatedSecret, err := db.UpdateUserSecret(ctx, updateParams)
		require.NoError(t, err)
		assert.Equal(t, "Updated workflow description", updatedSecret.Description)
		assert.Equal(t, "updated-workflow-value", updatedSecret.Value)

		// 6. DELETE
		err = db.DeleteUserSecret(ctx, createdSecret.ID)
		require.NoError(t, err)

		// Verify deletion
		_, err = db.GetUserSecret(ctx, createdSecret.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rows in result set")

		// Verify list is empty
		secrets, err = db.ListUserSecrets(ctx, testUser.ID)
		require.NoError(t, err)
		assert.Len(t, secrets, 0)
	})

	t.Run("UniqueConstraints", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Create a new user for this test
		testUser := dbgen.User(t, db, database.User{})

		// Create first secret
		secret1 := dbgen.UserSecret(t, db, database.UserSecret{
			UserID:      testUser.ID,
			Name:        "unique-test",
			Description: "First secret",
			Value:       "value1",
			EnvName:     "UNIQUE_ENV",
			FilePath:    "/unique/path",
		})

		// Try to create another secret with the same name (should fail)
		_, err := db.CreateUserSecret(ctx, database.CreateUserSecretParams{
			UserID:      testUser.ID,
			Name:        "unique-test", // Same name
			Description: "Second secret",
			Value:       "value2",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate key value")

		// Try to create another secret with the same env_name (should fail)
		_, err = db.CreateUserSecret(ctx, database.CreateUserSecretParams{
			UserID:      testUser.ID,
			Name:        "unique-test-2",
			Description: "Second secret",
			Value:       "value2",
			EnvName:     "UNIQUE_ENV", // Same env_name
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate key value")

		// Try to create another secret with the same file_path (should fail)
		_, err = db.CreateUserSecret(ctx, database.CreateUserSecretParams{
			UserID:      testUser.ID,
			Name:        "unique-test-3",
			Description: "Second secret",
			Value:       "value2",
			FilePath:    "/unique/path", // Same file_path
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate key value")

		// Create secret with empty env_name and file_path (should succeed)
		secret2 := dbgen.UserSecret(t, db, database.UserSecret{
			UserID:      testUser.ID,
			Name:        "unique-test-4",
			Description: "Second secret",
			Value:       "value2",
			EnvName:     "", // Empty env_name
			FilePath:    "", // Empty file_path
		})

		// Verify both secrets exist
		_, err = db.GetUserSecret(ctx, secret1.ID)
		require.NoError(t, err)
		_, err = db.GetUserSecret(ctx, secret2.ID)
		require.NoError(t, err)
	})
}

func TestUserSecretsAuthorization(t *testing.T) {
	t.Parallel()

	// Use raw database and wrap with dbauthz for authorization testing
	db, _ := dbtestutil.NewDB(t)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	authDB := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())

	// Create test users
	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})
	owner := dbgen.User(t, db, database.User{})
	orgAdmin := dbgen.User(t, db, database.User{})

	// Create organization for org-scoped roles
	org := dbgen.Organization(t, db, database.Organization{})

	// Create secrets for users
	user1Secret := dbgen.UserSecret(t, db, database.UserSecret{
		UserID:      user1.ID,
		Name:        "user1-secret",
		Description: "User 1's secret",
		Value:       "user1-value",
	})

	user2Secret := dbgen.UserSecret(t, db, database.UserSecret{
		UserID:      user2.ID,
		Name:        "user2-secret",
		Description: "User 2's secret",
		Value:       "user2-value",
	})

	testCases := []struct {
		name           string
		subject        rbac.Subject
		secretID       uuid.UUID
		expectedAccess bool
	}{
		{
			name: "UserCanAccessOwnSecrets",
			subject: rbac.Subject{
				ID:    user1.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
				Scope: rbac.ScopeAll,
			},
			secretID:       user1Secret.ID,
			expectedAccess: true,
		},
		{
			name: "UserCannotAccessOtherUserSecrets",
			subject: rbac.Subject{
				ID:    user1.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
				Scope: rbac.ScopeAll,
			},
			secretID:       user2Secret.ID,
			expectedAccess: false,
		},
		{
			name: "OwnerCannotAccessUserSecrets",
			subject: rbac.Subject{
				ID:    owner.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleOwner()},
				Scope: rbac.ScopeAll,
			},
			secretID:       user1Secret.ID,
			expectedAccess: false,
		},
		{
			name: "OrgAdminCannotAccessUserSecrets",
			subject: rbac.Subject{
				ID:    orgAdmin.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.ScopedRoleOrgAdmin(org.ID)},
				Scope: rbac.ScopeAll,
			},
			secretID:       user1Secret.ID,
			expectedAccess: false,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			authCtx := dbauthz.As(ctx, tc.subject)

			// Test GetUserSecret
			_, err := authDB.GetUserSecret(authCtx, tc.secretID)

			if tc.expectedAccess {
				require.NoError(t, err, "expected access to be granted")
			} else {
				require.Error(t, err, "expected access to be denied")
				assert.True(t, dbauthz.IsNotAuthorizedError(err), "expected authorization error")
			}
		})
	}
}

func TestWorkspaceBuildDeadlineConstraint(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:    user.ID,
		TemplateID: template.ID,
		Name:       "test-workspace",
		Deleted:    false,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		InitiatorID:    database.PrebuildsSystemUserID,
		Provisioner:    database.ProvisionerTypeEcho,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StartedAt:      sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true},
		CompletedAt:    sql.NullTime{Time: time.Now(), Valid: true},
	})
	workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.ID,
		JobID:             job.ID,
		BuildNumber:       1,
	})

	cases := []struct {
		name        string
		deadline    time.Time
		maxDeadline time.Time
		expectOK    bool
	}{
		{
			name:        "no deadline or max_deadline",
			deadline:    time.Time{},
			maxDeadline: time.Time{},
			expectOK:    true,
		},
		{
			name:        "deadline set when max_deadline is not set",
			deadline:    time.Now().Add(time.Hour),
			maxDeadline: time.Time{},
			expectOK:    true,
		},
		{
			name:        "deadline before max_deadline",
			deadline:    time.Now().Add(-time.Hour),
			maxDeadline: time.Now().Add(time.Hour),
			expectOK:    true,
		},
		{
			name:        "deadline is max_deadline",
			deadline:    time.Now().Add(time.Hour),
			maxDeadline: time.Now().Add(time.Hour),
			expectOK:    true,
		},

		{
			name:        "deadline after max_deadline",
			deadline:    time.Now().Add(time.Hour),
			maxDeadline: time.Now().Add(-time.Hour),
			expectOK:    false,
		},
		{
			name:        "deadline is not set when max_deadline is set",
			deadline:    time.Time{},
			maxDeadline: time.Now().Add(time.Hour),
			expectOK:    false,
		},
	}

	for _, c := range cases {
		err := db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:          workspaceBuild.ID,
			Deadline:    c.deadline,
			MaxDeadline: c.maxDeadline,
			UpdatedAt:   time.Now(),
		})
		if c.expectOK {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.True(t, database.IsCheckViolation(err, database.CheckWorkspaceBuildsDeadlineBelowMaxDeadline))
		}
	}
}

// TestGetLatestWorkspaceBuildsByWorkspaceIDs populates the database with
// workspaces and builds. It then tests that
// GetLatestWorkspaceBuildsByWorkspaceIDs returns the latest build for some
// subset of the workspaces.
func TestGetLatestWorkspaceBuildsByWorkspaceIDs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	admin := dbgen.User(t, db, database.User{})

	tv := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      admin.ID,
		}).
		Do()

	users := make([]database.User, 5)
	wrks := make([][]database.WorkspaceTable, len(users))
	exp := make(map[uuid.UUID]database.WorkspaceBuild)
	for i := range users {
		users[i] = dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         users[i].ID,
			OrganizationID: org.ID,
		})

		// Each user gets 2 workspaces.
		wrks[i] = make([]database.WorkspaceTable, 2)
		for wi := range wrks[i] {
			wrks[i][wi] = dbgen.Workspace(t, db, database.WorkspaceTable{
				TemplateID: tv.Template.ID,
				OwnerID:    users[i].ID,
			})

			// Choose a deterministic number of builds per workspace
			// No more than 5 builds though, that would be excessive.
			for j := int32(1); int(j) <= (i+wi)%5; j++ {
				wb := dbfake.WorkspaceBuild(t, db, wrks[i][wi]).
					Seed(database.WorkspaceBuild{
						WorkspaceID: wrks[i][wi].ID,
						BuildNumber: j + 1,
					}).
					Do()

				exp[wrks[i][wi].ID] = wb.Build // Save the final workspace build
			}
		}
	}

	// Only take half the users. And only take 1 workspace per user for the test.
	// The others are just noice. This just queries a subset of workspaces and builds
	// to make sure the noise doesn't interfere with the results.
	assertWrks := wrks[:len(users)/2]
	ctx := testutil.Context(t, testutil.WaitLong)
	ids := slice.Convert[[]database.WorkspaceTable, uuid.UUID](assertWrks, func(pair []database.WorkspaceTable) uuid.UUID {
		return pair[0].ID
	})

	require.Greater(t, len(ids), 0, "expected some workspace ids for test")
	builds, err := db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, ids)
	require.NoError(t, err)
	for _, b := range builds {
		expB, ok := exp[b.WorkspaceID]
		require.Truef(t, ok, "unexpected workspace build for workspace id %s", b.WorkspaceID)
		require.Equalf(t, expB.ID, b.ID, "unexpected workspace build id for workspace id %s", b.WorkspaceID)
		require.Equal(t, expB.BuildNumber, b.BuildNumber, "unexpected build number")
	}
}

func TestTasksWithStatusView(t *testing.T) {
	t.Parallel()

	createProvisionerJob := func(t *testing.T, db database.Store, org database.Organization, user database.User, buildStatus database.ProvisionerJobStatus) database.ProvisionerJob {
		t.Helper()

		var jobParams database.ProvisionerJob

		switch buildStatus {
		case database.ProvisionerJobStatusPending:
			jobParams = database.ProvisionerJob{
				OrganizationID: org.ID,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    user.ID,
			}
		case database.ProvisionerJobStatusRunning:
			jobParams = database.ProvisionerJob{
				OrganizationID: org.ID,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    user.ID,
				StartedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
			}
		case database.ProvisionerJobStatusFailed:
			jobParams = database.ProvisionerJob{
				OrganizationID: org.ID,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    user.ID,
				StartedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
				CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
				Error:          sql.NullString{Valid: true, String: "job failed"},
			}
		case database.ProvisionerJobStatusSucceeded:
			jobParams = database.ProvisionerJob{
				OrganizationID: org.ID,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    user.ID,
				StartedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
				CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
			}
		case database.ProvisionerJobStatusCanceling:
			jobParams = database.ProvisionerJob{
				OrganizationID: org.ID,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    user.ID,
				StartedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
				CanceledAt:     sql.NullTime{Valid: true, Time: dbtime.Now()},
			}
		case database.ProvisionerJobStatusCanceled:
			jobParams = database.ProvisionerJob{
				OrganizationID: org.ID,
				Type:           database.ProvisionerJobTypeWorkspaceBuild,
				InitiatorID:    user.ID,
				StartedAt:      sql.NullTime{Valid: true, Time: dbtime.Now()},
				CompletedAt:    sql.NullTime{Valid: true, Time: dbtime.Now()},
				CanceledAt:     sql.NullTime{Valid: true, Time: dbtime.Now()},
			}
		default:
			t.Errorf("invalid build status: %v", buildStatus)
		}

		return dbgen.ProvisionerJob(t, db, nil, jobParams)
	}

	createTask := func(
		ctx context.Context,
		t *testing.T,
		db database.Store,
		org database.Organization,
		user database.User,
		buildStatus database.ProvisionerJobStatus,
		buildTransition database.WorkspaceTransition,
		agentState database.WorkspaceAgentLifecycleState,
		appHealths []database.WorkspaceAppHealth,
	) database.Task {
		t.Helper()

		template := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})

		if buildStatus == "" {
			return dbgen.Task(t, db, database.TaskTable{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				Name:              "test-task",
				TemplateVersionID: templateVersion.ID,
				Prompt:            "Test prompt",
			})
		}

		job := createProvisionerJob(t, db, org, user, buildStatus)

		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			TemplateID:     template.ID,
			OwnerID:        user.ID,
		})
		workspaceID := uuid.NullUUID{Valid: true, UUID: workspace.ID}

		task := dbgen.Task(t, db, database.TaskTable{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			Name:              "test-task",
			WorkspaceID:       workspaceID,
			TemplateVersionID: templateVersion.ID,
			Prompt:            "Test prompt",
		})

		workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
			BuildNumber:       1,
			Transition:        buildTransition,
			InitiatorID:       user.ID,
			JobID:             job.ID,
		})
		workspaceBuildNumber := workspaceBuild.BuildNumber

		_, err := db.UpsertTaskWorkspaceApp(ctx, database.UpsertTaskWorkspaceAppParams{
			TaskID:               task.ID,
			WorkspaceBuildNumber: workspaceBuildNumber,
		})
		require.NoError(t, err)

		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})

		if agentState != "" {
			agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID: resource.ID,
			})
			workspaceAgentID := agent.ID

			_, err := db.UpsertTaskWorkspaceApp(ctx, database.UpsertTaskWorkspaceAppParams{
				TaskID:               task.ID,
				WorkspaceBuildNumber: workspaceBuildNumber,
				WorkspaceAgentID:     uuid.NullUUID{UUID: workspaceAgentID, Valid: true},
			})
			require.NoError(t, err)

			err = db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agent.ID,
				LifecycleState: agentState,
			})
			require.NoError(t, err)

			for i, health := range appHealths {
				app := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
					AgentID:     workspaceAgentID,
					Slug:        fmt.Sprintf("test-app-%d", i),
					DisplayName: fmt.Sprintf("Test App %d", i+1),
					Health:      health,
				})
				if i == 0 {
					// Assume the first app is the tasks app.
					_, err := db.UpsertTaskWorkspaceApp(ctx, database.UpsertTaskWorkspaceAppParams{
						TaskID:               task.ID,
						WorkspaceBuildNumber: workspaceBuildNumber,
						WorkspaceAgentID:     uuid.NullUUID{UUID: workspaceAgentID, Valid: true},
						WorkspaceAppID:       uuid.NullUUID{UUID: app.ID, Valid: true},
					})
					require.NoError(t, err)
				}
			}
		}

		return task
	}

	tests := []struct {
		name                      string
		buildStatus               database.ProvisionerJobStatus
		buildTransition           database.WorkspaceTransition
		agentState                database.WorkspaceAgentLifecycleState
		appHealths                []database.WorkspaceAppHealth
		expectedStatus            database.TaskStatus
		description               string
		expectBuildNumberValid    bool
		expectBuildNumber         int32
		expectWorkspaceAgentValid bool
		expectWorkspaceAppValid   bool
	}{
		{
			name:                      "NoWorkspace",
			expectedStatus:            "pending",
			description:               "Task with no workspace assigned",
			expectBuildNumberValid:    false,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "FailedBuild",
			buildStatus:               database.ProvisionerJobStatusFailed,
			buildTransition:           database.WorkspaceTransitionStart,
			expectedStatus:            database.TaskStatusError,
			description:               "Latest workspace build failed",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "CancelingBuild",
			buildStatus:               database.ProvisionerJobStatusCanceling,
			buildTransition:           database.WorkspaceTransitionStart,
			expectedStatus:            database.TaskStatusError,
			description:               "Latest workspace build is canceling",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "CanceledBuild",
			buildStatus:               database.ProvisionerJobStatusCanceled,
			buildTransition:           database.WorkspaceTransitionStart,
			expectedStatus:            database.TaskStatusError,
			description:               "Latest workspace build was canceled",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "StoppedWorkspace",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStop,
			expectedStatus:            database.TaskStatusPaused,
			description:               "Workspace is stopped",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "DeletedWorkspace",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionDelete,
			expectedStatus:            database.TaskStatusPaused,
			description:               "Workspace is deleted",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "PendingStart",
			buildStatus:               database.ProvisionerJobStatusPending,
			buildTransition:           database.WorkspaceTransitionStart,
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Workspace build is starting (pending)",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "RunningStart",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Workspace build is starting (running)",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: false,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "StartingAgent",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateStarting,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthInitializing},
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Workspace is running but agent is starting",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "CreatedAgent",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateCreated,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthInitializing},
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Workspace is running but agent is created",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "ReadyAgentInitializingApp",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthInitializing},
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Agent is ready but app is initializing",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "ReadyAgentHealthyApp",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthHealthy},
			expectedStatus:            database.TaskStatusActive,
			description:               "Agent is ready and app is healthy",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "ReadyAgentDisabledApp",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthDisabled},
			expectedStatus:            database.TaskStatusActive,
			description:               "Agent is ready and app health checking is disabled",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "ReadyAgentUnhealthyApp",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthUnhealthy},
			expectedStatus:            database.TaskStatusError,
			description:               "Agent is ready but app is unhealthy",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "AgentStartTimeout",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateStartTimeout,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthHealthy},
			expectedStatus:            database.TaskStatusActive,
			description:               "Agent start timed out but app is healthy, defer to app",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "AgentStartError",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateStartError,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthHealthy},
			expectedStatus:            database.TaskStatusActive,
			description:               "Agent start failed but app is healthy, defer to app",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "AgentShuttingDown",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateShuttingDown,
			expectedStatus:            database.TaskStatusUnknown,
			description:               "Agent is shutting down",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "AgentOff",
			buildStatus:               database.ProvisionerJobStatusSucceeded,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateOff,
			expectedStatus:            database.TaskStatusUnknown,
			description:               "Agent is off",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   false,
		},
		{
			name:                      "RunningJobReadyAgentHealthyApp",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthHealthy},
			expectedStatus:            database.TaskStatusActive,
			description:               "Running job with ready agent and healthy app should be active",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "RunningJobReadyAgentInitializingApp",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthInitializing},
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Running job with ready agent but initializing app should be initializing",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "RunningJobReadyAgentUnhealthyApp",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthUnhealthy},
			expectedStatus:            database.TaskStatusError,
			description:               "Running job with ready agent but unhealthy app should be error",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "RunningJobConnectingAgent",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateStarting,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthInitializing},
			expectedStatus:            database.TaskStatusInitializing,
			description:               "Running job with connecting agent should be initializing",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "RunningJobReadyAgentDisabledApp",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthDisabled},
			expectedStatus:            database.TaskStatusActive,
			description:               "Running job with ready agent and disabled app health checking should be active",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
		{
			name:                      "RunningJobReadyAgentHealthyTaskAppUnhealthyOtherAppIsOK",
			buildStatus:               database.ProvisionerJobStatusRunning,
			buildTransition:           database.WorkspaceTransitionStart,
			agentState:                database.WorkspaceAgentLifecycleStateReady,
			appHealths:                []database.WorkspaceAppHealth{database.WorkspaceAppHealthHealthy, database.WorkspaceAppHealthUnhealthy},
			expectedStatus:            database.TaskStatusActive,
			description:               "Running job with ready agent and multiple healthy apps should be active",
			expectBuildNumberValid:    true,
			expectBuildNumber:         1,
			expectWorkspaceAgentValid: true,
			expectWorkspaceAppValid:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})

			task := createTask(ctx, t, db, org, user, tt.buildStatus, tt.buildTransition, tt.agentState, tt.appHealths)

			got, err := db.GetTaskByID(ctx, task.ID)
			require.NoError(t, err)

			t.Logf("Task status debug: %s", got.StatusDebug)

			require.Equal(t, tt.expectedStatus, got.Status)

			require.Equal(t, tt.expectBuildNumberValid, got.WorkspaceBuildNumber.Valid)
			if tt.expectBuildNumberValid {
				require.Equal(t, tt.expectBuildNumber, got.WorkspaceBuildNumber.Int32)
			}

			require.Equal(t, tt.expectWorkspaceAgentValid, got.WorkspaceAgentID.Valid)
			if tt.expectWorkspaceAgentValid {
				require.NotEqual(t, uuid.Nil, got.WorkspaceAgentID.UUID)
			}

			require.Equal(t, tt.expectWorkspaceAppValid, got.WorkspaceAppID.Valid)
			if tt.expectWorkspaceAppValid {
				require.NotEqual(t, uuid.Nil, got.WorkspaceAppID.UUID)
			}
		})
	}
}

func TestGetTaskByWorkspaceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupTask func(t *testing.T, db database.Store, org database.Organization, user database.User, templateVersion database.TemplateVersion, workspace database.WorkspaceTable)
		wantErr   bool
	}{
		{
			name:    "task doesn't exist",
			wantErr: true,
		},
		{
			name: "task with no workspace id",
			setupTask: func(t *testing.T, db database.Store, org database.Organization, user database.User, templateVersion database.TemplateVersion, workspace database.WorkspaceTable) {
				dbgen.Task(t, db, database.TaskTable{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					Name:              "test-task",
					TemplateVersionID: templateVersion.ID,
					Prompt:            "Test prompt",
				})
			},
			wantErr: true,
		},
		{
			name: "task with workspace id",
			setupTask: func(t *testing.T, db database.Store, org database.Organization, user database.User, templateVersion database.TemplateVersion, workspace database.WorkspaceTable) {
				workspaceID := uuid.NullUUID{Valid: true, UUID: workspace.ID}
				dbgen.Task(t, db, database.TaskTable{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					Name:              "test-task",
					WorkspaceID:       workspaceID,
					TemplateVersionID: templateVersion.ID,
					Prompt:            "Test prompt",
				})
			},
			wantErr: false,
		},
	}

	db, _ := dbtestutil.NewDB(t)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})
			template := dbgen.Template(t, db, database.Template{
				OrganizationID: org.ID,
				CreatedBy:      user.ID,
			})
			templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: org.ID,
				TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
				CreatedBy:      user.ID,
			})
			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				TemplateID:     template.ID,
			})

			if tt.setupTask != nil {
				tt.setupTask(t, db, org, user, templateVersion, workspace)
			}

			ctx := testutil.Context(t, testutil.WaitLong)

			task, err := db.GetTaskByWorkspaceID(ctx, workspace.ID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.False(t, task.WorkspaceBuildNumber.Valid)
				require.False(t, task.WorkspaceAgentID.Valid)
				require.False(t, task.WorkspaceAppID.Valid)
			}
		})
	}
}

func TestTaskNameUniqueness(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user1.ID,
	})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user1.ID,
	})

	taskName := "my-task"

	// Create initial task for user1.
	task1 := dbgen.Task(t, db, database.TaskTable{
		OrganizationID:    org.ID,
		OwnerID:           user1.ID,
		Name:              taskName,
		TemplateVersionID: tv.ID,
		Prompt:            "Test prompt",
	})
	require.NotEqual(t, uuid.Nil, task1.ID)

	tests := []struct {
		name     string
		ownerID  uuid.UUID
		taskName string
		wantErr  bool
	}{
		{
			name:     "duplicate task name same user",
			ownerID:  user1.ID,
			taskName: taskName,
			wantErr:  true,
		},
		{
			name:     "duplicate task name different case same user",
			ownerID:  user1.ID,
			taskName: "MY-TASK",
			wantErr:  true,
		},
		{
			name:     "same task name different user",
			ownerID:  user2.ID,
			taskName: taskName,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			taskID := uuid.New()
			task, err := db.InsertTask(ctx, database.InsertTaskParams{
				ID:                 taskID,
				OrganizationID:     org.ID,
				OwnerID:            tt.ownerID,
				Name:               tt.taskName,
				TemplateVersionID:  tv.ID,
				TemplateParameters: json.RawMessage("{}"),
				Prompt:             "Test prompt",
				CreatedAt:          dbtime.Now(),
			})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotEqual(t, uuid.Nil, task.ID)
				require.NotEqual(t, task1.ID, task.ID)
				require.Equal(t, taskID, task.ID)
			}
		})
	}
}

func TestUsageEventsTrigger(t *testing.T) {
	t.Parallel()

	// This is not exposed in the querier interface intentionally.
	getDailyRows := func(ctx context.Context, sqlDB *sql.DB) []database.UsageEventsDaily {
		t.Helper()
		rows, err := sqlDB.QueryContext(ctx, "SELECT day, event_type, usage_data FROM usage_events_daily ORDER BY day ASC")
		require.NoError(t, err, "perform query")
		defer rows.Close()

		var out []database.UsageEventsDaily
		for rows.Next() {
			var row database.UsageEventsDaily
			err := rows.Scan(&row.Day, &row.EventType, &row.UsageData)
			require.NoError(t, err, "scan row")
			out = append(out, row)
		}
		return out
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

		// Assert there are no daily rows.
		rows := getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 0)

		// Insert a usage event.
		err := db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "1",
			EventType: "dc_managed_agents_v1",
			EventData: []byte(`{"count": 41}`),
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		// Assert there is one daily row that contains the correct data.
		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 1)
		require.Equal(t, "dc_managed_agents_v1", rows[0].EventType)
		require.JSONEq(t, `{"count": 41}`, string(rows[0].UsageData))
		// The read row might be `+0000` rather than `UTC` specifically, so just
		// ensure it's within 1 second of the expected time.
		require.WithinDuration(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), rows[0].Day, time.Second)

		// Insert a new usage event on the same UTC day, should increment the count.
		locSydney, err := time.LoadLocation("Australia/Sydney")
		require.NoError(t, err)
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "2",
			EventType: "dc_managed_agents_v1",
			EventData: []byte(`{"count": 1}`),
			// Insert it at a random point during the same day. Sydney is +1000 or
			// +1100, so 8am in Sydney is the previous day in UTC.
			CreatedAt: time.Date(2025, 1, 2, 8, 38, 57, 0, locSydney),
		})
		require.NoError(t, err)

		// There should still be only one daily row with the incremented count.
		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 1)
		require.Equal(t, "dc_managed_agents_v1", rows[0].EventType)
		require.JSONEq(t, `{"count": 42}`, string(rows[0].UsageData))
		require.WithinDuration(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), rows[0].Day, time.Second)

		// TODO: when we have a new event type, we should test that adding an
		// event with a different event type on the same day creates a new daily
		// row.

		// Insert a new usage event on a different day, should create a new daily
		// row.
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "3",
			EventType: "dc_managed_agents_v1",
			EventData: []byte(`{"count": 1}`),
			CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		// There should now be two daily rows.
		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 2)
		// Output is sorted by day ascending, so the first row should be the
		// previous day's row.
		require.Equal(t, "dc_managed_agents_v1", rows[0].EventType)
		require.JSONEq(t, `{"count": 42}`, string(rows[0].UsageData))
		require.WithinDuration(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), rows[0].Day, time.Second)
		require.Equal(t, "dc_managed_agents_v1", rows[1].EventType)
		require.JSONEq(t, `{"count": 1}`, string(rows[1].UsageData))
		require.WithinDuration(t, time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), rows[1].Day, time.Second)
	})

	t.Run("UnknownEventType", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

		// Relax the usage_events.event_type check constraint to see what
		// happens when we insert a usage event that the trigger doesn't know
		// about.
		_, err := sqlDB.ExecContext(ctx, "ALTER TABLE usage_events DROP CONSTRAINT usage_event_type_check")
		require.NoError(t, err)

		// Insert a usage event with an unknown event type.
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "broken",
			EventType: "dean's cool event",
			EventData: []byte(`{"my": "cool json"}`),
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		require.ErrorContains(t, err, "Unhandled usage event type in aggregate_usage_event")

		// The event should've been blocked.
		var count int
		err = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_events WHERE id = 'broken'").Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 0, count)

		// We should not have any daily rows.
		rows := getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 0)
	})
}

func TestListTasks(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	// Given: two organizations and two users, one of which is a member of both
	org1 := dbgen.Organization(t, db, database.Organization{})
	org2 := dbgen.Organization(t, db, database.Organization{})
	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org1.ID,
		UserID:         user1.ID,
	})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org2.ID,
		UserID:         user2.ID,
	})

	// Given: a template with an active version
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		CreatedBy:      user1.ID,
		OrganizationID: org1.ID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       user1.ID,
		OrganizationID:  org1.ID,
		ActiveVersionID: tv.ID,
	})

	// Helper function to create a task
	createTask := func(orgID, ownerID uuid.UUID) database.Task {
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: orgID,
			OwnerID:        ownerID,
			TemplateID:     tpl.ID,
		})
		pj := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{})
		sidebarAppID := uuid.New()
		wb := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			JobID:             pj.ID,
			TemplateVersionID: tv.ID,
			WorkspaceID:       ws.ID,
		})
		wr := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: pj.ID,
		})
		agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: wr.ID,
		})
		wa := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
			ID:      sidebarAppID,
			AgentID: agt.ID,
		})
		tsk := dbgen.Task(t, db, database.TaskTable{
			OrganizationID:    orgID,
			OwnerID:           ownerID,
			Prompt:            testutil.GetRandomName(t),
			TemplateVersionID: tv.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		})
		_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
			TaskID:               tsk.ID,
			WorkspaceBuildNumber: wb.BuildNumber,
			WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: agt.ID},
			WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: wa.ID},
		})
		t.Logf("task_id:%s owner_id:%s org_id:%s", tsk.ID, ownerID, orgID)
		return tsk
	}

	// Given: user1 has one task, user2 has one task, user3 has two tasks (one in each org)
	task1 := createTask(org1.ID, user1.ID)
	task2 := createTask(org1.ID, user2.ID)
	task3 := createTask(org2.ID, user2.ID)

	// Then: run various filters and assert expected results
	for _, tc := range []struct {
		name      string
		filter    database.ListTasksParams
		expectIDs []uuid.UUID
	}{
		{
			name: "no filter",
			filter: database.ListTasksParams{
				OwnerID:        uuid.Nil,
				OrganizationID: uuid.Nil,
			},
			expectIDs: []uuid.UUID{task3.ID, task2.ID, task1.ID},
		},
		{
			name: "filter by user ID",
			filter: database.ListTasksParams{
				OwnerID:        user1.ID,
				OrganizationID: uuid.Nil,
			},
			expectIDs: []uuid.UUID{task1.ID},
		},
		{
			name: "filter by organization ID",
			filter: database.ListTasksParams{
				OwnerID:        uuid.Nil,
				OrganizationID: org1.ID,
			},
			expectIDs: []uuid.UUID{task2.ID, task1.ID},
		},
		{
			name: "filter by user and organization ID",
			filter: database.ListTasksParams{
				OwnerID:        user2.ID,
				OrganizationID: org2.ID,
			},
			expectIDs: []uuid.UUID{task3.ID},
		},
		{
			name: "no results",
			filter: database.ListTasksParams{
				OwnerID:        user1.ID,
				OrganizationID: org2.ID,
			},
			expectIDs: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			tasks, err := db.ListTasks(ctx, tc.filter)
			require.NoError(t, err)
			require.Len(t, tasks, len(tc.expectIDs))

			for idx, eid := range tc.expectIDs {
				task := tasks[idx]
				assert.Equal(t, eid, task.ID, "task ID mismatch at index %d", idx)

				require.True(t, task.WorkspaceBuildNumber.Valid)
				require.Greater(t, task.WorkspaceBuildNumber.Int32, int32(0))
				require.True(t, task.WorkspaceAgentID.Valid)
				require.NotEqual(t, uuid.Nil, task.WorkspaceAgentID.UUID)
				require.True(t, task.WorkspaceAppID.Valid)
				require.NotEqual(t, uuid.Nil, task.WorkspaceAppID.UUID)
			}
		})
	}
}

func TestUpdateTaskWorkspaceID(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	// Create organization, users, template, and template version.
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
		CreatedBy:      user.ID,
	})

	// Create another template for mismatch test.
	template2 := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	tests := []struct {
		name      string
		setupTask func(t *testing.T) database.Task
		setupWS   func(t *testing.T) database.WorkspaceTable
		wantErr   bool
		wantNoRow bool
	}{
		{
			name: "successful update with matching template",
			setupTask: func(t *testing.T) database.Task {
				return dbgen.Task(t, db, database.TaskTable{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					Name:              testutil.GetRandomName(t),
					WorkspaceID:       uuid.NullUUID{},
					TemplateVersionID: templateVersion.ID,
					Prompt:            "Test prompt",
				})
			},
			setupWS: func(t *testing.T) database.WorkspaceTable {
				return dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: org.ID,
					OwnerID:        user.ID,
					TemplateID:     template.ID,
				})
			},
			wantErr:   false,
			wantNoRow: false,
		},
		{
			name: "task already has workspace_id",
			setupTask: func(t *testing.T) database.Task {
				existingWS := dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: org.ID,
					OwnerID:        user.ID,
					TemplateID:     template.ID,
				})
				return dbgen.Task(t, db, database.TaskTable{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					Name:              testutil.GetRandomName(t),
					WorkspaceID:       uuid.NullUUID{Valid: true, UUID: existingWS.ID},
					TemplateVersionID: templateVersion.ID,
					Prompt:            "Test prompt",
				})
			},
			setupWS: func(t *testing.T) database.WorkspaceTable {
				return dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: org.ID,
					OwnerID:        user.ID,
					TemplateID:     template.ID,
				})
			},
			wantErr:   false,
			wantNoRow: true, // No row should be returned because WHERE condition fails.
		},
		{
			name: "template mismatch between task and workspace",
			setupTask: func(t *testing.T) database.Task {
				return dbgen.Task(t, db, database.TaskTable{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					Name:              testutil.GetRandomName(t),
					WorkspaceID:       uuid.NullUUID{}, // NULL workspace_id
					TemplateVersionID: templateVersion.ID,
					Prompt:            "Test prompt",
				})
			},
			setupWS: func(t *testing.T) database.WorkspaceTable {
				return dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: org.ID,
					OwnerID:        user.ID,
					TemplateID:     template2.ID, // Different template, JOIN will fail.
				})
			},
			wantErr:   false,
			wantNoRow: true, // No row should be returned because JOIN condition fails.
		},
		{
			name: "task does not exist",
			setupTask: func(t *testing.T) database.Task {
				return database.Task{
					ID: uuid.New(), // Non-existent task ID.
				}
			},
			setupWS: func(t *testing.T) database.WorkspaceTable {
				return dbgen.Workspace(t, db, database.WorkspaceTable{
					OrganizationID: org.ID,
					OwnerID:        user.ID,
					TemplateID:     template.ID,
				})
			},
			wantErr:   false,
			wantNoRow: true,
		},
		{
			name: "workspace does not exist",
			setupTask: func(t *testing.T) database.Task {
				return dbgen.Task(t, db, database.TaskTable{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					Name:              testutil.GetRandomName(t),
					WorkspaceID:       uuid.NullUUID{},
					TemplateVersionID: templateVersion.ID,
					Prompt:            "Test prompt",
				})
			},
			setupWS: func(t *testing.T) database.WorkspaceTable {
				return database.WorkspaceTable{
					ID: uuid.New(), // Non-existent workspace ID.
				}
			},
			wantErr:   false,
			wantNoRow: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			task := tt.setupTask(t)
			workspace := tt.setupWS(t)

			updatedTask, err := db.UpdateTaskWorkspaceID(ctx, database.UpdateTaskWorkspaceIDParams{
				ID:          task.ID,
				WorkspaceID: uuid.NullUUID{Valid: true, UUID: workspace.ID},
			})

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			if tt.wantNoRow {
				require.ErrorIs(t, err, sql.ErrNoRows)
				return
			}

			require.NoError(t, err)
			require.Equal(t, task.ID, updatedTask.ID)
			require.True(t, updatedTask.WorkspaceID.Valid)
			require.Equal(t, workspace.ID, updatedTask.WorkspaceID.UUID)
			require.Equal(t, task.OrganizationID, updatedTask.OrganizationID)
			require.Equal(t, task.OwnerID, updatedTask.OwnerID)
			require.Equal(t, task.Name, updatedTask.Name)
			require.Equal(t, task.TemplateVersionID, updatedTask.TemplateVersionID)

			// Verify the update persisted by fetching the task again.
			fetchedTask, err := db.GetTaskByID(ctx, task.ID)
			require.NoError(t, err)
			require.True(t, fetchedTask.WorkspaceID.Valid)
			require.Equal(t, workspace.ID, fetchedTask.WorkspaceID.UUID)
		})
	}
}

func TestUpdateAIBridgeInterceptionEnded(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	t.Run("NonExistingInterception", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		got, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      uuid.New(),
			EndedAt: time.Now(),
		})
		require.ErrorContains(t, err, "no rows in result set")
		require.EqualValues(t, database.AIBridgeInterception{}, got)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		interceptions := []database.AIBridgeInterception{}

		for _, uid := range []uuid.UUID{{1}, {2}, {3}} {
			insertParams := database.InsertAIBridgeInterceptionParams{
				ID:          uid,
				InitiatorID: user.ID,
				Metadata:    json.RawMessage("{}"),
			}

			intc, err := db.InsertAIBridgeInterception(ctx, insertParams)
			require.NoError(t, err)
			require.Equal(t, uid, intc.ID)
			require.False(t, intc.EndedAt.Valid)
			interceptions = append(interceptions, intc)
		}

		intc0 := interceptions[0]
		endedAt := time.Now()
		// Mark first interception as done
		updated, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      intc0.ID,
			EndedAt: endedAt,
		})
		require.NoError(t, err)
		require.EqualValues(t, updated.ID, intc0.ID)
		require.True(t, updated.EndedAt.Valid)
		require.WithinDuration(t, endedAt, updated.EndedAt.Time, 5*time.Second)

		// Updating first interception again should fail
		updated, err = db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      intc0.ID,
			EndedAt: endedAt.Add(time.Hour),
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

		// Other interceptions should not have ended_at set
		for _, intc := range interceptions[1:] {
			got, err := db.GetAIBridgeInterceptionByID(ctx, intc.ID)
			require.NoError(t, err)
			require.False(t, got.EndedAt.Valid)
		}
	})
}

func TestDeleteExpiredAPIKeys(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	// Constant time for testing
	now := time.Date(2025, 11, 20, 12, 0, 0, 0, time.UTC)
	expiredBefore := now.Add(-time.Hour) // Anything before this is expired

	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})

	expiredTimes := []time.Time{
		expiredBefore.Add(-time.Hour * 24 * 365),
		expiredBefore.Add(-time.Hour * 24),
		expiredBefore.Add(-time.Hour),
		expiredBefore.Add(-time.Minute),
		expiredBefore.Add(-time.Second),
	}
	for _, exp := range expiredTimes {
		// Expired api keys
		dbgen.APIKey(t, db, database.APIKey{UserID: user.ID, ExpiresAt: exp})
	}

	unexpiredTimes := []time.Time{
		expiredBefore.Add(time.Hour * 24 * 365),
		expiredBefore.Add(time.Hour * 24),
		expiredBefore.Add(time.Hour),
		expiredBefore.Add(time.Minute),
		expiredBefore.Add(time.Second),
	}
	for _, unexp := range unexpiredTimes {
		// Unexpired api keys
		dbgen.APIKey(t, db, database.APIKey{UserID: user.ID, ExpiresAt: unexp})
	}

	// All keys are present before deletion
	keys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		LoginType: user.LoginType,
		UserID:    user.ID,
	})
	require.NoError(t, err)
	require.Len(t, keys, len(expiredTimes)+len(unexpiredTimes))

	// Delete expired keys
	// First verify the limit works by deleting one at a time
	deletedCount, err := db.DeleteExpiredAPIKeys(ctx, database.DeleteExpiredAPIKeysParams{
		Before:     expiredBefore,
		LimitCount: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), deletedCount)

	// Ensure it was deleted
	remaining, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		LoginType: user.LoginType,
		UserID:    user.ID,
	})
	require.NoError(t, err)
	require.Len(t, remaining, len(expiredTimes)+len(unexpiredTimes)-1)

	// Delete the rest of the expired keys
	deletedCount, err = db.DeleteExpiredAPIKeys(ctx, database.DeleteExpiredAPIKeysParams{
		Before:     expiredBefore,
		LimitCount: 100,
	})
	require.NoError(t, err)
	require.Equal(t, int64(len(expiredTimes)-1), deletedCount)

	// Ensure only unexpired keys remain
	remaining, err = db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		LoginType: user.LoginType,
		UserID:    user.ID,
	})
	require.NoError(t, err)
	require.Len(t, remaining, len(unexpiredTimes))
}

func TestGetAuthenticatedWorkspaceAgentAndBuildByAuthToken_ShutdownScripts(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      owner.ID,
	})
	ver := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID: uuid.NullUUID{
			UUID:  tpl.ID,
			Valid: true,
		},
		OrganizationID: tpl.OrganizationID,
		CreatedBy:      owner.ID,
	})

	t.Run("DuringStopBuild", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Create start build with succeeded job (already completed).
		startJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob)
		startJob = dbgen.ProvisionerJob(t, db, nil, startJob)
		startResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		startBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource.ID,
		})

		// Create stop build (becomes latest).
		stopJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob.ID,
		})

		// Agent should still authenticate during stop build execution.
		row, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent.AuthToken)
		require.NoError(t, err, "agent should authenticate during stop build execution")
		require.Equal(t, agent.ID, row.WorkspaceAgent.ID)
		require.Equal(t, startBuild.ID, row.WorkspaceBuild.ID, "should return start build, not stop build")
	})

	t.Run("AfterStopJobCompletes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Create start build with completed job.
		startJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob)
		startJob = dbgen.ProvisionerJob(t, db, nil, startJob)

		startResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource.ID,
		})

		// Create stop build (becomes latest) with completed job.
		stopJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &stopJob)
		stopJob = dbgen.ProvisionerJob(t, db, nil, stopJob)
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob.ID,
		})

		// Agent should NOT authenticate after stop job completes.
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent.AuthToken)
		require.ErrorIs(t, err, sql.ErrNoRows, "agent should not authenticate after stop job completes")
	})

	t.Run("FailedStartBuild", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Create START build with FAILED job.
		startJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusFailed, &startJob)
		startJob = dbgen.ProvisionerJob(t, db, nil, startJob)
		startResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource.ID,
		})

		// Create STOP build with running job.
		stopJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob.ID,
		})

		// Agent should NOT authenticate (start build failed).
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent.AuthToken)
		require.ErrorIs(t, err, sql.ErrNoRows, "agent from failed start build should not authenticate")
	})

	t.Run("PendingStopBuild", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Create start build with succeeded job.
		startJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob)
		startJob = dbgen.ProvisionerJob(t, db, nil, startJob)
		startResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		startBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource.ID,
		})

		// Create stop build with pending job (not started yet).
		stopJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusPending, &stopJob)
		stopJob = dbgen.ProvisionerJob(t, db, nil, stopJob)
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob.ID,
		})

		// Agent should authenticate during pending stop build.
		row, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent.AuthToken)
		require.NoError(t, err, "agent should authenticate during pending stop build")
		require.Equal(t, agent.ID, row.WorkspaceAgent.ID)
		require.Equal(t, startBuild.ID, row.WorkspaceBuild.ID, "should return start build")
	})

	t.Run("MultipleStartStopCycles", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Build 1: START (succeeded).
		startJob1 := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob1)
		startJob1 = dbgen.ProvisionerJob(t, db, nil, startJob1)
		startResource1 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob1.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob1.ID,
		})
		agent1 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource1.ID,
		})

		// Build 2: STOP (succeeded).
		stopJob1 := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &stopJob1)
		stopJob1 = dbgen.ProvisionerJob(t, db, nil, stopJob1)
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob1.ID,
		})

		// Build 3: START (succeeded).
		startJob2 := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob2)
		startJob2 = dbgen.ProvisionerJob(t, db, nil, startJob2)
		startResource2 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob2.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		startBuild2 := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       3,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob2.ID,
		})
		agent2 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource2.ID,
		})

		// Build 4: STOP (running).
		stopJob2 := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       4,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob2.ID,
		})

		// Agent from build 3 should authenticate.
		row, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent2.AuthToken)
		require.NoError(t, err, "agent from most recent start should authenticate during stop")
		require.Equal(t, agent2.ID, row.WorkspaceAgent.ID)
		require.Equal(t, startBuild2.ID, row.WorkspaceBuild.ID)

		// Agent from build 1 should NOT authenticate.
		_, err = db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent1.AuthToken)
		require.ErrorIs(t, err, sql.ErrNoRows, "agent from old cycle should not authenticate")
	})

	t.Run("WrongTransitionType", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Create first start build.
		startJob1 := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob1)
		startJob1 = dbgen.ProvisionerJob(t, db, nil, startJob1)
		startResource1 := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob1.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob1.ID,
		})
		agent1 := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource1.ID,
		})

		// Create another START build as latest (not STOP).
		startJob2 := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob2.ID,
		})

		// Agent from build 1 should NOT authenticate (latest is not STOP).
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent1.AuthToken)
		require.ErrorIs(t, err, sql.ErrNoRows, "agent should not authenticate when latest build is not STOP")
	})

	t.Run("MismatchedTemplateVersions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		// Create a different template version for the stop build.
		ver2 := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{
				UUID:  tpl.ID,
				Valid: true,
			},
			OrganizationID: tpl.OrganizationID,
			CreatedBy:      owner.ID,
		})

		// Create START build with version 1 (succeeded).
		startJob := database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &startJob)
		startJob = dbgen.ProvisionerJob(t, db, nil, startJob)
		startResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID:      startJob.ID,
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver.ID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
			InitiatorID:       owner.ID,
			JobID:             startJob.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: startResource.ID,
		})

		// Create STOP build with version 2 (running).
		stopJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			InitiatorID:    owner.ID,
			OrganizationID: org.ID,
			JobStatus:      database.ProvisionerJobStatusRunning,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: ver2.ID,
			BuildNumber:       2,
			Transition:        database.WorkspaceTransitionStop,
			InitiatorID:       owner.ID,
			JobID:             stopJob.ID,
		})

		// Agent should NOT authenticate (template versions don't match).
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(dbauthz.AsSystemRestricted(ctx), agent.AuthToken)
		require.ErrorIs(t, err, sql.ErrNoRows, "agent should not authenticate when template versions differ")
	})
}
