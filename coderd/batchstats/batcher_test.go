package batchstats_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/batchstats"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/cryptorand"
)

func TestBatchStats(t *testing.T) {
	var (
		batchSize = batchstats.DefaultBatchSize
	)
	t.Parallel()
	// Given: a fresh batcher with no data
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, _ := dbtestutil.NewDB(t)

	// Set up some test dependencies.
	deps := setupDeps(t, store)
	tick := make(chan time.Time)
	flushed := make(chan bool)

	b, err := batchstats.New(
		batchstats.WithStore(store),
		batchstats.WithBatchSize(batchSize),
		batchstats.WithLogger(log),
		batchstats.WithTicker(tick),
		batchstats.WithFlushed(flushed),
	)
	require.NoError(t, err)

	// Given: no data points are added for workspace
	// When: it becomes time to report stats
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
	})
	go func() {
		b.Run(ctx)
	}()
	t1 := time.Now()
	// Signal a tick and wait for a flush to complete.
	tick <- t1
	f := <-flushed
	require.False(t, f, "flush should not have been forced")
	t.Logf("flush 1 completed")

	// Then: it should report no stats.
	stats, err := store.GetWorkspaceAgentStats(ctx, t1)
	require.NoError(t, err)
	require.Empty(t, stats)

	// Given: a single data point is added for workspace
	t2 := time.Now()
	t.Logf("inserting 1 stat")
	require.NoError(t, b.Add(ctx, deps.Agent.ID, randAgentSDKStats(t)))

	// When: it becomes time to report stats
	// Signal a tick and wait for a flush to complete.
	tick <- t2
	f = <-flushed // Wait for a flush to complete.
	require.False(t, f, "flush should not have been forced")
	t.Logf("flush 2 completed")

	// Then: it should report a single stat.
	stats, err = store.GetWorkspaceAgentStats(ctx, t2)
	require.NoError(t, err)
	require.Len(t, stats, 1)

	// Given: a lot of data points are added for workspace
	// (equal to batch size)
	t3 := time.Now()
	t.Logf("inserting %d stats", batchSize)
	for i := 0; i < batchSize; i++ {
		require.NoError(t, b.Add(ctx, deps.Agent.ID, randAgentSDKStats(t)))
	}

	// When: the buffer is full
	// Wait for a flush to complete. This should be forced by filling the buffer.
	f = <-flushed
	require.True(t, f, "flush should have been forced")
	t.Logf("flush 3 completed")

	// Then: it should immediately flush its stats to store.
	stats, err = store.GetWorkspaceAgentStats(ctx, t3)
	require.NoError(t, err)
	if assert.Len(t, stats, 1) {
		assert.Greater(t, stats[0].AggregatedFrom, t3)
		assert.Equal(t, stats[0].AgentID, deps.Agent.ID)
		assert.Equal(t, stats[0].WorkspaceID, deps.Workspace.ID)
		assert.Equal(t, stats[0].TemplateID, deps.Template.ID)
		assert.NotZero(t, stats[0].WorkspaceRxBytes)
		assert.NotZero(t, stats[0].WorkspaceTxBytes)
		assert.NotZero(t, stats[0].WorkspaceConnectionLatency50)
		assert.NotZero(t, stats[0].WorkspaceConnectionLatency95)
		assert.NotZero(t, stats[0].SessionCountVSCode)
		assert.NotZero(t, stats[0].SessionCountSSH)
		assert.NotZero(t, stats[0].SessionCountJetBrains)
		assert.NotZero(t, stats[0].SessionCountReconnectingPTY)
	}
}

// randAgentSDKStats returns a random agentsdk.Stats
func randAgentSDKStats(t *testing.T, opts ...func(*agentsdk.Stats)) agentsdk.Stats {
	t.Helper()
	s := agentsdk.Stats{
		ConnectionsByProto: map[string]int64{
			"ssh":              mustRandInt64n(t, 9) + 1,
			"vscode":           mustRandInt64n(t, 9) + 1,
			"jetbrains":        mustRandInt64n(t, 9) + 1,
			"reconnecting_pty": mustRandInt64n(t, 9) + 1,
		},
		ConnectionCount:             mustRandInt64n(t, 99) + 1,
		ConnectionMedianLatencyMS:   float64(mustRandInt64n(t, 99) + 1),
		RxPackets:                   mustRandInt64n(t, 99) + 1,
		RxBytes:                     mustRandInt64n(t, 99) + 1,
		TxPackets:                   mustRandInt64n(t, 99) + 1,
		TxBytes:                     mustRandInt64n(t, 99) + 1,
		SessionCountVSCode:          mustRandInt64n(t, 9) + 1,
		SessionCountJetBrains:       mustRandInt64n(t, 9) + 1,
		SessionCountReconnectingPTY: mustRandInt64n(t, 9) + 1,
		SessionCountSSH:             mustRandInt64n(t, 9) + 1,
		Metrics:                     []agentsdk.AgentMetric{},
	}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// type Stats struct {
// 	ConnectionsByProto map[string]int64 `json:"connections_by_proto"`
// 	ConnectionCount int64 `json:"connection_count"`
// 	ConnectionMedianLatencyMS float64 `json:"connection_median_latency_ms"`
// 	RxPackets int64 `json:"rx_packets"`
// 	RxBytes int64 `json:"rx_bytes"`
// 	TxPackets int64 `json:"tx_packets"`
// 	TxBytes int64 `json:"tx_bytes"`
// 	SessionCountVSCode int64 `json:"session_count_vscode"`
// 	SessionCountJetBrains int64 `json:"session_count_jetbrains"`
// 	SessionCountReconnectingPTY int64 `json:"session_count_reconnecting_pty"`
// 	SessionCountSSH int64 `json:"session_count_ssh"`

// 	// Metrics collected by the agent
// 	Metrics []AgentMetric `json:"metrics"`
// }

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

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	_, err := store.InsertOrganizationMember(context.Background(), database.InsertOrganizationMemberParams{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Roles:          []string{rbac.RoleOrgMember(org.ID)},
	})
	require.NoError(t, err)
	tv := dbgen.TemplateVersion(t, store, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl := dbgen.Template(t, store, database.Template{
		CreatedBy:       user.ID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, store, database.Workspace{
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		LastUsedAt:     time.Now().Add(-time.Hour),
	})
	pj := dbgen.ProvisionerJob(t, store, database.ProvisionerJob{
		InitiatorID:    user.ID,
		OrganizationID: org.ID,
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

func mustRandInt64n(t *testing.T, n int64) int64 {
	t.Helper()
	i, err := cryptorand.Intn(int(n))
	require.NoError(t, err)
	return int64(i)
}
