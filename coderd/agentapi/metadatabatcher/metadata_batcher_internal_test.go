package metadatabatcher

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub/psmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// ============================================================================
// Custom gomock matchers for metadata batcher testing
// ============================================================================

// metadataParamsMatcher validates BatchUpdateWorkspaceAgentMetadataParams
// by checking all fields match expected values.
type metadataParamsMatcher struct {
	expectedAgentIDs []uuid.UUID
	expectedKeys     []string
	expectedValues   []string
	expectedErrors   []string
	expectedTimes    []time.Time
}

func (m metadataParamsMatcher) Matches(x interface{}) bool {
	params, ok := x.(database.BatchUpdateWorkspaceAgentMetadataParams)
	if !ok {
		return false
	}

	// All arrays must have the same length.
	expectedLen := len(m.expectedKeys)
	if len(params.WorkspaceAgentID) != expectedLen ||
		len(params.Key) != expectedLen ||
		len(params.Value) != expectedLen ||
		len(params.Error) != expectedLen ||
		len(params.CollectedAt) != expectedLen {
		return false
	}

	// Check each field matches expected values. We create a map of expected
	// entries and verify all actual entries match, handling any order.
	expectedEntries := make(map[string]bool)
	for i := 0; i < len(m.expectedKeys); i++ {
		key := fmt.Sprintf("%s|%s|%s|%s|%s",
			m.expectedAgentIDs[i].String(),
			m.expectedKeys[i],
			m.expectedValues[i],
			m.expectedErrors[i],
			m.expectedTimes[i].Format(time.RFC3339Nano))
		expectedEntries[key] = false // not yet found
	}

	// Check all actual entries are expected.
	for i := 0; i < len(params.Key); i++ {
		key := fmt.Sprintf("%s|%s|%s|%s|%s",
			params.WorkspaceAgentID[i].String(),
			params.Key[i],
			params.Value[i],
			params.Error[i],
			params.CollectedAt[i].Format(time.RFC3339Nano))

		if _, exists := expectedEntries[key]; !exists {
			return false
		}
		expectedEntries[key] = true
	}

	// Check all expected entries were found.
	for _, found := range expectedEntries {
		if !found {
			return false
		}
	}

	return true
}

func (m metadataParamsMatcher) String() string {
	return fmt.Sprintf("metadata params with %d entries (agents: %v, keys: %v)",
		len(m.expectedKeys), m.expectedAgentIDs, m.expectedKeys)
}

// matchMetadata creates a matcher that checks all values in the metadata params.
func matchMetadata(agentIDs []uuid.UUID, keys, values, errors []string, times []time.Time) gomock.Matcher {
	return metadataParamsMatcher{
		expectedAgentIDs: agentIDs,
		expectedKeys:     keys,
		expectedValues:   values,
		expectedErrors:   errors,
		expectedTimes:    times,
	}
}

// pubsubCapture captures and decodes pubsub publish calls to accumulate agent IDs.
type pubsubCapture struct {
	mu       sync.Mutex
	agentIDs map[uuid.UUID]bool
}

func newPubsubCapture() *pubsubCapture {
	return &pubsubCapture{
		agentIDs: make(map[uuid.UUID]bool),
	}
}

func (c *pubsubCapture) capture(event string, message []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Verify correct event.
	if event != MetadataBatchPubsubChannel {
		return xerrors.Errorf("unexpected event: %s", event)
	}

	// Decode base64-encoded agent IDs from payload.
	if len(message)%uuidBase64Size != 0 {
		return xerrors.Errorf("invalid payload size: %d", len(message))
	}

	numAgents := len(message) / uuidBase64Size
	for i := 0; i < numAgents; i++ {
		start := i * uuidBase64Size
		end := start + uuidBase64Size
		encoded := message[start:end]

		var uuidBytes [16]byte
		n, err := base64.RawStdEncoding.Decode(uuidBytes[:], encoded)
		if err != nil || n != 16 {
			return xerrors.Errorf("failed to decode UUID: %w", err)
		}

		agentID, err := uuid.FromBytes(uuidBytes[:])
		if err != nil {
			return xerrors.Errorf("failed to parse UUID: %w", err)
		}

		c.agentIDs[agentID] = true
	}

	return nil
}

func (c *pubsubCapture) assertContainsAll(t *testing.T, expected []uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check all expected IDs are present.
	for _, expectedID := range expected {
		require.True(t, c.agentIDs[expectedID], "expected agent ID %s not found in pubsub messages", expectedID)
	}

	// Check we don't have extra IDs.
	require.Equal(t, len(expected), len(c.agentIDs), "unexpected number of agent IDs in pubsub messages")
}

func (c *pubsubCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.agentIDs)
}

func TestMetadataBatcher(t *testing.T) {
	t.Parallel()

	// Given: a fresh batcher with no data
	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent IDs.
	agent1 := uuid.New()
	agent2 := uuid.New()

	// --- FLUSH 1: Empty flush (no calls expected) ---
	// No expectations set - if DB query called, test will fail.
	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	// Given: no metadata updates are added
	// When: it becomes time to flush
	// Then: no metadata should be updated (no DB call)
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	t.Log("flush 1 completed (expected 0 entries)")
	require.Equal(t, float64(0), prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker)))

	// --- FLUSH 2: Single agent with 2 metadata entries ---
	t2 := clock.Now()

	// Capture pubsub publish calls for this flush.
	pubsubCap2 := newPubsubCapture()

	// Expect exactly 1 database call with exact values.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent1, agent1},
				[]string{"key1", "key2"},
				[]string{"value1", "value2"},
				[]string{"", ""},
				[]time.Time{t2, t2},
			),
		).
		Return(nil).
		Times(1)

	// Expect exactly 1 pubsub publish with correct event and agent IDs.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap2.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Given: a single metadata update is added for agent1
	t.Log("adding metadata for 1 agent")
	require.NoError(t, b.Add(agent1, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t2, t2}))

	// Wait for all channel messages to be processed into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 2
	}, testutil.IntervalFast)

	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	t.Log("flush 2 completed (expected 2 entries)")
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		return float64(1) == val && totalMeta >= float64(2)
	}, testutil.IntervalFast)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap2.count() == 1
	}, testutil.IntervalFast)
	pubsubCap2.assertContainsAll(t, []uuid.UUID{agent1})

	// --- FLUSH 3: Multiple agents with 5 total metadata entries ---
	t3 := clock.Now()

	// Capture pubsub publish calls for this flush.
	pubsubCap3 := newPubsubCapture()

	// Expect exactly 1 database call with exact values for both agents.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent1, agent1, agent1, agent2, agent2},
				[]string{"key1", "key2", "key3", "key1", "key2"},
				[]string{"new_value1", "new_value2", "new_value3", "agent2_value1", "agent2_value2"},
				[]string{"", "", "", "", ""},
				[]time.Time{t3, t3, t3, t3, t3},
			),
		).
		Return(nil).
		Times(1)

	// Expect exactly 1 pubsub publish with both agent IDs.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap3.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Given: metadata updates are added for multiple agents
	t.Log("adding metadata for 2 agents")
	require.NoError(t, b.Add(agent1, []string{"key1", "key2", "key3"}, []string{"new_value1", "new_value2", "new_value3"}, []string{"", "", ""}, []time.Time{t3, t3, t3}))
	require.NoError(t, b.Add(agent2, []string{"key1", "key2"}, []string{"agent2_value1", "agent2_value2"}, []string{"", ""}, []time.Time{t3, t3}))

	// Wait for all channel messages to be processed into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 5
	}, testutil.IntervalFast)

	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	t.Log("flush 3 completed (expected 5 new entries)")
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		return float64(2) == val && totalMeta >= float64(7)
	}, testutil.IntervalFast)
	require.Equal(t, float64(7), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap3.count() == 2
	}, testutil.IntervalFast)
	pubsubCap3.assertContainsAll(t, []uuid.UUID{agent1, agent2})

	// --- FLUSH 4: Capacity flush with defaultMetadataBatchSize entries ---
	t4 := clock.Now()
	numAgents := defaultMetadataBatchSize

	// Pre-generate all agent IDs so we can assert on exact values.
	agentIDs := make([]uuid.UUID, numAgents)
	for i := 0; i < numAgents; i++ {
		agentIDs[i] = uuid.New()
	}

	// Build expected values for database assertion.
	expectedKeys := make([]string, numAgents)
	expectedValues := make([]string, numAgents)
	expectedErrors := make([]string, numAgents)
	expectedTimes := make([]time.Time, numAgents)
	for i := 0; i < numAgents; i++ {
		expectedKeys[i] = "key1"
		expectedValues[i] = "bulk_value"
		expectedErrors[i] = ""
		expectedTimes[i] = t4
	}

	// Capture pubsub publish calls for this flush.
	pubsubCap4 := newPubsubCapture()

	// Assert on exact database values.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(agentIDs, expectedKeys, expectedValues, expectedErrors, expectedTimes),
		).
		Return(nil).
		Times(1)

	// Pubsub will be called with chunking.
	// With 500 agents, we expect exactly 2 pubsub calls due to chunking (363 + 137).
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap4.capture(event, message))
		}).
		Return(nil).
		Times(2)

	// Add metadata updates using the pre-generated agent IDs.
	done := make(chan struct{})

	go func() {
		defer close(done)
		t.Logf("adding metadata for %d agents", numAgents)
		for i := 0; i < numAgents; i++ {
			require.NoError(t, b.Add(agentIDs[i], []string{"key1"}, []string{"bulk_value"}, []string{""}, []time.Time{t4}))
		}
	}()

	// Wait for all updates to be added
	<-done
	t.Log("flush 4 completed (capacity flush, expected", defaultMetadataBatchSize, "entries)")
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(507), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published (across all chunks).
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap4.count() == numAgents
	}, testutil.IntervalFast)
	pubsubCap4.assertContainsAll(t, agentIDs)
}

func TestMetadataBatcher_DropsWhenFull(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent IDs
	agent1 := uuid.New()
	agent2 := uuid.New()

	reg := prometheus.NewRegistry()
	// Create batcher with very small capacity
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithBatchSize(2),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	// --- FLUSH 1: First capacity flush with 2 entries ---
	t1 := clock.Now()
	pubsubCap1 := newPubsubCapture()

	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent1, agent2},
				[]string{"key1", "key1"},
				[]string{"value1", "value2"},
				[]string{"", ""},
				[]time.Time{t1, t1},
			),
		).
		Return(nil).
		Times(1)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap1.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Fill buffer to capacity
	require.NoError(t, b.Add(agent1, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	require.NoError(t, b.Add(agent2, []string{"key1"}, []string{"value2"}, []string{""}, []time.Time{t1}))

	// Buffer should now trigger automatic flush at capacity
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap1.count() == 2
	}, testutil.IntervalFast)
	pubsubCap1.assertContainsAll(t, []uuid.UUID{agent1, agent2})

	// --- FLUSH 2: Second capacity flush with 2 different entries ---
	t2 := clock.Now()
	pubsubCap2 := newPubsubCapture()

	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent1, agent2},
				[]string{"key2", "key2"},
				[]string{"value3", "value4"},
				[]string{"", ""},
				[]time.Time{t2, t2},
			),
		).
		Return(nil).
		Times(1)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap2.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Try to add another update - buffer is now empty but we'll fill it again
	require.NoError(t, b.Add(agent1, []string{"key2"}, []string{"value3"}, []string{""}, []time.Time{t2}))
	require.NoError(t, b.Add(agent2, []string{"key2"}, []string{"value4"}, []string{""}, []time.Time{t2}))

	// Wait for the second automatic flush
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(2) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(4), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap2.count() == 2
	}, testutil.IntervalFast)
	pubsubCap2.assertContainsAll(t, []uuid.UUID{agent1, agent2})
}

func TestMetadataBatcher_MultipleUpdatesForSameAgent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent ID.
	agent := uuid.New()

	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	t1 := clock.Now()
	t2 := t1.Add(time.Millisecond)

	// --- FLUSH: Scheduled flush with deduplicated entry (only most recent value) ---
	pubsubCap := newPubsubCapture()

	// Expect exactly 1 entry with the second (newer) value due to deduplication.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent},
				[]string{"key1"},
				[]string{"second_value"},
				[]string{""},
				[]time.Time{t2},
			),
		).
		Return(nil).
		Times(1)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Add first update
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"first_value"}, []string{""}, []time.Time{t1}))

	// Add second update for same agent+key (should deduplicate)
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"second_value"}, []string{""}, []time.Time{t2}))

	// Wait for all channel messages to be processed by the run() goroutine into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 1
	}, testutil.IntervalFast)

	// Flush - advance the full flush interval from when the batcher was created.
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Verify deduplication - only 1 entry should be flushed
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify agent ID was published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap.count() == 1
	}, testutil.IntervalFast)
	pubsubCap.assertContainsAll(t, []uuid.UUID{agent})
}

func TestMetadataBatcher_DeduplicationWithMixedKeys(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	// Generate mock agent ID.
	agent := uuid.New()

	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	t1 := clock.Now()
	t2 := t1.Add(time.Millisecond)

	// --- FLUSH: Scheduled flush with 3 deduplicated entries ---
	pubsubCap := newPubsubCapture()

	// Expect 3 entries: key1 (updated to new_value1), key2 (from first add), key3 (new).
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent, agent, agent},
				[]string{"key1", "key2", "key3"},
				[]string{"new_value1", "value2", "value3"},
				[]string{"", "", ""},
				[]time.Time{t2, t1, t2},
			),
		).
		Return(nil).
		Times(1)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Add updates with some duplicate keys and some unique keys
	require.NoError(t, b.Add(agent, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t1, t1}))

	// Update key1, add key3 - key2 stays from first update
	require.NoError(t, b.Add(agent, []string{"key1", "key3"}, []string{"new_value1", "value3"}, []string{"", ""}, []time.Time{t2, t2}))

	// Wait for all channel messages to be processed by the run() goroutine into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 3
	}, testutil.IntervalFast)

	// Flush - advance the full flush interval from when the batcher was created.
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Verify deduplication - 3 unique keys (key1 deduplicated, key2 and key3 unique)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(3), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify agent ID was published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap.count() == 1
	}, testutil.IntervalFast)
	pubsubCap.assertContainsAll(t, []uuid.UUID{agent})
}

func TestMetadataBatcher_TimestampOrdering(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	// Generate mock agent ID.
	agent := uuid.New()

	t1 := clock.Now()
	t2 := t1.Add(time.Second)
	t3 := t2.Add(time.Second)

	// Set up pubsub capture for the flush.
	pubsubCap := newPubsubCapture()

	// Expect the store to be called with only the newest timestamp.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agent},
				[]string{"key1"},
				[]string{"newest_value"},
				[]string{""},
				[]time.Time{t3},
			),
		).
		Return(nil).
		Times(1)

	// Expect pubsub publish to be called when flush happens.
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Add update with t2 timestamp
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newer_value"}, []string{""}, []time.Time{t2}))

	// Try to add older update with t1 timestamp - should be ignored
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"older_value"}, []string{""}, []time.Time{t1}))

	// Add even newer update with t3 timestamp - should overwrite
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newest_value"}, []string{""}, []time.Time{t3}))

	// Wait for all channel messages to be processed by the run() goroutine into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 1
	}, testutil.IntervalFast)

	// Flush and verify entry was sent.
	// Advance the full flush interval from when the batcher was created.
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap.count() == 1
	}, testutil.IntervalFast)
	pubsubCap.assertContainsAll(t, []uuid.UUID{agent})

	// Verify only 1 entry was flushed (newest timestamp wins)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(b.metrics.metadataTotal))
}

func TestMetadataBatcher_PubsubChunking(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	t1 := clock.Now()

	// Create enough agents to exceed maxAgentIDsPerChunk.
	// With base64 encoding, each UUID is 22 characters, so we can fit
	// ~363 agent IDs per chunk (8000 / 22 = 363.6).
	// Let's create 600 agents to force chunking into 2 messages.
	numAgents := 600
	agents := make([]uuid.UUID, numAgents)
	expectedKeys := make([]string, numAgents)
	expectedValues := make([]string, numAgents)
	expectedErrors := make([]string, numAgents)
	expectedTimes := make([]time.Time, numAgents)

	for i := 0; i < numAgents; i++ {
		agents[i] = uuid.New()
		expectedKeys[i] = "key1"
		expectedValues[i] = "value1"
		expectedErrors[i] = ""
		expectedTimes[i] = t1
	}

	// Set up pubsub capture for the flush.
	pubsubCap := newPubsubCapture()

	// With 600 agents and default batch size of 500:
	// - First flush at 500 agents (capacity): 2 pubsub chunks (363 + 137)
	// - Second flush at 100 agents (scheduled): 1 pubsub chunk
	// Total: 3 publishes, 2 store calls

	// Expect the store to be called twice - once for first 500, once for remaining 100.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				agents[:500],
				expectedKeys[:500],
				expectedValues[:500],
				expectedErrors[:500],
				expectedTimes[:500],
			),
		).
		Return(nil).
		Times(1)

	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				agents[500:],
				expectedKeys[500:],
				expectedValues[500:],
				expectedErrors[500:],
				expectedTimes[500:],
			),
		).
		Return(nil).
		Times(1)

	// Expect pubsub publish to be called when flush happens.
	// With base64 encoding, each UUID is 22 characters.
	// With 8KB limit, we can fit ~363 agents per chunk (8000 / 22 = 363.6).
	// With 600 agents and batch size of 500:
	// - First flush at 500 agents: 2 chunks (363 + 137)
	// - Second flush at 100 agents: 1 chunk
	// Total: 3 publishes
	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap.capture(event, message))
		}).
		Return(nil).
		Times(3)

	// Add first 499 metadata updates (just under the capacity threshold of 500)
	for i := 0; i < 499; i++ {
		require.NoError(t, b.Add(agents[i], []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	}

	// Wait for all channel messages to be processed into the batch.
	// Batch should have 499 entries, no capacity flush yet.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 499
	}, testutil.IntervalFast)

	// Add next 101 metadata updates (will trigger capacity flush at 500)
	for i := 499; i < numAgents; i++ {
		require.NoError(t, b.Add(agents[i], []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	}

	// Wait for all channel messages to be processed. The 500th entry should have
	// triggered an automatic capacity flush, leaving 100 entries in the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 100
	}, testutil.IntervalFast)

	// Verify capacity flush metrics and total metadata count.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		capacity := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		// Should have 1 capacity flush (500 entries) so far
		return capacity == float64(1) && totalMeta == float64(500)
	}, testutil.IntervalFast)

	// Flush remaining entries and verify all updates were processed
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap.count() == numAgents
	}, testutil.IntervalFast)
	pubsubCap.assertContainsAll(t, agents)

	// Verify that all metadata was flushed successfully.
	// We should have 1 capacity flush (500 entries) and 1 scheduled flush (100 entries).
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		capacity := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
		scheduled := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		// Check that we've had 1 capacity flush and 1 scheduled flush
		return capacity == float64(1) && scheduled == float64(1) && totalMeta == float64(600)
	}, testutil.IntervalFast)
	require.Equal(t, float64(numAgents), prom_testutil.ToFloat64(b.metrics.metadataTotal))
}

func TestMetadataBatcher_ConcurrentAddsToSameAgent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	// Single agent, multiple goroutines updating same keys concurrently
	agentID := uuid.New()
	numGoroutines := 20

	// Pre-calculate timestamps using clock advances
	timestamps := make([]time.Time, numGoroutines)
	initialTS := clock.Now()
	for i := 0; i < numGoroutines; i++ {
		timestamps[i] = initialTS.Add(time.Duration(i) * time.Millisecond)
	}

	// The latest timestamp will have the final values, since deduplication
	// keeps the newest value for each key.
	latestTimestamp := timestamps[numGoroutines-1]
	latestValue := fmt.Sprintf("value_from_goroutine_%d", numGoroutines-1)

	// Set up pubsub capture for the flush.
	pubsubCap := newPubsubCapture()

	// Expect the store to be called with exactly 3 keys (after deduplication).
	// The values should be from the goroutine with the latest timestamp.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				[]uuid.UUID{agentID, agentID, agentID},
				[]string{"key1", "key2", "key3"},
				[]string{latestValue, latestValue, latestValue},
				[]string{"", "", ""},
				[]time.Time{latestTimestamp, latestTimestamp, latestTimestamp},
			),
		).
		Return(nil).
		Times(1)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap.capture(event, message))
		}).
		Return(nil).
		Times(1)

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

	// Wait for all channel messages to be processed by the run() goroutine into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0 && len(b.batch) == 3
	}, testutil.IntervalFast)

	// Flush and check that we have exactly 3 keys (deduplication worked).
	// Advance the full flush interval from when the batcher was created.
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap.count() == 1
	}, testutil.IntervalFast)
	pubsubCap.assertContainsAll(t, []uuid.UUID{agentID})

	// Verify exactly 3 unique keys were flushed
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(3), prom_testutil.ToFloat64(b.metrics.metadataTotal))
}

func TestMetadataBatcher_AutomaticFlushOnCapacity(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	batchSize := 100
	reg := prometheus.NewRegistry()
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithBatchSize(batchSize),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	agentID := uuid.New()
	t1 := clock.Now()

	// Pre-generate all expected data for the capacity flush.
	expectedAgentIDs := make([]uuid.UUID, batchSize)
	expectedKeys := make([]string, batchSize)
	expectedValues := make([]string, batchSize)
	expectedErrors := make([]string, batchSize)
	expectedTimes := make([]time.Time, batchSize)

	for i := 0; i < batchSize-1; i++ {
		expectedAgentIDs[i] = agentID
		expectedKeys[i] = fmt.Sprintf("key%d", i)
		expectedValues[i] = fmt.Sprintf("value%d", i)
		expectedErrors[i] = ""
		expectedTimes[i] = t1
	}
	// The last entry is the capacity-triggering entry.
	expectedAgentIDs[batchSize-1] = agentID
	expectedKeys[batchSize-1] = "key_at_capacity"
	expectedValues[batchSize-1] = "value_at_capacity"
	expectedErrors[batchSize-1] = ""
	expectedTimes[batchSize-1] = t1

	// Set up pubsub capture for the flush.
	pubsubCap := newPubsubCapture()

	// Expect the store to be called with exactly batchSize entries.
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(
			gomock.Any(),
			matchMetadata(
				expectedAgentIDs,
				expectedKeys,
				expectedValues,
				expectedErrors,
				expectedTimes,
			),
		).
		Return(nil).
		Times(1)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(func(event string, message []byte) {
			require.NoError(t, pubsubCap.capture(event, message))
		}).
		Return(nil).
		Times(1)

	// Add entries up to but not exceeding capacity
	for i := 0; i < batchSize-1; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		require.NoError(t, b.Add(agentID, []string{key}, []string{value}, []string{""}, []time.Time{t1}))
	}

	// Verify no flush has occurred yet
	require.Equal(t, float64(0), prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity)))

	// Add one more entry to reach capacity - this should trigger automatic flush
	require.NoError(t, b.Add(agentID, []string{"key_at_capacity"}, []string{"value_at_capacity"}, []string{""}, []time.Time{t1}))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return pubsubCap.count() == 1
	}, testutil.IntervalFast)
	pubsubCap.assertContainsAll(t, []uuid.UUID{agentID})

	// Wait for automatic flush
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(batchSize), prom_testutil.ToFloat64(b.metrics.metadataTotal))
}
