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
		testCase := testCase
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

func mustLocation(t *testing.T, s string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(s)
	require.NoError(t, err)
	return loc
}
