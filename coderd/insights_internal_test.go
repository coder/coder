package coderd

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func Test_parseInsightsStartAndEndTime(t *testing.T) {
	t.Parallel()

	layout := insightsTimeLayout
	now := time.Now().UTC()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	thisHour := time.Date(y, m, d, now.Hour(), 0, 0, 0, time.UTC)
	thisHourRoundUp := thisHour.Add(time.Hour)

	helsinki, err := time.LoadLocation("Europe/Helsinki")
	require.NoError(t, err)

	type args struct {
		startTime string
		endTime   string
	}
	tests := []struct {
		name          string
		args          args
		wantStartTime time.Time
		wantEndTime   time.Time
		wantOk        bool
	}{
		{
			name: "Week",
			args: args{
				startTime: "2023-07-10T00:00:00Z",
				endTime:   "2023-07-17T00:00:00Z",
			},
			wantStartTime: time.Date(2023, 7, 10, 0, 0, 0, 0, time.UTC),
			wantEndTime:   time.Date(2023, 7, 17, 0, 0, 0, 0, time.UTC),
			wantOk:        true,
		},
		{
			name: "Today",
			args: args{
				startTime: today.Format(layout),
				endTime:   thisHour.Format(layout),
			},
			wantStartTime: time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC),
			wantEndTime:   time.Date(today.Year(), today.Month(), today.Day(), thisHour.Hour(), 0, 0, 0, time.UTC),
			wantOk:        true,
		},
		{
			name: "Today with minutes and seconds",
			args: args{
				startTime: today.Format(layout),
				endTime:   thisHour.Add(time.Minute + time.Second).Format(layout),
			},
			wantOk: false,
		},
		{
			name: "Today (hour round up)",
			args: args{
				startTime: today.Format(layout),
				endTime:   thisHourRoundUp.Format(layout),
			},
			wantStartTime: time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC),
			wantEndTime:   time.Date(today.Year(), today.Month(), thisHourRoundUp.Day(), thisHourRoundUp.Hour(), 0, 0, 0, time.UTC),
			wantOk:        true,
		},
		{
			name: "Other timezone week",
			args: args{
				startTime: "2023-07-10T00:00:00+03:00",
				endTime:   "2023-07-17T00:00:00+03:00",
			},
			wantStartTime: time.Date(2023, 7, 10, 0, 0, 0, 0, helsinki),
			wantEndTime:   time.Date(2023, 7, 17, 0, 0, 0, 0, helsinki),
			wantOk:        true,
		},
		{
			name: "Daylight savings time",
			args: args{
				startTime: "2023-03-26T00:00:00+02:00",
				endTime:   "2023-03-27T00:00:00+03:00",
			},
			wantStartTime: time.Date(2023, 3, 26, 0, 0, 0, 0, helsinki),
			wantEndTime:   time.Date(2023, 3, 27, 0, 0, 0, 0, helsinki),
			wantOk:        true,
		},
		{
			name: "Bad format",
			args: args{
				startTime: "2023-07-10",
				endTime:   "2023-07-17",
			},
			wantOk: false,
		},
		{
			name: "Zero time",
			args: args{
				startTime: (time.Time{}).Format(layout),
				endTime:   (time.Time{}).Format(layout),
			},
			wantOk: false,
		},
		{
			name: "Time in future",
			args: args{
				startTime: today.AddDate(0, 0, 1).Format(layout),
				endTime:   today.AddDate(0, 0, 2).Format(layout),
			},
			wantOk: false,
		},
		{
			name: "End before start",
			args: args{
				startTime: today.Format(layout),
				endTime:   today.AddDate(0, 0, -1).Format(layout),
			},
			wantOk: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			gotStartTime, gotEndTime, gotOk := parseInsightsStartAndEndTime(context.Background(), rw, tt.args.startTime, tt.args.endTime)

			if !assert.Equal(t, tt.wantOk, gotOk) {
				//nolint:bodyclose
				t.Log("Status: ", rw.Result().StatusCode)
				t.Log("Body: ", rw.Body.String())
			}
			// assert.Equal is unable to test time equality with different
			// (but same) locations because the *time.Location names differ
			// between LoadLocation and Parse, so we use assert.WithinDuration.
			assert.WithinDuration(t, tt.wantStartTime, gotStartTime, 0)
			assert.True(t, tt.wantStartTime.Equal(gotStartTime))
			assert.WithinDuration(t, tt.wantEndTime, gotEndTime, 0)
			assert.True(t, tt.wantEndTime.Equal(gotEndTime))
		})
	}
}

func Test_parseInsightsInterval_week(t *testing.T) {
	t.Parallel()

	layout := insightsTimeLayout
	sydneyLoc, err := time.LoadLocation("Australia/Sydney") // Random location
	require.NoError(t, err)

	now := time.Now()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, sydneyLoc)

	thisHour := time.Date(y, m, d, now.Hour(), 0, 0, 0, sydneyLoc)
	twoHoursAgo := thisHour.Add(-2 * time.Hour)
	thirteenDaysAgo := today.AddDate(0, 0, -13)

	sixDaysAgo := today.AddDate(0, 0, -6)
	nineDaysAgo := today.AddDate(0, 0, -9)

	type args struct {
		startTime string
		endTime   string
	}
	tests := []struct {
		name   string
		args   args
		wantOk bool
	}{
		{
			name: "Two full weeks",
			args: args{
				startTime: "2023-08-10T00:00:00+02:00",
				endTime:   "2023-08-24T00:00:00+02:00",
			},
			wantOk: true,
		},
		{
			name: "Two weeks",
			args: args{
				startTime: thirteenDaysAgo.Format(layout),
				endTime:   twoHoursAgo.Format(layout),
			},
			wantOk: true,
		},
		{
			name: "One full week",
			args: args{
				startTime: "2023-09-06T00:00:00+02:00",
				endTime:   "2023-09-13T00:00:00+02:00",
			},
			wantOk: true,
		},
		{
			name: "6 days are acceptable",
			args: args{
				startTime: sixDaysAgo.Format(layout),
				endTime:   thisHour.Format(layout),
			},
			wantOk: true,
		},
		{
			name: "Shorter than a full week",
			args: args{
				startTime: "2023-09-08T00:00:00+02:00",
				endTime:   "2023-09-13T00:00:00+02:00",
			},
			wantOk: false,
		},
		{
			name: "9 days (7 + 2) are not acceptable",
			args: args{
				startTime: nineDaysAgo.Format(layout),
				endTime:   thisHour.Format(layout),
			},
			wantOk: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			startTime, endTime, ok := parseInsightsStartAndEndTime(context.Background(), rw, tt.args.startTime, tt.args.endTime)
			if !ok {
				//nolint:bodyclose
				t.Log("Status: ", rw.Result().StatusCode)
				t.Log("Body: ", rw.Body.String())
			}
			require.True(t, ok, "start_time and end_time must be valid")

			parsedInterval, gotOk := parseInsightsInterval(context.Background(), rw, "week", startTime, endTime)
			if !assert.Equal(t, tt.wantOk, gotOk) {
				//nolint:bodyclose
				t.Log("Status: ", rw.Result().StatusCode)
				t.Log("Body: ", rw.Body.String())
			}
			if tt.wantOk {
				assert.Equal(t, codersdk.InsightsReportIntervalWeek, parsedInterval)
			}
		})
	}
}

func TestLastReportIntervalHasAtLeastSixDays(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Europe/Warsaw")
	require.NoError(t, err)

	testCases := []struct {
		name      string
		startTime time.Time
		endTime   time.Time
		expected  bool
	}{
		{
			name:      "perfectly full week",
			startTime: time.Date(2023, time.September, 11, 12, 0, 0, 0, loc),
			endTime:   time.Date(2023, time.September, 18, 12, 0, 0, 0, loc),
			expected:  true,
		},
		{
			name:      "exactly 6 days apart",
			startTime: time.Date(2023, time.September, 11, 12, 0, 0, 0, loc),
			endTime:   time.Date(2023, time.September, 17, 12, 0, 0, 0, loc),
			expected:  true,
		},
		{
			name:      "less than 6 days apart",
			startTime: time.Date(2023, time.September, 11, 12, 0, 0, 0, time.UTC),
			endTime:   time.Date(2023, time.September, 17, 11, 0, 0, 0, time.UTC),
			expected:  false,
		},
		{
			name:      "forward DST change, 5 days and 23 hours apart",
			startTime: time.Date(2023, time.March, 22, 12, 0, 0, 0, loc), // A day before DST starts
			endTime:   time.Date(2023, time.March, 28, 12, 0, 0, 0, loc), // Exactly 6 "days" apart
			expected:  true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := lastReportIntervalHasAtLeastSixDays(tc.startTime, tc.endTime)
			if result != tc.expected {
				t.Errorf("Expected %v, but got %v for start time %v and end time %v", tc.expected, result, tc.startTime, tc.endTime)
			}
		})
	}
}
