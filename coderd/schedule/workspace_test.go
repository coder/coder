package schedule_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
)

func TestGetWorkspaceAutostartSchedule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		autostartEnabled    bool
		autostartSchedule   string
		expectedNotNil      bool
		expectedErrContains string
	}{
		{
			name:                "autostart disabled",
			autostartEnabled:    false,
			autostartSchedule:   "CRON_TZ=UTC 0 0 * * *",
			expectedNotNil:      false,
			expectedErrContains: "",
		},
		{
			name:                "autostart enabled, no schedule",
			autostartEnabled:    true,
			autostartSchedule:   "",
			expectedNotNil:      false,
			expectedErrContains: "",
		},
		{
			name:                "autostart enabled, invalid schedule",
			autostartEnabled:    true,
			autostartSchedule:   "dean was here",
			expectedNotNil:      false,
			expectedErrContains: "parse workspace autostart schedule",
		},
		{
			name:                "autostart enabled, OK schedule",
			autostartEnabled:    true,
			autostartSchedule:   "CRON_TZ=UTC 0 0 * * *",
			expectedNotNil:      true,
			expectedErrContains: "",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			sched, err := schedule.GetWorkspaceAutostartSchedule(schedule.TemplateScheduleOptions{
				UserAutostartEnabled: c.autostartEnabled,
			}, database.Workspace{
				AutostartSchedule: sql.NullString{
					String: c.autostartSchedule,
					Valid:  true,
				},
			})

			if c.expectedErrContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.expectedErrContains)
				return
			}
			require.NoError(t, err)
			if c.expectedNotNil {
				require.NotNil(t, sched)
			} else {
				require.Nil(t, sched)
			}
		})
	}
}

func TestWorkspaceTTL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		wsTTL               time.Duration
		userAutostopEnabled bool
		templateDefaultTTL  time.Duration
		expected            time.Duration
	}{
		{
			name:                "Workspace",
			wsTTL:               time.Hour,
			userAutostopEnabled: true,
			templateDefaultTTL:  time.Hour * 2,
			expected:            time.Hour,
		},
		{
			name:                "WorkspaceZero",
			wsTTL:               0,
			userAutostopEnabled: true,
			templateDefaultTTL:  time.Hour * 2,
			expected:            0,
		},
		{
			name:                "Template",
			wsTTL:               time.Hour,
			userAutostopEnabled: false,
			templateDefaultTTL:  time.Hour * 2,
			expected:            time.Hour * 2,
		},
		{
			name:                "TemplateZero",
			wsTTL:               time.Hour,
			userAutostopEnabled: false,
			templateDefaultTTL:  0,
			expected:            0,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := schedule.WorkspaceTTL(schedule.TemplateScheduleOptions{
				UserAutostopEnabled: c.userAutostopEnabled,
				DefaultTTL:          c.templateDefaultTTL,
			}, database.Workspace{
				Ttl: sql.NullInt64{
					Int64: int64(c.wsTTL),
					Valid: true,
				},
			})
			require.Equal(t, c.expected, got)
		})
	}
}

func TestMaybeBumpDeadline(t *testing.T) {
	t.Parallel()

	midnight := time.Date(2023, 10, 4, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name              string
		autostartSchedule string
		deadline          time.Time
		ttl               time.Duration
		expected          time.Time
	}{
		{
			name:              "not eligible",
			autostartSchedule: "CRON_TZ=UTC 0 0 * * *",
			deadline:          midnight.Add(time.Hour * -2),
			ttl:               time.Hour * 10,
			// not eligible because the deadline is over an hour before the next
			// autostart time
			expected: time.Time{},
		},
		{
			name:              "autostart before deadline by 1h",
			autostartSchedule: "CRON_TZ=UTC 0 0 * * *",
			deadline:          midnight.Add(time.Hour),
			ttl:               time.Hour * 10,
			expected:          midnight.Add(time.Hour * 10),
		},
		{
			name:              "autostart before deadline by 9h",
			autostartSchedule: "CRON_TZ=UTC 0 0 * * *",
			deadline:          midnight.Add(time.Hour * 9),
			ttl:               time.Hour * 10,
			// should still be bumped
			expected: midnight.Add(time.Hour * 10),
		},
		{
			name:              "eligible but exceeds next next autostart",
			autostartSchedule: "CRON_TZ=UTC 0 0 * * *",
			deadline:          midnight.Add(time.Hour * 1),
			// ttl causes next autostart + 25h to exceed the next next autostart
			ttl: time.Hour * 25,
			// should not be bumped to avoid infinite bumping every day
			expected: time.Time{},
		},
		{
			name:              "deadline is 1h before autostart",
			autostartSchedule: "CRON_TZ=UTC 0 0 * * *",
			deadline:          midnight.Add(time.Hour * -1).Add(time.Minute),
			ttl:               time.Hour * 10,
			// should still be bumped
			expected: midnight.Add(time.Hour * 10),
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			sched, err := cron.Weekly(c.autostartSchedule)
			require.NoError(t, err)

			got := schedule.MaybeBumpDeadline(sched, c.deadline, c.ttl)
			require.WithinDuration(t, c.expected, got, time.Minute)
		})
	}
}
