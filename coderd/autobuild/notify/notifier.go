package notify

import (
	"sort"
	"sync"
	"time"
)

// Notifier calls a Condition at most once for each count in countdown.
type Notifier struct {
	sync.Mutex
	condition  Condition
	notifiedAt map[time.Duration]bool
	countdown  []time.Duration
}

// Condition is a function that gets executed with a certain time.
type Condition func(now time.Time) (deadline time.Time, callback func())

// New returns a Notifier that calls cond once every time it polls.
// - Condition is a function that returns the deadline and a callback.
//   It should return the deadline for the notification, as well as a
//   callback function to execute once the time to the deadline is
//   less than one of the notify attempts. If deadline is the zero
//   time, callback will not be executed.
// - Callback is executed once for every time the difference between deadline
//   and the current time is less than an element of countdown.
// - To enforce a minimum interval between consecutive callbacks, truncate
//   the returned deadline to the minimum interval.
// - Duplicate values are removed from countdown, and it is sorted in
//   descending order.
func New(cond Condition, countdown ...time.Duration) *Notifier {
	// Ensure countdown is sorted in descending order and contains no duplicates.
	sort.Slice(unique(countdown), func(i, j int) bool {
		return countdown[i] < countdown[j]
	})

	n := &Notifier{
		countdown:  countdown,
		condition:  cond,
		notifiedAt: make(map[time.Duration]bool),
	}

	return n
}

// Poll polls once immediately, and then once for every value from ticker.
func (n *Notifier) Poll(ticker <-chan time.Time) {
	// poll once immediately
	n.pollOnce(time.Now())
	for t := range ticker {
		n.pollOnce(t)
	}
}

func (n *Notifier) pollOnce(tick time.Time) {
	n.Lock()
	defer n.Unlock()

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
	ks := make([]time.Duration, 0)
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
