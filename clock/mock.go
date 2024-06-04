package clock

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Mock is the testing implementation of Clock.  It tracks a time that monotonically increases
// during a test, triggering any timers or tickers automatically.
type Mock struct {
	mu sync.Mutex

	// cur is the current time
	cur time.Time
	// advancing is true when we are in the process of advancing the clock.  We don't support
	// multiple goroutines doing this at once.
	advancing bool

	all        []event
	nextTime   time.Time
	nextEvents []event
	traps      []*Trap
}

type event interface {
	next() time.Time
	fire(t time.Time)
}

func (m *Mock) TickerFunc(ctx context.Context, d time.Duration, f func() error, tags ...string) Waiter {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := newCall(clockFunctionTickerFunc, tags, withDuration(d))
	m.matchCallLocked(c)
	defer close(c.complete)
	t := &mockTickerFunc{
		ctx:  ctx,
		d:    d,
		f:    f,
		nxt:  m.cur.Add(d),
		mock: m,
		cond: sync.NewCond(&m.mu),
	}
	m.all = append(m.all, t)
	m.recomputeNextLocked()
	go t.waitForCtx()
	return t
}

func (m *Mock) NewTimer(d time.Duration, tags ...string) *Timer {
	if d < 0 {
		panic("duration must be positive or zero")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c := newCall(clockFunctionNewTimer, tags, withDuration(d))
	defer close(c.complete)
	m.matchCallLocked(c)
	ch := make(chan time.Time, 1)
	t := &Timer{
		C:    ch,
		c:    ch,
		nxt:  m.cur.Add(d),
		mock: m,
	}
	m.addTimerLocked(t)
	return t
}

func (m *Mock) addTimerLocked(t *Timer) {
	m.all = append(m.all, t)
	m.recomputeNextLocked()
}

func (m *Mock) recomputeNextLocked() {
	var best time.Time
	var events []event
	for _, e := range m.all {
		if best.IsZero() || e.next().Before(best) {
			best = e.next()
			events = []event{e}
			continue
		}
		if e.next().Equal(best) {
			events = append(events, e)
			continue
		}
	}
	m.nextTime = best
	m.nextEvents = events
}

func (m *Mock) removeTimer(t *Timer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeTimerLocked(t)
}

func (m *Mock) removeTimerLocked(t *Timer) {
	defer m.recomputeNextLocked()
	t.stopped = true
	var e event = t
	for i := range m.all {
		if m.all[i] == e {
			m.all = append(m.all[:i], m.all[i+1:]...)
			return
		}
	}
}

func (m *Mock) removeTickerFuncLocked(ct *mockTickerFunc) {
	defer m.recomputeNextLocked()
	var e event = ct
	for i := range m.all {
		if m.all[i] == e {
			m.all = append(m.all[:i], m.all[i+1:]...)
			return
		}
	}
}

func (m *Mock) matchCallLocked(c *Call) {
	var traps []*Trap
	for _, t := range m.traps {
		if t.matches(c) {
			traps = append(traps, t)
		}
	}
	if len(traps) == 0 {
		return
	}
	c.releases.Add(len(traps))
	m.mu.Unlock()
	for _, t := range traps {
		go t.catch(c)
	}
	c.releases.Wait()
	m.mu.Lock()
}

// Advance moves the clock forward by d, triggering any timers or tickers.  Advance will wait for
// tick functions of tickers created using TickerFunc to complete before returning from
// Advance. If multiple timers or tickers trigger simultaneously, they are all run on separate go
// routines.
func (m *Mock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(d)
}

func (m *Mock) advanceLocked(d time.Duration) {
	if m.advancing {
		panic("multiple simultaneous calls to Advance not supported")
	}
	m.advancing = true
	defer func() {
		m.advancing = false
	}()

	fin := m.cur.Add(d)
	for {
		// nextTime.IsZero implies no events scheduled
		if m.nextTime.IsZero() || m.nextTime.After(fin) {
			m.cur = fin
			return
		}

		if m.nextTime.After(m.cur) {
			m.cur = m.nextTime
		}

		wg := sync.WaitGroup{}
		for i := range m.nextEvents {
			e := m.nextEvents[i]
			t := m.cur
			wg.Add(1)
			go func() {
				e.fire(t)
				wg.Done()
			}()
		}
		// release the lock and let the events resolve.  This allows them to call back into the
		// Mock to query the time or set new timers.  Each event should remove or reschedule
		// itself from nextEvents.
		m.mu.Unlock()
		wg.Wait()
		m.mu.Lock()
	}
}

// Set the time to t.  If the time is after the current mocked time, then this is equivalent to
// Advance() with the difference.  You may only Set the time earlier than the current time before
// starting tickers and timers (e.g. at the start of your test case).
func (m *Mock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.Before(m.cur) {
		// past
		if !m.nextTime.IsZero() {
			panic("Set mock clock to the past after timers/tickers started")
		}
		m.cur = t
		return
	}
	// future, just advance as normal.
	m.advanceLocked(t.Sub(m.cur))
}

// Trapper allows the creation of Traps
type Trapper struct {
	// mock is the underlying Mock.  This is a thin wrapper around Mock so that
	// we can have our interface look like mClock.Trap().NewTimer("foo")
	mock *Mock
}

func (t Trapper) NewTimer(tags ...string) *Trap {
	return t.mock.newTrap(clockFunctionNewTimer, tags)
}

func (t Trapper) TimerStop(tags ...string) *Trap {
	return t.mock.newTrap(clockFunctionTimerStop, tags)
}

func (t Trapper) TimerReset(tags ...string) *Trap {
	return t.mock.newTrap(clockFunctionTimerReset, tags)
}

func (t Trapper) TickerFunc(tags ...string) *Trap {
	return t.mock.newTrap(clockFunctionTickerFunc, tags)
}

func (t Trapper) TickerFuncWait(tags ...string) *Trap {
	return t.mock.newTrap(clockFunctionTickerFuncWait, tags)
}

func (m *Mock) Trap() Trapper {
	return Trapper{m}
}

func (m *Mock) newTrap(fn clockFunction, tags []string) *Trap {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := &Trap{
		fn:    fn,
		tags:  tags,
		mock:  m,
		calls: make(chan *Call),
		done:  make(chan struct{}),
	}
	m.traps = append(m.traps, tr)
	return tr
}

// NewMock creates a new Mock with the time set to midnight UTC on Jan 1, 2024.
// You may re-set the time earlier than this, but only before timers or tickers
// are created.
func NewMock() *Mock {
	cur, err := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	if err != nil {
		panic(err)
	}
	return &Mock{
		cur: cur,
	}
}

var _ Clock = &Mock{}

type mockTickerFunc struct {
	ctx  context.Context
	d    time.Duration
	f    func() error
	nxt  time.Time
	mock *Mock

	// cond is a condition Locked on the main Mock.mu
	cond *sync.Cond
	// done is true when the ticker exits
	done bool
	// err holds the error when the ticker exits
	err error
}

func (m *mockTickerFunc) next() time.Time {
	return m.nxt
}

func (m *mockTickerFunc) fire(t time.Time) {
	m.mock.mu.Lock()
	defer m.mock.mu.Unlock()
	if m.done {
		return
	}
	if !m.nxt.Equal(t) {
		panic("mockTickerFunc fired at wrong time")
	}
	m.nxt = m.nxt.Add(m.d)
	m.mock.recomputeNextLocked()

	m.mock.mu.Unlock()
	err := m.f()
	m.mock.mu.Lock()
	if err != nil {
		m.exitLocked(err)
	}
}

func (m *mockTickerFunc) exitLocked(err error) {
	if m.done {
		return
	}
	m.done = true
	m.err = err
	m.mock.removeTickerFuncLocked(m)
	m.cond.Broadcast()
}

func (m *mockTickerFunc) waitForCtx() {
	<-m.ctx.Done()
	m.mock.mu.Lock()
	defer m.mock.mu.Unlock()
	m.exitLocked(m.ctx.Err())
}

func (m *mockTickerFunc) Wait(tags ...string) error {
	m.mock.mu.Lock()
	defer m.mock.mu.Unlock()
	c := newCall(clockFunctionTickerFuncWait, tags)
	m.mock.matchCallLocked(c)
	defer close(c.complete)
	for !m.done {
		m.cond.Wait()
	}
	return m.err
}

var _ Waiter = &mockTickerFunc{}

type clockFunction int

const (
	clockFunctionNewTimer clockFunction = iota
	clockFunctionTimerStop
	clockFunctionTimerReset
	clockFunctionTickerFunc
	clockFunctionTickerFuncWait
)

type callArg func(c *Call)

type Call struct {
	Time     time.Time
	Duration time.Duration
	Tags     []string

	fn       clockFunction
	releases sync.WaitGroup
	complete chan struct{}
}

func (c *Call) Release() {
	c.releases.Done()
	<-c.complete
}

// nolint: unused // it will be soon
func withTime(t time.Time) callArg {
	return func(c *Call) {
		c.Time = t
	}
}

func withDuration(d time.Duration) callArg {
	return func(c *Call) {
		c.Duration = d
	}
}

func newCall(fn clockFunction, tags []string, args ...callArg) *Call {
	c := &Call{
		fn:       fn,
		Tags:     tags,
		complete: make(chan struct{}),
	}
	for _, a := range args {
		a(c)
	}
	return c
}

type Trap struct {
	fn    clockFunction
	tags  []string
	mock  *Mock
	calls chan *Call
	done  chan struct{}
}

func (t *Trap) catch(c *Call) {
	select {
	case t.calls <- c:
	case <-t.done:
		c.Release()
	}
}

func (t *Trap) matches(c *Call) bool {
	if t.fn != c.fn {
		return false
	}
	for _, tag := range t.tags {
		if !slices.Contains(c.Tags, tag) {
			return false
		}
	}
	return true
}

func (t *Trap) Close() {
	t.mock.mu.Lock()
	defer t.mock.mu.Unlock()
	for i, tr := range t.mock.traps {
		if t == tr {
			t.mock.traps = append(t.mock.traps[:i], t.mock.traps[i+1:]...)
		}
	}
	close(t.done)
}

var ErrTrapClosed = errors.New("trap closed")

func (t *Trap) Wait(ctx context.Context) (*Call, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, ErrTrapClosed
	case c := <-t.calls:
		return c, nil
	}
}
