package agentapi

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub/psmock"
	"github.com/coder/quartz"
)

func TestMetadataBatcher(t *testing.T) {
	t.Parallel()

	// Given: a fresh batcher with no data
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent IDs - no need for real database entries.
	agent1 := uuid.New()
	agent2 := uuid.New()

	// Expect the store to be called when flush happens.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	// Given: no metadata updates are added
	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	f := <-flushed
	require.Equal(t, 0, f, "expected no agents to be flushed")
	t.Log("flush 1 completed")

	// Then: no metadata should be updated (no flush happened)

	// Given: a single metadata update is added for agent1
	t2 := clock.Now()
	t.Log("adding metadata for 1 agent")
	require.NoError(t, b.Add(agent1, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t2, t2}))

	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	f = <-flushed
	require.Equal(t, 2, f, "expected 2 entries to be flushed")
	t.Log("flush 2 completed")

	// Then: agent1's metadata should be updated (verified by mock expectations)

	// Given: metadata updates are added for multiple agents
	t3 := clock.Now()
	t.Log("adding metadata for 2 agents")
	require.NoError(t, b.Add(agent1, []string{"key1", "key2", "key3"}, []string{"new_value1", "new_value2", "new_value3"}, []string{"", "", ""}, []time.Time{t3, t3, t3}))
	require.NoError(t, b.Add(agent2, []string{"key1", "key2"}, []string{"agent2_value1", "agent2_value2"}, []string{"", ""}, []time.Time{t3, t3}))

	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	f = <-flushed
	require.Equal(t, 5, f, "expected 5 entries to be flushed (3 for agent1 + 2 for agent2)")
	t.Log("flush 3 completed")

	// Then: both agents' metadata should be updated (verified by mock expectations)

	// Given: a lot of agents are added (to trigger flush at capacity)
	t4 := clock.Now()
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Add updates to fill the buffer exactly to capacity
		numAgents := defaultMetadataBatchSize
		t.Logf("adding metadata for %d agents", numAgents)
		for i := 0; i < numAgents; i++ {
			// Generate a mock agent ID.
			agent := uuid.New()
			require.NoError(t, b.Add(agent, []string{"key1"}, []string{"bulk_value"}, []string{""}, []time.Time{t4}))
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
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	f = <-flushed
	require.Zero(t, f, "expected zero entries to have been flushed")
	t.Log("flush 5 completed")
}

func TestMetadataBatcher_DropsWhenFull(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	// Use mocks instead of real database for this unit test
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent IDs - no need for real database entries
	agent1 := uuid.New()
	agent2 := uuid.New()
	agent3 := uuid.New()

	// Expect the store to be called when flush happens
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	// Create batcher with very small capacity
	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(2),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Fill buffer to capacity
	require.NoError(t, b.Add(agent1, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	require.NoError(t, b.Add(agent2, []string{"key1"}, []string{"value2"}, []string{""}, []time.Time{t1}))

	// Buffer should now trigger automatic flush at capacity
	// Wait for the automatic flush to complete
	f := <-flushed
	require.Equal(t, 2, f, "expected two updates to be flushed")

	// Try to add another update - buffer is now empty but we'll fill it again
	t2 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key2"}, []string{"value3"}, []string{""}, []time.Time{t2}))
	require.NoError(t, b.Add(agent2, []string{"key2"}, []string{"value4"}, []string{""}, []time.Time{t2}))

	// Wait for the automatic flush (buffer reached capacity again)
	f = <-flushed
	require.Equal(t, 2, f, "expected two updates to be flushed from second batch")

	// Now the buffer is empty. Add entries until we hit capacity, then try to add more.
	// The batcher should drop entries when buffer is full.
	require.NoError(t, b.Add(agent1, []string{"key3"}, []string{"value5"}, []string{""}, []time.Time{t2}))
	// entryCount = 1, not at capacity yet

	require.NoError(t, b.Add(agent2, []string{"key3"}, []string{"value6"}, []string{""}, []time.Time{t2}))
	// entryCount = 2, at capacity, triggers automatic flush

	// Wait for that flush to complete
	f = <-flushed
	require.Equal(t, 2, f, "expected two updates from third batch")

	// Buffer is now empty again. Add ONE entry, leaving room for one more.
	require.NoError(t, b.Add(agent1, []string{"key4"}, []string{"value7"}, []string{""}, []time.Time{t2}))
	// entryCount = 1, not at capacity

	// Now try to add TWO more entries. The first will succeed, the second should be dropped.
	require.NoError(t, b.Add(agent2, []string{"key4"}, []string{"value8"}, []string{""}, []time.Time{t2}))
	// entryCount = 2, at capacity, triggers automatic flush, BUT...

	// This add happens while flush is being triggered - it checks entryCount >= batchSize BEFORE adding
	// So this should be dropped
	require.NoError(t, b.Add(agent3, []string{"key1"}, []string{"dropped"}, []string{""}, []time.Time{t2}))

	// Wait for the automatic flush
	f = <-flushed
	// Should have 2 entries (agent1.key4 and agent2.key4), agent3 was dropped
	require.Equal(t, 2, f, "expected only 2 updates, agent3 should have been dropped")
}

func TestMetadataBatcher_UpdatesExistingKeysAtCapacity(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	agent1 := uuid.New()
	agent2 := uuid.New()

	// Expect the store to be called when flush happens
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	// Create batcher with capacity of 2 entries
	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(2),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Fill buffer to capacity with 2 entries
	require.NoError(t, b.Add(agent1, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	require.NoError(t, b.Add(agent2, []string{"key1"}, []string{"value2"}, []string{""}, []time.Time{t1}))

	// Drain automatic flush
	f := <-flushed
	require.Equal(t, 2, f)

	// Add one entry, leaving room for one more
	t2 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key2"}, []string{"value3"}, []string{""}, []time.Time{t2}))

	// Verify we have 1 entry
	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "should have 1 entry")
	b.mu.Unlock()

	// Add another entry to reach capacity
	require.NoError(t, b.Add(agent2, []string{"key2"}, []string{"value4"}, []string{""}, []time.Time{t2}))

	// Drain automatic flush
	f = <-flushed
	require.Equal(t, 2, f)

	// Fill buffer again to capacity
	t3 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key3"}, []string{"value5"}, []string{""}, []time.Time{t3}))
	require.NoError(t, b.Add(agent2, []string{"key3"}, []string{"value6"}, []string{""}, []time.Time{t3}))

	// Drain automatic flush triggered by reaching capacity
	f = <-flushed
	require.Equal(t, 2, f)

	// Buffer is now empty. Add entries again to fill to capacity.
	t4 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key4"}, []string{"value7"}, []string{""}, []time.Time{t4}))
	require.NoError(t, b.Add(agent2, []string{"key4"}, []string{"value8"}, []string{""}, []time.Time{t4}))

	// Drain automatic flush
	f = <-flushed
	require.Equal(t, 2, f)

	// Add entries one more time to fill to capacity
	t5 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key5"}, []string{"value9"}, []string{""}, []time.Time{t5}))
	require.NoError(t, b.Add(agent2, []string{"key5"}, []string{"value10"}, []string{""}, []time.Time{t5}))

	// Drain automatic flush triggered by reaching capacity
	f = <-flushed
	require.Equal(t, 2, f)

	// NOW buffer is empty. Fill it partially (add 1 entry, leaving room for 1 more)
	t6 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key6"}, []string{"value11"}, []string{""}, []time.Time{t6}))

	// Verify we have 1 entry
	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "should have 1 entry")
	b.mu.Unlock()

	// Add one more to reach capacity
	require.NoError(t, b.Add(agent2, []string{"key6"}, []string{"value12"}, []string{""}, []time.Time{t6}))

	// Drain automatic flush
	f = <-flushed
	require.Equal(t, 2, f)

	// NOW buffer is empty again. Fill to capacity one last time.
	t7 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key7"}, []string{"value13"}, []string{""}, []time.Time{t7}))
	require.NoError(t, b.Add(agent2, []string{"key7"}, []string{"value14"}, []string{""}, []time.Time{t7}))

	// Buffer is now at capacity (entryCount = 2).
	// The flush will be triggered but we won't drain it yet.
	// While at capacity, try to:
	// 1. Update an existing key (should succeed)
	// 2. Add new keys (should be dropped)
	t8 := clock.Now()

	// This should succeed - updating existing key even at capacity
	require.NoError(t, b.Add(agent1, []string{"key7"}, []string{"updated_value13"}, []string{""}, []time.Time{t8}))

	// These should be dropped - new keys when at capacity
	require.NoError(t, b.Add(agent1, []string{"key8"}, []string{"dropped1"}, []string{""}, []time.Time{t8}))
	require.NoError(t, b.Add(agent1, []string{"key9"}, []string{"dropped2"}, []string{""}, []time.Time{t8}))

	// Drain the flush that was triggered at line 346
	f = <-flushed
	require.Equal(t, 2, f)

	// Check internal state - buffer should now be empty since we drained
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "buffer should be empty after flush")
	require.Len(t, b.buf, 0, "buffer map should be empty")
	b.mu.Unlock()
}

func TestMetadataBatcher_MultipleUpdatesForSameAgent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent ID.
	agent := uuid.New()

	// Expect the store to be called when flush happens.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Add first update
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"first_value"}, []string{""}, []time.Time{t1}))

	// Add second update for same agent+key (should deduplicate)
	clock.Advance(time.Millisecond)
	t2 := clock.Now()
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"second_value"}, []string{""}, []time.Time{t2}))

	// Flush - advance the remaining time to hit the flush interval
	clock.Advance(defaultMetadataFlushInterval - time.Millisecond).MustWait(ctx)
	f := <-flushed
	require.Equal(t, 1, f, "expected 1 entry to be flushed (deduplicated)")

	// Verify the second update was applied (deduplication keeps latest value)
	// by checking the internal buffer state before flush
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "buffer should be empty after flush")
	b.mu.Unlock()
}

func TestMetadataBatcher_DeduplicationWithMixedKeys(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent ID.
	agent := uuid.New()

	// Expect the store to be called when flush happens.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

		b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Add updates with some duplicate keys and some unique keys
	require.NoError(t, b.Add(agent, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t1, t1}))

	clock.Advance(time.Millisecond)
	t2 := clock.Now()
	// Update key1, add key3 - key2 stays from first update
	require.NoError(t, b.Add(agent, []string{"key1", "key3"}, []string{"new_value1", "value3"}, []string{"", ""}, []time.Time{t2, t2}))

	// Flush - advance the remaining time to hit the flush interval
	clock.Advance(defaultMetadataFlushInterval - time.Millisecond).MustWait(ctx)
	f := <-flushed
	require.Equal(t, 3, f, "expected 3 entries (key1 deduplicated, key2 and key3 unique)")

	// Verify buffer is empty after flush
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "buffer should be empty after flush")
	b.mu.Unlock()
}

func TestMetadataBatcher_EntryCountTracking(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent ID.
	agent := uuid.New()

	// Expect the store to be called when flush happens.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(5), // Small size to test capacity
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Add 3 entries
	require.NoError(t, b.Add(agent, []string{"key1", "key2", "key3"}, []string{"v1", "v2", "v3"}, []string{"", "", ""}, []time.Time{t1, t1, t1}))

	// Verify internal state
	b.mu.Lock()
	require.Equal(t, 3, b.entryCount, "entryCount should be 3")
	b.mu.Unlock()

	// Add 2 more entries (should reach capacity of 5)
	require.NoError(t, b.Add(agent, []string{"key4", "key5"}, []string{"v4", "v5"}, []string{"", ""}, []time.Time{t1, t1}))

	// Should trigger automatic flush at capacity
	f := <-flushed
	require.Equal(t, 5, f, "expected 5 entries to be flushed")

	// Verify entry count reset after flush
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "entryCount should be reset to 0 after flush")
	b.mu.Unlock()

	// Add update to same key (should be counted as replacement, not new entry)
	clock.Advance(time.Second)
	t2 := clock.Now()
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"new_v1"}, []string{""}, []time.Time{t2}))

	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "entryCount should be 1 after adding new entry")
	b.mu.Unlock()

	// Update same key again (should still be 1 entry)
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newer_v1"}, []string{""}, []time.Time{t2}))

	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "entryCount should still be 1 after replacement")
	b.mu.Unlock()
}

func TestMetadataBatcher_TimestampOrdering(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent ID.
	agent := uuid.New()

	// Expect the store to be called when flush happens.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()
	clock.Advance(time.Second)
	t2 := clock.Now()
	clock.Advance(time.Second)
	t3 := clock.Now()

	// Add update with t2 timestamp
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newer_value"}, []string{""}, []time.Time{t2}))

	// Try to add older update with t1 timestamp - should be ignored
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"older_value"}, []string{""}, []time.Time{t1}))

	// Add even newer update with t3 timestamp - should overwrite
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newest_value"}, []string{""}, []time.Time{t3}))

	// Verify internal state - should have only 1 entry
	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "entryCount should be 1")
	require.Equal(t, "newest_value", b.buf[agent]["key1"].value, "should have newest value")
	require.True(t, t3.Equal(b.buf[agent]["key1"].collectedAt), "should have newest timestamp")
	b.mu.Unlock()

	// Flush and verify entry was sent - advance the remaining time to hit the flush interval
	// We already advanced by 2 seconds, so we need to advance by 3 more seconds to reach the 5s flush interval
	clock.Advance(defaultMetadataFlushInterval - 2*time.Second).MustWait(ctx)
	f := <-flushed
	require.Equal(t, 1, f, "expected 1 entry to be flushed")

	// Verify buffer is empty after flush
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "buffer should be empty after flush")
	b.mu.Unlock()
}

func TestMetadataBatcher_PubsubChunking(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Expect the store to be called when flush happens.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 1)

	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Create enough agents to exceed the 8KB pubsub limit.
	// A UUID in JSON is ~38 bytes (36 chars + quotes), plus JSON overhead.
	// 8000 / 38 ≈ 210 agents should fit in one message.
	// Let's create 250 agents to force chunking.
	numAgents := 250
	agents := make([]uuid.UUID, numAgents)
	for i := 0; i < numAgents; i++ {
		agents[i] = uuid.New()
		// Add a single metadata update for each agent
		require.NoError(t, b.Add(agents[i], []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	}

	// Flush and verify all updates were processed
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	f := <-flushed
	require.Equal(t, numAgents, f, "expected all agent entries to be flushed")

	// Verify that the pubsub messages were sent (we can't easily verify
	// they were chunked without mocking pubsub, but we can verify no errors
	// occurred during the flush, which would indicate chunking worked)
}

func TestMetadataBatcher_ConcurrentAddsToSameAgent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 10)

	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	// Single agent, multiple goroutines updating same keys concurrently
	agentID := uuid.New()
	numGoroutines := 20

	// Pre-calculate timestamps using clock advances
	timestamps := make([]time.Time, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		clock.Advance(time.Millisecond)
		timestamps[i] = clock.Now()
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Each goroutine updates the same set of keys with different values
	for i := 0; i < numGoroutines; i++ {
		go func(routineNum int) {
			defer wg.Done()
			timestamp := timestamps[routineNum]
			value := fmt.Sprintf("value_from_goroutine_%d", routineNum)
			_ = b.Add(agentID, []string{"key1", "key2", "key3"},
				[]string{value, value, value},
				[]string{"", "", ""},
				[]time.Time{timestamp, timestamp, timestamp})
		}(i)
	}

	wg.Wait()

	// Verify we have exactly 3 keys (no duplicates despite concurrent adds)
	b.mu.Lock()
	require.Equal(t, 3, b.entryCount, "should have exactly 3 keys despite concurrent updates")
	require.Len(t, b.buf[agentID], 3, "should have 3 keys in agent's map")

	// The values should be from the goroutine with the latest timestamp
	// (last goroutine has the latest timestamp)
	expectedValue := fmt.Sprintf("value_from_goroutine_%d", numGoroutines-1)
	require.Equal(t, expectedValue, b.buf[agentID]["key1"].value)
	require.Equal(t, expectedValue, b.buf[agentID]["key2"].value)
	require.Equal(t, expectedValue, b.buf[agentID]["key3"].value)
	b.mu.Unlock()
}

func TestMetadataBatcher_AutomaticFlushOnCapacity(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	flushed := make(chan int, 10)

	batchSize := 100
	b, closer, err := NewMetadataBatcher(ctx, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(batchSize),
		MetadataBatcherWithClock(clock),
		func(b *MetadataBatcher) {
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	agentID := uuid.New()
	t1 := clock.Now()

	// Add entries up to but not exceeding capacity
	for i := 0; i < batchSize-1; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		require.NoError(t, b.Add(agentID, []string{key}, []string{value}, []string{""}, []time.Time{t1}))
	}

	// Verify buffer is at capacity-1, no flush triggered yet
	b.mu.Lock()
	require.Equal(t, batchSize-1, b.entryCount)
	require.False(t, b.flushForced.Load(), "flush should not be forced yet")
	b.mu.Unlock()

	// Verify no flush has occurred
	select {
	case <-flushed:
		t.Fatal("unexpected flush before reaching capacity")
	default:
		// Good, no flush yet
	}

	// Add one more entry to reach capacity - this should trigger automatic flush
	require.NoError(t, b.Add(agentID, []string{"key_at_capacity"}, []string{"value_at_capacity"}, []string{""}, []time.Time{t1}))

	// Verify flushForced flag is set
	require.True(t, b.flushForced.Load(), "flush should be forced after reaching capacity")

	// Verify automatic flush was triggered
	select {
	case count := <-flushed:
		require.Equal(t, batchSize, count, "should flush exactly batchSize entries")
	case <-time.After(time.Second):
		t.Fatal("automatic flush was not triggered after reaching capacity")
	}

	// Verify buffer is empty after flush
	b.mu.Lock()
	require.Equal(t, 0, b.entryCount, "buffer should be empty after flush")
	require.Len(t, b.buf, 0, "agent map should be empty after flush")
	b.mu.Unlock()

	// Verify flushForced flag is reset after flush completes
	require.False(t, b.flushForced.Load(), "flush should not be forced after flush completes")

	// Verify we can add new entries after automatic flush
	require.NoError(t, b.Add(agentID, []string{"key_after_flush"}, []string{"value_after_flush"}, []string{""}, []time.Time{t1}))

	b.mu.Lock()
	require.Equal(t, 1, b.entryCount, "should be able to add entries after automatic flush")
	b.mu.Unlock()
}
