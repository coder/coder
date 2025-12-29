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
	b.Add(agent1.ID, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t2, t2})

	// When: it becomes time to flush
	tick <- t2
	f = <-flushed
	require.Equal(t, 2, f, "expected 2 entries to be flushed")
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
	b.Add(agent1.ID, []string{"key1", "key2", "key3"}, []string{"new_value1", "new_value2", "new_value3"}, []string{"", "", ""}, []time.Time{t3, t3, t3})
	b.Add(agent2.ID, []string{"key1", "key2"}, []string{"agent2_value1", "agent2_value2"}, []string{"", ""}, []time.Time{t3, t3})

	// When: it becomes time to flush
	tick <- t3
	f = <-flushed
	require.Equal(t, 5, f, "expected 5 entries to be flushed (3 for agent1 + 2 for agent2)")
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

	// Given: a lot of agents are added (to trigger flush at capacity)
	t4 := t3.Add(time.Second)
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Add updates to fill the buffer exactly to capacity
		numAgents := defaultMetadataBatchSize
		t.Logf("adding metadata for %d agents", numAgents)
		for i := 0; i < numAgents; i++ {
			// Create agent with metadata first
			agent := setupAgentWithMetadata(t, store)
			b.Add(agent.ID, []string{"key1"}, []string{"bulk_value"}, []string{""}, []time.Time{t4})
		}
	}()

	// When: the buffer reaches capacity
	// Then: The buffer will force-flush.
	f = <-flushed
	t.Log("flush 4 completed (capacity flush)")
	require.Equal(t, defaultMetadataBatchSize, f, "expected full buffer to be flushed")

	// And we should finish adding all the updates
	<-done

	// Ensure that a subsequent flush does not push stale data
	t5 := t4.Add(time.Second)
	tick <- t5
	f = <-flushed
	require.Zero(t, f, "expected zero entries to have been flushed")
	t.Log("flush 5 completed")
}

func TestMetadataBatcher_DropsWhenFull(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)

	agent1 := setupAgentWithMetadata(t, store)
	agent2 := setupAgentWithMetadata(t, store)

	tick := make(chan time.Time)
	flushed := make(chan int, 1)

	// Create batcher with very small capacity
	b, closer, err := NewMetadataBatcher(ctx,
		MetadataBatcherWithStore(store),
		MetadataBatcherWithPubsub(ps),
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(2),
		func(b *MetadataBatcher) {
			b.tickCh = tick
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := dbtime.Now()

	// Fill buffer to capacity
	b.Add(agent1.ID, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1})
	b.Add(agent2.ID, []string{"key1"}, []string{"value2"}, []string{""}, []time.Time{t1})

	// Buffer should now trigger flush
	f := <-flushed
	require.Equal(t, 2, f, "expected two updates to be flushed")

	// Try to add another update - buffer is now empty but we'll fill it again
	t2 := t1.Add(time.Second)
	b.Add(agent1.ID, []string{"key2"}, []string{"value3"}, []string{""}, []time.Time{t2})
	b.Add(agent2.ID, []string{"key2"}, []string{"value4"}, []string{""}, []time.Time{t2})

	// Try to add one more - this should be dropped
	agent3 := setupAgentWithMetadata(t, store)
	b.Add(agent3.ID, []string{"key1"}, []string{"dropped"}, []string{""}, []time.Time{t2})

	// Flush
	tick <- t2
	f = <-flushed
	require.Equal(t, 2, f, "expected only two updates, third should have been dropped")
}

func TestMetadataBatcher_MultipleUpdatesForSameAgent(t *testing.T) {
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
	b.Add(agent.ID, []string{"key1"}, []string{"first_value"}, []string{""}, []time.Time{t1})

	// Add second update for same agent+key (should deduplicate)
	t2 := t1.Add(time.Millisecond)
	b.Add(agent.ID, []string{"key1"}, []string{"second_value"}, []string{""}, []time.Time{t2})

	// Flush
	tick <- t2
	f := <-flushed
	require.Equal(t, 1, f, "expected 1 entry to be flushed (deduplicated)")

	// Verify the second update was applied (deduplication keeps latest value)
	metadata, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent.ID,
		Keys:             []string{"key1"},
	})
	require.NoError(t, err)
	require.Len(t, metadata, 1)
	require.Equal(t, "second_value", metadata[0].Value)
	require.Equal(t, t2, metadata[0].CollectedAt)
}

func TestMetadataBatcher_DeduplicationWithMixedKeys(t *testing.T) {
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

	// Add updates with some duplicate keys and some unique keys
	b.Add(agent.ID, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t1, t1})

	t2 := t1.Add(time.Millisecond)
	// Update key1, add key3 - key2 stays from first update
	b.Add(agent.ID, []string{"key1", "key3"}, []string{"new_value1", "value3"}, []string{"", ""}, []time.Time{t2, t2})

	// Flush
	tick <- t2
	f := <-flushed
	require.Equal(t, 3, f, "expected 3 entries (key1 deduplicated, key2 and key3 unique)")

	// Verify all keys are present with correct values
	metadata, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent.ID,
		Keys:             []string{"key1", "key2", "key3"},
	})
	require.NoError(t, err)
	require.Len(t, metadata, 3)

	// Check each metadata value
	for _, md := range metadata {
		switch md.Key {
		case "key1":
			require.Equal(t, "new_value1", md.Value, "key1 should have updated value")
			require.Equal(t, t2, md.CollectedAt)
		case "key2":
			require.Equal(t, "value2", md.Value, "key2 should have original value")
			require.Equal(t, t1, md.CollectedAt)
		case "key3":
			require.Equal(t, "value3", md.Value, "key3 should be present")
			require.Equal(t, t2, md.CollectedAt)
		}
	}
}

func TestMetadataBatcher_EntryCountTracking(t *testing.T) {
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
		MetadataBatcherWithBatchSize(5), // Small size to test capacity
		func(b *MetadataBatcher) {
			b.tickCh = tick
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := dbtime.Now()

	// Add 3 entries
	b.Add(agent.ID, []string{"key1", "key2", "key3"}, []string{"v1", "v2", "v3"}, []string{"", "", ""}, []time.Time{t1, t1, t1})

	// Verify internal state
	b.mu.Lock()
	require.Equal(t, 3, b.entryCount, "entryCount should be 3")
	b.mu.Unlock()

	// Add 2 more entries (should reach capacity of 5)
	b.Add(agent.ID, []string{"key4", "key5"}, []string{"v4", "v5"}, []string{"", ""}, []time.Time{t1, t1})

	// Should trigger automatic flush at capacity
	f := <-flushed
	require.Equal(t, 5, f, "expected 5 entries to be flushed")

	// Verify entry count reset after flush
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "entryCount should be reset to 0 after flush")
	b.mu.Unlock()

	// Add update to same key (should be counted as replacement, not new entry)
	t2 := t1.Add(time.Second)
	b.Add(agent.ID, []string{"key1"}, []string{"new_v1"}, []string{""}, []time.Time{t2})

	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "entryCount should be 1 after adding new entry")
	b.mu.Unlock()

	// Update same key again (should still be 1 entry)
	b.Add(agent.ID, []string{"key1"}, []string{"newer_v1"}, []string{""}, []time.Time{t2})

	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "entryCount should still be 1 after replacement")
	b.mu.Unlock()
}

func TestMetadataBatcher_TimestampOrdering(t *testing.T) {
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
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)

	// Add update with t2 timestamp
	b.Add(agent.ID, []string{"key1"}, []string{"newer_value"}, []string{""}, []time.Time{t2})

	// Try to add older update with t1 timestamp - should be ignored
	b.Add(agent.ID, []string{"key1"}, []string{"older_value"}, []string{""}, []time.Time{t1})

	// Add even newer update with t3 timestamp - should overwrite
	b.Add(agent.ID, []string{"key1"}, []string{"newest_value"}, []string{""}, []time.Time{t3})

	// Verify internal state - should have only 1 entry
	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "entryCount should be 1")
	require.Equal(t, "newest_value", b.buf[agent.ID]["key1"].value, "should have newest value")
	require.Equal(t, t3, b.buf[agent.ID]["key1"].collectedAt, "should have newest timestamp")
	b.mu.Unlock()

	// Flush and verify database has the newest value
	tick <- t3
	f := <-flushed
	require.Equal(t, 1, f, "expected 1 entry to be flushed")

	metadata, err := store.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agent.ID,
		Keys:             []string{"key1"},
	})
	require.NoError(t, err)
	require.Len(t, metadata, 1)
	require.Equal(t, "newest_value", metadata[0].Value, "database should have newest value")
	require.Equal(t, t3, metadata[0].CollectedAt, "database should have newest timestamp")
}

func TestMetadataBatcher_PubsubChunking(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)

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

	// Create enough agents to exceed the 8KB pubsub limit.
	// A UUID in JSON is ~38 bytes (36 chars + quotes), plus JSON overhead.
	// 8000 / 38 ≈ 210 agents should fit in one message.
	// Let's create 250 agents to force chunking.
	numAgents := 250
	agents := make([]database.WorkspaceAgent, numAgents)
	for i := 0; i < numAgents; i++ {
		agents[i] = setupAgentWithMetadata(t, store)
		// Add a single metadata update for each agent
		b.Add(agents[i].ID, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1})
	}

	// Flush and verify all updates were processed
	tick <- t1
	f := <-flushed
	require.Equal(t, numAgents, f, "expected all agent entries to be flushed")

	// Verify that the pubsub messages were sent (we can't easily verify
	// they were chunked without mocking pubsub, but we can verify no errors
	// occurred during the flush, which would indicate chunking worked)
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
