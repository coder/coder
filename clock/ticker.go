package clock

import "time"

type Ticker struct {
	C <-chan time.Time
	//nolint: revive
	c       chan time.Time
	ticker  *time.Ticker  // realtime impl, if set
	d       time.Duration // period, if set
	nxt     time.Time     // next tick time
	mock    *Mock         // mock clock, if set
	stopped bool          // true if the ticker is not running
}

func (t *Ticker) fire(tt time.Time) {
	t.mock.mu.Lock()
	defer t.mock.mu.Unlock()
	if t.stopped {
		return
	}
	for !t.nxt.After(t.mock.cur) {
		t.nxt = t.nxt.Add(t.d)
	}
	t.mock.recomputeNextLocked()
	select {
	case t.c <- tt:
	default:
	}
}

func (t *Ticker) next() time.Time {
	return t.nxt
}

func (t *Ticker) Stop(tags ...string) {
	if t.ticker != nil {
		t.ticker.Stop()
		return
	}
	t.mock.mu.Lock()
	defer t.mock.mu.Unlock()
	c := newCall(clockFunctionTickerStop, tags)
	t.mock.matchCallLocked(c)
	defer close(c.complete)
	t.mock.removeEventLocked(t)
	t.stopped = true
}

func (t *Ticker) Reset(d time.Duration, tags ...string) {
	if t.ticker != nil {
		t.ticker.Reset(d)
		return
	}
	t.mock.mu.Lock()
	defer t.mock.mu.Unlock()
	c := newCall(clockFunctionTickerReset, tags, withDuration(d))
	t.mock.matchCallLocked(c)
	defer close(c.complete)
	t.nxt = t.mock.cur.Add(d)
	t.d = d
	if t.stopped {
		t.stopped = false
		t.mock.addEventLocked(t)
	} else {
		t.mock.recomputeNextLocked()
	}
}
