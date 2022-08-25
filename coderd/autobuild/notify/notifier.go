package notify

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Notifier calls a Condition at most once for each count in countdown.
type Notifier struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pollDone chan struct{}

	lock       sync.Mutex
	condition  Condition
	notifiedAt map[time.Duration]bool
	countdown  []time.Duration
}

// Condition is a function that gets executed with a certain time.
//   - It should return the deadline for the notification, as well as a
//     callback function to execute once the time to the deadline is
//     less than one of the notify attempts. If deadline is the zero
//     time, callback will not be executed.
//   - Callback is executed once for every time the difference between deadline
//     and the current time is less than an element of countdown.
//   - To enforce a minimum interval between consecutive callbacks, truncate
//     the returned deadline to the minimum interval.
type Condition func(now time.Time) (deadline time.Time, callback func())

// Notify is a convenience function that initializes a new Notifier
// with the given condition, interval, and countdown.
// It is the responsibility of the caller to call close to stop polling.
func Notify(cond Condition, interval time.Duration, countdown ...time.Duration) (closeFunc func()) {
	notifier := New(cond, countdown...)
	ticker := time.NewTicker(interval)
	go notifier.Poll(ticker.C)
	return func() {
		ticker.Stop()
		_ = notifier.Close()
	}
}

// New returns a Notifier that calls cond once every time it polls.
//   - Duplicate values are removed from countdown, and it is sorted in
//     descending order.
func New(cond Condition, countdown ...time.Duration) *Notifier {
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
	}

	return n
}

// Poll polls once immediately, and then once for every value from ticker.
// Poll exits when ticker is closed.
func (n *Notifier) Poll(ticker <-chan time.Time) {
	defer close(n.pollDone)

	// poll once immediately
	n.pollOnce(time.Now())
	for {
		select {
		case <-n.ctx.Done():
			return
		case t, ok := <-ticker:
			if !ok {
				return
			}
			n.pollOnce(t)
		}
	}
}

func (n *Notifier) Close() error {
	n.cancel()
	<-n.pollDone
	return nil
}

func (n *Notifier) pollOnce(tick time.Time) {
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
