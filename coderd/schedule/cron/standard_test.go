package cron_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/schedule/cron"
)

func TestStandard(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			name       string
			spec       string
			zone       string
			wantCron   string
			wantString string
		}{
			{
				name:       "EveryFiveMinutes",
				spec:       "*/5 * * * *",
				zone:       "UTC",
				wantCron:   "*/5 * * * *",
				wantString: "CRON_TZ=UTC */5 * * * *",
			},
			{
				name:       "Weekdays",
				spec:       "0 9 * * 1-5",
				zone:       "America/New_York",
				wantCron:   "0 9 * * 1-5",
				wantString: "CRON_TZ=America/New_York 0 9 * * 1-5",
			},
			{
				name:       "AllFieldsRestricted",
				spec:       "30 8 1,15 JAN-MAR MON-FRI",
				zone:       "Europe/Berlin",
				wantCron:   "30 8 1,15 JAN-MAR MON-FRI",
				wantString: "CRON_TZ=Europe/Berlin 30 8 1,15 JAN-MAR MON-FRI",
			},
			{
				name:       "ExtraSpacesAreNormalized",
				spec:       "  0   9 * * *  ",
				zone:       "Asia/Kolkata",
				wantCron:   "0 9 * * *",
				wantString: "CRON_TZ=Asia/Kolkata 0 9 * * *",
			},
			{
				name:       "LeapDay",
				spec:       "0 0 29 2 *",
				zone:       "UTC",
				wantCron:   "0 0 29 2 *",
				wantString: "CRON_TZ=UTC 0 0 29 2 *",
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				sched, err := cron.Standard(tc.spec, tc.zone)
				require.NoError(t, err)
				require.Equal(t, tc.wantCron, sched.Cron())
				require.Equal(t, tc.wantString, sched.String())
				require.Equal(t, tc.zone, sched.Location().String())
			})
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		testCases := []struct {
			name    string
			spec    string
			zone    string
			wantErr string
		}{
			{name: "EmptySpec", spec: "", zone: "UTC", wantErr: "empty"},
			{name: "BlankSpec", spec: "   ", zone: "UTC", wantErr: "empty"},
			{name: "FourFields", spec: "* * * *", zone: "UTC", wantErr: "expected 5 fields, got 4"},
			{name: "SixFields", spec: "0 * * * * *", zone: "UTC", wantErr: "expected 5 fields, got 6"},
			{name: "Descriptor", spec: "@daily", zone: "UTC", wantErr: "expected 5 fields, got 1"},
			{name: "CronTZPrefix", spec: "CRON_TZ=UTC 0 9 * * *", zone: "UTC", wantErr: "time zone"},
			{name: "TZPrefix", spec: "TZ=UTC 0 9 * * *", zone: "UTC", wantErr: "time zone"},
			{name: "MinuteOutOfRange", spec: "60 * * * *", zone: "UTC", wantErr: "above maximum"},
			{name: "NotANumber", spec: "a * * * *", zone: "UTC", wantErr: "parse"},
			{name: "NeverRuns", spec: "0 0 30 2 *", zone: "UTC", wantErr: "never runs"},
			{name: "EmptyZone", spec: "0 9 * * *", zone: "", wantErr: "time zone is empty"},
			{name: "LocalZone", spec: "0 9 * * *", zone: "Local", wantErr: "Local"},
			{name: "UnknownZone", spec: "0 9 * * *", zone: "Nowhere/Land", wantErr: "unknown time zone"},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				sched, err := cron.Standard(tc.spec, tc.zone)
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, sched)
			})
		}
	})
}

func TestStandard_Next(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		spec string
		zone string
		from string
		want []string
	}{
		{
			name: "WeekdaysSkipWeekend",
			spec: "0 9 * * 1-5",
			zone: "UTC",
			from: "2026-11-06T10:00:00Z",
			want: []string{"2026-11-09T09:00:00Z", "2026-11-10T09:00:00Z"},
		},
		{
			name: "FirstOfMonth",
			spec: "0 0 1 * *",
			zone: "UTC",
			from: "2026-01-31T12:00:00Z",
			want: []string{"2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z"},
		},
		{
			// Standard cron: with both fields restricted, either one matches.
			name: "DayOfMonthOrDayOfWeek",
			spec: "0 0 13 * 5",
			zone: "UTC",
			from: "2026-12-10T12:00:00Z",
			want: []string{"2026-12-11T00:00:00Z", "2026-12-13T00:00:00Z", "2026-12-18T00:00:00Z"},
		},
		{
			// 2100 is not a leap year, so the leap days are eight years apart,
			// which is longer than the five years robfig/cron searches.
			name: "LeapDayAcrossCentury",
			spec: "0 0 29 2 *",
			zone: "UTC",
			from: "2096-03-01T00:00:00Z",
			want: []string{"2104-02-29T00:00:00Z", "2108-02-29T00:00:00Z"},
		},
		{
			// America/Metlakatla moved from UTC+15:13:42 to UTC-08:46:18 on
			// 1867-10-19, so 15:00 on October 19 happened twice. The first
			// instance is the one at the old offset, a day before the second.
			name: "RepeatedTimeWithOffsetAboveFourteenHours",
			spec: "0 15 * * *",
			zone: "America/Metlakatla",
			from: "1867-10-16T00:00:00Z",
			want: []string{
				"1867-10-16T23:46:18Z", "1867-10-17T23:46:18Z",
				"1867-10-18T23:46:18Z", "1867-10-20T23:46:18Z",
			},
		},
		{
			name: "ZoneWithoutDST",
			spec: "0 9 * * *",
			zone: "Asia/Kolkata",
			from: "2026-11-01T00:00:00+05:30",
			want: []string{"2026-11-01T09:00:00+05:30", "2026-11-02T09:00:00+05:30"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(tc.spec, tc.zone)
			require.NoError(t, err)
			require.Equal(t, tc.want, nextN(t, sched, mustParseTime(t, time.RFC3339, tc.from), len(tc.want)))
		})
	}

	t.Run("ResultKeepsInputLocation", func(t *testing.T) {
		t.Parallel()
		sched, err := cron.Standard("0 9 * * *", "America/New_York")
		require.NoError(t, err)

		// 2026-11-01T00:00Z is 20:00 on October 31 in New York. The next 09:00
		// there is November 1, in standard time (UTC-5), after clocks went back.
		from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		got := sched.Next(from)
		require.Equal(t, time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC), got)
		require.Equal(t, time.UTC, got.Location())
	})

	t.Run("SubSecondInputIsExclusive", func(t *testing.T) {
		t.Parallel()
		sched, err := cron.Standard("* * * * *", "UTC")
		require.NoError(t, err)

		from := time.Date(2026, 11, 1, 12, 0, 0, 1, time.UTC)
		require.Equal(t, time.Date(2026, 11, 1, 12, 1, 0, 0, time.UTC), sched.Next(from))
		onTheMinute := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		require.Equal(t, time.Date(2026, 11, 1, 12, 1, 0, 0, time.UTC), sched.Next(onTheMinute))
	})
}

// The daylight saving rules of the Automations RFC: a repeated local time
// runs at its first instance, and a nonexistent local time runs at the first
// local time after the change.
func TestStandard_Next_DST(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		spec string
		zone string
		from string
		want []string
	}{
		// New York, clocks go forward on 2026-03-08: 02:00 becomes 03:00.
		{
			name: "NewYorkSpring/NonexistentTimeRunsAfterChange",
			spec: "30 2 * * *",
			zone: "America/New_York",
			from: "2026-03-07T12:00:00-05:00",
			want: []string{"2026-03-08T03:00:00-04:00", "2026-03-09T02:30:00-04:00", "2026-03-10T02:30:00-04:00"},
		},
		{
			name: "NewYorkSpring/NonexistentTimesAndGapEndRunOnce",
			spec: "0 2,3 * * *",
			zone: "America/New_York",
			from: "2026-03-08T00:00:00-05:00",
			want: []string{"2026-03-08T03:00:00-04:00", "2026-03-09T02:00:00-04:00", "2026-03-09T03:00:00-04:00"},
		},
		{
			name: "NewYorkSpring/ManyNonexistentTimesRunOnce",
			spec: "*/30 1-3 * * *",
			zone: "America/New_York",
			from: "2026-03-08T00:00:00-05:00",
			want: []string{
				"2026-03-08T01:00:00-05:00", "2026-03-08T01:30:00-05:00",
				"2026-03-08T03:00:00-04:00", "2026-03-08T03:30:00-04:00",
				"2026-03-09T01:00:00-04:00",
			},
		},
		{
			name: "NewYorkSpring/RunStartedInsideTheGap",
			spec: "30 2 * * *",
			zone: "America/New_York",
			from: "2026-03-08T01:59:59-05:00",
			want: []string{"2026-03-08T03:00:00-04:00", "2026-03-09T02:30:00-04:00"},
		},
		// New York, clocks go back on 2026-11-01: 02:00 becomes 01:00.
		{
			name: "NewYorkFall/RepeatedTimeRunsAtFirstInstance",
			spec: "30 1 * * *",
			zone: "America/New_York",
			from: "2026-10-31T12:00:00-04:00",
			want: []string{"2026-11-01T01:30:00-04:00", "2026-11-02T01:30:00-05:00", "2026-11-03T01:30:00-05:00"},
		},
		{
			name: "NewYorkFall/HourlyRunsOncePerLocalHour",
			spec: "0 * * * *",
			zone: "America/New_York",
			from: "2026-11-01T00:00:00-04:00",
			want: []string{"2026-11-01T01:00:00-04:00", "2026-11-01T02:00:00-05:00", "2026-11-01T03:00:00-05:00"},
		},
		{
			name: "NewYorkFall/EveryTwentyMinutesSkipsSecondInstances",
			spec: "*/20 1 * * *",
			zone: "America/New_York",
			from: "2026-11-01T00:30:00-04:00",
			want: []string{
				"2026-11-01T01:00:00-04:00", "2026-11-01T01:20:00-04:00", "2026-11-01T01:40:00-04:00",
				"2026-11-02T01:00:00-05:00",
			},
		},
		{
			name: "NewYorkFall/FromInsideRepeatedPeriodFirstInstance",
			spec: "30 1 * * *",
			zone: "America/New_York",
			from: "2026-11-01T01:10:00-04:00",
			want: []string{"2026-11-01T01:30:00-04:00", "2026-11-02T01:30:00-05:00"},
		},
		{
			name: "NewYorkFall/FromInsideRepeatedPeriodSecondInstance",
			spec: "30 1 * * *",
			zone: "America/New_York",
			from: "2026-11-01T01:10:00-05:00",
			want: []string{"2026-11-02T01:30:00-05:00"},
		},
		{
			name: "NewYorkFall/FromSecondInstanceNextTimeAfterRepeat",
			spec: "0 2 * * *",
			zone: "America/New_York",
			from: "2026-11-01T01:10:00-05:00",
			want: []string{"2026-11-01T02:00:00-05:00", "2026-11-02T02:00:00-05:00"},
		},
		// Berlin, clocks go forward on 2026-03-29 (02:00 to 03:00) and back on
		// 2026-10-25 (03:00 to 02:00).
		{
			name: "BerlinSpring/NonexistentTimeRunsAfterChange",
			spec: "30 2 * * *",
			zone: "Europe/Berlin",
			from: "2026-03-28T12:00:00+01:00",
			want: []string{"2026-03-29T03:00:00+02:00", "2026-03-30T02:30:00+02:00"},
		},
		{
			name: "BerlinFall/RepeatedTimeRunsAtFirstInstance",
			spec: "30 2 * * *",
			zone: "Europe/Berlin",
			from: "2026-10-24T12:00:00+02:00",
			want: []string{"2026-10-25T02:30:00+02:00", "2026-10-26T02:30:00+01:00"},
		},
		{
			name: "BerlinFall/TimeAfterRepeatedPeriodRunsOnce",
			spec: "0 3 * * *",
			zone: "Europe/Berlin",
			from: "2026-10-24T12:00:00+02:00",
			want: []string{"2026-10-25T03:00:00+01:00", "2026-10-26T03:00:00+01:00"},
		},
		// Lord Howe Island shifts by 30 minutes: forward on 2026-10-04 (02:00
		// to 02:30) and back on 2026-04-05 (02:00 to 01:30).
		{
			name: "LordHoweSpring/NonexistentTimeRunsAfterChange",
			spec: "15 2 * * *",
			zone: "Australia/Lord_Howe",
			from: "2026-10-03T12:00:00+10:30",
			want: []string{"2026-10-04T02:30:00+11:00", "2026-10-05T02:15:00+11:00"},
		},
		{
			name: "LordHoweFall/RepeatedTimeRunsAtFirstInstance",
			spec: "45 1 * * *",
			zone: "Australia/Lord_Howe",
			from: "2026-04-04T12:00:00+11:00",
			want: []string{"2026-04-05T01:45:00+11:00", "2026-04-06T01:45:00+10:30"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(tc.spec, tc.zone)
			require.NoError(t, err)
			from := mustParseTime(t, time.RFC3339, tc.from).In(mustLocation(t, tc.zone))
			require.Equal(t, tc.want, nextN(t, sched, from, len(tc.want)))
		})
	}
}

// TestStandard_Next_AroundDST compares Next with a minute-by-minute
// reading of the RFC rules, over three days around each clock change.
func TestStandard_Next_AroundDST(t *testing.T) {
	t.Parallel()

	// Only the hour and minute fields are restricted, so whether a wall-clock
	// minute matches depends only on its time of day.
	specs := []string{
		"* * * * *",
		"*/5 * * * *",
		"0 * * * *",
		"*/20 1 * * *",
		"*/30 1-3 * * *",
		"30 1 * * *",
		"45 1 * * *",
		"59 1 * * *",
		"0 2 * * *",
		"15 2 * * *",
		"30 2 * * *",
		"0 2,3 * * *",
		"0 3 * * *",
	}
	windows := []struct {
		name  string
		zone  string
		start string
	}{
		{"NewYorkSpring", "America/New_York", "2026-03-07T00:00:00Z"},
		{"NewYorkFall", "America/New_York", "2026-10-31T00:00:00Z"},
		{"BerlinSpring", "Europe/Berlin", "2026-03-28T00:00:00Z"},
		{"BerlinFall", "Europe/Berlin", "2026-10-24T00:00:00Z"},
		{"LordHoweSpring", "Australia/Lord_Howe", "2026-10-03T00:00:00Z"},
		{"LordHoweFall", "Australia/Lord_Howe", "2026-04-03T12:00:00Z"},
	}
	for _, w := range windows {
		t.Run(w.name, func(t *testing.T) {
			t.Parallel()
			loc := mustLocation(t, w.zone)
			from := mustParseTime(t, time.RFC3339, w.start)
			to := from.Add(72 * time.Hour)
			_, offsetFrom := from.In(loc).Zone()
			_, offsetTo := to.In(loc).Zone()
			require.NotEqual(t, offsetFrom, offsetTo, "window must contain a clock change")

			for _, spec := range specs {
				sched, err := cron.Standard(spec, w.zone)
				require.NoError(t, err)

				want := wantOccurrences(t, spec, loc, from, to)
				var got []time.Time
				// Start just before from, because Next excludes its input.
				for cur := from.Add(-time.Nanosecond); ; {
					next := sched.Next(cur)
					require.Truef(t, next.After(cur), "%s: Next(%s) = %s is not after its input", spec, cur, next)
					if !next.Before(to) {
						break
					}
					got = append(got, next)
					cur = next
				}
				require.Lenf(t, got, len(want), "%s: number of runs", spec)
				for i := range want {
					if !want[i].Equal(got[i]) {
						require.Failf(t, "wrong run", "%s: run %d: want %s, got %s",
							spec, i, want[i].In(loc).Format(time.RFC3339), got[i].In(loc).Format(time.RFC3339))
					}
				}
			}
		})
	}
}

func TestSchedule_MinInterval(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		spec string
		zone string
		want time.Duration
	}{
		{name: "EveryMinute", spec: "* * * * *", zone: "UTC", want: time.Minute},
		{name: "EveryFiveMinutes", spec: "*/5 * * * *", zone: "UTC", want: 5 * time.Minute},
		{name: "Hourly", spec: "0 * * * *", zone: "UTC", want: time.Hour},
		{name: "Daily", spec: "0 9 * * *", zone: "UTC", want: 24 * time.Hour},
		{name: "Weekdays", spec: "0 9 * * 1-5", zone: "UTC", want: 24 * time.Hour},
		{name: "TwoDaysAWeek", spec: "0 9 * * 1,3", zone: "UTC", want: 48 * time.Hour},
		{name: "Weekly", spec: "0 9 * * 1", zone: "UTC", want: 7 * 24 * time.Hour},
		{name: "Monthly", spec: "0 0 1 * *", zone: "UTC", want: 28 * 24 * time.Hour},
		{name: "GapAcrossMidnight", spec: "55 23,0 * * *", zone: "UTC", want: time.Hour},
		{name: "DayOfMonthOrDayOfWeek", spec: "0 0 13 * 5", zone: "UTC", want: 24 * time.Hour},
		// With both day fields set, a run is on either, and the closest runs
		// need a year where November 27 is a Wednesday (2002).
		{name: "DayOfMonthAndDayOfWeekInOneMonth", spec: "0 0 27 11 4", zone: "UTC", want: 24 * time.Hour},
		{name: "GapAcrossMidnightInOneMonth", spec: "55 23,0 27 11 4", zone: "UTC", want: time.Hour},
		// Min looks at one week from 1970-01-01 and reports 30m for these.
		{name: "CloseRunsOnTheFifteenth", spec: "0,30,32 12 15 * *", zone: "UTC", want: 2 * time.Minute},
		{name: "CloseRunsInFebruary", spec: "0,30,32 12 * 2 *", zone: "UTC", want: 2 * time.Minute},
		{name: "IgnoresDaylightSaving", spec: "30 2 * * *", zone: "America/New_York", want: 24 * time.Hour},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(tc.spec, tc.zone)
			require.NoError(t, err)
			require.Equal(t, tc.want, sched.MinInterval())
		})
	}

	// Schedules that run once a year or less report at least a year, so no
	// minimum interval rejects them.
	for _, spec := range []string{"0 0 29 2 *", "0 0 1 1 *"} {
		t.Run("Rare/"+spec, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(spec, "UTC")
			require.NoError(t, err)
			require.GreaterOrEqual(t, sched.MinInterval(), 365*24*time.Hour)
		})
	}
}

// nextN returns the n occurrences after from, formatted in from's location.
func nextN(t *testing.T, sched *cron.Schedule, from time.Time, n int) []string {
	t.Helper()
	got := make([]string, 0, n)
	for cur := from; len(got) < n; {
		cur = sched.Next(cur)
		require.False(t, cur.IsZero())
		got = append(got, cur.Format(time.RFC3339))
	}
	return got
}

// wantOccurrences returns the runs of spec in loc within [from, to), found by
// reading the RFC rules minute by minute. spec must restrict only the minute
// and hour fields, and from must not be inside a repeated period.
func wantOccurrences(t *testing.T, spec string, loc *time.Location, from, to time.Time) []time.Time {
	t.Helper()

	// UTC has no clock changes, so a UTC schedule shows which times of day
	// the fields select.
	utc, err := cron.Standard(spec, "UTC")
	require.NoError(t, err)
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	selected := make(map[time.Duration]bool, 24*60)
	for tod := time.Duration(0); tod < 24*time.Hour; tod += time.Minute {
		selected[tod] = utc.Next(day.Add(tod - time.Second)).Equal(day.Add(tod))
	}
	timeOfDay := func(wall time.Time) time.Duration {
		return time.Duration(wall.Hour())*time.Hour + time.Duration(wall.Minute())*time.Minute
	}

	var (
		want     []time.Time
		seen     = make(map[time.Time]bool)
		prevWall time.Time
	)
	add := func(instant time.Time) {
		if len(want) == 0 || !want[len(want)-1].Equal(instant) {
			want = append(want, instant)
		}
	}
	for instant := from; instant.Before(to); instant = instant.Add(time.Minute) {
		local := instant.In(loc)
		wall := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), 0, 0, time.UTC)

		// Wall-clock minutes that the clock skipped run at this instant, the
		// first one after the change.
		if !prevWall.IsZero() {
			for skipped := prevWall.Add(time.Minute); skipped.Before(wall); skipped = skipped.Add(time.Minute) {
				if selected[timeOfDay(skipped)] {
					add(instant)
				}
			}
		}
		// A wall-clock minute that already happened is a repeat.
		if !seen[wall] && selected[timeOfDay(wall)] {
			add(instant)
		}
		seen[wall] = true
		prevWall = wall
	}
	return want
}
