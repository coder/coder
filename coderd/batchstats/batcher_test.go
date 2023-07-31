package batchstats_test

import (
	"context"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/batchstats"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
)

func TestBatchStats(t *testing.T) {
	t.Parallel()
	// Given: a fresh batcher with no data
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	store, _ := dbtestutil.NewDB(t)
	ws1 := dbgen.Workspace(t, store, database.Workspace{
		LastUsedAt: time.Now().Add(-time.Hour),
	})
	ws2 := dbgen.Workspace(t, store, database.Workspace{
		LastUsedAt: time.Now().Add(-time.Hour),
	})
	// TODO: link to ws1 and ws2
	agt1 := dbgen.WorkspaceAgent(t, store, database.WorkspaceAgent{})
	agt2 := dbgen.WorkspaceAgent(t, store, database.WorkspaceAgent{})
	startedAt := time.Now()
	tick := make(chan time.Time)

	b, err := batchstats.New(
		batchstats.WithStore(store),
		batchstats.WithLogger(log),
		batchstats.WithTicker(tick),
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
	require.NoError(t, b.Add(ctx, agt1.ID, randAgentSDKStats(t)))
	// And it becomes time to report stats
	t2 := time.Now()
	tick <- t2

	// Then: it should report a single stat.
	stats, err = store.GetWorkspaceAgentStats(ctx, startedAt)
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
	t3 := time.Now()
	for i := 0; i < batchstats.DefaultBatchSize; i++ {
		if i%2 == 0 {
			require.NoError(t, b.Add(ctx, agt1.ID, randAgentSDKStats(t)))
		} else {
			require.NoError(t, b.Add(ctx, agt2.ID, randAgentSDKStats(t)))
		}
	}

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

// randInsertWorkspaceAgentStatParams returns a random InsertWorkspaceAgentStatParams
func randInsertWorkspaceAgentStatParams(t *testing.T, opts ...func(params *database.InsertWorkspaceAgentStatParams)) database.InsertWorkspaceAgentStatParams {
	t.Helper()
	p := database.InsertWorkspaceAgentStatParams{
		ID:                          uuid.New(),
		CreatedAt:                   time.Now(),
		UserID:                      uuid.New(),
		WorkspaceID:                 uuid.New(),
		TemplateID:                  uuid.New(),
		AgentID:                     uuid.New(),
		ConnectionsByProto:          []byte(`{"tcp": 1}`),
		ConnectionCount:             1,
		RxPackets:                   1,
		RxBytes:                     1,
		TxPackets:                   1,
		TxBytes:                     1,
		SessionCountVSCode:          1,
		SessionCountJetBrains:       1,
		SessionCountReconnectingPTY: 0,
		SessionCountSSH:             0,
		ConnectionMedianLatencyMS:   0,
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

//type InsertWorkspaceAgentStatParams struct {
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

//type GetWorkspaceAgentStatsRow struct {
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
