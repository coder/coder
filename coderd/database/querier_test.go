package database_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
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

func TestChatContextHydration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:                "test-model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault:            true,
		CompressionThreshold: 80,
	})

	// Chats are scoped per agent, so build two independent agents.
	newAgent := func() database.WorkspaceAgent {
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
		return dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	}
	agent := newAgent()
	otherAgent := newAgent()

	newChat := func(status database.ChatStatus, agentID uuid.UUID) database.Chat {
		return dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
			Status:            status,
		})
	}

	hashH := []byte{0x01, 0x02, 0x03}
	hashOther := []byte{0xff, 0xee}

	chatNull := newChat(database.ChatStatusWaiting, agent.ID)       // never hydrated
	chatMatch := newChat(database.ChatStatusRunning, agent.ID)      // already at hashH
	chatDrift := newChat(database.ChatStatusRunning, agent.ID)      // drifted, active
	chatTerminal := newChat(database.ChatStatusCompleted, agent.ID) // drifted, terminal
	chatArchived := newChat(database.ChatStatusRunning, agent.ID)   // drifted, archived
	chatOtherAgent := newChat(database.ChatStatusRunning, otherAgent.ID)

	// Pin starting hashes; chatNull is intentionally left NULL.
	require.NoError(t, db.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{ID: chatMatch.ID, AggregateHash: hashH}))
	for _, id := range []uuid.UUID{chatDrift.ID, chatTerminal.ID, chatArchived.ID, chatOtherAgent.ID} {
		require.NoError(t, db.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{ID: id, AggregateHash: hashOther}))
	}
	_, err := db.ArchiveChatByID(ctx, chatArchived.ID)
	require.NoError(t, err)

	// Hydrate stamps only the NULL-hash chat for this agent.
	require.NoError(t, db.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
		AgentID:       agent.ID,
		AggregateHash: hashH,
	}))
	gotNull, err := db.GetChatByID(ctx, chatNull.ID)
	require.NoError(t, err)
	require.Equal(t, hashH, gotNull.ContextAggregateHash, "NULL-hash chat is hydrated")
	gotDrift, err := db.GetChatByID(ctx, chatDrift.ID)
	require.NoError(t, err)
	require.Equal(t, hashOther, gotDrift.ContextAggregateHash, "hydrate must not overwrite an already-pinned hash")

	// Mark dirty: only the active, pinned, drifted chat for THIS agent flips.
	// chatNull (now matches), chatMatch (matches), chatTerminal (status
	// excluded), chatArchived (archived), and chatOtherAgent (other agent)
	// are all left clean.
	now := dbtime.Now()
	flipped, err := db.MarkChatsContextDirtyByAgent(ctx, database.MarkChatsContextDirtyByAgentParams{
		AgentID:       agent.ID,
		AggregateHash: hashH,
		DirtySince:    sql.NullTime{Time: now, Valid: true},
	})
	require.NoError(t, err)
	flippedIDs := make([]uuid.UUID, 0, len(flipped))
	for _, f := range flipped {
		flippedIDs = append(flippedIDs, f.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{chatDrift.ID}, flippedIDs)

	gotDrift, err = db.GetChatByID(ctx, chatDrift.ID)
	require.NoError(t, err)
	require.True(t, gotDrift.ContextDirtySince.Valid, "drifted chat is marked dirty")

	// Refresh re-pins to the latest hash and clears the dirty marker.
	require.NoError(t, db.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{ID: chatDrift.ID, AggregateHash: hashH}))
	gotDrift, err = db.GetChatByID(ctx, chatDrift.ID)
	require.NoError(t, err)
	require.Equal(t, hashH, gotDrift.ContextAggregateHash)
	require.False(t, gotDrift.ContextDirtySince.Valid, "refresh clears the dirty marker")

	// With every chat now matching, a second mark is a no-op.
	flipped, err = db.MarkChatsContextDirtyByAgent(ctx, database.MarkChatsContextDirtyByAgentParams{
		AgentID:       agent.ID,
		AggregateHash: hashH,
		DirtySince:    sql.NullTime{Time: now, Valid: true},
	})
	require.NoError(t, err)
	require.Empty(t, flipped)

	// The other agent's chat is never touched by this agent's push.
	gotOther, err := db.GetChatByID(ctx, chatOtherAgent.ID)
	require.NoError(t, err)
	require.Equal(t, hashOther, gotOther.ContextAggregateHash)
	require.False(t, gotOther.ContextDirtySince.Valid)
}

func TestGetAuthorizedChats(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	// Create users with different roles.
	owner := dbgen.User(t, db, database.User{
		RBACRoles: []string{rbac.RoleOwner().String()},
	})
	member := dbgen.User(t, db, database.User{})
	secondMember := dbgen.User(t, db, database.User{})

	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: member.ID, OrganizationID: org.ID, Roles: []string{rbac.RoleAgentsAccess()}})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: secondMember.ID, OrganizationID: org.ID, Roles: []string{rbac.RoleAgentsAccess()}})

	// Create FK dependencies: a chat provider and model config.
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
	})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:                "test-model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault:            true,
		CompressionThreshold: 80,
	})

	// Create 3 chats owned by owner.
	for i := range 3 {
		dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             fmt.Sprintf("owner chat %d", i+1),
		})
	}

	// Create 2 chats owned by member.
	for i := range 2 {
		dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           member.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             fmt.Sprintf("member chat %d", i+1),
		})
	}

	t.Run("sqlQuerier", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Member should only see their own 2 chats.
		memberSubject, _, err := httpmw.UserRBACSubject(ctx, db, member.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedMember, err := authorizer.Prepare(ctx, memberSubject, policy.ActionRead, rbac.ResourceChat.Type)
		require.NoError(t, err)
		memberRows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedMember)
		require.NoError(t, err)
		require.Len(t, memberRows, 2)
		for _, row := range memberRows {
			require.Equal(t, member.ID, row.Chat.OwnerID, "member should only see own chats")
		}

		// Owner should see at least the 5 pre-created chats (site-wide
		// access). Parallel subtests may add more, so use GreaterOrEqual.
		ownerSubject, _, err := httpmw.UserRBACSubject(ctx, db, owner.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedOwner, err := authorizer.Prepare(ctx, ownerSubject, policy.ActionRead, rbac.ResourceChat.Type)
		require.NoError(t, err)
		ownerRows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedOwner)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(ownerRows), 5)

		// secondMember has no chats and should see 0.
		secondSubject, _, err := httpmw.UserRBACSubject(ctx, db, secondMember.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedSecond, err := authorizer.Prepare(ctx, secondSubject, policy.ActionRead, rbac.ResourceChat.Type)
		require.NoError(t, err)
		secondRows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedSecond)
		require.NoError(t, err)
		require.Len(t, secondRows, 0)

		// Org admin should NOT see other users' chats when they are
		// in a different org than the chat owner.
		orgs, err := db.GetOrganizations(ctx, database.GetOrganizationsParams{})
		require.NoError(t, err)
		require.NotEmpty(t, orgs)
		orgAdmin := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         orgAdmin.ID,
			OrganizationID: orgs[0].ID,
			Roles:          []string{rbac.RoleOrgAdmin()},
		})
		orgAdminSubject, _, err := httpmw.UserRBACSubject(ctx, db, orgAdmin.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedOrgAdmin, err := authorizer.Prepare(ctx, orgAdminSubject, policy.ActionRead, rbac.ResourceChat.Type)
		require.NoError(t, err)
		orgAdminRows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedOrgAdmin)
		require.NoError(t, err)
		require.Len(t, orgAdminRows, 0, "org admin with no chats should see 0 chats")

		// Org admin in SAME org should see all chats in that org.
		sameOrgAdmin := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         sameOrgAdmin.ID,
			OrganizationID: org.ID,
			Roles:          []string{rbac.RoleOrgAdmin()},
		})
		sameOrgAdminSubject, _, err := httpmw.UserRBACSubject(ctx, db, sameOrgAdmin.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedSameOrgAdmin, err := authorizer.Prepare(ctx, sameOrgAdminSubject, policy.ActionRead, rbac.ResourceChat.Type)
		require.NoError(t, err)
		sameOrgAdminRows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedSameOrgAdmin)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(sameOrgAdminRows), 5, "same-org admin should see all chats in their org")

		// OwnedOnly filter: member queries their own chats.
		memberFilterSelf, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  member.ID,
		}, preparedMember)
		require.NoError(t, err)
		require.Len(t, memberFilterSelf, 2)

		// OwnedOnly filter: member queries owner's chats and sees 0.
		memberFilterOwner, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  owner.ID,
		}, preparedMember)
		require.NoError(t, err)
		require.Len(t, memberFilterOwner, 0)

		_, err = db.GetAuthorizedChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
		}, preparedMember)
		require.ErrorContains(t, err, "viewer_id required")

		_, err = db.GetAuthorizedChats(ctx, database.GetChatsParams{
			SharedOnly: true,
		}, preparedMember)
		require.ErrorContains(t, err, "viewer_id required")

		_, err = db.GetAuthorizedChats(ctx, database.GetChatsParams{
			SharedOnly: true,
			ViewerID:   member.ID,
		}, preparedMember)
		require.ErrorContains(t, err, "shared_with_user_id or shared_with_group_ids required")
	})

	t.Run("dbauthz", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		authzdb := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())

		// As member: should see only own 2 chats.
		memberSubject, _, err := httpmw.UserRBACSubject(ctx, authzdb, member.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		memberCtx := dbauthz.As(ctx, memberSubject)
		memberRows, err := authzdb.GetChats(memberCtx, database.GetChatsParams{})
		require.NoError(t, err)
		require.Len(t, memberRows, 2)
		for _, row := range memberRows {
			require.Equal(t, member.ID, row.Chat.OwnerID, "member should only see own chats")
		}

		// As owner: should see at least the 5 pre-created chats.
		ownerSubject, _, err := httpmw.UserRBACSubject(ctx, authzdb, owner.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		ownerCtx := dbauthz.As(ctx, ownerSubject)
		ownerRows, err := authzdb.GetChats(ownerCtx, database.GetChatsParams{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(ownerRows), 5)

		ownerSharedRows, err := authzdb.GetChats(ownerCtx, database.GetChatsParams{
			SharedOnly:         true,
			ViewerID:           owner.ID,
			SharedWithUserID:   owner.ID,
			SharedWithGroupIds: []string{},
		})
		require.NoError(t, err)
		require.Empty(t, ownerSharedRows, "shared-only must not include chats visible through owner RBAC")

		// As secondMember: should see 0 chats.
		secondSubject, _, err := httpmw.UserRBACSubject(ctx, authzdb, secondMember.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		secondCtx := dbauthz.As(ctx, secondSubject)
		secondRows, err := authzdb.GetChats(secondCtx, database.GetChatsParams{})
		require.NoError(t, err)
		require.Len(t, secondRows, 0)
	})

	t.Run("pagination", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Use a dedicated user for pagination to avoid interference
		// with the other parallel subtests.
		paginationUser := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: paginationUser.ID, OrganizationID: org.ID, Roles: []string{rbac.RoleAgentsAccess()}})
		for i := range 7 {
			dbgen.Chat(t, db, database.Chat{
				OrganizationID:    org.ID,
				OwnerID:           paginationUser.ID,
				LastModelConfigID: modelCfg.ID,
				Title:             fmt.Sprintf("pagination chat %d", i+1),
			})
		}

		pagUserSubject, _, err := httpmw.UserRBACSubject(ctx, db, paginationUser.ID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		preparedMember, err := authorizer.Prepare(ctx, pagUserSubject, policy.ActionRead, rbac.ResourceChat.Type)
		require.NoError(t, err)

		// Fetch first page with limit=2.
		page1, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
			LimitOpt: 2,
		}, preparedMember)
		require.NoError(t, err)
		require.Len(t, page1, 2)
		for _, row := range page1 {
			require.Equal(t, paginationUser.ID, row.Chat.OwnerID, "paginated results must belong to pagination user")
		}

		// Fetch remaining pages and collect all chat IDs.
		allIDs := make(map[uuid.UUID]struct{})
		for _, row := range page1 {
			allIDs[row.Chat.ID] = struct{}{}
		}
		offset := int32(2)
		for {
			page, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
				LimitOpt:  2,
				OffsetOpt: offset,
			}, preparedMember)
			require.NoError(t, err)
			for _, row := range page {
				require.Equal(t, paginationUser.ID, row.Chat.OwnerID, "paginated results must belong to pagination user")
				allIDs[row.Chat.ID] = struct{}{}
			}
			if len(page) < 2 {
				break
			}
			offset += int32(len(page)) //nolint:gosec // Test code, pagination values are small.
		}

		// All 7 member chats should be accounted for with no leakage.
		require.Len(t, allIDs, 7, "pagination should return all member chats exactly once")
	})
}

//nolint:tparallel,paralleltest // It toggles the global chat ACL flag.
func TestGetAuthorizedChatsACLSharing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	rbac.SetChatACLDisabled(false)
	t.Cleanup(func() { rbac.SetChatACLDisabled(false) })

	ctx := testutil.Context(t, testutil.WaitMedium)
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	owner := dbgen.User(t, db, database.User{})
	recipient := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
		Roles:          []string{rbac.RoleAgentsAccess()},
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         recipient.ID,
		OrganizationID: org.ID,
		Roles:          []string{rbac.RoleAgentsAccess()},
	})

	dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:                "test-model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault:            true,
		CompressionThreshold: 80,
	})

	ownerChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "shared owner chat",
	})
	recipientChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           recipient.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "recipient chat",
	})

	sharedACL := database.ChatACL{
		recipient.ID.String(): database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}},
	}
	err = db.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
		ID:       ownerChat.ID,
		UserACL:  sharedACL,
		GroupACL: database.ChatACL{},
	})
	require.NoError(t, err)

	recipientSubject, _, err := httpmw.UserRBACSubject(ctx, db, recipient.ID, rbac.ExpandableScope(rbac.ScopeAll))
	require.NoError(t, err)
	preparedRecipient, err := authorizer.Prepare(ctx, recipientSubject, policy.ActionRead, rbac.ResourceChat.Type)
	require.NoError(t, err)

	chatIDs := func(rows []database.GetChatsRow) []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.Chat.ID)
		}
		return ids
	}

	rows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedRecipient)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{ownerChat.ID, recipientChat.ID}, chatIDs(rows))

	sharedOnly, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
		SharedOnly:       true,
		ViewerID:         recipient.ID,
		SharedWithUserID: recipient.ID,
	}, preparedRecipient)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{ownerChat.ID}, chatIDs(sharedOnly))
	require.Equal(t, sharedACL, sharedOnly[0].Chat.UserACL)
	require.Empty(t, sharedOnly[0].Chat.GroupACL)

	ownedAndShared, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
		OwnedOnly:        true,
		SharedOnly:       true,
		ViewerID:         recipient.ID,
		SharedWithUserID: recipient.ID,
	}, preparedRecipient)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{ownerChat.ID, recipientChat.ID}, chatIDs(ownedAndShared))

	authzdb := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())
	recipientCtx := dbauthz.As(ctx, recipientSubject)
	authzRows, err := authzdb.GetChats(recipientCtx, database.GetChatsParams{})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{ownerChat.ID, recipientChat.ID}, chatIDs(authzRows))

	authzSharedOnly, err := authzdb.GetChats(recipientCtx, database.GetChatsParams{
		SharedOnly:       true,
		ViewerID:         recipient.ID,
		SharedWithUserID: recipient.ID,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{ownerChat.ID}, chatIDs(authzSharedOnly))

	rbac.SetChatACLDisabled(true)
	disabledRows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedRecipient)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{recipientChat.ID}, chatIDs(disabledRows))
}

//nolint:tparallel,paralleltest // It toggles the global chat ACL flag.
func TestGetAuthorizedChatsACLSharingGroupACL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	rbac.SetChatACLDisabled(false)
	t.Cleanup(func() { rbac.SetChatACLDisabled(false) })

	ctx := testutil.Context(t, testutil.WaitMedium)
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	owner := dbgen.User(t, db, database.User{})
	recipient := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
		Roles:          []string{rbac.RoleAgentsAccess()},
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         recipient.ID,
		OrganizationID: org.ID,
		Roles:          []string{rbac.RoleAgentsAccess()},
	})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: recipient.ID, GroupID: group.ID})

	dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:                "test-model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault:            true,
		CompressionThreshold: 80,
	})

	ownerChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "shared owner chat",
	})
	recipientChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           recipient.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "recipient chat",
	})

	sharedGroupACL := database.ChatACL{
		group.ID.String(): database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}},
	}
	err = db.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
		ID:       ownerChat.ID,
		UserACL:  database.ChatACL{},
		GroupACL: sharedGroupACL,
	})
	require.NoError(t, err)

	recipientSubject, _, err := httpmw.UserRBACSubject(ctx, db, recipient.ID, rbac.ExpandableScope(rbac.ScopeAll))
	require.NoError(t, err)
	preparedRecipient, err := authorizer.Prepare(ctx, recipientSubject, policy.ActionRead, rbac.ResourceChat.Type)
	require.NoError(t, err)

	chatIDs := func(rows []database.GetChatsRow) []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.Chat.ID)
		}
		return ids
	}

	rows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, preparedRecipient)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{ownerChat.ID, recipientChat.ID}, chatIDs(rows))

	sharedOnly, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{
		SharedOnly:         true,
		ViewerID:           recipient.ID,
		SharedWithGroupIds: []string{group.ID.String()},
	}, preparedRecipient)
	require.NoError(t, err)
	require.Len(t, sharedOnly, 1)
	require.Equal(t, ownerChat.ID, sharedOnly[0].Chat.ID)
	require.Empty(t, sharedOnly[0].Chat.UserACL)
	require.Equal(t, sharedGroupACL, sharedOnly[0].Chat.GroupACL)

	authzdb := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{}), coderdtest.AccessControlStorePointer())
	recipientCtx := dbauthz.As(ctx, recipientSubject)
	authzSharedOnly, err := authzdb.GetChats(recipientCtx, database.GetChatsParams{
		SharedOnly:         true,
		ViewerID:           recipient.ID,
		SharedWithGroupIds: []string{group.ID.String()},
	})
	require.NoError(t, err)
	require.Len(t, authzSharedOnly, 1)
	require.Equal(t, ownerChat.ID, authzSharedOnly[0].Chat.ID)
}

//nolint:tparallel,paralleltest // It toggles the global chat ACL flag.
func TestGetAuthorizedChatsByChatFileIDACLSharing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	rbac.SetChatACLDisabled(false)
	t.Cleanup(func() { rbac.SetChatACLDisabled(false) })

	ctx := testutil.Context(t, testutil.WaitMedium)
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	owner := dbgen.User(t, db, database.User{})
	recipient := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
		Roles:          []string{rbac.RoleAgentsAccess()},
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         recipient.ID,
		OrganizationID: org.ID,
		Roles:          []string{rbac.RoleAgentsAccess()},
	})

	dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:                "test-model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault:            true,
		CompressionThreshold: 80,
	})

	ownerChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "shared owner chat",
	})
	sharedACL := database.ChatACL{
		recipient.ID.String(): database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}},
	}
	err = db.UpdateChatACLByID(ctx, database.UpdateChatACLByIDParams{
		ID:       ownerChat.ID,
		UserACL:  sharedACL,
		GroupACL: database.ChatACL{},
	})
	require.NoError(t, err)

	fileRow, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
		Name:           "shared.txt",
		Mimetype:       "text/plain",
		Data:           []byte("shared file"),
	})
	require.NoError(t, err)

	rejected, err := db.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       ownerChat.ID,
		FileIds:      []uuid.UUID{fileRow.ID},
		MaxFileLinks: 10,
	})
	require.NoError(t, err)
	require.Zero(t, rejected)

	recipientSubject, _, err := httpmw.UserRBACSubject(ctx, db, recipient.ID, rbac.ExpandableScope(rbac.ScopeAll))
	require.NoError(t, err)
	preparedRecipient, err := authorizer.Prepare(ctx, recipientSubject, policy.ActionRead, rbac.ResourceChat.Type)
	require.NoError(t, err)

	rows, err := db.GetAuthorizedChatsByChatFileID(ctx, fileRow.ID, preparedRecipient)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, ownerChat.ID, rows[0].ID)
	require.Equal(t, sharedACL, rows[0].UserACL)
	require.Empty(t, rows[0].GroupACL)
}

func TestGetChatFileDataPrefixesByIDs(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	longData := bytes.Repeat([]byte("a"), 100)
	longFile, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
		Name:           "long.txt",
		Mimetype:       "text/plain",
		Data:           longData,
	})
	require.NoError(t, err)
	shortFile, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
		Name:           "short.txt",
		Mimetype:       "text/plain",
		Data:           []byte("tiny"),
	})
	require.NoError(t, err)

	rows, err := db.GetChatFileDataPrefixesByIDs(ctx, database.GetChatFileDataPrefixesByIDsParams{
		IDs:         []uuid.UUID{longFile.ID, shortFile.ID},
		PrefixBytes: 16,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	prefixes := make(map[uuid.UUID]database.GetChatFileDataPrefixesByIDsRow, len(rows))
	for _, row := range rows {
		prefixes[row.ID] = row
	}
	require.Equal(t, longData[:16], prefixes[longFile.ID].DataPrefix)
	require.Equal(t, []byte("tiny"), prefixes[shortFile.ID].DataPrefix)
	require.Equal(t, owner.ID, prefixes[longFile.ID].OwnerID)
	require.Equal(t, org.ID, prefixes[longFile.ID].OrganizationID)
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
	require.Equal(t, defProxy.IconURL, "/emojis/1f3e1.png")

	// Set the proxy values
	args := database.UpsertDefaultProxyParams{
		DisplayName: "displayname",
		IconURL:     "/icon.png",
	}
	err = db.UpsertDefaultProxy(ctx, args)
	require.NoError(t, err, "insert def proxy")

	defProxy, err = db.GetDefaultProxyConfig(ctx)
	require.NoError(t, err, "get def proxy")
	require.Equal(t, defProxy.DisplayName, args.DisplayName)
	require.Equal(t, defProxy.IconURL, args.IconURL)

	// Upsert values
	args = database.UpsertDefaultProxyParams{
		DisplayName: "newdisplayname",
		IconURL:     "/newicon.png",
	}
	err = db.UpsertDefaultProxy(ctx, args)
	require.NoError(t, err, "upsert def proxy")

	defProxy, err = db.GetDefaultProxyConfig(ctx)
	require.NoError(t, err, "get def proxy")
	require.Equal(t, defProxy.DisplayName, args.DisplayName)
	require.Equal(t, defProxy.IconURL, args.IconURL)

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

	t.Run("SkipsCanceledPendingJobs", func(t *testing.T) {
		t.Parallel()
		var (
			db, _ = dbtestutil.NewDB(t)
			ctx   = testutil.Context(t, testutil.WaitMedium)
			org   = dbgen.Organization(t, db, database.Organization{})
			now   = dbtime.Now()
		)

		// Insert a pending job (started_at is NULL).
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      now,
			UpdatedAt:      now,
			InitiatorID:    uuid.New(),
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

		// Cancel it while still pending. In production (workspacebuilds.go), canceling
		// a pending build sets completed_at but leaves started_at NULL since no
		// provisioner ever started the job.
		err = db.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
			ID:          job.ID,
			CanceledAt:  sql.NullTime{Time: now, Valid: true},
			CompletedAt: sql.NullTime{Time: now, Valid: true},
		})
		require.NoError(t, err)

		// AcquireProvisionerJob should skip this job since it's already completed.
		_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
			OrganizationID:  org.ID,
			StartedAt:       sql.NullTime{Time: now, Valid: true},
			WorkerID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
			Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
			ProvisionerTags: json.RawMessage(`{}`),
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
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

func TestInsertUserServiceAccountConstraints(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	// Happy path: should succeed.
	t.Run("ServiceAccountWithEmptyEmailAndLoginNone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		user, err := db.InsertUser(ctx, database.InsertUserParams{
			Email:            "",
			LoginType:        database.LoginTypeNone,
			ID:               uuid.New(),
			Username:         "sa-ok",
			RBACRoles:        []string{},
			IsServiceAccount: true,
		})
		require.NoError(t, err)
		require.True(t, user.IsServiceAccount)
		require.Empty(t, user.Email)
	})

	// Service account with a non-empty email should be rejected
	// by the users_email_not_empty constraint.
	t.Run("ServiceAccountWithNonEmptyEmail", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := db.InsertUser(ctx, database.InsertUserParams{
			Email:            "sa@coder.com",
			LoginType:        database.LoginTypeNone,
			ID:               uuid.New(),
			Username:         "sa-with-email",
			RBACRoles:        []string{},
			IsServiceAccount: true,
		})
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckUsersEmailNotEmpty))
	})

	// A non-service-account with empty email should be rejected
	// by the users_email_not_empty constraint.
	t.Run("RegularUserWithEmptyEmail", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := db.InsertUser(ctx, database.InsertUserParams{
			Email:            "",
			LoginType:        database.LoginTypePassword,
			ID:               uuid.New(),
			Username:         "regular-no-email",
			RBACRoles:        []string{},
			IsServiceAccount: false,
		})
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckUsersEmailNotEmpty))
	})

	// Service account with login_type!=none should be rejected
	// by the users_service_account_login_type constraint.
	t.Run("ServiceAccountWithPasswordLoginType", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := db.InsertUser(ctx, database.InsertUserParams{
			Email:            "",
			LoginType:        database.LoginTypePassword,
			ID:               uuid.New(),
			Username:         "sa-with-password",
			RBACRoles:        []string{},
			IsServiceAccount: true,
		})
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckUsersServiceAccountLoginType))
	})
}

func TestGetActiveUserCount(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Seed users: 2 active humans, 1 active service account,
	// 1 dormant, 1 deleted. Only the 2 active humans should
	// be counted for license seat purposes.
	_ = dbgen.User(t, db, database.User{
		Status: database.UserStatusActive,
	})
	_ = dbgen.User(t, db, database.User{
		Status: database.UserStatusActive,
	})
	_ = dbgen.User(t, db, database.User{
		Status:           database.UserStatusActive,
		IsServiceAccount: true,
	})
	_ = dbgen.User(t, db, database.User{
		Status: database.UserStatusDormant,
	})
	_ = dbgen.User(t, db, database.User{
		Status:  database.UserStatusActive,
		Deleted: true,
	})

	count, err := db.GetActiveUserCount(ctx, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
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

		require.ElementsMatch(t, slice.List(everyoneMembers, groupMemberIDs),
			slice.List([]database.OrganizationMember{memOne, memTwo}, orgMemberIDs))

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
				LookupRoles: slice.List(allRoles, roleToLookup),
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

			a := slice.List(filtered, normalizedRoleName)
			b := slice.List(found, normalizedRoleName)
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

func TestGetAuthorizationUserRolesImpliedOrgRole(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	regularUser := dbgen.User(t, db, database.User{})
	saUser := dbgen.User(t, db, database.User{IsServiceAccount: true})

	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         regularUser.ID,
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         saUser.ID,
	})

	ctx := testutil.Context(t, testutil.WaitShort)

	wantMember := rbac.RoleOrgMember() + ":" + org.ID.String()
	wantSA := rbac.RoleOrgServiceAccount() + ":" + org.ID.String()

	// Regular users get the implied organization-member role.
	regularRoles, err := db.GetAuthorizationUserRoles(ctx, regularUser.ID)
	require.NoError(t, err)
	require.Contains(t, regularRoles.Roles, wantMember)
	require.NotContains(t, regularRoles.Roles, wantSA)

	// Service accounts get the implied organization-service-account role.
	saRoles, err := db.GetAuthorizationUserRoles(ctx, saUser.ID)
	require.NoError(t, err)
	require.Contains(t, saRoles.Roles, wantSA)
	require.NotContains(t, saRoles.Roles, wantMember)
}

// TestGetAuthorizationUserRolesUnionsDefaultOrgMemberRoles verifies the
// resolve-at-read semantics for organizations.default_org_member_roles:
// every member's effective roles include the org's defaults, and changes
// to the column propagate on the next request. The union applies to
// regular users and to service accounts; the SQL array_cats the column
// for both code paths.
func TestGetAuthorizationUserRolesUnionsDefaultOrgMemberRoles(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	saUser := dbgen.User(t, db, database.User{IsServiceAccount: true})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         user.ID,
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         saUser.ID,
	})

	ctx := testutil.Context(t, testutil.WaitShort)

	// New orgs default to organization-workspace-access; both the regular
	// user's and the service account's effective roles must include the
	// scoped form.
	wantWorkspaceAccess := rbac.RoleOrgWorkspaceAccess() + ":" + org.ID.String()
	initial, err := db.GetAuthorizationUserRoles(ctx, user.ID)
	require.NoError(t, err)
	require.Contains(t, initial.Roles, wantWorkspaceAccess)
	initialSA, err := db.GetAuthorizationUserRoles(ctx, saUser.ID)
	require.NoError(t, err)
	require.Contains(t, initialSA.Roles, wantWorkspaceAccess)

	// Shrinking the org default to empty must immediately drop the role
	// from both effective sets.
	_, err = db.UpdateOrganization(ctx, database.UpdateOrganizationParams{
		ID:                    org.ID,
		UpdatedAt:             dbtime.Now(),
		Name:                  org.Name,
		DisplayName:           org.DisplayName,
		Description:           org.Description,
		Icon:                  org.Icon,
		DefaultOrgMemberRoles: []string{},
	})
	require.NoError(t, err)

	shrunk, err := db.GetAuthorizationUserRoles(ctx, user.ID)
	require.NoError(t, err)
	require.NotContains(t, shrunk.Roles, wantWorkspaceAccess)
	shrunkSA, err := db.GetAuthorizationUserRoles(ctx, saUser.ID)
	require.NoError(t, err)
	require.NotContains(t, shrunkSA.Roles, wantWorkspaceAccess)
}

func TestUpdateOrganizationWorkspaceSharingSettings(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	ctx := testutil.Context(t, testutil.WaitShort)

	updated, err := db.UpdateOrganizationWorkspaceSharingSettings(ctx, database.UpdateOrganizationWorkspaceSharingSettingsParams{
		ID:                       org.ID,
		ShareableWorkspaceOwners: database.ShareableWorkspaceOwnersNone,
		UpdatedAt:                dbtime.Now(),
	})
	require.NoError(t, err)
	require.Equal(t, database.ShareableWorkspaceOwnersNone, updated.ShareableWorkspaceOwners)

	got, err := db.GetOrganizationByID(ctx, org.ID)
	require.NoError(t, err)
	require.Equal(t, database.ShareableWorkspaceOwnersNone, got.ShareableWorkspaceOwners)
}

func TestDeleteWorkspaceACLsByOrganization(t *testing.T) {
	t.Parallel()

	t.Run("DeletesAll", func(t *testing.T) {
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

		err := db.DeleteWorkspaceACLsByOrganization(ctx, database.DeleteWorkspaceACLsByOrganizationParams{
			OrganizationID:         org1.ID,
			ExcludeServiceAccounts: false,
		})
		require.NoError(t, err)

		got1, err := db.GetWorkspaceByID(ctx, ws1.ID)
		require.NoError(t, err)
		require.Empty(t, got1.UserACL)
		require.Empty(t, got1.GroupACL)

		got2, err := db.GetWorkspaceByID(ctx, ws2.ID)
		require.NoError(t, err)
		require.NotEmpty(t, got2.UserACL)
	})

	t.Run("ExcludesServiceAccounts", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		regularUser := dbgen.User(t, db, database.User{})
		saUser := dbgen.User(t, db, database.User{IsServiceAccount: true})
		sharedUser := dbgen.User(t, db, database.User{})

		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         regularUser.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         saUser.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         sharedUser.ID,
		})

		regularWS := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        regularUser.ID,
			OrganizationID: org.ID,
			UserACL: database.WorkspaceACL{
				sharedUser.ID.String(): {
					Permissions: []policy.Action{policy.ActionRead},
				},
			},
		}).Do().Workspace

		saWS := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        saUser.ID,
			OrganizationID: org.ID,
			UserACL: database.WorkspaceACL{
				sharedUser.ID.String(): {
					Permissions: []policy.Action{policy.ActionRead},
				},
			},
		}).Do().Workspace

		ctx := testutil.Context(t, testutil.WaitShort)

		err := db.DeleteWorkspaceACLsByOrganization(ctx, database.DeleteWorkspaceACLsByOrganizationParams{
			OrganizationID:         org.ID,
			ExcludeServiceAccounts: true,
		})
		require.NoError(t, err)

		// Regular user workspace ACLs should be cleared.
		gotRegular, err := db.GetWorkspaceByID(ctx, regularWS.ID)
		require.NoError(t, err)
		require.Empty(t, gotRegular.UserACL)

		// Service account workspace ACLs should be preserved.
		gotSA, err := db.GetWorkspaceByID(ctx, saWS.ID)
		require.NoError(t, err)
		require.Equal(t, database.WorkspaceACL{
			sharedUser.ID.String(): {
				Permissions: []policy.Action{policy.ActionRead},
			},
		}, gotSA.UserACL)
	})
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

func TestBatchUpsertConnectionLogs(t *testing.T) {
	t.Parallel()

	createWorkspace := func(t *testing.T, db database.Store) database.WorkspaceTable {
		t.Helper()
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

	// zeroTime is the sentinel value that the SQL treats as "no
	// connect/disconnect time provided".
	zeroTime := time.Time{}

	defaultIP := pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IPv4(127, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}

	t.Run("SingleConnect", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		connectTime := dbtime.Now()

		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{connectTime},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{0},
			CodeValid:        []bool{false},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{""},
			DisconnectTime:   []time.Time{zeroTime},
		})
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.True(t, connectTime.Equal(rows[0].ConnectionLog.ConnectTime))
		require.False(t, rows[0].ConnectionLog.DisconnectTime.Valid,
			"disconnect_time should be NULL for a connect-only event")
	})

	t.Run("ConnectThenDisconnect", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		connectTime := dbtime.Now()

		// Insert connect.
		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{connectTime},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{0},
			CodeValid:        []bool{false},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{""},
			DisconnectTime:   []time.Time{zeroTime},
		})
		require.NoError(t, err)

		// Insert disconnect for same connection.
		disconnectTime := connectTime.Add(time.Second)
		err = db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{zeroTime},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{1},
			CodeValid:        []bool{true},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{"test disconnect"},
			DisconnectTime:   []time.Time{disconnectTime},
		})
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		row := rows[0].ConnectionLog
		require.True(t, connectTime.Equal(row.ConnectTime))
		require.True(t, row.DisconnectTime.Valid)
		require.True(t, disconnectTime.Equal(row.DisconnectTime.Time))
		require.Equal(t, "test disconnect", row.DisconnectReason.String)
		require.Equal(t, int32(1), row.Code.Int32)
	})

	t.Run("DuplicateConnectIsNoOp", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		connectTime := dbtime.Now()

		mkParams := func(ct time.Time, ip pqtype.Inet) database.BatchUpsertConnectionLogsParams {
			return database.BatchUpsertConnectionLogsParams{
				ID:               []uuid.UUID{uuid.New()},
				ConnectTime:      []time.Time{ct},
				OrganizationID:   []uuid.UUID{ws.OrganizationID},
				WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
				WorkspaceID:      []uuid.UUID{ws.ID},
				WorkspaceName:    []string{ws.Name},
				AgentName:        []string{"agent"},
				Type:             []database.ConnectionType{database.ConnectionTypeSsh},
				Code:             []int32{0},
				CodeValid:        []bool{false},
				Ip:               []pqtype.Inet{ip},
				UserAgent:        []string{""},
				UserID:           []uuid.UUID{uuid.Nil},
				SlugOrPort:       []string{""},
				ConnectionID:     []uuid.UUID{connID},
				DisconnectReason: []string{""},
				DisconnectTime:   []time.Time{zeroTime},
			}
		}

		err := db.BatchUpsertConnectionLogs(ctx, mkParams(connectTime, defaultIP))
		require.NoError(t, err)

		rows1, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows1, 1)

		// Second connect with later time and different IP.
		otherIP := pqtype.Inet{
			IPNet: net.IPNet{
				IP:   net.IPv4(10, 0, 0, 1),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			},
			Valid: true,
		}
		err = db.BatchUpsertConnectionLogs(ctx, mkParams(connectTime.Add(time.Second), otherIP))
		require.NoError(t, err)

		rows2, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows2, 1)

		// The LEAST logic should pick the earlier connect_time; IP and
		// other fields are not updated on conflict.
		require.True(t, connectTime.Equal(rows2[0].ConnectionLog.ConnectTime),
			"connect_time should remain the original (earlier) value")
	})

	t.Run("OrderIndependentConnectTime", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		disconnectTime := dbtime.Now()
		connectTime := disconnectTime.Add(-5 * time.Second)

		// Disconnect arrives first.
		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{disconnectTime},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{0},
			CodeValid:        []bool{true},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{"bye"},
			DisconnectTime:   []time.Time{disconnectTime},
		})
		require.NoError(t, err)

		// Connect arrives second with the real (earlier) connect_time.
		err = db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{connectTime},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{0},
			CodeValid:        []bool{false},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{""},
			DisconnectTime:   []time.Time{zeroTime},
		})
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.True(t, connectTime.Equal(rows[0].ConnectionLog.ConnectTime),
			"LEAST should pick the earlier connect_time")
	})

	t.Run("DisconnectFieldsAreWriteOnce", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		disconnectTime := dbtime.Now()

		mkDisconnect := func(reason string, code int32) database.BatchUpsertConnectionLogsParams {
			return database.BatchUpsertConnectionLogsParams{
				ID:               []uuid.UUID{uuid.New()},
				ConnectTime:      []time.Time{disconnectTime},
				OrganizationID:   []uuid.UUID{ws.OrganizationID},
				WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
				WorkspaceID:      []uuid.UUID{ws.ID},
				WorkspaceName:    []string{ws.Name},
				AgentName:        []string{"agent"},
				Type:             []database.ConnectionType{database.ConnectionTypeSsh},
				Code:             []int32{code},
				CodeValid:        []bool{true},
				Ip:               []pqtype.Inet{defaultIP},
				UserAgent:        []string{""},
				UserID:           []uuid.UUID{uuid.Nil},
				SlugOrPort:       []string{""},
				ConnectionID:     []uuid.UUID{connID},
				DisconnectReason: []string{reason},
				DisconnectTime:   []time.Time{disconnectTime},
			}
		}

		err := db.BatchUpsertConnectionLogs(ctx, mkDisconnect("first reason", 1))
		require.NoError(t, err)

		// Second disconnect with different reason and code.
		err = db.BatchUpsertConnectionLogs(ctx, mkDisconnect("second reason", 2))
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		row := rows[0].ConnectionLog
		require.Equal(t, "first reason", row.DisconnectReason.String,
			"disconnect_reason should not be overwritten")
		require.Equal(t, int32(1), row.Code.Int32,
			"code should not be overwritten")
	})

	t.Run("ConnectAfterDisconnectIsNoOp", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		disconnectTime := dbtime.Now()

		// Insert disconnect first.
		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{disconnectTime},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{42},
			CodeValid:        []bool{true},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{"server shutdown"},
			DisconnectTime:   []time.Time{disconnectTime},
		})
		require.NoError(t, err)

		rows1, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows1, 1)
		require.True(t, rows1[0].ConnectionLog.DisconnectTime.Valid)
		require.Equal(t, "server shutdown", rows1[0].ConnectionLog.DisconnectReason.String)
		require.Equal(t, int32(42), rows1[0].ConnectionLog.Code.Int32)

		// Insert connect for same connection_id.
		err = db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{disconnectTime.Add(time.Second)},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{0},
			CodeValid:        []bool{false},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{""},
			DisconnectTime:   []time.Time{zeroTime},
		})
		require.NoError(t, err)

		rows2, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows2, 1)
		row := rows2[0].ConnectionLog
		require.True(t, row.DisconnectTime.Valid,
			"disconnect_time should not be cleared by a later connect")
		require.Equal(t, "server shutdown", row.DisconnectReason.String,
			"disconnect_reason should not be cleared")
		require.Equal(t, int32(42), row.Code.Int32,
			"code should not be cleared")
	})

	t.Run("CodeZeroPreserved", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		now := dbtime.Now()

		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{now},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{0},
			CodeValid:        []bool{true},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{"normal"},
			DisconnectTime:   []time.Time{now},
		})
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.True(t, rows[0].ConnectionLog.Code.Valid, "code should be non-NULL")
		require.Equal(t, int32(0), rows[0].ConnectionLog.Code.Int32,
			"code=0 should be preserved, not treated as NULL")
	})

	t.Run("CodeNullWhenInvalid", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		connID := uuid.New()
		now := dbtime.Now()

		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               []uuid.UUID{uuid.New()},
			ConnectTime:      []time.Time{now},
			OrganizationID:   []uuid.UUID{ws.OrganizationID},
			WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
			WorkspaceID:      []uuid.UUID{ws.ID},
			WorkspaceName:    []string{ws.Name},
			AgentName:        []string{"agent"},
			Type:             []database.ConnectionType{database.ConnectionTypeSsh},
			Code:             []int32{99},
			CodeValid:        []bool{false},
			Ip:               []pqtype.Inet{defaultIP},
			UserAgent:        []string{""},
			UserID:           []uuid.UUID{uuid.Nil},
			SlugOrPort:       []string{""},
			ConnectionID:     []uuid.UUID{connID},
			DisconnectReason: []string{""},
			DisconnectTime:   []time.Time{zeroTime},
		})
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.False(t, rows[0].ConnectionLog.Code.Valid,
			"code should be NULL when code_valid is false")
	})

	t.Run("NullConnectionIDEvents", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		now := dbtime.Now()

		// Insert two web events with NULL connection_id (uuid.Nil →
		// NULL via NULLIF) for the same workspace/agent.
		for i := range 2 {
			err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
				ID:               []uuid.UUID{uuid.New()},
				ConnectTime:      []time.Time{now.Add(time.Duration(i) * time.Second)},
				OrganizationID:   []uuid.UUID{ws.OrganizationID},
				WorkspaceOwnerID: []uuid.UUID{ws.OwnerID},
				WorkspaceID:      []uuid.UUID{ws.ID},
				WorkspaceName:    []string{ws.Name},
				AgentName:        []string{"agent"},
				Type:             []database.ConnectionType{database.ConnectionTypeSsh},
				Code:             []int32{200},
				CodeValid:        []bool{true},
				Ip:               []pqtype.Inet{defaultIP},
				UserAgent:        []string{"Mozilla/5.0"},
				UserID:           []uuid.UUID{uuid.Nil},
				SlugOrPort:       []string{"web-terminal"},
				ConnectionID:     []uuid.UUID{uuid.Nil},
				DisconnectReason: []string{""},
				DisconnectTime:   []time.Time{zeroTime},
			})
			require.NoError(t, err)
		}

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, 2,
			"NULL connection_id rows should not conflict with each other")
	})

	t.Run("MultipleIndependentConnections", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()
		ws := createWorkspace(t, db)
		now := dbtime.Now()

		n := 5
		ids := make([]uuid.UUID, n)
		connectTimes := make([]time.Time, n)
		orgIDs := make([]uuid.UUID, n)
		ownerIDs := make([]uuid.UUID, n)
		wsIDs := make([]uuid.UUID, n)
		wsNames := make([]string, n)
		agentNames := make([]string, n)
		types := make([]database.ConnectionType, n)
		codes := make([]int32, n)
		codeValids := make([]bool, n)
		ips := make([]pqtype.Inet, n)
		userAgents := make([]string, n)
		userIDs := make([]uuid.UUID, n)
		slugOrPorts := make([]string, n)
		connIDs := make([]uuid.UUID, n)
		disconnectReasons := make([]string, n)
		disconnectTimes := make([]time.Time, n)

		for i := range n {
			ids[i] = uuid.New()
			connectTimes[i] = now.Add(time.Duration(i) * time.Second)
			orgIDs[i] = ws.OrganizationID
			ownerIDs[i] = ws.OwnerID
			wsIDs[i] = ws.ID
			wsNames[i] = ws.Name
			agentNames[i] = "agent"
			types[i] = database.ConnectionTypeSsh
			codes[i] = 0
			codeValids[i] = false
			ips[i] = defaultIP
			userAgents[i] = ""
			userIDs[i] = uuid.Nil
			slugOrPorts[i] = ""
			connIDs[i] = uuid.New()
			disconnectReasons[i] = ""
			disconnectTimes[i] = zeroTime
		}

		err := db.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
			ID:               ids,
			ConnectTime:      connectTimes,
			OrganizationID:   orgIDs,
			WorkspaceOwnerID: ownerIDs,
			WorkspaceID:      wsIDs,
			WorkspaceName:    wsNames,
			AgentName:        agentNames,
			Type:             types,
			Code:             codes,
			CodeValid:        codeValids,
			Ip:               ips,
			UserAgent:        userAgents,
			UserID:           userIDs,
			SlugOrPort:       slugOrPorts,
			ConnectionID:     connIDs,
			DisconnectReason: disconnectReasons,
			DisconnectTime:   disconnectTimes,
		})
		require.NoError(t, err)

		rows, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{LimitOpt: 10})
		require.NoError(t, err)
		require.Len(t, rows, n, "each unique connection_id should produce its own row")
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
		// Many daemons with identical tags should produce same results as one.
		{
			name: "duplicate-daemons-same-tags",
			jobTags: []database.StringMap{
				{"a": "1"},
				{"a": "1", "b": "2"},
			},
			daemonTags: []database.StringMap{
				{"a": "1", "b": "2"},
				{"a": "1", "b": "2"},
				{"a": "1", "b": "2"},
			},
			queueSizes:     []int64{2, 2},
			queuePositions: []int64{1, 2},
		},
		// Jobs that don't match any queried job's daemon should still
		// have correct queue positions.
		{
			name: "irrelevant-daemons-filtered",
			jobTags: []database.StringMap{
				{"a": "1"},
				{"x": "9"},
			},
			daemonTags: []database.StringMap{
				{"a": "1"},
				{"x": "9"},
			},
			queueSizes:     []int64{1},
			queuePositions: []int64{1},
			skipJobIDs:     map[int]struct{}{1: {}},
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

func TestGetProvisionerJobsByIDsWithQueuePosition_DuplicateDaemons(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	now := dbtime.Now()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Create 3 pending jobs with the same tags.
	jobs := make([]database.ProvisionerJob, 3)
	for i := range jobs {
		jobs[i] = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: now.Add(-time.Duration(3-i) * time.Minute),
			Tags:      database.StringMap{"scope": "organization", "owner": ""},
		})
	}

	// Create 50 daemons with identical tags (simulates scale).
	for i := range 50 {
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			Name:         fmt.Sprintf("daemon_%d", i),
			Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags:         database.StringMap{"scope": "organization", "owner": ""},
		})
	}

	jobIDs := make([]uuid.UUID, len(jobs))
	for i, j := range jobs {
		jobIDs[i] = j.ID
	}

	results, err := db.GetProvisionerJobsByIDsWithQueuePosition(ctx,
		database.GetProvisionerJobsByIDsWithQueuePositionParams{
			IDs:             jobIDs,
			StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
		})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// All daemons have identical tags, so queue should be same as
	// if there were just one daemon.
	for i, r := range results {
		assert.Equal(t, int64(3), r.QueueSize, "job %d queue size", i)
		assert.Equal(t, int64(i+1), r.QueuePosition, "job %d queue position", i)
	}
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
	}, slice.List(userGroups, onlyGroupIDs))

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
	}, slice.List(userGroups, onlyGroupIDs))

	// Verify extra user is unchanged
	extraUserGroups, err := db.GetGroups(ctx, database.GetGroupsParams{
		HasMemberID: extra.ID,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{
		orgA.ID, orgB.ID, // Everyone groups
		groupA1.ID, groupA2.ID, groupB1.ID, groupB2.ID, // Org groups
	}, slice.List(extraUserGroups, onlyGroupIDs))
}

func TestGetUserStatusCounts(t *testing.T) {
	t.Parallel()

	type testCase struct {
		timezone    string
		location    *time.Location
		reportFrom  time.Time
		reportUntil time.Time
	}
	testCases := []testCase{}

	// GetUserStatusCounts is sensitive to DST transitions, because it generates timestamps exactly
	// one day apart from one another, and specific days can have varying lengths depending on the timezone.
	// Therefore, we test with a variety of timezones.
	timezones := []string{
		"America/St_Johns",
		"Africa/Johannesburg",
		"America/New_York",
		"Europe/London",
		"Asia/Tokyo",
		"Australia/Sydney",
	}

	// assemble test cases
	for _, tz := range timezones {
		location, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatalf("failed to load location: %v", err)
		}

		// Testing based on the current system date will flake due to DST transitions.
		// Instead, we test with a fixed range of dates that is large enough to span multiple DST transitions.
		startOfTestDateRange := time.Date(2025, 1, 1, 0, 0, 0, 0, location)
		endOfTestDateRange := time.Date(2026, 1, 1, 0, 0, 0, 0, location)
		// To keep the number of test cases manageable given the large date range,
		// we test with a suitable large interval. This interval is also the length of each report.
		// this ensures we have full coverage of the date range.
		testDateRangeInterval := 60

		for reportFrom := startOfTestDateRange; !reportFrom.After(endOfTestDateRange); reportFrom = reportFrom.AddDate(0, 0, testDateRangeInterval) {
			testCases = append(testCases, testCase{
				timezone:    tz,
				location:    location,
				reportFrom:  dbtime.Time(reportFrom),
				reportUntil: dbtime.Time(reportFrom.AddDate(0, 0, testDateRangeInterval)),
			})
		}
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s/%s", tc.timezone, tc.reportUntil.Format("2006-01-02T15:04:05Z")), func(t *testing.T) {
			t.Parallel()

			userCreatedAt := tc.reportUntil.AddDate(0, 0, -60)
			firstStatusChange := userCreatedAt.AddDate(0, 0, 29)
			secondStatusChange := firstStatusChange.AddDate(0, 0, 29)

			t.Run("No Users", func(t *testing.T) {
				t.Parallel()
				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				counts, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					Tz:        tc.timezone,
					StartTime: tc.reportFrom,
					EndTime:   tc.reportUntil,
				})
				require.NoError(t, err)
				require.Empty(t, counts, "should return no results when there are no users")
			})

			t.Run("One User/Creation Only", func(t *testing.T) {
				t.Parallel()

				subTestCases := []struct {
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

				for _, stc := range subTestCases {
					t.Run(stc.name, func(t *testing.T) {
						t.Parallel()
						db, _ := dbtestutil.NewDB(t)
						ctx := testutil.Context(t, testutil.WaitShort)

						dbgen.User(t, db, database.User{
							Status:    stc.status,
							CreatedAt: userCreatedAt,
							UpdatedAt: userCreatedAt,
						})

						startTime := dbtime.StartOfDay(userCreatedAt)
						endTime := dbtime.StartOfDay(tc.reportUntil)
						userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
							Tz:        tc.timezone,
							StartTime: startTime,
							EndTime:   endTime,
						})
						require.NoError(t, err)

						numDays := 0
						for d := startTime; !d.After(endTime); d = d.AddDate(0, 0, 1) {
							numDays++
						}
						assert.Len(
							t,
							userStatusChanges,
							numDays,
							"should have 1 entry per day between the start and end time, including the end time",
						)

						for i, row := range userStatusChanges {
							require.Equal(t, stc.status, row.Status, "should have the correct status")

							rowDate := row.Date.In(tc.location)
							expectedDate := dbtime.StartOfDay(userCreatedAt).AddDate(0, 0, i)
							assert.True(
								t,
								rowDate.Equal(expectedDate),
								"expected date %s, but got %s for row %n",
								expectedDate.String(),
								rowDate.String(),
								i,
							)

							if row.Date.Before(userCreatedAt) {
								assert.Equal(t, int64(0), row.Count, "should have 0 users before creation")
							} else {
								assert.Equal(t, int64(1), row.Count, "should have 1 user after creation")
							}
						}
					})
				}
			})

			t.Run("One User/One Transition", func(t *testing.T) {
				t.Parallel()

				subTestCases := []struct {
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
							userCreatedAt: {
								database.UserStatusActive:  1,
								database.UserStatusDormant: 0,
							},
							firstStatusChange: {
								database.UserStatusDormant: 1,
								database.UserStatusActive:  0,
							},
							tc.reportUntil: {
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
							userCreatedAt: {
								database.UserStatusActive:    1,
								database.UserStatusSuspended: 0,
							},
							firstStatusChange: {
								database.UserStatusSuspended: 1,
								database.UserStatusActive:    0,
							},
							tc.reportUntil: {
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
							userCreatedAt: {
								database.UserStatusDormant: 1,
								database.UserStatusActive:  0,
							},
							firstStatusChange: {
								database.UserStatusActive:  1,
								database.UserStatusDormant: 0,
							},
							tc.reportUntil: {
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
							userCreatedAt: {
								database.UserStatusDormant:   1,
								database.UserStatusSuspended: 0,
							},
							firstStatusChange: {
								database.UserStatusSuspended: 1,
								database.UserStatusDormant:   0,
							},
							tc.reportUntil: {
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
							userCreatedAt: {
								database.UserStatusSuspended: 1,
								database.UserStatusActive:    0,
							},
							firstStatusChange: {
								database.UserStatusActive:    1,
								database.UserStatusSuspended: 0,
							},
							tc.reportUntil: {
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
							userCreatedAt: {
								database.UserStatusSuspended: 1,
								database.UserStatusDormant:   0,
							},
							firstStatusChange: {
								database.UserStatusDormant:   1,
								database.UserStatusSuspended: 0,
							},
							tc.reportUntil: {
								database.UserStatusDormant:   1,
								database.UserStatusSuspended: 0,
							},
						},
					},
				}

				for _, stc := range subTestCases {
					t.Run(stc.name, func(t *testing.T) {
						t.Parallel()
						db, _ := dbtestutil.NewDB(t)
						ctx := testutil.Context(t, testutil.WaitShort)

						user := dbgen.User(t, db, database.User{
							Status:    stc.initialStatus,
							CreatedAt: userCreatedAt,
							UpdatedAt: userCreatedAt,
						})

						user, err := db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
							ID:        user.ID,
							Status:    stc.targetStatus,
							UpdatedAt: firstStatusChange,
						})
						require.NoError(t, err)

						userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
							Tz:        tc.timezone,
							StartTime: dbtime.StartOfDay(userCreatedAt),
							EndTime:   dbtime.StartOfDay(tc.reportUntil),
						})
						require.NoError(t, err)

						for i, row := range userStatusChanges {
							rowDate := row.Date.In(tc.location)
							expectedDate := dbtime.StartOfDay(userCreatedAt).AddDate(0, 0, i/2)
							require.True(
								t,
								rowDate.Equal(expectedDate),
								"expected date %s, but got %s for row %n",
								expectedDate.String(),
								rowDate.String(),
								i,
							)
							switch {
							case row.Date.Before(userCreatedAt):
								require.Equal(t, int64(0), row.Count)
							case row.Date.Before(firstStatusChange):
								if row.Status == stc.initialStatus {
									require.Equal(t, int64(1), row.Count)
								} else if row.Status == stc.targetStatus {
									require.Equal(t, int64(0), row.Count)
								}
							case !row.Date.After(tc.reportUntil):
								if row.Status == stc.initialStatus {
									require.Equal(t, int64(0), row.Count)
								} else if row.Status == stc.targetStatus {
									require.Equal(t, int64(1), row.Count)
								}
							default:
								t.Errorf("date %q beyond expected range end %q", row.Date, tc.reportUntil)
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

				subTestCases := []testCase{
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

				for _, stc := range subTestCases {
					t.Run(stc.name, func(t *testing.T) {
						t.Parallel()

						db, _ := dbtestutil.NewDB(t)
						ctx := testutil.Context(t, testutil.WaitShort)

						user1 := dbgen.User(t, db, database.User{
							Status:    stc.user1Transition.from,
							CreatedAt: userCreatedAt,
							UpdatedAt: userCreatedAt,
						})
						user2 := dbgen.User(t, db, database.User{
							Status:    stc.user2Transition.from,
							CreatedAt: userCreatedAt,
							UpdatedAt: userCreatedAt,
						})

						user1, err := db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
							ID:        user1.ID,
							Status:    stc.user1Transition.to,
							UpdatedAt: firstStatusChange,
						})
						require.NoError(t, err)

						user2, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
							ID:        user2.ID,
							Status:    stc.user2Transition.to,
							UpdatedAt: secondStatusChange,
						})
						require.NoError(t, err)

						userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
							Tz:        tc.timezone,
							StartTime: dbtime.StartOfDay(userCreatedAt),
							EndTime:   dbtime.StartOfDay(tc.reportUntil),
						})
						require.NoError(t, err)
						require.NotEmpty(t, userStatusChanges)
						gotCounts := map[time.Time]map[database.UserStatus]int64{}
						for _, row := range userStatusChanges {
							dateInLocation := row.Date.In(tc.location)
							if gotCounts[dateInLocation] == nil {
								gotCounts[dateInLocation] = map[database.UserStatus]int64{}
							}
							gotCounts[dateInLocation][row.Status] = row.Count
						}

						expectedCounts := map[time.Time]map[database.UserStatus]int64{}
						for d := dbtime.StartOfDay(userCreatedAt); !d.After(dbtime.StartOfDay(tc.reportUntil)); d = d.AddDate(0, 0, 1) {
							expectedCounts[d] = map[database.UserStatus]int64{}

							// Default values
							expectedCounts[d][stc.user1Transition.from] = 0
							expectedCounts[d][stc.user1Transition.to] = 0
							expectedCounts[d][stc.user2Transition.from] = 0
							expectedCounts[d][stc.user2Transition.to] = 0

							// Counted Values
							switch {
							case d.Before(userCreatedAt):
								continue
							case d.Before(firstStatusChange):
								expectedCounts[d][stc.user1Transition.from]++
								expectedCounts[d][stc.user2Transition.from]++
							case d.Before(secondStatusChange):
								expectedCounts[d][stc.user1Transition.to]++
								expectedCounts[d][stc.user2Transition.from]++
							case !d.After(tc.reportUntil):
								expectedCounts[d][stc.user1Transition.to]++
								expectedCounts[d][stc.user2Transition.to]++
							default:
								t.Fatalf("date %q beyond expected range end %q", d, tc.reportUntil)
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
					CreatedAt: userCreatedAt,
					UpdatedAt: userCreatedAt,
				})

				userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					Tz:        tc.timezone,
					StartTime: dbtime.StartOfDay(userCreatedAt.Add(time.Hour * 24)),
					EndTime:   dbtime.StartOfDay(tc.reportUntil),
				})
				require.NoError(t, err)

				for i, row := range userStatusChanges {
					require.True(
						t,
						row.Date.In(tc.location).Equal(dbtime.StartOfDay(userCreatedAt).AddDate(0, 0, 1+i)),
						"expected date %s, but got %s for row %n",
						dbtime.StartOfDay(userCreatedAt).AddDate(0, 0, 1+i),
						row.Date.In(tc.location).String(),
						i,
					)
					require.Equal(t, database.UserStatusActive, row.Status)
					require.Equal(t, int64(1), row.Count)
				}
			})

			t.Run("User deleted before query range", func(t *testing.T) {
				t.Parallel()
				db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				user := dbgen.User(t, db, database.User{
					Status:    database.UserStatusActive,
					CreatedAt: userCreatedAt,
					UpdatedAt: userCreatedAt,
				})

				err := db.UpdateUserDeletedByID(ctx, user.ID)
				require.NoError(t, err)

				_, err = sqlDB.ExecContext(ctx, "UPDATE user_deleted SET deleted_at = $1 WHERE user_id = $2", tc.reportUntil, user.ID)
				require.NoError(t, err)

				userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					Tz:        tc.timezone,
					StartTime: tc.reportUntil.Add(time.Hour * 24),
					EndTime:   tc.reportUntil.Add(time.Hour * 48),
				})
				require.NoError(t, err)
				require.Empty(t, userStatusChanges)
			})

			t.Run("User deleted during query range", func(t *testing.T) {
				t.Parallel()

				db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)

				user := dbgen.User(t, db, database.User{
					Status:    database.UserStatusActive,
					CreatedAt: userCreatedAt,
					UpdatedAt: userCreatedAt,
				})

				err := db.UpdateUserDeletedByID(ctx, user.ID)
				require.NoError(t, err)

				_, err = sqlDB.ExecContext(ctx, "UPDATE user_deleted SET deleted_at = $1 WHERE user_id = $2", tc.reportUntil, user.ID)
				require.NoError(t, err)

				userStatusChanges, err := db.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
					Tz:        tc.timezone,
					StartTime: dbtime.StartOfDay(userCreatedAt),
					EndTime:   dbtime.StartOfDay(tc.reportUntil.Add(time.Hour * 24)),
				})
				require.NoError(t, err)
				for i, row := range userStatusChanges {
					row.Date = row.Date.In(tc.location)
					userStatusChanges[i] = row
					target := dbtime.StartOfDay(userCreatedAt).AddDate(0, 0, i)
					assert.True(
						t,
						row.Date.Equal(target),
						"expected date %s, but got %s for row %n",
						target.String(),
						row.Date.String(),
						i,
					)
					require.Equal(t, database.UserStatusActive, row.Status)
					switch {
					case row.Date.Before(userCreatedAt):
						require.Equal(t, int64(0), row.Count)
					case !row.Date.Before(tc.reportUntil):
						// On or after the deletion date, the user should not be counted.
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

func TestUpsertWorkspaceAppCannotRebindAcrossWorkspaces(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	ctx := testutil.Context(t, testutil.WaitShort)

	// createWorkspace builds the owner -> template -> version -> workspace chain
	// and returns the workspace plus its template version so callers can create
	// additional builds (and thus agents) within the same workspace.
	createWorkspace := func(t *testing.T) (database.WorkspaceTable, uuid.UUID) {
		t.Helper()
		user := dbgen.User(t, db, database.User{})
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			TemplateID:     template.ID,
			OwnerID:        user.ID,
		})
		return workspace, version.ID
	}

	// addAgent creates a build, resource, and agent for the workspace. The
	// build's JobID matches the resource's JobID so the upsert's
	// agent -> resource -> workspace_builds(job_id) -> workspace_id traversal
	// resolves to the workspace.
	addAgent := func(t *testing.T, workspace database.WorkspaceTable, versionID uuid.UUID, buildNumber int32) database.WorkspaceAgent {
		t.Helper()
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			BuildNumber:       buildNumber,
			JobID:             job.ID,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: versionID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		return dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
	}

	upsertApp := func(appID, agentID uuid.UUID, slug string) (database.WorkspaceApp, error) {
		return db.UpsertWorkspaceApp(ctx, database.UpsertWorkspaceAppParams{
			ID:           appID,
			CreatedAt:    dbtime.Now(),
			AgentID:      agentID,
			Slug:         slug,
			DisplayName:  "Code Server",
			Icon:         "/icon.png",
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
			OpenIn:       database.WorkspaceAppOpenInSlimWindow,
		})
	}

	// Given: two independent workspaces, each with an agent that resolves to its
	// own workspace.
	workspaceA, versionA := createWorkspace(t)
	workspaceB, versionB := createWorkspace(t)
	agentA := addAgent(t, workspaceA, versionA, 1)
	agentB := addAgent(t, workspaceB, versionB, 1)

	gotA, err := db.GetWorkspaceByAgentID(ctx, agentA.ID)
	require.NoError(t, err)
	require.Equal(t, workspaceA.ID, gotA.ID)
	gotB, err := db.GetWorkspaceByAgentID(ctx, agentB.ID)
	require.NoError(t, err)
	require.Equal(t, workspaceB.ID, gotB.ID)

	appID := uuid.New()
	const originalSlug = "code-server"

	// Initial insert under workspace A's agent succeeds (no conflict).
	app, err := upsertApp(appID, agentA.ID, originalSlug)
	require.NoError(t, err)
	require.Equal(t, appID, app.ID)
	require.Equal(t, agentA.ID, app.AgentID)
	require.Equal(t, originalSlug, app.Slug)

	// Upserting the same app id onto workspace B's agent is rejected because the
	// existing row and the incoming agent resolve to different workspaces. The
	// guard updates zero rows, so the :one query returns sql.ErrNoRows.
	_, err = upsertApp(appID, agentB.ID, "hijacked")
	require.ErrorIs(t, err, sql.ErrNoRows)

	// The app remains bound to workspace A's agent, unchanged.
	appsA, err := db.GetWorkspaceAppsByAgentID(ctx, agentA.ID)
	require.NoError(t, err)
	require.Len(t, appsA, 1)
	require.Equal(t, appID, appsA[0].ID)
	require.Equal(t, agentA.ID, appsA[0].AgentID)
	require.Equal(t, originalSlug, appsA[0].Slug)

	// Workspace B's agent has no app.
	appsB, err := db.GetWorkspaceAppsByAgentID(ctx, agentB.ID)
	require.NoError(t, err)
	require.Empty(t, appsB)

	// A legitimate rebuild of workspace A produces a new agent (agent IDs are
	// regenerated every build). Rebinding the persistent app to it succeeds
	// because both agents resolve to workspace A.
	agentA2 := addAgent(t, workspaceA, versionA, 2)
	app, err = upsertApp(appID, agentA2.ID, "code-server-v2")
	require.NoError(t, err)
	require.Equal(t, agentA2.ID, app.AgentID)
	require.Equal(t, "code-server-v2", app.Slug)

	appsA2, err := db.GetWorkspaceAppsByAgentID(ctx, agentA2.ID)
	require.NoError(t, err)
	require.Len(t, appsA2, 1)
	require.Equal(t, appID, appsA2[0].ID)

	// Set up a template-import agent. It is intentionally not associated with
	// a workspace build, so it resolves to no workspace.
	importJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		OrganizationID: org.ID,
	})
	importResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: importJob.ID,
	})
	importAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: importResource.ID,
	})
	_, err = db.GetWorkspaceByAgentID(ctx, importAgent.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "import agent must not resolve to a workspace")

	// An app that already belongs to a workspace cannot be rebound to a
	// template-import agent. Otherwise a second update could move it from
	// the import agent to a different workspace.
	_, err = upsertApp(appID, importAgent.ID, "hijacked-by-import")
	require.ErrorIs(t, err, sql.ErrNoRows)

	appsA2, err = db.GetWorkspaceAppsByAgentID(ctx, agentA2.ID)
	require.NoError(t, err)
	require.Len(t, appsA2, 1)
	require.Equal(t, appID, appsA2[0].ID)
	require.Equal(t, agentA2.ID, appsA2[0].AgentID)
	require.Equal(t, "code-server-v2", appsA2[0].Slug)

	appsImport, err := db.GetWorkspaceAppsByAgentID(ctx, importAgent.ID)
	require.NoError(t, err)
	require.Empty(t, appsImport)

	_, err = upsertApp(appID, agentB.ID, "hijacked-after-import")
	require.ErrorIs(t, err, sql.ErrNoRows)

	unownedAppID := uuid.New()
	_, err = upsertApp(unownedAppID, importAgent.ID, "import-app")
	require.NoError(t, err)

	// An app whose existing agent belongs to a template-import job resolves to
	// no workspace, so rebinding it is permitted. It is not a cross-tenant
	// victim.
	rebound, err := upsertApp(unownedAppID, agentA.ID, "import-app")
	require.NoError(t, err)
	require.Equal(t, agentA.ID, rebound.AgentID)
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

func setupWorkspaceAgentQueryResources(t *testing.T, db database.Store, count int) []database.WorkspaceResource {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		OrganizationID: org.ID,
	})

	resources := make([]database.WorkspaceResource, 0, count)
	for i := 0; i < count; i++ {
		resources = append(resources, dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		}))
	}

	return resources
}

func markWorkspaceAgentDeleted(ctx context.Context, t *testing.T, sqlDB *sql.DB, agentID uuid.UUID) {
	t.Helper()

	_, err := sqlDB.ExecContext(ctx, "UPDATE workspace_agents SET deleted = TRUE WHERE id = $1", agentID)
	require.NoError(t, err)
}

type workspaceBuildAgentQueryFixture struct {
	Workspace database.WorkspaceTable
	Build     database.WorkspaceBuild
	Agent     database.WorkspaceAgent
}

func setupWorkspaceBuildAgentQueryWorkspace(t testing.TB, db database.Store, deleted bool) database.WorkspaceTable {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	return dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		Deleted:        deleted,
	})
}

func setupWorkspaceBuildAgentQueryFixture(
	t testing.TB,
	db database.Store,
	authInstanceID string,
	name string,
	createdAt time.Time,
	workspace database.WorkspaceTable,
) workspaceBuildAgentQueryFixture {
	t.Helper()

	if workspace.ID == uuid.Nil {
		workspace = setupWorkspaceBuildAgentQueryWorkspace(t, db, false)
	}
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: workspace.TemplateID, Valid: true},
		OrganizationID: workspace.OrganizationID,
		CreatedBy:      workspace.OwnerID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: workspace.OrganizationID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.ID,
		JobID:             job.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		Name:       name,
		ResourceID: resource.ID,
		CreatedAt:  createdAt,
		AuthInstanceID: sql.NullString{
			String: authInstanceID,
			Valid:  true,
		},
	})

	return workspaceBuildAgentQueryFixture{
		Workspace: workspace,
		Build:     build,
		Agent:     agent,
	}
}

func setupProvisionerJobAgentQueryFixture(
	t testing.TB,
	db database.Store,
	authInstanceID string,
	name string,
	createdAt time.Time,
	jobType database.ProvisionerJobType,
) database.WorkspaceAgent {
	t.Helper()

	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type: jobType,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	return dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		Name:       name,
		ResourceID: resource.ID,
		CreatedAt:  createdAt,
		AuthInstanceID: sql.NullString{
			String: authInstanceID,
			Valid:  true,
		},
	})
}

func TestGetWorkspaceAgentsByInstanceID(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsAllMatchingRootAgents", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		resources := setupWorkspaceAgentQueryResources(t, db, 2)
		authInstanceID := fmt.Sprintf("instance-%s-%d", t.Name(), time.Now().UnixNano())
		olderCreatedAt := dbtime.Now().Add(-time.Hour)
		newerCreatedAt := dbtime.Now()

		olderAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resources[0].ID,
			CreatedAt:  olderCreatedAt,
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})
		newerAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resources[1].ID,
			CreatedAt:  newerCreatedAt,
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})

		ctx := testutil.Context(t, testutil.WaitShort)

		agents, err := db.GetWorkspaceAgentsByInstanceID(ctx, authInstanceID)
		require.NoError(t, err)
		require.Len(t, agents, 2)
		assert.Equal(t, []uuid.UUID{newerAgent.ID, olderAgent.ID}, []uuid.UUID{agents[0].ID, agents[1].ID})
	})

	t.Run("ExcludesDeletedAndSubAgents", func(t *testing.T) {
		t.Parallel()

		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		resources := setupWorkspaceAgentQueryResources(t, db, 2)
		authInstanceID := fmt.Sprintf("instance-%s-%d", t.Name(), time.Now().UnixNano())
		baseCreatedAt := dbtime.Now()

		rootAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resources[0].ID,
			CreatedAt:  baseCreatedAt.Add(-time.Hour),
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})
		_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ParentID:   uuid.NullUUID{UUID: rootAgent.ID, Valid: true},
			ResourceID: resources[0].ID,
			CreatedAt:  baseCreatedAt,
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})
		deletedRootAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resources[1].ID,
			CreatedAt:  baseCreatedAt.Add(time.Minute),
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		markWorkspaceAgentDeleted(ctx, t, sqlDB, deletedRootAgent.ID)

		agents, err := db.GetWorkspaceAgentsByInstanceID(ctx, authInstanceID)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		assert.Equal(t, rootAgent.ID, agents[0].ID)
		assert.False(t, agents[0].ParentID.Valid)
	})

	t.Run("OrdersNewestFirst", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		resources := setupWorkspaceAgentQueryResources(t, db, 2)
		authInstanceID := fmt.Sprintf("instance-%s-%d", t.Name(), time.Now().UnixNano())
		olderCreatedAt := dbtime.Now().Add(-time.Hour)
		newerCreatedAt := dbtime.Now()

		olderAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resources[0].ID,
			CreatedAt:  olderCreatedAt,
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})
		newerAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resources[1].ID,
			CreatedAt:  newerCreatedAt,
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})

		ctx := testutil.Context(t, testutil.WaitShort)

		agents, err := db.GetWorkspaceAgentsByInstanceID(ctx, authInstanceID)
		require.NoError(t, err)
		require.Len(t, agents, 2)
		assert.Equal(t, newerAgent.ID, agents[0].ID)
		assert.Equal(t, olderAgent.ID, agents[1].ID)
	})
}

func TestGetWorkspaceBuildAgentsByInstanceID(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsWorkspaceBuildRootAgentsNewestFirst", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		authInstanceID := fmt.Sprintf("instance-%s-%d", t.Name(), time.Now().UnixNano())
		olderCreatedAt := dbtime.Now().Add(-time.Hour)
		newerCreatedAt := dbtime.Now()

		older := setupWorkspaceBuildAgentQueryFixture(t, db, authInstanceID, "older", olderCreatedAt, database.WorkspaceTable{})
		newer := setupWorkspaceBuildAgentQueryFixture(t, db, authInstanceID, "newer", newerCreatedAt, database.WorkspaceTable{})

		ctx := testutil.Context(t, testutil.WaitShort)

		agents, err := db.GetWorkspaceBuildAgentsByInstanceID(ctx, authInstanceID)
		require.NoError(t, err)
		require.Len(t, agents, 2)
		assert.Equal(t, []uuid.UUID{newer.Agent.ID, older.Agent.ID}, []uuid.UUID{agents[0].WorkspaceAgent.ID, agents[1].WorkspaceAgent.ID})
		assert.Equal(t, []uuid.UUID{newer.Build.ID, older.Build.ID}, []uuid.UUID{agents[0].WorkspaceBuildID, agents[1].WorkspaceBuildID})
		assert.Equal(t, newer.Workspace.ID, agents[0].WorkspaceTable.ID)
		assert.Equal(t, older.Workspace.ID, agents[1].WorkspaceTable.ID)
		assert.Equal(t, newer.Workspace.OwnerID, agents[0].WorkspaceTable.OwnerID)
		assert.Equal(t, older.Workspace.OwnerID, agents[1].WorkspaceTable.OwnerID)
		assert.Equal(t, newer.Workspace.OrganizationID, agents[0].WorkspaceTable.OrganizationID)
		assert.Equal(t, older.Workspace.OrganizationID, agents[1].WorkspaceTable.OrganizationID)
		assert.False(t, agents[0].WorkspaceTable.Deleted)
		assert.False(t, agents[1].WorkspaceTable.Deleted)
	})

	t.Run("ExcludesDeletedAgentsSubAgentsAndNonWorkspaceBuildJobs", func(t *testing.T) {
		t.Parallel()

		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		authInstanceID := fmt.Sprintf("instance-%s-%d", t.Name(), time.Now().UnixNano())
		baseCreatedAt := dbtime.Now()

		root := setupWorkspaceBuildAgentQueryFixture(t, db, authInstanceID, "root", baseCreatedAt.Add(-time.Hour), database.WorkspaceTable{})
		_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ParentID:   uuid.NullUUID{UUID: root.Agent.ID, Valid: true},
			Name:       "sub",
			ResourceID: root.Agent.ResourceID,
			CreatedAt:  baseCreatedAt.Add(time.Minute),
			AuthInstanceID: sql.NullString{
				String: authInstanceID,
				Valid:  true,
			},
		})
		deletedAgent := setupWorkspaceBuildAgentQueryFixture(t, db, authInstanceID, "deleted", baseCreatedAt.Add(2*time.Minute), database.WorkspaceTable{})
		_ = setupProvisionerJobAgentQueryFixture(t, db, authInstanceID, "template-import", baseCreatedAt.Add(3*time.Minute), database.ProvisionerJobTypeTemplateVersionImport)
		_ = setupProvisionerJobAgentQueryFixture(t, db, authInstanceID, "dry-run", baseCreatedAt.Add(4*time.Minute), database.ProvisionerJobTypeTemplateVersionDryRun)

		ctx := testutil.Context(t, testutil.WaitShort)
		markWorkspaceAgentDeleted(ctx, t, sqlDB, deletedAgent.Agent.ID)

		agents, err := db.GetWorkspaceBuildAgentsByInstanceID(ctx, authInstanceID)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		assert.Equal(t, root.Agent.ID, agents[0].WorkspaceAgent.ID)
		assert.False(t, agents[0].WorkspaceAgent.ParentID.Valid)
		assert.Equal(t, root.Build.ID, agents[0].WorkspaceBuildID)
	})

	t.Run("ExcludesDeletedWorkspaces", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		authInstanceID := fmt.Sprintf("instance-%s-%d", t.Name(), time.Now().UnixNano())
		baseCreatedAt := dbtime.Now()
		active := setupWorkspaceBuildAgentQueryFixture(t, db, authInstanceID, "active", baseCreatedAt, database.WorkspaceTable{})
		deletedWorkspace := setupWorkspaceBuildAgentQueryWorkspace(t, db, true)
		_ = setupWorkspaceBuildAgentQueryFixture(t, db, authInstanceID, "deleted-workspace", baseCreatedAt.Add(time.Minute), deletedWorkspace)

		ctx := testutil.Context(t, testutil.WaitShort)

		agents, err := db.GetWorkspaceBuildAgentsByInstanceID(ctx, authInstanceID)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		assert.Equal(t, active.Agent.ID, agents[0].WorkspaceAgent.ID)
		assert.Equal(t, active.Workspace.ID, agents[0].WorkspaceTable.ID)
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

		// 2. READ by UserID and Name
		readByNameParams := database.GetUserSecretByUserIDAndNameParams{
			UserID: testUser.ID,
			Name:   "workflow-secret",
		}
		readByNameSecret, err := db.GetUserSecretByUserIDAndName(ctx, readByNameParams)
		require.NoError(t, err)
		assert.Equal(t, createdSecret.ID, readByNameSecret.ID)
		assert.Equal(t, "workflow-secret", readByNameSecret.Name)

		// 3. LIST (metadata only)
		secrets, err := db.ListUserSecrets(ctx, testUser.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 1)
		assert.Equal(t, createdSecret.ID, secrets[0].ID)

		// 4. LIST with values
		secretsWithValues, err := db.ListUserSecretsWithValues(ctx, testUser.ID)
		require.NoError(t, err)
		require.Len(t, secretsWithValues, 1)
		assert.Equal(t, "workflow-value", secretsWithValues[0].Value)

		// 5. UPDATE (partial - only description)
		updateParams := database.UpdateUserSecretByUserIDAndNameParams{
			UserID:            testUser.ID,
			Name:              "workflow-secret",
			UpdateDescription: true,
			Description:       "Updated workflow description",
		}

		updatedSecret, err := db.UpdateUserSecretByUserIDAndName(ctx, updateParams)
		require.NoError(t, err)
		assert.Equal(t, "Updated workflow description", updatedSecret.Description)
		assert.Equal(t, "workflow-value", updatedSecret.Value) // Value unchanged
		assert.Equal(t, "WORKFLOW_ENV", updatedSecret.EnvName) // EnvName unchanged

		// 6. DELETE
		_, err = db.DeleteUserSecretByUserIDAndName(ctx, database.DeleteUserSecretByUserIDAndNameParams{
			UserID: testUser.ID,
			Name:   "workflow-secret",
		})
		require.NoError(t, err)

		// Verify deletion
		_, err = db.GetUserSecretByUserIDAndName(ctx, readByNameParams)
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
		_, err = db.GetUserSecretByUserIDAndName(ctx, database.GetUserSecretByUserIDAndNameParams{
			UserID: testUser.ID, Name: secret1.Name,
		})
		require.NoError(t, err)
		_, err = db.GetUserSecretByUserIDAndName(ctx, database.GetUserSecretByUserIDAndNameParams{
			UserID: testUser.ID, Name: secret2.Name,
		})
		require.NoError(t, err)
	})
}

// TestUserSecretsSoftDeleteTrigger verifies that a user's secrets
// are deleted when the user is soft-deleted.
func TestUserSecretsSoftDeleteTrigger(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	// userA will be soft-deleted.
	userA := dbgen.User(t, db, database.User{})
	secretA1 := dbgen.UserSecret(t, db, database.UserSecret{
		UserID:   userA.ID,
		Name:     "secret-a-1",
		Value:    "value-a-1",
		EnvName:  "SECRET_A_1",
		FilePath: "/secrets/a/1",
	})
	secretA2 := dbgen.UserSecret(t, db, database.UserSecret{
		UserID:   userA.ID,
		Name:     "secret-a-2",
		Value:    "value-a-2",
		EnvName:  "SECRET_A_2",
		FilePath: "/secrets/a/2",
	})

	// Sanity-check the existing trigger behavior. An API key for
	// userA should also be wiped on soft-delete.
	_, _ = dbgen.APIKey(t, db, database.APIKey{UserID: userA.ID})

	userB := dbgen.User(t, db, database.User{})
	secretB := dbgen.UserSecret(t, db, database.UserSecret{
		UserID:   userB.ID,
		Name:     "secret-b",
		Value:    "value-b",
		EnvName:  "SECRET_B",
		FilePath: "/secrets/b",
	})

	require.NoError(t, db.UpdateUserDeletedByID(ctx, userA.ID))

	// userA's secrets are removed after soft-deletion.
	_, err := db.GetUserSecretByID(ctx, secretA1.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.GetUserSecretByID(ctx, secretA2.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	// userA's API key is also removed.
	apiKeysA, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		UserID:    userA.ID,
		LoginType: userA.LoginType,
	})
	require.NoError(t, err)
	require.Empty(t, apiKeysA)

	// userB's secret is unaffected.
	got, err := db.GetUserSecretByID(ctx, secretB.ID)
	require.NoError(t, err)
	require.Equal(t, secretB.ID, got.ID)

	// Trying to insert a new secret for the soft-deleted userA must fail.
	_, err = db.CreateUserSecret(ctx, database.CreateUserSecretParams{
		ID:       uuid.New(),
		UserID:   userA.ID,
		Name:     "post-delete",
		Value:    "value",
		EnvName:  "POST_DELETE_ENV",
		FilePath: "/secrets/post-delete",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot create user_secret for deleted user")
}

// TestOrgMembersSoftDeleteTrigger verifies that a user's organization
// memberships (and transitively their group memberships) are deleted
// when the user is soft-deleted.
func TestOrgMembersSoftDeleteTrigger(t *testing.T) {
	t.Parallel()

	// SingleOrg verifies the basic case: one org, one group, and a
	// control user whose membership must survive.
	t.Run("SingleOrg", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		org := dbgen.Organization(t, db, database.Organization{})

		// userA will be soft-deleted.
		userA := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         userA.ID,
		})

		// Add userA to a group in the org (should be cleaned up transitively).
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  userA.ID,
			GroupID: group.ID,
		})

		// userB is a control; their membership must not be touched.
		userB := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         userB.ID,
		})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  userB.ID,
			GroupID: group.ID,
		})

		// Soft-delete userA.
		require.NoError(t, db.UpdateUserDeletedByID(ctx, userA.ID))

		// userA should no longer appear in the organization.
		orgMembers, err := db.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: org.ID,
		})
		require.NoError(t, err)
		var memberIDs []uuid.UUID
		for _, m := range orgMembers {
			memberIDs = append(memberIDs, m.OrganizationMember.UserID)
		}
		require.NotContains(t, memberIDs, userA.ID)
		require.Contains(t, memberIDs, userB.ID)

		// The raw org membership rows should also be gone (not just hidden).
		rawOrgs, err := db.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{userA.ID})
		require.NoError(t, err)
		require.Empty(t, rawOrgs, "zombie org membership rows should not exist after soft-delete")

		// userA's group membership should also be removed by the cascading trigger.
		groupMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
			GroupID:       group.ID,
			IncludeSystem: true,
		})
		require.NoError(t, err)
		var groupMemberIDs []uuid.UUID
		for _, gm := range groupMembers {
			groupMemberIDs = append(groupMemberIDs, gm.UserID)
		}
		require.NotContains(t, groupMemberIDs, userA.ID)
		require.Contains(t, groupMemberIDs, userB.ID)
	})

	// MultipleOrgs verifies that memberships are cleaned up across
	// every organization the deleted user belonged to.
	t.Run("MultipleOrgs", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		org1 := dbgen.Organization(t, db, database.Organization{})
		org2 := dbgen.Organization(t, db, database.Organization{})

		// userA will be soft-deleted. They belong to both orgs.
		userA := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org1.ID,
			UserID:         userA.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org2.ID,
			UserID:         userA.ID,
		})

		// Add userA to a group in each org.
		group1 := dbgen.Group(t, db, database.Group{OrganizationID: org1.ID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  userA.ID,
			GroupID: group1.ID,
		})
		group2 := dbgen.Group(t, db, database.Group{OrganizationID: org2.ID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  userA.ID,
			GroupID: group2.ID,
		})

		// userB stays in org1 as a control.
		userB := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org1.ID,
			UserID:         userB.ID,
		})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  userB.ID,
			GroupID: group1.ID,
		})

		// Soft-delete userA.
		require.NoError(t, db.UpdateUserDeletedByID(ctx, userA.ID))

		// userA should be gone from both orgs.
		for _, org := range []database.Organization{org1, org2} {
			members, err := db.OrganizationMembers(ctx, database.OrganizationMembersParams{
				OrganizationID: org.ID,
			})
			require.NoError(t, err)
			for _, m := range members {
				require.NotEqual(t, userA.ID, m.OrganizationMember.UserID,
					"userA should not appear in org %s", org.ID)
			}
		}

		// No raw org membership rows should remain.
		rawOrgs, err := db.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{userA.ID})
		require.NoError(t, err)
		require.Empty(t, rawOrgs, "zombie org membership rows should not exist after soft-delete")

		// Group memberships in both orgs should be cleaned up.
		for _, g := range []struct {
			name    string
			groupID uuid.UUID
		}{
			{"org1-group", group1.ID},
			{"org2-group", group2.ID},
		} {
			groupMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
				GroupID:       g.groupID,
				IncludeSystem: true,
			})
			require.NoError(t, err, g.name)
			for _, gm := range groupMembers {
				require.NotEqual(t, userA.ID, gm.UserID, g.name)
			}
		}

		// userB's memberships are unaffected.
		org1Members, err := db.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: org1.ID,
		})
		require.NoError(t, err)
		var org1MemberIDs []uuid.UUID
		for _, m := range org1Members {
			org1MemberIDs = append(org1MemberIDs, m.OrganizationMember.UserID)
		}
		require.Contains(t, org1MemberIDs, userB.ID)
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
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:      user1.ID,
		Name:        "user1-secret",
		Description: "User 1's secret",
		Value:       "user1-value",
	})

	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:      user2.ID,
		Name:        "user2-secret",
		Description: "User 2's secret",
		Value:       "user2-value",
	})

	testCases := []struct {
		name           string
		subject        rbac.Subject
		lookupUserID   uuid.UUID
		lookupName     string
		expectedAccess bool
	}{
		{
			name: "UserCanAccessOwnSecrets",
			subject: rbac.Subject{
				ID:    user1.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
				Scope: rbac.ScopeAll,
			},
			lookupUserID:   user1.ID,
			lookupName:     "user1-secret",
			expectedAccess: true,
		},
		{
			name: "UserCannotAccessOtherUserSecrets",
			subject: rbac.Subject{
				ID:    user1.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
				Scope: rbac.ScopeAll,
			},
			lookupUserID:   user2.ID,
			lookupName:     "user2-secret",
			expectedAccess: false,
		},
		{
			name: "OwnerCannotAccessUserSecrets",
			subject: rbac.Subject{
				ID:    owner.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.RoleOwner()},
				Scope: rbac.ScopeAll,
			},
			lookupUserID:   user1.ID,
			lookupName:     "user1-secret",
			expectedAccess: false,
		},
		{
			name: "OrgAdminCannotAccessUserSecrets",
			subject: rbac.Subject{
				ID:    orgAdmin.ID.String(),
				Roles: rbac.RoleIdentifiers{rbac.ScopedRoleOrgAdmin(org.ID)},
				Scope: rbac.ScopeAll,
			},
			lookupUserID:   user1.ID,
			lookupName:     "user1-secret",
			expectedAccess: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)

			authCtx := dbauthz.As(ctx, tc.subject)

			_, err := authDB.GetUserSecretByUserIDAndName(authCtx, database.GetUserSecretByUserIDAndNameParams{
				UserID: tc.lookupUserID,
				Name:   tc.lookupName,
			})

			if tc.expectedAccess {
				require.NoError(t, err, "expected access to be granted")
			} else {
				require.Error(t, err, "expected access to be denied")
				assert.True(t, dbauthz.IsNotAuthorizedError(err), "expected authorization error")
			}
		})
	}
}

func TestUpdateWorkspaceBuildOrchestrationRetryByIDMaxAttempts(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionJob.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: version.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	buildJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: version.ID,
		InitiatorID:       user.ID,
		JobID:             buildJob.ID,
		Transition:        database.WorkspaceTransitionStop,
	})

	// Given: a pending orchestration row.
	now := dbtime.Now()
	orchestration, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:                       uuid.New(),
		CreatedAt:                now,
		UpdatedAt:                now,
		ParentBuildID:            parentBuild.ID,
		ChildTransition:          database.WorkspaceTransitionStart,
		ChildRichParameterValues: json.RawMessage("[]"),
	})
	require.NoError(t, err)
	require.Equal(t, workspace.ID, orchestration.WorkspaceID)

	const maxAttemptCount = 3
	const retryError = "some retryable child build failure"
	recordRetry := func(t *testing.T, wantAttempt int32, wantStatus string, wantNextRetry bool) {
		t.Helper()

		now := dbtime.Now()
		nextRetryAfter := now.Add(time.Minute)
		got, err := db.UpdateWorkspaceBuildOrchestrationRetryByID(ctx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
			Error: sql.NullString{
				String: retryError,
				Valid:  true,
			},
			NextRetryAfter:  nextRetryAfter,
			UpdatedAt:       now,
			ID:              orchestration.ID,
			MaxAttemptCount: maxAttemptCount,
		})
		require.NoError(t, err)
		require.Equal(t, wantAttempt, got.AttemptCount)
		require.Equal(t, wantStatus, got.Status)
		require.True(t, got.Error.Valid)
		require.Equal(t, retryError, got.Error.String)
		require.Equal(t, wantNextRetry, got.NextRetryAfter.Valid)
		if wantNextRetry {
			require.False(t, got.NextRetryAfter.Time.Before(nextRetryAfter))
		}
	}

	// When: retryable child build failures are recorded until one
	// attempt remains.
	recordRetry(t, 1, "pending", true)
	recordRetry(t, 2, "pending", true)

	// Then: the next attempt fails the row, clears its retry delay,
	// and prevents further retry updates.
	recordRetry(t, 3, "failed", false)
	_, err = db.UpdateWorkspaceBuildOrchestrationRetryByID(ctx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
		Error: sql.NullString{
			String: retryError,
			Valid:  true,
		},
		NextRetryAfter:  dbtime.Now().Add(time.Minute),
		UpdatedAt:       dbtime.Now(),
		ID:              orchestration.ID,
		MaxAttemptCount: maxAttemptCount,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestUpdateWorkspaceBuildOrchestrationRetryByIDPendingGateUnderContention(t *testing.T) {
	t.Parallel()

	db, _, _ := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionJob.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: version.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	buildJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: version.ID,
		InitiatorID:       user.ID,
		JobID:             buildJob.ID,
		Transition:        database.WorkspaceTransitionStop,
	})

	// Given: a pending orchestration row.
	now := dbtime.Now()
	orchestration, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:                       uuid.New(),
		CreatedAt:                now,
		UpdatedAt:                now,
		ParentBuildID:            parentBuild.ID,
		ChildTransition:          database.WorkspaceTransitionStart,
		ChildRichParameterValues: json.RawMessage("[]"),
	})
	require.NoError(t, err)

	const retryError = "some retryable child build failure"
	type retryResult struct {
		orchestration database.WorkspaceBuildOrchestration
		err           error
	}
	firstUpdated := make(chan retryResult, 1)
	releaseFirst := make(chan struct{})
	firstErr := make(chan error, 1)
	secondStarted := make(chan struct{}, 1)
	secondErr := make(chan error, 1)
	secondDone := make(chan struct{})

	// When: one retry update terminalizes the row while another
	// worker races to retry the same pending row.
	go func() {
		err := db.InTx(func(tx database.Store) error {
			got, err := tx.UpdateWorkspaceBuildOrchestrationRetryByID(ctx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
				Error: sql.NullString{
					String: retryError,
					Valid:  true,
				},
				NextRetryAfter: dbtime.Now().Add(time.Minute),
				UpdatedAt:      dbtime.Now(),
				ID:             orchestration.ID,
				// Use a single allowed retry so the winning update
				// immediately terminalizes the row.
				MaxAttemptCount: 1,
			})
			if err != nil {
				firstUpdated <- retryResult{err: err}
				return err
			}
			firstUpdated <- retryResult{orchestration: got}
			<-releaseFirst
			return nil
		}, nil)
		firstErr <- err
	}()

	firstResult := <-firstUpdated
	require.NoError(t, firstResult.err)

	go func() {
		secondStarted <- struct{}{}
		_, err := db.UpdateWorkspaceBuildOrchestrationRetryByID(ctx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
			Error: sql.NullString{
				String: retryError,
				Valid:  true,
			},
			NextRetryAfter:  dbtime.Now().Add(time.Minute),
			UpdatedAt:       dbtime.Now(),
			ID:              orchestration.ID,
			MaxAttemptCount: 1,
		})
		secondErr <- err
		close(secondDone)
	}()

	<-secondStarted
	// Then: while the first transaction is held open, the second
	// update does not complete.
	require.Never(t, func() bool {
		select {
		case <-secondDone:
			return true
		default:
			return false
		}
	}, time.Second, testutil.IntervalFast)

	close(releaseFirst)
	require.NoError(t, <-firstErr)

	// Then: after the first transaction commits the failed status, the
	// second update rechecks the pending gate and affects no rows.
	require.ErrorIs(t, <-secondErr, sql.ErrNoRows)
}

func TestGetNextPendingWorkspaceBuildOrchestrationForUpdateRetryDelay(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionJob.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: version.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})

	var buildNumber int32
	createOrchestration := func(t *testing.T, createdAt time.Time) database.WorkspaceBuildOrchestration {
		t.Helper()

		buildNumber++
		buildJob := database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		}
		setJobStatus(t, database.ProvisionerJobStatusSucceeded, &buildJob)
		buildJob = dbgen.ProvisionerJob(t, db, nil, buildJob)
		parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       workspace.ID,
			TemplateVersionID: version.ID,
			InitiatorID:       user.ID,
			JobID:             buildJob.ID,
			BuildNumber:       buildNumber,
			Transition:        database.WorkspaceTransitionStop,
		})

		orchestration, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
			ID:                       uuid.New(),
			CreatedAt:                createdAt,
			UpdatedAt:                createdAt,
			ParentBuildID:            parentBuild.ID,
			ChildTransition:          database.WorkspaceTransitionStart,
			ChildRichParameterValues: json.RawMessage("[]"),
		})
		require.NoError(t, err)
		require.Equal(t, workspace.ID, orchestration.WorkspaceID)
		return orchestration
	}

	claimNext := func(t *testing.T) (database.WorkspaceBuildOrchestration, error) {
		t.Helper()

		var orchestration database.WorkspaceBuildOrchestration
		err := db.InTx(func(tx database.Store) error {
			var err error
			orchestration, err = tx.GetNextPendingWorkspaceBuildOrchestrationForUpdate(ctx)
			return err
		}, nil)
		return orchestration, err
	}

	baseTime := dbtime.Now().Add(-time.Hour)

	// Given: an old pending orchestration row with a future retry delay.
	delayed := createOrchestration(t, baseTime)
	_, err := db.UpdateWorkspaceBuildOrchestrationRetryByID(ctx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
		Error: sql.NullString{
			String: "retry later",
			Valid:  true,
		},
		NextRetryAfter:  dbtime.Now().Add(time.Hour),
		UpdatedAt:       dbtime.Now(),
		ID:              delayed.ID,
		MaxAttemptCount: 3,
	})
	require.NoError(t, err)

	// When: the orchestrator claims the next eligible row.
	_, err = claimNext(t)

	// Then: no row is claimed before the retry time.
	require.ErrorIs(t, err, sql.ErrNoRows)

	// When: a later row is eligible immediately.
	eligible := createOrchestration(t, baseTime.Add(time.Minute))

	// Then: the old delayed row does not block the later eligible row.
	got, err := claimNext(t)
	require.NoError(t, err)
	require.Equal(t, eligible.ID, got.ID)

	// When: the delayed row's retry time has passed.
	_, err = db.UpdateWorkspaceBuildOrchestrationRetryByID(ctx, database.UpdateWorkspaceBuildOrchestrationRetryByIDParams{
		Error: sql.NullString{
			String: "retry now",
			Valid:  true,
		},
		NextRetryAfter:  dbtime.Now().Add(-time.Minute),
		UpdatedAt:       dbtime.Now(),
		ID:              delayed.ID,
		MaxAttemptCount: 3,
	})
	require.NoError(t, err)

	// Then: the delayed row is eligible again and is claimed first
	// because it is older than the later row.
	got, err = claimNext(t)
	require.NoError(t, err)
	require.Equal(t, delayed.ID, got.ID)
}

func TestUpdateWorkspaceBuildOrchestrationCompletedByIDWorkspaceMismatch(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionJob.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: version.ID,
		CreatedBy:       user.ID,
	})

	parentWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	otherWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})

	parentJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       parentWorkspace.ID,
		TemplateVersionID: version.ID,
		InitiatorID:       user.ID,
		JobID:             parentJob.ID,
		Transition:        database.WorkspaceTransitionStop,
	})

	// Given: a pending orchestration row.
	orchestration, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:                       uuid.New(),
		CreatedAt:                dbtime.Now(),
		UpdatedAt:                dbtime.Now(),
		ParentBuildID:            parentBuild.ID,
		ChildTransition:          database.WorkspaceTransitionStart,
		ChildRichParameterValues: json.RawMessage("[]"),
	})
	require.NoError(t, err)
	require.Equal(t, parentWorkspace.ID, orchestration.WorkspaceID)

	// Given: a child build whose workspace does not match the parent
	// build's workspace.
	childJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})
	childBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       otherWorkspace.ID,
		TemplateVersionID: version.ID,
		InitiatorID:       user.ID,
		JobID:             childJob.ID,
		Transition:        database.WorkspaceTransitionStart,
	})

	// When: the orchestration is completed with the child build.
	_, err = db.UpdateWorkspaceBuildOrchestrationCompletedByID(ctx, database.UpdateWorkspaceBuildOrchestrationCompletedByIDParams{
		ID:           orchestration.ID,
		ChildBuildID: uuid.NullUUID{UUID: childBuild.ID, Valid: true},
		UpdatedAt:    dbtime.Now(),
	})

	// Then: the composite foreign key rejects the mismatched child build.
	require.Error(t, err)
	require.True(t, database.IsForeignKeyViolation(err))
}

func TestInsertWorkspaceBuildOrchestrationPresetRequiresVersion(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionJob.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: version.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	parentJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})

	// Given: a parent build, a child preset, no child template
	// version.
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: version.ID,
		InitiatorID:       user.ID,
		JobID:             parentJob.ID,
		Transition:        database.WorkspaceTransitionStop,
	})
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: version.ID,
	})

	// When: the orchestration row is inserted.
	_, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:              uuid.New(),
		CreatedAt:       dbtime.Now(),
		UpdatedAt:       dbtime.Now(),
		ParentBuildID:   parentBuild.ID,
		ChildTransition: database.WorkspaceTransitionStart,
		ChildTemplateVersionPresetID: uuid.NullUUID{
			UUID:  preset.ID,
			Valid: true,
		},
		ChildRichParameterValues: json.RawMessage("[]"),
	})

	// Then: the check constraint rejects the missing child template
	// version.
	require.Error(t, err)
	require.True(t, database.IsCheckViolation(err))
}

func TestInsertWorkspaceBuildOrchestrationPresetVersionMismatch(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	versionOneJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	versionOne := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionOneJob.ID,
	})
	versionTwoJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
	})
	versionTwo := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		JobID:          versionTwoJob.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: versionOne.ID,
		CreatedBy:       user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	parentJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})

	// Given: a parent build and a child preset.
	parentBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: versionOne.ID,
		InitiatorID:       user.ID,
		JobID:             parentJob.ID,
		Transition:        database.WorkspaceTransitionStop,
	})
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: versionOne.ID,
	})

	// When: the orchestration row is inserted with a different child
	// template version.
	_, err := db.InsertWorkspaceBuildOrchestration(ctx, database.InsertWorkspaceBuildOrchestrationParams{
		ID:              uuid.New(),
		CreatedAt:       dbtime.Now(),
		UpdatedAt:       dbtime.Now(),
		ParentBuildID:   parentBuild.ID,
		ChildTransition: database.WorkspaceTransitionStart,
		ChildTemplateVersionID: uuid.NullUUID{
			UUID:  versionTwo.ID,
			Valid: true,
		},
		ChildTemplateVersionPresetID: uuid.NullUUID{
			UUID:  preset.ID,
			Valid: true,
		},
		ChildRichParameterValues: json.RawMessage("[]"),
	})

	// Then: the composite foreign key rejects the preset/version
	// mismatch.
	require.Error(t, err)
	require.True(t, database.IsForeignKeyViolation(err))
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

func TestWorkspaceACLObjectConstraint(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:    user.ID,
		TemplateID: template.ID,
		Deleted:    false,
	})

	t.Run("GroupACLNull", func(t *testing.T) {
		t.Parallel()

		var nilACL database.WorkspaceACL

		ctx := testutil.Context(t, testutil.WaitLong)
		err := db.UpdateWorkspaceACLByID(ctx, database.UpdateWorkspaceACLByIDParams{
			ID:       workspace.ID,
			GroupACL: nilACL,
			UserACL:  database.WorkspaceACL{},
		})
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckGroupAclIsObject))
	})

	t.Run("UserACLNull", func(t *testing.T) {
		t.Parallel()

		var nilACL database.WorkspaceACL

		ctx := testutil.Context(t, testutil.WaitLong)
		err := db.UpdateWorkspaceACLByID(ctx, database.UpdateWorkspaceACLByIDParams{
			ID:       workspace.ID,
			GroupACL: database.WorkspaceACL{},
			UserACL:  nilACL,
		})
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckUserAclIsObject))
	})

	t.Run("ValidEmptyObjects", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		err := db.UpdateWorkspaceACLByID(ctx, database.UpdateWorkspaceACLByIDParams{
			ID:       workspace.ID,
			GroupACL: database.WorkspaceACL{},
			UserACL:  database.WorkspaceACL{},
		})
		require.NoError(t, err)
	})
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
			expectedStatus:            database.TaskStatusPending,
			description:               "Workspace build pending (not yet picked up by provisioner)",
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

func TestDeleteTaskDeletesTaskSnapshot(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	task := dbgen.Task(t, db, database.TaskTable{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		TemplateVersionID: templateVersion.ID,
		Prompt:            "Test prompt",
	})

	err := db.UpsertTaskSnapshot(ctx, database.UpsertTaskSnapshotParams{
		TaskID:               task.ID,
		LogSnapshot:          json.RawMessage(`{"messages":[]}`),
		LogSnapshotCreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)

	_, err = db.DeleteTask(ctx, database.DeleteTaskParams{
		ID:        task.ID,
		DeletedAt: dbtime.Now(),
	})
	require.NoError(t, err)

	_, err = db.GetTaskSnapshot(ctx, task.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
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

	t.Run("HeartbeatAISeats", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

		// Insert a heartbeat event.
		err := db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "hb-1",
			EventType: "hb_ai_seats_v1",
			EventData: []byte(`{"count": 10}`),
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		rows := getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 1)
		require.Equal(t, "hb_ai_seats_v1", rows[0].EventType)
		require.JSONEq(t, `{"count": 10}`, string(rows[0].UsageData))

		// Insert a higher count on the same day — should take the max.
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "hb-2",
			EventType: "hb_ai_seats_v1",
			EventData: []byte(`{"count": 50}`),
			CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 1)
		require.JSONEq(t, `{"count": 50}`, string(rows[0].UsageData))

		// Insert a lower count on the same day — should keep the max (50).
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "hb-3",
			EventType: "hb_ai_seats_v1",
			EventData: []byte(`{"count": 25}`),
			CreatedAt: time.Date(2025, 1, 1, 18, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 1)
		require.JSONEq(t, `{"count": 50}`, string(rows[0].UsageData))

		// Insert on a different day.
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "hb-4",
			EventType: "hb_ai_seats_v1",
			EventData: []byte(`{"count": 5}`),
			CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 2)
		require.JSONEq(t, `{"count": 50}`, string(rows[0].UsageData))
		require.JSONEq(t, `{"count": 5}`, string(rows[1].UsageData))

		// Also insert a dc_managed_agents_v1 on the same first day to
		// verify different event types get separate daily rows.
		err = db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        "dc-1",
			EventType: "dc_managed_agents_v1",
			EventData: []byte(`{"count": 7}`),
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)

		rows = getDailyRows(ctx, sqlDB)
		require.Len(t, rows, 3)
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
			ID:             uuid.New(),
			EndedAt:        time.Now(),
			CredentialHint: "sk-a...efgh",
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
				ID:             uid,
				InitiatorID:    user.ID,
				Metadata:       json.RawMessage("{}"),
				Client:         sql.NullString{String: "client", Valid: true},
				CredentialKind: database.CredentialKindCentralized,
			}

			intc, err := db.InsertAIBridgeInterception(ctx, insertParams)
			require.NoError(t, err)
			require.Equal(t, uid, intc.ID)
			require.False(t, intc.EndedAt.Valid)
			require.True(t, intc.Client.Valid)
			require.Equal(t, "client", intc.Client.String)
			interceptions = append(interceptions, intc)
		}

		intc0 := interceptions[0]
		endedAt := time.Now()
		// Mark first interception as done
		updated, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:             intc0.ID,
			EndedAt:        endedAt,
			CredentialHint: "sk-a...efgh",
		})
		require.NoError(t, err)
		require.EqualValues(t, updated.ID, intc0.ID)
		require.True(t, updated.EndedAt.Valid)
		require.WithinDuration(t, endedAt, updated.EndedAt.Time, 5*time.Second)
		require.Equal(t, "sk-a...efgh", updated.CredentialHint)

		// Updating first interception again should fail
		updated, err = db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:             intc0.ID,
			EndedAt:        endedAt.Add(time.Hour),
			CredentialHint: "sk-a...efgh",
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

		// Other interceptions should not have ended_at set
		for _, intc := range interceptions[1:] {
			got, err := db.GetAIBridgeInterceptionByID(ctx, intc.ID)
			require.NoError(t, err)
			require.False(t, got.EndedAt.Valid)
		}
	})

	t.Run("CentralizedHintUpdated", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		intc, err := db.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
			ID:             uuid.New(),
			InitiatorID:    user.ID,
			Metadata:       json.RawMessage("{}"),
			CredentialKind: database.CredentialKindCentralized,
			CredentialHint: "",
		})
		require.NoError(t, err)

		updated, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:             intc.ID,
			EndedAt:        time.Now(),
			CredentialHint: "sk-a...efgh",
		})
		require.NoError(t, err)
		require.Equal(t, "sk-a...efgh", updated.CredentialHint)
	})

	t.Run("BYOKHintPreserved", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})
		intc, err := db.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
			ID:             uuid.New(),
			InitiatorID:    user.ID,
			Metadata:       json.RawMessage("{}"),
			CredentialKind: database.CredentialKindByok,
			CredentialHint: "sk-u...byok",
		})
		require.NoError(t, err)

		updated, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:             intc.ID,
			EndedAt:        time.Now(),
			CredentialHint: "sk-a...efgh",
		})
		require.NoError(t, err)
		require.Equal(t, "sk-u...byok", updated.CredentialHint)
	})
}

func TestAIBridgeInterceptionAgentFirewallColumns(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	afwSessionID := uuid.New()

	t.Run("InsertAndReadWithFirewallFieldsSet", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})

		inserted, err := db.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
			ID:                          uuid.New(),
			InitiatorID:                 user.ID,
			Metadata:                    json.RawMessage("{}"),
			CredentialKind:              database.CredentialKindCentralized,
			AgentFirewallSessionID:      uuid.NullUUID{UUID: afwSessionID, Valid: true},
			AgentFirewallSequenceNumber: sql.NullInt32{Int32: 5, Valid: true},
		})
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: afwSessionID, Valid: true}, inserted.AgentFirewallSessionID)
		require.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, inserted.AgentFirewallSequenceNumber)

		got, err := db.GetAIBridgeInterceptionByID(ctx, inserted.ID)
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: afwSessionID, Valid: true}, got.AgentFirewallSessionID)
		require.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, got.AgentFirewallSequenceNumber)
	})

	t.Run("InsertAndReadWithFirewallFieldsNull", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})

		inserted, err := db.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
			ID:             uuid.New(),
			InitiatorID:    user.ID,
			Metadata:       json.RawMessage("{}"),
			CredentialKind: database.CredentialKindCentralized,
			// AgentFirewallSessionID and AgentFirewallSequenceNumber omitted (zero → NULL).
		})
		require.NoError(t, err)
		require.False(t, inserted.AgentFirewallSessionID.Valid)
		require.False(t, inserted.AgentFirewallSequenceNumber.Valid)

		got, err := db.GetAIBridgeInterceptionByID(ctx, inserted.ID)
		require.NoError(t, err)
		require.False(t, got.AgentFirewallSessionID.Valid)
		require.False(t, got.AgentFirewallSequenceNumber.Valid)
	})

	t.Run("UpdatePreservesFields", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		user := dbgen.User(t, db, database.User{})

		inserted, err := db.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
			ID:                          uuid.New(),
			InitiatorID:                 user.ID,
			Metadata:                    json.RawMessage("{}"),
			CredentialKind:              database.CredentialKindCentralized,
			AgentFirewallSessionID:      uuid.NullUUID{UUID: afwSessionID, Valid: true},
			AgentFirewallSequenceNumber: sql.NullInt32{Int32: 5, Valid: true},
		})
		require.NoError(t, err)

		updated, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      inserted.ID,
			EndedAt: time.Now(),
		})
		require.NoError(t, err)
		require.True(t, updated.EndedAt.Valid)
		// UpdateAIBridgeInterceptionEnded must not clobber the agent firewall fields.
		require.Equal(t, uuid.NullUUID{UUID: afwSessionID, Valid: true}, updated.AgentFirewallSessionID)
		require.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, updated.AgentFirewallSequenceNumber)
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
		LoginType:      user.LoginType,
		UserID:         user.ID,
		IncludeExpired: true,
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
		LoginType:      user.LoginType,
		UserID:         user.ID,
		IncludeExpired: true,
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
		LoginType:      user.LoginType,
		UserID:         user.ID,
		IncludeExpired: true,
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
		row, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent.AuthToken)
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
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent.AuthToken)
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
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent.AuthToken)
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
		row, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent.AuthToken)
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
		row, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent2.AuthToken)
		require.NoError(t, err, "agent from most recent start should authenticate during stop")
		require.Equal(t, agent2.ID, row.WorkspaceAgent.ID)
		require.Equal(t, startBuild2.ID, row.WorkspaceBuild.ID)

		// Agent from build 1 should NOT authenticate.
		_, err = db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent1.AuthToken)
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
		_, err := db.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(ctx, agent1.AuthToken)
		require.ErrorIs(t, err, sql.ErrNoRows, "agent should not authenticate when latest build is not STOP")
	})
}

// Our `InsertWorkspaceAgentDevcontainers` query should ideally be `[]uuid.NullUUID` but unfortunately
// sqlc infers it as `[]uuid.UUID`. To ensure we don't insert a `uuid.Nil`, the query inserts NULL when
// passed with `uuid.Nil`. This test ensures we keep this behavior without regression.
func TestInsertWorkspaceAgentDevcontainers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		validSubagent []bool
	}{
		{"BothValid", []bool{true, true}},
		{"FirstValidSecondInvalid", []bool{true, false}},
		{"FirstInvalidSecondValid", []bool{false, true}},
		{"BothInvalid", []bool{false, false}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				db, _ = dbtestutil.NewDB(t)
				org   = dbgen.Organization(t, db, database.Organization{})
				job   = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					Type:           database.ProvisionerJobTypeTemplateVersionImport,
					OrganizationID: org.ID,
				})
				resource = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
				agent    = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
			)

			ids := make([]uuid.UUID, len(tc.validSubagent))
			names := make([]string, len(tc.validSubagent))
			workspaceFolders := make([]string, len(tc.validSubagent))
			configPaths := make([]string, len(tc.validSubagent))
			subagentIDs := make([]uuid.UUID, len(tc.validSubagent))

			for i, valid := range tc.validSubagent {
				ids[i] = uuid.New()
				names[i] = fmt.Sprintf("test-devcontainer-%d", i)
				workspaceFolders[i] = fmt.Sprintf("/workspace%d", i)
				configPaths[i] = fmt.Sprintf("/workspace%d/.devcontainer/devcontainer.json", i)

				if valid {
					subagentIDs[i] = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ResourceID: resource.ID,
						ParentID:   uuid.NullUUID{UUID: agent.ID, Valid: true},
					}).ID
				} else {
					subagentIDs[i] = uuid.Nil
				}
			}

			ctx := testutil.Context(t, testutil.WaitShort)

			// Given: We insert multiple devcontainer records.
			devcontainers, err := db.InsertWorkspaceAgentDevcontainers(ctx, database.InsertWorkspaceAgentDevcontainersParams{
				WorkspaceAgentID: agent.ID,
				CreatedAt:        dbtime.Now(),
				ID:               ids,
				Name:             names,
				WorkspaceFolder:  workspaceFolders,
				ConfigPath:       configPaths,
				SubagentID:       subagentIDs,
			})
			require.NoError(t, err)
			require.Len(t, devcontainers, len(tc.validSubagent))

			// Then: Verify each devcontainer has the correct SubagentID validity.
			// - When we pass `uuid.Nil`, we get a `uuid.NullUUID{Valid: false}`
			// - When we pass a valid UUID, we get a `uuid.NullUUID{Valid: true}`
			for i, valid := range tc.validSubagent {
				require.Equal(t, valid, devcontainers[i].SubagentID.Valid, "devcontainer %d: subagent_id validity mismatch", i)
				if valid {
					require.Equal(t, subagentIDs[i], devcontainers[i].SubagentID.UUID, "devcontainer %d: subagent_id UUID mismatch", i)
				}
			}

			// Perform the same check on data returned by
			// `GetWorkspaceAgentDevcontainersByAgentID` to ensure the fix is at
			// the data storage layer, instead of just at a query level.
			fetched, err := db.GetWorkspaceAgentDevcontainersByAgentID(ctx, agent.ID)
			require.NoError(t, err)
			require.Len(t, fetched, len(tc.validSubagent))

			// Sort fetched by name to ensure consistent ordering for comparison.
			slices.SortFunc(fetched, func(a, b database.WorkspaceAgentDevcontainer) int {
				return strings.Compare(a.Name, b.Name)
			})

			for i, valid := range tc.validSubagent {
				require.Equal(t, valid, fetched[i].SubagentID.Valid, "fetched devcontainer %d: subagent_id validity mismatch", i)
				if valid {
					require.Equal(t, subagentIDs[i], fetched[i].SubagentID.UUID, "fetched devcontainer %d: subagent_id UUID mismatch", i)
				}
			}
		})
	}
}

func TestGetEnabledChatModelConfigsUsesAIProviders(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	enabledProvider := dbgen.AIProvider(t, store, database.AIProvider{
		Type: database.AIProviderTypeOpenrouter,
		Name: "openrouter-" + uuid.NewString(),
	})
	disabledProvider := dbgen.AIProvider(t, store, database.AIProvider{
		Type: database.AIProviderTypeVercel,
		Name: "vercel-" + uuid.NewString(),
	}, func(params *database.InsertAIProviderParams) {
		params.Enabled = false
	})
	enabledConfig := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model: "openrouter-model-" + uuid.NewString(),
		AIProviderID: uuid.NullUUID{
			UUID:  enabledProvider.ID,
			Valid: true,
		},
	})
	disabledProviderConfig := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model: "vercel-model-" + uuid.NewString(),
		AIProviderID: uuid.NullUUID{
			UUID:  disabledProvider.ID,
			Valid: true,
		},
	})
	disabledModelConfig := dbgen.ChatModelConfig(t, store, database.ChatModelConfig{
		Model: "disabled-model-" + uuid.NewString(),
		AIProviderID: uuid.NullUUID{
			UUID:  enabledProvider.ID,
			Valid: true,
		},
	}, func(params *database.InsertChatModelConfigParams) {
		params.Enabled = false
	})

	configs, err := store.GetEnabledChatModelConfigs(ctx)
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(configs, func(row database.GetEnabledChatModelConfigsRow) bool {
		return row.ChatModelConfig.ID == enabledConfig.ID
	}))
	require.False(t, slices.ContainsFunc(configs, func(row database.GetEnabledChatModelConfigsRow) bool {
		return row.ChatModelConfig.ID == disabledProviderConfig.ID
	}))
	require.False(t, slices.ContainsFunc(configs, func(row database.GetEnabledChatModelConfigsRow) bool {
		return row.ChatModelConfig.ID == disabledModelConfig.ID
	}))

	config, err := store.GetEnabledChatModelConfigByID(ctx, enabledConfig.ID)
	require.NoError(t, err)
	require.Equal(t, enabledConfig.ID, config.ID)

	_, err = store.GetEnabledChatModelConfigByID(ctx, disabledProviderConfig.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	_, err = store.GetEnabledChatModelConfigByID(ctx, disabledModelConfig.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func insertChatModelConfigForTest(
	ctx context.Context,
	t testing.TB,
	store database.Store,
	providerType string,
	params database.InsertChatModelConfigParams,
) (database.ChatModelConfig, error) {
	t.Helper()
	if params.AIProviderID.Valid {
		return store.InsertChatModelConfig(ctx, params)
	}
	providerName := providerType
	if providerName == "" {
		providerName = "openai"
	}
	providers, err := store.GetAIProviders(ctx, database.GetAIProvidersParams{IncludeDisabled: true})
	if err != nil {
		return database.ChatModelConfig{}, err
	}
	var provider database.AIProvider
	for _, candidate := range providers {
		if candidate.Type != database.AIProviderType(providerName) {
			continue
		}
		if provider.ID == uuid.Nil || candidate.CreatedAt.After(provider.CreatedAt) {
			provider = candidate
		}
	}
	if provider.ID == uuid.Nil {
		provider = dbgen.AIProvider(t, store, database.AIProvider{
			Type: database.AIProviderType(providerName),
		})
	}
	params.AIProviderID = uuid.NullUUID{UUID: provider.ID, Valid: true}
	return store.InsertChatModelConfig(ctx, params)
}

func TestInsertChatMessages(t *testing.T) {
	t.Parallel()

	insertModelConfig := func(
		t *testing.T,
		store database.Store,
		ctx context.Context,
		userID uuid.UUID,
		provider string,
		model string,
		displayName string,
		isDefault bool,
	) database.ChatModelConfig {
		t.Helper()

		modelConfig, err := insertChatModelConfigForTest(ctx, t, store, provider, database.InsertChatModelConfigParams{
			Model:                model,
			DisplayName:          displayName,
			CreatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
			UpdatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
			Enabled:              true,
			IsDefault:            isDefault,
			ContextLimit:         128000,
			CompressionThreshold: 80,
			Options:              json.RawMessage(`{}`),
		})
		require.NoError(t, err)

		return modelConfig
	}

	setupChat := func(t *testing.T) (database.Store, context.Context, database.User, database.Chat, string, database.ChatModelConfig) {
		t.Helper()

		store, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		org := dbgen.Organization(t, store, database.Organization{})
		user := dbgen.User(t, store, database.User{})
		dbgen.OrganizationMember(t, store, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
		provider := "openai"

		dbgen.ChatProvider(t, store, database.ChatProvider{
			Provider:             provider,
			DisplayName:          "OpenAI",
			APIKey:               "test-key",
			Enabled:              true,
			CentralApiKeyEnabled: true,
		})

		modelConfigA := insertModelConfig(
			t,
			store,
			ctx,
			user.ID,
			provider,
			"test-model-a-"+uuid.NewString(),
			"Test Model A",
			true,
		)

		chat, err := store.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           user.ID,
			LastModelConfigID: modelConfigA.ID,
			Title:             "test-chat-" + uuid.NewString(),
		})
		require.NoError(t, err)

		return store, ctx, user, chat, provider, modelConfigA
	}

	insertMessage := func(t *testing.T, store database.Store, ctx context.Context, chatID, userID, modelConfigID uuid.UUID, content string) {
		t.Helper()
		apiKey, _ := dbgen.APIKey(t, store, database.APIKey{ID: uuid.NewString(), UserID: userID})

		_, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chatID,
			CreatedBy:           []uuid.UUID{userID},
			APIKeyID:            []string{apiKey.ID},
			ModelConfigID:       []uuid.UUID{modelConfigID},
			Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
			ContentVersion:      []int16{chatprompt.CurrentContentVersion},
			Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
			Content:             []string{fmt.Sprintf("%q", content)},
			InputTokens:         []int64{0},
			OutputTokens:        []int64{0},
			TotalTokens:         []int64{0},
			ReasoningTokens:     []int64{0},
			CacheCreationTokens: []int64{0},
			CacheReadTokens:     []int64{0},
			ContextLimit:        []int64{0},
			Compressed:          []bool{false},
			TotalCostMicros:     []int64{0},
			RuntimeMs:           []int64{0},
		})
		require.NoError(t, err)
	}

	t.Run("ModelSwitchUpdatesLastModelConfigID", func(t *testing.T) {
		t.Parallel()

		store, ctx, user, chat, provider, modelConfigA := setupChat(t)
		modelConfigB := insertModelConfig(
			t,
			store,
			ctx,
			user.ID,
			provider,
			"test-model-b-"+uuid.NewString(),
			"Test Model B",
			false,
		)

		insertMessage(t, store, ctx, chat.ID, user.ID, modelConfigB.ID, "switch models")

		gotChat, err := store.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, modelConfigA.ID, chat.LastModelConfigID)
		require.Equal(t, modelConfigB.ID, gotChat.LastModelConfigID)
	})

	t.Run("SameModelDoesNotBreakAnything", func(t *testing.T) {
		t.Parallel()

		store, ctx, user, chat, _, modelConfigA := setupChat(t)

		insertMessage(t, store, ctx, chat.ID, user.ID, modelConfigA.ID, "same model")

		gotChat, err := store.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, modelConfigA.ID, gotChat.LastModelConfigID)
	})

	t.Run("BatchInsertMultipleMessages", func(t *testing.T) {
		t.Parallel()

		store, ctx, user, chat, _, modelConfigA := setupChat(t)
		apiKey, _ := dbgen.APIKey(t, store, database.APIKey{ID: uuid.NewString(), UserID: user.ID})

		msgs, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chat.ID,
			CreatedBy:           []uuid.UUID{user.ID, uuid.Nil, uuid.Nil},
			APIKeyID:            []string{apiKey.ID, "", ""},
			ModelConfigID:       []uuid.UUID{modelConfigA.ID, modelConfigA.ID, modelConfigA.ID},
			Role:                []database.ChatMessageRole{database.ChatMessageRoleUser, database.ChatMessageRoleAssistant, database.ChatMessageRoleTool},
			ContentVersion:      []int16{chatprompt.CurrentContentVersion, chatprompt.CurrentContentVersion, chatprompt.CurrentContentVersion},
			Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth, database.ChatMessageVisibilityBoth, database.ChatMessageVisibilityBoth},
			Content:             []string{`"hello"`, `"response"`, `"tool result"`},
			InputTokens:         []int64{10, 0, 0},
			OutputTokens:        []int64{0, 20, 0},
			TotalTokens:         []int64{10, 20, 0},
			ReasoningTokens:     []int64{0, 5, 0},
			CacheCreationTokens: []int64{0, 0, 0},
			CacheReadTokens:     []int64{0, 0, 0},
			ContextLimit:        []int64{0, 0, 0},
			Compressed:          []bool{false, false, false},
			TotalCostMicros:     []int64{0, 100, 0},
			RuntimeMs:           []int64{0, 500, 0},
		})
		require.NoError(t, err)
		require.Len(t, msgs, 3)

		// Verify ordering and roles.
		require.Equal(t, database.ChatMessageRoleUser, msgs[0].Role)
		require.Equal(t, database.ChatMessageRoleAssistant, msgs[1].Role)
		require.Equal(t, database.ChatMessageRoleTool, msgs[2].Role)

		// Verify IDs are sequential.
		require.Less(t, msgs[0].ID, msgs[1].ID)
		require.Less(t, msgs[1].ID, msgs[2].ID)

		// Verify nullable fields: user message has CreatedBy set.
		require.True(t, msgs[0].CreatedBy.Valid)
		require.Equal(t, user.ID, msgs[0].CreatedBy.UUID)
		// Assistant and tool messages have NULL CreatedBy.
		require.False(t, msgs[1].CreatedBy.Valid)
		require.False(t, msgs[2].CreatedBy.Valid)

		// Verify token fields stored as NULL when zero.
		require.True(t, msgs[0].InputTokens.Valid)
		require.Equal(t, int64(10), msgs[0].InputTokens.Int64)
		require.False(t, msgs[0].OutputTokens.Valid) // 0 → NULL
		require.True(t, msgs[1].OutputTokens.Valid)
		require.Equal(t, int64(20), msgs[1].OutputTokens.Int64)

		// Verify cost: assistant has cost, others NULL.
		require.True(t, msgs[1].TotalCostMicros.Valid)
		require.Equal(t, int64(100), msgs[1].TotalCostMicros.Int64)
		require.False(t, msgs[0].TotalCostMicros.Valid)
		require.False(t, msgs[2].TotalCostMicros.Valid)

		// Verify runtime_ms on assistant message.
		require.True(t, msgs[1].RuntimeMs.Valid)
		require.Equal(t, int64(500), msgs[1].RuntimeMs.Int64)
		require.False(t, msgs[0].RuntimeMs.Valid)
	})
}

func TestGetChatMessagesForPromptByChatID(t *testing.T) {
	t.Parallel()

	// This test exercises a complex CTE query for prompt
	// reconstruction after compaction. It requires Postgres.
	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	// Helper: create a chat model config (required FK for chats).
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})

	// An AI provider row is required as a FK for model configs.
	provider := dbgen.AIProvider(t, db, database.AIProvider{
		Type:        database.AIProviderTypeOpenai,
		Name:        "test-" + uuid.NewString(),
		DisplayName: sql.NullString{String: "OpenAI", Valid: true},
		Enabled:     true,
	})
	dbgen.AIProviderKey(t, db, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "test-key",
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, db, "openai", database.InsertChatModelConfigParams{
		AIProviderID:         uuid.NullUUID{UUID: provider.ID, Valid: true},
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	newChat := func(t *testing.T) database.Chat {
		t.Helper()
		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "test-chat-" + uuid.NewString(),
		})
		require.NoError(t, err)
		return chat
	}

	insertMsg := func(
		t *testing.T,
		chatID uuid.UUID,
		role database.ChatMessageRole,
		vis database.ChatMessageVisibility,
		compressed bool,
		content string,
	) database.ChatMessage {
		t.Helper()
		results, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chatID,
			CreatedBy:           []uuid.UUID{uuid.Nil},
			ModelConfigID:       []uuid.UUID{uuid.Nil},
			Role:                []database.ChatMessageRole{role},
			ContentVersion:      []int16{chatprompt.CurrentContentVersion},
			Visibility:          []database.ChatMessageVisibility{vis},
			Compressed:          []bool{compressed},
			Content:             []string{`"` + content + `"`},
			InputTokens:         []int64{0},
			OutputTokens:        []int64{0},
			TotalTokens:         []int64{0},
			ReasoningTokens:     []int64{0},
			CacheCreationTokens: []int64{0},
			CacheReadTokens:     []int64{0},
			ContextLimit:        []int64{0},
			TotalCostMicros:     []int64{0},
			RuntimeMs:           []int64{0},
		})
		require.NoError(t, err)
		return results[0]
	}

	msgIDs := func(msgs []database.ChatMessage) []int64 {
		ids := make([]int64, len(msgs))
		for i, m := range msgs {
			ids[i] = m.ID
		}
		return ids
	}

	t.Run("NoCompaction", func(t *testing.T) {
		t.Parallel()
		chat := newChat(t)

		sys := insertMsg(t, chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityModel, false, "system prompt")
		usr := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, "hello")
		ast := insertMsg(t, chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false, "hi there")

		got, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, []int64{sys.ID, usr.ID, ast.ID}, msgIDs(got))
	})

	t.Run("UserOnlyVisibilityExcluded", func(t *testing.T) {
		t.Parallel()
		chat := newChat(t)

		// Messages with visibility=user should NOT appear in the
		// prompt (they are only for the UI).
		insertMsg(t, chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityModel, false, "system prompt")
		insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityUser, false, "user-only msg")
		usr := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, "hello")

		got, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		for _, m := range got {
			require.NotEqual(t, database.ChatMessageVisibilityUser, m.Visibility,
				"visibility=user messages should not appear in the prompt")
		}
		require.Contains(t, msgIDs(got), usr.ID)
	})

	t.Run("AfterCompaction", func(t *testing.T) {
		t.Parallel()
		chat := newChat(t)

		// Pre-compaction conversation.
		sys := insertMsg(t, chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityModel, false, "system prompt")
		preUser := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, "old question")
		preAsst := insertMsg(t, chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false, "old answer")

		// Compaction messages:
		// 1. Summary (role=user, visibility=model, compressed=true).
		summary := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, true, "compaction summary")
		// 2. Compressed assistant tool-call (visibility=user).
		insertMsg(t, chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, true, "tool call")
		// 3. Compressed tool result (visibility=both).
		insertMsg(t, chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, true, "tool result")

		// Post-compaction messages.
		postUser := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, "new question")
		postAsst := insertMsg(t, chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false, "new answer")

		got, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		gotIDs := msgIDs(got)

		// Must include: system prompt, summary, post-compaction.
		require.Contains(t, gotIDs, sys.ID, "system prompt must be included")
		require.Contains(t, gotIDs, summary.ID, "compaction summary must be included")
		require.Contains(t, gotIDs, postUser.ID, "post-compaction user msg must be included")
		require.Contains(t, gotIDs, postAsst.ID, "post-compaction assistant msg must be included")

		// Must exclude: pre-compaction non-system messages.
		require.NotContains(t, gotIDs, preUser.ID, "pre-compaction user msg must be excluded")
		require.NotContains(t, gotIDs, preAsst.ID, "pre-compaction assistant msg must be excluded")

		// Verify ordering.
		require.Equal(t, []int64{sys.ID, summary.ID, postUser.ID, postAsst.ID}, gotIDs)
	})

	t.Run("AfterCompactionSummaryIsUserRole", func(t *testing.T) {
		t.Parallel()
		chat := newChat(t)

		// After compaction the summary must appear as role=user so
		// that LLM APIs (e.g. Anthropic) see at least one
		// non-system message in the prompt.
		insertMsg(t, chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityModel, false, "system prompt")
		summary := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, true, "summary text")
		newUsr := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, "new question")

		got, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		hasNonSystem := false
		for _, m := range got {
			if m.Role != "system" {
				hasNonSystem = true
				break
			}
		}
		require.True(t, hasNonSystem,
			"prompt must contain at least one non-system message after compaction")
		require.Contains(t, msgIDs(got), summary.ID)
		require.Contains(t, msgIDs(got), newUsr.ID)
	})

	t.Run("CompressedToolResultNotPickedAsSummary", func(t *testing.T) {
		t.Parallel()
		chat := newChat(t)

		// The CTE uses visibility='model' (exact match). If it
		// used IN ('model','both'), the compressed tool result
		// (visibility=both) would be picked as the "summary"
		// instead of the actual summary.
		insertMsg(t, chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityModel, false, "system prompt")
		summary := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, true, "real summary")
		compressedTool := insertMsg(t, chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, true, "tool result")
		postUser := insertMsg(t, chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, "follow-up")

		got, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		gotIDs := msgIDs(got)
		require.Contains(t, gotIDs, summary.ID, "real summary must be included")
		require.NotContains(t, gotIDs, compressedTool.ID,
			"compressed tool result must not be included")
		require.Contains(t, gotIDs, postUser.ID)
	})
}

func TestGetWorkspaceBuildMetricsByResourceID(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			TemplateID:     uuid.NullUUID{UUID: tmpl.ID, Valid: true},
			CreatedBy:      user.ID,
		})
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID:   org.ID,
			TemplateID:       tmpl.ID,
			OwnerID:          user.ID,
			AutomaticUpdates: database.AutomaticUpdatesNever,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			TemplateVersionID: tv.ID,
			JobID:             job.ID,
			InitiatorID:       user.ID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})

		parentReadyAt := dbtime.Now()
		parentStartedAt := parentReadyAt.Add(-time.Second)
		_ = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID:     resource.ID,
			StartedAt:      sql.NullTime{Time: parentStartedAt, Valid: true},
			ReadyAt:        sql.NullTime{Time: parentReadyAt, Valid: true},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		})

		row, err := db.GetWorkspaceBuildMetricsByResourceID(ctx, resource.ID)
		require.NoError(t, err)
		require.True(t, row.AllAgentsReady)
		require.True(t, parentReadyAt.Equal(row.LastAgentReadyAt))
		require.Equal(t, "success", row.WorstStatus)
	})

	t.Run("SubAgentExcluded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			TemplateID:     uuid.NullUUID{UUID: tmpl.ID, Valid: true},
			CreatedBy:      user.ID,
		})
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID:   org.ID,
			TemplateID:       tmpl.ID,
			OwnerID:          user.ID,
			AutomaticUpdates: database.AutomaticUpdatesNever,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			TemplateVersionID: tv.ID,
			JobID:             job.ID,
			InitiatorID:       user.ID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})

		parentReadyAt := dbtime.Now()
		parentStartedAt := parentReadyAt.Add(-time.Second)
		parentAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID:     resource.ID,
			StartedAt:      sql.NullTime{Time: parentStartedAt, Valid: true},
			ReadyAt:        sql.NullTime{Time: parentReadyAt, Valid: true},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		})

		// Sub-agent with ready_at 1 hour later should be excluded.
		subAgentReadyAt := parentReadyAt.Add(time.Hour)
		subAgentStartedAt := subAgentReadyAt.Add(-time.Second)
		_ = dbgen.WorkspaceSubAgent(t, db, parentAgent, database.WorkspaceAgent{
			StartedAt:      sql.NullTime{Time: subAgentStartedAt, Valid: true},
			ReadyAt:        sql.NullTime{Time: subAgentReadyAt, Valid: true},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		})

		row, err := db.GetWorkspaceBuildMetricsByResourceID(ctx, resource.ID)
		require.NoError(t, err)
		require.True(t, row.AllAgentsReady)
		// LastAgentReadyAt should be the parent's, not the sub-agent's.
		require.True(t, parentReadyAt.Equal(row.LastAgentReadyAt))
		require.Equal(t, "success", row.WorstStatus)
	})
}

// TestUpsertAISeats verifies 'UpsertAISeatState' only returns true when a new
// row is inserted.
func TestUpsertAISeats(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)
	ctx := testutil.Context(t, testutil.WaitShort)

	now := dbtime.Now()

	user := dbgen.User(t, db, database.User{})
	newRow, err := db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:        user.ID,
		FirstUsedAt:   now.Add(time.Hour * -24),
		LastEventType: database.AISeatUsageReasonTask,
	})
	require.NoError(t, err)
	require.True(t, newRow)

	alreadyExists, err := db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:        user.ID,
		FirstUsedAt:   now.Add(time.Hour * -23),
		LastEventType: database.AISeatUsageReasonTask,
	})
	require.NoError(t, err)
	require.False(t, alreadyExists)

	alreadyExists, err = db.UpsertAISeatState(ctx, database.UpsertAISeatStateParams{
		UserID:        user.ID,
		FirstUsedAt:   now,
		LastEventType: database.AISeatUsageReasonTask,
	})
	require.NoError(t, err)
	require.False(t, alreadyExists)
}

func TestIncrementUserAIDailySpend(t *testing.T) {
	t.Parallel()

	// Use fixed dates to keep the test deterministic.
	day := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	nextDay := day.AddDate(0, 0, 1)

	// Given a sequence of costs upserted to the same (user, group, day),
	// when applied in order, then they accumulate into a single row.
	tests := []struct {
		name      string
		costs     []int64
		wantTotal int64
		wantErr   bool
	}{
		{name: "InsertsNewRow", costs: []int64{100}, wantTotal: 100},
		{name: "AccumulatesAcrossCalls", costs: []int64{100, 50, 30, 20}, wantTotal: 200},
		{name: "SchemaRejectsNegativeSpend", costs: []int64{-100}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

			var row database.AIUserDailySpend
			var err error
			for _, cost := range tt.costs {
				row, err = db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           user.ID,
					EffectiveGroupID: group.ID,
					Day:              day,
					CostMicros:       cost,
				})
				if err != nil {
					break
				}
			}
			if tt.wantErr {
				require.Error(t, err)
				require.True(t, database.IsCheckViolation(err, database.CheckAIUserDailySpendSpendMicrosCheck))
				return
			}
			require.NoError(t, err)
			require.Equal(t, user.ID, row.UserID)
			require.Equal(t, group.ID, row.EffectiveGroupID)
			require.Equal(t, tt.wantTotal, row.SpendMicros)
			require.True(t, row.Day.Equal(day),
				"row.Day = %s, want = %s", row.Day, day)
		})
	}

	// Given two users in the same group on the same day, when each upserts, then each gets its own row.
	t.Run("SeparateRowPerUser", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		userA := dbgen.User(t, db, database.User{})
		userB := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		userARow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: userA.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 100,
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), userARow.SpendMicros)

		userBRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: userB.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 25,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), userBRow.SpendMicros,
			"userB row must not include userA spend")
	})

	// Given one user across two groups on the same day, when each upserts, then each gets its own row.
	t.Run("SeparateRowPerEffectiveGroup", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		groupA := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
		groupB := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		groupARow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: groupA.ID, Day: day, CostMicros: 100,
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), groupARow.SpendMicros)

		groupBRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: groupB.ID, Day: day, CostMicros: 25,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), groupBRow.SpendMicros,
			"groupB row must not include groupA spend")
	})

	// Given existing spend on day, when the same user upserts on the next day, then a new row is created.
	t.Run("SeparateRowPerDay", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		dayRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 100,
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), dayRow.SpendMicros)

		// The ON CONFLICT target is the full PK including day, so this upsert
		// cannot modify the previous day's row by construction.
		nextDayRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: nextDay, CostMicros: 25,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), nextDayRow.SpendMicros,
			"nextDay row must not include day spend")
		require.True(t, nextDayRow.Day.Equal(nextDay))
	})

	// Given a non-midnight UTC time, when upserted, then it lands on the same row as the truncated day.
	t.Run("TruncatesDayToUTCMidnight", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 100,
		})
		require.NoError(t, err)

		dayNonTruncated := day.Add(14*time.Hour + 30*time.Minute)
		nonTruncatedRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: dayNonTruncated, CostMicros: 50,
		})
		require.NoError(t, err)
		require.Equal(t, int64(150), nonTruncatedRow.SpendMicros,
			"non-midnight UTC time should accumulate on the truncated day's row")
		require.True(t, nonTruncatedRow.Day.Equal(day),
			"row.Day = %s, want truncated = %s", nonTruncatedRow.Day, day)
	})

	// Given a non-UTC time that crosses the UTC date boundary, when upserted, then it lands on the UTC calendar day.
	t.Run("NormalizesNonUTCTimezones", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		// 2024-06-15 23:00 in UTC-5 is 2024-06-16 04:00 UTC, so this should land on nextDay (2024-06-16).
		localLate := time.Date(2024, 6, 15, 23, 0, 0, 0, time.FixedZone("UTC-5", -5*3600))
		nonUTCRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: localLate, CostMicros: 100,
		})
		require.NoError(t, err)
		require.True(t, nonUTCRow.Day.Equal(nextDay),
			"non-UTC input should land on the UTC calendar day (%s), got %s", nextDay, nonUTCRow.Day)
	})

	// Given a zero-cost upsert, when applied, then it is idempotent (creates a zero-spend row or leaves an existing one unchanged).
	t.Run("ZeroCostIsIdempotent", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		// Zero-cost upsert on a fresh key creates a row with spend = 0.
		newRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 0,
		})
		require.NoError(t, err)
		require.Equal(t, int64(0), newRow.SpendMicros)

		// After a real upsert, the row has spend = 100.
		updatedRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 100,
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), updatedRow.SpendMicros)

		// Zero-cost upsert on the existing row leaves spend unchanged.
		sameRow, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: day, CostMicros: 0,
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), sameRow.SpendMicros,
			"zero-cost upsert must not change existing spend")
	})
}

func TestGetUserAISpendSince(t *testing.T) {
	t.Parallel()

	// Use fixed dates to keep the test deterministic.
	monthStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	today := monthStart.AddDate(0, 0, 14)            // 2024-06-15
	prevMonthLastDay := monthStart.AddDate(0, 0, -1) // 2024-05-31

	type seedRow struct {
		day   time.Time
		spend int64
	}

	// Given seeded rows for a single (user, group), when querying since
	// monthStart, then the period sum is returned.
	tests := []struct {
		name      string
		rows      []seedRow
		wantSpend int64
	}{
		{name: "NoRows", wantSpend: 0},
		{name: "SingleRowOnToday", rows: []seedRow{{today, 100}}, wantSpend: 100},
		{name: "FirstOfMonthIncluded", rows: []seedRow{{monthStart, 50}}, wantSpend: 50},
		{name: "SumsMultipleDaysInMonth", rows: []seedRow{{monthStart, 50}, {today, 100}}, wantSpend: 150},
		{name: "ExcludesRowsBeforePeriodStart", rows: []seedRow{{prevMonthLastDay, 999}, {monthStart, 25}}, wantSpend: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

			for _, r := range tt.rows {
				_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           user.ID,
					EffectiveGroupID: group.ID,
					Day:              r.day,
					CostMicros:       r.spend,
				})
				require.NoError(t, err)
			}

			got, err := db.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
				UserID:           user.ID,
				EffectiveGroupID: group.ID,
				PeriodStart:      monthStart,
			})
			require.NoError(t, err)
			require.Equal(t, user.ID, got.UserID)
			require.Equal(t, group.ID, got.EffectiveGroupID)
			require.True(t, got.PeriodStart.Equal(monthStart),
				"PeriodStart = %s, want = %s", got.PeriodStart, monthStart)
			require.Equal(t, tt.wantSpend, got.SpendMicros)
		})
	}

	// Given two users with spend in the same group on the same day, when querying one user, then the other's spend is excluded.
	t.Run("SumExcludesOtherUsers", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		userA := dbgen.User(t, db, database.User{})
		userB := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: userA.ID, EffectiveGroupID: group.ID, Day: today, CostMicros: 100,
		})
		require.NoError(t, err)
		_, err = db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: userB.ID, EffectiveGroupID: group.ID, Day: today, CostMicros: 25,
		})
		require.NoError(t, err)

		got, err := db.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
			UserID:           userB.ID,
			EffectiveGroupID: group.ID,
			PeriodStart:      monthStart,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), got.SpendMicros,
			"userB sum must not include userA spend")
	})

	// Given one user with spend in two groups on the same day, when querying one group, then the other's spend is excluded.
	t.Run("SumExcludesOtherEffectiveGroups", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		groupA := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
		groupB := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: groupA.ID, Day: today, CostMicros: 100,
		})
		require.NoError(t, err)
		_, err = db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: groupB.ID, Day: today, CostMicros: 25,
		})
		require.NoError(t, err)

		got, err := db.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
			UserID:           user.ID,
			EffectiveGroupID: groupB.ID,
			PeriodStart:      monthStart,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), got.SpendMicros,
			"groupB sum must not include groupA spend")
	})

	// Given a non-UTC period_start that lands on the previous UTC day, when queried, then it normalizes and excludes the prior day's row.
	t.Run("NormalizesNonUTCPeriodStart", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})

		// Seed a row on prevMonthLastDay (which lies on May 31 UTC). A naive
		// query that does not normalize the period_start would include it.
		_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: prevMonthLastDay, CostMicros: 999,
		})
		require.NoError(t, err)
		_, err = db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID: user.ID, EffectiveGroupID: group.ID, Day: monthStart, CostMicros: 25,
		})
		require.NoError(t, err)

		// 2024-05-31 23:00 in UTC-5 is 2024-06-01 04:00 UTC, so the
		// normalized period_start lands on June 1.
		localLate := time.Date(2024, 5, 31, 23, 0, 0, 0, time.FixedZone("UTC-5", -5*3600))
		got, err := db.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
			UserID:           user.ID,
			EffectiveGroupID: group.ID,
			PeriodStart:      localLate,
		})
		require.NoError(t, err)
		require.True(t, got.PeriodStart.Equal(monthStart),
			"PeriodStart should be normalized to 2024-06-01 UTC, got %s", got.PeriodStart)
		require.Equal(t, int64(25), got.SpendMicros,
			"sum must exclude prevMonthLastDay row after normalization")
	})
}

func TestChatPinOrderQueries(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	setup := func(t *testing.T) (context.Context, database.Store, uuid.UUID, uuid.UUID, uuid.UUID) {
		t.Helper()

		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		owner := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})

		// Use background context for fixture setup so the
		// timed test context doesn't tick during DB init.
		bg := context.Background()
		dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider:             "openai",
			DisplayName:          "OpenAI",
			APIKey:               "test-key",
			Enabled:              true,
			CentralApiKeyEnabled: true,
		})

		modelCfg, err := insertChatModelConfigForTest(bg, t, db, "openai", database.InsertChatModelConfigParams{
			Model:                "test-model",
			DisplayName:          "Test Model",
			CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
			UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
			Enabled:              true,
			IsDefault:            true,
			ContextLimit:         128000,
			CompressionThreshold: 80,
			Options:              json.RawMessage(`{}`),
		})
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		return ctx, db, owner.ID, modelCfg.ID, org.ID
	}

	createChat := func(t *testing.T, ctx context.Context, db database.Store, ownerID, modelCfgID, orgID uuid.UUID, title string) database.Chat {
		t.Helper()

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    orgID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           ownerID,
			LastModelConfigID: modelCfgID,
			Title:             title,
		})
		require.NoError(t, err)
		return chat
	}

	requirePinOrders := func(t *testing.T, ctx context.Context, db database.Store, want map[uuid.UUID]int32) {
		t.Helper()

		for chatID, wantPinOrder := range want {
			chat, err := db.GetChatByID(ctx, chatID)
			require.NoError(t, err)
			require.EqualValues(t, wantPinOrder, chat.PinOrder)
		}
	}

	t.Run("PinChatByIDAppendsWithinOwner", func(t *testing.T) {
		t.Parallel()

		ctx, db, ownerID, modelCfgID, orgID := setup(t)
		first := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "first")
		second := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "second")
		third := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "third")

		otherOwner := dbgen.User(t, db, database.User{})
		other := createChat(t, ctx, db, otherOwner.ID, modelCfgID, orgID, "other-owner")

		require.NoError(t, db.PinChatByID(ctx, other.ID))
		require.NoError(t, db.PinChatByID(ctx, first.ID))
		require.NoError(t, db.PinChatByID(ctx, second.ID))
		require.NoError(t, db.PinChatByID(ctx, third.ID))

		requirePinOrders(t, ctx, db, map[uuid.UUID]int32{
			first.ID:  1,
			second.ID: 2,
			third.ID:  3,
			other.ID:  1,
		})
	})

	t.Run("UpdateChatPinOrderShiftsNeighborsAndClamps", func(t *testing.T) {
		t.Parallel()

		ctx, db, ownerID, modelCfgID, orgID := setup(t)
		first := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "first")
		second := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "second")
		third := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "third")

		for _, chat := range []database.Chat{first, second, third} {
			require.NoError(t, db.PinChatByID(ctx, chat.ID))
		}

		require.NoError(t, db.UpdateChatPinOrder(ctx, database.UpdateChatPinOrderParams{
			ID:       third.ID,
			PinOrder: 1,
		}))
		requirePinOrders(t, ctx, db, map[uuid.UUID]int32{
			first.ID:  2,
			second.ID: 3,
			third.ID:  1,
		})

		require.NoError(t, db.UpdateChatPinOrder(ctx, database.UpdateChatPinOrderParams{
			ID:       third.ID,
			PinOrder: 99,
		}))
		requirePinOrders(t, ctx, db, map[uuid.UUID]int32{
			first.ID:  1,
			second.ID: 2,
			third.ID:  3,
		})
	})

	t.Run("UnpinChatByIDCompactsPinnedChats", func(t *testing.T) {
		t.Parallel()

		ctx, db, ownerID, modelCfgID, orgID := setup(t)
		first := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "first")
		second := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "second")
		third := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "third")

		for _, chat := range []database.Chat{first, second, third} {
			require.NoError(t, db.PinChatByID(ctx, chat.ID))
		}

		require.NoError(t, db.UnpinChatByID(ctx, second.ID))
		requirePinOrders(t, ctx, db, map[uuid.UUID]int32{
			first.ID:  1,
			second.ID: 0,
			third.ID:  2,
		})
	})

	t.Run("ArchiveClearsPinAndExcludesFromRanking", func(t *testing.T) {
		t.Parallel()

		ctx, db, ownerID, modelCfgID, orgID := setup(t)
		first := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "first")
		second := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "second")
		third := createChat(t, ctx, db, ownerID, modelCfgID, orgID, "third")

		for _, chat := range []database.Chat{first, second, third} {
			require.NoError(t, db.PinChatByID(ctx, chat.ID))
		}

		// Archive the middle pin.
		_, err := db.ArchiveChatByID(ctx, second.ID)
		require.NoError(t, err)

		// Archived chat should have pin_order cleared. Remaining
		// pins keep their original positions; the next mutation
		// compacts via ROW_NUMBER().
		requirePinOrders(t, ctx, db, map[uuid.UUID]int32{
			first.ID:  1,
			second.ID: 0,
			third.ID:  3,
		})

		// Reorder among remaining active pins — archived chat
		// should not interfere with position calculation.
		require.NoError(t, db.UpdateChatPinOrder(ctx, database.UpdateChatPinOrderParams{
			ID:       third.ID,
			PinOrder: 1,
		}))
		// After reorder, ROW_NUMBER() compacts the sequence.
		requirePinOrders(t, ctx, db, map[uuid.UUID]int32{
			first.ID:  2,
			second.ID: 0,
			third.ID:  1,
		})
	})
}

func TestChatPinOrderConstraints(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})

	bg := context.Background()
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(bg, t, db, "openai", database.InsertChatModelConfigParams{
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	t.Run("ChildChatCannotBePinned", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		parent, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusCompleted,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "parent",
		})
		require.NoError(t, err)

		child, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusCompleted,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "child",
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		})
		require.NoError(t, err)

		err = db.PinChatByID(ctx, child.ID)
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckChatsPinOrderParentCheck))
	})

	t.Run("ArchivedChatCannotBePinned", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusCompleted,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "will be archived",
		})
		require.NoError(t, err)

		_, err = db.ArchiveChatByID(ctx, chat.ID)
		require.NoError(t, err)

		err = db.PinChatByID(ctx, chat.ID)
		require.Error(t, err)
		require.True(t, database.IsCheckViolation(err, database.CheckChatsPinOrderArchivedCheck))
	})
}

func TestChatLabels(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})

	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, db, "openai", database.InsertChatModelConfigParams{
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	t.Run("CreateWithLabels", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		labels := database.StringMap{"github.repo": "coder/coder", "env": "prod"}
		labelsJSON, err := json.Marshal(labels)
		require.NoError(t, err)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "labeled-chat",
			Labels: pqtype.NullRawMessage{
				RawMessage: labelsJSON,
				Valid:      true,
			},
		})
		require.NoError(t, err)
		require.Equal(t, database.StringMap{"github.repo": "coder/coder", "env": "prod"}, chat.Labels)
		require.Equal(t, owner.Username, chat.OwnerUsername)
		require.Equal(t, owner.Name, chat.OwnerName)

		// Read back and verify.
		fetched, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.Labels, fetched.Labels)
		require.Equal(t, owner.Username, fetched.OwnerUsername)
		require.Equal(t, owner.Name, fetched.OwnerName)
	})

	t.Run("CreateWithoutLabels", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "no-labels-chat",
		})
		require.NoError(t, err)
		// Default should be an empty map, not nil.
		require.NotNil(t, chat.Labels)
		require.Empty(t, chat.Labels)
	})

	t.Run("ListReturnsOwnerFields", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "owner-fields-chat-" + uuid.NewString(),
		})
		require.NoError(t, err)

		rows, err := db.GetChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  owner.ID,
		})
		require.NoError(t, err)

		chatIndex := slices.IndexFunc(rows, func(row database.GetChatsRow) bool {
			return row.Chat.ID == chat.ID
		})
		require.NotEqual(t, -1, chatIndex, "chat not found in GetChats result")
		require.Equal(t, owner.Username, rows[chatIndex].Chat.OwnerUsername)
		require.Equal(t, owner.Name, rows[chatIndex].Chat.OwnerName)
	})

	t.Run("ChildrenReturnOwnerFields", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		parent, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "owner-fields-parent-" + uuid.NewString(),
		})
		require.NoError(t, err)
		child, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "owner-fields-child-" + uuid.NewString(),
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		})
		require.NoError(t, err)

		rows, err := db.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
			ParentIds: []uuid.UUID{parent.ID},
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, child.ID, rows[0].Chat.ID)
		require.Equal(t, owner.Username, rows[0].Chat.OwnerUsername)
		require.Equal(t, owner.Name, rows[0].Chat.OwnerName)
	})

	t.Run("UpdateLabels", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "update-labels-chat",
		})
		require.NoError(t, err)
		require.Empty(t, chat.Labels)

		// Set labels.
		newLabels, err := json.Marshal(database.StringMap{"team": "backend"})
		require.NoError(t, err)
		updated, err := db.UpdateChatLabelsByID(ctx, database.UpdateChatLabelsByIDParams{
			ID:     chat.ID,
			Labels: newLabels,
		})
		require.NoError(t, err)
		require.Equal(t, database.StringMap{"team": "backend"}, updated.Labels)

		// Title should be unchanged.
		require.Equal(t, "update-labels-chat", updated.Title)

		// Clear labels by setting empty object.
		emptyLabels, err := json.Marshal(database.StringMap{})
		require.NoError(t, err)
		cleared, err := db.UpdateChatLabelsByID(ctx, database.UpdateChatLabelsByIDParams{
			ID:     chat.ID,
			Labels: emptyLabels,
		})
		require.NoError(t, err)
		require.Empty(t, cleared.Labels)
	})

	t.Run("UpdateTitleDoesNotAffectLabels", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		labels := database.StringMap{"pr": "1234"}
		labelsJSON, err := json.Marshal(labels)
		require.NoError(t, err)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           owner.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "original-title",
			Labels: pqtype.NullRawMessage{
				RawMessage: labelsJSON,
				Valid:      true,
			},
		})
		require.NoError(t, err)

		// Update title only — labels must survive.
		updated, err := db.UpdateChatByID(ctx, database.UpdateChatByIDParams{
			ID:    chat.ID,
			Title: "new-title",
		})
		require.NoError(t, err)
		require.Equal(t, "new-title", updated.Title)
		require.Equal(t, database.StringMap{"pr": "1234"}, updated.Labels)
		require.Equal(t, owner.Username, updated.OwnerUsername)
		require.Equal(t, owner.Name, updated.OwnerName)
	})

	t.Run("FilterByLabels", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Create three chats with different labels.
		for _, tc := range []struct {
			title  string
			labels database.StringMap
		}{
			{"filter-a", database.StringMap{"env": "prod", "team": "backend"}},
			{"filter-b", database.StringMap{"env": "prod", "team": "frontend"}},
			{"filter-c", database.StringMap{"env": "staging"}},
		} {
			labelsJSON, err := json.Marshal(tc.labels)
			require.NoError(t, err)
			_, err = db.InsertChat(ctx, database.InsertChatParams{
				OrganizationID:    org.ID,
				Status:            database.ChatStatusWaiting,
				ClientType:        database.ChatClientTypeUi,
				OwnerID:           owner.ID,
				LastModelConfigID: modelCfg.ID, Title: tc.title,
				Labels: pqtype.NullRawMessage{
					RawMessage: labelsJSON,
					Valid:      true,
				},
			})
			require.NoError(t, err)
		}

		// Filter by env=prod — should match filter-a and filter-b.
		filterJSON, err := json.Marshal(database.StringMap{"env": "prod"})
		require.NoError(t, err)
		results, err := db.GetChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  owner.ID,
			LabelFilter: pqtype.NullRawMessage{
				RawMessage: filterJSON,
				Valid:      true,
			},
		})
		require.NoError(t, err)

		titles := make([]string, 0, len(results))
		for _, c := range results {
			titles = append(titles, c.Chat.Title)
		}
		require.Contains(t, titles, "filter-a")
		require.Contains(t, titles, "filter-b")
		require.NotContains(t, titles, "filter-c")

		// Filter by env=prod AND team=backend — should match only filter-a.
		filterJSON, err = json.Marshal(database.StringMap{"env": "prod", "team": "backend"})
		require.NoError(t, err)
		results, err = db.GetChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  owner.ID,
			LabelFilter: pqtype.NullRawMessage{
				RawMessage: filterJSON,
				Valid:      true,
			},
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "filter-a", results[0].Chat.Title)
		// No filter should return all chats for this owner.
		allChats, err := db.GetChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  owner.ID,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(allChats), 3)
	})
}

func TestUpdateChatLastTurnSummary(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err)
	db := database.New(sqlDB)

	ctx := testutil.Context(t, testutil.WaitMedium)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})

	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, db, "openai", database.InsertChatModelConfigParams{
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "summary-chat",
	})
	require.NoError(t, err)

	affected, err := db.UpdateChatLastTurnSummary(ctx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: chat.HistoryVersion,
		LastTurnSummary:        sql.NullString{String: "resolved the issue", Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	fetched, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "resolved the issue", Valid: true}, fetched.LastTurnSummary)
	require.Equal(t, chat.UpdatedAt, fetched.UpdatedAt)

	affected, err = db.UpdateChatLastTurnSummary(ctx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: chat.HistoryVersion,
		LastTurnSummary:        sql.NullString{String: " \n\t ", Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	fetched, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, fetched.LastTurnSummary.Valid)
	require.Equal(t, chat.UpdatedAt, fetched.UpdatedAt)

	affected, err = db.UpdateChatLastTurnSummary(ctx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: chat.HistoryVersion,
		LastTurnSummary:        sql.NullString{String: "fresh summary", Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	// Advance updated_at with a title write so the next assertion can
	// prove the summary update preserves the stored value.
	advanced, err := db.UpdateChatByID(ctx, database.UpdateChatByIDParams{
		ID:    chat.ID,
		Title: "summary-chat-advanced",
	})
	require.NoError(t, err)

	affected, err = db.UpdateChatLastTurnSummary(ctx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: chat.HistoryVersion,
		LastTurnSummary:        sql.NullString{String: "still fresh summary", Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	fetched, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "still fresh summary", Valid: true}, fetched.LastTurnSummary)
	require.Equal(t, advanced.UpdatedAt, fetched.UpdatedAt)

	_, err = db.LockChatAndBumpSnapshotVersion(ctx, chat.ID)
	require.NoError(t, err)
	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{owner.ID},
		ModelConfigID:       []uuid.UUID{modelCfg.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
		Content:             []string{`[{"type":"text","text":"new request"}]`},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	affected, err = db.UpdateChatLastTurnSummary(ctx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: chat.HistoryVersion,
		LastTurnSummary:        sql.NullString{String: "stale summary", Valid: true},
	})
	require.NoError(t, err)
	require.Zero(t, affected)

	fetched, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "still fresh summary", Valid: true}, fetched.LastTurnSummary)
	require.NotEqual(t, chat.HistoryVersion, fetched.HistoryVersion)
}

func TestDeleteChatDebugDataAfterMessageIDIncludesTriggeredRuns(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-debug-rollback-" + uuid.NewString(),
	})
	require.NoError(t, err)

	const cutoff int64 = 50

	affectedRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: cutoff + 10, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff - 5, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
	})
	require.NoError(t, err)

	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      affectedRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
	})
	require.NoError(t, err)

	affectedByStepHistoryTipRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: cutoff - 1, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff - 1, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
	})
	require.NoError(t, err)

	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:               affectedByStepHistoryTipRun.ID,
		ChatID:              chat.ID,
		StepNumber:          1,
		Operation:           "stream",
		Status:              "interrupted",
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff + 7, Valid: true},
	})
	require.NoError(t, err)

	// affectedByStepAssistantMsgRun: run-level fields are at/below
	// the cutoff, but its step has assistant_message_id above the
	// cutoff.  This exercises the step.assistant_message_id > cutoff
	// branch of the UNION independently of history_tip_message_id.
	affectedByStepAssistantMsgRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: cutoff - 2, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff - 2, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
	})
	require.NoError(t, err)

	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:              affectedByStepAssistantMsgRun.ID,
		ChatID:             chat.ID,
		StepNumber:         1,
		Operation:          "stream",
		Status:             "completed",
		AssistantMessageID: sql.NullInt64{Int64: cutoff + 3, Valid: true},
	})
	require.NoError(t, err)

	unaffectedRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: cutoff, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
	})
	require.NoError(t, err)

	unaffectedStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:              unaffectedRun.ID,
		ChatID:             chat.ID,
		StepNumber:         1,
		Operation:          "stream",
		Status:             "in_progress",
		AssistantMessageID: sql.NullInt64{Int64: cutoff, Valid: true},
	})
	require.NoError(t, err)

	deletedRows, err := store.DeleteChatDebugDataAfterMessageID(ctx, database.DeleteChatDebugDataAfterMessageIDParams{
		ChatID:        chat.ID,
		MessageID:     cutoff,
		StartedBefore: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, deletedRows)

	_, err = store.GetChatDebugRunByID(ctx, affectedRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	affectedSteps, err := store.GetChatDebugStepsByRunID(ctx, affectedRun.ID)
	require.NoError(t, err)
	require.Empty(t, affectedSteps)

	_, err = store.GetChatDebugRunByID(ctx, affectedByStepHistoryTipRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	affectedByStepHistoryTipSteps, err := store.GetChatDebugStepsByRunID(ctx, affectedByStepHistoryTipRun.ID)
	require.NoError(t, err)
	require.Empty(t, affectedByStepHistoryTipSteps)

	// Verify the run caught by step-level assistant_message_id is
	// also deleted.  This would survive if the
	// step.assistant_message_id > @message_id clause were removed.
	_, err = store.GetChatDebugRunByID(ctx, affectedByStepAssistantMsgRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	affectedByStepAssistantMsgSteps, err := store.GetChatDebugStepsByRunID(ctx, affectedByStepAssistantMsgRun.ID)
	require.NoError(t, err)
	require.Empty(t, affectedByStepAssistantMsgSteps)

	remainingRuns, err := store.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID:   chat.ID,
		LimitVal: 100,
	})
	require.NoError(t, err)
	require.Len(t, remainingRuns, 1)
	require.Equal(t, unaffectedRun.ID, remainingRuns[0].ID)

	remainingRun, err := store.GetChatDebugRunByID(ctx, unaffectedRun.ID)
	require.NoError(t, err)
	require.Equal(t, unaffectedRun.ID, remainingRun.ID)

	remainingSteps, err := store.GetChatDebugStepsByRunID(ctx, unaffectedRun.ID)
	require.NoError(t, err)
	require.Len(t, remainingSteps, 1)
	require.Equal(t, unaffectedStep.ID, remainingSteps[0].ID)
}

// TestDeleteChatDebugDataAfterMessageIDStepLevelFieldBoundariesAndNulls
// verifies that DeleteChatDebugDataAfterMessageID handles step-level
// field boundaries and NULL combinations when run-level message IDs are
// below the cutoff. This complements the triggered-runs test with extra
// coverage for strict step-level comparisons and SQL NULL behavior.
func TestDeleteChatDebugDataAfterMessageIDStepLevelFieldBoundariesAndNulls(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-step-boundaries-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-debug-step-boundaries-" + uuid.NewString(),
	})
	require.NoError(t, err)

	const cutoff int64 = 100

	// insertRunBelowRunLevelCutoff creates a run whose run-level message
	// IDs cannot match the deletion query. The step-level fields decide
	// whether the run is deleted.
	insertRunBelowRunLevelCutoff := func(t *testing.T) database.ChatDebugRun {
		t.Helper()
		run, runErr := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
			ChatID:              chat.ID,
			ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
			TriggerMessageID:    sql.NullInt64{Int64: cutoff - 10, Valid: true},
			HistoryTipMessageID: sql.NullInt64{Int64: cutoff - 10, Valid: true},
			Kind:                "chat_turn",
			Status:              "in_progress",
			Provider:            sql.NullString{String: providerName, Valid: true},
			Model:               sql.NullString{String: modelName, Valid: true},
		})
		require.NoError(t, runErr)
		return run
	}

	// assistantAboveWithNullHistoryTipRun is deleted only through the
	// step.assistant_message_id clause.
	assistantAboveWithNullHistoryTipRun := insertRunBelowRunLevelCutoff(t)
	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:              assistantAboveWithNullHistoryTipRun.ID,
		ChatID:             chat.ID,
		StepNumber:         1,
		Operation:          "stream",
		Status:             "completed",
		AssistantMessageID: sql.NullInt64{Int64: cutoff + 5, Valid: true},
		// HistoryTipMessageID intentionally omitted (NULL).
	})
	require.NoError(t, err)

	// Add a nonmatching step to verify that one matching step is enough
	// to delete the run and cascade all of its steps.
	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:              assistantAboveWithNullHistoryTipRun.ID,
		ChatID:             chat.ID,
		StepNumber:         2,
		Operation:          "stream",
		Status:             "completed",
		AssistantMessageID: sql.NullInt64{Int64: cutoff - 5, Valid: true},
		// HistoryTipMessageID intentionally omitted (NULL).
	})
	require.NoError(t, err)

	// assistantAboveWithHistoryTipBelowRun is deleted through the
	// step.assistant_message_id clause while the step history tip stays
	// below the cutoff.
	assistantAboveWithHistoryTipBelowRun := insertRunBelowRunLevelCutoff(t)
	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:               assistantAboveWithHistoryTipBelowRun.ID,
		ChatID:              chat.ID,
		StepNumber:          1,
		Operation:           "stream",
		Status:              "completed",
		AssistantMessageID:  sql.NullInt64{Int64: cutoff + 20, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff - 3, Valid: true},
	})
	require.NoError(t, err)

	// assistantBelowWithNullHistoryTipRun survives because its step
	// assistant_message_id is below the cutoff and step history tip is
	// NULL.
	assistantBelowWithNullHistoryTipRun := insertRunBelowRunLevelCutoff(t)
	assistantBelowWithNullHistoryTipStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:              assistantBelowWithNullHistoryTipRun.ID,
		ChatID:             chat.ID,
		StepNumber:         1,
		Operation:          "stream",
		Status:             "completed",
		AssistantMessageID: sql.NullInt64{Int64: cutoff - 3, Valid: true},
	})
	require.NoError(t, err)

	// assistantAtBoundaryWithNullHistoryTipRun survives because the
	// query uses strict greater-than, not greater-than-or-equal.
	assistantAtBoundaryWithNullHistoryTipRun := insertRunBelowRunLevelCutoff(t)
	assistantAtBoundaryWithNullHistoryTipStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:              assistantAtBoundaryWithNullHistoryTipRun.ID,
		ChatID:             chat.ID,
		StepNumber:         1,
		Operation:          "stream",
		Status:             "completed",
		AssistantMessageID: sql.NullInt64{Int64: cutoff, Valid: true},
	})
	require.NoError(t, err)

	// historyTipAboveWithNullAssistantRun is deleted through the
	// step.history_tip_message_id clause while assistant_message_id is
	// NULL.
	historyTipAboveWithNullAssistantRun := insertRunBelowRunLevelCutoff(t)
	_, err = store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:               historyTipAboveWithNullAssistantRun.ID,
		ChatID:              chat.ID,
		StepNumber:          1,
		Operation:           "stream",
		Status:              "completed",
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff + 2, Valid: true},
		// AssistantMessageID intentionally omitted (NULL).
	})
	require.NoError(t, err)

	// historyTipAtBoundaryWithNullAssistantRun survives because the
	// step history tip uses strict greater-than, not greater-than-or-equal.
	historyTipAtBoundaryWithNullAssistantRun := insertRunBelowRunLevelCutoff(t)
	historyTipAtBoundaryWithNullAssistantStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:               historyTipAtBoundaryWithNullAssistantRun.ID,
		ChatID:              chat.ID,
		StepNumber:          1,
		Operation:           "stream",
		Status:              "completed",
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff, Valid: true},
		// AssistantMessageID intentionally omitted (NULL).
	})
	require.NoError(t, err)

	// bothStepMessageIDsNullRun survives because NULL > N evaluates to
	// NULL, not TRUE, in SQL.
	bothStepMessageIDsNullRun := insertRunBelowRunLevelCutoff(t)
	bothStepMessageIDsNullStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      bothStepMessageIDsNullRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "completed",
		// Both message IDs intentionally omitted (NULL).
	})
	require.NoError(t, err)

	deletedRows, err := store.DeleteChatDebugDataAfterMessageID(ctx, database.DeleteChatDebugDataAfterMessageIDParams{
		ChatID:        chat.ID,
		MessageID:     cutoff,
		StartedBefore: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, deletedRows)

	_, err = store.GetChatDebugRunByID(ctx, assistantAboveWithNullHistoryTipRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows,
		"assistant above cutoff with NULL history tip must be deleted")

	_, err = store.GetChatDebugRunByID(ctx, assistantAboveWithHistoryTipBelowRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows,
		"assistant above cutoff with history tip below cutoff must be deleted")

	_, err = store.GetChatDebugRunByID(ctx, historyTipAboveWithNullAssistantRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows,
		"NULL assistant with history tip above cutoff must be deleted")

	for _, deletedRun := range []struct {
		name string
		id   uuid.UUID
	}{
		{name: "assistant above cutoff with NULL history tip", id: assistantAboveWithNullHistoryTipRun.ID},
		{name: "assistant above cutoff with history tip below cutoff", id: assistantAboveWithHistoryTipBelowRun.ID},
		{name: "NULL assistant with history tip above cutoff", id: historyTipAboveWithNullAssistantRun.ID},
	} {
		steps, stepsErr := store.GetChatDebugStepsByRunID(ctx, deletedRun.id)
		require.NoError(t, stepsErr, "%s: get cascaded steps", deletedRun.name)
		require.Empty(t, steps, "%s: deleted run steps must cascade", deletedRun.name)
	}

	remainingAssistantBelowRun, err := store.GetChatDebugRunByID(ctx, assistantBelowWithNullHistoryTipRun.ID)
	require.NoError(t, err)
	require.Equal(t, assistantBelowWithNullHistoryTipRun.ID, remainingAssistantBelowRun.ID,
		"assistant below cutoff with NULL history tip must survive")

	remainingAssistantAtBoundaryRun, err := store.GetChatDebugRunByID(ctx, assistantAtBoundaryWithNullHistoryTipRun.ID)
	require.NoError(t, err)
	require.Equal(t, assistantAtBoundaryWithNullHistoryTipRun.ID, remainingAssistantAtBoundaryRun.ID,
		"assistant at cutoff boundary with NULL history tip must survive")

	remainingHistoryTipAtBoundaryRun, err := store.GetChatDebugRunByID(ctx, historyTipAtBoundaryWithNullAssistantRun.ID)
	require.NoError(t, err)
	require.Equal(t, historyTipAtBoundaryWithNullAssistantRun.ID, remainingHistoryTipAtBoundaryRun.ID,
		"history tip at cutoff boundary with NULL assistant must survive")

	remainingBothStepMessageIDsNullRun, err := store.GetChatDebugRunByID(ctx, bothStepMessageIDsNullRun.ID)
	require.NoError(t, err)
	require.Equal(t, bothStepMessageIDsNullRun.ID, remainingBothStepMessageIDsNullRun.ID,
		"both step message IDs NULL must survive")

	assistantBelowSteps, err := store.GetChatDebugStepsByRunID(ctx, assistantBelowWithNullHistoryTipRun.ID)
	require.NoError(t, err)
	require.Len(t, assistantBelowSteps, 1)
	require.Equal(t, assistantBelowWithNullHistoryTipStep.ID, assistantBelowSteps[0].ID)

	assistantAtBoundarySteps, err := store.GetChatDebugStepsByRunID(ctx, assistantAtBoundaryWithNullHistoryTipRun.ID)
	require.NoError(t, err)
	require.Len(t, assistantAtBoundarySteps, 1)
	require.Equal(t, assistantAtBoundaryWithNullHistoryTipStep.ID, assistantAtBoundarySteps[0].ID)

	historyTipAtBoundarySteps, err := store.GetChatDebugStepsByRunID(ctx, historyTipAtBoundaryWithNullAssistantRun.ID)
	require.NoError(t, err)
	require.Len(t, historyTipAtBoundarySteps, 1)
	require.Equal(t, historyTipAtBoundaryWithNullAssistantStep.ID, historyTipAtBoundarySteps[0].ID)

	bothStepMessageIDsNullSteps, err := store.GetChatDebugStepsByRunID(ctx, bothStepMessageIDsNullRun.ID)
	require.NoError(t, err)
	require.Len(t, bothStepMessageIDsNullSteps, 1)
	require.Equal(t, bothStepMessageIDsNullStep.ID, bothStepMessageIDsNullSteps[0].ID)

	remaining, err := store.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID:   chat.ID,
		LimitVal: 100,
	})
	require.NoError(t, err)
	require.Len(t, remaining, 4)
}

func TestFinalizeStaleChatDebugRows(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-finalize-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-finalize-" + uuid.NewString(),
	})
	require.NoError(t, err)

	// staleTime is well before the threshold so rows stamped with it
	// are considered stale.  The threshold sits between staleTime and
	// NOW(), letting us create rows that are stale-by-age and rows
	// that are fresh-by-age in the same test.
	staleTime := time.Now().Add(-2 * time.Hour)
	staleThreshold := time.Now().Add(-1 * time.Hour)

	// preExistingError is attached to staleStep so we can verify
	// that finalization preserves pre-existing error JSON rather
	// than clearing or overwriting it.
	preExistingError := json.RawMessage(`{"code":"timeout","message":"upstream deadline exceeded"}`)

	// --- staleRun: in_progress run with no finished_at --- should be
	// finalized.
	staleRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 1, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 1, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
		UpdatedAt:           sql.NullTime{Time: staleTime, Valid: true},
	})
	require.NoError(t, err)

	// staleStep: in_progress step attached to staleRun with a
	// pre-existing error JSON payload.
	staleStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      staleRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
		UpdatedAt:  sql.NullTime{Time: staleTime, Valid: true},
		Error: pqtype.NullRawMessage{
			RawMessage: preExistingError,
			Valid:      true,
		},
	})
	require.NoError(t, err)
	require.True(t, staleStep.Error.Valid,
		"precondition: error must be stored at insertion")

	// --- orphanStep: in_progress step whose run is already completed ---
	// Its own updated_at is old, so it should be finalized directly.
	// The step must be inserted while the run is still open because
	// InsertChatDebugStep requires finished_at IS NULL on the parent
	// run (atomic guard against appending steps to finalized runs).
	completedRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 2, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 2, Valid: true},
		Kind:                "chat_turn",
		Status:              "completed",
	})
	require.NoError(t, err)

	// Insert the step while the run is still open (finished_at IS NULL).
	orphanStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      completedRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
		UpdatedAt:  sql.NullTime{Time: staleTime, Valid: true},
	})
	require.NoError(t, err)

	// Now mark the run as completed with a finished_at timestamp,
	// leaving the step orphaned in in_progress state.
	_, err = store.UpdateChatDebugRun(ctx, database.UpdateChatDebugRunParams{
		ID:     completedRun.ID,
		ChatID: completedRun.ChatID,
		Status: sql.NullString{String: "completed", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		Now: time.Now(),
	})
	require.NoError(t, err)

	// --- cascadeRun: stale in_progress run with a FRESH step ---
	// The run's updated_at is old so the run itself is finalized by
	// age.  The step's updated_at is recent (default NOW()), so it is
	// NOT caught by the age predicate.  It must be finalized solely
	// via the cascade CTE clause: run_id IN (SELECT id FROM
	// finalized_runs).  Removing that clause would leave this step
	// stuck in 'in_progress'.
	cascadeRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 10, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 10, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
		UpdatedAt:           sql.NullTime{Time: staleTime, Valid: true},
	})
	require.NoError(t, err)

	// cascadeStep: recent updated_at (default NOW()), so only the
	// cascade path can finalize it.
	cascadeStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      cascadeRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
	})
	require.NoError(t, err)

	// The InsertChatDebugStep CTE atomically bumps the parent run's
	// updated_at to NOW(). Reset it back to staleTime so the run is
	// still caught by the age predicate in FinalizeStaleChatDebugRows.
	err = store.TouchChatDebugRunUpdatedAt(ctx, database.TouchChatDebugRunUpdatedAtParams{
		ID:     cascadeRun.ID,
		ChatID: chat.ID,
		Now:    staleTime,
	})
	require.NoError(t, err)

	// --- alreadyDone: completed run/step --- should NOT be touched.
	doneRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 3, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 3, Valid: true},
		Kind:                "chat_turn",
		Status:              "completed",
	})
	require.NoError(t, err)

	// Insert step while run is still open.
	doneStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      doneRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "completed",
	})
	require.NoError(t, err)

	// Now finalize both run and step.
	_, err = store.UpdateChatDebugRun(ctx, database.UpdateChatDebugRunParams{
		ID:     doneRun.ID,
		ChatID: doneRun.ChatID,
		Status: sql.NullString{String: "completed", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		Now: time.Now(),
	})
	require.NoError(t, err)

	_, err = store.UpdateChatDebugStep(ctx, database.UpdateChatDebugStepParams{
		ID:     doneStep.ID,
		ChatID: chat.ID,
		Status: sql.NullString{String: "completed", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		Now: time.Now(),
	})
	require.NoError(t, err)

	// --- errorRun: error run/step --- should NOT be touched either,
	// exercising the 'error' branch of the NOT IN clause.
	errorRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 4, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 4, Valid: true},
		Kind:                "chat_turn",
		Status:              "error",
	})
	require.NoError(t, err)

	// Insert step while run is still open.
	errorStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      errorRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "error",
	})
	require.NoError(t, err)

	// Now finalize both run and step.
	_, err = store.UpdateChatDebugRun(ctx, database.UpdateChatDebugRunParams{
		ID:     errorRun.ID,
		ChatID: errorRun.ChatID,
		Status: sql.NullString{String: "error", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		Now: time.Now(),
	})
	require.NoError(t, err)

	_, err = store.UpdateChatDebugStep(ctx, database.UpdateChatDebugStepParams{
		ID:     errorStep.ID,
		ChatID: chat.ID,
		Status: sql.NullString{String: "error", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		Now: time.Now(),
	})
	require.NoError(t, err)

	// --- freshRun: recent in_progress run with current timestamp ---
	// should NOT be finalized because its updated_at is after the
	// threshold, exercising the age predicate (not just terminal
	// status) as the survival reason.
	freshRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 20, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 20, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
		// UpdatedAt defaults to NOW(), which is after staleThreshold.
	})
	require.NoError(t, err)

	freshStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      freshRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
		// UpdatedAt defaults to NOW().
	})
	require.NoError(t, err)

	// --- Execute the finalization sweep. ---
	// Capture the @now timestamp so we can verify finalized rows
	// received exactly this value for updated_at and finished_at.
	nowParam := time.Now().Truncate(time.Microsecond)
	result, err := store.FinalizeStaleChatDebugRows(ctx, database.FinalizeStaleChatDebugRowsParams{
		Now:           nowParam,
		UpdatedBefore: staleThreshold,
	})
	require.NoError(t, err)

	// staleRun + cascadeRun were finalized; completedRun and doneRun
	// were already terminal, and freshRun survives because its
	// updated_at is after the threshold — so only 2 runs are expected.
	assert.EqualValues(t, 2, result.RunsFinalized,
		"stale + cascade in_progress runs should be finalized")
	// staleStep (age), orphanStep (age), cascadeStep (cascade only)
	// should all be finalized.
	assert.EqualValues(t, 3, result.StepsFinalized,
		"stale step + orphan step + cascade step should all be finalized")

	// Verify the stale run was set to interrupted with correct
	// timestamps matching the @now parameter.
	updatedStaleRun, err := store.GetChatDebugRunByID(ctx, staleRun.ID)
	require.NoError(t, err)
	assert.Equal(t, "interrupted", updatedStaleRun.Status)
	assert.True(t, updatedStaleRun.FinishedAt.Valid,
		"finalized run should have a finished_at timestamp")
	assert.WithinDuration(t, nowParam, updatedStaleRun.FinishedAt.Time, time.Microsecond,
		"finished_at should match the @now parameter")
	assert.WithinDuration(t, nowParam, updatedStaleRun.UpdatedAt, time.Microsecond,
		"updated_at should match the @now parameter")

	// Verify the stale step was set to interrupted and its
	// pre-existing error JSON was preserved.
	staleSteps, err := store.GetChatDebugStepsByRunID(ctx, staleRun.ID)
	require.NoError(t, err)
	require.Len(t, staleSteps, 1)
	assert.Equal(t, staleStep.ID, staleSteps[0].ID)
	assert.Equal(t, "interrupted", staleSteps[0].Status)
	assert.True(t, staleSteps[0].FinishedAt.Valid,
		"finalized step should have a finished_at timestamp")
	assert.WithinDuration(t, nowParam, staleSteps[0].FinishedAt.Time, time.Microsecond,
		"step finished_at should match the @now parameter")
	assert.WithinDuration(t, nowParam, staleSteps[0].UpdatedAt, time.Microsecond,
		"step updated_at should match the @now parameter")
	// The error JSON that was set at insertion time must survive
	// finalization. The query does not touch the error column, so
	// this proves the JSONB payload is preserved.
	assert.True(t, staleSteps[0].Error.Valid,
		"pre-existing error JSON must be preserved after finalization")
	assert.JSONEq(t, string(preExistingError), string(staleSteps[0].Error.RawMessage),
		"error JSON content must match the value set at insertion")

	// Verify the orphan step was also finalized with correct timestamps.
	orphanSteps, err := store.GetChatDebugStepsByRunID(ctx, completedRun.ID)
	require.NoError(t, err)
	require.Len(t, orphanSteps, 1)
	assert.Equal(t, orphanStep.ID, orphanSteps[0].ID)
	assert.Equal(t, "interrupted", orphanSteps[0].Status)
	assert.True(t, orphanSteps[0].FinishedAt.Valid,
		"orphan step should have a finished_at timestamp")
	assert.WithinDuration(t, nowParam, orphanSteps[0].FinishedAt.Time, time.Microsecond,
		"orphan step finished_at should match the @now parameter")
	assert.WithinDuration(t, nowParam, orphanSteps[0].UpdatedAt, time.Microsecond,
		"orphan step updated_at should match the @now parameter")
	// The orphan step had no error set; verify it remains null.
	assert.False(t, orphanSteps[0].Error.Valid,
		"step without pre-existing error should remain null after finalization")

	// Verify the cascade run was finalized with correct timestamps.
	updatedCascadeRun, err := store.GetChatDebugRunByID(ctx, cascadeRun.ID)
	require.NoError(t, err)
	assert.Equal(t, "interrupted", updatedCascadeRun.Status)
	assert.True(t, updatedCascadeRun.FinishedAt.Valid,
		"cascade run should have a finished_at timestamp")
	assert.WithinDuration(t, nowParam, updatedCascadeRun.FinishedAt.Time, time.Microsecond,
		"cascade run finished_at should match the @now parameter")
	assert.WithinDuration(t, nowParam, updatedCascadeRun.UpdatedAt, time.Microsecond,
		"cascade run updated_at should match the @now parameter")

	// Verify the cascade step was finalized despite its recent
	// updated_at, proving the cascade CTE clause is required.
	cascadeSteps, err := store.GetChatDebugStepsByRunID(ctx, cascadeRun.ID)
	require.NoError(t, err)
	require.Len(t, cascadeSteps, 1)
	assert.Equal(t, cascadeStep.ID, cascadeSteps[0].ID)
	assert.Equal(t, "interrupted", cascadeSteps[0].Status,
		"fresh step should be finalized via cascade, not age")
	assert.True(t, cascadeSteps[0].FinishedAt.Valid,
		"cascade step should have a finished_at timestamp")
	assert.WithinDuration(t, nowParam, cascadeSteps[0].FinishedAt.Time, time.Microsecond,
		"cascade step finished_at should match the @now parameter")
	assert.WithinDuration(t, nowParam, cascadeSteps[0].UpdatedAt, time.Microsecond,
		"cascade step updated_at should match the @now parameter")
	// The cascade step also had no error set.
	assert.False(t, cascadeSteps[0].Error.Valid,
		"cascade step without pre-existing error should remain null")

	// Verify the completed run/step are untouched.
	unchangedRun, err := store.GetChatDebugRunByID(ctx, doneRun.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", unchangedRun.Status)

	doneSteps, err := store.GetChatDebugStepsByRunID(ctx, doneRun.ID)
	require.NoError(t, err)
	require.Len(t, doneSteps, 1)
	assert.Equal(t, "completed", doneSteps[0].Status)

	// Verify the error run/step are untouched.
	unchangedErrorRun, err := store.GetChatDebugRunByID(ctx, errorRun.ID)
	require.NoError(t, err)
	assert.Equal(t, "error", unchangedErrorRun.Status)

	errorSteps, err := store.GetChatDebugStepsByRunID(ctx, errorRun.ID)
	require.NoError(t, err)
	require.Len(t, errorSteps, 1)
	assert.Equal(t, "error", errorSteps[0].Status)

	// Verify the fresh in_progress run survived due to recency,
	// not terminal status — its updated_at is after the threshold.
	unchangedFreshRun, err := store.GetChatDebugRunByID(ctx, freshRun.ID)
	require.NoError(t, err)
	assert.Equal(t, "in_progress", unchangedFreshRun.Status,
		"fresh in_progress run must survive due to recency")
	assert.False(t, unchangedFreshRun.FinishedAt.Valid,
		"fresh run should not have a finished_at timestamp")

	freshSteps, err := store.GetChatDebugStepsByRunID(ctx, freshRun.ID)
	require.NoError(t, err)
	require.Len(t, freshSteps, 1)
	assert.Equal(t, freshStep.ID, freshSteps[0].ID)
	assert.Equal(t, "in_progress", freshSteps[0].Status,
		"fresh in_progress step must survive due to recency")
	assert.False(t, freshSteps[0].FinishedAt.Valid,
		"fresh step should not have a finished_at timestamp")

	// A second sweep should be a no-op.
	result2, err := store.FinalizeStaleChatDebugRows(ctx, database.FinalizeStaleChatDebugRowsParams{
		Now:           time.Now(),
		UpdatedBefore: staleThreshold,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, result2.RunsFinalized,
		"second sweep should find nothing to finalize")
	assert.EqualValues(t, 0, result2.StepsFinalized,
		"second sweep should find nothing to finalize")
}

func TestChatDebugSQLGuards(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-guards-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chatA, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-guard-A-" + uuid.NewString(),
	})
	require.NoError(t, err)

	chatB, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-guard-B-" + uuid.NewString(),
	})
	require.NoError(t, err)

	runA, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chatA.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 1, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 1, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
	})
	require.NoError(t, err)

	stepA, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      runA.ID,
		ChatID:     chatA.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
	})
	require.NoError(t, err)

	// InsertChatDebugStep: valid run_id but chat_id belongs to a
	// different chat.  The INSERT...SELECT guard should produce zero
	// rows, surfacing as sql.ErrNoRows.
	t.Run("InsertChatDebugStep_MismatchedChatID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
			RunID:      runA.ID,
			ChatID:     chatB.ID, // wrong chat
			StepNumber: 2,
			Operation:  "stream",
			Status:     "in_progress",
		})
		require.ErrorIs(t, err, sql.ErrNoRows,
			"InsertChatDebugStep should fail when chat_id does not match the run's chat_id")
	})

	// UpdateChatDebugRun: valid run ID but wrong chat_id.
	t.Run("UpdateChatDebugRun_MismatchedChatID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := store.UpdateChatDebugRun(ctx, database.UpdateChatDebugRunParams{
			ID:     runA.ID,
			ChatID: chatB.ID, // wrong chat
			Status: sql.NullString{String: "completed", Valid: true},
			FinishedAt: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
			Now: time.Now(),
		})
		require.ErrorIs(t, err, sql.ErrNoRows,
			"UpdateChatDebugRun should fail when chat_id does not match")
	})

	// UpdateChatDebugStep: valid step ID but wrong chat_id.
	t.Run("UpdateChatDebugStep_MismatchedChatID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := store.UpdateChatDebugStep(ctx, database.UpdateChatDebugStepParams{
			ID:     stepA.ID,
			ChatID: chatB.ID, // wrong chat
			Status: sql.NullString{String: "completed", Valid: true},
			FinishedAt: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
			Now: time.Now(),
		})
		require.ErrorIs(t, err, sql.ErrNoRows,
			"UpdateChatDebugStep should fail when chat_id does not match")
	})
}

// TestChatDebugRunCOALESCEPreservation verifies that the COALESCE
// pattern in UpdateChatDebugRun preserves every field that was not
// explicitly supplied in the update.  If COALESCE were removed from
// any column, the corresponding field would silently null out.
func TestChatDebugRunCOALESCEPreservation(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-coalesce-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-debug-coalesce-" + uuid.NewString(),
	})
	require.NoError(t, err)

	rootChatID := uuid.New()
	parentChatID := uuid.New()

	// Insert a fully-populated run so every nullable field has a value.
	original, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		RootChatID:          uuid.NullUUID{UUID: rootChatID, Valid: true},
		ParentChatID:        uuid.NullUUID{UUID: parentChatID, Valid: true},
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: 42, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: 41, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
		Summary:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"key":"val"}`), Valid: true},
	})
	require.NoError(t, err)

	// Update only Status and FinishedAt. Every other nullable param
	// is left as its Go zero value (Valid: false → SQL NULL), which
	// the COALESCE pattern should interpret as "keep existing."
	now := time.Now()
	updated, err := store.UpdateChatDebugRun(ctx, database.UpdateChatDebugRunParams{
		ID:     original.ID,
		ChatID: chat.ID,
		Status: sql.NullString{String: "completed", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  now,
			Valid: true,
		},
		Now: now,
	})
	require.NoError(t, err)

	// Status and FinishedAt should be updated.
	require.Equal(t, "completed", updated.Status)
	require.True(t, updated.FinishedAt.Valid)

	// UpdatedAt should be set to the @now value we passed in.
	require.WithinDuration(t, now, updated.UpdatedAt, time.Millisecond,
		"updated_at should equal the @now parameter")

	// Every field not in the update call must be preserved exactly.
	require.Equal(t, original.RootChatID, updated.RootChatID,
		"RootChatID should survive a partial update")
	require.Equal(t, original.ParentChatID, updated.ParentChatID,
		"ParentChatID should survive a partial update")
	require.Equal(t, original.ModelConfigID, updated.ModelConfigID,
		"ModelConfigID should survive a partial update")
	require.Equal(t, original.TriggerMessageID, updated.TriggerMessageID,
		"TriggerMessageID should survive a partial update")
	require.Equal(t, original.HistoryTipMessageID, updated.HistoryTipMessageID,
		"HistoryTipMessageID should survive a partial update")
	require.Equal(t, original.Provider, updated.Provider,
		"Provider should survive a partial update")
	require.Equal(t, original.Model, updated.Model,
		"Model should survive a partial update")
	require.JSONEq(t, string(original.Summary), string(updated.Summary),
		"Summary should survive a partial update")
	require.Equal(t, original.Kind, updated.Kind,
		"Kind should survive a partial update")
	require.Equal(t, original.StartedAt.UTC(), updated.StartedAt.UTC(),
		"StartedAt should survive a partial update")
}

// TestChatDebugStepCOALESCEPreservation verifies that the COALESCE
// pattern in UpdateChatDebugStep preserves every field that was not
// explicitly supplied in the update. If COALESCE were removed from
// any column, the corresponding field would silently null out.
func TestChatDebugStepCOALESCEPreservation(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-step-coalesce-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-step-coalesce-" + uuid.NewString(),
	})
	require.NoError(t, err)

	run, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID: chat.ID,
		Kind:   "chat_turn",
		Status: "in_progress",
	})
	require.NoError(t, err)

	// Insert a fully-populated step so every nullable field has a value.
	original, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:               run.ID,
		ChatID:              chat.ID,
		StepNumber:          1,
		Operation:           "llm_call",
		Status:              "in_progress",
		HistoryTipMessageID: sql.NullInt64{Int64: 10, Valid: true},
		AssistantMessageID:  sql.NullInt64{Int64: 11, Valid: true},
		NormalizedRequest:   pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"prompt":"hello"}`), Valid: true},
		NormalizedResponse:  pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"text":"world"}`), Valid: true},
		Usage:               pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"tokens":42}`), Valid: true},
		Attempts:            pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"n":1}]`), Valid: true},
		Error:               pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"code":"transient"}`), Valid: true},
		Metadata:            pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"trace_id":"abc"}`), Valid: true},
	})
	require.NoError(t, err)

	// Update only Status and FinishedAt. Every other nullable param
	// is left as its Go zero value (Valid: false -> SQL NULL), which
	// the COALESCE pattern should interpret as "keep existing."
	now := time.Now()
	updated, err := store.UpdateChatDebugStep(ctx, database.UpdateChatDebugStepParams{
		ID:     original.ID,
		ChatID: chat.ID,
		Status: sql.NullString{String: "completed", Valid: true},
		FinishedAt: sql.NullTime{
			Time:  now,
			Valid: true,
		},
		Now: now,
	})
	require.NoError(t, err)

	// Status and FinishedAt should be updated.
	require.Equal(t, "completed", updated.Status)
	require.True(t, updated.FinishedAt.Valid)

	// UpdatedAt should be set to the @now value we passed in.
	require.WithinDuration(t, now, updated.UpdatedAt, time.Millisecond,
		"updated_at should equal the @now parameter")

	// Every field not in the update call must be preserved exactly.
	require.Equal(t, original.HistoryTipMessageID, updated.HistoryTipMessageID,
		"HistoryTipMessageID should survive a partial update")
	require.Equal(t, original.AssistantMessageID, updated.AssistantMessageID,
		"AssistantMessageID should survive a partial update")
	require.JSONEq(t, string(original.NormalizedRequest), string(updated.NormalizedRequest),
		"NormalizedRequest should survive a partial update")
	require.JSONEq(t, string(original.NormalizedResponse.RawMessage), string(updated.NormalizedResponse.RawMessage),
		"NormalizedResponse should survive a partial update")
	require.JSONEq(t, string(original.Usage.RawMessage), string(updated.Usage.RawMessage),
		"Usage should survive a partial update")
	require.JSONEq(t, string(original.Attempts), string(updated.Attempts),
		"Attempts should survive a partial update")
	require.JSONEq(t, string(original.Error.RawMessage), string(updated.Error.RawMessage),
		"Error should survive a partial update")
	require.JSONEq(t, string(original.Metadata), string(updated.Metadata),
		"Metadata should survive a partial update")
	require.Equal(t, original.Operation, updated.Operation,
		"Operation should survive a partial update")
	require.Equal(t, original.StepNumber, updated.StepNumber,
		"StepNumber should survive a partial update")
	require.Equal(t, original.StartedAt.UTC(), updated.StartedAt.UTC(),
		"StartedAt should survive a partial update")
}

// TestDeleteChatDebugDataAfterMessageIDNullMessagesSurvive verifies
// that runs whose message ID columns are all NULL are never matched
// by DeleteChatDebugDataAfterMessageID.  SQL's three-valued logic
// means NULL > N evaluates to NULL (not TRUE), so these rows must
// survive.  Without this test a future change could break the
// invariant with no test failure.
func TestDeleteChatDebugDataAfterMessageIDNullMessagesSurvive(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-null-msg-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-debug-null-msg-" + uuid.NewString(),
	})
	require.NoError(t, err)

	// Insert a run with all message ID columns left as NULL (Valid: false).
	nullMsgRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Kind:          "chat_turn",
		Status:        "in_progress",
		Provider:      sql.NullString{String: providerName, Valid: true},
		Model:         sql.NullString{String: modelName, Valid: true},
		// TriggerMessageID and HistoryTipMessageID intentionally
		// omitted (zero-value → SQL NULL).
	})
	require.NoError(t, err)

	// Attach a step with NULL message IDs too.
	nullMsgStep, err := store.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      nullMsgRun.ID,
		ChatID:     chat.ID,
		StepNumber: 1,
		Operation:  "stream",
		Status:     "in_progress",
		// HistoryTipMessageID and AssistantMessageID intentionally
		// omitted (zero-value → SQL NULL).
	})
	require.NoError(t, err)

	// Delete with an arbitrary cutoff. The run and its step should
	// survive because NULL > cutoff evaluates to NULL, not TRUE.
	deletedRows, err := store.DeleteChatDebugDataAfterMessageID(ctx, database.DeleteChatDebugDataAfterMessageIDParams{
		ChatID:        chat.ID,
		MessageID:     1,
		StartedBefore: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, deletedRows, "rows with NULL message IDs must not be deleted")

	// Verify run still exists.
	remaining, err := store.GetChatDebugRunByID(ctx, nullMsgRun.ID)
	require.NoError(t, err)
	require.Equal(t, nullMsgRun.ID, remaining.ID)

	// Verify step still exists.
	remainingSteps, err := store.GetChatDebugStepsByRunID(ctx, nullMsgRun.ID)
	require.NoError(t, err)
	require.Len(t, remainingSteps, 1)
	require.Equal(t, nullMsgStep.ID, remainingSteps[0].ID)
}

// TestDeleteChatDebugDataAfterMessageIDStartedBeforeFiltersNewerRuns
// verifies the started_before bound on DeleteChatDebugDataAfterMessageID.
// The bound exists so that retried cleanup (e.g. after edit or archive)
// cannot delete runs started by a replacement turn that races ahead of
// the retry window. Without this filter, a stale cleanup would wipe
// fresh debug rows.
func TestDeleteChatDebugDataAfterMessageIDStartedBeforeFiltersNewerRuns(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-started-before-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-debug-started-before-" + uuid.NewString(),
	})
	require.NoError(t, err)

	const cutoff int64 = 50

	// oldRun started an hour ago: must be deleted because it started
	// before the bound.
	oldStartedAt := time.Now().Add(-1 * time.Hour).UTC().
		Truncate(time.Microsecond)
	oldRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: cutoff + 1, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff + 1, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
		StartedAt:           sql.NullTime{Time: oldStartedAt, Valid: true},
		UpdatedAt:           sql.NullTime{Time: oldStartedAt, Valid: true},
	})
	require.NoError(t, err)

	// Bound sits between the two runs. Any run whose started_at is at
	// or after this instant must survive.
	cutoffTime := time.Now().Add(-30 * time.Minute).UTC().
		Truncate(time.Microsecond)

	// newRun started after cutoffTime with identical message_id values
	// that would otherwise match the delete predicate. It must survive
	// because started_before excludes it.
	newStartedAt := time.Now().UTC().Truncate(time.Microsecond)
	newRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: cutoff + 1, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: cutoff + 1, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: providerName, Valid: true},
		Model:               sql.NullString{String: modelName, Valid: true},
		StartedAt:           sql.NullTime{Time: newStartedAt, Valid: true},
		UpdatedAt:           sql.NullTime{Time: newStartedAt, Valid: true},
	})
	require.NoError(t, err)

	deletedRows, err := store.DeleteChatDebugDataAfterMessageID(ctx, database.DeleteChatDebugDataAfterMessageIDParams{
		ChatID:        chat.ID,
		MessageID:     cutoff,
		StartedBefore: cutoffTime,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, deletedRows,
		"only the pre-cutoff run should be deleted")

	// oldRun must be gone.
	_, err = store.GetChatDebugRunByID(ctx, oldRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	// newRun must survive the retry window.
	remaining, err := store.GetChatDebugRunByID(ctx, newRun.ID)
	require.NoError(t, err)
	require.Equal(t, newRun.ID, remaining.ID)
}

// TestDeleteChatDebugDataByChatIDStartedBeforeFiltersNewerRuns verifies
// the started_before bound on DeleteChatDebugDataByChatID. Archive
// cleanup retries rely on this bound to avoid deleting runs created
// by a replacement turn that starts after an unarchive races ahead of
// the retry window.
func TestDeleteChatDebugDataByChatIDStartedBeforeFiltersNewerRuns(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})

	providerName := "openai"
	modelName := "debug-model-by-chat-started-before-" + uuid.NewString()

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             providerName,
		DisplayName:          "Debug Provider",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, providerName, database.InsertChatModelConfigParams{
		Model:                modelName,
		DisplayName:          "Debug Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "chat-debug-by-chat-" + uuid.NewString(),
	})
	require.NoError(t, err)

	oldStartedAt := time.Now().Add(-1 * time.Hour).UTC().
		Truncate(time.Microsecond)
	oldRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Kind:          "chat_turn",
		Status:        "in_progress",
		Provider:      sql.NullString{String: providerName, Valid: true},
		Model:         sql.NullString{String: modelName, Valid: true},
		StartedAt:     sql.NullTime{Time: oldStartedAt, Valid: true},
		UpdatedAt:     sql.NullTime{Time: oldStartedAt, Valid: true},
	})
	require.NoError(t, err)

	cutoffTime := time.Now().Add(-30 * time.Minute).UTC().
		Truncate(time.Microsecond)

	newStartedAt := time.Now().UTC().Truncate(time.Microsecond)
	newRun, err := store.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Kind:          "chat_turn",
		Status:        "in_progress",
		Provider:      sql.NullString{String: providerName, Valid: true},
		Model:         sql.NullString{String: modelName, Valid: true},
		StartedAt:     sql.NullTime{Time: newStartedAt, Valid: true},
		UpdatedAt:     sql.NullTime{Time: newStartedAt, Valid: true},
	})
	require.NoError(t, err)

	deletedRows, err := store.DeleteChatDebugDataByChatID(ctx, database.DeleteChatDebugDataByChatIDParams{
		ChatID:        chat.ID,
		StartedBefore: cutoffTime,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, deletedRows,
		"only the pre-cutoff run should be deleted")

	_, err = store.GetChatDebugRunByID(ctx, oldRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	remaining, err := store.GetChatDebugRunByID(ctx, newRun.ID)
	require.NoError(t, err)
	require.Equal(t, newRun.ID, remaining.ID)
}

func TestGetChatsFilter(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	dbgen.OrganizationMember(t, store, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})

	provider := dbgen.AIProviderWithOptionalKey(t, store, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")

	modelCfg, err := store.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		AIProviderID:         uuid.NullUUID{UUID: provider.ID, Valid: true},
		Model:                "test-model-" + uuid.NewString(),
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// --- helpers ---

	createRoot := func(title string) database.Chat {
		t.Helper()
		chat, err := store.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             title,
		})
		require.NoError(t, err)
		return chat
	}

	createChild := func(root database.Chat, title string) database.Chat {
		t.Helper()
		chat, err := store.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             title,
			ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		})
		require.NoError(t, err)
		return chat
	}

	linkPR := func(chatID uuid.UUID, url, state string, draft bool) {
		t.Helper()
		now := time.Now()
		_, err := store.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
			ChatID:           chatID,
			Url:              sql.NullString{String: url, Valid: true},
			PullRequestState: sql.NullString{String: state, Valid: true},
			PullRequestTitle: "PR " + state,
			PullRequestDraft: draft,
			Additions:        1,
			Deletions:        1,
			ChangedFiles:     1,
			RefreshedAt:      now,
			StaleAt:          now.Add(time.Hour),
		})
		require.NoError(t, err)
	}

	linkPRFull := func(chatID uuid.UUID, url, state string, draft bool, prNumber int32, gitRemoteOrigin string, prTitle string) {
		t.Helper()
		now := time.Now()
		// First set the git remote origin via the reference upsert.
		if gitRemoteOrigin != "" {
			_, err := store.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chatID,
				Url:             sql.NullString{String: url, Valid: url != ""},
				GitBranch:       "main",
				GitRemoteOrigin: gitRemoteOrigin,
				StaleAt:         now.Add(time.Hour),
			})
			require.NoError(t, err)
		}
		// Then set PR metadata via the status upsert.
		_, err := store.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
			ChatID:           chatID,
			Url:              sql.NullString{String: url, Valid: url != ""},
			PullRequestState: sql.NullString{String: state, Valid: state != ""},
			PullRequestTitle: prTitle,
			PullRequestDraft: draft,
			PrNumber:         sql.NullInt32{Int32: prNumber, Valid: prNumber > 0},
			Additions:        1,
			Deletions:        1,
			ChangedFiles:     1,
			RefreshedAt:      now,
			StaleAt:          now.Add(time.Hour),
		})
		require.NoError(t, err)
	}

	makeUnread := func(chatID uuid.UUID) {
		t.Helper()
		_, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chatID,
			CreatedBy:           []uuid.UUID{user.ID},
			ModelConfigID:       []uuid.UUID{modelCfg.ID},
			Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
			Content:             []string{`[{"type":"text","text":"hello"}]`},
			ContentVersion:      []int16{0},
			Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
			InputTokens:         []int64{0},
			OutputTokens:        []int64{0},
			TotalTokens:         []int64{0},
			ReasoningTokens:     []int64{0},
			CacheCreationTokens: []int64{0},
			CacheReadTokens:     []int64{0},
			ContextLimit:        []int64{0},
			Compressed:          []bool{false},
			TotalCostMicros:     []int64{0},
			RuntimeMs:           []int64{0},
		})
		require.NoError(t, err)
	}

	markRead := func(chatID uuid.UUID) {
		t.Helper()
		lastMsg, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
			ChatID: chatID,
			Role:   database.ChatMessageRoleAssistant,
		})
		require.NoError(t, err)
		err = store.UpdateChatLastReadMessageID(ctx, database.UpdateChatLastReadMessageIDParams{
			ID:                chatID,
			LastReadMessageID: lastMsg.ID,
		})
		require.NoError(t, err)
	}

	// --- fixtures ---

	// Title-only chats (no PR, no unread).
	alphaProject := createRoot("alpha project")
	betaProject := createRoot("beta project")
	gammaUnrelated := createRoot("gamma unrelated")
	percentComplete := createRoot("100% complete")
	thousandOne := createRoot("1001 things")
	underscoreConfig := createRoot("user_name config")
	hyphenConfig := createRoot("user-name config")

	// PR-linked chats.
	draftPR := createRoot("draft pr chat")
	linkPR(draftPR.ID, "https://github.com/coder/coder/pull/1001", "open", true)
	makeUnread(draftPR.ID) // also unread

	openPR := createRoot("open pr chat")
	linkPR(openPR.ID, "https://github.com/coder/coder/pull/1002", "open", false)

	mergedPR := createRoot("merged pr chat")
	linkPR(mergedPR.ID, "https://github.com/coder/coder/pull/1003", "merged", false)

	closedPR := createRoot("closed pr chat")
	linkPR(closedPR.ID, "https://github.com/coder/coder/pull/1004", "closed", false)

	// Unread chat without PR.
	unreadNoPR := createRoot("unread no pr")
	makeUnread(unreadNoPR.ID)

	// Read chat (message exists but marked read).
	readChat := createRoot("read chat")
	makeUnread(readChat.ID)
	markRead(readChat.ID)

	// Child with draft PR (must not surface its parent).
	childParent := createRoot("child parent")
	makeUnread(childParent.ID)
	markRead(childParent.ID)
	childWithDraftPR := createChild(childParent, "child draft pr")
	linkPR(childWithDraftPR.ID, "https://github.com/coder/coder/pull/1005", "open", true)
	makeUnread(childWithDraftPR.ID)

	// Chats with specific PR numbers and repos for new filter tests.
	// Use "acme/widget" and "acme/other-repo" origins to avoid overlapping
	// with the "coder/coder" URLs in the earlier PR fixtures.
	prNumberChat := createRoot("pr number 42 chat")
	linkPRFull(prNumberChat.ID, "https://github.com/acme/widget/pull/42", "open", false, 42, "https://github.com/acme/widget.git", "Fix authentication bug")

	repoChat := createRoot("repo filter chat")
	linkPRFull(repoChat.ID, "https://github.com/acme/other-repo/pull/7", "merged", false, 7, "https://github.com/acme/other-repo.git", "Add feature X")

	prTitleChat := createRoot("pr title filter chat")
	linkPRFull(prTitleChat.ID, "https://github.com/acme/widget/pull/99", "open", false, 99, "https://github.com/acme/widget.git", "Deploy new dashboard")

	// All root chat IDs (for "returns everything" baseline).
	allRootIDs := []uuid.UUID{
		alphaProject.ID, betaProject.ID, gammaUnrelated.ID,
		percentComplete.ID, thousandOne.ID, underscoreConfig.ID, hyphenConfig.ID,
		draftPR.ID, openPR.ID, mergedPR.ID, closedPR.ID,
		unreadNoPR.ID, readChat.ID, childParent.ID,
		prNumberChat.ID, repoChat.ID, prTitleChat.ID,
	}

	// --- test cases ---

	tests := []struct {
		name   string
		params database.GetChatsParams
		want   []uuid.UUID
	}{
		// Title filter.
		{"Title/SubstringMatch", database.GetChatsParams{TitleQuery: "project"}, []uuid.UUID{alphaProject.ID, betaProject.ID}},
		{"Title/SingleResult", database.GetChatsParams{TitleQuery: "gamma"}, []uuid.UUID{gammaUnrelated.ID}},
		{"Title/CaseInsensitive", database.GetChatsParams{TitleQuery: "ALPHA"}, []uuid.UUID{alphaProject.ID}},
		{"Title/MultiWord", database.GetChatsParams{TitleQuery: "alpha project"}, []uuid.UUID{alphaProject.ID}},
		{"Title/NoMatch", database.GetChatsParams{TitleQuery: "nonexistent"}, nil},
		{"Title/EmptyReturnsAll", database.GetChatsParams{TitleQuery: ""}, allRootIDs},
		// % acts as wildcard since we don't escape ILIKE metacharacters.
		{"Title/PercentWildcard", database.GetChatsParams{TitleQuery: "100%"}, []uuid.UUID{percentComplete.ID, thousandOne.ID}},
		// _ acts as single-char wildcard.
		{"Title/UnderscoreWildcard", database.GetChatsParams{TitleQuery: "user_name"}, []uuid.UUID{underscoreConfig.ID, hyphenConfig.ID}},

		// PR status filter.
		{"PRStatus/Draft", database.GetChatsParams{PullRequestStatuses: []string{"draft"}}, []uuid.UUID{draftPR.ID}},
		{"PRStatus/Open", database.GetChatsParams{PullRequestStatuses: []string{"open"}}, []uuid.UUID{openPR.ID, prNumberChat.ID, prTitleChat.ID}},
		{"PRStatus/Merged", database.GetChatsParams{PullRequestStatuses: []string{"merged"}}, []uuid.UUID{mergedPR.ID, repoChat.ID}},
		{"PRStatus/Closed", database.GetChatsParams{PullRequestStatuses: []string{"closed"}}, []uuid.UUID{closedPR.ID}},
		{"PRStatus/MultiStatus", database.GetChatsParams{PullRequestStatuses: []string{"draft", "closed"}}, []uuid.UUID{draftPR.ID, closedPR.ID}},

		// Unread filter.
		{"Unread/MatchesUnread", database.GetChatsParams{HasUnread: sql.NullBool{Bool: true, Valid: true}}, []uuid.UUID{draftPR.ID, unreadNoPR.ID}},
		// HasUnread=false returns chats without unread messages.
		{"Unread/ExcludesRead", database.GetChatsParams{HasUnread: sql.NullBool{Bool: false, Valid: true}}, []uuid.UUID{alphaProject.ID, betaProject.ID, gammaUnrelated.ID, percentComplete.ID, thousandOne.ID, underscoreConfig.ID, hyphenConfig.ID, openPR.ID, mergedPR.ID, closedPR.ID, readChat.ID, childParent.ID, prNumberChat.ID, repoChat.ID, prTitleChat.ID}},

		// PR number filter.
		{"PRNumber/ExactMatch", database.GetChatsParams{PrNumber: 42}, []uuid.UUID{prNumberChat.ID}},
		{"PRNumber/NoMatch", database.GetChatsParams{PrNumber: 999}, nil},
		{"PRNumber/ZeroIsNoOp", database.GetChatsParams{PrNumber: 0}, allRootIDs},

		// Repo filter.
		{"Repo/SubstringMatch", database.GetChatsParams{RepoQuery: "acme/widget"}, []uuid.UUID{prNumberChat.ID, prTitleChat.ID}},
		{"Repo/DifferentRepo", database.GetChatsParams{RepoQuery: "acme/other-repo"}, []uuid.UUID{repoChat.ID}},
		{"Repo/NoMatch", database.GetChatsParams{RepoQuery: "nonexistent/repo"}, nil},
		{"Repo/CaseInsensitive", database.GetChatsParams{RepoQuery: "ACME/WIDGET"}, []uuid.UUID{prNumberChat.ID, prTitleChat.ID}},
		{"Repo/MatchesViaURL", database.GetChatsParams{RepoQuery: "coder/coder"}, []uuid.UUID{draftPR.ID, openPR.ID, mergedPR.ID, closedPR.ID}},

		// PR title filter.
		{"PRTitle/SubstringMatch", database.GetChatsParams{PrTitleQuery: "auth"}, []uuid.UUID{prNumberChat.ID}},
		{"PRTitle/CaseInsensitive", database.GetChatsParams{PrTitleQuery: "DEPLOY"}, []uuid.UUID{prTitleChat.ID}},
		{"PRTitle/NoMatch", database.GetChatsParams{PrTitleQuery: "nonexistent title"}, nil},

		// Composed filters.
		{"Composed/TitleAndPRStatus", database.GetChatsParams{TitleQuery: "draft", PullRequestStatuses: []string{"draft"}}, []uuid.UUID{draftPR.ID}},
		{"Composed/TitleAndUnread", database.GetChatsParams{TitleQuery: "draft pr", HasUnread: sql.NullBool{Bool: true, Valid: true}}, []uuid.UUID{draftPR.ID}},
		{"Composed/PRStatusAndUnread", database.GetChatsParams{PullRequestStatuses: []string{"draft"}, HasUnread: sql.NullBool{Bool: true, Valid: true}}, []uuid.UUID{draftPR.ID}},
		{"Composed/AllFilters", database.GetChatsParams{TitleQuery: "draft", PullRequestStatuses: []string{"draft"}, HasUnread: sql.NullBool{Bool: true, Valid: true}}, []uuid.UUID{draftPR.ID}},
		{"Composed/TitleNarrowsUnread", database.GetChatsParams{TitleQuery: "no pr", HasUnread: sql.NullBool{Bool: true, Valid: true}}, []uuid.UUID{unreadNoPR.ID}},
		{"Composed/PRNumberAndStatus", database.GetChatsParams{PrNumber: 42, PullRequestStatuses: []string{"closed"}}, nil},
		{"Composed/RepoAndPRTitle", database.GetChatsParams{RepoQuery: "acme/widget", PrTitleQuery: "auth"}, []uuid.UUID{prNumberChat.ID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Always scope to this user.
			params := tt.params
			params.OwnedOnly = true
			params.ViewerID = user.ID

			rows, err := store.GetChats(ctx, params)
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.Chat.ID)
			}

			if tt.want == nil {
				require.Empty(t, got)
			} else {
				require.ElementsMatch(t, tt.want, got)
			}
		})
	}
}

func TestChatHasUnread(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := context.Background()

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	dbgen.OrganizationMember(t, store, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})

	dbgen.ChatProvider(t, store, database.ChatProvider{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})

	modelCfg, err := insertChatModelConfigForTest(ctx, t, store, "openai", database.InsertChatModelConfigParams{
		Model:                "test-model-" + uuid.NewString(),
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 80,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	chat, err := store.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "test-chat-" + uuid.NewString(),
	})
	require.NoError(t, err)

	getHasUnread := func() bool {
		rows, err := store.GetChats(ctx, database.GetChatsParams{
			OwnedOnly: true,
			ViewerID:  user.ID,
		})
		require.NoError(t, err)
		for _, row := range rows {
			if row.Chat.ID == chat.ID {
				return row.HasUnread
			}
		}
		t.Fatal("chat not found in GetChats result")
		return false
	}

	// New chat with no messages: not unread.
	require.False(t, getHasUnread(), "new chat with no messages should not be unread")

	// Helper to insert a single chat message.
	insertMsg := func(role database.ChatMessageRole, text string) {
		t.Helper()
		_, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chat.ID,
			CreatedBy:           []uuid.UUID{user.ID},
			ModelConfigID:       []uuid.UUID{modelCfg.ID},
			Role:                []database.ChatMessageRole{role},
			Content:             []string{fmt.Sprintf(`[{"type":"text","text":%q}]`, text)},
			ContentVersion:      []int16{0},
			Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
			InputTokens:         []int64{0},
			OutputTokens:        []int64{0},
			TotalTokens:         []int64{0},
			ReasoningTokens:     []int64{0},
			CacheCreationTokens: []int64{0},
			CacheReadTokens:     []int64{0},
			ContextLimit:        []int64{0},
			Compressed:          []bool{false},
			TotalCostMicros:     []int64{0},
			RuntimeMs:           []int64{0},
		})
		require.NoError(t, err)
	}

	// Insert an assistant message: becomes unread.
	insertMsg(database.ChatMessageRoleAssistant, "hello")
	require.True(t, getHasUnread(), "chat with unread assistant message should be unread")

	// Mark as read: no longer unread.
	lastMsg, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	require.NoError(t, err)
	err = store.UpdateChatLastReadMessageID(ctx, database.UpdateChatLastReadMessageIDParams{
		ID:                chat.ID,
		LastReadMessageID: lastMsg.ID,
	})
	require.NoError(t, err)
	require.False(t, getHasUnread(), "chat should not be unread after marking as read")

	// Insert another assistant message: becomes unread again.
	insertMsg(database.ChatMessageRoleAssistant, "new message")
	require.True(t, getHasUnread(), "new assistant message after read should be unread")

	// Mark as read again, then verify user messages don't
	// trigger unread.
	lastMsg, err = store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	require.NoError(t, err)
	err = store.UpdateChatLastReadMessageID(ctx, database.UpdateChatLastReadMessageIDParams{
		ID:                chat.ID,
		LastReadMessageID: lastMsg.ID,
	})
	require.NoError(t, err)
	insertMsg(database.ChatMessageRoleUser, "user msg")
	require.False(t, getHasUnread(), "user messages should not trigger unread")
}

// TestSoftDeletePriorWorkspaceAgents verifies the invariant maintained by
// wsbuilder.Builder.Build: when a new build of a workspace is created, all
// agents belonging to prior builds of that same workspace are soft-deleted,
// and agents belonging to *other* workspaces are untouched.
func TestSoftDeletePriorWorkspaceAgents(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	// Helper: create a workspace + one build + its agent. Returns the IDs we
	// need to assert on. The agent uses the shared EC2-style auth_instance_id
	// so we can prove per-workspace scoping.
	type buildBundle struct {
		workspaceID uuid.UUID
		buildID     uuid.UUID
		agentID     uuid.UUID
	}

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	newBuild := func(t *testing.T, wsID uuid.UUID, buildNumber int32, instanceID string) buildBundle {
		t.Helper()
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wsID,
			JobID:             job.ID,
			TemplateVersionID: tplVersion.ID,
			BuildNumber:       buildNumber,
			Transition:        database.WorkspaceTransitionStart,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID:     resource.ID,
			AuthInstanceID: sql.NullString{String: instanceID, Valid: true},
		})
		return buildBundle{workspaceID: wsID, buildID: build.ID, agentID: agent.ID}
	}

	// Read `deleted` via raw SQL. GetWorkspaceAgentByID filters deleted rows
	// out, which is exactly what we want to observe here.
	agentDeleted := func(id uuid.UUID) bool {
		t.Helper()
		var deleted bool
		err := sqlDB.QueryRowContext(ctx,
			`SELECT deleted FROM workspace_agents WHERE id = $1`, id).Scan(&deleted)
		require.NoError(t, err)
		return deleted
	}

	// Two workspaces share a single EC2 instance ID across their lifetimes.
	wsA := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID
	wsB := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID
	instance := "i-shared"

	a1 := newBuild(t, wsA, 1, instance)
	a2 := newBuild(t, wsA, 2, instance)
	a3 := newBuild(t, wsA, 3, instance)
	b1 := newBuild(t, wsB, 1, instance)
	b2 := newBuild(t, wsB, 2, instance)

	// Sanity check: all agents start non-deleted.
	require.False(t, agentDeleted(a1.agentID))
	require.False(t, agentDeleted(a2.agentID))
	require.False(t, agentDeleted(a3.agentID))
	require.False(t, agentDeleted(b1.agentID))
	require.False(t, agentDeleted(b2.agentID))

	// Run: "wsA's current build is a3; soft-delete all other wsA agents."
	err := db.SoftDeletePriorWorkspaceAgents(ctx, database.SoftDeletePriorWorkspaceAgentsParams{
		WorkspaceID:    wsA,
		CurrentBuildID: a3.buildID,
	})
	require.NoError(t, err)

	assert.True(t, agentDeleted(a1.agentID), "wsA build 1 agent should be soft-deleted")
	assert.True(t, agentDeleted(a2.agentID), "wsA build 2 agent should be soft-deleted")
	assert.False(t, agentDeleted(a3.agentID), "wsA current build's agent must stay")
	assert.False(t, agentDeleted(b1.agentID), "wsB build 1 agent must not be touched")
	assert.False(t, agentDeleted(b2.agentID), "wsB build 2 agent must not be touched")

	// Idempotency: re-running with the same params is a no-op.
	err = db.SoftDeletePriorWorkspaceAgents(ctx, database.SoftDeletePriorWorkspaceAgentsParams{
		WorkspaceID:    wsA,
		CurrentBuildID: a3.buildID,
	})
	require.NoError(t, err)
	assert.False(t, agentDeleted(a3.agentID))

	// Now age wsB: new current build is b2; b1's agent should flip.
	err = db.SoftDeletePriorWorkspaceAgents(ctx, database.SoftDeletePriorWorkspaceAgentsParams{
		WorkspaceID:    wsB,
		CurrentBuildID: b2.buildID,
	})
	require.NoError(t, err)
	assert.True(t, agentDeleted(b1.agentID))
	assert.False(t, agentDeleted(b2.agentID))
}

// TestSoftDeleteWorkspaceAgentsByWorkspaceID verifies the delete-path
// invariant: when a workspace is soft-deleted, every one of its agents
// (across all builds) gets soft-deleted in the same transaction. Agents on
// *other* workspaces, even ones sharing an auth_instance_id, must be
// untouched.
func TestSoftDeleteWorkspaceAgentsByWorkspaceID(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	type buildBundle struct {
		workspaceID uuid.UUID
		buildID     uuid.UUID
		agentID     uuid.UUID
	}

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	newBuild := func(t *testing.T, wsID uuid.UUID, buildNumber int32, instanceID string) buildBundle {
		t.Helper()
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wsID,
			JobID:             job.ID,
			TemplateVersionID: tplVersion.ID,
			BuildNumber:       buildNumber,
			Transition:        database.WorkspaceTransitionStart,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID:     resource.ID,
			AuthInstanceID: sql.NullString{String: instanceID, Valid: true},
		})
		return buildBundle{workspaceID: wsID, buildID: build.ID, agentID: agent.ID}
	}

	agentDeleted := func(id uuid.UUID) bool {
		t.Helper()
		var deleted bool
		err := sqlDB.QueryRowContext(ctx,
			`SELECT deleted FROM workspace_agents WHERE id = $1`, id).Scan(&deleted)
		require.NoError(t, err)
		return deleted
	}

	// wsA: 3 builds (so multiple agents to sweep on delete).
	// wsB: 1 build, same auth_instance_id as wsA (proves scoping).
	wsA := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID
	wsB := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID
	instance := "i-shared"

	a1 := newBuild(t, wsA, 1, instance)
	a2 := newBuild(t, wsA, 2, instance)
	a3 := newBuild(t, wsA, 3, instance)
	b1 := newBuild(t, wsB, 1, instance)

	// Sanity: all 4 agents start non-deleted.
	for _, id := range []uuid.UUID{a1.agentID, a2.agentID, a3.agentID, b1.agentID} {
		require.False(t, agentDeleted(id))
	}

	err := db.SoftDeleteWorkspaceAgentsByWorkspaceID(ctx, wsA)
	require.NoError(t, err)

	// All wsA agents flipped; wsB's agent untouched.
	assert.True(t, agentDeleted(a1.agentID), "wsA build 1 agent")
	assert.True(t, agentDeleted(a2.agentID), "wsA build 2 agent")
	assert.True(t, agentDeleted(a3.agentID), "wsA build 3 agent")
	assert.False(t, agentDeleted(b1.agentID), "wsB agent must not be affected")

	// Idempotency: re-running is a no-op.
	err = db.SoftDeleteWorkspaceAgentsByWorkspaceID(ctx, wsA)
	require.NoError(t, err)
	assert.False(t, agentDeleted(b1.agentID))

	// Calling on an empty workspace (no agents) is a no-op and does not error.
	wsEmpty := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID
	err = db.SoftDeleteWorkspaceAgentsByWorkspaceID(ctx, wsEmpty)
	require.NoError(t, err)
}

// TestSoftDeleteWorkspaceAgentsPurgesContext verifies that both agent
// soft-delete queries hard-delete the agents' pushed context rows
// (workspace_agent_context_snapshots and
// workspace_agent_context_resources). Agents are only ever
// soft-deleted, so without this the context rows would accumulate
// forever.
func TestSoftDeleteWorkspaceAgentsPurgesContext(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	type buildBundle struct {
		buildID uuid.UUID
		agentID uuid.UUID
		agent   database.WorkspaceAgent
	}

	newBuild := func(t *testing.T, wsID uuid.UUID, buildNumber int32) buildBundle {
		t.Helper()
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       wsID,
			JobID:             job.ID,
			TemplateVersionID: tplVersion.ID,
			BuildNumber:       buildNumber,
			Transition:        database.WorkspaceTransitionStart,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
		return buildBundle{buildID: build.ID, agentID: agent.ID, agent: agent}
	}

	pushContext := func(t *testing.T, agentID uuid.UUID) {
		t.Helper()
		_, err := db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
			WorkspaceAgentID: agentID,
			Version:          1,
			AggregateHash:    []byte{0x01},
			ReceivedAt:       dbtime.Now(),
		})
		require.NoError(t, err)
		_, err = db.UpsertWorkspaceAgentContextResource(ctx, database.UpsertWorkspaceAgentContextResourceParams{
			WorkspaceAgentID: agentID,
			Source:           "/workspace/AGENTS.md",
			BodyKind:         database.WorkspaceAgentContextBodyKindInstructionFile,
			Body:             []byte(`{}`),
			ContentHash:      []byte{0x02},
			SizeBytes:        2,
			Status:           database.WorkspaceAgentContextResourceStatusOk,
			Now:              dbtime.Now(),
		})
		require.NoError(t, err)
	}

	hasContext := func(t *testing.T, agentID uuid.UUID) bool {
		t.Helper()
		_, err := db.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
		if errors.Is(err, sql.ErrNoRows) {
			resources, err := db.ListWorkspaceAgentContextResources(ctx, agentID)
			require.NoError(t, err)
			require.Empty(t, resources, "snapshot and resource rows must be deleted together")
			return false
		}
		require.NoError(t, err)
		return true
	}

	wsA := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID
	wsB := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
	}).ID

	a1 := newBuild(t, wsA, 1)
	a2 := newBuild(t, wsA, 2)
	b1 := newBuild(t, wsB, 1)

	pushContext(t, a1.agentID)
	pushContext(t, a2.agentID)
	pushContext(t, b1.agentID)

	// Soft-deleting wsA's prior agents purges a1's context but leaves
	// the current build's agent and other workspaces untouched.
	err := db.SoftDeletePriorWorkspaceAgents(ctx, database.SoftDeletePriorWorkspaceAgentsParams{
		WorkspaceID:    wsA,
		CurrentBuildID: a2.buildID,
	})
	require.NoError(t, err)
	assert.False(t, hasContext(t, a1.agentID), "prior build agent context must be purged")
	assert.True(t, hasContext(t, a2.agentID), "current build agent context must remain")
	assert.True(t, hasContext(t, b1.agentID), "other workspace agent context must remain")

	// Soft-deleting all of wsB's agents purges b1's context.
	err = db.SoftDeleteWorkspaceAgentsByWorkspaceID(ctx, wsB)
	require.NoError(t, err)
	assert.True(t, hasContext(t, a2.agentID), "other workspace agent context must remain")
	assert.False(t, hasContext(t, b1.agentID), "deleted workspace agent context must be purged")

	// Removing a sub-agent mid-build via DeleteWorkspaceSubAgentByID purges
	// only that sub-agent's context. The rebuild-time queries skip
	// already-deleted agents, so this is the sole cleanup opportunity.
	c1 := newBuild(t, wsA, 3)
	subAgent := dbgen.WorkspaceSubAgent(t, db, c1.agent, database.WorkspaceAgent{})
	pushContext(t, c1.agentID)
	pushContext(t, subAgent.ID)

	err = db.DeleteWorkspaceSubAgentByID(ctx, subAgent.ID)
	require.NoError(t, err)
	assert.True(t, hasContext(t, c1.agentID), "parent agent context must remain")
	assert.False(t, hasContext(t, subAgent.ID), "deleted sub-agent context must be purged")
}

func TestAIGatewayKeysTableConstraints(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	preExisting := database.InsertAIGatewayKeyParams{
		ID:           uuid.New(),
		Name:         "name",
		SecretPrefix: "key_test__1",
		HashedSecret: []byte("first-secret"),
	}
	_, err := db.InsertAIGatewayKey(ctx, preExisting)
	require.NoError(t, err)

	tests := []struct {
		name            string
		params          database.InsertAIGatewayKeyParams
		expectUniqueErr database.UniqueConstraint
		expectCheckErr  database.CheckConstraint
	}{
		{
			name:            "duplicate name",
			params:          aiGatewayKeyParams(preExisting.Name, "key_test002"),
			expectUniqueErr: database.UniqueAIGatewayKeysNameIndex,
		},
		{
			name:            "duplicate secret prefix",
			params:          aiGatewayKeyParams("different-key", preExisting.SecretPrefix),
			expectUniqueErr: database.UniqueAIGatewayKeysSecretPrefixIndex,
		},
		{
			name:            "duplicate hashed secret",
			params:          database.InsertAIGatewayKeyParams{ID: uuid.New(), Name: "other-name", SecretPrefix: "key_1234567", HashedSecret: preExisting.HashedSecret},
			expectUniqueErr: database.UniqueAIGatewayKeysHashedSecretIndex,
		},
		{
			name:           "empty name",
			params:         aiGatewayKeyParams("", "key_empty__"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "name with trailing dash",
			params:         aiGatewayKeyParams("other-name-", "key_trail__"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "name with consecutive dashes",
			params:         aiGatewayKeyParams("other--name", "key_consec_"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "name with underscore",
			params:         aiGatewayKeyParams("other_name", "key_undersc"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "name with space",
			params:         aiGatewayKeyParams("other name", "key_spacen_"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "name with leading dash",
			params:         aiGatewayKeyParams("-other-name", "key_leadng_"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "name longer than 64 characters",
			params:         aiGatewayKeyParams(strings.Repeat("a", 65), "key_longna_"),
			expectCheckErr: database.CheckAIGatewayKeysNameCheck,
		},
		{
			name:           "empty secret prefix",
			params:         aiGatewayKeyParams("check-empty-pfx", ""),
			expectCheckErr: database.CheckAIGatewayKeysSecretPrefixCheck,
		},
		{
			name:           "invalid secret prefix length",
			params:         aiGatewayKeyParams("check-short-pfx", "key_short"),
			expectCheckErr: database.CheckAIGatewayKeysSecretPrefixCheck,
		},
		{
			name:           "empty hashed secret",
			params:         database.InsertAIGatewayKeyParams{ID: uuid.New(), Name: "check-empty-hash", SecretPrefix: "key_ehash__", HashedSecret: []byte{}},
			expectCheckErr: database.CheckAIGatewayKeysHashedSecretCheck,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			_, err := db.InsertAIGatewayKey(ctx, tc.params)
			require.Error(t, err)
			requireAIGatewayKeysViolation(t, err, tc.expectUniqueErr, tc.expectCheckErr)
		})
	}
}

func TestAIGatewayKeysQueries(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	first := aiGatewayKeyParams("first-key", "key_first__")
	second := aiGatewayKeyParams("second-key", "key_second_")
	second.HashedSecret = []byte("second-secret")

	firstRow, err := db.InsertAIGatewayKey(ctx, first)
	require.NoError(t, err)
	require.Equal(t, first.ID, firstRow.ID)

	require.Equal(t, "first-key", firstRow.Name)
	require.Equal(t, first.SecretPrefix, firstRow.SecretPrefix)

	secondRow, err := db.InsertAIGatewayKey(ctx, second)
	require.NoError(t, err)
	require.Equal(t, second.ID, secondRow.ID)

	require.Equal(t, "second-key", secondRow.Name)
	require.Equal(t, second.SecretPrefix, secondRow.SecretPrefix)

	keys, err := db.ListAIGatewayKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 2)

	requireAIGatewayKeysRow(t, keys[0], first, firstRow.CreatedAt)
	require.False(t, keys[0].LastHeartbeatAt.Valid)
	requireAIGatewayKeysRow(t, keys[1], second, secondRow.CreatedAt)
	require.False(t, keys[1].LastHeartbeatAt.Valid)

	deleted, err := db.DeleteAIGatewayKey(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, deleted.ID)
	require.Equal(t, first.Name, deleted.Name)
	require.Equal(t, first.SecretPrefix, deleted.SecretPrefix)
	require.Equal(t, firstRow.CreatedAt, deleted.CreatedAt)

	_, err = db.DeleteAIGatewayKey(ctx, first.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	keys, err = db.ListAIGatewayKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	requireAIGatewayKeysRow(t, keys[0], second, secondRow.CreatedAt)
}

func TestGetAIGatewayKeyByHashedSecret(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	first := aiGatewayKeyParams("lookup-first", "key_lookup1")
	second := aiGatewayKeyParams("lookup-second", "key_lookup2")

	_, err := db.InsertAIGatewayKey(ctx, first)
	require.NoError(t, err)
	_, err = db.InsertAIGatewayKey(ctx, second)
	require.NoError(t, err)

	key, err := db.GetAIGatewayKeyByHashedSecret(ctx, first.HashedSecret)
	require.NoError(t, err)
	require.Equal(t, first.ID, key.ID)
	require.Equal(t, first.Name, key.Name)
	require.Equal(t, first.SecretPrefix, key.SecretPrefix)
	require.Equal(t, first.HashedSecret, key.HashedSecret)

	key, err = db.GetAIGatewayKeyByHashedSecret(ctx, second.HashedSecret)
	require.NoError(t, err)
	require.Equal(t, second.ID, key.ID)

	// An unknown secret returns no rows
	key, err = db.GetAIGatewayKeyByHashedSecret(ctx, []byte("does-not-exist"))
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Empty(t, key.ID)
}

func TestUpdateAIGatewayKeyLastHeartbeatAt(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	params := aiGatewayKeyParams("liveness-key", "key_live___")
	row, err := db.InsertAIGatewayKey(ctx, params)
	require.NoError(t, err)

	// last_heartbeat_at starts NULL until a session records liveness.
	keys, err := db.ListAIGatewayKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.False(t, keys[0].LastHeartbeatAt.Valid)

	rows, err := db.UpdateAIGatewayKeyLastHeartbeatAt(ctx, params.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	keys, err = db.ListAIGatewayKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.True(t, keys[0].LastHeartbeatAt.Valid)
	// The database stamps the timestamp, so compare against the row's
	// DB-generated CreatedAt to avoid client clock skew.
	require.False(t, keys[0].LastHeartbeatAt.Time.Before(row.CreatedAt))

	// Updating a key that does not exist is a no-op, not an error.
	rows, err = db.UpdateAIGatewayKeyLastHeartbeatAt(ctx, uuid.New())
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)

	// Set last_heartbeat_at to old time to confirm the update overwrites it with a fresh timestamp.
	staleTime := row.CreatedAt.Add(-time.Hour)
	_, err = sqlDB.ExecContext(ctx, "UPDATE ai_gateway_keys SET last_heartbeat_at = $1 WHERE id = $2", staleTime, params.ID)
	require.NoError(t, err)

	rows, err = db.UpdateAIGatewayKeyLastHeartbeatAt(ctx, params.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	keys, err = db.ListAIGatewayKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.True(t, keys[0].LastHeartbeatAt.Time.After(staleTime))
}

func aiGatewayKeyParams(name string, secretPrefix string) database.InsertAIGatewayKeyParams {
	return database.InsertAIGatewayKeyParams{
		ID:           uuid.New(),
		Name:         name,
		SecretPrefix: secretPrefix,
		HashedSecret: []byte("secret-" + name + "-" + secretPrefix),
	}
}

func requireAIGatewayKeysRow(t *testing.T, listRow database.ListAIGatewayKeysRow, insertParams database.InsertAIGatewayKeyParams, insertCreatedAt time.Time) {
	t.Helper()

	require.Equal(t, insertParams.ID, listRow.ID)
	require.Equal(t, insertParams.Name, listRow.Name)
	require.Equal(t, insertParams.SecretPrefix, listRow.SecretPrefix)
	require.Equal(t, insertCreatedAt, listRow.CreatedAt)
}

func requireAIGatewayKeysViolation(
	t *testing.T,
	err error,
	uniqueConstraint database.UniqueConstraint,
	checkConstraint database.CheckConstraint,
) {
	t.Helper()

	switch {
	case uniqueConstraint != "":
		require.True(t, database.IsUniqueViolation(err, uniqueConstraint), "expected %q unique violation, got %v", uniqueConstraint, err)
	case checkConstraint != "":
		require.True(t, database.IsCheckViolation(err, checkConstraint), "expected %q check violation, got %v", checkConstraint, err)
	default:
		require.FailNow(t, "test case must expect a constraint error")
	}
}
