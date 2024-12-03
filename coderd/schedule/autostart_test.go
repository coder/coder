package schedule_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/schedule"
)

func TestNextAllowedAutostart(t *testing.T) {
	t.Parallel()

	t.Run("WhenScheduleOutOfSync", func(t *testing.T) {
		t.Parallel()

		// 1st January 2024 is a Monday
		at := time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC)
		//  Monday-Friday 9:00AM UTC
		sched := "CRON_TZ=UTC 00 09 * * 1-5"
		// Only allow an autostart on mondays
		opts := schedule.TemplateScheduleOptions{
			AutostartRequirement: schedule.TemplateAutostartRequirement{
				DaysOfWeek: 0b00000001,
			},
		}

		// NextAutostart will return a non-allowed autostart time as
		// our AutostartRequirement only allows Mondays but we expect
		// this to return a Tuesday.
		next, allowed := schedule.NextAutostart(at, sched, opts)
		require.False(t, allowed)
		require.Equal(t, time.Date(2024, time.January, 2, 9, 0, 0, 0, time.UTC), next)

		// NextAllowedAutostart should return the next allowed autostart time.
		next, err := schedule.NextAllowedAutostart(at, sched, opts)
		require.NoError(t, err)
		require.Equal(t, time.Date(2024, time.January, 8, 9, 0, 0, 0, time.UTC), next)
	})
}
