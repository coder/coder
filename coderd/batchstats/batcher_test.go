package batchstats_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/batchstats"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/codersdk/agentsdk"
)

func TestBatchStats(t *testing.T) {
	t.Parallel()
	// Given: a fresh batcher with no data
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	store, _ := dbtestutil.NewDB(t)

	// Set up some test dependencies.
	deps1 := setupDeps(t, store)
	ws1, err := store.GetWorkspaceByID(ctx, deps1.Workspace.ID)
	deps2 := setupDeps(t, store)
	require.NoError(t, err)
	ws2, err := store.GetWorkspaceByID(ctx, deps2.Workspace.ID)
	require.NoError(t, err)
	startedAt := time.Now()
	tick := make(chan time.Time)
	flushed := make(chan struct{})

	b, err := batchstats.New(
		batchstats.WithStore(store),
		batchstats.WithLogger(log),
		batchstats.WithTicker(tick),
		batchstats.WithFlushed(flushed),
	)
	require.NoError(t, err)

	// When: it becomes time to report stats
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
	})
	go func() {
		b.Run(ctx)
	}()
	t1 := time.Now()
	tick <- t1
	<-flushed // Wait for a flush to complete.

	// Then: it should report no stats.
	stats, err := store.GetWorkspaceAgentStats(ctx, startedAt)
	require.NoError(t, err)
	require.Empty(t, stats)

	// Then: workspace last used time should not be updated
	updated1, err := store.GetWorkspaceByID(ctx, ws1.ID)
	require.NoError(t, err)
	require.Equal(t, ws1.LastUsedAt, updated1.LastUsedAt)
	updated2, err := store.GetWorkspaceByID(ctx, ws2.ID)
	require.NoError(t, err)
	require.Equal(t, ws2.LastUsedAt, updated2.LastUsedAt)

	// When: a single data point is added for ws1
	require.NoError(t, b.Add(ctx, deps1.Agent.ID, randAgentSDKStats(t)))
	// And it becomes time to report stats
	t2 := time.Now()
	tick <- t2
	<-flushed // Wait for a flush to complete.

	// Then: it should report a single stat.
	stats, err = store.GetWorkspaceAgentStats(ctx, t1)
	require.NoError(t, err)
	require.Len(t, stats, 1)

	// Then: ws1 last used time should be updated
	updated1, err = store.GetWorkspaceByID(ctx, ws1.ID)
	require.NoError(t, err)
	require.NotEqual(t, ws1.LastUsedAt, updated1.LastUsedAt)
	// And: ws2 last used time should not be updated
	updated2, err = store.GetWorkspaceByID(ctx, ws2.ID)
	require.NoError(t, err)
	require.Equal(t, ws2.LastUsedAt, updated2.LastUsedAt)

	// When: a lot of data points are added for both ws1 and ws2
	// (equal to batch size)
	for i := 0; i < batchstats.DefaultBatchSize; i++ {
		if i%2 == 0 {
			require.NoError(t, b.Add(ctx, deps1.Agent.ID, randAgentSDKStats(t)))
		} else {
			require.NoError(t, b.Add(ctx, deps2.Agent.ID, randAgentSDKStats(t)))
		}
	}
	t3 := time.Now()
	tick <- t3
	<-flushed // Wait for a flush to complete.

	// Then: it should immediately flush its stats to store.
	stats, err = store.GetWorkspaceAgentStats(ctx, t3)
	require.NoError(t, err)
	require.Len(t, stats, batchstats.DefaultBatchSize)

	// Then: ws1 and ws2 last used time should be updated
	updated1, err = store.GetWorkspaceByID(ctx, ws1.ID)
	require.NoError(t, err)
	require.NotEqual(t, ws1.LastUsedAt, updated1.LastUsedAt)
	updated2, err = store.GetWorkspaceByID(ctx, ws2.ID)
	require.NoError(t, err)
	require.NotEqual(t, ws2.LastUsedAt, updated2.LastUsedAt)
}

// randAgentSDKStats returns a random agentsdk.Stats
func randAgentSDKStats(t *testing.T, opts ...func(*agentsdk.Stats)) agentsdk.Stats {
	t.Helper()
	var s agentsdk.Stats
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// type InsertWorkspaceAgentStatParams struct {
//	ID                          uuid.UUID       `db:"id" json:"id"`
//	CreatedAt                   time.Time       `db:"created_at" json:"created_at"`
//	UserID                      uuid.UUID       `db:"user_id" json:"user_id"`
//	WorkspaceID                 uuid.UUID       `db:"workspace_id" json:"workspace_id"`
//	TemplateID                  uuid.UUID       `db:"template_id" json:"template_id"`
//	AgentID                     uuid.UUID       `db:"agent_id" json:"agent_id"`
//	ConnectionsByProto          json.RawMessage `db:"connections_by_proto" json:"connections_by_proto"`
//	ConnectionCount             int64           `db:"connection_count" json:"connection_count"`
//	RxPackets                   int64           `db:"rx_packets" json:"rx_packets"`
//	RxBytes                     int64           `db:"rx_bytes" json:"rx_bytes"`
//	TxPackets                   int64           `db:"tx_packets" json:"tx_packets"`
//	TxBytes                     int64           `db:"tx_bytes" json:"tx_bytes"`
//	SessionCountVSCode          int64           `db:"session_count_vscode" json:"session_count_vscode"`
//	SessionCountJetBrains       int64           `db:"session_count_jetbrains" json:"session_count_jetbrains"`
//	SessionCountReconnectingPTY int64           `db:"session_count_reconnecting_pty" json:"session_count_reconnecting_pty"`
//	SessionCountSSH             int64           `db:"session_count_ssh" json:"session_count_ssh"`
//	ConnectionMedianLatencyMS   float64         `db:"connection_median_latency_ms" json:"connection_median_latency_ms"`
//}

// type GetWorkspaceAgentStatsRow struct {
//	UserID                       uuid.UUID `db:"user_id" json:"user_id"`
//	AgentID                      uuid.UUID `db:"agent_id" json:"agent_id"`
//	WorkspaceID                  uuid.UUID `db:"workspace_id" json:"workspace_id"`
//	TemplateID                   uuid.UUID `db:"template_id" json:"template_id"`
//	AggregatedFrom               time.Time `db:"aggregated_from" json:"aggregated_from"`
//	WorkspaceRxBytes             int64     `db:"workspace_rx_bytes" json:"workspace_rx_bytes"`
//	WorkspaceTxBytes             int64     `db:"workspace_tx_bytes" json:"workspace_tx_bytes"`
//	WorkspaceConnectionLatency50 float64   `db:"workspace_connection_latency_50" json:"workspace_connection_latency_50"`
//	WorkspaceConnectionLatency95 float64   `db:"workspace_connection_latency_95" json:"workspace_connection_latency_95"`
//	AgentID_2                    uuid.UUID `db:"agent_id_2" json:"agent_id_2"`
//	SessionCountVSCode           int64     `db:"session_count_vscode" json:"session_count_vscode"`
//	SessionCountSSH              int64     `db:"session_count_ssh" json:"session_count_ssh"`
//	SessionCountJetBrains        int64     `db:"session_count_jetbrains" json:"session_count_jetbrains"`
//	SessionCountReconnectingPTY  int64     `db:"session_count_reconnecting_pty" json:"session_count_reconnecting_pty"`
//}

type deps struct {
	Agent     database.WorkspaceAgent
	Template  database.Template
	User      database.User
	Workspace database.Workspace
}

func setupDeps(t *testing.T, store database.Store) deps {
	t.Helper()

	user := dbgen.User(t, store, database.User{})
	tv := dbgen.TemplateVersion(t, store, database.TemplateVersion{})
	tpl := dbgen.Template(t, store, database.Template{
		CreatedBy:       user.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, store, database.Workspace{
		TemplateID: tpl.ID,
		OwnerID:    user.ID,
		LastUsedAt: time.Now().Add(-time.Hour),
	})
	pj := dbgen.ProvisionerJob(t, store, database.ProvisionerJob{
		InitiatorID: user.ID,
	})
	_ = dbgen.WorkspaceBuild(t, store, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
	})
	res := dbgen.WorkspaceResource(t, store, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agt := dbgen.WorkspaceAgent(t, store, database.WorkspaceAgent{
		ResourceID: res.ID,
	})
	return deps{
		Agent:     agt,
		Template:  tpl,
		User:      user,
		Workspace: ws,
	}
}
