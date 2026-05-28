package nats

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
		var a, b natsgo.Conn
		pool := []*natsgo.Conn{&a, &b}

		require.NotSame(t, pickConn(pool, "a"), pickConn(pool, "b"))
	})
}

func subjectForConn(t *testing.T, pool []*natsgo.Conn, conn *natsgo.Conn, prefix string) string {
	t.Helper()

	for i := 0; i < 10_000; i++ {
		subject := fmt.Sprintf("%s_%d", prefix, i)
		if pickConn(pool, subject) == conn {
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
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		ps, err := New(ctx, logger, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		const n = 50
		cancels := make([]func(), 0, n)
		for i := 0; i < n; i++ {
			c, err := ps.Subscribe(fmt.Sprintf("cc_evt_%d", i), func(context.Context, []byte) {})
			require.NoError(t, err)
			cancels = append(cancels, c)
		}
		t.Cleanup(func() {
			for _, c := range cancels {
				c()
			}
		})

		require.Equal(t, 2, ps.ns.NumClients(),
			"expected exactly 2 client connections (pubConn + subConn), got %d", ps.ns.NumClients())
		require.Len(t, ps.publishPool, 1, "default PublishConns must be 1")
		require.Len(t, ps.subscribePool, 1, "default SubscribeConns must be 1")
		require.NotSame(t, ps.publishPool[0], ps.subscribePool[0], "pubConn and subConn must be distinct")
	})
}

func Test_SubscribeWithErr(t *testing.T) {
	t.Parallel()

	t.Run("SameSubjectSharesSubscription", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		ps, err := New(ctx, logger, Options{})
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
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ps := newPubsub(ctx, logger, Options{})

		var subConnA, subConnB, pubConn natsgo.Conn
		ps.subscribePool = []*natsgo.Conn{&subConnA, &subConnB}
		matchingEvent := subjectForConn(t, ps.subscribePool, &subConnA, "disconnect_match")
		otherEvent := subjectForConn(t, ps.subscribePool, &subConnB, "disconnect_other")

		newLocal := func(event string) *localSub {
			return &localSub{
				event:      event,
				dropSignal: make(chan struct{}, 1),
			}
		}

		matchingSub := newLocal(matchingEvent)
		otherSub := newLocal(otherEvent)
		ps.subscriptions[matchingSub.event] = &natsSub{localSubs: map[*localSub]struct{}{matchingSub: {}}}
		ps.subscriptions[otherSub.event] = &natsSub{localSubs: map[*localSub]struct{}{otherSub: {}}}

		handlers := ps.buildConnHandlers()
		handlers.disconnectErr(&subConnA, xerrors.New("disconnect"))

		select {
		case <-matchingSub.dropSignal:
		default:
			require.Fail(t, "matching subscriber did not receive drop signal")
		}
		select {
		case <-otherSub.dropSignal:
			require.Fail(t, "non-matching subscriber received drop signal")
		default:
		}

		handlers.disconnectErr(&pubConn, xerrors.New("publisher disconnect"))
		select {
		case <-otherSub.dropSignal:
			require.Fail(t, "publisher connection disconnect signaled subscriber")
		default:
		}
	})
}

func Test_localSub_init(t *testing.T) {
	t.Parallel()

	t.Run("SerializesCallbacks", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		dataStarted := make(chan struct{})
		dropDelivered := make(chan struct{})
		release := make(chan struct{})
		var dataOnce sync.Once
		var dropOnce sync.Once
		var releaseOnce sync.Once
		var active atomic.Int64
		var concurrent atomic.Bool

		s := &localSub{
			ctx:    ctx,
			cancel: cancel,
			listener: func(_ context.Context, _ []byte, ferr error) {
				if active.Add(1) != 1 {
					concurrent.Store(true)
				}
				defer active.Add(-1)

				if errors.Is(ferr, pubsub.ErrDroppedMessages) {
					dropOnce.Do(func() { close(dropDelivered) })
					return
				}

				dataOnce.Do(func() { close(dataStarted) })
				<-release
			},
			queue:      make(chan []byte, 1),
			dropSignal: make(chan struct{}, 1),
		}
		s.init()
		t.Cleanup(func() {
			releaseOnce.Do(func() { close(release) })
			s.close()
		})

		s.enqueue([]byte("data"))
		require.Eventually(t, func() bool {
			select {
			case <-dataStarted:
				return true
			default:
				return false
			}
		}, testutil.WaitShort, testutil.IntervalFast)

		s.signalDrop()
		require.Never(t, func() bool {
			select {
			case <-dropDelivered:
				return true
			default:
				return false
			}
		}, testutil.IntervalMedium, testutil.IntervalFast,
			"drop callback must wait for the blocked data callback")
		require.False(t, concurrent.Load(), "listener callback ran concurrently")

		releaseOnce.Do(func() { close(release) })
		require.Eventually(t, func() bool {
			select {
			case <-dropDelivered:
				return true
			default:
				return false
			}
		}, testutil.WaitShort, testutil.IntervalFast)
		require.False(t, concurrent.Load(), "listener callback ran concurrently")
	})

	t.Run("CrossSubjectListenerIsolation", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		ps, err := New(ctx, logger, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		release := make(chan struct{})
		var releaseOnce sync.Once
		var slowDrops atomic.Int64
		var slowBlocked atomic.Bool
		slowCancel, err := ps.SubscribeWithErr("iso_slow", func(_ context.Context, _ []byte, ferr error) {
			if ferr != nil && errors.Is(ferr, pubsub.ErrDroppedMessages) {
				slowDrops.Add(1)
				return
			}
			if slowBlocked.CompareAndSwap(false, true) {
				<-release
			}
		})
		require.NoError(t, err)
		defer slowCancel()

		var fastCount atomic.Int64
		fastCancel, err := ps.Subscribe("iso_fast", func(_ context.Context, _ []byte) {
			fastCount.Add(1)
		})
		require.NoError(t, err)
		defer fastCancel()
		defer releaseOnce.Do(func() { close(release) })

		total := defaultListenerQueueSize + 256
		payload := make([]byte, 4*1024)
		for i := 0; i < total; i++ {
			require.NoError(t, ps.Publish("iso_slow", payload))
			require.NoError(t, ps.Publish("iso_fast", []byte("ping")))
		}
		require.NoError(t, ps.Flush())

		require.Eventually(t, func() bool {
			return fastCount.Load() >= int64(total)
		}, testutil.WaitLong, testutil.IntervalFast)
		require.Zero(t, slowDrops.Load(),
			"drop callback must wait for the blocked data callback")
		releaseOnce.Do(func() { close(release) })
		require.Eventually(t, func() bool {
			return slowDrops.Load() >= 1
		}, testutil.WaitLong, testutil.IntervalFast,
			"slow subscriber must receive at least one ErrDroppedMessages signal")

		require.GreaterOrEqual(t, fastCount.Load(), int64(total),
			"fast subscriber must keep receiving despite slow peer on shared subConn")
		require.Len(t, ps.subscribePool, 1)
		require.False(t, ps.subscribePool[0].IsClosed(), "subConn must not be closed by slow consumer")
		require.True(t, ps.subscribePool[0].IsConnected(), "subConn must stay connected")
		require.Equal(t, 2, ps.ns.NumClients(), "slow consumer must not disconnect subConn")
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

		a := newTestPubsub(t, newClusterTestOptions(t))
		b := newTestPubsub(t, newClusterTestOptions(t))
		c := newTestPubsub(t, newClusterTestOptions(t))

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrB}))
		waitForRoutes(t, a, 1)
		waitForRoutes(t, b, 1)

		globalEvent := uniqueSubject("global")
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

		cSubject := uniqueSubject("c-only-subscriber")
		cUnique := make(chan []byte, 8)
		cancelCUnique, err := c.Subscribe(cSubject, func(_ context.Context, msg []byte) {
			cUnique <- msg
		})
		require.NoError(t, err)
		defer cancelCUnique()

		// Add C to A's peer list. B and C should both receive global messages,
		// while the C-only subject should route only to C.
		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes, addrB, addrC)
		waitForRoutes(t, a, 2)
		waitForRoutes(t, c, 1)
		waitForRouteSubscription(t, a, globalEvent)
		waitForRouteSubscription(t, a, cSubject)

		publishAndFlush(t, a, globalEvent, "new-global-msg")
		require.Equal(t, "new-global-msg", string(receiveMessage(t, bGlobal)))
		require.Equal(t, "new-global-msg", string(receiveMessage(t, cGlobal)))

		publishAndFlush(t, a, cSubject, "c-unique-msg")
		require.Equal(t, "c-unique-msg", string(receiveMessage(t, cUnique)))

		// Remove B from A's peer list. Only C should receive the next messages.
		require.NoError(t, a.SetPeerAddresses([]string{addrC}))
		requireRoutesEqual(t, a.currentRoutes, addrC)

		publishAndFlush(t, a, globalEvent, "no-b-peer")
		require.Equal(t, "no-b-peer", string(receiveMessage(t, cGlobal)))

		publishAndFlush(t, a, cSubject, "c-messages-still-work")
		require.Equal(t, "c-messages-still-work", string(receiveMessage(t, cUnique)))
	})
}

func newClusterTestOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		ClusterHost: "127.0.0.1",
		ClusterPort: tcpPort(t),
	}
}

func tcpPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return addr.Port
}

func newTestPubsub(t *testing.T, opts Options) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithCancel(context.Background())
	ps, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
		cancel()
	})
	return ps
}

func clusterRouteAddress(t *testing.T, ps *Pubsub) string {
	t.Helper()
	addr := ps.ns.ClusterAddr()
	require.NotNil(t, addr)
	return addr.String()
}

func waitForRoutes(t *testing.T, ps *Pubsub, minRoutes int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return ps.ns.NumRoutes() >= minRoutes
	}, testutil.WaitLong, testutil.IntervalFast)
}

func waitForConfiguredRouteAddresses(t *testing.T, ps *Pubsub, addresses ...string) {
	t.Helper()
	want := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		normalized, err := normalizeHostPort(address)
		require.NoError(t, err)
		want[normalized] = struct{}{}
	}

	require.Eventually(t, func() bool {
		routes, err := ps.ns.Routez(nil)
		if err != nil {
			return false
		}

		got := make(map[string]struct{}, len(routes.Routes))
		for _, route := range routes.Routes {
			if !route.IsConfigured || route.IP == "" || route.Port == 0 {
				continue
			}
			got[net.JoinHostPort(route.IP, strconv.Itoa(route.Port))] = struct{}{}
		}
		if len(got) != len(want) {
			return false
		}
		for address := range want {
			if _, ok := got[address]; !ok {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
}

func waitForRouteSubscription(t *testing.T, ps *Pubsub, subject string) {
	t.Helper()
	require.Eventually(t, func() bool {
		routes, err := ps.ns.Routez(&natsserver.RoutezOptions{Subscriptions: true})
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
	}, testutil.WaitLong, testutil.IntervalFast)
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
	want, err := parsePeerAddresses(addresses)
	require.NoError(t, err)
	require.True(t, routeURLsEqual(want, routes), "want %v, got %v", routeStrings(want), routeStrings(routes))
}

func routeStrings(routes []*url.URL) []string {
	strings := make([]string, 0, len(routes))
	for _, route := range routes {
		strings = append(strings, route.String())
	}
	return strings
}

func uniqueSubject(prefix string) string {
	return fmt.Sprintf("cluster.%s.%d", prefix, time.Now().UnixNano())
}
