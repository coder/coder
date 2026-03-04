package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	osschatd "github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	entchatd "github.com/coder/coder/v2/enterprise/coderd/chatd"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func newTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	replicaID uuid.UUID,
	dialer func(
		ctx context.Context,
		chatID uuid.UUID,
		workerID uuid.UUID,
		requestHeader http.Header,
	) (
		[]codersdk.ChatStreamEvent,
		<-chan codersdk.ChatStreamEvent,
		func(),
		error,
	),
	clock quartz.Clock,
) *osschatd.Server {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := osschatd.New(osschatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		Pubsub:                     ps,
		SubscribeFn:                entchatd.NewMultiReplicaSubscribeFn(entchatd.MultiReplicaSubscribeConfig{DialerFn: dialer, Clock: clock}),
		PendingChatAcquireInterval: testutil.WaitSuperLong,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

// seedChatDependencies creates a user and chat model config in the
// database for use in relay tests.
func seedChatDependencies(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)
	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	return user, model
}

func TestSubscribeRelayReconnectsOnDrop(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	var callCount atomic.Int32

	provider := func(ctx context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call := callCount.Add(1)
		ch := make(chan codersdk.ChatStreamEvent, 10)
		if call == 1 {
			// First relay: send a part then close to simulate a drop.
			ch <- codersdk.ChatStreamEvent{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "first-relay"},
				},
			}
			close(ch)
		} else {
			// Second relay: send a different part, keep open.
			ch <- codersdk.ChatStreamEvent{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "second-relay"},
				},
			}
			// Don't close — keep alive so the subscriber stays connected.
		}
		return nil, ch, func() {}, nil
	}

	mclk := quartz.NewMock(t)
	// Trap the reconnect timer so we can fire it deterministically
	// instead of waiting real time.
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, provider, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat and mark it as running on a remote worker.
	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "relay-reconnect",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Should get the first relay part.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "first-relay" {
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Wait for the reconnect timer to be created after the relay
	// drop, then advance the mock clock to fire it immediately.
	trapReconnect.MustWait(ctx).MustRelease(ctx)
	mclk.Advance(500 * time.Millisecond).MustWait(ctx)

	// After the first relay closes, the reconnection should deliver
	// the second relay part.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "second-relay" {
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.GreaterOrEqual(t, int(callCount.Load()), 2)
}

func TestSubscribeRelayAsyncDoesNotBlock(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	dialStarted := make(chan struct{})
	dialContinue := make(chan struct{})

	provider := func(ctx context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		// Signal that the dial has started, then block until released.
		select {
		case <-dialStarted:
		default:
			close(dialStarted)
		}
		select {
		case <-dialContinue:
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		}
		ch := make(chan codersdk.ChatStreamEvent, 10)
		return nil, ch, func() {}, nil
	}

	subscriber := newTestServer(t, db, ps, subscriberID, provider, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat in pending status.
	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "relay-async-nonblock",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	// Subscribe before the chat is marked running so the relay opens
	// via pubsub notification (openRelayAsync path).
	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Now mark the chat as running on a remote worker. This publishes
	// a status notification which triggers openRelayAsync on the
	// subscriber.
	notify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: workerID.String(),
	}
	payload, err := json.Marshal(notify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), payload)
	require.NoError(t, err)

	// Wait for the relay dial to actually start (blocking in the
	// provider).
	select {
	case <-dialStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for relay dial to start")
	}

	// While the relay is still dialing (provider is blocked), publish
	// another status change. If openRelayAsync blocked the select loop
	// this event would never arrive.
	statusNotify := coderdpubsub.ChatStreamNotifyMessage{
		Status: string(database.ChatStatusWaiting),
	}
	statusPayload, err := json.Marshal(statusNotify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), statusPayload)
	require.NoError(t, err)

	// The waiting status event should arrive promptly despite the
	// relay still dialing.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			return event.Type == codersdk.ChatStreamEventTypeStatus &&
				event.Status != nil &&
				event.Status.Status == codersdk.ChatStatusWaiting
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast)

	// Unblock the relay dial so the test can clean up.
	close(dialContinue)
}

func TestSubscribeRelaySnapshotDelivered(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	provider := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		// Return a non-empty snapshot with two parts.
		snapshot := []codersdk.ChatStreamEvent{
			{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "snap-one"},
				},
			},
			{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "snap-two"},
				},
			},
		}
		ch := make(chan codersdk.ChatStreamEvent, 10)
		// Also send a live part after the snapshot.
		ch <- codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: "assistant",
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "live-part"},
			},
		}
		return snapshot, ch, func() {}, nil
	}

	subscriber := newTestServer(t, db, ps, subscriberID, provider, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat already running on a remote worker.
	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "relay-snapshot",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	initialSnapshot, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// The relay snapshot parts are forwarded through the events
	// channel by the enterprise SubscribeFn. Collect them along
	// with the live part.
	var receivedTexts []string
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil {
				receivedTexts = append(receivedTexts, event.MessagePart.Part.Text)
			}
			// We expect snap-one, snap-two, and live-part.
			return len(receivedTexts) >= 3
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Equal(t, []string{"snap-one", "snap-two", "live-part"}, receivedTexts)

	// The initial snapshot should still contain the status event
	// from the OSS preamble.
	var hasStatus bool
	for _, event := range initialSnapshot {
		if event.Type == codersdk.ChatStreamEventTypeStatus {
			hasStatus = true
		}
	}
	require.True(t, hasStatus, "initial snapshot should contain status event")
}

// TestSubscribeRelayStaleDialDiscardedAfterInterrupt verifies that when a
// user interrupts a streaming chat and sends a new message (which gets
// picked up by a different replica), an in-flight relay dial to the
// OLD replica is canceled/discarded and the relay connects to the
// NEW replica correctly.
func TestSubscribeRelayStaleDialDiscardedAfterInterrupt(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	oldWorkerID := uuid.New()
	newWorkerID := uuid.New()
	subscriberID := uuid.New()

	// Gate to hold the first dial until we're ready.
	firstDialStarted := make(chan struct{})
	releaseFirstDial := make(chan struct{})

	var callCount atomic.Int32

	provider := func(ctx context.Context, _ uuid.UUID, workerID uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call := callCount.Add(1)
		ch := make(chan codersdk.ChatStreamEvent, 10)
		if call == 1 {
			// First dial (to old worker): signal that we started,
			// then block until released or context canceled.
			close(firstDialStarted)
			select {
			case <-releaseFirstDial:
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			}
			// If we get here after being released (not canceled),
			// return a stale part — this should be discarded.
			ch <- codersdk.ChatStreamEvent{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "stale-part"},
				},
			}
			close(ch)
			return nil, ch, func() {}, nil
		}
		// Second dial (to new worker): return a valid part.
		ch <- codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: "assistant",
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "new-worker-part"},
			},
		}
		return nil, ch, func() {}, nil
	}

	subscriber := newTestServer(t, db, ps, subscriberID, provider, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "stale-dial-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	// Start chat in waiting state so Subscribe does NOT try an initial relay.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	// Subscribe while chat is in "waiting" state — no relay opened.
	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Now simulate the chat being picked up by the OLD worker via pubsub.
	// This triggers openRelayAsync in the merge loop.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: oldWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)
	oldRunningNotify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: oldWorkerID.String(),
	}
	oldRunningPayload, err := json.Marshal(oldRunningNotify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), oldRunningPayload)
	require.NoError(t, err)

	// Wait for the first dial goroutine to start (it's blocked in the provider).
	select {
	case <-firstDialStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first dial to start")
	}

	// Simulate interrupt: chat goes to "waiting".
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	waitingNotify := coderdpubsub.ChatStreamNotifyMessage{
		Status: string(database.ChatStatusWaiting),
	}
	waitingPayload, err := json.Marshal(waitingNotify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), waitingPayload)
	require.NoError(t, err)

	// Wait for the merge loop to process the waiting notification
	// and emit the status event before publishing the new running
	// notification. This avoids time.Sleep (banned by project
	// policy) and provides a deterministic sync point.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			return event.Type == codersdk.ChatStreamEventTypeStatus &&
				event.Status != nil &&
				event.Status.Status == codersdk.ChatStatusWaiting
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Now the chat transitions to running on the NEW worker.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: newWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)
	runningNotify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: newWorkerID.String(),
	}
	runningPayload, err := json.Marshal(runningNotify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), runningPayload)
	require.NoError(t, err)

	// Now release the first dial (if it wasn't already canceled).
	close(releaseFirstDial)

	// The subscriber should receive parts from the NEW worker, not the stale one.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "new-worker-part" {
				return true
			}
			// If we get the stale part, the bug is present.
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "stale-part" {
				t.Fatal("received stale part from old worker — relay did not cancel in-flight dial")
			}
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Drain the events channel for a while to ensure no late-arriving
	// stale part sneaks in after the require.Eventually above returned.
	// This closes the timing gap where "stale-part" could arrive after
	// "new-worker-part" was already consumed.
	require.Never(t, func() bool {
		select {
		case event := <-events:
			return event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "stale-part"
		default:
			return false
		}
	}, 2*time.Second, testutil.IntervalFast)
}

// TestSubscribeCancelDuringInFlightDial verifies that calling the
// subscription's cancel function while a relay dial goroutine is
// still blocking in the provider causes the provider's context to
// be canceled and the goroutine to return cleanly.
func TestSubscribeCancelDuringInFlightDial(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	dialStarted := make(chan struct{})
	dialExited := make(chan struct{})

	provider := func(ctx context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		// Signal the dial has started, then block until the context
		// is canceled.
		close(dialStarted)
		<-ctx.Done()
		close(dialExited)
		return nil, nil, nil, ctx.Err()
	}

	subscriber := newTestServer(t, db, ps, subscriberID, provider, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "cancel-inflight-dial",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	// Put the chat in waiting state so Subscribe does not open a
	// synchronous relay.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	_, _, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)

	// Publish a running notification to trigger openRelayAsync.
	notify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: workerID.String(),
	}
	payload, err := json.Marshal(notify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), payload)
	require.NoError(t, err)

	// Wait for the dial goroutine to block inside the provider.
	select {
	case <-dialStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for dial to start")
	}

	// Cancel the subscription while the dial is still in-flight.
	cancel()

	// The provider context must be canceled, causing the goroutine
	// to return cleanly.
	require.Eventually(t, func() bool {
		select {
		case <-dialExited:
			return true
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)
}

// TestSubscribeRelayRunningToRunningSwitch verifies that when a chat
// transitions directly from running(workerA) to running(workerB)
// without an intermediate waiting state, the relay switches to the
// new worker and discards parts from the old one.
func TestSubscribeRelayRunningToRunningSwitch(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerA := uuid.New()
	workerB := uuid.New()
	subscriberID := uuid.New()

	// Gate to hold workerA's dial until we verify cancellation.
	dialAStarted := make(chan struct{})
	dialAExited := make(chan struct{})

	var callCount atomic.Int32

	provider := func(ctx context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call := callCount.Add(1)
		if call == 1 {
			// First dial (to workerA): signal that we started,
			// then block until the context is canceled.
			close(dialAStarted)
			<-ctx.Done()
			close(dialAExited)
			return nil, nil, nil, ctx.Err()
		}
		// Second dial (to workerB): return a valid part.
		ch := make(chan codersdk.ChatStreamEvent, 10)
		ch <- codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: "assistant",
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "worker-b-part"},
			},
		}
		return nil, ch, func() {}, nil
	}

	subscriber := newTestServer(t, db, ps, subscriberID, provider, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "running-to-running",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	// Start in waiting state so Subscribe does not open a relay.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Transition to running on workerA.
	notifyA := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: workerA.String(),
	}
	payloadA, err := json.Marshal(notifyA)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), payloadA)
	require.NoError(t, err)

	// Wait for the workerA dial goroutine to block inside the
	// provider before publishing the workerB notification.
	select {
	case <-dialAStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for workerA dial to start")
	}

	// Immediately transition to running on workerB (no waiting in
	// between). This should cancel workerA's in-flight dial.
	notifyB := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: workerB.String(),
	}
	payloadB, err := json.Marshal(notifyB)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), payloadB)
	require.NoError(t, err)

	// Verify that the relay canceled workerA's stale dial.
	require.Eventually(t, func() bool {
		select {
		case <-dialAExited:
			return true
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	// We should receive the part from workerB.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "worker-b-part" {
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Equal(t, 2, int(callCount.Load()))
}

// TestSubscribeRelayFailedDialRetries verifies that when an async relay
// dial fails (returns an error), the merge loop schedules a reconnect
// timer and eventually re-dials successfully. This exercises the
// result.parts == nil path and the scheduleRelayReconnect() logic.
func TestSubscribeRelayFailedDialRetries(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	remoteWorkerID := uuid.New()
	subscriberID := uuid.New()

	var callCount atomic.Int32

	provider := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call := callCount.Add(1)
		if call == 1 {
			// First dial: fail with an error to trigger
			// scheduleRelayReconnect via the result.parts == nil path.
			return nil, nil, nil, xerrors.New("transient dial failure")
		}
		// Second dial: succeed and return a part.
		ch := make(chan codersdk.ChatStreamEvent, 10)
		ch <- codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: "assistant",
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "retry-success"},
			},
		}
		return nil, ch, func() {}, nil
	}

	mclk := quartz.NewMock(t)
	// Trap the reconnect timer so we can fire it deterministically.
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, provider, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat in waiting state so Subscribe does not open a
	// synchronous relay.
	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "failed-dial-retry",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	// Keep the chat in waiting state so Subscribe does not attempt
	// a synchronous relay dial.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Now mark the chat as running on the remote worker in the DB.
	// The reconnect timer calls params.DB.GetChatByID to check if
	// the chat is still running on a remote worker, so this must be
	// set before we advance the clock.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: remoteWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	// Publish a running notification with a remote workerID to
	// trigger openRelayAsync. The first dial will fail, causing
	// scheduleRelayReconnect to be called.
	notify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: remoteWorkerID.String(),
	}
	payload, err := json.Marshal(notify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), payload)
	require.NoError(t, err)

	// Wait for the reconnect timer to be created (after the failed
	// dial), then advance the mock clock to fire it.
	trapReconnect.MustWait(ctx).MustRelease(ctx)
	mclk.Advance(500 * time.Millisecond).MustWait(ctx)

	// The merge loop re-checks the DB, sees the chat is still
	// running on the remote worker, and dials again. The second
	// dial succeeds.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "retry-success" {
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.GreaterOrEqual(t, int(callCount.Load()), 2)
}

// TestSubscribeRunningLocalWorkerClosesRelay verifies that when a chat
// is running on a remote worker and a pubsub notification arrives
// saying the local worker (subscriberID) now owns the chat, the
// existing relay is closed and no new dial is started (the local
// worker serves directly without relaying).
func TestSubscribeRunningLocalWorkerClosesRelay(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	remoteWorkerID := uuid.New()
	subscriberID := uuid.New()

	var callCount atomic.Int32

	provider := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call := callCount.Add(1)
		ch := make(chan codersdk.ChatStreamEvent, 10)
		if call == 1 {
			// Initial synchronous dial to the remote worker.
			ch <- codersdk.ChatStreamEvent{
				Type: codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role: "assistant",
					Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "remote-part"},
				},
			}
			// Keep channel open so the relay stays active.
		}
		return nil, ch, func() {}, nil
	}

	subscriber := newTestServer(t, db, ps, subscriberID, provider, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create the chat already running on a remote worker so Subscribe
	// opens a synchronous relay.
	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "local-worker-closes-relay",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: remoteWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Consume the remote-part from the initial relay.
	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
				event.MessagePart != nil &&
				event.MessagePart.Part.Text == "remote-part" {
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Notify that the LOCAL worker now owns the chat. This should
	// close the relay without opening a new one.
	notify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: subscriberID.String(),
	}
	payload, err := json.Marshal(notify)
	require.NoError(t, err)
	err = ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), payload)
	require.NoError(t, err)

	// Give the system time to process the notification. No additional
	// dial should happen — only the initial synchronous one.
	require.Never(t, func() bool {
		return int(callCount.Load()) > 1
	}, 2*time.Second, testutil.IntervalFast)

	require.Equal(t, 1, int(callCount.Load()),
		"only the initial synchronous dial should have happened")
}

// TestSubscribeRelayMultipleReconnects verifies that the reconnect
// loop handles multiple consecutive relay drops, proving it is
// robust across repeated iterations — not just the single reconnect
// already covered by TestSubscribeRelayReconnectsOnDrop.
func TestSubscribeRelayMultipleReconnects(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	var callCount atomic.Int32

	provider := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call := callCount.Add(1)
		ch := make(chan codersdk.ChatStreamEvent, 10)
		part := codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: "assistant",
				Part: codersdk.ChatMessagePart{
					Type: codersdk.ChatMessagePartTypeText,
					Text: fmt.Sprintf("relay-%d", call),
				},
			},
		}
		ch <- part
		if call <= 2 {
			// First two dials: close channel to simulate relay
			// drop. This triggers scheduleRelayReconnect.
			close(ch)
		}
		// Third dial: keep channel open.
		return nil, ch, func() {}, nil
	}

	mclk := quartz.NewMock(t)
	// Trap the reconnect timer so we can fire both reconnects
	// deterministically.
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, provider, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, model := seedChatDependencies(ctx, t, db)

	// Create a chat already running on a remote worker so
	// Subscribe opens a synchronous relay immediately.
	chat, err := subscriber.CreateChat(ctx, osschatd.CreateOptions{
		OwnerID:            user.ID,
		Title:              "multiple-reconnects",
		ModelConfigID:      model.ID,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: "hello"}},
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Helper to consume a specific relay part.
	consumePart := func(text string) {
		t.Helper()
		require.Eventually(t, func() bool {
			select {
			case event := <-events:
				if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
					event.MessagePart != nil &&
					event.MessagePart.Part.Text == text {
					return true
				}
				return false
			default:
				return false
			}
		}, testutil.WaitMedium, testutil.IntervalFast)
	}

	// First relay: consumed immediately (synchronous dial).
	consumePart("relay-1")

	// First relay drops → reconnect timer created. Advance clock
	// to fire it.
	trapReconnect.MustWait(ctx).MustRelease(ctx)
	mclk.Advance(500 * time.Millisecond).MustWait(ctx)

	// Second relay part.
	consumePart("relay-2")

	// Second relay drops → another reconnect timer. Advance again.
	trapReconnect.MustWait(ctx).MustRelease(ctx)
	mclk.Advance(500 * time.Millisecond).MustWait(ctx)

	// Third relay part (channel stays open).
	consumePart("relay-3")
	require.GreaterOrEqual(t, int(callCount.Load()), 3)
}
