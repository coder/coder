package dbpurge_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/exp/slices"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Ensures no goroutines leak.
func TestPurge(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	opts := seedOpts{
		NumAgents:        10,
		NumLogsPerAgent:  10,
		NumStatsPerAgent: 10,
	}
	seed(t, db, opts)
	purger := dbpurge.New(context.Background(), slogtest.Make(t, nil), db)
	err := purger.Close()
	require.NoError(t, err)
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

func seed(t *testing.T, db database.Store, opts seedOpts) {
	t.Helper()

	// Create a number of agents
	agentIDs := make([]uuid.UUID, opts.NumAgents)
	agentWSes := make(map[uuid.UUID]dbfake.WorkspaceResponse)
	for i := 0; i < opts.NumAgents; i++ {
		agentID := uuid.New()
		wsr := dbfake.Workspace(t, db).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
			for _, agt := range agents {
				agt.Id = agentID.String()
			}
			return agents
		}).Do()
		agentIDs[i] = agentID
		agentWSes[agentID] = wsr
	}

	for _, agentID := range agentIDs {
		// Create a number of logs for each agent
		var entries []string
		for i := 0; i < opts.NumLogsPerAgent; i++ {
			randStr, err := cryptorand.String(1024)
			require.NoError(t, err)
			entries = append(entries, randStr)
		}
		_, err := db.InsertWorkspaceAgentLogs(context.Background(), database.InsertWorkspaceAgentLogsParams{
			AgentID:      agentID,
			CreatedAt:    time.Now(),
			Output:       entries,
			Level:        times(database.LogLevelInfo, opts.NumLogsPerAgent),
			LogSourceID:  uuid.UUID{},
			OutputLength: 0,
		})
		require.NoError(t, err)

		// Insert a number of stats for each agent
		err = db.InsertWorkspaceAgentStats(context.Background(), database.InsertWorkspaceAgentStatsParams{
			ID:                          times(agentID, opts.NumStatsPerAgent),
			CreatedAt:                   times(dbtime.Now(), opts.NumStatsPerAgent),
			UserID:                      times(agentWSes[agentID].Workspace.OwnerID, opts.NumStatsPerAgent),
			WorkspaceID:                 times(agentWSes[agentID].Workspace.ID, opts.NumStatsPerAgent),
			TemplateID:                  times(agentWSes[agentID].Workspace.TemplateID, opts.NumStatsPerAgent),
			AgentID:                     times(agentID, opts.NumStatsPerAgent),
			ConnectionsByProto:          fakeConnectionsByProto(t, opts.NumStatsPerAgent),
			ConnectionCount:             timesf(rand.Int63, opts.NumStatsPerAgent),
			RxPackets:                   timesf(rand.Int63, opts.NumStatsPerAgent),
			RxBytes:                     timesf(rand.Int63, opts.NumStatsPerAgent),
			TxPackets:                   timesf(rand.Int63, opts.NumStatsPerAgent),
			TxBytes:                     timesf(rand.Int63, opts.NumStatsPerAgent),
			SessionCountVSCode:          timesf(rand.Int63, opts.NumStatsPerAgent),
			SessionCountJetBrains:       timesf(rand.Int63, opts.NumStatsPerAgent),
			SessionCountReconnectingPTY: timesf(rand.Int63, opts.NumStatsPerAgent),
			SessionCountSSH:             timesf(rand.Int63, opts.NumStatsPerAgent),
			ConnectionMedianLatencyMS:   timesf(rand.Float64, opts.NumStatsPerAgent),
		})
		require.NoError(t, err)
	}
}

func fakeConnectionsByProto(t testing.TB, n int) json.RawMessage {
	ms := make([]map[string]int64, n)
	for i := 0; i < n; i++ {
		m := map[string]int64{
			"vscode": rand.Int63(),
			"tty":    rand.Int63(),
			"ssh":    rand.Int63(),
		}
		ms = append(ms, m)
	}
	bytes, err := json.Marshal(ms)
	require.NoError(t, err)
	return bytes
}

func times[T any](t T, n int) []T {
	ts := make([]T, n)
	for i := 0; i < n; i++ {
		ts[i] = t
	}
	return ts
}

func timesf[T any](f func() T, n int) []T {
	ts := make([]T, n)
	for i := 0; i < n; i++ {
		ts[i] = f()
	}
	return ts
}
