package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:paralleltest // t.Setenv
func TestParseCLISchedule(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		input            []string
		expectedSchedule string
		expectedError    string
		tzEnv            string
	}{
		{
			name:             "TimeAndDayOfWeekAndLocation",
			input:            []string{"09:00AM", "Sun-Sat", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "UTC",
		},
		{
			name:             "TimeOfDay24HourAndDayOfWeekAndLocation",
			input:            []string{"09:00", "Sun-Sat", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "UTC",
		},
		{
			name:             "TimeOfDay24HourAndDayOfWeekAndLocationButItsAllQuoted",
			input:            []string{"09:00 Sun-Sat America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "UTC",
		},
		{
			name:             "TimeOfDayOnly",
			input:            []string{"09:00AM"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "America/Chicago",
		},
		{
			name:             "Time24Military",
			input:            []string{"0900"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "America/Chicago",
		},
		{
			name:             "DayOfWeekAndTime",
			input:            []string{"09:00AM", "Sun-Sat"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "America/Chicago",
		},
		{
			name:             "TimeAndLocation",
			input:            []string{"09:00AM", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "UTC",
		},
		{
			name:             "LazyTime",
			input:            []string{"9am", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "UTC",
		},
		{
			name:             "ZeroPrefixedLazyTime",
			input:            []string{"09am", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "UTC",
		},
		{
			name:          "InvalidTime",
			input:         []string{"nine"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "DayOfWeekAndInvalidTime",
			input:         []string{"nine", "Sun-Sat"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "InvalidTimeAndLocation",
			input:         []string{"nine", "America/Chicago"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "DayOfWeekAndInvalidTimeAndLocation",
			input:         []string{"nine", "Sun-Sat", "America/Chicago"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "TimezoneProvidedInsteadOfLocation",
			input:         []string{"09:00AM", "Sun-Sat", "CST"},
			expectedError: errUnsupportedTimezone.Error(),
		},
		{
			name:          "WhoKnows",
			input:         []string{"Time", "is", "a", "human", "construct"},
			expectedError: errInvalidTimeFormat.Error(),
		},
	} {
		//nolint:paralleltest // t.Setenv
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("TZ", testCase.tzEnv)
			actualSchedule, actualError := parseCLISchedule(testCase.input...)
			if testCase.expectedError != "" {
				assert.Nil(t, actualSchedule)
				assert.ErrorContains(t, actualError, testCase.expectedError)
				return
			}
			assert.NoError(t, actualError)
			if assert.NotEmpty(t, actualSchedule) {
				assert.Equal(t, testCase.expectedSchedule, actualSchedule.String())
			}
		})
	}
}
