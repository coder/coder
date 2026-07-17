package usage_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/enterprise/coderd/usage"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// generatorTimerName must match the tag the Generator passes to
// clock.NewTimer so tests can trap its timers.
const generatorTimerName = "agent-runtime-generator"

// generatorHarness bundles a real database with seeded chat dependencies.
// The generator itself runs against a dbauthz-wrapped store so the tests
// also verify that the usage publisher subject holds the permissions the
// generator's queries require.
type generatorHarness struct {
	db      database.Store
	authzDB database.Store
	rawDB   *sql.DB

	user        database.User
	modelConfig database.ChatModelConfig
	chat        database.Chat
	chat2       database.Chat
}

func newGeneratorHarness(t *testing.T) *generatorHarness {
	t.Helper()
	db, _, rawDB := dbtestutil.NewDBWithSQLDB(t)
	log := slogtest.Make(t, nil)
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), log, coderdtest.AccessControlStorePointer())

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
	})
	mc := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "test-model",
		ContextLimit: 8192,
	})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: mc.ID,
	})
	chat2 := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: mc.ID,
	})
	return &generatorHarness{
		db:          db,
		authzDB:     authzDB,
		rawDB:       rawDB,
		user:        user,
		modelConfig: mc,
		chat:        chat,
		chat2:       chat2,
	}
}

// insertRuntimeMessage inserts an assistant message with the given runtime
// and backdates it to createdAt.
func (h *generatorHarness) insertRuntimeMessage(ctx context.Context, t *testing.T, chatID uuid.UUID, runtimeMs int64, createdAt time.Time, deleted bool) {
	t.Helper()
	msg := dbgen.ChatMessage(t, h.db, database.ChatMessage{
		ChatID:        chatID,
		CreatedBy:     uuid.NullUUID{UUID: h.user.ID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: h.modelConfig.ID, Valid: true},
		Role:          database.ChatMessageRoleAssistant,
		RuntimeMs:     sql.NullInt64{Int64: runtimeMs, Valid: true},
	})
	_, err := h.rawDB.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1, deleted = $2 WHERE id = $3", createdAt, deleted, msg.ID)
	require.NoError(t, err)
}

// fetchRuntimeEvents returns all hb_agent_runtime_v1 events keyed by their
// created_at (bucket start, UTC), plus a map of created_at to event ID.
func (h *generatorHarness) fetchRuntimeEvents(ctx context.Context, t *testing.T) (map[time.Time]int64, map[time.Time]string) {
	t.Helper()
	rows, err := h.rawDB.QueryContext(ctx, `
		SELECT id, (event_data->>'runtime_ms')::bigint, created_at
		FROM usage_events
		WHERE event_type = 'hb_agent_runtime_v1'
	`)
	require.NoError(t, err)
	defer rows.Close()

	runtimes := make(map[time.Time]int64)
	ids := make(map[time.Time]string)
	for rows.Next() {
		var (
			id        string
			runtimeMs int64
			createdAt time.Time
		)
		require.NoError(t, rows.Scan(&id, &runtimeMs, &createdAt))
		bucket := createdAt.UTC()
		_, ok := runtimes[bucket]
		require.False(t, ok, "duplicate event for bucket %s", bucket)
		runtimes[bucket] = runtimeMs
		ids[bucket] = id
	}
	require.NoError(t, rows.Err())
	return runtimes, ids
}

// expectedBuckets returns a map of every hourly bucket in [first, last] set
// to 0, then applies overrides.
func expectedBuckets(first, last time.Time, overrides map[time.Time]int64) map[time.Time]int64 {
	expected := make(map[time.Time]int64)
	for bucket := first; !bucket.After(last); bucket = bucket.Add(time.Hour) {
		expected[bucket] = 0
	}
	for bucket, runtimeMs := range overrides {
		expected[bucket] = runtimeMs
	}
	return expected
}

func TestGenerator(t *testing.T) {
	t.Parallel()

	// startTime is exactly on an hour boundary, so the first tick (which
	// fires 1-5 minutes later) always lands before the just-closed bucket
	// [13:00, 14:00) becomes eligible at 14:05.
	startTime := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC)

	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	h := newGeneratorHarness(t)
	clock := quartz.NewMock(t)
	clock.Set(startTime)

	var (
		// Buckets within the window at the first tick.
		bucketA = time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC)
		bucketB = time.Date(2025, 3, 10, 11, 0, 0, 0, time.UTC)
		// The most recent closed bucket; not eligible at the first tick.
		bucketC = time.Date(2025, 3, 10, 13, 0, 0, 0, time.UTC)
		// Window bounds at the first tick.
		windowFirst = startTime.Add(-usage.AgentRuntimeWindow) // 2025-03-03 14:00
		windowLast  = startTime.Add(-2 * time.Hour)            // 2025-03-10 12:00
	)

	// Bucket A: a message on the bucket start boundary, a message from a
	// second chat, and a soft-deleted message. All must be counted.
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 1000, bucketA, false)
	h.insertRuntimeMessage(ctx, t, h.chat2.ID, 2000, bucketA.Add(15*time.Minute), false)
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 4000, bucketA.Add(30*time.Minute), true)
	// Bucket B: a message exactly on the A/B boundary belongs to B.
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 8000, bucketB, false)
	// Older than the window: must never be generated.
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 16000, windowFirst.Add(-30*time.Minute), false)
	// Bucket C: only becomes eligible at 14:05, after the first tick.
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 32000, bucketC.Add(30*time.Minute), false)

	trap := clock.Trap().NewTimer(generatorTimerName)
	defer trap.Close()

	gen := usage.NewGenerator(clock, log, h.authzDB, usage.NewDBInserter())
	gen.Start(ctx)
	defer gen.Close()

	// The initial pass is delayed by a uniform random duration in [1m, 5m).
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	require.GreaterOrEqual(t, call.Duration, time.Minute)
	require.Less(t, call.Duration, 5*time.Minute)
	clock.Advance(call.Duration).MustWait(ctx)

	// The generator creates the next timer only after the tick completes,
	// so trapping it synchronizes with the end of the pass.
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)

	// The first pass fills every bucket in [windowFirst, windowLast]:
	// bucket C is not yet eligible and the bucket before windowFirst is
	// outside the window, message runtimes land in their buckets, and idle
	// hours are zero-filled.
	runtimes, ids := h.fetchRuntimeEvents(ctx, t)
	require.Equal(t, expectedBuckets(windowFirst, windowLast, map[time.Time]int64{
		bucketA: 7000,
		bucketB: 8000,
	}), runtimes)
	require.Equal(t, "hb_agent_runtime_v1:2025-03-10_10:00:00", ids[bucketA])

	// The next tick fires shortly after the next hour boundary (15:00), once
	// bucket [14:00, 15:00) is eligible.
	fireTime := clock.Now().Add(call.Duration)
	require.Equal(t, time.Date(2025, 3, 10, 15, 0, 0, 0, time.UTC), fireTime.Truncate(usage.AgentRuntimeInterval))
	require.GreaterOrEqual(t, fireTime.Sub(fireTime.Truncate(usage.AgentRuntimeInterval)), usage.AgentRuntimeEligibilityLag)
	clock.Advance(call.Duration).MustWait(ctx)
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)

	// The second tick fills only the two newly-eligible buckets: C (13:00)
	// and the idle 14:00. The window's start advanced by an hour, but the
	// bucket at the old windowFirst is kept (rows are never deleted).
	runtimes, _ = h.fetchRuntimeEvents(ctx, t)
	require.Equal(t, expectedBuckets(windowFirst, windowLast.Add(2*time.Hour), map[time.Time]int64{
		bucketA: 7000,
		bucketB: 8000,
		bucketC: 32000,
	}), runtimes)

	// A separate generator started later (e.g. another replica restarting)
	// finds nothing to do: all buckets in its window already exist.
	gen2 := usage.NewGenerator(clock, log, h.authzDB, usage.NewDBInserter())
	gen2.Start(ctx)
	defer gen2.Close()
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)
	clock.Advance(call.Duration).MustWait(ctx)
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)

	runtimes2, _ := h.fetchRuntimeEvents(ctx, t)
	require.Equal(t, runtimes, runtimes2)
}

// TestGeneratorBackfillAfterDowntime simulates a deployment that was down
// for several hours: a fresh generator's first pass backfills exactly the
// missing buckets.
func TestGeneratorBackfillAfterDowntime(t *testing.T) {
	t.Parallel()

	startTime := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC)

	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	h := newGeneratorHarness(t)

	windowFirst := startTime.Add(-usage.AgentRuntimeWindow)
	gapBucket := time.Date(2025, 3, 10, 11, 0, 0, 0, time.UTC)
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 5000, gapBucket.Add(45*time.Minute), false)

	// Simulate events generated before the deployment went down at 09:00:
	// every bucket in [windowFirst, 08:00] exists with runtime 1. These are
	// inserted through the unwrapped store, which performs no authz.
	inserter := usage.NewDBInserter()
	for bucket := windowFirst; !bucket.After(time.Date(2025, 3, 10, 8, 0, 0, 0, time.UTC)); bucket = bucket.Add(time.Hour) {
		err := inserter.InsertHeartbeatUsageEvent(ctx, h.db, "hb_agent_runtime_v1:"+bucket.Format("2006-01-02_15:04:05"), bucket, usagetypes.HBAgentRuntime{RuntimeMs: 1})
		require.NoError(t, err)
	}

	clock := quartz.NewMock(t)
	clock.Set(startTime)
	trap := clock.Trap().NewTimer(generatorTimerName)
	defer trap.Close()

	gen := usage.NewGenerator(clock, log, h.authzDB, usage.NewDBInserter())
	gen.Start(ctx)
	defer gen.Close()

	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	clock.Advance(call.Duration).MustWait(ctx)
	call = trap.MustWait(ctx)
	call.MustRelease(ctx)

	// The pass must fill exactly the gap [09:00, 12:00] and leave the
	// pre-existing rows untouched.
	runtimes, _ := h.fetchRuntimeEvents(ctx, t)
	expected := expectedBuckets(windowFirst, startTime.Add(-2*time.Hour), map[time.Time]int64{
		gapBucket: 5000,
	})
	for bucket := windowFirst; !bucket.After(time.Date(2025, 3, 10, 8, 0, 0, 0, time.UTC)); bucket = bucket.Add(time.Hour) {
		expected[bucket] = 1
	}
	require.Equal(t, expected, runtimes)
}

// TestGeneratorConcurrentReplicas runs two generators against the same
// database concurrently and verifies exactly one event is produced per
// bucket. Each replica gets its own mock clock (as real replicas have their
// own wall clocks) so their first passes can be fired independently and run
// at the same time.
func TestGeneratorConcurrentReplicas(t *testing.T) {
	t.Parallel()

	startTime := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC)

	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, nil)
	h := newGeneratorHarness(t)

	bucketA := time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC)
	h.insertRuntimeMessage(ctx, t, h.chat.ID, 1000, bucketA.Add(10*time.Minute), false)

	// Both replicas' first passes fire between 14:01 and 14:05, so they
	// compute identical windows regardless of their random startup jitter.
	var traps []*quartz.Trap
	for range 2 {
		clock := quartz.NewMock(t)
		clock.Set(startTime)
		trap := clock.Trap().NewTimer(generatorTimerName)
		t.Cleanup(trap.Close)
		traps = append(traps, trap)

		gen := usage.NewGenerator(clock, log, h.authzDB, usage.NewDBInserter())
		gen.Start(ctx)
		// Cleanups run LIFO, so each generator is closed before its trap.
		t.Cleanup(func() { _ = gen.Close() })

		call := trap.MustWait(ctx)
		call.MustRelease(ctx)
		// Fire the initial timer without waiting for the pass to complete so
		// both replicas' passes overlap.
		clock.Advance(call.Duration).MustWait(ctx)
	}

	// Wait for both passes to complete (each requests its next timer only
	// after the pass finishes).
	for _, trap := range traps {
		call := trap.MustWait(ctx)
		call.MustRelease(ctx)
	}

	// fetchRuntimeEvents fails on duplicate buckets; the expected map proves
	// both replicas raced without erroring or double-inserting.
	runtimes, _ := h.fetchRuntimeEvents(ctx, t)
	require.Equal(t, expectedBuckets(
		startTime.Add(-usage.AgentRuntimeWindow),
		startTime.Add(-2*time.Hour),
		map[time.Time]int64{bucketA: 1000},
	), runtimes)
}
