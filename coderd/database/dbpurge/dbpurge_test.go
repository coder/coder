package dbpurge_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/exp/slices"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())

	// Given: a number of agents with associated agent logs
	opts := seedOpts{
		NumAgents:        10,
		NumLogsPerAgent:  10,
		NumStatsPerAgent: 10,
	}
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	agentIDs, _ := seed(ctx, t, db, opts)

	// Set last connectd time for agents.
	// For half of the agents, set their last connected time to be older than one week ago.
	// For the other half, set their last connected time to be within the last week.
	for i := 0; i < opts.NumAgents; i++ {
		var connectedAt time.Time
		if i%2 == 0 {
			connectedAt = weekAgo.AddDate(0, 0, -randintn(7))
		} else {
			connectedAt = now.AddDate(0, 0, -randintn(7))
		}
		setAgentLastConnectedAt(ctx, t, db, agentIDs[i], connectedAt)
	}

	// Assert that some old logs exist
	var logsBefore int
	for i := 0; i < opts.NumAgents; i++ {
		agentLogFn(ctx, t, db, agentIDs[i], func(l database.WorkspaceAgentLog) {
			logsBefore++
		})
	}
	require.Greater(t, logsBefore, 0, "no agent logs were inserted")
	t.Logf("before: %d agent logs", logsBefore)

	// Run the purge
	purger := dbpurge.New(ctx, slogtest.Make(t, nil), db)
	err := purger.Close()
	require.NoError(t, err, "expected no error running purger")

	// Assert that no old logs exist
	var logsAfter int
	for i := 0; i < opts.NumAgents; i++ {
		agentLogFn(ctx, t, db, agentIDs[i], func(l database.WorkspaceAgentLog) {
			logsAfter++
		})
	}
	assert.Less(t, logsAfter, logsBefore, "expected fewer logs after running purger")
	assert.NotZero(t, logsAfter, "expected some logs to remain after running purger")
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
		return contains(daemons, "external-0") &&
			contains(daemons, "external-3")
	}, testutil.WaitShort, testutil.IntervalFast)
}

func contains(daemons []database.ProvisionerDaemon, name string) bool {
	return slices.ContainsFunc(daemons, func(d database.ProvisionerDaemon) bool {
		return d.Name == name
	})
}

type seedOpts struct {
	NumAgents        int
	NumLogsPerAgent  int
	NumStatsPerAgent int
}

func seed(ctx context.Context, t testing.TB, db database.Store, opts seedOpts) ([]uuid.UUID, []dbfake.WorkspaceResponse) {
	t.Helper()

	agentIDs := make([]uuid.UUID, opts.NumAgents)
	workspaces := make([]dbfake.WorkspaceResponse, opts.NumAgents)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	tv := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	}).Do()
	workspaceTemplate := database.Workspace{
		TemplateID:     tv.Template.ID,
		OrganizationID: tv.Template.OrganizationID,
		OwnerID:        user.ID,
	}

	for i := 0; i < opts.NumAgents; i++ {
		wsr := dbfake.WorkspaceBuild(t, db, workspaceTemplate).WithAgent().Do()
		resources, err := db.GetWorkspaceResourcesByJobID(ctx, wsr.Build.JobID)
		require.NoError(t, err)
		require.NotEmpty(t, resources)
		agents, err := db.GetWorkspaceAgentsByResourceIDs(ctx, []uuid.UUID{resources[0].ID})
		require.NoError(t, err)
		require.NotEmpty(t, agents)
		agentIDs[i] = agents[0].ID
		workspaces[i] = wsr
	}

	for i := 0; i < opts.NumAgents; i++ {
		// Create a number of logs for each agent
		var entries []string
		for i := 0; i < opts.NumLogsPerAgent; i++ {
			entries = append(entries, "an entry")
		}
		_, err := db.InsertWorkspaceAgentLogs(context.Background(), database.InsertWorkspaceAgentLogsParams{
			AgentID:      agentIDs[i],
			CreatedAt:    time.Now(),
			Output:       entries,
			Level:        times(database.LogLevelInfo, opts.NumLogsPerAgent),
			LogSourceID:  uuid.UUID{},
			OutputLength: 0,
		})
		require.NoError(t, err)

		// Insert a number of stats for each agent
		err = db.InsertWorkspaceAgentStats(context.Background(), database.InsertWorkspaceAgentStatsParams{
			ID:                          timesf(uuid.New, opts.NumStatsPerAgent),
			CreatedAt:                   times(dbtime.Now(), opts.NumStatsPerAgent),
			UserID:                      times(workspaces[i].Workspace.OwnerID, opts.NumStatsPerAgent),
			WorkspaceID:                 times(workspaces[i].Workspace.ID, opts.NumStatsPerAgent),
			TemplateID:                  times(workspaces[i].Workspace.TemplateID, opts.NumStatsPerAgent),
			AgentID:                     times(agentIDs[i], opts.NumStatsPerAgent),
			ConnectionsByProto:          fakeConnectionsByProto(t, opts.NumStatsPerAgent),
			ConnectionCount:             timesf(randint64, opts.NumStatsPerAgent),
			RxPackets:                   timesf(randint64, opts.NumStatsPerAgent),
			RxBytes:                     timesf(randint64, opts.NumStatsPerAgent),
			TxPackets:                   timesf(randint64, opts.NumStatsPerAgent),
			TxBytes:                     timesf(randint64, opts.NumStatsPerAgent),
			SessionCountVSCode:          timesf(randint64, opts.NumStatsPerAgent),
			SessionCountJetBrains:       timesf(randint64, opts.NumStatsPerAgent),
			SessionCountReconnectingPTY: timesf(randint64, opts.NumStatsPerAgent),
			SessionCountSSH:             timesf(randint64, opts.NumStatsPerAgent),
			ConnectionMedianLatencyMS:   timesf(rand.Float64, opts.NumStatsPerAgent),
		})
		require.NoError(t, err)
	}
	return agentIDs, workspaces
}

func agentLogFn(ctx context.Context, t testing.TB, db database.Store, agentID uuid.UUID, fn func(database.WorkspaceAgentLog)) {
	logs, err := db.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
		AgentID:      agentID,
		CreatedAfter: 0,
	})
	require.NoError(t, err)
	for _, l := range logs {
		fn(l)
	}
}

func setAgentLastConnectedAt(ctx context.Context, t testing.TB, db database.Store, agentID uuid.UUID, lastConnectedAt time.Time) {
	t.Helper()

	err := db.UpdateWorkspaceAgentConnectionByID(ctx, database.UpdateWorkspaceAgentConnectionByIDParams{
		ID:                     agentID,
		FirstConnectedAt:       sql.NullTime{Time: time.Unix(0, 0), Valid: true},
		LastConnectedAt:        sql.NullTime{Time: lastConnectedAt, Valid: true},
		LastConnectedReplicaID: uuid.NullUUID{},
		DisconnectedAt:         sql.NullTime{},
		UpdatedAt:              dbtime.Now(),
	})

	require.NoError(t, err)
}

func fakeConnectionsByProto(t testing.TB, n int) json.RawMessage {
	ms := make([]map[string]int64, n)
	for i := 0; i < n; i++ {
		m := map[string]int64{
			"vscode": randint64(),
			"tty":    randint64(),
			"ssh":    randint64(),
		}
		ms[i] = m
	}
	bytes, err := json.Marshal(ms)
	require.NoError(t, err)
	return bytes
}

// times returns a slice consisting of T repeated n times
func times[T any](t T, n int) []T {
	ts := make([]T, n)
	for i := 0; i < n; i++ {
		ts[i] = t
	}
	return ts
}

// timesf returns a slice consisting of running f n times
// and appending the results to a new slice of type T
func timesf[T any](f func() T, n int) []T {
	ts := make([]T, n)
	for i := 0; i < n; i++ {
		ts[i] = f()
	}
	return ts
}

//nolint:gosec // not used for crypto purposes
func randint64() int64 {
	return rand.Int63()
}

//nolint:gosec // not used for crypto purposes
func randintn(n int) int {
	return rand.Intn(n)
}
