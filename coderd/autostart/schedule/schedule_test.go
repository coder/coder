package schedule_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/autostart/schedule"
)

func Test_Weekly(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		spec          string
		at            time.Time
		expectedNext  time.Time
		expectedError string
	}{
		{
			name:          "with timezone",
			spec:          "CRON_TZ=US/Central 30 9 * * 1-5",
			at:            time.Date(2022, 4, 1, 14, 29, 0, 0, time.UTC),
			expectedNext:  time.Date(2022, 4, 1, 14, 30, 0, 0, time.UTC),
			expectedError: "",
		},
		{
			name:          "without timezone",
			spec:          "30 9 * * 1-5",
			at:            time.Date(2022, 4, 1, 9, 29, 0, 0, time.Local),
			expectedNext:  time.Date(2022, 4, 1, 9, 30, 0, 0, time.Local),
			expectedError: "",
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
			expectedError: "validate weekly schedule: expected month and dom to be *",
		},
		{
			name:          "valid schedule with 5 fields and timezone but month and dom not set to *",
			spec:          "CRON_TZ=Europe/Dublin 30 9 1 1 1-5",
			at:            time.Time{},
			expectedNext:  time.Time{},
			expectedError: "validate weekly schedule: expected month and dom to be *",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := schedule.Weekly(testCase.spec)
			if testCase.expectedError == "" {
				nextTime := actual.Next(testCase.at)
				require.NoError(t, err)
				require.Equal(t, testCase.expectedNext, nextTime)
				require.Equal(t, testCase.spec, actual.String())
			} else {
				require.EqualError(t, err, testCase.expectedError)
				require.Nil(t, actual)
			}
		})
	}
}
