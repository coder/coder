package admission_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw/admission"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// heldRequest is a request that has been admitted and is being served by
// holdHandler.
type heldRequest struct {
	ctx context.Context
	// finish makes the handler return when closed.
	finish chan struct{}
}

// holdHandler reports every request it serves on held and then blocks until
// the test finishes that request.
func holdHandler(held chan<- heldRequest) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		req := heldRequest{ctx: r.Context(), finish: make(chan struct{})}
		held <- req
		<-req.finish
		rw.WriteHeader(http.StatusNoContent)
	})
}

// serve sends a request to h in the background. The recorder may be read once
// the returned channel is closed.
func serve(ctx context.Context, h http.Handler) (*httptest.ResponseRecorder, <-chan struct{}) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rec, req)
	}()
	return rec, done
}

func newMetrics() *admission.Metrics {
	return &admission.Metrics{
		InFlight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "in_flight"}),
		Queued:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "queued"}),
		Rejected: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "rejected_total"}, []string{"reason"}),
	}
}

func rejected(m *admission.Metrics, reason admission.RejectReason) float64 {
	return promtest.ToFloat64(m.Rejected.WithLabelValues(string(reason)))
}

func TestLimiter(t *testing.T) {
	t.Parallel()

	t.Run("QueuedRequestTakesFreedSlot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		queueTrap := mClock.Trap().NewTimer("admission", "queue")
		defer queueTrap.Close()
		metrics := newMetrics()
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight:  1,
			MaxQueued:    1,
			QueueTimeout: time.Minute,
			Clock:        mClock,
			Metrics:      metrics,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		// The second request starts its queue timer only once it is counted
		// as queued, so the gauges are settled when the trap fires.
		secondRec, secondDone := serve(ctx, handler)
		queueTrap.MustWait(ctx).MustRelease(ctx)
		require.Equal(t, 1.0, promtest.ToFloat64(metrics.InFlight))
		require.Equal(t, 1.0, promtest.ToFloat64(metrics.Queued))

		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
		second := testutil.RequireReceive(ctx, t, held)
		require.Equal(t, 1.0, promtest.ToFloat64(metrics.InFlight))
		require.Equal(t, 0.0, promtest.ToFloat64(metrics.Queued))

		close(second.finish)
		testutil.TryReceive(ctx, t, secondDone)
		require.Equal(t, http.StatusNoContent, secondRec.Code)
		require.Equal(t, 0.0, promtest.ToFloat64(metrics.InFlight))
	})

	t.Run("RejectsWhenQueueIsFull", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		metrics := newMetrics()
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight:   1,
			RetryAfterMin: 5 * time.Second,
			RetryAfterMax: 15 * time.Second,
			Metrics:       metrics,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		rec, done := serve(ctx, handler)
		testutil.TryReceive(ctx, t, done)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		retryAfter, err := strconv.Atoi(rec.Header().Get("Retry-After"))
		require.NoError(t, err)
		require.GreaterOrEqual(t, retryAfter, 5)
		require.LessOrEqual(t, retryAfter, 15)
		require.Equal(t, 1.0, rejected(metrics, admission.RejectReasonQueueFull))

		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
	})

	t.Run("RejectsAfterQueueTimeout", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		queueTrap := mClock.Trap().NewTimer("admission", "queue")
		defer queueTrap.Close()
		metrics := newMetrics()
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight:  1,
			MaxQueued:    1,
			QueueTimeout: 10 * time.Second,
			Clock:        mClock,
			Metrics:      metrics,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		secondRec, secondDone := serve(ctx, handler)
		queueTrap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(10 * time.Second).MustWait(ctx)
		testutil.TryReceive(ctx, t, secondDone)
		require.Equal(t, http.StatusServiceUnavailable, secondRec.Code)
		require.NotEmpty(t, secondRec.Header().Get("Retry-After"))
		require.Equal(t, 1.0, rejected(metrics, admission.RejectReasonQueueTimeout))

		// The request that timed out no longer occupies the queue, so the
		// next one waits instead of being rejected.
		thirdRec, thirdDone := serve(ctx, handler)
		queueTrap.MustWait(ctx).MustRelease(ctx)
		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
		third := testutil.RequireReceive(ctx, t, held)
		close(third.finish)
		testutil.TryReceive(ctx, t, thirdDone)
		require.Equal(t, http.StatusNoContent, thirdRec.Code)
	})

	t.Run("QueueTimeoutIsSpread", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		queueTrap := mClock.Trap().NewTimer("admission", "queue")
		defer queueTrap.Close()
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight:        1,
			MaxQueued:          8,
			QueueTimeout:       5 * time.Second,
			QueueTimeoutJitter: 10 * time.Second,
			Clock:              mClock,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		// Requests that arrive together must not all be rejected together, or
		// clients that retry a fixed time later would stay in step forever.
		timeouts := make(map[time.Duration]struct{})
		queuedDone := make([]<-chan struct{}, 0, 8)
		for range 8 {
			_, done := serve(ctx, handler)
			queuedDone = append(queuedDone, done)
			call := queueTrap.MustWait(ctx)
			require.GreaterOrEqual(t, call.Duration, 5*time.Second)
			require.LessOrEqual(t, call.Duration, 15*time.Second)
			timeouts[call.Duration] = struct{}{}
			call.MustRelease(ctx)
		}
		require.Greater(t, len(timeouts), 1)

		// Every queued request is answered by the end of the longest wait.
		for {
			_, ok := mClock.Peek()
			if !ok {
				break
			}
			_, w := mClock.AdvanceNext()
			w.MustWait(ctx)
		}
		for _, done := range queuedDone {
			testutil.TryReceive(ctx, t, done)
		}
		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
	})

	t.Run("CanceledWhileQueued", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		queueTrap := mClock.Trap().NewTimer("admission", "queue")
		defer queueTrap.Close()
		metrics := newMetrics()
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight:  1,
			MaxQueued:    1,
			QueueTimeout: time.Minute,
			Clock:        mClock,
			Metrics:      metrics,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		clientCtx, cancel := context.WithCancel(ctx)
		secondRec, secondDone := serve(clientCtx, handler)
		queueTrap.MustWait(ctx).MustRelease(ctx)
		cancel()
		testutil.TryReceive(ctx, t, secondDone)
		require.Zero(t, secondRec.Body.Len(), "nothing is written to a client that left")
		require.Equal(t, 0.0, rejected(metrics, admission.RejectReasonQueueTimeout))
		require.Equal(t, 0.0, rejected(metrics, admission.RejectReasonQueueFull))

		// The canceled request gave up its place in the queue.
		_, thirdDone := serve(ctx, handler)
		queueTrap.MustWait(ctx).MustRelease(ctx)
		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
		third := testutil.RequireReceive(ctx, t, held)
		close(third.finish)
		testutil.TryReceive(ctx, t, thirdDone)
	})

	t.Run("ReleaseFreesExactlyOneSlot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight: 1,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		// The first request gives up its slot while its handler keeps
		// running, as a long-lived connection does after setup.
		admission.Release(first.ctx)
		admission.Release(first.ctx)
		_, secondDone := serve(ctx, handler)
		second := testutil.RequireReceive(ctx, t, held)

		// Neither the repeated release nor the first handler returning may
		// free the slot the second request now holds.
		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
		thirdRec, thirdDone := serve(ctx, handler)
		testutil.TryReceive(ctx, t, thirdDone)
		require.Equal(t, http.StatusServiceUnavailable, thirdRec.Code)

		close(second.finish)
		testutil.TryReceive(ctx, t, secondDone)
	})

	t.Run("MaxHoldFreesStalledSlot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		held := make(chan heldRequest)
		handler := admission.New(admission.Options{
			MaxInFlight: 1,
			MaxHold:     15 * time.Second,
			Clock:       mClock,
		}).Middleware(holdHandler(held))

		_, firstDone := serve(ctx, handler)
		first := testutil.RequireReceive(ctx, t, held)

		mClock.Advance(15 * time.Second).MustWait(ctx)
		_, secondDone := serve(ctx, handler)
		second := testutil.RequireReceive(ctx, t, held)

		// The first handler returning after its hold expired must not free
		// the slot the second request now holds.
		close(first.finish)
		testutil.TryReceive(ctx, t, firstDone)
		thirdRec, thirdDone := serve(ctx, handler)
		testutil.TryReceive(ctx, t, thirdDone)
		require.Equal(t, http.StatusServiceUnavailable, thirdRec.Code)

		close(second.finish)
		testutil.TryReceive(ctx, t, secondDone)
	})

	t.Run("NilLimiterAdmitsEverything", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		var limiter *admission.Limiter
		rec := httptest.NewRecorder()
		limiter.Middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Releasing without a slot must be harmless, because handlers
			// call it whether or not a limiter is configured.
			admission.Release(r.Context())
			rw.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))
		require.Equal(t, http.StatusNoContent, rec.Code)
	})
}
