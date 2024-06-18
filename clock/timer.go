package clock

import "time"

type Timer struct {
	C <-chan time.Time
	//nolint: revive
	c       chan time.Time
	timer   *time.Timer // realtime impl, if set
	nxt     time.Time   // next tick time
	mock    *Mock       // mock clock, if set
	fn      func()      // AfterFunc function, if set
	stopped bool        // True if stopped, false if running
}

func (t *Timer) fire(tt time.Time) {
	t.mock.removeTimer(t)
	if t.fn != nil {
		t.fn()
	} else {
		t.c <- tt
	}
}

func (t *Timer) next() time.Time {
	return t.nxt
}

func (t *Timer) Stop(tags ...string) bool {
	if t.timer != nil {
		return t.timer.Stop()
	}
	t.mock.mu.Lock()
	defer t.mock.mu.Unlock()
	c := newCall(clockFunctionTimerStop, tags)
	t.mock.matchCallLocked(c)
	defer close(c.complete)
	result := !t.stopped
	t.mock.removeTimerLocked(t)
	return result
}

func (t *Timer) Reset(d time.Duration, tags ...string) bool {
	if t.timer != nil {
		return t.timer.Reset(d)
	}
	t.mock.mu.Lock()
	defer t.mock.mu.Unlock()
	c := newCall(clockFunctionTimerReset, tags, withDuration(d))
	t.mock.matchCallLocked(c)
	defer close(c.complete)
	result := !t.stopped
	select {
	case <-t.c:
	default:
	}
	if d <= 0 {
		// zero or negative duration timer means we should immediately re-fire
		// it, rather than remove and re-add it.
		t.stopped = false
		go t.fire(t.mock.cur)
		return result
	}
	t.mock.removeTimerLocked(t)
	t.stopped = false
	t.nxt = t.mock.cur.Add(d)
	t.mock.addTimerLocked(t)
	return result
}
