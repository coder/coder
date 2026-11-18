package cron_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/testutil"
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
			{name: "BlankSpec", spec: "   ", zone: "UTC", wantErr: "empty"},
			{name: "FourFields", spec: "* * * *", zone: "UTC", wantErr: "expected 5 fields, got 4"},
			{name: "SixFields", spec: "0 * * * * *", zone: "UTC", wantErr: "expected 5 fields, got 6"},
			{name: "Descriptor", spec: "@daily", zone: "UTC", wantErr: "expected 5 fields, got 1"},
			{name: "CronTZPrefix", spec: "CRON_TZ=UTC 0 9 * * *", zone: "UTC", wantErr: "time zone"},
			{name: "TZPrefix", spec: "TZ=UTC 0 9 * * *", zone: "UTC", wantErr: "time zone"},
			{name: "NotANumber", spec: "a * * * *", zone: "UTC", wantErr: "parse"},
			{name: "NeverRuns", spec: "0 0 30 2 *", zone: "UTC", wantErr: "never runs"},
			{name: "EmptyZone", spec: "0 9 * * *", zone: "", wantErr: "time zone is empty"},
			{name: "UnknownZone", spec: "0 9 * * *", zone: "Nowhere/Land", wantErr: "unknown time zone"},
			// These names load as zones on hosts that have the files, and what
			// they hold depends on the host.
			{name: "LocalZone", spec: "0 9 * * *", zone: "Local", wantErr: `"Local" is not supported`},
			{name: "LocaltimeFile", spec: "0 9 * * *", zone: "localtime", wantErr: `"localtime" is not supported`},
			{name: "PosixrulesFile", spec: "0 9 * * *", zone: "posixrules", wantErr: `"posixrules" is not supported`},
			{name: "FactoryZone", spec: "0 9 * * *", zone: "Factory", wantErr: `"Factory" is not supported`},
			// time.LoadLocation cleans a path, so these reach the same files and
			// zones as the names above and below, under other names.
			{name: "PathToLocaltime", spec: "0 9 * * *", zone: "./localtime", wantErr: "is not a zone name"},
			{name: "DoubleSlashInZone", spec: "0 9 * * *", zone: "America//New_York", wantErr: "is not a zone name"},
			{name: "ParentPath", spec: "0 9 * * *", zone: "../zoneinfo/UTC", wantErr: "is not a zone name"},
			// Directories of zone data that some hosts have, with other versions
			// of the zones. The right ones give wrong runs. Rejected on every host.
			{name: "RightZone", spec: "0 9 * * *", zone: "right/America/New_York", wantErr: "is not a zone name"},
			{name: "PosixZone", spec: "0 9 * * *", zone: "posix/America/New_York", wantErr: "is not a zone name"},
			// The files that are not zones, in any directory.
			{name: "LocaltimeInADirectory", spec: "0 9 * * *", zone: "Etc/localtime", wantErr: `"Etc/localtime" is not supported`},
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
			name: "NewYorkFall/FromInsideRepeatedPeriodSecondInstance",
			spec: "30 1 * * *",
			zone: "America/New_York",
			from: "2026-11-01T01:10:00-05:00",
			want: []string{"2026-11-02T01:30:00-05:00"},
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
		// The Chatham Islands are at UTC+12:45 and UTC+13:45, east of every
		// other zone in use. Clocks go forward on 2026-09-27 (02:45 to 03:45)
		// and back on 2026-04-05 (03:45 to 02:45).
		{
			name: "ChathamSpring/NonexistentTimeRunsAfterChange",
			spec: "30 3 * * *",
			zone: "Pacific/Chatham",
			from: "2026-09-26T12:00:00+12:45",
			want: []string{"2026-09-27T03:45:00+13:45", "2026-09-28T03:30:00+13:45"},
		},
		{
			name: "ChathamFall/RepeatedTimeRunsAtFirstInstance",
			spec: "15 3 * * *",
			zone: "Pacific/Chatham",
			from: "2026-04-04T12:00:00+13:45",
			want: []string{"2026-04-05T03:15:00+13:45", "2026-04-06T03:15:00+12:45"},
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

// TestStandard_Next_InsideRepeatedPeriod calls Next from the second pass of a
// repeated period. Each reading that follows resolves to an instant of the first
// pass, which is before the call, so Next walks all of them before it finds the
// answer. That pins the bound on the readings that Next walks. In 1867 the
// clocks of Alaska went back exactly 24 hours, so a whole day repeated, the
// longest repeat in the zone data. For a schedule that runs every minute the
// walk visits one reading for each minute of that day, and one more when the
// clock changed at a time with seconds. That is 1,441 readings in Juneau, which
// changed at 15:33:32.
func TestStandard_Next_InsideRepeatedPeriod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		spec string
		zone string
		from string
		want string
	}{
		// 01:02 EST, in the second pass of 01:00 to 01:59 on 2026-11-01.
		{"NewYorkEveryMinute", "* * * * *", "America/New_York", "2026-11-01T06:02:00Z", "2026-11-01T07:00:00Z"},
		// From the instant of the change, the whole repeated day of 1867-10-19
		// follows. The change reading is 15:33:32, so the walk has 1,441 readings.
		{"JuneauEveryMinute", "* * * * *", "America/Juneau", "1867-10-19T00:31:13Z", "1867-10-20T00:31:41Z"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(tc.spec, tc.zone)
			require.NoError(t, err)
			got := nextWithin(t, sched, mustParseTime(t, time.RFC3339, tc.from))
			require.Equal(t, tc.want, got.UTC().Format(time.RFC3339))
		})
	}
}

// TestStandard_Next_Terminates is the case that hung: the last period of
// America/New_York ends before the instant on December 31 of a leap year, in
// Go's rule-based zone data. The call has a deadline, so a Next that never
// returns fails the test and does not hang the run.
func TestStandard_Next_Terminates(t *testing.T) {
	t.Parallel()

	sched, err := cron.Standard("0 9 * * *", "America/New_York")
	require.NoError(t, err)
	next := nextWithin(t, sched, time.Date(2040, 12, 30, 15, 0, 0, 0, time.UTC))
	require.Equal(t, time.Date(2040, 12, 31, 14, 0, 0, 0, time.UTC), next)
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
		{name: "Hourly", spec: "0 * * * *", zone: "UTC", want: time.Hour},
		{name: "OneTimeOfDay", spec: "0 9 * * *", zone: "UTC", want: 24 * time.Hour},
		// Above 24 hours MinInterval says only 24 hours.
		{name: "Weekly", spec: "0 9 * * 1", zone: "UTC", want: 24 * time.Hour},
		{name: "TwoTimesOfDay", spec: "0 9,17 * * *", zone: "UTC", want: 8 * time.Hour},
		{name: "GapAcrossMidnight", spec: "55 23,0 * * *", zone: "UTC", want: time.Hour},
		{name: "CloseTimesOnTheFifteenth", spec: "0,30,32 12 15 * *", zone: "UTC", want: 2 * time.Minute},
		// The runs are a minute apart, where the day of month and the day of
		// week fall on neighboring days.
		{name: "DayOfMonthOrDayOfWeek", spec: "0,59 0,23 15 6 0", zone: "UTC", want: time.Minute},
		// Too short: only Mondays run, and the Monday runs are 58 minutes
		// apart at least, but 23:58 and 00:00 are two minutes apart on the
		// clock.
		{name: "OverStrictAcrossMidnight", spec: "0,58 0,23 * * 1", zone: "UTC", want: 2 * time.Minute},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sched, err := cron.Standard(tc.spec, tc.zone)
			require.NoError(t, err)
			require.Equal(t, tc.want, sched.MinInterval())
		})
	}
}

// nextWithin returns sched.Next(from), and fails the test when that takes
// longer than testutil.WaitShort.
func nextWithin(t *testing.T, sched *cron.Schedule, from time.Time) time.Time {
	t.Helper()
	next := make(chan time.Time, 1)
	go func() { next <- sched.Next(from) }()
	return testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, next)
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
