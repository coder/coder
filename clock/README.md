# Quartz

A Go time testing library for writing deterministic unit tests

_Note: Quartz is the name I'm targeting for the standalone open source project when we spin this
out._

## Why another time testing library?

Writing good unit tests for components and functions that use the `time` package is difficult, even
though several open source libraries exist. In building Quartz, we took some inspiration from

- [github.com/benbjohnson/clock](https://github.com/benbjohnson/clock)
- Tailscale's [tstest.Clock](https://github.com/coder/tailscale/blob/main/tstest/clock.go)
- [github.com/aspenmesh/tock](https://github.com/aspenmesh/tock)

Quartz shares the high level design of a `Clock` interface that closely resembles the functions in
the `time` standard library, and a "real" clock passes thru to the standard library in production,
while a mock clock gives precise control in testing.

Our high level goal is to write unit tests that

1. execute quickly
2. don't flake
3. are straightforward to write and understand

For several reasons, this is a tall order when it comes to code that depends on time, and we found
the existing libraries insufficient for our goals.

For tests to execute quickly without flakes, we want to focus on _determinism_: the test should run
the same each time, and it should be easy to force the system into a known state (no races) before
executing test assertions. `time.Sleep`, `runtime.Gosched()`, and
polling/[Eventually](https://pkg.go.dev/github.com/stretchr/testify/assert#Eventually) are all
symptoms of an inability to do this easily.

### Preventing test flakes

The following example comes from the README from benbjohnson/clock:

```go
mock := clock.NewMock()
count := 0

// Kick off a timer to increment every 1 mock second.
go func() {
    ticker := mock.Ticker(1 * time.Second)
    for {
        <-ticker.C
        count++
    }
}()
runtime.Gosched()

// Move the clock forward 10 seconds.
mock.Add(10 * time.Second)

// This prints 10.
fmt.Println(count)
```

The first race condition is fairly obvious: moving the clock forward 10 seconds may generate 10
ticks on the `ticker.C` channel, but there is no guarantee that `count++` executes before
`fmt.Println(count)`.

The second race condition is more subtle, but `runtime.Gosched()` is the tell. Since the ticker
is started on a separate goroutine, there is no guarantee that `mock.Ticker()` executes before
`mock.Add()`. `runtime.Gosched()` is an attempt to get this to happen, but it makes no hard
promises. On a busy system, especially when running tests in parallel, this can flake, advance the
time 10 seconds first, then start the ticker and never generate a tick.

Let's talk about how Quartz tackles these problems.

In our experience, an extremely common use case is creating a ticker then doing a 2-arm `select`
with ticks in one and context expiring in another, i.e.

```go
t := time.NewTicker(duration)
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-t.C:
        err := do()
        if err != nil {
            return err
        }
    }
}
```

In Quartz, we refactor this to be more compact and testing friendly:

```go
t := clock.TickerFunc(ctx, duration, do)
return t.Wait()
```

This affords the mock `Clock` the ability to explicitly know when processing of a tick is finished
because it's wrapped in the function passed to `TickerFunc` (`do()` in this example).

In Quartz, when you advance the clock, you are returned an object you can `Wait()` on to ensure all
ticks and timers triggered are finished. This solves the first race condition in the example.

(As an aside, we still support a traditional standard library-style `Ticker`. You may find it useful
if you want to keep your code as close as possible to the standard library, or if you need to use
the channel in a larger `select` block. In that case, you'll have to find some other mechanism to
sync tick processing to your test code.)

To prevent race conditions related to the starting of the ticker, Quartz allows you to set "traps"
for calls that access the clock.

```go
func TestTicker(t *testing.T) {
    mClock := quartz.NewMock(t)
    trap := mClock.Trap().TickerFunc()
    defer trap.Close() // stop trapping at end
    go runMyTicker(mClock) // async calls TickerFunc()
    call := trap.Wait(context.Background()) // waits for a call and blocks its return
    call.Release() // allow the TickerFunc() call to return
    // optionally check the duration using call.Duration
    // Move the clock forward 1 tick
    mClock.Advance(time.Second).MustWait(context.Background())
    // assert results of the tick
}
```

Trapping and then releasing the call to `TickerFunc()` ensures the ticker is started at a
deterministic time, so our calls to `Advance()` will have a predictable effect.

Take a look at `TestExampleTickerFunc` in `example_test.go` for a complete worked example.

### Complex time dependence

Another difficult issue to handle when unit testing is when some code under test makes multiple
calls that depend on the time, and you want to simulate some time passing between them.

A very basic example is measuring how long something took:

```go
var measurement time.Duration
go func(clock quartz.Clock) {
    start := clock.Now()
    doSomething()
    measurement = clock.Since(start)
}(mClock)

// how to get measurement to be, say, 5 seconds?
```

The two calls into the clock happen asynchronously, so we need to be able to advance the clock after
the first call to `Now()` but before the call to `Since()`. Doing this with the libraries we
mentioned above means that you have to be able to mock out or otherwise block the completion of
`doSomething()`.

But, with the trap functionality we mentioned in the previous section, you can deterministically
control the time each call sees.

```go
trap := mClock.Trap().Since()
var measurement time.Duration
go func(clock quartz.Clock) {
    start := clock.Now()
    doSomething()
    measurement = clock.Since(start)
}(mClock)

c := trap.Wait(ctx)
mClock.Advance(5*time.Second)
c.Release()
```

We wait until we trap the `clock.Since()` call, which implies that `clock.Now()` has completed, then
advance the mock clock 5 seconds. Finally, we release the `clock.Since()` call. Any changes to the
clock that happen _before_ we release the call will be included in the time used for the
`clock.Since()` call.

As a more involved example, consider an inactivity timeout: we want something to happen if there is
no activity recorded for some period, say 10 minutes in the following example:

```go
type InactivityTimer struct {
    mu sync.Mutex
    activity time.Time
    clock quartz.Clock
}

func (i *InactivityTimer) Start() {
    i.mu.Lock()
    defer i.mu.Unlock()
    next := i.clock.Until(i.activity.Add(10*time.Minute))
    t := i.clock.AfterFunc(next, func() {
        i.mu.Lock()
        defer i.mu.Unlock()
        next := i.clock.Until(i.activity.Add(10*time.Minute))
        if next == 0 {
            i.timeoutLocked()
            return
        }
        t.Reset(next)
    })
}
```

The actual contents of `timeoutLocked()` doesn't matter for this example, and assume there are other
functions that record the latest `activity`.

We found that some time testing libraries hold a lock on the mock clock while calling the function
passed to `AfterFunc`, resulting in a deadlock if you made clock calls from within.

Others allow this sort of thing, but don't have the flexibility to test edge cases. There is a
subtle bug in our `Start()` function. The timer may pop a little late, and/or some measurable real
time may elapse before `Until()` gets called inside the `AfterFunc`. If there hasn't been activity,
`next` might be negative.

To test this in Quartz, we'll use a trap. We only want to trap the inner `Until()` call, not the
initial one, so to make testing easier we can "tag" the call we want. Like this:

```go
func (i *InactivityTimer) Start() {
    i.mu.Lock()
    defer i.mu.Unlock()
    next := i.clock.Until(i.activity.Add(10*time.Minute))
    t := i.clock.AfterFunc(next, func() {
        i.mu.Lock()
        defer i.mu.Unlock()
        next := i.clock.Until(i.activity.Add(10*time.Minute), "inner")
        if next == 0 {
            i.timeoutLocked()
            return
        }
        t.Reset(next)
    })
}
```

All Quartz `Clock` functions, and functions on returned timers and tickers support zero or more
string tags that allow traps to match on them.

```go
func TestInactivityTimer_Late(t *testing.T) {
    // set a timeout on the test itself, so that if Wait functions get blocked, we don't have to
    // wait for the default test timeout of 10 minutes.
    ctx, cancel := context.WithTimeout(10*time.Second)
    defer cancel()
    mClock := quartz.NewMock(t)
    trap := mClock.Trap.Until("inner")
    defer trap.Close()

    it := &InactivityTimer{
        activity: mClock.Now(),
        clock: mClock,
    }
    it.Start()

    // Trigger the AfterFunc
    w := mClock.Advance(10*time.Minute)
    c := trap.Wait(ctx)
    // Advance the clock a few ms to simulate a busy system
    mClock.Advance(3*time.Millisecond)
    c.Release() // Until() returns
    w.MustWait(ctx) // Wait for the AfterFunc to wrap up

    // Assert that the timeoutLocked() function was called
}
```

This test case will fail with our bugged implementation, since the triggered AfterFunc won't call
`timeoutLocked()` and instead will reset the timer with a negative number. The fix is easy, use
`next <= 0` as the comparison.
