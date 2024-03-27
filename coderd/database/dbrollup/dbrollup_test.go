package dbrollup_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbrollup"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRollup_Close(t *testing.T) {
	t.Parallel()
	rolluper := dbrollup.New(slogtest.Make(t, nil), dbmem.New(), dbrollup.WithInterval(250*time.Millisecond))
	err := rolluper.Close()
	require.NoError(t, err)
}

type wrapUpsertDB struct {
	database.Store
	resume <-chan struct{}
}

func (w *wrapUpsertDB) InTx(fn func(database.Store) error, opts *sql.TxOptions) error {
	return w.Store.InTx(func(tx database.Store) error {
		return fn(&wrapUpsertDB{Store: tx, resume: w.resume})
	}, opts)
}

func (w *wrapUpsertDB) UpsertTemplateUsageStats(ctx context.Context) error {
	<-w.resume
	return w.Store.UpsertTemplateUsageStats(ctx)
}

func TestRollup_TwoInstancesUseLocking(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("Skipping test; only works with PostgreSQL.")
	}

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)

	var (
		org   = dbgen.Organization(t, db, database.Organization{})
		user  = dbgen.User(t, db, database.User{Name: "user1"})
		tpl   = dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
		ver   = dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, CreatedBy: user.ID})
		ws    = dbgen.Workspace(t, db, database.Workspace{OrganizationID: org.ID, TemplateID: tpl.ID, OwnerID: user.ID})
		job   = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID})
		build = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: job.ID, TemplateVersionID: ver.ID})
		res   = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
		agent = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
	)

	refTime := dbtime.Now().Truncate(time.Hour)
	_ = dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		TemplateID:                tpl.ID,
		WorkspaceID:               ws.ID,
		AgentID:                   agent.ID,
		UserID:                    user.ID,
		CreatedAt:                 refTime.Add(-time.Minute),
		ConnectionMedianLatencyMS: 1,
		ConnectionCount:           1,
		SessionCountSSH:           1,
	})

	closeRolluper := func(rolluper *dbrollup.Rolluper, resume chan struct{}) {
		close(resume)
		err := rolluper.Close()
		require.NoError(t, err)
	}

	interval := dbrollup.WithInterval(250 * time.Millisecond)
	events1 := make(chan dbrollup.Event)
	resume1 := make(chan struct{}, 1)
	rolluper1 := dbrollup.New(
		logger.Named("dbrollup1"),
		&wrapUpsertDB{Store: db, resume: resume1},
		interval,
		dbrollup.WithEventChannel(events1),
	)
	defer closeRolluper(rolluper1, resume1)

	events2 := make(chan dbrollup.Event)
	resume2 := make(chan struct{}, 1)
	rolluper2 := dbrollup.New(
		logger.Named("dbrollup2"),
		&wrapUpsertDB{Store: db, resume: resume2},
		interval,
		dbrollup.WithEventChannel(events2),
	)
	defer closeRolluper(rolluper2, resume2)

	_, _ = <-events1, <-events2 // Deplete init event, resume operation.

	ctx := testutil.Context(t, testutil.WaitMedium)

	// One of the rollup instances should roll up and the other should not.
	var ev1, ev2 dbrollup.Event
	select {
	case <-ctx.Done():
		t.Fatal("timed out waiting for rollup to occur")
	case ev1 = <-events1:
		resume2 <- struct{}{}
		ev2 = <-events2
	case ev2 = <-events2:
		resume1 <- struct{}{}
		ev1 = <-events1
	}

	require.NotEqual(t, ev1, ev2, "one of the rollup instances should have rolled up and the other not")

	rows, err := db.GetTemplateUsageStats(ctx, database.GetTemplateUsageStatsParams{
		StartTime: refTime.Add(-time.Hour).Truncate(time.Hour),
		EndTime:   refTime,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestRollupTemplateUsageStats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	anHourAgo := dbtime.Now().Add(-time.Hour).Truncate(time.Hour)
	anHourAndSixMonthsAgo := anHourAgo.AddDate(0, -6, 0)

	var (
		org   = dbgen.Organization(t, db, database.Organization{})
		user  = dbgen.User(t, db, database.User{Name: "user1"})
		tpl   = dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
		ver   = dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, CreatedBy: user.ID})
		ws    = dbgen.Workspace(t, db, database.Workspace{OrganizationID: org.ID, TemplateID: tpl.ID, OwnerID: user.ID})
		job   = dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{OrganizationID: org.ID})
		build = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: job.ID, TemplateVersionID: ver.ID})
		res   = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
		agent = dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})
		app   = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{AgentID: agent.ID})
	)

	// Stats inserted 6 months + 1 day ago, should be excluded.
	_ = dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		TemplateID:                tpl.ID,
		WorkspaceID:               ws.ID,
		AgentID:                   agent.ID,
		UserID:                    user.ID,
		CreatedAt:                 anHourAndSixMonthsAgo.AddDate(0, 0, -1),
		ConnectionMedianLatencyMS: 1,
		ConnectionCount:           1,
		SessionCountSSH:           1,
	})
	_ = dbgen.WorkspaceAppStat(t, db, database.WorkspaceAppStat{
		UserID:           user.ID,
		WorkspaceID:      ws.ID,
		AgentID:          agent.ID,
		SessionStartedAt: anHourAndSixMonthsAgo.AddDate(0, 0, -1),
		SessionEndedAt:   anHourAndSixMonthsAgo.AddDate(0, 0, -1).Add(time.Minute),
		SlugOrPort:       app.Slug,
	})

	// Stats inserted 6 months - 1 day ago, should be rolled up.
	wags1 := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		TemplateID:                  tpl.ID,
		WorkspaceID:                 ws.ID,
		AgentID:                     agent.ID,
		UserID:                      user.ID,
		CreatedAt:                   anHourAndSixMonthsAgo.AddDate(0, 0, 1),
		ConnectionMedianLatencyMS:   1,
		ConnectionCount:             1,
		SessionCountReconnectingPTY: 1,
	})
	wags2 := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		TemplateID:                  tpl.ID,
		WorkspaceID:                 ws.ID,
		AgentID:                     agent.ID,
		UserID:                      user.ID,
		CreatedAt:                   wags1.CreatedAt.Add(time.Minute),
		ConnectionMedianLatencyMS:   1,
		ConnectionCount:             1,
		SessionCountReconnectingPTY: 1,
	})
	// wags2 and waps1 overlap, so total usage is 4 - 1.
	waps1 := dbgen.WorkspaceAppStat(t, db, database.WorkspaceAppStat{
		UserID:           user.ID,
		WorkspaceID:      ws.ID,
		AgentID:          agent.ID,
		SessionStartedAt: wags2.CreatedAt,
		SessionEndedAt:   wags2.CreatedAt.Add(time.Minute),
		SlugOrPort:       app.Slug,
	})
	waps2 := dbgen.WorkspaceAppStat(t, db, database.WorkspaceAppStat{
		UserID:           user.ID,
		WorkspaceID:      ws.ID,
		AgentID:          agent.ID,
		SessionStartedAt: waps1.SessionEndedAt,
		SessionEndedAt:   waps1.SessionEndedAt.Add(time.Minute),
		SlugOrPort:       app.Slug,
	})
	_ = waps2 // Keep the name for documentation.

	// The data is already present, so we can rely on initial rollup to occur.
	events := make(chan dbrollup.Event, 1)
	rolluper := dbrollup.New(logger, db, dbrollup.WithInterval(250*time.Millisecond), dbrollup.WithEventChannel(events))
	defer rolluper.Close()

	<-events // Deplete init event, resume operation.

	ctx := testutil.Context(t, testutil.WaitMedium)

	select {
	case <-ctx.Done():
		t.Fatal("timed out waiting for rollup to occur")
	case ev := <-events:
		require.True(t, ev.TemplateUsageStats, "expected template usage stats to be rolled up")
	}

	stats, err := db.GetTemplateUsageStats(ctx, database.GetTemplateUsageStatsParams{
		StartTime: anHourAndSixMonthsAgo.Add(-time.Minute),
		EndTime:   anHourAgo,
	})
	require.NoError(t, err)
	require.Len(t, stats, 1)

	require.Equal(t, database.TemplateUsageStat{
		TemplateID:          tpl.ID,
		UserID:              user.ID,
		StartTime:           wags1.CreatedAt,
		EndTime:             wags1.CreatedAt.Add(30 * time.Minute),
		MedianLatencyMs:     sql.NullFloat64{Float64: 1, Valid: true},
		UsageMins:           3,
		ReconnectingPtyMins: 2,
		AppUsageMins: database.StringMapOfInt{
			app.Slug: 2,
		},
	}, stats[0])
}
