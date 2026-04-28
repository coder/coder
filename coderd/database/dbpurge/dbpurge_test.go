package dbpurge_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbrollup"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationsmock"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
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
	mDB.EXPECT().GetChatRetentionDays(gomock.Any()).Return(int32(0), nil).AnyTimes()
	mDB.EXPECT().GetChatAutoArchiveDays(gomock.Any(), codersdk.DefaultChatAutoArchiveDays).Return(int32(0), nil).AnyTimes()
	mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("db_purge")).Return(nil).Times(2)
	purger := dbpurge.New(context.Background(), testutil.Logger(t), mDB, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
	<-done // wait for doTick() to run.
	require.NoError(t, purger.Close())
}

//nolint:paralleltest // It uses LockIDDBPurge.
func TestMetrics(t *testing.T) {
	t.Run("SuccessfulIteration", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		reg := prometheus.NewRegistry()
		clk := quartz.NewMock(t)
		now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
		clk.Set(now).MustWait(ctx)

		db, _ := dbtestutil.NewDB(t)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		user := dbgen.User(t, db, database.User{})

		oldExpiredKey, _ := dbgen.APIKey(t, db, database.APIKey{
			UserID:    user.ID,
			ExpiresAt: now.Add(-8 * 24 * time.Hour), // Expired 8 days ago
			TokenName: "old-expired-key",
		})

		_, err := db.GetAPIKeyByID(ctx, oldExpiredKey.ID)
		require.NoError(t, err, "key should exist before purge")

		done := awaitDoTick(ctx, t, clk)
		closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
			Retention: codersdk.RetentionConfig{
				APIKeys: serpent.Duration(7 * 24 * time.Hour), // 7 days retention
			},
		}, reg, nopAuditorPtr(t), dbpurge.WithClock(clk))
		defer closer.Close()
		testutil.TryReceive(ctx, t, done)

		hist := promhelp.HistogramValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "true",
		})
		require.NotNil(t, hist)
		require.Greater(t, hist.GetSampleCount(), uint64(0), "should have at least one sample")

		expiredAPIKeys := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "expired_api_keys",
		})
		require.Greater(t, expiredAPIKeys, 0, "should have deleted at least one expired API key")

		_, err = db.GetAPIKeyByID(ctx, oldExpiredKey.ID)
		require.Error(t, err, "key should be deleted after purge")

		workspaceAgentLogs := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "workspace_agent_logs",
		})
		require.GreaterOrEqual(t, workspaceAgentLogs, 0)

		aibridgeRecords := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "aibridge_records",
		})
		require.GreaterOrEqual(t, aibridgeRecords, 0)

		connectionLogs := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "connection_logs",
		})
		require.GreaterOrEqual(t, connectionLogs, 0)

		auditLogs := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "audit_logs",
		})
		require.GreaterOrEqual(t, auditLogs, 0)

		chats := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "chats",
		})
		require.GreaterOrEqual(t, chats, 0)

		chatFiles := promhelp.CounterValue(t, reg, "coderd_dbpurge_records_purged_total", prometheus.Labels{
			"record_type": "chat_files",
		})
		require.GreaterOrEqual(t, chatFiles, 0)
	})

	t.Run("FailedIteration", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		reg := prometheus.NewRegistry()
		clk := quartz.NewMock(t)
		now := clk.Now()
		clk.Set(now).MustWait(ctx)

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		mDB.EXPECT().GetChatRetentionDays(gomock.Any()).Return(int32(0), nil).AnyTimes()
		mDB.EXPECT().GetChatAutoArchiveDays(gomock.Any(), codersdk.DefaultChatAutoArchiveDays).Return(int32(0), nil).AnyTimes()
		mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("db_purge")).
			Return(xerrors.New("simulated database error")).
			MinTimes(1)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		done := awaitDoTick(ctx, t, clk)
		closer := dbpurge.New(ctx, logger, mDB, &codersdk.DeploymentValues{}, reg, nopAuditorPtr(t), dbpurge.WithClock(clk))
		defer closer.Close()
		testutil.TryReceive(ctx, t, done)

		hist := promhelp.HistogramValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "false",
		})
		require.NotNil(t, hist)
		require.Greater(t, hist.GetSampleCount(), uint64(0), "should have at least one sample")

		successHist := promhelp.MetricValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "true",
		})
		require.Nil(t, successHist, "should not have success=true metric on failure")
	})

	// A failed retention read must not block unrelated purges,
	// but must skip the chat passes and surface as a failed
	// iteration via the metric.
	t.Run("FailedChatRetentionRead", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		reg := prometheus.NewRegistry()
		clk := quartz.NewMock(t)
		now := clk.Now()
		clk.Set(now).MustWait(ctx)

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		mDB.EXPECT().GetChatRetentionDays(gomock.Any()).
			Return(int32(0), xerrors.New("simulated retention read error")).
			MinTimes(1)
		// Both reads happen before the bail; InTx still runs
		// so unrelated purges commit best-effort.
		mDB.EXPECT().GetChatAutoArchiveDays(gomock.Any(), codersdk.DefaultChatAutoArchiveDays).
			Return(int32(0), nil).AnyTimes()
		mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("db_purge")).
			Return(nil).MinTimes(1)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		done := awaitDoTick(ctx, t, clk)
		closer := dbpurge.New(ctx, logger, mDB, &codersdk.DeploymentValues{}, reg, nopAuditorPtr(t), dbpurge.WithClock(clk))
		defer closer.Close()
		testutil.TryReceive(ctx, t, done)

		hist := promhelp.HistogramValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "false",
		})
		require.NotNil(t, hist)
		require.Greater(t, hist.GetSampleCount(), uint64(0),
			"failed retention read must record a failed iteration")

		successHist := promhelp.MetricValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "true",
		})
		require.Nil(t, successHist, "should not have success=true metric on retention read failure")
	})

	// Same contract as FailedChatRetentionRead, but the
	// auto-archive read is the half that fails.
	t.Run("FailedChatAutoArchiveRead", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		reg := prometheus.NewRegistry()
		clk := quartz.NewMock(t)
		now := clk.Now()
		clk.Set(now).MustWait(ctx)

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		mDB.EXPECT().GetChatRetentionDays(gomock.Any()).Return(int32(30), nil).AnyTimes()
		mDB.EXPECT().GetChatAutoArchiveDays(gomock.Any(), codersdk.DefaultChatAutoArchiveDays).
			Return(int32(0), xerrors.New("simulated auto-archive read error")).
			MinTimes(1)
		// InTx still runs so unrelated purges commit; chat
		// passes inside the tx are skipped.
		mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("db_purge")).
			Return(nil).MinTimes(1)

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		done := awaitDoTick(ctx, t, clk)
		closer := dbpurge.New(ctx, logger, mDB, &codersdk.DeploymentValues{}, reg, nopAuditorPtr(t), dbpurge.WithClock(clk))
		defer closer.Close()
		testutil.TryReceive(ctx, t, done)

		hist := promhelp.HistogramValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "false",
		})
		require.NotNil(t, hist)
		require.Greater(t, hist.GetSampleCount(), uint64(0),
			"failed auto-archive read must record a failed iteration")

		successHist := promhelp.MetricValue(t, reg, "coderd_dbpurge_iteration_duration_seconds", prometheus.Labels{
			"success": "true",
		})
		require.Nil(t, successHist, "should not have success=true metric on auto-archive read failure")
	})
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
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
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
	closer = dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
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
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
		Retention: codersdk.RetentionConfig{
			WorkspaceAgentLogs: serpent.Duration(7 * 24 * time.Hour),
		},
	}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))

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

// tickDriver drives one or more dbpurge ticks against a single
// dbpurge.New instance. Unlike awaitDoTick it must be constructed
// *before* dbpurge.New so its traps are installed when the forced
// initial tick fires. awaitInitial waits for the forced tick's
// doTick to complete without advancing the clock, so no loop
// iteration has yet run; awaitNext then explicitly drives each
// subsequent iteration. This keeps each tick's observable state
// isolated and deterministic, which matters for tests where
// per-tick work differs (e.g. batch-size pagination).
type tickDriver struct {
	clk       *quartz.Mock
	trapNow   *quartz.Trap
	trapStop  *quartz.Trap
	trapReset *quartz.Trap
}

func newTickDriver(t *testing.T, clk *quartz.Mock) *tickDriver {
	t.Helper()
	d := &tickDriver{
		clk:       clk,
		trapNow:   clk.Trap().Now(),
		trapStop:  clk.Trap().TickerStop(),
		trapReset: clk.Trap().TickerReset(),
	}
	return d
}

// close releases all traps. Call this via defer *after* the defer
// that closes the dbpurge instance so trap closure releases the
// shutdown ticker.Stop() rather than blocking on it.
func (d *tickDriver) close() {
	d.trapReset.Close()
	d.trapStop.Close()
	d.trapNow.Close()
}

// awaitInitial waits for the forced initial tick's doTick to
// complete. No loop iteration runs because the clock has not been
// advanced.
func (d *tickDriver) awaitInitial(ctx context.Context, t *testing.T) {
	t.Helper()
	d.trapNow.MustWait(ctx).MustRelease(ctx)
	d.trapReset.MustWait(ctx).MustRelease(ctx)
}

// awaitNext advances the clock by the tick interval, lets the loop
// receive the tick and run doTick, and waits for the ensuing
// ticker.Reset so the driver is ready for another awaitNext.
func (d *tickDriver) awaitNext(ctx context.Context, t *testing.T) {
	t.Helper()
	dur, w := d.clk.AdvanceNext()
	require.Equal(t, 10*time.Minute, dur)
	w.MustWait(ctx)
	d.trapStop.MustWait(ctx).MustRelease(ctx)
	d.trapReset.MustWait(ctx).MustRelease(ctx)
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

func TestDeleteOldWorkspaceAgentLogsRetention(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)

	testCases := []struct {
		name            string
		retentionConfig codersdk.RetentionConfig
		logsAge         time.Duration
		expectDeleted   bool
	}{
		{
			name: "RetentionEnabled",
			retentionConfig: codersdk.RetentionConfig{
				WorkspaceAgentLogs: serpent.Duration(7 * 24 * time.Hour), // 7 days
			},
			logsAge:       8 * 24 * time.Hour, // 8 days ago
			expectDeleted: true,
		},
		{
			name: "RetentionDisabled",
			retentionConfig: codersdk.RetentionConfig{
				WorkspaceAgentLogs: serpent.Duration(0),
			},
			logsAge:       60 * 24 * time.Hour, // 60 days ago
			expectDeleted: false,
		},

		{
			name: "CustomRetention30Days",
			retentionConfig: codersdk.RetentionConfig{
				WorkspaceAgentLogs: serpent.Duration(30 * 24 * time.Hour), // 30 days
			},
			logsAge:       31 * 24 * time.Hour, // 31 days ago
			expectDeleted: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			clk := quartz.NewMock(t)
			clk.Set(now).MustWait(ctx)

			oldTime := now.Add(-tc.logsAge)

			db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID})
			tmpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, ActiveVersionID: tv.ID, CreatedBy: user.ID})

			ws := dbgen.Workspace(t, db, database.WorkspaceTable{Name: "test-ws", OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})
			wb1 := mustCreateWorkspaceBuild(t, db, org, tv, ws.ID, oldTime, 1)
			wb2 := mustCreateWorkspaceBuild(t, db, org, tv, ws.ID, oldTime, 2)
			agent1 := mustCreateAgent(t, db, wb1)
			agent2 := mustCreateAgent(t, db, wb2)
			mustCreateAgentLogs(ctx, t, db, agent1, &oldTime, "agent 1 logs")
			mustCreateAgentLogs(ctx, t, db, agent2, &oldTime, "agent 2 logs")

			// Run the purge.
			done := awaitDoTick(ctx, t, clk)
			closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
				Retention: tc.retentionConfig,
			}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
			defer closer.Close()
			testutil.TryReceive(ctx, t, done)

			// Verify results.
			if tc.expectDeleted {
				assertNoWorkspaceAgentLogs(ctx, t, db, agent1.ID)
			} else {
				assertWorkspaceAgentLogs(ctx, t, db, agent1.ID, "agent 1 logs")
			}
			// Latest build logs are always retained.
			assertWorkspaceAgentLogs(ctx, t, db, agent2.ID, "agent 2 logs")
		})
	}
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
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
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
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
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
	closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
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

func TestDeleteOldConnectionLogs(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
	retentionPeriod := 30 * 24 * time.Hour
	afterThreshold := now.Add(-retentionPeriod).Add(-24 * time.Hour) // 31 days ago (older than threshold)
	beforeThreshold := now.Add(-15 * 24 * time.Hour)                 // 15 days ago (newer than threshold)

	testCases := []struct {
		name                  string
		retentionConfig       codersdk.RetentionConfig
		oldLogTime            time.Time
		recentLogTime         *time.Time // nil means no recent log created
		expectOldDeleted      bool
		expectedLogsRemaining int
	}{
		{
			name: "RetentionEnabled",
			retentionConfig: codersdk.RetentionConfig{
				ConnectionLogs: serpent.Duration(retentionPeriod),
			},
			oldLogTime:            afterThreshold,
			recentLogTime:         &beforeThreshold,
			expectOldDeleted:      true,
			expectedLogsRemaining: 1, // only recent log remains
		},
		{
			name: "RetentionDisabled",
			retentionConfig: codersdk.RetentionConfig{
				ConnectionLogs: serpent.Duration(0),
			},
			oldLogTime:            now.Add(-365 * 24 * time.Hour), // 1 year ago
			recentLogTime:         nil,
			expectOldDeleted:      false,
			expectedLogsRemaining: 1, // old log is kept
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			clk := quartz.NewMock(t)
			clk.Set(now).MustWait(ctx)

			db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			// Setup test fixtures.
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID})
			tmpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, ActiveVersionID: tv.ID, CreatedBy: user.ID})
			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OwnerID:        user.ID,
				OrganizationID: org.ID,
				TemplateID:     tmpl.ID,
			})

			// Create old connection log.
			oldLog := dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
				ID:               uuid.New(),
				Time:             tc.oldLogTime,
				OrganizationID:   org.ID,
				WorkspaceOwnerID: user.ID,
				WorkspaceID:      workspace.ID,
				WorkspaceName:    workspace.Name,
				AgentName:        "agent1",
				Type:             database.ConnectionTypeSsh,
				ConnectionStatus: database.ConnectionStatusConnected,
			})

			// Create recent connection log if specified.
			var recentLog database.ConnectionLog
			if tc.recentLogTime != nil {
				recentLog = dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
					ID:               uuid.New(),
					Time:             *tc.recentLogTime,
					OrganizationID:   org.ID,
					WorkspaceOwnerID: user.ID,
					WorkspaceID:      workspace.ID,
					WorkspaceName:    workspace.Name,
					AgentName:        "agent2",
					Type:             database.ConnectionTypeSsh,
					ConnectionStatus: database.ConnectionStatusConnected,
				})
			}

			// Run the purge.
			done := awaitDoTick(ctx, t, clk)
			closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
				Retention: tc.retentionConfig,
			}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
			defer closer.Close()
			testutil.TryReceive(ctx, t, done)

			// Verify results.
			logs, err := db.GetConnectionLogsOffset(ctx, database.GetConnectionLogsOffsetParams{
				LimitOpt: 100,
			})
			require.NoError(t, err)
			require.Len(t, logs, tc.expectedLogsRemaining, "unexpected number of logs remaining")

			logIDs := make([]uuid.UUID, len(logs))
			for i, log := range logs {
				logIDs[i] = log.ConnectionLog.ID
			}

			if tc.expectOldDeleted {
				require.NotContains(t, logIDs, oldLog.ID, "old connection log should be deleted")
			} else {
				require.Contains(t, logIDs, oldLog.ID, "old connection log should NOT be deleted")
			}

			if tc.recentLogTime != nil {
				require.Contains(t, logIDs, recentLog.ID, "recent connection log should be kept")
			}
		})
	}
}

func TestDeleteOldAIBridgeRecords(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
	retentionPeriod := 30 * 24 * time.Hour                                // 30 days
	afterThreshold := now.Add(-retentionPeriod).Add(-24 * time.Hour)      // 31 days ago (older than threshold)
	beforeThreshold := now.Add(-15 * 24 * time.Hour)                      // 15 days ago (newer than threshold)
	closeBeforeThreshold := now.Add(-retentionPeriod).Add(24 * time.Hour) // 29 days ago

	type testFixtures struct {
		oldInterception            database.AIBridgeInterception
		oldInterceptionWithRelated database.AIBridgeInterception
		recentInterception         database.AIBridgeInterception
		nearThresholdInterception  database.AIBridgeInterception
	}

	testCases := []struct {
		name      string
		retention time.Duration
		verify    func(t *testing.T, ctx context.Context, db database.Store, fixtures testFixtures)
	}{
		{
			name:      "RetentionEnabled",
			retention: retentionPeriod,
			verify: func(t *testing.T, ctx context.Context, db database.Store, fixtures testFixtures) {
				t.Helper()

				interceptions, err := db.GetAIBridgeInterceptions(ctx)
				require.NoError(t, err)
				require.Len(t, interceptions, 2, "expected 2 interceptions remaining")

				interceptionIDs := make([]uuid.UUID, len(interceptions))
				for i, interception := range interceptions {
					interceptionIDs[i] = interception.ID
				}

				require.NotContains(t, interceptionIDs, fixtures.oldInterception.ID, "old interception should be deleted")
				require.NotContains(t, interceptionIDs, fixtures.oldInterceptionWithRelated.ID, "old interception with related records should be deleted")
				require.Contains(t, interceptionIDs, fixtures.recentInterception.ID, "recent interception should be kept")
				require.Contains(t, interceptionIDs, fixtures.nearThresholdInterception.ID, "near threshold interception should be kept")

				// Verify related records were deleted for old interception.
				oldTokenUsages, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, fixtures.oldInterceptionWithRelated.ID)
				require.NoError(t, err)
				require.Empty(t, oldTokenUsages, "old token usages should be deleted")

				oldUserPrompts, err := db.GetAIBridgeUserPromptsByInterceptionID(ctx, fixtures.oldInterceptionWithRelated.ID)
				require.NoError(t, err)
				require.Empty(t, oldUserPrompts, "old user prompts should be deleted")

				oldToolUsages, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, fixtures.oldInterceptionWithRelated.ID)
				require.NoError(t, err)
				require.Empty(t, oldToolUsages, "old tool usages should be deleted")

				// Verify related records were NOT deleted for near-threshold interception.
				newTokenUsages, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, fixtures.nearThresholdInterception.ID)
				require.NoError(t, err)
				require.Len(t, newTokenUsages, 1, "near threshold token usages should not be deleted")

				newUserPrompts, err := db.GetAIBridgeUserPromptsByInterceptionID(ctx, fixtures.nearThresholdInterception.ID)
				require.NoError(t, err)
				require.Len(t, newUserPrompts, 1, "near threshold user prompts should not be deleted")

				newToolUsages, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, fixtures.nearThresholdInterception.ID)
				require.NoError(t, err)
				require.Len(t, newToolUsages, 1, "near threshold tool usages should not be deleted")
			},
		},
		{
			name:      "RetentionDisabled",
			retention: 0,
			verify: func(t *testing.T, ctx context.Context, db database.Store, fixtures testFixtures) {
				t.Helper()

				interceptions, err := db.GetAIBridgeInterceptions(ctx)
				require.NoError(t, err)
				require.Len(t, interceptions, 4, "expected all 4 interceptions to be retained")

				interceptionIDs := make([]uuid.UUID, len(interceptions))
				for i, interception := range interceptions {
					interceptionIDs[i] = interception.ID
				}

				require.Contains(t, interceptionIDs, fixtures.oldInterception.ID, "old interception should be kept")
				require.Contains(t, interceptionIDs, fixtures.oldInterceptionWithRelated.ID, "old interception with related records should be kept")
				require.Contains(t, interceptionIDs, fixtures.recentInterception.ID, "recent interception should be kept")
				require.Contains(t, interceptionIDs, fixtures.nearThresholdInterception.ID, "near threshold interception should be kept")

				// Verify all related records were kept.
				oldTokenUsages, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, fixtures.oldInterceptionWithRelated.ID)
				require.NoError(t, err)
				require.Len(t, oldTokenUsages, 1, "old token usages should be kept")

				oldUserPrompts, err := db.GetAIBridgeUserPromptsByInterceptionID(ctx, fixtures.oldInterceptionWithRelated.ID)
				require.NoError(t, err)
				require.Len(t, oldUserPrompts, 1, "old user prompts should be kept")

				oldToolUsages, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, fixtures.oldInterceptionWithRelated.ID)
				require.NoError(t, err)
				require.Len(t, oldToolUsages, 1, "old tool usages should be kept")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			clk := quartz.NewMock(t)
			clk.Set(now).MustWait(ctx)

			db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			user := dbgen.User(t, db, database.User{})

			// Create old AI Bridge interception (should be deleted when retention enabled).
			oldInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.New(),
				APIKeyID:    sql.NullString{},
				InitiatorID: user.ID,
				Provider:    "anthropic",
				Model:       "claude-3-5-sonnet",
				StartedAt:   afterThreshold,
			}, &afterThreshold)

			// Create old interception with related records (should all be deleted when retention enabled).
			oldInterceptionWithRelated := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.New(),
				APIKeyID:    sql.NullString{},
				InitiatorID: user.ID,
				Provider:    "openai",
				Model:       "gpt-4",
				StartedAt:   afterThreshold,
			}, &afterThreshold)

			_ = dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				ID:                 uuid.New(),
				InterceptionID:     oldInterceptionWithRelated.ID,
				ProviderResponseID: "resp-1",
				InputTokens:        100,
				OutputTokens:       50,
				CreatedAt:          afterThreshold,
			})

			_ = dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
				ID:                 uuid.New(),
				InterceptionID:     oldInterceptionWithRelated.ID,
				ProviderResponseID: "resp-1",
				Prompt:             "test prompt",
				CreatedAt:          afterThreshold,
			})

			_ = dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
				ID:                 uuid.New(),
				InterceptionID:     oldInterceptionWithRelated.ID,
				ProviderResponseID: "resp-1",
				Tool:               "test-tool",
				ServerUrl:          sql.NullString{String: "http://test", Valid: true},
				Input:              "{}",
				Injected:           true,
				CreatedAt:          afterThreshold,
			})

			// Create recent AI Bridge interception (should be kept).
			recentInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.New(),
				APIKeyID:    sql.NullString{},
				InitiatorID: user.ID,
				Provider:    "anthropic",
				Model:       "claude-3-5-sonnet",
				StartedAt:   beforeThreshold,
			}, &beforeThreshold)

			// Create interception close to threshold (should be kept).
			nearThresholdInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.New(),
				APIKeyID:    sql.NullString{},
				InitiatorID: user.ID,
				Provider:    "anthropic",
				Model:       "claude-3-5-sonnet",
				StartedAt:   closeBeforeThreshold,
			}, &closeBeforeThreshold)

			_ = dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				ID:                 uuid.New(),
				InterceptionID:     nearThresholdInterception.ID,
				ProviderResponseID: "resp-1",
				InputTokens:        100,
				OutputTokens:       50,
				CreatedAt:          closeBeforeThreshold,
			})

			_ = dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
				ID:                 uuid.New(),
				InterceptionID:     nearThresholdInterception.ID,
				ProviderResponseID: "resp-1",
				Prompt:             "test prompt",
				CreatedAt:          closeBeforeThreshold,
			})

			_ = dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
				ID:                 uuid.New(),
				InterceptionID:     nearThresholdInterception.ID,
				ProviderResponseID: "resp-1",
				Tool:               "test-tool",
				ServerUrl:          sql.NullString{String: "http://test", Valid: true},
				Input:              "{}",
				Injected:           true,
				CreatedAt:          closeBeforeThreshold,
			})

			fixtures := testFixtures{
				oldInterception:            oldInterception,
				oldInterceptionWithRelated: oldInterceptionWithRelated,
				recentInterception:         recentInterception,
				nearThresholdInterception:  nearThresholdInterception,
			}

			// Run the purge with configured retention period.
			done := awaitDoTick(ctx, t, clk)
			closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
				AI: codersdk.AIConfig{
					BridgeConfig: codersdk.AIBridgeConfig{
						Retention: serpent.Duration(tc.retention),
					},
				},
			}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
			defer closer.Close()
			testutil.TryReceive(ctx, t, done)

			tc.verify(t, ctx, db, fixtures)
		})
	}
}

func TestDeleteOldAuditLogs(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
	retentionPeriod := 30 * 24 * time.Hour
	afterThreshold := now.Add(-retentionPeriod).Add(-24 * time.Hour) // 31 days ago (older than threshold)
	beforeThreshold := now.Add(-15 * 24 * time.Hour)                 // 15 days ago (newer than threshold)

	testCases := []struct {
		name                  string
		retentionConfig       codersdk.RetentionConfig
		oldLogTime            time.Time
		recentLogTime         *time.Time // nil means no recent log created
		expectOldDeleted      bool
		expectedLogsRemaining int
	}{
		{
			name: "RetentionEnabled",
			retentionConfig: codersdk.RetentionConfig{
				AuditLogs: serpent.Duration(retentionPeriod),
			},
			oldLogTime:            afterThreshold,
			recentLogTime:         &beforeThreshold,
			expectOldDeleted:      true,
			expectedLogsRemaining: 1, // only recent log remains
		},
		{
			name: "RetentionDisabled",
			retentionConfig: codersdk.RetentionConfig{
				AuditLogs: serpent.Duration(0),
			},
			oldLogTime:            now.Add(-365 * 24 * time.Hour), // 1 year ago
			recentLogTime:         nil,
			expectOldDeleted:      false,
			expectedLogsRemaining: 1, // old log is kept
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			clk := quartz.NewMock(t)
			clk.Set(now).MustWait(ctx)

			db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			// Setup test fixtures.
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})

			// Create old audit log.
			oldLog := dbgen.AuditLog(t, db, database.AuditLog{
				UserID:         user.ID,
				OrganizationID: org.ID,
				Time:           tc.oldLogTime,
				Action:         database.AuditActionCreate,
				ResourceType:   database.ResourceTypeWorkspace,
			})

			// Create recent audit log if specified.
			var recentLog database.AuditLog
			if tc.recentLogTime != nil {
				recentLog = dbgen.AuditLog(t, db, database.AuditLog{
					UserID:         user.ID,
					OrganizationID: org.ID,
					Time:           *tc.recentLogTime,
					Action:         database.AuditActionCreate,
					ResourceType:   database.ResourceTypeWorkspace,
				})
			}

			// Run the purge.
			done := awaitDoTick(ctx, t, clk)
			closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
				Retention: tc.retentionConfig,
			}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
			defer closer.Close()
			testutil.TryReceive(ctx, t, done)

			// Verify results.
			logs, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
				LimitOpt: 100,
			})
			require.NoError(t, err)
			require.Len(t, logs, tc.expectedLogsRemaining, "unexpected number of logs remaining")

			logIDs := make([]uuid.UUID, len(logs))
			for i, log := range logs {
				logIDs[i] = log.AuditLog.ID
			}

			if tc.expectOldDeleted {
				require.NotContains(t, logIDs, oldLog.ID, "old audit log should be deleted")
			} else {
				require.Contains(t, logIDs, oldLog.ID, "old audit log should NOT be deleted")
			}

			if tc.recentLogTime != nil {
				require.Contains(t, logIDs, recentLog.ID, "recent audit log should be kept")
			}
		})
	}

	// ConnectionEventsNotDeleted is a special case that tests multiple audit
	// action types, so it's kept as a separate subtest.
	t.Run("ConnectionEventsNotDeleted", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		clk := quartz.NewMock(t)
		clk.Set(now).MustWait(ctx)

		db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})

		// Create old connection events (should NOT be deleted by audit logs retention).
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

		// Create old non-connection audit log (should be deleted).
		oldCreateLog := dbgen.AuditLog(t, db, database.AuditLog{
			UserID:         user.ID,
			OrganizationID: org.ID,
			Time:           afterThreshold,
			Action:         database.AuditActionCreate,
			ResourceType:   database.ResourceTypeWorkspace,
		})

		// Run the purge with audit logs retention enabled.
		done := awaitDoTick(ctx, t, clk)
		closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
			Retention: codersdk.RetentionConfig{
				AuditLogs: serpent.Duration(retentionPeriod),
			},
		}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
		defer closer.Close()
		testutil.TryReceive(ctx, t, done)

		// Verify results.
		logs, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
			LimitOpt: 100,
		})
		require.NoError(t, err)
		require.Len(t, logs, 4, "should have 4 connection event logs remaining")

		logIDs := make([]uuid.UUID, len(logs))
		for i, log := range logs {
			logIDs[i] = log.AuditLog.ID
		}

		// Connection events should NOT be deleted by audit logs retention.
		require.Contains(t, logIDs, oldConnectLog.ID, "old connect log should NOT be deleted by audit logs retention")
		require.Contains(t, logIDs, oldDisconnectLog.ID, "old disconnect log should NOT be deleted by audit logs retention")
		require.Contains(t, logIDs, oldOpenLog.ID, "old open log should NOT be deleted by audit logs retention")
		require.Contains(t, logIDs, oldCloseLog.ID, "old close log should NOT be deleted by audit logs retention")

		// Non-connection event should be deleted.
		require.NotContains(t, logIDs, oldCreateLog.ID, "old create log should be deleted by audit logs retention")
	})
}

func TestDeleteExpiredAPIKeys(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)

	testCases := []struct {
		name                    string
		retentionConfig         codersdk.RetentionConfig
		oldExpiredTime          time.Time
		recentExpiredTime       *time.Time // nil means no recent expired key created
		activeTime              *time.Time // nil means no active key created
		expectOldExpiredDeleted bool
		expectedKeysRemaining   int
	}{
		{
			name: "RetentionEnabled",
			retentionConfig: codersdk.RetentionConfig{
				APIKeys: serpent.Duration(7 * 24 * time.Hour), // 7 days
			},
			oldExpiredTime:          now.Add(-8 * 24 * time.Hour),      // Expired 8 days ago
			recentExpiredTime:       ptr(now.Add(-6 * 24 * time.Hour)), // Expired 6 days ago
			activeTime:              ptr(now.Add(24 * time.Hour)),      // Expires tomorrow
			expectOldExpiredDeleted: true,
			expectedKeysRemaining:   2, // recent expired + active
		},
		{
			name: "RetentionDisabled",
			retentionConfig: codersdk.RetentionConfig{
				APIKeys: serpent.Duration(0),
			},
			oldExpiredTime:          now.Add(-365 * 24 * time.Hour), // Expired 1 year ago
			recentExpiredTime:       nil,
			activeTime:              nil,
			expectOldExpiredDeleted: false,
			expectedKeysRemaining:   1, // old expired is kept
		},

		{
			name: "CustomRetention30Days",
			retentionConfig: codersdk.RetentionConfig{
				APIKeys: serpent.Duration(30 * 24 * time.Hour), // 30 days
			},
			oldExpiredTime:          now.Add(-31 * 24 * time.Hour),      // Expired 31 days ago
			recentExpiredTime:       ptr(now.Add(-29 * 24 * time.Hour)), // Expired 29 days ago
			activeTime:              nil,
			expectOldExpiredDeleted: true,
			expectedKeysRemaining:   1, // only recent expired remains
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			clk := quartz.NewMock(t)
			clk.Set(now).MustWait(ctx)

			db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			user := dbgen.User(t, db, database.User{})

			// Create API key that expired long ago.
			oldExpiredKey, _ := dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: tc.oldExpiredTime,
				TokenName: "old-expired-key",
			})

			// Create API key that expired recently if specified.
			var recentExpiredKey database.APIKey
			if tc.recentExpiredTime != nil {
				recentExpiredKey, _ = dbgen.APIKey(t, db, database.APIKey{
					UserID:    user.ID,
					ExpiresAt: *tc.recentExpiredTime,
					TokenName: "recent-expired-key",
				})
			}

			// Create API key that hasn't expired yet if specified.
			var activeKey database.APIKey
			if tc.activeTime != nil {
				activeKey, _ = dbgen.APIKey(t, db, database.APIKey{
					UserID:    user.ID,
					ExpiresAt: *tc.activeTime,
					TokenName: "active-key",
				})
			}

			// Run the purge.
			done := awaitDoTick(ctx, t, clk)
			closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{
				Retention: tc.retentionConfig,
			}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
			defer closer.Close()
			testutil.TryReceive(ctx, t, done)

			// Verify total keys remaining.
			keys, err := db.GetAPIKeysLastUsedAfter(ctx, time.Time{})
			require.NoError(t, err)
			require.Len(t, keys, tc.expectedKeysRemaining, "unexpected number of keys remaining")

			// Verify results.
			_, err = db.GetAPIKeyByID(ctx, oldExpiredKey.ID)
			if tc.expectOldExpiredDeleted {
				require.Error(t, err, "old expired key should be deleted")
			} else {
				require.NoError(t, err, "old expired key should NOT be deleted")
			}

			if tc.recentExpiredTime != nil {
				_, err = db.GetAPIKeyByID(ctx, recentExpiredKey.ID)
				require.NoError(t, err, "recently expired key should be kept")
			}

			if tc.activeTime != nil {
				_, err = db.GetAPIKeyByID(ctx, activeKey.ID)
				require.NoError(t, err, "active key should be kept")
			}
		})
	}
}

// ptr is a helper to create a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}

// nopAuditorPtr returns an atomic pointer to a nop auditor for tests.
func nopAuditorPtr(t *testing.T) *atomic.Pointer[audit.Auditor] {
	t.Helper()
	nop := audit.NewNop()
	var p atomic.Pointer[audit.Auditor]
	p.Store(&nop)
	return &p
}

// mockAuditorPtr wraps a *MockAuditor in an atomic pointer for tests.
func mockAuditorPtr(m *audit.MockAuditor) *atomic.Pointer[audit.Auditor] {
	a := audit.Auditor(m)
	var p atomic.Pointer[audit.Auditor]
	p.Store(&a)
	return &p
}

//nolint:paralleltest // It uses LockIDDBPurge.
func TestDeleteOldChatFiles(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	// createChatFile inserts a chat file and backdates created_at.
	createChatFile := func(ctx context.Context, t *testing.T, db database.Store, rawDB *sql.DB, ownerID, orgID uuid.UUID, createdAt time.Time) uuid.UUID {
		t.Helper()
		row, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
			OwnerID:        ownerID,
			OrganizationID: orgID,
			Name:           "test.png",
			Mimetype:       "image/png",
			Data:           []byte("fake-image-data"),
		})
		require.NoError(t, err)
		_, err = rawDB.ExecContext(ctx, "UPDATE chat_files SET created_at = $1 WHERE id = $2", createdAt, row.ID)
		require.NoError(t, err)
		return row.ID
	}

	// createChat inserts a chat and optionally archives it, then
	// backdates updated_at to control the "archived since" window.
	createChat := func(ctx context.Context, t *testing.T, db database.Store, rawDB *sql.DB, ownerID, orgID, modelConfigID uuid.UUID, archived bool, updatedAt time.Time) database.Chat {
		t.Helper()
		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    orgID,
			OwnerID:           ownerID,
			LastModelConfigID: modelConfigID,
			Title:             "test-chat",
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
		})
		require.NoError(t, err)
		if archived {
			_, err = db.ArchiveChatByID(ctx, chat.ID)
			require.NoError(t, err)
		}
		_, err = rawDB.ExecContext(ctx, "UPDATE chats SET updated_at = $1 WHERE id = $2", updatedAt, chat.ID)
		require.NoError(t, err)
		return chat
	}
	// setupChatDeps creates the common dependencies needed for
	// chat-related tests: user, org, org member, provider, model config.
	type chatDeps struct {
		user        database.User
		org         database.Organization
		modelConfig database.ChatModelConfig
	}
	setupChatDeps := func(ctx context.Context, t *testing.T, db database.Store) chatDeps {
		t.Helper()
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
		_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
			Provider:             "openai",
			DisplayName:          "OpenAI",
			Enabled:              true,
			CentralApiKeyEnabled: true,
		})
		require.NoError(t, err)
		mc, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
			Provider:     "openai",
			Model:        "test-model",
			ContextLimit: 8192,
			Options:      json.RawMessage("{}"),
		})
		require.NoError(t, err)
		return chatDeps{user: user, org: org, modelConfig: mc}
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "ChatRetentionDisabled",
			run: func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				clk := quartz.NewMock(t)
				clk.Set(now).MustWait(ctx)

				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				deps := setupChatDeps(ctx, t, db)

				// Disable retention.
				err := db.UpsertChatRetentionDays(ctx, int32(0))
				require.NoError(t, err)

				// Create an old archived chat and an orphaned old file.
				oldChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-31*24*time.Hour))
				oldFileID := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))

				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				// Both should still exist.
				_, err = db.GetChatByID(ctx, oldChat.ID)
				require.NoError(t, err, "chat should not be deleted when retention is disabled")
				_, err = db.GetChatFileByID(ctx, oldFileID)
				require.NoError(t, err, "chat file should not be deleted when retention is disabled")
			},
		},
		{
			name: "OldArchivedChatsDeleted",
			run: func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				clk := quartz.NewMock(t)
				clk.Set(now).MustWait(ctx)

				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				deps := setupChatDeps(ctx, t, db)

				err := db.UpsertChatRetentionDays(ctx, int32(30))
				require.NoError(t, err)

				// Old archived chat (31 days) — should be deleted.
				oldChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-31*24*time.Hour))
				// Insert a message so we can verify CASCADE.
				_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
					ChatID:              oldChat.ID,
					CreatedBy:           []uuid.UUID{deps.user.ID},
					ModelConfigID:       []uuid.UUID{deps.modelConfig.ID},
					Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
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
					ProviderResponseID:  []string{""},
				})
				require.NoError(t, err)

				// Recently archived chat (10 days) — should be retained.
				recentChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-10*24*time.Hour))

				// Active chat — should be retained.
				activeChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, false, now)

				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				// Old archived chat should be gone.
				_, err = db.GetChatByID(ctx, oldChat.ID)
				require.Error(t, err, "old archived chat should be deleted")

				// Its messages should be gone too (CASCADE).
				msgs, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
					ChatID:  oldChat.ID,
					AfterID: 0,
				})
				require.NoError(t, err)
				require.Empty(t, msgs, "messages should be cascade-deleted")

				// Recent archived and active chats should remain.
				_, err = db.GetChatByID(ctx, recentChat.ID)
				require.NoError(t, err, "recently archived chat should be retained")
				_, err = db.GetChatByID(ctx, activeChat.ID)
				require.NoError(t, err, "active chat should be retained")
			},
		},
		{
			name: "OrphanedOldFilesDeleted",
			run: func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				clk := quartz.NewMock(t)
				clk.Set(now).MustWait(ctx)

				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				deps := setupChatDeps(ctx, t, db)

				err := db.UpsertChatRetentionDays(ctx, int32(30))
				require.NoError(t, err)

				// File A: 31 days old, NOT in any chat -> should be deleted.
				fileA := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))

				// File B: 31 days old, in an active chat -> should be retained.
				fileB := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))
				activeChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, false, now)
				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       activeChat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{fileB},
				})
				require.NoError(t, err)

				// File C: 10 days old, NOT in any chat -> should be retained (too young).
				fileC := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-10*24*time.Hour))

				// File near boundary: 29d23h old — close to threshold.
				fileBoundary := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-30*24*time.Hour).Add(time.Hour))

				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				_, err = db.GetChatFileByID(ctx, fileA)
				require.Error(t, err, "orphaned old file A should be deleted")

				_, err = db.GetChatFileByID(ctx, fileB)
				require.NoError(t, err, "file B in active chat should be retained")

				_, err = db.GetChatFileByID(ctx, fileC)
				require.NoError(t, err, "young file C should be retained")

				_, err = db.GetChatFileByID(ctx, fileBoundary)
				require.NoError(t, err, "file near 30d boundary should be retained")
			},
		},
		{
			name: "ArchivedChatFilesDeleted",
			run: func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				clk := quartz.NewMock(t)
				clk.Set(now).MustWait(ctx)

				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				deps := setupChatDeps(ctx, t, db)

				err := db.UpsertChatRetentionDays(ctx, int32(30))
				require.NoError(t, err)

				// File D: 31 days old, in a chat archived 31 days ago -> should be deleted.
				fileD := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))
				oldArchivedChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-31*24*time.Hour))
				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       oldArchivedChat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{fileD},
				})
				require.NoError(t, err)
				// LinkChatFiles does not update chats.updated_at, so backdate.
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET updated_at = $1 WHERE id = $2",
					now.Add(-31*24*time.Hour), oldArchivedChat.ID)
				require.NoError(t, err)

				// File E: 31 days old, in a chat archived 10 days ago -> should be retained.
				fileE := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))
				recentArchivedChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-10*24*time.Hour))
				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       recentArchivedChat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{fileE},
				})
				require.NoError(t, err)
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET updated_at = $1 WHERE id = $2",
					now.Add(-10*24*time.Hour), recentArchivedChat.ID)
				require.NoError(t, err)

				// File F: 31 days old, in BOTH an active chat AND an old archived chat -> should be retained.
				fileF := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))
				anotherOldArchivedChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-31*24*time.Hour))
				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       anotherOldArchivedChat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{fileF},
				})
				require.NoError(t, err)
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET updated_at = $1 WHERE id = $2",
					now.Add(-31*24*time.Hour), anotherOldArchivedChat.ID)
				require.NoError(t, err)

				activeChatForF := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, false, now)
				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       activeChatForF.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{fileF},
				})
				require.NoError(t, err)

				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				_, err = db.GetChatFileByID(ctx, fileD)
				require.Error(t, err, "file D in old archived chat should be deleted")

				_, err = db.GetChatFileByID(ctx, fileE)
				require.NoError(t, err, "file E in recently archived chat should be retained")

				_, err = db.GetChatFileByID(ctx, fileF)
				require.NoError(t, err, "file F in active + old archived chat should be retained")
			},
		},
		{
			name: "UnarchiveAfterFilePurge",
			run: func(t *testing.T) {
				// Validates that when dbpurge deletes chat_files rows,
				// the FK cascade on chat_file_links automatically
				// removes the stale links. Unarchiving a chat after
				// file purge should show only surviving files.
				ctx := testutil.Context(t, testutil.WaitLong)
				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				deps := setupChatDeps(ctx, t, db)

				// Create a chat with three attached files.
				fileA := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)
				fileB := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)
				fileC := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)

				chat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, false, now)
				_, err := db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       chat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{fileA, fileB, fileC},
				})
				require.NoError(t, err)

				// Archive the chat.
				_, err = db.ArchiveChatByID(ctx, chat.ID)
				require.NoError(t, err)

				// Simulate dbpurge deleting files A and B. The FK
				// cascade on chat_file_links_file_id_fkey should
				// automatically remove the corresponding link rows.
				_, err = rawDB.ExecContext(ctx, "DELETE FROM chat_files WHERE id = ANY($1)", pq.Array([]uuid.UUID{fileA, fileB}))
				require.NoError(t, err)

				// Unarchive the chat.
				_, err = db.UnarchiveChatByID(ctx, chat.ID)
				require.NoError(t, err)

				// Only file C should remain linked (FK cascade
				// removed the links for deleted files A and B).
				files, err := db.GetChatFileMetadataByChatID(ctx, chat.ID)
				require.NoError(t, err)
				require.Len(t, files, 1, "only surviving file should be linked")
				require.Equal(t, fileC, files[0].ID)

				// Edge case: delete the last file too. The chat
				// should have zero linked files, not an error.
				_, err = db.ArchiveChatByID(ctx, chat.ID)
				require.NoError(t, err)
				_, err = rawDB.ExecContext(ctx, "DELETE FROM chat_files WHERE id = $1", fileC)
				require.NoError(t, err)
				_, err = db.UnarchiveChatByID(ctx, chat.ID)
				require.NoError(t, err)

				files, err = db.GetChatFileMetadataByChatID(ctx, chat.ID)
				require.NoError(t, err)
				require.Empty(t, files, "all-files-deleted should yield empty result")

				// Test parent+child cascade: deleting files should
				// clean up links for both parent and child chats
				// independently via FK cascade.
				parentChat := createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, false, now)
				childChat, err := db.InsertChat(ctx, database.InsertChatParams{
					OrganizationID:    deps.org.ID,
					OwnerID:           deps.user.ID,
					LastModelConfigID: deps.modelConfig.ID,
					Title:             "child-chat",
					Status:            database.ChatStatusWaiting,
					ClientType:        database.ChatClientTypeUi,
				})
				require.NoError(t, err)

				// Set root_chat_id to link child to parent.
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET root_chat_id = $1 WHERE id = $2", parentChat.ID, childChat.ID)
				require.NoError(t, err)

				// Attach different files to parent and child.
				parentFileKeep := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)
				parentFileStale := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)
				childFileKeep := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)
				childFileStale := createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now)

				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       parentChat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{parentFileKeep, parentFileStale},
				})
				require.NoError(t, err)
				_, err = db.LinkChatFiles(ctx, database.LinkChatFilesParams{
					ChatID:       childChat.ID,
					MaxFileLinks: 100,
					FileIds:      []uuid.UUID{childFileKeep, childFileStale},
				})
				require.NoError(t, err)

				// Archive via parent (cascades to child).
				_, err = db.ArchiveChatByID(ctx, parentChat.ID)
				require.NoError(t, err)

				// Delete one file from each chat.
				_, err = rawDB.ExecContext(ctx, "DELETE FROM chat_files WHERE id = ANY($1)",
					pq.Array([]uuid.UUID{parentFileStale, childFileStale}))
				require.NoError(t, err)

				// Unarchive via parent.
				_, err = db.UnarchiveChatByID(ctx, parentChat.ID)
				require.NoError(t, err)

				parentFiles, err := db.GetChatFileMetadataByChatID(ctx, parentChat.ID)
				require.NoError(t, err)
				require.Len(t, parentFiles, 1)
				require.Equal(t, parentFileKeep, parentFiles[0].ID,
					"parent should retain only non-stale file")

				childFiles, err := db.GetChatFileMetadataByChatID(ctx, childChat.ID)
				require.NoError(t, err)
				require.Len(t, childFiles, 1)
				require.Equal(t, childFileKeep, childFiles[0].ID,
					"child should retain only non-stale file")
			},
		},
		{
			name: "BatchLimitFiles",
			run: func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				deps := setupChatDeps(ctx, t, db)

				// Create 3 deletable orphaned files (all 31 days old).
				for range 3 {
					createChatFile(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, now.Add(-31*24*time.Hour))
				}

				// Delete with limit 2 — should delete 2, leave 1.
				deleted, err := db.DeleteOldChatFiles(ctx, database.DeleteOldChatFilesParams{
					BeforeTime: now.Add(-30 * 24 * time.Hour),
					LimitCount: 2,
				})
				require.NoError(t, err)
				require.Equal(t, int64(2), deleted, "should delete exactly 2 files")

				// Delete again — should delete the remaining 1.
				deleted, err = db.DeleteOldChatFiles(ctx, database.DeleteOldChatFilesParams{
					BeforeTime: now.Add(-30 * 24 * time.Hour),
					LimitCount: 2,
				})
				require.NoError(t, err)
				require.Equal(t, int64(1), deleted, "should delete remaining 1 file")
			},
		},
		{
			name: "BatchLimitChats",
			run: func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
				deps := setupChatDeps(ctx, t, db)

				// Create 3 deletable old archived chats.
				for range 3 {
					createChat(ctx, t, db, rawDB, deps.user.ID, deps.org.ID, deps.modelConfig.ID, true, now.Add(-31*24*time.Hour))
				}

				// Delete with limit 2 — should delete 2, leave 1.
				deleted, err := db.DeleteOldChats(ctx, database.DeleteOldChatsParams{
					BeforeTime: now.Add(-30 * 24 * time.Hour),
					LimitCount: 2,
				})
				require.NoError(t, err)
				require.Equal(t, int64(2), deleted, "should delete exactly 2 chats")

				// Delete again — should delete the remaining 1.
				deleted, err = db.DeleteOldChats(ctx, database.DeleteOldChatsParams{
					BeforeTime: now.Add(-30 * 24 * time.Hour),
					LimitCount: 2,
				})
				require.NoError(t, err)
				require.Equal(t, int64(1), deleted, "should delete remaining 1 chat")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

// helpers for TestAutoArchiveInactiveChats. Kept scoped to the
// test so they don't leak into the package surface area.
func archiveTestDeps(ctx context.Context, t *testing.T, db database.Store) chatAutoArchiveDeps {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)
	mc, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:     "openai",
		Model:        "test-model",
		ContextLimit: 8192,
		Options:      json.RawMessage("{}"),
	})
	require.NoError(t, err)
	return chatAutoArchiveDeps{user: user, org: org, modelConfig: mc}
}

type chatAutoArchiveDeps struct {
	user        database.User
	org         database.Organization
	modelConfig database.ChatModelConfig
}

// archiveHarness bundles the per-subtest setup shared by every
// TestAutoArchiveInactiveChats case. Subtests read fields off the
// harness directly instead of repeating six lines of identical
// plumbing.
type archiveHarness struct {
	ctx    context.Context
	clk    *quartz.Mock
	db     database.Store
	rawDB  *sql.DB
	logger slog.Logger
	deps   chatAutoArchiveDeps
}

func newArchiveHarness(t *testing.T, now time.Time) *archiveHarness {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	clk := quartz.NewMock(t)
	clk.Set(now).MustWait(ctx)
	db, _, rawDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	return &archiveHarness{
		ctx:    ctx,
		clk:    clk,
		db:     db,
		rawDB:  rawDB,
		logger: logger,
		deps:   archiveTestDeps(ctx, t, db),
	}
}

// createArchiveChat inserts a chat with an optional backdated
// created_at. Title is propagated through so tests can assert on
// digest contents.
func createArchiveChat(ctx context.Context, t *testing.T, db database.Store, rawDB *sql.DB, deps chatAutoArchiveDeps, title string, createdAt time.Time) database.Chat {
	t.Helper()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    deps.org.ID,
		OwnerID:           deps.user.ID,
		LastModelConfigID: deps.modelConfig.ID,
		Title:             title,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
	})
	require.NoError(t, err)
	_, err = rawDB.ExecContext(ctx, "UPDATE chats SET created_at = $1, updated_at = $1 WHERE id = $2", createdAt, chat.ID)
	require.NoError(t, err)
	return chat
}

// insertTextMessage appends a non-deleted user message with a
// backdated created_at. Used to establish "last activity" for the
// auto-archive query's LATERAL subquery.
func insertTextMessage(ctx context.Context, t *testing.T, db database.Store, rawDB *sql.DB, chatID, userID, modelConfigID uuid.UUID, createdAt time.Time) {
	t.Helper()
	msgs, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{userID},
		ModelConfigID:       []uuid.UUID{modelConfigID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
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
		ProviderResponseID:  []string{""},
	})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	_, err = rawDB.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1 WHERE id = $2", createdAt, msgs[0].ID)
	require.NoError(t, err)
}

//nolint:paralleltest // It uses LockIDDBPurge.
func TestAutoArchiveInactiveChats(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "AutoArchiveDisabled",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.Zero(t, codersdk.DefaultChatAutoArchiveDays)
				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, codersdk.DefaultChatAutoArchiveDays))

				// Chat older than any reasonable cutoff.
				staleChat := createArchiveChat(ctx, t, db, rawDB, deps, "stale-chat", now.Add(-365*24*time.Hour))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				// Not archived, no audits, no digests.
				refreshed, err := db.GetChatByID(ctx, staleChat.ID)
				require.NoError(t, err)
				require.False(t, refreshed.Archived, "chat should stay active when auto-archive is disabled")

				require.Empty(t, auditor.AuditLogs(), "no audit log entries expected")
				require.Empty(t, enqueuer.Sent(), "no digest notifications expected")
			},
		},
		{
			name: "ArchivesInactiveRoot",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				// Regression guard: ensure that both auto-archive and retention
				// are both set to a distinct non-zero value.
				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(90)))
				require.NoError(t, db.UpsertChatRetentionDays(ctx, int32(30)))

				// Inactive root: newest message 100 days old.
				staleChat := createArchiveChat(ctx, t, db, rawDB, deps, "stale-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, staleChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-100*24*time.Hour))

				// Active root: message 10 days old, within cutoff.
				activeChat := createArchiveChat(ctx, t, db, rawDB, deps, "active-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, activeChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-10*24*time.Hour))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				refreshedStale, err := db.GetChatByID(ctx, staleChat.ID)
				require.NoError(t, err)
				require.True(t, refreshedStale.Archived, "stale chat should be auto-archived")

				refreshedActive, err := db.GetChatByID(ctx, activeChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedActive.Archived, "active chat should stay live")

				// Exactly one audit entry, for the stale root.
				logs := auditor.AuditLogs()
				require.Len(t, logs, 1, "expected one audit entry")
				require.Equal(t, staleChat.ID, logs[0].ResourceID)
				require.Equal(t, database.ResourceTypeChat, logs[0].ResourceType)
				require.Equal(t, database.AuditActionWrite, logs[0].Action)
				require.Contains(t, string(logs[0].AdditionalFields), "chat_auto_archive",
					"audit entry must carry the auto-archive subsystem tag")

				// Exactly one digest, addressed to the owner.
				sent := enqueuer.Sent()
				require.Len(t, sent, 1, "expected one digest notification")
				require.Equal(t, notifications.TemplateChatAutoArchiveDigest, sent[0].TemplateID)
				require.Equal(t, deps.user.ID, sent[0].UserID)
				// Ensure that config-derived fields flow through to payload.
				require.Equal(t, "90", sent[0].Data["auto_archive_days"])
				require.Equal(t, "30", sent[0].Data["retention_days"])
			},
		},
		{
			name: "ExactCutoffBoundary",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(90)))
				// The forced initial tick uses start = now. Compute
				// the cutoff from that tick's perspective so the
				// boundary is deterministic.
				cutoff := now.Add(-90 * 24 * time.Hour)

				// Message exactly at the cutoff: query uses strict <,
				// so this chat must survive.
				exactChat := createArchiveChat(ctx, t, db, rawDB, deps, "exact", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, exactChat.ID, deps.user.ID, deps.modelConfig.ID, cutoff)

				// Message one second before the cutoff: should be archived.
				justOverChat := createArchiveChat(ctx, t, db, rawDB, deps, "just-over", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, justOverChat.ID, deps.user.ID, deps.modelConfig.ID, cutoff.Add(-time.Second))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				// Use newTickDriver for precise tick control so we
				// observe the forced initial tick's results without
				// racing with a second tick.
				driver := newTickDriver(t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithClock(clk))
				// Defer driver.close() after closer.Close(): defers
				// run LIFO, so driver cleanup frees shutdown's
				// ticker.Stop() before the dbpurge goroutine blocks
				// on it.
				defer closer.Close()
				defer driver.close()
				driver.awaitInitial(ctx, t)

				refreshedExact, err := db.GetChatByID(ctx, exactChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedExact.Archived, "chat at exact cutoff must survive (strict <)")

				refreshedOver, err := db.GetChatByID(ctx, justOverChat.ID)
				require.NoError(t, err)
				require.True(t, refreshedOver.Archived, "chat one second past cutoff must be archived")

				require.Len(t, auditor.AuditLogs(), 1, "only the just-over chat should produce an audit entry")
			},
		},
		{
			name: "DeletedMessagesIgnored",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(90)))

				// Chat created 120 days ago with a recent message
				// (10 days old) that is then soft-deleted. The
				// LATERAL subquery filters cm.deleted = false, so
				// the chat should fall back to created_at and be
				// archived.
				chat := createArchiveChat(ctx, t, db, rawDB, deps, "deleted-msg", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, chat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-10*24*time.Hour))
				// Soft-delete all messages on this chat.
				_, err := rawDB.ExecContext(ctx, "UPDATE chat_messages SET deleted = true WHERE chat_id = $1", chat.ID)
				require.NoError(t, err)

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				refreshed, err := db.GetChatByID(ctx, chat.ID)
				require.NoError(t, err)
				require.True(t, refreshed.Archived, "chat with only deleted messages should be archived")
				require.Len(t, auditor.AuditLogs(), 1)
			},
		},
		{
			name: "ChildActivityKeepsRootAlive",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(90)))

				// Stale root with no messages of its own.
				root := createArchiveChat(ctx, t, db, rawDB, deps, "stale-root", now.Add(-120*24*time.Hour))

				// Child linked to root with a recent message (10 days old,
				// well within the 90-day cutoff).
				child := createArchiveChat(ctx, t, db, rawDB, deps, "active-child", now.Add(-120*24*time.Hour))
				_, err := rawDB.ExecContext(ctx, "UPDATE chats SET parent_chat_id = $1, root_chat_id = $1 WHERE id = $2", root.ID, child.ID)
				require.NoError(t, err)
				insertTextMessage(ctx, t, db, rawDB, child.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-10*24*time.Hour))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				refreshedRoot, err := db.GetChatByID(ctx, root.ID)
				require.NoError(t, err)
				require.False(t, refreshedRoot.Archived, "root must stay active because child has recent activity")

				refreshedChild, err := db.GetChatByID(ctx, child.ID)
				require.NoError(t, err)
				require.False(t, refreshedChild.Archived, "child must stay active")

				require.Empty(t, auditor.AuditLogs(), "no chats should be archived")
				require.Empty(t, enqueuer.Sent(), "no notifications should be sent")
			},
		},
		{
			name: "SkipsActiveStatusChats",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(90)))

				// Stale chats whose status prevents archiving.
				runningChat := createArchiveChat(ctx, t, db, rawDB, deps, "running-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, runningChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-100*24*time.Hour))
				_, err := rawDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", database.ChatStatusRunning, runningChat.ID)
				require.NoError(t, err)

				requiresActionChat := createArchiveChat(ctx, t, db, rawDB, deps, "requires-action-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, requiresActionChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-100*24*time.Hour))
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", database.ChatStatusRequiresAction, requiresActionChat.ID)
				require.NoError(t, err)

				pendingChat := createArchiveChat(ctx, t, db, rawDB, deps, "pending-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, pendingChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-100*24*time.Hour))
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", database.ChatStatusPending, pendingChat.ID)
				require.NoError(t, err)

				pausedChat := createArchiveChat(ctx, t, db, rawDB, deps, "paused-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, pausedChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-100*24*time.Hour))
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", database.ChatStatusPaused, pausedChat.ID)
				require.NoError(t, err)

				// Control: a stale chat with archivable status that
				// should be archived.
				completedChat := createArchiveChat(ctx, t, db, rawDB, deps, "completed-chat", now.Add(-120*24*time.Hour))
				insertTextMessage(ctx, t, db, rawDB, completedChat.ID, deps.user.ID, deps.modelConfig.ID, now.Add(-100*24*time.Hour))
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", database.ChatStatusCompleted, completedChat.ID)
				require.NoError(t, err)

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				refreshedRunning, err := db.GetChatByID(ctx, runningChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedRunning.Archived, "running chat must not be archived")

				refreshedRA, err := db.GetChatByID(ctx, requiresActionChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedRA.Archived, "requires_action chat must not be archived")

				refreshedPending, err := db.GetChatByID(ctx, pendingChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedPending.Archived, "pending chat must not be archived")

				refreshedPaused, err := db.GetChatByID(ctx, pausedChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedPaused.Archived, "paused chat must not be archived")

				refreshedCompleted, err := db.GetChatByID(ctx, completedChat.ID)
				require.NoError(t, err)
				require.True(t, refreshedCompleted.Archived, "completed stale chat should be archived")

				logs := auditor.AuditLogs()
				require.Len(t, logs, 1, "only the completed chat should produce an audit entry")
				require.Equal(t, completedChat.ID, logs[0].ResourceID)

				// Assert number of sent notifications to catch dispatch regressions.
				sent := enqueuer.Sent()
				require.Len(t, sent, 1, "expected one digest notification for the completed chat")
				require.Equal(t, notifications.TemplateChatAutoArchiveDigest, sent[0].TemplateID)
				require.Equal(t, deps.user.ID, sent[0].UserID)
			},
		},
		{
			name: "SkipsPinnedAndChildren",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				// Pinned stale chat: should be skipped.
				pinnedChat := createArchiveChat(ctx, t, db, rawDB, deps, "pinned-chat", now.Add(-90*24*time.Hour))
				_, err := rawDB.ExecContext(ctx, "UPDATE chats SET pin_order = 1 WHERE id = $1", pinnedChat.ID)
				require.NoError(t, err)

				// Stale root with a child.
				root := createArchiveChat(ctx, t, db, rawDB, deps, "root-chat", now.Add(-90*24*time.Hour))
				child := createArchiveChat(ctx, t, db, rawDB, deps, "child-chat", now.Add(-90*24*time.Hour))
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET parent_chat_id = $1, root_chat_id = $1 WHERE id = $2", root.ID, child.ID)
				require.NoError(t, err)
				// Give the child an active status to prove the cascade is
				// status-blind by design. If someone adds a status filter
				// to the cascade CTE, this assertion will catch it.
				_, err = rawDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", database.ChatStatusRunning, child.ID)
				require.NoError(t, err)

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				refreshedPinned, err := db.GetChatByID(ctx, pinnedChat.ID)
				require.NoError(t, err)
				require.False(t, refreshedPinned.Archived, "pinned chat must be skipped")

				refreshedRoot, err := db.GetChatByID(ctx, root.ID)
				require.NoError(t, err)
				require.True(t, refreshedRoot.Archived, "root should be archived")

				refreshedChild, err := db.GetChatByID(ctx, child.ID)
				require.NoError(t, err)
				require.True(t, refreshedChild.Archived, "child should be cascade-archived")

				// One audit entry for the root; the cascaded child is
				// not audited individually.
				require.Len(t, auditor.AuditLogs(), 1)

				// Digest should list only the root (one row).
				sent := enqueuer.Sent()
				require.Len(t, sent, 1)
				data := sent[0].Data
				require.NotNil(t, data)
				chats, ok := data["archived_chats"].([]map[string]any)
				require.True(t, ok, "archived_chats should be []map[string]any")
				require.Len(t, chats, 1, "digest should only list the root")
				require.Equal(t, "root-chat", chats[0]["title"])
			},
		},
		{
			name: "DigestOverflowCap",
			run: func(t *testing.T) {
				// 27 inactive roots exceed chatAutoArchiveDigestMaxChats
				// (25). All 27 should archive, but the digest payload
				// lists at most 25 titles and surfaces the rest via
				// additional_archived_count so the template can render
				// "...and N more".
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				const total = 27
				for i := range total {
					createArchiveChat(ctx, t, db, rawDB, deps,
						fmt.Sprintf("stale-%02d", i),
						now.Add(-60*24*time.Hour))
				}

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				// All 27 roots archived (one audit each).
				require.Len(t, auditor.AuditLogs(), total)

				sent := enqueuer.Sent()
				require.Len(t, sent, 1, "one digest per owner")
				chats, ok := sent[0].Data["archived_chats"].([]map[string]any)
				require.True(t, ok, "archived_chats should be []map[string]any")
				require.Len(t, chats, 25, "digest caps titles at 25")
				require.Equal(t, "2", sent[0].Data["additional_archived_count"],
					"overflow count is total - cap")
				// Humanized timestamp is computed from LastActivityAt
				// and the tick-start time, not a static fixture, so we
				// only assert the suffix the humanizer emits.
				humanized, _ := chats[0]["last_activity_humanized"].(string)
				require.Contains(t, humanized, "ago",
					"last_activity_humanized should be a past relative time")
			},
		},
		{
			name: "MultipleOwners",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps
				user2 := dbgen.User(t, db, database.User{})
				_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user2.ID, OrganizationID: deps.org.ID})

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				// Two stale roots per owner, backdated well past
				// the 30-day cutoff.
				u1Deps := deps
				u2Deps := chatAutoArchiveDeps{user: user2, org: deps.org, modelConfig: deps.modelConfig}
				createArchiveChat(ctx, t, db, rawDB, u1Deps, "u1-a", now.Add(-60*24*time.Hour))
				createArchiveChat(ctx, t, db, rawDB, u1Deps, "u1-b", now.Add(-60*24*time.Hour))
				createArchiveChat(ctx, t, db, rawDB, u2Deps, "u2-a", now.Add(-60*24*time.Hour))
				createArchiveChat(ctx, t, db, rawDB, u2Deps, "u2-b", now.Add(-60*24*time.Hour))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				// Four audit rows, one per archived root, attributed
				// to the owning user so downstream consumers can
				// correlate per-owner activity.
				logs := auditor.AuditLogs()
				require.Len(t, logs, 4)
				auditsByUser := map[uuid.UUID]int{}
				for _, l := range logs {
					auditsByUser[l.UserID]++
				}
				require.Equal(t, 2, auditsByUser[deps.user.ID])
				require.Equal(t, 2, auditsByUser[user2.ID])

				// One digest per owner, each listing only that owner's
				// two chats.
				sent := enqueuer.Sent()
				require.Len(t, sent, 2, "expected one digest per owner")

				byUser := map[uuid.UUID][]string{}
				for _, s := range sent {
					require.Equal(t, notifications.TemplateChatAutoArchiveDigest, s.TemplateID)
					chats, ok := s.Data["archived_chats"].([]map[string]any)
					require.True(t, ok, "archived_chats should be []map[string]any")
					for _, c := range chats {
						title, _ := c["title"].(string)
						byUser[s.UserID] = append(byUser[s.UserID], title)
					}
				}
				require.Contains(t, byUser, deps.user.ID)
				require.Contains(t, byUser, user2.ID)
				slices.Sort(byUser[deps.user.ID])
				slices.Sort(byUser[user2.ID])
				require.Equal(t, []string{"u1-a", "u1-b"}, byUser[deps.user.ID])
				require.Equal(t, []string{"u2-a", "u2-b"}, byUser[user2.ID])
			},
		},
		{
			name: "SecondTickIdempotent",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				// Two stale roots seeded before the first tick.
				firstA := createArchiveChat(ctx, t, db, rawDB, deps, "first-a", now.Add(-60*24*time.Hour))
				firstB := createArchiveChat(ctx, t, db, rawDB, deps, "first-b", now.Add(-60*24*time.Hour))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				driver := newTickDriver(t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk))
				// Defer driver.close() after closer.Close(): defers
				// run LIFO, so this frees shutdown's ticker.Stop()
				// before the dbpurge goroutine blocks on it.
				defer closer.Close()
				defer driver.close()
				driver.awaitInitial(ctx, t)

				// Tick 1: both archived, one digest.
				require.Len(t, auditor.AuditLogs(), 2, "tick 1 audits")
				require.Len(t, enqueuer.Sent(), 1, "tick 1 digests")

				// Seed a third stale root between ticks so tick 2 has
				// genuine work and we can distinguish "ignored already
				// archived" from "ignored everything".
				third := createArchiveChat(ctx, t, db, rawDB, deps, "second-c", now.Add(-60*24*time.Hour))

				driver.awaitNext(ctx, t)

				// Tick 2: exactly one new audit + one new digest for
				// the third chat; tick 1's rows must not be re-archived.
				require.Len(t, auditor.AuditLogs(), 3, "tick 2 cumulative audits")
				sent := enqueuer.Sent()
				require.Len(t, sent, 2, "tick 2 cumulative digests")
				chats, ok := sent[1].Data["archived_chats"].([]map[string]any)
				require.True(t, ok, "archived_chats should be []map[string]any")
				require.Len(t, chats, 1, "tick 2 digest lists only the new chat")
				require.Equal(t, "second-c", chats[0]["title"])

				// First-tick chats stayed archived.
				for _, id := range []uuid.UUID{firstA.ID, firstB.ID, third.ID} {
					refreshed, err := db.GetChatByID(ctx, id)
					require.NoError(t, err)
					require.True(t, refreshed.Archived, "chat %s should remain archived", id)
				}
			},
		},
		{
			name: "BatchSizePagination",
			run: func(t *testing.T) {
				// With 27 stale roots and batch size 20, tick 1
				// archives 20, tick 2 archives the remaining 7, and
				// tick 3 archives none. We assert the dispatch side
				// effects (audits, digests) follow the same pattern:
				// dispatch only runs when rows > 0, so tick 3 emits
				// no new audits or digests.
				//
				// The two-digest count asserted here is a consequence
				// of the per-tick enqueue model, not a product
				// invariant. notification_messages dedupe does not
				// collapse these because each tick's payload differs.
				// If enqueue is ever restructured to one notification
				// per owner per day, this assertion changes with it.
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				const total = 27
				for i := range total {
					createArchiveChat(ctx, t, db, rawDB, deps,
						fmt.Sprintf("page-%02d", i),
						now.Add(-60*24*time.Hour))
				}

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)
				enqueuer := notificationstest.NewFakeEnqueuer()
				driver := newTickDriver(t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(enqueuer), dbpurge.WithClock(clk), dbpurge.WithChatAutoArchiveBatchSize(20))
				// Defer driver.close() after closer.Close() so trap
				// cleanup frees shutdown's ticker.Stop() before the
				// dbpurge goroutine blocks on it.
				defer closer.Close()
				defer driver.close()
				driver.awaitInitial(ctx, t)

				// Tick 1: first batch (20) archived.
				require.Len(t, auditor.AuditLogs(), 20, "tick 1 audits")
				sent := enqueuer.Sent()
				require.Len(t, sent, 1, "tick 1 digests")
				chats1, ok := sent[0].Data["archived_chats"].([]map[string]any)
				require.True(t, ok, "archived_chats should be []map[string]any")
				require.Len(t, chats1, 20, "tick 1 digest lists all 20 titles")
				require.NotContains(t, sent[0].Data, "additional_archived_count",
					"no overflow when batch <= digest cap; 20 <= 25")

				driver.awaitNext(ctx, t)

				// Tick 2: remaining 7 archived.
				require.Len(t, auditor.AuditLogs(), 27, "tick 2 cumulative audits")
				sent = enqueuer.Sent()
				require.Len(t, sent, 2, "tick 2 cumulative digests")
				chats2, ok := sent[1].Data["archived_chats"].([]map[string]any)
				require.True(t, ok, "archived_chats should be []map[string]any")
				require.Len(t, chats2, 7, "tick 2 digest lists remaining 7")

				driver.awaitNext(ctx, t)

				// Tick 3: nothing left to archive. The dispatch is
				// gated on len(archivedChats) > 0, so no new audits
				// or digests are produced. If that gate is ever
				// removed, update this assertion intentionally.
				require.Len(t, auditor.AuditLogs(), 27, "tick 3 cumulative audits unchanged")
				require.Len(t, enqueuer.Sent(), 2, "tick 3 cumulative digests unchanged")
			},
		},
		{
			name: "ShutdownCancelsDigestDispatch",
			run: func(t *testing.T) {
				// Two owners with one stale root each. The first
				// EnqueueWithData call blocks until ctx is canceled.
				// Closing the purger must propagate cancellation
				// into the in-flight call and short-circuit the
				// rest of the loop, so Close returns promptly
				// instead of hanging on dispatch.
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps
				user2 := dbgen.User(t, db, database.User{})
				_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user2.ID, OrganizationID: deps.org.ID})

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				u1Deps := deps
				u2Deps := chatAutoArchiveDeps{user: user2, org: deps.org, modelConfig: deps.modelConfig}
				createArchiveChat(ctx, t, db, rawDB, u1Deps, "u1-stale", now.Add(-60*24*time.Hour))
				createArchiveChat(ctx, t, db, rawDB, u2Deps, "u2-stale", now.Add(-60*24*time.Hour))

				// Dispatch iterates owner IDs in ascending UUID order (convention).
				expectedFirst := deps.user.ID
				if user2.ID.String() < deps.user.ID.String() {
					expectedFirst = user2.ID
				}

				ctrl := gomock.NewController(t)
				mockEnq := notificationsmock.NewMockEnqueuer(ctrl)
				started := make(chan struct{})
				mockEnq.EXPECT().EnqueueWithData(gomock.Any(), gomock.Eq(expectedFirst), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, _, _ uuid.UUID, _ map[string]string, _ map[string]any, _ string, _ ...uuid.UUID) ([]uuid.UUID, error) {
						close(started)
						<-ctx.Done()
						return nil, ctx.Err()
					}).Times(1)

				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), nopAuditorPtr(t), dbpurge.WithNotificationsEnqueuer(mockEnq), dbpurge.WithClock(clk))

				// Wait for the forced initial tick to reach the first
				// enqueue, which then blocks on ctx.Done().
				testutil.TryReceive(ctx, t, started)

				// Blocked enqueue receives ctx cancellation via the parent context.
				// Loop-head check abandons the remaining owner instead of trying to enqueue.
				done := make(chan error)
				go func() { done <- closer.Close() }()
				testutil.RequireReceive(ctx, t, done)
			},
		},
		{
			// A transient enqueue failure for one owner must not abort the dispatch loop.
			name: "TransientEnqueueFailureDoesNotAbortLoop",
			run: func(t *testing.T) {
				h := newArchiveHarness(t, now)
				ctx, clk, db, rawDB, logger, deps := h.ctx, h.clk, h.db, h.rawDB, h.logger, h.deps
				user2 := dbgen.User(t, db, database.User{})
				_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user2.ID, OrganizationID: deps.org.ID})

				require.NoError(t, db.UpsertChatAutoArchiveDays(ctx, int32(30)))

				u1Deps := deps
				u2Deps := chatAutoArchiveDeps{user: user2, org: deps.org, modelConfig: deps.modelConfig}
				createArchiveChat(ctx, t, db, rawDB, u1Deps, "u1-stale", now.Add(-60*24*time.Hour))
				createArchiveChat(ctx, t, db, rawDB, u2Deps, "u2-stale", now.Add(-60*24*time.Hour))

				auditor := audit.NewMock()
				auditorPtr := mockAuditorPtr(auditor)

				ctrl := gomock.NewController(t)
				mockEnq := notificationsmock.NewMockEnqueuer(ctrl)
				var calls atomic.Int32
				var successUserID uuid.UUID
				mockEnq.EXPECT().EnqueueWithData(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, userID, _ uuid.UUID, _ map[string]string, _ map[string]any, _ string, _ ...uuid.UUID) ([]uuid.UUID, error) {
						if calls.Add(1) == 1 {
							return nil, xerrors.New("simulated transient enqueue failure")
						}
						successUserID = userID
						return nil, nil
					}).Times(2)

				done := awaitDoTick(ctx, t, clk)
				closer := dbpurge.New(ctx, logger, db, &codersdk.DeploymentValues{}, prometheus.NewRegistry(), auditorPtr, dbpurge.WithNotificationsEnqueuer(mockEnq), dbpurge.WithClock(clk))
				defer closer.Close()
				testutil.TryReceive(ctx, t, done)

				// Both owners must have been audited regardless of
				// digest enqueue outcomes; the audit and digest
				// paths are independent.
				require.Len(t, auditor.AuditLogs(), 2, "both archived roots must be audited")

				// gomock's .Times(2) already enforces both calls
				// happened; this assertion makes the contract
				// explicit at the test site.
				require.Equal(t, int32(2), calls.Load(),
					"loop must attempt every owner even when one fails")

				// The second attempt succeeded for one of the two owners.
				require.Contains(t, []uuid.UUID{deps.user.ID, user2.ID}, successUserID,
					"successful digest must belong to one of the two owners")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}
