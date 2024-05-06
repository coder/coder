package dbpurge_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/exp/slices"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbrollup"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Ensures no goroutines leak.
func TestPurge(t *testing.T) {
	t.Parallel()
	purger := dbpurge.New(context.Background(), slogtest.Make(t, nil), dbmem.New())
	err := purger.Close()
	require.NoError(t, err)
}

func TestDeleteOldWorkspaceAgentStats(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	now := dbtime.Now()

	defer func() {
		if t.Failed() {
			t.Logf("Test failed, printing rows...")
			ctx := testutil.Context(t, testutil.WaitShort)
			buf := &bytes.Buffer{}
			enc := json.NewEncoder(buf)
			enc.SetIndent("", "\t")
			wasRows, err := db.GetWorkspaceAgentStats(ctx, now.AddDate(0, -7, 0))
			if err == nil {
				_, _ = fmt.Fprintf(buf, "workspace agent stats: ")
				_ = enc.Encode(wasRows)
			}
			tusRows, err := db.GetTemplateUsageStats(context.Background(), database.GetTemplateUsageStatsParams{
				StartTime: now.AddDate(0, -7, 0),
				EndTime:   now,
			})
			if err == nil {
				_, _ = fmt.Fprintf(buf, "template usage stats: ")
				_ = enc.Encode(tusRows)
			}
			s := bufio.NewScanner(buf)
			for s.Scan() {
				t.Log(s.Text())
			}
			_ = s.Err()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// given
	// Note: We use increments of 2 hours to ensure we avoid any DST
	// conflicts, verifying DST behavior is beyond the scope of this
	// test.
	// Let's use RxBytes to identify stat entries.
	// Stat inserted 6 months + 2 hour ago, should be deleted.
	first := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt:                 now.AddDate(0, -6, 0).Add(-2 * time.Hour),
		ConnectionCount:           1,
		ConnectionMedianLatencyMS: 1,
		RxBytes:                   1111,
		SessionCountSSH:           1,
	})

	// Stat inserted 6 months - 2 hour ago, should not be deleted before rollup.
	second := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt:                 now.AddDate(0, -6, 0).Add(2 * time.Hour),
		ConnectionCount:           1,
		ConnectionMedianLatencyMS: 1,
		RxBytes:                   2222,
		SessionCountSSH:           1,
	})

	// Stat inserted 6 months - 1 day - 4 hour ago, should not be deleted at all.
	third := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt:                 now.AddDate(0, -6, 0).AddDate(0, 0, 1).Add(4 * time.Hour),
		ConnectionCount:           1,
		ConnectionMedianLatencyMS: 1,
		RxBytes:                   3333,
		SessionCountSSH:           1,
	})

	// when
	closer := dbpurge.New(ctx, logger, db)
	defer closer.Close()

	// then
	var stats []database.GetWorkspaceAgentStatsRow
	var err error
	require.Eventuallyf(t, func() bool {
		// Query all stats created not earlier than 7 months ago
		stats, err = db.GetWorkspaceAgentStats(ctx, now.AddDate(0, -7, 0))
		if err != nil {
			return false
		}
		return !containsWorkspaceAgentStat(stats, first) &&
			containsWorkspaceAgentStat(stats, second)
	}, testutil.WaitShort, testutil.IntervalFast, "it should delete old stats: %v", stats)

	// when
	events := make(chan dbrollup.Event)
	rolluper := dbrollup.New(logger, db, dbrollup.WithEventChannel(events))
	defer rolluper.Close()

	_, _ = <-events, <-events

	// Start a new purger to immediately trigger delete after rollup.
	_ = closer.Close()
	closer = dbpurge.New(ctx, logger, db)
	defer closer.Close()

	// then
	require.Eventuallyf(t, func() bool {
		// Query all stats created not earlier than 7 months ago
		stats, err = db.GetWorkspaceAgentStats(ctx, now.AddDate(0, -7, 0))
		if err != nil {
			return false
		}
		return !containsWorkspaceAgentStat(stats, first) &&
			!containsWorkspaceAgentStat(stats, second) &&
			containsWorkspaceAgentStat(stats, third)
	}, testutil.WaitShort, testutil.IntervalFast, "it should delete old stats after rollup: %v", stats)
}

func containsWorkspaceAgentStat(stats []database.GetWorkspaceAgentStatsRow, needle database.WorkspaceAgentStat) bool {
	return slices.ContainsFunc(stats, func(s database.GetWorkspaceAgentStatsRow) bool {
		return s.WorkspaceRxBytes == needle.RxBytes
	})
}

func TestDeleteOldWorkspaceAgentLogs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID})
	tmpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, ActiveVersionID: tv.ID, CreatedBy: user.ID})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	now := dbtime.Now()

	t.Run("AgentHasNotConnectedSinceWeek_LogsExpired", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		agent := mustCreateAgentWithLogs(ctx, t, db, user, org, tmpl, tv, now.Add(-8*24*time.Hour), t.Name())

		// when
		closer := dbpurge.New(ctx, logger, db)
		defer closer.Close()

		// then
		require.Eventually(t, func() bool {
			agentLogs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
				AgentID: agent,
			})
			if err != nil {
				return false
			}
			return !containsAgentLog(agentLogs, t.Name())
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("AgentConnectedSixDaysAgo_LogsValid", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// given
		agent := mustCreateAgentWithLogs(ctx, t, db, user, org, tmpl, tv, now.Add(-6*24*time.Hour), t.Name())

		// when
		closer := dbpurge.New(ctx, logger, db)
		defer closer.Close()

		// then
		require.Eventually(t, func() bool {
			agentLogs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
				AgentID: agent,
			})
			if err != nil {
				return false
			}
			return containsAgentLog(agentLogs, t.Name())
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func mustCreateAgentWithLogs(ctx context.Context, t *testing.T, db database.Store, user database.User, org database.Organization, tmpl database.Template, tv database.TemplateVersion, agentLastConnectedAt time.Time, output string) uuid.UUID {
	agent := mustCreateAgent(t, db, user, org, tmpl, tv)

	err := db.UpdateWorkspaceAgentConnectionByID(ctx, database.UpdateWorkspaceAgentConnectionByIDParams{
		ID:              agent.ID,
		LastConnectedAt: sql.NullTime{Time: agentLastConnectedAt, Valid: true},
	})
	require.NoError(t, err)
	_, err = db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:   agent.ID,
		CreatedAt: agentLastConnectedAt,
		Output:    []string{output},
		Level:     []database.LogLevel{database.LogLevelDebug},
	})
	require.NoError(t, err)
	return agent.ID
}

func mustCreateAgent(t *testing.T, db database.Store, user database.User, org database.Organization, tmpl database.Template, tv database.TemplateVersion) database.WorkspaceAgent {
	workspace := dbgen.Workspace(t, db, database.Workspace{OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
		TemplateVersionID: tv.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID:      job.ID,
		Transition: database.WorkspaceTransitionStart,
	})
	return dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})
}

func containsAgentLog(daemons []database.WorkspaceAgentLog, output string) bool {
	return slices.ContainsFunc(daemons, func(d database.WorkspaceAgentLog) bool {
		return d.Output == output
	})
}

func TestDeleteOldProvisionerDaemons(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	defaultOrg := dbgen.Organization(t, db, database.Organization{})
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	now := dbtime.Now()

	// given
	_, err := db.UpsertProvisionerDaemon(ctx, database.UpsertProvisionerDaemonParams{
		// Provisioner daemon created 14 days ago, and checked in just before 7 days deadline.
		Name:         "external-0",
		Provisioners: []database.ProvisionerType{"echo"},
		Tags:         database.StringMap{provisionersdk.TagScope: provisionersdk.ScopeOrganization},
		CreatedAt:    now.AddDate(0, 0, -14),
		// Note: adding an hour and a minute to account for DST variations
		LastSeenAt:     sql.NullTime{Valid: true, Time: now.AddDate(0, 0, -7).Add(61 * time.Minute)},
		Version:        "1.0.0",
		APIVersion:     proto.CurrentVersion.String(),
		OrganizationID: defaultOrg.ID,
	})
	require.NoError(t, err)
	_, err = db.UpsertProvisionerDaemon(ctx, database.UpsertProvisionerDaemonParams{
		// Provisioner daemon created 8 days ago, and checked in last time an hour after creation.
		Name:           "external-1",
		Provisioners:   []database.ProvisionerType{"echo"},
		Tags:           database.StringMap{provisionersdk.TagScope: provisionersdk.ScopeOrganization},
		CreatedAt:      now.AddDate(0, 0, -8),
		LastSeenAt:     sql.NullTime{Valid: true, Time: now.AddDate(0, 0, -8).Add(time.Hour)},
		Version:        "1.0.0",
		APIVersion:     proto.CurrentVersion.String(),
		OrganizationID: defaultOrg.ID,
	})
	require.NoError(t, err)
	_, err = db.UpsertProvisionerDaemon(ctx, database.UpsertProvisionerDaemonParams{
		// Provisioner daemon created 9 days ago, and never checked in.
		Name:         "alice-provisioner",
		Provisioners: []database.ProvisionerType{"echo"},
		Tags: database.StringMap{
			provisionersdk.TagScope: provisionersdk.ScopeUser,
			provisionersdk.TagOwner: uuid.NewString(),
		},
		CreatedAt:      now.AddDate(0, 0, -9),
		Version:        "1.0.0",
		APIVersion:     proto.CurrentVersion.String(),
		OrganizationID: defaultOrg.ID,
	})
	require.NoError(t, err)
	_, err = db.UpsertProvisionerDaemon(ctx, database.UpsertProvisionerDaemonParams{
		// Provisioner daemon created 6 days ago, and never checked in.
		Name:         "bob-provisioner",
		Provisioners: []database.ProvisionerType{"echo"},
		Tags: database.StringMap{
			provisionersdk.TagScope: provisionersdk.ScopeUser,
			provisionersdk.TagOwner: uuid.NewString(),
		},
		CreatedAt:      now.AddDate(0, 0, -6),
		LastSeenAt:     sql.NullTime{Valid: true, Time: now.AddDate(0, 0, -6)},
		Version:        "1.0.0",
		APIVersion:     proto.CurrentVersion.String(),
		OrganizationID: defaultOrg.ID,
	})
	require.NoError(t, err)

	// when
	closer := dbpurge.New(ctx, logger, db)
	defer closer.Close()

	// then
	require.Eventually(t, func() bool {
		daemons, err := db.GetProvisionerDaemons(ctx)
		if err != nil {
			return false
		}

		daemonNames := make([]string, 0, len(daemons))
		for _, d := range daemons {
			daemonNames = append(daemonNames, d.Name)
		}
		t.Logf("found %d daemons: %v", len(daemons), daemonNames)

		return containsProvisionerDaemon(daemons, "external-0") &&
			!containsProvisionerDaemon(daemons, "external-1") &&
			!containsProvisionerDaemon(daemons, "alice-provisioner") &&
			containsProvisionerDaemon(daemons, "bob-provisioner")
	}, testutil.WaitShort, testutil.IntervalSlow)
}

func containsProvisionerDaemon(daemons []database.ProvisionerDaemon, name string) bool {
	return slices.ContainsFunc(daemons, func(d database.ProvisionerDaemon) bool {
		return d.Name == name
	})
}
