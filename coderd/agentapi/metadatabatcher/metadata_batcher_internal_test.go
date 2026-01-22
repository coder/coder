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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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

// metadataParamsMatcher validates BatchUpdateWorkspaceAgentMetadataParams by checking all fields match expected values.
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

	// Check each field matches expected values. We create a map of expected entries and verify all actual entries match.
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
	t  *testing.T
	mu sync.Mutex

	agentIDs map[uuid.UUID]struct{}
}

func newPubsubCapture(t *testing.T) *pubsubCapture {
	return &pubsubCapture{
		agentIDs: make(map[uuid.UUID]struct{}),
		t:        t,
	}
}

func (c *pubsubCapture) capture(event string, message []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Verify correct event.
	assert.Equal(c.t, event, MetadataBatchPubsubChannel)

	// Decode base64-encoded agent IDs from payload.
	assert.Equal(c.t, len(message)%UUIDBase64Size, 0)

	numAgents := len(message) / UUIDBase64Size
	for i := 0; i < numAgents; i++ {
		start := i * UUIDBase64Size
		end := start + UUIDBase64Size
		encoded := message[start:end]

		var uuidBytes [16]byte
		n, err := base64.RawStdEncoding.Decode(uuidBytes[:], encoded)
		assert.NoError(c.t, err)
		assert.Equal(c.t, n, 16)

		agentID, err := uuid.FromBytes(uuidBytes[:])
		assert.NoError(c.t, err)

		c.agentIDs[agentID] = struct{}{}
	}
}

func (c *pubsubCapture) requireContainsAll(expected []uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check we don't have extra IDs.
	require.Equal(c.t, len(expected), len(c.agentIDs), "unexpected number of agent IDs in pubsub messages")

	// Check all expected IDs are present.
	for _, expectedID := range expected {
		_, ok := c.agentIDs[expectedID]
		require.True(c.t, ok, "expected agent ID %s not found in pubsub messages", expectedID)
	}
}

func (c *pubsubCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.agentIDs)
}

func (c *pubsubCapture) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agentIDs = make(map[uuid.UUID]struct{})
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

	// Trap timer reset calls so we can wait for them to complete.
	resetTrap := clock.Trap().TimerReset("metadataBatcher", "scheduledFlush")
	defer resetTrap.Close()
	capacityResetTrap := clock.Trap().TimerReset("metadataBatcher", "capacityFlush")
	defer capacityResetTrap.Close()

	// Generate mock agent IDs.
	agent1 := uuid.New()
	agent2 := uuid.New()

	// Create a single pubsub capture to reuse across all flushes.
	psCap := newPubsubCapture(t)

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
	resetTrap.MustWait(ctx).MustRelease(ctx) // Wait for timer reset after flush
	t.Log("flush 1 completed (expected 0 entries)")
	require.Equal(t, float64(0), prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker)))

	// --- FLUSH 2: Single agent with 2 metadata entries ---
	t2 := clock.Now()

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
		Do(psCap.capture).
		Return(nil).
		Times(1)

	// Given: a single metadata update is added for agent1
	t.Log("adding metadata for 1 agent")

	// Capture dropped count before adding.
	droppedBefore := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

	require.NoError(t, b.Add(agent1, []string{"key1", "key2"}, []string{"value1", "value2"}, []string{"", ""}, []time.Time{t2, t2}))

	// Wait for the channel to be processed and verify nothing was dropped.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		channelEmpty := len(b.updateCh) == 0
		nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
		batchHasExpected := int(b.currentBatchLen.Load()) == 2
		return channelEmpty && nothingDropped && batchHasExpected
	}, testutil.IntervalFast)

	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	resetTrap.MustWait(ctx).MustRelease(ctx) // Wait for timer reset after flush
	t.Log("flush 2 completed (expected 2 entries)")
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		return float64(1) == val && totalMeta >= float64(2)
	}, testutil.IntervalFast)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return psCap.count() == 1
	}, testutil.IntervalFast)
	psCap.requireContainsAll([]uuid.UUID{agent1})

	// --- FLUSH 3: Multiple agents with 5 total metadata entries ---
	t3 := clock.Now()

	// Clear pubsub capture for the next flush.
	psCap.clear()

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
		Do(psCap.capture).
		Return(nil).
		Times(1)

	// Given: metadata updates are added for multiple agents
	t.Log("adding metadata for 2 agents")

	// Capture dropped count before any adds.
	droppedBefore = prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

	require.NoError(t, b.Add(agent1, []string{"key1", "key2", "key3"}, []string{"new_value1", "new_value2", "new_value3"}, []string{"", "", ""}, []time.Time{t3, t3, t3}))
	require.NoError(t, b.Add(agent2, []string{"key1", "key2"}, []string{"agent2_value1", "agent2_value2"}, []string{"", ""}, []time.Time{t3, t3}))

	// Wait for all channel messages to be processed into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		channelEmpty := len(b.updateCh) == 0
		nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
		batchHasExpected := int(b.currentBatchLen.Load()) == 5
		return channelEmpty && nothingDropped && batchHasExpected
	}, testutil.IntervalFast)

	// When: it becomes time to flush
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)
	resetTrap.MustWait(ctx).MustRelease(ctx) // Wait for timer reset after flush
	t.Log("flush 3 completed (expected 5 new entries)")
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		val := prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
		totalMeta := prom_testutil.ToFloat64(b.metrics.metadataTotal)
		return float64(2) == val && totalMeta >= float64(7)
	}, testutil.IntervalFast)
	require.Equal(t, float64(7), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return psCap.count() == 2
	}, testutil.IntervalFast)
	psCap.requireContainsAll([]uuid.UUID{agent1, agent2})

	// --- FLUSH 4: Capacity flush with defaultMetadataBatchSize entries ---
	t4 := clock.Now()
	numAgents := defaultMetadataBatchSize

	// Clear pubsub capture for the next flush.
	psCap.clear()

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
		Do(psCap.capture).
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
	capacityResetTrap.MustWait(ctx).MustRelease(ctx) // Wait for timer reset after capacity flush
	t.Log("flush 4 completed (capacity flush, expected", defaultMetadataBatchSize, "entries)")
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity))
	}, testutil.IntervalFast)
	require.Equal(t, float64(507), prom_testutil.ToFloat64(b.metrics.metadataTotal))

	// Wait for pubsub capture to complete and verify all agent IDs were published (across all chunks).
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return psCap.count() == numAgents
	}, testutil.IntervalFast)
	psCap.requireContainsAll(agentIDs)
}

func TestMetadataBatcher_DropsWhenFull(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)
	clock := quartz.NewMock(t)

	reg := prometheus.NewRegistry()
	// Batch size of 2 means channel capacity = 10 (2 * 5)
	b, err := NewBatcher(ctx, reg, store, ps,
		WithLogger(log),
		WithBatchSize(2),
		WithClock(clock),
	)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	t1 := clock.Now()

	// Channels to control when the store call blocks/unblocks
	flushStarted := make(chan struct{})
	unblockFlush := make(chan struct{})

	pubsubCap := newPubsubCapture(t)

	// Make the first store call block until we signal. After unblocking,
	// the 10 queued entries will trigger 5 more capacity flushes (10/2 = 5).
	// Total expected flushes: 1 (initial) + 5 (queued) = 6
	firstCall := true
	store.EXPECT().
		BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, params database.BatchUpdateWorkspaceAgentMetadataParams) error {
			if firstCall {
				firstCall = false
				close(flushStarted) // Signal that first flush has started
				<-unblockFlush      // Wait for signal to continue
			}
			return nil
		}).
		Times(6)

	ps.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Do(pubsubCap.capture).
		Return(nil).
		Times(6)

	// Add 2 entries - this will trigger capacity flush (batch size = 2) that blocks
	agent1 := uuid.New()
	agent2 := uuid.New()
	require.NoError(t, b.Add(agent1, []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	require.NoError(t, b.Add(agent2, []string{"key1"}, []string{"value2"}, []string{""}, []time.Time{t1}))

	// Wait for flush to start and block in the store call
	<-flushStarted

	// Now the flush is blocked. Channel capacity is 10.
	// Fill the channel with 10 entries
	droppedBefore := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

	for i := 0; i < 10; i++ {
		agent := uuid.New()
		require.NoError(t, b.Add(agent, []string{"key1"}, []string{fmt.Sprintf("value%d", i)}, []string{""}, []time.Time{t1}))
	}

	// Channel should now be full. Next add should drop.
	agentDropped := uuid.New()
	require.NoError(t, b.Add(agentDropped, []string{"key1"}, []string{"dropped"}, []string{""}, []time.Time{t1}))

	// Verify that 1 key was dropped
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		dropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)
		return dropped == droppedBefore+1
	}, testutil.IntervalFast)

	// Unblock the flush
	close(unblockFlush)

	// Wait for all queued entries to be processed (channel should be empty)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(b.updateCh) == 0
	}, testutil.IntervalFast)

	// Verify final state: 1 key was dropped, 12 metadata sent in 6 capacity batches
	require.Equal(t, droppedBefore+1, prom_testutil.ToFloat64(b.metrics.droppedKeysTotal))
	require.Equal(t, float64(12), prom_testutil.ToFloat64(b.metrics.metadataTotal))
	require.Equal(t, float64(6), prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushCapacity)))

}

// TestMetadataBatcher_Deduplication executes two Add calls, the second with a later timestamp than the first, to check
// that existing keys within a batch have their values updated.
func TestMetadataBatcher_Deduplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		// First Add call
		add1Keys   []string
		add1Values []string

		// Second Add call
		add2Keys   []string
		add2Values []string

		// Expected result after deduplication
		wantKeys   []string
		wantValues []string
	}{
		{
			name: "same key updated twice keeps newest",

			add1Keys:   []string{"key1"},
			add1Values: []string{"first_value"},

			add2Keys:   []string{"key1"},
			add2Values: []string{"second_value"},

			wantKeys:   []string{"key1"},
			wantValues: []string{"second_value"},
		},
		{
			name: "mixed keys with partial overlap",

			add1Keys:   []string{"key1", "key2"},
			add1Values: []string{"value1", "value2"},

			add2Keys:   []string{"key1", "key3"},
			add2Values: []string{"new_value1", "value3"},

			wantKeys:   []string{"key1", "key2", "key3"},
			wantValues: []string{"new_value1", "value2", "value3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
			ctrl := gomock.NewController(t)
			store := dbmock.NewMockStore(ctrl)
			ps := psmock.NewMockPubsub(ctrl)
			clock := quartz.NewMock(t)

			agent := uuid.New()

			reg := prometheus.NewRegistry()
			b, err := NewBatcher(ctx, reg, store, ps,
				WithLogger(log),
				WithClock(clock),
			)
			require.NoError(t, err)
			t.Cleanup(b.Close)

			// Set up timestamps - t2 is 1ms after t1
			t1 := clock.Now()
			t2 := t1.Add(time.Millisecond)

			// Create time slices for add1 (all t1) and add2 (all t2)
			add1Times := make([]time.Time, len(tt.add1Keys))
			for i := range add1Times {
				add1Times[i] = t1
			}
			add2Times := make([]time.Time, len(tt.add2Keys))
			for i := range add2Times {
				add2Times[i] = t2
			}

			// Build expected times based on which add they came from.
			// If a key appears in add2, it gets t2 (newer), otherwise t1.
			expectedTimes := make([]time.Time, len(tt.wantKeys))
			for i, wantKey := range tt.wantKeys {
				// Check if key appears in add2 (newer)
				foundInAdd2 := false
				for _, add2Key := range tt.add2Keys {
					if add2Key == wantKey {
						expectedTimes[i] = t2
						foundInAdd2 = true
						break
					}
				}
				if !foundInAdd2 {
					// Must be from add1
					expectedTimes[i] = t1
				}
			}

			// Set up mock expectations
			psCap := newPubsubCapture(t)

			// Build expected errors (all empty) and agent IDs (all same agent)
			expectedErrors := make([]string, len(tt.wantKeys))
			for i := range expectedErrors {
				expectedErrors[i] = ""
			}
			expectedAgents := make([]uuid.UUID, len(tt.wantKeys))
			for i := range expectedAgents {
				expectedAgents[i] = agent
			}

			store.EXPECT().
				BatchUpdateWorkspaceAgentMetadata(
					gomock.Any(),
					matchMetadata(
						expectedAgents,
						tt.wantKeys,
						tt.wantValues,
						expectedErrors,
						expectedTimes,
					),
				).
				Return(nil).
				Times(1)

			ps.EXPECT().
				Publish(gomock.Any(), gomock.Any()).
				Do(psCap.capture).
				Return(nil).
				Times(1)

			// Perform the adds
			droppedBefore := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

			// First add with all empty error strings
			add1Errors := make([]string, len(tt.add1Keys))
			require.NoError(t, b.Add(agent, tt.add1Keys, tt.add1Values, add1Errors, add1Times))

			// Second add with all empty error strings
			add2Errors := make([]string, len(tt.add2Keys))
			require.NoError(t, b.Add(agent, tt.add2Keys, tt.add2Values, add2Errors, add2Times))

			// Wait for all channel messages to be processed into the batch
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				channelEmpty := len(b.updateCh) == 0
				nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
				batchHasExpected := int(b.currentBatchLen.Load()) == len(tt.wantKeys)
				return channelEmpty && nothingDropped && batchHasExpected
			}, testutil.IntervalFast)

			// Trigger scheduled flush
			clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

			// Verify flush occurred with correct number of entries
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
			}, testutil.IntervalFast)
			require.Equal(t, float64(len(tt.wantKeys)), prom_testutil.ToFloat64(b.metrics.metadataTotal))

			// Verify pubsub published the agent ID
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				return psCap.count() == 1
			}, testutil.IntervalFast)
			psCap.requireContainsAll([]uuid.UUID{agent})
		})
	}
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
	psCap := newPubsubCapture(t)

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
		Do(psCap.capture).
		Return(nil).
		Times(1)

	// Add update with t2 timestamp
	// Capture dropped count before any adds.
	droppedBefore := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newer_value"}, []string{""}, []time.Time{t2}))

	// Try to add older update with t1 timestamp - should be ignored
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"older_value"}, []string{""}, []time.Time{t1}))

	// Add even newer update with t3 timestamp - should overwrite
	require.NoError(t, b.Add(agent, []string{"key1"}, []string{"newest_value"}, []string{""}, []time.Time{t3}))

	// Wait for all channel messages to be processed by the run() goroutine into the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		channelEmpty := len(b.updateCh) == 0
		nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
		batchHasExpected := int(b.currentBatchLen.Load()) == 1
		return channelEmpty && nothingDropped && batchHasExpected
	}, testutil.IntervalFast)

	// Flush and verify entry was sent.
	// Advance the full flush interval from when the batcher was created.
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return psCap.count() == 1
	}, testutil.IntervalFast)
	psCap.requireContainsAll([]uuid.UUID{agent})

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
	psCap := newPubsubCapture(t)

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
		Do(psCap.capture).
		Return(nil).
		Times(3)

	// Add first 499 metadata updates (just under the capacity threshold of 500)
	// Capture dropped count before any adds.
	droppedBefore := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

	for i := 0; i < 499; i++ {
		require.NoError(t, b.Add(agents[i], []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	}

	// Wait for all channel messages to be processed into the batch.
	// Batch should have 499 entries, no capacity flush yet.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		channelEmpty := len(b.updateCh) == 0
		nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
		batchHasExpected := int(b.currentBatchLen.Load()) == 499
		return channelEmpty && nothingDropped && batchHasExpected
	}, testutil.IntervalFast)

	// Add next 101 metadata updates (will trigger capacity flush at 500)
	for i := 499; i < numAgents; i++ {
		require.NoError(t, b.Add(agents[i], []string{"key1"}, []string{"value1"}, []string{""}, []time.Time{t1}))
	}

	// Wait for all channel messages to be processed. The 500th entry should have
	// triggered an automatic capacity flush, leaving 100 entries in the batch.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		channelEmpty := len(b.updateCh) == 0
		nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
		batchHasExpected := int(b.currentBatchLen.Load()) == 100
		return channelEmpty && nothingDropped && batchHasExpected
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
		return psCap.count() == numAgents
	}, testutil.IntervalFast)
	psCap.requireContainsAll(agents)

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
	timestamps := make([]time.Time, numGoroutines)
	initialTS := clock.Now()
	for i := 0; i < numGoroutines; i++ {
		timestamps[i] = initialTS.Add(time.Duration(i) * time.Millisecond)
	}

	// The latest timestamp will have the final values, since deduplication keeps the newest value for each key.
	latestTimestamp := timestamps[numGoroutines-1]
	latestValue := fmt.Sprintf("value_from_goroutine_%d", numGoroutines-1)

	// Set up pubsub capture for the flush.
	psCap := newPubsubCapture(t)

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
		Do(psCap.capture).
		Return(nil).
		Times(1)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Capture dropped count before any adds.
	droppedBefore := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal)

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
		channelEmpty := len(b.updateCh) == 0
		nothingDropped := prom_testutil.ToFloat64(b.metrics.droppedKeysTotal) == droppedBefore
		batchHasExpected := int(b.currentBatchLen.Load()) == 3
		return channelEmpty && nothingDropped && batchHasExpected
	}, testutil.IntervalFast)

	// Flush and check that we have exactly 3 keys (deduplication worked).
	// Advance the full flush interval from when the batcher was created.
	clock.Advance(defaultMetadataFlushInterval).MustWait(ctx)

	// Wait for pubsub capture to complete and verify all agent IDs were published.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return psCap.count() == 1
	}, testutil.IntervalFast)
	psCap.requireContainsAll([]uuid.UUID{agentID})

	// Verify exactly 3 unique keys were flushed
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return float64(1) == prom_testutil.ToFloat64(b.metrics.batchesTotal.WithLabelValues(flushTicker))
	}, testutil.IntervalFast)
	require.Equal(t, float64(3), prom_testutil.ToFloat64(b.metrics.metadataTotal))
}
