package cron_test

import (
	"archive/zip"
	"fmt"
	"go/build"
	"io"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rbcron "github.com/robfig/cron/v3"
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
				name:       "ZoneUnderEtc",
				spec:       "0 9 * * *",
				zone:       "Etc/UTC",
				wantCron:   "0 9 * * *",
				wantString: "CRON_TZ=Etc/UTC 0 9 * * *",
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
			{name: "NeverRunsInShortMonth", spec: "0 0 31 4,6,9,11 *", zone: "UTC", wantErr: "never runs"},
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
			{name: "PathToLocaltimeDoubleSlash", spec: "0 9 * * *", zone: ".//localtime", wantErr: "is not a zone name"},
			{name: "PathToLocaltimeTwoDots", spec: "0 9 * * *", zone: "././localtime", wantErr: "is not a zone name"},
			{name: "PathToPosixrules", spec: "0 9 * * *", zone: "./posixrules", wantErr: "is not a zone name"},
			{name: "PathToFactory", spec: "0 9 * * *", zone: "./Factory", wantErr: "is not a zone name"},
			{name: "PathThroughParent", spec: "0 9 * * *", zone: "Etc/./../localtime", wantErr: "is not a zone name"},
			{name: "PathToZone", spec: "0 9 * * *", zone: "./America/New_York", wantErr: "is not a zone name"},
			{name: "DoubleSlashInZone", spec: "0 9 * * *", zone: "America//New_York", wantErr: "is not a zone name"},
			{name: "DotInZone", spec: "0 9 * * *", zone: "America/./New_York", wantErr: "is not a zone name"},
			{name: "TrailingSlash", spec: "0 9 * * *", zone: "America/New_York/", wantErr: "is not a zone name"},
			{name: "TrailingSlashOnLocal", spec: "0 9 * * *", zone: "Local/", wantErr: "is not a zone name"},
			{name: "AbsolutePath", spec: "0 9 * * *", zone: "/etc/localtime", wantErr: "is not a zone name"},
			{name: "ParentPath", spec: "0 9 * * *", zone: "../zoneinfo/UTC", wantErr: "is not a zone name"},
			{name: "OnlyADot", spec: "0 9 * * *", zone: ".", wantErr: "is not a zone name"},
			// Directories of zone data that some hosts have, with other versions
			// of the zones. The right ones give wrong runs. Rejected on every host.
			{name: "RightZone", spec: "0 9 * * *", zone: "right/America/New_York", wantErr: "is not a zone name"},
			{name: "PosixZone", spec: "0 9 * * *", zone: "posix/America/New_York", wantErr: "is not a zone name"},
			{name: "RightUTC", spec: "0 9 * * *", zone: "right/UTC", wantErr: "is not a zone name"},
			{name: "PosixUTC", spec: "0 9 * * *", zone: "posix/UTC", wantErr: "is not a zone name"},
			{name: "PosixFactory", spec: "0 9 * * *", zone: "posix/Factory", wantErr: "is not a zone name"},
			{name: "RightFactory", spec: "0 9 * * *", zone: "right/Factory", wantErr: "is not a zone name"},
			// The files that are not zones, in any directory.
			{name: "FactoryInADirectory", spec: "0 9 * * *", zone: "Etc/Factory", wantErr: `"Etc/Factory" is not supported`},
			{name: "LocaltimeInADirectory", spec: "0 9 * * *", zone: "Etc/localtime", wantErr: `"Etc/localtime" is not supported`},
			{name: "PosixrulesInADirectory", spec: "0 9 * * *", zone: "Etc/posixrules", wantErr: `"Etc/posixrules" is not supported`},
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

func TestStandard_ErrorHoldsTheZoneNameOnce(t *testing.T) {
	t.Parallel()
	for _, zone := range []string{"Nowhere/Land", strings.Repeat("x", 300), "Etc/Factory", "./localtime", "posix/UTC"} {
		_, err := cron.Standard("0 9 * * *", zone)
		require.Error(t, err)
		require.Equalf(t, 1, strings.Count(err.Error(), zone), "error %q", err)
	}
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
			// Zone data before 1970 needs no special case.
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
			// The old offset of Metlakatla is more than 15 hours, so a search that
			// looks 15 hours around a reading misses it for the last 14 minutes
			// of the repeated day: the first instance of 15:35 on October 19 is
			// 00:21:18Z that day, before the change at 00:31:13Z.
			name: "RepeatedTimeInTheLastMinutesBeforeTheChange",
			spec: "35 15 * * *",
			zone: "America/Metlakatla",
			from: "1867-10-18T00:00:00Z",
			want: []string{"1867-10-18T00:21:18Z", "1867-10-19T00:21:18Z", "1867-10-21T00:21:18Z"},
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
// answer. That pins the bound on the readings that Next walks. Kwajalein
// repeated 23 hours. In 1867 the clocks of Alaska went back exactly 24 hours, so
// a whole day repeated, the longest repeat in the zone data. For a schedule that
// runs every minute the walk visits one reading for each minute of that day, and
// one more when the clock changed at a time with seconds. That is 1,440 readings
// in Sitka, which changed at a whole minute, and 1,441 in Juneau, which changed
// at 15:33:32, as did Adak, Anchorage, Atka, Metlakatla, Nome and Yakutat.
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
		{"NewYorkEveryFiveMinutes", "*/5 * * * *", "America/New_York", "2026-11-01T06:02:00Z", "2026-11-01T07:00:00Z"},
		{"BerlinEveryFiveMinutes", "*/5 * * * *", "Europe/Berlin", "2026-10-25T01:02:00Z", "2026-10-25T02:00:00Z"},
		{"LordHoweEveryMinute", "* * * * *", "Australia/Lord_Howe", "2026-04-04T15:01:00Z", "2026-04-04T15:30:00Z"},
		// Clocks went from UTC+11 to UTC-12 and repeated 23 hours: 1,380 readings.
		{"KwajaleinEveryMinute", "* * * * *", "Pacific/Kwajalein", "1969-09-30T13:00:00Z", "1969-10-01T12:00:00Z"},
		// From the instant of the change, the whole repeated day of 1867-10-19
		// follows. The change reading is 15:30:00, so the walk has 1,440 readings.
		{"SitkaEveryMinute", "* * * * *", "America/Sitka", "1867-10-19T00:31:13Z", "1867-10-20T00:31:13Z"},
		// The same day, but the change reading is 15:33:32: 1,441 readings.
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

// TestStandard_Next_AroundClockChanges compares Next with a minute-by-minute
// reading of the RFC rules, over three days around every clock change of each
// zone and around December 31, in years up to 2100. Past the last change in a
// zone's data, Go computes offsets from a rule instead of a list. The host's
// data reaches that point after 2037 and the data that Go embeds in a binary
// reaches it earlier, so both sources of zone data run.
func TestStandard_Next_AroundClockChanges(t *testing.T) {
	t.Parallel()

	// A spec that restricts the day fields still has the model's times of day.
	specs := []string{
		"*/5 * * * *",
		"0 * * * *",
		"*/20 1 * * *",
		"30 1 * * *",
		"15 3 * * *",
		"45 2 * * *",
		"30 2 * * *",
		"30 3 * * *",
		"0 2,3 * * *",
		"59 23 * * *",
		"0 0 * * *",
		"59 23 31 12 *",
		"0 0 1 1 *",
	}
	zones := []string{"America/New_York", "Europe/Berlin", "Australia/Lord_Howe", "Pacific/Chatham"}
	years := []int{2026, 2028, 2040, 2096, 2100}

	for _, source := range zoneSources() {
		for _, zone := range zones {
			loc := source.load(t, zone)
			for _, year := range years {
				windows := clockChangeWindows(loc, year)
				// Two clock changes, and December 31.
				require.Len(t, windows, 3, "%s %s %d", source.name, zone, year)
				for _, from := range windows {
					t.Run(fmt.Sprintf("%s/%s/%s", source.name, zone, from.Format("2006-01-02")), func(t *testing.T) {
						t.Parallel()
						to := from.Add(72 * time.Hour)
						for _, spec := range specs {
							sched, err := cron.StandardWithLocation(spec, loc)
							require.NoError(t, err)

							want := wantOccurrences(t, spec, loc, from, to)
							var got []time.Time
							// Start just before from, because Next excludes its input.
							for cur := from.Add(-time.Nanosecond); ; {
								next := nextWithin(t, sched, cur)
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
		}
	}
}

// TestStandard_Next_Terminates calls Next from many instants around December
// 31 of every leap year up to 2100, in several zones and both sources of zone
// data. Each call has a deadline, so a Next that never returns fails the test
// and does not hang the run.
func TestStandard_Next_Terminates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		spec    string
		maxWait time.Duration
	}{
		{"0 9 * * *", 25 * time.Hour},
		{"*/15 * * * *", 75 * time.Minute},
		{"59 23 31 12 *", 367 * 24 * time.Hour},
		{"0 0 1 1 *", 367 * 24 * time.Hour},
	}
	zones := []string{
		"America/New_York", "Europe/Berlin", "Australia/Lord_Howe", "Pacific/Chatham", "Asia/Kolkata",
	}
	for _, source := range zoneSources() {
		for _, zone := range zones {
			loc := source.load(t, zone)
			t.Run(source.name+"/"+zone, func(t *testing.T) {
				t.Parallel()
				for _, tc := range testCases {
					sched, err := cron.StandardWithLocation(tc.spec, loc)
					require.NoError(t, err)
					for year := 2028; year <= 2100; year += 4 {
						// Four days, a prime number of minutes apart.
						start := time.Date(year, time.December, 29, 0, 0, 0, 0, time.UTC)
						for from := start; from.Before(start.Add(96 * time.Hour)); from = from.Add(97 * time.Minute) {
							next := nextWithin(t, sched, from)
							require.Truef(t, next.After(from), "%s %s: Next(%s) = %s is not after its input", zone, tc.spec, from, next)
							require.LessOrEqualf(t, next.Sub(from), tc.maxWait, "%s %s: Next(%s) = %s", zone, tc.spec, from, next)
						}
					}
				}
			})
		}
	}

	// The case that hung: the last period of America/New_York ends before the
	// instant on December 31 of a leap year, in Go's rule-based zone data.
	for _, source := range zoneSources() {
		t.Run(source.name+"/FirstReport", func(t *testing.T) {
			t.Parallel()
			sched, err := cron.StandardWithLocation("0 9 * * *", source.load(t, "America/New_York"))
			require.NoError(t, err)
			next := nextWithin(t, sched, time.Date(2040, 12, 30, 15, 0, 0, 0, time.UTC))
			require.Equal(t, time.Date(2040, 12, 31, 14, 0, 0, 0, time.UTC), next)
		})
	}
}

// Weekly and Daily keep the daylight saving behavior of robfig/cron, which
// Standard replaces. This pins them, so that a change that sends them through
// the wall-clock path shows up here.
func TestWeeklyAndDaily_DaylightSaving(t *testing.T) {
	t.Parallel()
	newYork := mustLocation(t, "America/New_York")

	t.Run("WeeklyRunsRepeatedTimeTwice", func(t *testing.T) {
		t.Parallel()
		sched, err := cron.Weekly("CRON_TZ=America/New_York 30 1 * * *")
		require.NoError(t, err)
		from := mustParseTime(t, time.RFC3339, "2026-10-31T12:00:00-04:00").In(newYork)
		require.Equal(t,
			[]string{"2026-11-01T01:30:00-04:00", "2026-11-01T01:30:00-05:00", "2026-11-02T01:30:00-05:00"},
			nextN(t, sched, from, 3))
	})

	t.Run("DailySkipsNonexistentTime", func(t *testing.T) {
		t.Parallel()
		sched, err := cron.Daily("CRON_TZ=America/New_York 30 2 * * *")
		require.NoError(t, err)
		from := mustParseTime(t, time.RFC3339, "2026-03-07T12:00:00-05:00").In(newYork)
		require.Equal(t,
			[]string{"2026-03-09T02:30:00-04:00", "2026-03-10T02:30:00-04:00"},
			nextN(t, sched, from, 2))
	})
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
		{name: "OneTimeOfDay", spec: "0 9 * * *", zone: "UTC", want: 24 * time.Hour},
		{name: "Weekdays", spec: "0 9 * * 1-5", zone: "UTC", want: 24 * time.Hour},
		// Above 24 hours MinInterval says only 24 hours.
		{name: "Weekly", spec: "0 9 * * 1", zone: "UTC", want: 24 * time.Hour},
		{name: "Monthly", spec: "0 0 1 * *", zone: "UTC", want: 24 * time.Hour},
		{name: "LeapDay", spec: "0 0 29 2 *", zone: "UTC", want: 24 * time.Hour},
		{name: "TwoTimesOfDay", spec: "0 9,17 * * *", zone: "UTC", want: 8 * time.Hour},
		{name: "GapAcrossMidnight", spec: "55 23,0 * * *", zone: "UTC", want: time.Hour},
		{name: "CloseTimesOnTheFifteenth", spec: "0,30,32 12 15 * *", zone: "UTC", want: 2 * time.Minute},
		{name: "CloseTimesInFebruary", spec: "0,30,32 12 * 2 *", zone: "UTC", want: 2 * time.Minute},
		// The runs of both are a minute or an hour apart, where the day of
		// month and the day of week fall on neighboring days.
		{name: "DayOfMonthOrDayOfWeek", spec: "0,59 0,23 15 6 0", zone: "UTC", want: time.Minute},
		{name: "DayOfMonthOrDayOfWeekHours", spec: "0 0,23 15 6 0", zone: "UTC", want: time.Hour},
		// Too short: only Mondays run, and the Monday runs are 58 minutes
		// apart at least, but 23:58 and 00:00 are two minutes apart on the
		// clock.
		{name: "OverStrictAcrossMidnight", spec: "0,58 0,23 * * 1", zone: "UTC", want: 2 * time.Minute},
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
}

// TestSchedule_MinInterval_Limits pins the two limits that MinInterval documents,
// with the runs that Next gives.
func TestSchedule_MinInterval_Limits(t *testing.T) {
	t.Parallel()

	t.Run("OffsetChangeShortensAGap", func(t *testing.T) {
		t.Parallel()
		newYork := mustLocation(t, "America/New_York")
		sched, err := cron.Standard("1,58 1,3 * * *", "America/New_York")
		require.NoError(t, err)
		require.Equal(t, 57*time.Minute, sched.MinInterval())

		// Clocks go forward on 2026-03-08, so 01:58 EST and 03:01 EDT are three
		// minutes apart.
		from := mustParseTime(t, time.RFC3339, "2026-03-08T01:30:00-05:00").In(newYork)
		first := sched.Next(from)
		second := sched.Next(first)
		require.Equal(t, time.Date(2026, 3, 8, 6, 58, 0, 0, time.UTC), first.UTC())
		require.Equal(t, time.Date(2026, 3, 8, 7, 1, 0, 0, time.UTC), second.UTC())
		require.Equal(t, 3*time.Minute, second.Sub(first))
		require.Less(t, second.Sub(first), sched.MinInterval())
	})

	t.Run("PairAcrossMidnightWithoutDaysInARow", func(t *testing.T) {
		t.Parallel()
		sched, err := cron.Standard("0,58 0,23 * * 1", "UTC")
		require.NoError(t, err)
		require.Equal(t, 2*time.Minute, sched.MinInterval())

		// Only Mondays run, so 23:58 and 00:00 never happen two minutes apart.
		var shortest time.Duration
		previous := sched.Next(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
		for range 16 {
			next := sched.Next(previous)
			if shortest == 0 || next.Sub(previous) < shortest {
				shortest = next.Sub(previous)
			}
			previous = next
		}
		require.Equal(t, 58*time.Minute, shortest)
		require.Greater(t, shortest, sched.MinInterval())
	})
}

// TestSchedule_MinInterval_NeverLongerThanRunGap checks MinInterval of random
// specs against the runs of a day-by-day model of cron over four years, in UTC,
// which has no offset change: it is never longer than the shortest time between
// two runs, and it is equal to it when every day runs.
func TestSchedule_MinInterval_NeverLongerThanRunGap(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(20261111, 32063)) //nolint:gosec // The same specs on every run.
	list := func(lo, hi int) []int {
		values := make([]int, 1+rng.IntN(3))
		for i := range values {
			values[i] = lo + rng.IntN(hi-lo+1)
		}
		return values
	}
	field := func(star int, values []int) string {
		if rng.IntN(star) == 0 {
			return "*"
		}
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i] = fmt.Sprint(v)
		}
		return strings.Join(parts, ",")
	}

	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	compared, equalWhenEveryDayRuns := 0, 0
	for range 400 {
		minutes, hours := list(0, 59), list(0, 23)
		doms, months, dows := list(1, 31), list(1, 12), list(0, 6)
		spec := strings.Join([]string{
			field(8, minutes), field(4, hours), field(2, doms), field(2, months), field(2, dows),
		}, " ")
		sched, err := cron.Standard(spec, "UTC")
		if err != nil {
			// Not a schedule that runs, such as the 31st of April.
			continue
		}

		timesOfDay := runsOfDay(t, spec)
		daysRun := dayRule(spec)
		var (
			shortest   time.Duration
			lastRun    time.Time
			runs       int
			longestDay = timesOfDay[len(timesOfDay)-1]
		)
		record := func(gap time.Duration) {
			if shortest == 0 || gap < shortest {
				shortest = gap
			}
		}
		for day := first; day.Before(last); day = day.AddDate(0, 0, 1) {
			if !daysRun(day) {
				continue
			}
			if runs == 0 {
				// Every day that runs has the same times, so the gaps inside
				// a day are the same too.
				for i := 1; i < len(timesOfDay); i++ {
					record(timesOfDay[i] - timesOfDay[i-1])
				}
			} else {
				record(day.Add(timesOfDay[0]).Sub(lastRun))
			}
			lastRun = day.Add(longestDay)
			runs += len(timesOfDay)
		}
		if runs < 2 {
			continue
		}
		compared++
		require.LessOrEqualf(t, sched.MinInterval(), shortest, "spec %q", spec)
		fields := strings.Fields(spec)
		if fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
			require.Equalf(t, shortest, sched.MinInterval(), "spec %q", spec)
			equalWhenEveryDayRuns++
		}
	}
	// The generator must cover many specs, and many that run every day.
	require.Greater(t, compared, 250)
	require.Greater(t, equalWhenEveryDayRuns, 10)
}

// zoneSource loads time zones from one source of zone data.
type zoneSource struct {
	name string
	load func(t *testing.T, name string) *time.Location
}

// zoneSources returns the zone data of the host and the data that Go embeds in
// a binary that imports time/tzdata, as cmd/coder does. The embedded data is
// the smaller kind: it lists fewer changes and gives a rule for later years.
func zoneSources() []zoneSource {
	return []zoneSource{
		{name: "host", load: func(t *testing.T, name string) *time.Location {
			t.Helper()
			return mustLocation(t, name)
		}},
		{name: "embedded", load: func(t *testing.T, name string) *time.Location {
			t.Helper()
			archive, err := zip.OpenReader(filepath.Join(build.Default.GOROOT, "lib", "time", "zoneinfo.zip"))
			require.NoError(t, err)
			defer archive.Close()
			file, err := archive.Open(name)
			require.NoError(t, err)
			defer file.Close()
			data, err := io.ReadAll(file)
			require.NoError(t, err)
			loc, err := time.LoadLocationFromTZData(name, data)
			require.NoError(t, err)
			return loc
		}},
	}
}

// clockChangeWindows returns the start of a three-day window around each clock
// change of loc in year, and of one around December 31.
func clockChangeWindows(loc *time.Location, year int) []time.Time {
	var windows []time.Time
	for day := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC); day.Year() == year; day = day.AddDate(0, 0, 1) {
		_, today := day.In(loc).Zone()
		_, tomorrow := day.AddDate(0, 0, 1).In(loc).Zone()
		if today != tomorrow {
			windows = append(windows, day.Add(-24*time.Hour))
		}
	}
	return append(windows, time.Date(year, time.December, 30, 0, 0, 0, 0, time.UTC))
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

// wantOccurrences returns the runs of spec in loc within [from, to), found by
// reading the RFC rules minute by minute. from must not be inside a repeated
// period.
func wantOccurrences(t *testing.T, spec string, loc *time.Location, from, to time.Time) []time.Time {
	t.Helper()

	// UTC has no clock changes, so a UTC schedule lists the wall-clock
	// readings that match the fields. It comes from robfig/cron, not from the
	// code under test.
	utc := parseUTC(t, spec)
	matching := make(map[int64]bool)
	for reading := utc.Next(from.Add(-48 * time.Hour)); reading.Before(to.Add(48 * time.Hour)); reading = utc.Next(reading) {
		matching[reading.Unix()] = true
	}

	var (
		want     []time.Time
		seen     = make(map[int64]bool)
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
				if matching[skipped.Unix()] {
					add(instant)
				}
			}
		}
		// A wall-clock minute that already happened is a repeat.
		if !seen[wall.Unix()] && matching[wall.Unix()] {
			add(instant)
		}
		seen[wall.Unix()] = true
		prevWall = wall
	}
	return want
}

// parseUTC parses a five-field spec with robfig/cron, in UTC.
func parseUTC(t *testing.T, spec string) rbcron.Schedule {
	t.Helper()
	parser := rbcron.NewParser(rbcron.Minute | rbcron.Hour | rbcron.Dom | rbcron.Month | rbcron.Dow)
	sched, err := parser.Parse("CRON_TZ=UTC " + spec)
	require.NoError(t, err)
	return sched
}

// runsOfDay returns the times of day, in order, at which spec runs on a day
// that it runs.
func runsOfDay(t *testing.T, spec string) []time.Duration {
	t.Helper()
	fields := strings.Fields(spec)
	every := parseUTC(t, strings.Join([]string{fields[0], fields[1], "*", "*", "*"}, " "))
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var times []time.Duration
	for run := every.Next(day.Add(-time.Second)); run.Before(day.AddDate(0, 0, 1)); run = every.Next(run) {
		times = append(times, run.Sub(day))
	}
	return times
}

// dayRule returns a function that reports whether spec runs on a day, by the
// rule of cron: the month must match, and the day of month and day of week must
// both match, or either when both fields are restricted.
func dayRule(spec string) func(day time.Time) bool {
	fields := strings.Fields(spec)
	set := func(field string) map[int]bool {
		values := make(map[int]bool)
		for _, part := range strings.Split(field, ",") {
			var value int
			_, err := fmt.Sscan(part, &value)
			if err == nil {
				values[value] = true
			}
		}
		return values
	}
	var (
		months, doms, dows = set(fields[3]), set(fields[2]), set(fields[4])
		anyMonth           = fields[3] == "*"
		anyDom, anyDow     = fields[2] == "*", fields[4] == "*"
	)
	return func(day time.Time) bool {
		if !anyMonth && !months[int(day.Month())] {
			return false
		}
		dom := anyDom || doms[day.Day()]
		dow := anyDow || dows[int(day.Weekday())]
		if anyDom || anyDow {
			return dom && dow
		}
		return dom || dow
	}
}
