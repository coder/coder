//nolint:testpackage // Uses internal fields and helpers for pool assertions.
package nats

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// newPoolPubsub is a small wrapper that builds a Pubsub with the given
// PublishConns count and ensures it is closed on test cleanup.
func newPoolPubsub(t *testing.T, publishConns int) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{PublishConns: publishConns})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

// TestPublishPool_DefaultIsOne asserts that the historical zero-value
// Options preserves single-publisher-connection behavior: exactly one
// owned pubConn, distinct from the single owned subConn, and exactly
// two server-side client connections.
func TestPublishPool_DefaultIsOne(t *testing.T) {
	t.Parallel()
	ps := newPoolPubsub(t, 0)
	require.Len(t, ps.pubConns, 1, "PublishConns=0 must default to a single publish connection")
	require.Len(t, ps.subConns, 1, "SubscribeConns=0 must default to a single subscribe connection")
	require.True(t, ps.ownsPubConns, "New must own its publish connections")
	require.True(t, ps.ownsSubConns, "New must own its subscribe connections")
	require.NotSame(t, ps.pubConns[0], ps.subConns[0], "pubConns[0] and subConns[0] must be distinct connections")
	require.Equal(t, 2, ps.ns.NumClients(),
		"default Options must produce exactly 2 server-side client connections (1 pub + 1 sub)")
}

// TestPublishPool_NegativeDefaults asserts that a negative PublishConns
// value is normalized to 1 rather than producing zero connections or
// erroring.
func TestPublishPool_NegativeDefaults(t *testing.T) {
	t.Parallel()
	ps := newPoolPubsub(t, -5)
	require.Len(t, ps.pubConns, 1, "negative PublishConns must default to a single publish connection")
}

// TestPublishPool_CreatesN asserts that Options.PublishConns=N creates
// exactly N owned publisher connections plus one subscriber connection,
// and that all entries are non-nil and connected.
func TestPublishPool_CreatesN(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newPoolPubsub(t, n)
	require.Len(t, ps.pubConns, n)
	require.True(t, ps.ownsPubConns)
	require.Len(t, ps.subConns, 1, "SubscribeConns default must still be 1")
	seen := make(map[*natsgo.Conn]struct{}, n)
	for i, nc := range ps.pubConns {
		require.NotNil(t, nc, "pubConns[%d] must be non-nil", i)
		require.True(t, nc.IsConnected(), "pubConns[%d] must be connected", i)
		require.NotSame(t, nc, ps.subConns[0], "pubConns[%d] must be distinct from subConns[0]", i)
		_, dup := seen[nc]
		require.False(t, dup, "pubConns[%d] must be a distinct *natsgo.Conn", i)
		seen[nc] = struct{}{}
	}
	// The server must report exactly N pub conns + 1 sub conn.
	require.Equal(t, n+1, ps.ns.NumClients(),
		"server must observe exactly %d client connections (%d pub + 1 sub)", n+1, n)
}

// TestPublishPool_PickPubConn_StablePerSubject asserts that pickPubConn
// is deterministic: repeated calls for the same subject always return
// the same connection, and that the underlying selection is a
// well-defined function of the subject string alone (no per-process
// randomization).
func TestPublishPool_PickPubConn_StablePerSubject(t *testing.T) {
	t.Parallel()
	ps := newPoolPubsub(t, 8)
	subjects := []string{
		"coder.v1.event.alpha",
		"coder.v1.event.beta",
		"coder.v1.event.gamma",
		"coder.v1.event.delta",
		"coder.v1.event.epsilon",
		"coder.v1.event.zeta",
	}
	for _, s := range subjects {
		first := ps.pickPubConn(s)
		for i := 0; i < 32; i++ {
			require.Same(t, first, ps.pickPubConn(s),
				"pickPubConn(%q) must be stable across calls", s)
		}
	}
}

// TestPublishPool_PickPubConn_DistributesSubjects asserts that
// pickPubConn spreads a moderate variety of distinct subjects across
// multiple entries of the pool. We do not require uniform distribution
// (FNV-1a does not guarantee that), but we do require that not every
// subject hashes to the same connection, which would defeat the whole
// optimization.
func TestPublishPool_PickPubConn_DistributesSubjects(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newPoolPubsub(t, n)
	counts := make(map[*natsgo.Conn]int, n)
	for i := 0; i < 64; i++ {
		// Use a subject namespace close to what LegacyEventSubject
		// produces so the test reflects realistic hashing input.
		subj := fmt.Sprintf("coder.v1.legacy.event_%03d", i)
		counts[ps.pickPubConn(subj)]++
	}
	require.GreaterOrEqual(t, len(counts), 2,
		"pickPubConn must distribute 64 distinct subjects across at least 2 of %d conns, got %d", n, len(counts))
}

// TestPublishPool_SingleConnPicksOnlyEntry asserts that with a single
// publish connection pickPubConn always returns the one entry, even
// for many distinct subjects.
func TestPublishPool_SingleConnPicksOnlyEntry(t *testing.T) {
	t.Parallel()
	ps := newPoolPubsub(t, 1)
	only := ps.pubConns[0]
	for i := 0; i < 32; i++ {
		subj := fmt.Sprintf("coder.v1.legacy.solo_%03d", i)
		require.Same(t, only, ps.pickPubConn(subj))
	}
}

// TestPublishPool_PublishUsesHashedConn drives real publishes and
// verifies that each pubConn's Stats().OutMsgs grew only for subjects
// that pickPubConn assigned to it. This confirms Publish actually
// routes via pickPubConn (not e.g. always pubConns[0]) and that
// same-subject Publishes preserve per-subject ordering at the
// connection level by going to a single conn.
func TestPublishPool_PublishUsesHashedConn(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newPoolPubsub(t, n)

	// Find a set of legacy event names that covers >=2 distinct
	// publisher conns. We bound the search so an unlucky hash
	// distribution does not loop forever; FNV-1a over short prefixed
	// strings is well-spread in practice.
	expectedPerConn := make(map[*natsgo.Conn]int, n)
	type entry struct {
		event string
		conn  *natsgo.Conn
	}
	var events []entry
	for i := 0; len(expectedPerConn) < 2 && i < 4096; i++ {
		evt := fmt.Sprintf("evt_%04d", i)
		subj, err := LegacyEventSubject(evt)
		require.NoError(t, err)
		conn := ps.pickPubConn(string(subj))
		events = append(events, entry{event: evt, conn: conn})
		expectedPerConn[conn]++
	}
	require.GreaterOrEqual(t, len(expectedPerConn), 2,
		"could not find events spanning at least 2 pubConns; FNV distribution unexpectedly degenerate")

	// Snapshot outbound message counters before publish.
	before := make(map[*natsgo.Conn]uint64, len(ps.pubConns))
	for _, nc := range ps.pubConns {
		before[nc] = nc.Stats().OutMsgs
	}

	for _, e := range events {
		require.NoError(t, ps.Publish(e.event, []byte("x")))
	}
	require.NoError(t, ps.Flush())

	// Each pubConn must have observed exactly expectedPerConn[conn]
	// additional outbound publishes; conns that pickPubConn never
	// selected must have unchanged counters.
	for _, nc := range ps.pubConns {
		got := nc.Stats().OutMsgs - before[nc]
		// expectedPerConn[nc] is bounded by the event search loop above
		// (<=4096); the int -> uint64 conversion is therefore safe.
		want := uint64(expectedPerConn[nc]) //nolint:gosec // bounded by test event count
		require.Equal(t, want, got,
			"pubConn %p OutMsgs delta mismatch: want %d, got %d", nc, want, got)
	}
}

// TestPublishPool_SameSubjectSameConn_Concurrent asserts that
// concurrent publishes for the same subject all funnel through a
// single publisher connection, which is the property the wrapper
// relies on to preserve per-subject ordering across the pool.
func TestPublishPool_SameSubjectSameConn_Concurrent(t *testing.T) {
	t.Parallel()
	const n = 8
	ps := newPoolPubsub(t, n)

	const event = "ordering_evt"
	subj, err := LegacyEventSubject(event)
	require.NoError(t, err)
	expected := ps.pickPubConn(string(subj))

	// Snapshot the chosen conn before; every other conn's counter
	// must stay flat after a burst of same-subject publishes.
	beforeChosen := expected.Stats().OutMsgs
	beforeOthers := make(map[*natsgo.Conn]uint64, len(ps.pubConns)-1)
	for _, nc := range ps.pubConns {
		if nc == expected {
			continue
		}
		beforeOthers[nc] = nc.Stats().OutMsgs
	}

	const publishers = 8
	const perPublisher = 128
	var wg sync.WaitGroup
	wg.Add(publishers)
	for i := 0; i < publishers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perPublisher; j++ {
				assert.NoError(t, ps.Publish(event, []byte("p")))
			}
		}()
	}
	wg.Wait()
	require.NoError(t, ps.Flush())

	require.Equal(t, uint64(publishers*perPublisher),
		expected.Stats().OutMsgs-beforeChosen,
		"all same-subject publishes must land on the hashed publisher conn")
	for nc, before := range beforeOthers {
		require.Equal(t, uint64(0), nc.Stats().OutMsgs-before,
			"non-selected pubConn %p must not see any of the same-subject publishes", nc)
	}
}

// TestPublishPool_FlushFlushesAllConns installs a deterministic test
// hook on Flush and asserts every publisher index in the pool is
// visited exactly once per Flush call.
func TestPublishPool_FlushFlushesAllConns(t *testing.T) {
	t.Parallel()
	const n = 5
	ps := newPoolPubsub(t, n)

	var mu sync.Mutex
	var seen []int
	ps.testHookOnFlushConn = func(idx int) {
		mu.Lock()
		seen = append(seen, idx)
		mu.Unlock()
	}

	require.NoError(t, ps.Flush())

	mu.Lock()
	got := append([]int(nil), seen...)
	mu.Unlock()
	require.Len(t, got, n, "Flush must invoke the per-conn hook exactly once per pool entry")
	// Order is the slice order, which matches construction order;
	// assert that here to keep the contract pinned.
	for i := 0; i < n; i++ {
		require.Equal(t, i, got[i], "Flush must visit pubConns in slice order")
	}
}

// TestPublishPool_FlushAlsoExercisesRealDelivery is a sanity smoke
// test: with a multi-conn pool, a Subscribe + several Publishes across
// distinct subjects must still all be observed by the subscriber
// after Flush returns. This guards against accidental regressions
// where Publish writes succeed but the wrapper forgets to flush some
// conns.
func TestPublishPool_FlushAlsoExercisesRealDelivery(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newPoolPubsub(t, n)

	var got atomic.Int64
	const totalEvents = 32
	done := make(chan struct{})
	cancels := make([]func(), 0, totalEvents)
	for i := 0; i < totalEvents; i++ {
		event := fmt.Sprintf("flush_real_%02d", i)
		c, err := ps.Subscribe(event, func(_ context.Context, _ []byte) {
			if got.Add(1) == int64(totalEvents) {
				close(done)
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

	for i := 0; i < totalEvents; i++ {
		event := fmt.Sprintf("flush_real_%02d", i)
		require.NoError(t, ps.Publish(event, []byte("payload")))
	}
	require.NoError(t, ps.Flush())

	select {
	case <-done:
	case <-time.After(testutil.WaitLong):
		t.Fatalf("only observed %d / %d deliveries after Flush returned", got.Load(), totalEvents)
	}
}

// TestPublishPool_CloseClosesAllOwnedConns asserts that Close drains
// every owned publisher connection (every pubConns entry transitions
// to IsClosed) and that double-Close is a no-op.
func TestPublishPool_CloseClosesAllOwnedConns(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	const n = 3
	ps, err := New(ctx, logger, Options{PublishConns: n})
	require.NoError(t, err)
	require.Len(t, ps.pubConns, n)

	// Capture conn refs before Close clears any state.
	conns := append([]*natsgo.Conn(nil), ps.pubConns...)
	subConns := append([]*natsgo.Conn(nil), ps.subConns...)

	require.NoError(t, ps.Close())
	for i, nc := range conns {
		require.True(t, nc.IsClosed(), "pubConns[%d] must be closed after Close", i)
	}
	for i, nc := range subConns {
		require.True(t, nc.IsClosed(), "subConns[%d] must be closed after Close", i)
	}

	// Idempotent: second Close must succeed without re-draining or
	// panicking on already-closed conns.
	require.NoError(t, ps.Close())
}

// TestPublishPool_NewFromConn_HasSingleAliasedPubConn asserts that the
// NewFromConn path produces a one-entry pubConns slice that aliases
// the externally supplied connection, that Close does not drain the
// external conn, and that Options.PublishConns is effectively ignored
// (NewFromConn does not take Options).
func TestPublishPool_NewFromConn_HasSingleAliasedPubConn(t *testing.T) {
	t.Parallel()
	// Build a host Pubsub solely to obtain an in-process *natsgo.Conn
	// we control independently of the pubsub under test.
	host := newPoolPubsub(t, 1)
	external := host.pubConns[0]

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	p, err := NewFromConn(logger, external)
	require.NoError(t, err)
	require.Len(t, p.pubConns, 1, "NewFromConn must expose exactly one publish conn")
	require.Len(t, p.subConns, 1, "NewFromConn must expose exactly one subscribe conn")
	require.Same(t, external, p.pubConns[0], "NewFromConn pubConns[0] must alias the supplied connection")
	require.Same(t, external, p.subConns[0], "NewFromConn subConns[0] must alias the supplied connection")
	require.False(t, p.ownsPubConns, "NewFromConn must not claim ownership of the external pub conn")
	require.False(t, p.ownsSubConns, "NewFromConn must not claim ownership of the external sub conn")

	require.NoError(t, p.Close())
	require.False(t, external.IsClosed(), "Close on a NewFromConn Pubsub must not close the external conn")
}
