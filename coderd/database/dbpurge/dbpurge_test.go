package dbpurge_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/exp/slices"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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

	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	now := dbtime.Now()

	// given
	_, err := db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		// Provisioner daemon created 14 days ago, and checked in just before 7 days deadline.
		ID:           uuid.New(),
		Name:         "external-0",
		Provisioners: []database.ProvisionerType{"echo"},
		CreatedAt:    now.Add(-14 * 24 * time.Hour),
		UpdatedAt:    sql.NullTime{Valid: true, Time: now.Add(-7 * 24 * time.Hour).Add(time.Minute)},
	})
	require.NoError(t, err)
	_, err = db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		// Provisioner daemon created 8 days ago, and checked in last time an hour after creation.
		ID:           uuid.New(),
		Name:         "external-1",
		Provisioners: []database.ProvisionerType{"echo"},
		CreatedAt:    now.Add(-8 * 24 * time.Hour),
		UpdatedAt:    sql.NullTime{Valid: true, Time: now.Add(-8 * 24 * time.Hour).Add(time.Hour)},
	})
	require.NoError(t, err)
	_, err = db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		// Provisioner daemon created 9 days ago, and never checked in.
		ID:           uuid.New(),
		Name:         "external-2",
		Provisioners: []database.ProvisionerType{"echo"},
		CreatedAt:    now.Add(-9 * 24 * time.Hour),
	})
	require.NoError(t, err)
	_, err = db.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		// Provisioner daemon created 6 days ago, and never checked in.
		ID:           uuid.New(),
		Name:         "external-3",
		Provisioners: []database.ProvisionerType{"echo"},
		CreatedAt:    now.Add(-6 * 24 * time.Hour),
		UpdatedAt:    sql.NullTime{Valid: true, Time: now.Add(-6 * 24 * time.Hour)},
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
		return containsProvisionerDaemon(daemons, "external-0") &&
			!containsProvisionerDaemon(daemons, "external-1") &&
			!containsProvisionerDaemon(daemons, "external-2") &&
			containsProvisionerDaemon(daemons, "external-3")
	}, testutil.WaitShort, testutil.IntervalFast)
}

func containsProvisionerDaemon(daemons []database.ProvisionerDaemon, name string) bool {
	return slices.ContainsFunc(daemons, func(d database.ProvisionerDaemon) bool {
		return d.Name == name
	})
}
