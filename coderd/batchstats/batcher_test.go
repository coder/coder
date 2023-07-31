package batchstats_test

import (
	"context"
	"github.com/coder/coder/codersdk/agentsdk"
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
	startedAt := time.Now()
	tick := make(chan time.Time)

	b, closer := batchstats.New(
		batchstats.WithStore(store),
		batchstats.WithLogger(log),
		batchstats.WithTicker(tick),
	)
	t.Cleanup(closer)

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
	b.Add(agentsdk.AgentMetric{})
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
		b.Add(agentsdk.AgentMetric{})
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
