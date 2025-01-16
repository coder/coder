package notify_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/autobuild/notify"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestNotifier(t *testing.T) {
	t.Parallel()

	now := time.Date(2022, 5, 13, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		Name              string
		Countdown         []time.Duration
		PollInterval      time.Duration
		NTicks            int
		ConditionDeadline time.Time
		NumConditions     int
		NumCallbacks      int
	}{
		{
			Name:              "zero deadline",
			Countdown:         durations(),
			PollInterval:      time.Second,
			NTicks:            0,
			ConditionDeadline: time.Time{},
			NumConditions:     1,
			NumCallbacks:      0,
		},
		{
			Name:              "no calls",
			Countdown:         durations(),
			PollInterval:      time.Second,
			NTicks:            0,
			ConditionDeadline: now,
			NumConditions:     1,
			NumCallbacks:      0,
		},
		{
			Name:              "exactly one call",
			Countdown:         durations(time.Second),
			PollInterval:      time.Second,
			NTicks:            1,
			ConditionDeadline: now.Add(time.Second),
			NumConditions:     2,
			NumCallbacks:      1,
		},
		{
			Name:              "two calls",
			Countdown:         durations(4*time.Second, 2*time.Second),
			PollInterval:      time.Second,
			NTicks:            5,
			ConditionDeadline: now.Add(5 * time.Second),
			NumConditions:     6,
			NumCallbacks:      2,
		},
		{
			Name:              "wrong order should not matter",
			Countdown:         durations(2*time.Second, 4*time.Second),
			PollInterval:      time.Second,
			NTicks:            5,
			ConditionDeadline: now.Add(5 * time.Second),
			NumConditions:     6,
			NumCallbacks:      2,
		},
		{
			Name:              "ssh autostop notify",
			Countdown:         durations(5*time.Minute, time.Minute),
			PollInterval:      30 * time.Second,
			NTicks:            120,
			ConditionDeadline: now.Add(30 * time.Minute),
			NumConditions:     121,
			NumCallbacks:      2,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			mClock := quartz.NewMock(t)
			mClock.Set(now).MustWait(ctx)
			numConditions := 0
			numCalls := 0
			cond := func(time.Time) (time.Time, func()) {
				numConditions++
				return testCase.ConditionDeadline, func() {
					numCalls++
				}
			}

			trap := mClock.Trap().TickerFunc("notifier", "poll")
			defer trap.Close()

			n := notify.New(cond, testCase.PollInterval, testCase.Countdown, notify.WithTestClock(mClock))
			defer n.Close()

			trap.MustWait(ctx).Release() // ensure ticker started
			for i := 0; i < testCase.NTicks; i++ {
				interval, w := mClock.AdvanceNext()
				w.MustWait(ctx)
				require.Equal(t, testCase.PollInterval, interval)
			}

			require.Equal(t, testCase.NumCallbacks, numCalls)
			require.Equal(t, testCase.NumConditions, numConditions)
		})
	}
}

func durations(ds ...time.Duration) []time.Duration {
	return ds
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}
