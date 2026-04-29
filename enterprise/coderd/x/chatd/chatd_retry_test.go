package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	entchatd "github.com/coder/coder/v2/enterprise/coderd/x/chatd"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// mulPhi multiplies a duration by math.Phi to compute the next
// step in retry.Retrier's φ-growth backoff sequence. If
// TestRelayReconnectUsesExponentialBackoff starts failing after a
// retry library bump, check whether the growth factor has changed.
func mulPhi(d time.Duration) time.Duration {
	return time.Duration(float64(d) * math.Phi)
}

// setChatRunningAndPublish marks the chat row as running on workerID
// and publishes a matching status notification. It keeps the DB row
// and pubsub notification in sync so the async reconnect loop
// re-dials on each timer fire (the reconnect branch re-checks DB
// status before calling openRelayAsync).
func setChatRunningAndPublish(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	chatID, workerID uuid.UUID,
) {
	t.Helper()
	now := time.Now()
	_, err := db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chatID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: now, Valid: true},
		HeartbeatAt: sql.NullTime{Time: now, Valid: true},
	})
	require.NoError(t, err)
	payload, err := json.Marshal(coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(database.ChatStatusRunning),
		WorkerID: workerID.String(),
	})
	require.NoError(t, err)
	require.NoError(t, ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chatID), payload))
}

// TestRelayDialErrorIsUnrecoverable locks the classification policy.
// Adding a new HTTP status to the unrecoverable set should force a
// test edit too.
func TestRelayDialErrorIsUnrecoverable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"unauthorized", http.StatusUnauthorized, true},
		{"forbidden", http.StatusForbidden, true},
		{"internal_server", http.StatusInternalServerError, false},
		{"bad_gateway", http.StatusBadGateway, false},
		{"service_unavailable", http.StatusServiceUnavailable, false},
		{"too_many_requests", http.StatusTooManyRequests, false},
		{"pre_response", 0, false},
		{"bad_request", http.StatusBadRequest, false},
		{"not_found", http.StatusNotFound, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &entchatd.RelayDialError{HTTPStatus: tc.status, Err: io.EOF}
			require.Equal(t, tc.want, e.IsUnrecoverable(),
				"status=%d", tc.status)
		})
	}
}

// TestRelayReconnectUsesExponentialBackoff asserts that the reconnect
// timer follows the φ-growth sequence produced by
// github.com/coder/retry's defaults, floored at relayRetryFloor.
func TestRelayReconnectUsesExponentialBackoff(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	var failCount atomic.Int32
	dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		failCount.Add(1)
		return nil, nil, nil, &entchatd.RelayDialError{
			HTTPStatus: http.StatusBadGateway,
			Err:        io.EOF,
		}
	}

	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, dialer, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
	chat := seedWaitingChat(ctx, t, db, org.ID, user, model, "relay-backoff")

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Kick the async relay loop and keep the DB row in sync so
	// each reconnect timer fire triggers another dial.
	setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID)
	// Expected sequence from retry.Retrier math:
	//   attempt 1 → floor (500ms)
	//   attempt n → prev × φ  (capped at ceil)
	floor := 500 * time.Millisecond
	expected := []time.Duration{
		floor,
		mulPhi(floor),
		mulPhi(mulPhi(floor)),
		mulPhi(mulPhi(mulPhi(floor))),
		mulPhi(mulPhi(mulPhi(mulPhi(floor)))),
	}

	for i, want := range expected {
		call := trapReconnect.MustWait(ctx)
		require.Equal(t, want, call.Duration,
			"attempt %d: want %v got %v", i+1, want, call.Duration)
		call.MustRelease(ctx)
		mclk.Advance(want).MustWait(ctx)
	}

	// We expect 1 initial attempt + 5 reconnects fired by the
	// trapped timer = 6 dials before the cap-check runs. Use
	// Eventually so we don't race the final dial goroutine that
	// the last Advance kicked off.
	require.Eventually(t, func() bool {
		return failCount.Load() >= 6
	}, testutil.WaitShort, testutil.IntervalFast,
		"expected 6 dials, got %d", failCount.Load())

	// The events channel must remain open - we're still under the
	// cap.
	select {
	case ev, open := <-events:
		if !open {
			t.Fatalf("events channel closed prematurely; retries should continue below cap")
		}
		// Allow through events that might have been queued; just
		// confirm it's not a terminal error.
		if ev.Type == codersdk.ChatStreamEventTypeError {
			t.Fatalf("unexpected terminal error: %v", ev.Error)
		}
	default:
	}
}

// TestRelayReconnectResetsOnSuccess exercises the path where a
// successful dial resets the retry state so the next failure starts
// over at the floor delay.
// TestRelayRepeatedDropsHitCap verifies the cap covers a peer that
// accepts the handshake and immediately drops it. Without a proper
// cap, such a peer would produce one reconnect per floor delay
// forever. The retry counter must accumulate across dial-success /
// parts-close cycles so the cap trips.
func TestRelayRepeatedDropsHitCap(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	opened := make(chan chan codersdk.ChatStreamEvent, 32)
	var call atomic.Int32
	dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		call.Add(1)
		ch := make(chan codersdk.ChatStreamEvent, 1)
		opened <- ch
		return nil, ch, func() {}, nil
	}

	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, dialer, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
	chat := seedWaitingChat(ctx, t, db, org.ID, user, model, "relay-drops")

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Kick off the first async dial.
	setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID)

	// Close the first dial's parts channel so the merge loop
	// schedules a reconnect. Then advance 6 reconnect timers,
	// closing the parts channel each time so the cycle is:
	//   dial -> success -> parts-close -> next() -> reconnect.
	// 1 initial dial + 6 timer-driven dials = 7 total; the 7th
	// parts-close trips the cap.
	for i := 0; i < 7; i++ {
		var ch chan codersdk.ChatStreamEvent
		select {
		case ch = <-opened:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for dial %d", i+1)
		}
		// Closing the parts channel triggers the relayPartsCh
		// close branch, which calls retryState.next() and
		// schedules the next reconnect.
		close(ch)
		if i == 6 {
			// 7th parts-close should trip the cap; no more
			// reconnect timers.
			break
		}
		call := trapReconnect.MustWait(ctx)
		call.MustRelease(ctx)
		mclk.Advance(call.Duration).MustWait(ctx)
	}

	// A terminal error event must arrive on the events channel.
	var errEvent *codersdk.ChatStreamEvent
	require.Eventually(t, func() bool {
		select {
		case ev, open := <-events:
			if !open {
				return errEvent != nil
			}
			if ev.Type == codersdk.ChatStreamEventTypeError {
				errEvent = &ev
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast,
		"expected a terminal error event after repeated drops hit cap")
	require.NotNil(t, errEvent.Error)
	require.Contains(t, errEvent.Error.Message, "relay connection failed")

	// We should have observed exactly 7 dials before tear-down.
	require.Equal(t, int32(7), call.Load(),
		"expected 7 dials (1 initial + 6 reconnect retries) before cap")
}

// TestRelayStopsAfterIntermittentCap verifies the cap-reached
// tear-down path: after N intermittent failures the merge loop emits
// one error event, closes the events channel, and stops dialing.
func TestRelayStopsAfterIntermittentCap(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	var callCount atomic.Int32
	dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		callCount.Add(1)
		return nil, nil, nil, &entchatd.RelayDialError{
			HTTPStatus: http.StatusBadGateway,
			Err:        io.EOF,
		}
	}

	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, dialer, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
	chat := seedWaitingChat(ctx, t, db, org.ID, user, model, "relay-cap")

	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID)
	// Advance through N consecutive reconnect timers. Each one
	// triggers a dial, which fails and schedules the next timer.
	// After the Nth failure the retry state says giveUp=true on
	// the next .next() call, so the merge loop tears down.
	for i := 0; i < 6; i++ {
		call := trapReconnect.MustWait(ctx)
		call.MustRelease(ctx)
		mclk.Advance(call.Duration).MustWait(ctx)
	}

	// Wait for the terminal error event to arrive. mergedEvents
	// closes inside the enterprise merge goroutine, but OSS only
	// nil-outs relayEvents on close - the outer events channel
	// stays open for pubsub/local, so we wait for the error event
	// itself rather than channel closure.
	var errEvent *codersdk.ChatStreamEvent
	require.Eventually(t, func() bool {
		select {
		case ev, open := <-events:
			if !open {
				return errEvent != nil
			}
			if ev.Type == codersdk.ChatStreamEventTypeError {
				errEvent = &ev
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast,
		"expected a terminal error event")
	require.NotNil(t, errEvent, "expected a terminal error event")
	require.NotNil(t, errEvent.Error)
	require.Contains(t, errEvent.Error.Message, "relay connection failed")
	require.Contains(t, errEvent.Error.Message, "6")

	// Ensure the cap fires at attempt N+1 - the retry state allows
	// relayMaxRetries successful next() calls before flipping
	// giveUp. With one initial dial + 6 reconnect-timer fires the
	// 7th .next() trips the cap and tears down, so we see 7 dials
	// total and nothing further.
	totalDials := callCount.Load()
	require.Equal(t, int32(7), totalDials,
		"expected exactly relayMaxRetries+1 dials before cap; got %d", totalDials)
}

// chatByIDErrorStore wraps a database.Store and forces GetChatByID
// to return a caller-supplied error once after N successful calls.
// This lets the initial Subscribe call succeed (OSS's initial state
// load needs a real Chat to wire up the relay) while subsequent
// reconnect-branch calls exercise the DB-error retry path.
type chatByIDErrorStore struct {
	database.Store
	err      error
	okRemain atomic.Int32 // number of calls allowed to delegate before erroring.
}

func (s *chatByIDErrorStore) GetChatByID(ctx context.Context, id uuid.UUID) (database.Chat, error) {
	if s.okRemain.Add(-1) >= 0 {
		return s.Store.GetChatByID(ctx, id)
	}
	return database.Chat{}, s.err
}

// TestRelayReconnectStopsAfterDBErrorCap verifies the reconnect-timer
// branch's DB-error path shares the same retry budget as dial
// failures and trips the cap after enough consecutive DB errors.
func TestRelayReconnectStopsAfterDBErrorCap(t *testing.T) {
	t.Parallel()

	realDB, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	var callCount atomic.Int32
	dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		callCount.Add(1)
		return nil, nil, nil, &entchatd.RelayDialError{
			HTTPStatus: http.StatusBadGateway,
			Err:        io.EOF,
		}
	}

	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	// The server sees a DB whose GetChatByID always errors after
	// the initial Subscribe snapshot load. Other methods delegate
	// to the real DB, so seeding below still works.
	failingDB := &chatByIDErrorStore{
		Store: realDB,
		err:   xerrors.New("mock: GetChatByID always fails"),
	}
	// Allow one successful GetChatByID (the Subscribe preamble's
	// initial state load). All subsequent calls return the mock
	// error, exercising the reconnect-branch DB-error path.
	failingDB.okRemain.Store(1)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, realDB)
	chat := seedWaitingChat(ctx, t, realDB, org.ID, user, model, "relay-db-error")

	subscriber := newTestServer(t, failingDB, ps, subscriberID, dialer, mclk)
	_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Flip to running so the merge loop starts an async dial. The
	// dial fails (attempts=1, reconnect scheduled). From there each
	// reconnect timer fires, the merge loop calls GetChatByID, the
	// failing DB returns an error, and retryState.next() increments.
	//
	// Budget: 1 dial-failure + 6 DB-failures = 7 next() calls; the
	// 7th trips the cap.
	setChatRunningAndPublish(ctx, t, realDB, ps, chat.ID, workerID)
	for i := 0; i < 6; i++ {
		call := trapReconnect.MustWait(ctx)
		call.MustRelease(ctx)
		mclk.Advance(call.Duration).MustWait(ctx)
	}

	var errEvent *codersdk.ChatStreamEvent
	require.Eventually(t, func() bool {
		select {
		case ev, open := <-events:
			if !open {
				return errEvent != nil
			}
			if ev.Type == codersdk.ChatStreamEventTypeError {
				errEvent = &ev
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast,
		"expected terminal error event after DB-error cap")
	require.NotNil(t, errEvent.Error)
	require.Contains(t, errEvent.Error.Message, "relay connection failed")
	require.Contains(t, errEvent.Error.Message, "6")

	// Exactly 1 dial fired: the one that triggered the initial
	// reconnect schedule. All subsequent next() calls come from the
	// DB-error branch without calling the dialer.
	require.Equal(t, int32(1), callCount.Load(),
		"expected exactly 1 dial; reconnects should short-circuit on DB error")
}

// TestRelayStopsImmediatelyOnUnauthorized tests the unrecoverable
// branch and its table of status codes.
func TestRelayStopsImmediatelyOnUnauthorized(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		status            int
		wantUnrecoverable bool
		wantMsgContains   string
	}{
		{"401", http.StatusUnauthorized, true, "401"},
		{"403", http.StatusForbidden, true, "403"},
		{"500_intermittent", http.StatusInternalServerError, false, ""},
		{"zero_intermittent", 0, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			workerID := uuid.New()
			subscriberID := uuid.New()

			var callCount atomic.Int32
			dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
				[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
			) {
				callCount.Add(1)
				return nil, nil, nil, &entchatd.RelayDialError{
					HTTPStatus: tc.status,
					Err:        io.EOF,
				}
			}

			mclk := quartz.NewMock(t)
			trapReconnect := mclk.Trap().NewTimer("reconnect")
			defer trapReconnect.Close()

			subscriber := newTestServer(t, db, ps, subscriberID, dialer, mclk)

			ctx := testutil.Context(t, testutil.WaitLong)
			user, org, model := seedChatDependencies(ctx, t, db)
			chat := seedWaitingChat(ctx, t, db, org.ID, user, model,
				"relay-unrec-"+tc.name)

			_, events, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
			require.True(t, ok)
			t.Cleanup(cancel)

			setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID)
			if tc.wantUnrecoverable {
				// First dial should tear the relay down.
				var errEvent *codersdk.ChatStreamEvent
				require.Eventually(t, func() bool {
					select {
					case ev, open := <-events:
						if !open {
							return errEvent != nil
						}
						if ev.Type == codersdk.ChatStreamEventTypeError {
							errEvent = &ev
							return true
						}
						return false
					default:
						return false
					}
				}, testutil.WaitShort, testutil.IntervalFast,
					"expected terminal error event")
				require.NotNil(t, errEvent)
				require.Contains(t, errEvent.Error.Message, "relay authentication failed")
				require.Contains(t, errEvent.Error.Message, tc.wantMsgContains)
				require.Equal(t, int32(1), callCount.Load(),
					"unrecoverable errors must not retry; got %d dials", callCount.Load())
			} else {
				// Intermittent: fire one reconnect timer
				// and confirm the dialer is called again.
				call := trapReconnect.MustWait(ctx)
				call.MustRelease(ctx)
				mclk.Advance(call.Duration).MustWait(ctx)
				require.Eventually(t, func() bool {
					return callCount.Load() >= 2
				}, testutil.WaitShort, testutil.IntervalFast,
					"intermittent should retry at least once")
			}
		})
	}
}

// TestRelayBackoffResetsOnStatusChange checks that closeRelay (driven
// by a status notification) resets the retry counter so subsequent
// dials against a new target start at the floor delay.
func TestRelayBackoffResetsOnStatusChange(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID1 := uuid.New()
	workerID2 := uuid.New()
	subscriberID := uuid.New()

	dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		return nil, nil, nil, &entchatd.RelayDialError{
			HTTPStatus: http.StatusBadGateway,
			Err:        io.EOF,
		}
	}

	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, dialer, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
	chat := seedWaitingChat(ctx, t, db, org.ID, user, model, "relay-reset-on-status")

	_, _, cancel, ok := subscriber.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Drive the async openRelayAsync path with workerID1.
	setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID1)

	// Drive 3 intermittent failures so attempts=3 and the delay
	// has grown past the floor. After each loop iteration the 4th
	// reconnect timer is queued - consume it too so our later
	// assertion sees the reset's timer, not a stale one.
	for i := 0; i < 3; i++ {
		call := trapReconnect.MustWait(ctx)
		call.MustRelease(ctx)
		mclk.Advance(call.Duration).MustWait(ctx)
	}
	// Grab the next trapped timer (the grown one scheduled after
	// the 3rd dial fails) but don't advance it - we want to see it
	// replaced by a fresh floor-delay timer after the reset.
	grown := trapReconnect.MustWait(ctx)
	require.Greater(t, grown.Duration, 500*time.Millisecond,
		"sanity: pre-reset delay should have grown past the floor")
	grown.MustRelease(ctx)

	// Flip the chat to waiting; closeRelay runs (because the
	// status notification no longer points at a running peer) and
	// should reset the retry state.
	_, err := db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)
	waitingPayload, err := json.Marshal(coderdpubsub.ChatStreamNotifyMessage{
		Status: string(database.ChatStatusWaiting),
	})
	require.NoError(t, err)
	require.NoError(t, ps.Publish(coderdpubsub.ChatStreamNotifyChannel(chat.ID), waitingPayload))

	// Flip back to running on a different worker. This triggers a
	// fresh openRelayAsync which fails, arming a reconnect timer.
	// That timer's delay must be the floor, proving the reset.
	setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID2)

	call := trapReconnect.MustWait(ctx)
	require.Equal(t, 500*time.Millisecond, call.Duration,
		"retry state must reset after status change; got grown delay %v", call.Duration)
	call.MustRelease(ctx)
}

// TestRelayBackoffRespectsContextCancel is a regression guard: the
// reconnect timer must respect ctx cancellation promptly.
func TestRelayBackoffRespectsContextCancel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	dialer := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ http.Header) (
		[]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), error,
	) {
		return nil, nil, nil, &entchatd.RelayDialError{
			HTTPStatus: http.StatusBadGateway,
			Err:        io.EOF,
		}
	}

	mclk := quartz.NewMock(t)
	trapReconnect := mclk.Trap().NewTimer("reconnect")
	defer trapReconnect.Close()

	subscriber := newTestServer(t, db, ps, subscriberID, dialer, mclk)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
	chat := seedWaitingChat(ctx, t, db, org.ID, user, model, "relay-cancel")

	subCtx, subCancel := context.WithCancel(ctx)
	_, events, cancel, ok := subscriber.Subscribe(subCtx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	setChatRunningAndPublish(ctx, t, db, ps, chat.ID, workerID)

	// Wait for the first reconnect timer to arm.
	call := trapReconnect.MustWait(ctx)
	call.MustRelease(ctx)

	// Cancel the subscriber context. The events channel should
	// close promptly (the merge goroutine's select exits on
	// ctx.Done).
	subCancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, open := <-events; !open {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(testutil.WaitShort):
		t.Fatal("events channel did not close after ctx cancel")
	}
}

// TestDialRelayReal401 exercises the real dialRelay path against an
// httptest server that returns 401 on the stream endpoint. It
// validates that the websocket library's handshake failure
// propagates through as *RelayDialError with HTTPStatus == 401.
//
// This is the one test that uses the real coder/websocket library
// on the failure path - a safety net against library upgrades
// silently breaking status capture.
func TestDialRelayReal401(t *testing.T) {
	t.Parallel()

	// An httptest server that 401s every request on the stream
	// endpoint. Any other path gets a 404.
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !streamPathRE.MatchString(r.URL.Path) {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusUnauthorized)
		_, _ = rw.Write([]byte(`{"message":"unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	db, _ := dbtestutil.NewDB(t)
	workerID := uuid.New()
	subscriberID := uuid.New()

	// Wire real config (no DialerFn override) so dialRelay runs
	// end-to-end against the httptest server. Seeding a waiting
	// chat (below) keeps Subscribe's initial synchronous dial a
	// no-op; we then push a running status notification to the
	// merge loop so it invokes dialRelay via the async path, where
	// the 401 tear-down logic lives.
	cfg := entchatd.MultiReplicaSubscribeConfig{
		ResolveReplicaAddress: func(_ context.Context, _ uuid.UUID) (string, bool) {
			return srv.URL, true
		},
		ReplicaHTTPClient: srv.Client(),
		ReplicaIDFn:       func() uuid.UUID { return subscriberID },
	}
	subscribeFn := entchatd.NewMultiReplicaSubscribeFn(cfg)

	ctx := testutil.Context(t, testutil.WaitMedium)
	user, org, model := seedChatDependencies(ctx, t, db)
	// Seed a waiting chat - no sync dial - then push a running
	// status notification to trigger the async dial via the real
	// dialRelay path.
	chat := seedWaitingChat(ctx, t, db, org.ID, user, model, "relay-real-401")

	statusCh := make(chan osschatd.StatusNotification, 1)
	evs := subscribeFn(ctx, osschatd.SubscribeFnParams{
		ChatID:              chat.ID,
		Chat:                chat,
		WorkerID:            subscriberID,
		StatusNotifications: statusCh,
		RequestHeader:       http.Header{codersdk.SessionTokenHeader: {"test-token"}},
		DB:                  db,
		Logger:              slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	statusCh <- osschatd.StatusNotification{
		Status:   database.ChatStatusRunning,
		WorkerID: workerID,
	}

	// Wait for a terminal error event. On a real 401 handshake,
	// the classifier flags it unrecoverable → one dial, then
	// error event, then channel close.
	var errEvent *codersdk.ChatStreamEvent
	deadline := time.After(testutil.WaitMedium)
waitErr:
	for {
		select {
		case ev, open := <-evs:
			if !open {
				break waitErr
			}
			if ev.Type == codersdk.ChatStreamEventTypeError {
				errEvent = &ev
			}
		case <-deadline:
			break waitErr
		}
	}

	require.NotNil(t, errEvent, "expected terminal error event from real 401 dial")
	require.NotNil(t, errEvent.Error)
	require.Contains(t, errEvent.Error.Message, "relay authentication failed")
	require.Contains(t, errEvent.Error.Message, "401")
}

// streamPathRE matches the chat stream endpoint path built by
// buildRelayURL. Compiled at package scope so the httptest handler
// below doesn't pay regexp.Compile per request.
var streamPathRE = regexp.MustCompile(
	`^/api/experimental/chats/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/stream$`,
)
