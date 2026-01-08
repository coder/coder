package agentapi

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub/psmock"
	"github.com/coder/coder/v2/testutil"
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

	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	// Given: no metadata updates are added
	// When: it becomes time to flush
	// Then: no metadata should be updated (flush() returns 0)
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	t.Log("flush 1 completed (expected 0 entries)")
	require.Equal(t, float64(0), prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker)))

	// Given: a single metadata update is added for agent1
	t2 := clock.Now()
	t.Log("adding metadata for 1 agent")
	require.NoError(t, b.Add(agent1, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t2, t2}))

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// When: it becomes time to flush
	// Then: agent1's metadata should be updated (verified by mock expectations)
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	t.Log("flush 2 completed (expected 2 entries)")
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		// Check that we've had 1 scheduled flush and 2 metadata entries flushed
		return float64(1) == val && totalMeta >= float64(2)
	}, testutil.IntervalFast)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Given: metadata updates are added for multiple agents
	t3 := clock.Now()
	t.Log("adding metadata for 2 agents")
	require.NoError(t, b.Add(agent1, []string{"key1", "key2", "key3"}, []string{"new_value1", "new_value2", "new_value3"}, []string{"", "", ""}, []time.Time{t3, t3, t3}))
	require.NoError(t, b.Add(agent2, []string{"key1", "key2"}, []string{"agent2_value1", "agent2_value2"}, []string{"", ""}, []time.Time{t3, t3}))

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// When: it becomes time to flush
	// Then: both agents' metadata should be updated (verified by mock expectations)
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	t.Log("flush 3 completed (expected 5 new entries)")
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		fmt.Printf("val=%v, totalMeta=%v\n", val, totalMeta)
		// Check that we've had 2 scheduled flushes and that the total metadata count is at least 7
		// We use Eventually because the flush might not complete immediately after clock advance
		return float64(2) == val && totalMeta >= float64(7)
	}, testutil.IntervalFast)
	require.Equal(t, float64(7), prom_testutil.ToFloat64(b.metrics.metadataTotal))

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

	// Wait for all updates to be added
	<-done
	t.Log("flush 4 completed (capacity flush, expected", defaultMetadataBatchSize, "entries)")
	t.Log("flush 5 completed (expected 0 entries)")
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fmt.Println("val", prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker)))
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(507), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	reg := prometheus.NewRegistry()
	// Create batcher with very small capacity
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(2),
		MetadataBatcherWithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Fill buffer to capacity
	require.NoError(t, b.Add(agent1, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	require.NoError(t, b.Add(agent2, []string{"key1"}, []string{"value2"}, []string{""}, []time.Time{t1}))

	// Buffer should now trigger automatic flush at capacity
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Try to add another update - buffer is now empty but we'll fill it again
	t2 := clock.Now()
	require.NoError(t, b.Add(agent1, []string{"key2"}, []string{"value3"}, []string{""}, []time.Time{t2}))
	require.NoError(t, b.Add(agent2, []string{"key2"}, []string{"value4"}, []string{""}, []time.Time{t2}))

	// Wait for the second automatic flush
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(2) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(4), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Add first update
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"first_value"}, []string{""}, []time.Time{t1}))

	// Add second update for same agent+key (should deduplicate)
	clock.Advance(time.Millisecond).MustWait(ctx)
	t2 := clock.Now()
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"second_value"}, []string{""}, []time.Time{t2}))

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// Flush - advance the remaining time to hit the flush interval
	clock.Advance(defaultMetadataFlushInterval - time.Millisecond).MustWait(ctx)

	// Verify deduplication - only 1 entry should be flushed
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()

	// Add updates with some duplicate keys and some unique keys
	require.NoError(t, b.Add(agent, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t1, t1}))

	clock.Advance(time.Millisecond).MustWait(ctx)
	t2 := clock.Now()
	// Update key1, add key3 - key2 stays from first update
	require.NoError(t, b.Add(agent, []string{"key1", "key3"}, []string{"new_value1", "value3"}, []string{"", ""}, []time.Time{t2, t2}))

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// Flush - advance the remaining time to hit the flush interval
	clock.Advance(defaultMetadataFlushInterval - time.Millisecond).MustWait(ctx)

	// Verify deduplication - 3 unique keys (key1 deduplicated, key2 and key3 unique)
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(3), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	t1 := clock.Now()
	clock.Advance(time.Second).MustWait(ctx)
	t2 := clock.Now()
	clock.Advance(time.Second).MustWait(ctx)
	t3 := clock.Now()

	// Add update with t2 timestamp
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newer_value"}, []string{""}, []time.Time{t2}))

	// Try to add older update with t1 timestamp - should be ignored
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"older_value"}, []string{""}, []time.Time{t1}))

	// Add even newer update with t3 timestamp - should overwrite
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newest_value"}, []string{""}, []time.Time{t3}))

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// Flush and verify entry was sent - advance the remaining time to hit the flush interval
	// We already advanced by 2 seconds, so we need to advance by 3 more seconds to reach the 5s flush interval
	clock.Advance(defaultMetadataFlushInterval - 2*time.Second).MustWait(ctx)

	// Verify only 1 entry was flushed (newest timestamp wins)
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
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

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// Flush and verify all updates were processed
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Verify that all metadata was flushed successfully
	// Use Eventually to handle async flush completion
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		// Check that we've had 1 scheduled flush and all metadata was flushed
		return float64(1) == val && totalMeta >= float64(numAgents)
	}, testutil.IntervalFast)
	require.Equal(t, float64(numAgents), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	// Single agent, multiple goroutines updating same keys concurrently
	agentID := uuid.New()
	numGoroutines := 20

	// Pre-calculate timestamps using clock advances
	timestamps := make([]time.Time, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		clock.Advance(time.Millisecond).MustWait(ctx)
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

	// Wait for all channel messages to be processed by the run() goroutine
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// Flush and check that we have exactly 3 keys (deduplication worked)
	// We advanced the clock by numGoroutines milliseconds above, so advance by the remaining time
	remainingTime := defaultMetadataFlushInterval - time.Duration(numGoroutines)*time.Millisecond
	clock.Advance(remainingTime).MustWait(ctx)

	// Verify exactly 3 unique keys were flushed
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(3), prom_testutil.ToFloat64(b.metrics.metadataTotal))
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

	batchSize := 100
	reg := prometheus.NewRegistry()
	b, closer, err := NewMetadataBatcher(ctx, reg, store, ps,
		MetadataBatcherWithLogger(log),
		MetadataBatcherWithBatchSize(batchSize),
		MetadataBatcherWithClock(clock),
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

	// Verify no flush has occurred yet
	ctx = testutil.Context(t, testutil.WaitShort)
	require.Equal(t, float64(0), prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity)))

	// Add one more entry to reach capacity - this should trigger automatic flush
	require.NoError(t, b.Add(agentID, []string{"key_at_capacity"}, []string{"value_at_capacity"}, []string{""}, []time.Time{t1}))

	// Wait for automatic flush
	ctx = testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(batchSize), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Verify we can add new entries after automatic flush
	require.NoError(t, b.Add(agentID, []string{"key_after_flush"}, []string{"value_after_flush"}, []string{""}, []time.Time{t1}))
}
