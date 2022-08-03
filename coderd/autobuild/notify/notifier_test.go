package notify_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/autobuild/notify"
)

func TestNotifier(t *testing.T) {
	t.Parallel()

	now := time.Now()

	testCases := []struct {
		Name              string
		Countdown         []time.Duration
		Ticks             []time.Time
		ConditionDeadline time.Time
		NumConditions     int64
		NumCallbacks      int64
	}{
		{
			Name:              "zero deadline",
			Countdown:         durations(),
			Ticks:             fakeTicker(now, time.Second, 0),
			ConditionDeadline: time.Time{},
			NumConditions:     1,
			NumCallbacks:      0,
		},
		{
			Name:              "no calls",
			Countdown:         durations(),
			Ticks:             fakeTicker(now, time.Second, 0),
			ConditionDeadline: now,
			NumConditions:     1,
			NumCallbacks:      0,
		},
		{
			Name:              "exactly one call",
			Countdown:         durations(time.Second),
			Ticks:             fakeTicker(now, time.Second, 1),
			ConditionDeadline: now.Add(time.Second),
			NumConditions:     2,
			NumCallbacks:      1,
		},
		{
			Name:              "two calls",
			Countdown:         durations(4*time.Second, 2*time.Second),
			Ticks:             fakeTicker(now, time.Second, 5),
			ConditionDeadline: now.Add(5 * time.Second),
			NumConditions:     6,
			NumCallbacks:      2,
		},
		{
			Name:              "wrong order should not matter",
			Countdown:         durations(2*time.Second, 4*time.Second),
			Ticks:             fakeTicker(now, time.Second, 5),
			ConditionDeadline: now.Add(5 * time.Second),
			NumConditions:     6,
			NumCallbacks:      2,
		},
		{
			Name:              "ssh autostop notify",
			Countdown:         durations(5*time.Minute, time.Minute),
			Ticks:             fakeTicker(now, 30*time.Second, 120),
			ConditionDeadline: now.Add(30 * time.Minute),
			NumConditions:     121,
			NumCallbacks:      2,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			ch := make(chan time.Time)
			numConditions := atomic.NewInt64(0)
			numCalls := atomic.NewInt64(0)
			cond := func(time.Time) (time.Time, func()) {
				numConditions.Inc()
				return testCase.ConditionDeadline, func() {
					numCalls.Inc()
				}
			}
			var wg sync.WaitGroup
			go func() {
				defer wg.Done()
				n := notify.New(cond, testCase.Countdown...)
				defer n.Close()
				n.Poll(ch)
			}()
			wg.Add(1)
			for _, tick := range testCase.Ticks {
				ch <- tick
			}
			close(ch)
			wg.Wait()
			require.Equal(t, testCase.NumCallbacks, numCalls.Load())
			require.Equal(t, testCase.NumConditions, numConditions.Load())
		})
	}
}

func durations(ds ...time.Duration) []time.Duration {
	return ds
}

func fakeTicker(t time.Time, d time.Duration, n int) []time.Time {
	var ts []time.Time
	for i := 1; i <= n; i++ {
		ts = append(ts, t.Add(time.Duration(n)*d))
	}
	return ts
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
