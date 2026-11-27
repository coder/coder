// Package admission limits how many HTTP requests may be in an expensive
// phase at the same time. Requests over the limit wait in a bounded queue, and
// requests the queue cannot hold are turned away with a Retry-After hint
// before they consume anything else.
package admission

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// RejectReason says why a request was not admitted.
type RejectReason string

const (
	// RejectReasonQueueFull means no slot was free and the queue had no room.
	RejectReasonQueueFull RejectReason = "queue_full"
	// RejectReasonQueueTimeout means the request waited for the whole queue
	// timeout without a slot becoming free.
	RejectReasonQueueTimeout RejectReason = "queue_timeout"
)

// Metrics are the collectors a Limiter keeps up to date. The caller creates
// and registers them, so each use of a Limiter can name its own.
type Metrics struct {
	// InFlight is the number of requests holding a slot.
	InFlight prometheus.Gauge
	// Queued is the number of requests waiting for a slot.
	Queued prometheus.Gauge
	// Rejected counts rejected requests. Its only label is the RejectReason.
	Rejected *prometheus.CounterVec
}

// Options configures a Limiter.
type Options struct {
	// MaxInFlight is the number of requests that may hold a slot at once.
	MaxInFlight int
	// MaxQueued is the number of requests that may wait for a slot. A request
	// that arrives while the queue is full is rejected immediately.
	MaxQueued int
	// QueueTimeout is how long a request waits for a slot before it is
	// rejected. Keep it, plus QueueTimeoutJitter, below the response timeout
	// of any proxy in front of the server.
	QueueTimeout time.Duration
	// QueueTimeoutJitter is the most that is added to QueueTimeout. Each
	// request waits a different, uniformly drawn share of it, so requests
	// that arrived together are not rejected together. That spreads out
	// clients that retry a fixed time after a rejection, without any change
	// to the clients.
	QueueTimeoutJitter time.Duration
	// MaxHold frees a slot that its request has held for this long. Zero
	// means no limit.
	MaxHold time.Duration
	// RetryAfterMin and RetryAfterMax bound the delay that a rejected client
	// is told to wait. The delay is drawn uniformly from that range in whole
	// seconds so that clients that honor it do not all return together.
	RetryAfterMin time.Duration
	RetryAfterMax time.Duration
	// Clock defaults to the real clock.
	Clock quartz.Clock
	// Metrics is optional.
	Metrics *Metrics
}

// Limiter admits a bounded number of requests at a time. A request holds its
// slot until its handler returns, until Release is called with its context, or
// until Options.MaxHold elapses, whichever comes first. Releasing early lets a
// long-lived handler, such as one serving a WebSocket, be limited only for the
// expensive part of its work.
type Limiter struct {
	opts Options
	// slots holds one element per request that holds a slot. Sending takes a
	// slot and receiving frees one, so blocked senders are the queue and the
	// runtime hands a freed slot to the sender that has waited longest.
	slots  chan struct{}
	queued atomic.Int64
}

// New returns a Limiter. Options.MaxInFlight must be positive.
func New(opts Options) *Limiter {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	return &Limiter{
		opts:  opts,
		slots: make(chan struct{}, opts.MaxInFlight),
	}
}

type releaseContextKey struct{}

// Release frees the slot held by the request that ctx belongs to. It does
// nothing if the request holds no slot or has already released it.
func Release(ctx context.Context) {
	if release, ok := ctx.Value(releaseContextKey{}).(func()); ok {
		release()
	}
}

// Middleware admits the request, waits for a slot, or rejects the request
// with 503 Service Unavailable. A nil Limiter admits every request.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		admitted, reason := l.acquire(ctx)
		if !admitted {
			// An empty reason means the client went away while it was queued,
			// so there is nobody left to answer.
			if reason != "" {
				l.reject(rw, r, reason)
			}
			return
		}
		release := l.hold()
		defer release()
		next.ServeHTTP(rw, r.WithContext(context.WithValue(ctx, releaseContextKey{}, release)))
	})
}

// acquire takes a slot, waiting in the queue if none is free. If it returns
// false with an empty reason, ctx ended while the request was queued.
func (l *Limiter) acquire(ctx context.Context) (bool, RejectReason) {
	select {
	case l.slots <- struct{}{}:
		return true, ""
	default:
	}

	if l.queued.Add(1) > int64(l.opts.MaxQueued) {
		l.queued.Add(-1)
		return false, RejectReasonQueueFull
	}
	defer l.queued.Add(-1)
	if m := l.opts.Metrics; m != nil {
		m.Queued.Inc()
		defer m.Queued.Dec()
	}

	timeout := l.opts.QueueTimeout
	if l.opts.QueueTimeoutJitter > 0 {
		// #nosec G404 - spreading out rejections does not need a cryptographic source.
		timeout += rand.N(l.opts.QueueTimeoutJitter + 1)
	}
	timer := l.opts.Clock.NewTimer(timeout, "admission", "queue")
	defer timer.Stop()
	select {
	case l.slots <- struct{}{}:
		return true, ""
	case <-timer.C:
		return false, RejectReasonQueueTimeout
	case <-ctx.Done():
		return false, ""
	}
}

// hold accounts for a slot that was just acquired and returns the function
// that frees it. The function may be called any number of times.
func (l *Limiter) hold() func() {
	if m := l.opts.Metrics; m != nil {
		m.InFlight.Inc()
	}
	release := sync.OnceFunc(func() {
		<-l.slots
		if m := l.opts.Metrics; m != nil {
			m.InFlight.Dec()
		}
	})
	if l.opts.MaxHold <= 0 {
		return release
	}
	timer := l.opts.Clock.AfterFunc(l.opts.MaxHold, release, "admission", "hold")
	return func() {
		timer.Stop()
		release()
	}
}

func (l *Limiter) reject(rw http.ResponseWriter, r *http.Request, reason RejectReason) {
	if m := l.opts.Metrics; m != nil {
		m.Rejected.WithLabelValues(string(reason)).Inc()
	}
	rw.Header().Set("Retry-After", strconv.Itoa(l.retryAfterSeconds()))
	httpapi.Write(r.Context(), rw, http.StatusServiceUnavailable, codersdk.Response{
		Message: "The server is busy with other connections. Try again after the delay in the Retry-After header.",
	})
}

func (l *Limiter) retryAfterSeconds() int {
	// Retry-After has a resolution of one second, and zero would invite an
	// immediate retry.
	lo := max(1, int(l.opts.RetryAfterMin/time.Second))
	hi := max(lo, int(l.opts.RetryAfterMax/time.Second))
	// #nosec G404 - spreading out retries does not need a cryptographic source.
	return lo + rand.IntN(hi-lo+1)
}
