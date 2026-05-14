//nolint:testpackage // Uses internal fields and helpers for sub-pool assertions.
package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

// newSubPoolPubsub is a small helper that builds a Pubsub with the
// requested SubscribeConns count and ensures it is closed on test
// cleanup.
func newSubPoolPubsub(t *testing.T, subConns int) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{SubscribeConns: subConns})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

// TestSubscribePool_DefaultIsOne asserts that the historical zero-value
// Options preserves single-subscriber-connection behavior: exactly one
// owned subConn, distinct from the single owned pubConn, and exactly
// two server-side client connections.
func TestSubscribePool_DefaultIsOne(t *testing.T) {
	t.Parallel()
	ps := newSubPoolPubsub(t, 0)
	require.Len(t, ps.subConns, 1, "SubscribeConns=0 must default to a single subscribe connection")
	require.True(t, ps.ownsSubConns, "New must own its subscribe connections")
	require.Len(t, ps.pubConns, 1, "default PublishConns must be 1")
	require.NotSame(t, ps.subConns[0], ps.pubConns[0], "subConns[0] and pubConns[0] must be distinct connections")
	require.Equal(t, 2, ps.ns.NumClients(),
		"default Options must produce exactly 2 server-side client connections (1 pub + 1 sub)")
}

// TestSubscribePool_NegativeDefaults asserts that a negative
// SubscribeConns value is normalized to 1 rather than producing zero
// connections or erroring.
func TestSubscribePool_NegativeDefaults(t *testing.T) {
	t.Parallel()
	ps := newSubPoolPubsub(t, -5)
	require.Len(t, ps.subConns, 1, "negative SubscribeConns must default to a single subscribe connection")
}

// TestSubscribePool_CreatesN asserts that Options.SubscribeConns=N
// creates exactly N owned subscriber connections plus the default
// single publisher connection, and that all entries are non-nil and
// connected.
func TestSubscribePool_CreatesN(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)
	require.Len(t, ps.subConns, n)
	require.True(t, ps.ownsSubConns)
	seen := make(map[*natsgo.Conn]struct{}, n)
	for i, nc := range ps.subConns {
		require.NotNil(t, nc, "subConns[%d] must be non-nil", i)
		require.True(t, nc.IsConnected(), "subConns[%d] must be connected", i)
		require.NotSame(t, nc, ps.pubConns[0], "subConns[%d] must be distinct from pubConns[0]", i)
		_, dup := seen[nc]
		require.False(t, dup, "subConns[%d] must be a distinct *natsgo.Conn", i)
		seen[nc] = struct{}{}
	}
	// The server must report exactly N sub conns + 1 pub conn.
	require.Equal(t, n+1, ps.ns.NumClients(),
		"server must observe exactly %d client connections (1 pub + %d sub)", n+1, n)
}

// TestSubscribePool_PickSubConn_StablePerSubject asserts that
// pickSubConn is deterministic: repeated calls for the same subject
// always return the same connection, with no per-process
// randomization.
func TestSubscribePool_PickSubConn_StablePerSubject(t *testing.T) {
	t.Parallel()
	ps := newSubPoolPubsub(t, 8)
	subjects := []string{
		"coder.v1.event.alpha",
		"coder.v1.event.beta",
		"coder.v1.event.gamma",
		"coder.v1.event.delta",
		"coder.v1.event.epsilon",
		"coder.v1.event.zeta",
	}
	for _, s := range subjects {
		first := ps.pickSubConn(s)
		for i := 0; i < 32; i++ {
			require.Same(t, first, ps.pickSubConn(s),
				"pickSubConn(%q) must be stable across calls", s)
		}
	}
}

// TestSubscribePool_SingleConnPicksOnlyEntry asserts that with a
// single subscribe connection pickSubConn always returns the one
// entry, even for many distinct subjects.
func TestSubscribePool_SingleConnPicksOnlyEntry(t *testing.T) {
	t.Parallel()
	ps := newSubPoolPubsub(t, 1)
	only := ps.subConns[0]
	for i := 0; i < 32; i++ {
		subj := fmt.Sprintf("coder.v1.legacy.solo_%03d", i)
		require.Same(t, only, ps.pickSubConn(subj))
	}
}

// TestSubscribePool_PickSubConn_DistributesSubjects asserts that
// pickSubConn spreads a moderate variety of distinct subjects across
// multiple entries of the pool. We do not require uniform distribution
// (FNV-1a does not guarantee that), but we do require that not every
// subject hashes to the same connection, which would defeat the
// whole optimization.
func TestSubscribePool_PickSubConn_DistributesSubjects(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)
	counts := make(map[*natsgo.Conn]int, n)
	for i := 0; i < 64; i++ {
		subj := fmt.Sprintf("coder.v1.legacy.event_%03d", i)
		counts[ps.pickSubConn(subj)]++
	}
	require.GreaterOrEqual(t, len(counts), 2,
		"pickSubConn must distribute 64 distinct subjects across at least 2 of %d conns, got %d", n, len(counts))
}

// TestSubscribePool_SubscribeUsesHashedConn creates Subscribes for a
// set of subjects spanning >=2 subscriber conns and verifies that
// each subscriber conn's Stats().InMsgs grows only for subjects that
// pickSubConn assigned to it. This confirms that the underlying
// *natsgo.Subscription for a subject lives on the hashed conn (not
// always subConns[0]).
func TestSubscribePool_SubscribeUsesHashedConn(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)

	// Find a set of legacy event names that covers >=2 distinct
	// subscriber conns. Bounded loop in case the hash distribution
	// is degenerate over a small label set.
	type entry struct {
		event string
		conn  *natsgo.Conn
	}
	expectedPerConn := make(map[*natsgo.Conn]int, n)
	var events []entry
	for i := 0; len(expectedPerConn) < 2 && i < 4096; i++ {
		evt := fmt.Sprintf("sub_evt_%04d", i)
		subj, err := LegacyEventSubject(evt)
		require.NoError(t, err)
		conn := ps.pickSubConn(string(subj))
		events = append(events, entry{event: evt, conn: conn})
		expectedPerConn[conn]++
	}
	require.GreaterOrEqual(t, len(expectedPerConn), 2,
		"could not find events spanning at least 2 subConns; FNV distribution unexpectedly degenerate")

	// Snapshot inbound message counters before subscribe/publish.
	before := make(map[*natsgo.Conn]uint64, len(ps.subConns))
	for _, nc := range ps.subConns {
		before[nc] = nc.Stats().InMsgs
	}

	// Subscribe to every event, then publish exactly one message per
	// event so each subject's underlying conn must see exactly one
	// inbound delivery.
	delivered := make(map[string]chan struct{}, len(events))
	cancels := make([]func(), 0, len(events))
	for _, e := range events {
		ch := make(chan struct{}, 1)
		delivered[e.event] = ch
		c, err := ps.Subscribe(e.event, func(_ context.Context, _ []byte) {
			select {
			case ch <- struct{}{}:
			default:
			}
		})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})

	for _, e := range events {
		require.NoError(t, ps.Publish(e.event, []byte("x")))
	}
	require.NoError(t, ps.Flush())

	// Wait for all deliveries so InMsgs is fully accounted for.
	ctx := testutil.Context(t, testutil.WaitLong)
	for _, e := range events {
		select {
		case <-delivered[e.event]:
		case <-ctx.Done():
			t.Fatalf("delivery for event %q timed out", e.event)
		}
	}

	for _, nc := range ps.subConns {
		got := nc.Stats().InMsgs - before[nc]
		// expectedPerConn[nc] is bounded by the event search loop
		// above (<=4096); the int -> uint64 conversion is safe.
		want := uint64(expectedPerConn[nc]) //nolint:gosec // bounded by test event count
		require.Equal(t, want, got,
			"subConn %p InMsgs delta mismatch: want %d, got %d", nc, want, got)
	}
}

// TestSubscribePool_SameSubjectCoalescesOnOneConn asserts that
// multiple local subscribers for the same subject share exactly one
// underlying *natsgo.Subscription on the single hashed subscriber
// conn, and no other subscriber conn observes any inbound traffic for
// that subject.
func TestSubscribePool_SameSubjectCoalescesOnOneConn(t *testing.T) {
	t.Parallel()
	const n = 8
	ps := newSubPoolPubsub(t, n)

	const event = "coalesce_sub_evt"
	subj, err := LegacyEventSubject(event)
	require.NoError(t, err)
	expected := ps.pickSubConn(string(subj))

	// Snapshot inbound counters for all conns.
	beforeChosen := expected.Stats().InMsgs
	beforeOthers := make(map[*natsgo.Conn]uint64, len(ps.subConns)-1)
	for _, nc := range ps.subConns {
		if nc == expected {
			continue
		}
		beforeOthers[nc] = nc.Stats().InMsgs
	}

	// Attach many local subscribers on the same event; they must
	// coalesce onto one shared *natsgo.Subscription.
	const numLocal = 16
	var got [numLocal]atomic.Int64
	cancels := make([]func(), 0, numLocal)
	for i := 0; i < numLocal; i++ {
		i := i
		c, err := ps.Subscribe(event, func(_ context.Context, _ []byte) {
			got[i].Add(1)
		})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})

	// Exactly one shared underlying subscription on this subject.
	require.Equal(t, 1, listenerCountTotalShared(ps),
		"same-subject coalescing must yield exactly 1 shared subscription")
	require.Equal(t, numLocal, listenerCountForSubject(ps, string(subj)),
		"all local subscribers must attach to the same shared subscription")

	const numPublishes = 32
	for i := 0; i < numPublishes; i++ {
		require.NoError(t, ps.Publish(event, []byte("p")))
	}
	require.NoError(t, ps.Flush())

	ctx := testutil.Context(t, testutil.WaitLong)
	require.Eventually(t, func() bool {
		for i := 0; i < numLocal; i++ {
			if got[i].Load() < int64(numPublishes) {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast, "all listeners must receive all publishes")
	_ = ctx

	// The hashed conn must see exactly numPublishes inbound messages
	// (one per publish, not numPublishes*numLocal: coalescing means
	// the server only delivers once per shared subscription).
	require.Equal(t, uint64(numPublishes),
		expected.Stats().InMsgs-beforeChosen,
		"hashed subConn must receive exactly one inbound per publish for the coalesced subscription")
	for nc, before := range beforeOthers {
		require.Equal(t, uint64(0), nc.Stats().InMsgs-before,
			"non-selected subConn %p must not see any same-subject deliveries", nc)
	}
}

// listenerCountTotalShared returns the total number of shared
// subscriptions currently tracked by p. Helper for assertions in
// subscribepool_test.go.
func listenerCountTotalShared(p *Pubsub) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sharedBySubject)
}

// TestSubscribePool_SharedSubsDistributedAcrossConns drives a wave of
// Subscribes for many distinct events and asserts that the resulting
// shared *natsgo.Subscriptions are spread across at least two
// subscriber conns. This is the headline regression guard for the
// subscriber pool: without subject hashing, every shared sub would
// land on the same conn and the pool would be useless.
func TestSubscribePool_SharedSubsDistributedAcrossConns(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)

	const total = 64
	cancels := make([]func(), 0, total)
	expected := make(map[*natsgo.Conn]int, n)
	for i := 0; i < total; i++ {
		evt := fmt.Sprintf("distrib_evt_%04d", i)
		subj, err := LegacyEventSubject(evt)
		require.NoError(t, err)
		expected[ps.pickSubConn(string(subj))]++
		c, err := ps.Subscribe(evt, func(context.Context, []byte) {})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})

	require.GreaterOrEqual(t, len(expected), 2,
		"expected shared subs to land on at least 2 subConns (FNV distribution degenerate?)")

	// Cross-check via NumSubscriptions, the *natsgo.Conn-level
	// counter of registered subscriptions. Each shared sub maps to
	// exactly one *natsgo.Subscription on its hashed conn.
	for _, nc := range ps.subConns {
		want := expected[nc]
		require.Equal(t, want, nc.NumSubscriptions(),
			"subConn %p must own exactly the subscriptions assigned to it by pickSubConn", nc)
	}
}

// TestSubscribePool_ReadinessHoldsOnNonzeroConn picks an event whose
// shared subscription lives on a nonzero subscriber conn, then
// verifies that a publish issued immediately after the SubscribeWithErr
// returns is observed by the listener. This guards the readiness
// barrier (Flush + SetPendingLimits) on a non-default conn.
func TestSubscribePool_ReadinessHoldsOnNonzeroConn(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)

	// Find a legacy event whose subject hashes to subConns[i] with
	// i > 0 so we exercise the non-default conn path.
	var event string
	for i := 0; i < 4096; i++ {
		evt := fmt.Sprintf("readiness_nonzero_%04d", i)
		subj, err := LegacyEventSubject(evt)
		require.NoError(t, err)
		conn := ps.pickSubConn(string(subj))
		if conn != ps.subConns[0] {
			event = evt
			break
		}
	}
	require.NotEmpty(t, event, "could not find event hashing to a nonzero subConn")

	got := make(chan []byte, 1)
	cancel, err := ps.SubscribeWithErr(event, func(_ context.Context, msg []byte, _ error) {
		select {
		case got <- msg:
		default:
		}
	})
	require.NoError(t, err)
	defer cancel()

	// Publish-after-subscribe must not be lost: the readiness barrier
	// in attachListener guarantees Flush + SetPendingLimits have
	// completed on the owning subConn before SubscribeWithErr returns.
	require.NoError(t, ps.Publish(event, []byte("hello")))
	require.NoError(t, ps.Flush())

	ctx := testutil.Context(t, testutil.WaitLong)
	select {
	case msg := <-got:
		require.Equal(t, "hello", string(msg))
	case <-ctx.Done():
		t.Fatal("publish-after-subscribe lost on nonzero subConn")
	}
}

// TestSubscribePool_SlowConsumerOnNonzeroConn verifies that async
// slow-consumer errors for a shared subscription that lives on a
// nonzero subscriber conn still surface pubsub.ErrDroppedMessages to
// the local listener. This guards the sharedByNATS-based routing
// (which is keyed on *natsgo.Subscription, not on the owning conn).
func TestSubscribePool_SlowConsumerOnNonzeroConn(t *testing.T) {
	t.Parallel()
	const n = 4
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	// Tight pending limits so the parked listener overflows quickly.
	ps, err := New(ctx, logger, Options{
		SubscribeConns: n,
		PendingLimits:  PendingLimits{Msgs: 8, Bytes: 1024 * 1024},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	// Pick an event whose shared sub lands on a non-default subConn.
	var event string
	for i := 0; i < 4096; i++ {
		evt := fmt.Sprintf("slow_nonzero_%04d", i)
		subj, err := LegacyEventSubject(evt)
		require.NoError(t, err)
		if ps.pickSubConn(string(subj)) != ps.subConns[0] {
			event = evt
			break
		}
	}
	require.NotEmpty(t, event, "could not find event hashing to a nonzero subConn")

	type delivery struct {
		msg []byte
		err error
	}
	deliveries := make(chan delivery, 64)
	release := make(chan struct{})
	var blocked atomic.Bool

	subCancel, err := ps.SubscribeWithErr(event, func(_ context.Context, msg []byte, ferr error) {
		if !blocked.Swap(true) {
			<-release
		}
		deliveries <- delivery{msg: msg, err: ferr}
	})
	require.NoError(t, err)
	defer subCancel()

	for i := 0; i < 50; i++ {
		require.NoError(t, ps.Publish(event, []byte("burst")))
	}
	require.NoError(t, ps.Flush())
	close(release)

	deadline := time.After(testutil.WaitLong)
	sawDrop := false
collect:
	for {
		select {
		case d := <-deliveries:
			if d.err != nil && errors.Is(d.err, pubsub.ErrDroppedMessages) {
				sawDrop = true
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	require.True(t, sawDrop, "expected at least one ErrDroppedMessages callback for a shared sub on a nonzero subConn")
}

// TestSubscribePool_CloseClosesAllOwnedSubConns asserts that Close
// drains every owned subscriber connection (every subConns entry
// transitions to IsClosed) and that double-Close is a no-op.
func TestSubscribePool_CloseClosesAllOwnedSubConns(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	const n = 3
	ps, err := New(ctx, logger, Options{SubscribeConns: n})
	require.NoError(t, err)
	require.Len(t, ps.subConns, n)

	// Capture conn refs before Close clears any state.
	subConns := append([]*natsgo.Conn(nil), ps.subConns...)
	pubConn := ps.pubConns[0]

	require.NoError(t, ps.Close())
	for i, nc := range subConns {
		require.True(t, nc.IsClosed(), "subConns[%d] must be closed after Close", i)
	}
	require.True(t, pubConn.IsClosed(), "pubConns[0] must be closed after Close")

	// Idempotent: second Close must succeed without re-draining or
	// panicking on already-closed conns.
	require.NoError(t, ps.Close())
}

// TestSubscribePool_PublishHotPathLockFree asserts that the Publish
// hot path remains lock-free with respect to p.mu under a configured
// subscriber pool. We hold p.mu from a background goroutine and
// confirm Publish does not block on it. If Publish ever started
// taking p.mu the call below would deadlock; the test bounds the
// wait so a regression turns into a clear failure rather than a
// hung suite.
func TestSubscribePool_PublishHotPathLockFree(t *testing.T) {
	t.Parallel()
	ps := newSubPoolPubsub(t, 4)

	// Park p.mu in a goroutine. Releasing it on test cleanup means
	// even if the assertion below fails, the goroutine eventually
	// unblocks rather than leaking.
	release := make(chan struct{})
	held := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ps.mu.Lock()
		close(held)
		<-release
		ps.mu.Unlock()
	}()
	t.Cleanup(func() {
		close(release)
		wg.Wait()
	})
	<-held

	done := make(chan error, 1)
	go func() {
		done <- ps.Publish("hot_path_evt", []byte("x"))
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(testutil.WaitShort):
		t.Fatal("Publish blocked on p.mu while another goroutine held it; hot path is no longer lock-free")
	}
}
