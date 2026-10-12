package cron_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/schedule/cron"
)

func Test_Weekly(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name               string
		spec               string
		at                 time.Time
		expectedNext       time.Time
		expectedMin        time.Duration
		expectedDaysOfWeek string
		expectedError      string
		expectedCron       string
		expectedLocation   *time.Location
		expectedString     string
		expectedTime       string
	}{
		{
			name:               "with timezone",
			spec:               "CRON_TZ=US/Central 30 9 * * 1-5",
			at:                 time.Date(2022, 4, 1, 14, 29, 0, 0, time.UTC),
			expectedNext:       time.Date(2022, 4, 1, 14, 30, 0, 0, time.UTC),
			expectedMin:        24 * time.Hour,
			expectedDaysOfWeek: "Mon-Fri",
			expectedError:      "",
			expectedCron:       "30 9 * * 1-5",
			expectedLocation:   mustLocation(t, "US/Central"),
			expectedString:     "CRON_TZ=US/Central 30 9 * * 1-5",
			expectedTime:       "9:30AM",
		},
		{
			name:               "without timezone",
			spec:               "30 9 * * 1-5",
			at:                 time.Date(2022, 4, 1, 9, 29, 0, 0, time.UTC),
			expectedNext:       time.Date(2022, 4, 1, 9, 30, 0, 0, time.UTC),
			expectedMin:        24 * time.Hour,
			expectedDaysOfWeek: "Mon-Fri",
			expectedError:      "",
			expectedCron:       "30 9 * * 1-5",
			expectedLocation:   time.UTC,
			expectedString:     "CRON_TZ=UTC 30 9 * * 1-5",
			expectedTime:       "9:30AM",
		},
		{
			name:               "24h format",
			spec:               "30 13 * * 1-5",
			at:                 time.Date(2022, 4, 1, 13, 29, 0, 0, time.UTC),
			expectedNext:       time.Date(2022, 4, 1, 13, 30, 0, 0, time.UTC),
			expectedMin:        24 * time.Hour,
			expectedDaysOfWeek: "Mon-Fri",
			expectedError:      "",
			expectedCron:       "30 13 * * 1-5",
			expectedLocation:   time.UTC,
			expectedString:     "CRON_TZ=UTC 30 13 * * 1-5",
			expectedTime:       "1:30PM",
		},
		{
			name:               "convoluted with timezone",
			spec:               "CRON_TZ=US/Central */5 12-18 * * 1,3,6",
			at:                 time.Date(2022, 4, 1, 14, 29, 0, 0, time.UTC),
			expectedNext:       time.Date(2022, 4, 2, 17, 0, 0, 0, time.UTC), // Apr 1 was a Friday in 2022
			expectedMin:        5 * time.Minute,
			expectedDaysOfWeek: "Mon,Wed,Sat",
			expectedError:      "",
			expectedCron:       "*/5 12-18 * * 1,3,6",
			expectedLocation:   mustLocation(t, "US/Central"),
			expectedString:     "CRON_TZ=US/Central */5 12-18 * * 1,3,6",
			expectedTime:       "cron(*/5 12-18)",
		},
		{
			name:               "another convoluted example",
			spec:               "CRON_TZ=US/Central 10,20,40-50 * * * *",
			at:                 time.Date(2022, 4, 1, 14, 29, 0, 0, time.UTC),
			expectedNext:       time.Date(2022, 4, 1, 14, 40, 0, 0, time.UTC),
			expectedMin:        time.Minute,
			expectedDaysOfWeek: "daily",
			expectedError:      "",
			expectedCron:       "10,20,40-50 * * * *",
			expectedLocation:   mustLocation(t, "US/Central"),
			expectedString:     "CRON_TZ=US/Central 10,20,40-50 * * * *",
			expectedTime:       "cron(10,20,40-50 *)",
		},
		{
			name:          "time.Local will bite you",
			spec:          "CRON_TZ=Local 30 9 * * 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "schedules scoped to time.Local are not supported",
		},
		{
			name:          "invalid schedule",
			spec:          "asdfasdfasdfsd",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix",
		},
		{
			name:          "invalid location",
			spec:          "CRON_TZ=Fictional/Country 30 9 * * 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "parse schedule: provided bad location Fictional/Country: unknown time zone Fictional/Country",
		},
		{
			name:          "invalid schedule with 3 fields",
			spec:          "CRON_TZ=Fictional/Country 30 9 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix",
		},
		{
			name:          "invalid schedule with 3 fields and no timezone",
			spec:          "30 9 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "validate weekly schedule: expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix",
		},
		{
			name:          "valid schedule with 5 fields but month and dom not set to *",
			spec:          "30 9 1 1 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "validate weekly schedule: expected day-of-month and month to be *",
		},
		{
			name:          "valid schedule with 5 fields and timezone but month and dom not set to *",
			spec:          "CRON_TZ=Europe/Dublin 30 9 1 1 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "validate weekly schedule: expected day-of-month and month to be *",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := cron.Weekly(testCase.spec)
			if testCase.expectedError == "" {
				nextTime := actual.Next(testCase.at)
				require.NoError(t, err)
				require.Equal(t, testCase.expectedNext, nextTime)
				require.Equal(t, testCase.expectedCron, actual.Cron())
				require.Equal(t, testCase.expectedLocation, actual.Location())
				require.Equal(t, testCase.expectedString, actual.String())
				require.Equal(t, testCase.expectedMin, actual.Min())
				require.Equal(t, testCase.expectedDaysOfWeek, actual.DaysOfWeek())
				require.Equal(t, testCase.expectedTime, actual.Time())
			} else {
				require.EqualError(t, err, testCase.expectedError)
				require.Nil(t, actual)
			}
		})
	}
}

func TestIsWithinRange(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                string
		spec                string
		at                  time.Time
		expectedWithinRange bool
		expectedError       string
	}{
		// "* 9-18 * * 1-5" should be interpreted as a continuous time range from 09:00:00 to 18:59:59, Monday through Friday
		{
			name:                "Right before the start of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 8:59:59 UTC"),
			expectedWithinRange: false,
		},
		{
			name:                "Start of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 9:00:00 UTC"),
			expectedWithinRange: true,
		},
		{
			name:                "9:01 AM - One minute after the start of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 9:01:00 UTC"),
			expectedWithinRange: true,
		},
		{
			name:                "2PM - The middle of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 14:00:00 UTC"),
			expectedWithinRange: true,
		},
		{
			name:                "6PM - One hour before the end of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 18:00:00 UTC"),
			expectedWithinRange: true,
		},
		{
			name:                "End of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 18:59:59 UTC"),
			expectedWithinRange: true,
		},
		{
			name:                "Right after the end of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 19:00:00 UTC"),
			expectedWithinRange: false,
		},
		{
			name:                "7:01PM - One minute after the end of the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 19:01:00 UTC"),
			expectedWithinRange: false,
		},
		{
			name:                "2AM - Significantly outside the time range",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Mon, 02 Jun 2025 02:00:00 UTC"),
			expectedWithinRange: false,
		},
		{
			name:                "Outside the day range #1",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Sat, 07 Jun 2025 14:00:00 UTC"),
			expectedWithinRange: false,
		},
		{
			name:                "Outside the day range #2",
			spec:                "* 9-18 * * 1-5",
			at:                  mustParseTime(t, time.RFC1123, "Sun, 08 Jun 2025 14:00:00 UTC"),
			expectedWithinRange: false,
		},
		{
			name:                "Check that Sunday is supported with value 0",
			spec:                "* 9-18 * * 0",
			at:                  mustParseTime(t, time.RFC1123, "Sun, 08 Jun 2025 14:00:00 UTC"),
			expectedWithinRange: true,
		},
		{
			name:          "Check that value 7 is rejected as out of range",
			spec:          "* 9-18 * * 7",
			at:            mustParseTime(t, time.RFC1123, "Sun, 08 Jun 2025 14:00:00 UTC"),
			expectedError: "end of range (7) above maximum (6): 7",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Weekly(testCase.spec)
			if testCase.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.expectedError)
				return
			}
			require.NoError(t, err)
			withinRange := sched.IsWithinRange(testCase.at)
			require.Equal(t, testCase.expectedWithinRange, withinRange)
		})
	}
}

func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	parsedTime, err := time.Parse(layout, value)
	require.NoError(t, err)
	return parsedTime
}

func mustLocation(t *testing.T, s string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(s)
	require.NoError(t, err)
	return loc
}

func TestStandard(t *testing.T) {
	t.Parallel()

	newYork := mustLocation(t, "America/New_York")
	testCases := []struct {
		name          string
		spec          string
		timeZone      string
		at            time.Time
		expectedNext  time.Time
		expectedError string
	}{
		{
			// Weekly rejects day-of-month and month restrictions.
			name:         "day of month and month",
			spec:         "0 9 15 6 *",
			timeZone:     "UTC",
			at:           time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedNext: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		},
		{
			// 09:00 stays 09:00 wall-clock time across the spring DST change.
			name:         "before spring forward",
			spec:         "0 9 * * *",
			timeZone:     "America/New_York",
			at:           time.Date(2026, 3, 7, 10, 0, 0, 0, newYork),
			expectedNext: time.Date(2026, 3, 8, 9, 0, 0, 0, newYork),
		},
		{
			name:         "after spring forward",
			spec:         "0 9 * * *",
			timeZone:     "America/New_York",
			at:           time.Date(2026, 3, 8, 10, 0, 0, 0, newYork),
			expectedNext: time.Date(2026, 3, 9, 9, 0, 0, 0, newYork),
		},
		{name: "invalid field", spec: "61 9 * * *", timeZone: "UTC", expectedError: "parse schedule"},
		{name: "six fields", spec: "0 0 9 * * *", timeZone: "UTC", expectedError: "exactly 5 fields"},
		{name: "four fields", spec: "0 9 * *", timeZone: "UTC", expectedError: "exactly 5 fields"},
		{name: "cron tz prefix", spec: "CRON_TZ=UTC 0 9 * *", timeZone: "UTC", expectedError: "not supported"},
		{name: "descriptor", spec: "@every 1h", timeZone: "UTC", expectedError: "exactly 5 fields"},
		{name: "missing time zone", spec: "0 9 * * *", timeZone: "", expectedError: "time zone is required"},
		{name: "local time zone", spec: "0 9 * * *", timeZone: "Local", expectedError: "Local is not supported"},
		{name: "unknown time zone", spec: "0 9 * * *", timeZone: "Mars/Olympus", expectedError: "invalid time zone"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(testCase.spec, testCase.timeZone)
			if testCase.expectedError != "" {
				require.ErrorContains(t, err, testCase.expectedError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.timeZone, sched.Location().String())
			require.Equal(t, testCase.spec, sched.Cron())
			require.True(t, testCase.expectedNext.Equal(sched.Next(testCase.at)), "expected %s, got %s", testCase.expectedNext, sched.Next(testCase.at))
		})
	}
}

func TestStandardDaylightSaving(t *testing.T) {
	t.Parallel()

	utc := func(year int, month time.Month, day, hour, minute int) time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	}
	testCases := []struct {
		name     string
		spec     string
		timeZone string
		from     time.Time
		expected []time.Time
	}{
		{
			// 01:30 occurs at 05:30Z (EDT) and 06:30Z (EST) on 2026-11-01.
			name:     "fall back runs a repeated time once",
			spec:     "30 1 * * *",
			timeZone: "America/New_York",
			from:     utc(2026, 10, 31, 12, 0),
			expected: []time.Time{utc(2026, 11, 1, 5, 30), utc(2026, 11, 2, 6, 30)},
		},
		{
			// 06:00Z is 01:00 EST, the second occurrence of 01:00.
			name:     "fall back hourly skips the repeated hour",
			spec:     "0 * * * *",
			timeZone: "America/New_York",
			from:     utc(2026, 11, 1, 3, 30),
			expected: []time.Time{utc(2026, 11, 1, 4, 0), utc(2026, 11, 1, 5, 0), utc(2026, 11, 1, 7, 0), utc(2026, 11, 1, 8, 0)},
		},
		{
			// 02:30 does not exist on 2027-03-14; 03:00 EDT is 07:00Z.
			name:     "spring forward runs a skipped time after the gap",
			spec:     "30 2 * * *",
			timeZone: "America/New_York",
			from:     utc(2027, 3, 13, 12, 0),
			expected: []time.Time{utc(2027, 3, 14, 7, 0), utc(2027, 3, 15, 6, 30)},
		},
		{
			name:     "spring forward merges skipped times into one run",
			spec:     "*/15 2 * * *",
			timeZone: "America/New_York",
			from:     utc(2027, 3, 13, 12, 0),
			expected: []time.Time{utc(2027, 3, 14, 7, 0), utc(2027, 3, 15, 6, 0), utc(2027, 3, 15, 6, 15)},
		},
		{
			name:     "spring forward runs the gap end once",
			spec:     "0 3 * * *",
			timeZone: "America/New_York",
			from:     utc(2027, 3, 13, 12, 0),
			expected: []time.Time{utc(2027, 3, 14, 7, 0), utc(2027, 3, 15, 7, 0)},
		},
		{
			name:     "spring forward merges a skipped time with the gap end",
			spec:     "0 2,3 * * *",
			timeZone: "America/New_York",
			from:     utc(2027, 3, 13, 12, 0),
			expected: []time.Time{utc(2027, 3, 14, 7, 0), utc(2027, 3, 15, 6, 0), utc(2027, 3, 15, 7, 0)},
		},
		{
			// 02:30 occurs at 00:30Z (CEST) and 01:30Z (CET) on 2026-10-25.
			name:     "berlin fall back",
			spec:     "30 2 * * *",
			timeZone: "Europe/Berlin",
			from:     utc(2026, 10, 24, 12, 0),
			expected: []time.Time{utc(2026, 10, 25, 0, 30), utc(2026, 10, 26, 1, 30)},
		},
		{
			// 03:00 CEST is 01:00Z on 2027-03-28.
			name:     "berlin spring forward",
			spec:     "30 2 * * *",
			timeZone: "Europe/Berlin",
			from:     utc(2027, 3, 27, 12, 0),
			expected: []time.Time{utc(2027, 3, 28, 1, 0), utc(2027, 3, 29, 0, 30)},
		},
		{
			// Lord Howe shifts by 30 minutes. 01:45 occurs at +11:00
			// (14:45Z) and +10:30 (15:15Z) on 2027-04-04 local.
			name:     "lord howe fall back",
			spec:     "45 1 * * *",
			timeZone: "Australia/Lord_Howe",
			from:     utc(2027, 4, 3, 0, 0),
			expected: []time.Time{utc(2027, 4, 3, 14, 45), utc(2027, 4, 4, 15, 15)},
		},
		{
			// 02:15 does not exist on 2026-10-04 local; 02:30 +11:00 is
			// 15:30Z.
			name:     "lord howe spring forward",
			spec:     "15 2 * * *",
			timeZone: "Australia/Lord_Howe",
			from:     utc(2026, 10, 3, 0, 0),
			expected: []time.Time{utc(2026, 10, 3, 15, 30), utc(2026, 10, 4, 15, 15)},
		},
		{
			name:     "utc",
			spec:     "0 9 * * *",
			timeZone: "UTC",
			from:     utc(2026, 10, 31, 12, 0),
			expected: []time.Time{utc(2026, 11, 1, 9, 0), utc(2026, 11, 2, 9, 0)},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(testCase.spec, testCase.timeZone)
			require.NoError(t, err)
			got := make([]time.Time, 0, len(testCase.expected))
			next := testCase.from
			for range testCase.expected {
				next = sched.Next(next)
				got = append(got, next.UTC())
			}
			require.Equal(t, testCase.expected, got)
		})
	}
}

func TestStandardNextIsStrictlyIncreasing(t *testing.T) {
	t.Parallel()

	for _, timeZone := range []string{"America/New_York", "Europe/Berlin", "Australia/Lord_Howe", "UTC"} {
		t.Run(timeZone, func(t *testing.T) {
			t.Parallel()
			loc := mustLocation(t, timeZone)
			sched, err := cron.Standard("*/15 * * * *", timeZone)
			require.NoError(t, err)

			// Over a year of runs, every run is after the previous one
			// and no wall time runs twice.
			start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(1, 0, 0)
			walls := make(map[string]time.Time)
			prev := start
			for {
				next := sched.Next(prev)
				require.True(t, next.After(prev), "Next(%s) = %s", prev, next)
				if next.After(end) {
					break
				}
				wall := next.In(loc).Format("2006-01-02 15:04")
				first, ok := walls[wall]
				require.False(t, ok, "wall time %s runs at %s and %s", wall, first, next)
				walls[wall] = next
				prev = next
			}
		})
	}

	// Next(t) is after t when t is inside the repeated hour (either
	// occurrence) or at the end of the gap.
	sched, err := cron.Standard("*/15 * * * *", "America/New_York")
	require.NoError(t, err)
	for _, at := range []time.Time{
		time.Date(2026, 11, 1, 5, 10, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 6, 10, 0, 0, time.UTC),
		time.Date(2027, 3, 14, 6, 59, 59, 0, time.UTC),
		time.Date(2027, 3, 14, 7, 0, 0, 0, time.UTC),
	} {
		require.True(t, sched.Next(at).After(at), "Next(%s) = %s", at, sched.Next(at))
	}
}

func TestShortestInterval(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	testCases := []struct {
		spec     string
		expected time.Duration
		ok       bool
	}{
		{spec: "* * * * *", expected: time.Minute, ok: true},
		{spec: "*/5 * * * *", expected: 5 * time.Minute, ok: true},
		{spec: "0,1 9 * * *", expected: time.Minute, ok: true},
		{spec: "0 9 * * *", expected: 24 * time.Hour, ok: true},
		{spec: "0,59 0,23 * * *", expected: time.Minute, ok: true},
		{spec: "0 9 * * 1", expected: 7 * 24 * time.Hour, ok: true},
		{spec: "0 9 29 2 *", ok: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.spec, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(testCase.spec, "America/New_York")
			require.NoError(t, err)
			got, ok := sched.ShortestInterval(from)
			require.Equal(t, testCase.ok, ok)
			if ok {
				require.Equal(t, testCase.expected, got)
			}
		})
	}
}
