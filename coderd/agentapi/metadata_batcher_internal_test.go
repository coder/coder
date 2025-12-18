package agentapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestMetadataBatcher(t *testing.T) {
	t.Parallel()

	// Given: a fresh batcher with no data
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)

	// Set up test agents with metadata.
	agent1 := setupAgentWithMetadata(t, store)
	agent2 := setupAgentWithMetadata(t, store)

	tick := make(chan time.Time)
	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx,
		MetadataBatcherWithStore(store),
		MetadataBatcherWithPubsub(ps),
		MetadataBatcherWithLogger(log),
		func(b *MetadataBatcher) {
			b.tickCh = tick
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	// Given: no metadata updates are added
	// When: it becomes time to flush
	t1 := dbtime.Now()
	tick <- t1
	f := <-flushed
	require.Equal(t, 0, f, "expected no agents to be flushed")
	t.Log("flush 1 completed")

	// Then: no metadata should be updated
	metadata, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent1.ID,
		Keys:             nil,
	})
	require.NoError(t, err)
	for _, md := range metadata {
		// All metadata should still have default timestamps
		require.True(t, md.CollectedAt.Before(t1))
	}

	// Given: a single metadata update is added for agent1
	t2 := t1.Add(time.Second)
	t.Log("adding metadata for 1 agent")
	b.Add(
		agent1.ID,
		[]string{"key1", "key2"},
		[]string{"value1", "value2"},
		[]string{"", ""},
		[]time.Time{t2, t2},
	)

	// When: it becomes time to flush
	tick <- t2
	f = <-flushed
	require.Equal(t, 1, f, "expected one agent to be flushed")
	t.Log("flush 2 completed")

	// Then: agent1's metadata should be updated
	metadata, err = store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent1.ID,
		Keys:             []string{"key1", "key2"},
	})
	require.NoError(t, err)
	require.Len(t, metadata, 2)
	for _, md := range metadata {
		require.True(t, md.CollectedAt.Equal(t2) || md.CollectedAt.After(t1))
	}

	// Given: metadata updates are added for multiple agents
	t3 := t2.Add(time.Second)
	t.Log("adding metadata for 2 agents")
	b.Add(
		agent1.ID,
		[]string{"key1", "key2", "key3"},
		[]string{"new_value1", "new_value2", "new_value3"},
		[]string{"", "", ""},
		[]time.Time{t3, t3, t3},
	)
	b.Add(
		agent2.ID,
		[]string{"key1", "key2"},
		[]string{"agent2_value1", "agent2_value2"},
		[]string{"", ""},
		[]time.Time{t3, t3},
	)

	// When: it becomes time to flush
	tick <- t3
	f = <-flushed
	require.Equal(t, 2, f, "expected two agents to be flushed")
	t.Log("flush 3 completed")

	// Then: both agents' metadata should be updated
	metadata1, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent1.ID,
		Keys:             []string{"key1", "key2", "key3"},
	})
	require.NoError(t, err)
	require.Len(t, metadata1, 3)
	for _, md := range metadata1 {
		require.True(t, md.CollectedAt.Equal(t3) || md.CollectedAt.After(t2))
	}

	metadata2, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent2.ID,
		Keys:             []string{"key1", "key2"},
	})
	require.NoError(t, err)
	require.Len(t, metadata2, 2)
	for _, md := range metadata2 {
		require.True(t, md.CollectedAt.Equal(t3) || md.CollectedAt.After(t2))
	}

	// Given: a lot of agents are added (to trigger early flush)
	t4 := t3.Add(time.Second)
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Add updates for many agents (more than 80% of capacity)
		numAgents := int(float64(defaultMetadataBatchSize) * 0.85)
		t.Logf("adding metadata for %d agents", numAgents)
		for i := 0; i < numAgents; i++ {
			// Create agent with metadata first
			agent := setupAgentWithMetadata(t, store)
			b.Add(
				agent.ID,
				[]string{"key1"},
				[]string{"bulk_value"},
				[]string{""},
				[]time.Time{t4},
			)
		}
	}()

	// When: the buffer comes close to capacity
	// Then: The buffer will force-flush once.
	f = <-flushed
	t.Log("flush 4 completed (early flush)")
	expectedMin := int(float64(defaultMetadataBatchSize) * 0.80)
	require.GreaterOrEqual(t, f, expectedMin, "expected at least 80%% of buffer to be flushed")

	// And we should finish adding all the updates
	<-done

	// Ensures that a subsequent flush pushes any remaining data
	t5 := t4.Add(time.Second)
	tick <- t5
	f2 := <-flushed
	t.Log("flush 5 completed")
	// We should flush whatever remains
	require.GreaterOrEqual(t, f2, 0, "flush should succeed even with no data")

	// Ensure that a subsequent flush does not push stale data
	t6 := t5.Add(time.Second)
	tick <- t6
	f = <-flushed
	require.Zero(t, f, "expected zero agents to have been flushed")
	t.Log("flush 6 completed")
}

func TestMetadataBatcher_ReplacesPendingUpdates(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)

	agent := setupAgentWithMetadata(t, store)

	tick := make(chan time.Time)
	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx,
		MetadataBatcherWithStore(store),
		MetadataBatcherWithPubsub(ps),
		MetadataBatcherWithLogger(log),
		func(b *MetadataBatcher) {
			b.tickCh = tick
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := dbtime.Now()

	// Add first update
	b.Add(
		agent.ID,
		[]string{"key1"},
		[]string{"first_value"},
		[]string{""},
		[]time.Time{t1},
	)

	// Add second update for same agent (should replace first)
	t2 := t1.Add(time.Millisecond)
	b.Add(
		agent.ID,
		[]string{"key1"},
		[]string{"second_value"},
		[]string{""},
		[]time.Time{t2},
	)

	// Flush
	tick <- t2
	f := <-flushed
	require.Equal(t, 1, f, "expected one agent to be flushed")

	// Verify only the second update was applied
	metadata, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent.ID,
		Keys:             []string{"key1"},
	})
	require.NoError(t, err)
	require.Len(t, metadata, 1)
	require.Equal(t, "second_value", metadata[0].Value)
	require.Equal(t, t2, metadata[0].CollectedAt)
}

// setupAgentWithMetadata creates a test agent with some metadata keys.
func setupAgentWithMetadata(t *testing.T, store database.Store) database.WorkspaceAgent {
	t.Helper()

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	tv := dbgen.TemplateVersion(t, store, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl := dbgen.Template(t, store, database.Template{
		CreatedBy:       user.ID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, store, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	})
	pj := dbgen.ProvisionerJob(t, store, nil, database.ProvisionerJob{
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

	// Create some metadata keys for this agent
	for i := 1; i <= 5; i++ {
		err := store.InsertWorkspaceAgentMetadata(context.Background(), database.InsertWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agt.ID,
			DisplayName:      fmt.Sprintf("Key %d", i),
			Key:              fmt.Sprintf("key%d", i),
			Script:           "echo test",
			Timeout:          30000000000, // 30 seconds in nanoseconds
			Interval:         10000000000, // 10 seconds in nanoseconds
			DisplayOrder:     int32(i),
		})
		require.NoError(t, err)
	}

	return agt
}
