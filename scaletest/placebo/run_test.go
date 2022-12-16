package placebo_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/scaletest/placebo"
)

func Test_Runner(t *testing.T) {
	t.Skip("This test is flakey, see https://github.com/coder/coder/actions/runs/3463709674/jobs/5784335013#step:9:215")
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
			Sleep: httpapi.Duration(100 * time.Millisecond),
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
			Sleep:  httpapi.Duration(100 * time.Millisecond),
			Jitter: httpapi.Duration(100 * time.Millisecond),
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

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		r := placebo.NewRunner(placebo.Config{
			Sleep: httpapi.Duration(100 * time.Millisecond),
		})

		//nolint:gocritic // we're testing timeouts here so we want specific values
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := r.Run(ctx, "", io.Discard)
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("FailureChance", func(t *testing.T) {
		t.Parallel()

		r := placebo.NewRunner(placebo.Config{
			FailureChance: 1,
		})

		logs := bytes.NewBuffer(nil)
		err := r.Run(context.Background(), "", logs)
		require.Error(t, err)
		require.Contains(t, logs.String(), ":(")
	})
}
