package nats

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func Test_defaultPendingLimits(t *testing.T) {
	t.Parallel()

	const defaultBytes = 512 * 1024 * 1024
	testCases := []struct {
		name string
		in   PendingLimits
		want PendingLimits
	}{
		{
			name: "AllZero",
			in:   PendingLimits{},
			want: PendingLimits{Msgs: -1, Bytes: defaultBytes},
		},
		{
			name: "MsgsOnly",
			in:   PendingLimits{Msgs: 8},
			want: PendingLimits{Msgs: 8, Bytes: defaultBytes},
		},
		{
			name: "BytesOnly",
			in:   PendingLimits{Bytes: 1024},
			want: PendingLimits{Msgs: -1, Bytes: 1024},
		},
		{
			name: "NegativeMsgs",
			in:   PendingLimits{Msgs: -2},
			want: PendingLimits{Msgs: -2, Bytes: defaultBytes},
		},
		{
			name: "NegativeBytes",
			in:   PendingLimits{Bytes: -2},
			want: PendingLimits{Msgs: -1, Bytes: -2},
		},
		{
			name: "NegativeBoth",
			in:   PendingLimits{Msgs: -2, Bytes: -3},
			want: PendingLimits{Msgs: -2, Bytes: -3},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, defaultPendingLimits(tc.in))
		})
	}
}

func Test_pickConn(t *testing.T) {
	t.Parallel()

	t.Run("DifferentSubjects", func(t *testing.T) {
		t.Parallel()
		a := new(fakeConn)
		b := new(fakeConn)
		pool := []conn{a, b}
		ca := pickConn(pool, "a")
		cb := pickConn(pool, "b")
		require.NotSame(t, ca, cb)
	})
}

func subjectForConn(t *testing.T, pool []conn, c conn, prefix string) string {
	t.Helper()

	for i := range 10_000 {
		subject := fmt.Sprintf("%s_%d", prefix, i)
		if pickConn(pool, subject) == c {
			return subject
		}
	}
	require.FailNow(t, "no subject matched requested connection")
	return ""
}

func Test_New(t *testing.T) {
	t.Parallel()

	t.Run("ConnectionCount", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, defaultTestOptions())
		t.Cleanup(func() { _ = ps.Close() })

		const n = 50
		cancels := make([]func(), 0, n)
		for i := range n {
			c, err := ps.Subscribe(fmt.Sprintf("cc_evt_%d", i), func(_ context.Context, _ []byte) {})
			require.NoError(t, err)
			cancels = append(cancels, c)
		}
		t.Cleanup(func() {
			for _, c := range cancels {
				c()
			}
		})

		require.Equal(t, 2, ps.Server.NumClients(),
			"expected exactly 2 client connections (pubConn + subConn), got %d", ps.Server.NumClients())
		require.Len(t, ps.publishPool, 1, "default PublishConns must be 1")
		require.Len(t, ps.subscribePool, 1, "default SubscribeConns must be 1")
		require.NotSame(t, ps.publishPool[0], ps.subscribePool[0], "pubConn and subConn must be distinct")
	})
}

func Test_SubscribeWithErr(t *testing.T) {
	t.Parallel()

	t.Run("SameSubjectSharesSubscription", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil)
		ctx := testutil.Context(t, testutil.WaitShort)
		ps, err := New(ctx, logger, defaultTestOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		cancelA, err := ps.Subscribe("coalesce_evt", func(context.Context, []byte) {})
		require.NoError(t, err)
		t.Cleanup(cancelA)
		cancelB, err := ps.Subscribe("coalesce_evt", func(context.Context, []byte) {})
		require.NoError(t, err)
		t.Cleanup(cancelB)

		ps.mu.Lock()
		defer ps.mu.Unlock()
		require.Len(t, ps.subscriptions, 1)
	})
}

func Test_Pubsub_buildConnHandlers(t *testing.T) {
	t.Parallel()

	t.Run("DisconnectSignalsDropsForMatchingSubscriberConn", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)
		ctx := testutil.Context(t, testutil.WaitShort)
		ps := newPubsub(ctx, logger, defaultTestOptions())

		var subConnA, subConnB, pubConn natsgo.Conn
		ps.subscribePool = []conn{&subConnA, &subConnB}
		matchingEvent := subjectForConn(t, ps.subscribePool, &subConnA, "disconnect_match")
		otherEvent := subjectForConn(t, ps.subscribePool, &subConnB, "disconnect_other")

		newLocal := func(event string, errCh chan error) *localSub {
			queue := pubsub.NewMsgQueue(ctx, nil, func(_ context.Context, _ []byte, err error) {
				testutil.RequireSend(ctx, t, errCh, err)
			})
			// normally, closing the pubsub would clean this, but we don't actually close pubsub in this test because
			// it uses fake connections. So, we need to close these to avoid leaking goroutines.
			t.Cleanup(func() {
				queue.Close()
			})
			return &localSub{
				event: event,
				queue: queue,
			}
		}

		matchErr := make(chan error)
		matchingSub := newLocal(matchingEvent, matchErr)
		otherErr := make(chan error)
		otherSub := newLocal(otherEvent, otherErr)
		ps.subscriptions[matchingSub.event] = &groupSub{localSubs: map[*localSub]struct{}{matchingSub: {}}}
		ps.subscriptions[otherSub.event] = &groupSub{localSubs: map[*localSub]struct{}{otherSub: {}}}

		handlers := ps.buildConnHandlers()
		handlers.disconnectErr(&subConnA, xerrors.New("disconnect"))

		err := testutil.RequireReceive(ctx, t, matchErr)
		require.ErrorIs(t, err, pubsub.ErrDroppedMessages)
		select {
		case <-otherErr:
			require.Fail(t, "non-matching subscriber received drop signal")
		default:
		}

		handlers.disconnectErr(&pubConn, xerrors.New("publisher disconnect"))
		select {
		case <-otherErr:
			require.Fail(t, "publisher connection disconnect signaled subscriber")
		default:
		}
	})

	// Regression test for a deadlock: handleAsyncError used to call the
	// blocking subGetter.get while holding p.mu, and subscribeGroup's
	// error path acquired p.mu before closing subscribeDone. A
	// slow-consumer error arriving while an initial subscribe was in
	// flight, combined with that subscribe failing, wedged both
	// goroutines permanently.
	t.Run("SlowConsumerDuringInFlightSubscribeDoesNotDeadlock", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, &slogtest.Options{
			IgnoredErrorIs: []error{assert.AnError},
		})
		ctx := testutil.Context(t, testutil.WaitShort)
		ps := newPubsub(ctx, logger, defaultTestOptions())
		bc := &blockingConn{started: make(chan struct{}), unblock: make(chan struct{})}
		ps.subscribePool = []conn{bc}
		handlers := ps.buildConnHandlers()

		// Subscribe so the initial NATS subscribe is in flight, blocked
		// in bc.Subscribe.
		subErr := make(chan error, 1)
		go func() {
			_, err := ps.SubscribeWithErr("foo", func(context.Context, []byte, error) {})
			subErr <- err
		}()
		testutil.TryReceive(ctx, t, bc.started)

		// Deliver a slow-consumer async error, as the connection's async
		// callback dispatcher would. It cannot match the subscription
		// until the in-flight subscribe resolves.
		asyncDone := make(chan struct{})
		go func() {
			defer close(asyncDone)
			handlers.errH(nil, &natsgo.Subscription{}, natsgo.ErrSlowConsumer)
		}()

		// Wait until handleAsyncError is blocked waiting on the
		// in-flight subscribe.
		require.Eventually(t, func() bool {
			return goroutineBlockedInChanReceive("handleAsyncError")
		}, testutil.WaitShort, testutil.IntervalFast)

		// Fail the in-flight subscribe. Its cleanup must complete and
		// unblock handleAsyncError rather than deadlock on p.mu.
		close(bc.unblock)

		err := testutil.TryReceive(ctx, t, subErr)
		require.ErrorIs(t, err, assert.AnError)
		testutil.TryReceive(ctx, t, asyncDone)

		// The failed group must be removed so the event can be
		// subscribed again. Removal happens after subscribeDone closes,
		// so poll.
		require.Eventually(t, func() bool {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			return len(ps.subscriptions) == 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func Test_Pubsub_connectedMetric(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	ctx := testutil.Context(t, testutil.WaitShort)
	ps := newPubsub(ctx, logger, defaultTestOptions())
	handlers := ps.buildConnHandlers()

	// Two owned connections, all up.
	ps.metrics.markConnected(2)
	require.Equal(t, 1.0, promtestutil.ToFloat64(ps.metrics.connected))
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.disconnectionsTotal))

	// First disconnect drops the gauge to 0 and counts a disconnection.
	handlers.disconnectErr(nil, xerrors.New("boom"))
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.connected))
	require.Equal(t, 1.0, promtestutil.ToFloat64(ps.metrics.disconnectionsTotal))

	// Second disconnect: still down, counts again.
	handlers.disconnectErr(nil, xerrors.New("boom"))
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.connected))
	require.Equal(t, 2.0, promtestutil.ToFloat64(ps.metrics.disconnectionsTotal))

	// One reconnect with the other still down keeps the gauge at 0.
	handlers.reconnect(nil)
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.connected))

	// Once every owned connection is back the gauge returns to 1.
	handlers.reconnect(nil)
	require.Equal(t, 1.0, promtestutil.ToFloat64(ps.metrics.connected))
}

func Test_Pubsub_failureMetrics(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	ctx := testutil.Context(t, testutil.WaitShort)
	ps := newPubsub(ctx, logger, defaultTestOptions())
	// Closing makes Publish and Subscribe fail fast so we can exercise the
	// success="false" label without needing the embedded server to error.
	require.NoError(t, ps.Close())

	require.Error(t, ps.Publish("evt", []byte("payload")))
	_, err := ps.Subscribe("evt", func(context.Context, []byte) {})
	require.Error(t, err)

	require.Equal(t, 1.0, promtestutil.ToFloat64(ps.metrics.publishesTotal.WithLabelValues("false")))
	require.Equal(t, 1.0, promtestutil.ToFloat64(ps.metrics.subscribesTotal.WithLabelValues("false")))
}

func Test_Pubsub_gracefulCloseDoesNotCountDisconnect(t *testing.T) {
	t.Parallel()

	ps := newTestPubsub(t, defaultTestOptions())
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.disconnectionsTotal))
	require.Equal(t, 1.0, promtestutil.ToFloat64(ps.metrics.connected))

	handlers := ps.buildConnHandlers()
	require.NoError(t, ps.Close())

	// Close reports disconnected even though the disconnect handler is
	// suppressed for our own connection closes.
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.connected))

	// Late reconnect callbacks during the shutdown window must not flip
	// the gauge back to 1: markClosed zeroes totalConns so the connected
	// guard stays false even if every owned connection reports a
	// reconnect. Fire one per owned connection to exercise that.
	for range len(ps.publishPool) + len(ps.subscribePool) {
		handlers.reconnect(nil)
	}
	require.Equal(t, 0.0, promtestutil.ToFloat64(ps.metrics.connected))

	// Closing our own connections must not invoke the disconnect handler,
	// so disconnections_total stays 0. The async callback would fire
	// within milliseconds if it were going to, so a short window catches a
	// regression without making the test slow.
	require.Never(t, func() bool {
		return promtestutil.ToFloat64(ps.metrics.disconnectionsTotal) > 0
	}, 2*time.Second, testutil.IntervalFast)
}

func Test_localSub(t *testing.T) {
	t.Parallel()

	t.Run("SameSubjectSlowListenerDoesNotBlockPeer", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		ps, err := New(ctx, logger, defaultTestOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		release := make(chan struct{})
		defer close(release)

		// The blocking listener wedges on its first delivery and never
		// returns, so its dispatcher goroutine only ever runs the body once.
		blocked := make(chan struct{}, 1)
		slowCancel, err := ps.Subscribe("subject", func(context.Context, []byte) {
			blocked <- struct{}{}
			<-release
		})
		require.NoError(t, err)
		defer slowCancel()

		// Wedge the slow listener's dispatcher goroutine before the fast
		// listener subscribes, so the fast listener only ever sees the pings
		// published below.
		require.NoError(t, ps.Publish("subject", []byte("blocking listener")))
		require.NoError(t, ps.Flush())
		testutil.RequireReceive(ctx, t, blocked)

		var fastCount atomic.Int64
		fastCancel, err := ps.Subscribe("subject", func(context.Context, []byte) {
			fastCount.Add(1)
		})
		require.NoError(t, err)
		defer fastCancel()

		// Both listeners share one NATS subscription. The fast listener has its
		// own bounded inbox and dispatcher goroutine, so it must receive every
		// ping even though its same-subject peer is stuck. fastMsgs stays well
		// under the inbox cap, so no overflow drop is possible and the count is
		// deterministic.
		const fastMsgs = 64
		for range fastMsgs {
			require.NoError(t, ps.Publish("subject", []byte("ping")))
		}
		require.NoError(t, ps.Flush())
		require.Eventually(t, func() bool {
			return fastCount.Load() == int64(fastMsgs)
		}, testutil.WaitLong, testutil.IntervalFast,
			"fast listener must keep receiving while same-subject peer is blocked")

		// One coalesced subscription on one subConn; the slow consumer must
		// not tear it down.
		require.Len(t, ps.subscribePool, 1)
		natsConn, ok := ps.subscribePool[0].(*natsgo.Conn)
		require.True(t, ok)
		require.False(t, natsConn.IsClosed(), "subConn must not be closed by slow consumer")
		require.True(t, natsConn.IsConnected(), "subConn must stay connected")

		err = ps.Close()
		require.NoError(t, err)
		require.Empty(t, ps.subscriptions)
	})
}

func TestPubsubCluster(t *testing.T) {
	t.Parallel()
	// OK verifies that SetPeerAddresses changes the active cluster topology.
	// A starts connected to B, then C is added and receives both global and
	// C-only messages. B is then removed from A's peers, while C continues to
	// receive global and C-only messages.
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		opts := clusterTestOptions(t)
		a := newTestPubsub(t, opts)
		b := newTestPubsub(t, opts)
		c := newTestPubsub(t, opts)

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)

		require.NoError(t, a.setPeerAddresses([]string{addrB}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrB, opts.ClusterAuthToken),
		)

		globalEvent := "global"
		bGlobal := make(chan []byte, 8)
		cancelBGlobal, err := b.Subscribe(globalEvent, func(_ context.Context, msg []byte) {
			bGlobal <- msg
		})
		require.NoError(t, err)
		defer cancelBGlobal()

		waitForRouteSubscription(t, a, globalEvent)
		publishAndFlush(t, a, globalEvent, "from-a-to-b")
		require.Equal(t, "from-a-to-b", string(receiveMessage(t, bGlobal)))

		// Add C's subscriptions before adding C as an extra peer to A.
		cGlobal := make(chan []byte, 8)
		cancelCGlobal, err := c.Subscribe(globalEvent, func(_ context.Context, msg []byte) {
			cGlobal <- msg
		})
		require.NoError(t, err)
		defer cancelCGlobal()

		cSubject := "c-only-subscriber"
		cUnique := make(chan []byte, 8)
		cancelCUnique, err := c.Subscribe(cSubject, func(_ context.Context, msg []byte) {
			cUnique <- msg
		})
		require.NoError(t, err)
		defer cancelCUnique()

		// Add C to A's peer list. B and C should both receive global messages,
		// while the C-only subject should route only to C.
		require.NoError(t, a.setPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrB, opts.ClusterAuthToken),
			addrWithAuth(t, addrC, opts.ClusterAuthToken),
		)

		waitForRouteSubscription(t, a, globalEvent)
		waitForRouteSubscription(t, a, cSubject)

		publishAndFlush(t, a, globalEvent, "new-global-msg")
		require.Equal(t, "new-global-msg", string(receiveMessage(t, bGlobal)))
		require.Equal(t, "new-global-msg", string(receiveMessage(t, cGlobal)))

		publishAndFlush(t, a, cSubject, "c-unique-msg")
		require.Equal(t, "c-unique-msg", string(receiveMessage(t, cUnique)))

		// Remove B from A's peer list. Only C should receive the next messages.
		require.NoError(t, a.setPeerAddresses([]string{addrC}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrC, opts.ClusterAuthToken),
		)

		publishAndFlush(t, a, globalEvent, "no-b-peer")
		require.Equal(t, "no-b-peer", string(receiveMessage(t, cGlobal)))

		publishAndFlush(t, a, cSubject, "c-messages-still-work")
		require.Equal(t, "c-messages-still-work", string(receiveMessage(t, cUnique)))
	})

	// InvalidAuthRejected asserts the cluster route listener rejects
	// connections that do not present the configured ClusterAuthToken.
	// We dial the route listener directly with the nats.go client, which
	// surfaces a typed nats.ErrAuthorization for protocol-level -ERR
	// 'Authorization Violation' responses.
	t.Run("ClusterAuthRequired", func(t *testing.T) {
		t.Parallel()

		ps := newTestPubsub(t, clusterTestOptions(t))
		routeURL := clusterRouteAddress(t, ps)

		_, err := natsgo.Connect(routeURL,
			natsgo.Token("wrong-token"),
			natsgo.MaxReconnects(0),
			natsgo.RetryOnFailedConnect(false),
			natsgo.Timeout(testutil.WaitShort),
		)
		require.ErrorIs(t, err, natsgo.ErrAuthorization,
			"route dial with wrong token must be rejected")

		_, err = natsgo.Connect(routeURL,
			natsgo.MaxReconnects(0),
			natsgo.RetryOnFailedConnect(false),
			natsgo.Timeout(testutil.WaitShort),
		)
		require.ErrorIs(t, err, natsgo.ErrAuthorization,
			"unauthenticated route dial must be rejected")
	})

	// ClientAuthRequired asserts the local NATS client listener also requires
	// the configured ClusterAuthToken, so loopback clients cannot bypass auth.
	t.Run("ClientAuthRequired", func(t *testing.T) {
		t.Parallel()

		opts := clusterTestOptions(t)
		ps := newTestPubsub(t, opts)
		clientURL := ps.Server.ClientURL()

		_, err := natsgo.Connect(clientURL,
			natsgo.MaxReconnects(0),
			natsgo.RetryOnFailedConnect(false),
			natsgo.Timeout(testutil.WaitShort),
		)
		require.ErrorIs(t, err, natsgo.ErrAuthorization,
			"unauthenticated client connect must be rejected")

		nc, err := natsgo.Connect(clientURL,
			natsgo.Token(opts.ClusterAuthToken),
			natsgo.Timeout(testutil.WaitShort),
		)
		require.NoError(t, err, "authenticated client connect with matching token must succeed")
		nc.Close()
	})
}

func TestSubscribeError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		fConn *fakeConn
	}{
		{
			name: "Subscribe",
			fConn: &fakeConn{
				subError: assert.AnError,
			},
		},
		{
			name: "Flush",
			fConn: &fakeConn{
				flushError: assert.AnError,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, &slogtest.Options{
				IgnoredErrorIs: []error{natsgo.ErrConnectionClosed, assert.AnError},
			})
			ctx := testutil.Context(t, testutil.WaitShort)
			ps := newPubsub(ctx, logger, defaultTestOptions())
			ps.subscribePool = []conn{tc.fConn}
			cancel, err := ps.SubscribeWithErr("foo", func(ctx context.Context, message []byte, err error) {
				t.Error("should not get any events")
			})
			require.ErrorIs(t, err, assert.AnError)
			require.Nil(t, cancel)
			// subscribeDone closes before failed-group cleanup to avoid a
			// lock-order deadlock, so SubscribeWithErr may return first.
			require.Eventually(t, func() bool {
				ps.mu.Lock()
				defer ps.mu.Unlock()
				return len(ps.subscriptions) == 0
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}

// goroutineBlockedInChanReceive reports whether any goroutine is blocked
// in a channel receive with the given function name on its stack.
func goroutineBlockedInChanReceive(funcName string) bool {
	buf := make([]byte, 4<<20)
	n := runtime.Stack(buf, true)
	for _, g := range strings.Split(string(buf[:n]), "\n\n") {
		if strings.Contains(g, "[chan receive]") && strings.Contains(g, funcName) {
			return true
		}
	}
	return false
}

func defaultTestOptions() Options {
	return Options{disableCluster: true}
}

// testClusterTLSTimeout relaxes the cluster route TLS handshake timeout in
// tests. NATS defaults to a tight 2s, which is flaky under load and in CI;
// production keeps the default until it is shown to need changing.
const testClusterTLSTimeout = 10 * time.Second

func clusterTestOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		ClusterHost:       "127.0.0.1",
		ClusterPort:       natsserver.RANDOM_PORT,
		disableCluster:    false,
		ClusterAuthToken:  fmt.Sprintf("shared-token-%d", time.Now().UnixNano()),
		clusterTLSTimeout: testClusterTLSTimeout,
	}
}

func newTestPubsub(t *testing.T, opts Options) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	ps, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
	})
	return ps
}

func clusterRouteAddress(t *testing.T, ps *Pubsub) string {
	t.Helper()
	addr := ps.Server.ClusterAddr()
	require.NotNil(t, addr)
	return "nats://" + addr.String()
}

func addrWithAuth(t *testing.T, addr string, authToken string) string {
	t.Helper()
	u, err := url.Parse(addr)
	require.NoError(t, err)
	u.User = url.UserPassword(defaultClusterTokenUsername, authToken)
	return u.String()
}

func waitForRouteSubscription(t *testing.T, ps *Pubsub, subject string) {
	t.Helper()
	require.Eventually(t, func() bool {
		routes, err := ps.Server.Routez(&natsserver.RoutezOptions{Subscriptions: true})
		if err != nil {
			return false
		}
		for _, route := range routes.Routes {
			for _, sub := range route.Subs {
				if sub == subject {
					return true
				}
			}
		}
		return false
	}, testutil.WaitShort, testutil.IntervalFast)
}

func publishAndFlush(t *testing.T, ps *Pubsub, event, message string) {
	t.Helper()
	require.NoError(t, ps.Publish(event, []byte(message)))
	require.NoError(t, ps.Flush())
}

func receiveMessage(t *testing.T, got <-chan []byte) []byte {
	t.Helper()
	select {
	case msg := <-got:
		return msg
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for message")
		return nil
	}
}

func requireRoutesEqual(t *testing.T, routes []*url.URL, addresses ...string) {
	t.Helper()

	rrs := routeStrings(routes)

	slices.Sort(rrs)
	slices.Sort(addresses)

	require.True(t, slices.Equal(rrs, addresses), "want %v, got %v", rrs, addresses)
}

func routeStrings(routes []*url.URL) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.String())
	}
	return out
}

type fakeConn struct {
	subError   error
	flushError error
}

func (*fakeConn) Publish(string, []byte) error {
	// TODO implement me
	panic("implement me")
}

func (*fakeConn) Close() {
	// TODO implement me
	panic("implement me")
}

func (f *fakeConn) Flush() error {
	return f.flushError
}

func (f *fakeConn) Subscribe(string, natsgo.MsgHandler) (*natsgo.Subscription, error) {
	return &natsgo.Subscription{}, f.subError
}

// blockingConn holds Subscribe in flight until unblock is closed, then
// fails the subscribe.
type blockingConn struct {
	// started is closed when Subscribe is entered.
	started chan struct{}
	// unblock releases Subscribe, which then returns an error.
	unblock chan struct{}
}

func (*blockingConn) Publish(string, []byte) error { return nil }

func (*blockingConn) Close() {}

func (*blockingConn) Flush() error { return nil }

func (b *blockingConn) Subscribe(string, natsgo.MsgHandler) (*natsgo.Subscription, error) {
	close(b.started)
	<-b.unblock
	return nil, assert.AnError
}
