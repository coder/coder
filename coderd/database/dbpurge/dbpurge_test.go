package dbpurge_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbrollup"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// Ensures no goroutines leak.
//
//nolint:paralleltest // It uses LockIDDBPurge.
func TestPurge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// We want to make sure dbpurge is actually started so that this test is meaningful.
	clk := quartz.NewMock(t)
	done := awaitDoTick(ctx, t, clk)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("db_purge")).Return(nil).Times(2)
	purger := dbpurge.New(context.Background(), testutil.Logger(t), mDB, &codersdk.DeploymentValues{}, clk)
	<-done // wait for doTick() to run.
	require.NoError(t, purger.Close())
}

//nolint:paralleltest // It uses LockIDDBPurge.
func TestDeleteOldWorkspaceAgentStats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	now := dbtime.Now()
	// TODO: must refactor DeleteOldWorkspaceAgentStats to allow passing in cutoff
	//       before using quarts.NewMock()
	clk := quartz.NewReal()
	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	defer func() {
		if t.Failed() {
			t.Log("Test failed, printing rows...")
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

	// given
	// Note: We use increments of 2 hours to ensure we avoid any DST
	// conflicts, verifying DST behavior is beyond the scope of this
	// test.
	// Let's use RxBytes to identify stat entries.
	// Stat inserted 180 days + 2 hour ago, should be deleted.
	first := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt:                 now.AddDate(0, 0, -180).Add(-2 * time.Hour),
		ConnectionCount:           1,
		ConnectionMedianLatencyMS: 1,
		RxBytes:                   1111,
		SessionCountSSH:           1,
	})

	// Stat inserted 180 days - 2 hour ago, should not be deleted before rollup.
	second := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt:                 now.AddDate(0, 0, -180).Add(2 * time.Hour),
		ConnectionCount:           1,
		ConnectionMedianLatencyMS: 1,
		RxBytes:                   2222,
		SessionCountSSH:           1,
	})

	// Stat inserted 179 days - 4 hour ago, should not be deleted at all.
	third := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt:                 now.AddDate(0, 0, -179).Add(4 * time.Hour),
		ConnectionCount:           1,
		ConnectionMedianLatencyMS: 1,
		RxBytes:                   3333,
		SessionCountSSH:           1,
	})

	// when
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, clk)
	defer closer.Close()

	// then
	var stats []database.GetWorkspaceAgentStatsRow
	var err error
	require.Eventuallyf(t, func() bool {
		// Query all stats created not earlier than ~7 months ago
		stats, err = db.GetWorkspaceAgentStats(ctx, now.AddDate(0, 0, -210))
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
	closer = dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, clk)
	defer closer.Close()

	// then
	require.Eventuallyf(t, func() bool {
		// Query all stats created not earlier than ~7 months ago
		stats, err = db.GetWorkspaceAgentStats(ctx, now.AddDate(0, 0, -210))
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

//nolint:paralleltest // It uses LockIDDBPurge.
func TestDeleteOldWorkspaceAgentLogs(t *testing.T) {
	ctx := testutil.Context(t, testutil.WaitShort)
	clk := quartz.NewMock(t)
	now := dbtime.Now()
	threshold := now.Add(-7 * 24 * time.Hour)
	beforeThreshold := threshold.Add(-24 * time.Hour)
	afterThreshold := threshold.Add(24 * time.Hour)
	clk.Set(now).MustWait(ctx)

	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID})
	tmpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, ActiveVersionID: tv.ID, CreatedBy: user.ID})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Given the following:

	// Workspace A was built twice before the threshold, and never connected on
	// either attempt.
	wsA := dbgen.Workspace(t, db, database.WorkspaceTable{Name: "a", OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
	wbA1 := mustCreateWorkspaceBuild(t, db, org, tv, wsA.ID, beforeThreshold, 1)
	wbA2 := mustCreateWorkspaceBuild(t, db, org, tv, wsA.ID, beforeThreshold, 2)
	agentA1 := mustCreateAgent(t, db, wbA1)
	agentA2 := mustCreateAgent(t, db, wbA2)
	mustCreateAgentLogs(ctx, t, db, agentA1, nil, "agent a1 logs should be deleted")
	mustCreateAgentLogs(ctx, t, db, agentA2, nil, "agent a2 logs should be retained")

	// Workspace B was built twice before the threshold.
	wsB := dbgen.Workspace(t, db, database.WorkspaceTable{Name: "b", OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
	wbB1 := mustCreateWorkspaceBuild(t, db, org, tv, wsB.ID, beforeThreshold, 1)
	wbB2 := mustCreateWorkspaceBuild(t, db, org, tv, wsB.ID, beforeThreshold, 2)
	agentB1 := mustCreateAgent(t, db, wbB1)
	agentB2 := mustCreateAgent(t, db, wbB2)
	mustCreateAgentLogs(ctx, t, db, agentB1, &beforeThreshold, "agent b1 logs should be deleted")
	mustCreateAgentLogs(ctx, t, db, agentB2, &beforeThreshold, "agent b2 logs should be retained")

	// Workspace C was built once before the threshold, and once after.
	wsC := dbgen.Workspace(t, db, database.WorkspaceTable{Name: "c", OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
	wbC1 := mustCreateWorkspaceBuild(t, db, org, tv, wsC.ID, beforeThreshold, 1)
	wbC2 := mustCreateWorkspaceBuild(t, db, org, tv, wsC.ID, afterThreshold, 2)
	agentC1 := mustCreateAgent(t, db, wbC1)
	agentC2 := mustCreateAgent(t, db, wbC2)
	mustCreateAgentLogs(ctx, t, db, agentC1, &beforeThreshold, "agent c1 logs should be deleted")
	mustCreateAgentLogs(ctx, t, db, agentC2, &afterThreshold, "agent c2 logs should be retained")

	// Workspace D was built twice after the threshold.
	wsD := dbgen.Workspace(t, db, database.WorkspaceTable{Name: "d", OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
	wbD1 := mustCreateWorkspaceBuild(t, db, org, tv, wsD.ID, afterThreshold, 1)
	wbD2 := mustCreateWorkspaceBuild(t, db, org, tv, wsD.ID, afterThreshold, 2)
	agentD1 := mustCreateAgent(t, db, wbD1)
	agentD2 := mustCreateAgent(t, db, wbD2)
	mustCreateAgentLogs(ctx, t, db, agentD1, &afterThreshold, "agent d1 logs should be retained")
	mustCreateAgentLogs(ctx, t, db, agentD2, &afterThreshold, "agent d2 logs should be retained")

	// Workspace E was build once after threshold but never connected.
	wsE := dbgen.Workspace(t, db, database.WorkspaceTable{Name: "e", OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
	wbE1 := mustCreateWorkspaceBuild(t, db, org, tv, wsE.ID, beforeThreshold, 1)
	agentE1 := mustCreateAgent(t, db, wbE1)
	mustCreateAgentLogs(ctx, t, db, agentE1, nil, "agent e1 logs should be retained")

	// when dbpurge runs

	// After dbpurge completes, the ticker is reset. Trap this call.

	done := awaitDoTick(ctx, t, clk)
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, clk)
	defer closer.Close()
	<-done // doTick() has now run.

	// then logs related to the following agents should be deleted:
	// Agent A1 never connected, was created before the threshold, and is not the
	// latest build.
	assertNoWorkspaceAgentLogs(ctx, t, db, agentA1.ID)
	// Agent B1 is not the latest build and the logs are from before threshold.
	assertNoWorkspaceAgentLogs(ctx, t, db, agentB1.ID)
	// Agent C1 is not the latest build and the logs are from before threshold.
	assertNoWorkspaceAgentLogs(ctx, t, db, agentC1.ID)

	// then logs related to the following agents should be retained:
	// Agent A2 is the latest build.
	assertWorkspaceAgentLogs(ctx, t, db, agentA2.ID, "agent a2 logs should be retained")
	// Agent B2 is the latest build.
	assertWorkspaceAgentLogs(ctx, t, db, agentB2.ID, "agent b2 logs should be retained")
	// Agent C2 is the latest build.
	assertWorkspaceAgentLogs(ctx, t, db, agentC2.ID, "agent c2 logs should be retained")
	// Agents D1, D2, and E1 are all after threshold.
	assertWorkspaceAgentLogs(ctx, t, db, agentD1.ID, "agent d1 logs should be retained")
	assertWorkspaceAgentLogs(ctx, t, db, agentD2.ID, "agent d2 logs should be retained")
	assertWorkspaceAgentLogs(ctx, t, db, agentE1.ID, "agent e1 logs should be retained")
}

func awaitDoTick(ctx context.Context, t *testing.T, clk *quartz.Mock) chan struct{} {
	t.Helper()
	ch := make(chan struct{})
	trapNow := clk.Trap().Now()
	trapStop := clk.Trap().TickerStop()
	trapReset := clk.Trap().TickerReset()
	go func() {
		defer close(ch)
		defer trapReset.Close()
		defer trapStop.Close()
		defer trapNow.Close()
		// Wait for the initial tick signified by a call to Now().
		trapNow.MustWait(ctx).MustRelease(ctx)
		// doTick runs here. Wait for the next
		// ticker reset event that signifies it's completed.
		trapReset.MustWait(ctx).MustRelease(ctx)
		// Ensure that the next tick happens in 10 minutes from start.
		d, w := clk.AdvanceNext()
		if !assert.Equal(t, 10*time.Minute, d) {
			return
		}
		w.MustWait(ctx)
		// Wait for the ticker stop event.
		trapStop.MustWait(ctx).MustRelease(ctx)
	}()

	return ch
}

func assertNoWorkspaceAgentLogs(ctx context.Context, t *testing.T, db database.Store, agentID uuid.UUID) {
	t.Helper()
	agentLogs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
		AgentID:      agentID,
		CreatedAfter: 0,
	})
	require.NoError(t, err)
	assert.Empty(t, agentLogs)
}

func assertWorkspaceAgentLogs(ctx context.Context, t *testing.T, db database.Store, agentID uuid.UUID, msg string) {
	t.Helper()
	agentLogs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
		AgentID:      agentID,
		CreatedAfter: 0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, agentLogs)
	for _, al := range agentLogs {
		assert.Equal(t, msg, al.Output)
	}
}

func mustCreateWorkspaceBuild(t *testing.T, db database.Store, org database.Organization, tv database.TemplateVersion, wsID uuid.UUID, createdAt time.Time, n int32) database.WorkspaceBuild {
	t.Helper()
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		CreatedAt:      createdAt,
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
	})
	wb := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		CreatedAt:         createdAt,
		WorkspaceID:       wsID,
		JobID:             job.ID,
		TemplateVersionID: tv.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       n,
	})
	require.Equal(t, createdAt.UTC(), wb.CreatedAt.UTC())
	return wb
}

func mustCreateAgent(t *testing.T, db database.Store, wb database.WorkspaceBuild) database.WorkspaceAgent {
	t.Helper()
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID:      wb.JobID,
		Transition: database.WorkspaceTransitionStart,
		CreatedAt:  wb.CreatedAt,
	})

	ws, err := db.GetWorkspaceByID(context.Background(), wb.WorkspaceID)
	require.NoError(t, err)

	wa := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		Name:             fmt.Sprintf("%s%d", ws.Name, wb.BuildNumber),
		ResourceID:       resource.ID,
		CreatedAt:        wb.CreatedAt,
		FirstConnectedAt: sql.NullTime{},
		DisconnectedAt:   sql.NullTime{},
		LastConnectedAt:  sql.NullTime{},
	})
	require.Equal(t, wb.CreatedAt.UTC(), wa.CreatedAt.UTC())
	return wa
}

func mustCreateAgentLogs(ctx context.Context, t *testing.T, db database.Store, agent database.WorkspaceAgent, agentLastConnectedAt *time.Time, output string) {
	t.Helper()
	if agentLastConnectedAt != nil {
		require.NoError(t, db.UpdateWorkspaceAgentConnectionByID(ctx, database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:              agent.ID,
			LastConnectedAt: sql.NullTime{Time: *agentLastConnectedAt, Valid: true},
		}))
	}
	_, err := db.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:   agent.ID,
		CreatedAt: agent.CreatedAt,
		Output:    []string{output},
		Level:     []database.LogLevel{database.LogLevelDebug},
	})
	require.NoError(t, err)
	// Make sure that agent logs have been collected.
	agentLogs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
		AgentID: agent.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, agentLogs, "agent logs must be present")
}

//nolint:paralleltest // It uses LockIDDBPurge.
func TestDeleteOldProvisionerDaemons(t *testing.T) {
	// TODO: must refactor DeleteOldProvisionerDaemons to allow passing in cutoff
	//       before using quartz.NewMock
	clk := quartz.NewReal()
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
		KeyID:          codersdk.ProvisionerKeyUUIDBuiltIn,
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
		KeyID:          codersdk.ProvisionerKeyUUIDBuiltIn,
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
		KeyID:          codersdk.ProvisionerKeyUUIDBuiltIn,
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
		KeyID:          codersdk.ProvisionerKeyUUIDBuiltIn,
	})
	require.NoError(t, err)

	// when
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, clk)
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

//nolint:paralleltest // It uses LockIDDBPurge.
func TestDeleteOldAuditLogConnectionEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	clk := quartz.NewMock(t)
	now := dbtime.Now()
	afterThreshold := now.Add(-91 * 24 * time.Hour)       // 91 days ago (older than 90 day threshold)
	beforeThreshold := now.Add(-30 * 24 * time.Hour)      // 30 days ago (newer than 90 day threshold)
	closeBeforeThreshold := now.Add(-89 * 24 * time.Hour) // 89 days ago
	clk.Set(now).MustWait(ctx)

	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	oldConnectLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           afterThreshold,
		Action:         database.AuditActionConnect,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	oldDisconnectLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           afterThreshold,
		Action:         database.AuditActionDisconnect,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	oldOpenLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           afterThreshold,
		Action:         database.AuditActionOpen,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	oldCloseLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           afterThreshold,
		Action:         database.AuditActionClose,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	recentConnectLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           beforeThreshold,
		Action:         database.AuditActionConnect,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	oldNonConnectionLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           afterThreshold,
		Action:         database.AuditActionCreate,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	nearThresholdConnectLog := dbgen.AuditLog(t, db, database.AuditLog{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Time:           closeBeforeThreshold,
		Action:         database.AuditActionConnect,
		ResourceType:   database.ResourceTypeWorkspace,
	})

	// Run the purge
	done := awaitDoTick(ctx, t, clk)
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, clk)
	defer closer.Close()
	// Wait for tick
	testutil.TryReceive(ctx, t, done)

	// Verify results by querying all audit logs
	logs, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{})
	require.NoError(t, err)

	// Extract log IDs for comparison
	logIDs := make([]uuid.UUID, len(logs))
	for i, log := range logs {
		logIDs[i] = log.AuditLog.ID
	}

	require.NotContains(t, logIDs, oldConnectLog.ID, "old connect log should be deleted")
	require.NotContains(t, logIDs, oldDisconnectLog.ID, "old disconnect log should be deleted")
	require.NotContains(t, logIDs, oldOpenLog.ID, "old open log should be deleted")
	require.NotContains(t, logIDs, oldCloseLog.ID, "old close log should be deleted")
	require.Contains(t, logIDs, recentConnectLog.ID, "recent connect log should be kept")
	require.Contains(t, logIDs, nearThresholdConnectLog.ID, "near threshold connect log should be kept")
	require.Contains(t, logIDs, oldNonConnectionLog.ID, "old non-connection log should be kept")
}

func TestDeleteOldAuditLogConnectionEventsLimit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	now := dbtime.Now()
	threshold := now.Add(-90 * 24 * time.Hour)

	for i := 0; i < 5; i++ {
		dbgen.AuditLog(t, db, database.AuditLog{
			UserID:         user.ID,
			OrganizationID: org.ID,
			Time:           threshold.Add(-time.Duration(i+1) * time.Hour),
			Action:         database.AuditActionConnect,
			ResourceType:   database.ResourceTypeWorkspace,
		})
	}

	err := db.DeleteOldAuditLogConnectionEvents(ctx, database.DeleteOldAuditLogConnectionEventsParams{
		BeforeTime: threshold,
		LimitCount: 1,
	})
	require.NoError(t, err)

	logs, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{})
	require.NoError(t, err)

	require.Len(t, logs, 4)

	err = db.DeleteOldAuditLogConnectionEvents(ctx, database.DeleteOldAuditLogConnectionEventsParams{
		BeforeTime: threshold,
		LimitCount: 100,
	})
	require.NoError(t, err)

	logs, err = db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{})
	require.NoError(t, err)

	require.Len(t, logs, 0)
}

func TestExpireOldAPIKeys(t *testing.T) {
	t.Parallel()

	// Given: a number of workspaces and API keys owned by a regular user and the prebuilds system user.
	var (
		ctx    = testutil.Context(t, testutil.WaitShort)
		now    = dbtime.Now()
		db, _  = dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		org    = dbgen.Organization(t, db, database.Organization{})
		user   = dbgen.User(t, db, database.User{})
		tpl    = dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
		userWs = dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:    user.ID,
			TemplateID: tpl.ID,
		})
		prebuildsWs = dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:    database.PrebuildsSystemUserID,
			TemplateID: tpl.ID,
		})
		createAPIKey = func(userID uuid.UUID, name string) database.APIKey {
			k, _ := dbgen.APIKey(t, db, database.APIKey{UserID: userID, TokenName: name, ExpiresAt: now.Add(time.Hour)}, func(iap *database.InsertAPIKeyParams) {
				iap.TokenName = name
			})
			return k
		}
		assertKeyActive = func(kid string) {
			k, err := db.GetAPIKeyByID(ctx, kid)
			require.NoError(t, err)
			assert.True(t, k.ExpiresAt.After(now))
		}
		assertKeyExpired = func(kid string) {
			k, err := db.GetAPIKeyByID(ctx, kid)
			require.NoError(t, err)
			assert.True(t, k.ExpiresAt.Equal(now))
		}
		unnamedUserAPIKey         = createAPIKey(user.ID, "")
		unnamedPrebuildsAPIKey    = createAPIKey(database.PrebuildsSystemUserID, "")
		namedUserAPIKey           = createAPIKey(user.ID, "my-token")
		namedPrebuildsAPIKey      = createAPIKey(database.PrebuildsSystemUserID, "also-my-token")
		userWorkspaceAPIKey1      = createAPIKey(user.ID, provisionerdserver.WorkspaceSessionTokenName(user.ID, userWs.ID))
		userWorkspaceAPIKey2      = createAPIKey(user.ID, provisionerdserver.WorkspaceSessionTokenName(user.ID, prebuildsWs.ID))
		prebuildsWorkspaceAPIKey1 = createAPIKey(database.PrebuildsSystemUserID, provisionerdserver.WorkspaceSessionTokenName(database.PrebuildsSystemUserID, prebuildsWs.ID))
		prebuildsWorkspaceAPIKey2 = createAPIKey(database.PrebuildsSystemUserID, provisionerdserver.WorkspaceSessionTokenName(database.PrebuildsSystemUserID, userWs.ID))
	)

	// When: we call ExpirePrebuildsAPIKeys
	err := db.ExpirePrebuildsAPIKeys(ctx, now)
	// Then: no errors is reported.
	require.NoError(t, err)

	// We do not touch user API keys.
	assertKeyActive(unnamedUserAPIKey.ID)
	assertKeyActive(namedUserAPIKey.ID)
	assertKeyActive(userWorkspaceAPIKey1.ID)
	assertKeyActive(userWorkspaceAPIKey2.ID)
	// Unnamed prebuilds API keys get expired.
	assertKeyExpired(unnamedPrebuildsAPIKey.ID)
	// API keys for workspaces still owned by prebuilds user remain active until claimed.
	assertKeyActive(prebuildsWorkspaceAPIKey1.ID)
	// API keys for workspaces no longer owned by prebuilds user get expired.
	assertKeyExpired(prebuildsWorkspaceAPIKey2.ID)
	// Out of an abundance of caution, we do not expire explicitly named prebuilds API keys.
	assertKeyActive(namedPrebuildsAPIKey.ID)
}

func TestDeleteOldTelemetryHeartbeats(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	clk := quartz.NewMock(t)
	now := clk.Now().UTC()

	// Insert telemetry heartbeats.
	err := db.InsertTelemetryLock(ctx, database.InsertTelemetryLockParams{
		EventType:      "aibridge_interceptions_summary",
		PeriodEndingAt: now.Add(-25 * time.Hour), // should be purged
	})
	require.NoError(t, err)
	err = db.InsertTelemetryLock(ctx, database.InsertTelemetryLockParams{
		EventType:      "aibridge_interceptions_summary",
		PeriodEndingAt: now.Add(-23 * time.Hour), // should be kept
	})
	require.NoError(t, err)
	err = db.InsertTelemetryLock(ctx, database.InsertTelemetryLockParams{
		EventType:      "aibridge_interceptions_summary",
		PeriodEndingAt: now, // should be kept
	})
	require.NoError(t, err)

	done := awaitDoTick(ctx, t, clk)
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, clk)
	defer closer.Close()
	<-done // doTick() has now run.

	require.Eventuallyf(t, func() bool {
		// We use an SQL queries directly here because we don't expose queries
		// for deleting heartbeats in the application code.
		var totalCount int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM telemetry_locks;
		`).Scan(&totalCount)
		assert.NoError(t, err)

		var oldCount int
		err = sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM telemetry_locks WHERE period_ending_at < $1;
		`, now.Add(-24*time.Hour)).Scan(&oldCount)
		assert.NoError(t, err)

		// Expect 2 heartbeats remaining and none older than 24 hours.
		t.Logf("eventually: total count: %d, old count: %d", totalCount, oldCount)
		return totalCount == 2 && oldCount == 0
	}, testutil.WaitShort, testutil.IntervalFast, "it should delete old telemetry heartbeats")
}

//nolint:paralleltest // It uses LockIDDBPurge.
func TestDeleteOldAIBridgeRecords(t *testing.T) {
	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
	recentRecordAge := 1 * time.Hour // Always retained as a control.

	tests := []struct {
		name           string
		retention      time.Duration
		recordAge      time.Duration
		expectDeletion bool
	}{
		{
			name:           "retention enabled, old records deleted",
			retention:      30 * 24 * time.Hour, // 30 days
			recordAge:      31 * 24 * time.Hour, // 31 days old
			expectDeletion: true,
		},
		{
			name:           "retention enabled, recent records kept",
			retention:      30 * 24 * time.Hour, // 30 days
			recordAge:      15 * 24 * time.Hour, // 15 days old
			expectDeletion: false,
		},
		{
			name:           "retention disabled, old records kept",
			retention:      0,                    // disabled
			recordAge:      365 * 24 * time.Hour, // 1 year old
			expectDeletion: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)

			clk := quartz.NewMock(t)
			recordTime := now.Add(-tt.recordAge)
			recentRecordTime := now.Add(-recentRecordAge)
			clk.Set(now).MustWait(ctx)

			db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			user := dbgen.User(t, db, database.User{})

			// Create AI Bridge interception with related records.
			interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.New(),
				APIKeyID:    sql.NullString{},
				InitiatorID: user.ID,
				Provider:    "anthropic",
				Model:       "claude-3-5-sonnet",
				StartedAt:   recordTime,
			}, &recordTime)

			_ = dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				ID:                 uuid.New(),
				InterceptionID:     interception.ID,
				ProviderResponseID: "resp-1",
				InputTokens:        100,
				OutputTokens:       50,
				CreatedAt:          recordTime,
			})

			_ = dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
				ID:                 uuid.New(),
				InterceptionID:     interception.ID,
				ProviderResponseID: "resp-1",
				Prompt:             "test prompt",
				CreatedAt:          recordTime,
			})

			_ = dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
				ID:                 uuid.New(),
				InterceptionID:     interception.ID,
				ProviderResponseID: "resp-1",
				Tool:               "test-tool",
				ServerUrl:          sql.NullString{String: "http://test", Valid: true},
				Input:              "{}",
				Injected:           true,
				CreatedAt:          recordTime,
			})

			// Create a recent interception that should always be retained.
			recentInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.New(),
				APIKeyID:    sql.NullString{},
				InitiatorID: user.ID,
				Provider:    "anthropic",
				Model:       "claude-3-5-sonnet",
				StartedAt:   recentRecordTime,
			}, &recentRecordTime)

			// Run the purge.
			done := awaitDoTick(ctx, t, clk)
			closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
				AI: codersdk.AIConfig{
					BridgeConfig: codersdk.AIBridgeConfig{
						Retention: serpent.Duration(tt.retention),
					},
				},
			}, clk)
			defer closer.Close()
			testutil.TryReceive(ctx, t, done)

			// Verify interceptions.
			interceptions, err := db.GetAIBridgeInterceptions(ctx)
			require.NoError(t, err)

			interceptionIDs := make([]uuid.UUID, len(interceptions))
			for i, ic := range interceptions {
				interceptionIDs[i] = ic.ID
			}

			// Recent interception should always be retained.
			require.Contains(t, interceptionIDs, recentInterception.ID, "recent interception should always be retained")

			if tt.expectDeletion {
				require.NotContains(t, interceptionIDs, interception.ID, "interception should be deleted")
			} else {
				require.Contains(t, interceptionIDs, interception.ID, "interception should be retained")
			}

			// Verify related records follow the same fate.
			tokenUsages, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, interception.ID)
			require.NoError(t, err)
			userPrompts, err := db.GetAIBridgeUserPromptsByInterceptionID(ctx, interception.ID)
			require.NoError(t, err)
			toolUsages, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, interception.ID)
			require.NoError(t, err)

			if tt.expectDeletion {
				require.Empty(t, tokenUsages, "token usages should be deleted")
				require.Empty(t, userPrompts, "user prompts should be deleted")
				require.Empty(t, toolUsages, "tool usages should be deleted")
			} else {
				require.Len(t, tokenUsages, 1, "token usages should be retained")
				require.Len(t, userPrompts, 1, "user prompts should be retained")
				require.Len(t, toolUsages, 1, "tool usages should be retained")
			}
		})
	}
}
