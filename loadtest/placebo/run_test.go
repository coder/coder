package placebo_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/loadtest/placebo"
)

func Test_Runner(t *testing.T) {
	t.Parallel()

	t.Run("NoSleep", func(t *testing.T) {
		t.Parallel()

		r := placebo.NewRunner(placebo.Config{})
		start := time.Now()
		logs := bytes.NewBuffer(nil)
		err := r.Run(context.Background(), "", logs)
		require.NoError(t, err)

		require.WithinDuration(t, time.Now(), start, 100*time.Millisecond)
		require.Empty(t, logs.String())
	})

	t.Run("Sleep", func(t *testing.T) {
		t.Parallel()

		r := placebo.NewRunner(placebo.Config{
			Sleep: 100 * time.Millisecond,
		})

		start := time.Now()
		logs := bytes.NewBuffer(nil)
		err := r.Run(context.Background(), "", logs)
		require.NoError(t, err)

		require.WithinRange(t, time.Now(), start.Add(90*time.Millisecond), start.Add(200*time.Millisecond))
		require.Contains(t, logs.String(), "sleeping for 100ms")
	})

	t.Run("Jitter", func(t *testing.T) {
		t.Parallel()

		r := placebo.NewRunner(placebo.Config{
			Sleep:  100 * time.Millisecond,
			Jitter: 100 * time.Millisecond,
		})

		start := time.Now()
		logs := bytes.NewBuffer(nil)
		err := r.Run(context.Background(), "", logs)
		require.NoError(t, err)

		require.WithinRange(t, time.Now(), start.Add(90*time.Millisecond), start.Add(300*time.Millisecond))
		logsStr := logs.String()
		require.Contains(t, logsStr, "sleeping for")
		require.NotContains(t, logsStr, "sleeping for 100ms")
	})
}
