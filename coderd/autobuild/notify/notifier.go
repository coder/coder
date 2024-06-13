package notify

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/coder/coder/v2/clock"
)

// Notifier triggers callbacks at given intervals until some event happens.  The
// intervals (e.g. 10 minute warning, 5 minute warning) are given in the
// countdown.  The Notifier periodically polls the condition to get the time of
// the event (the Condition's deadline) and the callback.  The callback is
// called at most once per entry in the countdown, the first time the time to
// the deadline is shorter than the duration.
type Notifier struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pollDone chan struct{}

	lock       sync.Mutex
	condition  Condition
	notifiedAt map[time.Duration]bool
	countdown  []time.Duration

	// for testing
	clock clock.Clock
}

// Condition is a function that gets executed periodically, and receives the
// current time as an argument.
//   - It should return the deadline for the notification, as well as a
//     callback function to execute. If deadline is the zero
//     time, callback will not be executed.
//   - Callback is executed once for every time the difference between deadline
//     and the current time is less than an element of countdown.
//   - To enforce a minimum interval between consecutive callbacks, truncate
//     the returned deadline to the minimum interval.
type Condition func(now time.Time) (deadline time.Time, callback func())

type Option func(*Notifier)

// WithTestClock is used in tests to inject a mock Clock
func WithTestClock(clk clock.Clock) Option {
	return func(n *Notifier) {
		n.clock = clk
	}
}

// New returns a Notifier that calls cond once every time it polls.
//   - Duplicate values are removed from countdown, and it is sorted in
//     descending order.
func New(cond Condition, interval time.Duration, countdown []time.Duration, opts ...Option) *Notifier {
	// Ensure countdown is sorted in descending order and contains no duplicates.
	ct := unique(countdown)
	sort.Slice(ct, func(i, j int) bool {
		return ct[i] < ct[j]
	})

	ctx, cancel := context.WithCancel(context.Background())
	n := &Notifier{
		ctx:        ctx,
		cancel:     cancel,
		pollDone:   make(chan struct{}),
		countdown:  ct,
		condition:  cond,
		notifiedAt: make(map[time.Duration]bool),
		clock:      clock.NewReal(),
	}
	for _, opt := range opts {
		opt(n)
	}
	go n.poll(interval)

	return n
}

// poll polls once immediately, and then periodically according to the interval.
// Poll exits when ticker is closed.
func (n *Notifier) poll(interval time.Duration) {
	defer close(n.pollDone)

	// poll once immediately
	_ = n.pollOnce()
	tkr := n.clock.TickerFunc(n.ctx, interval, n.pollOnce, "notifier", "poll")
	_ = tkr.Wait()
}

func (n *Notifier) Close() {
	n.cancel()
	<-n.pollDone
}

// pollOnce only returns an error so it matches the signature expected of TickerFunc
// nolint: revive // bare returns are fine here
func (n *Notifier) pollOnce() (_ error) {
	tick := n.clock.Now()
	n.lock.Lock()
	defer n.lock.Unlock()

	deadline, callback := n.condition(tick)
	if deadline.IsZero() {
		return
	}

	timeRemaining := deadline.Sub(tick)
	for _, tock := range n.countdown {
		if n.notifiedAt[tock] {
			continue
		}
		if timeRemaining > tock {
			continue
		}
		callback()
		n.notifiedAt[tock] = true
		return
	}
	return
}

func unique(ds []time.Duration) []time.Duration {
	m := make(map[time.Duration]bool)
	for _, d := range ds {
		m[d] = true
	}
	var ks []time.Duration
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
