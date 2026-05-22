package nats //nolint:testpackage // Exercises internal pubsub state and helpers.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func Test_parsePeerAddresses(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			" nats://127.0.0.1:4222 ",
			"nats://[::1]:7222",
			"nats://example.com:6222",
		})
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://127.0.0.1:4222",
			"nats://[::1]:7222",
			"nats://example.com:6222",
		}, routeStrings(routes))

		routes[0].Host = "mutated:4222"
		routes2, err := parsePeerAddresses([]string{"nats://127.0.0.1:4222"})
		require.NoError(t, err)
		require.Equal(t, "nats://127.0.0.1:4222", routes2[0].String())
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses(nil)
		require.NoError(t, err)
		require.Empty(t, routes)
	})

	t.Run("FiltersSortsAndDedupes", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			"nats://b.example:6222",
			"nats://a.example:6222",
			"nats://b.example:6222",
			"nats://self.example:6222",
		}, "self.example:6222")
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://a.example:6222",
			"nats://b.example:6222",
		}, routeStrings(routes))
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		for _, address := range []string{
			"",
			"   ",
			"http://127.0.0.1:4222",
			"nats://127.0.0.1",
			"nats://:4222",
			"nats://127.0.0.1:0",
			"nats://127.0.0.1:bad",
			"nats://user@127.0.0.1:4222",
			"nats://127.0.0.1:4222/path",
			"nats://127.0.0.1:4222?x=1",
			"nats://127.0.0.1:4222#frag",
		} {
			t.Run(address, func(t *testing.T) {
				t.Parallel()
				_, err := parsePeerAddresses([]string{address})
				require.Error(t, err)
			})
		}
	})
}

//nolint:paralleltest // Cluster tests bind free ports and reload shared route state.
func TestPubsubCluster(t *testing.T) {
	t.Run("LocalRoundTrip", func(t *testing.T) {
		ps := newTestPubsub(t, newClusterTestOptions(t))
		event := uniqueSubject("local")
		got := make(chan []byte, 1)
		cancel, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish(event, []byte("hello")))
		require.NoError(t, ps.Flush())
		require.Equal(t, "hello", string(receiveMessage(t, got)))
	})

	t.Run("SetPeerAddressesReloadsConfiguredRoutes", func(t *testing.T) {
		a := newTestPubsub(t, newClusterTestOptions(t))
		b := newTestPubsub(t, newClusterTestOptions(t))
		c := newTestPubsub(t, newClusterTestOptions(t))

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrB}))
		waitForRoutes(t, a, 1)
		waitForRoutes(t, b, 1)

		sharedEvent := uniqueSubject("shared")
		gotBShared := make(chan []byte, 8)
		cancelBShared, err := b.Subscribe(sharedEvent, func(_ context.Context, msg []byte) {
			gotBShared <- msg
		})
		require.NoError(t, err)
		defer cancelBShared()

		publishAndFlush(t, a, sharedEvent, "from-a-to-b")
		require.Equal(t, "from-a-to-b", string(receiveMessage(t, gotBShared)))

		gotCShared := make(chan []byte, 8)
		cancelCShared, err := c.Subscribe(sharedEvent, func(_ context.Context, msg []byte) {
			gotCShared <- msg
		})
		require.NoError(t, err)
		defer cancelCShared()

		eventC := uniqueSubject("add-c")
		gotCUnique := make(chan []byte, 8)
		cancelCUnique, err := c.Subscribe(eventC, func(_ context.Context, msg []byte) {
			gotCUnique <- msg
		})
		require.NoError(t, err)
		defer cancelCUnique()

		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes, addrB, addrC)
		waitForRoutes(t, a, 2)
		waitForRoutes(t, c, 1)

		publishAndFlush(t, a, sharedEvent, "from-a-to-b-and-c")
		require.Equal(t, "from-a-to-b-and-c", string(receiveMessage(t, gotBShared)))
		require.Equal(t, "from-a-to-b-and-c", string(receiveMessage(t, gotCShared)))

		publishAndFlush(t, a, eventC, "from-a-to-c")
		require.Equal(t, "from-a-to-c", string(receiveMessage(t, gotCUnique)))

		require.NoError(t, a.SetPeerAddresses([]string{addrC}))
		requireRoutesEqual(t, a.currentRoutes, addrC)

		publishAndFlush(t, a, sharedEvent, "after-remove-b-shared")
		require.Equal(t, "after-remove-b-shared", string(receiveMessage(t, gotCShared)))

		publishAndFlush(t, a, eventC, "after-remove-b-unique")
		require.Equal(t, "after-remove-b-unique", string(receiveMessage(t, gotCUnique)))
	})

	t.Run("SetPeerAddressesStandaloneConfigError", func(t *testing.T) {
		ps := newTestPubsub(t, Options{})
		err := ps.SetPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("SetPeerAddressesClosed", func(t *testing.T) {
		ps := newTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.Close())
		err := ps.SetPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("SetPeerAddressesDropsSelfRoute", func(t *testing.T) {
		ps := newTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.SetPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}

func newClusterTestOptions(t *testing.T) Options {
	t.Helper()
	return Options{ClusterPort: freeTCPPort(t)}
}

func freeTCPPort(t *testing.T) int {
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
	return "nats://" + addr.String()
}

func waitForRoutes(t *testing.T, ps *Pubsub, minRoutes int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return ps.ns.NumRoutes() >= minRoutes
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
