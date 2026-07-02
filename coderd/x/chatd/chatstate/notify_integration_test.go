package chatstate_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// publishedOn returns the indices into f.Pub.channels (and f.Pub.payloads)
// that match the given channel name, in order.
func publishedOn(f *testFixture, channel string) []int {
	var idx []int
	for i, c := range f.Pub.channels {
		if c == channel {
			idx = append(idx, i)
		}
	}
	return idx
}

// TestCreateChatPublishesAfterCommit asserts that a successful
// CreateChat call publishes exactly one chat:update message on the
// per-chat channel after the inner transaction commits.
func TestCreateChatPublishesAfterCommit(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	res := createTestChat(t, f)

	channel := coderdpubsub.ChatStateUpdateChannel(res.Chat.ID)
	idx := publishedOn(f, channel)
	require.Len(t, idx, 1, "exactly one chat:update for the new chat")

	var msg coderdpubsub.ChatStateUpdateMessage
	require.NoError(t, json.Unmarshal(f.Pub.payloads[idx[0]], &msg))
	require.Equal(t, res.Chat.SnapshotVersion, msg.SnapshotVersion)
	require.Equal(t, string(database.ChatStatusRunning), msg.Status)
}

// TestUpdatePublishesAfterCommit asserts that ChatMachine.Update
// publishes one chat:update on the per-chat channel after the inner
// transaction commits, even when the callback performs no transition.
func TestUpdatePublishesAfterCommit(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	createIdx := publishedOn(f, coderdpubsub.ChatStateUpdateChannel(created.Chat.ID))
	require.Len(t, createIdx, 1, "create published one chat:update")

	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil }))

	updIdx := publishedOn(f, coderdpubsub.ChatStateUpdateChannel(created.Chat.ID))
	require.Len(t, updIdx, 2, "no-op Update still publishes a chat:update")

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	var msg coderdpubsub.ChatStateUpdateMessage
	require.NoError(t, json.Unmarshal(f.Pub.payloads[updIdx[1]], &msg))
	require.Equal(t, after.SnapshotVersion, msg.SnapshotVersion)
}

// TestUpdatePublishesOneFinalChatUpdateForTransitionBundle bundles
// several transitions inside one Update callback and verifies the
// commit publishes exactly one chat:update on the per-chat channel
// (not one per transition).
func TestUpdatePublishesOneFinalChatUpdateForTransitionBundle(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	baseUpdates := len(publishedOn(f, coderdpubsub.ChatStateUpdateChannel(created.Chat.ID)))

	// chatstate.StateR0 -> chatstate.StateW (FinishTurn) ->
	// chatstate.StateXW (SetArchived true) -> chatstate.StateW
	// (SetArchived false).
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := tx.FinishTurn(chatstate.FinishTurnInput{}); err != nil {
			return err
		}
		if _, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true}); err != nil {
			return err
		}
		if _, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: false}); err != nil {
			return err
		}
		return nil
	}))

	updIdx := publishedOn(f, coderdpubsub.ChatStateUpdateChannel(created.Chat.ID))
	require.Equal(t, baseUpdates+1, len(updIdx),
		"three-transition bundle publishes exactly one final chat:update")

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, after.Status,
		"ends in "+chatstate.StateW.String())
}

// TestUpdateAppliesTransitionBundleSequentially verifies that
// transitions chained inside a single Update callback see each
// other's effects: later transitions validate against the state
// produced by earlier ones (chatstate.StateR0 -> chatstate.StateW
// is rejected when called twice because the second call sees
// chatstate.StateW and FinishTurn is no longer allowed).
func TestUpdateAppliesTransitionBundleSequentially(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := tx.FinishTurn(chatstate.FinishTurnInput{}); err != nil {
			return err
		}
		// Second FinishTurn should fail because state is now chatstate.StateW.
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	// Failed bundle rolls back: state must not have advanced past
	// chatstate.StateR0.
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, created.Chat.ID),
		"failed bundle rolls back the whole transaction")
}

// TestFailedUpdatePublishesNothing verifies that a callback error
// rolls back the snapshot bump and publishes nothing.
func TestFailedUpdatePublishesNothing(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	publishedBefore := len(f.Pub.channels)
	beforeChat := f.readChat(ctx, t, created.Chat.ID)

	sentinel := xerrors.New("forced failure")
	err := m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, publishedBefore, len(f.Pub.channels), "failed update publishes nothing")

	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, beforeChat.SnapshotVersion, after.SnapshotVersion,
		"failed update rolls back snapshot bump")
}

// TestLockPublishesNothing verifies that Lock does not publish even
// though it locks the chat row.
func TestLockPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	publishedBefore := len(f.Pub.channels)
	require.NoError(t, m.Lock(ctx, func(_ database.Store) error { return nil }))
	require.Equal(t, publishedBefore, len(f.Pub.channels), "Lock publishes nothing")
}

// TestPublishBufferWithRolledBackOuterTransactionPublishesNothing
// wires a chatstate machine through a PublishBuffer and exercises
// the buffer primitive directly: when the caller discards before
// flushing, the inner publisher receives nothing. ChatMachine.Update
// uses the same primitive internally with a deferred Discard;
// callers no longer drive Flush or Discard themselves.
func TestPublishBufferWithRolledBackOuterTransactionPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	// Run one normal Update to establish a stable baseline channel
	// count. CreateChat plus this Update may publish chat:update
	// and chat:ownership messages depending on ownership, so we
	// take the snapshot after that activity settles.
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil }))
	baseline := len(f.Pub.channels)

	// Now exercise the PublishBuffer rollback path explicitly. The
	// outer transaction "rolls back": the caller buffers messages,
	// discards them, then flushes. The inner publisher must see
	// none of the buffered messages.
	buf := chatstate.NewPublishBuffer(f.Pub)
	require.NoError(t, buf.Publish("chat:update:bogus", []byte("payload")))
	require.NoError(t, buf.Publish("chat:ownership", []byte("payload")))
	buf.Discard()
	require.NoError(t, buf.Flush())

	require.Equal(t, baseline, len(f.Pub.channels),
		"discarded buffer publishes nothing through the inner publisher")
}

// TestChatUpdateMessagePayloadShape verifies the JSON shape of the
// chat:update payload contains every field consumers depend on:
// snapshot_version, history_version, queue_version,
// retry_state_version, generation_attempt, status, archived, and
// worker_id / runner_id, with explicit nulls when unowned.
func TestChatUpdateMessagePayloadShape(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	channel := coderdpubsub.ChatStateUpdateChannel(created.Chat.ID)

	// The create update is unowned and must still include explicit
	// null ownership fields.
	createIdx := publishedOn(f, channel)
	require.NotEmpty(t, createIdx)
	var createRaw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(f.Pub.payloads[createIdx[0]], &createRaw))
	require.JSONEq(t, `null`, string(createRaw["worker_id"]))
	require.JSONEq(t, `null`, string(createRaw["runner_id"]))

	// Acquire ownership so worker_id and runner_id are present.
	worker := uuid.New()
	runner := uuid.New()
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: worker, RunnerID: runner})
		return err
	}))

	// Find the last chat:update message.
	idx := publishedOn(f, channel)
	require.NotEmpty(t, idx)
	last := f.Pub.payloads[idx[len(idx)-1]]

	// Strict-decode against the typed struct.
	var typed coderdpubsub.ChatStateUpdateMessage
	require.NoError(t, json.Unmarshal(last, &typed))
	require.Greater(t, typed.SnapshotVersion, int64(0))
	require.NotNil(t, typed.WorkerID)
	require.Equal(t, worker, *typed.WorkerID)
	require.NotNil(t, typed.RunnerID)
	require.Equal(t, runner, *typed.RunnerID)
	require.Equal(t, string(database.ChatStatusRunning), typed.Status)
	require.False(t, typed.Archived)

	// Permissive decode to assert exact JSON keys.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(last, &raw))
	for _, key := range []string{
		"snapshot_version",
		"history_version",
		"queue_version",
		"retry_state_version",
		"generation_attempt",
		"status",
		"archived",
		"worker_id",
		"runner_id",
	} {
		_, ok := raw[key]
		require.True(t, ok, "payload missing key %q", key)
	}
}

// TestChatOwnershipMessagePayloadShape verifies the JSON shape of
// chat:ownership: chat_id and snapshot_version.
func TestChatOwnershipMessagePayloadShape(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	// CreateChat publishes one ownership hint because the new chat is
	// unowned and runnable.
	created := createTestChat(t, f)

	idx := publishedOn(f, coderdpubsub.ChatStateOwnershipChannel)
	require.NotEmpty(t, idx, "CreateChat publishes at least one chat:ownership hint")

	payload := f.Pub.payloads[idx[len(idx)-1]]
	var typed coderdpubsub.ChatStateOwnershipMessage
	require.NoError(t, json.Unmarshal(payload, &typed))
	require.Equal(t, created.Chat.ID, typed.ChatID)
	require.Greater(t, typed.SnapshotVersion, int64(0))

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &raw))
	for _, key := range []string{"chat_id", "snapshot_version"} {
		_, ok := raw[key]
		require.True(t, ok, "ownership payload missing key %q", key)
	}
}

// TestOwnershipNotificationUsesDatabaseHeartbeatStaleness verifies
// that an ownership hint fires when the heartbeat is stale by the
// database's clock, regardless of what the local Go clock says. We
// rewrite the heartbeat row to a deterministically old timestamp via
// raw SQL and confirm the post-commit hint is sent on a subsequent
// runnable Update.
func TestOwnershipNotificationUsesDatabaseHeartbeatStaleness(t *testing.T) {
	t.Parallel()
	tf := newTriggerFixture(t)
	f := tf.f
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	// Acquire ownership; this writes a fresh heartbeat.
	worker := uuid.New()
	runner := uuid.New()
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: worker, RunnerID: runner})
		return err
	}))
	hb, err := f.DB.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
		ChatID:   created.Chat.ID,
		RunnerID: runner,
	})
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), hb.HeartbeatAt, time.Minute,
		"Acquire wrote a fresh heartbeat")

	// Snapshot ownership-hint count before the test trigger.
	ownershipBefore := f.Pub.ownershipPublishCount()

	// Force the heartbeat to a deterministically old time.
	_, err = tf.sqlDB.ExecContext(ctx, `
		UPDATE chat_heartbeats
		SET heartbeat_at = NOW() - INTERVAL '1 hour'
		WHERE chat_id = $1 AND runner_id = $2
	`, created.Chat.ID, runner)
	require.NoError(t, err)

	// Confirm database-side staleness check agrees.
	stale, err := f.DB.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID:       created.Chat.ID,
		RunnerID:     runner,
		StaleSeconds: chatstate.HeartbeatStaleSeconds,
	})
	require.NoError(t, err)
	require.True(t, stale, "heartbeat is stale per database time")

	// Run a no-op Update. The chat is runnable (chatstate.StateR0)
	// and the heartbeat is stale, so post-commit logic must publish
	// exactly one chat:ownership hint.
	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil }))

	ownershipAfter := f.Pub.ownershipPublishCount()
	require.Equal(t, ownershipBefore+1, ownershipAfter,
		"stale heartbeat triggers a fresh ownership hint")
}

// TestUpdateContextCancellationPublishesNothing verifies that
// canceling the caller's context (between the inner commit and the
// publish loop's first call) does not corrupt state. We exercise the
// simpler observable contract: when the user cancels before Update
// gets to do anything, nothing is published. The strict before-publish
// race is exercised in concurrency tests with channel sync.
func TestUpdateContextCancellationPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	publishedBefore := len(f.Pub.channels)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil })
	require.Error(t, err)
	require.Equal(t, publishedBefore, len(f.Pub.channels),
		"caller-aborted update publishes nothing")
}
